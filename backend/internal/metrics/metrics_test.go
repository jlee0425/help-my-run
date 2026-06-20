package metrics

import (
	"encoding/json"
	"testing"
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
