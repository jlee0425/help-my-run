package readiness

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestColorConstants(t *testing.T) {
	if ColorGreen != "green" || ColorAmber != "amber" || ColorRed != "red" {
		t.Errorf("colors = (%q,%q,%q), want (green,amber,red)", ColorGreen, ColorAmber, ColorRed)
	}
}

func TestThresholdConstants(t *testing.T) {
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"RedSleepHours", RedSleepHours, 5.0},
		{"AmberSleepHours", AmberSleepHours, 6.5},
		{"RedSleepScore", float64(RedSleepScore), 50},
		{"AmberSleepScore", float64(AmberSleepScore), 65},
		{"RedHRVDropPct", RedHRVDropPct, -15.0},
		{"AmberHRVDropPct", AmberHRVDropPct, -7.0},
		{"RedRHRRiseBpm", RedRHRRiseBpm, 7.0},
		{"AmberRHRRiseBpm", AmberRHRRiseBpm, 4.0},
		{"RedBodyBattery", float64(RedBodyBattery), 30},
		{"AmberBodyBattery", float64(AmberBodyBattery), 50},
		{"BaselineWindowDays", float64(BaselineWindowDays), 14},
		{"MinBaselineDays", float64(MinBaselineDays), 3},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestReadinessDriversJSONTags(t *testing.T) {
	f := 6.1
	i := int64(48)
	d := ReadinessDrivers{
		Date:            "2026-06-20",
		SleepHours:      &f,
		SleepScore:      &i,
		HRVLastNightMs:  &i,
		HRVBaselineMs:   &f,
		HRVDeltaPct:     &f,
		RHRLastNight:    &i,
		RHRBaseline:     &f,
		RHRDeltaBpm:     &f,
		BodyBatteryHigh: &i,
		RecoveryTrend:   "declining",
		DataComplete:    true,
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	got := string(b)
	wantKeys := []string{
		`"date":`, `"sleep_hours":`, `"sleep_score":`, `"hrv_last_night_ms":`,
		`"hrv_baseline_ms":`, `"hrv_delta_pct":`, `"rhr_last_night":`, `"rhr_baseline":`,
		`"rhr_delta_bpm":`, `"body_battery_high":`, `"recovery_trend":"declining"`,
		`"data_complete":true`,
	}
	for _, k := range wantKeys {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}

	empty := ReadinessDrivers{Date: "2026-06-20", RecoveryTrend: "stable"}
	eb, _ := json.Marshal(empty)
	if !strings.Contains(string(eb), `"sleep_hours":null`) {
		t.Errorf("nil SleepHours = %s, want sleep_hours:null", eb)
	}
}

func TestReadinessJSONTags(t *testing.T) {
	r := Readiness{
		Color:   ColorAmber,
		Drivers: ReadinessDrivers{Date: "2026-06-20", RecoveryTrend: "stable"},
		Reasons: []string{"HRV -17.8% vs baseline"},
	}
	b, _ := json.Marshal(r)
	got := string(b)
	for _, k := range []string{`"color":"amber"`, `"drivers":`, `"reasons":["HRV -17.8% vs baseline"]`} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}
