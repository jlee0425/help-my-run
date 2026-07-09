package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/auth"
	"help-my-run/backend/internal/store"
)

// newFreshServer is newTestServer WITHOUT auth setup (first-run state).
func newFreshServer(t *testing.T) (http.Handler, *store.Store, *auth.Service) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	a := auth.New(s)
	deps := Deps{
		Store: s,
		Auth:  a,
		SyncFunc: func(ctx context.Context) (string, int, *string) {
			return "ok", 0, nil
		},
		Coach:    &fakeCoach{},
		ImageDir: t.TempDir(),
		Agent:    &fakeAgent{},
		Progress: &fakeProgress{},
		Streams:  &fakeStreams{},
		Chat:     &fakeChat{},
	}
	return NewRouter(deps), s, a
}

func doJSON(t *testing.T, h http.Handler, method, path, body string, mod func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "hmr_session" {
			return c
		}
	}
	t.Fatalf("no hmr_session cookie in response")
	return nil
}

func TestAuthStateAndSetupFlow(t *testing.T) {
	h, _, _ := newFreshServer(t)

	// Fresh DB: setup required, protected routes locked.
	rec := doJSON(t, h, http.MethodGet, "/api/auth/state", "", nil)
	var st authStateDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if rec.Code != 200 || !st.SetupRequired || st.Authed {
		t.Fatalf("state = %d %+v, want 200 setup_required", rec.Code, st)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/status", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected pre-setup = %d, want 401", rec.Code)
	}

	// Weak password rejected.
	if rec := doJSON(t, h, http.MethodPost, "/api/setup", `{"password":"short"}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("weak setup = %d, want 400", rec.Code)
	}

	// Setup succeeds: api_token + session cookie.
	rec = doJSON(t, h, http.MethodPost, "/api/setup", `{"password":"a strong password"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup = %d (%s)", rec.Code, rec.Body.String())
	}
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	tok := out["api_token"]
	if !strings.HasPrefix(tok, "hmr_") {
		t.Fatalf("api_token = %q, want hmr_ prefix", tok)
	}
	ck := sessionCookie(t, rec)
	if !ck.HttpOnly || ck.Path != "/" {
		t.Fatalf("cookie flags = %+v", ck)
	}

	// Second setup: 409.
	if rec := doJSON(t, h, http.MethodPost, "/api/setup", `{"password":"another password"}`, nil); rec.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want 409", rec.Code)
	}

	// State with the cookie: authed.
	rec = doJSON(t, h, http.MethodGet, "/api/auth/state", "", func(r *http.Request) { r.AddCookie(ck) })
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.SetupRequired || !st.Authed {
		t.Fatalf("state after setup = %+v", st)
	}

	// Protected route: cookie works, bearer works, garbage doesn't.
	if rec := doJSON(t, h, http.MethodGet, "/api/status", "", func(r *http.Request) { r.AddCookie(ck) }); rec.Code != 200 {
		t.Fatalf("status via cookie = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/status", "", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+tok) }); rec.Code != 200 {
		t.Fatalf("status via bearer = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/status", "", func(r *http.Request) { r.Header.Set("Authorization", "Bearer hmr_bogus") }); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status via bogus bearer = %d, want 401", rec.Code)
	}

	// CSRF: cookie-authed cross-site POST rejected; same-site allowed.
	if rec := doJSON(t, h, http.MethodPost, "/api/sync", "", func(r *http.Request) {
		r.AddCookie(ck)
		r.Header.Set("Sec-Fetch-Site", "cross-site")
	}); rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site POST = %d, want 403", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/sync", "", func(r *http.Request) {
		r.AddCookie(ck)
		r.Header.Set("Sec-Fetch-Site", "same-origin")
	}); rec.Code != 200 {
		t.Fatalf("same-origin POST = %d, want 200", rec.Code)
	}
	// Bearer cross-site is exempt (no ambient credential).
	if rec := doJSON(t, h, http.MethodPost, "/api/sync", "", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+tok)
		r.Header.Set("Sec-Fetch-Site", "cross-site")
	}); rec.Code != 200 {
		t.Fatalf("bearer cross-site POST = %d, want 200", rec.Code)
	}

	// Logout: cookie invalidated.
	if rec := doJSON(t, h, http.MethodPost, "/api/logout", "", func(r *http.Request) { r.AddCookie(ck) }); rec.Code != http.StatusNoContent {
		t.Fatalf("logout = %d", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/status", "", func(r *http.Request) { r.AddCookie(ck) }); rec.Code != http.StatusUnauthorized {
		t.Fatalf("status after logout = %d, want 401", rec.Code)
	}
}

func TestLoginThrottleAndPasswordChange(t *testing.T) {
	h, _, _ := newFreshServer(t)
	if rec := doJSON(t, h, http.MethodPost, "/api/setup", `{"password":"a strong password"}`, nil); rec.Code != 200 {
		t.Fatalf("setup = %d", rec.Code)
	}

	if rec := doJSON(t, h, http.MethodPost, "/api/login", `{"password":"wrong"}`, nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong login = %d, want 401", rec.Code)
	}
	// Immediate retry throttled (backoff active).
	if rec := doJSON(t, h, http.MethodPost, "/api/login", `{"password":"a strong password"}`, nil); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("throttled login = %d, want 429", rec.Code)
	}
}

func TestChangePasswordAndTokenRegenerate(t *testing.T) {
	h, _, _ := newFreshServer(t)
	rec := doJSON(t, h, http.MethodPost, "/api/setup", `{"password":"a strong password"}`, nil)
	ck := sessionCookie(t, rec)
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	oldTok := out["api_token"]

	if rec := doJSON(t, h, http.MethodPost, "/api/auth/password", `{"current":"nope","new":"next strong password"}`,
		func(r *http.Request) { r.AddCookie(ck) }); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad current = %d, want 401", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/auth/password", `{"current":"a strong password","new":"next strong password"}`,
		func(r *http.Request) { r.AddCookie(ck) }); rec.Code != http.StatusNoContent {
		t.Fatalf("change password = %d", rec.Code)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/auth/token", "", func(r *http.Request) { r.AddCookie(ck) })
	if rec.Code != 200 {
		t.Fatalf("regenerate = %d", rec.Code)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	newTok := out["api_token"]
	if newTok == "" || newTok == oldTok {
		t.Fatalf("regenerated token = %q (old %q)", newTok, oldTok)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/status", "", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+oldTok) }); rec.Code != http.StatusUnauthorized {
		t.Fatalf("old token after regenerate = %d, want 401", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodGet, "/api/status", "", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+newTok) }); rec.Code != 200 {
		t.Fatalf("new token = %d, want 200", rec.Code)
	}
}

func TestStatusIncludesAgentSchedule(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/status", testToken)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp statusResp
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// No profile row -> defaults: enabled, next run computed from 05:30 UTC.
	if !resp.AgentEnabled {
		t.Errorf("agent_enabled = false, want true (default)")
	}
	if resp.AgentNextRun == nil || !strings.Contains(*resp.AgentNextRun, "T") {
		t.Errorf("agent_next_run = %v, want RFC3339", resp.AgentNextRun)
	}
}

func TestNonAPIRoutesFallToWebUI(t *testing.T) {
	h, _ := newTestServer(t)
	// The SPA handler is the router's NotFound: /settings serves index.html
	// when the UI is built into dist/, or the 503 build hint when it isn't —
	// never the API's JSON 404.
	rec := doJSON(t, h, http.MethodGet, "/settings", "", nil)
	builtOK := rec.Code == http.StatusOK && strings.Contains(rec.Body.String(), "<html")
	unbuiltOK := rec.Code == http.StatusServiceUnavailable && strings.Contains(rec.Body.String(), "make build")
	if !builtOK && !unbuiltOK {
		t.Fatalf("/settings = %d %q, want SPA html or 503 build hint", rec.Code, rec.Body.String())
	}
	// Unknown API path stays JSON 404.
	rec = doJSON(t, h, http.MethodGet, "/api/nope", "", nil)
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("/api/nope = %d %q, want JSON 404", rec.Code, rec.Body.String())
	}
}

func TestPushSubscribeLifecycle(t *testing.T) {
	h, s := newTestServer(t)
	// No Push service wired in newTestServer -> key/test degrade to 503.
	if rec := do(t, h, http.MethodGet, "/api/push/vapid-public-key", testToken); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("vapid key unwired = %d, want 503", rec.Code)
	}
	// Subscribe/unsubscribe only need the store.
	body := `{"endpoint":"https://push.example/e1","keys":{"p256dh":"pk","auth":"ak"}}`
	if rec := doJSON(t, h, http.MethodPost, "/api/push/subscribe", body, addBearer); rec.Code != http.StatusNoContent {
		t.Fatalf("subscribe = %d", rec.Code)
	}
	subs, _ := s.ListPushSubscriptions()
	if len(subs) != 1 || subs[0].P256dh != "pk" {
		t.Fatalf("stored subs = %+v", subs)
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/push/subscribe", `{"endpoint":"x"}`, addBearer); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad subscribe = %d, want 400", rec.Code)
	}
	if rec := doJSON(t, h, http.MethodDelete, "/api/push/subscribe", `{"endpoint":"https://push.example/e1"}`, addBearer); rec.Code != http.StatusNoContent {
		t.Fatalf("unsubscribe = %d", rec.Code)
	}
	subs, _ = s.ListPushSubscriptions()
	if len(subs) != 0 {
		t.Fatalf("subs after unsubscribe = %+v", subs)
	}
}

func addBearer(r *http.Request) { r.Header.Set("Authorization", "Bearer "+testToken) }

// M5 review fix: a failed Garmin sync must surface as 502 + top-level error,
// never a silent 200 (real-world: Garmin 400 "requested date range is too big").
func TestSyncErrorIsNotASilent200(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "syncerr.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	msg := "worker exit 1: fetch failed: API call client error (400): API Error 400 - requested date range is too big."
	deps := Deps{
		Store: s, Auth: testAuth(t, s),
		SyncFunc: func(ctx context.Context) (string, int, *string) { return "error", 0, &msg },
		Coach:    &fakeCoach{}, ImageDir: t.TempDir(), Agent: &fakeAgent{},
		Progress: &fakeProgress{}, Streams: &fakeStreams{}, Chat: &fakeChat{},
	}
	h := NewRouter(deps)

	rec := doJSON(t, h, http.MethodPost, "/api/sync", "", addBearer)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("failed sync = %d, want 502 (body=%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		Error  string `json:"error"`
		Garmin struct {
			Status string  `json:"status"`
			Error  *string `json:"error"`
		} `json:"garmin"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(body.Error, "date range is too big") {
		t.Errorf("top-level error = %q, want the worker message", body.Error)
	}
	if body.Garmin.Status != "error" || body.Garmin.Error == nil {
		t.Errorf("garmin source = %+v, want embedded error preserved", body.Garmin)
	}
}
