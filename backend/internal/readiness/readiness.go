// Package readiness computes a deterministic daily readiness gate (GREEN/AMBER/RED)
// from M0 garmin_* recovery rows plus the M1 recovery-trend signal. Like internal/metrics
// it is PURE: callers pass plain slices and an explicit `now`; there is no DB and no clock,
// so the engine is fully table-test friendly. JSON tags are snake_case (repo convention).
package readiness

import (
	"fmt"
	"time"

	"help-my-run/backend/internal/metrics"
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

// signalLevel is the per-signal severity used internally during aggregation.
type signalLevel int

const (
	levelGreen signalLevel = iota
	levelAmber
	levelRed
)

// worse returns the more severe of two levels.
func worse(a, b signalLevel) signalLevel {
	if b > a {
		return b
	}
	return a
}

// pick collects one *int64 metric across the given days (used to build a baseline
// window). days are the prior days (recovery[1:1+BaselineWindowDays]).
func pick(days []store.RecoveryDay, get func(store.RecoveryDay) *int64) []*int64 {
	out := make([]*int64, 0, len(days))
	for _, d := range days {
		out = append(out, get(d))
	}
	return out
}

// Assess computes readiness from recovery rows (most-recent-first, as ListRecovery
// returns) for the given local date. `now` is unused for arithmetic but kept for
// signature symmetry with metrics and future trend windows.
func Assess(recovery []store.RecoveryDay, now time.Time) Readiness {
	_ = now // reserved for future trend windows; arithmetic is row-driven.

	drivers := ReadinessDrivers{
		RecoveryTrend: metricsTrend(recovery),
		DataComplete:  true,
	}
	var reasons []string

	if len(recovery) == 0 {
		drivers.DataComplete = false
		return Readiness{
			Color:   ColorAmber,
			Drivers: drivers,
			Reasons: []string{"No recovery data for last night"},
		}
	}

	last := recovery[0]
	drivers.Date = last.Date

	var window []store.RecoveryDay
	if len(recovery) > 1 {
		end := 1 + BaselineWindowDays
		if end > len(recovery) {
			end = len(recovery)
		}
		window = recovery[1:end]
	}

	level := levelGreen
	amberCount := 0
	redCount := 0
	note := func(sig signalLevel, reason string) {
		switch sig {
		case levelAmber:
			amberCount++
			reasons = append(reasons, reason)
		case levelRed:
			redCount++
			reasons = append(reasons, reason)
		}
		level = worse(level, sig)
	}

	// --- Sleep hours ---
	if sh := sleepHours(last.Sleep); sh != nil {
		drivers.SleepHours = sh
		switch {
		case *sh < RedSleepHours:
			note(levelRed, fmt.Sprintf("Sleep %.1fh (<%.1fh)", *sh, RedSleepHours))
		case *sh < AmberSleepHours:
			note(levelAmber, fmt.Sprintf("Sleep %.1fh (<%.1fh)", *sh, AmberSleepHours))
		}
	} else {
		drivers.DataComplete = false
	}

	// --- Sleep score ---
	if last.Sleep != nil && last.Sleep.Score != nil {
		ss := *last.Sleep.Score
		drivers.SleepScore = ptrI(ss)
		switch {
		case ss < RedSleepScore:
			note(levelRed, fmt.Sprintf("Sleep score %d (<%d)", ss, RedSleepScore))
		case ss < AmberSleepScore:
			note(levelAmber, fmt.Sprintf("Sleep score %d (<%d)", ss, AmberSleepScore))
		}
	}

	// --- HRV vs baseline ---
	if last.HRV != nil && last.HRV.LastNightAvgMs != nil {
		hv := *last.HRV.LastNightAvgMs
		drivers.HRVLastNightMs = ptrI(hv)
		if base, ok := baseline(pick(window, func(d store.RecoveryDay) *int64 {
			if d.HRV == nil {
				return nil
			}
			return d.HRV.LastNightAvgMs
		})); ok {
			drivers.HRVBaselineMs = ptrF(base)
			delta := pctDelta(hv, base)
			drivers.HRVDeltaPct = ptrF(delta)
			switch {
			case delta <= RedHRVDropPct:
				note(levelRed, fmt.Sprintf("HRV %.1f%% vs baseline", delta))
			case delta <= AmberHRVDropPct:
				note(levelAmber, fmt.Sprintf("HRV %.1f%% vs baseline", delta))
			}
		} else {
			drivers.DataComplete = false
		}
	} else {
		drivers.DataComplete = false
	}

	// --- RHR vs baseline ---
	if last.RHR != nil && last.RHR.RestingHR != nil {
		rv := *last.RHR.RestingHR
		drivers.RHRLastNight = ptrI(rv)
		if base, ok := baseline(pick(window, func(d store.RecoveryDay) *int64 {
			if d.RHR == nil {
				return nil
			}
			return d.RHR.RestingHR
		})); ok {
			drivers.RHRBaseline = ptrF(base)
			delta := bpmDelta(rv, base)
			drivers.RHRDeltaBpm = ptrF(delta)
			switch {
			case delta >= RedRHRRiseBpm:
				note(levelRed, fmt.Sprintf("RHR +%.1f bpm vs baseline", delta))
			case delta >= AmberRHRRiseBpm:
				note(levelAmber, fmt.Sprintf("RHR +%.1f bpm vs baseline", delta))
			}
		} else {
			drivers.DataComplete = false
		}
	} else {
		drivers.DataComplete = false
	}

	// --- Body Battery overnight high ---
	if last.BodyBattery != nil && last.BodyBattery.High != nil {
		bb := *last.BodyBattery.High
		drivers.BodyBatteryHigh = ptrI(bb)
		switch {
		case bb < RedBodyBattery:
			note(levelRed, fmt.Sprintf("Body Battery high %d (<%d)", bb, RedBodyBattery))
		case bb < AmberBodyBattery:
			note(levelAmber, fmt.Sprintf("Body Battery high %d (<%d)", bb, AmberBodyBattery))
		}
	} else {
		drivers.DataComplete = false
	}

	// --- Recovery-trend modifier: applied ONLY when NO direct signal already fired. ---
	// Intentional, shipped semantics (do not "fix" to always apply):
	//   - "declining": adds exactly one amber-weight signal, but ONLY when no
	//     per-signal amber/red was recorded above. If any direct signal already
	//     fired, the trend is deliberately NOT additive — otherwise a single
	//     direct amber + a trend amber would total 2 ambers and confirm a
	//     spurious RED, and a direct red would be double-counted. So the trend
	//     can promote GREEN->AMBER but never pushes an already-flagged day to a
	//     worse color.
	//   - "improving" (and "stable"): no-op. The trend never cancels or downgrades
	//     a direct concern, and with no direct concern there is nothing to act on.
	if amberCount == 0 && redCount == 0 {
		switch drivers.RecoveryTrend {
		case "declining":
			amberCount++
			level = worse(level, levelAmber)
			reasons = append(reasons, "Recovery trend declining")
		case "improving":
			// no-op: nothing to cancel (see block comment).
		}
	}

	// --- Aggregate (worst-wins with confirmation). ---
	var color Color
	switch {
	case redCount >= 1 || amberCount >= 2:
		color = ColorRed
	case amberCount == 1 || !drivers.DataComplete:
		color = ColorAmber
	default:
		color = ColorGreen
	}

	if !drivers.DataComplete && color == ColorAmber {
		reasons = append(reasons, "Incomplete last-night data — conservative")
	}

	return Readiness{Color: color, Drivers: drivers, Reasons: reasons}
}

// metricsTrend delegates to the exported metrics.RecoveryTrend so readiness reuses
// the identical M1 trend computation rather than re-implementing it.
func metricsTrend(recovery []store.RecoveryDay) string {
	return metrics.RecoveryTrend(recovery)
}

// ptrF / ptrI are convenience constructors for the *float64 / *int64 driver fields.
func ptrF(v float64) *float64 { return &v }
func ptrI(v int64) *int64     { return &v }

// sleepHours converts a sleep record's duration (seconds) to hours; nil if absent.
func sleepHours(s *store.SleepFields) *float64 {
	if s == nil || s.DurationS == nil {
		return nil
	}
	h := float64(*s.DurationS) / 3600.0
	return &h
}

// meanI64 averages the non-nil values; ok=false if none present. count is the
// number of non-nil values averaged.
func meanI64(vals []*int64) (mean float64, count int) {
	var sum float64
	for _, v := range vals {
		if v != nil {
			sum += float64(*v)
			count++
		}
	}
	if count == 0 {
		return 0, 0
	}
	return sum / float64(count), count
}

// baseline returns the mean of the available values over the baseline window,
// ok=false when fewer than MinBaselineDays non-nil values are present (baseline
// unavailable -> that signal contributes no delta and forces DataComplete=false).
func baseline(vals []*int64) (mean float64, ok bool) {
	m, count := meanI64(vals)
	if count < MinBaselineDays {
		return 0, false
	}
	return m, true
}

// pctDelta is (last-baseline)/baseline*100; 0 when baseline is 0.
func pctDelta(last int64, base float64) float64 {
	if base == 0 {
		return 0
	}
	return (float64(last) - base) / base * 100.0
}

// bpmDelta is last-baseline (positive = elevated RHR).
func bpmDelta(last int64, base float64) float64 {
	return float64(last) - base
}
