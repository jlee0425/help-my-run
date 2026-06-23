package chat

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/store"
)

func i64p(v int64) *int64     { return &v }
func f64p(v float64) *float64 { return &v }
func strp(v string) *string   { return &v }

// newPackEngine builds a chat Engine over a fresh temp store + a real progress
// engine (the pack reuses progress.Report for Signals). The llm Runner is never
// called by buildContextPack, so a no-op client is fine.
func newPackEngine(t *testing.T) (*Engine, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "chat.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	c := &llm.Client{Runner: &captureRunner{}, Model: "m"}
	pe := progress.New(s, c, "m")
	return New(s, c, pe, "m", 6), s
}

// seedProfile writes the single athlete_profile row with the given constraints.
func seedProfile(t *testing.T, s *store.Store, constraints, goal string) {
	t.Helper()
	if err := s.UpsertAthleteProfile(store.AthleteProfile{
		TargetWeeklyKm:     30,
		ProgressionMode:    "build",
		Zone2CeilingBpm:    i64p(145),
		ThresholdBpm:       i64p(170),
		MaxHRBpm:           i64p(190),
		RunConstraintsJSON: constraints,
		GoalText:           goal,
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
}

func TestBuildContextPackCapsAndFlatten(t *testing.T) {
	e, s := newPackEngine(t)
	seedProfile(t, s, `{"no_back_to_back":true}`, "aerobic base")

	// 20 runs -> Activities capped at 14. Use distinct ids + descending dates so
	// ListActivities (most-recent-first) is deterministic.
	for i := 0; i < 20; i++ {
		day := 28 - i // 2026-06-28 down to 2026-06-09
		start := "2026-06-" + twoDigits(day) + "T06:00:00Z"
		if err := s.UpsertActivity(store.Activity{
			ActivityID: int64(1000 + i), Name: "run", Type: "Run",
			StartTime: start, DistanceM: 10000, MovingTimeS: 3000,
			AvgHR: f64p(150), MaxHR: f64p(170), AvgCadence: f64p(86), ElevationGainM: f64p(80),
			RawJSON: "{}",
		}); err != nil {
			t.Fatalf("seed activity %d: %v", i, err)
		}
	}
	// One non-run (Ride) must be filtered out by metrics.IsRun.
	if err := s.UpsertActivity(store.Activity{
		ActivityID: 9999, Name: "ride", Type: "Ride",
		StartTime: "2026-06-29T06:00:00Z", DistanceM: 40000, MovingTimeS: 3600, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("seed ride: %v", err)
	}

	// 20 recovery days -> Recovery capped at 14 (ListRecovery(14) caps at source).
	for i := 0; i < 20; i++ {
		day := 28 - i
		date := "2026-06-" + twoDigits(day)
		if err := s.UpsertSleep(store.SleepRow{
			Date: date, DurationS: i64p(27000), Score: i64p(82), RawJSON: "{}",
		}); err != nil {
			t.Fatalf("seed sleep %d: %v", i, err)
		}
		if err := s.UpsertHrv(store.HrvRow{
			Date: date, LastNightAvgMs: i64p(48), Status: strp("BALANCED"), RawJSON: "{}",
		}); err != nil {
			t.Fatalf("seed hrv %d: %v", i, err)
		}
		if err := s.UpsertRhr(store.RhrRow{Date: date, RestingHR: i64p(47), RawJSON: "{}"}); err != nil {
			t.Fatalf("seed rhr %d: %v", i, err)
		}
		if err := s.UpsertBodyBattery(store.BodyBatteryRow{
			Date: date, High: i64p(91), Low: i64p(14), RawJSON: "{}",
		}); err != nil {
			t.Fatalf("seed bb %d: %v", i, err)
		}
	}

	// 8 stream analyses -> StreamSummary capped at 5. Decoded TimeInZoneJSON.
	tiz := `[{"zone":1,"seconds":300,"pct":10},{"zone":2,"seconds":1800,"pct":60},{"zone":3,"seconds":600,"pct":20},{"zone":4,"seconds":180,"pct":6},{"zone":5,"seconds":120,"pct":4}]`
	for i := 0; i < 8; i++ {
		if err := s.UpsertStreamAnalysis(store.StreamAnalysisRow{
			ActivityID: int64(1000 + i), TimeInZoneJSON: tiz, DecouplingPct: f64p(5.5),
			ZonesJSON: `{"z1_hi":116,"z2_hi":145,"z3_hi":157.5,"z4_hi":170}`,
			HasHR:     true, ComputedAt: "2026-06-28T07:00:00Z",
		}); err != nil {
			t.Fatalf("seed analysis %d: %v", i, err)
		}
	}

	pack, err := e.buildContextPack(context.Background())
	if err != nil {
		t.Fatalf("buildContextPack: %v", err)
	}

	if len(pack.Activities) != activityPackLimit {
		t.Errorf("activities = %d, want %d (cap)", len(pack.Activities), activityPackLimit)
	}
	if len(pack.Recovery) > recoveryPackLimit {
		t.Errorf("recovery = %d, want <= %d (cap)", len(pack.Recovery), recoveryPackLimit)
	}
	if len(pack.StreamSummary) != streamPackLimit {
		t.Errorf("stream summary = %d, want %d (cap)", len(pack.StreamSummary), streamPackLimit)
	}
	// Signals come from progress.Report (fixed 6 keys).
	if len(pack.Signals) != 6 {
		t.Errorf("signals = %d, want 6 fixed trend keys", len(pack.Signals))
	}
	// No Ride leaked in (run filter).
	for _, a := range pack.Activities {
		if a.Type == "Ride" {
			t.Errorf("non-run leaked into activities: %+v", a)
		}
	}
	// Pace derived + distance flattened: 3000s / 10km = 300 s/km -> "5:00/km".
	a0 := pack.Activities[0]
	if a0.DistanceKm != 10 {
		t.Errorf("distance_km = %v, want 10", a0.DistanceKm)
	}
	if a0.Pace != "5:00/km" {
		t.Errorf("pace = %q, want 5:00/km", a0.Pace)
	}
	// Recovery flatten: sleep_hours = 27000/3600 = 7.5; rhr/hrv/bb carried.
	r0 := pack.Recovery[0]
	if r0.SleepHours == nil || *r0.SleepHours != 7.5 {
		t.Errorf("sleep_hours = %v, want 7.5", r0.SleepHours)
	}
	if r0.RestingHR == nil || *r0.RestingHR != 47 {
		t.Errorf("resting_hr = %v, want 47", r0.RestingHR)
	}
	if r0.HRVLastNightMs == nil || *r0.HRVLastNightMs != 48 {
		t.Errorf("hrv_last_night_ms = %v, want 48", r0.HRVLastNightMs)
	}
	if r0.BodyBatteryHi == nil || *r0.BodyBatteryHi != 91 {
		t.Errorf("body_battery_high = %v, want 91", r0.BodyBatteryHi)
	}
	// Stream pack: TimeInZoneJSON decoded to ZonePct (5 zones, pct carried).
	sp0 := pack.StreamSummary[0]
	if len(sp0.TimeInZone) != 5 {
		t.Fatalf("time_in_zone = %d zones, want 5", len(sp0.TimeInZone))
	}
	if sp0.TimeInZone[1].Zone != 2 || sp0.TimeInZone[1].Pct != 60 {
		t.Errorf("zone2 = %+v, want zone 2 pct 60", sp0.TimeInZone[1])
	}
	if sp0.DecouplingPct == nil || *sp0.DecouplingPct != 5.5 {
		t.Errorf("decoupling = %v, want 5.5", sp0.DecouplingPct)
	}
	if !sp0.HasHR {
		t.Error("has_hr = false, want true")
	}
	// Profile constraints preserved as raw JSON.
	if string(pack.Profile.RunConstraints) != `{"no_back_to_back":true}` {
		t.Errorf("run_constraints = %s, want passthrough", pack.Profile.RunConstraints)
	}
	if pack.Profile.GoalText != "aerobic base" {
		t.Errorf("goal_text = %q, want aerobic base", pack.Profile.GoalText)
	}
	if pack.GeneratedAt == "" {
		t.Error("generated_at empty")
	}
}

func TestBuildContextPackEmptyConstraintsDefaultsToObject(t *testing.T) {
	e, s := newPackEngine(t)
	seedProfile(t, s, "", "") // empty constraints -> "{}"

	pack, err := e.buildContextPack(context.Background())
	if err != nil {
		t.Fatalf("buildContextPack: %v", err)
	}
	if string(pack.Profile.RunConstraints) != "{}" {
		t.Errorf("run_constraints = %s, want {}", pack.Profile.RunConstraints)
	}
}

func TestBuildContextPackThinData(t *testing.T) {
	e, s := newPackEngine(t)
	seedProfile(t, s, "{}", "")
	// No activities, recovery, or stream analyses seeded.

	pack, err := e.buildContextPack(context.Background())
	if err != nil {
		t.Fatalf("buildContextPack thin: %v", err)
	}
	// Slices are empty (non-nil), not nil-panic.
	if pack.Activities == nil || len(pack.Activities) != 0 {
		t.Errorf("activities = %v, want non-nil empty", pack.Activities)
	}
	if pack.Recovery == nil || len(pack.Recovery) != 0 {
		t.Errorf("recovery = %v, want non-nil empty", pack.Recovery)
	}
	if pack.StreamSummary == nil || len(pack.StreamSummary) != 0 {
		t.Errorf("stream summary = %v, want non-nil empty", pack.StreamSummary)
	}
	// Pack still marshals (compact, no panic).
	if _, err := json.Marshal(pack); err != nil {
		t.Fatalf("marshal thin pack: %v", err)
	}
}

// twoDigits zero-pads a 1..31 day for fixture date strings.
func twoDigits(d int) string {
	if d < 10 {
		return "0" + string(rune('0'+d))
	}
	return string(rune('0'+d/10)) + string(rune('0'+d%10))
}
