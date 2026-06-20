package api

import (
	"encoding/json"
	"net/http"
	"testing"
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
