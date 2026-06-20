package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/store"
)

// fakeCoach is the injected api.Coach for handler tests.
type fakeCoach struct {
	parseErr   error
	genErr     error
	fitnessErr error
	lastWeek   string
	lastEdited *llm.CrossFitWeekParsed
	lastImage  string
}

func (f *fakeCoach) ParseCrossFit(ctx context.Context, weekStart, imagePath string) (llm.CrossFitWeekParsed, string, error) {
	f.lastWeek = weekStart
	f.lastImage = imagePath
	if f.parseErr != nil {
		return llm.CrossFitWeekParsed{}, "", f.parseErr
	}
	return llm.CrossFitWeekParsed{
		WeekStart: weekStart,
		Days:      []llm.CrossFitDay{{Date: weekStart, Dow: "Mon", HasCrossFit: true, Focus: "Squat", CNSLoad: "high", LegLoad: "high"}},
	}, `{"week_start":"` + weekStart + `"}`, nil
}

func (f *fakeCoach) GeneratePlan(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (llm.PlanParsed, string, string, error) {
	f.lastWeek = weekStart
	f.lastEdited = edited
	if f.genErr != nil {
		return llm.PlanParsed{}, "", "", f.genErr
	}
	return llm.PlanParsed{
		FitnessSummary: "ok read",
		WeeklyTargetKm: 20,
		Days:           []llm.PlanDay{{Date: weekStart, Dow: "Mon", RunType: "rest", DistanceKm: 0, OptionalIfCNS: false, Rationale: "rest"}},
		WeekRationale:  "para",
		OneFlag:        "flag",
	}, `{"context":"pack"}`, "claude-opus-4-8", nil
}

func (f *fakeCoach) Fitness(ctx context.Context) (metrics.FitnessMetrics, error) {
	if f.fitnessErr != nil {
		return metrics.FitnessMetrics{}, f.fitnessErr
	}
	return metrics.FitnessMetrics{WeeklyVolumeKm: 18.2, SafeWeeklyTargetKm: 20, RecoveryTrend: "improving"}, nil
}

func doBody(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProfileGetSeeded(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/profile", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body profileDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TargetWeeklyKm != 20 || body.ProgressionMode != "build" {
		t.Errorf("profile = %+v, want target 20 build", body)
	}
}

func TestProfileRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/profile", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestProfilePut(t *testing.T) {
	h, s := newTestServer(t)
	body := `{"target_weekly_km":25,"progression_mode":"hold","zone2_ceiling_bpm":140,"threshold_bpm":165,"max_hr_bpm":190,"run_constraints_json":"{\"crossfit_days\":[\"Mon\"]}","goal_text":"Build cardio"}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out profileDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.TargetWeeklyKm != 25 || out.ProgressionMode != "hold" || out.UpdatedAt == "" {
		t.Errorf("put resp = %+v, want target 25 hold updated_at set", out)
	}
	p, _ := s.GetAthleteProfile()
	if p.TargetWeeklyKm != 25 || p.GoalText != "Build cardio" {
		t.Errorf("stored = %+v, want target 25 goal set", p)
	}
}

func TestCrossfitParse(t *testing.T) {
	h, s := newTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("image", "schedule.png")
	_, _ = fw.Write([]byte("PNG"))
	_ = mw.WriteField("week_start", "2026-06-22")
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/crossfit/parse", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var week llm.CrossFitWeekParsed
	if err := json.Unmarshal(rec.Body.Bytes(), &week); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if week.WeekStart != "2026-06-22" || len(week.Days) != 1 {
		t.Errorf("week = %+v, want 2026-06-22 1 day", week)
	}
	if _, err := s.GetCrossFitWeek("2026-06-22"); err != nil {
		t.Errorf("crossfit week not stored: %v", err)
	}
}

func TestCrossfitParseMissingFile(t *testing.T) {
	h, _ := newTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("week_start", "2026-06-22")
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/crossfit/parse", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (missing file)", rec.Code)
	}
}

func TestPlanGenerate(t *testing.T) {
	h, s := newTestServer(t)
	_ = s.UpsertCrossFitWeek(store.CrossFitWeek{
		WeekStart:  "2026-06-22",
		ParsedJSON: `{"week_start":"2026-06-22","days":[]}`,
	})
	rec := doBody(t, h, http.MethodPost, "/api/plan/generate", testToken, `{"week_start":"2026-06-22"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body planResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID <= 0 || body.WeekStart != "2026-06-22" || body.GeneratedAt == "" {
		t.Errorf("plan resp = %+v, want id/week/generated_at set", body)
	}
	if body.WeeklyTargetKm != 20 || body.OneFlag != "flag" || len(body.Days) != 1 {
		t.Errorf("plan body = %+v", body)
	}
	got, err := s.GetLatestPlan("2026-06-22")
	if err != nil {
		t.Fatalf("plan not stored: %v", err)
	}
	if got.Model != "claude-opus-4-8" || got.ContextPackJSON == nil {
		t.Errorf("stored plan = %+v, want model + context pack", got)
	}
}

func TestPlanGenerateMissingWeek(t *testing.T) {
	h, _ := newTestServer(t)
	rec := doBody(t, h, http.MethodPost, "/api/plan/generate", testToken, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (missing week_start)", rec.Code)
	}
}

func TestPlanGenerateNoCrossFitWeek(t *testing.T) {
	// Cold start: a valid week_start but no stored CrossFit week and no edited
	// week in the body -> 404 (the user must parse a photo first).
	h, _ := newTestServer(t)
	rec := doBody(t, h, http.MethodPost, "/api/plan/generate", testToken, `{"week_start":"2026-06-29"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (no crossfit week stored), body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no crossfit week for that week") {
		t.Errorf("body = %q, want 'no crossfit week for that week'", rec.Body.String())
	}
}

func TestPlanGet(t *testing.T) {
	h, s := newTestServer(t)
	ctx := `{"c":"p"}`
	_, _ = s.InsertPlan(store.Plan{
		WeekStart: "2026-06-22", GeneratedAt: "2026-06-20T08:00:00Z", Status: "generated",
		PlanJSON: `{"fitness_summary":"f","weekly_target_km":20,"days":[{"date":"2026-06-22","dow":"Mon","run_type":"rest","distance_km":0,"pace_target":"","time_note":"","optional_if_cns":false,"rationale":"r"}],"week_rationale":"wr","one_flag":"of"}`,
		FitnessSummary: "f", ContextPackJSON: &ctx, Model: "claude-opus-4-8",
	})
	rec := do(t, h, http.MethodGet, "/api/plan?week=2026-06-22", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body planResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.WeekStart != "2026-06-22" || body.OneFlag != "of" || body.WeeklyTargetKm != 20 {
		t.Errorf("plan get = %+v", body)
	}
}

func TestPlanGetMissing(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/plan?week=2026-06-29", testToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no plan for week") {
		t.Errorf("body = %q, want 'no plan for week'", rec.Body.String())
	}
}

func TestFitnessHandler(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/fitness", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body metrics.FitnessMetrics
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.WeeklyVolumeKm != 18.2 || body.RecoveryTrend != "improving" {
		t.Errorf("fitness = %+v", body)
	}
}

func TestStravaConnectPersistsState(t *testing.T) {
	h, s := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/strava/connect", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body connectResp
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	u := body.AuthorizeURL
	idx := strings.Index(u, "state=")
	if idx < 0 {
		t.Fatalf("authorize URL has no state: %s", u)
	}
	state := u[idx+len("state="):]
	if amp := strings.IndexByte(state, '&'); amp >= 0 {
		state = state[:amp]
	}
	if state == "" {
		t.Fatal("empty state")
	}
	if err := s.ConsumeOAuthState(state); err != nil {
		t.Errorf("state %q not persisted by connect: %v", state, err)
	}
}

func TestStravaCallbackRejectsBadState(t *testing.T) {
	h, s := newTestServer(t)
	// No state was saved -> callback with an unknown state must NOT persist tokens.
	rec := do(t, h, http.MethodGet, "/api/strava/callback?code=the-code&state=forged", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (HTML page)", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "failed") {
		t.Errorf("body = %q, want failure text on bad state", rec.Body.String())
	}
	if _, err := s.GetStravaTokens(); err != store.ErrNotFound {
		t.Errorf("tokens persisted on forged state: %v", err)
	}
}
