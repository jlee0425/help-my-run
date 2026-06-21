// Package progress computes deterministic cardio-capacity trend signals from
// M0 store rows + M1 metrics. ComputeProgress is pure (no DB, no clock): callers
// pass slices + an explicit `now`, so it is table-test friendly (mirrors metrics).
package progress

import (
	"math"
	"sort"
	"time"

	"help-my-run/backend/internal/metrics"
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

// weekBuckets returns `weeks` contiguous half-open 7-day windows (start, end]
// ending at now, oldest-first (so index 0 is the oldest week).
func weekBuckets(weeks int, now time.Time) []weekBucket {
	out := make([]weekBucket, weeks)
	end := now
	for i := weeks - 1; i >= 0; i-- {
		start := end.AddDate(0, 0, -7)
		out[i] = weekBucket{start: start, end: end}
		end = start
	}
	return out
}

// summarize derives (current, baseline, deltaAbs, direction) from a weekly
// series. current = last non-nil; baseline = first non-nil; deltaAbs =
// current-baseline. Direction is the raw VALUE movement (up = value increased),
// independent of lowerIsBetter (the app maps direction+lowerIsBetter to a
// good/bad color). isPace selects the absolute paceEps deadband; otherwise a
// relative relDeadband (fraction of baseline) is used. All-nil -> (nil,nil,nil,flat).
func summarize(series []*float64, lowerIsBetter, isPace bool) (cur, base, delta *float64, dir TrendDirection) {
	_ = lowerIsBetter // retained for self-documentation; direction is raw value movement
	for _, v := range series {
		if v != nil {
			if base == nil {
				base = v
			}
			cur = v
		}
	}
	if cur == nil || base == nil {
		return nil, nil, nil, DirectionFlat
	}
	d := *cur - *base
	delta = &d

	if isPace {
		switch {
		case d > paceEps:
			return cur, base, delta, DirectionUp
		case d < -paceEps:
			return cur, base, delta, DirectionDown
		default:
			return cur, base, delta, DirectionFlat
		}
	}
	// Relative deadband for non-pace signals.
	var rel float64
	if *base != 0 {
		rel = d / math.Abs(*base)
	}
	switch {
	case rel > relDeadband:
		return cur, base, delta, DirectionUp
	case rel < -relDeadband:
		return cur, base, delta, DirectionDown
	default:
		return cur, base, delta, DirectionFlat
	}
}

// inBucket reports whether t falls in the half-open bucket (start, end].
func inBucket(t time.Time, b weekBucket) bool {
	return t.After(b.start) && !t.After(b.end)
}

// refHRBand returns the reference-HR band (Zone2 ceiling or documented default).
func refHRBand(profile store.AthleteProfile) (lo, hi float64) {
	ref := defaultRefHRBpm
	if profile.Zone2CeilingBpm != nil {
		ref = float64(*profile.Zone2CeilingBpm)
	}
	return ref - refHRBandBpm, ref + refHRBandBpm
}

// paceAtHRSeries builds the headline weekly-median pace (sec/km) of in-band runs
// per bucket. A bucket with no qualifying in-band run -> nil (gap). Lower=better.
func paceAtHRSeries(acts []store.Activity, profile store.AthleteProfile, buckets []weekBucket) []*float64 {
	lo, hi := refHRBand(profile)
	out := make([]*float64, len(buckets))
	for bi, b := range buckets {
		var paces []float64
		for _, a := range acts {
			if !metrics.IsRun(a.Type) || a.DistanceM <= 0 || a.MovingTimeS <= 0 || a.AvgHR == nil {
				continue
			}
			if *a.AvgHR < lo || *a.AvgHR > hi {
				continue
			}
			t, ok := metrics.ParseStart(a.StartTime)
			if !ok || !inBucket(t, b) {
				continue
			}
			paces = append(paces, float64(a.MovingTimeS)/(a.DistanceM/1000.0))
		}
		if len(paces) == 0 {
			continue // gap
		}
		sort.Float64s(paces)
		m := metrics.Median(paces)
		out[bi] = &m
	}
	return out
}

// weeklyLoadSeries builds per-bucket running km. A bucket with zero run km -> 0.0
// (not a gap: zero IS data).
func weeklyLoadSeries(acts []store.Activity, buckets []weekBucket) []*float64 {
	out := make([]*float64, len(buckets))
	for bi, b := range buckets {
		var km float64
		for _, a := range acts {
			if !metrics.IsRun(a.Type) {
				continue
			}
			t, ok := metrics.ParseStart(a.StartTime)
			if !ok || !inBucket(t, b) {
				continue
			}
			km += a.DistanceM / 1000.0
		}
		v := km
		out[bi] = &v
	}
	return out
}

// rhrSeries: per-bucket mean of in-bucket non-nil resting HR. Empty -> nil (gap).
func rhrSeries(rec []store.RecoveryDay, buckets []weekBucket) []*float64 {
	return recoveryMeanSeries(rec, buckets, func(d store.RecoveryDay) *int64 {
		if d.RHR == nil {
			return nil
		}
		return d.RHR.RestingHR
	})
}

// hrvSeries: per-bucket mean of in-bucket non-nil HRV last-night avg. Empty -> nil.
func hrvSeries(rec []store.RecoveryDay, buckets []weekBucket) []*float64 {
	return recoveryMeanSeries(rec, buckets, func(d store.RecoveryDay) *int64 {
		if d.HRV == nil {
			return nil
		}
		return d.HRV.LastNightAvgMs
	})
}

// recoveryMeanSeries averages pick(d) over in-bucket recovery days (date is a
// YYYY-MM-DD string, bucketed at midnight UTC). Empty bucket -> nil (gap).
func recoveryMeanSeries(rec []store.RecoveryDay, buckets []weekBucket, pick func(store.RecoveryDay) *int64) []*float64 {
	out := make([]*float64, len(buckets))
	for bi, b := range buckets {
		var sum float64
		var n int
		for _, d := range rec {
			t, ok := parseDate(d.Date)
			if !ok || !inBucket(t, b) {
				continue
			}
			if v := pick(d); v != nil {
				sum += float64(*v)
				n++
			}
		}
		if n == 0 {
			continue
		}
		m := sum / float64(n)
		out[bi] = &m
	}
	return out
}

// vo2maxSeries: latest (most-recent dated) non-nil VO2max reading within each
// bucket. pts may be most-recent-first; we track the max date seen per bucket.
func vo2maxSeries(pts []store.Vo2maxPoint, buckets []weekBucket) []*float64 {
	out := make([]*float64, len(buckets))
	bestDate := make([]string, len(buckets))
	for _, p := range pts {
		if p.Vo2max == nil {
			continue
		}
		t, ok := parseDate(p.Date)
		if !ok {
			continue
		}
		for bi, b := range buckets {
			if !inBucket(t, b) {
				continue
			}
			if p.Date > bestDate[bi] { // lexical compare works for YYYY-MM-DD
				bestDate[bi] = p.Date
				v := *p.Vo2max
				out[bi] = &v
			}
		}
	}
	return out
}

// parseDate parses a YYYY-MM-DD store date at 00:00:00 UTC.
func parseDate(date string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
