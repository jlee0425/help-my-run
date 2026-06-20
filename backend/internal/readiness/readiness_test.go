package readiness

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

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

func mkDay(date string, durationS, sleepScore, hrvMs, rhr, bbHigh *int64) store.RecoveryDay {
	rd := store.RecoveryDay{Date: date}
	if durationS != nil || sleepScore != nil {
		rd.Sleep = &store.SleepFields{DurationS: durationS, Score: sleepScore}
	}
	if hrvMs != nil {
		rd.HRV = &store.HrvFields{LastNightAvgMs: hrvMs}
	}
	if bbHigh != nil {
		rd.BodyBattery = &store.BodyBatteryFields{High: bbHigh}
	}
	if rhr != nil {
		rd.RHR = &store.RhrFields{RestingHR: rhr}
	}
	return rd
}

func baselineRows(n int, hrvMs, rhr int64) []store.RecoveryDay {
	out := make([]store.RecoveryDay, 0, n)
	for i := 0; i < n; i++ {
		date := "2026-06-" + twoDigit(19-i)
		out = append(out, mkDay(date, i64p(27000), i64p(85), i64p(hrvMs), i64p(rhr), i64p(80)))
	}
	return out
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func TestAssess(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")

	tests := []struct {
		name      string
		lastNight store.RecoveryDay
		baseHRV   int64
		baseRHR   int64
		wantColor Color
	}{
		{
			name:      "all nominal -> GREEN",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorGreen,
		},
		{
			name:      "HRV -17% (red) -> RED",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(48), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
		{
			name:      "HRV -8% (one amber) -> AMBER",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(53), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorAmber,
		},
		{
			name:      "two amber signals (HRV -8% + RHR +5) -> RED (confirmation)",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(53), i64p(55), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
		{
			name:      "short sleep 4.5h -> RED",
			lastNight: mkDay("2026-06-20", i64p(16200), i64p(85), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
		{
			name:      "sleep 6.0h (amber) only -> AMBER",
			lastNight: mkDay("2026-06-20", i64p(21600), i64p(85), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorAmber,
		},
		{
			name:      "body battery 25 -> RED",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(25)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
		{
			name:      "body battery 45 (amber) only -> AMBER",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(45)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorAmber,
		},
		{
			name:      "sleep score 60 (amber) only -> AMBER",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(60), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorAmber,
		},
		{
			name:      "sleep score 45 -> RED",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(45), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := append([]store.RecoveryDay{tt.lastNight}, baselineRows(MinBaselineDays, tt.baseHRV, tt.baseRHR)...)
			r := Assess(rows, now)
			if r.Color != tt.wantColor {
				t.Errorf("Assess color = %q, want %q (drivers=%+v reasons=%v)", r.Color, tt.wantColor, r.Drivers, r.Reasons)
			}
			if r.Drivers.Date != "2026-06-20" {
				t.Errorf("Drivers.Date = %q, want 2026-06-20", r.Drivers.Date)
			}
			if r.Color != ColorGreen && len(r.Reasons) == 0 {
				t.Errorf("non-green color %q must have reasons", r.Color)
			}
		})
	}
}

func TestAssessEmpty(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")
	r := Assess(nil, now)
	if r.Color != ColorAmber {
		t.Errorf("Assess(nil) color = %q, want amber (conservative)", r.Color)
	}
	if r.Drivers.DataComplete {
		t.Errorf("Assess(nil) DataComplete = true, want false")
	}
}

func TestAssessMissingBaselineForcesAmber(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")
	rows := []store.RecoveryDay{
		mkDay("2026-06-20", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
		mkDay("2026-06-19", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
	}
	r := Assess(rows, now)
	if r.Drivers.DataComplete {
		t.Errorf("DataComplete = true with <MinBaselineDays prior days, want false")
	}
	if r.Color != ColorAmber {
		t.Errorf("color = %q, want amber when baseline missing", r.Color)
	}
	if r.Drivers.HRVDeltaPct != nil {
		t.Errorf("HRVDeltaPct = %v, want nil when baseline unavailable", *r.Drivers.HRVDeltaPct)
	}
}

func TestAssessDriverNumbers(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")
	rows := []store.RecoveryDay{
		mkDay("2026-06-20", i64p(21960), i64p(62), i64p(48), i64p(54), i64p(61)), // 6.1h
		mkDay("2026-06-19", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
		mkDay("2026-06-18", i64p(27000), i64p(85), i64p(59), i64p(50), i64p(80)),
		mkDay("2026-06-17", i64p(27000), i64p(85), i64p(58), i64p(51), i64p(80)),
	}
	r := Assess(rows, now)
	d := r.Drivers
	if d.SleepHours == nil || math.Abs(*d.SleepHours-6.1) > 1e-9 {
		t.Errorf("SleepHours = %v, want 6.1", d.SleepHours)
	}
	if d.SleepScore == nil || *d.SleepScore != 62 {
		t.Errorf("SleepScore = %v, want 62", d.SleepScore)
	}
	if d.HRVLastNightMs == nil || *d.HRVLastNightMs != 48 {
		t.Errorf("HRVLastNightMs = %v, want 48", d.HRVLastNightMs)
	}
	if d.HRVBaselineMs == nil || math.Abs(*d.HRVBaselineMs-58.333333333333336) > 1e-9 {
		t.Errorf("HRVBaselineMs = %v, want ~58.333", d.HRVBaselineMs)
	}
	if d.HRVDeltaPct == nil || math.Abs(*d.HRVDeltaPct-(-17.714285714285715)) > 1e-9 {
		t.Errorf("HRVDeltaPct = %v, want ~-17.714", d.HRVDeltaPct)
	}
	if d.RHRLastNight == nil || *d.RHRLastNight != 54 {
		t.Errorf("RHRLastNight = %v, want 54", d.RHRLastNight)
	}
	if d.RHRBaseline == nil || math.Abs(*d.RHRBaseline-50.333333333333336) > 1e-9 {
		t.Errorf("RHRBaseline = %v, want ~50.333", d.RHRBaseline)
	}
	if d.RHRDeltaBpm == nil || math.Abs(*d.RHRDeltaBpm-3.6666666666666643) > 1e-9 {
		t.Errorf("RHRDeltaBpm = %v, want ~3.667", d.RHRDeltaBpm)
	}
	if d.BodyBatteryHigh == nil || *d.BodyBatteryHigh != 61 {
		t.Errorf("BodyBatteryHigh = %v, want 61", d.BodyBatteryHigh)
	}
	if !d.DataComplete {
		t.Errorf("DataComplete = false, want true (last night + 3 baseline days present)")
	}
}

func TestAssessThenFallbackEndToEnd(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")
	rows := []store.RecoveryDay{
		mkDay("2026-06-20", i64p(21960), i64p(62), i64p(53), i64p(50), i64p(61)), // 6.1h, score 62, hrv 53
		mkDay("2026-06-19", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
		mkDay("2026-06-18", i64p(27000), i64p(85), i64p(59), i64p(50), i64p(80)),
		mkDay("2026-06-17", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
	}
	r := Assess(rows, now)

	// Single-amber fixture lands deterministically on AMBER.
	single := []store.RecoveryDay{
		mkDay("2026-06-20", i64p(27000), i64p(62), i64p(58), i64p(50), i64p(80)), // only sleep score 62 amber
		mkDay("2026-06-19", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
		mkDay("2026-06-18", i64p(27000), i64p(85), i64p(59), i64p(50), i64p(80)),
		mkDay("2026-06-17", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
	}
	ra := Assess(single, now)
	if ra.Color != ColorAmber {
		t.Fatalf("single-amber fixture color = %q, want amber", ra.Color)
	}

	session := &FallbackSession{
		Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6,
		PaceTarget: "5:05/km", TimeNote: "~20:00 after CrossFit",
	}
	dec := Fallback(ra.Color, session, "6:00/km")
	if dec.Action != FbSoften {
		t.Errorf("Action = %q, want SOFTEN", dec.Action)
	}
	if dec.Adjusted == nil || dec.Adjusted.DistanceKm != 4.5 || dec.Adjusted.PaceTarget != "6:00/km" {
		t.Errorf("Adjusted = %+v, want 4.5km @ 6:00/km", dec.Adjusted)
	}
	if r.Color == ColorRed {
		mv := Fallback(r.Color, session, "6:00/km")
		if mv.Action != FbMove || mv.Adjusted == nil || mv.Adjusted.RunType != "recovery" {
			t.Errorf("RED fallback = %+v, want MOVE to recovery", mv)
		}
	}
}

func mustNow(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}
