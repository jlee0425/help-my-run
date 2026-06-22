package store

import (
	"database/sql"
	"errors"
	"time"
)

// Activity is a normalized Strava run (one row in activities).
type Activity struct {
	StravaID       int64
	Name           string
	Type           string
	SportType      *string
	StartTime      string
	StartTimeLocal *string
	DistanceM      float64
	MovingTimeS    int64
	ElapsedTimeS   int64
	AvgHR          *float64
	MaxHR          *float64
	AvgSpeed       *float64
	MaxSpeed       *float64
	AvgCadence     *float64
	ElevationGainM *float64
	RawJSON        string
}

// Split is one Strava lap mapped into activity_splits.
type Split struct {
	ActivityID   int64
	Idx          int64
	DistanceM    float64
	ElapsedTimeS int64
	MovingTimeS  *int64
	AvgHR        *float64
	MaxHR        *float64
	AvgSpeed     *float64
}

// UpsertActivity inserts or updates an activity by strava_id.
func (s *Store) UpsertActivity(a Activity) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO activities (
			strava_id, name, type, sport_type, start_time, start_time_local,
			distance_m, moving_time_s, elapsed_time_s,
			avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m,
			raw_json, synced_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(strava_id) DO UPDATE SET
			name=excluded.name, type=excluded.type, sport_type=excluded.sport_type,
			start_time=excluded.start_time, start_time_local=excluded.start_time_local,
			distance_m=excluded.distance_m, moving_time_s=excluded.moving_time_s,
			elapsed_time_s=excluded.elapsed_time_s,
			avg_hr=excluded.avg_hr, max_hr=excluded.max_hr, avg_speed=excluded.avg_speed,
			max_speed=excluded.max_speed, avg_cadence=excluded.avg_cadence,
			elevation_gain_m=excluded.elevation_gain_m,
			raw_json=excluded.raw_json, synced_at=excluded.synced_at`,
		a.StravaID, a.Name, a.Type, a.SportType, a.StartTime, a.StartTimeLocal,
		a.DistanceM, a.MovingTimeS, a.ElapsedTimeS,
		a.AvgHR, a.MaxHR, a.AvgSpeed, a.MaxSpeed, a.AvgCadence, a.ElevationGainM,
		a.RawJSON, now)
	return err
}

// GetActivity returns one activity by strava_id, or ErrNotFound. raw_json is not loaded.
func (s *Store) GetActivity(stravaID int64) (Activity, error) {
	var a Activity
	err := s.DB.QueryRow(`
		SELECT strava_id, name, type, sport_type, start_time, start_time_local,
		       distance_m, moving_time_s, elapsed_time_s,
		       avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m
		FROM activities
		WHERE strava_id = ?`, stravaID).Scan(
		&a.StravaID, &a.Name, &a.Type, &a.SportType, &a.StartTime, &a.StartTimeLocal,
		&a.DistanceM, &a.MovingTimeS, &a.ElapsedTimeS,
		&a.AvgHR, &a.MaxHR, &a.AvgSpeed, &a.MaxSpeed, &a.AvgCadence, &a.ElevationGainM)
	if errors.Is(err, sql.ErrNoRows) {
		return Activity{}, ErrNotFound
	}
	if err != nil {
		return Activity{}, err
	}
	return a, nil
}

// ListActivities returns up to limit activities, most-recent-first by start_time.
// raw_json is intentionally not loaded (not needed by the list response).
func (s *Store) ListActivities(limit int) ([]Activity, error) {
	rows, err := s.DB.Query(`
		SELECT strava_id, name, type, sport_type, start_time, start_time_local,
		       distance_m, moving_time_s, elapsed_time_s,
		       avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m
		FROM activities
		ORDER BY start_time DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(
			&a.StravaID, &a.Name, &a.Type, &a.SportType, &a.StartTime, &a.StartTimeLocal,
			&a.DistanceM, &a.MovingTimeS, &a.ElapsedTimeS,
			&a.AvgHR, &a.MaxHR, &a.AvgSpeed, &a.MaxSpeed, &a.AvgCadence, &a.ElevationGainM,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// LatestActivityStartTime returns the max start_time across all activities (ISO
// string), or ErrNotFound when there are no activities. Used as the Strava
// incremental sync cursor.
func (s *Store) LatestActivityStartTime() (string, error) {
	var t sql.NullString
	err := s.DB.QueryRow(`SELECT MAX(start_time) FROM activities`).Scan(&t)
	if err != nil {
		return "", err
	}
	if !t.Valid {
		return "", ErrNotFound
	}
	return t.String, nil
}

// UpsertSplits upserts all splits for an activity (by activity_id+idx PK).
func (s *Store) UpsertSplits(activityID int64, splits []Split) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO activity_splits (
			activity_id, idx, distance_m, elapsed_time_s, moving_time_s,
			avg_hr, max_hr, avg_speed
		) VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(activity_id, idx) DO UPDATE SET
			distance_m=excluded.distance_m, elapsed_time_s=excluded.elapsed_time_s,
			moving_time_s=excluded.moving_time_s, avg_hr=excluded.avg_hr,
			max_hr=excluded.max_hr, avg_speed=excluded.avg_speed`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sp := range splits {
		if _, err := stmt.Exec(
			sp.ActivityID, sp.Idx, sp.DistanceM, sp.ElapsedTimeS, sp.MovingTimeS,
			sp.AvgHR, sp.MaxHR, sp.AvgSpeed,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
