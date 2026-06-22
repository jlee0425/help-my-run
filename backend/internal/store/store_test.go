package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// newTestStore opens a fresh, migrated store in a temp dir. Shared by all
// store tests.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", dbPath, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return s
}

func TestOpenAndMigrate(t *testing.T) {
	s := newTestStore(t)

	wantTables := []string{
		"strava_tokens", "activities", "activity_splits",
		"garmin_sleep", "garmin_hrv", "garmin_body_battery", "garmin_rhr",
		"garmin_vo2max",
		"sync_log",
		"activity_streams", "stream_analyses", "stream_fetch_log",
		"garmin_activities",
	}
	for _, tbl := range wantTables {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migrate: %v", tbl, err)
		}
	}
}

func TestMigrateSeedsSyncLog(t *testing.T) {
	s := newTestStore(t)

	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM sync_log`).Scan(&n); err != nil {
		t.Fatalf("count sync_log: %v", err)
	}
	if n != 2 {
		t.Errorf("sync_log row count = %d, want 2 (strava, garmin)", n)
	}
	for _, src := range []string{"strava", "garmin"} {
		var status string
		err := s.DB.QueryRow(`SELECT status FROM sync_log WHERE source=?`, src).Scan(&status)
		if err != nil {
			t.Errorf("sync_log source %q missing: %v", src, err)
			continue
		}
		if status != "never" {
			t.Errorf("sync_log[%q].status = %q, want %q", src, status, "never")
		}
	}
}

func TestMigrateIdempotent(t *testing.T) {
	s := newTestStore(t)
	// Second Migrate() must be a no-op, not an error.
	if err := s.Migrate(); err != nil {
		t.Fatalf("second Migrate() error = %v, want nil", err)
	}
}

func TestStravaTokensRoundTrip(t *testing.T) {
	s := newTestStore(t)

	// Not connected yet.
	if _, err := s.GetStravaTokens(); err != ErrNotFound {
		t.Fatalf("GetStravaTokens() on empty = %v, want ErrNotFound", err)
	}

	in := StravaTokens{
		AccessToken:  "acc",
		RefreshToken: "ref",
		ExpiresAt:    1737000000,
		Scope:        "read,activity:read_all",
		AthleteID:    12345678,
	}
	if err := s.SaveStravaTokens(in); err != nil {
		t.Fatalf("SaveStravaTokens() error = %v", err)
	}

	got, err := s.GetStravaTokens()
	if err != nil {
		t.Fatalf("GetStravaTokens() error = %v", err)
	}
	if got.AccessToken != in.AccessToken || got.RefreshToken != in.RefreshToken ||
		got.ExpiresAt != in.ExpiresAt || got.Scope != in.Scope || got.AthleteID != in.AthleteID {
		t.Errorf("GetStravaTokens() = %+v, want %+v", got, in)
	}

	// Overwrite (id is always 1).
	in.AccessToken = "acc2"
	in.ExpiresAt = 1737099999
	if err := s.SaveStravaTokens(in); err != nil {
		t.Fatalf("SaveStravaTokens() overwrite error = %v", err)
	}
	got, _ = s.GetStravaTokens()
	if got.AccessToken != "acc2" || got.ExpiresAt != 1737099999 {
		t.Errorf("after overwrite got %+v, want AccessToken=acc2 ExpiresAt=1737099999", got)
	}

	var rows int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM strava_tokens`).Scan(&rows); err != nil {
		t.Fatalf("count strava_tokens: %v", err)
	}
	if rows != 1 {
		t.Errorf("strava_tokens row count = %d, want 1 (single-row table)", rows)
	}
}

func TestSyncLogGetAndUpdate(t *testing.T) {
	s := newTestStore(t)

	// Seeded default.
	sl, err := s.GetSyncLog("strava")
	if err != nil {
		t.Fatalf("GetSyncLog(strava) error = %v", err)
	}
	if sl.Status != "never" || sl.LastSyncedAt != nil || sl.Error != nil {
		t.Errorf("seed sync_log = %+v, want status=never, nils", sl)
	}

	syncedAt := "2026-06-19T05:00:30Z"
	upd := SyncLog{
		Source:       "strava",
		LastSyncedAt: &syncedAt,
		LastRunAt:    &syncedAt,
		Status:       "ok",
		Error:        nil,
	}
	if err := s.UpdateSyncLog(upd); err != nil {
		t.Fatalf("UpdateSyncLog() error = %v", err)
	}
	got, _ := s.GetSyncLog("strava")
	if got.Status != "ok" || got.LastSyncedAt == nil || *got.LastSyncedAt != syncedAt {
		t.Errorf("after update got %+v, want status=ok last_synced_at=%s", got, syncedAt)
	}

	// Error path keeps last_synced_at nil but sets error.
	errMsg := "worker exit 1: re-run worker.py login"
	if err := s.UpdateSyncLog(SyncLog{
		Source: "garmin", LastSyncedAt: nil, LastRunAt: &syncedAt,
		Status: "error", Error: &errMsg,
	}); err != nil {
		t.Fatalf("UpdateSyncLog(garmin error) error = %v", err)
	}
	gg, _ := s.GetSyncLog("garmin")
	if gg.Status != "error" || gg.Error == nil || *gg.Error != errMsg {
		t.Errorf("garmin sync_log = %+v, want status=error error=%q", gg, errMsg)
	}
}

func f64p(v float64) *float64 { return &v }

func TestUpsertAndListActivities(t *testing.T) {
	s := newTestStore(t)

	a1 := Activity{
		StravaID: 100, Name: "Morning Run", Type: "Run", SportType: strp("Run"),
		StartTime: "2026-06-17T06:00:00Z", StartTimeLocal: strp("2026-06-17T08:00:00"),
		DistanceM: 10000, MovingTimeS: 3000, ElapsedTimeS: 3100,
		AvgHR: f64p(150), MaxHR: f64p(170), AvgSpeed: f64p(3.3), MaxSpeed: f64p(4.9),
		AvgCadence: f64p(86), ElevationGainM: f64p(80),
		RawJSON: `{"id":100}`,
	}
	a2 := Activity{
		StravaID: 200, Name: "Evening Run", Type: "Run", SportType: strp("TrailRun"),
		StartTime: "2026-06-18T18:00:00Z", StartTimeLocal: nil,
		DistanceM: 5000, MovingTimeS: 1500, ElapsedTimeS: 1500,
		AvgHR: nil, MaxHR: nil, AvgSpeed: nil, MaxSpeed: nil,
		AvgCadence: nil, ElevationGainM: nil,
		RawJSON: `{"id":200}`,
	}

	if err := s.UpsertActivity(a1); err != nil {
		t.Fatalf("UpsertActivity(a1) error = %v", err)
	}
	if err := s.UpsertActivity(a2); err != nil {
		t.Fatalf("UpsertActivity(a2) error = %v", err)
	}

	got, err := s.ListActivities(30)
	if err != nil {
		t.Fatalf("ListActivities error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListActivities len = %d, want 2", len(got))
	}
	// Most-recent-first by start_time: a2 (06-18) before a1 (06-17).
	if got[0].StravaID != 200 || got[1].StravaID != 100 {
		t.Errorf("order = [%d,%d], want [200,100]", got[0].StravaID, got[1].StravaID)
	}
	// Nullable preserved on a2.
	if got[0].AvgHR != nil {
		t.Errorf("a2.AvgHR = %v, want nil", got[0].AvgHR)
	}
	// Value preserved on a1.
	if got[1].AvgHR == nil || *got[1].AvgHR != 150 {
		t.Errorf("a1.AvgHR = %v, want 150", got[1].AvgHR)
	}

	// Re-upsert a1 with a changed name -> update, not duplicate.
	a1.Name = "Renamed Run"
	if err := s.UpsertActivity(a1); err != nil {
		t.Fatalf("re-UpsertActivity error = %v", err)
	}
	got, _ = s.ListActivities(30)
	if len(got) != 2 {
		t.Fatalf("after re-upsert len = %d, want 2", len(got))
	}
	for _, a := range got {
		if a.StravaID == 100 && a.Name != "Renamed Run" {
			t.Errorf("a1.Name = %q, want %q", a.Name, "Renamed Run")
		}
	}

	// limit clamps result count.
	lim, _ := s.ListActivities(1)
	if len(lim) != 1 || lim[0].StravaID != 200 {
		t.Errorf("ListActivities(1) = %v, want single [200]", lim)
	}
}

func TestUpsertSplits(t *testing.T) {
	s := newTestStore(t)

	a := Activity{
		StravaID: 300, Name: "Splits Run", Type: "Run", SportType: strp("Run"),
		StartTime: "2026-06-19T06:00:00Z", DistanceM: 4000,
		MovingTimeS: 1200, ElapsedTimeS: 1200, RawJSON: `{"id":300}`,
	}
	if err := s.UpsertActivity(a); err != nil {
		t.Fatalf("UpsertActivity error = %v", err)
	}

	splits := []Split{
		{ActivityID: 300, Idx: 1, DistanceM: 1000, ElapsedTimeS: 300,
			MovingTimeS: i64p(295), AvgHR: f64p(140), MaxHR: f64p(150), AvgSpeed: f64p(3.3)},
		{ActivityID: 300, Idx: 2, DistanceM: 1000, ElapsedTimeS: 305,
			MovingTimeS: nil, AvgHR: nil, MaxHR: nil, AvgSpeed: f64p(3.2)},
	}
	if err := s.UpsertSplits(300, splits); err != nil {
		t.Fatalf("UpsertSplits error = %v", err)
	}

	var n int
	if err := s.DB.QueryRow(
		`SELECT COUNT(*) FROM activity_splits WHERE activity_id=300`).Scan(&n); err != nil {
		t.Fatalf("count splits: %v", err)
	}
	if n != 2 {
		t.Errorf("split count = %d, want 2", n)
	}

	// Re-upsert is idempotent (same PK activity_id+idx).
	if err := s.UpsertSplits(300, splits); err != nil {
		t.Fatalf("re-UpsertSplits error = %v", err)
	}
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activity_splits WHERE activity_id=300`).Scan(&n)
	if n != 2 {
		t.Errorf("after re-upsert split count = %d, want 2", n)
	}
}

func strp(v string) *string { return &v }
func i64p(v int64) *int64   { return &v }

func TestUpsertGarminAndListRecovery(t *testing.T) {
	s := newTestStore(t)

	// 06-18 has all four; 06-17 has sleep + rhr only (hrv & body_battery missing).
	if err := s.UpsertSleep(SleepRow{
		Date: "2026-06-18", DurationS: i64p(27000), DeepS: i64p(6300), LightS: i64p(14400),
		RemS: i64p(5400), AwakeS: i64p(900), Score: i64p(82), RawJSON: `{"d":1}`,
	}); err != nil {
		t.Fatalf("UpsertSleep 18: %v", err)
	}
	if err := s.UpsertSleep(SleepRow{
		Date: "2026-06-17", DurationS: i64p(25800), DeepS: i64p(5400), LightS: i64p(13800),
		RemS: i64p(4800), AwakeS: i64p(1800), Score: i64p(71), RawJSON: `{"d":2}`,
	}); err != nil {
		t.Fatalf("UpsertSleep 17: %v", err)
	}
	if err := s.UpsertHrv(HrvRow{
		Date: "2026-06-18", LastNightAvgMs: i64p(48), Status: strp("BALANCED"), RawJSON: `{"h":1}`,
	}); err != nil {
		t.Fatalf("UpsertHrv 18: %v", err)
	}
	if err := s.UpsertBodyBattery(BodyBatteryRow{
		Date: "2026-06-18", Charged: i64p(62), Drained: i64p(78), High: i64p(91), Low: i64p(14),
		RawJSON: `{"b":1}`,
	}); err != nil {
		t.Fatalf("UpsertBodyBattery 18: %v", err)
	}
	if err := s.UpsertRhr(RhrRow{Date: "2026-06-18", RestingHR: i64p(47), RawJSON: `{"r":1}`}); err != nil {
		t.Fatalf("UpsertRhr 18: %v", err)
	}
	if err := s.UpsertRhr(RhrRow{Date: "2026-06-17", RestingHR: i64p(49), RawJSON: `{"r":2}`}); err != nil {
		t.Fatalf("UpsertRhr 17: %v", err)
	}

	// Distinct recovery dates across all four tables = {06-17, 06-18} = 2.
	n, err := s.CountRecoveryDays()
	if err != nil {
		t.Fatalf("CountRecoveryDays error = %v", err)
	}
	if n != 2 {
		t.Errorf("CountRecoveryDays = %d, want 2", n)
	}

	rec, err := s.ListRecovery(30)
	if err != nil {
		t.Fatalf("ListRecovery error = %v", err)
	}
	if len(rec) != 2 {
		t.Fatalf("ListRecovery len = %d, want 2", len(rec))
	}
	// Most-recent-first: 06-18 then 06-17.
	if rec[0].Date != "2026-06-18" || rec[1].Date != "2026-06-17" {
		t.Errorf("dates = [%s,%s], want [2026-06-18,2026-06-17]", rec[0].Date, rec[1].Date)
	}
	// 06-18 fully populated.
	d18 := rec[0]
	if d18.Sleep == nil || d18.Sleep.Score == nil || *d18.Sleep.Score != 82 {
		t.Errorf("06-18 sleep.score = %v, want 82", d18.Sleep)
	}
	if d18.HRV == nil || d18.HRV.Status == nil || *d18.HRV.Status != "BALANCED" {
		t.Errorf("06-18 hrv = %v, want BALANCED", d18.HRV)
	}
	if d18.BodyBattery == nil || d18.BodyBattery.High == nil || *d18.BodyBattery.High != 91 {
		t.Errorf("06-18 body_battery.high = %v, want 91", d18.BodyBattery)
	}
	if d18.RHR == nil || d18.RHR.RestingHR == nil || *d18.RHR.RestingHR != 47 {
		t.Errorf("06-18 rhr = %v, want 47", d18.RHR)
	}
	// 06-17 has sleep + rhr; hrv and body_battery must be nil.
	d17 := rec[1]
	if d17.HRV != nil {
		t.Errorf("06-17 hrv = %v, want nil", d17.HRV)
	}
	if d17.BodyBattery != nil {
		t.Errorf("06-17 body_battery = %v, want nil", d17.BodyBattery)
	}
	if d17.Sleep == nil || d17.RHR == nil {
		t.Errorf("06-17 sleep/rhr missing: sleep=%v rhr=%v", d17.Sleep, d17.RHR)
	}

	// Re-upsert sleep 06-18 with a new score -> update, not duplicate.
	if err := s.UpsertSleep(SleepRow{
		Date: "2026-06-18", DurationS: i64p(27000), Score: i64p(90), RawJSON: `{"d":1}`,
	}); err != nil {
		t.Fatalf("re-UpsertSleep: %v", err)
	}
	rec, _ = s.ListRecovery(30)
	if len(rec) != 2 {
		t.Fatalf("after re-upsert len = %d, want 2", len(rec))
	}
	if rec[0].Sleep == nil || rec[0].Sleep.Score == nil || *rec[0].Sleep.Score != 90 {
		t.Errorf("06-18 sleep.score after re-upsert = %v, want 90", rec[0].Sleep)
	}

	// days limit clamps result.
	one, _ := s.ListRecovery(1)
	if len(one) != 1 || one[0].Date != "2026-06-18" {
		t.Errorf("ListRecovery(1) = %v, want single [2026-06-18]", one)
	}
}

func TestM1MigrationCreatesTables(t *testing.T) {
	s := newTestStore(t)

	wantTables := []string{"athlete_profile", "crossfit_weeks", "plans"}
	for _, tbl := range wantTables {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found after migrate: %v", tbl, err)
		}
	}

	// plans index present.
	var idx string
	if err := s.DB.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_plans_week_start'`,
	).Scan(&idx); err != nil {
		t.Errorf("idx_plans_week_start not found: %v", err)
	}
}

func TestM1MigrationSeedsProfile(t *testing.T) {
	s := newTestStore(t)

	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM athlete_profile`).Scan(&n); err != nil {
		t.Fatalf("count athlete_profile: %v", err)
	}
	if n != 1 {
		t.Errorf("athlete_profile row count = %d, want 1 (seeded single row)", n)
	}

	var id int
	var target float64
	var mode, constraints, goal string
	if err := s.DB.QueryRow(
		`SELECT id, target_weekly_km, progression_mode, run_constraints_json, goal_text
		 FROM athlete_profile WHERE id = 1`,
	).Scan(&id, &target, &mode, &constraints, &goal); err != nil {
		t.Fatalf("scan seeded profile: %v", err)
	}
	if id != 1 || target != 20 || mode != "build" || constraints != "{}" || goal != "" {
		t.Errorf("seed = id %d target %v mode %q constraints %q goal %q, want 1/20/build/{}/empty",
			id, target, mode, constraints, goal)
	}

	// CHECK (id = 1) rejects a second row.
	_, err := s.DB.Exec(`INSERT INTO athlete_profile (id, updated_at) VALUES (2, 'x')`)
	if err == nil {
		t.Error("inserting id=2 succeeded, want CHECK (id = 1) violation")
	}
}

func TestUpsertGarminActivity(t *testing.T) {
	s := newTestStore(t)

	// Insert one row with all fields.
	if err := s.UpsertGarminActivity(GarminActivityRow{
		GarminActivityID: 14820001234,
		StartTime:        "2026-06-22 05:00:00",
		DurationS:        f64p(3300),
		DistanceM:        f64p(10000),
		ActivityType:     strp("running"),
		RawJSON:          `{"activityId":14820001234}`,
	}); err != nil {
		t.Fatalf("UpsertGarminActivity insert: %v", err)
	}

	// Nullable fields stored as NULL when nil.
	if err := s.UpsertGarminActivity(GarminActivityRow{
		GarminActivityID: 14820005678,
		StartTime:        "2026-06-21 06:00:00",
		DurationS:        nil,
		DistanceM:        nil,
		ActivityType:     nil,
		RawJSON:          "null",
	}); err != nil {
		t.Fatalf("UpsertGarminActivity null-fields: %v", err)
	}

	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM garmin_activities`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 2 {
		t.Fatalf("row count = %d, want 2", n)
	}

	// Verify stored values + NULL preservation.
	var st, raw string
	var dur, dist sql.NullFloat64
	var atype sql.NullString
	if err := s.DB.QueryRow(
		`SELECT start_time, duration_s, distance_m, activity_type, raw_json
		 FROM garmin_activities WHERE garmin_activity_id=?`, 14820005678).Scan(
		&st, &dur, &dist, &atype, &raw); err != nil {
		t.Fatalf("scan null row: %v", err)
	}
	if dur.Valid || dist.Valid || atype.Valid {
		t.Errorf("null row: dur=%v dist=%v atype=%v, want all NULL", dur, dist, atype)
	}
	if raw != "null" {
		t.Errorf("raw_json = %q, want %q", raw, "null")
	}

	// Re-upsert by garmin_activity_id -> update, not duplicate.
	if err := s.UpsertGarminActivity(GarminActivityRow{
		GarminActivityID: 14820001234,
		StartTime:        "2026-06-22 05:00:30",
		DurationS:        f64p(3400),
		DistanceM:        f64p(10100),
		ActivityType:     strp("trail_running"),
		RawJSON:          `{"activityId":14820001234,"v":2}`,
	}); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM garmin_activities`).Scan(&n)
	if n != 2 {
		t.Fatalf("after re-upsert count = %d, want 2 (idempotent by PK)", n)
	}
	var gotType string
	_ = s.DB.QueryRow(`SELECT activity_type FROM garmin_activities WHERE garmin_activity_id=?`, 14820001234).Scan(&gotType)
	if gotType != "trail_running" {
		t.Errorf("activity_type after re-upsert = %q, want trail_running", gotType)
	}
}
