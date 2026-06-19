// Package sync orchestrates Strava + Garmin ingestion and records sync_log.
package sync

import (
	"context"
	"encoding/json"
	"time"

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

	// Incremental window: since last successful sync, else ~30-day backfill.
	after := time.Now().AddDate(0, 0, -30).Unix()
	if sl, err := s.GetSyncLog(source); err == nil && sl.LastSyncedAt != nil {
		if ts, perr := time.Parse(time.RFC3339, *sl.LastSyncedAt); perr == nil {
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
		StravaID:       a.ID,
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
