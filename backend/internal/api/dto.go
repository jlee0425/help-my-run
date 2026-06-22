package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON writes v as application/json with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- /health ---
type healthResp struct {
	Status string `json:"status"`
}

// --- /api/status ---
type sourceStatus struct {
	Connected    bool    `json:"connected"`
	LastSyncedAt *string `json:"last_synced_at"`
	LastRunAt    *string `json:"last_run_at"`
	Status       string  `json:"status"`
	Error        *string `json:"error"`
}
type stravaStatus struct {
	sourceStatus
	AthleteID *int64 `json:"athlete_id"`
}
type statusCounts struct {
	Activities   int `json:"activities"`
	RecoveryDays int `json:"recovery_days"`
}
type statusResp struct {
	Strava stravaStatus `json:"strava"`
	Garmin sourceStatus `json:"garmin"`
	Counts statusCounts `json:"counts"`
}

// --- /api/strava/connect ---
type connectResp struct {
	AuthorizeURL string `json:"authorizeUrl"`
}

// --- /api/sync ---
type syncSourceResult struct {
	Status string  `json:"status"`
	Synced int     `json:"synced"`
	Error  *string `json:"error"`
}
type syncResp struct {
	Strava syncSourceResult `json:"strava"`
	Garmin syncSourceResult `json:"garmin"`
}

// --- /api/activities ---
type activityDTO struct {
	StravaID       int64    `json:"strava_id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	SportType      *string  `json:"sport_type"`
	StartTime      string   `json:"start_time"`
	StartTimeLocal *string  `json:"start_time_local"`
	DistanceM      float64  `json:"distance_m"`
	MovingTimeS    int64    `json:"moving_time_s"`
	ElapsedTimeS   int64    `json:"elapsed_time_s"`
	AvgHR          *float64 `json:"avg_hr"`
	MaxHR          *float64 `json:"max_hr"`
	AvgSpeed       *float64 `json:"avg_speed"`
	MaxSpeed       *float64 `json:"max_speed"`
	AvgCadence     *float64 `json:"avg_cadence"`
	ElevationGainM *float64 `json:"elevation_gain_m"`
}
type activitiesResp struct {
	Activities []activityDTO `json:"activities"`
}

// --- /api/recovery ---
type sleepDTO struct {
	DurationS *int64 `json:"duration_s"`
	DeepS     *int64 `json:"deep_s"`
	LightS    *int64 `json:"light_s"`
	RemS      *int64 `json:"rem_s"`
	AwakeS    *int64 `json:"awake_s"`
	Score     *int64 `json:"score"`
}
type hrvDTO struct {
	LastNightAvgMs *int64  `json:"last_night_avg_ms"`
	Status         *string `json:"status"`
}
type bodyBatteryDTO struct {
	Charged *int64 `json:"charged"`
	Drained *int64 `json:"drained"`
	High    *int64 `json:"high"`
	Low     *int64 `json:"low"`
}
type rhrDTO struct {
	RestingHR *int64 `json:"resting_hr"`
}
type recoveryDayDTO struct {
	Date        string          `json:"date"`
	Sleep       *sleepDTO       `json:"sleep"`
	HRV         *hrvDTO         `json:"hrv"`
	BodyBattery *bodyBatteryDTO `json:"body_battery"`
	RHR         *rhrDTO         `json:"rhr"`
}
type recoveryResp struct {
	Recovery []recoveryDayDTO `json:"recovery"`
}

// --- M1 /api/profile (extended in M2) ---
type profileDTO struct {
	TargetWeeklyKm     float64 `json:"target_weekly_km"`
	ProgressionMode    string  `json:"progression_mode"`
	Zone2CeilingBpm    *int64  `json:"zone2_ceiling_bpm"`
	ThresholdBpm       *int64  `json:"threshold_bpm"`
	MaxHRBpm           *int64  `json:"max_hr_bpm"`
	RunConstraintsJSON string  `json:"run_constraints_json"`
	GoalText           string  `json:"goal_text"`
	DailyRunTime       string  `json:"daily_run_time"` // "HH:MM" 24h local (M2)
	Timezone           string  `json:"timezone"`       // IANA (M2)
	AgentEnabled       bool    `json:"agent_enabled"`  // M2 daily agent on/off
	UpdatedAt          string  `json:"updated_at,omitempty"`
}

// --- M1 /api/plan/generate + /api/plan ---
type planResponseDTO struct {
	ID             int64        `json:"id"`
	WeekStart      string       `json:"week_start"`
	GeneratedAt    string       `json:"generated_at"`
	FitnessSummary string       `json:"fitness_summary"`
	WeeklyTargetKm float64      `json:"weekly_target_km"`
	Days           []planDayDTO `json:"days"`
	WeekRationale  string       `json:"week_rationale"`
	OneFlag        string       `json:"one_flag"`
}
type planDayDTO struct {
	Date          string  `json:"date"`
	Dow           string  `json:"dow"`
	RunType       string  `json:"run_type"`
	DistanceKm    float64 `json:"distance_km"`
	PaceTarget    string  `json:"pace_target"`
	TimeNote      string  `json:"time_note"`
	OptionalIfCNS bool    `json:"optional_if_cns"`
	Rationale     string  `json:"rationale"`
}

// --- M2 /api/push/register ---
type pushRegisterRequestDTO struct {
	ExpoPushToken string `json:"expo_push_token"`
	Platform      string `json:"platform"` // "ios"|"android"
}
type pushRegisterResponseDTO struct {
	ExpoPushToken string `json:"expo_push_token"`
	Platform      string `json:"platform"`
	UpdatedAt     string `json:"updated_at"`
}

// --- M2 /api/today ---
type readinessDriversDTO struct {
	Date            string   `json:"date"`
	SleepHours      *float64 `json:"sleep_hours"`
	SleepScore      *int64   `json:"sleep_score"`
	HRVLastNightMs  *int64   `json:"hrv_last_night_ms"`
	HRVBaselineMs   *float64 `json:"hrv_baseline_ms"`
	HRVDeltaPct     *float64 `json:"hrv_delta_pct"`
	RHRLastNight    *int64   `json:"rhr_last_night"`
	RHRBaseline     *float64 `json:"rhr_baseline"`
	RHRDeltaBpm     *float64 `json:"rhr_delta_bpm"`
	BodyBatteryHigh *int64   `json:"body_battery_high"`
	RecoveryTrend   string   `json:"recovery_trend"`
	DataComplete    bool     `json:"data_complete"`
}
type todayResponseDTO struct {
	Date             string              `json:"date"`
	ReadinessColor   string              `json:"readiness_color"`
	Drivers          readinessDriversDTO `json:"drivers"`
	Reasons          []string            `json:"reasons"`
	Action           string              `json:"action"`
	OriginalSession  *planDayDTO         `json:"original_session"`
	EffectiveSession *planDayDTO         `json:"effective_session"`
	Rationale        string              `json:"rationale"`
	Source           string              `json:"source"`
	Stale            bool                `json:"stale"`
}

// --- M3.2 /api/activities/{id}/analysis + /stream/fetch (snake_case wire JSON) ---
type zoneTimeDTO struct {
	Zone    int     `json:"zone"`
	Seconds float64 `json:"seconds"`
	Pct     float64 `json:"pct"`
}
type zoneBoundsDTO struct {
	Z1Hi float64 `json:"z1_hi"`
	Z2Hi float64 `json:"z2_hi"`
	Z3Hi float64 `json:"z3_hi"`
	Z4Hi float64 `json:"z4_hi"`
}
type streamAnalysisDTO struct {
	ActivityID    int64         `json:"activity_id"`
	HasStream     bool          `json:"has_stream"`
	HasHR         bool          `json:"has_hr"`
	TimeInZone    []zoneTimeDTO `json:"time_in_zone"`
	DecouplingPct *float64      `json:"decoupling_pct"`
	PaHRFirst     *float64      `json:"pa_hr_first"`
	PaHRSecond    *float64      `json:"pa_hr_second"`
	Zones         zoneBoundsDTO `json:"zones"`
	Source        string        `json:"source"`
	ComputedAt    string        `json:"computed_at"`
}
