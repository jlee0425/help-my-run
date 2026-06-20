package store

import "time"

// SaveOAuthState persists a generated CSRF state for later one-time validation
// in the Strava callback.
func (s *Store) SaveOAuthState(state string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO oauth_states (state, created_at) VALUES (?, ?)
		ON CONFLICT(state) DO UPDATE SET created_at = excluded.created_at`,
		state, now)
	return err
}

// ConsumeOAuthState deletes the stored state, returning ErrNotFound if it was
// never saved (or already consumed). Single-use: a second consume fails.
func (s *Store) ConsumeOAuthState(state string) error {
	res, err := s.DB.Exec(`DELETE FROM oauth_states WHERE state = ?`, state)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
