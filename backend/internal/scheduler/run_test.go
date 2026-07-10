package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock is a controllable Clock: tests drive virtual time by sending on ch.
type fakeClock struct {
	mu   sync.Mutex
	now  time.Time
	ch   chan time.Time
	last time.Duration
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) setNow(t time.Time) {
	f.mu.Lock()
	f.now = t
	f.mu.Unlock()
}

func (f *fakeClock) NewTimer(d time.Duration) (<-chan time.Time, func() bool) {
	f.mu.Lock()
	f.last = d
	f.mu.Unlock()
	return f.ch, func() bool { return true }
}

// staticProvider returns a fixed Config+enabled (no DB) for tests that don't
// exercise live re-reads.
func staticProvider(cfg Config, enabled bool) ConfigProvider {
	return func() (Config, bool, error) { return cfg, enabled, nil }
}

func TestRunFiresOncePerDay(t *testing.T) {
	utc := time.UTC
	start := time.Date(2026, 6, 20, 3, 0, 0, 0, utc) // before T=05:30
	clk := &fakeClock{now: start, ch: make(chan time.Time)}
	cfg := Config{Hour: 5, Minute: 30, Loc: utc}

	var mu sync.Mutex
	var fires []string
	step := make(chan struct{}, 8)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, clk, staticProvider(cfg, true), func(_ context.Context, d string, _ bool) {
			mu.Lock()
			fires = append(fires, d)
			mu.Unlock()
			step <- struct{}{}
		})
		close(done)
	}()

	clk.setNow(time.Date(2026, 6, 20, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now()
	<-step // wait for fn to record 06-20

	clk.ch <- clk.Now() // SAME day: ignored by in-process guard (no step)

	clk.setNow(time.Date(2026, 6, 21, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now()
	<-step // wait for fn to record 06-21

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(fires) != 2 || fires[0] != "2026-06-20" || fires[1] != "2026-06-21" {
		t.Fatalf("fires = %v, want [2026-06-20 2026-06-21]", fires)
	}
}

// M6: the nightly backup rides the scheduler callback, so fn must STILL fire
// on schedule when the agent is disabled — with enabled=false so fn can skip
// just the agent.
func TestRunDisabledStillFiresWithEnabledFalse(t *testing.T) {
	utc := time.UTC
	clk := &fakeClock{now: time.Date(2026, 6, 20, 3, 0, 0, 0, utc), ch: make(chan time.Time)}
	cfg := Config{Hour: 5, Minute: 30, Loc: utc}

	var mu sync.Mutex
	var enableds []bool
	step := make(chan struct{}, 8)
	prov := func() (Config, bool, error) { return cfg, false, nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, clk, prov, func(_ context.Context, _ string, enabled bool) {
			mu.Lock()
			enableds = append(enableds, enabled)
			mu.Unlock()
			step <- struct{}{}
		})
		close(done)
	}()

	clk.setNow(time.Date(2026, 6, 20, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now()
	<-step // day 1 fires despite disabled
	clk.setNow(time.Date(2026, 6, 21, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now()
	<-step // day 2 fires despite disabled

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(enableds) != 2 || enableds[0] || enableds[1] {
		t.Fatalf("fires(enabled) = %v, want [false false] (fn always fires, agent skip is fn's job)", enableds)
	}
}

func TestRunRecomputesNextFireWhenTimeChanges(t *testing.T) {
	utc := time.UTC
	clk := &fakeClock{now: time.Date(2026, 6, 20, 3, 0, 0, 0, utc), ch: make(chan time.Time)}

	// provider returns 05:30 first, then 07:00 — Run must recompute the durations.
	var mu sync.Mutex
	call := 0
	prov := func() (Config, bool, error) {
		mu.Lock()
		defer mu.Unlock()
		call++
		if call == 1 {
			return Config{Hour: 5, Minute: 30, Loc: utc}, true, nil
		}
		return Config{Hour: 7, Minute: 0, Loc: utc}, true, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, clk, prov, func(context.Context, string, bool) {})
		close(done)
	}()

	// First iteration is scheduled for 05:30 (from 03:00 -> 05:30 = 2h30m). Drive
	// that fire so Run loops back and re-reads the provider (which now returns 07:00).
	clk.setNow(time.Date(2026, 6, 20, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now()

	// After re-reading the 07:00 schedule, the next timer is 05:30 -> 07:00 = 1h30m.
	// Poll until that recomputed duration is observed (proves the live re-read).
	deadline := time.Now().Add(2 * time.Second)
	for {
		clk.mu.Lock()
		last := clk.last
		clk.mu.Unlock()
		if last == 90*time.Minute {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("next fire not recomputed for 07:00; last timer = %v, want 1h30m", last)
		}
		time.Sleep(2 * time.Millisecond)
	}

	cancel()
	<-done
}

func TestRunStopsOnContextCancel(t *testing.T) {
	utc := time.UTC
	clk := &fakeClock{now: time.Date(2026, 6, 20, 3, 0, 0, 0, utc), ch: make(chan time.Time)}
	cfg := Config{Hour: 5, Minute: 30, Loc: utc}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, clk, staticProvider(cfg, true), func(context.Context, string, bool) {})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
