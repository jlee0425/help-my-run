package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"help-my-run/backend/internal/config"
)

func TestWireServesHealthAndAuth(t *testing.T) {
	cfg := &config.Config{
		StravaClientID:     "12345",
		StravaClientSecret: "secret",
		StravaRedirectURL:  "http://localhost:8080/api/strava/callback",
		APIToken:           "tok",
		DBPath:             filepath.Join(t.TempDir(), "wire.db"),
		Port:               "8080",
		PythonBin:          "/bin/cat",
		WorkerScript:       "/dev/null",
	}

	app, err := Wire(cfg)
	if err != nil {
		t.Fatalf("Wire error = %v", err)
	}
	t.Cleanup(func() { _ = app.Store.Close() })

	// /health: no auth, 200.
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/health = %d, want 200", rec.Code)
	}

	// /api/status without token: 401.
	rec = httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/status (no auth) = %d, want 401", rec.Code)
	}

	// /api/status with token: 200 (DB migrated, sync_log seeded).
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/status (auth) = %d, want 200", rec.Code)
	}
}

func TestWireInjectsCoach(t *testing.T) {
	cfg := &config.Config{
		StravaClientID:     "12345",
		StravaClientSecret: "secret",
		StravaRedirectURL:  "http://localhost:8080/api/strava/callback",
		APIToken:           "tok",
		DBPath:             filepath.Join(t.TempDir(), "coach-wire.db"),
		Port:               "8080",
		PythonBin:          "/bin/cat",
		WorkerScript:       "/dev/null",
		ClaudeBin:          "claude",
		ClaudeModel:        "claude-opus-4-8",
		ImageDir:           filepath.Join(t.TempDir(), "cfimg"),
	}
	app, err := Wire(cfg)
	if err != nil {
		t.Fatalf("Wire error = %v", err)
	}
	t.Cleanup(func() { _ = app.Store.Close() })

	// /api/fitness is bearer-protected and served by the injected coach -> 200
	// (computes from an empty store, no claude needed).
	req := httptest.NewRequest(http.MethodGet, "/api/fitness", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/fitness = %d, want 200 (coach wired)", rec.Code)
	}
}

func TestRunSyncOnBoot(t *testing.T) {
	called := make(chan struct{}, 1)
	fn := func(ctx context.Context) {
		select {
		case called <- struct{}{}:
		default:
		}
	}
	runSyncOnBoot(context.Background(), fn)
	// runSyncOnBoot invokes fn in a goroutine (non-blocking startup), so wait for
	// the boot sync to land rather than reading the channel synchronously.
	select {
	case <-called:
		// ok: sync ran exactly once on boot.
	case <-time.After(2 * time.Second):
		t.Fatal("runSyncOnBoot did not invoke the sync fn")
	}
}
