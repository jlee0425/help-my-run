package agent

import (
	"context"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	syncpkg "help-my-run/backend/internal/sync"
)

// RealSyncer binds the sync dependencies and adapts sync.SyncAll to the Syncer
// seam used by the Agent.
type RealSyncer struct {
	store    *store.Store
	runner   garmin.Runner
	extraEnv []string
}

// NewRealSyncer constructs the production Syncer.
func NewRealSyncer(s *store.Store, r garmin.Runner, extraEnv []string) *RealSyncer {
	return &RealSyncer{store: s, runner: r, extraEnv: extraEnv}
}

// SyncAll runs the Garmin sync with the bound deps. No stream trickle in the
// daily agent loop (recent-window stream backfill rides the server's periodic sync).
//
// It first guards on the Garmin token store: when it does not yet exist (the
// one-time `worker.py login` has not populated it), the sync is skipped without
// invoking the worker, so the agent never contributes to Garmin's per-IP login
// rate limit. A "skipped" result writes no sync_log row, so any prior ok/error
// row — and the connected=(Status=="ok") status derivation — is preserved.
func (r *RealSyncer) SyncAll(ctx context.Context) syncpkg.AllResult {
	if !syncpkg.TokenStoreReady(syncpkg.TokenStorePathFromEnv(r.extraEnv)) {
		msg := "garmin token store not found; skipping sync (run garmin login)"
		return syncpkg.AllResult{Garmin: syncpkg.SourceResult{Status: "skipped", Synced: 0, Error: &msg}}
	}
	return syncpkg.SyncAll(ctx, r.store, r.runner, r.extraEnv, nil)
}
