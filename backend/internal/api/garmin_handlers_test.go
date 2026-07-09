package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/garmin"
)

// stubGarminLogin scripts LoginManager outcomes for handler tests.
type stubGarminLogin struct {
	startRes garmin.LoginResult
	mfaRes   garmin.LoginResult
	gotEmail string
	gotCode  string
}

func (s *stubGarminLogin) Start(ctx context.Context, email, password string) garmin.LoginResult {
	s.gotEmail = email
	return s.startRes
}

func (s *stubGarminLogin) SubmitMFA(ctx context.Context, loginID, code string) garmin.LoginResult {
	s.gotCode = code
	return s.mfaRes
}

func garminTestServer(t *testing.T, stub *stubGarminLogin, tokenstore string) http.Handler {
	t.Helper()
	h, s := newTestServer(t)
	_ = h
	deps := Deps{
		Store: s, Auth: testAuth(t, s),
		SyncFunc: func(ctx context.Context) (string, int, *string) { return "ok", 0, nil },
		Coach:    &fakeCoach{}, ImageDir: t.TempDir(), Agent: &fakeAgent{},
		Progress: &fakeProgress{}, Streams: &fakeStreams{}, Chat: &fakeChat{},
		GarminLogin: stub, GarminTokenstore: tokenstore,
	}
	return NewRouter(deps)
}

func TestGarminLoginFlow(t *testing.T) {
	stub := &stubGarminLogin{
		startRes: garmin.LoginResult{State: garmin.LoginMFARequired, LoginID: "lg1"},
		mfaRes:   garmin.LoginResult{State: garmin.LoginOK},
	}
	h := garminTestServer(t, stub, t.TempDir())

	rec := doBody(t, h, http.MethodPost, "/api/garmin/login", testToken,
		`{"email":"you@example.com","password":"pw"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("login = %d (%s), want 202", rec.Code, rec.Body.String())
	}
	var out map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out["status"] != "mfa_required" || out["login_id"] != "lg1" {
		t.Fatalf("body = %v", out)
	}
	if stub.gotEmail != "you@example.com" {
		t.Fatalf("email passed = %q", stub.gotEmail)
	}

	rec = doBody(t, h, http.MethodPost, "/api/garmin/login/mfa", testToken,
		`{"login_id":"lg1","code":"424242"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("mfa = %d (%s), want 200", rec.Code, rec.Body.String())
	}
	if stub.gotCode != "424242" {
		t.Fatalf("code passed = %q", stub.gotCode)
	}
}

func TestGarminLoginErrorMapping(t *testing.T) {
	cases := []struct {
		kind string
		want int
	}{
		{"auth_failed", http.StatusUnauthorized},
		{"rate_limited", http.StatusTooManyRequests},
		{"busy", http.StatusConflict},
		{"timeout", http.StatusGone},
		{"expired", http.StatusGone},
	}
	for _, tc := range cases {
		stub := &stubGarminLogin{startRes: garmin.LoginResult{State: garmin.LoginError, ErrKind: tc.kind}}
		h := garminTestServer(t, stub, t.TempDir())
		rec := doBody(t, h, http.MethodPost, "/api/garmin/login", testToken,
			`{"email":"e@x","password":"p"}`)
		if rec.Code != tc.want {
			t.Errorf("kind %s = %d, want %d", tc.kind, rec.Code, tc.want)
		}
	}
}

func TestGarminLoginValidation(t *testing.T) {
	h := garminTestServer(t, &stubGarminLogin{}, t.TempDir())
	if rec := doBody(t, h, http.MethodPost, "/api/garmin/login", testToken, `{"email":"only"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing password = %d, want 400", rec.Code)
	}
	if rec := doBody(t, h, http.MethodPost, "/api/garmin/login/mfa", testToken, `{"code":"1"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing login_id = %d, want 400", rec.Code)
	}
}

func TestGarminStatusAndDisconnect(t *testing.T) {
	tokenstore := t.TempDir()
	// Populated tokenstore -> connected.
	if err := os.WriteFile(filepath.Join(tokenstore, "oauth1_token.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := garminTestServer(t, &stubGarminLogin{}, tokenstore)

	rec := do(t, h, http.MethodGet, "/api/garmin/status", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var st struct {
		Connected bool `json:"connected"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if !st.Connected {
		t.Fatalf("connected = false with populated tokenstore")
	}

	rec = doBody(t, h, http.MethodPost, "/api/garmin/disconnect", testToken, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("disconnect = %d", rec.Code)
	}
	entries, _ := os.ReadDir(tokenstore)
	if len(entries) != 0 {
		t.Fatalf("tokenstore not emptied: %d entries", len(entries))
	}
	rec = do(t, h, http.MethodGet, "/api/garmin/status", testToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &st)
	if st.Connected {
		t.Fatalf("connected = true after disconnect")
	}
}
