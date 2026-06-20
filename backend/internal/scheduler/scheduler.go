// Package scheduler runs a callback once per day at a configured local time.
// The Clock is injectable so the daily fire is deterministic in tests.
package scheduler

import (
	"context"
	"time"
)

// Clock is the injectable time source (deterministic in tests).
type Clock interface {
	Now() time.Time
	// NewTimer returns a channel that fires after d plus a stop func.
	NewTimer(d time.Duration) (<-chan time.Time, func() bool)
}

// RealClock backs the production scheduler with the time package.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// NewTimer wraps time.NewTimer.
func (RealClock) NewTimer(d time.Duration) (<-chan time.Time, func() bool) {
	t := time.NewTimer(d)
	return t.C, t.Stop
}

// Config is the daily schedule: fire at Hour:Minute in Loc, once per local day.
type Config struct {
	Hour, Minute int
	Loc          *time.Location
}

// nextFire returns the next instant strictly after `from` at cfg.Hour:cfg.Minute
// in cfg.Loc. If today's time already passed (or equals from), returns tomorrow's.
// AddDate preserves the wall clock across DST.
func nextFire(from time.Time, cfg Config) time.Time {
	now := from.In(cfg.Loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), cfg.Hour, cfg.Minute, 0, 0, cfg.Loc)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// ConfigProvider re-reads the live schedule (from athlete_profile in production)
// on every loop iteration. It returns the resolved Config (HH:MM + IANA tz already
// parsed by the caller), whether the agent is enabled, and any error.
type ConfigProvider func() (cfg Config, enabled bool, err error)

// errRetry is how long Run waits before retrying after a ConfigProvider error.
const errRetry = time.Minute

// Run blocks until ctx is cancelled, invoking fn once per scheduled local day.
// fn receives ctx and the local date (YYYY-MM-DD) it fired for. The schedule is
// re-read via `next` on EVERY iteration, so changing daily_run_time/timezone
// recomputes the next fire on the following cycle and toggling agent_enabled=false
// suppresses fn WITHOUT a restart. An in-process guard (lastFired) prevents
// same-process double fires; the PERSISTENT once-per-day guard is owned by fn
// (agent.RunDaily checks agent_runs).
func Run(ctx context.Context, clk Clock, next ConfigProvider, fn func(ctx context.Context, localDate string)) {
	var lastFired string
	for {
		cfg, enabled, err := next()
		if err != nil {
			// Can't read the schedule; wait a bit and retry rather than spin.
			c, stop := clk.NewTimer(errRetry)
			select {
			case <-ctx.Done():
				stop()
				return
			case <-c:
				continue
			}
		}

		fireAt := nextFire(clk.Now(), cfg)
		d := fireAt.Sub(clk.Now())
		if d < 0 {
			d = 0
		}
		c, stop := clk.NewTimer(d)
		select {
		case <-ctx.Done():
			stop()
			return
		case <-c:
			if !enabled {
				// Agent disabled in the profile: skip this fire, re-read next cycle.
				continue
			}
			localDate := clk.Now().In(cfg.Loc).Format("2006-01-02")
			if localDate != lastFired {
				lastFired = localDate
				fn(ctx, localDate)
			}
		}
	}
}
