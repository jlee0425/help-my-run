package store

import "testing"

func TestChatMessagesAppendListClear(t *testing.T) {
	s := newTestStore(t)

	// Append three turns: user, assistant, user (in order).
	id1, err := s.AppendChatMessage("user", "how is my zone 2 pace?")
	if err != nil {
		t.Fatalf("AppendChatMessage 1: %v", err)
	}
	id2, err := s.AppendChatMessage("assistant", "your pace at Z2 improved ~8 s/km")
	if err != nil {
		t.Fatalf("AppendChatMessage 2: %v", err)
	}
	id3, err := s.AppendChatMessage("user", "and my decoupling?")
	if err != nil {
		t.Fatalf("AppendChatMessage 3: %v", err)
	}
	// AUTOINCREMENT ids are monotonically increasing.
	if !(id1 < id2 && id2 < id3) {
		t.Errorf("ids not increasing: %d, %d, %d", id1, id2, id3)
	}

	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Fatalf("row count = %d, want 3", n)
	}

	// ListChatMessages(2) returns the last 2 turns, OLDEST-FIRST (ids ascending):
	// the assistant turn (id2) then the newest user turn (id3).
	got, err := s.ListChatMessages(2)
	if err != nil {
		t.Fatalf("ListChatMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("list len = %d, want 2", len(got))
	}
	if got[0].ID != id2 || got[1].ID != id3 {
		t.Errorf("order = [%d,%d], want [%d,%d] (oldest-first)", got[0].ID, got[1].ID, id2, id3)
	}
	if got[0].Role != "assistant" || got[1].Role != "user" {
		t.Errorf("roles = [%q,%q], want [assistant,user]", got[0].Role, got[1].Role)
	}
	if got[1].Content != "and my decoupling?" {
		t.Errorf("content = %q, want %q", got[1].Content, "and my decoupling?")
	}
	// created_at is server-stamped (non-empty RFC3339).
	if got[0].CreatedAt == "" || got[1].CreatedAt == "" {
		t.Errorf("created_at empty: %+v", got)
	}

	// A limit larger than the row count returns all rows, oldest-first.
	all, err := s.ListChatMessages(50)
	if err != nil {
		t.Fatalf("ListChatMessages(50): %v", err)
	}
	if len(all) != 3 || all[0].ID != id1 || all[2].ID != id3 {
		t.Errorf("all = %+v, want 3 rows id1..id3 oldest-first", all)
	}

	// ClearChatMessages wipes the thread.
	if err := s.ClearChatMessages(); err != nil {
		t.Fatalf("ClearChatMessages: %v", err)
	}
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&n)
	if n != 0 {
		t.Errorf("after clear count = %d, want 0", n)
	}

	// Clear on an empty thread is a no-op (no error).
	if err := s.ClearChatMessages(); err != nil {
		t.Errorf("ClearChatMessages on empty: %v", err)
	}

	// List on an empty thread returns an empty (non-nil) slice, not a panic.
	empty, err := s.ListChatMessages(10)
	if err != nil {
		t.Fatalf("ListChatMessages on empty: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("empty list = %v, want non-nil len 0", empty)
	}
}
