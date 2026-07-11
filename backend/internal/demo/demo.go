package demo

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/streams"
)

// fixtureJSON is the authored 84-day demo dataset, generated from BuildDataset()
// by gen.go (`go generate ./internal/demo`) and embedded so the single-binary
// promise holds. All dates are day offsets (0 = today); Seed materializes them
// against the boot date so the demo never rots.
//
//go:generate go run gen.go
//go:embed fixture.json
var fixtureJSON []byte

// --- Fixture schema (offset-based; materialized against `now` at seed time) ---

// Dataset is the whole authored demo story. The CrossFit week and the today-plan
// override are computed in the seeder relative to now's Monday, so they always
// contain day 0 regardless of weekday.
type Dataset struct {
	Profile    ProfileSpec    `json:"profile"`
	Activities []ActivitySpec `json:"activities"`
	Recovery   []RecoverySpec `json:"recovery"`
	Vo2max     []Vo2maxSpec   `json:"vo2max"`
	Streams    []StreamSpec   `json:"streams"`
	Decisions  []DecisionSpec `json:"decisions"`
	Chat       []ChatSpec     `json:"chat"`
	PlanWeek   PlanWeekSpec   `json:"plan_week"`
}

// ProfileSpec mirrors the fields of store.AthleteProfile the demo sets.
type ProfileSpec struct {
	TargetWeeklyKm     float64 `json:"target_weekly_km"`
	ProgressionMode    string  `json:"progression_mode"`
	Zone2CeilingBpm    int64   `json:"zone2_ceiling_bpm"`
	ThresholdBpm       int64   `json:"threshold_bpm"`
	MaxHRBpm           int64   `json:"max_hr_bpm"`
	RunConstraintsJSON string  `json:"run_constraints_json"`
	GoalText           string  `json:"goal_text"`
	DailyRunTime       string  `json:"daily_run_time"`
	Timezone           string  `json:"timezone"`
	AgentEnabled       bool    `json:"agent_enabled"`
	GoalsJSON          string  `json:"goals_json"`
	WeekJSON           string  `json:"week_json"`
	GuardrailsJSON     string  `json:"guardrails_json"`
}

// ActivitySpec is one run, positioned by day offset with a start time-of-day.
type ActivitySpec struct {
	DayOffset      int     `json:"day_offset"`
	StartHour      int     `json:"start_hour"`
	StartMin       int     `json:"start_min"`
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	DistanceM      float64 `json:"distance_m"`
	MovingTimeS    int64   `json:"moving_time_s"`
	ElapsedTimeS   int64   `json:"elapsed_time_s"`
	AvgHR          float64 `json:"avg_hr"`
	MaxHR          float64 `json:"max_hr"`
	AvgSpeed       float64 `json:"avg_speed"`
	AvgCadence     float64 `json:"avg_cadence"`
	ElevationGainM float64 `json:"elevation_gain_m"`
}

// RecoverySpec is one calendar day of Garmin recovery data.
type RecoverySpec struct {
	DayOffset      int    `json:"day_offset"`
	SleepDurationS int64  `json:"sleep_duration_s"`
	SleepDeepS     int64  `json:"sleep_deep_s"`
	SleepLightS    int64  `json:"sleep_light_s"`
	SleepRemS      int64  `json:"sleep_rem_s"`
	SleepAwakeS    int64  `json:"sleep_awake_s"`
	SleepScore     int64  `json:"sleep_score"`
	HrvMs          int64  `json:"hrv_ms"`
	HrvStatus      string `json:"hrv_status"`
	BbCharged      int64  `json:"bb_charged"`
	BbDrained      int64  `json:"bb_drained"`
	BbHigh         int64  `json:"bb_high"`
	BbLow          int64  `json:"bb_low"`
	Rhr            int64  `json:"rhr"`
}

// Vo2maxSpec is one sparse VO2max reading.
type Vo2maxSpec struct {
	DayOffset int     `json:"day_offset"`
	Vo2max    float64 `json:"vo2max"`
}

// StreamSpec is one pre-computed per-run stream analysis, referencing an
// activity by its day offset. TimeInZone/Zones use the streams engine's types
// so the stored JSON is byte-identical to what the engine writes.
type StreamSpec struct {
	ActivityDayOffset int                `json:"activity_day_offset"`
	HasHR             bool               `json:"has_hr"`
	DecouplingPct     float64            `json:"decoupling_pct"`
	PaHRFirst         float64            `json:"pa_hr_first"`
	PaHRSecond        float64            `json:"pa_hr_second"`
	TimeInZone        []streams.ZoneTime `json:"time_in_zone"`
	Zones             streams.ZoneBounds `json:"zones"`
}

// SessionSpec is a PlanDay minus the date/dow (the seeder stamps those).
type SessionSpec struct {
	RunType       string  `json:"run_type"`
	DistanceKm    float64 `json:"distance_km"`
	PaceTarget    string  `json:"pace_target"`
	TimeNote      string  `json:"time_note"`
	OptionalIfCNS bool    `json:"optional_if_cns"`
	Rationale     string  `json:"rationale"`
}

// DriversSpec mirrors readiness.ReadinessDrivers (minus date, which the seeder
// stamps) plus the agent's "reasons"/"stale" siblings — the exact shape the web
// Today page consumes from daily_decisions.drivers_json.
type DriversSpec struct {
	SleepHours      float64  `json:"sleep_hours"`
	SleepScore      int64    `json:"sleep_score"`
	HRVLastNightMs  int64    `json:"hrv_last_night_ms"`
	HRVBaselineMs   float64  `json:"hrv_baseline_ms"`
	HRVDeltaPct     float64  `json:"hrv_delta_pct"`
	RHRLastNight    int64    `json:"rhr_last_night"`
	RHRBaseline     float64  `json:"rhr_baseline"`
	RHRDeltaBpm     float64  `json:"rhr_delta_bpm"`
	BodyBatteryHigh int64    `json:"body_battery_high"`
	RecoveryTrend   string   `json:"recovery_trend"`
	DataComplete    bool     `json:"data_complete"`
	Reasons         []string `json:"reasons"`
	Stale           bool     `json:"stale"`
}

// DecisionSpec is one daily_decisions row, positioned by day offset.
type DecisionSpec struct {
	DayOffset       int          `json:"day_offset"`
	Color           string       `json:"color"`
	Action          string       `json:"action"`
	Source          string       `json:"source"`
	Rationale       string       `json:"rationale"`
	Drivers         DriversSpec  `json:"drivers"`
	OriginalSession *SessionSpec `json:"original_session"`
	AdjustedSession *SessionSpec `json:"adjusted_session"`
}

// ChatSpec is one persisted chat turn.
type ChatSpec struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// PlanWeekSpec is the current-week plan template (Mon→Sun, weekday-indexed).
// The seeder materializes dates from now's Monday and overrides the day matching
// today with the centerpiece tempo session so Today and Plan stay coherent.
type PlanWeekSpec struct {
	FitnessSummary string        `json:"fitness_summary"`
	WeeklyTargetKm float64       `json:"weekly_target_km"`
	WeekRationale  string        `json:"week_rationale"`
	OneFlag        string        `json:"one_flag"`
	Days           []SessionSpec `json:"days"` // exactly 7, Mon..Sun
}

// --- Seeding ---

// Seed populates a migrated store with the demo dataset. All fixture dates are
// day offsets (0 = today) materialized against now's local date. It is
// idempotent: upserts overwrite, and the append-only plan/chat rows are guarded.
func Seed(s *store.Store, now time.Time) error {
	var ds Dataset
	if err := json.Unmarshal(fixtureJSON, &ds); err != nil {
		return fmt.Errorf("demo: parse fixture: %w", err)
	}
	return seedDataset(s, ds, now)
}

func seedDataset(s *store.Store, ds Dataset, now time.Time) error {
	// Profile first (zones feed stream analysis interpretation).
	p := ds.Profile
	if err := s.UpsertAthleteProfile(store.AthleteProfile{
		TargetWeeklyKm:     p.TargetWeeklyKm,
		ProgressionMode:    p.ProgressionMode,
		Zone2CeilingBpm:    iptr(p.Zone2CeilingBpm),
		ThresholdBpm:       iptr(p.ThresholdBpm),
		MaxHRBpm:           iptr(p.MaxHRBpm),
		RunConstraintsJSON: p.RunConstraintsJSON,
		GoalText:           p.GoalText,
		DailyRunTime:       p.DailyRunTime,
		Timezone:           p.Timezone,
		AgentEnabled:       p.AgentEnabled,
		GoalsJSON:          p.GoalsJSON,
		WeekJSON:           p.WeekJSON,
		GuardrailsJSON:     p.GuardrailsJSON,
	}); err != nil {
		return fmt.Errorf("demo: profile: %w", err)
	}

	// Activities (must precede stream analyses — FK on activity_id).
	for _, a := range ds.Activities {
		start := dayTime(now, a.DayOffset, a.StartHour, a.StartMin)
		startStr := start.Format(time.RFC3339)
		if err := s.UpsertActivity(store.Activity{
			ActivityID:     activityID(a.DayOffset),
			Name:           a.Name,
			Type:           a.Type,
			StartTime:      startStr,
			StartTimeLocal: sptr(startStr),
			DistanceM:      a.DistanceM,
			MovingTimeS:    a.MovingTimeS,
			ElapsedTimeS:   a.ElapsedTimeS,
			AvgHR:          fptr(a.AvgHR),
			MaxHR:          fptr(a.MaxHR),
			AvgSpeed:       fptr(a.AvgSpeed),
			AvgCadence:     fptr(a.AvgCadence),
			ElevationGainM: fptr(a.ElevationGainM),
			RawJSON:        "{}",
		}); err != nil {
			return fmt.Errorf("demo: activity: %w", err)
		}
	}

	// Recovery (one row per source table per day).
	for _, r := range ds.Recovery {
		date := dayDate(now, r.DayOffset)
		if err := s.UpsertSleep(store.SleepRow{
			Date: date, DurationS: iptr(r.SleepDurationS), DeepS: iptr(r.SleepDeepS),
			LightS: iptr(r.SleepLightS), RemS: iptr(r.SleepRemS), AwakeS: iptr(r.SleepAwakeS),
			Score: iptr(r.SleepScore), RawJSON: "{}",
		}); err != nil {
			return fmt.Errorf("demo: sleep: %w", err)
		}
		if err := s.UpsertHrv(store.HrvRow{
			Date: date, LastNightAvgMs: iptr(r.HrvMs), Status: sptr(r.HrvStatus), RawJSON: "{}",
		}); err != nil {
			return fmt.Errorf("demo: hrv: %w", err)
		}
		if err := s.UpsertBodyBattery(store.BodyBatteryRow{
			Date: date, Charged: iptr(r.BbCharged), Drained: iptr(r.BbDrained),
			High: iptr(r.BbHigh), Low: iptr(r.BbLow), RawJSON: "{}",
		}); err != nil {
			return fmt.Errorf("demo: body battery: %w", err)
		}
		if err := s.UpsertRhr(store.RhrRow{
			Date: date, RestingHR: iptr(r.Rhr), RawJSON: "{}",
		}); err != nil {
			return fmt.Errorf("demo: rhr: %w", err)
		}
	}

	// VO2max history.
	for _, v := range ds.Vo2max {
		if err := s.UpsertVo2max(store.Vo2maxRow{
			Date: dayDate(now, v.DayOffset), Vo2max: fptr(v.Vo2max), RawJSON: "{}",
		}); err != nil {
			return fmt.Errorf("demo: vo2max: %w", err)
		}
	}

	// Pre-computed stream analyses for the most recent runs.
	for _, st := range ds.Streams {
		tiz, err := json.Marshal(st.TimeInZone)
		if err != nil {
			return fmt.Errorf("demo: stream time_in_zone: %w", err)
		}
		zones, err := json.Marshal(st.Zones)
		if err != nil {
			return fmt.Errorf("demo: stream zones: %w", err)
		}
		if err := s.UpsertStreamAnalysis(store.StreamAnalysisRow{
			ActivityID:     activityID(st.ActivityDayOffset),
			TimeInZoneJSON: string(tiz),
			DecouplingPct:  fptr(st.DecouplingPct),
			PaHRFirst:      fptr(st.PaHRFirst),
			PaHRSecond:     fptr(st.PaHRSecond),
			ZonesJSON:      string(zones),
			HasHR:          st.HasHR,
			ComputedAt:     dayTime(now, st.ActivityDayOffset, 12, 0).Format(time.RFC3339),
		}); err != nil {
			return fmt.Errorf("demo: stream analysis: %w", err)
		}
	}

	// Daily decisions (last 14 days incl. today).
	for _, d := range ds.Decisions {
		date := dayDate(now, d.DayOffset)
		dow := weekdayOf(date)
		driversJSON, err := marshalDrivers(date, d.Drivers)
		if err != nil {
			return fmt.Errorf("demo: drivers: %w", err)
		}
		if err := s.UpsertDailyDecision(store.DailyDecision{
			Date:                date,
			ReadinessColor:      d.Color,
			DriversJSON:         driversJSON,
			OriginalSessionJSON: sessionJSON(date, dow, d.OriginalSession),
			AdjustedSessionJSON: sessionJSON(date, dow, d.AdjustedSession),
			Action:              d.Action,
			Rationale:           d.Rationale,
			Source:              d.Source,
		}); err != nil {
			return fmt.Errorf("demo: decision: %w", err)
		}
	}

	// Current-week plan (materialized, with the today→tempo override) + CrossFit.
	weekStart := mondayOfTime(now)
	if err := seedPlan(s, ds, now, weekStart); err != nil {
		return err
	}
	if err := seedCrossFit(s, now, weekStart); err != nil {
		return err
	}

	// Chat thread (append-only: guard against duplication on re-seed).
	if existing, err := s.ListChatMessages(1); err != nil {
		return fmt.Errorf("demo: chat check: %w", err)
	} else if len(existing) == 0 {
		for _, c := range ds.Chat {
			if _, err := s.AppendChatMessage(c.Role, c.Content); err != nil {
				return fmt.Errorf("demo: chat: %w", err)
			}
		}
	}
	return nil
}

// seedPlan materializes the current-week plan from the fixture template and
// inserts it once (idempotent). See materializePlanWeek for the placement rules
// that keep the week coherent (summing to the 30 km target) on every boot day.
func seedPlan(s *store.Store, ds Dataset, now time.Time, weekStart string) error {
	// Idempotency: skip if a plan for this week already exists.
	if _, err := s.GetLatestPlan(weekStart); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("demo: plan check: %w", err)
	}

	mon, err := time.Parse("2006-01-02", weekStart)
	if err != nil {
		return fmt.Errorf("demo: week start: %w", err)
	}
	days := materializePlanWeek(mon, dayDate(now, 0), ds.PlanWeek, decisionOffset0Original(ds.Decisions))

	plan := llm.PlanParsed{
		FitnessSummary: ds.PlanWeek.FitnessSummary,
		WeeklyTargetKm: ds.PlanWeek.WeeklyTargetKm,
		Days:           days,
		WeekRationale:  ds.PlanWeek.WeekRationale,
		OneFlag:        ds.PlanWeek.OneFlag,
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("demo: plan marshal: %w", err)
	}
	if _, err := s.InsertPlan(store.Plan{
		WeekStart:      weekStart,
		GeneratedAt:    now.UTC().Format(time.RFC3339),
		Status:         "generated",
		PlanJSON:       string(planJSON),
		FitnessSummary: plan.FitnessSummary,
		Model:          "demo",
	}); err != nil {
		return fmt.Errorf("demo: insert plan: %w", err)
	}
	return nil
}

// materializePlanWeek renders the demo plan for the ISO week of mon as a coherent
// base week that ALWAYS sums to the 30 km target regardless of which weekday the
// demo boots on: one protected long run (the week's biggest single session), one
// easy aerobic run, and today's planned tempo — the centerpiece the Today page
// softens. Distances/paces/notes come from the authored template; only the day
// placement is computed so exactly one long / one easy / one tempo land on
// distinct days and no run ever collides with "today".
func materializePlanWeek(mon time.Time, today string, tmpl PlanWeekSpec, todayTempo *SessionSpec) []llm.PlanDay {
	days := make([]llm.PlanDay, 7)
	todayIdx := -1
	for i := 0; i < 7; i++ {
		d := mon.AddDate(0, 0, i)
		date := d.Format("2006-01-02")
		days[i] = llm.PlanDay{Date: date, Dow: d.Format("Mon"), RunType: "rest", Rationale: restRationale(d.Weekday())}
		if date == today {
			todayIdx = i
		}
	}

	// Protected long: prefer the weekend (Sat, then Sun), else the latest day that
	// isn't today. One easy aerobic run on an earlier day that isn't today/long.
	longIdx := firstFree([]int{5, 6, 4, 3, 2, 1, 0}, todayIdx, -1)
	easyIdx := firstFree([]int{1, 2, 3, 4, 0, 6, 5}, todayIdx, longIdx)
	placeSession(&days[longIdx], findSession(tmpl.Days, "long"))
	placeSession(&days[easyIdx], findSession(tmpl.Days, "easy"))
	if todayIdx >= 0 {
		placeSession(&days[todayIdx], todayTempo) // today shows the planned tempo
	}
	return days
}

// placeSession copies a SessionSpec's fields onto a PlanDay (keeping its
// materialized date/dow). A nil spec leaves the day unchanged.
func placeSession(pd *llm.PlanDay, ss *SessionSpec) {
	if ss == nil {
		return
	}
	pd.RunType = ss.RunType
	pd.DistanceKm = ss.DistanceKm
	pd.PaceTarget = ss.PaceTarget
	pd.TimeNote = ss.TimeNote
	pd.OptionalIfCNS = ss.OptionalIfCNS
	pd.Rationale = ss.Rationale
}

// findSession returns the first template day of the given run_type, or nil.
func findSession(days []SessionSpec, runType string) *SessionSpec {
	for i := range days {
		if days[i].RunType == runType {
			return &days[i]
		}
	}
	return nil
}

// firstFree returns the first candidate index that is neither avoidA nor avoidB.
func firstFree(candidates []int, avoidA, avoidB int) int {
	for _, c := range candidates {
		if c != avoidA && c != avoidB {
			return c
		}
	}
	return candidates[len(candidates)-1]
}

// restRationale gives a running-rest day a plausible CrossFit-aware note.
func restRationale(wd time.Weekday) string {
	if wd == time.Monday {
		return "CrossFit leg day — running rest."
	}
	return "CrossFit / rest — legs stay fresh for lifting."
}

// seedCrossFit upserts the parsed CrossFit week(s) so the day BEFORE today always
// carries the heavy leg day today's centerpiece rationale cites ("yesterday's
// CrossFit leg day"), for EVERY boot weekday. This week's row is always seeded
// (the plan references it). When the demo boots on a Monday, yesterday is the
// previous week's Sunday, so that week is seeded too — otherwise "yesterday" would
// fall outside the current week and the rationale would reference a day that
// doesn't exist.
func seedCrossFit(s *store.Store, now time.Time, weekStart string) error {
	yesterday := dayDate(now, -1)
	if err := upsertDemoCrossFit(s, weekStart, yesterday); err != nil {
		return err
	}
	if prev := mondayOfDate(yesterday); prev != weekStart {
		if err := upsertDemoCrossFit(s, prev, yesterday); err != nil {
			return err
		}
	}
	return nil
}

// upsertDemoCrossFit seeds one CrossFit week, marking legDay (if it falls within
// the week) as a heavy squat/leg day.
func upsertDemoCrossFit(s *store.Store, weekStart, legDay string) error {
	week := demoCrossFitWeek(weekStart)
	for i := range week.Days {
		if week.Days[i].Date == legDay {
			week.Days[i].HasCrossFit = true
			week.Days[i].Focus = "Squat strength + heavy metcon"
			week.Days[i].CNSLoad = llm.LoadHigh
			week.Days[i].LegLoad = llm.LoadHigh
			week.Days[i].Notes = "Heavy back squats — legs cooked"
		}
	}
	parsed, err := json.Marshal(week)
	if err != nil {
		return fmt.Errorf("demo: crossfit marshal: %w", err)
	}
	if err := s.UpsertCrossFitWeek(store.CrossFitWeek{
		WeekStart:  weekStart,
		ParsedJSON: string(parsed),
	}); err != nil {
		return fmt.Errorf("demo: crossfit: %w", err)
	}
	return nil
}

// demoCrossFitWeek builds a plausible Mon→Sun CrossFit week anchored on weekStart.
// Shared by the seeder and the DemoRunner Stage-1 route.
func demoCrossFitWeek(weekStart string) llm.CrossFitWeekParsed {
	mon, err := time.Parse("2006-01-02", weekStart)
	if err != nil {
		mon, _ = time.Parse("2006-01-02", mondayOfTime(time.Now()))
	}
	tmpl := []struct {
		has      bool
		focus    string
		cns, leg llm.Load
		notes    string
	}{
		{true, "Squat strength + metcon", llm.LoadHigh, llm.LoadHigh, "Heavy back squats"},
		{true, "Gymnastics + engine", llm.LoadMed, llm.LoadLow, "Pull-ups, rowing intervals"},
		{true, "Olympic lifting", llm.LoadMed, llm.LoadHigh, "Clean & jerk complex"},
		{true, "Barbell skill (light)", llm.LoadLow, llm.LoadLow, "Technique day — legs fresh"},
		{true, "Conditioning", llm.LoadMed, llm.LoadMed, "Mixed-modal WOD"},
		{false, "", llm.LoadLow, llm.LoadLow, "Rest / optional long run"},
		{false, "", llm.LoadLow, llm.LoadLow, "Rest"},
	}
	days := make([]llm.CrossFitDay, 7)
	for i := 0; i < 7; i++ {
		d := mon.AddDate(0, 0, i)
		days[i] = llm.CrossFitDay{
			Date: d.Format("2006-01-02"), Dow: d.Format("Mon"),
			HasCrossFit: tmpl[i].has, Focus: tmpl[i].focus,
			CNSLoad: tmpl[i].cns, LegLoad: tmpl[i].leg, Notes: tmpl[i].notes,
		}
	}
	return llm.CrossFitWeekParsed{WeekStart: weekStart, Days: days}
}

// --- Materialization helpers ---

// activityID derives a stable, unique activity id from a day offset (one run per
// offset). Streams reference activities by the same formula.
func activityID(offset int) int64 { return 8_000_000_000 + int64(-offset) }

// dayDate returns the YYYY-MM-DD local date `offset` days from now.
func dayDate(now time.Time, offset int) string {
	d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return d.AddDate(0, 0, offset).Format("2006-01-02")
}

// dayTime returns the local time `offset` days from now at hour:min.
func dayTime(now time.Time, offset, hour, min int) time.Time {
	d := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())
	return d.AddDate(0, 0, offset)
}

// dayTimeMonday returns midnight-today (used only for weekday math).
func dayTimeMonday(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// mondayOfTime returns the Monday (YYYY-MM-DD) of the ISO week containing now.
func mondayOfTime(now time.Time) string {
	d := dayTimeMonday(now)
	off := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -off).Format("2006-01-02")
}

// mondayOfDate returns the Monday (YYYY-MM-DD) of the ISO week containing the
// given YYYY-MM-DD date (the date itself on parse failure).
func mondayOfDate(date string) string {
	d, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	off := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -off).Format("2006-01-02")
}

// marshalDrivers renders the daily_decisions.drivers_json exactly like the agent:
// ReadinessDrivers fields plus the "date", "reasons", and "stale" siblings.
func marshalDrivers(date string, d DriversSpec) (string, error) {
	wrap := struct {
		Date string `json:"date"`
		DriversSpec
	}{Date: date, DriversSpec: d}
	if wrap.Reasons == nil {
		wrap.Reasons = []string{}
	}
	b, err := json.Marshal(wrap)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// sessionJSON marshals a SessionSpec into a PlanDay-shaped JSON string (nil for
// rest/REST_DAY), stamping the materialized date + dow.
func sessionJSON(date, dow string, ss *SessionSpec) *string {
	if ss == nil {
		return nil
	}
	pd := llm.PlanDay{
		Date: date, Dow: dow, RunType: ss.RunType, DistanceKm: ss.DistanceKm,
		PaceTarget: ss.PaceTarget, TimeNote: ss.TimeNote,
		OptionalIfCNS: ss.OptionalIfCNS, Rationale: ss.Rationale,
	}
	b, _ := json.Marshal(pd)
	return sptr(string(b))
}

// decisionOffset0Original returns today's (offset 0) original session, or nil.
func decisionOffset0Original(decs []DecisionSpec) *SessionSpec {
	for _, d := range decs {
		if d.DayOffset == 0 {
			return d.OriginalSession
		}
	}
	return nil
}

func fptr(v float64) *float64 { return &v }
func iptr(v int64) *int64     { return &v }
func sptr(v string) *string   { return &v }

// round1 rounds to one decimal place (kept deterministic for fixture stability).
func round1(v float64) float64 { return math.Round(v*10) / 10 }
