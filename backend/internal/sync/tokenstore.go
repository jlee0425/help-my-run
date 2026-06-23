package sync

import (
	"os"
	"path/filepath"
	"strings"
)

// defaultTokenStore mirrors config.Config.GarminTokenstore's default and is used
// when no GARMIN_TOKENSTORE entry is present in an env slice.
const defaultTokenStore = "~/.garminconnect"

// tokenStoreEnvKey is the env var the worker subprocess reads to locate the
// garminconnect OAuth token directory.
const tokenStoreEnvKey = "GARMIN_TOKENSTORE="

// TokenStorePathFromEnv scans an env slice (KEY=VALUE entries, as passed to the
// worker subprocess) for GARMIN_TOKENSTORE and returns its value, or the default
// "~/.garminconnect" when absent. An explicit empty value is returned as-is.
func TokenStorePathFromEnv(env []string) string {
	for _, e := range env {
		if strings.HasPrefix(e, tokenStoreEnvKey) {
			return strings.TrimPrefix(e, tokenStoreEnvKey)
		}
	}
	return defaultTokenStore
}

// TokenStoreReady reports whether the Garmin token store at path is usable: it
// expands a leading "~" to the user home dir, then returns true IFF that path is
// an existing directory containing at least one entry. garminconnect writes
// OAuth token files into this directory after a successful login, so a populated
// directory means the backend can resume a session without hitting Garmin's
// per-IP login rate limit. A missing/empty/unreadable dir, a non-directory path,
// or a failure to resolve "~" all yield false.
func TokenStoreReady(path string) bool {
	expanded, ok := expandHome(path)
	if !ok {
		return false
	}
	entries, err := os.ReadDir(expanded)
	if err != nil {
		// Missing dir, not-a-dir, or unreadable: not ready.
		return false
	}
	return len(entries) > 0
}

// expandHome replaces a leading "~" (alone or before a path separator) with the
// user home dir. It returns (path, false) when the home dir cannot be resolved.
func expandHome(path string) (string, bool) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path, false
		}
		if path == "~" {
			return home, true
		}
		return filepath.Join(home, path[len("~/"):]), true
	}
	return path, true
}
