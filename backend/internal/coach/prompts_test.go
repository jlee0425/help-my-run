package coach

import (
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/readiness"
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

func TestDailyAdjustPromptShape(t *testing.T) {
	if !strings.Contains(dailyAdjustPrompt, "STAND") ||
		!strings.Contains(dailyAdjustPrompt, "SOFTEN") ||
		!strings.Contains(dailyAdjustPrompt, "MOVE") ||
		!strings.Contains(dailyAdjustPrompt, "REST_DAY") {
		t.Errorf("dailyAdjustPrompt missing an action token")
	}
	if !strings.Contains(dailyAdjustPrompt, "adjusted_session") || !strings.Contains(dailyAdjustPrompt, "rationale") {
		t.Errorf("dailyAdjustPrompt missing output keys")
	}
	if !strings.Contains(dailyAdjustPrompt, "single JSON object") {
		t.Errorf("dailyAdjustPrompt missing JSON-only instruction")
	}
}

func TestBuildDailyAdjustInput(t *testing.T) {
	rd := readiness.Readiness{
		Color:   readiness.ColorAmber,
		Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true},
		Reasons: []string{"HRV -17.8% vs baseline"},
	}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6}
	in := buildDailyAdjustInput("2026-06-20", rd, today,
		metrics.FitnessMetrics{EasyPace: "6:00/km"},
		ProfilePack{TargetWeeklyKm: 20}, nil, "build week")
	if in.Date != "2026-06-20" {
		t.Errorf("date = %q", in.Date)
	}
	if in.Readiness.Color != readiness.ColorAmber {
		t.Errorf("readiness color = %q", in.Readiness.Color)
	}
	if in.TodaySession == nil || in.TodaySession.RunType != "tempo" {
		t.Errorf("today session = %+v", in.TodaySession)
	}
	if in.Metrics.EasyPace != "6:00/km" {
		t.Errorf("metrics easy pace = %q", in.Metrics.EasyPace)
	}
	if in.Profile.TargetWeeklyKm != 20 {
		t.Errorf("profile target = %v", in.Profile.TargetWeeklyKm)
	}
	if in.WeekRationale != "build week" {
		t.Errorf("week rationale = %q", in.WeekRationale)
	}
}
