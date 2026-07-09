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
)

const (
	minPasswordLen = 8
	sessionTTL     = 30 * 24 * time.Hour
	maxBackoff     = 60 * time.Second
)

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
func (a *Service) Setup(password string) (sessionID, apiToken string, err error) {
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
	sessionID, err = a.newSession()
	if err != nil {
		return "", "", err
	}
	return sessionID, apiToken, nil
}

// Login verifies the password and mints a session. Failures feed an in-memory
// exponential backoff (2^n seconds, capped at 60s).
func (a *Service) Login(password string) (string, error) {
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
	return a.newSession()
}

// newSession mints and persists a session, returning the plaintext id.
func (a *Service) newSession() (string, error) {
	id, idHash := NewSecret("")
	now := a.now().UTC()
	if err := a.Store.InsertSession(idHash,
		now.Format(time.RFC3339), now.Add(sessionTTL).Format(time.RFC3339)); err != nil {
		return "", err
	}
	return id, nil
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
