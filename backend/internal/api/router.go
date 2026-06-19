package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

// SyncFunc runs both syncs and returns flattened per-source results:
// (stravaStatus, stravaSynced, stravaErr, garminStatus, garminSynced, garminErr).
// Wiring (main.go) adapts the sync package to this signature so the api package
// does not import sync (avoids an import cycle and keeps handlers testable).
type SyncFunc func(ctx context.Context) (string, int, *string, string, int, *string)

// Deps are the handler dependencies injected by main.go (and tests).
type Deps struct {
	Store    *store.Store
	Strava   *strava.Client
	APIToken string
	SyncFunc SyncFunc
}

// NewRouter builds the chi router with public + bearer-protected routes.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	h := &handlers{d: d}

	// Public (no auth).
	r.Get("/health", h.health)
	r.Get("/api/strava/callback", h.stravaCallback)

	// Protected.
	r.Group(func(r chi.Router) {
		r.Use(BearerAuth(d.APIToken))
		r.Get("/api/status", h.status)
		r.Get("/api/strava/connect", h.stravaConnect)
		r.Post("/api/sync", h.sync)
		r.Get("/api/activities", h.activities)
		r.Get("/api/recovery", h.recovery)
	})

	return r
}
