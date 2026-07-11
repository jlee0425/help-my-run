package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

// claudeStatusDTO is GET /api/claude/status.
type claudeStatusDTO struct {
	BinaryFound   bool   `json:"binary_found"`
	Authenticated bool   `json:"authenticated"`
	Model         string `json:"model"`
	Detail        string `json:"detail"`
	CheckedAt     string `json:"checked_at"`
}

// ClaudeProbe checks the claude CLI's health. Injected so handler tests use
// canned envelopes (no real claude, no quota burn).
type ClaudeProbe struct {
	Bin    string
	Model  string
	Runner llm.Runner // one claude -p invocation

	mu      sync.Mutex
	cached  *claudeStatusDTO
	fetched time.Time
}

const claudeStatusTTL = 10 * time.Minute

// Status probes (or serves the ≤10-min cache). refresh bypasses the cache.
func (p *ClaudeProbe) Status(ctx context.Context, refresh bool) claudeStatusDTO {
	p.mu.Lock()
	if !refresh && p.cached != nil && time.Since(p.fetched) < claudeStatusTTL {
		out := *p.cached
		p.mu.Unlock()
		return out
	}
	p.mu.Unlock()

	out := claudeStatusDTO{Model: p.Model, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
	if _, err := exec.LookPath(p.Bin); err != nil {
		out.Detail = "`claude` CLI not installed."
	} else {
		out.BinaryFound = true
		probeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		args := []string{
			"-p", "Reply with exactly: ok",
			"--model", p.Model,
			"--output-format", "json",
			"--allowedTools", "",
			"--no-session-persistence",
		}
		raw, runErr := p.Runner.Run(probeCtx, args, "")
		var env llm.Envelope
		if len(raw) > 0 {
			env, _ = llm.ParseEnvelope(raw)
		}
		if runErr != nil || env.IsError {
			out.Detail = llm.ClassifyFailure(env, runErr)
		} else {
			out.Authenticated = true
			out.Detail = "Subscription active."
		}
	}

	p.mu.Lock()
	c := out
	p.cached = &c
	p.fetched = time.Now()
	p.mu.Unlock()
	return out
}

// Invalidate clears the cache (token set/removed).
func (p *ClaudeProbe) Invalidate() {
	p.mu.Lock()
	p.cached = nil
	p.mu.Unlock()
}

// GET /api/claude/status[?refresh=true]
func (h *handlers) claudeStatus(w http.ResponseWriter, r *http.Request) {
	if h.d.Demo {
		// Don't probe for a real CLI the demo visitor won't have — present the
		// card honestly: the coach works, on sample data.
		writeJSON(w, http.StatusOK, claudeStatusDTO{
			BinaryFound:   true,
			Authenticated: true,
			Model:         "demo",
			Detail:        "Demo mode — coach responses are curated samples. Self-host to run the live coach on your Claude subscription.",
			CheckedAt:     time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	if h.d.Claude == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "claude probe not wired"})
		return
	}
	writeJSON(w, http.StatusOK, h.d.Claude.Status(r.Context(), r.URL.Query().Get("refresh") == "true"))
}

// PUT /api/claude/token {token} — store a `claude setup-token` value
// (subscription path; NEVER an Anthropic API key).
func (h *handlers) claudeTokenSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	tok := strings.TrimSpace(in.Token)
	// claude setup-token mints sk-ant-oat… OAuth tokens. An sk-ant-api… key
	// would silently switch billing to the metered API — refuse it.
	if !strings.HasPrefix(tok, "sk-ant-oat") {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "not a setup-token — run `claude setup-token` (subscription); API keys are not supported",
		})
		return
	}
	if err := h.d.Store.SetSetting(store.SettingClaudeToken, tok); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if h.d.Claude != nil {
		h.d.Claude.Invalidate()
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/claude/token
func (h *handlers) claudeTokenDelete(w http.ResponseWriter, r *http.Request) {
	if err := h.d.Store.DeleteSetting(store.SettingClaudeToken); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if h.d.Claude != nil {
		h.d.Claude.Invalidate()
	}
	w.WriteHeader(http.StatusNoContent)
}
