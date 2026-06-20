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

func TestDeviceTokenUpsertListDelete(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.ListDeviceTokens(); err != nil {
		t.Fatalf("ListDeviceTokens on empty error = %v", err)
	}

	in := DeviceToken{ExpoPushToken: "ExponentPushToken[abc]", Platform: "ios", UpdatedAt: "set-by-store"}
	if err := s.UpsertDeviceToken(in); err != nil {
		t.Fatalf("UpsertDeviceToken error = %v", err)
	}
	toks, err := s.ListDeviceTokens()
	if err != nil {
		t.Fatalf("ListDeviceTokens error = %v", err)
	}
	if len(toks) != 1 || toks[0].ExpoPushToken != "ExponentPushToken[abc]" || toks[0].Platform != "ios" {
		t.Fatalf("tokens = %+v, want one ios token", toks)
	}
	if toks[0].UpdatedAt == "" || toks[0].UpdatedAt == "set-by-store" {
		t.Errorf("UpdatedAt = %q, want server-set", toks[0].UpdatedAt)
	}

	in.Platform = "android"
	if err := s.UpsertDeviceToken(in); err != nil {
		t.Fatalf("re-UpsertDeviceToken error = %v", err)
	}
	toks, _ = s.ListDeviceTokens()
	if len(toks) != 1 || toks[0].Platform != "android" {
		t.Errorf("after re-upsert = %+v, want one android token", toks)
	}

	if err := s.DeleteDeviceToken("ExponentPushToken[abc]"); err != nil {
		t.Fatalf("DeleteDeviceToken error = %v", err)
	}
	toks, _ = s.ListDeviceTokens()
	if len(toks) != 0 {
		t.Errorf("after delete len = %d, want 0", len(toks))
	}
	if err := s.DeleteDeviceToken("ExponentPushToken[missing]"); err != nil {
		t.Errorf("DeleteDeviceToken(missing) error = %v, want nil", err)
	}
}

func TestDailyDecisionUpsertGet(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.GetDailyDecision("2026-06-20"); err != ErrNotFound {
		t.Fatalf("GetDailyDecision on empty = %v, want ErrNotFound", err)
	}

	orig := `{"date":"2026-06-20","run_type":"tempo","distance_km":6}`
	adj := `{"date":"2026-06-20","run_type":"easy","distance_km":4.5}`
	raw := `{"action":"SOFTEN"}`
	in := DailyDecision{
		Date:                "2026-06-20",
		ReadinessColor:      "amber",
		DriversJSON:         `{"date":"2026-06-20","data_complete":true}`,
		OriginalSessionJSON: &orig,
		AdjustedSessionJSON: &adj,
		Action:              "SOFTEN",
		Rationale:           "HRV down; trimmed tempo to easy.",
		Source:              "ai",
		RawResponse:         &raw,
		CreatedAt:           "set-by-store",
		UpdatedAt:           "set-by-store",
	}
	if err := s.UpsertDailyDecision(in); err != nil {
		t.Fatalf("UpsertDailyDecision error = %v", err)
	}

	got, err := s.GetDailyDecision("2026-06-20")
	if err != nil {
		t.Fatalf("GetDailyDecision error = %v", err)
	}
	if got.ReadinessColor != "amber" || got.Action != "SOFTEN" || got.Source != "ai" {
		t.Errorf("got = %+v, want amber/SOFTEN/ai", got)
	}
	if got.OriginalSessionJSON == nil || *got.OriginalSessionJSON != orig {
		t.Errorf("OriginalSessionJSON = %v, want %q", got.OriginalSessionJSON, orig)
	}
	if got.AdjustedSessionJSON == nil || *got.AdjustedSessionJSON != adj {
		t.Errorf("AdjustedSessionJSON = %v, want %q", got.AdjustedSessionJSON, adj)
	}
	if got.RawResponse == nil || *got.RawResponse != raw {
		t.Errorf("RawResponse = %v, want %q", got.RawResponse, raw)
	}
	if got.CreatedAt == "" || got.CreatedAt == "set-by-store" {
		t.Errorf("CreatedAt = %q, want server-set", got.CreatedAt)
	}
	createdFirst := got.CreatedAt

	in.AdjustedSessionJSON = &orig
	in.Action = "STAND"
	in.Rationale = "Reverted to original session."
	if err := s.UpsertDailyDecision(in); err != nil {
		t.Fatalf("second UpsertDailyDecision error = %v", err)
	}
	got, _ = s.GetDailyDecision("2026-06-20")
	if got.Action != "STAND" || got.AdjustedSessionJSON == nil || *got.AdjustedSessionJSON != orig {
		t.Errorf("after re-upsert = %+v, want STAND with adjusted==original", got)
	}
	if got.CreatedAt != createdFirst {
		t.Errorf("CreatedAt changed on update: %q -> %q", createdFirst, got.CreatedAt)
	}

	in2 := DailyDecision{
		Date:           "2026-06-21",
		ReadinessColor: "green",
		DriversJSON:    `{"date":"2026-06-21"}`,
		Action:         "REST_DAY",
		Rationale:      "Rest day as planned.",
		Source:         "fallback",
	}
	if err := s.UpsertDailyDecision(in2); err != nil {
		t.Fatalf("UpsertDailyDecision rest error = %v", err)
	}
	got2, _ := s.GetDailyDecision("2026-06-21")
	if got2.OriginalSessionJSON != nil || got2.AdjustedSessionJSON != nil || got2.RawResponse != nil {
		t.Errorf("rest day = %+v, want nil session/raw pointers", got2)
	}
}

func TestAgentRunUpsertGetLatest(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.LatestAgentRun(); err != ErrNotFound {
		t.Fatalf("LatestAgentRun on empty = %v, want ErrNotFound", err)
	}
	if _, err := s.GetAgentRun("2026-06-20"); err != ErrNotFound {
		t.Fatalf("GetAgentRun on empty = %v, want ErrNotFound", err)
	}

	if err := s.UpsertAgentRun(AgentRun{
		LastRunDate: "2026-06-20", Status: "ok", RanAt: "set-by-store",
	}); err != nil {
		t.Fatalf("UpsertAgentRun error = %v", err)
	}
	got, err := s.GetAgentRun("2026-06-20")
	if err != nil {
		t.Fatalf("GetAgentRun error = %v", err)
	}
	if got.LastRunDate != "2026-06-20" || got.Status != "ok" {
		t.Errorf("got = %+v, want 2026-06-20/ok", got)
	}
	if got.RanAt == "" || got.RanAt == "set-by-store" {
		t.Errorf("RanAt = %q, want server-set", got.RanAt)
	}

	errMsg := "claude timeout"
	if err := s.UpsertAgentRun(AgentRun{
		LastRunDate: "2026-06-20", Status: "error", Error: &errMsg, RanAt: "x",
	}); err != nil {
		t.Fatalf("re-UpsertAgentRun error = %v", err)
	}
	got, _ = s.GetAgentRun("2026-06-20")
	if got.Status != "error" || got.Error == nil || *got.Error != errMsg {
		t.Errorf("after re-upsert = %+v, want status=error error=%q", got, errMsg)
	}
	var n int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM agent_runs`).Scan(&n)
	if n != 1 {
		t.Errorf("agent_runs count = %d, want 1 (same last_run_date)", n)
	}

	if err := s.UpsertAgentRun(AgentRun{LastRunDate: "2026-06-21", Status: "ok", RanAt: "y"}); err != nil {
		t.Fatalf("UpsertAgentRun 21 error = %v", err)
	}
	latest, err := s.LatestAgentRun()
	if err != nil {
		t.Fatalf("LatestAgentRun error = %v", err)
	}
	if latest.LastRunDate != "2026-06-21" {
		t.Errorf("LatestAgentRun = %+v, want 2026-06-21", latest)
	}

	// DeleteAgentRun resets the persistent once-per-day guard (force path).
	if err := s.DeleteAgentRun("2026-06-20"); err != nil {
		t.Fatalf("DeleteAgentRun error = %v", err)
	}
	if _, err := s.GetAgentRun("2026-06-20"); err != ErrNotFound {
		t.Errorf("GetAgentRun after delete = %v, want ErrNotFound", err)
	}
	if _, err := s.GetAgentRun("2026-06-21"); err != nil {
		t.Errorf("DeleteAgentRun removed the wrong row: 2026-06-21 err = %v, want nil", err)
	}
	if err := s.DeleteAgentRun("2026-06-20"); err != nil {
		t.Errorf("DeleteAgentRun(missing) error = %v, want nil (no-op)", err)
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
