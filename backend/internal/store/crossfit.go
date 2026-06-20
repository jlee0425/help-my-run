package store

import (
	"database/sql"
	"errors"
	"time"
)

// CrossFitWeek maps to crossfit_weeks (one row per Monday week_start).
type CrossFitWeek struct {
	WeekStart   string
	ImagePath   *string
	ParsedJSON  string // Stage-1 parsed object, verbatim
	RawResponse *string
	CreatedAt   string
	UpdatedAt   string
}

// GetCrossFitWeek returns the row for weekStart, or ErrNotFound.
func (s *Store) GetCrossFitWeek(weekStart string) (CrossFitWeek, error) {
	var w CrossFitWeek
	var img, raw sql.NullString
	err := s.DB.QueryRow(`
		SELECT week_start, image_path, parsed_json, raw_response, created_at, updated_at
		FROM crossfit_weeks WHERE week_start = ?`, weekStart).
		Scan(&w.WeekStart, &img, &w.ParsedJSON, &raw, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CrossFitWeek{}, ErrNotFound
	}
	if err != nil {
		return CrossFitWeek{}, err
	}
	if img.Valid {
		w.ImagePath = &img.String
	}
	if raw.Valid {
		w.RawResponse = &raw.String
	}
	return w, nil
}

// UpsertCrossFitWeek upserts a parsed CrossFit week by week_start. created_at is
// set on insert and preserved on update; updated_at is always set to now.
func (s *Store) UpsertCrossFitWeek(w CrossFitWeek) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO crossfit_weeks
			(week_start, image_path, parsed_json, raw_response, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(week_start) DO UPDATE SET
			image_path   = excluded.image_path,
			parsed_json  = excluded.parsed_json,
			raw_response = excluded.raw_response,
			updated_at   = excluded.updated_at`,
		w.WeekStart, w.ImagePath, w.ParsedJSON, w.RawResponse, now, now)
	return err
}
