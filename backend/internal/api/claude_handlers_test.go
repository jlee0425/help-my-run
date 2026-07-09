package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/store"
)

// cannedRunner returns fixed claude -p envelopes and counts invocations.
type cannedRunner struct {
	out   []byte
	err   error
	calls int
}

func (c *cannedRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	c.calls++
	return c.out, c.err
}

func claudeTestServer(t *testing.T, runner *cannedRunner, bin string) (http.Handler, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "claude.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	deps := Deps{
		Store: s, Auth: testAuth(t, s),
		SyncFunc: func(ctx context.Context) (string, int, *string) { return "ok", 0, nil },
		Coach:    &fakeCoach{}, ImageDir: t.TempDir(), Agent: &fakeAgent{},
		Progress: &fakeProgress{}, Streams: &fakeStreams{}, Chat: &fakeChat{},
		Claude:   &ClaudeProbe{Bin: bin, Model: "claude-opus-4-8", Runner: runner},
	}
	return NewRouter(deps), s
}

// existingBin returns a path guaranteed to satisfy exec.LookPath.
func existingBin(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestClaudeStatusAuthenticated(t *testing.T) {
	runner := &cannedRunner{out: []byte(`{"type":"result","subtype":"success","is_error":false,"result":"ok"}`)}
	h, _ := claudeTestServer(t, runner, existingBin(t))

	rec := do(t, h, http.MethodGet, "/api/claude/status", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body.String())
	}
	var out claudeStatusDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.BinaryFound || !out.Authenticated || out.Model != "claude-opus-4-8" {
		t.Fatalf("dto = %+v", out)
	}

	// Second call within the TTL: served from cache (no extra probe).
	_ = do(t, h, http.MethodGet, "/api/claude/status", testToken)
	if runner.calls != 1 {
		t.Fatalf("probe calls = %d, want 1 (cached)", runner.calls)
	}
	// refresh=true bypasses the cache.
	_ = do(t, h, http.MethodGet, "/api/claude/status?refresh=true", testToken)
	if runner.calls != 2 {
		t.Fatalf("probe calls = %d, want 2 after refresh", runner.calls)
	}
}

func TestClaudeStatusNotLoggedIn(t *testing.T) {
	runner := &cannedRunner{out: []byte(`{"type":"result","is_error":true,"result":"Please run claude auth login"}`)}
	h, _ := claudeTestServer(t, runner, existingBin(t))
	rec := do(t, h, http.MethodGet, "/api/claude/status", testToken)
	var out claudeStatusDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.BinaryFound || out.Authenticated {
		t.Fatalf("dto = %+v", out)
	}
	if out.Detail != "Claude not logged in — run `claude auth login`." {
		t.Fatalf("detail = %q", out.Detail)
	}
}

func TestClaudeStatusBinaryMissing(t *testing.T) {
	runner := &cannedRunner{}
	h, _ := claudeTestServer(t, runner, filepath.Join(t.TempDir(), "definitely-not-claude"))
	rec := do(t, h, http.MethodGet, "/api/claude/status", testToken)
	var out claudeStatusDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.BinaryFound || out.Authenticated || runner.calls != 0 {
		t.Fatalf("dto = %+v calls=%d", out, runner.calls)
	}
}

func TestClaudeTokenLifecycle(t *testing.T) {
	runner := &cannedRunner{out: []byte(`{"type":"result","is_error":false,"result":"ok"}`)}
	h, s := claudeTestServer(t, runner, existingBin(t))

	// API keys are refused (subscription-only constraint).
	rec := doBody(t, h, http.MethodPut, "/api/claude/token", testToken, `{"token":"sk-ant-api03-nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("api key = %d, want 400", rec.Code)
	}

	rec = doBody(t, h, http.MethodPut, "/api/claude/token", testToken, `{"token":"sk-ant-oat01-good"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("set token = %d (%s)", rec.Code, rec.Body.String())
	}
	if tok, err := s.GetSetting(store.SettingClaudeToken); err != nil || tok != "sk-ant-oat01-good" {
		t.Fatalf("stored = %q, %v", tok, err)
	}

	rec = doBody(t, h, http.MethodDelete, "/api/claude/token", testToken, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete token = %d", rec.Code)
	}
	if _, err := s.GetSetting(store.SettingClaudeToken); err == nil {
		t.Fatalf("token survives delete")
	}
}
