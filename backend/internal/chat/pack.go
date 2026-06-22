package chat

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/streams"
)

// Pack size caps (token budget). Summary-level only; no raw streams.
const (
	activityPackLimit = 14
	recoveryPackLimit = 14
	streamPackLimit   = 5

	// activityCandidateLimit is the DB-level fetch size for activities. It must
	// exceed activityPackLimit so the run filter (metrics.IsRun) can still yield
	// a full activityPackLimit runs even when recent non-runs (e.g. Ride) occupy
	// the most-recent slots. 200 matches the codebase floor used by coach.Fitness
	// and progress.activityLimit. ListActivities is most-recent-first.
	activityCandidateLimit = 200
)

// Engine is the M3.3 chat engine: builds the deterministic context pack, sends
// it + the last-N turns + the new message to claude -p, persists both turns, and
// returns the assistant turn. On any llm.Call failure it returns the typed error
// (no fallback, no fabrication) — the one divergence from coach/progress.
// *Engine satisfies api.Chat.
type Engine struct {
	store        *store.Store
	llm          *llm.Client
	progress     *progress.Engine // reused for the pack's Signals block
	model        string
	historyTurns int // CHAT_HISTORY_TURNS (last-N turns sent per call)
}

// New constructs a chat Engine. Reuses the SHARED llm.Client from main.go.
func New(s *store.Store, c *llm.Client, p *progress.Engine, model string, historyTurns int) *Engine {
	return &Engine{store: s, llm: c, progress: p, model: model, historyTurns: historyTurns}
}

// ChatContextPack is the deterministic, token-bounded summary of the athlete's
// data sent to claude -p alongside the question. Summary-level only (no raw
// streams). Serialized with json.Marshal (compact) onto stdin.
type ChatContextPack struct {
	GeneratedAt   string                  `json:"generated_at"`   // RFC3339 UTC
	Profile       ProfilePack             `json:"profile"`
	Signals       []progress.TrendSummary `json:"signals"`        // M3.1 trends, fixed order
	Activities    []ActivityPack          `json:"activities"`     // last ~14 runs, summary fields
	Recovery      []RecoveryPack          `json:"recovery"`       // last ~14 days, summary
	StreamSummary []StreamPack            `json:"stream_summary"` // last ~5 runs: TIZ + decoupling
}

// ProfilePack mirrors coach.ProfilePack (zones/goal/constraints).
type ProfilePack struct {
	TargetWeeklyKm  float64         `json:"target_weekly_km"`
	ProgressionMode string          `json:"progression_mode"`
	Zone2CeilingBpm *int64          `json:"zone2_ceiling_bpm"`
	ThresholdBpm    *int64          `json:"threshold_bpm"`
	MaxHRBpm        *int64          `json:"max_hr_bpm"`
	RunConstraints  json.RawMessage `json:"run_constraints"` // {} if empty/invalid
	GoalText        string          `json:"goal_text"`
}

// ActivityPack is one recent run summarized (no raw_json).
type ActivityPack struct {
	StartTime   string   `json:"start_time"`  // RFC3339
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	DistanceKm  float64  `json:"distance_km"` // DistanceM/1000, rounded
	MovingTimeS int64    `json:"moving_time_s"`
	Pace        string   `json:"pace"` // metrics.FormatPace(MovingTimeS/(DistanceM/1000))
	AvgHR       *float64 `json:"avg_hr"`
	MaxHR       *float64 `json:"max_hr"`
	AvgCadence  *float64 `json:"avg_cadence"`
	ElevGainM   *float64 `json:"elev_gain_m"`
}

// RecoveryPack is one day's recovery summary (flattened from store.RecoveryDay).
type RecoveryPack struct {
	Date           string   `json:"date"`        // YYYY-MM-DD
	SleepHours     *float64 `json:"sleep_hours"` // SleepFields.DurationS/3600
	SleepScore     *int64   `json:"sleep_score"`
	HRVLastNightMs *int64   `json:"hrv_last_night_ms"`
	HRVStatus      *string  `json:"hrv_status"`
	RestingHR      *int64   `json:"resting_hr"`
	BodyBatteryHi  *int64   `json:"body_battery_high"`
}

// StreamPack is one recent run's time-in-zone + decoupling.
type StreamPack struct {
	ActivityID    int64     `json:"activity_id"`
	ComputedAt    string    `json:"computed_at"`
	HasHR         bool      `json:"has_hr"`
	TimeInZone    []ZonePct `json:"time_in_zone"` // decoded from TimeInZoneJSON
	DecouplingPct *float64  `json:"decoupling_pct"`
}

// ZonePct is a trimmed streams.ZoneTime (drop seconds; pct is enough for the LLM).
type ZonePct struct {
	Zone int     `json:"zone"`
	Pct  float64 `json:"pct"`
}

// buildContextPack assembles the pack from the live store + the progress engine.
// All getters here are verbatim store/engine signatures. A missing profile
// (store.ErrNotFound) or any store error is a setup error returned to the caller.
func (e *Engine) buildContextPack(ctx context.Context) (ChatContextPack, error) {
	// Signals: reuse progress.Report (folds in acts/recovery/vo2max/profile/streams
	// + ComputeProgress) for the fixed 6 trend keys.
	rep, err := e.progress.Report(ctx, progress.DefaultWeeks)
	if err != nil {
		return ChatContextPack{}, err
	}

	prof, err := e.store.GetAthleteProfile()
	if err != nil {
		return ChatContextPack{}, err
	}
	rc := json.RawMessage(prof.RunConstraintsJSON)
	if len(rc) == 0 || !json.Valid(rc) {
		rc = json.RawMessage(`{}`)
	}

	acts, err := e.store.ListActivities(activityCandidateLimit)
	if err != nil {
		return ChatContextPack{}, err
	}
	rec, err := e.store.ListRecovery(recoveryPackLimit)
	if err != nil {
		return ChatContextPack{}, err
	}
	saRows, err := e.store.ListStreamAnalyses(streamPackLimit)
	if err != nil {
		return ChatContextPack{}, err
	}

	return ChatContextPack{
		GeneratedAt: nowUTC(),
		Profile: ProfilePack{
			TargetWeeklyKm:  prof.TargetWeeklyKm,
			ProgressionMode: prof.ProgressionMode,
			Zone2CeilingBpm: prof.Zone2CeilingBpm,
			ThresholdBpm:    prof.ThresholdBpm,
			MaxHRBpm:        prof.MaxHRBpm,
			RunConstraints:  rc,
			GoalText:        prof.GoalText,
		},
		Signals:       rep.Signals,
		Activities:    activityPacks(acts),
		Recovery:      recoveryPacks(rec),
		StreamSummary: streamPacks(saRows),
	}, nil
}

// activityPacks filters to runs (metrics.IsRun), caps at activityPackLimit, and
// summarizes each (pace derived; distance in km). Returns a non-nil slice.
func activityPacks(acts []store.Activity) []ActivityPack {
	out := make([]ActivityPack, 0, activityPackLimit)
	for _, a := range acts {
		if !metrics.IsRun(a.Type) {
			continue
		}
		if len(out) >= activityPackLimit {
			break
		}
		km := a.DistanceM / 1000
		pace := ""
		if a.DistanceM > 0 {
			pace = metrics.FormatPace(float64(a.MovingTimeS) / km)
		}
		out = append(out, ActivityPack{
			StartTime:   a.StartTime,
			Name:        a.Name,
			Type:        a.Type,
			DistanceKm:  round1(km),
			MovingTimeS: a.MovingTimeS,
			Pace:        pace,
			AvgHR:       a.AvgHR,
			MaxHR:       a.MaxHR,
			AvgCadence:  a.AvgCadence,
			ElevGainM:   a.ElevationGainM,
		})
	}
	return out
}

// recoveryPacks flattens store.RecoveryDay sub-records (nil-guarded). Returns a
// non-nil slice capped at recoveryPackLimit.
func recoveryPacks(rec []store.RecoveryDay) []RecoveryPack {
	out := make([]RecoveryPack, 0, recoveryPackLimit)
	for _, d := range rec {
		if len(out) >= recoveryPackLimit {
			break
		}
		rp := RecoveryPack{Date: d.Date}
		if d.Sleep != nil {
			if d.Sleep.DurationS != nil {
				h := round1(float64(*d.Sleep.DurationS) / 3600)
				rp.SleepHours = &h
			}
			rp.SleepScore = d.Sleep.Score
		}
		if d.HRV != nil {
			rp.HRVLastNightMs = d.HRV.LastNightAvgMs
			rp.HRVStatus = d.HRV.Status
		}
		if d.RHR != nil {
			rp.RestingHR = d.RHR.RestingHR
		}
		if d.BodyBattery != nil {
			rp.BodyBatteryHi = d.BodyBattery.High
		}
		out = append(out, rp)
	}
	return out
}

// streamPacks decodes TimeInZoneJSON -> []streams.ZoneTime, projects to ZonePct,
// and carries decoupling/has_hr/computed_at. Returns a non-nil slice capped at
// streamPackLimit. A row with unparseable TimeInZoneJSON gets an empty zone list.
func streamPacks(rows []store.StreamAnalysisRow) []StreamPack {
	out := make([]StreamPack, 0, streamPackLimit)
	for _, r := range rows {
		if len(out) >= streamPackLimit {
			break
		}
		var zt []streams.ZoneTime
		_ = json.Unmarshal([]byte(r.TimeInZoneJSON), &zt) // empty on error -> nil zones below
		zones := make([]ZonePct, 0, len(zt))
		for _, z := range zt {
			zones = append(zones, ZonePct{Zone: z.Zone, Pct: round1(z.Pct)})
		}
		out = append(out, StreamPack{
			ActivityID:    r.ActivityID,
			ComputedAt:    r.ComputedAt,
			HasHR:         r.HasHR,
			TimeInZone:    zones,
			DecouplingPct: r.DecouplingPct,
		})
	}
	return out
}

// round1 rounds to one decimal place (token-budget tidiness for the pack).
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}

// nowUTC is the server clock as RFC3339 UTC (matches store timestamping).
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
