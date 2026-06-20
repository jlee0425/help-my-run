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
		Run(ctx, clk, staticProvider(cfg, true), func(_ context.Context, d string) {
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

func TestRunDisabledDoesNotFire(t *testing.T) {
	utc := time.UTC
	clk := &fakeClock{now: time.Date(2026, 6, 20, 3, 0, 0, 0, utc), ch: make(chan time.Time)}
	cfg := Config{Hour: 5, Minute: 30, Loc: utc}

	var mu sync.Mutex
	fired := false
	// provider reports agent_enabled=false; the timer fires but fn must NOT run.
	prov := func() (Config, bool, error) { return cfg, false, nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, clk, prov, func(context.Context, string) {
			mu.Lock()
			fired = true
			mu.Unlock()
		})
		close(done)
	}()

	clk.setNow(time.Date(2026, 6, 20, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now() // delivers the fire; disabled provider must suppress fn
	clk.setNow(time.Date(2026, 6, 21, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now() // next cycle, still disabled

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if fired {
		t.Fatal("fn fired while agent_enabled=false, want suppressed")
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
		Run(ctx, clk, prov, func(context.Context, string) {})
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
		Run(ctx, clk, staticProvider(cfg, true), func(context.Context, string) {})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
