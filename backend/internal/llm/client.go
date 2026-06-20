// Package llm invokes the Claude Code CLI (`claude -p`) headlessly and extracts
// the model's JSON from the result envelope. The Runner is injectable so tests
// use canned envelopes (no real claude/network).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Envelope is the parsed `claude -p --output-format json` result envelope.
type Envelope struct {
	Type           string  `json:"type"`
	Subtype        string  `json:"subtype"`
	IsError        bool    `json:"is_error"`
	APIErrorStatus *int    `json:"api_error_status"`
	Result         string  `json:"result"`
	StopReason     string  `json:"stop_reason"`
	SessionID      string  `json:"session_id"`
	NumTurns       int     `json:"num_turns"`
	DurationMs     int     `json:"duration_ms"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
}

// ParseEnvelope unmarshals raw `claude -p` stdout into an Envelope.
func ParseEnvelope(b []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return Envelope{}, fmt.Errorf("llm: parse envelope: %w", err)
	}
	return e, nil
}

// Runner executes one `claude -p` call. Injectable for tests.
type Runner interface {
	Run(ctx context.Context, args []string, stdin string) (stdout []byte, err error)
}

// errExecNotFound is the sentinel for a missing claude binary (matches exec.ErrNotFound).
var errExecNotFound = exec.ErrNotFound

// ExecRunner is the production Runner backed by os/exec.
type ExecRunner struct {
	Bin string // claude binary path (Config.ClaudeBin)
}

// Run executes Bin with args, writing stdin, capturing stdout. A non-zero exit
// surfaces stderr in the error; a missing binary surfaces exec.ErrNotFound.
func (r ExecRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return stdout.Bytes(), fmt.Errorf("claude exit %d: %s", ee.ExitCode(), stderr.String())
		}
		return nil, err // includes exec.ErrNotFound
	}
	return stdout.Bytes(), nil
}

// ErrMalformedJSON is returned when the model output cannot be extracted/parsed
// even after the one retry.
var ErrMalformedJSON = errors.New("llm: malformed JSON from claude -p")

// ExtractJSON pulls the JSON object out of a claude -p .result string and
// unmarshals it into v (inner-JSON extraction rule steps 1–4; no retry).
func ExtractJSON(result string, v any) error {
	s := strings.TrimSpace(result)
	if s == "" {
		return ErrMalformedJSON
	}
	// Strip a fenced ```json / ``` block.
	if strings.HasPrefix(s, "```") {
		nl := strings.IndexByte(s, '\n')
		if nl >= 0 {
			s = s[nl+1:]
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	} else if !strings.HasPrefix(s, "{") {
		// Take the first { .. last } span.
		first := strings.IndexByte(s, '{')
		last := strings.LastIndexByte(s, '}')
		if first < 0 || last < first {
			return ErrMalformedJSON
		}
		s = s[first : last+1]
	}
	if err := json.Unmarshal([]byte(s), v); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedJSON, err)
	}
	return nil
}

// ClassifyFailure maps a failed call (exit error and/or is_error envelope) to a
// user-facing message (success/failure determination rules).
func ClassifyFailure(env Envelope, runErr error) string {
	if errors.Is(runErr, errExecNotFound) {
		return "`claude` CLI not installed."
	}
	low := strings.ToLower(env.Result)
	// Not-logged-in / auth-failure keywords.
	for _, kw := range []string{"login", "logged in", "authenticate", "unauthorized", "credential"} {
		if strings.Contains(low, kw) {
			return "Claude not logged in — run `claude auth login`."
		}
	}
	// Usage / rate / billing-limit keywords (or an explicit API error status).
	if env.APIErrorStatus != nil {
		return "Claude rate/usage limit hit — try later."
	}
	for _, kw := range []string{"limit", "rate", "credit", "quota"} {
		if strings.Contains(low, kw) {
			return "Claude rate/usage limit hit — try later."
		}
	}
	// Generic fallback: surface the raw .result (handler maps this to a 502-class error).
	if env.Result != "" {
		return "Claude error: " + env.Result
	}
	if runErr != nil {
		return "Claude error: " + runErr.Error()
	}
	return "Claude error (unknown)."
}

// Client wraps a Runner with model/flag defaults.
type Client struct {
	Runner  Runner
	Model   string
	Timeout time.Duration
}
