package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

// M5: goals/week/guardrails JSON strings round-trip through PUT+GET /api/profile.
func TestProfileV2FieldsRoundTrip(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"target_weekly_km":25,"progression_mode":"build","run_constraints_json":"{}",` +
		`"goal_text":"engine","daily_run_time":"06:00","timezone":"UTC","agent_enabled":true,` +
		`"goals_json":"[\"crossfit\",\"fitness\"]",` +
		`"week_json":"{\"runs_per_week\":4,\"crossfit_days\":3,\"rest_day\":\"monday\"}",` +
		`"guardrails_json":"{\"no_b2b_hard\":true,\"protect_long_run\":true,\"easy_stays_easy\":true,\"hrv_backoff\":true,\"load_cap_55\":true}"}`

	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d (%s)", rec.Code, rec.Body.String())
	}

	rec = do(t, h, http.MethodGet, "/api/profile", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d", rec.Code)
	}
	var out profileDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.GoalsJSON != `["crossfit","fitness"]` {
		t.Errorf("goals_json = %q", out.GoalsJSON)
	}
	var week map[string]any
	if err := json.Unmarshal([]byte(out.WeekJSON), &week); err != nil || week["rest_day"] != "monday" {
		t.Errorf("week_json = %q (%v)", out.WeekJSON, err)
	}
	var gr map[string]bool
	if err := json.Unmarshal([]byte(out.GuardrailsJSON), &gr); err != nil || !gr["no_b2b_hard"] || !gr["load_cap_55"] {
		t.Errorf("guardrails_json = %q (%v)", out.GuardrailsJSON, err)
	}
}
