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

// compile-time guard that context is imported (Run added in the next task).
var _ = context.Background
