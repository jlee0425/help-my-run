package store

import "testing"

func TestM2MigrationCreatesTables(t *testing.T) {
	s := newTestStore(t)

	wantTables := []string{"device_tokens", "daily_decisions", "agent_runs"}
	for _, tbl := range wantTables {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migrate: %v", tbl, err)
		}
	}

	var idx string
	if err := s.DB.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_agent_runs_last_run_date'`,
	).Scan(&idx); err != nil {
		t.Errorf("idx_agent_runs_last_run_date not found: %v", err)
	}
}

func TestM2MigrationAddsProfileColumns(t *testing.T) {
	s := newTestStore(t)

	var runTime, tz string
	var enabled int64
	if err := s.DB.QueryRow(
		`SELECT daily_run_time, timezone, agent_enabled FROM athlete_profile WHERE id = 1`,
	).Scan(&runTime, &tz, &enabled); err != nil {
		t.Fatalf("scan new profile columns: %v", err)
	}
	if runTime != "05:30" || tz != "UTC" || enabled != 1 {
		t.Errorf("defaults = (%q,%q,%d), want (05:30,UTC,1)", runTime, tz, enabled)
	}
}
