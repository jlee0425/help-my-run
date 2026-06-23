package coach

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

func newCoachStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "coach.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// captureRunner records the args + stdin and returns a canned envelope.
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

const stage1Env = `{"type":"result","subtype":"success","is_error":false,"result":"{\"week_start\":\"2026-06-22\",\"days\":[{\"date\":\"2026-06-22\",\"dow\":\"Mon\",\"has_crossfit\":true,\"focus\":\"Back squat\",\"cns_load\":\"high\",\"leg_load\":\"high\",\"notes\":\"\"}]}"}`

const stage2Env = `{"type":"result","subtype":"success","is_error":false,"result":"{\"fitness_summary\":\"ok\",\"weekly_target_km\":20,\"days\":[{\"date\":\"2026-06-22\",\"dow\":\"Mon\",\"run_type\":\"rest\",\"distance_km\":0,\"pace_target\":\"\",\"time_note\":\"\",\"optional_if_cns\":false,\"rationale\":\"rest\"}],\"week_rationale\":\"para\",\"one_flag\":\"flag\"}"}`

func TestParseCrossFitBuildsStage1AndParses(t *testing.T) {
	s := newCoachStore(t)
	r := &captureRunner{out: []byte(stage1Env)}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	week, raw, err := c.ParseCrossFit(context.Background(), "2026-06-22", "/tmp/cfimg/2026-06-22.jpg")
	if err != nil {
		t.Fatalf("ParseCrossFit error = %v", err)
	}
	if week.WeekStart != "2026-06-22" || len(week.Days) != 1 || week.Days[0].CNSLoad != llm.LoadHigh {
		t.Errorf("parsed week = %+v", week)
	}
	if raw == "" {
		t.Error("raw response empty, want canonical re-marshaled week JSON")
	}
	joined := strings.Join(r.args, " ")
	if !strings.Contains(joined, "/tmp/cfimg/2026-06-22.jpg") {
		t.Errorf("args missing image path: %v", r.args)
	}
	if !hasPair(r.args, "--output-format", "json") || !hasPair(r.args, "--model", "claude-opus-4-8") {
		t.Errorf("args missing model/output-format: %v", r.args)
	}
	if !hasFlag(r.args, "--add-dir") || !hasFlag(r.args, "-p") {
		t.Errorf("args missing --add-dir/-p: %v", r.args)
	}
}

func TestParseCrossFitMalformedThenValidRetriesOnce(t *testing.T) {
	s := newCoachStore(t)
	r := &seqCoachRunner{outs: [][]byte{
		[]byte(`{"type":"result","is_error":false,"result":"not json"}`),
		[]byte(stage1Env),
	}}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")
	week, _, err := c.ParseCrossFit(context.Background(), "2026-06-22", "/tmp/cfimg/2026-06-22.jpg")
	if err != nil {
		t.Fatalf("ParseCrossFit error = %v", err)
	}
	if r.calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry)", r.calls)
	}
	if week.WeekStart != "2026-06-22" {
		t.Errorf("parsed week wrong after retry: %+v", week)
	}
}

type seqCoachRunner struct {
	outs  [][]byte
	calls int
}

func (r *seqCoachRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	i := r.calls
	r.calls++
	if i < len(r.outs) {
		return r.outs[i], nil
	}
	return r.outs[len(r.outs)-1], nil
}

func TestGeneratePlanPipesContextPackAndParses(t *testing.T) {
	s := newCoachStore(t)
	_ = s.UpsertCrossFitWeek(store.CrossFitWeek{
		WeekStart:  "2026-06-22",
		ParsedJSON: `{"week_start":"2026-06-22","days":[{"date":"2026-06-22","dow":"Mon","has_crossfit":true,"focus":"x","cns_load":"high","leg_load":"high","notes":""}]}`,
	})
	r := &captureRunner{out: []byte(stage2Env)}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	plan, ctxPack, model, err := c.GeneratePlan(context.Background(), "2026-06-22", nil)
	if err != nil {
		t.Fatalf("GeneratePlan error = %v", err)
	}
	if plan.WeeklyTargetKm != 20 || plan.OneFlag != "flag" || len(plan.Days) != 1 {
		t.Errorf("plan = %+v", plan)
	}
	if model != "claude-opus-4-8" {
		t.Errorf("model = %q, want claude-opus-4-8", model)
	}
	if r.body == "" {
		t.Fatal("stdin empty, want context pack piped")
	}
	if !strings.Contains(r.body, `"crossfit_week"`) || !strings.Contains(r.body, `"metrics"`) {
		t.Errorf("piped context pack missing fields: %s", r.body)
	}
	if ctxPack != r.body {
		t.Errorf("returned context pack != piped stdin")
	}
	joined := strings.Join(r.args, " ")
	if !strings.Contains(joined, "running coach") {
		t.Errorf("Stage-2 args missing coach brain: %v", r.args)
	}
	if hasFlag(r.args, "--add-dir") {
		t.Errorf("Stage-2 must not pass --add-dir: %v", r.args)
	}
}

func TestGeneratePlanUsesEditedWeekOverStored(t *testing.T) {
	s := newCoachStore(t)
	_ = s.UpsertCrossFitWeek(store.CrossFitWeek{
		WeekStart:  "2026-06-22",
		ParsedJSON: `{"week_start":"2026-06-22","days":[]}`,
	})
	r := &captureRunner{out: []byte(stage2Env)}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	edited := &llm.CrossFitWeekParsed{
		WeekStart: "2026-06-22",
		Days:      []llm.CrossFitDay{{Date: "2026-06-22", Dow: "Mon", HasCrossFit: false, Focus: "EDITED", CNSLoad: "low", LegLoad: "low"}},
	}
	if _, _, _, err := c.GeneratePlan(context.Background(), "2026-06-22", edited); err != nil {
		t.Fatalf("GeneratePlan(edited) error = %v", err)
	}
	if !strings.Contains(r.body, "EDITED") {
		t.Errorf("context pack did not use edited week: %s", r.body)
	}
}

func TestFitnessComputesFromStore(t *testing.T) {
	s := newCoachStore(t)
	_ = s.UpsertActivity(store.Activity{
		ActivityID: 1, Type: "Run", Name: "r", StartTime: "2026-06-19T06:00:00Z",
		DistanceM: 6000, MovingTimeS: 2160, ElapsedTimeS: 2160, RawJSON: "{}",
	})
	c := New(s, &llm.Client{Runner: &captureRunner{}, Model: "m"}, "m", "/tmp/cfimg")
	m, err := c.Fitness(context.Background())
	if err != nil {
		t.Fatalf("Fitness error = %v", err)
	}
	if m.SafeWeeklyTargetKm <= 0 {
		t.Errorf("SafeWeeklyTargetKm = %v, want > 0 (profile target fallback)", m.SafeWeeklyTargetKm)
	}
}

func hasFlag(args []string, f string) bool {
	for _, a := range args {
		if a == f {
			return true
		}
	}
	return false
}

func hasPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}
