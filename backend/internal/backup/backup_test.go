package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
)

// newTestStore opens a migrated store in a temp dir with one recognizable row.
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "live.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if err := s.SetSetting("backup_canary", "chirp"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	return s
}

func TestRunSnapshotIsAReadableDatabase(t *testing.T) {
	s := newTestStore(t)
	dir := filepath.Join(t.TempDir(), "backups")

	at := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)
	path, err := runAt(s, "", dir, 14, at)
	if err != nil {
		t.Fatalf("runAt: %v", err)
	}
	if want := filepath.Join(dir, "helpmyrun-2026-07-10.db"); path != want {
		t.Fatalf("snapshot path = %q, want %q", path, want)
	}

	// The snapshot must be a consistent, openable database with the data in it.
	snap, err := store.Open(path)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = snap.Close() }()
	got, err := snap.GetSetting("backup_canary")
	if err != nil || got != "chirp" {
		t.Fatalf("snapshot canary = %q,%v want chirp,nil", got, err)
	}
}

func TestRunSameDayRerunReplacesSnapshot(t *testing.T) {
	s := newTestStore(t)
	dir := filepath.Join(t.TempDir(), "backups")
	at := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)

	if _, err := runAt(s, "", dir, 14, at); err != nil {
		t.Fatalf("first runAt: %v", err)
	}
	// A rerun on the same date (restart, forced run) must not fail on the
	// existing file — VACUUM INTO refuses to overwrite.
	if err := s.SetSetting("backup_canary", "second"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	path, err := runAt(s, "", dir, 14, at)
	if err != nil {
		t.Fatalf("second runAt: %v", err)
	}
	snap, err := store.Open(path)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = snap.Close() }()
	if got, _ := snap.GetSetting("backup_canary"); got != "second" {
		t.Fatalf("rerun snapshot canary = %q, want second (fresh snapshot)", got)
	}
}

func TestRunCopiesTokenstore(t *testing.T) {
	s := newTestStore(t)
	dir := filepath.Join(t.TempDir(), "backups")

	tokenstore := t.TempDir()
	for _, name := range []string{"oauth1_token.json", "oauth2_token.json"} {
		if err := os.WriteFile(filepath.Join(tokenstore, name), []byte(`{"tok":"`+name+`"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	at := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)
	if _, err := runAt(s, tokenstore, dir, 14, at); err != nil {
		t.Fatalf("runAt: %v", err)
	}

	copied := filepath.Join(dir, "tokenstore-2026-07-10")
	for _, name := range []string{"oauth1_token.json", "oauth2_token.json"} {
		b, err := os.ReadFile(filepath.Join(copied, name))
		if err != nil {
			t.Fatalf("copied token %s: %v", name, err)
		}
		if want := `{"tok":"` + name + `"}`; string(b) != want {
			t.Errorf("copied %s = %q, want %q", name, b, want)
		}
	}
}

func TestRunSkipsMissingTokenstore(t *testing.T) {
	s := newTestStore(t)
	dir := filepath.Join(t.TempDir(), "backups")
	at := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)

	// Not logged into Garmin yet: no tokenstore is not an error, and no
	// tokenstore-* dir appears. The DB snapshot still happens.
	if _, err := runAt(s, filepath.Join(t.TempDir(), "absent"), dir, 14, at); err != nil {
		t.Fatalf("runAt with missing tokenstore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tokenstore-2026-07-10")); !os.IsNotExist(err) {
		t.Fatalf("tokenstore dir created for a missing source (stat err = %v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "helpmyrun-2026-07-10.db")); err != nil {
		t.Fatalf("db snapshot missing when tokenstore absent: %v", err)
	}
}

func TestRunPrunesToNewestKeep(t *testing.T) {
	s := newTestStore(t)
	dir := filepath.Join(t.TempDir(), "backups")
	tokenstore := t.TempDir()
	if err := os.WriteFile(filepath.Join(tokenstore, "oauth1_token.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Five nightly runs, keep 3: only the newest 3 snapshot+tokenstore pairs
	// survive. An unrelated file in the dir is left alone.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	for day := 6; day <= 10; day++ {
		at := time.Date(2026, 7, day, 6, 0, 0, 0, time.UTC)
		if _, err := runAt(s, tokenstore, dir, 3, at); err != nil {
			t.Fatalf("runAt day %d: %v", day, err)
		}
	}

	for day, want := range map[int]bool{6: false, 7: false, 8: true, 9: true, 10: true} {
		date := time.Date(2026, 7, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		_, dbErr := os.Stat(filepath.Join(dir, "helpmyrun-"+date+".db"))
		_, tsErr := os.Stat(filepath.Join(dir, "tokenstore-"+date))
		if got := dbErr == nil; got != want {
			t.Errorf("snapshot %s present = %v, want %v", date, got, want)
		}
		if got := tsErr == nil; got != want {
			t.Errorf("tokenstore copy %s present = %v, want %v", date, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "README.txt")); err != nil {
		t.Errorf("unrelated file pruned: %v", err)
	}
}

func TestRunKeepZeroMeansNoPruning(t *testing.T) {
	s := newTestStore(t)
	dir := filepath.Join(t.TempDir(), "backups")
	for day := 8; day <= 10; day++ {
		at := time.Date(2026, 7, day, 6, 0, 0, 0, time.UTC)
		if _, err := runAt(s, "", dir, 0, at); err != nil {
			t.Fatalf("runAt day %d: %v", day, err)
		}
	}
	entries, err := filepath.Glob(filepath.Join(dir, "helpmyrun-*.db"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("snapshots = %d, want all 3 kept (keep<=0 disables pruning)", len(entries))
	}
}

func TestRunFailsWhenDirUncreatable(t *testing.T) {
	s := newTestStore(t)
	// A file where the backup dir should be makes MkdirAll fail.
	blocked := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runAt(s, "", blocked, 14, time.Now().UTC()); err == nil {
		t.Fatal("runAt into a non-directory succeeded, want error")
	}
}

// M6 review: a crash mid-backup must never leave a partial file at the final
// dated path (a restore would trust it). Snapshots are written to a temp name
// and renamed; failures clean up.
func TestRunLeavesNoPartialsOnFailureAndNoTempOnSuccess(t *testing.T) {
	s := newTestStore(t)
	dir := filepath.Join(t.TempDir(), "backups")
	tokenstore := t.TempDir()
	if err := os.WriteFile(filepath.Join(tokenstore, "oauth1_token.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 7, 10, 6, 0, 0, 0, time.UTC)

	// Seed debris a previous crash could have left mid-write.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "helpmyrun-2026-07-09.db.tmp"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "tokenstore-2026-07-09.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}

	// Success: exactly the two artifacts; the crash debris was swept.
	if _, err := runAt(s, tokenstore, dir, 14, at); err != nil {
		t.Fatalf("runAt: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dir after success = %v, want [helpmyrun-2026-07-10.db tokenstore-2026-07-10]", names)
	}

	// Failure (closed DB): error out, and the successful day-1 artifacts are
	// untouched — no partial day-2 snapshot appears at the final path.
	_ = s.Close()
	at2 := time.Date(2026, 7, 11, 6, 0, 0, 0, time.UTC)
	if _, err := runAt(s, tokenstore, dir, 14, at2); err == nil {
		t.Fatal("runAt on a closed DB succeeded, want error")
	}
	if _, err := os.Stat(filepath.Join(dir, "helpmyrun-2026-07-11.db")); !os.IsNotExist(err) {
		t.Fatalf("partial snapshot left at final path after failure (stat err = %v)", err)
	}
	entries, err = os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dir after failure = %v, want only day-1 artifacts", names)
	}
}
