// Package store owns the SQLite database: opening it (modernc, WAL), running
// embedded goose migrations, and typed query/upsert functions per table.
package store

import (
	"database/sql"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Store wraps the single *sql.DB used by the whole backend (one writer).
type Store struct {
	DB *sql.DB
}

// Open opens (creating if needed) the SQLite database at dbPath with WAL mode,
// foreign keys on, and a busy timeout. The connection pool is capped at one
// open connection because SQLite allows a single writer.
func Open(dbPath string) (*Store, error) {
	dsn := "file:" + dbPath +
		"?_pragma=journal_mode(WAL)" +
		"&_pragma=foreign_keys(ON)" +
		"&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.DB.Close()
}
