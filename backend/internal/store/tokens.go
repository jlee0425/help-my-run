package store

import (
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned by getters when no matching row exists.
var ErrNotFound = errors.New("store: not found")

// StravaTokens is the persisted OAuth token set (single row, id=1).
type StravaTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64 // unix epoch seconds (Strava expires_at)
	Scope        string
	AthleteID    int64
}

// GetStravaTokens returns the single stored token row, or ErrNotFound.
func (s *Store) GetStravaTokens() (StravaTokens, error) {
	var t StravaTokens
	var scope sql.NullString
	var athleteID sql.NullInt64
	err := s.DB.QueryRow(`
		SELECT access_token, refresh_token, expires_at, scope, athlete_id
		FROM strava_tokens WHERE id = 1`).
		Scan(&t.AccessToken, &t.RefreshToken, &t.ExpiresAt, &scope, &athleteID)
	if errors.Is(err, sql.ErrNoRows) {
		return StravaTokens{}, ErrNotFound
	}
	if err != nil {
		return StravaTokens{}, err
	}
	t.Scope = scope.String
	t.AthleteID = athleteID.Int64
	return t, nil
}

// SaveStravaTokens upserts the single token row (id always 1).
func (s *Store) SaveStravaTokens(t StravaTokens) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO strava_tokens
			(id, access_token, refresh_token, expires_at, scope, athlete_id, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			access_token  = excluded.access_token,
			refresh_token = excluded.refresh_token,
			expires_at    = excluded.expires_at,
			scope         = excluded.scope,
			athlete_id    = excluded.athlete_id,
			updated_at    = excluded.updated_at`,
		t.AccessToken, t.RefreshToken, t.ExpiresAt, t.Scope, t.AthleteID, now)
	return err
}
