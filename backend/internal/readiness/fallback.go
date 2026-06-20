package readiness

import "math"

// FallbackAction is the deterministic action string (matches llm.DailyAction values
// and the daily_decisions CHECK constraint: STAND|SOFTEN|MOVE|REST_DAY).
type FallbackAction string

const (
	FbStand   FallbackAction = "STAND"
	FbSoften  FallbackAction = "SOFTEN"
	FbMove    FallbackAction = "MOVE"
	FbRestDay FallbackAction = "REST_DAY"
)

// FallbackSession mirrors the llm.PlanDay fields the fallback rule reads/writes.
// readiness stays free of an llm import; the coach/agent converts *llm.PlanDay
// to/from this struct at the call site.
type FallbackSession struct {
	Date          string
	Dow           string
	RunType       string
	DistanceKm    float64
	PaceTarget    string
	TimeNote      string
	OptionalIfCNS bool
	Rationale     string
}

// FallbackDecision is the deterministic-rule output. Adjusted is nil for REST_DAY.
type FallbackDecision struct {
	Action    FallbackAction
	Adjusted  *FallbackSession
	Rationale string
}

// easyTypes are the run types treated as already-easy (no quality to remove).
var easyTypes = map[string]bool{"easy": true, "recovery": true, "rest": true}

// isEasyType reports whether a run type is already easy/recovery/rest.
func isEasyType(runType string) bool { return easyTypes[runType] }

// roundHalf rounds to the nearest 0.5 (e.g. 4.74 -> 4.5, 4.75 -> 5.0).
func roundHalf(x float64) float64 { return math.Round(x*2) / 2 }

// Fallback applies the deterministic readiness->action rule (M2 contract §2) used
// when claude -p is unavailable. `easyPace` is the athlete's computed easy pace
// (coach.Fitness(ctx).EasyPace); it eases tempo/interval targets on AMBER/RED.
// Pure function: no DB, no clock, no llm import.
func Fallback(color Color, session *FallbackSession, easyPace string) FallbackDecision {
	if session == nil {
		var r string
		switch color {
		case ColorGreen:
			r = "Rest day as planned; you're well recovered."
		default:
			r = "Rest day — readiness low, stay recovered."
		}
		return FallbackDecision{Action: FbRestDay, Adjusted: nil, Rationale: r}
	}

	switch color {
	case ColorRed:
		if isEasyType(session.RunType) {
			adj := *session
			adj.DistanceKm = roundHalf(session.DistanceKm * 0.5)
			adj.PaceTarget = easyPace
			adj.OptionalIfCNS = true
			adj.Rationale = "Low readiness — distance halved, kept easy."
			return FallbackDecision{Action: FbSoften, Adjusted: &adj, Rationale: adj.Rationale}
		}
		dist := session.DistanceKm
		if dist > 4 {
			dist = 4
		}
		adj := FallbackSession{
			Date:          session.Date,
			Dow:           session.Dow,
			RunType:       "recovery",
			DistanceKm:    roundHalf(dist),
			PaceTarget:    easyPace,
			TimeNote:      session.TimeNote,
			OptionalIfCNS: true,
			Rationale:     "Low readiness — moved to easy recovery.",
		}
		return FallbackDecision{Action: FbMove, Adjusted: &adj, Rationale: adj.Rationale}

	case ColorAmber:
		if isEasyType(session.RunType) {
			adj := *session
			adj.Rationale = "Reduced readiness — easy run stands."
			return FallbackDecision{Action: FbStand, Adjusted: &adj, Rationale: adj.Rationale}
		}
		adj := *session
		adj.DistanceKm = roundHalf(session.DistanceKm * 0.75)
		adj.PaceTarget = easyPace
		adj.Rationale = "Reduced readiness — trimmed volume/intensity."
		return FallbackDecision{Action: FbSoften, Adjusted: &adj, Rationale: adj.Rationale}

	default: // ColorGreen
		adj := *session
		if adj.Rationale == "" {
			adj.Rationale = "Well recovered — session stands as planned."
		}
		return FallbackDecision{Action: FbStand, Adjusted: &adj, Rationale: "Well recovered — session stands as planned."}
	}
}
