package config

import (
	"os"
	"testing"

	"github.com/kelseyhightower/envconfig"
)

// setEnv sets the given env vars for the duration of the test and TRULY UNSETS
// any others that Load reads, so tests are hermetic. We must Unsetenv (not set
// to "") because envconfig's required check uses os.LookupEnv: a set-but-empty
// var counts as PRESENT and would defeat TestLoadMissingRequired.
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	all := []string{
		"API_TOKEN", "DB_PATH", "PORT",
		"GARMIN_EMAIL", "GARMIN_PASSWORD", "GARMIN_TOKENSTORE",
		"PYTHON_BIN", "WORKER_SCRIPT", "ANTHROPIC_API_KEY",
		"CLAUDE_BIN", "CLAUDE_MODEL", "IMAGE_DIR",
		"STREAM_RECENT_WEEKS", "STREAM_FETCH_BUDGET",
		"CHAT_HISTORY_TURNS",
	}
	for _, k := range all {
		// t.Setenv first to register restoration of the original value on
		// cleanup, then Unsetenv to actually clear it for this test.
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func requiredEnv() map[string]string {
	return map[string]string{
		"API_TOKEN": "tok",
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.DBPath != "./helpmyrun.db" {
		t.Errorf("DBPath = %q, want default %q", cfg.DBPath, "./helpmyrun.db")
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want default %q", cfg.Port, "8080")
	}
	if cfg.GarminTokenstore != "~/.garminconnect" {
		t.Errorf("GarminTokenstore = %q, want default %q", cfg.GarminTokenstore, "~/.garminconnect")
	}
	if cfg.PythonBin != "garmin-worker/.venv/bin/python" {
		t.Errorf("PythonBin = %q, want default %q", cfg.PythonBin, "garmin-worker/.venv/bin/python")
	}
	if cfg.WorkerScript != "garmin-worker/worker.py" {
		t.Errorf("WorkerScript = %q, want default %q", cfg.WorkerScript, "garmin-worker/worker.py")
	}
}

func TestLoadExplicit(t *testing.T) {
	env := requiredEnv()
	env["DB_PATH"] = "/tmp/x.db"
	env["PORT"] = "9090"
	env["GARMIN_EMAIL"] = "you@example.com"
	env["GARMIN_PASSWORD"] = "pw"
	env["GARMIN_TOKENSTORE"] = "/tmp/gc"
	env["PYTHON_BIN"] = "/usr/bin/python3"
	env["WORKER_SCRIPT"] = "/srv/worker.py"
	env["ANTHROPIC_API_KEY"] = "sk-ant"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.DBPath != "/tmp/x.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/tmp/x.db")
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.GarminEmail != "you@example.com" {
		t.Errorf("GarminEmail = %q, want %q", cfg.GarminEmail, "you@example.com")
	}
	if cfg.PythonBin != "/usr/bin/python3" {
		t.Errorf("PythonBin = %q, want %q", cfg.PythonBin, "/usr/bin/python3")
	}
	if cfg.AnthropicAPIKey != "sk-ant" {
		t.Errorf("AnthropicAPIKey = %q, want %q", cfg.AnthropicAPIKey, "sk-ant")
	}
}

func TestLoadMissingRequired(t *testing.T) {
	env := requiredEnv()
	delete(env, "API_TOKEN")
	setEnv(t, env)

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for missing API_TOKEN")
	}
}

func TestLoadM1Defaults(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.ClaudeBin != "claude" {
		t.Errorf("ClaudeBin = %q, want default %q", cfg.ClaudeBin, "claude")
	}
	if cfg.ClaudeModel != "claude-opus-4-8" {
		t.Errorf("ClaudeModel = %q, want default %q", cfg.ClaudeModel, "claude-opus-4-8")
	}
	if cfg.ImageDir != "./data/crossfit" {
		t.Errorf("ImageDir = %q, want default %q", cfg.ImageDir, "./data/crossfit")
	}
}

func TestLoadM1Explicit(t *testing.T) {
	env := requiredEnv()
	env["CLAUDE_BIN"] = "/usr/local/bin/claude"
	env["CLAUDE_MODEL"] = "claude-opus-4-8"
	env["IMAGE_DIR"] = "/srv/data/cf"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.ClaudeBin != "/usr/local/bin/claude" {
		t.Errorf("ClaudeBin = %q, want %q", cfg.ClaudeBin, "/usr/local/bin/claude")
	}
	if cfg.ImageDir != "/srv/data/cf" {
		t.Errorf("ImageDir = %q, want %q", cfg.ImageDir, "/srv/data/cf")
	}
}

func TestM2ConfigDefaults(t *testing.T) {
	t.Setenv("API_TOKEN", "tok")

	var c Config
	if err := envconfig.Process("", &c); err != nil {
		t.Fatalf("envconfig.Process error = %v", err)
	}
	if c.AgentEnabledDefault != true {
		t.Errorf("AgentEnabledDefault = %v, want true", c.AgentEnabledDefault)
	}
	if c.AgentRunTime != "05:30" {
		t.Errorf("AgentRunTime = %q, want 05:30", c.AgentRunTime)
	}
	if c.AgentTimezone != "UTC" {
		t.Errorf("AgentTimezone = %q, want UTC", c.AgentTimezone)
	}
	if c.AgentTickInterval != "1m" {
		t.Errorf("AgentTickInterval = %q, want 1m", c.AgentTickInterval)
	}
	if c.ExpoPushBaseURL != "https://exp.host" {
		t.Errorf("ExpoPushBaseURL = %q, want https://exp.host", c.ExpoPushBaseURL)
	}
}

func TestM3_2StreamConfigDefaults(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.StreamRecentWeeks != 12 {
		t.Errorf("StreamRecentWeeks = %d, want default 12", cfg.StreamRecentWeeks)
	}
	if cfg.StreamFetchBudget != 10 {
		t.Errorf("StreamFetchBudget = %d, want default 10", cfg.StreamFetchBudget)
	}
}

func TestM3_2StreamConfigOverrides(t *testing.T) {
	env := requiredEnv()
	env["STREAM_RECENT_WEEKS"] = "8"
	env["STREAM_FETCH_BUDGET"] = "25"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.StreamRecentWeeks != 8 {
		t.Errorf("StreamRecentWeeks = %d, want 8", cfg.StreamRecentWeeks)
	}
	if cfg.StreamFetchBudget != 25 {
		t.Errorf("StreamFetchBudget = %d, want 25", cfg.StreamFetchBudget)
	}
}

func TestM2ConfigOverrides(t *testing.T) {
	t.Setenv("API_TOKEN", "tok")
	t.Setenv("AGENT_ENABLED", "false")
	t.Setenv("AGENT_RUN_TIME", "06:00")
	t.Setenv("AGENT_TZ", "Asia/Seoul")
	t.Setenv("EXPO_PUSH_BASE_URL", "http://localhost:9999")

	var c Config
	if err := envconfig.Process("", &c); err != nil {
		t.Fatalf("envconfig.Process error = %v", err)
	}
	if c.AgentEnabledDefault != false || c.AgentRunTime != "06:00" ||
		c.AgentTimezone != "Asia/Seoul" || c.ExpoPushBaseURL != "http://localhost:9999" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestM3_3ChatConfigDefault(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.ChatHistoryTurns != 6 {
		t.Errorf("ChatHistoryTurns = %d, want default 6", cfg.ChatHistoryTurns)
	}
}

func TestM3_3ChatConfigOverride(t *testing.T) {
	env := requiredEnv()
	env["CHAT_HISTORY_TURNS"] = "10"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.ChatHistoryTurns != 10 {
		t.Errorf("ChatHistoryTurns = %d, want 10", cfg.ChatHistoryTurns)
	}
}
