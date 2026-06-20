// Package metrics computes deterministic fitness numbers from M0 store rows.
// All functions here are pure (no DB, no clock) so they are table-test friendly:
// callers pass plain slices and an explicit `now`.
package metrics

import (
	"fmt"
	"math"
	"sort"
	"time"

	"help-my-run/backend/internal/store"
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

// runTypes are the Strava activity types counted as runs for volume/load.
var runTypes = map[string]bool{
	"Run":        true,
	"TrailRun":   true,
	"VirtualRun": true,
}

// isRun reports whether a Strava activity type counts toward running volume.
func isRun(typ string) bool { return runTypes[typ] }

// parseStart parses an activity StartTime (RFC3339 UTC). ok=false if unparseable.
func parseStart(startTime string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// distanceKmInWindow sums run distance (km) for activities whose start time is
// within (from, to] — from exclusive, to inclusive. Non-runs and unparseable
// rows are skipped.
func distanceKmInWindow(acts []store.Activity, from, to time.Time) float64 {
	var km float64
	for _, a := range acts {
		if !isRun(a.Type) {
			continue
		}
		t, ok := parseStart(a.StartTime)
		if !ok {
			continue
		}
		if t.After(from) && !t.After(to) {
			km += a.DistanceM / 1000.0
		}
	}
	return km
}

// weeklyVolumeKm is run km over the last 7 days (now-7d, now].
func weeklyVolumeKm(acts []store.Activity, now time.Time) float64 {
	return distanceKmInWindow(acts, now.AddDate(0, 0, -7), now)
}

// fourWeekAvgKm is mean weekly run km over the last 28 days.
func fourWeekAvgKm(acts []store.Activity, now time.Time) float64 {
	total := distanceKmInWindow(acts, now.AddDate(0, 0, -28), now)
	return total / 4.0
}

// round2 rounds to 2 decimal places.
func round2(v float64) float64 { return math.Round(v*100) / 100 }

// acuteChronicRatio is the 7-day load divided by the 28-day mean weekly load,
// both using run km as the load proxy. Returns 0 when there is no chronic
// baseline (28-day load is 0). Balanced is roughly 0.8–1.3.
func acuteChronicRatio(acts []store.Activity, now time.Time) float64 {
	acute := distanceKmInWindow(acts, now.AddDate(0, 0, -7), now)
	chronic := distanceKmInWindow(acts, now.AddDate(0, 0, -28), now) / 4.0
	if chronic == 0 {
		return 0
	}
	return round2(acute / chronic)
}

// median returns the median of a sorted, non-empty slice.
func median(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2.0
}

// runPacesSecPerKm returns sorted (ascending) sec/km for qualifying runs in the
// last 28 days: runs with positive distance and moving time.
func runPacesSecPerKm(acts []store.Activity, now time.Time) []float64 {
	from := now.AddDate(0, 0, -28)
	var paces []float64
	for _, a := range acts {
		if !isRun(a.Type) || a.DistanceM <= 0 || a.MovingTimeS <= 0 {
			continue
		}
		t, ok := parseStart(a.StartTime)
		if !ok || !t.After(from) || t.After(now) {
			continue
		}
		secPerKm := float64(a.MovingTimeS) / (a.DistanceM / 1000.0)
		paces = append(paces, secPerKm)
	}
	sort.Float64s(paces)
	return paces
}

// paceEstimates returns (easyPace, thresholdPace) formatted as "M:SS/km" from
// activity summaries over the last 28 days. Easy = median of all qualifying
// runs; threshold = median of the fastest 25% (never slower than easy). Returns
// ("","") when there are no qualifying runs.
func paceEstimates(acts []store.Activity, now time.Time) (string, string) {
	paces := runPacesSecPerKm(acts, now)
	if len(paces) == 0 {
		return "", ""
	}
	easySec := median(paces)

	// Fastest 25% (at least 1). paces is ascending, so fastest are at the front.
	k := int(math.Ceil(float64(len(paces)) * 0.25))
	if k < 1 {
		k = 1
	}
	thrSec := median(paces[:k])
	if thrSec > easySec {
		thrSec = easySec
	}
	return formatPace(easySec), formatPace(thrSec)
}
