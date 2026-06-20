package coach

import (
	"fmt"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/readiness"
)

// stage1Template instructs claude -p to read the saved schedule image and emit
// ONLY the Stage-1 CrossFit-week JSON.
const stage1Template = `You are parsing a CrossFit box's weekly programming photo for a runner who also does CrossFit.

Read the image at this absolute path: %s

Known athlete pattern (hints): CrossFit Monday–Friday ~18:15–19:15; Thursday is a barbell-skill day (lighter legs/CNS); Saturday/Sunday usually rest.

The week starts on Monday %s.

Produce EXACTLY 7 day objects (Mon→Sun). Output ONLY a single JSON object (no prose, no markdown fences) of this shape:
{
  "week_start": "%s",
  "days": [
    {"date":"YYYY-MM-DD","dow":"Mon","has_crossfit":true,"focus":"...","cns_load":"low|med|high","leg_load":"low|med|high","notes":"..."}
  ]
}
Rules: cns_load and leg_load are exactly one of "low","med","high". focus and notes are "" when empty. has_crossfit is false on rest days.`

// buildStage1Prompt fills the Stage-1 template with the image path and week start.
func buildStage1Prompt(imagePath, weekStart string) string {
	return fmt.Sprintf(stage1Template, imagePath, weekStart, weekStart)
}

// coachBrainPrompt is the Stage-2 instruction block prepended to the context
// pack (piped on stdin). It asks for ONLY the Stage-2 plan JSON.
const coachBrainPrompt = `You are a CrossFit-aware running coach. Build a 7-day running plan for the upcoming week from the JSON context pack on stdin (computed fitness metrics, athlete profile + constraints, the parsed CrossFit week, and last week's plan if present).

Coaching rules:
- Periodize toward general aerobic improvement. Ramp weekly volume by no more than ~10% over baseline; include a cutback week when the metrics flag one.
- Place hard/quality runs (tempo, intervals, long) on low-CNS and low-leg CrossFit days and on weekends. Thursday is a barbell-skill day (lighter) — a good quality slot.
- Keep hard runs OFF heavy-leg / high-CNS CrossFit days. Easy stays easy.
- Evening doubles run ~20:00 after CrossFit; set time_note accordingly. Mark a run optional_if_cns:true when it follows a high-CNS day and could be skipped.
- Respect the athlete's run_constraints and weekly target; aim near safe_weekly_target_km.

Output ONLY a single JSON object (no prose, no markdown fences) of this shape:
{
  "fitness_summary": "...",
  "weekly_target_km": 0,
  "days": [
    {"date":"YYYY-MM-DD","dow":"Mon","run_type":"easy|tempo|recovery|long|rest|intervals","distance_km":0,"pace_target":"5:45/km","time_note":"~20:00 after CrossFit","optional_if_cns":false,"rationale":"one line"}
  ],
  "week_rationale": "paragraph on placement + progression",
  "one_flag": "the single most important caution"
}
Produce EXACTLY 7 day objects (Mon→Sun). distance_km is 0 and pace_target/time_note are "" for rest days.`

// dailyAdjustPrompt is the M2 Coach Brain instruction block for the single-day
// adjust. The structured DailyAdjustInput is piped on stdin; the model returns
// ONLY a DailyDecisionParsed JSON object.
const dailyAdjustPrompt = `You are a CrossFit-aware running coach making a SINGLE-DAY adjustment. You receive a JSON context on stdin: today's date, a deterministic readiness assessment (color GREEN/AMBER/RED plus the driver numbers and reasons), today's already-planned running session (or null if no run is scheduled), computed fitness metrics, the athlete profile + constraints, and today's CrossFit day.

Decide ONE action for TODAY ONLY (do not touch other days):
- STAND  — keep today's session exactly as planned.
- SOFTEN — keep the session but reduce volume and/or intensity (lower distance_km, ease pace, or mark optional_if_cns).
- MOVE   — replace a quality session (tempo/intervals/long) with easy/recovery or rest.

Rules:
- Respect the readiness gate: GREEN -> default STAND; AMBER -> SOFTEN unless already easy; RED -> MOVE quality to easy/recovery, or shorten an easy run. Never increase load on AMBER/RED.
- If no run is scheduled today (today_session is null), return action "REST_DAY" with a null adjusted_session and a short readiness note.
- Keep the adjusted session faithful to the M1 plan's intent and the athlete's constraints.

Output ONLY a single JSON object (no prose, no markdown fences) of this EXACT shape:
{
  "action": "STAND|SOFTEN|MOVE|REST_DAY",
  "adjusted_session": {
    "date":"YYYY-MM-DD","dow":"Mon","run_type":"easy|tempo|recovery|long|rest|intervals",
    "distance_km":0,"pace_target":"5:45/km","time_note":"~20:00 after CrossFit",
    "optional_if_cns":false,"rationale":"one line"
  },
  "rationale": "one or two sentences: what changed and why, referencing the readiness drivers"
}
For action "STAND", adjusted_session MUST equal today's session. For "REST_DAY", adjusted_session MUST be null. distance_km is 0 and pace_target/time_note are "" for rest.`

// buildDailyAdjustInput assembles the stdin payload for the daily-adjust call.
func buildDailyAdjustInput(date string, rd readiness.Readiness, today *llm.PlanDay, fit metrics.FitnessMetrics, profile ProfilePack, crossfitToday *llm.CrossFitDay, weekRationale string) DailyAdjustInput {
	return DailyAdjustInput{
		Date:          date,
		Readiness:     rd,
		TodaySession:  today,
		Metrics:       fit,
		Profile:       profile,
		CrossFitToday: crossfitToday,
		WeekRationale: weekRationale,
	}
}
