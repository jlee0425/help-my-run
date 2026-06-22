package progress

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

func newProgressStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "progress.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

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

// failRunner returns an is_error envelope (fallback trigger).
type failRunner struct{ calls int }

func (r *failRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	r.calls++
	return []byte(`{"type":"result","is_error":true,"result":"please login"}`), nil
}

const readEnv = `{"type":"result","subtype":"success","is_error":false,"result":"{\"text\":\"Engine improving: pace at Z2 dropped.\"}"}`

func fp(v float64) *float64 { return &v }

// seedTrendData inserts enough activities + vo2max so EnoughData is true.
func seedTrendData(t *testing.T, s *store.Store) {
	t.Helper()
	_ = s.UpsertActivity(store.Activity{
		StravaID: 1, Name: "easy", Type: "Run",
		StartTime: "2026-06-15T06:00:00Z", DistanceM: 10000, MovingTimeS: 3300,
		AvgHR: fp(145), RawJSON: "{}",
	})
	_ = s.UpsertActivity(store.Activity{
		StravaID: 2, Name: "easy2", Type: "Run",
		StartTime: "2026-04-06T06:00:00Z", DistanceM: 10000, MovingTimeS: 3500,
		AvgHR: fp(145), RawJSON: "{}",
	})
	_ = s.UpsertVo2max(store.Vo2maxRow{Date: "2026-06-15", Vo2max: fp(52), RawJSON: "{}"})
	_ = s.UpsertVo2max(store.Vo2maxRow{Date: "2026-04-06", Vo2max: fp(50), RawJSON: "{}"})
}

func TestReportBuildsFromStore(t *testing.T) {
	s := newProgressStore(t)
	seedTrendData(t, s)
	e := New(s, &llm.Client{Runner: &captureRunner{}, Model: "m"}, "m")

	rep, err := e.Report(context.Background(), 12)
	if err != nil {
		t.Fatalf("Report error = %v", err)
	}
	if rep.Weeks != 12 {
		t.Errorf("weeks = %d, want 12", rep.Weeks)
	}
	if len(rep.Signals) == 0 {
		t.Error("signals empty, want >=1 signal card")
	}
}

func TestAnalyzeAIPath(t *testing.T) {
	s := newProgressStore(t)
	seedTrendData(t, s)
	r := &captureRunner{out: []byte(readEnv)}
	e := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8")

	read, err := e.Analyze(context.Background(), 12)
	if err != nil {
		t.Fatalf("Analyze error = %v", err)
	}
	if read.Source != "ai" {
		t.Errorf("source = %q, want ai", read.Source)
	}
	if read.Text != "Engine improving: pace at Z2 dropped." {
		t.Errorf("text = %q", read.Text)
	}
	// stdin carries the computed signals + window.
	if !strings.Contains(r.body, `"weeks"`) || !strings.Contains(r.body, `"signals"`) {
		t.Errorf("stdin missing weeks/signals: %s", r.body)
	}
	// argv carries the read prompt + flags.
	joined := strings.Join(r.args, " ")
	if !strings.Contains(joined, "progress read") {
		t.Errorf("args missing progress-read prompt: %v", r.args)
	}
	if !hasPair(r.args, "--output-format", "json") || !hasPair(r.args, "--model", "claude-opus-4-8") {
		t.Errorf("args missing model/output-format: %v", r.args)
	}
}

func TestAnalyzeFallbackOnFailure(t *testing.T) {
	s := newProgressStore(t)
	seedTrendData(t, s)
	r := &failRunner{}
	e := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8")

	read, err := e.Analyze(context.Background(), 12)
	if err != nil {
		t.Fatalf("Analyze fallback returned error = %v, want nil", err)
	}
	if read.Source != "fallback" {
		t.Errorf("source = %q, want fallback", read.Source)
	}
	if read.Text == "" {
		t.Error("fallback text empty, want templated summary")
	}
}

func TestAnalyzeFallbackNotEnoughData(t *testing.T) {
	s := newProgressStore(t) // empty store -> EnoughData false
	r := &failRunner{}
	e := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8")

	read, err := e.Analyze(context.Background(), 12)
	if err != nil {
		t.Fatalf("Analyze error = %v", err)
	}
	if read.Source != "fallback" {
		t.Errorf("source = %q, want fallback", read.Source)
	}
	if !strings.Contains(read.Text, "Not enough history") {
		t.Errorf("text = %q, want not-enough-history message", read.Text)
	}
}

func TestActivityLimitSizesToWindow(t *testing.T) {
	// Long window: cap is window-derived (weeks*14) so the deepest weeks are not
	// truncated for a high-volume athlete (runs + CrossFit).
	if got := activityLimit(MaxWeeks); got != MaxWeeks*14 {
		t.Errorf("activityLimit(%d) = %d, want %d", MaxWeeks, got, MaxWeeks*14)
	}
	if got := activityLimit(20); got != 20*14 {
		t.Errorf("activityLimit(20) = %d, want %d", got, 20*14)
	}
	// Short window: floor keeps the cap >= 200 (matching the codebase convention)
	// so short windows still pull enough baseline points.
	if got := activityLimit(MinWeeks); got != 200 {
		t.Errorf("activityLimit(%d) = %d, want 200 (floor)", MinWeeks, got)
	}
	if got := activityLimit(DefaultWeeks); got != 200 {
		t.Errorf("activityLimit(%d) = %d, want 200 (floor, 12*14=168 < 200)", DefaultWeeks, got)
	}
}

func TestReportIncludesDecouplingSignal(t *testing.T) {
	s := newProgressStore(t)
	seedTrendData(t, s) // activities 1 & 2 exist (2026-06-15, 2026-04-06)
	dp1, dp2 := fp(4.0), fp(7.0)
	if err := s.UpsertStreamAnalysis(store.StreamAnalysisRow{
		ActivityID: 1, TimeInZoneJSON: "[]", DecouplingPct: dp1,
		ZonesJSON: `{"z1_hi":116,"z2_hi":145,"z3_hi":157.5,"z4_hi":170}`,
		HasHR:     true, ComputedAt: "2026-06-15T07:00:00Z",
	}); err != nil {
		t.Fatalf("upsert analysis 1: %v", err)
	}
	if err := s.UpsertStreamAnalysis(store.StreamAnalysisRow{
		ActivityID: 2, TimeInZoneJSON: "[]", DecouplingPct: dp2,
		ZonesJSON: `{"z1_hi":116,"z2_hi":145,"z3_hi":157.5,"z4_hi":170}`,
		HasHR:     true, ComputedAt: "2026-04-06T07:00:00Z",
	}); err != nil {
		t.Fatalf("upsert analysis 2: %v", err)
	}
	e := New(s, &llm.Client{Runner: &captureRunner{}, Model: "m"}, "m")

	rep, err := e.Report(context.Background(), 12)
	if err != nil {
		t.Fatalf("Report error = %v", err)
	}
	var dec *TrendSummary
	for i := range rep.Signals {
		if rep.Signals[i].Key == SignalDecoupling {
			dec = &rep.Signals[i]
		}
	}
	if dec == nil {
		t.Fatal("decoupling signal absent from report")
	}
	if dec.Unit != "%" || !dec.LowerIsBetter {
		t.Errorf("decoupling card = %+v, want unit=%% lowerIsBetter=true", dec)
	}
	if countNonNil(dec.Series) < 2 {
		t.Errorf("decoupling series non-nil = %d, want >=2 (two weeks)", countNonNil(dec.Series))
	}
}

func hasPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}
