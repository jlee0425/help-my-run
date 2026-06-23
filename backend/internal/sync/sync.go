// Package sync orchestrates Strava + Garmin ingestion and records sync_log.
package sync

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

// refreshBuffer refreshes the Strava token if it expires within this window.
const refreshBuffer = 60 * time.Second

// SourceResult is the per-source sync outcome (matches the /api/sync contract).
type SourceResult struct {
	Status string  // "ok" | "error"
	Synced int     // rows upserted
	Error  *string // non-nil when Status=="error"
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

// SyncStrava refreshes the access token if needed, pulls activities since the
// last successful sync (or a ~30-day backfill), upserts activities + laps, and
// records sync_log. Returns the per-source result.
func SyncStrava(ctx context.Context, s *store.Store, client *strava.Client) SourceResult {
	const source = "strava"

	tok, err := s.GetStravaTokens()
	if err != nil {
		return errResult(s, source, err)
	}

	// Refresh if expired (or within the buffer).
	if tok.ExpiresAt <= time.Now().Add(refreshBuffer).Unix() {
		tr, err := client.Refresh(ctx, tok.RefreshToken)
		if err != nil {
			return errResult(s, source, err)
		}
		tok.AccessToken = tr.AccessToken
		tok.RefreshToken = tr.RefreshToken
		tok.ExpiresAt = tr.ExpiresAt
		if tr.Scope != "" {
			tok.Scope = tr.Scope
		}
		if tr.Athlete != nil {
			tok.AthleteID = tr.Athlete.ID
		}
		if err := s.SaveStravaTokens(tok); err != nil {
			return errResult(s, source, err)
		}
	}

	// Incremental window: since the latest stored activity start_time, else a
	// ~30-day backfill on a fresh DB.
	after := time.Now().AddDate(0, 0, -30).Unix()
	if latest, err := s.LatestActivityStartTime(); err == nil {
		if ts, perr := time.Parse(time.RFC3339, latest); perr == nil {
			after = ts.Unix()
		}
	}

	acts, err := client.ListActivities(ctx, tok.AccessToken, after)
	if err != nil {
		return errResult(s, source, err)
	}

	synced := 0
	for _, a := range acts {
		raw, _ := json.Marshal(a)
		if err := s.UpsertActivity(mapActivity(a, string(raw))); err != nil {
			return errResult(s, source, err)
		}
		laps, err := client.ListLaps(ctx, tok.AccessToken, a.ID)
		if err != nil {
			return errResult(s, source, err)
		}
		if err := s.UpsertSplits(a.ID, mapLaps(a.ID, laps)); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	return okResult(s, source, synced)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mapActivity(a strava.SummaryActivity, raw string) store.Activity {
	return store.Activity{
		ActivityID:     a.ID,
		Name:           a.Name,
		Type:           a.Type,
		SportType:      strPtr(a.SportType),
		StartTime:      a.StartDate,
		StartTimeLocal: strPtr(a.StartDateLocal),
		DistanceM:      a.Distance,
		MovingTimeS:    a.MovingTime,
		ElapsedTimeS:   a.ElapsedTime,
		AvgHR:          a.AverageHeartrate,
		MaxHR:          a.MaxHeartrate,
		AvgSpeed:       a.AverageSpeed,
		MaxSpeed:       a.MaxSpeed,
		AvgCadence:     a.AverageCadence,
		ElevationGainM: a.TotalElevationGain,
		RawJSON:        raw,
	}
}

func mapLaps(activityID int64, laps []strava.Lap) []store.Split {
	out := make([]store.Split, 0, len(laps))
	for _, l := range laps {
		out = append(out, store.Split{
			ActivityID:   activityID,
			Idx:          l.LapIndex,
			DistanceM:    l.Distance,
			ElapsedTimeS: l.ElapsedTime,
			MovingTimeS:  l.MovingTime,
			AvgHR:        l.AverageHeartrate,
			MaxHR:        l.MaxHeartrate,
			AvgSpeed:     l.AverageSpeed,
		})
	}
	return out
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
			atype = *a.ActivityType
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

// AllResult is the combined sync outcome for both sources (the /api/sync body).
type AllResult struct {
	Strava SourceResult
	Garmin SourceResult
}

// SyncAll runs both syncs sequentially (single SQLite writer) and returns both
// results. A failure in one source never aborts the other. When st is non-nil
// and the Strava sync succeeds, it trickles a budgeted recent-window stream
// backfill between the two syncs (never erroring the surrounding sync).
func SyncAll(ctx context.Context, s *store.Store, client *strava.Client, r garmin.Runner, extraEnv []string, st *StreamTrickle) AllResult {
	res := AllResult{Strava: SyncStrava(ctx, s, client)}
	if res.Strava.Status == "ok" && st != nil && st.Fetcher != nil {
		TrickleStreams(ctx, s, st.Fetcher, st.Weeks, st.Budget, time.Now())
	}
	res.Garmin = SyncGarmin(ctx, s, r, extraEnv)
	return res
}
