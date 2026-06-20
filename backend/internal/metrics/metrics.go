// Package metrics computes deterministic fitness numbers from M0 store rows.
// All functions here are pure (no DB, no clock) so they are table-test friendly:
// callers pass plain slices and an explicit `now`.
package metrics

import (
	"fmt"
	"math"
)

// FitnessMetrics is the computed fitness read returned by ComputeFitness and
// served at GET /api/fitness. JSON tags are snake_case (matches M0 dto.go).
type FitnessMetrics struct {
	WeeklyVolumeKm     float64 `json:"weekly_volume_km"`      // recent (last 7-day) running km
	FourWeekAvgKm      float64 `json:"four_week_avg_km"`      // mean weekly km over last 4 wks
	AcuteChronicRatio  float64 `json:"acute_chronic_ratio"`   // 7-day vs 28-day load ratio
	EasyPace           string  `json:"easy_pace"`             // "6:00/km"
	ThresholdPace      string  `json:"threshold_pace"`        // "5:05/km"
	RecoveryTrend      string  `json:"recovery_trend"`        // "improving" | "stable" | "declining"
	SafeWeeklyTargetKm float64 `json:"safe_weekly_target_km"` // baseline × progression, ≤~10% ramp
	IsCutbackWeek      bool    `json:"is_cutback_week"`
}

// formatPace renders seconds-per-km as "M:SS/km" (zero-padded seconds).
// Returns "" for non-positive input (no data).
func formatPace(secPerKm float64) string {
	if secPerKm <= 0 {
		return ""
	}
	total := int(math.Round(secPerKm))
	min := total / 60
	sec := total % 60
	return fmt.Sprintf("%d:%02d/km", min, sec)
}
