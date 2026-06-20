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

// recoveryDeadband is the relative change (fraction) below which a signal's
// recent-vs-older difference is treated as flat.
const recoveryDeadband = 0.03

// avgPtr averages the non-nil int64 values; ok=false if none present.
func avgPtr(vals []*int64) (float64, bool) {
	var sum float64
	var n int
	for _, v := range vals {
		if v != nil {
			sum += float64(*v)
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

// signalVote compares recent vs older averages and returns +1 (improving),
// -1 (declining), or 0 (flat/absent). higherIsBetter inverts the polarity for
// signals where a lower value is better.
func signalVote(recent, older []*int64, higherIsBetter bool) int {
	r, okR := avgPtr(recent)
	o, okO := avgPtr(older)
	if !okR || !okO || o == 0 {
		return 0
	}
	change := (r - o) / o
	if change > recoveryDeadband {
		if higherIsBetter {
			return 1
		}
		return -1
	}
	if change < -recoveryDeadband {
		if higherIsBetter {
			return -1
		}
		return 1
	}
	return 0
}

// recoveryTrend classifies the recent ~14-day recovery direction as
// "improving" | "stable" | "declining" by majority vote across HRV
// (last-night avg ms), sleep score, and Body Battery net (charged-drained).
// recovery is most-recent-first (as ListRecovery returns). Needs >= 2 days.
func recoveryTrend(recovery []store.RecoveryDay) string {
	rows := recovery
	if len(rows) > 14 {
		rows = rows[:14]
	}
	if len(rows) < 2 {
		return "stable"
	}
	half := len(rows) / 2
	recent := rows[:half]
	older := rows[half:]

	collect := func(days []store.RecoveryDay, pick func(store.RecoveryDay) *int64) []*int64 {
		out := make([]*int64, 0, len(days))
		for _, d := range days {
			out = append(out, pick(d))
		}
		return out
	}

	hrv := func(d store.RecoveryDay) *int64 {
		if d.HRV == nil {
			return nil
		}
		return d.HRV.LastNightAvgMs
	}
	sleep := func(d store.RecoveryDay) *int64 {
		if d.Sleep == nil {
			return nil
		}
		return d.Sleep.Score
	}
	bbNet := func(d store.RecoveryDay) *int64 {
		if d.BodyBattery == nil || d.BodyBattery.Charged == nil || d.BodyBattery.Drained == nil {
			return nil
		}
		net := *d.BodyBattery.Charged - *d.BodyBattery.Drained
		return &net
	}

	score := 0
	score += signalVote(collect(recent, hrv), collect(older, hrv), true)
	score += signalVote(collect(recent, sleep), collect(older, sleep), true)
	score += signalVote(collect(recent, bbNet), collect(older, bbNet), true)

	switch {
	case score > 0:
		return "improving"
	case score < 0:
		return "declining"
	default:
		return "stable"
	}
}

// cutbackEpoch is an anchor Monday used to index weeks for the cutback cadence.
var cutbackEpoch = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

// weekIndexSince returns the number of whole 7-day weeks from cutbackEpoch to t.
func weekIndexSince(t time.Time) int {
	days := int(t.UTC().Sub(cutbackEpoch).Hours() / 24)
	return days / 7
}

// isCutbackWeek reports whether the week containing `now` is a cutback week
// (every 4th week: weekIndex % 4 == 3).
func isCutbackWeek(now time.Time) bool {
	return weekIndexSince(now)%4 == 3
}

// round1 rounds to 1 decimal place.
func round1(v float64) float64 { return math.Round(v*10) / 10 }

// safeWeeklyTarget computes the next-week volume target from a baseline (recent
// run km) and the athlete profile. Cutback weeks pull back to 80% of baseline.
// "build" mode ramps +10% (capped at 1.5× the profile's stated target, floored
// at baseline); "hold" holds baseline. With no history (baseline <= 0) it falls
// back to the profile's stated target. Rounded to 1 decimal.
func safeWeeklyTarget(baseline float64, profile store.AthleteProfile, cutback bool) float64 {
	if baseline <= 0 {
		baseline = profile.TargetWeeklyKm
	}
	if cutback {
		return round1(baseline * 0.80)
	}
	if profile.ProgressionMode == "hold" {
		return round1(baseline)
	}
	// "build" (default): +10% ramp, capped at 1.5× stated target, floored at baseline.
	target := baseline * 1.10
	if cap := profile.TargetWeeklyKm * 1.5; cap > 0 && target > cap {
		target = cap
	}
	if target < baseline {
		target = baseline
	}
	return round1(target)
}
