package progress

import (
	"fmt"
	"strings"

	"help-my-run/backend/internal/metrics"
)

// ProgressReadInput is the JSON piped to claude -p stdin for the progress read:
// the computed trends + window + the athlete's goal context. snake_case wire JSON.
type ProgressReadInput struct {
	Weeks    int            `json:"weeks"`
	Signals  []TrendSummary `json:"signals"`
	GoalText string         `json:"goal_text"`
}

// ProgressRead is the analyze result (mirrors M2 source = ai|fallback semantics).
type ProgressRead struct {
	Text   string `json:"text"`
	Source string `json:"source"` // "ai" | "fallback"
}

// progressReadPrompt is the claude -p instruction block for the on-demand read.
// The structured ProgressReadInput is piped on stdin; the model returns ONLY a
// {"text": "..."} JSON object.
const progressReadPrompt = `You are a CrossFit-aware running coach giving a short progress read. You receive a JSON
context on stdin: the number of weeks in the window, the athlete's goal text, and an array
of computed trend signals. Each signal has: key, label, unit, current, baseline, delta_abs,
direction (up|down|flat), lower_is_better, and a weekly series (nulls are weeks with no data).

The athlete is training to improve aerobic capacity for RX CrossFit, NOT to race. Interpret
each signal correctly: for pace_at_hr and resting_hr, LOWER is better (a falling value is
improvement); for vo2max and hrv_baseline, HIGHER is better; weekly_load is context, not a
fitness verdict. Faster pace at the same heart rate = a stronger engine.

Write 2-4 sentences, plain text (NO markdown, NO bullet list), that tell the athlete whether
their engine is improving and which signal is the clearest evidence. Be concrete (cite a
number or two). If the data is thin, say so honestly.

Output ONLY a single JSON object (no prose outside it, no markdown fences) of this EXACT shape:
{"text": "..."}`

// fmtVal renders a signal value respecting its unit (pace formatted as M:SS/km).
func fmtVal(unit string, v float64) string {
	if unit == "s/km" {
		return metrics.FormatPace(v)
	}
	return fmt.Sprintf("%g%s", v, unit)
}

// fallbackProgressText is the deterministic, no-LLM one-paragraph summary built
// from the computed report. Used whenever the claude -p read fails.
func fallbackProgressText(rep ProgressReport) string {
	if !rep.EnoughData {
		return "Not enough history yet to read a trend — keep logging runs and syncing Garmin."
	}
	var clauses []string
	for _, sg := range rep.Signals {
		if sg.Current == nil || sg.Baseline == nil {
			continue
		}
		verb := "held"
		switch sg.Direction {
		case DirectionUp:
			if sg.LowerIsBetter {
				verb = "worsened"
			} else {
				verb = "improved"
			}
		case DirectionDown:
			if sg.LowerIsBetter {
				verb = "improved"
			} else {
				verb = "worsened"
			}
		}
		clauses = append(clauses, fmt.Sprintf("%s %s (%s → %s)",
			sg.Label, verb, fmtVal(sg.Unit, *sg.Baseline), fmtVal(sg.Unit, *sg.Current)))
	}
	if len(clauses) == 0 {
		return "Not enough history yet to read a trend — keep logging runs and syncing Garmin."
	}
	return fmt.Sprintf("Over the last %d weeks: %s.", rep.Weeks, strings.Join(clauses, "; "))
}
