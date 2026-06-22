# Milestone 3.2.1 (Activate Garmin .FIT Fallback) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Activate the dormant M3.2 Garmin `.FIT` fallback so that any run lacking a Strava HR stream still gets HR (and therefore time-in-zone + decoupling) by lazily matching it to a Garmin activity and pulling the Garmin `.FIT`.

**Architecture:** Extends the M0+M1+M2+M3.1+M3.2 codebase on `main`. The Python worker's existing `fetch` command additionally ingests the recent-window Garmin activities list, which `SyncGarmin` upserts into a new `garmin_activities` table; the streams engine's `resolveGarminID` is changed from its `(0,false)` stub to a lazy start-time match (±tolerance, tie-broken by duration then distance, run-type only), which lights up the already-built `FetchAndAnalyze` → `RunGarminFetchFIT` → parse → `source=garmin` path; the run-detail screen surfaces the analysis source as a badge. Nothing in the FIT-parse machinery changes — this slice supplies only the missing Strava↔Garmin id mapping.

**Tech Stack:** Go (chi router, `modernc.org/sqlite`, goose migrations) for the backend; Python (`python-garminconnect`, `garmin-fit-sdk`) for the recovery worker; Expo / React Native for the app. No new external dependencies — `garmin-fit-sdk` is already pinned and installed in the worker venv from M3.2.

---

## Setup Prerequisites

- This plan builds directly on the M3.2 code merged to `main` in `/home/jake/project/help-my-run`. All FIT machinery (worker `stream` subcommand, `garmin-fit-sdk` parse, `RunGarminFetchFIT`, the `FetchAndAnalyze` fallback call site) already exists and is unit-tested; only `resolveGarminID` is a dormant `(0,false)` stub.
- `garmin-fit-sdk` is already pinned and installed in the worker venv (`garmin-worker/.venv`); no dependency changes are needed. Activate it with `source garmin-worker/.venv/bin/activate`, or invoke pytest directly via `garmin-worker/.venv/bin/python -m pytest`.
- `garminconnect==0.3.6` is installed; its `get_activities_by_date(startdate, enddate, activitytype, sortorder)` method exists and is used as-is.
- After this milestone, a Garmin sync now also ingests the recent-window **activities list** (the worker's existing `fetch` command emits a new top-level `activities` array; `SyncGarmin` upserts it). No new worker subcommand and no `SyncGarmin` signature change.
- Working dirs: backend `cd /home/jake/project/help-my-run/backend`; worker `cd /home/jake/project/help-my-run/garmin-worker`; app `cd /home/jake/project/help-my-run/app`.
- Latest existing migration is `00006_m3_2_streams.sql`; this plan adds `00007_m3_2_1_garmin_activities.sql` (auto-embedded by the `//go:embed migrations/*.sql` glob in `store/migrate.go` — do NOT touch `migrate.go`).

---

## File Structure

**Locating edit sites: use textual anchors.** Every edit below is located by the shown TEXTUAL ANCHOR (match the shown text, not line numbers); copy all names verbatim.

### New files
- `backend/internal/store/migrations/00007_m3_2_1_garmin_activities.sql` — `garmin_activities` table + `idx_garmin_activities_start_time` index.
- `backend/internal/garmin/types_test.go` — `WorkerOutput.Activities` unmarshal test.
- `backend/internal/streams/helpers_test.go` — `f64p`/`strp` pointer helpers for the `streams` test package (Task 11 Step 0; the `streams` package has no copy of `store_test.go`'s helpers).
- `garmin-worker/tests/test_normalize_activity.py` — `normalize_garmin_activity` + `build_output(activities=)` unit tests.

### Modified files — worker (`garmin-worker/`)
- `garmin_worker/client.py` — `+method` `get_activities_by_date` (1:1 delegate).
- `garmin_worker/normalize.py` — `+func` `normalize_garmin_activity`; `+param` `activities` to `build_output` (emit last).
- `garmin_worker/fetcher.py` — `+` activities fetch+normalize in `run_fetch` (try/except → `[]`); pass `activities=`.
- `garmin_worker/cli.py` — `+` synthetic `_DRY_ACTIVITIES_RAW` in `_run_dry_fetch`; pass `activities=` to `build_output`.
- `tests/test_fetcher.py` — `+` `_MockClient.get_activities_by_date`; activities-shape / run-type-filter / degrade-to-`[]` asserts; update legacy key-set assertion.
- `tests/test_fetch_cli.py` — `+` `--dry-run` activities key-set + content test.

### Modified files — backend (`backend/`)
- `internal/store/garmin.go` — `+` `GarminActivityRow`, `UpsertGarminActivity`, `GarminActivityCandidate`, `FindGarminActivitiesNear`.
- `internal/store/activities.go` — `+` `GetActivity(stravaID)` getter; `+` `"errors"` import.
- `internal/store/store_test.go` — `+` `"garmin_activities"` in `wantTables`; `+` `"database/sql"` import; `+` upsert + nearest-match roundtrip tests.
- `internal/garmin/types.go` — `+` `WorkerOutput.Activities`; `+` `GarminActivity` struct.
- `internal/garmin/testdata/worker_output.json` — `+` `"activities"` array (≥1 running element).
- `internal/sync/sync.go` — `+` activities upsert loop in `SyncGarmin`.
- `internal/sync/sync_test.go` — bump expected `synced`, add `garmin_activities` count + run-type asserts; add `"activities":[]` to the backfill-window inline JSON.
- `internal/config/config.go` — `+` `GarminMatchToleranceS` field.
- `internal/config/config_test.go` — `+` default + override tests.
- `internal/streams/engine.go` — `+` `matchToleranceS` field + `New` param; replace `resolveGarminID` body; `+` `absF` helper.
- `internal/streams/engine_test.go` — update `newTestEngine` `New(...)` call (+tolerance arg); `+` `TestResolveGarminID` (table-driven); `+` end-to-end activation test.
- `cmd/server/main.go` — update `streams.New(...)` call site (+`cfg.GarminMatchToleranceS`).
- `.env.example` — `+` `GARMIN_MATCH_TOLERANCE_S=120`.

### Modified files — app (`app/`)
- `app/run/[id].tsx` — `+` `source-badge` `<Text>` + `sourceBadge` style.
- `app/__tests__/run-detail.test.tsx` — `+` garmin/strava/no-HR badge presence/absence tests.
- `src/api/types.ts` — **NO CHANGE** (`StreamAnalysis.source: 'strava' | 'garmin' | ''` already present; Go wire DTO already emits it).

---

## Shared Contracts

All anchors are TEXTUAL — match the shown text, not line numbers. Copy names verbatim.

### C1. Migration `00007_m3_2_1_garmin_activities.sql`

**Path (new):** `backend/internal/store/migrations/00007_m3_2_1_garmin_activities.sql`. Auto-picked-up by the embed glob `//go:embed migrations/*.sql` in `store/migrate.go` (do NOT touch). Latest existing migration confirmed `00006_m3_2_streams.sql`; no `00007` exists.

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE garmin_activities (
    garmin_activity_id INTEGER PRIMARY KEY,
    start_time         TEXT NOT NULL,
    duration_s         REAL,
    distance_m         REAL,
    activity_type      TEXT,
    raw_json           TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_garmin_activities_start_time ON garmin_activities (start_time);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE garmin_activities;
-- +goose StatementEnd
```

Notes: `start_time` `NOT NULL` (it is the match key and is always present). `duration_s`/`distance_m`/`activity_type` nullable (defensive: Garmin may omit). `raw_json` `NOT NULL` mirroring all other `garmin_*` tables; the upsert always passes a string (`rawString` → `"null"` when empty). Index on `start_time` ascending (NOT `DESC` — the nearest-match query filters a window around a point) mirrors the column choice (`start_time`) of `00001_init.sql`'s `idx_activities_start_time`, but intentionally omits `DESC` — direction is irrelevant for the `ABS(strftime)` window query. Goose format copied from `00006_m3_2_streams.sql`. `DROP INDEX` not needed in Down (SQLite drops indexes with the table).

### C2. Garmin activities ingestion contract

**Verified `garminconnect` method (`garminconnect==0.3.6`):**
```python
get_activities_by_date(
    startdate: str,                  # "YYYY-MM-DD" (reuse SyncGarmin's `since`)
    enddate: str | None = None,      # "YYYY-MM-DD"
    activitytype: str | None = None, # pass "running" → server-side run filter
    sortorder: str | None = None,
) -> list[dict]                      # flat list, auto-paginated
```
Per-element JSON keys (read as plain dicts, NOT typed models): `el["activityId"]` (int), `el["startTimeGMT"]` (str, GMT/UTC, format `"YYYY-MM-DD HH:MM:SS"` — space, no `T`, likely no zone suffix), `el["duration"]` (float s), `el["distance"]` (float m), `el["activityType"]["typeKey"]` (str, e.g. `"running"`, `"trail_running"`, `"treadmill_running"`).

**Decision (locked):** the activities list is emitted by the existing `fetch` command as a new top-level `activities` array — NO new subcommand. This keeps `SyncGarmin` a single `RunGarminFetch` call. A list-fetch failure degrades to `activities: []` (never fails the whole `fetch`).

**Go `WorkerOutput` extension (`backend/internal/garmin/types.go`)** — field last + new struct mirroring `Vo2maxDay`:
```go
type WorkerOutput struct {
	Since       string           `json:"since"`
	Until       string           `json:"until"`
	FetchedAt   string           `json:"fetched_at"`
	Sleep       []SleepDay       `json:"sleep"`
	HRV         []HrvDay         `json:"hrv"`
	BodyBattery []BodyBatteryDay `json:"body_battery"`
	RHR         []RhrDay         `json:"rhr"`
	VO2Max      []Vo2maxDay      `json:"vo2max"`
	Activities  []GarminActivity `json:"activities"` // M3.2.1
}

// GarminActivity is one element of the Garmin activities list (§2.x).
type GarminActivity struct {
	GarminActivityID int64           `json:"garmin_activity_id"`
	StartTime        string          `json:"start_time"`
	DurationS        *float64        `json:"duration_s"`
	DistanceM        *float64        `json:"distance_m"`
	ActivityType     *string         `json:"activity_type"`
	RawJSON          json.RawMessage `json:"raw_json"`
}
```

**`SyncGarmin` ingest (`backend/internal/sync/sync.go`)** — no signature change (`func SyncGarmin(ctx, s *store.Store, r garmin.Runner, extraEnv []string) SourceResult`). Add ONE loop after the `out.VO2Max` loop, before `return okResult(...)`, mirroring the existing upsert loops + `rawString` helper:
```go
for _, a := range out.Activities {
	if err := s.UpsertGarminActivity(store.GarminActivityRow{
		GarminActivityID: a.GarminActivityID,
		StartTime:        a.StartTime,
		DurationS:        a.DurationS,
		DistanceM:        a.DistanceM,
		ActivityType:     a.ActivityType,
		RawJSON:          rawString(a.RawJSON),
	}); err != nil {
		return errResult(s, source, err)
	}
	synced++
}
```
Recent window: reuses the existing `since` (`sync_log.last_synced_at` or 84-day backfill, `Format("2006-01-02")`), passed through to `get_activities_by_date` in the worker. No new since/until.

### C3. Store contract (`backend/internal/store/garmin.go`)

**Row struct + Upsert (mirror `Vo2maxRow` / `UpsertVo2max`):**
```go
// GarminActivityRow maps to garmin_activities.
type GarminActivityRow struct {
	GarminActivityID int64
	StartTime        string
	DurationS        *float64
	DistanceM        *float64
	ActivityType     *string
	RawJSON          string
}

// UpsertGarminActivity upserts one garmin_activities row by garmin_activity_id.
func (s *Store) UpsertGarminActivity(r GarminActivityRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_activities
			(garmin_activity_id, start_time, duration_s, distance_m, activity_type, raw_json)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(garmin_activity_id) DO UPDATE SET
			start_time=excluded.start_time, duration_s=excluded.duration_s,
			distance_m=excluded.distance_m, activity_type=excluded.activity_type,
			raw_json=excluded.raw_json`,
		r.GarminActivityID, r.StartTime, r.DurationS, r.DistanceM, r.ActivityType, r.RawJSON)
	return err
}
```

**Nearest-match query (run-type, ±tolerance, ordered for tie-break):** Candidate-returning shape (lets the engine apply the duration→distance tie-break against the Strava fields it holds). `startISO` is the Strava activity's `start_time` (RFC3339). Tolerance compared via SQLite `strftime('%s', ...)` epoch diff. Run-type filter: `activity_type LIKE '%running%'` (covers `running`/`trail_running`/`treadmill_running`).
```go
// GarminActivityCandidate is one run-type garmin_activities row within the
// start-time tolerance window of a query time (for resolveGarminID tie-break).
type GarminActivityCandidate struct {
	GarminActivityID int64
	StartTime        string
	DurationS        *float64
	DistanceM        *float64
}

// FindGarminActivitiesNear returns run-type garmin_activities whose start_time is
// within ±toleranceSec of startISO, ordered by absolute start-time delta ascending.
// Empty slice (not error) when none match. Caller tie-breaks by duration then distance.
func (s *Store) FindGarminActivitiesNear(startISO string, toleranceSec int) ([]GarminActivityCandidate, error) {
	rows, err := s.DB.Query(`
		SELECT garmin_activity_id, start_time, duration_s, distance_m
		FROM garmin_activities
		WHERE activity_type LIKE '%running%'
		  AND ABS(strftime('%s', start_time) - strftime('%s', ?)) <= ?
		ORDER BY ABS(strftime('%s', start_time) - strftime('%s', ?)) ASC`,
		startISO, toleranceSec, startISO)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GarminActivityCandidate
	for rows.Next() {
		var c GarminActivityCandidate
		if err := rows.Scan(&c.GarminActivityID, &c.StartTime, &c.DurationS, &c.DistanceM); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
```
**Timezone note (spec §10):** `strftime('%s', ...)` parses both RFC3339 (`2026-06-22T05:00:00Z`, Strava) and `"YYYY-MM-DD HH:MM:SS"` (Garmin GMT) as UTC, so the epoch diff is a true UTC comparison — the ±120s window absorbs skew. If live `startTimeGMT` carries no zone, SQLite treats it as UTC (correct, since it IS GMT).

**Single-activity getter (`backend/internal/store/activities.go`) — NEW, required by `resolveGarminID`.** No `GetActivity(id)` exists today. Add it mirroring `GetActivityStream`'s `QueryRow` + `sql.ErrNoRows`→`ErrNotFound` pattern, selecting the same column list as `ListActivities`:
```go
// GetActivity returns one activity by strava_id, or ErrNotFound. raw_json is not loaded.
func (s *Store) GetActivity(stravaID int64) (Activity, error) {
	var a Activity
	err := s.DB.QueryRow(`
		SELECT strava_id, name, type, sport_type, start_time, start_time_local,
		       distance_m, moving_time_s, elapsed_time_s,
		       avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m
		FROM activities
		WHERE strava_id = ?`, stravaID).Scan(
		&a.StravaID, &a.Name, &a.Type, &a.SportType, &a.StartTime, &a.StartTimeLocal,
		&a.DistanceM, &a.MovingTimeS, &a.ElapsedTimeS,
		&a.AvgHR, &a.MaxHR, &a.AvgSpeed, &a.MaxSpeed, &a.AvgCadence, &a.ElevationGainM)
	if errors.Is(err, sql.ErrNoRows) {
		return Activity{}, ErrNotFound
	}
	if err != nil {
		return Activity{}, err
	}
	return a, nil
}
```
(Add `"errors"` import to `activities.go`; `"database/sql"` already imported.)

### C4. `resolveGarminID` activation contract (`backend/internal/streams/engine.go`)

**Signature kept exactly** (`(int64, bool)`), input still `stravaActivityID int64` — the `Engine` holds `store` + the new tolerance field, so it loads the Strava comparison fields itself. The `FetchAndAnalyze` call site (`if gid, ok := e.resolveGarminID(activityID); ok`) is UNCHANGED.

**Algorithm:** load activity (`e.store.GetActivity`) → `FindGarminActivitiesNear(a.StartTime, e.matchToleranceS)` → if empty, `(0,false)` → else pick the candidate whose `DurationS` is closest to the Strava `MovingTimeS` (fallback to candidate ordering / `DistanceM` on ties) → return `(garminID, true)`. Tie-break order locked: **duration (vs `MovingTimeS`) then distance**, per spec §7. Run-type filter is in the SQL.

```go
func (e *Engine) resolveGarminID(stravaActivityID int64) (int64, bool) {
	act, err := e.store.GetActivity(stravaActivityID)
	if err != nil {
		return 0, false // unknown activity → no match (graceful degrade)
	}
	cands, err := e.store.FindGarminActivitiesNear(act.StartTime, e.matchToleranceS)
	if err != nil || len(cands) == 0 {
		return 0, false
	}

	bestID := int64(0)
	bestSet := false
	var bestDur, bestDist float64
	target := float64(act.MovingTimeS)
	for _, c := range cands {
		durDelta := 1e18
		if c.DurationS != nil {
			durDelta = absF(*c.DurationS - target)
		}
		distDelta := 1e18
		if c.DistanceM != nil {
			distDelta = absF(*c.DistanceM - act.DistanceM)
		}
		if !bestSet || durDelta < bestDur || (durDelta == bestDur && distDelta < bestDist) {
			bestID, bestDur, bestDist, bestSet = c.GarminActivityID, durDelta, distDelta, true
		}
	}
	if !bestSet {
		return 0, false
	}
	return bestID, true
}
```
Add a small `absF(x float64) float64` helper (avoids a `math` import for one call).

**Engine struct + `New` threading** (config knob must reach the engine):
```go
type Engine struct {
	store           *store.Store
	strava          *strava.Client
	runner          garmin.Runner
	extraEnv        []string
	matchToleranceS int // M3.2.1: GARMIN_MATCH_TOLERANCE_S
}

func New(s *store.Store, sc *strava.Client, runner garmin.Runner, extraEnv []string, matchToleranceS int) *Engine {
	return &Engine{store: s, strava: sc, runner: runner, extraEnv: extraEnv, matchToleranceS: matchToleranceS}
}
```
**Single call site** to update: `main.go` `streamsEngine := streams.New(s, stravaClient, runner, extraEnv)` → append `, cfg.GarminMatchToleranceS`. **Test harness** `streams/engine_test.go` `newTestEngine` → `New(..., garmin.Runner{}, nil, 120)`.

`FetchAndAnalyze` itself is unchanged — activating `resolveGarminID` lights up the existing fallback block (`source = "garmin"` flows through `computeAndStore` → `a.Source = raw.Source` → DTO → app).

### C5. Config (`backend/internal/config/config.go`)

Add alongside the M3.2 stream block:
```go
	// M3.2: stream fetch trickle.
	StreamRecentWeeks int `envconfig:"STREAM_RECENT_WEEKS" default:"12"`
	StreamFetchBudget int `envconfig:"STREAM_FETCH_BUDGET" default:"10"`

	// M3.2.1: Garmin .FIT fallback start-time match tolerance (seconds).
	GarminMatchToleranceS int `envconfig:"GARMIN_MATCH_TOLERANCE_S" default:"120"`
```
Loaded automatically by the existing `envconfig.Process("", &c)` in `Load()`. Document `GARMIN_MATCH_TOLERANCE_S=120` in `.env.example` next to the existing `GARMIN_*` vars. Only this one knob is needed (spec §7 names only the ±120s tolerance).

### C6. App source badge (`app/app/run/[id].tsx`)

`app/src/api/types.ts` `StreamAnalysis.source: 'strava' | 'garmin' | ''` already exists; Go wire (`dto.go` `Source string json:"source"`, `stream_handlers.go` `Source: a.Source`) already emits it. Pure render add inside the existing `a && a.has_stream && a.has_hr` block, above the "Time in zone" subheading:
```tsx
{a && a.has_stream && a.has_hr ? (
  <View style={styles.section}>
    {a.source === 'garmin' ? (
      <Text testID="source-badge" style={styles.sourceBadge}>HR via Garmin .FIT</Text>
    ) : null}
    <Text style={styles.subheading}>Time in zone</Text>
    {a.time_in_zone.map((z) => (
      <ZoneBar key={z.zone} z={z} />
    ))}
  </View>
) : null}
```
Add to `StyleSheet.create({...})`:
```tsx
  sourceBadge: {
    alignSelf: 'flex-start', fontSize: 12, fontWeight: '600', color: '#fc4c02',
    backgroundColor: '#fff0e8', borderRadius: 6, paddingHorizontal: 8, paddingVertical: 3, marginTop: 8,
  },
```
`testID="source-badge"`; badge text exactly `"HR via Garmin .FIT"` (spec §3.4/§4). Expo SDK 56 — read `https://docs.expo.dev/versions/v56.0.0/` before editing app code (per `app/AGENTS.md`). Only RN `View`/`Text`/`StyleSheet` used, all already imported.

### Flagged unverified (carry into implementation)
1. **`startTimeGMT` string format/zone** — no live fixture; normalizer reads it raw, SQLite `strftime('%s', ...)` parses both Garmin (`YYYY-MM-DD HH:MM:SS`, GMT) and Strava RFC3339 (`...Z`) as UTC. Verify against live data at integration; if a non-standard suffix appears, normalize in `normalize_garmin_activity` before storing.
2. **Run `typeKey` set** — `LIKE '%running%'` covers `running`/`trail_running`/`treadmill_running`; confirm no false positives against real data.
3. **End-to-end activation test seam** — `garmin.Runner` is a concrete struct (no interface): stub via `/bin/sh` script echoing FIT JSON (pattern in `garmin/runner_test.go`); `FetchAndAnalyze` makes a real `e.strava.GetActivityStreams` call, so the Strava side must be stubbed via `strava.NewWithBase(id, secret, cb, httptestURL)` returning a no-HR stream.

---

## Tasks

**Sequencing (hard dependencies):** Tasks 1→2→3 are store-side and sequential (they share `garmin.go`/`store_test.go`). Tasks 4→5→6→7 are the worker side and sequential (Task 5 makes the worker package red until Task 6; do NOT commit between 5 and 6). Tasks 8→9 are the Go ingest side (9 consumes the types from 8 and `UpsertGarminActivity` from 2). Task 10 (config) is independent. Task 11 (`resolveGarminID` activation) depends on Tasks 1–3 (store symbols), 10 (config knob), and adds `GetActivity` if not already present. Task 12 (end-to-end activation) depends on Task 11. Task 13 (app badge) is fully independent (mocks the analysis) and may land in any order. Recommended order is the numeric order below.

---

### Task 1: Migration 00007 garmin_activities + wantTables

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/store/migrations/00007_m3_2_1_garmin_activities.sql`
- Test (Modify): `/home/jake/project/help-my-run/backend/internal/store/store_test.go` (`TestOpenAndMigrate` → `wantTables` slice)

- [ ] **Step 1: Failing test — add `"garmin_activities"` to `wantTables`.**
  In `store_test.go`, `TestOpenAndMigrate`, edit the `wantTables` literal by TEXTUAL ANCHOR. Replace:
  ```go
  	wantTables := []string{
  		"strava_tokens", "activities", "activity_splits",
  		"garmin_sleep", "garmin_hrv", "garmin_body_battery", "garmin_rhr",
  		"garmin_vo2max",
  		"sync_log",
  		"activity_streams", "stream_analyses", "stream_fetch_log",
  	}
  ```
  with:
  ```go
  	wantTables := []string{
  		"strava_tokens", "activities", "activity_splits",
  		"garmin_sleep", "garmin_hrv", "garmin_body_battery", "garmin_rhr",
  		"garmin_vo2max",
  		"sync_log",
  		"activity_streams", "stream_analyses", "stream_fetch_log",
  		"garmin_activities",
  	}
  ```

- [ ] **Step 2: Run — expect FAIL.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestOpenAndMigrate -v`
  Expected: FAIL — `table "garmin_activities" not found after migrate: sql: no rows in result set` (table does not exist yet).

- [ ] **Step 3: Minimal impl — create migration 00007 (full file).**
  Write `/home/jake/project/help-my-run/backend/internal/store/migrations/00007_m3_2_1_garmin_activities.sql` (auto-picked by `//go:embed migrations/*.sql` in `migrate.go` — do NOT touch migrate.go):
  ```sql
  -- +goose Up
  -- +goose StatementBegin
  CREATE TABLE garmin_activities (
      garmin_activity_id INTEGER PRIMARY KEY,
      start_time         TEXT NOT NULL,
      duration_s         REAL,
      distance_m         REAL,
      activity_type      TEXT,
      raw_json           TEXT NOT NULL
  );
  -- +goose StatementEnd

  -- +goose StatementBegin
  CREATE INDEX idx_garmin_activities_start_time ON garmin_activities (start_time);
  -- +goose StatementEnd

  -- +goose Down
  -- +goose StatementBegin
  DROP TABLE garmin_activities;
  -- +goose StatementEnd
  ```
  (Goose Up/Down + StatementBegin/End mirrors `00006_m3_2_streams.sql`. Index ascending — NOT `DESC` — for the windowed nearest-match query. `raw_json NOT NULL` mirrors all other `garmin_*` tables.)

- [ ] **Step 4: Run — expect PASS.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run 'TestOpenAndMigrate|TestMigrateIdempotent' -v`
  Expected: PASS (table present; second `Migrate()` still a no-op).

- [ ] **Step 5: Commit.**
  ```bash
  cd /home/jake/project/help-my-run && git add backend/internal/store/migrations/00007_m3_2_1_garmin_activities.sql backend/internal/store/store_test.go && git commit -m "feat(store): add 00007 garmin_activities migration"
  ```

---

### Task 2: GarminActivityRow + UpsertGarminActivity (store)

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/store/garmin.go` (append at the END of the file, after the `ListRecovery` function's closing brace — the last function in the file)
- Test (Modify): `/home/jake/project/help-my-run/backend/internal/store/store_test.go` (new test func + uses existing `newTestStore`, `f64p`, `strp`)

- [ ] **Step 1: Failing test — upsert roundtrip + idempotency.**
  Append to `store_test.go`:
  ```go
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
  ```
  NOTE: this test uses `sql.NullFloat64`/`sql.NullString`. `store_test.go` currently imports only `path/filepath`, `testing` — add the `"database/sql"` import. Edit the import block by TEXTUAL ANCHOR, replacing:
  ```go
  import (
  	"path/filepath"
  	"testing"
  )
  ```
  with:
  ```go
  import (
  	"database/sql"
  	"path/filepath"
  	"testing"
  )
  ```

- [ ] **Step 2: Run — expect FAIL (compile error).**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestUpsertGarminActivity -v`
  Expected: FAIL — `undefined: GarminActivityRow` / `s.UpsertGarminActivity undefined`.

- [ ] **Step 3: Minimal impl — append `GarminActivityRow` + `UpsertGarminActivity` to `garmin.go`.**
  Append at the END of `garmin.go`, after the `ListRecovery` function's closing `}` (the last function in the file — `ListRecovery`'s closing brace is the TEXTUAL ANCHOR; `UpsertVo2max` is NOT the last func, it is followed by `ListVo2max`/`CountRecoveryDays`/`ListRecovery`):
  ```go
  // GarminActivityRow maps to garmin_activities.
  type GarminActivityRow struct {
  	GarminActivityID int64
  	StartTime        string
  	DurationS        *float64
  	DistanceM        *float64
  	ActivityType     *string
  	RawJSON          string
  }

  // UpsertGarminActivity upserts one garmin_activities row by garmin_activity_id.
  func (s *Store) UpsertGarminActivity(r GarminActivityRow) error {
  	_, err := s.DB.Exec(`
  		INSERT INTO garmin_activities
  			(garmin_activity_id, start_time, duration_s, distance_m, activity_type, raw_json)
  		VALUES (?,?,?,?,?,?)
  		ON CONFLICT(garmin_activity_id) DO UPDATE SET
  			start_time=excluded.start_time, duration_s=excluded.duration_s,
  			distance_m=excluded.distance_m, activity_type=excluded.activity_type,
  			raw_json=excluded.raw_json`,
  		r.GarminActivityID, r.StartTime, r.DurationS, r.DistanceM, r.ActivityType, r.RawJSON)
  	return err
  }
  ```

- [ ] **Step 4: Run — expect PASS.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestUpsertGarminActivity -v`
  Expected: PASS.

- [ ] **Step 5: Commit.**
  ```bash
  cd /home/jake/project/help-my-run && git add backend/internal/store/garmin.go backend/internal/store/store_test.go && git commit -m "feat(store): add GarminActivityRow + UpsertGarminActivity"
  ```

---

### Task 3: FindGarminActivitiesNear (nearest-by-start_time, run-type filter, tie-break candidates)

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/store/garmin.go` (append at the END of the file, after the `UpsertGarminActivity` function added in Task 2 — which is now the last function in the file, following the original `ListRecovery`)
- Test (Modify): `/home/jake/project/help-my-run/backend/internal/store/store_test.go` (new test func)

- [ ] **Step 1: Failing test — within/outside tolerance, ordering, run-type filter, tie-break candidates, no-match.**
  Append to `store_test.go`:
  ```go
  func TestFindGarminActivitiesNear(t *testing.T) {
  	s := newTestStore(t)

  	// Seed: query anchor is RFC3339 "2026-06-22T05:00:00Z".
  	// A) exactly on time, running                          -> in window, delta 0
  	// B) +60s, running                                     -> in window, delta 60
  	// C) +119s, treadmill_running                          -> in window (run-type), delta 119
  	// D) +200s, running                                    -> OUTSIDE 120s window
  	// E) on time, cycling                                  -> run-type filtered OUT
  	mustUpsertGA(t, s, 1, "2026-06-22 05:00:00", f64p(3300), f64p(10000), "running")
  	mustUpsertGA(t, s, 2, "2026-06-22 05:01:00", f64p(3200), f64p(9800), "running")
  	mustUpsertGA(t, s, 3, "2026-06-22 05:01:59", f64p(3100), f64p(9500), "treadmill_running")
  	mustUpsertGA(t, s, 4, "2026-06-22 05:03:20", f64p(3300), f64p(10000), "running")
  	mustUpsertGA(t, s, 5, "2026-06-22 05:00:00", f64p(3300), f64p(40000), "cycling")

  	cands, err := s.FindGarminActivitiesNear("2026-06-22T05:00:00Z", 120)
  	if err != nil {
  		t.Fatalf("FindGarminActivitiesNear: %v", err)
  	}
  	// In-window run-type rows: 1, 2, 3 (NOT 4 outside window, NOT 5 cycling).
  	if len(cands) != 3 {
  		t.Fatalf("len = %d, want 3 (ids 1,2,3); got %+v", len(cands), cands)
  	}
  	// Ordered by absolute start-time delta ascending: 1 (0s), 2 (60s), 3 (119s).
  	if cands[0].GarminActivityID != 1 || cands[1].GarminActivityID != 2 || cands[2].GarminActivityID != 3 {
  		t.Errorf("order = [%d,%d,%d], want [1,2,3]",
  			cands[0].GarminActivityID, cands[1].GarminActivityID, cands[2].GarminActivityID)
  	}
  	// Candidate carries duration/distance for the engine tie-break.
  	if cands[0].DurationS == nil || *cands[0].DurationS != 3300 {
  		t.Errorf("cand[0].DurationS = %v, want 3300", cands[0].DurationS)
  	}
  	if cands[0].DistanceM == nil || *cands[0].DistanceM != 10000 {
  		t.Errorf("cand[0].DistanceM = %v, want 10000", cands[0].DistanceM)
  	}

  	// Tie-break shape: two equidistant candidates (±60s) -> both returned, both
  	// in window, caller resolves by duration/distance.
  	tie, err := s.FindGarminActivitiesNear("2026-06-22T05:00:30Z", 120)
  	if err != nil {
  		t.Fatalf("tie query: %v", err)
  	}
  	// At 05:00:30: id1 (-30s), id2 (+30s) both delta 30; id3 (+89s); id4 (+170s) out.
  	if len(tie) != 3 {
  		t.Fatalf("tie len = %d, want 3 (ids 1,2,3)", len(tie))
  	}
  	// First two are the equidistant pair (1 and 2 in either order), third is id3.
  	if tie[2].GarminActivityID != 3 {
  		t.Errorf("tie[2] = %d, want 3 (furthest in-window)", tie[2].GarminActivityID)
  	}
  	gotPair := map[int64]bool{tie[0].GarminActivityID: true, tie[1].GarminActivityID: true}
  	if !gotPair[1] || !gotPair[2] {
  		t.Errorf("tie pair = %v, want {1,2}", gotPair)
  	}

  	// No-match -> empty slice (not error).
  	none, err := s.FindGarminActivitiesNear("2026-06-22T09:00:00Z", 120)
  	if err != nil {
  		t.Fatalf("no-match query: %v", err)
  	}
  	if len(none) != 0 {
  		t.Errorf("no-match len = %d, want 0", len(none))
  	}
  }

  // mustUpsertGA seeds one garmin_activities row or fails the test.
  func mustUpsertGA(t *testing.T, s *Store, id int64, start string, dur, dist *float64, atype string) {
  	t.Helper()
  	if err := s.UpsertGarminActivity(GarminActivityRow{
  		GarminActivityID: id, StartTime: start, DurationS: dur, DistanceM: dist,
  		ActivityType: strp(atype), RawJSON: "{}",
  	}); err != nil {
  		t.Fatalf("seed garmin_activity %d: %v", id, err)
  	}
  }
  ```

- [ ] **Step 2: Run — expect FAIL (compile error).**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestFindGarminActivitiesNear -v`
  Expected: FAIL — `undefined: GarminActivityCandidate` / `s.FindGarminActivitiesNear undefined`.

- [ ] **Step 3: Minimal impl — append `GarminActivityCandidate` + `FindGarminActivitiesNear` to `garmin.go`.**
  Append at the END of `garmin.go`, after the `UpsertGarminActivity` func added in Task 2 (now the last function in the file — its closing `}` is the TEXTUAL ANCHOR):
  ```go
  // GarminActivityCandidate is one run-type garmin_activities row within the
  // start-time tolerance window of a query time (for resolveGarminID tie-break).
  type GarminActivityCandidate struct {
  	GarminActivityID int64
  	StartTime        string
  	DurationS        *float64
  	DistanceM        *float64
  }

  // FindGarminActivitiesNear returns run-type garmin_activities whose start_time is
  // within ±toleranceSec of startISO, ordered by absolute start-time delta ascending.
  // Empty slice (not error) when none match. Caller tie-breaks by duration then distance.
  func (s *Store) FindGarminActivitiesNear(startISO string, toleranceSec int) ([]GarminActivityCandidate, error) {
  	rows, err := s.DB.Query(`
  		SELECT garmin_activity_id, start_time, duration_s, distance_m
  		FROM garmin_activities
  		WHERE activity_type LIKE '%running%'
  		  AND ABS(strftime('%s', start_time) - strftime('%s', ?)) <= ?
  		ORDER BY ABS(strftime('%s', start_time) - strftime('%s', ?)) ASC`,
  		startISO, toleranceSec, startISO)
  	if err != nil {
  		return nil, err
  	}
  	defer rows.Close()
  	var out []GarminActivityCandidate
  	for rows.Next() {
  		var c GarminActivityCandidate
  		if err := rows.Scan(&c.GarminActivityID, &c.StartTime, &c.DurationS, &c.DistanceM); err != nil {
  			return nil, err
  		}
  		out = append(out, c)
  	}
  	return out, rows.Err()
  }
  ```
  (Run-type filter `activity_type LIKE '%running%'` covers `running`/`trail_running`/`treadmill_running`. `strftime('%s', ...)` parses both Garmin `YYYY-MM-DD HH:MM:SS` GMT and Strava RFC3339 `...Z` as UTC, so the epoch diff is a true UTC comparison absorbing skew within the ±120s window.)

- [ ] **Step 4: Run — expect PASS.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestFindGarminActivitiesNear -v`
  Expected: PASS (within/outside tolerance, ordering, run-type filter, equidistant tie-break pair, no-match → empty all verified).

- [ ] **Step 5: Full store package run + commit.**
  ```bash
  cd /home/jake/project/help-my-run/backend && go test ./internal/store/...
  cd /home/jake/project/help-my-run && git add backend/internal/store/garmin.go backend/internal/store/store_test.go && git commit -m "feat(store): add FindGarminActivitiesNear nearest-match query"
  ```
  Expected: store package PASS.

---

### Task 4: Worker — normalize_garmin_activity (pure normalizer)

**Files:**
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/normalize.py` (add func after `normalize_vo2max_day`, before `build_output`)
- Test (Create): `/home/jake/project/help-my-run/garmin-worker/tests/test_normalize_activity.py`

- [ ] **Step 1: Failing test — assert the six-key contract shape + nested typeKey + raw preservation.**
  Create `tests/test_normalize_activity.py`:
  ```python
  from garmin_worker import normalize


  def test_normalize_garmin_activity_maps_all_fields():
      el = {
          "activityId": 14820001234,
          "startTimeGMT": "2026-06-22 05:00:00",
          "duration": 3300.0,
          "distance": 10000.0,
          "activityType": {"typeKey": "running"},
          "extra": "ignored-but-kept-in-raw",
      }
      out = normalize.normalize_garmin_activity(el)
      assert set(out.keys()) == {
          "garmin_activity_id", "start_time", "duration_s",
          "distance_m", "activity_type", "raw_json",
      }
      assert out["garmin_activity_id"] == 14820001234
      assert out["start_time"] == "2026-06-22 05:00:00"
      assert out["duration_s"] == 3300.0
      assert out["distance_m"] == 10000.0
      assert out["activity_type"] == "running"  # nested activityType.typeKey
      assert out["raw_json"] == el  # ORIGINAL element preserved


  def test_normalize_garmin_activity_trail_run_typekey():
      el = {
          "activityId": 99,
          "startTimeGMT": "2026-06-21 06:00:00",
          "duration": 2700.0,
          "distance": 8000.0,
          "activityType": {"typeKey": "trail_running"},
      }
      out = normalize.normalize_garmin_activity(el)
      assert out["activity_type"] == "trail_running"


  def test_normalize_garmin_activity_missing_fields_are_none():
      out = normalize.normalize_garmin_activity({"activityId": 7})
      assert out["garmin_activity_id"] == 7
      assert out["start_time"] is None
      assert out["duration_s"] is None
      assert out["distance_m"] is None
      assert out["activity_type"] is None  # no activityType -> safe-walk None
      assert out["raw_json"] == {"activityId": 7}


  def test_normalize_garmin_activity_none_input():
      out = normalize.normalize_garmin_activity(None)
      assert out["garmin_activity_id"] is None
      assert out["activity_type"] is None
      assert out["raw_json"] == {}


  def test_normalize_garmin_activity_json_serializable():
      import json
      out = normalize.normalize_garmin_activity({
          "activityId": 1, "startTimeGMT": "2026-06-22 05:00:00",
          "duration": 100.0, "distance": 200.0,
          "activityType": {"typeKey": "running"},
      })
      json.loads(json.dumps(out))  # must not raise
  ```

- [ ] **Step 2: Run — expect FAIL.**
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest tests/test_normalize_activity.py -q`
  Expected: FAIL — `AttributeError: module 'garmin_worker.normalize' has no attribute 'normalize_garmin_activity'`.

- [ ] **Step 3: Minimal impl — add `normalize_garmin_activity` to `normalize.py`.**
  Insert immediately after `normalize_vo2max_day` (its `return {...}` block ending — the line before `def build_output(` — is the TEXTUAL ANCHOR). Insert:
  ```python
  def normalize_garmin_activity(el: Optional[dict]) -> dict:
      """Map one get_activities_by_date element -> the §2.x activity contract.

      Reads plain-dict keys (worker never uses garminconnect typed models).
      raw_json preserves the ORIGINAL element. activity_type is the nested typeKey.
      """
      el = el or {}
      return {
          "garmin_activity_id": el.get("activityId"),
          "start_time": el.get("startTimeGMT"),
          "duration_s": el.get("duration"),
          "distance_m": el.get("distance"),
          "activity_type": _get(el, "activityType", "typeKey"),
          "raw_json": el,
      }
  ```
  (Reuses the existing `_get` safe-walk; pure — no I/O, no garminconnect import.)

- [ ] **Step 4: Run — expect PASS.**
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest tests/test_normalize_activity.py -q`
  Expected: PASS (5 tests).

- [ ] **Step 5: Commit.**
  ```bash
  cd /home/jake/project/help-my-run && git add garmin-worker/garmin_worker/normalize.py garmin-worker/tests/test_normalize_activity.py && git commit -m "feat(worker): add normalize_garmin_activity normalizer"
  ```

---

### Task 5: Worker — build_output gains `activities` param (emit last)

**Files:**
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/normalize.py` (`build_output`)
- Test (Modify): `/home/jake/project/help-my-run/garmin-worker/tests/test_normalize_activity.py` (add a `build_output` key-order test)

- [ ] **Step 1: Failing test — `build_output` emits `activities` last in fixed key order.**
  Append to `tests/test_normalize_activity.py`:
  ```python
  def test_build_output_emits_activities_last():
      out = normalize.build_output(
          since="2026-06-14", until="2026-06-15", fetched_at="t",
          sleep=[], hrv=[], body_battery=[], rhr=[], vo2max=[],
          activities=[{"garmin_activity_id": 1}],
      )
      assert list(out.keys()) == [
          "since", "until", "fetched_at",
          "sleep", "hrv", "body_battery", "rhr", "vo2max", "activities",
      ]
      assert out["activities"] == [{"garmin_activity_id": 1}]
  ```

- [ ] **Step 2: Run — expect FAIL.**
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest tests/test_normalize_activity.py::test_build_output_emits_activities_last -q`
  Expected: FAIL — `TypeError: build_output() got an unexpected keyword argument 'activities'`.

- [ ] **Step 3: Minimal impl — add `activities` param to `build_output` (keyword-only, emit last).**
  Edit `normalize.py` `build_output` by TEXTUAL ANCHOR. Replace the signature + return block:
  ```python
  def build_output(
      *,
      since: str,
      until: str,
      fetched_at: str,
      sleep: list,
      hrv: list,
      body_battery: list,
      rhr: list,
      vo2max: list,
  ) -> dict:
      """Assemble the full worker stdout object (CONTRACTS §2.1).

      Key order is fixed to match the contract exactly.
      """
      return {
          "since": since,
          "until": until,
          "fetched_at": fetched_at,
          "sleep": sleep,
          "hrv": hrv,
          "body_battery": body_battery,
          "rhr": rhr,
          "vo2max": vo2max,
      }
  ```
  with:
  ```python
  def build_output(
      *,
      since: str,
      until: str,
      fetched_at: str,
      sleep: list,
      hrv: list,
      body_battery: list,
      rhr: list,
      vo2max: list,
      activities: list,
  ) -> dict:
      """Assemble the full worker stdout object (CONTRACTS §2.1).

      Key order is fixed to match the contract exactly.
      """
      return {
          "since": since,
          "until": until,
          "fetched_at": fetched_at,
          "sleep": sleep,
          "hrv": hrv,
          "body_battery": body_battery,
          "rhr": rhr,
          "vo2max": vo2max,
          "activities": activities,
      }
  ```
  NOTE: `activities` is now a REQUIRED keyword arg — `run_fetch` (Task 6) and `_run_dry_fetch` (Task 7) MUST be updated to pass it. The existing `test_fetcher.py` calls `run_fetch` (not `build_output` directly), so Task 6 keeps it green; `test_fetch_cli.py` monkeypatches `cli.run_fetch` with a fake that never calls `build_output`, so it is unaffected.

- [ ] **Step 4: Run — expect PASS (this test) + confirm existing fetcher tests now FAIL (drives Task 6).**
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest tests/test_normalize_activity.py::test_build_output_emits_activities_last -q`
  Expected: PASS.
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest tests/test_fetcher.py -q`
  Expected: FAIL — `run_fetch` still calls `build_output(...)` without `activities` → `TypeError: build_output() missing 1 required keyword-only argument: 'activities'`. This is the failing-test seam for Task 6.

- [ ] **Step 5: Commit (defer green of fetcher to Task 6 — commit together with Task 6).**
  Do NOT commit standalone (package is red). Proceed directly to Task 6; commit both at Task 6 Step 5.

---

### Task 6: Worker — run_fetch fetches+normalizes activities (degrade to []) + client delegate

**Files:**
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/client.py` (add `get_activities_by_date` delegate)
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/fetcher.py` (`run_fetch`: fetch+normalize activities, pass `activities=`)
- Test (Modify): `/home/jake/project/help-my-run/garmin-worker/tests/test_fetcher.py` (`_MockClient` gains `get_activities_by_date`; new asserts)

- [ ] **Step 1: Failing tests — `_MockClient.get_activities_by_date` + normalized activities shape + run-type filter call + degrade-to-[] on failure.**
  Edit `tests/test_fetcher.py`. First, extend `_MockClient` by TEXTUAL ANCHOR — replace the `__init__`. Replace:
  ```python
      def __init__(self, hrv_map=None, vo2max_map=None, raise_on=None):
          self.calls = []
          self._hrv_map = hrv_map or {}
          self._vo2max_map = vo2max_map or {}
          self._raise_on = raise_on  # (method_name, exception) to raise
  ```
  with:
  ```python
      def __init__(self, hrv_map=None, vo2max_map=None, raise_on=None, activities=None):
          self.calls = []
          self._hrv_map = hrv_map or {}
          self._vo2max_map = vo2max_map or {}
          self._raise_on = raise_on  # (method_name, exception) to raise
          self._activities = activities  # None -> default one running element
  ```
  Then add this method to `_MockClient` immediately after `get_max_metrics` (its `return [...]` block close is the TEXTUAL ANCHOR):
  ```python
      def get_activities_by_date(self, startdate, enddate=None, activitytype=None):
          self.calls.append(("activities", startdate, enddate, activitytype))
          self._maybe_raise("get_activities_by_date")
          if self._activities is not None:
              return self._activities
          return [
              {
                  "activityId": 14820001234,
                  "startTimeGMT": "2026-06-22 05:00:00",
                  "duration": 3300.0,
                  "distance": 10000.0,
                  "activityType": {"typeKey": "running"},
              }
          ]
  ```
  Append these new test functions to `tests/test_fetcher.py`:
  ```python
  def test_run_fetch_top_level_shape_includes_activities():
      mc = _MockClient()
      out = fetcher.run_fetch(
          mc, since="2026-06-14", until="2026-06-15",
          fetched_at="2026-06-15T05:00:12Z", sleep_fn=_noop_sleep,
      )
      assert list(out.keys()) == [
          "since", "until", "fetched_at", "sleep", "hrv", "body_battery",
          "rhr", "vo2max", "activities",
      ]


  def test_run_fetch_normalizes_activities():
      mc = _MockClient()
      out = fetcher.run_fetch(
          mc, since="2026-06-14", until="2026-06-15",
          fetched_at="t", sleep_fn=_noop_sleep,
      )
      assert len(out["activities"]) == 1
      a = out["activities"][0]
      assert a["garmin_activity_id"] == 14820001234
      assert a["start_time"] == "2026-06-22 05:00:00"
      assert a["duration_s"] == 3300.0
      assert a["distance_m"] == 10000.0
      assert a["activity_type"] == "running"
      assert "raw_json" in a


  def test_run_fetch_activities_uses_running_filter_over_window():
      mc = _MockClient()
      fetcher.run_fetch(
          mc, since="2026-06-14", until="2026-06-15",
          fetched_at="t", sleep_fn=_noop_sleep,
      )
      act_calls = [c for c in mc.calls if c[0] == "activities"]
      # one call, whole window, run-type filtered server-side.
      assert act_calls == [("activities", "2026-06-14", "2026-06-15", "running")]


  def test_run_fetch_skips_activities_without_id():
      mc = _MockClient(activities=[
          {"activityId": 1, "startTimeGMT": "2026-06-22 05:00:00",
           "duration": 100.0, "distance": 200.0, "activityType": {"typeKey": "running"}},
          {"startTimeGMT": "2026-06-22 06:00:00"},  # no activityId -> skipped
          "not-a-dict",                              # non-dict -> skipped
      ])
      out = fetcher.run_fetch(
          mc, since="2026-06-14", until="2026-06-15",
          fetched_at="t", sleep_fn=_noop_sleep,
      )
      assert [a["garmin_activity_id"] for a in out["activities"]] == [1]


  def test_run_fetch_activities_failure_degrades_to_empty():
      # A list-fetch failure must NOT fail the whole recovery sync (spec §10).
      mc = _MockClient(raise_on=("get_activities_by_date", RuntimeError("acts boom")))
      out = fetcher.run_fetch(
          mc, since="2026-06-14", until="2026-06-15",
          fetched_at="t", sleep_fn=_noop_sleep,
      )
      assert out["activities"] == []
      # Other sources still populated (degrade is isolated to activities).
      assert len(out["sleep"]) == 2
  ```

- [ ] **Step 2: Run — expect FAIL.**
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest tests/test_fetcher.py -q`
  Expected: FAIL — the legacy `test_run_fetch_top_level_shape_and_echo` fails (`run_fetch` still passes 7 keys / hits the `build_output` `TypeError` from Task 5), and the new tests fail (`activities` key missing / `_MockClient.get_activities_by_date` not exercised by `run_fetch`).
  NOTE: the legacy `test_run_fetch_top_level_shape_and_echo` asserts the 8-key set WITHOUT `activities`; it MUST be updated. Edit it by TEXTUAL ANCHOR — replace:
  ```python
      assert list(out.keys()) == [
          "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr", "vo2max",
      ]
  ```
  with:
  ```python
      assert list(out.keys()) == [
          "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr", "vo2max", "activities",
      ]
  ```

- [ ] **Step 3: Minimal impl — client delegate + run_fetch activities path.**
  (a) `client.py` — add the 1:1 delegate after `download_activity_original` (its closing `return self._g.download_activity(...)` line is the TEXTUAL ANCHOR). Append:
  ```python
      def get_activities_by_date(
          self, startdate: str, enddate: Optional[str] = None,
          activitytype: Optional[str] = None, sortorder: Optional[str] = None,
      ) -> list:
          return self._g.get_activities_by_date(startdate, enddate, activitytype, sortorder)
  ```
  (`Optional` is already imported in `client.py`. The 4-param signature is a 1:1 mirror of the real `garminconnect` method incl. `sortorder`; `run_fetch` calls it positionally as `get_activities_by_date(since, until, "running")` and relies on the `sortorder=None` default.)

  (b) `fetcher.py` `run_fetch` — after the per-day loop, before `return normalize.build_output(`, insert the activities fetch (try/except → `[]`), then pass `activities=activities`. Edit by TEXTUAL ANCHOR — replace:
  ```python
          if i < len(dates) - 1:
              sleep_fn(_PER_DAY_DELAY_S)

      return normalize.build_output(
          since=since,
          until=until,
          fetched_at=fetched_at,
          sleep=sleep,
          hrv=hrv,
          body_battery=body_battery,
          rhr=rhr,
          vo2max=vo2max,
      )
  ```
  with:
  ```python
          if i < len(dates) - 1:
              sleep_fn(_PER_DAY_DELAY_S)

      # Garmin activities list: one run-type-filtered call over the whole window.
      # A list-fetch failure must NOT fail the whole recovery sync (spec §10).
      try:
          raw_acts = client.get_activities_by_date(since, until, "running") or []
          activities = [
              normalize.normalize_garmin_activity(el)
              for el in raw_acts
              if isinstance(el, dict) and el.get("activityId") is not None
          ]
      except Exception:
          activities = []

      return normalize.build_output(
          since=since,
          until=until,
          fetched_at=fetched_at,
          sleep=sleep,
          hrv=hrv,
          body_battery=body_battery,
          rhr=rhr,
          vo2max=vo2max,
          activities=activities,
      )
  ```

- [ ] **Step 4: Run — expect PASS (full fetcher + normalize-activity suites).**
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest tests/test_fetcher.py tests/test_normalize_activity.py -q`
  Expected: PASS (legacy fetcher tests green again; new activities tests green; build_output key-order green).

- [ ] **Step 5: Commit (Task 5 + Task 6 together — package now green).**
  ```bash
  cd /home/jake/project/help-my-run && git add garmin-worker/garmin_worker/normalize.py garmin-worker/garmin_worker/client.py garmin-worker/garmin_worker/fetcher.py garmin-worker/tests/test_fetcher.py && git commit -m "feat(worker): emit activities list from fetch (run_fetch + client delegate + build_output)"
  ```

---

### Task 7: Worker CLI — _run_dry_fetch + fetch key-set assertion include `activities`

**Files:**
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/cli.py` (`_run_dry_fetch`: add synthetic `activities`, pass `activities=`)
- Test (Modify): `/home/jake/project/help-my-run/garmin-worker/tests/test_fetch_cli.py` (add a `--dry-run` activities key-set + content test)

- [ ] **Step 1: Failing test — `--dry-run` fetch output includes a non-empty `activities` list with the six keys.**
  Append to `tests/test_fetch_cli.py`:
  ```python
  def test_dry_run_fetch_includes_activities(capsys):
      rc = cli.main(["fetch", "--since", "2026-06-14", "--until", "2026-06-15", "--dry-run"])
      assert rc == 0
      captured = capsys.readouterr()
      assert captured.err == ""
      out = json.loads(captured.out)
      assert list(out.keys()) == [
          "since", "until", "fetched_at",
          "sleep", "hrv", "body_battery", "rhr", "vo2max", "activities",
      ]
      assert len(out["activities"]) >= 1
      a = out["activities"][0]
      assert set(a.keys()) == {
          "garmin_activity_id", "start_time", "duration_s",
          "distance_m", "activity_type", "raw_json",
      }
      assert a["garmin_activity_id"] is not None
      assert "running" in a["activity_type"]
  ```
  (Uses the real `_run_dry_fetch` path — no monkeypatch — so it exercises the synthetic activities + `build_output(activities=)`. Avoids the quirky `fake_run_fetch` in `test_fetch_live_success_prints_json` which is left untouched.)

- [ ] **Step 2: Run — expect FAIL.**
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest tests/test_fetch_cli.py::test_dry_run_fetch_includes_activities -q`
  Expected: FAIL — `_run_dry_fetch` calls `build_output(...)` without `activities` → `TypeError: build_output() missing 1 required keyword-only argument: 'activities'` (from Task 5).

- [ ] **Step 3: Minimal impl — add synthetic `_DRY_ACTIVITIES_RAW` + wire into `_run_dry_fetch`.**
  (a) `cli.py` — add a synthetic constant after `_DRY_VO2MAX_RAW` (its closing `}` is the TEXTUAL ANCHOR). Insert:
  ```python
  _DRY_ACTIVITIES_RAW = [
      {"activityId": 14820001234, "startTimeGMT": "2026-06-14 05:00:00", "duration": 3300.0, "distance": 10000.0, "activityType": {"typeKey": "running"}},
      {"activityId": 14820005678, "startTimeGMT": "2026-06-15 06:00:00", "duration": 2700.0, "distance": 8000.0, "activityType": {"typeKey": "trail_running"}},
  ]
  ```
  (b) `_run_dry_fetch` — edit by TEXTUAL ANCHOR. Replace:
  ```python
      vo2max = [normalize.normalize_vo2max_day(d, raw) for d, raw in sorted(_DRY_VO2MAX_RAW.items())]
      return normalize.build_output(
          since=since,
          until=until,
          fetched_at="2026-06-15T05:00:12Z",
          sleep=sleep,
          hrv=hrv,
          body_battery=body_battery,
          rhr=rhr,
          vo2max=vo2max,
      )
  ```
  with:
  ```python
      vo2max = [normalize.normalize_vo2max_day(d, raw) for d, raw in sorted(_DRY_VO2MAX_RAW.items())]
      activities = [normalize.normalize_garmin_activity(el) for el in _DRY_ACTIVITIES_RAW]
      return normalize.build_output(
          since=since,
          until=until,
          fetched_at="2026-06-15T05:00:12Z",
          sleep=sleep,
          hrv=hrv,
          body_battery=body_battery,
          rhr=rhr,
          vo2max=vo2max,
          activities=activities,
      )
  ```

- [ ] **Step 4: Run — expect PASS (new test + whole worker suite green).**
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest tests/test_fetch_cli.py::test_dry_run_fetch_includes_activities -q`
  Expected: PASS.
  Cmd: `cd /home/jake/project/help-my-run/garmin-worker && .venv/bin/python -m pytest -q`
  Expected: full worker suite PASS (existing `test_fetch_live_success_prints_json` 7-key fake untouched and still green; all new activities tests green).

- [ ] **Step 5: Commit.**
  ```bash
  cd /home/jake/project/help-my-run && git add garmin-worker/garmin_worker/cli.py garmin-worker/tests/test_fetch_cli.py && git commit -m "feat(worker): emit synthetic activities in --dry-run fetch"
  ```

---

### Task 8: Go WorkerOutput.Activities + GarminActivity struct

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/garmin/types.go` (`WorkerOutput` + new `GarminActivity`)
- Test (Create): `/home/jake/project/help-my-run/backend/internal/garmin/types_test.go`

- [ ] **Step 1: Failing test — unmarshal a worker JSON blob into `WorkerOutput`, assert `Activities`.**
  Create `backend/internal/garmin/types_test.go`:
  ```go
  package garmin

  import (
  	"encoding/json"
  	"testing"
  )

  func TestWorkerOutputUnmarshalActivities(t *testing.T) {
  	const blob = `{
  		"since":"2026-06-14","until":"2026-06-15","fetched_at":"t",
  		"sleep":[],"hrv":[],"body_battery":[],"rhr":[],"vo2max":[],
  		"activities":[
  			{"garmin_activity_id":14820001234,"start_time":"2026-06-22 05:00:00",
  			 "duration_s":3300.0,"distance_m":10000.0,"activity_type":"running",
  			 "raw_json":{"activityId":14820001234}},
  			{"garmin_activity_id":14820005678,"start_time":"2026-06-21 06:00:00",
  			 "duration_s":null,"distance_m":null,"activity_type":null,
  			 "raw_json":null}
  		]
  	}`
  	var out WorkerOutput
  	if err := json.Unmarshal([]byte(blob), &out); err != nil {
  		t.Fatalf("unmarshal: %v", err)
  	}
  	if len(out.Activities) != 2 {
  		t.Fatalf("Activities len = %d, want 2", len(out.Activities))
  	}
  	a := out.Activities[0]
  	if a.GarminActivityID != 14820001234 {
  		t.Errorf("GarminActivityID = %d, want 14820001234", a.GarminActivityID)
  	}
  	if a.StartTime != "2026-06-22 05:00:00" {
  		t.Errorf("StartTime = %q, want 2026-06-22 05:00:00", a.StartTime)
  	}
  	if a.DurationS == nil || *a.DurationS != 3300 {
  		t.Errorf("DurationS = %v, want 3300", a.DurationS)
  	}
  	if a.DistanceM == nil || *a.DistanceM != 10000 {
  		t.Errorf("DistanceM = %v, want 10000", a.DistanceM)
  	}
  	if a.ActivityType == nil || *a.ActivityType != "running" {
  		t.Errorf("ActivityType = %v, want running", a.ActivityType)
  	}
  	if string(a.RawJSON) != `{"activityId":14820001234}` {
  		t.Errorf("RawJSON = %s, want raw element", a.RawJSON)
  	}
  	// Null nested fields stay nil; raw_json:null -> RawJSON == "null".
  	b := out.Activities[1]
  	if b.DurationS != nil || b.DistanceM != nil || b.ActivityType != nil {
  		t.Errorf("null row: dur=%v dist=%v atype=%v, want all nil", b.DurationS, b.DistanceM, b.ActivityType)
  	}
  	if string(b.RawJSON) != "null" {
  		t.Errorf("null raw_json = %s, want literal null", b.RawJSON)
  	}
  }
  ```

- [ ] **Step 2: Run — expect FAIL (compile error).**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/garmin/ -run TestWorkerOutputUnmarshalActivities -v`
  Expected: FAIL — `out.Activities undefined (type WorkerOutput has no field or method Activities)` / `undefined: GarminActivity`.

- [ ] **Step 3: Minimal impl — extend `WorkerOutput` + add `GarminActivity` struct.**
  Edit `types.go` `WorkerOutput` by TEXTUAL ANCHOR. Replace:
  ```go
  type WorkerOutput struct {
  	Since       string           `json:"since"`
  	Until       string           `json:"until"`
  	FetchedAt   string           `json:"fetched_at"`
  	Sleep       []SleepDay       `json:"sleep"`
  	HRV         []HrvDay         `json:"hrv"`
  	BodyBattery []BodyBatteryDay `json:"body_battery"`
  	RHR         []RhrDay         `json:"rhr"`
  	VO2Max      []Vo2maxDay      `json:"vo2max"`
  }
  ```
  with:
  ```go
  type WorkerOutput struct {
  	Since       string           `json:"since"`
  	Until       string           `json:"until"`
  	FetchedAt   string           `json:"fetched_at"`
  	Sleep       []SleepDay       `json:"sleep"`
  	HRV         []HrvDay         `json:"hrv"`
  	BodyBattery []BodyBatteryDay `json:"body_battery"`
  	RHR         []RhrDay         `json:"rhr"`
  	VO2Max      []Vo2maxDay      `json:"vo2max"`
  	Activities  []GarminActivity `json:"activities"` // M3.2.1
  }

  // GarminActivity is one element of the Garmin activities list (§2.x).
  type GarminActivity struct {
  	GarminActivityID int64           `json:"garmin_activity_id"`
  	StartTime        string          `json:"start_time"`
  	DurationS        *float64        `json:"duration_s"`
  	DistanceM        *float64        `json:"distance_m"`
  	ActivityType     *string         `json:"activity_type"`
  	RawJSON          json.RawMessage `json:"raw_json"`
  }
  ```
  (`encoding/json` already imported in `types.go`.)

- [ ] **Step 4: Run — expect PASS.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/garmin/ -run TestWorkerOutputUnmarshalActivities -v`
  Expected: PASS.

- [ ] **Step 5: Commit.**
  ```bash
  cd /home/jake/project/help-my-run && git add backend/internal/garmin/types.go backend/internal/garmin/types_test.go && git commit -m "feat(garmin): add WorkerOutput.Activities + GarminActivity type"
  ```

---

### Task 9: SyncGarmin upserts activities into garmin_activities + fixture

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/sync/sync.go` (`SyncGarmin`: add activities upsert loop after the `out.VO2Max` loop)
- Modify: `/home/jake/project/help-my-run/backend/internal/garmin/testdata/worker_output.json` (add `"activities"` array, ≥1 running element)
- Test (Modify): `/home/jake/project/help-my-run/backend/internal/sync/sync_test.go` (`TestSyncGarminUpsertsAllTables`: bump expected `synced`, add `garmin_activities` count assertion, and a run-type/start_time assertion scoped via the `dailySleepDTO` anchor unique to that test; `TestSyncGarminBackfillWindowIs84Days`: add `"activities":[]` to its inline empty JSON)

- [ ] **Step 1: Failing test — add `"activities"` to fixture + assert ingest.**
  (a) Edit `backend/internal/garmin/testdata/worker_output.json` — add an `"activities"` array (2 running elements). Edit by TEXTUAL ANCHOR — replace the trailing `vo2max` block + closing brace:
  ```json
    "vo2max": [
      { "date": "2026-06-14", "vo2max": 51.0, "raw_json": {"generic": {"vo2MaxValue": 51.0}} },
      { "date": "2026-06-15", "vo2max": 52.0, "raw_json": {"generic": {"vo2MaxValue": 52.0}} }
    ]
  }
  ```
  with:
  ```json
    "vo2max": [
      { "date": "2026-06-14", "vo2max": 51.0, "raw_json": {"generic": {"vo2MaxValue": 51.0}} },
      { "date": "2026-06-15", "vo2max": 52.0, "raw_json": {"generic": {"vo2MaxValue": 52.0}} }
    ],
    "activities": [
      { "garmin_activity_id": 14820001234, "start_time": "2026-06-14 05:00:00", "duration_s": 3300.0, "distance_m": 10000.0, "activity_type": "running", "raw_json": {"activityId": 14820001234} },
      { "garmin_activity_id": 14820005678, "start_time": "2026-06-15 06:00:00", "duration_s": 2700.0, "distance_m": 8000.0, "activity_type": "trail_running", "raw_json": {"activityId": 14820005678} }
    ]
  }
  ```

  (b) Edit `sync_test.go` `TestSyncGarminUpsertsAllTables`. Bump the `synced` expectation (9 → 11: +2 activities) and add the count assertion. Replace by TEXTUAL ANCHOR:
  ```go
  	// Fixture has 2 sleep + 1 hrv + 2 bb + 2 rhr + 2 vo2max = 9 upserts.
  	if res.Synced != 9 {
  		t.Errorf("synced = %d, want 9", res.Synced)
  	}

  	counts := map[string]int{
  		"garmin_sleep": 0, "garmin_hrv": 0, "garmin_body_battery": 0, "garmin_rhr": 0,
  		"garmin_vo2max": 0,
  	}
  ```
  with:
  ```go
  	// Fixture has 2 sleep + 1 hrv + 2 bb + 2 rhr + 2 vo2max + 2 activities = 11 upserts.
  	if res.Synced != 11 {
  		t.Errorf("synced = %d, want 11", res.Synced)
  	}

  	counts := map[string]int{
  		"garmin_sleep": 0, "garmin_hrv": 0, "garmin_body_battery": 0, "garmin_rhr": 0,
  		"garmin_vo2max": 0, "garmin_activities": 0,
  	}
  ```
  And extend the assertion block — replace:
  ```go
  	if counts["garmin_sleep"] != 2 || counts["garmin_hrv"] != 1 ||
  		counts["garmin_body_battery"] != 2 || counts["garmin_rhr"] != 2 ||
  		counts["garmin_vo2max"] != 2 {
  		t.Errorf("counts = %+v, want sleep2 hrv1 bb2 rhr2 vo2max2", counts)
  	}
  ```
  with:
  ```go
  	if counts["garmin_sleep"] != 2 || counts["garmin_hrv"] != 1 ||
  		counts["garmin_body_battery"] != 2 || counts["garmin_rhr"] != 2 ||
  		counts["garmin_vo2max"] != 2 || counts["garmin_activities"] != 2 {
  		t.Errorf("counts = %+v, want sleep2 hrv1 bb2 rhr2 vo2max2 activities2", counts)
  	}
  ```
  Also add a raw_json/PK-correctness assertion in `TestSyncGarminUpsertsAllTables` ONLY. Insert it immediately AFTER that test's existing sleep raw_json check — the `t.Errorf` that references `dailySleepDTO` (its closing `}` is the TEXTUAL ANCHOR; `dailySleepDTO` is unique to `TestSyncGarminUpsertsAllTables`, so this scopes the edit unambiguously) — and BEFORE that same test's `sl, _ := s.GetSyncLog("garmin")` line (do NOT use the bare `GetSyncLog("garmin")` line as the anchor — it occurs in two tests). Append:
  ```go
  	// Activity ingested with run-type + start_time persisted.
  	var atype, ast string
  	_ = s.DB.QueryRow(
  		`SELECT activity_type, start_time FROM garmin_activities WHERE garmin_activity_id=?`,
  		14820001234).Scan(&atype, &ast)
  	if atype != "running" || ast != "2026-06-14 05:00:00" {
  		t.Errorf("garmin_activity 14820001234 = (%q,%q), want (running, 2026-06-14 05:00:00)", atype, ast)
  	}
  ```

  (c) Edit `sync_test.go` `TestSyncGarminBackfillWindowIs84Days` — its inline empty JSON must include `"activities":[]` so the unmarshal stays well-formed. Replace by TEXTUAL ANCHOR:
  ```go
  		`echo '{"since":"x","until":"x","fetched_at":"x","sleep":[],"hrv":[],"body_battery":[],"rhr":[],"vo2max":[]}'` + "\n"
  ```
  with:
  ```go
  		`echo '{"since":"x","until":"x","fetched_at":"x","sleep":[],"hrv":[],"body_battery":[],"rhr":[],"vo2max":[],"activities":[]}'` + "\n"
  ```

- [ ] **Step 2: Run — expect FAIL.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/sync/ -run TestSyncGarminUpsertsAllTables -v`
  Expected: FAIL — `synced = 9, want 11` and `count garmin_activities: ... = 0` (SyncGarmin does not yet upsert activities; the loop is missing).

- [ ] **Step 3: Minimal impl — add the activities upsert loop in `SyncGarmin`.**
  Edit `sync.go` `SyncGarmin` by TEXTUAL ANCHOR. Insert the new loop after the `out.VO2Max` loop and before `return okResult(...)`. Replace:
  ```go
  	for _, d := range out.VO2Max {
  		if err := s.UpsertVo2max(store.Vo2maxRow{
  			Date: d.Date, Vo2max: d.VO2Max, RawJSON: rawString(d.RawJSON),
  		}); err != nil {
  			return errResult(s, source, err)
  		}
  		synced++
  	}
  	return okResult(s, source, synced)
  ```
  with:
  ```go
  	for _, d := range out.VO2Max {
  		if err := s.UpsertVo2max(store.Vo2maxRow{
  			Date: d.Date, Vo2max: d.VO2Max, RawJSON: rawString(d.RawJSON),
  		}); err != nil {
  			return errResult(s, source, err)
  		}
  		synced++
  	}
  	for _, a := range out.Activities {
  		if err := s.UpsertGarminActivity(store.GarminActivityRow{
  			GarminActivityID: a.GarminActivityID,
  			StartTime:        a.StartTime,
  			DurationS:        a.DurationS,
  			DistanceM:        a.DistanceM,
  			ActivityType:     a.ActivityType,
  			RawJSON:          rawString(a.RawJSON),
  		}); err != nil {
  			return errResult(s, source, err)
  		}
  		synced++
  	}
  	return okResult(s, source, synced)
  ```
  (Mirrors the existing upsert loops + `rawString` helper. Recent window reuses the existing `since` already passed to `RunGarminFetch` → `get_activities_by_date` in the worker; no new since/until. `SyncGarmin` signature unchanged.)

- [ ] **Step 4: Run — expect PASS (sync + dependent packages).**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/sync/ -run 'TestSyncGarmin' -v`
  Expected: PASS (`TestSyncGarminUpsertsAllTables` 11 upserts + 2 garmin_activities + run-type assertion; `TestSyncGarminError`/`TestSyncGarminBackfillWindowIs84Days` still green).
  Cmd: `cd /home/jake/project/help-my-run/backend && go build ./... && go test ./internal/sync/... ./internal/garmin/... ./internal/store/...`
  Expected: build + all three packages PASS.

- [ ] **Step 5: Commit.**
  ```bash
  cd /home/jake/project/help-my-run && git add backend/internal/sync/sync.go backend/internal/sync/sync_test.go backend/internal/garmin/testdata/worker_output.json && git commit -m "feat(sync): ingest garmin_activities in SyncGarmin"
  ```

---

### Task 10: Config knob `GarminMatchToleranceS` (`GARMIN_MATCH_TOLERANCE_S`, default 120)

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/config/config.go` (struct `Config`, after the M3.2 stream block)
- Modify: `/home/jake/project/help-my-run/.env.example` (next to `GARMIN_*` vars)
- Test (Modify/Create): `/home/jake/project/help-my-run/backend/internal/config/config_test.go` (`TestLoadGarminMatchToleranceDefault` + override)

This task is independent of Tasks 11–12; do it before them so the knob exists to thread into the engine.

- [ ] **Step 1: Write the failing test.** Append to `config_test.go` (create it if absent — if creating, prepend `package config` + imports `os`, `testing`). The default case needs the three `required` vars set so `Load()` succeeds. FULL test code:
  ```go
  func TestLoadGarminMatchToleranceDefault(t *testing.T) {
  	t.Setenv("STRAVA_CLIENT_ID", "id")
  	t.Setenv("STRAVA_CLIENT_SECRET", "secret")
  	t.Setenv("STRAVA_REDIRECT_URL", "http://cb")
  	t.Setenv("API_TOKEN", "tok")
  	os.Unsetenv("GARMIN_MATCH_TOLERANCE_S")

  	c, err := Load()
  	if err != nil {
  		t.Fatalf("Load() error = %v", err)
  	}
  	if c.GarminMatchToleranceS != 120 {
  		t.Errorf("GarminMatchToleranceS = %d, want 120 (default)", c.GarminMatchToleranceS)
  	}
  }

  func TestLoadGarminMatchToleranceOverride(t *testing.T) {
  	t.Setenv("STRAVA_CLIENT_ID", "id")
  	t.Setenv("STRAVA_CLIENT_SECRET", "secret")
  	t.Setenv("STRAVA_REDIRECT_URL", "http://cb")
  	t.Setenv("API_TOKEN", "tok")
  	t.Setenv("GARMIN_MATCH_TOLERANCE_S", "45")

  	c, err := Load()
  	if err != nil {
  		t.Fatalf("Load() error = %v", err)
  	}
  	if c.GarminMatchToleranceS != 45 {
  		t.Errorf("GarminMatchToleranceS = %d, want 45 (override)", c.GarminMatchToleranceS)
  	}
  }
  ```

- [ ] **Step 2: Run — expect FAIL (compile error: undefined field `GarminMatchToleranceS`).**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/config/ -run TestLoadGarminMatchTolerance -v`
  Expected: `c.GarminMatchToleranceS undefined (type *Config has no field or method GarminMatchToleranceS)`.

- [ ] **Step 3: Minimal impl.** In `config.go`, edit by TEXTUAL ANCHOR — find the M3.2 block:
  ```go
  	// M3.2: stream fetch trickle.
  	StreamRecentWeeks int `envconfig:"STREAM_RECENT_WEEKS" default:"12"`
  	StreamFetchBudget int `envconfig:"STREAM_FETCH_BUDGET" default:"10"`
  ```
  Replace with (append the new field after it):
  ```go
  	// M3.2: stream fetch trickle.
  	StreamRecentWeeks int `envconfig:"STREAM_RECENT_WEEKS" default:"12"`
  	StreamFetchBudget int `envconfig:"STREAM_FETCH_BUDGET" default:"10"`

  	// M3.2.1: Garmin .FIT fallback start-time match tolerance (seconds).
  	GarminMatchToleranceS int `envconfig:"GARMIN_MATCH_TOLERANCE_S" default:"120"`
  ```

- [ ] **Step 4: Document in `.env.example`.** Edit by ANCHOR — find the existing Garmin block (the `GARMIN_TOKENSTORE=` line) and add below it:
  ```
  # M3.2.1: Garmin .FIT fallback start-time match tolerance (seconds).
  GARMIN_MATCH_TOLERANCE_S=120
  ```

- [ ] **Step 5: Run — expect PASS.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/config/ -run TestLoadGarminMatchTolerance -v`
  Expected: `ok  help-my-run/backend/internal/config` (both subtests PASS).

- [ ] **Step 6: Commit.**
  ```bash
  cd /home/jake/project/help-my-run && git add backend/internal/config/config.go backend/internal/config/config_test.go .env.example && git commit -m "feat(config): add GARMIN_MATCH_TOLERANCE_S knob (default 120) for M3.2.1 Garmin .FIT match" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 11: Activate `resolveGarminID` — replace `(0,false)` stub with start-time + duration/distance match

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/store/activities.go` — add `GetActivity(stravaID)` getter + `"errors"` import (CONTRACTS §C3, if not already present)
- Modify: `/home/jake/project/help-my-run/backend/internal/streams/engine.go` — struct `Engine`, func `New`, func `resolveGarminID`; add `absF` helper
- Modify: `/home/jake/project/help-my-run/backend/cmd/server/main.go` — the `streams.New(...)` call site (the single `streams.New(s, stravaClient, runner, extraEnv)`)
- Create/Test: `/home/jake/project/help-my-run/backend/internal/streams/helpers_test.go` — `f64p`/`strp` pointer helpers for the `streams` test package (Step 0; reused by Task 12)
- Modify/Test: `/home/jake/project/help-my-run/backend/internal/streams/engine_test.go` — `newTestEngine` `New(...)` call; add `TestResolveGarminID` (table-driven)

**Depends on:** Task 10 (`cfg.GarminMatchToleranceS`) and the store tasks (Task 2 `UpsertGarminActivity`, Task 3 `GarminActivityCandidate`/`FindGarminActivitiesNear`, Task 1 migration `00007`). Those store symbols MUST exist before this task's test compiles.

- [ ] **Step 0: Add pointer helpers `f64p`/`strp` to the `streams` test package (REQUIRED — they do NOT exist there).**
  `f64p`/`strp` are defined ONLY in `backend/internal/store/store_test.go` (package `store`); the `streams` test package has none. `TestResolveGarminID` (Step 3) and `TestFetchAndAnalyzeGarminFallbackActivates` (Task 12) both call `f64p(...)`/`strp(...)`, so they would fail to compile (`undefined: f64p` / `undefined: strp`) without these. Create a NEW file `/home/jake/project/help-my-run/backend/internal/streams/helpers_test.go` (package `streams`) with the EXACT signatures from `store_test.go`:
  ```go
  package streams

  // f64p / strp are pointer helpers for table-driven tests (M3.2.1).
  // Same signatures as store_test.go's f64p/strp; defined here because the
  // streams test package has its own scope (those store helpers are not visible).
  func f64p(v float64) *float64 { return &v }
  func strp(v string) *string   { return &v }
  ```
  Define them ONCE, here. Task 12 reuses these (same package) and MUST NOT redefine them — a second declaration in package `streams` is a duplicate-symbol compile error. If a future `f64p`/`strp` already exists anywhere in the `streams` test package, skip this file and reuse the existing ones.

- [ ] **Step 1: Add `GetActivity(stravaID)` to `store/activities.go` (consumed by `resolveGarminID`).**
  Append to `activities.go` (mirrors `GetActivityStream`'s `QueryRow` + `sql.ErrNoRows`→`ErrNotFound` pattern, selecting the same columns as `ListActivities`):
  ```go
  // GetActivity returns one activity by strava_id, or ErrNotFound. raw_json is not loaded.
  func (s *Store) GetActivity(stravaID int64) (Activity, error) {
  	var a Activity
  	err := s.DB.QueryRow(`
  		SELECT strava_id, name, type, sport_type, start_time, start_time_local,
  		       distance_m, moving_time_s, elapsed_time_s,
  		       avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m
  		FROM activities
  		WHERE strava_id = ?`, stravaID).Scan(
  		&a.StravaID, &a.Name, &a.Type, &a.SportType, &a.StartTime, &a.StartTimeLocal,
  		&a.DistanceM, &a.MovingTimeS, &a.ElapsedTimeS,
  		&a.AvgHR, &a.MaxHR, &a.AvgSpeed, &a.MaxSpeed, &a.AvgCadence, &a.ElevationGainM)
  	if errors.Is(err, sql.ErrNoRows) {
  		return Activity{}, ErrNotFound
  	}
  	if err != nil {
  		return Activity{}, err
  	}
  	return a, nil
  }
  ```
  Add the `"errors"` import to `activities.go` (the import block is the TEXTUAL ANCHOR; `"database/sql"` is already imported). Confirm the `Activity` struct's field names match the scan targets above; if any differ, MATCH the M0 names in `store/activities.go` (do not invent).

- [ ] **Step 2: Thread the tolerance into the harness FIRST (so the package still compiles for the new test).** In `engine_test.go`, edit by ANCHOR — the current `newTestEngine` body:
  ```go
  	// Strava client + runner are unused by GetOrComputeAnalysis; pass minimal values.
  	return New(s, strava.NewWithBase("1", "x", "http://cb", "http://unused"), garmin.Runner{}, nil)
  ```
  Replace with (append the `120` tolerance arg — matches the new `New` signature):
  ```go
  	// Strava client + runner are unused by GetOrComputeAnalysis; pass minimal values.
  	// matchToleranceS=120 (M3.2.1 default); existing GetOrComputeAnalysis tests never call resolveGarminID.
  	return New(s, strava.NewWithBase("1", "x", "http://cb", "http://unused"), garmin.Runner{}, nil, 120)
  ```

- [ ] **Step 3: Write the failing table-driven test.** Append to `engine_test.go`. Seeds `garmin_activities` via `s.UpsertGarminActivity` + the Strava `activities` row via `s.UpsertActivity`, then asserts `resolveGarminID` (called directly — same package). FULL test code:
  ```go
  func TestResolveGarminID(t *testing.T) {
  	// Strava reference run: starts 2026-06-22T05:00:00Z, moving 1800s, 5000m.
  	const stravaID = int64(900)
  	const startISO = "2026-06-22T05:00:00Z"

  	type cand struct {
  		id      int64
  		start   string
  		durS    *float64 // nil -> NULL duration_s
  		distM   *float64 // nil -> NULL distance_m
  		actType *string  // nil -> NULL activity_type (excluded by LIKE '%running%')
  	}
  	run := strp("running")
  	trail := strp("trail_running")
  	bike := strp("cycling")

  	tests := []struct {
  		name    string
  		seedAct bool // upsert the Strava activities row?
  		cands   []cand
  		wantID  int64
  		wantOK  bool
  	}{
  		{
  			name:    "single match within tolerance",
  			seedAct: true,
  			cands: []cand{
  				{id: 1, start: "2026-06-22T05:00:30Z", durS: f64p(1805), distM: f64p(5010), actType: run},
  			},
  			wantID: 1, wantOK: true,
  		},
  		{
  			name:    "no candidate within tolerance -> false",
  			seedAct: true,
  			cands: []cand{
  				{id: 2, start: "2026-06-22T05:10:00Z", durS: f64p(1800), distM: f64p(5000), actType: run}, // +600s > 120
  			},
  			wantID: 0, wantOK: false,
  		},
  		{
  			name:    "tie-break by closest duration (vs MovingTimeS)",
  			seedAct: true,
  			cands: []cand{
  				{id: 3, start: "2026-06-22T05:00:10Z", durS: f64p(1900), distM: f64p(5000), actType: run}, // |1900-1800|=100
  				{id: 4, start: "2026-06-22T05:00:20Z", durS: f64p(1810), distM: f64p(9999), actType: run}, // |1810-1800|=10  (wins on duration)
  			},
  			wantID: 4, wantOK: true,
  		},
  		{
  			name:    "tie-break by distance when duration ties",
  			seedAct: true,
  			cands: []cand{
  				{id: 5, start: "2026-06-22T05:00:10Z", durS: f64p(1820), distM: f64p(5200), actType: run},   // durDelta=20, distDelta=200
  				{id: 6, start: "2026-06-22T05:00:20Z", durS: f64p(1820), distM: f64p(5010), actType: trail}, // durDelta=20, distDelta=10 (wins)
  			},
  			wantID: 6, wantOK: true,
  		},
  		{
  			name:    "non-run candidate excluded -> false",
  			seedAct: true,
  			cands: []cand{
  				{id: 7, start: "2026-06-22T05:00:05Z", durS: f64p(1800), distM: f64p(5000), actType: bike},
  			},
  			wantID: 0, wantOK: false,
  		},
  		{
  			name:    "unknown strava activity -> false",
  			seedAct: false,
  			cands: []cand{
  				{id: 8, start: "2026-06-22T05:00:05Z", durS: f64p(1800), distM: f64p(5000), actType: run},
  			},
  			wantID: 0, wantOK: false,
  		},
  	}

  	for _, tt := range tests {
  		t.Run(tt.name, func(t *testing.T) {
  			s := newStreamsStore(t)
  			if tt.seedAct {
  				if err := s.UpsertActivity(store.Activity{
  					StravaID: stravaID, Name: "run", Type: "Run",
  					StartTime: startISO, DistanceM: 5000, MovingTimeS: 1800, ElapsedTimeS: 1850, RawJSON: "{}",
  				}); err != nil {
  					t.Fatalf("upsert activity: %v", err)
  				}
  			}
  			for _, c := range tt.cands {
  				if err := s.UpsertGarminActivity(store.GarminActivityRow{
  					GarminActivityID: c.id, StartTime: c.start, DurationS: c.durS,
  					DistanceM: c.distM, ActivityType: c.actType, RawJSON: "null",
  				}); err != nil {
  					t.Fatalf("upsert garmin activity %d: %v", c.id, err)
  				}
  			}
  			e := newTestEngine(t, s)
  			gid, ok := e.resolveGarminID(stravaID)
  			if ok != tt.wantOK || gid != tt.wantID {
  				t.Errorf("resolveGarminID = (%d,%v), want (%d,%v)", gid, ok, tt.wantID, tt.wantOK)
  			}
  		})
  	}
  }
  ```

- [ ] **Step 4: Run — expect FAIL.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/streams/ -run TestResolveGarminID -v`
  Expected before the engine edit: `too many arguments in call to New` (Step 2 added the 5th arg) → then after the New-signature edit but before the body edit: subtests "single match within tolerance"/"tie-break…" FAIL with `resolveGarminID = (0,false), want (1,true)`.

- [ ] **Step 5: Minimal impl — thread `matchToleranceS` into the Engine.** In `engine.go`, edit by ANCHOR the struct:
  ```go
  type Engine struct {
  	store    *store.Store
  	strava   *strava.Client
  	runner   garmin.Runner
  	extraEnv []string
  }
  ```
  Replace with:
  ```go
  type Engine struct {
  	store           *store.Store
  	strava          *strava.Client
  	runner          garmin.Runner
  	extraEnv        []string
  	matchToleranceS int // M3.2.1: GARMIN_MATCH_TOLERANCE_S — start-time match window (s).
  }
  ```
  Then edit by ANCHOR the constructor:
  ```go
  func New(s *store.Store, sc *strava.Client, runner garmin.Runner, extraEnv []string) *Engine {
  	return &Engine{store: s, strava: sc, runner: runner, extraEnv: extraEnv}
  }
  ```
  Replace with:
  ```go
  func New(s *store.Store, sc *strava.Client, runner garmin.Runner, extraEnv []string, matchToleranceS int) *Engine {
  	return &Engine{store: s, strava: sc, runner: runner, extraEnv: extraEnv, matchToleranceS: matchToleranceS}
  }
  ```

- [ ] **Step 6: Minimal impl — replace the `resolveGarminID` body + add `absF`.** In `engine.go`, edit by ANCHOR — replace the ENTIRE current stub (the doc comment block plus body, from `// resolveGarminID maps a Strava activity id` through the closing `}` of `func (e *Engine) resolveGarminID...{ ... return 0, false }`) with:
  ```go
  // resolveGarminID maps a Strava activity id to a Garmin download id by lazy
  // start-time match (±matchToleranceS) over garmin_activities, tie-broken by
  // closest duration (vs the Strava MovingTimeS) then closest distance. Run-type
  // filtering happens in the store query (LIKE '%running%'). Returns ok=false to
  // skip the FIT fallback (unknown activity, no candidate in window, or store error
  // → graceful degrade to the Strava no-HR series). M3.2.1 activates the dormant
  // M3.2 FIT fallback by replacing the prior (0,false) stub.
  func (e *Engine) resolveGarminID(stravaActivityID int64) (int64, bool) {
  	act, err := e.store.GetActivity(stravaActivityID)
  	if err != nil {
  		return 0, false // unknown activity → no match (graceful degrade)
  	}
  	cands, err := e.store.FindGarminActivitiesNear(act.StartTime, e.matchToleranceS)
  	if err != nil || len(cands) == 0 {
  		return 0, false
  	}

  	bestID := int64(0)
  	bestSet := false
  	var bestDur, bestDist float64
  	target := float64(act.MovingTimeS)
  	for _, c := range cands {
  		durDelta := 1e18
  		if c.DurationS != nil {
  			durDelta = absF(*c.DurationS - target)
  		}
  		distDelta := 1e18
  		if c.DistanceM != nil {
  			distDelta = absF(*c.DistanceM - act.DistanceM)
  		}
  		if !bestSet || durDelta < bestDur || (durDelta == bestDur && distDelta < bestDist) {
  			bestID, bestDur, bestDist, bestSet = c.GarminActivityID, durDelta, distDelta, true
  		}
  	}
  	if !bestSet {
  		return 0, false
  	}
  	return bestID, true
  }

  // absF is the float64 absolute value (avoids a math import for one call).
  func absF(x float64) float64 {
  	if x < 0 {
  		return -x
  	}
  	return x
  }
  ```

- [ ] **Step 7: Minimal impl — update the `main.go` call site.** In `cmd/server/main.go`, edit by ANCHOR:
  ```go
  	streamsEngine := streams.New(s, stravaClient, runner, extraEnv)
  ```
  Replace with:
  ```go
  	streamsEngine := streams.New(s, stravaClient, runner, extraEnv, cfg.GarminMatchToleranceS)
  ```
  (Confirm the loaded config var is named `cfg` at this call site; if it differs, MATCH that local name.)

- [ ] **Step 8: Run — expect PASS (resolveGarminID test + full streams + build).**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/streams/ -run TestResolveGarminID -v && go build ./... && go test ./internal/streams/ ./internal/config/ ./cmd/...`
  Expected: all `TestResolveGarminID` subtests PASS; `go build ./...` succeeds (main.go call site fixed); existing `GetOrComputeAnalysis*` tests still PASS (harness now passes `120`).

- [ ] **Step 9: Commit.**
  ```bash
  cd /home/jake/project/help-my-run && git add backend/internal/store/activities.go backend/internal/streams/engine.go backend/internal/streams/engine_test.go backend/internal/streams/helpers_test.go backend/cmd/server/main.go && git commit -m "feat(streams): activate resolveGarminID start-time+duration/distance match (M3.2.1)" -m "Replace the dormant (0,false) stub with a lazy garmin_activities match within GARMIN_MATCH_TOLERANCE_S, tie-broken by duration then distance; thread the knob through Engine + main.go; add store.GetActivity." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 12: End-to-end activation test — no-HR Strava stream + seeded `garmin_activities` + stub FIT runner → `source=garmin`

**Files:**
- Test: `/home/jake/project/help-my-run/backend/internal/streams/engine_test.go` — add `TestFetchAndAnalyzeGarminFallbackActivates`

**Depends on:** Task 11 (active `resolveGarminID`), Task 10 (knob), and the store tasks (`UpsertGarminActivity`, migration `00007`). No production-code change — this exercises the now-lit `FetchAndAnalyze` fallback block end-to-end.

This test stands up THREE seams the existing engine tests never touched:
1. **Strava HTTP** — `httptest.Server` returning a no-HR `StreamSet` JSON (`time`/`velocity_smooth`/`distance`, NO `heartrate` key), wired via `strava.NewWithBase(id, secret, cb, srv.URL)`.
2. **Strava token** — `FetchAndAnalyze` calls `e.accessToken(ctx)` → `s.GetStravaTokens()`; seed a non-expired token row so no refresh HTTP call is made.
3. **FIT runner** — a `/bin/sh` stub script (the `runner_test.go` pattern) echoing FIT JSON WITH HR, wired as `garmin.Runner{Python:"/bin/sh", Script: script}`.

- [ ] **Step 1: Confirm the no-HR StreamSet wire shape (read-only, drives the fixture).** A no-HR Strava stream OMITS the `heartrate` key entirely (`strava/types.go`: "A missing-HR run OMITS the `heartrate` key"). `Stream` is `{ "data": [...] }` (json tag `data`). So the server body is:
  `{"time":{"data":[0,1,2,3]},"velocity_smooth":{"data":[2,2,2,2]},"distance":{"data":[0,2,4,6]}}`
  `FromStravaStreams` → `Series{T:[0,1,2,3], V:[2,2,2,2], Dist:[0,2,4,6], HR:nil}` → `HasHR()==false` → fallback fires.

- [ ] **Step 2: Write the failing end-to-end test.** Append to `engine_test.go`. Add imports `net/http`, `net/http/httptest`, `os`, `runtime`, `time` to the file's import block (only the ones not already present). This test calls `f64p(...)`/`strp(...)` — they are ALREADY defined in `streams/helpers_test.go` (Task 11 Step 0, same package); do NOT redefine them here (a duplicate `func f64p`/`func strp` in package `streams` is a compile error). FULL test code:
  ```go
  func TestFetchAndAnalyzeGarminFallbackActivates(t *testing.T) {
  	if runtime.GOOS == "windows" {
  		t.Skip("uses /bin/sh for the FIT runner stub")
  	}
  	const stravaID = int64(14820001234)
  	const garminID = int64(555)
  	const startISO = "2026-06-22T05:00:00Z"

  	s := newStreamsStore(t)

  	// (1) Strava activity row (so resolveGarminID can load start/duration/distance).
  	if err := s.UpsertActivity(store.Activity{
  		StravaID: stravaID, Name: "no-HR run", Type: "Run",
  		StartTime: startISO, DistanceM: 5000, MovingTimeS: 1800, ElapsedTimeS: 1850, RawJSON: "{}",
  	}); err != nil {
  		t.Fatalf("upsert activity: %v", err)
  	}

  	// (2) Seeded garmin_activities row that matches within tolerance (run-type).
  	if err := s.UpsertGarminActivity(store.GarminActivityRow{
  		GarminActivityID: garminID, StartTime: "2026-06-22T05:00:20Z",
  		DurationS: f64p(1805), DistanceM: f64p(5010), ActivityType: strp("running"), RawJSON: "null",
  	}); err != nil {
  		t.Fatalf("upsert garmin activity: %v", err)
  	}

  	// (3) Non-expired Strava token so accessToken() does NOT attempt a refresh HTTP call.
  	if err := s.SaveStravaTokens(store.StravaTokens{
  		AccessToken: "live-token", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour).Unix(),
  		Scope: "read", AthleteID: 1,
  	}); err != nil {
  		t.Fatalf("save tokens: %v", err)
  	}

  	// (4) Strava streams HTTP stub: NO "heartrate" key -> a no-HR stream.
  	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  		w.Header().Set("Content-Type", "application/json")
  		_, _ = w.Write([]byte(`{"time":{"data":[0,1,2,3]},"velocity_smooth":{"data":[2.0,2.0,2.0,2.0]},"distance":{"data":[0,2,4,6]}}`))
  	}))
  	t.Cleanup(srv.Close)
  	sc := strava.NewWithBase("1", "x", "http://cb", srv.URL)

  	// (5) FIT runner stub: a /bin/sh script echoing the FIT JSON WITH HR. It
  	// asserts it was called with the resolved garmin id + the strava echo id.
  	const fitOut = `{"activity_id":14820001234,"source":"garmin","fetched_at":"2026-06-22T05:00:12Z","series":{"t":[0,1,2,3],"hr":[140,142,150,152],"v":[2.0,2.0,2.0,2.0],"dist":[0,2,4,6]}}`
  	script := filepath.Join(t.TempDir(), "fitstub.sh")
  	body := "#!/bin/sh\n" +
  		"echo \"$@\" | grep -q -- '--activity-id 555' || { echo 'missing --activity-id 555' 1>&2; exit 2; }\n" +
  		"echo \"$@\" | grep -q -- '--echo-id 14820001234' || { echo 'missing --echo-id 14820001234' 1>&2; exit 2; }\n" +
  		"echo '" + fitOut + "'\n"
  	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
  		t.Fatalf("write fit stub: %v", err)
  	}
  	runner := garmin.Runner{Python: "/bin/sh", Script: script}

  	// matchToleranceS=120: the 20s start delta is within tolerance.
  	e := New(s, sc, runner, nil, 120)

  	got, err := e.FetchAndAnalyze(context.Background(), stravaID)
  	if err != nil {
  		t.Fatalf("FetchAndAnalyze error = %v", err)
  	}

  	// Source flipped to garmin (the fallback supplied HR via the .FIT).
  	if got.Source != "garmin" {
  		t.Errorf("Source = %q, want garmin (FIT fallback activated)", got.Source)
  	}
  	if !got.HasHR {
  		t.Errorf("HasHR = false, want true (Garmin .FIT carried HR)")
  	}
  	// Time-in-zone present (one bucket per zone) and decoupling computed.
  	if len(got.TimeInZone) == 0 {
  		t.Errorf("TimeInZone empty, want zone buckets from the Garmin HR series")
  	}
  	if got.DecouplingPct == nil {
  		t.Errorf("DecouplingPct = nil, want computed (4-sample HR+pace series)")
  	}

  	// The stored raw stream is persisted with source=garmin.
  	raw, err := s.GetActivityStream(stravaID)
  	if err != nil {
  		t.Fatalf("GetActivityStream: %v", err)
  	}
  	if raw.Source != "garmin" {
  		t.Errorf("stored activity_streams.source = %q, want garmin", raw.Source)
  	}
  	ser, err := DecompressSeries(raw.SeriesGz)
  	if err != nil {
  		t.Fatalf("decompress stored series: %v", err)
  	}
  	if len(ser.HR) != 4 || ser.HR[0] != 140 {
  		t.Errorf("stored series.HR = %v, want [140 142 150 152] (from the .FIT)", ser.HR)
  	}
  }
  ```

- [ ] **Step 3: Run — expect FAIL only on a missing dependency name; otherwise this is the green-bar driver.** Run after Task 11:
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/streams/ -run TestFetchAndAnalyzeGarminFallbackActivates -v`
  Expected interim FAIL signals to watch (each maps to a contract symbol the store/engine tasks supply): `undefined: store.GarminActivityRow` / `s.UpsertGarminActivity undefined` (store task incomplete) or `no such table: garmin_activities` (migration `00007` missing). If those tasks are done, expect PASS directly (no new production code in this task).
  Verify the precondition seams: `store.StravaTokens` + `s.SaveStravaTokens` + `s.GetStravaTokens` exist (M0); `store.ActivityStream.Source` + `s.GetActivityStream` exist (M3.2). If `SaveStravaTokens`/`StravaTokens` field names differ, MATCH the M0 names from `store/tokens.go` (do not invent). Likewise confirm the FIT runner CLI flag names (`--activity-id`, `--echo-id`) against `garmin/runner_test.go`; MATCH the live flags if they differ.

- [ ] **Step 4: Run — expect PASS.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/streams/ -run TestFetchAndAnalyzeGarminFallbackActivates -v`
  Expected: `--- PASS: TestFetchAndAnalyzeGarminFallbackActivates` — asserts `Source=="garmin"`, `HasHR`, non-empty `TimeInZone`, non-nil `DecouplingPct`, and stored `activity_streams.source=="garmin"` with the .FIT HR series.

- [ ] **Step 5: Full streams-package regression.**
  Cmd: `cd /home/jake/project/help-my-run/backend && go test ./internal/streams/ -v`
  Expected: all engine tests PASS (the existing `GetOrComputeAnalysis*` set + `TestResolveGarminID` + this activation test).

- [ ] **Step 6: Commit.**
  ```bash
  cd /home/jake/project/help-my-run && git add backend/internal/streams/engine_test.go && git commit -m "test(streams): end-to-end Garmin .FIT fallback activation (no-HR Strava + seeded garmin_activities + stub FIT runner -> source=garmin)" -m "Stubs the three seams the M3.2 tests never hit: httptest Strava streams (no heartrate key), a seeded non-expired token, and a /bin/sh FIT runner echoing a HR series. Asserts source=garmin + time-in-zone/decoupling computed (M3.2.1)." -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
  ```

---

### Task 13: App — run-detail Garmin source badge ("HR via Garmin .FIT")

**Files:**
- Modify: `/home/jake/project/help-my-run/app/app/run/[id].tsx` — `RunDetailScreen` return JSX (the `a && a.has_stream && a.has_hr` block) + `StyleSheet.create({...})`
- Test: `/home/jake/project/help-my-run/app/app/__tests__/run-detail.test.tsx` — add a `describe('RunDetailScreen — source badge')` block
- NO CHANGE: `/home/jake/project/help-my-run/app/src/api/types.ts` (`StreamAnalysis.source: 'strava' | 'garmin' | ''` already present; Go wire DTO already emits it — do NOT touch)

This task is fully independent (it mocks the analysis), so it can land in any order. Per `app/AGENTS.md`: Expo SDK 56 — read `https://docs.expo.dev/versions/v56.0.0/` before editing. Only RN `View`/`Text`/`StyleSheet` used (all already imported in `[id].tsx`).

- [ ] **Step 1: Read Expo SDK 56 docs (required by `app/AGENTS.md`).**
  Open `https://docs.expo.dev/versions/v56.0.0/` (use WebFetch). This task only uses React Native `View`/`Text`/`StyleSheet` (no Expo SDK API surface), so confirm no API drift then proceed.

- [ ] **Step 2: Write the FAILING test (FULL code).**
  Append this `describe` block to `/home/jake/project/help-my-run/app/app/__tests__/run-detail.test.tsx`, AFTER the existing `describe('RunDetailScreen — loading', ...)` block (i.e. after its closing `});` on the last line). It reuses the file's existing `analysis` literal, `mockHookState`, and the `expo-router` + `../../src/api/hooks` mocks already in scope. The `afterEach` already resets `mockHookState.analysis` back to `source: 'strava'`, so no extra cleanup is needed.
  ```tsx
  describe('RunDetailScreen — source badge', () => {
    it('shows "HR via Garmin .FIT" badge when source is garmin', async () => {
      mockHookState.analysis = {
        data: { ...analysis, source: 'garmin' },
        isPending: false, isError: false,
      };
      const { getByTestId } = await render(<RunDetailScreen />);
      const badge = getByTestId('source-badge');
      expect(badge).toBeTruthy();
      expect(badge.props.children).toBe('HR via Garmin .FIT');
    });

    it('does NOT show the source badge when source is strava', async () => {
      mockHookState.analysis = {
        data: { ...analysis, source: 'strava' },
        isPending: false, isError: false,
      };
      const { queryByTestId } = await render(<RunDetailScreen />);
      expect(queryByTestId('source-badge')).toBeNull();
    });

    it('does NOT show the source badge when there is no HR (no zone section)', async () => {
      mockHookState.analysis = {
        data: { ...analysis, source: 'garmin', has_hr: false, time_in_zone: [], decoupling_pct: null },
        isPending: false, isError: false,
      };
      const { queryByTestId } = await render(<RunDetailScreen />);
      expect(queryByTestId('source-badge')).toBeNull();
    });
  });
  ```

- [ ] **Step 3: Run the test — expect FAIL.**
  Cmd: `cd /home/jake/project/help-my-run/app && npm test -- run-detail`
  Expected: the new `source badge` describe FAILS. The first case throws `Unable to find an element with testID: source-badge` (badge not yet rendered); the strava + no-HR cases PASS (no badge exists yet). The pre-existing describes still PASS. Confirm the failure is specifically the missing `source-badge` test (red), not a harness/import error.

- [ ] **Step 4: Implement — add the badge JSX (minimal, TEXTUAL ANCHOR edit).**
  In `/home/jake/project/help-my-run/app/app/run/[id].tsx`, inside `RunDetailScreen`'s return, find the `a && a.has_stream && a.has_hr` block (verbatim current text):
  ```tsx
      {a && a.has_stream && a.has_hr ? (
        <View style={styles.section}>
          <Text style={styles.subheading}>Time in zone</Text>
          {a.time_in_zone.map((z) => (
            <ZoneBar key={z.zone} z={z} />
          ))}
        </View>
      ) : null}
  ```
  Replace it with (badge added above the "Time in zone" subheading, keyed on `a.source === 'garmin'`):
  ```tsx
      {a && a.has_stream && a.has_hr ? (
        <View style={styles.section}>
          {a.source === 'garmin' ? (
            <Text testID="source-badge" style={styles.sourceBadge}>HR via Garmin .FIT</Text>
          ) : null}
          <Text style={styles.subheading}>Time in zone</Text>
          {a.time_in_zone.map((z) => (
            <ZoneBar key={z.zone} z={z} />
          ))}
        </View>
      ) : null}
  ```

- [ ] **Step 5: Implement — add the `sourceBadge` style (TEXTUAL ANCHOR edit).**
  In the same file's `const styles = StyleSheet.create({...})`, find the anchor line (verbatim):
  ```tsx
    section: { gap: 4 },
  ```
  Replace it with (append the new `sourceBadge` style entry right after `section`):
  ```tsx
    section: { gap: 4 },
    sourceBadge: {
      alignSelf: 'flex-start', fontSize: 12, fontWeight: '600', color: '#fc4c02',
      backgroundColor: '#fff0e8', borderRadius: 6, paddingHorizontal: 8, paddingVertical: 3, marginTop: 8,
    },
  ```
  (`#fc4c02` is the Strava-orange already used by `zoneFill`/`button` in this file. No new imports: `View`/`Text`/`StyleSheet` are already imported.)

- [ ] **Step 6: Run the test — expect PASS.**
  Cmd: `cd /home/jake/project/help-my-run/app && npm test -- run-detail`
  Expected: ALL describes PASS, including the three new `source badge` cases (garmin → badge with text `"HR via Garmin .FIT"`; strava → no badge; no-HR → no badge because the whole zone section is gated off). The pre-existing describes remain green.

- [ ] **Step 7: Typecheck (no type change, but verify the pure-render add compiles).**
  Cmd: `cd /home/jake/project/help-my-run/app && npx tsc --noEmit`
  Expected: clean (0 errors). `a.source` is already typed `'strava' | 'garmin' | ''` on `StreamAnalysis`, so `a.source === 'garmin'` is a valid narrow with no `types.ts` change.

- [ ] **Step 8: Commit.**
  Cmd: `cd /home/jake/project/help-my-run && git add app/app/run/\[id\].tsx app/app/__tests__/run-detail.test.tsx && git commit -m "feat(app): surface Garmin .FIT source badge on run detail (M3.2.1)"`
  Stage ONLY the two app files (no `types.ts` — unchanged).

---

## Definition of Done

Each M3.2.1 success criterion (spec §3) maps to the task(s) below; all must be green plus the manual no-HR-run check.

- **Criterion 1 — worker ingests the recent-window Garmin activities list (id, start_time, duration, distance, type) into a new `garmin_activities` table during sync.**
  - Table + index: Task 1 (`TestOpenAndMigrate`, migration `00007`).
  - Store upsert: Task 2 (`TestUpsertGarminActivity`).
  - Worker normalize + emit: Tasks 4 (`normalize_garmin_activity`), 5 (`build_output(activities=)`), 6 (`run_fetch` activities path + run-type filter + degrade-to-`[]`), 7 (`--dry-run` activities).
  - Go wire + ingest: Tasks 8 (`WorkerOutput.Activities`/`GarminActivity`), 9 (`SyncGarmin` upsert loop, `TestSyncGarminUpsertsAllTables` asserts the 2 `garmin_activities` rows).

- **Criterion 2 — `resolveGarminID(stravaActivity)` returns a confident Garmin id via start-time match (±~120 s) tie-broken by closest duration/distance; no confident match → `(0,false)`.**
  - Nearest-match query: Task 3 (`TestFindGarminActivitiesNear` — within/outside tolerance, ordering, run-type filter, tie pair, no-match → empty).
  - Activation + tie-break logic + `GetActivity`: Task 11 (`TestResolveGarminID` — match within tolerance, tie-break by duration then distance, non-run excluded, unknown activity → false).
  - Config knob (±120s default): Task 10 (`TestLoadGarminMatchToleranceDefault`/`Override`).

- **Criterion 3 — a no-HR Strava run now fetches HR via the Garmin `.FIT` fallback, is stored with `source = garmin`, and its time-in-zone + decoupling compute.**
  - End-to-end activation: Task 12 (`TestFetchAndAnalyzeGarminFallbackActivates` — no-HR Strava stream + seeded `garmin_activities` + stub FIT runner → `Source=="garmin"`, `HasHR`, non-empty `TimeInZone`, non-nil `DecouplingPct`, stored `activity_streams.source=="garmin"` with the .FIT HR series).

- **Criterion 4 — the run-detail screen shows the analysis source ("HR via Garmin .FIT") when the fallback supplied it.**
  - App badge: Task 13 (`run-detail.test.tsx` — badge present + correct text when `source: 'garmin'`; absent when `source: 'strava'`; absent when no HR).

- **Criterion 5 — graceful degradation throughout (list fetch failure, no match, or FIT parse failure → existing no-HR state; never a fabricated analysis).**
  - List-fetch failure → `activities: []`: Task 6 (`test_run_fetch_activities_failure_degrades_to_empty`).
  - No match → `(0,false)` (degrade to no-HR): Task 3 (no-match → empty) + Task 11 (`no candidate within tolerance` / `non-run` / `unknown strava activity` → false). The `FetchAndAnalyze` no-HR path on `ok==false` is unchanged from M3.2 (covered by existing engine tests).

- **Manual check (spec §9 "Manual" + criterion 3):** with the server running against live data, a real run that has no Strava HR stream is fetched (`POST /api/activities/{id}/stream/fetch`), gets HR via the Garmin fallback, is stored with `source = garmin`, its time-in-zone + decoupling populate, and the run-detail screen shows the "HR via Garmin .FIT" badge. Confirm `LIKE '%running%'` produced no false-positive non-run match and that live `startTimeGMT` parses correctly under `strftime('%s', ...)` (flagged unverified items 1 & 2).
