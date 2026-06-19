// Package strava is a small, base-URL-injectable client for the Strava API
// (OAuth + activities + laps) used by the sync layer.
package strava

// TokenResponse is the Strava /oauth/token reply (exchange + refresh).
type TokenResponse struct {
	TokenType    string          `json:"token_type"`
	AccessToken  string          `json:"access_token"`
	RefreshToken string          `json:"refresh_token"`
	ExpiresAt    int64           `json:"expires_at"` // unix seconds
	ExpiresIn    int64           `json:"expires_in"`
	Scope        string          `json:"scope"`
	Athlete      *SummaryAthlete `json:"athlete"`
}

// SummaryAthlete is the minimal athlete sub-object on a token response.
type SummaryAthlete struct {
	ID int64 `json:"id"`
}

// SummaryActivity is a Strava activity (run). HR/speed/cadence are pointers
// because they are absent when no sensor was present.
type SummaryActivity struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Type               string   `json:"type"`
	SportType          string   `json:"sport_type"`
	StartDate          string   `json:"start_date"`
	StartDateLocal     string   `json:"start_date_local"`
	Distance           float64  `json:"distance"`
	MovingTime         int64    `json:"moving_time"`
	ElapsedTime        int64    `json:"elapsed_time"`
	AverageHeartrate   *float64 `json:"average_heartrate"`
	MaxHeartrate       *float64 `json:"max_heartrate"`
	AverageSpeed       *float64 `json:"average_speed"`
	MaxSpeed           *float64 `json:"max_speed"`
	AverageCadence     *float64 `json:"average_cadence"`
	TotalElevationGain *float64 `json:"total_elevation_gain"`
}

// Lap is a Strava lap (mapped to activity_splits).
type Lap struct {
	LapIndex         int64    `json:"lap_index"`
	Distance         float64  `json:"distance"`
	ElapsedTime      int64    `json:"elapsed_time"`
	MovingTime       *int64   `json:"moving_time"`
	AverageHeartrate *float64 `json:"average_heartrate"`
	MaxHeartrate     *float64 `json:"max_heartrate"`
	AverageSpeed     *float64 `json:"average_speed"`
}
