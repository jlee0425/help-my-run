// Package demo provides the offline demo dataset seeder and a canned llm.Runner
// so the whole product (Today / Trends / Plan / Coach / chat) can be shown with
// no Garmin account and no Claude subscription. The real coach/chat/progress
// engines run unchanged — only the LLM call is substituted by DemoRunner, and
// only the data source is the embedded fixture.
package demo

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"help-my-run/backend/internal/llm"
)

// SampleLabel is stamped onto every canned coach output so a demo visitor always
// knows the response is not from a live Claude subscription. Honesty is the point.
const SampleLabel = "⚠ sample response — the live coach runs on your Claude subscription"

// Routing markers: distinctive substrings of each engine's claude -p prompt
// (args[1]). They are PINNED to the real prompt constants by
// TestRoutingMarkersPinnedToSource so a reworded prompt fails the suite instead
// of silently breaking demo routing at runtime. Sources:
//   - markerDailyAdjust, markerPlan, markerCrossFit: internal/coach/prompts.go
//     (dailyAdjustPrompt, coachBrainPrompt, stage1Template)
//   - markerChat: internal/chat/prompts.go (chatPrompt)
//   - markerProgress: internal/progress/prompts.go (progressReadPrompt)
const (
	markerDailyAdjust = "SINGLE-DAY adjustment"
	markerPlan        = "Build a 7-day running plan"
	markerCrossFit    = "weekly programming photo"
	markerChat        = "data analyst and running coach"
	markerProgress    = "short progress read"
)

// Runner is the canned llm.Runner used in demo mode. It is stateless (empty
// struct, value-usable); chat rotation lives in a package-level atomic counter so
// Runner{} satisfies llm.Runner without a constructor.
type Runner struct{}

// compile-time assertion that Runner satisfies the llm.Runner seam.
var _ llm.Runner = Runner{}

// chatCounter drives chat-answer rotation across calls (atomic = thread-safe).
var chatCounter atomic.Uint64

// Run routes on args[1] (the `-p` prompt each engine passes) and returns a
// claude-CLI success envelope whose .result is the curated payload JSON as a
// string — exactly what llm.Client.Call expects (ParseEnvelope + ExtractJSON).
// An unknown prompt yields a generic valid envelope, never an error.
func (Runner) Run(_ context.Context, args []string, stdin string) ([]byte, error) {
	prompt := ""
	if len(args) >= 2 {
		prompt = args[1]
	}
	switch {
	case strings.Contains(prompt, markerDailyAdjust):
		return dailyAdjustEnvelope(stdin)
	case strings.Contains(prompt, markerPlan):
		return planEnvelope(stdin)
	case strings.Contains(prompt, markerCrossFit):
		return crossfitEnvelope(prompt)
	case strings.Contains(prompt, markerChat):
		return chatEnvelope()
	case strings.Contains(prompt, markerProgress):
		return progressEnvelope()
	default:
		return genericEnvelope()
	}
}

// envelope wraps a payload as the `claude -p --output-format json` success
// envelope: .result carries the payload JSON as a string (matches llm.Envelope).
func envelope(payload any) ([]byte, error) {
	inner, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(llm.Envelope{
		Type:    "result",
		Subtype: "success",
		IsError: false,
		Result:  string(inner),
	})
}

// dailyAdjustEnvelope returns an amber-friendly SOFTEN decision (llm.DailyDecisionParsed).
func dailyAdjustEnvelope(stdin string) ([]byte, error) {
	var in struct {
		Date         string       `json:"date"`
		TodaySession *llm.PlanDay `json:"today_session"`
	}
	_ = json.Unmarshal([]byte(stdin), &in)

	date, dow := in.Date, weekdayOf(in.Date)
	if in.TodaySession != nil {
		if in.TodaySession.Date != "" {
			date = in.TodaySession.Date
		}
		if in.TodaySession.Dow != "" {
			dow = in.TodaySession.Dow
		}
	}

	adj := &llm.PlanDay{
		Date:          date,
		Dow:           dow,
		RunType:       "easy",
		DistanceKm:    6,
		PaceTarget:    "6:30/km",
		TimeNote:      "Evening — keep it conversational",
		OptionalIfCNS: true,
		Rationale:     "Eased to an easy 6 km so you absorb yesterday's CrossFit load.",
	}
	dec := llm.DailyDecisionParsed{
		Action:          llm.ActionSoften,
		AdjustedSession: adj,
		Rationale: "Amber readiness: HRV is suppressed and yesterday was a heavy CrossFit leg day, " +
			"so I'm keeping the run but swapping the tempo for an easy 6 km. " + SampleLabel,
	}
	return envelope(dec)
}

// planEnvelope returns a realistic CrossFit-aware Stage-2 plan (llm.PlanParsed).
func planEnvelope(stdin string) ([]byte, error) {
	var in struct {
		WeekStart string `json:"week_start"`
	}
	_ = json.Unmarshal([]byte(stdin), &in)
	weekStart := in.WeekStart
	if weekStart == "" {
		weekStart = mondayOfTime(time.Now())
	}
	plan := llm.PlanParsed{
		FitnessSummary: "Aerobic base is trending up — VO2max near 47.5 and easy pace improving at a steady heart rate.",
		WeeklyTargetKm: 30,
		Days:           demoPlanDays(weekStart),
		WeekRationale: "One tempo on a low-CNS day, a protected weekend long run, everything else easy to keep " +
			"legs fresh for CrossFit. " + SampleLabel,
		OneFlag: "Keep the tempo controlled while HRV finishes recovering.",
	}
	return envelope(plan)
}

// demoPlanDays builds a Mon→Sun plan week anchored on weekStart.
func demoPlanDays(weekStart string) []llm.PlanDay {
	mon, err := time.Parse("2006-01-02", weekStart)
	if err != nil {
		mon, _ = time.Parse("2006-01-02", mondayOfTime(time.Now()))
	}
	tmpl := []struct {
		rt, pace, note, rat string
		km                  float64
	}{
		{"rest", "", "", "CrossFit leg day — running rest.", 0},
		{"easy", "6:20/km", "Evening after CrossFit", "Aerobic base, conversational.", 8},
		{"easy", "6:30/km", "Morning", "Recovery shakeout.", 6},
		{"tempo", "5:15/km", "Evening — barbell-skill day, legs are fresh", "Weekly threshold work on the low-CNS day.", 8},
		{"easy", "6:30/km", "Evening after CrossFit", "Easy aerobic under the Zone 2 ceiling.", 6},
		{"long", "6:40/km", "Morning", "Weekend long run — the aerobic centerpiece.", 16},
		{"rest", "", "", "Rest — recover for the week ahead.", 0},
	}
	out := make([]llm.PlanDay, 7)
	for i := 0; i < 7; i++ {
		d := mon.AddDate(0, 0, i)
		out[i] = llm.PlanDay{
			Date:       d.Format("2006-01-02"),
			Dow:        d.Format("Mon"),
			RunType:    tmpl[i].rt,
			DistanceKm: tmpl[i].km,
			PaceTarget: tmpl[i].pace,
			TimeNote:   tmpl[i].note,
			Rationale:  tmpl[i].rat,
		}
	}
	return out
}

// crossfitEnvelope returns a plausible Stage-1 CrossFit week (llm.CrossFitWeekParsed).
// The week start is parsed from the prompt ("The week starts on Monday <date>."),
// falling back to the current week.
func crossfitEnvelope(prompt string) ([]byte, error) {
	weekStart := extractWeekStart(prompt)
	if weekStart == "" {
		weekStart = mondayOfTime(time.Now())
	}
	return envelope(demoCrossFitWeek(weekStart))
}

// extractWeekStart pulls the "Monday YYYY-MM-DD" date out of the Stage-1 prompt.
func extractWeekStart(prompt string) string {
	const key = "week starts on Monday "
	i := strings.Index(prompt, key)
	if i < 0 {
		return ""
	}
	rest := prompt[i+len(key):]
	if len(rest) < 10 {
		return ""
	}
	cand := rest[:10]
	if _, err := time.Parse("2006-01-02", cand); err != nil {
		return ""
	}
	return cand
}

// chatEnvelope returns one of a small set of curated coaching answers, rotating
// by call count so consecutive questions read differently. Each ends with the label.
func chatEnvelope() ([]byte, error) {
	answers := []string{
		"Your engine is genuinely improving, not just your mileage. Over the last 12 weeks your easy pace " +
			"dropped from about 6:35 to 6:20 per km at the same ~140 bpm, and VO2max ticked up from 46.0 to 47.5.",
		"Around 10 days ago your HRV fell to the low-40s (about 27% under your ~60 ms baseline) and resting HR " +
			"rose 7 bpm — a short overreach from stacking heavy CrossFit legs onto your long run. You've recovered, " +
			"though today's HRV is still a touch suppressed, so you're amber.",
		"Long-run decoupling is holding under 5%, which means you're staying aerobically efficient deep into the " +
			"run — a good sign the base is real and not just fresh-leg speed.",
	}
	idx := int(chatCounter.Add(1)-1) % len(answers)
	payload := struct {
		Text string `json:"text"`
	}{Text: answers[idx] + "\n\n" + SampleLabel}
	return envelope(payload)
}

// progressEnvelope returns a sensible 12-week narrative ({"text": ...}).
func progressEnvelope() ([]byte, error) {
	payload := struct {
		Text string `json:"text"`
	}{Text: "Over the last 12 weeks your engine is clearly stronger: easy pace fell from ~6:35 to ~6:20 per km " +
		"at the same ~140 bpm, VO2max rose 46.0→47.5, and long-run decoupling stayed under 5%. Resting HR held in " +
		"the low-50s. The one blip was a short overreach about 10 days ago, which you've recovered from. " + SampleLabel}
	return envelope(payload)
}

// genericEnvelope is the fallback for an unrecognized prompt: a valid, non-error
// envelope carrying a short labelled message (never an error).
func genericEnvelope() ([]byte, error) {
	payload := struct {
		Text string `json:"text"`
	}{Text: "Demo coach placeholder. " + SampleLabel}
	return envelope(payload)
}

// weekdayOf returns the short weekday name for an ISO date, or "" if unparseable.
func weekdayOf(isoDate string) string {
	if t, err := time.Parse("2006-01-02", isoDate); err == nil {
		return t.Format("Mon")
	}
	return ""
}
