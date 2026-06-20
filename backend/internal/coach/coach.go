// Package coach assembles the context pack and orchestrates the two claude -p
// stages (image parse + plan generation) on top of store + metrics + llm.
package coach

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/readiness"
	"help-my-run/backend/internal/store"
)

// Coach wires the store, the llm client, the model name, and the image dir.
type Coach struct {
	store    *store.Store
	llm      *llm.Client
	model    string
	imageDir string
}

// New constructs a Coach.
func New(s *store.Store, c *llm.Client, model, imageDir string) *Coach {
	return &Coach{store: s, llm: c, model: model, imageDir: imageDir}
}

// ProfilePack is the profile slice of the context pack.
type ProfilePack struct {
	TargetWeeklyKm  float64         `json:"target_weekly_km"`
	ProgressionMode string          `json:"progression_mode"`
	Zone2CeilingBpm *int64          `json:"zone2_ceiling_bpm"`
	ThresholdBpm    *int64          `json:"threshold_bpm"`
	MaxHRBpm        *int64          `json:"max_hr_bpm"`
	RunConstraints  json.RawMessage `json:"run_constraints"`
	GoalText        string          `json:"goal_text"`
}

// ContextPack is the Stage-2 input (piped to stdin; stored for reproducibility).
type ContextPack struct {
	GeneratedAt  string                 `json:"generated_at"`
	WeekStart    string                 `json:"week_start"`
	Metrics      metrics.FitnessMetrics `json:"metrics"`
	Profile      ProfilePack            `json:"profile"`
	CrossFitWeek llm.CrossFitWeekParsed `json:"crossfit_week"`
	LastWeekPlan *llm.PlanParsed        `json:"last_week_plan"`
}

// DailyAdjustInput is the JSON piped to claude -p stdin for the daily adjust
// (M2 Coach Brain). snake_case wire JSON.
type DailyAdjustInput struct {
	Date          string                 `json:"date"`
	Readiness     readiness.Readiness    `json:"readiness"`
	TodaySession  *llm.PlanDay           `json:"today_session"`
	Metrics       metrics.FitnessMetrics `json:"metrics"`
	Profile       ProfilePack            `json:"profile"`
	CrossFitToday *llm.CrossFitDay       `json:"crossfit_today"`
	WeekRationale string                 `json:"week_rationale"`
}

// stage1Args builds the claude -p argv for Stage 1 (image → CrossFit week).
func (c *Coach) stage1Args(prompt string) []string {
	return []string{
		"-p", prompt,
		"--model", c.model,
		"--output-format", "json",
		"--allowedTools", "Read",
		"--add-dir", c.imageDir,
		"--no-session-persistence",
	}
}

// stage2Args builds the claude -p argv for Stage 2 (context pack → plan).
func (c *Coach) stage2Args() []string {
	return []string{
		"-p", coachBrainPrompt,
		"--model", c.model,
		"--output-format", "json",
		"--allowedTools", "",
		"--no-session-persistence",
	}
}

// ParseCrossFit runs Stage 1: reads the saved image, returns the parsed week and
// its canonical JSON re-marshaling (stored as crossfit_weeks.raw_response — NOT the
// byte-for-byte claude -p .result, which Call does not surface). Storage is the
// handler's responsibility.
func (c *Coach) ParseCrossFit(ctx context.Context, weekStart, imagePath string) (llm.CrossFitWeekParsed, string, error) {
	prompt := buildStage1Prompt(imagePath, weekStart)
	args := c.stage1Args(prompt)

	var week llm.CrossFitWeekParsed
	if err := c.llm.Call(ctx, args, "", &week); err != nil {
		return llm.CrossFitWeekParsed{}, "", err
	}
	raw, _ := json.Marshal(week)
	return week, string(raw), nil
}

// GeneratePlan runs Stage 2: builds the context pack (using edited if supplied,
// else the stored week), pipes it on stdin, returns the plan, the serialized
// context pack, and the model.
func (c *Coach) GeneratePlan(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (llm.PlanParsed, string, string, error) {
	pack, err := c.buildContextPack(ctx, weekStart, edited)
	if err != nil {
		return llm.PlanParsed{}, "", "", err
	}
	packJSON, err := json.Marshal(pack)
	if err != nil {
		return llm.PlanParsed{}, "", "", err
	}

	var plan llm.PlanParsed
	if err := c.llm.Call(ctx, c.stage2Args(), string(packJSON), &plan); err != nil {
		return llm.PlanParsed{}, "", "", err
	}
	return plan, string(packJSON), c.model, nil
}

// Fitness computes the current fitness read from the local store.
func (c *Coach) Fitness(ctx context.Context) (metrics.FitnessMetrics, error) {
	acts, err := c.store.ListActivities(200)
	if err != nil {
		return metrics.FitnessMetrics{}, err
	}
	rec, err := c.store.ListRecovery(60)
	if err != nil {
		return metrics.FitnessMetrics{}, err
	}
	prof, err := c.store.GetAthleteProfile()
	if err != nil {
		return metrics.FitnessMetrics{}, err
	}
	return metrics.ComputeFitness(acts, rec, prof, time.Now().UTC()), nil
}

// buildContextPack assembles metrics + profile + crossfit week + last plan.
func (c *Coach) buildContextPack(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (ContextPack, error) {
	prof, err := c.store.GetAthleteProfile()
	if err != nil {
		return ContextPack{}, err
	}
	fit, err := c.Fitness(ctx)
	if err != nil {
		return ContextPack{}, err
	}

	// CrossFit week: edited overrides stored.
	var week llm.CrossFitWeekParsed
	if edited != nil {
		week = *edited
	} else {
		stored, gerr := c.store.GetCrossFitWeek(weekStart)
		if gerr != nil {
			return ContextPack{}, gerr
		}
		if uerr := json.Unmarshal([]byte(stored.ParsedJSON), &week); uerr != nil {
			return ContextPack{}, uerr
		}
	}

	// Last week's plan (best-effort; nil if none).
	var last *llm.PlanParsed
	prevMonday, perr := time.Parse("2006-01-02", weekStart)
	if perr == nil {
		prev := prevMonday.AddDate(0, 0, -7).Format("2006-01-02")
		if lp, lerr := c.store.GetLatestPlan(prev); lerr == nil {
			var pp llm.PlanParsed
			if json.Unmarshal([]byte(lp.PlanJSON), &pp) == nil {
				last = &pp
			}
		}
	}

	rc := json.RawMessage(prof.RunConstraintsJSON)
	if len(rc) == 0 || !json.Valid(rc) {
		rc = json.RawMessage(`{}`)
	}
	return ContextPack{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		WeekStart:   weekStart,
		Metrics:     fit,
		Profile: ProfilePack{
			TargetWeeklyKm:  prof.TargetWeeklyKm,
			ProgressionMode: prof.ProgressionMode,
			Zone2CeilingBpm: prof.Zone2CeilingBpm,
			ThresholdBpm:    prof.ThresholdBpm,
			MaxHRBpm:        prof.MaxHRBpm,
			RunConstraints:  rc,
			GoalText:        prof.GoalText,
		},
		CrossFitWeek: week,
		LastWeekPlan: last,
	}, nil
}

// dailyAdjustArgs builds the claude -p argv for the daily-adjust (Coach Brain) call.
func (c *Coach) dailyAdjustArgs() []string {
	return []string{
		"-p", dailyAdjustPrompt,
		"--model", c.model,
		"--output-format", "json",
		"--allowedTools", "",
		"--no-session-persistence",
	}
}

// AdjustToday runs the daily-adjust claude -p call. By the llm.Call error
// contract, Call returns ONLY *llm.CallError (classified model/process failure) or
// llm.ErrMalformedJSON (unparseable output, post-retry) — i.e. every non-nil error
// here is a model failure, so any such error triggers the deterministic fallback
// rule (§2 table via readiness.Fallback). Errors from the pre-call setup (Fitness,
// GetAthleteProfile, input marshal) are NOT model failures and are returned to the
// caller. Returns the parsed decision, the raw re-marshaled JSON (empty on
// fallback), the source ("ai"|"fallback"), and an error only for those setup issues.
func (c *Coach) AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error) {
	fit, err := c.Fitness(ctx)
	if err != nil {
		return llm.DailyDecisionParsed{}, "", "", err
	}
	prof, err := c.store.GetAthleteProfile()
	if err != nil {
		return llm.DailyDecisionParsed{}, "", "", err
	}
	rc := json.RawMessage(prof.RunConstraintsJSON)
	if len(rc) == 0 || !json.Valid(rc) {
		rc = json.RawMessage(`{}`)
	}
	pp := ProfilePack{
		TargetWeeklyKm:  prof.TargetWeeklyKm,
		ProgressionMode: prof.ProgressionMode,
		Zone2CeilingBpm: prof.Zone2CeilingBpm,
		ThresholdBpm:    prof.ThresholdBpm,
		MaxHRBpm:        prof.MaxHRBpm,
		RunConstraints:  rc,
		GoalText:        prof.GoalText,
	}

	in := buildDailyAdjustInput(date, rd, today, fit, pp, nil, "")
	inputJSON, err := json.Marshal(in)
	if err != nil {
		return llm.DailyDecisionParsed{}, "", "", err
	}

	var decision llm.DailyDecisionParsed
	// llm.Call only returns *llm.CallError or llm.ErrMalformedJSON, both of which are
	// model failures, so any non-nil error here -> deterministic fallback.
	if cerr := c.llm.Call(ctx, c.dailyAdjustArgs(), string(inputJSON), &decision); cerr != nil {
		log.Printf("coach.AdjustToday: claude failed (%v); using deterministic fallback", cerr)
		fb := fallbackDecision(date, rd, today, fit)
		return fb, "", "fallback", nil
	}
	raw, _ := json.Marshal(decision)
	return decision, string(raw), "ai", nil
}

// planDayToFallbackSession converts a *llm.PlanDay into the readiness package's
// llm-free FallbackSession (nil today -> nil session, i.e. no run). `date` forces
// the local date onto the session (the coach always stamps today's date).
func planDayToFallbackSession(date string, today *llm.PlanDay) *readiness.FallbackSession {
	if today == nil {
		return nil
	}
	return &readiness.FallbackSession{
		Date:          date,
		Dow:           today.Dow,
		RunType:       today.RunType,
		DistanceKm:    today.DistanceKm,
		PaceTarget:    today.PaceTarget,
		TimeNote:      today.TimeNote,
		OptionalIfCNS: today.OptionalIfCNS,
		Rationale:     today.Rationale,
	}
}

// fallbackSessionToPlanDay converts a readiness.FallbackSession back into a
// *llm.PlanDay (nil -> nil for REST_DAY).
func fallbackSessionToPlanDay(fs *readiness.FallbackSession) *llm.PlanDay {
	if fs == nil {
		return nil
	}
	return &llm.PlanDay{
		Date:          fs.Date,
		Dow:           fs.Dow,
		RunType:       fs.RunType,
		DistanceKm:    fs.DistanceKm,
		PaceTarget:    fs.PaceTarget,
		TimeNote:      fs.TimeNote,
		OptionalIfCNS: fs.OptionalIfCNS,
		Rationale:     fs.Rationale,
	}
}

// fallbackDecision is the deterministic readiness->action rule applied when the
// claude -p daily-adjust call fails. It DELEGATES to readiness.Fallback (the
// single source of truth for the §2 rule table, roundHalf capping, and rationale
// strings) so the shipped fallback path is exactly what the Task 8/9 readiness
// tests cover. No rule logic lives here — only the *llm.PlanDay <-> FallbackSession
// conversion.
func fallbackDecision(date string, rd readiness.Readiness, today *llm.PlanDay, fit metrics.FitnessMetrics) llm.DailyDecisionParsed {
	dec := readiness.Fallback(rd.Color, planDayToFallbackSession(date, today), fit.EasyPace)
	return llm.DailyDecisionParsed{
		Action:          llm.DailyAction(dec.Action),
		AdjustedSession: fallbackSessionToPlanDay(dec.Adjusted),
		Rationale:       dec.Rationale,
	}
}
