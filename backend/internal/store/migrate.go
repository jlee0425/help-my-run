package store

import (
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Migrate runs all pending goose migrations against the store. The goose
// dialect is "sqlite3" even though the sql.Open driver is "sqlite" (modernc);
// the two names are independent. Migrate is idempotent.
func (s *Store) Migrate() error {
	goose.SetBaseFS(embedMigrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return err
	}
	return goose.Up(s.DB, "migrations")
}
