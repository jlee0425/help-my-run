package store

import "database/sql"

// SyncLog is one per-source row of sync state (source PK).
type SyncLog struct {
	Source       string
	LastSyncedAt *string // ISO-8601 UTC, nil if never succeeded
	LastRunAt    *string // ISO-8601 UTC, nil if never attempted
	Status       string  // "ok" | "error" | "never"
	Error        *string // non-nil only when Status=="error"
}

// GetSyncLog returns the sync_log row for source, or ErrNotFound.
func (s *Store) GetSyncLog(source string) (SyncLog, error) {
	var sl SyncLog
	var lastSynced, lastRun, errMsg sql.NullString
	err := s.DB.QueryRow(`
		SELECT source, last_synced_at, last_run_at, status, error
		FROM sync_log WHERE source = ?`, source).
		Scan(&sl.Source, &lastSynced, &lastRun, &sl.Status, &errMsg)
	if err == sql.ErrNoRows {
		return SyncLog{}, ErrNotFound
	}
	if err != nil {
		return SyncLog{}, err
	}
	if lastSynced.Valid {
		sl.LastSyncedAt = &lastSynced.String
	}
	if lastRun.Valid {
		sl.LastRunAt = &lastRun.String
	}
	if errMsg.Valid {
		sl.Error = &errMsg.String
	}
	return sl, nil
}

// UpdateSyncLog upserts the sync_log row for sl.Source.
func (s *Store) UpdateSyncLog(sl SyncLog) error {
	_, err := s.DB.Exec(`
		INSERT INTO sync_log (source, last_synced_at, last_run_at, status, error)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(source) DO UPDATE SET
			last_synced_at = excluded.last_synced_at,
			last_run_at    = excluded.last_run_at,
			status         = excluded.status,
			error          = excluded.error`,
		sl.Source, sl.LastSyncedAt, sl.LastRunAt, sl.Status, sl.Error)
	return err
}
