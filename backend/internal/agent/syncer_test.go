package agent

import (
	"context"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
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

	// A worker that produces no valid JSON -> SyncGarmin errors (status "error"),
	// proving the adapter ran SyncAll end-to-end with the bound deps.
	rs := NewRealSyncer(s, garmin.Runner{Python: "/bin/cat", Script: "/dev/null"}, nil)
	res := rs.SyncAll(context.Background())
	if res.Garmin.Status != "error" {
		t.Errorf("garmin status = %q, want error (worker produced no output)", res.Garmin.Status)
	}
}
