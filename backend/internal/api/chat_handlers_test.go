package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

// newChatServer wires a server whose Chat seam is the given fake, returning the
// handler + the live store (GET/DELETE read/write the store directly).
func newChatServer(t *testing.T, fc *fakeChat) (http.Handler, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "chat-api.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	deps := Deps{
		Store:    s,
		Strava:   strava.NewWithBase("1", "x", "http://cb", "http://unused"),
		APIToken: testToken,
		Coach:    &fakeCoach{},
		ImageDir: t.TempDir(),
		Agent:    &fakeAgent{},
		Pusher:   &fakePusher{},
		Progress: &fakeProgress{},
		Streams:  &fakeStreams{},
		Chat:     fc,
	}
	return NewRouter(deps), s
}

func TestChatRequiresAuth(t *testing.T) {
	h, _ := newChatServer(t, &fakeChat{})
	for _, m := range []string{http.MethodPost, http.MethodGet, http.MethodDelete} {
		rec := do(t, h, m, "/api/chat", "")
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s /api/chat unauth = %d, want 401", m, rec.Code)
		}
	}
}

func TestChatPostHappyPath(t *testing.T) {
	fc := &fakeChat{msg: store.ChatMessage{
		Role: "assistant", Content: "Your Z2 pace dropped ~8 s/km.", CreatedAt: "2026-06-22T09:14:02Z",
	}}
	h, _ := newChatServer(t, fc)
	rec := doBody(t, h, http.MethodPost, "/api/chat", testToken, `{"message":"How is my Zone 2 pace trending?"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var out chatMessageDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Role != "assistant" {
		t.Errorf("role = %q, want assistant", out.Role)
	}
	if out.Content != "Your Z2 pace dropped ~8 s/km." {
		t.Errorf("content = %q", out.Content)
	}
	if out.CreatedAt != "2026-06-22T09:14:02Z" {
		t.Errorf("created_at = %q", out.CreatedAt)
	}
	if fc.lastMsg != "How is my Zone 2 pace trending?" {
		t.Errorf("Answer got message %q", fc.lastMsg)
	}
}

func TestChatPostEmptyMessage(t *testing.T) {
	h, _ := newChatServer(t, &fakeChat{})
	rec := doBody(t, h, http.MethodPost, "/api/chat", testToken, `{"message":"   "}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "message required") {
		t.Errorf("body = %q, want message required", rec.Body.String())
	}
}

func TestChatPostBadBody(t *testing.T) {
	h, _ := newChatServer(t, &fakeChat{})
	rec := doBody(t, h, http.MethodPost, "/api/chat", testToken, `not json`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestChatPostLLMFailure502(t *testing.T) {
	fc := &fakeChat{answerErr: errors.New("claude -p: not logged in")}
	h, _ := newChatServer(t, fc)
	rec := doBody(t, h, http.MethodPost, "/api/chat", testToken, `{"message":"hi"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not logged in") {
		t.Errorf("body = %q, want the typed error surfaced", rec.Body.String())
	}
}

func TestChatHistory(t *testing.T) {
	h, s := newChatServer(t, &fakeChat{})
	if _, err := s.AppendChatMessage("user", "q1"); err != nil {
		t.Fatalf("append: %v", err)
	}
	if _, err := s.AppendChatMessage("assistant", "a1"); err != nil {
		t.Fatalf("append: %v", err)
	}
	rec := do(t, h, http.MethodGet, "/api/chat?limit=50", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out chatHistoryResp
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(out.Messages))
	}
	// oldest-first
	if out.Messages[0].Role != "user" || out.Messages[0].Content != "q1" {
		t.Errorf("msg[0] = %+v, want user/q1", out.Messages[0])
	}
	if out.Messages[1].Role != "assistant" || out.Messages[1].Content != "a1" {
		t.Errorf("msg[1] = %+v, want assistant/a1", out.Messages[1])
	}
}

func TestChatClear(t *testing.T) {
	h, s := newChatServer(t, &fakeChat{})
	if _, err := s.AppendChatMessage("user", "q1"); err != nil {
		t.Fatalf("append: %v", err)
	}
	rec := do(t, h, http.MethodDelete, "/api/chat", testToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
	msgs, err := s.ListChatMessages(50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("after clear, messages = %d, want 0", len(msgs))
	}
}
