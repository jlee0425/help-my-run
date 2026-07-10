package store

import (
	"database/sql"
	"errors"
	"time"
)

// Settings keys used by the auth/claude/webpush subsystems (app_settings table).
const (
	SettingPasswordHash = "password_hash"
	SettingAPITokenHash = "api_token_hash"
	SettingClaudeToken  = "claude_oauth_token"
	SettingVAPIDPublic  = "vapid_public"
	SettingVAPIDPrivate = "vapid_private"
)

// Session is one sessions row (owner login). IDHash is the SHA-256 hex of the
// cookie value; the plaintext session id is never stored. UserAgent/CreatedIP
// exist so the owner can recognize devices in Settings (M6).
type Session struct {
	IDHash     string
	CreatedAt  string
	LastSeenAt string
	ExpiresAt  string
	UserAgent  string
	CreatedIP  string
}

// GetSetting returns the app_settings value for key, or ErrNotFound.
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.DB.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

// SetSetting upserts an app_settings key. updated_at is stamped server-side.
func (s *Store) SetSetting(key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, now)
	return err
}

// DeleteSetting removes an app_settings key. Missing key is a no-op.
func (s *Store) DeleteSetting(key string) error {
	_, err := s.DB.Exec(`DELETE FROM app_settings WHERE key = ?`, key)
	return err
}

// InsertSession stores a new session (last_seen_at = created_at).
func (s *Store) InsertSession(idHash, createdAt, expiresAt, userAgent, createdIP string) error {
	_, err := s.DB.Exec(`
		INSERT INTO sessions (id_hash, created_at, last_seen_at, expires_at, user_agent, created_ip)
		VALUES (?, ?, ?, ?, ?, ?)`,
		idHash, createdAt, createdAt, expiresAt, userAgent, createdIP)
	return err
}

// GetSession returns the session row for idHash, or ErrNotFound.
func (s *Store) GetSession(idHash string) (Session, error) {
	var sess Session
	err := s.DB.QueryRow(`
		SELECT id_hash, created_at, last_seen_at, expires_at, user_agent, created_ip
		FROM sessions WHERE id_hash = ?`, idHash).
		Scan(&sess.IDHash, &sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt,
			&sess.UserAgent, &sess.CreatedIP)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	if err != nil {
		return Session{}, err
	}
	return sess, nil
}

// ListSessions returns all sessions, most-recently-seen first (Settings devices).
func (s *Store) ListSessions() ([]Session, error) {
	rows, err := s.DB.Query(`
		SELECT id_hash, created_at, last_seen_at, expires_at, user_agent, created_ip
		FROM sessions ORDER BY last_seen_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Session, 0)
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.IDHash, &sess.CreatedAt, &sess.LastSeenAt, &sess.ExpiresAt,
			&sess.UserAgent, &sess.CreatedIP); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// TouchSession updates the sliding-expiry bookkeeping for a session.
func (s *Store) TouchSession(idHash, lastSeen, expiresAt string) error {
	_, err := s.DB.Exec(`
		UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE id_hash = ?`,
		lastSeen, expiresAt, idHash)
	return err
}

// DeleteSession removes one session (logout / expiry). Missing row is a no-op.
func (s *Store) DeleteSession(idHash string) error {
	_, err := s.DB.Exec(`DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

// DeleteAllSessions removes every session (password reset / regenerate).
func (s *Store) DeleteAllSessions() error {
	_, err := s.DB.Exec(`DELETE FROM sessions`)
	return err
}
