package chat

import (
	"context"
	stderrors "errors"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/store"
)

// captureRunner records args + stdin and returns a canned envelope.
type captureRunner struct {
	out  []byte
	args []string
	body string
}

func (r *captureRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	r.args = args
	r.body = stdin
	return r.out, nil
}

// failRunner returns an is_error envelope (classified failure -> *llm.CallError).
type failRunner struct{ calls int }

func (r *failRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	r.calls++
	return []byte(`{"type":"result","is_error":true,"result":"please login"}`), nil
}

const chatEnv = `{"type":"result","subtype":"success","is_error":false,"result":"{\"text\":\"Your Z2 pace improved ~8 s/km over 12 weeks.\"}"}`

func hasPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}

// newEngineStore builds a fresh migrated store + a seeded profile (buildContextPack
// requires the profile row), returning a chat Engine wired to the given runner.
func newEngineStore(t *testing.T, r llm.Runner) (*Engine, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "engine.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := s.UpsertAthleteProfile(store.AthleteProfile{
		TargetWeeklyKm: 30, ProgressionMode: "build", RunConstraintsJSON: "{}", GoalText: "base",
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	c := &llm.Client{Runner: r, Model: "m"}
	pe := progress.New(s, c, "m")
	return New(s, c, pe, "m", 6), s
}

func TestAnswerSuccessPersistsBothTurns(t *testing.T) {
	r := &captureRunner{out: []byte(chatEnv)}
	e, s := newEngineStore(t, r)

	msg, err := e.Answer(context.Background(), "How is my Zone 2 pace trending?")
	if err != nil {
		t.Fatalf("Answer error = %v", err)
	}
	// Returns the assistant turn parsed from the {"text":...} envelope.
	if msg.Role != "assistant" {
		t.Errorf("role = %q, want assistant", msg.Role)
	}
	if msg.Content != "Your Z2 pace improved ~8 s/km over 12 weeks." {
		t.Errorf("content = %q", msg.Content)
	}
	if msg.CreatedAt == "" || msg.ID == 0 {
		t.Errorf("assistant turn not stamped/persisted: %+v", msg)
	}

	// Both turns persisted (user + assistant) = 2 rows.
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Errorf("row count = %d, want 2 (user + assistant)", n)
	}

	// stdin (r.body) carries the pack, the history, and the new message.
	if !strings.Contains(r.body, `"pack"`) || !strings.Contains(r.body, `"generated_at"`) {
		t.Errorf("stdin missing pack: %s", r.body)
	}
	if !strings.Contains(r.body, `"history"`) {
		t.Errorf("stdin missing history: %s", r.body)
	}
	if !strings.Contains(r.body, "How is my Zone 2 pace trending?") {
		t.Errorf("stdin missing message: %s", r.body)
	}
	// History carries the just-appended user turn (oldest-first).
	if !strings.Contains(r.body, `"Role":"user"`) && !strings.Contains(r.body, `"role":"user"`) {
		t.Errorf("stdin history missing user turn: %s", r.body)
	}

	// argv carries the chat prompt + flags (verbatim mirror of analyzeArgs).
	joined := strings.Join(r.args, " ")
	if !strings.Contains(joined, "data analyst") {
		t.Errorf("args missing chat system prompt: %v", r.args)
	}
	if !hasPair(r.args, "--model", "m") || !hasPair(r.args, "--output-format", "json") {
		t.Errorf("args missing model/output-format: %v", r.args)
	}
	if !hasPair(r.args, "--allowedTools", "") {
		t.Errorf("args missing allowedTools: %v", r.args)
	}
}

func TestAnswerCarriesPriorHistory(t *testing.T) {
	r := &captureRunner{out: []byte(chatEnv)}
	e, s := newEngineStore(t, r)
	// Seed two prior turns so history (last-N) carries them.
	if _, err := s.AppendChatMessage("user", "earlier question"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := s.AppendChatMessage("assistant", "earlier answer"); err != nil {
		t.Fatalf("seed assistant: %v", err)
	}

	if _, err := e.Answer(context.Background(), "follow up"); err != nil {
		t.Fatalf("Answer error = %v", err)
	}
	if !strings.Contains(r.body, "earlier question") || !strings.Contains(r.body, "earlier answer") {
		t.Errorf("stdin history missing prior turns: %s", r.body)
	}
	if !strings.Contains(r.body, "follow up") {
		t.Errorf("stdin missing new message: %s", r.body)
	}
}

func TestAnswerLLMFailureReturnsTypedErrorNoFabrication(t *testing.T) {
	r := &failRunner{}
	e, s := newEngineStore(t, r)

	msg, err := e.Answer(context.Background(), "anything")
	if err == nil {
		t.Fatal("Answer error = nil, want typed llm error (no fallback)")
	}
	// Typed error: *llm.CallError (classified failure).
	var ce *llm.CallError
	if !errorsAs(err, &ce) {
		t.Errorf("error type = %T, want *llm.CallError", err)
	}
	// No fabricated assistant turn returned.
	if msg.Content != "" || msg.ID != 0 {
		t.Errorf("msg = %+v, want zero value (no fabrication)", msg)
	}
	// The user turn was persisted, but NO assistant turn -> exactly 1 row.
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM chat_messages`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("row count = %d, want 1 (user only; no assistant on llm failure)", n)
	}
}

func errorsAs(err error, target any) bool {
	return stderrors.As(err, target)
}
