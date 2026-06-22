// Package config loads and validates process configuration from the
// environment (optionally seeded from a .env file).
package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration. Field tags map to the env var names
// defined in the M0 contracts (§4).
type Config struct {
	StravaClientID     string `envconfig:"STRAVA_CLIENT_ID" required:"true"`
	StravaClientSecret string `envconfig:"STRAVA_CLIENT_SECRET" required:"true"`
	StravaRedirectURL  string `envconfig:"STRAVA_REDIRECT_URL" required:"true"`
	APIToken           string `envconfig:"API_TOKEN" required:"true"`

	DBPath string `envconfig:"DB_PATH" default:"./helpmyrun.db"`
	Port   string `envconfig:"PORT" default:"8080"`

	GarminEmail      string `envconfig:"GARMIN_EMAIL"`
	GarminPassword   string `envconfig:"GARMIN_PASSWORD"`
	GarminTokenstore string `envconfig:"GARMIN_TOKENSTORE" default:"~/.garminconnect"`

	PythonBin    string `envconfig:"PYTHON_BIN" default:"garmin-worker/.venv/bin/python"`
	WorkerScript string `envconfig:"WORKER_SCRIPT" default:"garmin-worker/worker.py"`

	// M1: Claude Code headless + image storage.
	ClaudeBin   string `envconfig:"CLAUDE_BIN" default:"claude"`
	ClaudeModel string `envconfig:"CLAUDE_MODEL" default:"claude-opus-4-8"`
	ImageDir    string `envconfig:"IMAGE_DIR" default:"./data/crossfit"`

	AnthropicAPIKey string `envconfig:"ANTHROPIC_API_KEY"` // stub (subscription path; unused)

	// M2: agentic daily coach. The live schedule (time/tz/enable) is re-read from
	// athlete_profile on every scheduler.Run iteration (see scheduler.ConfigProvider,
	// Task 25), so PUT /api/profile edits apply without a restart; these are only
	// first-boot defaults + the push test seam, NOT the runtime source.
	AgentEnabledDefault bool   `envconfig:"AGENT_ENABLED" default:"true"`
	AgentRunTime        string `envconfig:"AGENT_RUN_TIME" default:"05:30"`
	AgentTimezone       string `envconfig:"AGENT_TZ" default:"UTC"`
	AgentTickInterval   string `envconfig:"AGENT_TICK_INTERVAL" default:"1m"`
	ExpoPushBaseURL     string `envconfig:"EXPO_PUSH_BASE_URL" default:"https://exp.host"`

	// M3.2: stream fetch trickle.
	StreamRecentWeeks int `envconfig:"STREAM_RECENT_WEEKS" default:"12"`
	StreamFetchBudget int `envconfig:"STREAM_FETCH_BUDGET" default:"10"`

	// M3.2.1: Garmin .FIT fallback start-time match tolerance (seconds).
	GarminMatchToleranceS int `envconfig:"GARMIN_MATCH_TOLERANCE_S" default:"120"`
}

// Load reads .env (if present) into the process environment, then maps env
// vars into a Config. Missing required vars return an error.
func Load() (*Config, error) {
	_ = godotenv.Load() // no error if .env absent; real env still used
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, err
	}
	return &c, nil
}
