package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
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

func TestSyncStravaRefreshesAndUpserts(t *testing.T) {
	s := newStore(t)
	// Store an EXPIRED token so the sync must refresh first.
	if err := s.SaveStravaTokens(store.StravaTokens{
		AccessToken: "old-acc", RefreshToken: "old-ref",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(), Scope: "activity:read_all", AthleteID: 1,
	}); err != nil {
		t.Fatalf("seed tokens: %v", err)
	}

	var refreshed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/oauth/token":
			refreshed = true
			_, _ = w.Write([]byte(`{"token_type":"Bearer","access_token":"fresh","refresh_token":"fresh-ref","expires_at":4102444800,"expires_in":21600,"scope":"activity:read_all","athlete":{"id":1}}`))
		case r.URL.Path == "/api/v3/athlete/activities":
			if r.URL.Query().Get("page") == "1" {
				_, _ = w.Write([]byte(`[{"id":900,"name":"Run","type":"Run","sport_type":"Run","start_date":"2026-06-18T06:00:00Z","start_date_local":"2026-06-18T08:00:00Z","distance":10000,"moving_time":3000,"elapsed_time":3050,"average_heartrate":150,"max_heartrate":170,"average_speed":3.3,"max_speed":4.5,"average_cadence":85,"total_elevation_gain":50}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		case r.URL.Path == "/api/v3/activities/900/laps":
			_, _ = w.Write([]byte(`[{"lap_index":1,"distance":5000,"elapsed_time":1500,"moving_time":1490,"average_heartrate":148,"max_heartrate":160,"average_speed":3.3}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := strava.NewWithBase("123", "secret", "http://cb", srv.URL)
	res := SyncStrava(context.Background(), s, client)

	if res.Status != "ok" || res.Error != nil {
		t.Fatalf("result = %+v, want ok/no-error", res)
	}
	if res.Synced != 1 {
		t.Errorf("synced = %d, want 1", res.Synced)
	}
	if !refreshed {
		t.Error("expected token refresh on expired token")
	}
	// Fresh token persisted.
	tok, _ := s.GetStravaTokens()
	if tok.AccessToken != "fresh" || tok.RefreshToken != "fresh-ref" {
		t.Errorf("tokens = %+v, want fresh/fresh-ref", tok)
	}
	// Activity + lap upserted.
	acts, _ := s.ListActivities(30)
	if len(acts) != 1 || acts[0].StravaID != 900 {
		t.Fatalf("activities = %+v, want one id=900", acts)
	}
	var nLaps int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activity_splits WHERE activity_id=900`).Scan(&nLaps)
	if nLaps != 1 {
		t.Errorf("laps = %d, want 1", nLaps)
	}
	// sync_log updated.
	sl, _ := s.GetSyncLog("strava")
	if sl.Status != "ok" || sl.LastSyncedAt == nil {
		t.Errorf("sync_log = %+v, want ok with last_synced_at", sl)
	}
}

func TestSyncStravaNotConnected(t *testing.T) {
	s := newStore(t)
	client := strava.NewWithBase("123", "secret", "http://cb", "http://unused")
	res := SyncStrava(context.Background(), s, client)
	if res.Status != "error" || res.Error == nil {
		t.Fatalf("result = %+v, want error when not connected", res)
	}
	sl, _ := s.GetSyncLog("strava")
	if sl.Status != "error" {
		t.Errorf("sync_log status = %q, want error", sl.Status)
	}
}

func TestSyncGarminUpsertsAllTables(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)

	fixture, err := filepath.Abs(filepath.Join("..", "garmin", "testdata", "worker_output.json"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	// Stub "worker": a shell script that cats the fixture and ignores the fetch
	// args. (GNU coreutils `cat` rejects the runner's `--since` flag as an
	// unknown option, so a /bin/sh script is used instead of /bin/cat.)
	script := filepath.Join(t.TempDir(), "stub.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat '"+fixture+"'\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	res := SyncGarmin(context.Background(), s, r, nil)
	if res.Status != "ok" || res.Error != nil {
		t.Fatalf("result = %+v, want ok", res)
	}
	// Fixture has 2 sleep + 1 hrv + 2 bb + 2 rhr = 7 upserts.
	if res.Synced != 7 {
		t.Errorf("synced = %d, want 7", res.Synced)
	}

	counts := map[string]int{
		"garmin_sleep": 0, "garmin_hrv": 0, "garmin_body_battery": 0, "garmin_rhr": 0,
	}
	for tbl := range counts {
		var n int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM ` + tbl).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		counts[tbl] = n
	}
	if counts["garmin_sleep"] != 2 || counts["garmin_hrv"] != 1 ||
		counts["garmin_body_battery"] != 2 || counts["garmin_rhr"] != 2 {
		t.Errorf("counts = %+v, want sleep2 hrv1 bb2 rhr2", counts)
	}

	// raw_json persisted from the worker.
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

func TestSyncAllPartialFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)
	if err := s.SaveStravaTokens(store.StravaTokens{
		AccessToken: "acc", RefreshToken: "ref",
		ExpiresAt: time.Now().Add(time.Hour).Unix(), Scope: "activity:read_all", AthleteID: 1,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// No activities -> 0 synced, status ok.
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	client := strava.NewWithBase("123", "secret", "http://cb", srv.URL)

	// Garmin worker fails.
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho boom 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	out := SyncAll(context.Background(), s, client, r, nil)
	if out.Strava.Status != "ok" {
		t.Errorf("strava = %+v, want ok", out.Strava)
	}
	if out.Garmin.Status != "error" || out.Garmin.Error == nil {
		t.Errorf("garmin = %+v, want error", out.Garmin)
	}
}

func TestSyncStravaCursorFromLatestActivity(t *testing.T) {
	var gotAfter string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth/token"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token_type":"Bearer","access_token":"acc","refresh_token":"ref","expires_at":4102444800,"expires_in":21600,"scope":"activity:read_all"}`))
		case strings.Contains(r.URL.Path, "/athlete/activities"):
			gotAfter = r.URL.Query().Get("after")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "cur.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = s.SaveStravaTokens(store.StravaTokens{AccessToken: "acc", RefreshToken: "ref", ExpiresAt: 4102444800})
	_ = s.UpsertActivity(store.Activity{
		StravaID: 1, Type: "Run", Name: "r", StartTime: "2026-06-18T18:00:00Z",
		DistanceM: 8000, MovingTimeS: 2400, ElapsedTimeS: 2400, RawJSON: "{}",
	})

	client := strava.NewWithBase("1", "x", "http://cb", srv.URL)
	res := SyncStrava(context.Background(), s, client)
	if res.Status != "ok" {
		t.Fatalf("sync status = %q (err=%v), want ok", res.Status, res.Error)
	}

	wantUnix := mustUnix(t, "2026-06-18T18:00:00Z")
	if gotAfter != wantUnix {
		t.Errorf("after = %q, want %q (latest stored activity start_time)", gotAfter, wantUnix)
	}
}

func mustUnix(t *testing.T, rfc string) string {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		t.Fatalf("parse %q: %v", rfc, err)
	}
	return strconv.FormatInt(ts.Unix(), 10)
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

	// Let a few ticks happen, then cancel.
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
