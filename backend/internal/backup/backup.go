// Package backup takes nightly snapshots of the SQLite database and the Garmin
// tokenstore into a rotation-pruned directory (M6). A restore is: stop the
// service, copy the snapshot over DB_PATH, copy the tokenstore dir back, start.
package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"help-my-run/backend/internal/store"
)

// Run snapshots db into dir as helpmyrun-YYYY-MM-DD.db (VACUUM INTO — atomic
// and consistent under WAL), copies the Garmin tokenstore alongside it, and
// prunes both sets to the newest keep. It returns the snapshot path.
func Run(db *store.Store, tokenstore, dir string, keep int) (string, error) {
	return runAt(db, tokenstore, dir, keep, time.Now().UTC())
}

func runAt(db *store.Store, tokenstore, dir string, keep int, at time.Time) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("backup dir: %w", err)
	}
	if err := sweepTemp(dir); err != nil {
		return "", err
	}
	date := at.Format("2006-01-02")

	// Snapshot to a temp name, then rename: the final dated path only ever
	// holds a COMPLETE snapshot, so a crash mid-VACUUM can't leave a partial
	// file that a later restore would trust. (VACUUM INTO also refuses to
	// overwrite, so the temp file doubles as the same-day-rerun path.)
	snapshot := filepath.Join(dir, "helpmyrun-"+date+".db")
	tmp := snapshot + ".tmp"
	if _, err := db.DB.Exec(`VACUUM INTO ?`, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("vacuum into %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, snapshot); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("finalize snapshot: %w", err)
	}

	if err := copyTokenstore(tokenstore, filepath.Join(dir, "tokenstore-"+date)); err != nil {
		return "", err
	}

	if err := prune(dir, keep); err != nil {
		return "", err
	}
	return snapshot, nil
}

// sweepTemp removes *.tmp debris a crashed previous run may have left.
func sweepTemp(dir string) error {
	for _, glob := range []string{"helpmyrun-*.db.tmp", "tokenstore-*.tmp"} {
		matches, err := filepath.Glob(filepath.Join(dir, glob))
		if err != nil {
			return fmt.Errorf("sweep glob %s: %w", glob, err)
		}
		for _, m := range matches {
			if err := os.RemoveAll(m); err != nil {
				return fmt.Errorf("sweep %s: %w", m, err)
			}
		}
	}
	return nil
}

// prune deletes all but the newest keep snapshots and tokenstore copies. The
// date-stamped names sort chronologically, so lexical order is age order.
// keep <= 0 disables pruning. Files that aren't ours are never touched.
func prune(dir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	for _, glob := range []string{"helpmyrun-*.db", "tokenstore-*"} {
		matches, err := filepath.Glob(filepath.Join(dir, glob))
		if err != nil {
			return fmt.Errorf("prune glob %s: %w", glob, err)
		}
		sort.Strings(matches)
		for _, old := range matches[:max(0, len(matches)-keep)] {
			if err := os.RemoveAll(old); err != nil {
				return fmt.Errorf("prune %s: %w", old, err)
			}
		}
	}
	return nil
}

// copyTokenstore copies the flat garminconnect token dir to dst, staging into
// dst.tmp and swapping at the end so dst is never a partial copy. A missing
// source is not an error — the owner just hasn't logged into Garmin yet.
func copyTokenstore(src, dst string) error {
	if src == "" {
		return nil
	}
	entries, err := os.ReadDir(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read tokenstore: %w", err)
	}
	tmp := dst + ".tmp"
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return fmt.Errorf("tokenstore copy dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue // garminconnect writes a flat dir; skip anything odd
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return fmt.Errorf("read token %s: %w", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(tmp, e.Name()), b, 0o600); err != nil {
			return fmt.Errorf("write token %s: %w", e.Name(), err)
		}
	}
	// Same-day rerun: replace wholesale so the copy never mixes generations.
	if err := os.RemoveAll(dst); err != nil {
		return fmt.Errorf("clear stale tokenstore copy: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("finalize tokenstore copy: %w", err)
	}
	return nil
}
