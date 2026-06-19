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
