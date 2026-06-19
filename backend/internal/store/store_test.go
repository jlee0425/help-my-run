package store

import (
	"path/filepath"
	"testing"
)

// newTestStore opens a fresh, migrated store in a temp dir. Shared by all
// store tests.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", dbPath, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return s
}

func TestOpenAndMigrate(t *testing.T) {
	s := newTestStore(t)

	wantTables := []string{
		"strava_tokens", "activities", "activity_splits",
		"garmin_sleep", "garmin_hrv", "garmin_body_battery", "garmin_rhr",
		"sync_log",
	}
	for _, tbl := range wantTables {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migrate: %v", tbl, err)
		}
	}
}

func TestMigrateSeedsSyncLog(t *testing.T) {
	s := newTestStore(t)

	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM sync_log`).Scan(&n); err != nil {
		t.Fatalf("count sync_log: %v", err)
	}
	if n != 2 {
		t.Errorf("sync_log row count = %d, want 2 (strava, garmin)", n)
	}
	for _, src := range []string{"strava", "garmin"} {
		var status string
		err := s.DB.QueryRow(`SELECT status FROM sync_log WHERE source=?`, src).Scan(&status)
		if err != nil {
			t.Errorf("sync_log source %q missing: %v", src, err)
			continue
		}
		if status != "never" {
			t.Errorf("sync_log[%q].status = %q, want %q", src, status, "never")
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	s := newTestStore(t)
	// Second Migrate() must be a no-op, not an error.
	if err := s.Migrate(); err != nil {
		t.Fatalf("second Migrate() error = %v, want nil", err)
	}
}

func TestStravaTokensRoundTrip(t *testing.T) {
	s := newTestStore(t)

	// Not connected yet.
	if _, err := s.GetStravaTokens(); err != ErrNotFound {
		t.Fatalf("GetStravaTokens() on empty = %v, want ErrNotFound", err)
	}

	in := StravaTokens{
		AccessToken:  "acc",
		RefreshToken: "ref",
		ExpiresAt:    1737000000,
		Scope:        "read,activity:read_all",
		AthleteID:    12345678,
	}
	if err := s.SaveStravaTokens(in); err != nil {
		t.Fatalf("SaveStravaTokens() error = %v", err)
	}

	got, err := s.GetStravaTokens()
	if err != nil {
		t.Fatalf("GetStravaTokens() error = %v", err)
	}
	if got.AccessToken != in.AccessToken || got.RefreshToken != in.RefreshToken ||
		got.ExpiresAt != in.ExpiresAt || got.Scope != in.Scope || got.AthleteID != in.AthleteID {
		t.Errorf("GetStravaTokens() = %+v, want %+v", got, in)
	}

	// Overwrite (id is always 1).
	in.AccessToken = "acc2"
	in.ExpiresAt = 1737099999
	if err := s.SaveStravaTokens(in); err != nil {
		t.Fatalf("SaveStravaTokens() overwrite error = %v", err)
	}
	got, _ = s.GetStravaTokens()
	if got.AccessToken != "acc2" || got.ExpiresAt != 1737099999 {
		t.Errorf("after overwrite got %+v, want AccessToken=acc2 ExpiresAt=1737099999", got)
	}

	var rows int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM strava_tokens`).Scan(&rows); err != nil {
		t.Fatalf("count strava_tokens: %v", err)
	}
	if rows != 1 {
		t.Errorf("strava_tokens row count = %d, want 1 (single-row table)", rows)
	}
}

func TestSyncLogGetAndUpdate(t *testing.T) {
	s := newTestStore(t)

	// Seeded default.
	sl, err := s.GetSyncLog("strava")
	if err != nil {
		t.Fatalf("GetSyncLog(strava) error = %v", err)
	}
	if sl.Status != "never" || sl.LastSyncedAt != nil || sl.Error != nil {
		t.Errorf("seed sync_log = %+v, want status=never, nils", sl)
	}

	syncedAt := "2026-06-19T05:00:30Z"
	upd := SyncLog{
		Source:       "strava",
		LastSyncedAt: &syncedAt,
		LastRunAt:    &syncedAt,
		Status:       "ok",
		Error:        nil,
	}
	if err := s.UpdateSyncLog(upd); err != nil {
		t.Fatalf("UpdateSyncLog() error = %v", err)
	}
	got, _ := s.GetSyncLog("strava")
	if got.Status != "ok" || got.LastSyncedAt == nil || *got.LastSyncedAt != syncedAt {
		t.Errorf("after update got %+v, want status=ok last_synced_at=%s", got, syncedAt)
	}

	// Error path keeps last_synced_at nil but sets error.
	errMsg := "worker exit 1: re-run worker.py login"
	if err := s.UpdateSyncLog(SyncLog{
		Source: "garmin", LastSyncedAt: nil, LastRunAt: &syncedAt,
		Status: "error", Error: &errMsg,
	}); err != nil {
		t.Fatalf("UpdateSyncLog(garmin error) error = %v", err)
	}
	gg, _ := s.GetSyncLog("garmin")
	if gg.Status != "error" || gg.Error == nil || *gg.Error != errMsg {
		t.Errorf("garmin sync_log = %+v, want status=error error=%q", gg, errMsg)
	}
}
