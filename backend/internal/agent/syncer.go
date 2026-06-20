package agent

import (
	"context"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
	syncpkg "help-my-run/backend/internal/sync"
)

// RealSyncer binds the sync dependencies and adapts sync.SyncAll to the Syncer
// seam used by the Agent.
type RealSyncer struct {
	store    *store.Store
	strava   *strava.Client
	runner   garmin.Runner
	extraEnv []string
}

// NewRealSyncer constructs the production Syncer.
func NewRealSyncer(s *store.Store, sc *strava.Client, r garmin.Runner, extraEnv []string) *RealSyncer {
	return &RealSyncer{store: s, strava: sc, runner: r, extraEnv: extraEnv}
}

// SyncAll runs both syncs with the bound deps.
func (r *RealSyncer) SyncAll(ctx context.Context) syncpkg.AllResult {
	return syncpkg.SyncAll(ctx, r.store, r.strava, r.runner, r.extraEnv)
}
