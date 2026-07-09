package garmin

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// LoginState is the outcome of one login step (M5 spec §7).
type LoginState string

const (
	LoginOK          LoginState = "ok"
	LoginMFARequired LoginState = "mfa_required"
	LoginError       LoginState = "error"
)

// LoginResult is the manager's answer to Start/SubmitMFA. ErrKind values:
// bad_input | auth_failed | rate_limited | timeout | busy | expired | internal.
type LoginResult struct {
	State   LoginState
	LoginID string // set when State == LoginMFARequired
	ErrKind string // set when State == LoginError
}

// protoLine is one worker stdout protocol line.
type protoLine struct {
	Status     string `json:"status"`
	Error      string `json:"error"`
	Tokenstore string `json:"tokenstore"`
}

// pendingLogin is a login blocked on an MFA code.
type pendingLogin struct {
	id    string
	cmd   *exec.Cmd
	stdin io.WriteCloser
	lines chan protoLine
	timer *time.Timer
}

// LoginManager drives the worker's `login-web` subprocess: at most one login
// in flight; credentials go over stdin, never argv/env/logs.
type LoginManager struct {
	Python   string
	Script   string
	ExtraEnv []string
	Timeout  time.Duration // whole-login deadline (creds -> ok/error), default 5m

	mu      sync.Mutex
	pending *pendingLogin
}

// NewLoginManager constructs a manager (Timeout defaults to 5 minutes).
func NewLoginManager(python, script string, extraEnv []string) *LoginManager {
	return &LoginManager{Python: python, Script: script, ExtraEnv: extraEnv, Timeout: 5 * time.Minute}
}

// Start begins a login. It returns ok/error terminally, or mfa_required with a
// LoginID for SubmitMFA. A second Start while one is pending returns busy.
func (m *LoginManager) Start(ctx context.Context, email, password string) LoginResult {
	m.mu.Lock()
	if m.pending != nil {
		m.mu.Unlock()
		return LoginResult{State: LoginError, ErrKind: "busy"}
	}
	m.mu.Unlock()

	cmd := exec.Command(m.Python, m.Script, "login-web")
	if len(m.ExtraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), m.ExtraEnv...)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return LoginResult{State: LoginError, ErrKind: "internal"}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return LoginResult{State: LoginError, ErrKind: "internal"}
	}
	if err := cmd.Start(); err != nil {
		return LoginResult{State: LoginError, ErrKind: "internal"}
	}

	lines := make(chan protoLine, 4)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			var pl protoLine
			if jerr := json.Unmarshal(sc.Bytes(), &pl); jerr != nil {
				pl = protoLine{Status: "error", Error: "protocol"}
			}
			lines <- pl
		}
		close(lines)
	}()

	// Credentials ride stdin only.
	creds, _ := json.Marshal(map[string]string{"email": email, "password": password})
	if _, err := fmt.Fprintf(stdin, "%s\n", creds); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return LoginResult{State: LoginError, ErrKind: "internal"}
	}

	first, ok := m.waitLine(ctx, lines)
	if !ok {
		_ = stdin.Close()
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return LoginResult{State: LoginError, ErrKind: "timeout"}
	}

	switch first.Status {
	case "ok":
		_ = stdin.Close()
		_ = cmd.Wait()
		return LoginResult{State: LoginOK}
	case "mfa_required":
		id := newLoginID()
		p := &pendingLogin{id: id, cmd: cmd, stdin: stdin, lines: lines}
		p.timer = time.AfterFunc(m.Timeout, func() { m.expire(id) })
		m.mu.Lock()
		m.pending = p
		m.mu.Unlock()
		return LoginResult{State: LoginMFARequired, LoginID: id}
	default:
		_ = stdin.Close()
		_ = cmd.Wait()
		return LoginResult{State: LoginError, ErrKind: errKind(first.Error)}
	}
}

// SubmitMFA delivers the code to the pending login and waits for the verdict.
func (m *LoginManager) SubmitMFA(ctx context.Context, loginID, code string) LoginResult {
	m.mu.Lock()
	p := m.pending
	if p == nil || p.id != loginID {
		m.mu.Unlock()
		return LoginResult{State: LoginError, ErrKind: "expired"}
	}
	m.pending = nil // claim it; re-stash never happens (login-web is single-shot)
	m.mu.Unlock()
	p.timer.Stop()

	payload, _ := json.Marshal(map[string]string{"code": code})
	if _, err := fmt.Fprintf(p.stdin, "%s\n", payload); err != nil {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
		return LoginResult{State: LoginError, ErrKind: "internal"}
	}
	final, ok := m.waitLine(ctx, p.lines)
	_ = p.stdin.Close()
	if !ok {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
		return LoginResult{State: LoginError, ErrKind: "timeout"}
	}
	_ = p.cmd.Wait()
	if final.Status == "ok" {
		return LoginResult{State: LoginOK}
	}
	return LoginResult{State: LoginError, ErrKind: errKind(final.Error)}
}

// waitLine reads the next protocol line or gives up after Timeout/ctx-done.
func (m *LoginManager) waitLine(ctx context.Context, lines chan protoLine) (protoLine, bool) {
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	select {
	case pl, open := <-lines:
		if !open {
			return protoLine{}, false
		}
		return pl, true
	case <-time.After(timeout):
		return protoLine{}, false
	case <-ctx.Done():
		return protoLine{}, false
	}
}

// expire is the pending-login deadline: kill the worker and free the slot.
func (m *LoginManager) expire(loginID string) {
	m.mu.Lock()
	p := m.pending
	if p == nil || p.id != loginID {
		m.mu.Unlock()
		return
	}
	m.pending = nil
	m.mu.Unlock()
	_ = p.stdin.Close()
	_ = p.cmd.Process.Kill()
	_, _ = p.cmd.Process.Wait()
}

// errKind normalizes worker error kinds to the API's vocabulary.
func errKind(kind string) string {
	switch kind {
	case "bad_input", "auth_failed", "rate_limited":
		return kind
	default:
		return "auth_failed"
	}
}

// newLoginID mints an opaque id for the pending MFA exchange.
func newLoginID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
