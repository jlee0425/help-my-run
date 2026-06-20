package readiness

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"help-my-run/backend/internal/store"
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

func i64p(v int64) *int64 { return &v }

func TestSleepHours(t *testing.T) {
	if got := sleepHours(nil); got != nil {
		t.Errorf("sleepHours(nil) = %v, want nil", got)
	}
	if got := sleepHours(&store.SleepFields{}); got != nil {
		t.Errorf("sleepHours(empty) = %v, want nil", got)
	}
	got := sleepHours(&store.SleepFields{DurationS: i64p(27000)})
	if got == nil || math.Abs(*got-7.5) > 1e-9 {
		t.Errorf("sleepHours(27000) = %v, want 7.5", got)
	}
}

func TestBaseline(t *testing.T) {
	if got, ok := baseline([]*int64{i64p(50), i64p(52)}); ok || got != 0 {
		t.Errorf("baseline(2 vals) = (%v,%v), want (0,false)", got, ok)
	}
	got, ok := baseline([]*int64{i64p(48), i64p(50), i64p(52)})
	if !ok || math.Abs(got-50.0) > 1e-9 {
		t.Errorf("baseline(3 vals) = (%v,%v), want (50,true)", got, ok)
	}
	got, ok = baseline([]*int64{i64p(48), nil, i64p(50), i64p(52), nil})
	if !ok || math.Abs(got-50.0) > 1e-9 {
		t.Errorf("baseline(with nils) = (%v,%v), want (50,true)", got, ok)
	}
	if _, ok := baseline([]*int64{nil, nil, nil}); ok {
		t.Errorf("baseline(all nil) ok = true, want false")
	}
}

func TestPctDelta(t *testing.T) {
	got := pctDelta(48, 58.4)
	if math.Abs(got-(-17.808219178082192)) > 1e-9 {
		t.Errorf("pctDelta(48,58.4) = %v, want ~-17.808", got)
	}
	if got := pctDelta(48, 0); got != 0 {
		t.Errorf("pctDelta(48,0) = %v, want 0", got)
	}
}

func TestBpmDelta(t *testing.T) {
	if got := bpmDelta(54, 50.2); math.Abs(got-3.8) > 1e-9 {
		t.Errorf("bpmDelta(54,50.2) = %v, want 3.8", got)
	}
}
