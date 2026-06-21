package progress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrendSummaryJSONTags(t *testing.T) {
	cur := 330.0
	base := 350.0
	delta := -20.0
	w := 345.0
	ts := TrendSummary{
		Key:           SignalPaceAtHR,
		Label:         "Pace @ Z2 HR",
		Unit:          "s/km",
		Current:       &cur,
		Baseline:      &base,
		DeltaAbs:      &delta,
		Direction:     DirectionDown,
		LowerIsBetter: true,
		Series:        []*float64{&base, nil, &w},
	}
	b, err := json.Marshal(ts)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	got := string(b)
	for _, k := range []string{
		`"key":"pace_at_hr"`, `"label":"Pace @ Z2 HR"`, `"unit":"s/km"`,
		`"current":330`, `"baseline":350`, `"delta_abs":-20`,
		`"direction":"down"`, `"lower_is_better":true`,
		`"series":[350,null,345]`,
	} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}

func TestProgressReportJSONTags(t *testing.T) {
	rep := ProgressReport{Weeks: 12, GeneratedAt: "2026-06-21T07:00:00Z", EnoughData: false, Signals: []TrendSummary{}}
	b, _ := json.Marshal(rep)
	got := string(b)
	for _, k := range []string{
		`"weeks":12`, `"generated_at":"2026-06-21T07:00:00Z"`,
		`"enough_data":false`, `"signals":[]`,
	} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}

func TestSignalConstants(t *testing.T) {
	if SignalPaceAtHR != "pace_at_hr" || SignalVo2max != "vo2max" ||
		SignalRestingHR != "resting_hr" || SignalHRVBaseline != "hrv_baseline" ||
		SignalWeeklyLoad != "weekly_load" {
		t.Errorf("signal key constants drifted from contract")
	}
	if DefaultWeeks != 12 || MinWeeks != 4 || MaxWeeks != 52 {
		t.Errorf("week bounds drifted: %d/%d/%d", DefaultWeeks, MinWeeks, MaxWeeks)
	}
}
