package store

import (
	"errors"
	"testing"
)

func TestSettingsRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.GetSetting("password_hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSetting absent = %v, want ErrNotFound", err)
	}
	if err := s.SetSetting("password_hash", "h1"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	got, err := s.GetSetting("password_hash")
	if err != nil || got != "h1" {
		t.Fatalf("GetSetting = %q,%v want h1,nil", got, err)
	}
	// Upsert overwrites.
	if err := s.SetSetting("password_hash", "h2"); err != nil {
		t.Fatalf("SetSetting overwrite: %v", err)
	}
	got, _ = s.GetSetting("password_hash")
	if got != "h2" {
		t.Fatalf("GetSetting after overwrite = %q, want h2", got)
	}
	if err := s.DeleteSetting("password_hash"); err != nil {
		t.Fatalf("DeleteSetting: %v", err)
	}
	if _, err := s.GetSetting("password_hash"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSetting after delete = %v, want ErrNotFound", err)
	}
	// Deleting a missing key is a no-op.
	if err := s.DeleteSetting("nope"); err != nil {
		t.Fatalf("DeleteSetting missing: %v", err)
	}
}

func TestSessionsLifecycle(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.GetSession("abc"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetSession absent = %v, want ErrNotFound", err)
	}
	if err := s.InsertSession("abc", "2026-07-09T00:00:00Z", "2026-08-08T00:00:00Z"); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	sess, err := s.GetSession("abc")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if sess.IDHash != "abc" || sess.CreatedAt != "2026-07-09T00:00:00Z" ||
		sess.LastSeenAt != "2026-07-09T00:00:00Z" || sess.ExpiresAt != "2026-08-08T00:00:00Z" {
		t.Fatalf("GetSession = %+v", sess)
	}
	if err := s.TouchSession("abc", "2026-07-10T00:00:00Z", "2026-08-09T00:00:00Z"); err != nil {
		t.Fatalf("TouchSession: %v", err)
	}
	sess, _ = s.GetSession("abc")
	if sess.LastSeenAt != "2026-07-10T00:00:00Z" || sess.ExpiresAt != "2026-08-09T00:00:00Z" {
		t.Fatalf("after touch = %+v", sess)
	}
	if err := s.InsertSession("def", "2026-07-09T00:00:00Z", "2026-08-08T00:00:00Z"); err != nil {
		t.Fatalf("InsertSession 2: %v", err)
	}
	if err := s.DeleteSession("abc"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := s.GetSession("abc"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted session still present")
	}
	if _, err := s.GetSession("def"); err != nil {
		t.Fatalf("other session should survive: %v", err)
	}
	if err := s.DeleteAllSessions(); err != nil {
		t.Fatalf("DeleteAllSessions: %v", err)
	}
	if _, err := s.GetSession("def"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteAllSessions left rows behind")
	}
}
