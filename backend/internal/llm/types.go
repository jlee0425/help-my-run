package llm

// Load is the CNS/leg load enum (exactly low|med|high).
type Load string

const (
	LoadLow  Load = "low"
	LoadMed  Load = "med"
	LoadHigh Load = "high"
)

// CrossFitDay is one day in the Stage-1 parsed week.
type CrossFitDay struct {
	Date        string `json:"date"`
	Dow         string `json:"dow"`
	HasCrossFit bool   `json:"has_crossfit"`
	Focus       string `json:"focus"`
	CNSLoad     Load   `json:"cns_load"`
	LegLoad     Load   `json:"leg_load"`
	Notes       string `json:"notes"`
}

// CrossFitWeekParsed is the Stage-1 model output.
type CrossFitWeekParsed struct {
	WeekStart string        `json:"week_start"`
	Days      []CrossFitDay `json:"days"`
}

// PlanDay is one day in the Stage-2 plan.
type PlanDay struct {
	Date          string  `json:"date"`
	Dow           string  `json:"dow"`
	RunType       string  `json:"run_type"`
	DistanceKm    float64 `json:"distance_km"`
	PaceTarget    string  `json:"pace_target"`
	TimeNote      string  `json:"time_note"`
	OptionalIfCNS bool    `json:"optional_if_cns"`
	Rationale     string  `json:"rationale"`
}

// PlanParsed is the Stage-2 model output.
type PlanParsed struct {
	FitnessSummary string    `json:"fitness_summary"`
	WeeklyTargetKm float64   `json:"weekly_target_km"`
	Days           []PlanDay `json:"days"`
	WeekRationale  string    `json:"week_rationale"`
	OneFlag        string    `json:"one_flag"`
}

// DailyAction is the single-day adjust action enum (M2 daily coach).
type DailyAction string

const (
	ActionStand   DailyAction = "STAND"
	ActionSoften  DailyAction = "SOFTEN"
	ActionMove    DailyAction = "MOVE"
	ActionRestDay DailyAction = "REST_DAY"
)

// DailyDecisionParsed is the EXACT JSON claude -p emits for the daily adjust.
// AdjustedSession reuses PlanDay verbatim; it is nil for REST_DAY.
type DailyDecisionParsed struct {
	Action          DailyAction `json:"action"`
	AdjustedSession *PlanDay    `json:"adjusted_session"`
	Rationale       string      `json:"rationale"`
}
