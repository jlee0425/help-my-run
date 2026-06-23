package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTokenStorePathFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  []string
		want string
	}{
		{
			name: "present returns value",
			env:  []string{"FOO=bar", "GARMIN_TOKENSTORE=/tmp/gc", "BAZ=qux"},
			want: "/tmp/gc",
		},
		{
			name: "absent returns default",
			env:  []string{"FOO=bar", "BAZ=qux"},
			want: "~/.garminconnect",
		},
		{
			name: "nil env returns default",
			env:  nil,
			want: "~/.garminconnect",
		},
		{
			name: "empty value returns empty (explicit override)",
			env:  []string{"GARMIN_TOKENSTORE="},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TokenStorePathFromEnv(tt.env); got != tt.want {
				t.Errorf("TokenStorePathFromEnv(%v) = %q, want %q", tt.env, got, tt.want)
			}
		})
	}
}

func TestTokenStoreReady_NonexistentPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist")
	if TokenStoreReady(path) {
		t.Errorf("TokenStoreReady(%q) = true, want false (missing dir)", path)
	}
}

func TestTokenStoreReady_EmptyDir(t *testing.T) {
	dir := t.TempDir() // exists but empty
	if TokenStoreReady(dir) {
		t.Errorf("TokenStoreReady(%q) = true, want false (empty dir)", dir)
	}
}

func TestTokenStoreReady_DirWithFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "oauth1_token.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	if !TokenStoreReady(dir) {
		t.Errorf("TokenStoreReady(%q) = false, want true (dir with token file)", dir)
	}
}

func TestTokenStoreReady_FileNotDir(t *testing.T) {
	// A plain file (not a directory) is not a ready token store.
	f := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if TokenStoreReady(f) {
		t.Errorf("TokenStoreReady(%q) = true, want false (path is a file)", f)
	}
}

func TestTokenStoreReady_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	// Create a populated dir under HOME, reference it via a leading "~".
	base := ".helpmyrun-tokenstore-test"
	abs := filepath.Join(home, base)
	if err := os.MkdirAll(abs, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(abs) })
	if err := os.WriteFile(filepath.Join(abs, "tok"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	tilde := "~/" + base
	if !TokenStoreReady(tilde) {
		t.Errorf("TokenStoreReady(%q) = false, want true (~ should expand to %q)", tilde, abs)
	}

	// And an empty ~-prefixed dir resolves but is not ready.
	emptyBase := ".helpmyrun-tokenstore-empty-test"
	emptyAbs := filepath.Join(home, emptyBase)
	if err := os.MkdirAll(emptyAbs, 0o700); err != nil {
		t.Fatalf("mkdir empty: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(emptyAbs) })
	if TokenStoreReady("~/" + emptyBase) {
		t.Errorf("TokenStoreReady(~/%s) = true, want false (empty resolved dir)", emptyBase)
	}
}
