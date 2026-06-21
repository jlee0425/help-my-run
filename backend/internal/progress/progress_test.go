package progress

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }

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

func TestWeekBucketsOldestFirstContiguous(t *testing.T) {
	now := time.Date(2026, 6, 21, 7, 0, 0, 0, time.UTC)
	bs := weekBuckets(3, now)
	if len(bs) != 3 {
		t.Fatalf("len = %d, want 3", len(bs))
	}
	// oldest-first: last bucket ends at now.
	if !bs[2].end.Equal(now) {
		t.Errorf("last bucket end = %v, want now %v", bs[2].end, now)
	}
	// contiguous half-open: each start == previous end.
	if !bs[1].start.Equal(bs[0].end) || !bs[2].start.Equal(bs[1].end) {
		t.Errorf("buckets not contiguous: %+v", bs)
	}
	// 7-day width.
	if bs[2].end.Sub(bs[2].start) != 7*24*time.Hour {
		t.Errorf("bucket width = %v, want 168h", bs[2].end.Sub(bs[2].start))
	}
}

func TestSummarizeDeltasAndDirection(t *testing.T) {
	tests := []struct {
		name          string
		series        []*float64
		lowerIsBetter bool
		wantCur       *float64
		wantBase      *float64
		wantDelta     *float64
		wantDir       TrendDirection
	}{
		{
			name:          "pace falling -> down (lower is better)",
			series:        []*float64{f64(350), nil, f64(330)},
			lowerIsBetter: true,
			wantCur:       f64(330), wantBase: f64(350), wantDelta: f64(-20), wantDir: DirectionDown,
		},
		{
			name:          "vo2max rising -> up",
			series:        []*float64{f64(50), f64(51), f64(52)},
			lowerIsBetter: false,
			wantCur:       f64(52), wantBase: f64(50), wantDelta: f64(2), wantDir: DirectionUp,
		},
		{
			name:          "flat within rel deadband",
			series:        []*float64{f64(50), f64(50.5)},
			lowerIsBetter: false,
			wantCur:       f64(50.5), wantBase: f64(50), wantDelta: f64(0.5), wantDir: DirectionFlat,
		},
		{
			name:          "all nil -> nil summary, flat",
			series:        []*float64{nil, nil},
			lowerIsBetter: false,
			wantCur:       nil, wantBase: nil, wantDelta: nil, wantDir: DirectionFlat,
		},
		{
			name:          "single point -> cur==base, zero delta, flat",
			series:        []*float64{nil, f64(42), nil},
			lowerIsBetter: false,
			wantCur:       f64(42), wantBase: f64(42), wantDelta: f64(0), wantDir: DirectionFlat,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cur, base, delta, dir := summarize(tt.series, tt.lowerIsBetter, false)
			eqp := func(a, b *float64) bool {
				if a == nil || b == nil {
					return a == b
				}
				return *a == *b
			}
			if !eqp(cur, tt.wantCur) || !eqp(base, tt.wantBase) || !eqp(delta, tt.wantDelta) {
				t.Errorf("cur/base/delta = %v/%v/%v, want %v/%v/%v", cur, base, delta, tt.wantCur, tt.wantBase, tt.wantDelta)
			}
			if dir != tt.wantDir {
				t.Errorf("dir = %q, want %q", dir, tt.wantDir)
			}
		})
	}
}

func TestSummarizePaceEpsDeadband(t *testing.T) {
	// pace signal (isPace=true): 350 -> 350.3 is within 0.5 eps -> flat.
	_, _, _, dir := summarize([]*float64{f64(350), f64(350.3)}, true, true)
	if dir != DirectionFlat {
		t.Errorf("dir = %q, want flat (within paceEps)", dir)
	}
	// 350 -> 348 exceeds eps, value fell -> down.
	_, _, _, dir = summarize([]*float64{f64(350), f64(348)}, true, true)
	if dir != DirectionDown {
		t.Errorf("dir = %q, want down", dir)
	}
}
