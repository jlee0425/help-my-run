package auth

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
)

func newTestService(t *testing.T) (*Service, *time.Time) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	now := time.Date(2026, 7, 9, 8, 0, 0, 0, time.UTC)
	a := New(s)
	a.Now = func() time.Time { return now }
	return a, &now
}

func TestSetupLifecycle(t *testing.T) {
	a, _ := newTestService(t)

	req, err := a.SetupRequired()
	if err != nil || !req {
		t.Fatalf("SetupRequired fresh = %v,%v want true,nil", req, err)
	}
	if _, _, err := a.Setup("short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("Setup(short) = %v, want ErrWeakPassword", err)
	}
	sid, tok, err := a.Setup("a strong password")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if sid == "" || tok == "" {
		t.Fatalf("Setup returned empty session/token")
	}
	if req, _ := a.SetupRequired(); req {
		t.Fatalf("SetupRequired after setup = true")
	}
	if _, _, err := a.Setup("another password"); !errors.Is(err, ErrAlreadySetup) {
		t.Fatalf("second Setup = %v, want ErrAlreadySetup", err)
	}
	if !a.ValidateSession(sid) {
		t.Fatalf("setup session not valid")
	}
	if !a.ValidateAPIToken(tok) {
		t.Fatalf("setup api token not valid")
	}
	if a.ValidateAPIToken("hmr_deadbeef") {
		t.Fatalf("bogus token validated")
	}
}

func TestLoginBackoffAndSessions(t *testing.T) {
	a, now := newTestService(t)
	if _, _, err := a.Setup("a strong password"); err != nil {
		t.Fatalf("Setup: %v", err)
	}

	if _, err := a.Login("nope"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("Login wrong = %v, want ErrBadCredentials", err)
	}
	// Immediately retrying after a failure is throttled (backoff starts at 2s).
	if _, err := a.Login("a strong password"); !errors.Is(err, ErrThrottled) {
		t.Fatalf("Login during backoff = %v, want ErrThrottled", err)
	}
	// After the backoff window, correct password succeeds.
	*now = now.Add(5 * time.Second)
	sid, err := a.Login("a strong password")
	if err != nil {
		t.Fatalf("Login after backoff: %v", err)
	}
	if !a.ValidateSession(sid) {
		t.Fatalf("login session invalid")
	}

	// Sliding expiry: 31 days idle -> invalid and deleted.
	*now = now.Add(31 * 24 * time.Hour)
	if a.ValidateSession(sid) {
		t.Fatalf("session valid after 31 idle days")
	}

	// Logout invalidates.
	*now = now.Add(time.Minute)
	sid2, err := a.Login("a strong password")
	if err != nil {
		t.Fatalf("re-login: %v", err)
	}
	if err := a.Logout(sid2); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if a.ValidateSession(sid2) {
		t.Fatalf("session valid after logout")
	}
}

func TestChangePasswordAndRegenerate(t *testing.T) {
	a, now := newTestService(t)
	sid, tok, err := a.Setup("a strong password")
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	if err := a.ChangePassword("wrong", "next strong password"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("ChangePassword wrong current = %v", err)
	}
	if err := a.ChangePassword("a strong password", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("ChangePassword weak next = %v", err)
	}
	if err := a.ChangePassword("a strong password", "next strong password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if _, err := a.Login("a strong password"); !errors.Is(err, ErrBadCredentials) {
		t.Fatalf("old password still works")
	}
	*now = now.Add(5 * time.Second) // clear the failed-attempt backoff
	if _, err := a.Login("next strong password"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}

	newTok, err := a.RegenerateAPIToken()
	if err != nil {
		t.Fatalf("RegenerateAPIToken: %v", err)
	}
	if a.ValidateAPIToken(tok) {
		t.Fatalf("old api token still valid after regenerate")
	}
	if !a.ValidateAPIToken(newTok) {
		t.Fatalf("new api token invalid")
	}

	// ResetPassword clears everything.
	if err := a.ResetPassword(); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if req, _ := a.SetupRequired(); !req {
		t.Fatalf("SetupRequired after reset = false")
	}
	if a.ValidateSession(sid) || a.ValidateAPIToken(newTok) {
		t.Fatalf("credentials survive reset")
	}
}
