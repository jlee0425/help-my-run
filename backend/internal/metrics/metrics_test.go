package metrics

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
)

func TestFitnessMetricsJSONTags(t *testing.T) {
	m := FitnessMetrics{
		WeeklyVolumeKm:     18.2,
		FourWeekAvgKm:      17.4,
		AcuteChronicRatio:  1.05,
		EasyPace:           "6:00/km",
		ThresholdPace:      "5:05/km",
		RecoveryTrend:      "improving",
		SafeWeeklyTargetKm: 20.0,
		IsCutbackWeek:      false,
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	got := string(b)
	wantKeys := []string{
		`"weekly_volume_km":18.2`,
		`"four_week_avg_km":17.4`,
		`"acute_chronic_ratio":1.05`,
		`"easy_pace":"6:00/km"`,
		`"threshold_pace":"5:05/km"`,
		`"recovery_trend":"improving"`,
		`"safe_weekly_target_km":20`,
		`"is_cutback_week":false`,
	}
	for _, k := range wantKeys {
		if !contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestFormatPace(t *testing.T) {
	tests := []struct {
		name     string
		secPerKm float64
		want     string
	}{
		{"zero -> empty", 0, ""},
		{"negative -> empty", -5, ""},
		{"6:00", 360, "6:00/km"},
		{"5:05", 305, "5:05/km"},
		{"rounds to nearest second", 305.4, "5:05/km"},
		{"rounds up", 305.6, "5:06/km"},
		{"single-digit seconds zero-padded", 363, "6:03/km"},
		{"carry to next minute", 359.6, "6:00/km"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPace(tt.secPerKm); got != tt.want {
				t.Errorf("formatPace(%v) = %q, want %q", tt.secPerKm, got, tt.want)
			}
		})
	}
}

func TestIsRun(t *testing.T) {
	tests := []struct {
		typ  string
		want bool
	}{
		{"Run", true},
		{"TrailRun", true},
		{"VirtualRun", true},
		{"Ride", false},
		{"Workout", false},
		{"WeightTraining", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isRun(tt.typ); got != tt.want {
			t.Errorf("isRun(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestDistanceKmInWindow(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z") // Monday
	acts := []store.Activity{
		// 2 days ago, run, 10 km -> in 7-day window.
		{StravaID: 1, Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000},
		// 6 days ago, trail run, 5 km -> in 7-day window.
		{StravaID: 2, Type: "TrailRun", StartTime: "2026-06-16T18:00:00Z", DistanceM: 5000},
		// 10 days ago, run, 8 km -> outside 7-day, inside 28-day.
		{StravaID: 3, Type: "Run", StartTime: "2026-06-12T06:00:00Z", DistanceM: 8000},
		// 2 days ago but a Ride -> excluded (not a run).
		{StravaID: 4, Type: "Ride", StartTime: "2026-06-20T07:00:00Z", DistanceM: 40000},
		// unparseable start -> skipped.
		{StravaID: 5, Type: "Run", StartTime: "not-a-time", DistanceM: 99000},
	}
	// 7-day window: [now-7d, now] -> acts 1 (10) + 2 (5) = 15 km.
	if got := distanceKmInWindow(acts, now.AddDate(0, 0, -7), now); got != 15 {
		t.Errorf("7-day distance = %v, want 15", got)
	}
	// 28-day window: acts 1+2+3 = 23 km.
	if got := distanceKmInWindow(acts, now.AddDate(0, 0, -28), now); got != 23 {
		t.Errorf("28-day distance = %v, want 23", got)
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}

// nearlyEqual compares accumulated-division float64s with an epsilon to avoid
// brittle exact-equality failures (e.g. 18200/1000 vs 18.2). Defined once here;
// reused by the volume/average assertions below.
func nearlyEqual(got, want float64) bool { return math.Abs(got-want) <= 1e-9 }

func TestWeeklyVolumeKm(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")
	acts := []store.Activity{
		{StravaID: 1, Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000},
		{StravaID: 2, Type: "Run", StartTime: "2026-06-17T06:00:00Z", DistanceM: 8200},
		{StravaID: 3, Type: "Run", StartTime: "2026-06-10T06:00:00Z", DistanceM: 6000}, // >7d ago
	}
	if got := weeklyVolumeKm(acts, now); !nearlyEqual(got, 18.2) {
		t.Errorf("weeklyVolumeKm = %v, want 18.2", got)
	}
	// No runs in window -> 0.
	if got := weeklyVolumeKm(nil, now); !nearlyEqual(got, 0) {
		t.Errorf("weeklyVolumeKm(nil) = %v, want 0", got)
	}
}

func TestFourWeekAvgKm(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")
	acts := []store.Activity{
		{StravaID: 1, Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000}, // wk0
		{StravaID: 2, Type: "Run", StartTime: "2026-06-14T06:00:00Z", DistanceM: 6000},  // wk1
		{StravaID: 3, Type: "Run", StartTime: "2026-06-07T06:00:00Z", DistanceM: 8000},  // wk2
		{StravaID: 4, Type: "Run", StartTime: "2026-05-30T06:00:00Z", DistanceM: 8000},  // wk3 (23 days ago - in)
		{StravaID: 5, Type: "Run", StartTime: "2026-05-10T06:00:00Z", DistanceM: 50000}, // >28d ago, excluded
	}
	// 28-day total = 10+6+8+8 = 32 km; /4 = 8.0.
	if got := fourWeekAvgKm(acts, now); !nearlyEqual(got, 8.0) {
		t.Errorf("fourWeekAvgKm = %v, want 8.0", got)
	}
}

func TestAcuteChronicRatio(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")
	tests := []struct {
		name string
		acts []store.Activity
		want float64
	}{
		{
			name: "balanced ~1.05",
			acts: []store.Activity{
				{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000},
				{Type: "Run", StartTime: "2026-06-17T06:00:00Z", DistanceM: 8000},
				{Type: "Run", StartTime: "2026-06-13T06:00:00Z", DistanceM: 17400},
				{Type: "Run", StartTime: "2026-06-06T06:00:00Z", DistanceM: 17000},
				{Type: "Run", StartTime: "2026-05-30T06:00:00Z", DistanceM: 16000},
			},
			// acute=18; 28d total=68.4; chronic=17.1; 18/17.1=1.0526 -> 1.05.
			want: 1.05,
		},
		{
			name: "no chronic baseline -> 0",
			acts: nil,
			want: 0,
		},
		{
			name: "spike 2.0",
			acts: []store.Activity{
				{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 20000},
				{Type: "Run", StartTime: "2026-06-05T06:00:00Z", DistanceM: 20000},
			},
			want: 2.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acuteChronicRatio(tt.acts, now); got != tt.want {
				t.Errorf("acuteChronicRatio = %v, want %v", got, tt.want)
			}
		})
	}
}
