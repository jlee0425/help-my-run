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
