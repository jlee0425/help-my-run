package progress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProgressReadJSONTags(t *testing.T) {
	pr := ProgressRead{Text: "ok", Source: "ai"}
	b, _ := json.Marshal(pr)
	got := string(b)
	if !strings.Contains(got, `"text":"ok"`) || !strings.Contains(got, `"source":"ai"`) {
		t.Errorf("JSON = %s", got)
	}
}

func TestProgressReadInputJSONTags(t *testing.T) {
	in := ProgressReadInput{Weeks: 12, GoalText: "RX CrossFit engine", Signals: []TrendSummary{{Key: SignalPaceAtHR}}}
	b, _ := json.Marshal(in)
	got := string(b)
	for _, k := range []string{`"weeks":12`, `"goal_text":"RX CrossFit engine"`, `"signals":[`, `"key":"pace_at_hr"`} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}

func TestProgressReadPromptShape(t *testing.T) {
	for _, sub := range []string{"progress read", "pace_at_hr", "lower_is_better", `{"text":`, "RX CrossFit"} {
		if !strings.Contains(progressReadPrompt, sub) {
			t.Errorf("prompt missing %q", sub)
		}
	}
}

func TestFallbackProgressTextNotEnoughData(t *testing.T) {
	rep := ProgressReport{Weeks: 12, EnoughData: false}
	got := fallbackProgressText(rep)
	if !strings.Contains(got, "Not enough history") {
		t.Errorf("got %q, want not-enough-data sentence", got)
	}
}

func TestFallbackProgressTextImprovedClauses(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	rep := ProgressReport{
		Weeks:      12,
		EnoughData: true,
		Signals: []TrendSummary{
			{ // pace fell 350->330: improved (lowerIsBetter), formatted via metrics.FormatPace
				Key: SignalPaceAtHR, Label: "Pace @ Z2 HR", Unit: "s/km",
				Current: f(330), Baseline: f(350), DeltaAbs: f(-20),
				Direction: DirectionDown, LowerIsBetter: true,
			},
			{ // vo2max rose 50->52: improved (higher better)
				Key: SignalVo2max, Label: "VO2max", Unit: "ml/kg/min",
				Current: f(52), Baseline: f(50), DeltaAbs: f(2),
				Direction: DirectionUp, LowerIsBetter: false,
			},
			{ // resting HR rose 47->50: worsened (lowerIsBetter)
				Key: SignalRestingHR, Label: "Resting HR", Unit: "bpm",
				Current: f(50), Baseline: f(47), DeltaAbs: f(3),
				Direction: DirectionUp, LowerIsBetter: true,
			},
			{ // no data: skipped
				Key: SignalHRVBaseline, Label: "HRV baseline", Unit: "ms",
				Current: nil, Baseline: nil,
			},
		},
	}
	got := fallbackProgressText(rep)
	if !strings.Contains(got, "Over the last 12 weeks") {
		t.Errorf("missing prefix: %q", got)
	}
	if !strings.Contains(got, "Pace @ Z2 HR improved") {
		t.Errorf("pace clause wrong: %q", got)
	}
	// pace formatted M:SS/km via metrics.FormatPace
	if !strings.Contains(got, "5:50/km") || !strings.Contains(got, "5:30/km") {
		t.Errorf("pace not formatted: %q", got)
	}
	if !strings.Contains(got, "VO2max improved") {
		t.Errorf("vo2max clause wrong: %q", got)
	}
	if !strings.Contains(got, "Resting HR worsened") {
		t.Errorf("resting hr clause wrong: %q", got)
	}
	if strings.Contains(got, "HRV baseline") {
		t.Errorf("HRV (no data) should be skipped: %q", got)
	}
}
