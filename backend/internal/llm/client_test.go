package llm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return b
}

// envelopeResult unwraps a fixture file's .result via the Envelope type so the
// ExtractJSON tests operate on what the model actually emitted.
func envelopeResult(t *testing.T, name string) string {
	t.Helper()
	env, err := ParseEnvelope(loadFixture(t, name))
	if err != nil {
		t.Fatalf("ParseEnvelope(%s): %v", name, err)
	}
	return env.Result
}

func TestParseEnvelopeFlags(t *testing.T) {
	env, err := ParseEnvelope(loadFixture(t, "not_logged_in_envelope.json"))
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if env.Subtype != "success" {
		t.Errorf("subtype = %q, want success (the trap case)", env.Subtype)
	}
	if !env.IsError {
		t.Error("IsError = false, want true (branch on IsError not subtype)")
	}
}

func TestExtractJSONFencedBlock(t *testing.T) {
	type week struct {
		WeekStart string `json:"week_start"`
		Days      []struct {
			Dow     string `json:"dow"`
			CNSLoad string `json:"cns_load"`
		} `json:"days"`
	}
	var w week
	if err := ExtractJSON(envelopeResult(t, "stage1_envelope.json"), &w); err != nil {
		t.Fatalf("ExtractJSON(fenced) error = %v", err)
	}
	if w.WeekStart != "2026-06-22" || len(w.Days) != 1 || w.Days[0].Dow != "Mon" || w.Days[0].CNSLoad != "high" {
		t.Errorf("parsed = %+v, want week 2026-06-22 Mon high", w)
	}
}

func TestExtractJSONBareWithProse(t *testing.T) {
	type plan struct {
		WeeklyTargetKm float64 `json:"weekly_target_km"`
		OneFlag        string  `json:"one_flag"`
	}
	var p plan
	if err := ExtractJSON(envelopeResult(t, "stage2_envelope.json"), &p); err != nil {
		t.Fatalf("ExtractJSON(prose+json) error = %v", err)
	}
	if p.WeeklyTargetKm != 20 || p.OneFlag != "Watch Thursday." {
		t.Errorf("parsed = %+v, want target 20 flag 'Watch Thursday.'", p)
	}
}

func TestExtractJSONMalformed(t *testing.T) {
	var v map[string]any
	err := ExtractJSON(envelopeResult(t, "malformed_envelope.json"), &v)
	if err == nil {
		t.Fatal("ExtractJSON(malformed) error = nil, want error")
	}
}

func TestClassifyFailureNotLoggedIn(t *testing.T) {
	env, _ := ParseEnvelope(loadFixture(t, "not_logged_in_envelope.json"))
	msg := ClassifyFailure(env, nil)
	if msg == "" {
		t.Fatal("ClassifyFailure returned empty")
	}
	if !contains(msg, "logged in") {
		t.Errorf("classify = %q, want a 'logged in' hint", msg)
	}
}

func TestClassifyFailureBinaryNotFound(t *testing.T) {
	msg := ClassifyFailure(Envelope{}, errExecNotFound)
	if !contains(msg, "not installed") {
		t.Errorf("classify = %q, want a 'not installed' hint", msg)
	}
}

func TestClassifyFailureAuthKeywords(t *testing.T) {
	// Each of these .result texts should map to the not-logged-in message.
	for _, kw := range []string{
		"Please login first", "You are not logged in", "failed to authenticate",
		"401 Unauthorized", "no valid credential found",
	} {
		msg := ClassifyFailure(Envelope{Result: kw}, nil)
		if !contains(msg, "logged in") {
			t.Errorf("classify(%q) = %q, want a 'logged in' hint", kw, msg)
		}
	}
}

func TestClassifyFailureUsageLimitKeywords(t *testing.T) {
	// Each of these .result texts should map to the rate/usage-limit message.
	for _, kw := range []string{
		"usage limit reached", "rate limited", "out of credit", "monthly quota exceeded",
	} {
		msg := ClassifyFailure(Envelope{Result: kw}, nil)
		if !contains(msg, "limit") {
			t.Errorf("classify(%q) = %q, want a 'limit' hint", kw, msg)
		}
	}
}

func TestExecRunnerStubViaSh(t *testing.T) {
	// The ExecRunner shells out; stub it with /bin/sh printing a fixture.
	fixture, _ := filepath.Abs(filepath.Join("testdata", "stage1_envelope.json"))
	r := ExecRunner{Bin: "/bin/sh"}
	out, err := r.Run(context.Background(),
		[]string{"-c", "cat '" + fixture + "'"}, "")
	if err != nil {
		t.Fatalf("ExecRunner.Run error = %v", err)
	}
	env, err := ParseEnvelope(out)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if env.Subtype != "success" {
		t.Errorf("subtype = %q, want success", env.Subtype)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

var _ = errors.Is
