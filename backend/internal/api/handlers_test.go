package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/store"
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
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string) {
			return "ok", 0, nil
		},
		Coach:    &fakeCoach{},
		ImageDir: t.TempDir(),
		Agent:    &fakeAgent{},
		Pusher:   &fakePusher{},
		Progress: &fakeProgress{},
		Streams:  &fakeStreams{},
		Chat:     &fakeChat{},
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
	// M4: connected is derived from the garmin sync_log status, NOT recovery-data
	// presence — seed a SUCCESSFUL garmin sync_log row + one recovery day so the
	// counts are non-zero.
	now := "2026-06-23T05:00:00Z"
	_ = s.UpdateSyncLog(store.SyncLog{
		Source: "garmin", LastSyncedAt: &now, LastRunAt: &now, Status: "ok", Error: nil,
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
	if !body.Garmin.Connected {
		t.Errorf("garmin = %+v, want connected (sync_log status ok)", body.Garmin)
	}
	if body.Garmin.Status != "ok" {
		t.Errorf("garmin.status = %q, want ok", body.Garmin.Status)
	}
	if body.Counts.RecoveryDays != 1 {
		t.Errorf("counts.recovery_days = %d, want 1", body.Counts.RecoveryDays)
	}
}

func TestRemovedStravaRoutesReturn404(t *testing.T) {
	h, _ := newTestServer(t)
	for _, p := range []string{"/api/strava/connect", "/api/strava/callback"} {
		rec := do(t, h, http.MethodGet, p, testToken)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 (route removed)", p, rec.Code)
		}
	}
}

func i64(v int64) *int64 { return &v }

func TestActivitiesHandler(t *testing.T) {
	h, s := newTestServer(t)
	_ = s.UpsertActivity(store.Activity{
		ActivityID: 11, Name: "A", Type: "Run", SportType: sp("Run"),
		StartTime: "2026-06-18T06:00:00Z", StartTimeLocal: sp("2026-06-18T08:00:00"),
		DistanceM: 10000, MovingTimeS: 3000, ElapsedTimeS: 3050,
		AvgHR: fp(150), RawJSON: "{}",
	})
	_ = s.UpsertActivity(store.Activity{
		ActivityID: 12, Name: "B", Type: "Run", SportType: nil,
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
	if body.Activities[0].ActivityID != 11 {
		t.Errorf("first id = %d, want 11", body.Activities[0].ActivityID)
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

func sp(v string) *string   { return &v }
func fp(v float64) *float64 { return &v }

func TestSyncHandler(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	deps := Deps{
		Store:    s,
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string) {
			return "ok", 3, nil
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
	if body.Garmin.Status != "ok" || body.Garmin.Synced != 3 {
		t.Errorf("garmin = %+v, want ok/3", body.Garmin)
	}
}

func TestSyncRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodPost, "/api/sync", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
