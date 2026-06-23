// Package garmin invokes the Python worker via os/exec and parses its JSON
// (the contracts §2 shape).
package garmin

import "encoding/json"

// WorkerOutput is the top-level worker stdout JSON.
type WorkerOutput struct {
	Since       string           `json:"since"`
	Until       string           `json:"until"`
	FetchedAt   string           `json:"fetched_at"`
	Sleep       []SleepDay       `json:"sleep"`
	HRV         []HrvDay         `json:"hrv"`
	BodyBattery []BodyBatteryDay `json:"body_battery"`
	RHR         []RhrDay         `json:"rhr"`
	VO2Max      []Vo2maxDay      `json:"vo2max"`
	Activities  []GarminActivity `json:"activities"` // M3.2.1
}

// GarminActivity is one element of the Garmin activities list (§2.x), enriched
// to the full canonical-activity record. DurationS is removed; MovingTimeS and
// ElapsedTimeS carry the worker's float durations (rounded to int64 on upsert).
type GarminActivity struct {
	GarminActivityID int64           `json:"garmin_activity_id"`
	Name             string          `json:"name"`
	StartTime        string          `json:"start_time"`
	StartTimeLocal   *string         `json:"start_time_local"`
	ActivityType     *string         `json:"activity_type"`
	DistanceM        *float64        `json:"distance_m"`
	MovingTimeS      *float64        `json:"moving_time_s"`
	ElapsedTimeS     *float64        `json:"elapsed_time_s"`
	AvgHR            *float64        `json:"avg_hr"`
	MaxHR            *float64        `json:"max_hr"`
	AvgSpeed         *float64        `json:"avg_speed"`
	MaxSpeed         *float64        `json:"max_speed"`
	AvgCadence       *float64        `json:"avg_cadence"`
	ElevationGainM   *float64        `json:"elevation_gain_m"`
	RawJSON          json.RawMessage `json:"raw_json"`
}

// SleepDay is one per-day sleep entry. RawJSON is kept verbatim for the store.
type SleepDay struct {
	Date      string          `json:"date"`
	DurationS *int64          `json:"duration_s"`
	DeepS     *int64          `json:"deep_s"`
	LightS    *int64          `json:"light_s"`
	RemS      *int64          `json:"rem_s"`
	AwakeS    *int64          `json:"awake_s"`
	Score     *int64          `json:"score"`
	RawJSON   json.RawMessage `json:"raw_json"`
}

// HrvDay is one per-day HRV entry.
type HrvDay struct {
	Date           string          `json:"date"`
	LastNightAvgMs *int64          `json:"last_night_avg_ms"`
	Status         *string         `json:"status"`
	RawJSON        json.RawMessage `json:"raw_json"`
}

// BodyBatteryDay is one per-day Body Battery entry.
type BodyBatteryDay struct {
	Date    string          `json:"date"`
	Charged *int64          `json:"charged"`
	Drained *int64          `json:"drained"`
	High    *int64          `json:"high"`
	Low     *int64          `json:"low"`
	RawJSON json.RawMessage `json:"raw_json"`
}

// RhrDay is one per-day resting-HR entry.
type RhrDay struct {
	Date      string          `json:"date"`
	RestingHR *int64          `json:"resting_hr"`
	RawJSON   json.RawMessage `json:"raw_json"`
}

// Vo2maxDay is one per-day VO2max entry (running VO2max from get_max_metrics).
type Vo2maxDay struct {
	Date    string          `json:"date"`
	VO2Max  *float64        `json:"vo2max"`
	RawJSON json.RawMessage `json:"raw_json"`
}

// FITStreamOutput is the worker `stream` subcommand stdout JSON (§2.6).
type FITStreamOutput struct {
	ActivityID int64     `json:"activity_id"` // echoed activity id (store PK)
	Source     string    `json:"source"`      // "garmin"
	FetchedAt  string    `json:"fetched_at"`
	Series     FITSeries `json:"series"`
}

// FITSeries is the normalized struct-of-arrays series (matches streams.Series).
type FITSeries struct {
	T    []float64 `json:"t"`
	HR   []float64 `json:"hr"`
	V    []float64 `json:"v"`
	Dist []float64 `json:"dist"`
}
