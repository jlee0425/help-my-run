package store

import "testing"

func TestOAuthStateSaveAndConsume(t *testing.T) {
	s := newTestStore(t)

	if err := s.SaveOAuthState("abc123"); err != nil {
		t.Fatalf("SaveOAuthState() error = %v", err)
	}

	// Consume succeeds once.
	if err := s.ConsumeOAuthState("abc123"); err != nil {
		t.Fatalf("ConsumeOAuthState(abc123) error = %v, want nil", err)
	}
	// Second consume of the same state fails (single-use).
	if err := s.ConsumeOAuthState("abc123"); err != ErrNotFound {
		t.Errorf("second ConsumeOAuthState = %v, want ErrNotFound", err)
	}
}

func TestConsumeUnknownOAuthState(t *testing.T) {
	s := newTestStore(t)
	if err := s.ConsumeOAuthState("never-saved"); err != ErrNotFound {
		t.Errorf("ConsumeOAuthState(unknown) = %v, want ErrNotFound", err)
	}
}
