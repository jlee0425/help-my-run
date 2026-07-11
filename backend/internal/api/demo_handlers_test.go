package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/auth"
	"help-my-run/backend/internal/store"
)

// newDemoServer builds a router in demo mode: no owner setup, no session —
// every request is the owner (M6.5).
func newDemoServer(t *testing.T) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "demo.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	deps := Deps{
		Store: s,
		Auth:  auth.New(s),
		Demo:  true,
		SyncFunc: func(ctx context.Context) (string, int, *string) {
			t.Error("SyncFunc must not be reachable in demo mode")
			return "ok", 0, nil
		},
		Coach:    &fakeCoach{},
		ImageDir: t.TempDir(),
		Agent:    &fakeAgent{},
		Progress: &fakeProgress{},
		Streams:  &fakeStreams{},
		Chat:     &fakeChat{},
	}
	return NewRouter(deps)
}

func TestDemoModeBypassesAuth(t *testing.T) {
	h := newDemoServer(t)
	rec := doJSON(t, h, http.MethodGet, "/api/status", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/status unauthenticated in demo = %d, want 200", rec.Code)
	}
}

func TestDemoModeAuthState(t *testing.T) {
	h := newDemoServer(t)
	rec := doJSON(t, h, http.MethodGet, "/api/auth/state", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("auth/state = %d", rec.Code)
	}
	var body struct {
		SetupRequired bool `json:"setup_required"`
		Authed        bool `json:"authed"`
		Demo          bool `json:"demo"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.SetupRequired || !body.Authed || !body.Demo {
		t.Fatalf("demo auth state = %+v, want setup_required=false authed=true demo=true", body)
	}
}

func TestNonDemoAuthStateDemoFalse(t *testing.T) {
	h, _, _ := newFreshServer(t)
	rec := doJSON(t, h, http.MethodGet, "/api/auth/state", "", nil)
	var body struct {
		Demo bool `json:"demo"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.Demo {
		t.Fatal("non-demo server advertises demo=true")
	}
}

// Endpoints that make no sense in demo return 409 with a loud message.
func TestDemoModeGuardedEndpoints(t *testing.T) {
	h := newDemoServer(t)
	// Every endpoint with an effect outside the in-memory DB — credential
	// mutations, Garmin-worker spawns, disk writes, network egress — must 409.
	cases := []struct{ method, path string }{
		{http.MethodPost, "/api/auth/password"},
		{http.MethodPost, "/api/auth/token"},
		{http.MethodPut, "/api/claude/token"},
		{http.MethodDelete, "/api/claude/token"},
		{http.MethodPost, "/api/garmin/login"},
		{http.MethodPost, "/api/garmin/login/mfa"},
		{http.MethodPost, "/api/garmin/disconnect"},
		{http.MethodPost, "/api/sync"},
		{http.MethodPost, "/api/crossfit/parse"},            // disk write + multipart intake
		{http.MethodPost, "/api/plan/generate"},             // tail of the upload flow
		{http.MethodPost, "/api/activities/9/stream/fetch"}, // spawns the Garmin worker
		{http.MethodPost, "/api/push/subscribe"},            // stores caller-supplied URL (SSRF)
		{http.MethodDelete, "/api/push/subscribe"},
		{http.MethodPost, "/api/push/test"}, // POSTs to that URL (SSRF)
	}
	for _, tc := range cases {
		rec := doJSON(t, h, tc.method, tc.path, "", nil)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s %s = %d, want 409", tc.method, tc.path, rec.Code)
			continue
		}
		var body struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		if body.Error == "" || !containsDemoWord(body.Error) {
			t.Errorf("%s %s error = %q, want a 'demo mode' message", tc.method, tc.path, body.Error)
		}
	}
}

func containsDemoWord(s string) bool {
	for i := 0; i+4 <= len(s); i++ {
		if s[i:i+4] == "demo" {
			return true
		}
	}
	return false
}

// Garmin shows as connected in demo so the Settings card isn't a dead CTA —
// and /api/status agrees with /api/garmin/status.
func TestDemoModeGarminStatusConnected(t *testing.T) {
	h := newDemoServer(t)
	for _, path := range []string{"/api/garmin/status", "/api/status"} {
		rec := doJSON(t, h, http.MethodGet, path, "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d", path, rec.Code)
		}
		if !containsWord(rec.Body.String(), `"connected":true`) {
			t.Errorf("%s not connected in demo: %s", path, rec.Body.String())
		}
	}
}

// The Claude card presents a working (sample) coach, never "CLI not installed".
func TestDemoModeClaudeStatus(t *testing.T) {
	h := newDemoServer(t)
	rec := doJSON(t, h, http.MethodGet, "/api/claude/status", "", nil)
	var body struct {
		BinaryFound   bool   `json:"binary_found"`
		Authenticated bool   `json:"authenticated"`
		Detail        string `json:"detail"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.BinaryFound || !body.Authenticated || !containsWord(body.Detail, "sample") {
		t.Fatalf("demo claude status = %+v, want a working sample-coach card", body)
	}
}

// The centerpiece: agent/run is reachable in demo (the "Run coach now" button)
// but the no-op syncer means it never errors on a missing Garmin worker, and it
// re-produces today's amber decision from seeded data.
func TestDemoModeAgentRunIsSafeAndCanned(t *testing.T) {
	h := newDemoServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/agent/run", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("agent/run in demo = %d (%s), want 200", rec.Code, rec.Body.String())
	}
}

func containsWord(hay, needle string) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if hay[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
