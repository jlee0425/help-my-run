package streams

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

// Engine is the DB-loading wrapper around the pure compute functions. It owns the
// analysis cache + recompute-on-zone-change logic and the on-demand/trickle
// fetch path. *Engine satisfies api.Streams and sync.streamFetcher.
type Engine struct {
	store    *store.Store
	strava   *strava.Client
	runner   garmin.Runner
	extraEnv []string
}

// New constructs a streams Engine. The strava client + garmin runner power
// FetchAndAnalyze; GetOrComputeAnalysis uses only the store.
func New(s *store.Store, sc *strava.Client, runner garmin.Runner, extraEnv []string) *Engine {
	return &Engine{store: s, strava: sc, runner: runner, extraEnv: extraEnv}
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
	if err != nil {
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

// FetchAndAnalyze fetches the per-run stream if missing (Strava primary; Garmin
// FIT fallback when Strava lacks HR), stores it gzipped, computes + caches the
// analysis, and returns it. If a raw stream already exists it just (re)computes
// via GetOrComputeAnalysis. Propagates *strava.ErrRateLimited unchanged.
func (e *Engine) FetchAndAnalyze(ctx context.Context, activityID int64) (StreamAnalysis, error) {
	if has, err := e.store.HasActivityStream(activityID); err != nil {
		return StreamAnalysis{}, err
	} else if has {
		return e.GetOrComputeAnalysis(ctx, activityID)
	}

	token, err := e.accessToken(ctx)
	if err != nil {
		return StreamAnalysis{}, err
	}
	ss, err := e.strava.GetActivityStreams(ctx, token, activityID)
	if err != nil {
		return StreamAnalysis{}, err // includes *strava.ErrRateLimited
	}
	ser := FromStravaStreams(ss)
	source := "strava"

	// Garmin FIT fallback only when Strava carried no HR and a Garmin id resolves.
	if !ser.HasHR() {
		if gid, ok := e.resolveGarminID(activityID); ok {
			if out, ferr := e.runner.RunGarminFetchFIT(ctx, gid, activityID, e.extraEnv); ferr == nil && len(out.Series.HR) > 0 {
				ser = Series{T: out.Series.T, HR: out.Series.HR, V: out.Series.V, Dist: out.Series.Dist}
				source = "garmin"
			}
			// On FIT failure: degrade to the Strava (no-HR) series already in ser.
		}
	}

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

// refreshBuffer mirrors sync.refreshBuffer: refresh if the token expires within
// this window (M0 token-refresh mechanism, replicated for the fetch path).
const refreshBuffer = 60 * time.Second

// accessToken returns a VALID Strava access token, refreshing via the same M0
// mechanism sync.SyncStrava uses (refresh-if-expired, persist the rotated
// refresh token). Without this, GetActivityStreams 401s on a stale token (FIX 4).
func (e *Engine) accessToken(ctx context.Context) (string, error) {
	tok, err := e.store.GetStravaTokens()
	if err != nil {
		return "", err
	}
	if tok.ExpiresAt <= time.Now().Add(refreshBuffer).Unix() {
		tr, rerr := e.strava.Refresh(ctx, tok.RefreshToken)
		if rerr != nil {
			return "", rerr
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
		if serr := e.store.SaveStravaTokens(tok); serr != nil {
			return "", serr
		}
	}
	return tok.AccessToken, nil
}

// resolveGarminID maps a Strava activity id to a Garmin download id when one is
// resolvable (v1: best-effort). Returns ok=false to skip the FIT fallback.
//
// *** M3.2 SHIPS THE GARMIN .FIT FALLBACK DORMANT (intentional). ***
// There is NO Strava<->Garmin activity-id mapping in the M3.2 data model, so this
// ALWAYS returns (0, false). The FIT fetch/parse infrastructure (Task 7 worker
// `stream` subcommand + RunGarminFetchFIT) and this call site are fully built and
// unit-tested, but the fallback NEVER fires at runtime in M3.2 — every no-HR
// Strava run degrades to the "no HR" state with the Strava (no-HR) raw stored.
// The fallback ACTIVATES only once a future slice adds a Strava<->Garmin
// activity-id mapping (e.g. ingest Garmin activity ids + match by start_time).
// Do NOT delete the FIT code: the infra is intentionally landed ahead of activation.
func (e *Engine) resolveGarminID(stravaActivityID int64) (int64, bool) {
	// v1: no mapping table exists (DORMANT — see the note above and the spec §3.1/§4).
	// Return false so the fallback is skipped and the Strava (no-HR) raw is stored.
	// A later id-mapping slice flips this on; the call site already degrades gracefully.
	return 0, false
}
