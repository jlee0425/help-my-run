package sync

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

func TestSyncGarminUpsertsActivitiesAndRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)

	fixture, err := filepath.Abs(filepath.Join("..", "garmin", "testdata", "worker_output.json"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	script := filepath.Join(t.TempDir(), "stub.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat '"+fixture+"'\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	res := SyncGarmin(context.Background(), s, r, nil)
	if res.Status != "ok" || res.Error != nil {
		t.Fatalf("result = %+v, want ok", res)
	}
	// Fixture: 2 sleep + 1 hrv + 2 bb + 2 rhr + 2 vo2max + 2 activities = 11 upserts.
	if res.Synced != 11 {
		t.Errorf("synced = %d, want 11", res.Synced)
	}

	// Activities now land in the canonical `activities` table (M4 re-key).
	var nAct int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM activities`).Scan(&nAct); err != nil {
		t.Fatalf("count activities: %v", err)
	}
	if nAct != 2 {
		t.Errorf("activities = %d, want 2 (Garmin-ingested)", nAct)
	}
	a, err := s.GetActivity(14820001234)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if a.Type != "running" {
		t.Errorf("activity type = %q, want running", a.Type)
	}

	// raw_json persisted from the worker for recovery.
	var raw string
	_ = s.DB.QueryRow(`SELECT raw_json FROM garmin_sleep WHERE date='2026-06-14'`).Scan(&raw)
	if !strings.Contains(raw, "dailySleepDTO") {
		t.Errorf("sleep raw_json = %q, want it to contain dailySleepDTO", raw)
	}

	sl, _ := s.GetSyncLog("garmin")
	if sl.Status != "ok" || sl.LastSyncedAt == nil {
		t.Errorf("sync_log = %+v, want ok with last_synced_at", sl)
	}
}

func TestSyncGarminError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 're-run worker.py login' 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	res := SyncGarmin(context.Background(), s, r, nil)
	if res.Status != "error" || res.Error == nil {
		t.Fatalf("result = %+v, want error", res)
	}
	if !strings.Contains(*res.Error, "re-run worker.py login") {
		t.Errorf("error = %q, want stderr surfaced", *res.Error)
	}
	sl, _ := s.GetSyncLog("garmin")
	if sl.Status != "error" || sl.Error == nil {
		t.Errorf("sync_log = %+v, want error", sl)
	}
}

func TestSyncAllGarminOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)

	// Garmin worker fails -> AllResult.Garmin is error; no Strava field exists.
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho boom 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	out := SyncAll(context.Background(), s, r, nil, nil)
	if out.Garmin.Status != "error" || out.Garmin.Error == nil {
		t.Errorf("garmin = %+v, want error", out.Garmin)
	}
}

func TestSyncGarminBackfillWindowIs84Days(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)

	dir := t.TempDir()
	argfile := filepath.Join(dir, "args.txt")
	script := filepath.Join(dir, "capture.sh")
	body := "#!/bin/sh\necho \"$@\" > '" + argfile + "'\n" +
		`echo '{"since":"x","until":"x","fetched_at":"x","sleep":[],"hrv":[],"body_battery":[],"rhr":[],"vo2max":[],"activities":[]}'` + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	res := SyncGarmin(context.Background(), s, r, nil)
	if res.Status != "ok" {
		t.Fatalf("status = %q (err=%v), want ok", res.Status, res.Error)
	}

	gotArgs, err := os.ReadFile(argfile)
	if err != nil {
		t.Fatalf("read argfile: %v", err)
	}
	want := time.Now().AddDate(0, 0, -84).Format("2006-01-02")
	if !strings.Contains(string(gotArgs), "--since "+want) {
		t.Errorf("worker args = %q, want --since %s (~12-week backfill)", strings.TrimSpace(string(gotArgs)), want)
	}
}

func TestRunTickerCallsAndStops(t *testing.T) {
	var calls int32
	fn := func(ctx context.Context) { atomic.AddInt32(&calls, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunTicker(ctx, 10*time.Millisecond, fn)
		close(done)
	}()

	time.Sleep(55 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunTicker did not stop within 1s of cancel")
	}

	if n := atomic.LoadInt32(&calls); n < 1 {
		t.Errorf("tick calls = %d, want >= 1", n)
	}
}
