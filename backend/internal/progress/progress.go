// Package progress computes deterministic cardio-capacity trend signals from
// M0 store rows + M1 metrics. ComputeProgress is pure (no DB, no clock): callers
// pass slices + an explicit `now`, so it is table-test friendly (mirrors metrics).
package progress

import (
	"time"

	"help-my-run/backend/internal/store"
)

// Canonical signal keys (CONTRACTS §3.1) — use verbatim.
const (
	SignalPaceAtHR    = "pace_at_hr"   // headline: weekly-median pace of in-band runs (sec/km)
	SignalVo2max      = "vo2max"       // Garmin VO2max
	SignalRestingHR   = "resting_hr"   // garmin_rhr
	SignalHRVBaseline = "hrv_baseline" // garmin_hrv last-night avg ms
	SignalWeeklyLoad  = "weekly_load"  // weekly running km (M1 metrics)
)

// Window constants (CONTRACTS §3.3).
const (
	DefaultWeeks         = 12
	MinWeeks             = 4
	MaxWeeks             = 52
	enoughDataMinSignals = 2 // >= this many FITNESS signals (weekly_load excluded) with >= 2 non-nil weekly points
)

// Reference-HR band constants (CONTRACTS §3.4).
const (
	// refHRBandBpm is the ± window around the reference HR (spec §7: ±5 bpm).
	refHRBandBpm = 5.0
	// defaultRefHRBpm is the fallback reference HR when profile.Zone2CeilingBpm
	// is nil (documented constant per spec §7).
	defaultRefHRBpm = 145.0
	// paceEps is the sec/km deadband for pace_at_hr direction classification.
	paceEps = 0.5
	// relDeadband is the relative (fraction) deadband for the non-pace signals
	// (mirrors metrics.recoveryDeadband = 0.03).
	relDeadband = 0.03
)

// TrendDirection is the value-movement direction of a signal over the window.
type TrendDirection string

const (
	DirectionUp   TrendDirection = "up"
	DirectionDown TrendDirection = "down"
	DirectionFlat TrendDirection = "flat"
)

// TrendSummary is one signal's trend card: weekly series + headline summary.
// Series has exactly weeks entries, oldest-first; nil = a week with no
// qualifying data (rendered as a gap, never interpolated).
type TrendSummary struct {
	Key           string         `json:"key"`
	Label         string         `json:"label"`
	Unit          string         `json:"unit"`
	Current       *float64       `json:"current"`
	Baseline      *float64       `json:"baseline"`
	DeltaAbs      *float64       `json:"delta_abs"`
	Direction     TrendDirection `json:"direction"`
	LowerIsBetter bool           `json:"lower_is_better"`
	Series        []*float64     `json:"series"`
}

// ProgressReport is the full deterministic read served at GET /api/progress.
type ProgressReport struct {
	Weeks       int            `json:"weeks"`
	GeneratedAt string         `json:"generated_at"`
	Signals     []TrendSummary `json:"signals"`
	EnoughData  bool           `json:"enough_data"`
}

// weekBucket is a half-open 7-day window (start, end] in UTC.
type weekBucket struct {
	start time.Time
	end   time.Time
}

// ensure store is referenced (used by ComputeProgress in a later task).
var _ = store.Activity{}
