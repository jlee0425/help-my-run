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
func (r *RealSyncer) SyncAll(ctx context.Context) syncpkg.AllResult {
	return syncpkg.SyncAll(ctx, r.store, r.runner, r.extraEnv, nil)
}
