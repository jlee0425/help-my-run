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
