package progress

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

// Engine reads the store, runs ComputeProgress, and (for Analyze) calls the
// shared llm.Client. It is the concrete impl of the api.Progress seam (M3.1).
// The deterministic computation lives in ComputeProgress (pure); only Report and
// Analyze here read the store / call claude. *Engine satisfies api.Progress.
type Engine struct {
	store *store.Store
	llm   *llm.Client
	model string
}

// New constructs a progress Engine.
func New(s *store.Store, c *llm.Client, model string) *Engine {
	return &Engine{store: s, llm: c, model: model}
}

// Report builds the deterministic ProgressReport over `weeks` weekly buckets
// ending at now (UTC). Reads activities, recovery, vo2max, and the profile, then
// delegates to the pure ComputeProgress.
func (e *Engine) Report(ctx context.Context, weeks int) (ProgressReport, error) {
	acts, err := e.store.ListActivities(500)
	if err != nil {
		return ProgressReport{}, err
	}
	rec, err := e.store.ListRecovery(7 * MaxWeeks) // enough days to cover the deepest window
	if err != nil {
		return ProgressReport{}, err
	}
	vo2, err := e.store.ListVo2max(7 * MaxWeeks)
	if err != nil {
		return ProgressReport{}, err
	}
	prof, err := e.store.GetAthleteProfile()
	if err != nil {
		return ProgressReport{}, err
	}
	return ComputeProgress(acts, rec, vo2, prof, weeks, time.Now().UTC()), nil
}

// analyzeArgs builds the claude -p argv for the progress read (mirrors
// coach.dailyAdjustArgs).
func (e *Engine) analyzeArgs() []string {
	return []string{
		"-p", progressReadPrompt,
		"--model", e.model,
		"--output-format", "json",
		"--allowedTools", "",
		"--no-session-persistence",
	}
}

// Analyze runs the claude -p progress read over the computed trends. It mirrors
// coach.AdjustToday exactly: setup failures (Report/profile) return an error;
// every llm.Call failure (always *llm.CallError or llm.ErrMalformedJSON) -> log +
// deterministic templated fallback with Source:"fallback"; success -> Source:"ai".
func (e *Engine) Analyze(ctx context.Context, weeks int) (ProgressRead, error) {
	rep, err := e.Report(ctx, weeks)
	if err != nil {
		return ProgressRead{}, err
	}
	prof, err := e.store.GetAthleteProfile()
	if err != nil {
		return ProgressRead{}, err
	}
	in := ProgressReadInput{Weeks: rep.Weeks, Signals: rep.Signals, GoalText: prof.GoalText}
	inputJSON, err := json.Marshal(in)
	if err != nil {
		return ProgressRead{}, err
	}

	var parsed struct {
		Text string `json:"text"`
	}
	if cerr := e.llm.Call(ctx, e.analyzeArgs(), string(inputJSON), &parsed); cerr != nil {
		log.Printf("progress.Analyze: claude failed (%v); using deterministic fallback", cerr)
		return ProgressRead{Text: fallbackProgressText(rep), Source: "fallback"}, nil
	}
	return ProgressRead{Text: parsed.Text, Source: "ai"}, nil
}
