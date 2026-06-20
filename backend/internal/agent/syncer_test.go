package agent

import (
	"context"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

func TestRealSyncerCallsSyncAll(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "rs.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// No strava tokens seeded -> SyncStrava errors (status "error"), proving the
	// adapter ran SyncAll end-to-end with the bound deps.
	rs := NewRealSyncer(s, strava.NewWithBase("id", "sec", "http://localhost/cb", "http://localhost"),
		garmin.Runner{Python: "/bin/cat", Script: "/dev/null"}, nil)
	res := rs.SyncAll(context.Background())
	if res.Strava.Status != "error" {
		t.Errorf("strava status = %q, want error (no tokens)", res.Strava.Status)
	}
}
