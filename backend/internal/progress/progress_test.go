package progress

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
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

// mkRun builds a Strava run activity for fixtures.
func mkRun(start string, distM float64, movingS int64, avgHR *float64) store.Activity {
	return store.Activity{Type: "Run", StartTime: start, DistanceM: distM, MovingTimeS: movingS, AvgHR: avgHR}
}

func TestPaceAtHRSeriesBandAndMedianAndGap(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	hr := func(v float64) *float64 { return &v }
	z2 := int64(145) // band [140,150]
	prof := store.AthleteProfile{Zone2CeilingBpm: &z2}
	acts := []store.Activity{
		// wk2: in-band, 5km @ 1750s (350s/km) and 5km @ 1650s (330s/km) -> median 340
		mkRun("2026-06-20T07:00:00Z", 5000, 1750, hr(145)),
		mkRun("2026-06-19T07:00:00Z", 5000, 1650, hr(148)),
		mkRun("2026-06-18T07:00:00Z", 5000, 1500, hr(120)), // out of band -> ignored
		// wk0 (oldest): one in-band 5km @ 1800s (360s/km)
		// (date must land in wk0 = (2026-05-31, 2026-06-07]; the plan's
		// 2026-06-08 fixture actually fell in wk1 — corrected to 2026-06-05)
		mkRun("2026-06-05T07:00:00Z", 5000, 1800, hr(143)),
		// wk1: NO in-band run -> gap (nil)
		mkRun("2026-06-14T07:00:00Z", 5000, 1500, hr(160)), // out of band
	}
	got := paceAtHRSeries(acts, prof, weekBuckets(3, now))
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] == nil || *got[0] != 360 {
		t.Errorf("wk0 = %v, want 360", got[0])
	}
	if got[1] != nil {
		t.Errorf("wk1 = %v, want nil (gap, no in-band run)", got[1])
	}
	if got[2] == nil || *got[2] != 340 {
		t.Errorf("wk2 = %v, want median 340", got[2])
	}
}

func TestPaceAtHRSeriesDefaultRefHRWhenProfileNil(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	hr := func(v float64) *float64 { return &v }
	prof := store.AthleteProfile{} // nil Zone2CeilingBpm -> defaultRefHRBpm=145, band [140,150]
	acts := []store.Activity{
		mkRun("2026-06-20T07:00:00Z", 5000, 1750, hr(146)), // in default band
		mkRun("2026-06-19T07:00:00Z", 5000, 1500, hr(120)), // out
	}
	got := paceAtHRSeries(acts, prof, weekBuckets(1, now))
	if got[0] == nil || *got[0] != 350 {
		t.Errorf("wk0 = %v, want 350 (default ref HR band)", got[0])
	}
}

func TestWeeklyLoadSeries(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	acts := []store.Activity{
		mkRun("2026-06-20T07:00:00Z", 10000, 3000, nil), // wk1: 10km
		mkRun("2026-06-19T07:00:00Z", 5000, 1500, nil),  // wk1: +5km = 15km
		mkRun("2026-06-12T07:00:00Z", 8000, 2400, nil),  // wk0: 8km
	}
	got := weeklyLoadSeries(acts, weekBuckets(2, now))
	if got[0] == nil || *got[0] != 8 {
		t.Errorf("wk0 = %v, want 8", got[0])
	}
	if got[1] == nil || *got[1] != 15 {
		t.Errorf("wk1 = %v, want 15", got[1])
	}
}

func TestRecoverySeriesMeanAndGap(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	i := func(v int64) *int64 { return &v }
	// recovery is most-recent-first (as ListRecovery returns).
	rec := []store.RecoveryDay{
		{Date: "2026-06-20", RHR: &store.RhrFields{RestingHR: i(48)}, HRV: &store.HrvFields{LastNightAvgMs: i(52)}},
		{Date: "2026-06-19", RHR: &store.RhrFields{RestingHR: i(50)}, HRV: &store.HrvFields{LastNightAvgMs: i(50)}},
		// wk0: a single RHR day; no HRV -> HRV gap that week
		{Date: "2026-06-12", RHR: &store.RhrFields{RestingHR: i(52)}},
	}
	rhr := rhrSeries(rec, weekBuckets(2, now))
	if rhr[0] == nil || *rhr[0] != 52 {
		t.Errorf("rhr wk0 = %v, want 52", rhr[0])
	}
	if rhr[1] == nil || *rhr[1] != 49 { // mean(48,50)
		t.Errorf("rhr wk1 = %v, want 49", rhr[1])
	}
	hrv := hrvSeries(rec, weekBuckets(2, now))
	if hrv[0] != nil {
		t.Errorf("hrv wk0 = %v, want nil (no HRV that week)", hrv[0])
	}
	if hrv[1] == nil || *hrv[1] != 51 { // mean(52,50)
		t.Errorf("hrv wk1 = %v, want 51", hrv[1])
	}
}

func TestVo2maxSeriesLastInBucket(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	f := func(v float64) *float64 { return &v }
	// ListVo2max returns most-recent-first; date-only strings.
	pts := []store.Vo2maxPoint{
		{Date: "2026-06-20", Vo2max: f(52)},
		{Date: "2026-06-18", Vo2max: f(51)}, // same wk1 but earlier -> last (latest) is 52
		{Date: "2026-06-10", Vo2max: f(50)}, // wk0
	}
	got := vo2maxSeries(pts, weekBuckets(2, now))
	if got[0] == nil || *got[0] != 50 {
		t.Errorf("wk0 = %v, want 50", got[0])
	}
	if got[1] == nil || *got[1] != 52 {
		t.Errorf("wk1 = %v, want 52 (latest in bucket)", got[1])
	}
}

func TestComputeProgressFullReport(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	hr := func(v float64) *float64 { return &v }
	i := func(v int64) *int64 { return &v }
	f := func(v float64) *float64 { return &v }
	z2 := int64(145)
	prof := store.AthleteProfile{Zone2CeilingBpm: &z2}

	acts := []store.Activity{
		// newest week in-band 5km@1650s (330)
		mkRun("2026-06-20T07:00:00Z", 5000, 1650, hr(146)),
		// oldest week (12w back) in-band 5km@1750s (350)
		mkRun("2026-03-31T07:00:00Z", 5000, 1750, hr(144)),
	}
	rec := []store.RecoveryDay{
		{Date: "2026-06-20", RHR: &store.RhrFields{RestingHR: i(47)}, HRV: &store.HrvFields{LastNightAvgMs: i(52)}},
		{Date: "2026-03-31", RHR: &store.RhrFields{RestingHR: i(50)}, HRV: &store.HrvFields{LastNightAvgMs: i(46)}},
	}
	vo2 := []store.Vo2maxPoint{
		{Date: "2026-06-19", Vo2max: f(52)},
		{Date: "2026-04-01", Vo2max: f(50)},
	}

	rep := ComputeProgress(acts, rec, vo2, prof, 12, now)
	if rep.Weeks != 12 {
		t.Errorf("Weeks = %d, want 12", rep.Weeks)
	}
	if rep.GeneratedAt == "" {
		t.Error("GeneratedAt empty")
	}
	if len(rep.Signals) != 5 {
		t.Fatalf("len(Signals) = %d, want 5", len(rep.Signals))
	}
	// Signal order is fixed: pace_at_hr, vo2max, resting_hr, hrv_baseline, weekly_load.
	wantOrder := []string{SignalPaceAtHR, SignalVo2max, SignalRestingHR, SignalHRVBaseline, SignalWeeklyLoad}
	for i, s := range rep.Signals {
		if s.Key != wantOrder[i] {
			t.Errorf("signal[%d].Key = %q, want %q", i, s.Key, wantOrder[i])
		}
		if len(s.Series) != 12 {
			t.Errorf("signal[%d] series len = %d, want 12", i, len(s.Series))
		}
	}
	pace := rep.Signals[0]
	if pace.Unit != "s/km" || !pace.LowerIsBetter {
		t.Errorf("pace card = %+v", pace)
	}
	if pace.Current == nil || *pace.Current != 330 || pace.Baseline == nil || *pace.Baseline != 350 {
		t.Errorf("pace cur/base = %v/%v, want 330/350", pace.Current, pace.Baseline)
	}
	if pace.Direction != DirectionDown {
		t.Errorf("pace direction = %q, want down", pace.Direction)
	}
	rhr := rep.Signals[2]
	if !rhr.LowerIsBetter {
		t.Error("resting_hr lower_is_better should be true")
	}
	if !rep.EnoughData {
		t.Error("EnoughData = false, want true (>=2 signals with >=2 points)")
	}
}

func TestComputeProgressNotEnoughData(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	hr := func(v float64) *float64 { return &v }
	z2 := int64(145)
	prof := store.AthleteProfile{Zone2CeilingBpm: &z2}
	// Only one in-band run total -> pace has 1 point; nothing else -> < 2 signals w/ >=2 pts.
	acts := []store.Activity{mkRun("2026-06-20T07:00:00Z", 5000, 1650, hr(146))}
	rep := ComputeProgress(acts, nil, nil, prof, 12, now)
	if rep.EnoughData {
		t.Error("EnoughData = true, want false (thin history)")
	}
	// Signals still computed (the handler decides whether to blank them); contract
	// §3.3: EnoughData is the only thin-history gate.
	if len(rep.Signals) != 5 {
		t.Errorf("len(Signals) = %d, want 5", len(rep.Signals))
	}
}
