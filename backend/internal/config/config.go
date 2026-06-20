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
