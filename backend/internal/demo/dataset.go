package demo

import (
	"math"

	"help-my-run/backend/internal/streams"
)

// BuildDataset returns the authored, offset-based demo story. It is the single
// source of truth for fixture.json: gen.go marshals it, Seed reads the embedded
// result, and TestFixtureMatchesBuilder pins the two together. It is
// deterministic (no time.Now / no randomness) so regeneration is stable.
func BuildDataset() Dataset {
	acts := buildActivities()
	movingByOffset := map[int]int64{}
	for _, a := range acts {
		movingByOffset[a.DayOffset] = a.MovingTimeS
	}
	return Dataset{
		Profile:    buildProfile(),
		Activities: acts,
		Recovery:   buildRecovery(),
		Vo2max:     buildVo2max(),
		Streams:    buildStreams(movingByOffset),
		Decisions:  buildDecisions(),
		Chat:       buildChat(),
		PlanWeek:   buildPlanWeek(),
	}
}

func buildProfile() ProfileSpec {
	return ProfileSpec{
		TargetWeeklyKm:     30,
		ProgressionMode:    "build",
		Zone2CeilingBpm:    145,
		ThresholdBpm:       165,
		MaxHRBpm:           185,
		RunConstraintsJSON: "{}",
		GoalText: "Build a bigger aerobic engine for RX CrossFit — lower resting HR and faster " +
			"recovery between efforts — while holding a steady base. Not training for a race.",
		DailyRunTime:   "06:00",
		Timezone:       "UTC",
		AgentEnabled:   true,
		GoalsJSON:      `["crossfit","fitness"]`,
		WeekJSON:       `{"runs_per_week":3,"crossfit_days":5,"rest_day":"sunday"}`,
		GuardrailsJSON: `{"no_b2b_hard":true,"protect_long_run":true,"easy_stays_easy":true,"hrv_backoff":true,"load_cap_55":true}`,
	}
}

// buildActivities authors ~34 runs over 84 days. Weeks 2..11 are the steady
// base block (3 runs/week: long/easy/tempo); easy pace improves gently at the
// SAME ~141 bpm as weeks get more recent (≈6:35/km early → ≈6:20/km late) so the
// Pace@Z2-HR trend on the Trends page corroborates the coach's "aerobic engine"
// claim. The most recent fortnight is disrupted by the overreach dip (daily
// decisions −12..−9 are red REST_DAY): a big long run on −13, then genuine REST
// through the red window (NO runs), then easy recovery. This keeps run load
// consistent with the readiness story — no hard run ever lands on a red rest day.
//
// Long runs sit just above the Z2 reference band (~151 bpm > 150 = Zone2 ceiling
// 145 + 5) so the Pace@Z2-HR headline reflects the EASY runs only, matching the
// "at the same ~140 bpm" narrative exactly instead of averaging in the longs.
func buildActivities() []ActivitySpec {
	var runs []ActivitySpec
	add := func(offset int, name string, km, paceSecPerKm, avgHR, maxHR, cadence, elevGain float64, hour, min int) {
		km = round1(km)
		moving := int64(km * paceSecPerKm)
		runs = append(runs, ActivitySpec{
			DayOffset:      offset,
			StartHour:      hour,
			StartMin:       min,
			Name:           name,
			Type:           "Run",
			DistanceM:      round1(km * 1000),
			MovingTimeS:    moving,
			ElapsedTimeS:   moving + int64(float64(moving)*0.03) + 45,
			AvgHR:          avgHR,
			MaxHR:          maxHR,
			AvgSpeed:       round4(1000.0 / paceSecPerKm),
			AvgCadence:     cadence,
			ElevationGainM: round1(elevGain),
		})
	}

	for w := 1; w <= 11; w++ {
		base := 7 * w
		cutback := w == 4 // cutback week ~ day -28
		longKm := 18 - float64(w-1)*0.55
		tempoKm := 8.0
		easyKm := 8.0
		if cutback {
			longKm, tempoKm, easyKm = 10, 5, 5
		}
		// prog: 0 (oldest, w=11) → 1 (most recent full block, w=2). Easy/long pace
		// improve toward the present at a fixed HR — the seeded aerobic gain.
		prog := float64(11-w) / 9.0
		easyPace := round1(395 - 13*prog) // 395 (6:35) → 382 s/km @ 141 bpm
		longPace := round1(408 - 8*prog)  // 408 → 400 s/km @ 151 bpm (above Z2 band)
		add(-(base + 6), "Long run", longKm, longPace, 151, 163, 166, longKm*7, 8, 0)
		if w == 1 {
			// The overreach dip (decisions −12..−9 = red REST_DAY) disrupted this
			// week: keep the −13 long (green day, the effort that helped tip the
			// dip) but seed NO easy/tempo on the red rest days −11/−9. A genuinely
			// overreached athlete rested.
			continue
		}
		add(-(base + 4), "Easy aerobic", easyKm, easyPace, 141, 153, 172, easyKm*5, 6, 15)
		add(-(base + 2), "Tempo", tempoKm, 315, 158, 173, 176, tempoKm*4, 18, 30)
	}
	// This week (post-dip recovery): the recovery jog and shakeout sit BELOW the
	// Z2 band (<140 bpm) so they don't dilute the Pace@Z2-HR trend; the −3 easy
	// aerobic is the fastest in-band easy run of the block (6:20/km @ 141 bpm) —
	// the engine payoff the Trends page shows. No hard runs in the last 3 days.
	add(-8, "Recovery jog", 5, 392, 136, 147, 170, 20, 6, 30)
	add(-5, "Easy shakeout", 6, 384, 139, 151, 171, 30, 6, 20)
	add(-3, "Easy aerobic", 7, 380, 141, 153, 172, 36, 6, 15)
	return runs
}

// buildRecovery authors 84 calendar days: a wobbling baseline, an overreach dip
// days -12..-9 (HRV tanks, sleep/RHR worsen), and a today (0) still suppressed.
func buildRecovery() []RecoverySpec {
	out := make([]RecoverySpec, 0, 84)
	for off := -83; off <= 0; off++ {
		hrv := int64(62) + wobble(off, 5, 6)
		score := int64(78) + wobble(off, 3, 9)
		rhr := int64(51) + wobble(off, 6, 2)
		bbHigh := int64(82) + wobble(off, 4, 6)
		bbCharged := int64(66) + wobble(off, 5, 8)
		bbDrained := int64(52) + wobble(off, 3, 7)
		bbLow := int64(16) + wobble(off, 4, 5)
		dur := int64(25200) + wobble(off, 4, 1500)
		status := "balanced"

		switch {
		case off >= -12 && off <= -9: // overreach dip (recovering across the window)
			hrv = int64(42 + (off + 12))      // 42,43,44,45
			score = int64(51 + (off+12)*2)    // 51,53,55,57
			rhr = int64(58 + (off + 9))       // 58 → 57 → 56 → 55
			bbHigh = int64(36 + (off+12)*3)   // 36,39,42,45
			dur = int64(20700 + (off+12)*300) // ~5.75h → 6.5h
			status = "low"
		case off == 0: // today: still slightly suppressed → amber
			hrv, score, rhr, bbHigh, dur = 55, 68, 54, 46, 21600
			status = "unbalanced"
		}

		out = append(out, RecoverySpec{
			DayOffset:      off,
			SleepDurationS: dur,
			SleepDeepS:     int64(math.Round(float64(dur) * 0.18)),
			SleepRemS:      int64(math.Round(float64(dur) * 0.22)),
			SleepAwakeS:    int64(math.Round(float64(dur) * 0.05)),
			SleepLightS:    dur - int64(math.Round(float64(dur)*0.18)) - int64(math.Round(float64(dur)*0.22)) - int64(math.Round(float64(dur)*0.05)),
			SleepScore:     score,
			HrvMs:          hrv,
			HrvStatus:      status,
			BbCharged:      bbCharged,
			BbDrained:      bbDrained,
			BbHigh:         bbHigh,
			BbLow:          bbLow,
			Rhr:            rhr,
		})
	}
	return out
}

// buildVo2max authors a sparse 46.0 → ~47.5 climb (every ~4 days).
func buildVo2max() []Vo2maxSpec {
	var out []Vo2maxSpec
	for off := -83; off <= -1; off += 4 {
		v := 46.0 + float64(83+off)/83.0*1.5
		out = append(out, Vo2maxSpec{DayOffset: off, Vo2max: round1(v)})
	}
	return out
}

// buildStreams authors pre-computed analyses for the 6 most recent runs. Seconds
// are derived from each run's moving time so time-in-zone sums correctly.
func buildStreams(moving map[int]int64) []StreamSpec {
	zones := streams.ZoneBounds{Z1Hi: 116, Z2Hi: 145, Z3Hi: 155, Z4Hi: 165}
	type spec struct {
		off     int
		dec     float64
		paFirst float64
		pct     [5]float64
	}
	// The 6 most recent runs that actually exist (−9/−11 were dropped — they fell
	// on red REST_DAY days). Decoupling/pa:HR improve toward the present; the long
	// run holds decoupling under 5%, the tempo runs hotter as expected.
	specs := []spec{
		{-3, 2.8, 0.0187, [5]float64{8, 72, 16, 3, 1}},   // easy — freshest, best efficiency
		{-5, 3.0, 0.0185, [5]float64{9, 73, 14, 3, 1}},   // easy shakeout
		{-8, 3.2, 0.0184, [5]float64{14, 74, 10, 1, 1}},  // recovery jog (very easy)
		{-13, 4.6, 0.0166, [5]float64{5, 58, 29, 6, 2}},  // long run (decoupling < 5%)
		{-16, 7.2, 0.0202, [5]float64{3, 18, 30, 42, 7}}, // tempo (harder, higher decoupling)
		{-18, 3.6, 0.0182, [5]float64{8, 72, 16, 3, 1}},  // easy (earlier week)
	}
	out := make([]StreamSpec, 0, len(specs))
	for _, s := range specs {
		mv := moving[s.off]
		out = append(out, StreamSpec{
			ActivityDayOffset: s.off,
			HasHR:             true,
			DecouplingPct:     s.dec,
			PaHRFirst:         s.paFirst,
			PaHRSecond:        round4(s.paFirst * (1 - s.dec/100.0)),
			TimeInZone:        demoZoneTimes(mv, s.pct),
			Zones:             zones,
		})
	}
	return out
}

// buildDecisions authors the last 14 daily decisions: green STAND baseline, a
// red REST_DAY overreach block (-12..-9), and today's amber SOFTEN centerpiece.
func buildDecisions() []DecisionSpec {
	easy := &SessionSpec{
		RunType: "easy", DistanceKm: 7, PaceTarget: "6:25/km",
		TimeNote: "Evening after CrossFit", Rationale: "Aerobic base — conversational effort.",
	}
	green := DriversSpec{
		SleepHours: 7.2, SleepScore: 80, HRVLastNightMs: 63, HRVBaselineMs: 61, HRVDeltaPct: 3.3,
		RHRLastNight: 51, RHRBaseline: 51, RHRDeltaBpm: 0, BodyBatteryHigh: 84,
		RecoveryTrend: "stable", DataComplete: true, Reasons: []string{}, Stale: false,
	}
	red := DriversSpec{
		SleepHours: 5.8, SleepScore: 52, HRVLastNightMs: 44, HRVBaselineMs: 60, HRVDeltaPct: -26.7,
		RHRLastNight: 58, RHRBaseline: 51, RHRDeltaBpm: 7.0, BodyBatteryHigh: 38,
		RecoveryTrend: "declining", DataComplete: true,
		Reasons: []string{"HRV -26.7% vs baseline", "RHR +7.0 bpm vs baseline", "Sleep score 52 (<65)"},
		Stale:   false,
	}
	today := DriversSpec{
		SleepHours: 6.0, SleepScore: 68, HRVLastNightMs: 55, HRVBaselineMs: 61, HRVDeltaPct: -9.8,
		RHRLastNight: 54, RHRBaseline: 51, RHRDeltaBpm: 3.0, BodyBatteryHigh: 46,
		RecoveryTrend: "declining", DataComplete: true,
		Reasons: []string{"HRV -9.8% vs baseline"}, Stale: false,
	}

	greenDay := func(off int) DecisionSpec {
		return DecisionSpec{
			DayOffset: off, Color: "green", Action: "STAND", Source: "ai",
			Rationale: "Well recovered — proceed with the easy run as planned.",
			Drivers:   green, OriginalSession: easy, AdjustedSession: easy,
		}
	}
	redDay := func(off int) DecisionSpec {
		return DecisionSpec{
			DayOffset: off, Color: "red", Action: "REST_DAY", Source: "ai",
			Rationale: "Readiness red — HRV down sharply and resting HR elevated. Full rest today to recover.",
			Drivers:   red,
		}
	}

	decs := []DecisionSpec{greenDay(-13)}
	for _, off := range []int{-12, -11, -10, -9} {
		decs = append(decs, redDay(off))
	}
	for _, off := range []int{-8, -7, -6, -5, -4, -3, -2, -1} {
		decs = append(decs, greenDay(off))
	}
	decs = append(decs, DecisionSpec{
		DayOffset: 0, Color: "amber", Action: "SOFTEN", Source: "ai",
		Rationale: "Yesterday's CrossFit leg day plus a suppressed HRV (about -10% vs baseline) leave you amber. " +
			"Keeping the run but swapping the tempo for an easy 6 km so you absorb the work instead of digging deeper.",
		Drivers: today,
		OriginalSession: &SessionSpec{
			RunType: "tempo", DistanceKm: 8, PaceTarget: "5:15/km",
			TimeNote: "Evening, after CrossFit", Rationale: "Weekly tempo — controlled threshold work.",
		},
		AdjustedSession: &SessionSpec{
			RunType: "easy", DistanceKm: 6, PaceTarget: "6:30/km",
			TimeNote: "Evening — keep it conversational", OptionalIfCNS: true,
			Rationale: "Downgraded from tempo — legs and HRV need a break.",
		},
	})
	return decs
}

// buildChat authors two grounded exchanges; assistant turns carry the label.
func buildChat() []ChatSpec {
	return []ChatSpec{
		{Role: "user", Content: "Is my aerobic base actually improving, or am I just running more?"},
		{Role: "assistant", Content: "Your engine is genuinely improving, not just your mileage. Over the last 12 weeks " +
			"your easy pace dropped from about 6:35/km to 6:20/km at the same ~140 bpm, and VO2max rose from 46.0 to 47.5. " +
			"Long-run decoupling is holding under 5%, so you're staying aerobically efficient deep into the run.\n\n" + SampleLabel},
		{Role: "user", Content: "Why did my readiness go red last week?"},
		{Role: "assistant", Content: "Around 10 days ago your HRV dropped into the low-40s (about 27% below your ~60 ms " +
			"baseline) and your resting HR climbed 7 bpm — classic short-term overreaching, most likely from stacking " +
			"heavy CrossFit legs on top of your long run. You backed off for a few days and both recovered, though today's " +
			"HRV is still slightly suppressed, so you're amber.\n\n" + SampleLabel},
	}
}

// buildPlanWeek authors the current-week template (Mon→Sun): a coherent ~30 km
// base week whose run distances SUM to the 30 km target (easy 8 + tempo 8 + long
// 14, long the biggest single session), matching the profile TargetWeeklyKm and
// the rationale. The seeder (materializePlanWeek) places today's planned tempo,
// one protected long, and one easy on distinct days so the materialized plan
// still sums to 30 km on every boot weekday.
func buildPlanWeek() PlanWeekSpec {
	return PlanWeekSpec{
		FitnessSummary: "A 12-week aerobic base built steadily — VO2max 46.0→47.5 and easy pace trending down at the " +
			"same heart rate. One overreach dip ~10 days ago, now recovered.",
		WeeklyTargetKm: 30,
		WeekRationale: "Volume held at the 30 km base: one protected weekend long run, a single quality tempo on a " +
			"low-CNS day, and an easy aerobic day — everything else is CrossFit or rest so legs stay fresh for lifting.",
		OneFlag: "Keep the tempo controlled — HRV is still recovering from last week's dip.",
		Days: []SessionSpec{
			{RunType: "rest", Rationale: "CrossFit leg day — running rest."},
			{RunType: "easy", DistanceKm: 8, PaceTarget: "6:20/km", TimeNote: "Evening after CrossFit", Rationale: "Aerobic base, conversational."},
			{RunType: "rest", Rationale: "Heavy CrossFit conditioning — running rest."},
			{RunType: "tempo", DistanceKm: 8, PaceTarget: "5:15/km", TimeNote: "Evening — barbell-skill day, legs fresh", Rationale: "Weekly threshold work on the low-CNS day."},
			{RunType: "rest", Rationale: "CrossFit — running rest before the long run."},
			{RunType: "long", DistanceKm: 14, PaceTarget: "6:40/km", TimeNote: "Morning", Rationale: "Weekend long run — the aerobic centerpiece."},
			{RunType: "rest", Rationale: "Rest — recover for the week ahead."},
		},
	}
}

// demoZoneTimes builds a 5-zone dwell breakdown from a moving-time total and a
// percentage split (shape matches streams.TimeInZone output).
func demoZoneTimes(moving int64, pct [5]float64) []streams.ZoneTime {
	out := make([]streams.ZoneTime, 5)
	for z := 0; z < 5; z++ {
		out[z] = streams.ZoneTime{
			Zone:    z + 1,
			Seconds: round1(float64(moving) * pct[z] / 100.0),
			Pct:     pct[z],
		}
	}
	return out
}

// wobble is a deterministic ±amp sinusoidal jitter keyed on the day offset.
func wobble(off int, period, amp float64) int64 {
	return int64(math.Round(amp * math.Sin(float64(off)/period)))
}

// round4 rounds to four decimals (Pa:HR / speed values).
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }
