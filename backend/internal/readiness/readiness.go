// Package readiness computes a deterministic daily readiness gate (GREEN/AMBER/RED)
// from M0 garmin_* recovery rows plus the M1 recovery-trend signal. Like internal/metrics
// it is PURE: callers pass plain slices and an explicit `now`; there is no DB and no clock,
// so the engine is fully table-test friendly. JSON tags are snake_case (repo convention).
package readiness

import (
	"time"

	"help-my-run/backend/internal/store"
)

// Color is the readiness gate result.
type Color string

const (
	ColorGreen Color = "green"
	ColorAmber Color = "amber"
	ColorRed   Color = "red"
)

// Baseline + classification thresholds. Exported so tests reference the same numbers
// the engine uses (no magic-number drift between impl and tests).
const (
	BaselineWindowDays = 14 // baseline = mean over the prior up-to-14 days (excluding last night)
	MinBaselineDays    = 3  // fewer than this with the metric -> baseline unavailable

	RedSleepHours   = 5.0 // < 5.0h slept -> RED contribution
	AmberSleepHours = 6.5 // < 6.5h slept -> AMBER contribution
	RedSleepScore   = 50  // sleep score < 50 -> RED
	AmberSleepScore = 65  // sleep score < 65 -> AMBER

	RedHRVDropPct   = -15.0 // HRV delta <= -15% vs baseline -> RED
	AmberHRVDropPct = -7.0  // HRV delta <= -7% vs baseline -> AMBER

	RedRHRRiseBpm   = 7.0 // RHR delta >= +7 bpm vs baseline -> RED
	AmberRHRRiseBpm = 4.0 // RHR delta >= +4 bpm vs baseline -> AMBER

	RedBodyBattery   = 30 // overnight BodyBattery high < 30 -> RED
	AmberBodyBattery = 50 // overnight BodyBattery high < 50 -> AMBER
)

// ReadinessDrivers are the raw numbers that decided the color. Pointers are nil
// when the underlying Garmin metric (or its baseline) is missing for last night.
type ReadinessDrivers struct {
	Date            string   `json:"date"`              // local date assessed (YYYY-MM-DD)
	SleepHours      *float64 `json:"sleep_hours"`       // last night sleep duration in hours
	SleepScore      *int64   `json:"sleep_score"`       // Garmin sleep score 0-100
	HRVLastNightMs  *int64   `json:"hrv_last_night_ms"` // overnight avg HRV
	HRVBaselineMs   *float64 `json:"hrv_baseline_ms"`   // mean HRV over baseline window
	HRVDeltaPct     *float64 `json:"hrv_delta_pct"`     // (last - baseline)/baseline * 100
	RHRLastNight    *int64   `json:"rhr_last_night"`    // last night resting HR
	RHRBaseline     *float64 `json:"rhr_baseline"`      // mean RHR over baseline window
	RHRDeltaBpm     *float64 `json:"rhr_delta_bpm"`     // last - baseline (positive = elevated)
	BodyBatteryHigh *int64   `json:"body_battery_high"` // last night BodyBattery.High (overnight peak)
	RecoveryTrend   string   `json:"recovery_trend"`    // "improving"|"stable"|"declining"
	DataComplete    bool     `json:"data_complete"`     // false if last-night data missing -> conservative AMBER
}

// Readiness is the readiness engine's output.
type Readiness struct {
	Color   Color            `json:"color"`
	Drivers ReadinessDrivers `json:"drivers"`
	Reasons []string         `json:"reasons"` // human-readable bullets, e.g. "HRV -18% vs baseline"
}

// Assess computes readiness from recovery rows (most-recent-first, as ListRecovery
// returns) for the given local date. `now` is unused for arithmetic but kept for
// signature symmetry with metrics and future trend windows.
//
// Implemented in a later step; this stub keeps the package compiling for the
// types/constants tests.
func Assess(recovery []store.RecoveryDay, now time.Time) Readiness {
	return Readiness{}
}
