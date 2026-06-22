package store

import (
	"database/sql"
	"errors"
	"time"
)

// ActivityStream is one stored raw per-sample stream (one activity_streams row).
// SeriesGz is the gzipped JSON of the normalized streams.Series (a BLOB).
type ActivityStream struct {
	ActivityID int64
	Source     string // "strava" | "garmin"
	SeriesGz   []byte
	FetchedAt  string // RFC3339 UTC, set server-side
}

// UpsertActivityStream inserts or updates the raw stream for an activity by
// activity_id (= Strava id). FetchedAt is set server-side to now (UTC RFC3339)
// when empty.
func (s *Store) UpsertActivityStream(as ActivityStream) error {
	if as.FetchedAt == "" {
		as.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.DB.Exec(`
		INSERT INTO activity_streams (activity_id, source, series_gz, fetched_at)
		VALUES (?,?,?,?)
		ON CONFLICT(activity_id) DO UPDATE SET
			source=excluded.source, series_gz=excluded.series_gz, fetched_at=excluded.fetched_at`,
		as.ActivityID, as.Source, as.SeriesGz, as.FetchedAt)
	return err
}

// GetActivityStream returns the stored raw stream for activityID, or ErrNotFound.
func (s *Store) GetActivityStream(activityID int64) (ActivityStream, error) {
	var as ActivityStream
	err := s.DB.QueryRow(`
		SELECT activity_id, source, series_gz, fetched_at
		FROM activity_streams WHERE activity_id = ?`, activityID).
		Scan(&as.ActivityID, &as.Source, &as.SeriesGz, &as.FetchedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ActivityStream{}, ErrNotFound
	}
	if err != nil {
		return ActivityStream{}, err
	}
	return as, nil
}

// HasActivityStream reports whether a raw stream is stored for activityID.
func (s *Store) HasActivityStream(activityID int64) (bool, error) {
	var one int
	err := s.DB.QueryRow(`SELECT 1 FROM activity_streams WHERE activity_id = ?`, activityID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// StreamAnalysisRow is one cached per-run analysis (one stream_analyses row).
// TimeInZoneJSON and ZonesJSON are opaque marshaled strings to the store.
type StreamAnalysisRow struct {
	ActivityID     int64
	TimeInZoneJSON string
	DecouplingPct  *float64
	PaHRFirst      *float64
	PaHRSecond     *float64
	ZonesJSON      string
	HasHR          bool
	ComputedAt     string // RFC3339 UTC
}

// UpsertStreamAnalysis inserts or updates the cached analysis for an activity.
func (s *Store) UpsertStreamAnalysis(r StreamAnalysisRow) error {
	hasHR := int64(0)
	if r.HasHR {
		hasHR = 1
	}
	_, err := s.DB.Exec(`
		INSERT INTO stream_analyses (
			activity_id, time_in_zone_json, decoupling_pct, pa_hr_first, pa_hr_second,
			zones_json, has_hr, computed_at
		) VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(activity_id) DO UPDATE SET
			time_in_zone_json=excluded.time_in_zone_json,
			decoupling_pct=excluded.decoupling_pct,
			pa_hr_first=excluded.pa_hr_first, pa_hr_second=excluded.pa_hr_second,
			zones_json=excluded.zones_json, has_hr=excluded.has_hr,
			computed_at=excluded.computed_at`,
		r.ActivityID, r.TimeInZoneJSON, r.DecouplingPct, r.PaHRFirst, r.PaHRSecond,
		r.ZonesJSON, hasHR, r.ComputedAt)
	return err
}

// scanStreamAnalysis scans one row (shared by Get and List).
func scanStreamAnalysis(sc interface{ Scan(...any) error }) (StreamAnalysisRow, error) {
	var r StreamAnalysisRow
	var dp, p1, p2 sql.NullFloat64
	var hasHR int64
	if err := sc.Scan(
		&r.ActivityID, &r.TimeInZoneJSON, &dp, &p1, &p2,
		&r.ZonesJSON, &hasHR, &r.ComputedAt,
	); err != nil {
		return StreamAnalysisRow{}, err
	}
	if dp.Valid {
		r.DecouplingPct = &dp.Float64
	}
	if p1.Valid {
		r.PaHRFirst = &p1.Float64
	}
	if p2.Valid {
		r.PaHRSecond = &p2.Float64
	}
	r.HasHR = hasHR != 0
	return r, nil
}

// GetStreamAnalysis returns the cached analysis for activityID, or ErrNotFound.
func (s *Store) GetStreamAnalysis(activityID int64) (StreamAnalysisRow, error) {
	row := s.DB.QueryRow(`
		SELECT activity_id, time_in_zone_json, decoupling_pct, pa_hr_first, pa_hr_second,
		       zones_json, has_hr, computed_at
		FROM stream_analyses WHERE activity_id = ?`, activityID)
	r, err := scanStreamAnalysis(row)
	if errors.Is(err, sql.ErrNoRows) {
		return StreamAnalysisRow{}, ErrNotFound
	}
	if err != nil {
		return StreamAnalysisRow{}, err
	}
	return r, nil
}

// ListStreamAnalyses returns up to limit cached analyses, most-recent-first by
// the joined activity start_time (the order the progress decoupling series wants).
func (s *Store) ListStreamAnalyses(limit int) ([]StreamAnalysisRow, error) {
	rows, err := s.DB.Query(`
		SELECT sa.activity_id, sa.time_in_zone_json, sa.decoupling_pct, sa.pa_hr_first,
		       sa.pa_hr_second, sa.zones_json, sa.has_hr, sa.computed_at
		FROM stream_analyses sa
		JOIN activities a ON a.strava_id = sa.activity_id
		ORDER BY a.start_time DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StreamAnalysisRow
	for rows.Next() {
		r, err := scanStreamAnalysis(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// StreamFetchLog is the single-row (source='strava') trickle state for the
// recent-window stream backfill. Mirrors SyncLog conventions; nullable TEXT
// fields are *string.
type StreamFetchLog struct {
	Source           string
	CursorTime       *string // oldest start_time reached in the recent window
	LastRunAt        *string // RFC3339 UTC of last trickle attempt
	LastFetched      int64   // count fetched in last run
	TotalFetched     int64   // cumulative streams fetched
	Status           string  // "ok" | "error" | "rate_limited" | "never"
	Error            *string // non-nil only on error/rate_limited
	RateLimitedUntil *string // RFC3339 UTC; trickle resumes after this
}

// GetStreamFetchLog returns the single stream_fetch_log row (source='strava'),
// or ErrNotFound if the seed is somehow absent.
func (s *Store) GetStreamFetchLog() (StreamFetchLog, error) {
	var fl StreamFetchLog
	var cursor, lastRun, errMsg, rlUntil sql.NullString
	err := s.DB.QueryRow(`
		SELECT source, cursor_time, last_run_at, last_fetched, total_fetched,
		       status, error, rate_limited_until
		FROM stream_fetch_log WHERE source = 'strava'`).
		Scan(&fl.Source, &cursor, &lastRun, &fl.LastFetched, &fl.TotalFetched,
			&fl.Status, &errMsg, &rlUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return StreamFetchLog{}, ErrNotFound
	}
	if err != nil {
		return StreamFetchLog{}, err
	}
	if cursor.Valid {
		fl.CursorTime = &cursor.String
	}
	if lastRun.Valid {
		fl.LastRunAt = &lastRun.String
	}
	if errMsg.Valid {
		fl.Error = &errMsg.String
	}
	if rlUntil.Valid {
		fl.RateLimitedUntil = &rlUntil.String
	}
	return fl, nil
}

// UpdateStreamFetchLog upserts the single stream_fetch_log row.
func (s *Store) UpdateStreamFetchLog(fl StreamFetchLog) error {
	if fl.Source == "" {
		fl.Source = "strava"
	}
	_, err := s.DB.Exec(`
		INSERT INTO stream_fetch_log (
			source, cursor_time, last_run_at, last_fetched, total_fetched,
			status, error, rate_limited_until
		) VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(source) DO UPDATE SET
			cursor_time        = excluded.cursor_time,
			last_run_at        = excluded.last_run_at,
			last_fetched       = excluded.last_fetched,
			total_fetched      = excluded.total_fetched,
			status             = excluded.status,
			error              = excluded.error,
			rate_limited_until = excluded.rate_limited_until`,
		fl.Source, fl.CursorTime, fl.LastRunAt, fl.LastFetched, fl.TotalFetched,
		fl.Status, fl.Error, fl.RateLimitedUntil)
	return err
}

// ListRecentRunsWithoutStream returns up to limit run ids whose start_time is
// >= sinceISO and that have NO activity_streams row, most-recent-first. Used by
// the recent-window trickle.
func (s *Store) ListRecentRunsWithoutStream(sinceISO string, limit int) ([]int64, error) {
	rows, err := s.DB.Query(`
		SELECT a.strava_id
		FROM activities a
		LEFT JOIN activity_streams st ON st.activity_id = a.strava_id
		WHERE st.activity_id IS NULL
		  AND a.type = 'Run'
		  AND a.start_time >= ?
		ORDER BY a.start_time DESC
		LIMIT ?`, sinceISO, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
