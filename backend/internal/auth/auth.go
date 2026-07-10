// Package auth owns the owner login: a single argon2id password, sliding
// server-side sessions (cookie), and a bearer API token for scripts. All
// secrets are stored hashed in app_settings/sessions (Task/spec M5 §4).
package auth

import (
	"errors"
	"sync"
	"time"

	"help-my-run/backend/internal/store"
)

// Typed failures the handlers map to HTTP statuses.
var (
	ErrAlreadySetup   = errors.New("auth: already set up")
	ErrNotSetup       = errors.New("auth: not set up")
	ErrWeakPassword   = errors.New("auth: password must be at least 8 characters")
	ErrBadCredentials = errors.New("auth: bad credentials")
	ErrThrottled      = errors.New("auth: too many attempts")
	ErrUnknownSession = errors.New("auth: unknown session")
)

const (
	minPasswordLen = 8
	sessionTTL     = 30 * 24 * time.Hour
	maxBackoff     = 60 * time.Second
)

// SessionMeta is where a session came from — recorded so the owner can
// recognize devices in Settings (M6). Zero value is fine for scripts/tests.
type SessionMeta struct {
	UserAgent string
	IP        string
}

// Service implements owner auth against the store. Now is injectable for tests
// (nil -> time.Now).
type Service struct {
	Store *store.Store
	Now   func() time.Time

	mu          sync.Mutex
	failures    int
	nextAllowed time.Time
}

// New constructs a Service.
func New(s *store.Store) *Service {
	return &Service{Store: s}
}

func (a *Service) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

// SetupRequired reports whether no owner password has been set yet.
func (a *Service) SetupRequired() (bool, error) {
	_, err := a.Store.GetSetting(store.SettingPasswordHash)
	if errors.Is(err, store.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

// Setup sets the owner password (first run only), returning a fresh session id
// and the plaintext API token (displayed once).
func (a *Service) Setup(password string, meta SessionMeta) (sessionID, apiToken string, err error) {
	req, err := a.SetupRequired()
	if err != nil {
		return "", "", err
	}
	if !req {
		return "", "", ErrAlreadySetup
	}
	if len(password) < minPasswordLen {
		return "", "", ErrWeakPassword
	}
	hash, err := HashPassword(password)
	if err != nil {
		return "", "", err
	}
	if err := a.Store.SetSetting(store.SettingPasswordHash, hash); err != nil {
		return "", "", err
	}
	apiToken, tokenHash := NewSecret("hmr_")
	if err := a.Store.SetSetting(store.SettingAPITokenHash, tokenHash); err != nil {
		return "", "", err
	}
	sessionID, err = a.newSession(meta)
	if err != nil {
		return "", "", err
	}
	return sessionID, apiToken, nil
}

// Login verifies the password and mints a session. Failures feed an in-memory
// exponential backoff (2^n seconds, capped at 60s).
func (a *Service) Login(password string, meta SessionMeta) (string, error) {
	a.mu.Lock()
	if a.now().Before(a.nextAllowed) {
		a.mu.Unlock()
		return "", ErrThrottled
	}
	a.mu.Unlock()

	hash, err := a.Store.GetSetting(store.SettingPasswordHash)
	if errors.Is(err, store.ErrNotFound) {
		return "", ErrNotSetup
	}
	if err != nil {
		return "", err
	}
	if !VerifyPassword(password, hash) {
		a.mu.Lock()
		a.failures++
		delay := time.Duration(1<<uint(a.failures)) * time.Second
		if delay > maxBackoff {
			delay = maxBackoff
		}
		a.nextAllowed = a.now().Add(delay)
		a.mu.Unlock()
		return "", ErrBadCredentials
	}
	a.mu.Lock()
	a.failures = 0
	a.nextAllowed = time.Time{}
	a.mu.Unlock()
	return a.newSession(meta)
}

// newSession mints and persists a session, returning the plaintext id.
func (a *Service) newSession(meta SessionMeta) (string, error) {
	id, idHash := NewSecret("")
	now := a.now().UTC()
	if err := a.Store.InsertSession(idHash,
		now.Format(time.RFC3339), now.Add(sessionTTL).Format(time.RFC3339),
		meta.UserAgent, meta.IP); err != nil {
		return "", err
	}
	return id, nil
}

// Sessions returns all live sessions for the Settings devices list, purging
// expired rows first — devices that never came back would otherwise linger
// forever (ValidateSession only deletes the session it is handed).
func (a *Service) Sessions() ([]store.Session, error) {
	sessions, err := a.Store.ListSessions()
	if err != nil {
		return nil, err
	}
	now := a.now().UTC().Format(time.RFC3339)
	live := sessions[:0]
	for _, s := range sessions {
		if s.ExpiresAt <= now {
			if err := a.Store.DeleteSession(s.IDHash); err != nil {
				return nil, err
			}
			continue
		}
		live = append(live, s)
	}
	return live, nil
}

// RevokeOtherSessions deletes every session except the one for currentID. An
// unknown currentID (stale cookie on a bearer-authenticated request) returns
// ErrUnknownSession instead of deleting every live session.
func (a *Service) RevokeOtherSessions(currentID string) error {
	current := HashSecret(currentID)
	sessions, err := a.Store.ListSessions()
	if err != nil {
		return err
	}
	found := false
	for _, s := range sessions {
		if s.IDHash == current {
			found = true
			break
		}
	}
	if !found {
		return ErrUnknownSession
	}
	for _, s := range sessions {
		if s.IDHash != current {
			if err := a.Store.DeleteSession(s.IDHash); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidateSession reports whether sessionID is a live session; valid sessions
// get their sliding expiry extended, expired ones are deleted.
func (a *Service) ValidateSession(sessionID string) bool {
	if sessionID == "" {
		return false
	}
	idHash := HashSecret(sessionID)
	sess, err := a.Store.GetSession(idHash)
	if err != nil {
		return false
	}
	exp, err := time.Parse(time.RFC3339, sess.ExpiresAt)
	if err != nil || a.now().After(exp) {
		_ = a.Store.DeleteSession(idHash)
		return false
	}
	now := a.now().UTC()
	_ = a.Store.TouchSession(idHash,
		now.Format(time.RFC3339), now.Add(sessionTTL).Format(time.RFC3339))
	return true
}

// ValidateAPIToken reports whether token matches the stored API-token hash.
func (a *Service) ValidateAPIToken(token string) bool {
	if token == "" {
		return false
	}
	want, err := a.Store.GetSetting(store.SettingAPITokenHash)
	if err != nil {
		return false
	}
	return HashSecret(token) == want
}

// Logout deletes the session for sessionID (no-op when absent).
func (a *Service) Logout(sessionID string) error {
	return a.Store.DeleteSession(HashSecret(sessionID))
}

// ChangePassword verifies current and stores a new hash.
func (a *Service) ChangePassword(current, next string) error {
	hash, err := a.Store.GetSetting(store.SettingPasswordHash)
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotSetup
	}
	if err != nil {
		return err
	}
	if !VerifyPassword(current, hash) {
		return ErrBadCredentials
	}
	if len(next) < minPasswordLen {
		return ErrWeakPassword
	}
	newHash, err := HashPassword(next)
	if err != nil {
		return err
	}
	return a.Store.SetSetting(store.SettingPasswordHash, newHash)
}

// RegenerateAPIToken replaces the API token, returning the new plaintext
// (displayed once).
func (a *Service) RegenerateAPIToken() (string, error) {
	token, tokenHash := NewSecret("hmr_")
	if err := a.Store.SetSetting(store.SettingAPITokenHash, tokenHash); err != nil {
		return "", err
	}
	return token, nil
}

// ResetPassword clears the password, all sessions, and the API token — the
// `server --reset-password` lockout escape hatch. Setup runs again afterwards.
func (a *Service) ResetPassword() error {
	if err := a.Store.DeleteSetting(store.SettingPasswordHash); err != nil {
		return err
	}
	if err := a.Store.DeleteSetting(store.SettingAPITokenHash); err != nil {
		return err
	}
	return a.Store.DeleteAllSessions()
}
