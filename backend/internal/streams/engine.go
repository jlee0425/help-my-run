package streams

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
)

// Engine is the DB-loading wrapper around the pure compute functions. It owns the
// analysis cache + recompute-on-zone-change logic and the on-demand/trickle
// fetch path. *Engine satisfies api.Streams and sync.streamFetcher.
type Engine struct {
	store    *store.Store
	runner   garmin.Runner
	extraEnv []string
}

// New constructs a streams Engine. The garmin runner powers FetchAndAnalyze;
// GetOrComputeAnalysis uses only the store.
func New(s *store.Store, runner garmin.Runner, extraEnv []string) *Engine {
	return &Engine{store: s, runner: runner, extraEnv: extraEnv}
}

// rowToAnalysis decodes a stored StreamAnalysisRow into a StreamAnalysis.
func rowToAnalysis(r store.StreamAnalysisRow, source string) (StreamAnalysis, error) {
	var tiz []ZoneTime
	if err := json.Unmarshal([]byte(r.TimeInZoneJSON), &tiz); err != nil {
		return StreamAnalysis{}, err
	}
	var zb ZoneBounds
	if err := json.Unmarshal([]byte(r.ZonesJSON), &zb); err != nil {
		return StreamAnalysis{}, err
	}
	return StreamAnalysis{
		ActivityID:    r.ActivityID,
		HasHR:         r.HasHR,
		TimeInZone:    tiz,
		DecouplingPct: r.DecouplingPct,
		PaHRFirst:     r.PaHRFirst,
		PaHRSecond:    r.PaHRSecond,
		Zones:         zb,
		Source:        source,
		ComputedAt:    r.ComputedAt,
	}, nil
}

// analysisToRow marshals a StreamAnalysis into a storable row. time_in_zone is
// stored as "[]" (never null) when there is no HR.
func analysisToRow(a StreamAnalysis) (store.StreamAnalysisRow, error) {
	tiz := a.TimeInZone
	if tiz == nil {
		tiz = []ZoneTime{}
	}
	tizJSON, err := json.Marshal(tiz)
	if err != nil {
		return store.StreamAnalysisRow{}, err
	}
	zonesJSON, err := json.Marshal(a.Zones)
	if err != nil {
		return store.StreamAnalysisRow{}, err
	}
	return store.StreamAnalysisRow{
		ActivityID:     a.ActivityID,
		TimeInZoneJSON: string(tizJSON),
		DecouplingPct:  a.DecouplingPct,
		PaHRFirst:      a.PaHRFirst,
		PaHRSecond:     a.PaHRSecond,
		ZonesJSON:      string(zonesJSON),
		HasHR:          a.HasHR,
		ComputedAt:     a.ComputedAt,
	}, nil
}

// computeAndStore decompresses the raw stream, runs Analyze with zb, stamps
// source + computed_at, persists the row, and returns the analysis.
func (e *Engine) computeAndStore(activityID int64, raw store.ActivityStream, zb ZoneBounds) (StreamAnalysis, error) {
	ser, err := DecompressSeries(raw.SeriesGz)
	if err != nil {
		return StreamAnalysis{}, err
	}
	a := Analyze(activityID, ser, zb)
	a.Source = raw.Source
	a.ComputedAt = time.Now().UTC().Format(time.RFC3339)
	row, err := analysisToRow(a)
	if err != nil {
		return StreamAnalysis{}, err
	}
	if err := e.store.UpsertStreamAnalysis(row); err != nil {
		return StreamAnalysis{}, err
	}
	return a, nil
}

// GetOrComputeAnalysis returns the cached StreamAnalysis, recomputing from the
// stored raw stream when the cached zones_json differs from the CURRENT profile
// zones. Returns store.ErrNotFound when no raw stream is stored for the activity.
// No activity_streams re-fetch ever occurs on a zone change.
func (e *Engine) GetOrComputeAnalysis(ctx context.Context, activityID int64) (StreamAnalysis, error) {
	raw, err := e.store.GetActivityStream(activityID)
	if err != nil {
		return StreamAnalysis{}, err // ErrNotFound when not fetched
	}
	prof, err := e.store.GetAthleteProfile()
	if errors.Is(err, store.ErrNotFound) {
		// A missing profile row must NOT be conflated with a missing stream:
		// fall back to a zero-value profile so ZonesFromProfile uses defaults.
		prof = store.AthleteProfile{}
	} else if err != nil {
		return StreamAnalysis{}, err
	}
	current := ZonesFromProfile(prof)
	curJSON, err := json.Marshal(current)
	if err != nil {
		return StreamAnalysis{}, err
	}

	cached, err := e.store.GetStreamAnalysis(activityID)
	if errors.Is(err, store.ErrNotFound) {
		return e.computeAndStore(activityID, raw, current)
	}
	if err != nil {
		return StreamAnalysis{}, err
	}
	if cached.ZonesJSON != string(curJSON) {
		return e.computeAndStore(activityID, raw, current)
	}
	return rowToAnalysis(cached, raw.Source)
}

// FetchAndAnalyze fetches the per-run Garmin .FIT stream if missing, stores it
// gzipped, computes + caches the analysis, and returns it. If a raw stream
// already exists it just (re)computes via GetOrComputeAnalysis.
func (e *Engine) FetchAndAnalyze(ctx context.Context, activityID int64) (StreamAnalysis, error) {
	if has, err := e.store.HasActivityStream(activityID); err != nil {
		return StreamAnalysis{}, err
	} else if has {
		return e.GetOrComputeAnalysis(ctx, activityID)
	}

	gid, _ := e.resolveGarminID(activityID) // identity: gid == activityID
	out, err := e.runner.RunGarminFetchFIT(ctx, gid, activityID, e.extraEnv)
	if err != nil {
		return StreamAnalysis{}, err
	}
	ser := Series{T: out.Series.T, HR: out.Series.HR, V: out.Series.V, Dist: out.Series.Dist}
	source := "garmin"

	gz, err := CompressSeries(ser)
	if err != nil {
		return StreamAnalysis{}, err
	}
	if err := e.store.UpsertActivityStream(store.ActivityStream{
		ActivityID: activityID, Source: source, SeriesGz: gz,
	}); err != nil {
		return StreamAnalysis{}, err
	}
	return e.GetOrComputeAnalysis(ctx, activityID)
}

// resolveGarminID is identity in M4: activities.activity_id IS the Garmin
// download id. Always (id, true). The dormant match path was removed with
// garmin_activities.
func (e *Engine) resolveGarminID(activityID int64) (int64, bool) {
	return activityID, true
}
