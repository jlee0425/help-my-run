// Package config loads and validates process configuration from the
// environment (optionally seeded from a .env file).
package config

import (
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration. Field tags map to the env var names
// defined in the M0 contracts (§4).
type Config struct {
	// M5: API_TOKEN, GARMIN_EMAIL, and GARMIN_PASSWORD are gone from the
	// environment — the owner password/API token live in the DB (web setup),
	// and Garmin login happens in the web UI (creds pass over stdin, once).
	DBPath string `envconfig:"DB_PATH" default:"./helpmyrun.db"`
	Port   string `envconfig:"PORT" default:"8080"`

	GarminTokenstore string `envconfig:"GARMIN_TOKENSTORE" default:"~/.garminconnect"`

	PythonBin    string `envconfig:"PYTHON_BIN" default:"garmin-worker/.venv/bin/python"`
	WorkerScript string `envconfig:"WORKER_SCRIPT" default:"garmin-worker/worker.py"`

	// M1: Claude Code headless + image storage.
	ClaudeBin   string `envconfig:"CLAUDE_BIN" default:"claude"`
	ClaudeModel string `envconfig:"CLAUDE_MODEL" default:"claude-opus-4-8"`
	ImageDir    string `envconfig:"IMAGE_DIR" default:"./data/crossfit"`

	// M2: agentic daily coach. The live schedule (time/tz/enable) is re-read from
	// athlete_profile on every scheduler.Run iteration (see scheduler.ConfigProvider,
	// Task 25), so PUT /api/profile edits apply without a restart; these are only
	// first-boot defaults + the push test seam, NOT the runtime source.
	AgentEnabledDefault bool   `envconfig:"AGENT_ENABLED" default:"true"`
	AgentRunTime        string `envconfig:"AGENT_RUN_TIME" default:"05:30"`
	AgentTimezone       string `envconfig:"AGENT_TZ" default:"UTC"`

	// M3.2: stream fetch trickle.
	StreamRecentWeeks int `envconfig:"STREAM_RECENT_WEEKS" default:"12"`
	StreamFetchBudget int `envconfig:"STREAM_FETCH_BUDGET" default:"10"`

	// M3.3: chat rolling-history turns sent per claude -p call.
	ChatHistoryTurns int `envconfig:"CHAT_HISTORY_TURNS" default:"6"`

	// M6: nightly backups. BackupDir's real default depends on DB_PATH, so it
	// stays empty here and resolves via ResolvedBackupDir.
	BackupDir  string `envconfig:"BACKUP_DIR"`
	BackupKeep int    `envconfig:"BACKUP_KEEP" default:"14"`

	// M6.5: demo mode — set by the --demo flag only, never from the
	// environment (an accidental DEMO=true must not bypass auth).
	Demo bool `ignored:"true"`
}

// ResolvedBackupDir returns BACKUP_DIR, defaulting to a backups/ dir next to
// the database file.
func (c *Config) ResolvedBackupDir() string {
	if c.BackupDir != "" {
		return c.BackupDir
	}
	return filepath.Join(filepath.Dir(c.DBPath), "backups")
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
