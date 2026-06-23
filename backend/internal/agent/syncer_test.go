package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
)

// readyTokenStore returns a populated temp dir and the matching extraEnv slice
// so RealSyncer.SyncAll passes the token-store gate and actually runs the sync.
func readyTokenStore(t *testing.T) []string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "oauth1_token.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("seed token store: %v", err)
	}
	return []string{"GARMIN_TOKENSTORE=" + dir}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "rs.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func TestRealSyncerCallsSyncAll(t *testing.T) {
	s := openStore(t)

	// A worker that produces no valid JSON -> SyncGarmin errors (status "error"),
	// proving the adapter ran SyncAll end-to-end with the bound deps. extraEnv
	// points at a populated token store so the gate lets the sync proceed.
	rs := NewRealSyncer(s, garmin.Runner{Python: "/bin/cat", Script: "/dev/null"}, readyTokenStore(t))
	res := rs.SyncAll(context.Background())
	if res.Garmin.Status != "error" {
		t.Errorf("garmin status = %q, want error (worker produced no output)", res.Garmin.Status)
	}
}

func TestRealSyncerSkipsWhenTokenStoreMissing(t *testing.T) {
	s := openStore(t)

	// extraEnv points the token store at a nonexistent dir -> gate skips the sync
	// WITHOUT invoking the worker (so even a bogus runner is never run).
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	rs := NewRealSyncer(s, garmin.Runner{Python: "/bin/false", Script: "/dev/null"},
		[]string{"GARMIN_TOKENSTORE=" + missing})

	res := rs.SyncAll(context.Background())
	if res.Garmin.Status != "skipped" {
		t.Fatalf("garmin status = %q, want skipped (token store missing)", res.Garmin.Status)
	}
	if res.Garmin.Synced != 0 {
		t.Errorf("synced = %d, want 0", res.Garmin.Synced)
	}
	if res.Garmin.Error == nil || *res.Garmin.Error == "" {
		t.Errorf("error note = %v, want a non-empty skip note", res.Garmin.Error)
	}

	// A skipped run must NOT write sync_log: the seed row (status "never" from
	// 00001_init) is preserved untouched, so status() still derives
	// connected=(Status=="ok")=false. A skip must never masquerade as a real run.
	sl, err := s.GetSyncLog("garmin")
	if err != nil {
		t.Fatalf("GetSyncLog: %v", err)
	}
	if sl.Status != "never" {
		t.Errorf("sync_log garmin status = %q after skip; want seed %q preserved (skip must not write sync_log)", sl.Status, "never")
	}
}
