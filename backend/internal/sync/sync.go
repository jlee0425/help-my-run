// Package sync orchestrates Garmin ingestion and records sync_log.
package sync

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
)

// SourceResult is the per-source sync outcome (matches the /api/sync contract).
// "skipped" is set by callers (not by SyncGarmin/SyncAll) when the sync was not
// attempted — e.g. the Garmin token store does not yet exist, so contacting
// Garmin would only add to its per-IP login rate limit. A "skipped" result does
// NOT write sync_log (a prior ok/error row is preserved) and is not "ok", so the
// stream-trickle gate and the status() connected=(Status=="ok") derivation both
// correctly treat it as not-connected.
type SourceResult struct {
	Status string  // "ok" | "error" | "skipped"
	Synced int     // rows upserted
	Error  *string // non-nil when Status=="error" (or a note when "skipped")
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

func errResult(s *store.Store, source string, err error) SourceResult {
	msg := err.Error()
	now := nowUTC()
	_ = s.UpdateSyncLog(store.SyncLog{
		Source: source, LastSyncedAt: prevSynced(s, source), LastRunAt: &now,
		Status: "error", Error: &msg,
	})
	return SourceResult{Status: "error", Synced: 0, Error: &msg}
}

// prevSynced preserves the prior last_synced_at on an errored run.
func prevSynced(s *store.Store, source string) *string {
	if sl, err := s.GetSyncLog(source); err == nil {
		return sl.LastSyncedAt
	}
	return nil
}

func okResult(s *store.Store, source string, synced int) SourceResult {
	now := nowUTC()
	_ = s.UpdateSyncLog(store.SyncLog{
		Source: source, LastSyncedAt: &now, LastRunAt: &now,
		Status: "ok", Error: nil,
	})
	return SourceResult{Status: "ok", Synced: synced, Error: nil}
}

// garminBackfillDays is the default look-back when Garmin has never synced.
// M3.1: 84d (~12 weeks) to seed VO2max + recovery trend history (spec §4).
// NOTE: first-sync-only; subsequent syncs use sync_log.last_synced_at. This
// also deepens sleep/hrv/bb/rhr first-sync history (acceptable, arguably
// desirable — those trends feed the progress engine too).
const garminBackfillDays = 84

// SyncGarmin runs the Python worker since the last successful Garmin sync (or a
// ~84-day / ~12-week backfill), upserts the five garmin_* tables, and records
// sync_log.
func SyncGarmin(ctx context.Context, s *store.Store, r garmin.Runner, extraEnv []string) SourceResult {
	const source = "garmin"

	since := time.Now().AddDate(0, 0, -garminBackfillDays).Format("2006-01-02")
	if sl, err := s.GetSyncLog(source); err == nil && sl.LastSyncedAt != nil {
		if ts, perr := time.Parse(time.RFC3339, *sl.LastSyncedAt); perr == nil {
			since = ts.Format("2006-01-02")
		}
	}

	out, err := r.RunGarminFetch(ctx, since, extraEnv)
	if err != nil {
		return errResult(s, source, err)
	}

	synced := 0
	for _, d := range out.Sleep {
		if err := s.UpsertSleep(store.SleepRow{
			Date: d.Date, DurationS: d.DurationS, DeepS: d.DeepS, LightS: d.LightS,
			RemS: d.RemS, AwakeS: d.AwakeS, Score: d.Score, RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	for _, d := range out.HRV {
		if err := s.UpsertHrv(store.HrvRow{
			Date: d.Date, LastNightAvgMs: d.LastNightAvgMs, Status: d.Status,
			RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	for _, d := range out.BodyBattery {
		if err := s.UpsertBodyBattery(store.BodyBatteryRow{
			Date: d.Date, Charged: d.Charged, Drained: d.Drained, High: d.High, Low: d.Low,
			RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	for _, d := range out.RHR {
		if err := s.UpsertRhr(store.RhrRow{
			Date: d.Date, RestingHR: d.RestingHR, RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	for _, d := range out.VO2Max {
		if err := s.UpsertVo2max(store.Vo2maxRow{
			Date: d.Date, Vo2max: d.VO2Max, RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	for _, a := range out.Activities {
		atype := ""
		if a.ActivityType != nil {
			atype = canonicalActivityType(*a.ActivityType)
		}
		dist := 0.0
		if a.DistanceM != nil {
			dist = *a.DistanceM
		}
		if err := s.UpsertActivity(store.Activity{
			ActivityID:     a.GarminActivityID,
			Name:           a.Name,
			Type:           atype,
			SportType:      nil, // Garmin list has no sportType
			StartTime:      a.StartTime,
			StartTimeLocal: a.StartTimeLocal,
			DistanceM:      dist,
			MovingTimeS:    f64ptrToI64(a.MovingTimeS),
			ElapsedTimeS:   f64ptrToI64(a.ElapsedTimeS),
			AvgHR:          a.AvgHR,
			MaxHR:          a.MaxHR,
			AvgSpeed:       a.AvgSpeed,
			MaxSpeed:       a.MaxSpeed,
			AvgCadence:     a.AvgCadence,
			ElevationGainM: a.ElevationGainM,
			RawJSON:        rawString(a.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	return okResult(s, source, synced)
}

// canonicalActivityType maps a Garmin activityType.typeKey (lowercase snake_case
// such as "running", "trail_running", "treadmill_running", "virtual_run") to the
// canonical run vocabulary the rest of the system expects ({Run, TrailRun,
// VirtualRun}, matching metrics.runTypes). Garmin's whole run family contains the
// substring "run" while non-run types (cycling, lap_swimming, strength_training,
// hiking, walking, yoga, …) do not, so non-run keys pass through unchanged — only
// runs matter downstream. Normalizing here, at the Go ingest boundary, keeps
// metrics.IsRun and all existing test fixtures (which seed "Run") working without
// touching any downstream consumer.
func canonicalActivityType(garminTypeKey string) string {
	k := strings.ToLower(garminTypeKey)
	switch {
	case strings.Contains(k, "trail") && strings.Contains(k, "run"):
		return "TrailRun"
	case strings.Contains(k, "virtual") && strings.Contains(k, "run"):
		return "VirtualRun"
	case strings.Contains(k, "run"):
		return "Run"
	default:
		return garminTypeKey
	}
}

// f64ptrToI64 dereferences a nullable worker duration to a rounded int64,
// defaulting nil to 0 (the activities INTEGER columns are NOT NULL).
func f64ptrToI64(p *float64) int64 {
	if p == nil {
		return 0
	}
	return int64(math.Round(*p))
}

// rawString renders a json.RawMessage to a string for the raw_json column,
// defaulting to "null" when empty.
func rawString(m json.RawMessage) string {
	if len(m) == 0 {
		return "null"
	}
	return string(m)
}

// AllResult is the combined sync outcome (the /api/sync body). Garmin-only in M4.
type AllResult struct {
	Garmin SourceResult
}

// SyncAll runs the Garmin sync and returns the result. When st is non-nil and the
// Garmin sync succeeds, it trickles a budgeted recent-window stream backfill
// (never erroring the surrounding sync).
func SyncAll(ctx context.Context, s *store.Store, r garmin.Runner, extraEnv []string, st *StreamTrickle) AllResult {
	res := AllResult{Garmin: SyncGarmin(ctx, s, r, extraEnv)}
	if res.Garmin.Status == "ok" && st != nil && st.Fetcher != nil {
		TrickleStreams(ctx, s, st.Fetcher, st.Weeks, st.Budget, time.Now())
	}
	return res
}
