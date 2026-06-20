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
		Coach:    &fakeCoach{},
		ImageDir: t.TempDir(),
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

func TestActivitiesHandler(t *testing.T) {
	h, s := newTestServer(t)
	_ = s.UpsertActivity(store.Activity{
		StravaID: 11, Name: "A", Type: "Run", SportType: sp("Run"),
		StartTime: "2026-06-18T06:00:00Z", StartTimeLocal: sp("2026-06-18T08:00:00"),
		DistanceM: 10000, MovingTimeS: 3000, ElapsedTimeS: 3050,
		AvgHR: fp(150), RawJSON: "{}",
	})
	_ = s.UpsertActivity(store.Activity{
		StravaID: 12, Name: "B", Type: "Run", SportType: nil,
		StartTime: "2026-06-17T06:00:00Z", DistanceM: 5000,
		MovingTimeS: 1500, ElapsedTimeS: 1500, RawJSON: "{}",
	})

	rec := do(t, h, http.MethodGet, "/api/activities", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body activitiesResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Activities) != 2 {
		t.Fatalf("len = %d, want 2", len(body.Activities))
	}
	// Most-recent-first.
	if body.Activities[0].StravaID != 11 {
		t.Errorf("first id = %d, want 11", body.Activities[0].StravaID)
	}
	if body.Activities[0].AvgHR == nil || *body.Activities[0].AvgHR != 150 {
		t.Errorf("avg_hr = %v, want 150", body.Activities[0].AvgHR)
	}
	if body.Activities[1].SportType != nil {
		t.Errorf("sport_type = %v, want null", body.Activities[1].SportType)
	}

	// limit=1 clamps.
	rec = do(t, h, http.MethodGet, "/api/activities?limit=1", testToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Activities) != 1 {
		t.Errorf("limit=1 len = %d, want 1", len(body.Activities))
	}
}

func TestRecoveryHandler(t *testing.T) {
	h, s := newTestServer(t)
	_ = s.UpsertSleep(store.SleepRow{Date: "2026-06-18", DurationS: i64(27000), Score: i64(82), RawJSON: "{}"})
	_ = s.UpsertRhr(store.RhrRow{Date: "2026-06-18", RestingHR: i64(47), RawJSON: "{}"})
	// 06-17: rhr only.
	_ = s.UpsertRhr(store.RhrRow{Date: "2026-06-17", RestingHR: i64(49), RawJSON: "{}"})

	rec := do(t, h, http.MethodGet, "/api/recovery", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body recoveryResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Recovery) != 2 {
		t.Fatalf("len = %d, want 2", len(body.Recovery))
	}
	d18 := body.Recovery[0]
	if d18.Date != "2026-06-18" || d18.Sleep == nil || d18.Sleep.Score == nil || *d18.Sleep.Score != 82 {
		t.Errorf("06-18 sleep wrong: %+v", d18)
	}
	if d18.HRV != nil || d18.BodyBattery != nil {
		t.Errorf("06-18 hrv/bb = %v/%v, want both null", d18.HRV, d18.BodyBattery)
	}
	if d18.RHR == nil || *d18.RHR.RestingHR != 47 {
		t.Errorf("06-18 rhr wrong: %+v", d18.RHR)
	}
	// days=1 clamps to the most-recent date.
	rec = do(t, h, http.MethodGet, "/api/recovery?days=1", testToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Recovery) != 1 || body.Recovery[0].Date != "2026-06-18" {
		t.Errorf("days=1 = %+v, want single 2026-06-18", body.Recovery)
	}
}

func sp(v string) *string  { return &v }
func fp(v float64) *float64 { return &v }

func TestStravaCallbackExchangesAndPersists(t *testing.T) {
	// Fake Strava token endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Errorf("path = %s, want /oauth/token", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token_type":"Bearer","access_token":"acc","refresh_token":"ref","expires_at":4102444800,"expires_in":21600,"scope":"read,activity:read_all","athlete":{"id":777}}`))
	}))
	defer srv.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "cb.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Persist the CSRF state the callback URL below carries (callback now validates it).
	if err := s.SaveOAuthState("xyz"); err != nil {
		t.Fatalf("SaveOAuthState: %v", err)
	}
	deps := Deps{
		Store:    s,
		Strava:   strava.NewWithBase("12345", "secret", "http://localhost:8080/api/strava/callback", srv.URL),
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) { return "ok", 0, nil, "ok", 0, nil },
	}
	h := NewRouter(deps)

	// Callback has NO auth header. The URL must carry &state=xyz (saved above).
	rec := do(t, h, http.MethodGet, "/api/strava/callback?code=the-code&scope=read,activity:read_all&state=xyz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "You can close this tab.") {
		t.Errorf("body = %q, want close-tab text", rec.Body.String())
	}
	tok, err := s.GetStravaTokens()
	if err != nil {
		t.Fatalf("tokens not persisted: %v", err)
	}
	if tok.AccessToken != "acc" || tok.RefreshToken != "ref" || tok.AthleteID != 777 {
		t.Errorf("tokens = %+v, want acc/ref/777", tok)
	}
}

func TestStravaCallbackAccessDenied(t *testing.T) {
	h, s := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/strava/callback?error=access_denied", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "failed") {
		t.Errorf("body = %q, want failure text", rec.Body.String())
	}
	// No tokens stored.
	if _, err := s.GetStravaTokens(); err != store.ErrNotFound {
		t.Errorf("GetStravaTokens err = %v, want ErrNotFound", err)
	}
}

func TestSyncHandler(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	gerr := "worker exit 1: re-run worker.py login"
	deps := Deps{
		Store:    s,
		Strava:   strava.NewWithBase("1", "x", "http://cb", "http://unused"),
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) {
			return "ok", 3, nil, "error", 0, &gerr
		},
	}
	h := NewRouter(deps)

	rec := do(t, h, http.MethodPost, "/api/sync", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body syncResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Strava.Status != "ok" || body.Strava.Synced != 3 {
		t.Errorf("strava = %+v, want ok/3", body.Strava)
	}
	if body.Garmin.Status != "error" || body.Garmin.Error == nil || *body.Garmin.Error != gerr {
		t.Errorf("garmin = %+v, want error with %q", body.Garmin, gerr)
	}
}

func TestSyncRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodPost, "/api/sync", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
