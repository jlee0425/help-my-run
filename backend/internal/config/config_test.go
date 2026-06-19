package config

import (
	"os"
	"testing"
)

// setEnv sets the given env vars for the duration of the test and TRULY UNSETS
// any others that Load reads, so tests are hermetic. We must Unsetenv (not set
// to "") because envconfig's required check uses os.LookupEnv: a set-but-empty
// var counts as PRESENT and would defeat TestLoadMissingRequired.
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	all := []string{
		"STRAVA_CLIENT_ID", "STRAVA_CLIENT_SECRET", "STRAVA_REDIRECT_URL",
		"API_TOKEN", "DB_PATH", "PORT",
		"GARMIN_EMAIL", "GARMIN_PASSWORD", "GARMIN_TOKENSTORE",
		"PYTHON_BIN", "WORKER_SCRIPT", "ANTHROPIC_API_KEY",
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
		"STRAVA_CLIENT_ID":     "123456",
		"STRAVA_CLIENT_SECRET": "secret",
		"STRAVA_REDIRECT_URL":  "http://localhost:8080/api/strava/callback",
		"API_TOKEN":            "tok",
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.StravaClientID != "123456" {
		t.Errorf("StravaClientID = %q, want %q", cfg.StravaClientID, "123456")
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
