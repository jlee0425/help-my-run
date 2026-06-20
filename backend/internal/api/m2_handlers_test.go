package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"help-my-run/backend/internal/agent"
	"help-my-run/backend/internal/store"
)

func TestProfileGetReturnsM2Defaults(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/profile", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out profileDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.DailyRunTime != "05:30" || out.Timezone != "UTC" || out.AgentEnabled != true {
		t.Errorf("M2 defaults = (%q,%q,%v), want (05:30,UTC,true)", out.DailyRunTime, out.Timezone, out.AgentEnabled)
	}
}

func TestProfilePutPersistsM2Fields(t *testing.T) {
	h, s := newTestServer(t)
	body := `{"target_weekly_km":25,"progression_mode":"hold","run_constraints_json":"{}","daily_run_time":"06:15","timezone":"Asia/Seoul","agent_enabled":false}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out profileDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.DailyRunTime != "06:15" || out.Timezone != "Asia/Seoul" || out.AgentEnabled != false {
		t.Errorf("resp M2 = %+v", out)
	}
	p, _ := s.GetAthleteProfile()
	if p.DailyRunTime != "06:15" || p.Timezone != "Asia/Seoul" || p.AgentEnabled != false {
		t.Errorf("stored M2 = %+v", p)
	}
}

func TestProfilePutDefaultsEmptyM2Fields(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"target_weekly_km":20,"progression_mode":"build","run_constraints_json":"{}","daily_run_time":"","timezone":"","agent_enabled":true}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out profileDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.DailyRunTime != "05:30" || out.Timezone != "UTC" {
		t.Errorf("defaults on empty = (%q,%q), want (05:30,UTC)", out.DailyRunTime, out.Timezone)
	}
}

func TestProfilePutRejectsBadRunTime(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"target_weekly_km":20,"progression_mode":"build","run_constraints_json":"{}","daily_run_time":"5:30","timezone":"UTC","agent_enabled":true}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for bad run time (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestProfilePutRejectsBadTimezone(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"target_weekly_km":20,"progression_mode":"build","run_constraints_json":"{}","daily_run_time":"06:00","timezone":"Mars/Phobos","agent_enabled":true}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for bad tz (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPushRegisterStoresToken(t *testing.T) {
	h, s := newTestServer(t)
	body := `{"expo_push_token":"ExponentPushToken[abc]","platform":"ios"}`
	rec := doBody(t, h, http.MethodPost, "/api/push/register", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out pushRegisterResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.ExpoPushToken != "ExponentPushToken[abc]" || out.Platform != "ios" || out.UpdatedAt == "" {
		t.Errorf("resp = %+v", out)
	}
	toks, _ := s.ListDeviceTokens()
	if len(toks) != 1 || toks[0].ExpoPushToken != "ExponentPushToken[abc]" {
		t.Errorf("stored tokens = %+v, want one", toks)
	}
}

func TestPushRegisterRejectsEmptyToken(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"expo_push_token":"","platform":"ios"}`
	rec := doBody(t, h, http.MethodPost, "/api/push/register", testToken, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPushRegisterRejectsBadPlatform(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"expo_push_token":"ExponentPushToken[x]","platform":"windows"}`
	rec := doBody(t, h, http.MethodPost, "/api/push/register", testToken, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPushRegisterRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"expo_push_token":"ExponentPushToken[x]","platform":"ios"}`
	rec := doBody(t, h, http.MethodPost, "/api/push/register", "", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func seedDecision(t *testing.T, s *store.Store, date string) {
	t.Helper()
	orig := `{"date":"` + date + `","dow":"Fri","run_type":"tempo","distance_km":6,"pace_target":"5:05/km","time_note":"~20:00 after CrossFit","optional_if_cns":false,"rationale":"Threshold work."}`
	adj := `{"date":"` + date + `","dow":"Fri","run_type":"easy","distance_km":4.5,"pace_target":"6:00/km","time_note":"~20:00 after CrossFit","optional_if_cns":true,"rationale":"Trimmed to easy."}`
	raw := `{"action":"SOFTEN"}`
	if err := s.UpsertDailyDecision(store.DailyDecision{
		Date:                date,
		ReadinessColor:      "amber",
		DriversJSON:         `{"date":"` + date + `","sleep_hours":6.1,"hrv_delta_pct":-17.8,"recovery_trend":"declining","data_complete":true,"reasons":["HRV -17.8% vs baseline"],"stale":true}`,
		OriginalSessionJSON: &orig,
		AdjustedSessionJSON: &adj,
		Action:              "SOFTEN",
		Rationale:           "HRV is 18% below baseline.",
		Source:              "ai",
		RawResponse:         &raw,
	}); err != nil {
		t.Fatalf("seedDecision: %v", err)
	}
}

func TestTodayReturnsDecision(t *testing.T) {
	h, s := newTestServer(t)
	seedDecision(t, s, "2026-06-20")
	rec := do(t, h, http.MethodGet, "/api/today?date=2026-06-20", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out todayResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Date != "2026-06-20" || out.ReadinessColor != "amber" || out.Action != "SOFTEN" || out.Source != "ai" {
		t.Errorf("today = %+v", out)
	}
	if out.Drivers.RecoveryTrend != "declining" || out.Drivers.DataComplete != true {
		t.Errorf("drivers = %+v", out.Drivers)
	}
	if len(out.Reasons) == 0 || out.Reasons[0] != "HRV -17.8% vs baseline" {
		t.Errorf("reasons = %v", out.Reasons)
	}
	if !out.Stale {
		t.Errorf("stale = false, want true (read back from drivers_json)")
	}
	if out.OriginalSession == nil || out.OriginalSession.RunType != "tempo" || out.OriginalSession.DistanceKm != 6 {
		t.Errorf("original = %+v", out.OriginalSession)
	}
	if out.EffectiveSession == nil || out.EffectiveSession.RunType != "easy" || out.EffectiveSession.DistanceKm != 4.5 {
		t.Errorf("effective = %+v", out.EffectiveSession)
	}
}

func TestTodayBadDate(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/today?date=2026-13-99", testToken)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestTodayNotFound(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/today?date=2026-06-20", testToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
	if !contains(rec.Body.String(), `"error":"no decision for date"`) {
		t.Errorf("body = %s, want no-decision error", rec.Body.String())
	}
}

func TestUndoTodayRevertsToOriginal(t *testing.T) {
	h, s := newTestServer(t)
	seedDecision(t, s, "2026-06-20")
	rec := doBody(t, h, http.MethodPost, "/api/today/undo?date=2026-06-20", testToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out todayResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Action != "STAND" {
		t.Errorf("action = %q, want STAND after undo", out.Action)
	}
	if out.EffectiveSession == nil || out.OriginalSession == nil ||
		out.EffectiveSession.RunType != out.OriginalSession.RunType ||
		out.EffectiveSession.DistanceKm != out.OriginalSession.DistanceKm {
		t.Errorf("after undo effective != original: eff=%+v orig=%+v", out.EffectiveSession, out.OriginalSession)
	}
	got, err := s.GetDailyDecision("2026-06-20")
	if err != nil {
		t.Fatalf("GetDailyDecision: %v", err)
	}
	if got.Action != "STAND" || got.AdjustedSessionJSON == nil || *got.AdjustedSessionJSON != *got.OriginalSessionJSON {
		t.Errorf("stored after undo = %+v", got)
	}
}

func TestUndoTodayNotFound(t *testing.T) {
	h, _ := newTestServer(t)
	rec := doBody(t, h, http.MethodPost, "/api/today/undo?date=2026-06-20", testToken, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestAgentRunInvokesAgentAndReturnsResult(t *testing.T) {
	_, s := newTestServer(t)
	fa := &fakeAgent{result: agent.RunResult{
		Date: "2026-06-20", Skipped: false, ReadinessColor: "amber",
		Action: "SOFTEN", Source: "ai", Stale: false, Pushed: true,
	}}
	h2 := NewRouter(Deps{
		Store: s, Strava: nil, APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) {
			return "ok", 0, nil, "ok", 0, nil
		},
		Coach: &fakeCoach{}, ImageDir: t.TempDir(), Agent: fa, Pusher: &fakePusher{},
	})
	rec := doBody(t, h2, http.MethodPost, "/api/agent/run?date=2026-06-20&force=true", testToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out agent.RunResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Date != "2026-06-20" || out.Action != "SOFTEN" || !out.Pushed {
		t.Errorf("run result = %+v", out)
	}
	if fa.lastDate != "2026-06-20" || fa.lastForce != true {
		t.Errorf("agent invoked with date=%q force=%v, want 2026-06-20/true", fa.lastDate, fa.lastForce)
	}
}

func TestAgentRunBadDate(t *testing.T) {
	h, _ := newTestServer(t)
	rec := doBody(t, h, http.MethodPost, "/api/agent/run?date=nope", testToken, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestTodayRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/today", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestRegisteredTokenIsDroppable(t *testing.T) {
	h, s := newTestServer(t)
	body := `{"expo_push_token":"ExponentPushToken[drop]","platform":"android"}`
	if rec := doBody(t, h, http.MethodPost, "/api/push/register", testToken, body); rec.Code != http.StatusOK {
		t.Fatalf("register status = %d", rec.Code)
	}
	toks, _ := s.ListDeviceTokens()
	if len(toks) != 1 {
		t.Fatalf("tokens after register = %d, want 1", len(toks))
	}
	if err := s.DeleteDeviceToken("ExponentPushToken[drop]"); err != nil {
		t.Fatalf("DeleteDeviceToken: %v", err)
	}
	toks, _ = s.ListDeviceTokens()
	if len(toks) != 0 {
		t.Errorf("tokens after drop = %d, want 0", len(toks))
	}
}
