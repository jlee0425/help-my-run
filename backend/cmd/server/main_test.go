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

func testCfg(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		StravaClientID:      "id",
		StravaClientSecret:  "secret",
		StravaRedirectURL:   "http://localhost:8080/api/strava/callback",
		APIToken:            "tok",
		DBPath:              filepath.Join(t.TempDir(), "wire.db"),
		Port:                "8080",
		ClaudeBin:           "claude",
		ClaudeModel:         "claude-opus-4-8",
		ImageDir:            t.TempDir(),
		AgentEnabledDefault: true,
		AgentRunTime:        "05:30",
		AgentTimezone:       "Asia/Seoul",
		AgentTickInterval:   "1m",
		ExpoPushBaseURL:     "https://exp.host",
	}
}

func TestWireBuildsM2Graph(t *testing.T) {
	app, err := Wire(testCfg(t))
	if err != nil {
		t.Fatalf("Wire error = %v", err)
	}
	defer func() { _ = app.Store.Close() }()

	if app.Agent == nil {
		t.Error("app.Agent = nil, want a wired *agent.Agent")
	}
	if app.Pusher == nil {
		t.Error("app.Pusher = nil, want a wired *push.Client")
	}
	if app.Handler == nil {
		t.Error("app.Handler = nil")
	}
}

func TestWireTzdataLoadsSeoul(t *testing.T) {
	_, err := loadAgentLocation("Asia/Seoul")
	if err != nil {
		t.Fatalf("loadAgentLocation(Asia/Seoul) error = %v", err)
	}
}

func TestWiredHandlerServesToday404(t *testing.T) {
	app, err := Wire(testCfg(t))
	if err != nil {
		t.Fatalf("Wire error = %v", err)
	}
	defer func() { _ = app.Store.Close() }()

	req := httptest.NewRequest("GET", "/api/today?date=2026-06-20", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("today status = %d, want 404 (no decision seeded)", rec.Code)
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
