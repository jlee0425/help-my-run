package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

// M5: the profile slice of the chat pack carries goals/week/guardrails so the
// coach persona sees the athlete's wizard answers.
func TestProfilePackCarriesV2Fields(t *testing.T) {
	pp := ProfilePack{
		TargetWeeklyKm:  25,
		ProgressionMode: "build",
		RunConstraints:  rawJSONOr("", `{}`),
		GoalText:        "engine",
		Goals:           rawJSONOr(`["crossfit","fitness"]`, `[]`),
		Week:            rawJSONOr(`{"runs_per_week":4,"crossfit_days":3,"rest_day":"monday"}`, `{}`),
		Guardrails:      rawJSONOr(`{"no_b2b_hard":true,"load_cap_55":true}`, `{}`),
	}
	b, err := json.Marshal(pp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, marker := range []string{`"no_b2b_hard":true`, `"rest_day":"monday"`, `"crossfit"`} {
		if !strings.Contains(s, marker) {
			t.Errorf("pack missing %s in %s", marker, s)
		}
	}
}

func TestRawJSONOrFallsBack(t *testing.T) {
	if got := string(rawJSONOr("", `[]`)); got != `[]` {
		t.Errorf("empty -> %q", got)
	}
	if got := string(rawJSONOr("{broken", `{}`)); got != `{}` {
		t.Errorf("invalid -> %q", got)
	}
	if got := string(rawJSONOr(`{"a":1}`, `{}`)); got != `{"a":1}` {
		t.Errorf("valid -> %q", got)
	}
}
