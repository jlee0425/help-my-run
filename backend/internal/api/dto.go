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
