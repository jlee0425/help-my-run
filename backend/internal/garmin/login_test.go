package garmin

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

// fakeManager builds a LoginManager driving testdata/fake_login_web.py in mode.
func fakeManager(t *testing.T, mode string, timeout time.Duration) *LoginManager {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH")
	}
	m := NewLoginManager(py, "testdata/fake_login_web.py", []string{"FAKE_MODE=" + mode})
	if timeout > 0 {
		m.Timeout = timeout
	}
	return m
}

func TestLoginOKWithoutMFA(t *testing.T) {
	m := fakeManager(t, "ok", 0)
	res := m.Start(context.Background(), "you@example.com", "pw")
	if res.State != LoginOK {
		t.Fatalf("state = %q (errkind=%q), want ok", res.State, res.ErrKind)
	}
}

func TestLoginMFAHandshake(t *testing.T) {
	m := fakeManager(t, "mfa", 0)
	res := m.Start(context.Background(), "you@example.com", "pw")
	if res.State != LoginMFARequired || res.LoginID == "" {
		t.Fatalf("start = %+v, want mfa_required with login id", res)
	}
	done := m.SubmitMFA(context.Background(), res.LoginID, "424242")
	if done.State != LoginOK {
		t.Fatalf("mfa submit = %+v, want ok", done)
	}
	// The pending slot is freed: a new login can start.
	res2 := m.Start(context.Background(), "you@example.com", "pw")
	if res2.State != LoginMFARequired {
		t.Fatalf("second start = %+v", res2)
	}
	if bad := m.SubmitMFA(context.Background(), res2.LoginID, "000000"); bad.State != LoginError || bad.ErrKind != "auth_failed" {
		t.Fatalf("bad code = %+v, want auth_failed", bad)
	}
}

func TestLoginAuthAndRateLimitKinds(t *testing.T) {
	if res := fakeManager(t, "autherr", 0).Start(context.Background(), "e", "p"); res.State != LoginError || res.ErrKind != "auth_failed" {
		t.Fatalf("autherr = %+v", res)
	}
	if res := fakeManager(t, "ratelimit", 0).Start(context.Background(), "e", "p"); res.State != LoginError || res.ErrKind != "rate_limited" {
		t.Fatalf("ratelimit = %+v", res)
	}
	if res := fakeManager(t, "garbage", 0).Start(context.Background(), "e", "p"); res.State != LoginError {
		t.Fatalf("garbage = %+v, want error", res)
	}
}

func TestLoginTimeoutKillsWorker(t *testing.T) {
	m := fakeManager(t, "hang", 300*time.Millisecond)
	start := time.Now()
	res := m.Start(context.Background(), "e", "p")
	if res.State != LoginError || res.ErrKind != "timeout" {
		t.Fatalf("hang = %+v, want timeout", res)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("timeout did not kill promptly")
	}
	// Slot freed after timeout.
	if res := fakeManager(t, "ok", 0).Start(context.Background(), "e", "p"); res.State != LoginOK {
		t.Fatalf("after timeout = %+v", res)
	}
}

func TestLoginBusyWhilePendingMFA(t *testing.T) {
	m := fakeManager(t, "mfa", 0)
	res := m.Start(context.Background(), "e", "p")
	if res.State != LoginMFARequired {
		t.Fatalf("start = %+v", res)
	}
	if busy := m.Start(context.Background(), "e", "p"); busy.State != LoginError || busy.ErrKind != "busy" {
		t.Fatalf("concurrent start = %+v, want busy", busy)
	}
	// Clean up the pending login.
	_ = m.SubmitMFA(context.Background(), res.LoginID, "424242")
}

func TestSubmitMFAUnknownID(t *testing.T) {
	m := fakeManager(t, "mfa", 0)
	if res := m.SubmitMFA(context.Background(), "nope", "1"); res.State != LoginError || res.ErrKind != "expired" {
		t.Fatalf("unknown id = %+v, want expired", res)
	}
}
