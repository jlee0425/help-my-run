package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

const testToken = "test-token"

func newTestServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	deps := Deps{
		Store:    s,
		Strava:   strava.NewWithBase("12345", "secret", "http://localhost:8080/api/strava/callback", "https://www.strava.com"),
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) {
			return "ok", 0, nil, "ok", 0, nil
		},
	}
	return NewRouter(deps), s
}

func do(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthNoAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body healthResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestStatusRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/status", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"unauthorized"`) {
		t.Errorf("body = %q, want unauthorized", rec.Body.String())
	}
}

func TestStatusOK(t *testing.T) {
	h, s := newTestServer(t)
	// Connect Strava + add one recovery day so counts are non-zero.
	_ = s.SaveStravaTokens(store.StravaTokens{
		AccessToken: "a", RefreshToken: "r", ExpiresAt: 4102444800, Scope: "activity:read_all", AthleteID: 999,
	})
	_ = s.UpsertRhr(store.RhrRow{Date: "2026-06-18", RestingHR: i64(47), RawJSON: "{}"})

	rec := do(t, h, http.MethodGet, "/api/status", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body statusResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Strava.Connected || body.Strava.AthleteID == nil || *body.Strava.AthleteID != 999 {
		t.Errorf("strava = %+v, want connected athlete 999", body.Strava)
	}
	if body.Strava.Status != "never" {
		t.Errorf("strava.status = %q, want never (seeded)", body.Strava.Status)
	}
	if !body.Garmin.Connected {
		t.Errorf("garmin.connected = %v, want true (one rhr row)", body.Garmin.Connected)
	}
	if body.Counts.RecoveryDays != 1 {
		t.Errorf("counts.recovery_days = %d, want 1", body.Counts.RecoveryDays)
	}
}

func TestConnectURL(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/strava/connect", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body connectResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(body.AuthorizeURL, "https://www.strava.com/oauth/authorize?") {
		t.Errorf("authorizeUrl = %q, want strava authorize URL", body.AuthorizeURL)
	}
	if !strings.Contains(body.AuthorizeURL, "scope=activity%3Aread_all") {
		t.Errorf("authorizeUrl = %q, want scope activity:read_all", body.AuthorizeURL)
	}
}

func i64(v int64) *int64 { return &v }
func ptrTime() string    { return time.Now().UTC().Format(time.RFC3339) }
