package coach

import (
	"strings"
	"testing"
)

func TestBuildStage1PromptReferencesImage(t *testing.T) {
	p := buildStage1Prompt("/data/crossfit/2026-06-22.jpg", "2026-06-22")
	if !strings.Contains(p, "/data/crossfit/2026-06-22.jpg") {
		t.Errorf("prompt missing image path:\n%s", p)
	}
	if !strings.Contains(p, "2026-06-22") {
		t.Errorf("prompt missing week_start")
	}
	for _, k := range []string{"week_start", "has_crossfit", "cns_load", "leg_load"} {
		if !strings.Contains(p, k) {
			t.Errorf("prompt missing key hint %q", k)
		}
	}
	if !strings.Contains(strings.ToLower(p), "json") {
		t.Errorf("prompt does not ask for JSON")
	}
	if !strings.Contains(strings.ToLower(p), "read") {
		t.Errorf("prompt does not instruct read")
	}
}

func TestCoachBrainPromptHasGuidance(t *testing.T) {
	if !strings.Contains(strings.ToLower(coachBrainPrompt), "coach") {
		t.Errorf("coachBrainPrompt missing coach framing")
	}
	for _, k := range []string{"weekly_target_km", "run_type", "optional_if_cns", "week_rationale", "one_flag"} {
		if !strings.Contains(coachBrainPrompt, k) {
			t.Errorf("coachBrainPrompt missing output key %q", k)
		}
	}
	for _, want := range []string{"10%", "CNS", "Thursday", "JSON"} {
		if !strings.Contains(coachBrainPrompt, want) {
			t.Errorf("coachBrainPrompt missing rule mention %q", want)
		}
	}
}
