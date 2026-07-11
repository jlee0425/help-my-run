package demo

import (
	"context"
	"os"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
)

// newDemoClient wires the DemoRunner behind the real llm.Client so tests
// exercise the exact ParseEnvelope + ExtractJSON path the engines use.
func newDemoClient() *llm.Client {
	return &llm.Client{Runner: Runner{}}
}

func TestRunner_DailyAdjust(t *testing.T) {
	c := newDemoClient()
	args := []string{"-p", markerDailyAdjust}
	stdin := `{"date":"2026-07-11","today_session":{"date":"2026-07-11","dow":"Sat","run_type":"tempo","distance_km":8,"pace_target":"5:15/km","time_note":"","optional_if_cns":false,"rationale":""}}`
	var dec llm.DailyDecisionParsed
	if err := c.Call(context.Background(), args, stdin, &dec); err != nil {
		t.Fatalf("Call error = %v", err)
	}
	if dec.Action != llm.ActionSoften {
		t.Errorf("action = %q, want SOFTEN", dec.Action)
	}
	if dec.AdjustedSession == nil {
		t.Fatalf("adjusted_session is nil, want a softened easy session")
	}
	if dec.AdjustedSession.RunType != "easy" {
		t.Errorf("adjusted run_type = %q, want easy", dec.AdjustedSession.RunType)
	}
	if dec.AdjustedSession.Date != "2026-07-11" {
		t.Errorf("adjusted date = %q, want 2026-07-11", dec.AdjustedSession.Date)
	}
	if !strings.Contains(dec.Rationale, SampleLabel) {
		t.Errorf("rationale missing SampleLabel: %q", dec.Rationale)
	}
}

func TestRunner_Plan(t *testing.T) {
	c := newDemoClient()
	args := []string{"-p", markerPlan}
	stdin := `{"week_start":"2026-07-06"}`
	var plan llm.PlanParsed
	if err := c.Call(context.Background(), args, stdin, &plan); err != nil {
		t.Fatalf("Call error = %v", err)
	}
	if len(plan.Days) != 7 {
		t.Fatalf("plan days = %d, want 7", len(plan.Days))
	}
	if plan.Days[0].Date != "2026-07-06" {
		t.Errorf("day[0] date = %q, want 2026-07-06 (Monday)", plan.Days[0].Date)
	}
	if plan.WeeklyTargetKm <= 0 {
		t.Errorf("weekly_target_km = %v, want > 0", plan.WeeklyTargetKm)
	}
	if !strings.Contains(plan.WeekRationale, SampleLabel) {
		t.Errorf("week_rationale missing SampleLabel: %q", plan.WeekRationale)
	}
}

func TestRunner_CrossFit(t *testing.T) {
	c := newDemoClient()
	prompt := "You are parsing a CrossFit box's weekly programming photo.\nThe week starts on Monday 2026-07-06."
	args := []string{"-p", prompt}
	var wk llm.CrossFitWeekParsed
	if err := c.Call(context.Background(), args, "", &wk); err != nil {
		t.Fatalf("Call error = %v", err)
	}
	if wk.WeekStart != "2026-07-06" {
		t.Errorf("week_start = %q, want 2026-07-06", wk.WeekStart)
	}
	if len(wk.Days) != 7 {
		t.Fatalf("crossfit days = %d, want 7", len(wk.Days))
	}
	if wk.Days[0].Date != "2026-07-06" {
		t.Errorf("day[0] date = %q, want 2026-07-06", wk.Days[0].Date)
	}
	for i, d := range wk.Days {
		switch d.CNSLoad {
		case llm.LoadLow, llm.LoadMed, llm.LoadHigh:
		default:
			t.Errorf("day[%d] cns_load = %q, want low|med|high", i, d.CNSLoad)
		}
	}
}

func TestRunner_ChatRotates(t *testing.T) {
	c := newDemoClient()
	args := []string{"-p", markerChat}
	stdin := `{"pack":{},"history":[],"message":"How is my base?"}`
	var a, b struct {
		Text string `json:"text"`
	}
	if err := c.Call(context.Background(), args, stdin, &a); err != nil {
		t.Fatalf("Call #1 error = %v", err)
	}
	if err := c.Call(context.Background(), args, stdin, &b); err != nil {
		t.Fatalf("Call #2 error = %v", err)
	}
	if a.Text == "" || b.Text == "" {
		t.Fatalf("empty chat text")
	}
	if a.Text == b.Text {
		t.Errorf("consecutive chat answers did not rotate")
	}
	if !strings.Contains(a.Text, SampleLabel) || !strings.Contains(b.Text, SampleLabel) {
		t.Errorf("chat answers missing SampleLabel")
	}
}

func TestRunner_Progress(t *testing.T) {
	c := newDemoClient()
	args := []string{"-p", markerProgress}
	stdin := `{"weeks":12,"signals":[],"goal_text":"base"}`
	var parsed struct {
		Text string `json:"text"`
	}
	if err := c.Call(context.Background(), args, stdin, &parsed); err != nil {
		t.Fatalf("Call error = %v", err)
	}
	if parsed.Text == "" {
		t.Fatalf("empty progress text")
	}
	if !strings.Contains(parsed.Text, SampleLabel) {
		t.Errorf("progress text missing SampleLabel: %q", parsed.Text)
	}
}

func TestRunner_UnknownPromptIsValidEnvelope(t *testing.T) {
	out, err := Runner{}.Run(context.Background(), []string{"-p", "something entirely unrelated"}, "")
	if err != nil {
		t.Fatalf("Run error = %v (unknown prompt must never error)", err)
	}
	env, err := llm.ParseEnvelope(out)
	if err != nil {
		t.Fatalf("ParseEnvelope error = %v", err)
	}
	if env.IsError {
		t.Errorf("envelope is_error = true, want false")
	}
	if !strings.Contains(env.Result, SampleLabel) {
		t.Errorf("unknown-route result missing SampleLabel: %q", env.Result)
	}
}

// TestRoutingMarkersPinnedToSource pins the routing substrings to the real
// engine prompt constants: if a prompt is reworded without updating the marker,
// this fails instead of silently breaking demo routing at runtime.
func TestRoutingMarkersPinnedToSource(t *testing.T) {
	cases := []struct {
		file    string
		markers []string
	}{
		{"../coach/prompts.go", []string{markerDailyAdjust, markerPlan, markerCrossFit}},
		{"../chat/prompts.go", []string{markerChat}},
		{"../progress/prompts.go", []string{markerProgress}},
	}
	for _, tc := range cases {
		b, err := os.ReadFile(tc.file)
		if err != nil {
			t.Fatalf("read %s: %v", tc.file, err)
		}
		src := string(b)
		for _, m := range tc.markers {
			if !strings.Contains(src, m) {
				t.Errorf("%s no longer contains routing marker %q — update the marker or the demo route", tc.file, m)
			}
		}
	}
}
