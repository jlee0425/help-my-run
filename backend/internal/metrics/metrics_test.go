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
		{ActivityID: 1, Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000},
		// 6 days ago, trail run, 5 km -> in 7-day window.
		{ActivityID: 2, Type: "TrailRun", StartTime: "2026-06-16T18:00:00Z", DistanceM: 5000},
		// 10 days ago, run, 8 km -> outside 7-day, inside 28-day.
		{ActivityID: 3, Type: "Run", StartTime: "2026-06-12T06:00:00Z", DistanceM: 8000},
		// 2 days ago but a Ride -> excluded (not a run).
		{ActivityID: 4, Type: "Ride", StartTime: "2026-06-20T07:00:00Z", DistanceM: 40000},
		// unparseable start -> skipped.
		{ActivityID: 5, Type: "Run", StartTime: "not-a-time", DistanceM: 99000},
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
		{ActivityID: 1, Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000},
		{ActivityID: 2, Type: "Run", StartTime: "2026-06-17T06:00:00Z", DistanceM: 8200},
		{ActivityID: 3, Type: "Run", StartTime: "2026-06-10T06:00:00Z", DistanceM: 6000}, // >7d ago
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
		{ActivityID: 1, Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000}, // wk0
		{ActivityID: 2, Type: "Run", StartTime: "2026-06-14T06:00:00Z", DistanceM: 6000},  // wk1
		{ActivityID: 3, Type: "Run", StartTime: "2026-06-07T06:00:00Z", DistanceM: 8000},  // wk2
		{ActivityID: 4, Type: "Run", StartTime: "2026-05-30T06:00:00Z", DistanceM: 8000},  // wk3 (23 days ago - in)
		{ActivityID: 5, Type: "Run", StartTime: "2026-05-10T06:00:00Z", DistanceM: 50000}, // >28d ago, excluded
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

func TestPaceEstimates(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")

	t.Run("no runs -> empty", func(t *testing.T) {
		easy, thr := paceEstimates(nil, now)
		if easy != "" || thr != "" {
			t.Errorf("paceEstimates(nil) = (%q,%q), want empty", easy, thr)
		}
	})

	t.Run("mixed runs", func(t *testing.T) {
		acts := []store.Activity{
			// easy ~6:00/km (360 s/km): 5km in 1800s.
			{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 5000, MovingTimeS: 1800},
			// easy ~6:20/km (380 s/km): 5km in 1900s.
			{Type: "Run", StartTime: "2026-06-18T06:00:00Z", DistanceM: 5000, MovingTimeS: 1900},
			// recovery ~6:40/km (400 s/km): 5km in 2000s.
			{Type: "Run", StartTime: "2026-06-16T06:00:00Z", DistanceM: 5000, MovingTimeS: 2000},
			// tempo ~5:05/km (305 s/km): 5km in 1525s. -> fastest.
			{Type: "Run", StartTime: "2026-06-14T06:00:00Z", DistanceM: 5000, MovingTimeS: 1525},
			// a Ride -> excluded.
			{Type: "Ride", StartTime: "2026-06-19T06:00:00Z", DistanceM: 20000, MovingTimeS: 3000},
			// zero distance -> skipped.
			{Type: "Run", StartTime: "2026-06-15T06:00:00Z", DistanceM: 0, MovingTimeS: 100},
		}
		// Qualifying sec/km sorted: [305, 360, 380, 400]. median = (360+380)/2 = 370 -> 6:10/km.
		// fastest 25% = ceil(4*0.25)=1 -> [305] -> median 305 -> 5:05/km.
		easy, thr := paceEstimates(acts, now)
		if easy != "6:10/km" {
			t.Errorf("easy = %q, want 6:10/km", easy)
		}
		if thr != "5:05/km" {
			t.Errorf("threshold = %q, want 5:05/km", thr)
		}
	})

	t.Run("threshold never slower than easy", func(t *testing.T) {
		acts := []store.Activity{
			{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 5000, MovingTimeS: 1800},
			{Type: "Run", StartTime: "2026-06-18T06:00:00Z", DistanceM: 5000, MovingTimeS: 1800},
		}
		easy, thr := paceEstimates(acts, now)
		if easy != "6:00/km" || thr != "6:00/km" {
			t.Errorf("got (easy=%q,thr=%q), want both 6:00/km", easy, thr)
		}
	})
}

func TestRecoveryTrend(t *testing.T) {
	day := func(date string, hrv, sleep, charged, drained *int64) store.RecoveryDay {
		rd := store.RecoveryDay{Date: date}
		if hrv != nil {
			rd.HRV = &store.HrvFields{LastNightAvgMs: hrv}
		}
		if sleep != nil {
			rd.Sleep = &store.SleepFields{Score: sleep}
		}
		if charged != nil || drained != nil {
			rd.BodyBattery = &store.BodyBatteryFields{Charged: charged, Drained: drained}
		}
		return rd
	}
	ip := func(v int64) *int64 { return &v }

	t.Run("no data -> stable", func(t *testing.T) {
		if got := recoveryTrend(nil); got != "stable" {
			t.Errorf("recoveryTrend(nil) = %q, want stable", got)
		}
	})

	t.Run("improving HRV and sleep", func(t *testing.T) {
		rec := []store.RecoveryDay{
			day("2026-06-22", ip(60), ip(85), ip(80), ip(40)), // recent
			day("2026-06-21", ip(58), ip(84), ip(78), ip(42)),
			day("2026-06-20", ip(59), ip(86), ip(82), ip(38)),
			day("2026-06-19", ip(48), ip(72), ip(60), ip(55)), // older
			day("2026-06-18", ip(47), ip(70), ip(58), ip(57)),
			day("2026-06-17", ip(49), ip(71), ip(62), ip(53)),
		}
		if got := recoveryTrend(rec); got != "improving" {
			t.Errorf("recoveryTrend = %q, want improving", got)
		}
	})

	t.Run("declining", func(t *testing.T) {
		rec := []store.RecoveryDay{
			day("2026-06-22", ip(45), ip(65), ip(50), ip(60)), // recent (worse)
			day("2026-06-21", ip(44), ip(64), ip(48), ip(62)),
			day("2026-06-20", ip(46), ip(66), ip(52), ip(58)),
			day("2026-06-19", ip(58), ip(82), ip(78), ip(40)), // older (better)
			day("2026-06-18", ip(57), ip(81), ip(76), ip(42)),
			day("2026-06-17", ip(59), ip(83), ip(80), ip(38)),
		}
		if got := recoveryTrend(rec); got != "declining" {
			t.Errorf("recoveryTrend = %q, want declining", got)
		}
	})

	t.Run("flat within deadband -> stable", func(t *testing.T) {
		rec := []store.RecoveryDay{
			day("2026-06-22", ip(50), ip(80), ip(70), ip(50)),
			day("2026-06-21", ip(50), ip(80), ip(70), ip(50)),
			day("2026-06-20", ip(50), ip(80), ip(70), ip(50)),
			day("2026-06-19", ip(50), ip(80), ip(70), ip(50)),
		}
		if got := recoveryTrend(rec); got != "stable" {
			t.Errorf("recoveryTrend = %q, want stable", got)
		}
	})

	t.Run("single day -> stable (cannot split halves)", func(t *testing.T) {
		rec := []store.RecoveryDay{day("2026-06-22", ip(50), ip(80), ip(70), ip(50))}
		if got := recoveryTrend(rec); got != "stable" {
			t.Errorf("recoveryTrend(1 day) = %q, want stable", got)
		}
	})
}

func TestIsCutbackWeek(t *testing.T) {
	tests := []struct {
		name string
		now  string
		want bool
	}{
		{"epoch week (idx 0)", "2026-01-05T12:00:00Z", false},
		{"idx 1", "2026-01-12T12:00:00Z", false},
		{"idx 2", "2026-01-19T12:00:00Z", false},
		{"idx 3 -> cutback", "2026-01-26T12:00:00Z", true},
		{"idx 4", "2026-02-02T12:00:00Z", false},
		{"idx 7 -> cutback", "2026-02-23T12:00:00Z", true},
		{"mid-week still counts by week", "2026-01-28T23:00:00Z", true}, // within idx-3 week
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := mustTime(t, tt.now)
			if got := isCutbackWeek(now); got != tt.want {
				t.Errorf("isCutbackWeek(%s) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestSafeWeeklyTarget(t *testing.T) {
	prof := func(target float64, mode string) store.AthleteProfile {
		return store.AthleteProfile{TargetWeeklyKm: target, ProgressionMode: mode}
	}

	tests := []struct {
		name     string
		baseline float64
		profile  store.AthleteProfile
		cutback  bool
		want     float64
	}{
		{"build ramps 10%", 20, prof(40, "build"), false, 22.0},
		{"hold stays flat", 20, prof(40, "hold"), false, 20.0},
		{"cutback = 80% of baseline", 20, prof(40, "build"), true, 16.0},
		{"build capped at 1.5x stated target", 20, prof(20, "build"), false, 22.0}, // 22 < 30 cap, ok
		{"build cap binds", 25, prof(20, "build"), false, 27.5},                    // 27.5 < 30 cap
		{"build hard cap", 28, prof(20, "build"), false, 30.0},                     // 30.8 -> capped to 30
		{"no history falls back to profile target", 0, prof(20, "build"), false, 22.0},
		{"rounds to 1 decimal", 18.16, prof(40, "build"), false, 20.0}, // 18.16*1.1=19.976 -> 20.0
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeWeeklyTarget(tt.baseline, tt.profile, tt.cutback)
			if got != tt.want {
				t.Errorf("safeWeeklyTarget(%v, %+v, cutback=%v) = %v, want %v",
					tt.baseline, tt.profile, tt.cutback, got, tt.want)
			}
		})
	}
}

func TestComputeFitness(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z") // Monday; weekIndex non-cutback

	acts := []store.Activity{
		// last 7 days: 10 + 8.2 = 18.2 km.
		{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000, MovingTimeS: 3600}, // 6:00/km
		{Type: "Run", StartTime: "2026-06-17T06:00:00Z", DistanceM: 8200, MovingTimeS: 3000},  // ~6:06/km
		// 8-28 days: more volume for chronic/4-week avg + a tempo for threshold.
		{Type: "Run", StartTime: "2026-06-13T06:00:00Z", DistanceM: 8000, MovingTimeS: 2440}, // 5:05/km tempo
		{Type: "Run", StartTime: "2026-06-06T06:00:00Z", DistanceM: 16000, MovingTimeS: 6400},
		{Type: "Run", StartTime: "2026-05-30T06:00:00Z", DistanceM: 16000, MovingTimeS: 6400},
		{Type: "Ride", StartTime: "2026-06-19T06:00:00Z", DistanceM: 40000, MovingTimeS: 3600}, // excluded
	}
	ip := func(v int64) *int64 { return &v }
	recovery := []store.RecoveryDay{
		{Date: "2026-06-21", HRV: &store.HrvFields{LastNightAvgMs: ip(60)}, Sleep: &store.SleepFields{Score: ip(85)}},
		{Date: "2026-06-20", HRV: &store.HrvFields{LastNightAvgMs: ip(59)}, Sleep: &store.SleepFields{Score: ip(86)}},
		{Date: "2026-06-15", HRV: &store.HrvFields{LastNightAvgMs: ip(48)}, Sleep: &store.SleepFields{Score: ip(72)}},
		{Date: "2026-06-14", HRV: &store.HrvFields{LastNightAvgMs: ip(47)}, Sleep: &store.SleepFields{Score: ip(70)}},
	}
	profile := store.AthleteProfile{TargetWeeklyKm: 40, ProgressionMode: "build"}

	m := ComputeFitness(acts, recovery, profile, now)

	if m.WeeklyVolumeKm != 18.2 {
		t.Errorf("WeeklyVolumeKm = %v, want 18.2", m.WeeklyVolumeKm)
	}
	// 28-day total runs = 18.2 + 8 + 16 + 16 = 58.2 -> /4 = 14.55.
	if m.FourWeekAvgKm != 14.55 {
		t.Errorf("FourWeekAvgKm = %v, want 14.55", m.FourWeekAvgKm)
	}
	// acute=18.2; chronic=14.55; 18.2/14.55=1.2509 -> 1.25.
	if m.AcuteChronicRatio != 1.25 {
		t.Errorf("AcuteChronicRatio = %v, want 1.25", m.AcuteChronicRatio)
	}
	if m.EasyPace == "" || m.ThresholdPace == "" {
		t.Errorf("paces should be set, got easy=%q thr=%q", m.EasyPace, m.ThresholdPace)
	}
	if m.RecoveryTrend != "improving" {
		t.Errorf("RecoveryTrend = %q, want improving", m.RecoveryTrend)
	}
	// baseline = max(18.2, 14.55) = 18.2; non-cutback build -> 18.2*1.1=20.02 -> 20.0
	if !m.IsCutbackWeek && m.SafeWeeklyTargetKm != 20.0 {
		t.Errorf("SafeWeeklyTargetKm = %v, want 20.0 (non-cutback)", m.SafeWeeklyTargetKm)
	}
	if m.IsCutbackWeek != isCutbackWeek(now) {
		t.Errorf("IsCutbackWeek = %v, want %v", m.IsCutbackWeek, isCutbackWeek(now))
	}
}

func TestComputeFitnessEmpty(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")
	profile := store.AthleteProfile{TargetWeeklyKm: 20, ProgressionMode: "build"}
	m := ComputeFitness(nil, nil, profile, now)

	if m.WeeklyVolumeKm != 0 || m.FourWeekAvgKm != 0 || m.AcuteChronicRatio != 0 {
		t.Errorf("empty volumes = %+v, want zeros", m)
	}
	if m.EasyPace != "" || m.ThresholdPace != "" {
		t.Errorf("empty paces = (%q,%q), want empty", m.EasyPace, m.ThresholdPace)
	}
	if m.RecoveryTrend != "stable" {
		t.Errorf("empty RecoveryTrend = %q, want stable", m.RecoveryTrend)
	}
	// baseline 0 -> fallback to profile target 20; non-cutback build -> 22.0.
	if !m.IsCutbackWeek && m.SafeWeeklyTargetKm != 22.0 {
		t.Errorf("empty SafeWeeklyTargetKm = %v, want 22.0", m.SafeWeeklyTargetKm)
	}
}

func TestRecoveryTrendExported(t *testing.T) {
	ip := func(v int64) *int64 { return &v }
	day := func(date string, hrv, sleep int64) store.RecoveryDay {
		return store.RecoveryDay{
			Date:  date,
			HRV:   &store.HrvFields{LastNightAvgMs: ip(hrv)},
			Sleep: &store.SleepFields{Score: ip(sleep)},
		}
	}
	rec := []store.RecoveryDay{
		day("2026-06-22", 60, 85), day("2026-06-21", 58, 84), day("2026-06-20", 59, 86),
		day("2026-06-19", 48, 72), day("2026-06-18", 47, 70), day("2026-06-17", 49, 71),
	}
	if got, want := RecoveryTrend(rec), recoveryTrend(rec); got != want {
		t.Errorf("RecoveryTrend = %q, want %q (parity with private)", got, want)
	}
	if got := RecoveryTrend(rec); got != "improving" {
		t.Errorf("RecoveryTrend = %q, want improving", got)
	}
	if got := RecoveryTrend(nil); got != "stable" {
		t.Errorf("RecoveryTrend(nil) = %q, want stable", got)
	}
}

func TestExportedHelpersForProgress(t *testing.T) {
	if !IsRun("Run") || IsRun("Ride") {
		t.Errorf("IsRun broken: Run=%v Ride=%v", IsRun("Run"), IsRun("Ride"))
	}
	tm, ok := ParseStart("2026-06-21T07:00:00Z")
	if !ok || tm.Year() != 2026 {
		t.Errorf("ParseStart = %v ok=%v", tm, ok)
	}
	if got := Median([]float64{1, 2, 3}); got != 2 {
		t.Errorf("Median([1,2,3]) = %v, want 2", got)
	}
	if got := FormatPace(360); got != "6:00/km" {
		t.Errorf("FormatPace(360) = %q, want 6:00/km", got)
	}
}
