# Milestone 4 (Garmin-Only — Drop Strava) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run the entire help-my-run app on free Garmin data by removing the Strava API dependency: repopulate the canonical `activities` table from the Garmin worker, serve per-second streams from the `.FIT` worker only, and delete the entire Strava package/OAuth/Connect-Strava flow.

**Architecture:** M4 pivots the merged M0–M3.3 codebase to Garmin-only. Activities are repopulated from the Garmin worker (enriched `normalize_garmin_activity` → `GarminActivity` → `store.UpsertActivity`); streams are `.FIT`-only with `resolveGarminID` as identity (`activities.activity_id` IS the Garmin download id); the `strava` package, OAuth endpoints, Connect-Strava screen, and `STRAVA_*` env vars are removed; migration `00009` re-keys `activities.strava_id → activity_id` and drops `strava_tokens`/`oauth_states`/`garmin_activities`. The downstream engines (metrics/progress/coach/chat) are unchanged — they read the canonical `activities` table.

**Tech Stack:** Go (chi router, modernc.org/sqlite driver, goose embedded migrations) backend; Python Garmin worker (`python-garminconnect` + `garmin-fit-sdk`); Claude CLI (subscription path, `ANTHROPIC_API_KEY` unset); Expo / React Native app (TypeScript, React Query).

---

## Setup Prerequisites

- **Builds on M3.3 `main`.** All of M0 + M1 + M2 + M3.1 + M3.2 + M3.2.1 + M3.3 are merged to `main` at `/home/jake/project/help-my-run`. Start from a clean tree on a fresh M4 branch.
- **`make garmin-login` is required** before a real sync. M4 makes Garmin the ONLY data path: the one-time, MFA-aware Garmin login (tokens stored in `GARMIN_TOKENSTORE`) must succeed so the worker can authenticate. A failed/expired login surfaces the worker's "re-run garmin login" message.
- **No Strava anymore.** There is no Strava API app, no `STRAVA_*` env vars, no OAuth flow. Do not configure any Strava credentials.
- **`ANTHROPIC_API_KEY` must stay UNSET.** This is the subscription path — the Claude CLI uses the logged-in subscription, not a metered API key. Leaving the key set would switch to paid API billing. Keep it unset in `.env`.
- **Empty DB / no data migration.** Sync never succeeded under placeholder Strava creds, so the DB has no real activity data; the `activities` re-key (migration `00009`) is clean — no data migration step.

---

## File Structure

> **Locating edit sites: use textual anchors.** Every edit below is specified by a textual anchor (a function name, struct name, comment, or unique code fragment) — NOT by literal line numbers (line numbers drift as edits land). Find the anchor, then apply the change.

> **Build stays green after every task.** Tasks are ordered so `go build ./...` compiles after each commit. The re-key (struct field + column) lands with ALL consumers in one commit; Garmin activity ingest is wired in before `SyncStrava`/the `strava` package are deleted; streams rewire before strava streams are removed; config/app removal is last. CORE tasks gate on per-package `go test` + whole-module `go build`; whole-module `go test ./...` is fully green only after STRIP Task 3 (api+main rewire) and STRIP Task 4 (package delete).

### ADDED
- `backend/internal/store/migrations/00009_m4_garmin_only.sql` — re-key `activities` (`strava_id → activity_id`); drop `strava_tokens`/`oauth_states`/`garmin_activities`.
- `garmin-worker/tests/fixtures/activity_list_element.json` — enriched 15-field Garmin list-element fixture.
- `backend/internal/store/migration_00009_test.go` — 00009 drop + round-trip test (only if not already added by the migration task; see Task 1/Task 12).

### DELETED
- `backend/internal/strava/` (whole dir: `client.go`, `types.go`, `client_test.go`, `testdata/strava_activities.json`, `testdata/strava_laps.json`, `testdata/strava_streams.json`, `testdata/strava_token.json`, `testdata/.gitkeep`) — Strava removal.
- `backend/internal/streams/strava.go` — `FromStravaStreams`/`minLen`/`clip` gone (FIT-only).
- `backend/internal/streams/strava_test.go` — tests the deleted `FromStravaStreams`.
- `backend/internal/store/oauth_state.go` — `SaveOAuthState`/`ConsumeOAuthState` (OAuth-state CSRF moot).

### MODIFIED — backend Go
- `backend/internal/store/activities.go` — `Activity.StravaID→ActivityID`; re-key `UpsertActivity`/`GetActivity`/`ListActivities` SQL+scan; `Split` doc tweak; `LatestActivityStartTime` doc tweak (kept through CORE; deleted in STRIP Task 2 with its caller).
- `backend/internal/store/streams.go` — `ListStreamAnalyses` + `ListRecentRunsWithoutStream` `a.strava_id → a.activity_id`.
- `backend/internal/store/store_test.go` — `wantTables` drop `strava_tokens`/`garmin_activities`; add `TestMigration00009RekeysAndDrops`; re-key `StravaID:`→`ActivityID:`; delete strava-token + garmin_activities tests.
- `backend/internal/store/activities_test.go` — re-key `StravaID:`→`ActivityID:`.
- `backend/internal/store/garmin.go` — delete `GarminActivityRow`/`UpsertGarminActivity`/`GarminActivityCandidate`/`FindGarminActivitiesNear` (STRIP Task 4).
- `backend/internal/store/tokens.go` — delete `StravaTokens`/`GetStravaTokens`/`SaveStravaTokens`; KEEP `ErrNotFound`; trim imports (STRIP Task 4).
- `backend/internal/sync/sync.go` — rewire `SyncGarmin` activities loop → `UpsertActivity` + `f64ptrToI64` (CORE); delete `SyncStrava`/`mapActivity`/`mapLaps`/`strPtr`/`refreshBuffer`; `AllResult` Garmin-only; `SyncAll` drops `client`; remove strava import; package doc (STRIP Task 2).
- `backend/internal/sync/streams_sync.go` — remove strava import + `*strava.ErrRateLimited` branch + rate-limit guard; remove `errors` import (STRIP Task 2).
- `backend/internal/agent/syncer.go` — remove strava field/import; `NewRealSyncer` drops `sc`; `SyncAll` drops `r.strava` (STRIP Task 2).
- `backend/internal/streams/engine.go` — drop `strava`+`matchToleranceS` fields; `New` slimmed; `FetchAndAnalyze` FIT-only; delete `accessToken`/`refreshBuffer`/`absF`; `resolveGarminID`→identity; remove strava import (STRIP Task 1).
- `backend/internal/garmin/types.go` — enrich `GarminActivity` (15 fields); comment on `FITStreamOutput.ActivityID`.
- `backend/internal/garmin/runner.go` — comment update on `RunGarminFetchFIT` echo id.
- `backend/internal/api/router.go` — remove strava import + `Deps.Strava`; remove 2 strava routes; reshape `SyncFunc` to 3-tuple (STRIP Task 3).
- `backend/internal/api/handlers.go` — `status()` Garmin-only; delete `stravaConnect`/`randomState`/`stravaCallback`/`writeHTML`; `sync()` Garmin-only; `activities()` re-key; trim imports (STRIP Task 3; `activities()` re-key in CORE Task 2).
- `backend/internal/api/dto.go` — delete `stravaStatus`/`connectResp`; `statusResp`/`syncResp` Garmin-only; `activityDTO` re-key (re-key in CORE Task 2; deletions in STRIP Task 3).
- `backend/internal/api/stream_handlers.go` — remove strava import + 429 branch in `fetchStream` (STRIP Task 3).
- `backend/internal/progress/engine.go` — `streamPoints`: `a.StravaID→a.ActivityID` (CORE Task 2).
- `backend/internal/config/config.go` — remove 3 `STRAVA_*` fields + `GarminMatchToleranceS` (STRIP/CONFIG Task).
- `backend/cmd/server/main.go` — remove strava import/client/`App.Strava`; rewire `Wire`+`main` to Garmin-only signatures; log line (STRIP Task 3).

### MODIFIED — worker (Python)
- `garmin-worker/garmin_worker/normalize.py` — enrich `normalize_garmin_activity` (15 keys) + `_gmt_to_rfc3339` RFC3339 start_time.
- `garmin-worker/garmin_worker/cli.py` — extend `_DRY_ACTIVITIES_RAW` with new keys.
- `garmin-worker/tests/test_normalize_activity.py` — widen key-set assertions; enriched + null cases.
- `garmin-worker/tests/test_fetch_cli.py` — `test_dry_run_fetch_includes_activities` 15-key set.
- `garmin-worker/tests/fixtures/dry_run_expected.json` — enrich `activities[]` to the 15-key shape.

### MODIFIED — app (TS/TSX)
- `app/src/api/types.ts` — `Status` drop strava; delete `ConnectResponse`; `SyncResponse` Garmin-only; `Activity.strava_id→activity_id`; `StreamAnalysis.source` drop `'strava'`.
- `app/src/api/hooks.ts` — delete `useConnectStrava`; drop `ConnectResponse` + `WebBrowser` imports.
- `app/app/settings.tsx` — remove Strava section + `useConnectStrava`/`stravaConnected`; `sync-result`→Garmin-only.
- `app/app/index.tsx` — remove strava status; re-key `strava_id→activity_id` (keyExtractor/Link/testID).

### MODIFIED — docs/config
- `.env.example` — remove `STRAVA_*` block + `GARMIN_MATCH_TOLERANCE_S`.
- `README.md` — Garmin-only rewrite (lines anchored 3, 13, 28, 84, 100; keep ANTHROPIC-unset + absolute-path notes).

### MODIFIED — tests (rewrite/re-key)
- Backend: `api/{handlers_test.go, stream_handlers_test.go, image_security_test.go, chat_handlers_test.go, progress_handlers_test.go, m2_handlers_test.go}`, `sync/{sync_test.go, streams_sync_test.go}`, `streams/engine_test.go`, `agent/syncer_test.go`, `coach/coach_test.go`, `chat/pack_test.go`, `metrics/metrics_test.go`, `config/config_test.go`, store activity/migration tests.
- Worker: `tests/test_normalize_activity.py`, `test_fetch_cli.py` (+ `test_cli.py`/`test_fetcher.py` if they assert the 6-key set).
- App: `app/app/__tests__/{settings,index,run-detail}.test.tsx`, `app/src/api/__tests__/{types,client,hooks,hooks-streams}.test.ts(x)`.

---

## Shared Contracts

### Contract A — Migration `00009` (DDL verbatim)

New file: `backend/internal/store/migrations/00009_m4_garmin_only.sql`. Migrations are embedded goose (`store.go:2`); driver is **modernc.org/sqlite** (`store.go:8`), which supports `ALTER TABLE ... RENAME COLUMN` (SQLite ≥ 3.25.0). `activities` is referenced by THREE FK tables — `activity_splits` (`00001_init.sql:46`), `activity_streams` and `stream_analyses` (`00006_m3_2_streams.sql:8,22`) — all with `REFERENCES activities (strava_id)`; SQLite auto-rewrites ALL of them to `activities (activity_id)` on `RENAME COLUMN`; FKs are ON (`store.go:22`). All three rewrites are exercised by Task 1's `TestMigration00009RekeysAndDrops`. The seeded `sync_log` row `('strava', …)` (`00001_init.sql:91`) stays harmless (no code reads it after M4); leave it.

```sql
-- +goose Up
-- +goose StatementBegin
ALTER TABLE activities RENAME COLUMN strava_id TO activity_id;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS strava_tokens;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS oauth_states;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS garmin_activities;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE activities RENAME COLUMN activity_id TO strava_id;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE strava_tokens (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    access_token  TEXT    NOT NULL,
    refresh_token TEXT    NOT NULL,
    expires_at    INTEGER NOT NULL,
    scope         TEXT,
    athlete_id    INTEGER,
    updated_at    TEXT    NOT NULL
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TABLE oauth_states (
    state      TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);
-- +goose StatementEnd
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
```

Down DDL is verbatim from `00001_init.sql:4-12` (strava_tokens), `00003_oauth_state.sql:3-6` (oauth_states), `00007_m3_2_1_garmin_activities.sql:3-14` (garmin_activities + index).

### Contract B — Re-key (`strava_id → activity_id`; `StravaID → ActivityID`)

Updated Go `Activity` struct — `backend/internal/store/activities.go` (anchor doc `// Activity is a normalized Strava run`):

```go
// Activity is a normalized Garmin run (one row in activities).
type Activity struct {
	ActivityID     int64 // = Garmin activityId
	Name           string
	Type           string
	SportType      *string // always nil from Garmin (list has no sportType)
	StartTime      string
	StartTimeLocal *string
	DistanceM      float64
	MovingTimeS    int64
	ElapsedTimeS   int64
	AvgHR          *float64
	MaxHR          *float64
	AvgSpeed       *float64
	MaxSpeed       *float64
	AvgCadence     *float64
	ElevationGainM *float64
	RawJSON        string
}
```

Store fns that change (SQL column names + scan targets only; signatures keep param rename):
- `UpsertActivity(a Activity) error` — INSERT/`ON CONFLICT`/bind `strava_id`→`activity_id`, `a.StravaID`→`a.ActivityID`.
- `GetActivity(activityID int64)` — param `stravaID→activityID`; SELECT/`WHERE`/bind/scan re-key.
- `ListActivities(limit int)` — SELECT + inner `Scan(&a.StravaID)`→`&a.ActivityID`. `ORDER BY start_time DESC` unchanged.
- `LatestActivityStartTime()` — doc only `Strava incremental sync cursor.`→`Garmin incremental cursor.`; **KEPT through CORE** (caller `sync.go:87` in `SyncStrava` still present); deleted in STRIP Task 2 with its caller.
- `Split`/`UpsertSplits(activityID int64, ...)` — keyed on `activity_splits(activity_id, idx)` (already correct); no SQL change. `Split` doc `// Split is one Strava lap`→`// Split is one lap`. Kept (dead-but-valid after Garmin path drops laps).

`backend/internal/store/streams.go`:
- `ListStreamAnalyses` (anchor `JOIN activities a ON a.strava_id = sa.activity_id`) → `ON a.activity_id = sa.activity_id`.
- `ListRecentRunsWithoutStream` (anchors `SELECT a.strava_id`, `LEFT JOIN activity_streams st ON st.activity_id = a.strava_id`) → `SELECT a.activity_id`, `ON st.activity_id = a.activity_id`.

COMPLETE non-test consumer list (`grep StravaID backend --include=*.go | grep -v _test` = exactly 5 files):

| File:symbol (anchor) | Change |
|---|---|
| `store/activities.go` | struct field + 4 fns above |
| `store/streams.go` `ListStreamAnalyses`, `ListRecentRunsWithoutStream` | SQL `a.strava_id`→`a.activity_id` |
| `api/handlers.go` `activities()` (anchor `StravaID: a.StravaID`) | → `ActivityID: a.ActivityID` |
| `api/dto.go` `activityDTO` (anchor `StravaID int64 \`json:"strava_id"\``) | → `ActivityID int64 \`json:"activity_id"\`` |
| `progress/engine.go` `streamPoints()` (anchor `startByID[a.StravaID] = a.StartTime`) | → `startByID[a.ActivityID] = a.StartTime` |

> `sync/sync.go` `mapActivity` (anchor `StravaID: a.ID`) is DELETED with the mapper (STRIP Task 2), not re-keyed.
> `coach/coach.go` `Fitness()` + `chat/pack.go` `BuildPack` call `ListActivities(...)` but never read `.StravaID` (grep hits are test-only). `metrics` reads only `Type/StartTime/DistanceM`. No engine-logic change.

Test files setting `StravaID:` to re-key to `ActivityID:`: `coach/coach_test.go`, `chat/pack_test.go`, `metrics/metrics_test.go`, plus store/api/progress `_test.go` touching the field. (`chat/engine_test.go` `.ID` hits are `ChatMessage.ID` — leave.)

### Contract C — Garmin activity ingestion

**No `get_activity(id)` detail call** — `garminconnect` v0.3.6 `typed.Activity` (`typed.py:396-453`) declares every needed field on the list element. The existing `client.get_activities_by_date(since, until, "running")` (`fetcher.py:77`) is sufficient. All enriched fields are `Optional`/`default=None` → nullable everywhere.

> **`sport_type` has no Garmin source** — list has `activityType.typeKey` (→ `type`) but no separate `sportType`. `Activity.SportType`/`activities.sport_type`/DTO `sport_type` are always `null`. Keep the column/field/key; worker emits no `sport_type`.

> **`start_time` → RFC3339 (Decision #1, RESOLVED).** Worker normalizes Garmin `startTimeGMT` `"YYYY-MM-DD HH:MM:SS"` → `"YYYY-MM-DDTHH:MM:SSZ"` (replace the single space with `T`, append `Z`) so the Go engines' `time.Parse(time.RFC3339, …)` (pace-at-HR/progress bucketing) succeeds. `start_time_local` keeps Garmin's space format (local wall-clock, no offset). Null `startTimeGMT` → `None`.

Worker enriched normalizer — `garmin-worker/garmin_worker/normalize.py` `normalize_garmin_activity(el)` (15 keys): `garmin_activity_id`, `name`, `start_time` (RFC3339), `start_time_local`, `activity_type` (`activityType.typeKey`), `distance_m`, `moving_time_s` (`movingDuration`), `elapsed_time_s` (`elapsedDuration` else `duration`), `avg_hr` (`averageHR`), `max_hr` (`maxHR`), `avg_speed` (`averageSpeed`), `max_speed` (`maxSpeed`), `avg_cadence` (`averageRunningCadenceInStepsPerMinute`), `elevation_gain_m` (`elevationGain`), `raw_json` (original element). `fetcher.py:74-84` `run_fetch` needs no structural change. `cli.py` `_DRY_ACTIVITIES_RAW` gains the new keys.

Go `WorkerOutput.Activities` → `store.UpsertActivity` — `backend/internal/garmin/types.go` `GarminActivity` (enriched, 15 fields; `DurationS` removed; `MovingTimeS`/`ElapsedTimeS` are `*float64`):

```go
type GarminActivity struct {
	GarminActivityID int64           `json:"garmin_activity_id"`
	Name             string          `json:"name"`
	StartTime        string          `json:"start_time"`
	StartTimeLocal   *string         `json:"start_time_local"`
	ActivityType     *string         `json:"activity_type"`
	DistanceM        *float64        `json:"distance_m"`
	MovingTimeS      *float64        `json:"moving_time_s"`
	ElapsedTimeS     *float64        `json:"elapsed_time_s"`
	AvgHR            *float64        `json:"avg_hr"`
	MaxHR            *float64        `json:"max_hr"`
	AvgSpeed         *float64        `json:"avg_speed"`
	MaxSpeed         *float64        `json:"max_speed"`
	AvgCadence       *float64        `json:"avg_cadence"`
	ElevationGainM   *float64        `json:"elevation_gain_m"`
	RawJSON          json.RawMessage `json:"raw_json"`
}
```

`backend/internal/sync/sync.go` `SyncGarmin` activities loop → `store.UpsertActivity` (deref/round durations to int64 via `f64ptrToI64(p *float64) int64` = nil→0 else `int64(math.Round(*p))`). `Type` defaults `""` (NOT NULL column); `Name` is Go `string` (JSON `null` → `""`). No `UpsertSplits` call (Garmin list has no laps).

### Contract D — Streams (FIT-only, identity resolve)

`backend/internal/streams/engine.go`:
- `Engine` struct — delete `strava *strava.Client` + `matchToleranceS int`; keep `store`, `runner`, `extraEnv`.
- `New(s, runner, extraEnv)` — slimmed (was `New(s, sc, runner, extraEnv, matchToleranceS)`).
- `FetchAndAnalyze` — FIT sole source: `gid, _ := e.resolveGarminID(activityID)`; `out, err := e.runner.RunGarminFetchFIT(ctx, gid, activityID, e.extraEnv)`; `ser := Series{...out.Series...}`; `source := "garmin"`; then unchanged compress + `UpsertActivityStream` + `GetOrComputeAnalysis`. Remove `accessToken` call, `e.strava.GetActivityStreams`, `FromStravaStreams`, the `!ser.HasHR()` fallback, the `*strava.ErrRateLimited` doc mention.
- DELETE `accessToken`, engine `const refreshBuffer`, `absF`.
- `resolveGarminID` → identity: `return activityID, true`.
- DELETE file `backend/internal/streams/strava.go` (`FromStravaStreams`/`minLen`/`clip`).
- Imports: remove `help-my-run/backend/internal/strava`; keep `garmin`, `store`, `encoding/json`, `errors`, `time`, `context`.

`backend/internal/sync/streams_sync.go`: remove `strava` import + `errors` (its only use was `errors.As` for the rate-limit branch); `TrickleStreams` keeps `const source = "strava"` (Decision #2 — opaque `stream_fetch_log` key); remove the `*strava.ErrRateLimited` branch + the top `rate_limited_until` guard; on any fetch error record `Status: "error"` and stop.

`backend/internal/store/streams.go` `stream_fetch_log.source` (Decision #2, RESOLVED): keep all three sites (`GetStreamFetchLog` `WHERE source='strava'`, `UpdateStreamFetchLog` default, `streams_sync.go const source`) as the opaque `'strava'` internal key — NO migration, no re-key. This is NOT an `internal/strava` import, so the package grep gate is unaffected.

`backend/internal/garmin/runner.go` `RunGarminFetchFIT` doc `echoActivityID is the Strava id` → `echoActivityID is the activity id (identity: equals garminActivityID)`. `types.go` `FITStreamOutput.ActivityID` comment `echoed Strava id (store PK)` → `echoed activity id (store PK)`.

`backend/internal/api/stream_handlers.go`: remove `strava` import; in `fetchStream` remove the 429 `errors.As(err, &rl)` block (on error: `writeJSON(w, http.StatusInternalServerError, …)`); doc `// 429 on Strava rate limit; 500 otherwise.`→`// 500 on fetch error.`; keep `errors` (used by `errors.Is(err, store.ErrNotFound)` in `activityAnalysis`).

### Contract E — Removal

Files DELETED (entire): `backend/internal/strava/` (dir), `backend/internal/streams/strava.go`, `backend/internal/streams/strava_test.go`, `backend/internal/store/oauth_state.go`.

Store symbols removed:
- `store/tokens.go` — delete `StravaTokens`/`GetStravaTokens`/`SaveStravaTokens`; KEEP `ErrNotFound` (trim file to just `ErrNotFound` + `import "errors"`).
- `store/garmin.go` — delete `GarminActivityRow`/`UpsertGarminActivity`/`GarminActivityCandidate`/`FindGarminActivitiesNear`; keep all sleep/hrv/bb/rhr/vo2max/recovery fns.
- `store/activities.go` — delete `LatestActivityStartTime` (with its caller, STRIP Task 2).

Sync symbols removed (`sync/sync.go`): `SyncStrava`, `mapActivity`, `mapLaps`, `strPtr`, `const refreshBuffer`; remove `strava` import (keep `encoding/json`, `time`); `AllResult` → `{ Garmin SourceResult }`; `SyncAll(ctx, s, r, extraEnv, st)` drops `client *strava.Client`; package doc → Garmin-only.

Agent (`agent/syncer.go`): remove `strava` import + `RealSyncer.strava`; `NewRealSyncer(s, r, extraEnv)` drops `sc`; `SyncAll` → `syncpkg.SyncAll(ctx, r.store, r.runner, r.extraEnv, nil)`.

API (`api/router.go`): remove `strava` import + `Deps.Strava`; remove routes `GET /api/strava/callback` + `GET /api/strava/connect` (404 automatically); `SyncFunc` → `func(ctx context.Context) (string, int, *string)`.
(`api/handlers.go`): `status()` Garmin-only — `connected` = "last worker invocation authenticated successfully" (spec §3.4/§7), derived from the garmin `sync_log` status: `garminConn := garminLog.Status == "ok"` (the row `status()` already loads via `GetSyncLog("garmin")`). NOT `recoveryDays > 0` (recovery-data presence is not authentication). `recoveryDays` is still reported in `Counts`. Delete `stravaConnect`/`randomState`/`stravaCallback`/`writeHTML`; `sync()` → `syncResp{Garmin: ...}`; trim `crypto/rand`/`encoding/hex`/`store` imports.
(`api/dto.go`): delete `stravaStatus`/`connectResp`; `statusResp`/`syncResp` Garmin-only.

Final `/api/status` Garmin-only DTO:
```json
{ "garmin": { "connected": true, "last_synced_at": "2026-06-23T05:00:00Z", "last_run_at": "2026-06-23T05:00:00Z", "status": "ok", "error": null }, "counts": { "activities": 12, "recovery_days": 84 } }
```
`/api/sync` body: `{ "garmin": { "status": "ok", "synced": 12, "error": null } }`.

main.go (`backend/cmd/server/main.go`): remove `strava` import + `App.Strava`; delete `stravaClient := strava.New(...)`; `streams.New(s, runner, extraEnv)`; `syncFunc` → `res := syncpkg.SyncAll(ctx, s, runner, extraEnv, streamTrickle(...)); return res.Garmin.Status, res.Garmin.Synced, res.Garmin.Error`; `agent.NewRealSyncer(s, runner, extraEnv)`; remove `Strava:` from `api.Deps`+`App`; `main()` drops `stravaClient`; log line `log.Printf("sync: garmin=%s/%d", res.Garmin.Status, res.Garmin.Synced)`.

App (`app/`): `types.ts` (`Status` drop strava; delete `ConnectResponse`; `SyncResponse` Garmin-only; `Activity.strava_id→activity_id`; `StreamAnalysis.source` `'garmin' | ''`); `hooks.ts` (delete `useConnectStrava`; drop `ConnectResponse`+`expo-web-browser` imports); `settings.tsx` (remove Strava section + `useConnectStrava`/`stravaConnected`; `sync-result` Garmin-only); `index.tsx` (remove strava status; re-key `strava_id→activity_id` in keyExtractor/Link/testID). `run/[id].tsx` + `src/api/client.ts`: no change.

### Contract F — Config / .env / README

`backend/internal/config/config.go`: REMOVE `StravaClientID`/`StravaClientSecret`/`StravaRedirectURL` (3 `required:"true"`) + `GarminMatchToleranceS` (anchor `GarminMatchToleranceS int \`envconfig:"GARMIN_MATCH_TOLERANCE_S" default:"120"\``). KEEP `StreamRecentWeeks`, `StreamFetchBudget`, `APIToken` (now the only `required:"true"`), all Garmin/Python/Claude/agent vars.

`.env.example`: remove the `# --- Strava OAuth (required) ---` block (`STRAVA_CLIENT_ID`/`STRAVA_CLIENT_SECRET`/`STRAVA_REDIRECT_URL`) + the `GARMIN_MATCH_TOLERANCE_S=120` line + comment. Keep ANTHROPIC-unset + ABSOLUTE-path notes. Final keys: `API_TOKEN`, `DB_PATH`, `PORT`, `GARMIN_EMAIL`, `GARMIN_PASSWORD`, `GARMIN_TOKENSTORE`, `PYTHON_BIN`, `WORKER_SCRIPT`, `CLAUDE_BIN`, `CLAUDE_MODEL`, `IMAGE_DIR`, `ANTHROPIC_API_KEY` (unset), `AGENT_ENABLED`, `AGENT_RUN_TIME`, `AGENT_TZ`, `AGENT_TICK_INTERVAL`, `EXPO_PUSH_BASE_URL`, `STREAM_RECENT_WEEKS`, `STREAM_FETCH_BUDGET`, `CHAT_HISTORY_TURNS`. Only `API_TOKEN` is required.

`README.md` — Garmin-only rewrite (keep ANTHROPIC-unset + absolute-path notes): line 3 (drop "from Strava" / "connect Strava"), line 13 (delete Strava-app prerequisite bullet), line 28 (`# edit .env and fill in API_TOKEN, GARMIN_EMAIL, GARMIN_PASSWORD, …` absolute paths), line 84 ("…then tap Sync now."), line 100 (drop "Strava secret").

### Two flagged plan-time decisions — RESOLVED
1. **`start_time` format** → **RFC3339** (worker replaces space→`T`, appends `Z`); Go stores verbatim. Propagates to both fixtures (`dry_run_expected.json`, `worker_output.json`).
2. **`stream_fetch_log.source` key** → **keep opaque `'strava'`** (no migration; 3 sites already agree). Not an `internal/strava` import → package grep gate satisfied.

### Build-hygiene gates (spec §8)
`grep -rn "internal/strava" backend` = 0; `grep -rni strava app/src app/app` (excl node_modules) = 0; `go build ./...`, `go test ./...`, `gofmt -l` empty, worker `pytest`, app `jest` + `tsc` all green; removed routes `/api/strava/connect` + `/api/strava/callback` → 404; manual `make garmin-login` → sync → real runs land in `activities` → Progress/streams/plan/chat work.

---

## Tasks

> **Global build-safe ordering.** CORE Tasks 1–4 land the migration + re-key (with ALL consumers) and Garmin activity ingest WITHOUT deleting any strava package/symbol — `go build ./...` stays green because every strava/garmin_activities Go symbol still exists (only the dropped *tables* and *table-hitting tests* change). STRIP Tasks 5–8 then rewire streams, delete `SyncStrava`/strava follow-ups, rewire api+main, remove config, and finally delete the `strava` package + dead store fns/tables. CONFIG/DOCS/APP Tasks 9–13 close out config, docs, and the Expo app.
>
> **Tasks 1 + 2 are ONE commit** (migration column rename + Go field re-key are inseparable). Whole-module `go build ./...` first compiles green at the END of CORE Task 2 and stays green thereafter. Whole-module `go test ./...` is fully green only after STRIP Task 8.

---

### Task 1: Migration 00009 — re-key activities + drop Strava tables (migration SQL + migration test)

> **NO STANDALONE COMMIT.** Adds the migration file + a failing migration test + updates `wantTables`. Build/tests stay RED until Task 2 supplies the Go re-key. Commit happens at the end of Task 2.

**Files:**
- Create: `backend/internal/store/migrations/00009_m4_garmin_only.sql`
- Modify: `backend/internal/store/store_test.go` — `TestOpenAndMigrate` (`wantTables`); ADD `TestMigration00009RekeysAndDrops`
- Test: `go test ./internal/store/`

- [ ] **Step 1: Write the migration file.** Create `backend/internal/store/migrations/00009_m4_garmin_only.sql` verbatim from **Contract A**.
> Down DDL copied verbatim from `00001_init.sql:4-12`, `00003_oauth_state.sql:3-6`, `00007_m3_2_1_garmin_activities.sql:3-14`. The `activity_splits` FK `REFERENCES activities (strava_id)` (`00001_init.sql:46`) is auto-rewritten by SQLite on `RENAME COLUMN` — verified in Step 2 round-trip.

- [ ] **Step 2: Failing test — update `wantTables` + add the 00009 migration test.** In `backend/internal/store/store_test.go`, edit `TestOpenAndMigrate`'s `wantTables` to REMOVE `"strava_tokens"` and `"garmin_activities"`:
```go
	wantTables := []string{
		"activities", "activity_splits",
		"garmin_sleep", "garmin_hrv", "garmin_body_battery", "garmin_rhr",
		"garmin_vo2max",
		"sync_log",
		"activity_streams", "stream_analyses", "stream_fetch_log",
		"chat_messages",
	}
```
  Then ADD this test (append after `TestMigrateIdempotent`):
```go
func TestMigration00009RekeysAndDrops(t *testing.T) {
	s := newTestStore(t)

	// activities is re-keyed: activity_id exists, strava_id does not.
	var col string
	err := s.DB.QueryRow(
		`SELECT name FROM pragma_table_info('activities') WHERE name='activity_id'`,
	).Scan(&col)
	if err != nil {
		t.Fatalf("activities.activity_id missing after 00009: %v", err)
	}
	if err := s.DB.QueryRow(
		`SELECT name FROM pragma_table_info('activities') WHERE name='strava_id'`,
	).Scan(&col); err == nil {
		t.Errorf("activities.strava_id still present after 00009, want renamed away")
	}

	// Dropped tables are gone.
	for _, tbl := range []string{"strava_tokens", "oauth_states", "garmin_activities"} {
		var name string
		err := s.DB.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err == nil {
			t.Errorf("table %q still present after 00009, want dropped", tbl)
		}
	}

	// FK to activities still resolves through the renamed PK column: inserting a
	// split for an existing activity succeeds; an orphan split is rejected.
	if err := s.UpsertActivity(Activity{
		ActivityID: 4242, Name: "fk", Type: "Run", StartTime: "2026-06-22T05:00:00Z",
		DistanceM: 1000, MovingTimeS: 300, ElapsedTimeS: 300, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("UpsertActivity for FK check: %v", err)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO activity_splits (activity_id, idx, distance_m, elapsed_time_s) VALUES (4242,1,1000,300)`,
	); err != nil {
		t.Errorf("split insert for existing activity failed (FK rewrite broken?): %v", err)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO activity_splits (activity_id, idx, distance_m, elapsed_time_s) VALUES (9999,1,1000,300)`,
	); err == nil {
		t.Errorf("orphan split insert succeeded, want FK violation (FK rewrite broken)")
	}

	// activities is referenced by THREE FK tables — also assert the rewrite for
	// activity_streams (00006) and stream_analyses (00006), not just
	// activity_splits (00001). Each FK clause REFERENCES activities(strava_id)
	// (00006_m3_2_streams.sql) must be auto-rewritten to activities(activity_id)
	// on RENAME COLUMN: a child for the existing activity inserts; an orphan rejects.
	if _, err := s.DB.Exec(
		`INSERT INTO activity_streams (activity_id, source, series_gz, fetched_at)
		 VALUES (4242,'garmin',x'00','2026-06-22T05:00:00Z')`,
	); err != nil {
		t.Errorf("activity_streams insert for existing activity failed (FK rewrite broken?): %v", err)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO activity_streams (activity_id, source, series_gz, fetched_at)
		 VALUES (9999,'garmin',x'00','2026-06-22T05:00:00Z')`,
	); err == nil {
		t.Errorf("orphan activity_streams insert succeeded, want FK violation (FK rewrite broken)")
	}
	if _, err := s.DB.Exec(
		`INSERT INTO stream_analyses
		   (activity_id, time_in_zone_json, zones_json, has_hr, computed_at)
		 VALUES (4242,'{}','{}',1,'2026-06-22T05:00:00Z')`,
	); err != nil {
		t.Errorf("stream_analyses insert for existing activity failed (FK rewrite broken?): %v", err)
	}
	if _, err := s.DB.Exec(
		`INSERT INTO stream_analyses
		   (activity_id, time_in_zone_json, zones_json, has_hr, computed_at)
		 VALUES (9999,'{}','{}',1,'2026-06-22T05:00:00Z')`,
	); err == nil {
		t.Errorf("orphan stream_analyses insert succeeded, want FK violation (FK rewrite broken)")
	}
}
```

- [ ] **Step 3: Run — expect FAIL (compile error).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ 2>&1 | head -40`. Expected: **compile failure** — the new test references `Activity{ActivityID: ...}` which does not exist yet (`store/activities.go` still has `StravaID`). **Do not commit.** Proceed directly to Task 2.

---

### Task 2: Re-key `Activity.StravaID → ActivityID` + all store fns + every consumer call site (one cohesive change)

> Makes Task 1's migration test + all existing store/api/progress tests compile and pass. **The commit at the end of this task covers BOTH Task 1 and Task 2** (migration + re-key land together). Whole-module `go build ./...` is green at this commit.

**Files:**
- Modify: `backend/internal/store/activities.go` — `Activity` doc+field; `UpsertActivity`; `GetActivity`; `ListActivities`; `Split` doc; `LatestActivityStartTime` doc (KEPT)
- Modify: `backend/internal/store/streams.go` — `ListStreamAnalyses`, `ListRecentRunsWithoutStream`
- Modify: `backend/internal/sync/sync.go` — `mapActivity` `StravaID: a.ID` → `ActivityID: a.ID` (compile-bridge; the mapper is DELETED whole in STRIP Task 6, but it must compile in between to keep `go build ./...` green at the end of this task)
- Modify: `backend/internal/api/dto.go` — `activityDTO.StravaID` field
- Modify: `backend/internal/api/handlers.go` — `activities()`
- Modify: `backend/internal/progress/engine.go` — `streamPoints()`
- Modify: `backend/internal/store/store_test.go` — re-key `StravaID:`; DELETE `TestStravaTokensRoundTrip`; DELETE `TestUpsertGarminActivity`/`TestFindGarminActivitiesNear`/`mustUpsertGA`; re-key `TestUpsertAndListActivities` + `TestUpsertSplits`
- Modify: `backend/internal/store/activities_test.go` — re-key `StravaID:`→`ActivityID:`
- Modify: `backend/internal/store/streams_test.go` — `seedActivity` helper `StravaID:`→`ActivityID:`
- Modify: `backend/internal/sync/sync_test.go` — `.StravaID`→`.ActivityID` (read, ~line 84, inside the Strava-refresh test) AND `StravaID:`→`ActivityID:` (literal, ~line 265). Needed so the `sync` package TEST binary COMPILES — CORE Task 4 gates on `go test ./internal/sync/ -run TestSyncGarmin`, which compiles the whole package's test files. (`sync_test.go` is rewritten WHOLE in STRIP Task 6; this is just the minimal compile-bridge for the intervening CORE Task 4 gate.)
- Modify: `backend/internal/sync/streams_sync_test.go` — `StravaID: id`→`ActivityID: id` (compile-bridge: same `sync` package as `sync_test.go`, so it must compile for Task 4's `-run TestSyncGarmin` gate; rewritten WHOLE in STRIP Task 6)
- Modify: `backend/internal/api/handlers_test.go` — `StravaID:`→`ActivityID:` (2 seed literals) + `.StravaID`→`.ActivityID` (2 reads)
- Modify: `backend/internal/coach/coach_test.go` — `StravaID:`→`ActivityID:`
- Modify: `backend/internal/chat/pack_test.go` — `StravaID:`→`ActivityID:` (2 literals)
- Modify: `backend/internal/metrics/metrics_test.go` — `StravaID:`→`ActivityID:` (all literals)
- Modify: `backend/internal/progress/engine_test.go` — `StravaID:`→`ActivityID:` (4 literals)
- Test: `go test ./internal/store/ ./internal/progress/ ./internal/coach/ ./internal/chat/ ./internal/metrics/` (these six packages PASS). `./internal/api/` is **builds-green only** at this task: its 6 Strava OAuth tests (`TestStatusOK`, `TestConnectURL`, `TestStravaConnectPersistsState`, `TestStravaCallbackExchangesAndPersists`, `TestStravaCallbackAccessDenied`, `TestStravaCallbackRejectsBadState`) still exist and hit the dropped `oauth_states`/`strava_tokens` tables, so they FAIL at runtime; they are DELETED/rewritten by STRIP Task 7 Step 8. Gate `internal/api` on the test binary compiling (`go vet ./internal/api/`), not on `go test` passing.

> **Scope boundary:** Touch ONLY the re-key + the now-broken tests that block compilation. Do NOT delete `SyncStrava`/`strava` package — those are STRIP tasks. `tokens.go`/`StravaTokens` + `garmin.go` `UpsertGarminActivity`/etc. Go symbols stay intact (build-valid; only the dropped tables make them runtime-dead). `go build ./...` is green after this task even though `sync.go` still calls `GetStravaTokens`/`UpsertGarminActivity` — those Go fns still exist.
> **Compile-bridge for `sync.go` `mapActivity`:** `mapActivity` (anchor `StravaID: a.ID,`) still sets the renamed struct field. It is DELETED WHOLE in STRIP Task 6, but until then it must compile against the re-keyed struct — so re-key its single line here (`StravaID: a.ID,` → `ActivityID: a.ID,`). Without this, `go build ./...` FAILS at the end of this task with "unknown field StravaID in struct literal". (Same reasoning that KEPT `LatestActivityStartTime` for build-green ordering.)
> **Test compile-bridge:** Every `_test.go` that constructs `store.Activity{StravaID: …}` or reads `.StravaID` breaks the per-package TEST build after the rename. The whole-repo set (verified by `grep -rn 'StravaID:\|\.StravaID' backend`) is: `store/{store_test,activities_test,streams_test}.go`, `api/handlers_test.go`, `progress/engine_test.go`, `metrics/metrics_test.go`, `coach/coach_test.go`, `chat/pack_test.go`, `sync/sync_test.go` (+ `sync/streams_sync_test.go` and `streams/engine_test.go`, owned by STRIP Tasks 6/5 which rewrite those whole files). Re-key the CORE-owned set in Steps 4–5b so `go test ./...` for those packages compiles; the two STRIP-owned files are rewritten in their tasks.

- [ ] **Step 1: Re-key `store/activities.go`.**

  Doc + struct (anchor `// Activity is a normalized Strava run` and `StravaID       int64`) → per **Contract B** `Activity` struct (with `// = Garmin activityId` and `SportType *string` comment).

  `Split` doc (anchor `// Split is one Strava lap mapped into activity_splits.`):
```go
// Split is one lap mapped into activity_splits.
```

  `UpsertActivity` (anchor `// UpsertActivity inserts or updates an activity by strava_id.` through the bind args):
```go
// UpsertActivity inserts or updates an activity by activity_id.
func (s *Store) UpsertActivity(a Activity) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO activities (
			activity_id, name, type, sport_type, start_time, start_time_local,
			distance_m, moving_time_s, elapsed_time_s,
			avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m,
			raw_json, synced_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(activity_id) DO UPDATE SET
			name=excluded.name, type=excluded.type, sport_type=excluded.sport_type,
			start_time=excluded.start_time, start_time_local=excluded.start_time_local,
			distance_m=excluded.distance_m, moving_time_s=excluded.moving_time_s,
			elapsed_time_s=excluded.elapsed_time_s,
			avg_hr=excluded.avg_hr, max_hr=excluded.max_hr, avg_speed=excluded.avg_speed,
			max_speed=excluded.max_speed, avg_cadence=excluded.avg_cadence,
			elevation_gain_m=excluded.elevation_gain_m,
			raw_json=excluded.raw_json, synced_at=excluded.synced_at`,
		a.ActivityID, a.Name, a.Type, a.SportType, a.StartTime, a.StartTimeLocal,
		a.DistanceM, a.MovingTimeS, a.ElapsedTimeS,
		a.AvgHR, a.MaxHR, a.AvgSpeed, a.MaxSpeed, a.AvgCadence, a.ElevationGainM,
		a.RawJSON, now)
	return err
}
```

  `GetActivity` (anchor `// GetActivity returns one activity by strava_id` through scan):
```go
// GetActivity returns one activity by activity_id, or ErrNotFound. raw_json is not loaded.
func (s *Store) GetActivity(activityID int64) (Activity, error) {
	var a Activity
	err := s.DB.QueryRow(`
		SELECT activity_id, name, type, sport_type, start_time, start_time_local,
		       distance_m, moving_time_s, elapsed_time_s,
		       avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m
		FROM activities
		WHERE activity_id = ?`, activityID).Scan(
		&a.ActivityID, &a.Name, &a.Type, &a.SportType, &a.StartTime, &a.StartTimeLocal,
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

  `ListActivities` SELECT + scan (anchor SELECT `strava_id, name, type` and inner `&a.StravaID`):
```go
	rows, err := s.DB.Query(`
		SELECT activity_id, name, type, sport_type, start_time, start_time_local,
		       distance_m, moving_time_s, elapsed_time_s,
		       avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m
		FROM activities
		ORDER BY start_time DESC
		LIMIT ?`, limit)
```
  scan target:
```go
		if err := rows.Scan(
			&a.ActivityID, &a.Name, &a.Type, &a.SportType, &a.StartTime, &a.StartTimeLocal,
			&a.DistanceM, &a.MovingTimeS, &a.ElapsedTimeS,
			&a.AvgHR, &a.MaxHR, &a.AvgSpeed, &a.MaxSpeed, &a.AvgCadence, &a.ElevationGainM,
		); err != nil {
```

  `LatestActivityStartTime` — **KEEP it** (do NOT delete). Update only its doc comment (anchor `Used as the Strava incremental sync cursor.`) → `Used as the Garmin incremental cursor.`. Its caller `sync.go:87` (`SyncStrava`) is removed in STRIP Task 2; deleting the method now would break `go build`. (Overrides the contract's "delete recommended" to preserve build-green ordering.)

- [ ] **Step 2: Re-key `store/streams.go` SQL.** `ListStreamAnalyses` (anchor `JOIN activities a ON a.strava_id = sa.activity_id`) → `ON a.activity_id = sa.activity_id`. `ListRecentRunsWithoutStream`: `SELECT a.strava_id`→`SELECT a.activity_id`; `LEFT JOIN activity_streams st ON st.activity_id = a.strava_id`→`ON st.activity_id = a.activity_id`.

- [ ] **Step 3: Re-key the 3 non-test consumer sites + the `sync.go` compile-bridge.**
  `api/dto.go` (anchor `StravaID       int64    \`json:"strava_id"\``):
```go
	ActivityID     int64    `json:"activity_id"`
```
  `api/handlers.go activities()` (anchor `StravaID: a.StravaID, Name: a.Name`):
```go
			ActivityID: a.ActivityID, Name: a.Name, Type: a.Type, SportType: a.SportType,
```
  `progress/engine.go streamPoints()` (anchor `startByID[a.StravaID] = a.StartTime`):
```go
		startByID[a.ActivityID] = a.StartTime
```
  `sync/sync.go mapActivity()` — compile-bridge (anchor `StravaID:       a.ID,` inside `mapActivity`'s returned `store.Activity{...}`):
```go
		ActivityID:     a.ID,
```
  This is the ONLY edit to `sync.go` in CORE; `mapActivity`/`SyncStrava`/the strava import are deleted whole in STRIP Task 6. It must compile here so `go build ./...` (Step 6) is green.

- [ ] **Step 4: Fix the now-broken store tests (compilation gate).** In `store/store_test.go`:
  - `TestUpsertAndListActivities`: replace every `StravaID:` with `ActivityID:` and every read `.StravaID` (e.g. `got[0].StravaID`, `lim[0].StravaID`) with `.ActivityID`.
  - `TestUpsertSplits`: `StravaID: 300` → `ActivityID: 300`.
  - DELETE `TestStravaTokensRoundTrip` (anchor `func TestStravaTokensRoundTrip`) — `strava_tokens` table dropped by 00009.
  - DELETE `TestUpsertGarminActivity` (anchor `func TestUpsertGarminActivity`), `TestFindGarminActivitiesNear` (anchor `func TestFindGarminActivitiesNear`), helper `mustUpsertGA` (anchor `func mustUpsertGA`) — `garmin_activities` table dropped by 00009.
  - DELETE the whole file `store/oauth_state_test.go` (`git rm internal/store/oauth_state_test.go`) — its `TestOAuthStateSaveAndConsume`/`TestConsumeUnknownOAuthState` exercise the `oauth_states` table dropped by 00009, so they FAIL at runtime here (same class as `TestStravaTokensRoundTrip`). The production `store/oauth_state.go` (`SaveOAuthState`/`ConsumeOAuthState`) STAYS — it is still called by `handlers.go` `stravaConnect`/`stravaCallback` (build-green); both the production file and those handler callers are removed in STRIP Tasks 8/7. Removing the test now also avoids STRIP Task 8 (which `git rm`s `oauth_state.go`) leaving an orphaned non-compiling test file.
  - `TestMigrateSeedsSyncLog` and `TestSyncLogGetAndUpdate`: NO change (they query `sync_log`, untouched by 00009; both `strava`+`garmin` rows still seeded by `00001`).
> **Build-green guard:** `store/garmin.go` STILL defines `UpsertGarminActivity`/etc. and `tokens.go` STILL defines `StravaTokens`/`GetStravaTokens`/`SaveStravaTokens`; `sync.go`/`engine.go` still call them — those Go symbols + callers are removed in STRIP tasks. Here only the *store tests* that exercise the dropped tables are removed; `go build ./...` stays green.

- [ ] **Step 5: Fix `store/activities_test.go`.** Both tests (`TestLatestActivityStartTimeEmpty`, `TestLatestActivityStartTime`) use `StravaID:`. Since Step 1 KEEPS `LatestActivityStartTime`, only re-key the literals: `StravaID: 1`→`ActivityID: 1`, `StravaID: 2`→`ActivityID: 2`. Both tests stay.

- [ ] **Step 5a: Re-key the remaining store test helper `store/streams_test.go`.** In `seedActivity` (anchor `if err := s.UpsertActivity(Activity{` followed by `StravaID: id, Name: "Run", Type: "Run",`), change `StravaID: id` → `ActivityID: id`. This helper seeds the FK parent for the stream round-trip tests; without the re-key `internal/store` fails to COMPILE its test binary.

- [ ] **Step 5b: Re-key the consumer-package test literals (TEST-build compile gate).** These packages' production code reads only `Type/StartTime/DistanceM`/etc. (no `.StravaID`), but their `_test.go` files construct `store.Activity{StravaID: …}` (and one reads `.StravaID`), so the per-package TEST build breaks until re-keyed. Re-key every `StravaID:` literal → `ActivityID:` and every `.StravaID` read → `.ActivityID` in:
  - `api/handlers_test.go`: 2 seed literals (anchors `StravaID: 11, Name: "A"`, `StravaID: 12, Name: "B"`) → `ActivityID:`; 2 reads (anchor `body.Activities[0].StravaID`) → `.ActivityID` (the DTO field re-keyed in Step 3 means the decoded struct field is now `ActivityID`).
  - `coach/coach_test.go`: 1 literal (anchor `StravaID: 1, Type: "Run", Name: "r"`) → `ActivityID:`.
  - `chat/pack_test.go`: 2 literals (anchors `StravaID: int64(1000 + i)`, `StravaID: 9999, Name: "ride"`) → `ActivityID:`.
  - `metrics/metrics_test.go`: every `StravaID:` literal (anchor `{StravaID: 1, Type: "Run"`, …) → `ActivityID:` (use a scoped find/replace of `StravaID:` → `ActivityID:` within this file).
  - `progress/engine_test.go`: 4 literals (anchors `StravaID: 1, Name: "easy"`, `StravaID: 2, Name: "easy2"`, twice each) → `ActivityID:`.
  - `sync/sync_test.go`: TWO sites — the `.StravaID` read (anchor `if len(acts) != 1 || acts[0].StravaID != 900 {`, ~line 84, inside the Strava-refresh test) → `acts[0].ActivityID`, AND the literal (anchor `StravaID: 1, Type: "Run", Name: "r", StartTime:`, ~line 265) → `ActivityID:`. This is a minimal compile-bridge ONLY: `sync_test.go` is otherwise still a Strava-symbol test file at CORE (calls `GetStravaTokens`/`SyncStrava`, which still exist) and is rewritten WHOLE in STRIP Task 6. It must compile here because CORE Task 4 gates on `go test ./internal/sync/ -run TestSyncGarmin`, which compiles every test file in the package.
> `sync/streams_sync_test.go` (`StravaID: id`) and `streams/engine_test.go` (`StravaID:` ×4) are rewritten WHOLE by STRIP Task 6/Task 5. `streams/engine_test.go` is rewritten in Task 5 (it also references the old 5-arg `New`, so it cannot compile at CORE regardless) — do NOT touch it here; `internal/streams`' TEST build is red until Task 5 (only `go build ./...` is the CORE gate). `sync/streams_sync_test.go` references the same `StravaID` field and the `sync` package test binary; since Task 4 needs `internal/sync`'s test binary to compile for its `-run TestSyncGarmin` gate, ALSO re-key `streams_sync_test.go`'s single literal here (anchor `s.UpsertActivity(store.Activity{StravaID: id, Name: "r", Type: "Run",`) → `ActivityID: id` as a compile-bridge (Task 6 rewrites the file whole). `chat/engine_test.go` `.ID` hits are `ChatMessage.ID` — leave.

- [ ] **Step 6: Run — `go build ./...` green + the re-keyed packages PASS; `internal/api` builds-green only.** Command: `cd /home/jake/project/help-my-run/backend && go build ./... 2>&1 | head -30 && go test ./internal/store/ ./internal/progress/ ./internal/coach/ ./internal/chat/ ./internal/metrics/ 2>&1 | tail -30 && go vet ./internal/api/`. Expected: `go build ./...` succeeds (all packages compile — `sync`/`agent`/`streams` still reference strava + garmin_activities Go symbols which all still exist, and `mapActivity` now uses `ActivityID`). `internal/store` PASS (incl `TestMigration00009RekeysAndDrops` + new `wantTables`); `internal/progress`/`internal/coach`/`internal/chat`/`internal/metrics` PASS. `go vet ./internal/api/` clean (test binary compiles).
> NOTE: `internal/api` is **builds-green only** at this task, NOT test-green. Its 6 Strava OAuth tests (`TestStatusOK`, `TestConnectURL`, `TestStravaConnectPersistsState`, `TestStravaCallbackExchangesAndPersists`, `TestStravaCallbackAccessDenied`, `TestStravaCallbackRejectsBadState`) still exist at CORE and FAIL at runtime against the dropped `oauth_states`/`strava_tokens` tables; they are DELETED/rewritten in STRIP Task 7 Step 8. Do NOT run `go test ./internal/api/` as a CORE gate — gate it on `go vet ./internal/api/` (test binary compiles). Likewise `go test ./...` will still FAIL the TEST build of `internal/sync`/`internal/streams` (whole-file rewrites owned by STRIP Tasks 5/6) — `sync/streams_sync_test.go` + `streams/engine_test.go` still reference `StravaID:`/old `New` signatures. The CORE gate here is `go build ./...` green + the five re-keyed non-api packages' tests green + `internal/api` test binary compiling.

- [ ] **Step 7: gofmt + commit (covers Task 1 + Task 2).** Command: `cd /home/jake/project/help-my-run/backend && gofmt -w ./internal/store/ ./internal/api/ ./internal/progress/ ./internal/sync/ ./internal/coach/ ./internal/chat/ ./internal/metrics/ && gofmt -l ./internal/ | head`. Expected: empty. Commit message: `M4: migration 00009 re-key activities (strava_id->activity_id) + drop strava tables; re-key Activity struct + all store fns + consumers`.

---

### Task 3: Enrich the worker activity normalizer (15-key full activity) + RFC3339 start_time (TDD)

> Fully independent of Go. Standalone commit. Decisions applied: (1) `start_time`→RFC3339; (2) `stream_fetch_log.source` unchanged.

**Files:**
- Modify: `garmin-worker/garmin_worker/normalize.py` — `normalize_garmin_activity` (anchor `def normalize_garmin_activity`)
- Modify: `garmin-worker/garmin_worker/cli.py` — `_DRY_ACTIVITIES_RAW` (anchor `_DRY_ACTIVITIES_RAW = [`)
- Modify: `garmin-worker/tests/test_normalize_activity.py`
- Modify: `garmin-worker/tests/test_fetch_cli.py` — `test_dry_run_fetch_includes_activities` (key-set)
- Modify: `garmin-worker/tests/test_fetcher.py` — `test_run_fetch_normalizes_activities` asserts the OLD 6-key contract (`start_time` space format + `duration_s`); re-key it to the 15-key/RFC3339 contract
- Modify: `garmin-worker/tests/fixtures/dry_run_expected.json` — widen `activities[]`
- Create: `garmin-worker/tests/fixtures/activity_list_element.json`
- Test: `cd /home/jake/project/help-my-run/garmin-worker && python3 -m pytest tests/test_normalize_activity.py tests/test_fetch_cli.py tests/test_fetcher.py -q`

- [ ] **Step 1: Failing test — rewrite `test_normalize_activity.py`.** Replace the whole file:
```python
import json

from garmin_worker import normalize

FULL_EL = {
    "activityId": 14820001234,
    "activityName": "Morning Run",
    "startTimeGMT": "2026-06-22 05:00:00",
    "startTimeLocal": "2026-06-22 07:00:00",
    "activityType": {"typeKey": "running"},
    "distance": 10000.0,
    "movingDuration": 3200.0,
    "elapsedDuration": 3300.0,
    "duration": 3300.0,
    "averageHR": 148.0,
    "maxHR": 168.0,
    "averageSpeed": 3.05,
    "maxSpeed": 4.2,
    "averageRunningCadenceInStepsPerMinute": 172.0,
    "elevationGain": 85.0,
    "extra": "ignored-but-kept-in-raw",
}

EXPECTED_KEYS = {
    "garmin_activity_id", "name", "start_time", "start_time_local",
    "activity_type", "distance_m", "moving_time_s", "elapsed_time_s",
    "avg_hr", "max_hr", "avg_speed", "max_speed", "avg_cadence",
    "elevation_gain_m", "raw_json",
}


def test_normalize_garmin_activity_maps_all_15_fields():
    out = normalize.normalize_garmin_activity(FULL_EL)
    assert set(out.keys()) == EXPECTED_KEYS
    assert out["garmin_activity_id"] == 14820001234
    assert out["name"] == "Morning Run"
    # RFC3339: space -> T, append Z.
    assert out["start_time"] == "2026-06-22T05:00:00Z"
    # local stays Garmin space format (no offset available).
    assert out["start_time_local"] == "2026-06-22 07:00:00"
    assert out["activity_type"] == "running"
    assert out["distance_m"] == 10000.0
    assert out["moving_time_s"] == 3200.0
    assert out["elapsed_time_s"] == 3300.0
    assert out["avg_hr"] == 148.0
    assert out["max_hr"] == 168.0
    assert out["avg_speed"] == 3.05
    assert out["max_speed"] == 4.2
    assert out["avg_cadence"] == 172.0
    assert out["elevation_gain_m"] == 85.0
    assert out["raw_json"] == FULL_EL  # ORIGINAL element preserved


def test_elapsed_falls_back_to_duration_when_elapsedDuration_missing():
    el = dict(FULL_EL)
    del el["elapsedDuration"]
    out = normalize.normalize_garmin_activity(el)
    assert out["elapsed_time_s"] == 3300.0  # from "duration"


def test_normalize_garmin_activity_trail_run_typekey():
    out = normalize.normalize_garmin_activity({
        "activityId": 99, "activityType": {"typeKey": "trail_running"},
    })
    assert out["activity_type"] == "trail_running"


def test_normalize_garmin_activity_all_enriched_fields_none():
    out = normalize.normalize_garmin_activity({"activityId": 7})
    assert set(out.keys()) == EXPECTED_KEYS
    assert out["garmin_activity_id"] == 7
    assert out["name"] is None
    assert out["start_time"] is None        # missing startTimeGMT -> None (no RFC3339 coercion)
    assert out["start_time_local"] is None
    assert out["activity_type"] is None
    assert out["distance_m"] is None
    assert out["moving_time_s"] is None
    assert out["elapsed_time_s"] is None
    assert out["avg_hr"] is None
    assert out["max_hr"] is None
    assert out["avg_speed"] is None
    assert out["max_speed"] is None
    assert out["avg_cadence"] is None
    assert out["elevation_gain_m"] is None
    assert out["raw_json"] == {"activityId": 7}


def test_normalize_garmin_activity_none_input():
    out = normalize.normalize_garmin_activity(None)
    assert out["garmin_activity_id"] is None
    assert out["start_time"] is None
    assert out["raw_json"] == {}


def test_normalize_garmin_activity_json_serializable():
    json.loads(json.dumps(normalize.normalize_garmin_activity(FULL_EL)))


def test_normalize_garmin_activity_from_fixture():
    import pathlib
    p = pathlib.Path(__file__).parent / "fixtures" / "activity_list_element.json"
    el = json.loads(p.read_text())
    out = normalize.normalize_garmin_activity(el)
    assert set(out.keys()) == EXPECTED_KEYS
    assert out["avg_hr"] is not None
    assert out["avg_cadence"] is not None
    assert out["elevation_gain_m"] is not None
    assert out["start_time"].endswith("Z")


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

- [ ] **Step 2: Add the enriched fixture.** Create `garmin-worker/tests/fixtures/activity_list_element.json`:
```json
{
  "activityId": 14820001234,
  "activityName": "Morning Run",
  "startTimeGMT": "2026-06-22 05:00:00",
  "startTimeLocal": "2026-06-22 07:00:00",
  "activityType": { "typeKey": "running" },
  "distance": 10000.0,
  "movingDuration": 3200.0,
  "elapsedDuration": 3300.0,
  "duration": 3300.0,
  "averageHR": 148.0,
  "maxHR": 168.0,
  "averageSpeed": 3.05,
  "maxSpeed": 4.2,
  "averageRunningCadenceInStepsPerMinute": 172.0,
  "elevationGain": 85.0
}
```

- [ ] **Step 3: Run — expect FAIL.** Command: `cd /home/jake/project/help-my-run/garmin-worker && python3 -m pytest tests/test_normalize_activity.py -q 2>&1 | tail -25`. Expected: FAIL — current normalizer returns the 6-key set (`duration_s`, no `name`/`avg_hr`/etc.); `start_time` is the raw space string not RFC3339.

- [ ] **Step 4: Minimal impl — rewrite `normalize_garmin_activity` in `normalize.py`.** Replace the function body (anchor `def normalize_garmin_activity(el: Optional[dict]) -> dict:` through its `return {...}`):
```python
def _gmt_to_rfc3339(s: Optional[str]) -> Optional[str]:
    """Garmin startTimeGMT "YYYY-MM-DD HH:MM:SS" -> RFC3339 "YYYY-MM-DDTHH:MM:SSZ".

    Returns None for a falsy input. Only the single space between the date and
    time is replaced; a value that is already RFC3339 (has 'T') is returned as-is
    apart from ensuring a trailing 'Z'."""
    if not s:
        return None
    out = s.replace(" ", "T", 1)
    if not out.endswith("Z"):
        out = out + "Z"
    return out


def normalize_garmin_activity(el: Optional[dict]) -> dict:
    """Map one get_activities_by_date element -> the full activity contract.

    Reads plain-dict keys (worker never uses garminconnect typed models). Every
    field is Optional on the Garmin list element, so all enriched values may be
    None. start_time is normalized to RFC3339 (so the Go engines' time.Parse with
    RFC3339 succeeds); start_time_local keeps Garmin's local wall-clock string.
    elapsed_time_s falls back to "duration" when "elapsedDuration" is absent.
    raw_json preserves the ORIGINAL element.
    """
    el = el or {}
    return {
        "garmin_activity_id": el.get("activityId"),
        "name": el.get("activityName"),
        "start_time": _gmt_to_rfc3339(el.get("startTimeGMT")),
        "start_time_local": el.get("startTimeLocal"),
        "activity_type": _get(el, "activityType", "typeKey"),
        "distance_m": el.get("distance"),
        "moving_time_s": el.get("movingDuration"),
        "elapsed_time_s": el.get("elapsedDuration") if el.get("elapsedDuration") is not None else el.get("duration"),
        "avg_hr": el.get("averageHR"),
        "max_hr": el.get("maxHR"),
        "avg_speed": el.get("averageSpeed"),
        "max_speed": el.get("maxSpeed"),
        "avg_cadence": el.get("averageRunningCadenceInStepsPerMinute"),
        "elevation_gain_m": el.get("elevationGain"),
        "raw_json": el,
    }
```
> `elapsed_time_s` uses an explicit `is not None` check (not `or`) so a legitimate `0.0` elapsed is not clobbered by the `duration` fallback.

- [ ] **Step 5: Run normalize tests — expect PASS.** Command: `cd /home/jake/project/help-my-run/garmin-worker && python3 -m pytest tests/test_normalize_activity.py -q 2>&1 | tail -15`. Expected: all PASS.

- [ ] **Step 6: Update `_DRY_ACTIVITIES_RAW` + dry-run fixture + dry-run test.** In `cli.py`, replace `_DRY_ACTIVITIES_RAW` (anchor `_DRY_ACTIVITIES_RAW = [`):
```python
_DRY_ACTIVITIES_RAW = [
    {"activityId": 14820001234, "activityName": "Morning Run", "startTimeGMT": "2026-06-14 05:00:00", "startTimeLocal": "2026-06-14 07:00:00", "duration": 3300.0, "elapsedDuration": 3300.0, "movingDuration": 3200.0, "distance": 10000.0, "activityType": {"typeKey": "running"}, "averageHR": 148.0, "maxHR": 168.0, "averageSpeed": 3.05, "maxSpeed": 4.2, "averageRunningCadenceInStepsPerMinute": 172.0, "elevationGain": 85.0},
    {"activityId": 14820005678, "activityName": "Trail Run", "startTimeGMT": "2026-06-15 06:00:00", "startTimeLocal": "2026-06-15 08:00:00", "duration": 2700.0, "elapsedDuration": 2700.0, "movingDuration": 2650.0, "distance": 8000.0, "activityType": {"typeKey": "trail_running"}, "averageHR": 152.0, "maxHR": 175.0, "averageSpeed": 2.96, "maxSpeed": 3.9, "averageRunningCadenceInStepsPerMinute": 168.0, "elevationGain": 210.0},
]
```
  In `tests/test_fetch_cli.py`, `test_dry_run_fetch_includes_activities` (anchor the `set(a.keys()) == {` block):
```python
    assert set(a.keys()) == {
        "garmin_activity_id", "name", "start_time", "start_time_local",
        "activity_type", "distance_m", "moving_time_s", "elapsed_time_s",
        "avg_hr", "max_hr", "avg_speed", "max_speed", "avg_cadence",
        "elevation_gain_m", "raw_json",
    }
```
  Regenerate `tests/fixtures/dry_run_expected.json`'s `activities[]` (`start_time` is RFC3339; `raw_json` is the full original element):
```json
  "activities": [
    {
      "garmin_activity_id": 14820001234,
      "name": "Morning Run",
      "start_time": "2026-06-14T05:00:00Z",
      "start_time_local": "2026-06-14 07:00:00",
      "activity_type": "running",
      "distance_m": 10000.0,
      "moving_time_s": 3200.0,
      "elapsed_time_s": 3300.0,
      "avg_hr": 148.0,
      "max_hr": 168.0,
      "avg_speed": 3.05,
      "max_speed": 4.2,
      "avg_cadence": 172.0,
      "elevation_gain_m": 85.0,
      "raw_json": {"activityId": 14820001234, "activityName": "Morning Run", "startTimeGMT": "2026-06-14 05:00:00", "startTimeLocal": "2026-06-14 07:00:00", "duration": 3300.0, "elapsedDuration": 3300.0, "movingDuration": 3200.0, "distance": 10000.0, "activityType": {"typeKey": "running"}, "averageHR": 148.0, "maxHR": 168.0, "averageSpeed": 3.05, "maxSpeed": 4.2, "averageRunningCadenceInStepsPerMinute": 172.0, "elevationGain": 85.0}
    },
    {
      "garmin_activity_id": 14820005678,
      "name": "Trail Run",
      "start_time": "2026-06-15T06:00:00Z",
      "start_time_local": "2026-06-15 08:00:00",
      "activity_type": "trail_running",
      "distance_m": 8000.0,
      "moving_time_s": 2650.0,
      "elapsed_time_s": 2700.0,
      "avg_hr": 152.0,
      "max_hr": 175.0,
      "avg_speed": 2.96,
      "max_speed": 3.9,
      "avg_cadence": 168.0,
      "elevation_gain_m": 210.0,
      "raw_json": {"activityId": 14820005678, "activityName": "Trail Run", "startTimeGMT": "2026-06-15 06:00:00", "startTimeLocal": "2026-06-15 08:00:00", "duration": 2700.0, "elapsedDuration": 2700.0, "movingDuration": 2650.0, "distance": 8000.0, "activityType": {"typeKey": "trail_running"}, "averageHR": 152.0, "maxHR": 175.0, "averageSpeed": 2.96, "maxSpeed": 3.9, "averageRunningCadenceInStepsPerMinute": 168.0, "elevationGain": 210.0}
    }
  ]
```
> **Anchor caution:** Before editing `dry_run_expected.json`, confirm which test loads it: `grep -rn dry_run_expected tests/`. If a test does a full-object `==`, the non-activities sections (sleep/hrv/bb/rhr/vo2max/since/until/fetched_at) MUST remain byte-identical — only swap the `activities` array. Keep `fetched_at` `"2026-06-15T05:00:12Z"` and key order intact.

- [ ] **Step 6-FIT: Verify + lock the `.FIT` download to the REAL garminconnect method (M4 makes Garmin `.FIT` the SOLE stream source).** VERIFIED in the worker venv (`garmin-worker/.venv/bin/python -c "import garminconnect; print([m for m in dir(garminconnect.Garmin) if 'download' in m.lower()]); print(list(garminconnect.Garmin.ActivityDownloadFormat))"`): there is NO `download_activity_original` on `garminconnect.Garmin` — the real method is `download_activity(activity_id, dl_fmt=ActivityDownloadFormat.TCX) -> bytes`, and `ActivityDownloadFormat.ORIGINAL` returns **the raw ZIP bytes of the `.fit`** (the wrapped client's docstring: *"For 'Original' will return the zip file content, up to user to extract it."*). The worker's `normalize_fit_stream(raw)` ALREADY extracts the `.fit` from that zip (`zipfile.ZipFile(io.BytesIO(raw))` in `normalize.py`), so the zip return type needs no new handling — only the call must hit the real method.
  - **client delegate (`garmin_worker/client.py`):** the `GarminClient.download_activity_original` delegate (anchor `def download_activity_original(self, activity_id: str) -> bytes:`) is ALREADY correct — it calls `self._g.download_activity(activity_id, ActivityDownloadFormat.ORIGINAL)`. Leave the call; only confirm it (the FIX bug exists if a worker ever calls `garminconnect.Garmin.download_activity_original` directly — it does not; `run_fit_fetch` goes through this wrapper). If the delegate is missing/renamed in the tree, (re)add it as shown above.
  - **`fetcher.py` `run_fit_fetch` (anchor `raw = client.download_activity_original(activity_id)`):** keep the call (it routes to the real `download_activity(..., ORIGINAL)` via the wrapper). Fix the stale Strava-era doc (anchor `activity_id is GARMIN's download id; echo_id is the Strava id echoed back so`):
```python
def run_fit_fetch(client, *, activity_id: str, echo_id: int, fetched_at: str) -> dict:
    """Download + parse one activity's FIT -> the §2.6 stream object.

    activity_id is GARMIN's download id; echo_id is the activity id (identity:
    equals the Garmin id in M4) echoed back so the Go store row keys correctly
    (§7 id mapping). The ORIGINAL download is a ZIP of the .fit; the client
    delegate calls garminconnect download_activity(id, ORIGINAL) and
    normalize_fit_stream extracts the .fit from the zip."""
    raw = client.download_activity_original(activity_id)
    series = normalize.normalize_fit_stream(raw)
    return normalize.build_fit_output(activity_id=echo_id, fetched_at=fetched_at, series=series)
```
  - **Add a monkeypatched unit test** (no Garmin login, no real `.fit` parse) to `tests/test_fetcher.py` proving `run_fit_fetch` (1) calls the wrapper's download with the GARMIN id, (2) feeds the bytes to `normalize_fit_stream`, and (3) echoes `echo_id`:
```python
def test_run_fit_fetch_uses_download_and_echoes_id(monkeypatch):
    class _FitClient:
        def __init__(self):
            self.dl_args = None
        def download_activity_original(self, activity_id):
            self.dl_args = activity_id
            return b"ZIP-BYTES"  # opaque; normalize is monkeypatched below

    seen = {}
    def _fake_normalize(raw):
        seen["raw"] = raw
        return {"t": [0.0, 1.0], "hr": [120, 130], "v": [2.0, 2.0], "dist": [0.0, 2.0]}
    monkeypatch.setattr(fetcher.normalize, "normalize_fit_stream", _fake_normalize)

    c = _FitClient()
    out = fetcher.run_fit_fetch(c, activity_id="14820001234", echo_id=14820001234, fetched_at="t")
    assert c.dl_args == "14820001234"          # real download hit with the garmin id
    assert seen["raw"] == b"ZIP-BYTES"          # zip bytes piped to the parser
    assert out["activity_id"] == 14820001234    # echo id is the store PK
    assert out["source"] == "garmin"
    assert out["series"]["hr"] == [120, 130]
```
  - **Optional real-parse guard test** (skips when `garmin_fit_sdk` is absent): if you add an end-to-end parse test that calls `normalize_fit_stream` over a real zip fixture, guard the SDK import with `pytest.importorskip("garmin_fit_sdk")` so the suite stays green where the optional dep is not installed. (The monkeypatched test above needs no such guard — it never imports the SDK.)
> **One integration-verify point (offline-unverifiable):** the exact `download_activity(id, ORIGINAL)` HTTP response shape (a zip containing exactly one `*.fit`) is only confirmable against a live Garmin account. The code path is implemented against the documented/installed `garminconnect` 0.3.6 API (verified by `dir`/`getsource` in the venv); confirm during the Task 13 Step 4 manual `make garmin-login` → sync run that real streams land (success criterion #2).

- [ ] **Step 6a: Re-key `tests/test_fetcher.py` to the 15-key/RFC3339 contract.** `test_run_fetch_normalizes_activities` (anchor `def test_run_fetch_normalizes_activities`) still asserts the OLD 6-key contract against `_MockClient`'s default element (`{"activityId": 14820001234, "startTimeGMT": "2026-06-22 05:00:00", "duration": 3300.0, "distance": 10000.0, "activityType": {"typeKey": "running"}}` — note: NO `movingDuration`/`elapsedDuration`). Replace the two failing assertions:
```python
    assert a["start_time"] == "2026-06-22 05:00:00"
    assert a["duration_s"] == 3300.0
```
  with the new-contract values (RFC3339 `start_time`; `duration_s` is gone — the mock has only `duration`, which falls back to `elapsed_time_s`; `moving_time_s` is `None` since the mock has no `movingDuration`):
```python
    assert a["start_time"] == "2026-06-22T05:00:00Z"
    assert a["moving_time_s"] is None        # mock has no movingDuration
    assert a["elapsed_time_s"] == 3300.0     # falls back to "duration"
```
  Leave the other assertions in that test (`garmin_activity_id == 14820001234`, `distance_m == 10000.0`, `activity_type == "running"`, `"raw_json" in a`) — they hold under the new contract. No other test in `test_fetcher.py` asserts activity keys (`test_run_fetch_skips_activities_without_id` / `test_run_fetch_activities_failure_degrades_to_empty` check only ids/empties). `grep -n "duration_s\|garmin_activity_id\|start_time" tests/test_fetcher.py` after the edit should show NO `duration_s` for activities (the `sleep[0]["duration_s"]` assertion at line ~95 is a sleep field — leave it).

- [ ] **Step 7: Run full worker suite — expect PASS.** Command: `cd /home/jake/project/help-my-run/garmin-worker && python3 -m pytest -q 2>&1 | tail -15`. Expected: all PASS. (`test_cli.py` does not assert activity keys — verify with `grep -n "duration_s\|start_time\|garmin_activity_id" tests/test_cli.py`; if a `cli` dry-run test full-object-`==`s the dry-run fixture, it is already covered by the Step 6 fixture regen.)

- [ ] **Step 8: Commit.** Message: `M4: enrich Garmin activity normalizer to full 15-key record + RFC3339 start_time; update dry-run + fixtures`.

---

### Task 4: Go `WorkerOutput.Activities` full-activity struct + `SyncGarmin` upsert into `activities` (TDD, stub-worker)

> Depends on Task 2 (`UpsertActivity` re-keyed) and Task 3 (enriched worker JSON). The SyncGarmin loop changes from `UpsertGarminActivity(garmin_activities)` to `UpsertActivity(activities)`. Commits last among CORE.

**Files:**
- Modify: `backend/internal/garmin/types.go` — `GarminActivity` struct (anchor `type GarminActivity struct`); `FITStreamOutput.ActivityID` comment (anchor `// echoed Strava id (store PK)`)
- Modify: `backend/internal/garmin/runner.go` — `RunGarminFetchFIT` doc comment (anchor `echoActivityID is the Strava id`)
- Modify: `backend/internal/sync/sync.go` — add `math` import + `f64ptrToI64`; rewrite the `out.Activities` loop in `SyncGarmin` (anchor `for _, a := range out.Activities {`)
- Modify: `backend/internal/garmin/testdata/worker_output.json` — enrich `activities[]`
- Modify: `backend/internal/sync/sync_test.go` — `TestSyncGarminUpsertsAllTables` (anchor the `garmin_activities` count + `SELECT activity_type, start_time FROM garmin_activities`)
- Test: `go test ./internal/garmin/ ./internal/sync/ -run TestSyncGarmin`

> **Scope/ordering:** Gate via `-run TestSyncGarmin` so this task is verifiable independently of the Strava-test deletions (which are STRIP Task 5). This task supplies the `types.go` enrich + `f64ptrToI64` + the loop body + the fixture + the SyncGarmin assertions; STRIP Task 5 supplies the surrounding `SyncStrava`/`AllResult`/`SyncAll` deletions.

- [ ] **Step 1: Failing test — enrich the Go fixture + rewrite the activities assertions.** Enrich `backend/internal/garmin/testdata/worker_output.json` `activities[]` (anchor `"activities": [`) to the 15-key shape with RFC3339 `start_time`:
```json
  "activities": [
    { "garmin_activity_id": 14820001234, "name": "Morning Run", "start_time": "2026-06-14T05:00:00Z", "start_time_local": "2026-06-14 07:00:00", "activity_type": "running", "distance_m": 10000.0, "moving_time_s": 3200.0, "elapsed_time_s": 3300.0, "avg_hr": 148.0, "max_hr": 168.0, "avg_speed": 3.05, "max_speed": 4.2, "avg_cadence": 172.0, "elevation_gain_m": 85.0, "raw_json": {"activityId": 14820001234} },
    { "garmin_activity_id": 14820005678, "name": "Trail Run", "start_time": "2026-06-15T06:00:00Z", "start_time_local": "2026-06-15 08:00:00", "activity_type": "trail_running", "distance_m": 8000.0, "moving_time_s": 2650.0, "elapsed_time_s": 2700.0, "avg_hr": 152.0, "max_hr": 175.0, "avg_speed": 2.96, "max_speed": 3.9, "avg_cadence": 168.0, "elevation_gain_m": 210.0, "raw_json": {"activityId": 14820005678} }
  ]
```
  Then in `sync_test.go` `TestSyncGarminUpsertsAllTables`: edit the `counts` map (anchor `"garmin_vo2max": 0, "garmin_activities": 0,`):
```go
	counts := map[string]int{
		"garmin_sleep": 0, "garmin_hrv": 0, "garmin_body_battery": 0, "garmin_rhr": 0,
		"garmin_vo2max": 0, "activities": 0,
	}
```
  the count assertion (anchor `counts["garmin_vo2max"] != 2 || counts["garmin_activities"] != 2`):
```go
	if counts["garmin_sleep"] != 2 || counts["garmin_hrv"] != 1 ||
		counts["garmin_body_battery"] != 2 || counts["garmin_rhr"] != 2 ||
		counts["garmin_vo2max"] != 2 || counts["activities"] != 2 {
		t.Errorf("counts = %+v, want sleep2 hrv1 bb2 rhr2 vo2max2 activities2", counts)
	}
```
  the activity-row assertion (anchor `SELECT activity_type, start_time FROM garmin_activities WHERE garmin_activity_id=?`):
```go
	// Activity ingested into the canonical activities table, re-keyed by Garmin id.
	var atype, ast, aname string
	var avgHR sql.NullFloat64
	var avgCad sql.NullFloat64
	var movS, elapS int64
	_ = s.DB.QueryRow(
		`SELECT type, start_time, name, avg_hr, avg_cadence, moving_time_s, elapsed_time_s
		   FROM activities WHERE activity_id=?`,
		14820001234).Scan(&atype, &ast, &aname, &avgHR, &avgCad, &movS, &elapS)
	if atype != "running" || ast != "2026-06-14T05:00:00Z" || aname != "Morning Run" {
		t.Errorf("activity 14820001234 = (%q,%q,%q), want (running, 2026-06-14T05:00:00Z, Morning Run)", atype, ast, aname)
	}
	if !avgHR.Valid || avgHR.Float64 != 148 {
		t.Errorf("avg_hr = %v, want 148", avgHR)
	}
	if !avgCad.Valid || avgCad.Float64 != 172 {
		t.Errorf("avg_cadence = %v, want 172", avgCad)
	}
	// float worker durations rounded into the INTEGER columns.
	if movS != 3200 || elapS != 3300 {
		t.Errorf("moving/elapsed = (%d,%d), want (3200,3300)", movS, elapS)
	}
```
  **Add `"database/sql"` to the `sync_test.go` import block** (not currently imported). The `res.Synced != 11` assertion is unchanged (2 sleep + 1 hrv + 2 bb + 2 rhr + 2 vo2max + 2 activities = 11).

- [ ] **Step 2: Run — expect FAIL.** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/sync/ -run TestSyncGarminUpsertsAllTables 2>&1 | tail -25`. Expected: FAIL — the worker JSON now carries 15-key activities but `garmin.GarminActivity` only has 6 fields (enriched fields dropped at unmarshal); AND `SyncGarmin` still calls `UpsertGarminActivity` into `garmin_activities` which 00009 dropped → "no such table" (status=error) or 0 rows in `activities`.

- [ ] **Step 3: Impl — enrich `garmin/types.go` `GarminActivity`.** Replace the struct (anchor `type GarminActivity struct`) per **Contract C** `GarminActivity` (with the doc comment noting `DurationS` removed, durations `*float64`).
> `Name` is `string` (not `*string`): worker JSON `null` → Go `string` `""`, matching `activities.name NOT NULL` default.
  Update `FITStreamOutput.ActivityID` comment (anchor `// echoed Strava id (store PK)`):
```go
	ActivityID int64     `json:"activity_id"` // echoed activity id (store PK)
```

- [ ] **Step 4: Impl — `runner.go` doc comment.** Anchor `// Garmin download id; echoActivityID is the Strava id echoed back as`:
```go
// RunGarminFetchFIT runs `<python> <script> stream --activity-id <garminID>
// --echo-id <echoID>`, parsing the §2.6 stdout JSON. garminActivityID is the
// Garmin download id; echoActivityID is the activity id (identity: equals
// garminActivityID) echoed back as out.ActivityID so the store row keys
// correctly.
```

- [ ] **Step 5: Impl — `sync.go` helper + rewired loop.** Add `"math"` to the import block (with `"encoding/json"`, `"time"`). Add the helper (anchor: just above `// rawString renders` or below `SyncGarmin`):
```go
// f64ptrToI64 dereferences a nullable worker duration to a rounded int64,
// defaulting nil to 0 (the activities INTEGER columns are NOT NULL).
func f64ptrToI64(p *float64) int64 {
	if p == nil {
		return 0
	}
	return int64(math.Round(*p))
}
```
  Replace the `out.Activities` loop in `SyncGarmin` (anchor `for _, a := range out.Activities {` through its closing `}` + `synced++`):
```go
	for _, a := range out.Activities {
		atype := ""
		if a.ActivityType != nil {
			atype = *a.ActivityType
		}
		dist := 0.0
		if a.DistanceM != nil {
			dist = *a.DistanceM
		}
		if err := s.UpsertActivity(store.Activity{
			ActivityID:     a.GarminActivityID,
			Name:           a.Name,
			Type:           atype,
			SportType:      nil, // Garmin list has no sportType
			StartTime:      a.StartTime,
			StartTimeLocal: a.StartTimeLocal,
			DistanceM:      dist,
			MovingTimeS:    f64ptrToI64(a.MovingTimeS),
			ElapsedTimeS:   f64ptrToI64(a.ElapsedTimeS),
			AvgHR:          a.AvgHR,
			MaxHR:          a.MaxHR,
			AvgSpeed:       a.AvgSpeed,
			MaxSpeed:       a.MaxSpeed,
			AvgCadence:     a.AvgCadence,
			ElevationGainM: a.ElevationGainM,
			RawJSON:        rawString(a.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
```
> No `UpsertSplits` call (Garmin list has no laps).

- [ ] **Step 6: Run — expect PASS.** Command: `cd /home/jake/project/help-my-run/backend && go build ./... 2>&1 | head && go test ./internal/garmin/ ./internal/sync/ -run 'TestSyncGarmin|TestGarmin' 2>&1 | tail -20`. Expected: build green; `TestSyncGarminUpsertsAllTables`, `TestSyncGarminError`, `TestSyncGarminBackfillWindowIs84Days` PASS.
> `go build ./...` stays green: the only `UpsertGarminActivity` call was the loop just rewired; `store/garmin.go` still *defines* it (removed in STRIP Task 8). The `garmin_activities` table is gone (00009) but `SyncGarmin` no longer references it.

- [ ] **Step 7: gofmt + commit.** Command: `cd /home/jake/project/help-my-run/backend && gofmt -w ./internal/garmin/ ./internal/sync/ && gofmt -l ./internal/garmin/ ./internal/sync/`. Expected: empty. Commit message: `M4: enrich Go GarminActivity to full record; SyncGarmin upserts activities into the canonical activities table (re-keyed by Garmin id)`.

---

### Task 5: Streams — Garmin .FIT only (identity resolve, drop Strava client)

> Runs after CORE (depends on `activities.activity_id`/re-keyed `GetActivity`). First STRIP task. Removes the last `GetActivity`-via-resolve + `FindGarminActivitiesNear` + token-fn users in streams.

**Files:**
- Modify: `backend/internal/streams/engine.go` (`Engine`, `New`, `FetchAndAnalyze`, `resolveGarminID`; delete `accessToken`, `refreshBuffer`, `absF`)
- Delete: `backend/internal/streams/strava.go`
- Delete: `backend/internal/streams/strava_test.go`
- Modify: `backend/internal/garmin/runner.go` (already done in CORE Task 4 — verify), `backend/internal/garmin/types.go` (already done — verify)
- Test: `backend/internal/streams/engine_test.go` (rewrite)

- [ ] **Step 1: Write the failing test rewrite for `engine_test.go`.** Replace the whole file (it currently imports `strava`, calls `New(...,strava,...,120)`, tests match-based resolve):
```go
package streams

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
)

func newStreamsStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "streams.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func newTestEngine(t *testing.T, s *store.Store) *Engine {
	t.Helper()
	// M4: runner is unused by GetOrComputeAnalysis; pass a zero Runner.
	return New(s, garmin.Runner{}, nil)
}

// seedRawStream stores an activity + its gzipped raw stream (no analysis yet).
func seedRawStream(t *testing.T, s *store.Store, id int64, ser Series) {
	t.Helper()
	if err := s.UpsertActivity(store.Activity{
		ActivityID: id, Name: "run", Type: "Run",
		StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000, MovingTimeS: 3300, ElapsedTimeS: 3300, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}
	gz, err := CompressSeries(ser)
	if err != nil {
		t.Fatalf("compress: %v", err)
	}
	if err := s.UpsertActivityStream(store.ActivityStream{
		ActivityID: id, Source: "garmin", SeriesGz: gz, FetchedAt: "2026-06-20T07:00:00Z",
	}); err != nil {
		t.Fatalf("upsert stream: %v", err)
	}
}

func TestGetOrComputeAnalysisFirstCompute(t *testing.T) {
	s := newStreamsStore(t)
	ser := Series{T: []float64{0, 1, 2, 3}, HR: []float64{120, 120, 130, 130}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}}
	seedRawStream(t, s, 100, ser)
	e := newTestEngine(t, s)

	got, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetOrComputeAnalysis error = %v", err)
	}
	if !got.HasHR || len(got.TimeInZone) != 5 {
		t.Errorf("analysis = %+v, want HasHR + 5 zones", got)
	}
	if got.ComputedAt == "" {
		t.Error("ComputedAt empty, want set on compute")
	}
	if got.Source != "garmin" {
		t.Errorf("Source = %q, want garmin (carried from raw)", got.Source)
	}
	if _, err := s.GetStreamAnalysis(100); err != nil {
		t.Errorf("analysis not cached: %v", err)
	}
}

func TestGetOrComputeAnalysisReturnsCached(t *testing.T) {
	s := newStreamsStore(t)
	ser := Series{T: []float64{0, 1, 2, 3}, HR: []float64{120, 120, 130, 130}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}}
	seedRawStream(t, s, 100, ser)
	e := newTestEngine(t, s)

	first, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.ComputedAt != first.ComputedAt {
		t.Errorf("recomputed unexpectedly: %q != %q", second.ComputedAt, first.ComputedAt)
	}
}

func TestGetOrComputeAnalysisRecomputesOnZoneChange(t *testing.T) {
	s := newStreamsStore(t)
	ser := Series{
		T:  []float64{0, 1, 2, 3},
		HR: []float64{140, 140, 150, 150}, // 2 below 145, 2 above (default)
		V:  []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6},
	}
	seedRawStream(t, s, 100, ser)
	e := newTestEngine(t, s)

	first, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	firstZones := first.Zones

	z2 := int64(155)
	thr := int64(170)
	if err := s.UpsertAthleteProfile(store.AthleteProfile{
		TargetWeeklyKm: 20, ProgressionMode: "build", RunConstraintsJSON: "{}",
		Zone2CeilingBpm: &z2, ThresholdBpm: &thr,
	}); err != nil {
		t.Fatalf("upsert profile: %v", err)
	}

	second, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Zones == firstZones {
		t.Errorf("zones unchanged after profile change: %+v", second.Zones)
	}
	if second.Zones.Z2Hi != 155 {
		t.Errorf("recomputed Z2Hi = %v, want 155", second.Zones.Z2Hi)
	}
	row, err := s.GetStreamAnalysis(100)
	if err != nil {
		t.Fatalf("get analysis row: %v", err)
	}
	want, _ := json.Marshal(ZonesFromProfile(store.AthleteProfile{Zone2CeilingBpm: &z2, ThresholdBpm: &thr}))
	if row.ZonesJSON != string(want) {
		t.Errorf("cached zones_json = %s, want %s", row.ZonesJSON, want)
	}
}

func TestGetOrComputeAnalysisNotFetched(t *testing.T) {
	s := newStreamsStore(t)
	if err := s.UpsertActivity(store.Activity{
		ActivityID: 100, Name: "run", Type: "Run",
		StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000, MovingTimeS: 3300, ElapsedTimeS: 3300, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}
	e := newTestEngine(t, s)
	_, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != store.ErrNotFound {
		t.Errorf("err = %v, want store.ErrNotFound (no raw stream stored)", err)
	}
}

// With a raw stream stored but NO athlete_profile row, GetOrComputeAnalysis must
// SUCCEED using default zones — a missing profile must not be conflated with a
// missing stream.
func TestGetOrComputeAnalysisMissingProfileUsesDefaults(t *testing.T) {
	s := newStreamsStore(t)
	ser := Series{T: []float64{0, 1, 2, 3}, HR: []float64{120, 120, 130, 130}, V: []float64{2, 2, 2, 2}, Dist: []float64{0, 2, 4, 6}}
	seedRawStream(t, s, 100, ser)

	if _, err := s.DB.Exec(`DELETE FROM athlete_profile`); err != nil {
		t.Fatalf("delete profile: %v", err)
	}
	if _, err := s.GetAthleteProfile(); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("precondition: GetAthleteProfile err = %v, want store.ErrNotFound", err)
	}

	e := newTestEngine(t, s)
	got, err := e.GetOrComputeAnalysis(context.Background(), 100)
	if err != nil {
		t.Fatalf("GetOrComputeAnalysis with missing profile error = %v, want success with default zones", err)
	}
	want := ZonesFromProfile(store.AthleteProfile{})
	if got.Zones != want {
		t.Errorf("zones = %+v, want defaults %+v", got.Zones, want)
	}
}

// M4: resolveGarminID is identity — the activity id IS the Garmin download id.
func TestResolveGarminIDIsIdentity(t *testing.T) {
	s := newStreamsStore(t)
	e := newTestEngine(t, s)
	for _, id := range []int64{1, 14820001234, 999} {
		gid, ok := e.resolveGarminID(id)
		if !ok || gid != id {
			t.Errorf("resolveGarminID(%d) = (%d,%v), want (%d,true)", id, gid, ok, id)
		}
	}
}

// M4: FetchAndAnalyze uses the .FIT worker as the SOLE stream source.
func TestFetchAndAnalyzeFITOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh for the FIT runner stub")
	}
	const activityID = int64(14820001234)
	const startISO = "2026-06-22T05:00:00Z"

	s := newStreamsStore(t)
	if err := s.UpsertActivity(store.Activity{
		ActivityID: activityID, Name: "run", Type: "Run",
		StartTime: startISO, DistanceM: 5000, MovingTimeS: 1800, ElapsedTimeS: 1850, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}

	// FIT runner stub: /bin/sh script echoing the FIT JSON with HR. It asserts
	// it was called with the identity garmin id == echo id.
	const fitOut = `{"activity_id":14820001234,"source":"garmin","fetched_at":"2026-06-22T05:00:12Z","series":{"t":[0,1,2,3],"hr":[140,142,150,152],"v":[2.0,2.0,2.0,2.0],"dist":[0,2,4,6]}}`
	script := filepath.Join(t.TempDir(), "fitstub.sh")
	body := "#!/bin/sh\n" +
		"echo \"$@\" | grep -q -- '--activity-id 14820001234' || { echo 'missing --activity-id 14820001234' 1>&2; exit 2; }\n" +
		"echo \"$@\" | grep -q -- '--echo-id 14820001234' || { echo 'missing --echo-id 14820001234' 1>&2; exit 2; }\n" +
		"echo '" + fitOut + "'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write fit stub: %v", err)
	}
	runner := garmin.Runner{Python: "/bin/sh", Script: script}

	e := New(s, runner, nil)

	got, err := e.FetchAndAnalyze(context.Background(), activityID)
	if err != nil {
		t.Fatalf("FetchAndAnalyze error = %v", err)
	}
	if got.Source != "garmin" {
		t.Errorf("Source = %q, want garmin", got.Source)
	}
	if !got.HasHR {
		t.Errorf("HasHR = false, want true (Garmin .FIT carried HR)")
	}
	if len(got.TimeInZone) == 0 {
		t.Errorf("TimeInZone empty, want zone buckets from the Garmin HR series")
	}
	if got.DecouplingPct == nil {
		t.Errorf("DecouplingPct = nil, want computed (4-sample HR+pace series)")
	}

	raw, err := s.GetActivityStream(activityID)
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
		t.Errorf("stored series.HR = %v, want [140 142 150 152]", ser.HR)
	}
}

// M4: FIT fetch error propagates from FetchAndAnalyze (no Strava degrade path).
func TestFetchAndAnalyzeFITErrorPropagates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh for the FIT runner stub")
	}
	s := newStreamsStore(t)
	if err := s.UpsertActivity(store.Activity{
		ActivityID: 7, Name: "run", Type: "Run",
		StartTime: "2026-06-22T05:00:00Z", DistanceM: 5000, MovingTimeS: 1800, ElapsedTimeS: 1850, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert activity: %v", err)
	}
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'fit boom' 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	e := New(s, garmin.Runner{Python: "/bin/sh", Script: script}, nil)
	if _, err := e.FetchAndAnalyze(context.Background(), 7); err == nil {
		t.Fatal("FetchAndAnalyze err = nil, want propagated FIT error")
	}
}
```
  Also **delete** `backend/internal/streams/strava_test.go` (`git rm`) in this step.

- [ ] **Step 2: Run — expect compile FAIL.** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/streams/ 2>&1 | head -30`. Expected: `not enough arguments in call to New / have (*store.Store, garmin.Runner, []string) / want (*store.Store, *strava.Client, garmin.Runner, []string, int)` (test rewritten, impl not yet changed).

- [ ] **Step 3: Delete `strava.go` and rewrite `engine.go`.** First `git rm backend/internal/streams/strava.go`. Then edit `engine.go`:

  (a) Imports — remove the `strava` line. Final:
```go
import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
)
```
  (b) `Engine` struct (anchor `type Engine struct {`):
```go
// Engine is the DB-loading wrapper around the pure compute functions. It owns the
// analysis cache + recompute-on-zone-change logic and the on-demand/trickle
// fetch path. *Engine satisfies api.Streams and sync.streamFetcher.
type Engine struct {
	store    *store.Store
	runner   garmin.Runner
	extraEnv []string
}
```
  (c) `New` (anchor `func New(s *store.Store, sc *strava.Client, runner garmin.Runner, extraEnv []string, matchToleranceS int) *Engine`):
```go
// New constructs a streams Engine. The garmin runner powers FetchAndAnalyze;
// GetOrComputeAnalysis uses only the store.
func New(s *store.Store, runner garmin.Runner, extraEnv []string) *Engine {
	return &Engine{store: s, runner: runner, extraEnv: extraEnv}
}
```
  (d) `FetchAndAnalyze` (anchor `func (e *Engine) FetchAndAnalyze`):
```go
// FetchAndAnalyze fetches the per-run Garmin .FIT stream if missing, stores it
// gzipped, computes + caches the analysis, and returns it. If a raw stream
// already exists it just (re)computes via GetOrComputeAnalysis.
func (e *Engine) FetchAndAnalyze(ctx context.Context, activityID int64) (StreamAnalysis, error) {
	if has, err := e.store.HasActivityStream(activityID); err != nil {
		return StreamAnalysis{}, err
	} else if has {
		return e.GetOrComputeAnalysis(ctx, activityID)
	}

	gid, _ := e.resolveGarminID(activityID) // identity: gid == activityID
	out, err := e.runner.RunGarminFetchFIT(ctx, gid, activityID, e.extraEnv)
	if err != nil {
		return StreamAnalysis{}, err
	}
	ser := Series{T: out.Series.T, HR: out.Series.HR, V: out.Series.V, Dist: out.Series.Dist}
	source := "garmin"

	gz, err := CompressSeries(ser)
	if err != nil {
		return StreamAnalysis{}, err
	}
	if err := e.store.UpsertActivityStream(store.ActivityStream{
		ActivityID: activityID, Source: source, SeriesGz: gz,
	}); err != nil {
		return StreamAnalysis{}, err
	}
	return e.GetOrComputeAnalysis(ctx, activityID)
}
```
  (e) DELETE `const refreshBuffer` (anchor `const refreshBuffer = 60 * time.Second`) and the entire `accessToken` method (anchor `func (e *Engine) accessToken`), including doc comments.
  (f) Replace `resolveGarminID` (anchor `func (e *Engine) resolveGarminID`) and DELETE `absF` (anchor `func absF`):
```go
// resolveGarminID is identity in M4: activities.activity_id IS the Garmin
// download id. Always (id, true). The dormant match path was removed with
// garmin_activities.
func (e *Engine) resolveGarminID(activityID int64) (int64, bool) {
	return activityID, true
}
```
(No caller of `e.store.GetActivity`/`e.store.FindGarminActivitiesNear` remains in this file; `errors`+`time` stay in use by `GetOrComputeAnalysis`/`computeAndStore`.)

- [ ] **Step 4: Run streams tests — expect PASS.** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/streams/`. Expected: `ok`.

- [ ] **Step 5: Verify garmin comments (done in CORE Task 4).** Confirm `runner.go` `RunGarminFetchFIT` doc + `types.go` `FITStreamOutput.ActivityID` comment are already updated (CORE Task 4 Steps 3-4). If not present, apply them now.

- [ ] **Step 6: Build the changed packages (gofmt + build).** Command: `cd /home/jake/project/help-my-run/backend && gofmt -l internal/streams/ internal/garmin/ && go build ./internal/streams/... ./internal/garmin/...`. Expected: empty gofmt; build succeeds.
> Whole-module `go build ./...` still FAILS here because `main.go`/`sync`/`api`/`agent` still call the old 5-arg `streams.New` — fixed in Tasks 6-7. Gate this task on the two changed packages only.

- [ ] **Step 7: Commit.** `git add -A && git commit` message: `M4 STRIP: streams Garmin .FIT only — identity resolveGarminID, drop Strava client + FromStravaStreams`.

---

### Task 6: Sync — delete SyncStrava + Strava M0 follow-ups; SyncAll Garmin-only; rewire RealSyncer

**Files:**
- Modify: `backend/internal/sync/sync.go` (delete `SyncStrava`/`mapActivity`/`mapLaps`/`strPtr`/`refreshBuffer`; `AllResult` Garmin-only; `SyncAll` drops `client`; remove strava import; package doc)
- Modify: `backend/internal/sync/streams_sync.go` (remove strava import + `*strava.ErrRateLimited` branch + rate-limit guard; remove `errors` import)
- Modify: `backend/internal/agent/syncer.go` (`RealSyncer` drop strava field/import; `NewRealSyncer` drop `sc`; `SyncAll` drop `r.strava`)
- Modify: `backend/internal/store/activities.go` (delete dead `LatestActivityStartTime`; `Split` doc already tweaked in CORE Task 2)
- Test: `backend/internal/sync/sync_test.go`, `backend/internal/sync/streams_sync_test.go`, `backend/internal/agent/syncer_test.go` (rewrite)

- [ ] **Step 1: Rewrite `sync_test.go` (failing test).** Replace the whole file (drop all Strava tests; Garmin-only `SyncAll`):
```go
package sync

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

func TestSyncGarminUpsertsActivitiesAndRecovery(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)

	fixture, err := filepath.Abs(filepath.Join("..", "garmin", "testdata", "worker_output.json"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	script := filepath.Join(t.TempDir(), "stub.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat '"+fixture+"'\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	res := SyncGarmin(context.Background(), s, r, nil)
	if res.Status != "ok" || res.Error != nil {
		t.Fatalf("result = %+v, want ok", res)
	}
	// Fixture: 2 sleep + 1 hrv + 2 bb + 2 rhr + 2 vo2max + 2 activities = 11 upserts.
	if res.Synced != 11 {
		t.Errorf("synced = %d, want 11", res.Synced)
	}

	// Activities now land in the canonical `activities` table (M4 re-key).
	var nAct int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM activities`).Scan(&nAct); err != nil {
		t.Fatalf("count activities: %v", err)
	}
	if nAct != 2 {
		t.Errorf("activities = %d, want 2 (Garmin-ingested)", nAct)
	}
	a, err := s.GetActivity(14820001234)
	if err != nil {
		t.Fatalf("GetActivity: %v", err)
	}
	if a.Type != "running" {
		t.Errorf("activity type = %q, want running", a.Type)
	}

	// raw_json persisted from the worker for recovery.
	var raw string
	_ = s.DB.QueryRow(`SELECT raw_json FROM garmin_sleep WHERE date='2026-06-14'`).Scan(&raw)
	if !strings.Contains(raw, "dailySleepDTO") {
		t.Errorf("sleep raw_json = %q, want it to contain dailySleepDTO", raw)
	}

	sl, _ := s.GetSyncLog("garmin")
	if sl.Status != "ok" || sl.LastSyncedAt == nil {
		t.Errorf("sync_log = %+v, want ok with last_synced_at", sl)
	}
}

func TestSyncGarminError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 're-run worker.py login' 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	res := SyncGarmin(context.Background(), s, r, nil)
	if res.Status != "error" || res.Error == nil {
		t.Fatalf("result = %+v, want error", res)
	}
	if !strings.Contains(*res.Error, "re-run worker.py login") {
		t.Errorf("error = %q, want stderr surfaced", *res.Error)
	}
	sl, _ := s.GetSyncLog("garmin")
	if sl.Status != "error" || sl.Error == nil {
		t.Errorf("sync_log = %+v, want error", sl)
	}
}

func TestSyncAllGarminOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)

	// Garmin worker fails -> AllResult.Garmin is error; no Strava field exists.
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho boom 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	out := SyncAll(context.Background(), s, r, nil, nil)
	if out.Garmin.Status != "error" || out.Garmin.Error == nil {
		t.Errorf("garmin = %+v, want error", out.Garmin)
	}
}

func TestSyncGarminBackfillWindowIs84Days(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)

	dir := t.TempDir()
	argfile := filepath.Join(dir, "args.txt")
	script := filepath.Join(dir, "capture.sh")
	body := "#!/bin/sh\necho \"$@\" > '" + argfile + "'\n" +
		`echo '{"since":"x","until":"x","fetched_at":"x","sleep":[],"hrv":[],"body_battery":[],"rhr":[],"vo2max":[],"activities":[]}'` + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	res := SyncGarmin(context.Background(), s, r, nil)
	if res.Status != "ok" {
		t.Fatalf("status = %q (err=%v), want ok", res.Status, res.Error)
	}

	gotArgs, err := os.ReadFile(argfile)
	if err != nil {
		t.Fatalf("read argfile: %v", err)
	}
	want := time.Now().AddDate(0, 0, -84).Format("2006-01-02")
	if !strings.Contains(string(gotArgs), "--since "+want) {
		t.Errorf("worker args = %q, want --since %s (~12-week backfill)", strings.TrimSpace(string(gotArgs)), want)
	}
}

func TestRunTickerCallsAndStops(t *testing.T) {
	var calls int32
	fn := func(ctx context.Context) { atomic.AddInt32(&calls, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunTicker(ctx, 10*time.Millisecond, fn)
		close(done)
	}()

	time.Sleep(55 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunTicker did not stop within 1s of cancel")
	}

	if n := atomic.LoadInt32(&calls); n < 1 {
		t.Errorf("tick calls = %d, want >= 1", n)
	}
}
```
> NOTE: `TestSyncGarminUpsertsActivitiesAndRecovery` asserts the 2 fixture activities land in `activities` (CORE Task 4 behavior). `activity_type` of `14820001234` is `running` per `worker_output.json`. If the helper/fns differ (`RunTicker` naming, `GetSyncLog` shape), match the existing names.

- [ ] **Step 2: Rewrite `streams_sync_test.go` (failing test).** Drop strava + 429/rate-limit tests; keep budget; add a generic-error stop test:
```go
package sync

import (
	"context"
	"errors"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
)

func newSyncTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/sync.db")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

// fakeFetcher records FetchAndAnalyze calls and can return a generic error.
type fakeFetcher struct {
	calls     []int64
	failAfter int // return an error on the (failAfter+1)-th call; 0 = never
}

func (f *fakeFetcher) FetchAndAnalyze(ctx context.Context, id int64) error {
	if f.failAfter > 0 && len(f.calls) >= f.failAfter {
		return errors.New("fetch failed")
	}
	f.calls = append(f.calls, id)
	return nil
}

func seedRun(t *testing.T, s *store.Store, id int64, st string) {
	t.Helper()
	if err := s.UpsertActivity(store.Activity{ActivityID: id, Name: "r", Type: "Run",
		StartTime: st, DistanceM: 5000, MovingTimeS: 1500, ElapsedTimeS: 1500, RawJSON: "{}"}); err != nil {
		t.Fatalf("seed run %d: %v", id, err)
	}
}

func TestTrickleStreamsRespectsBudget(t *testing.T) {
	s := newSyncTestStore(t)
	now := time.Date(2026, 6, 22, 6, 0, 0, 0, time.UTC)
	for i := int64(1); i <= 5; i++ {
		seedRun(t, s, i, now.AddDate(0, 0, -int(i)).Format(time.RFC3339))
	}
	f := &fakeFetcher{}
	n := TrickleStreams(context.Background(), s, f, 12, 3, now)
	if n != 3 || len(f.calls) != 3 {
		t.Fatalf("fetched = %d / calls %d, want 3 / 3 (budget)", n, len(f.calls))
	}
	log, err := s.GetStreamFetchLog()
	if err != nil {
		t.Fatalf("GetStreamFetchLog: %v", err)
	}
	if log.Status != "ok" || log.LastFetched != 3 || log.TotalFetched != 3 {
		t.Errorf("log = %+v, want ok / last 3 / total 3", log)
	}
}

func TestTrickleStreamsStopsAndRecordsErrorOnFetchFailure(t *testing.T) {
	s := newSyncTestStore(t)
	now := time.Date(2026, 6, 22, 6, 0, 0, 0, time.UTC)
	for i := int64(1); i <= 5; i++ {
		seedRun(t, s, i, now.AddDate(0, 0, -int(i)).Format(time.RFC3339))
	}
	f := &fakeFetcher{failAfter: 2}
	n := TrickleStreams(context.Background(), s, f, 12, 10, now)
	if n != 2 {
		t.Errorf("fetched = %d, want 2 before the error", n)
	}
	log, _ := s.GetStreamFetchLog()
	if log.Status != "error" || log.Error == nil {
		t.Errorf("log = %+v, want error status with error message", log)
	}
}
```
> `TestTrickleStreamsSkipsWhileRateLimited` is **removed** (rate-limit guard deleted per Contract D). If the plan-author keeps the guard, restore that test + the guard.

- [ ] **Step 3: Rewrite `agent/syncer_test.go` (failing test).**
```go
package agent

import (
	"context"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
)

func TestRealSyncerCallsSyncAll(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "rs.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// A worker that produces no valid JSON -> SyncGarmin errors (status "error"),
	// proving the adapter ran SyncAll end-to-end with the bound deps.
	rs := NewRealSyncer(s, garmin.Runner{Python: "/bin/cat", Script: "/dev/null"}, nil)
	res := rs.SyncAll(context.Background())
	if res.Garmin.Status != "error" {
		t.Errorf("garmin status = %q, want error (worker produced no output)", res.Garmin.Status)
	}
}
```

- [ ] **Step 4: Run — expect compile FAIL.** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/sync/ ./internal/agent/ 2>&1 | head -25`. Expected: `not enough arguments in call to SyncAll / want (..., *strava.Client, ...)`, `not enough arguments in call to NewRealSyncer`.

- [ ] **Step 5: Edit `sync.go`.**
  (a) Package doc (anchor `// Package sync orchestrates Strava + Garmin ingestion and records sync_log.`): → `// Package sync orchestrates Garmin ingestion and records sync_log.`
  (b) Imports — remove `"help-my-run/backend/internal/strava"`. Keep `context`, `encoding/json`, `math` (added CORE Task 4), `time`, `garmin`, `store`.
  (c) DELETE `const refreshBuffer` (anchor `const refreshBuffer = 60 * time.Second`) + doc.
  (d) DELETE `SyncStrava` (anchor `func SyncStrava(ctx context.Context, s *store.Store, client *strava.Client) SourceResult {` through `return okResult(s, source, synced)`).
  (e) DELETE `strPtr` (anchor `func strPtr(s string) *string {`), `mapActivity` (anchor `func mapActivity(a strava.SummaryActivity, raw string) store.Activity {`), `mapLaps` (anchor `func mapLaps(activityID int64, laps []strava.Lap) []store.Split {`).
  (f) `AllResult` (anchor `type AllResult struct {`):
```go
// AllResult is the combined sync outcome (the /api/sync body). Garmin-only in M4.
type AllResult struct {
	Garmin SourceResult
}
```
  (g) `SyncAll` (anchor `func SyncAll(ctx context.Context, s *store.Store, client *strava.Client, r garmin.Runner, extraEnv []string, st *StreamTrickle) AllResult {`):
```go
// SyncAll runs the Garmin sync and returns the result. When st is non-nil and the
// Garmin sync succeeds, it trickles a budgeted recent-window stream backfill
// (never erroring the surrounding sync).
func SyncAll(ctx context.Context, s *store.Store, r garmin.Runner, extraEnv []string, st *StreamTrickle) AllResult {
	res := AllResult{Garmin: SyncGarmin(ctx, s, r, extraEnv)}
	if res.Garmin.Status == "ok" && st != nil && st.Fetcher != nil {
		TrickleStreams(ctx, s, st.Fetcher, st.Weeks, st.Budget, time.Now())
	}
	return res
}
```

- [ ] **Step 6: Edit `streams_sync.go`.**
  (a) Imports → remove `"errors"` + `"help-my-run/backend/internal/strava"`:
```go
import (
	"context"
	"time"

	"help-my-run/backend/internal/store"
)
```
  (b) `TrickleStreams` (anchors `const source = "strava"` and `var rl *strava.ErrRateLimited`). Keep `const source = "strava"` (opaque key, Decision #2). Remove the rate-limit top guard block AND the `errors.As(err, &rl)` 429 branch:
```go
// TrickleStreams fetches up to budget recent-window (last `weeks`) runs lacking a
// stream, most-recent-first, recording resumable progress in stream_fetch_log.
// On any fetch error it stops and records "error". Returns the count fetched.
// Never errors the surrounding Garmin sync.
func TrickleStreams(ctx context.Context, s *store.Store, f streamFetcher, weeks, budget int, now time.Time) int {
	const source = "strava" // opaque single-row stream_fetch_log key (M4)

	sinceISO := now.AddDate(0, 0, -7*weeks).UTC().Format(time.RFC3339)
	ids, err := s.ListRecentRunsWithoutStream(sinceISO, budget)
	if err != nil {
		recordTrickle(s, store.StreamFetchLog{Source: source, Status: "error",
			Error: strptrErr(err), LastRunAt: nowPtr(now)}, prevTotal(s))
		return 0
	}

	fetched := 0
	for _, id := range ids {
		if err := f.FetchAndAnalyze(ctx, id); err != nil {
			recordTrickle(s, store.StreamFetchLog{Source: source, Status: "error",
				Error: strptrErr(err), LastFetched: int64(fetched), LastRunAt: nowPtr(now)},
				prevTotal(s)+int64(fetched))
			return fetched
		}
		fetched++
	}

	recordTrickle(s, store.StreamFetchLog{Source: source, Status: "ok",
		LastFetched: int64(fetched), LastRunAt: nowPtr(now)}, prevTotal(s)+int64(fetched))
	return fetched
}
```
(`recordTrickle`, `prevTotal`, `nowPtr`, `strptrErr`, `streamFetcher`, `StreamTrickle` unchanged. If the existing body differs structurally, preserve its helpers and only remove the strava import + the two rate-limit blocks.)

- [ ] **Step 7: Edit `agent/syncer.go`.**
```go
package agent

import (
	"context"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	syncpkg "help-my-run/backend/internal/sync"
)

// RealSyncer binds the sync dependencies and adapts sync.SyncAll to the Syncer
// seam used by the Agent.
type RealSyncer struct {
	store    *store.Store
	runner   garmin.Runner
	extraEnv []string
}

// NewRealSyncer constructs the production Syncer.
func NewRealSyncer(s *store.Store, r garmin.Runner, extraEnv []string) *RealSyncer {
	return &RealSyncer{store: s, runner: r, extraEnv: extraEnv}
}

// SyncAll runs the Garmin sync with the bound deps. No stream trickle in the
// daily agent loop (recent-window stream backfill rides the server's periodic sync).
func (r *RealSyncer) SyncAll(ctx context.Context) syncpkg.AllResult {
	return syncpkg.SyncAll(ctx, r.store, r.runner, r.extraEnv, nil)
}
```

- [ ] **Step 8: Delete dead `LatestActivityStartTime` in `store/activities.go`.** DELETE the whole `LatestActivityStartTime` method (anchor `func (s *Store) LatestActivityStartTime()`) + doc — its only caller was the now-deleted `SyncStrava`. (`Split` doc already tweaked in CORE Task 2.) Keep `database/sql`/`time` imports (still used by `GetActivity`/`UpsertActivity`). Re-key `store/activities_test.go` already done in CORE Task 2 (the `LatestActivityStartTime` tests still pass against the method until now — DELETE those two tests, `TestLatestActivityStartTimeEmpty` + `TestLatestActivityStartTime`, in this step since the method is removed).

- [ ] **Step 9: Run sync + agent + store tests — expect PASS.** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/sync/ ./internal/agent/ ./internal/store/`. Expected: `ok` for all three.
> Whole-module `go build ./...` still fails until Task 7 fixes `main.go`/`api`; gate this task on the three packages + `gofmt -l internal/sync/ internal/agent/ internal/store/` empty.

- [ ] **Step 10: Commit.** `git add -A && git commit` message: `M4 STRIP: sync Garmin-only — delete SyncStrava/mapActivity/mapLaps, SyncAll drops strava client, RealSyncer rewired, delete LatestActivityStartTime`.

---

### Task 7: API + main wiring — remove Strava OAuth routes/handlers, Garmin-only status/sync DTO, SyncFunc 3-tuple

> This is the first point the WHOLE tree compiles green (closes the last 5-arg `streams.New` / 6-tuple `SyncFunc` / `SyncAll(client)` gaps). Includes the config field removal so `cfg.GarminMatchToleranceS` and the 3 `STRAVA_*` consumers all disappear together.

**Files:**
- Modify: `backend/internal/api/router.go`, `handlers.go`, `dto.go`, `stream_handlers.go`
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/internal/config/config.go` (remove 3 `STRAVA_*` + `GarminMatchToleranceS`)
- Test: `backend/internal/api/{handlers_test.go, stream_handlers_test.go, image_security_test.go, chat_handlers_test.go, progress_handlers_test.go, m2_handlers_test.go}` (Deps drop `Strava`; `SyncFunc` 3-tuple; drop Strava assertions)

- [ ] **Step 1: Edit `config.go` (pure field removal).** Remove the 3 STRAVA lines (anchors `StravaClientID`, `StravaClientSecret`, `StravaRedirectURL`) and `GarminMatchToleranceS` (anchor `GarminMatchToleranceS int \`envconfig:"GARMIN_MATCH_TOLERANCE_S" default:"120"\``) + its `// M3.2.1:` comment. After edit, the struct's first field is `APIToken string ... required:"true"` (the only required var). Verify: `cd /home/jake/project/help-my-run/backend && go build ./internal/config/` → no output.

- [ ] **Step 2: Rewrite `dto.go` Strava-only types.**
  - DELETE `type stravaStatus` (anchor `type stravaStatus struct {`).
  - `statusResp` (anchor `type statusResp struct {`):
```go
type statusResp struct {
	Garmin sourceStatus `json:"garmin"`
	Counts statusCounts `json:"counts"`
}
```
  - DELETE `type connectResp` (anchor `type connectResp struct {`).
  - `syncResp` (anchor `type syncResp struct {`):
```go
type syncResp struct {
	Garmin syncSourceResult `json:"garmin"`
}
```
  (`sourceStatus`, `statusCounts`, `syncSourceResult` kept. `activityDTO` already re-keyed in CORE Task 2.)

- [ ] **Step 3: Rewrite `router.go`.**
  - Remove import `"help-my-run/backend/internal/strava"`.
  - `SyncFunc` typedef (anchor `type SyncFunc func(ctx context.Context) (string, int, *string, string, int, *string)`):
```go
// SyncFunc runs the Garmin sync and returns the flattened result:
// (status, synced, err). Wiring (main.go) adapts the sync package to this
// signature so the api package does not import sync (avoids an import cycle).
type SyncFunc func(ctx context.Context) (string, int, *string)
```
  - `Deps` struct (anchor `Strava   *strava.Client`): delete that field line.
  - Remove route lines (anchors `r.Get("/api/strava/callback", h.stravaCallback)` and `r.Get("/api/strava/connect", h.stravaConnect)`).

- [ ] **Step 4: Rewrite `handlers.go`.**
  - Imports (anchor block): remove `"crypto/rand"`, `"encoding/hex"`, `"help-my-run/backend/internal/store"`. Final:
```go
import (
	"net/http"
	"strconv"
)
```
  - `status()` (anchor `func (h *handlers) status`):
```go
func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	s := h.d.Store

	recoveryDays, _ := s.CountRecoveryDays()

	var activitiesCount int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activities`).Scan(&activitiesCount)

	garminLog, _ := s.GetSyncLog("garmin")
	// "connected" = the last worker invocation authenticated successfully
	// (spec §3.4/§7), derived from the garmin sync_log status — NOT a
	// recovery-data-presence proxy. A fresh/never-run DB has status "never"
	// (00001 seed) -> connected:false; a failed login -> status "error" ->
	// connected:false; a successful sync -> "ok" -> connected:true.
	garminConn := garminLog.Status == "ok"

	resp := statusResp{
		Garmin: sourceStatus{
			Connected:    garminConn,
			LastSyncedAt: garminLog.LastSyncedAt,
			LastRunAt:    garminLog.LastRunAt,
			Status:       garminLog.Status,
			Error:        garminLog.Error,
		},
		Counts: statusCounts{Activities: activitiesCount, RecoveryDays: recoveryDays},
	}
	writeJSON(w, http.StatusOK, resp)
}
```
  (Match the existing `sourceStatus`/`statusCounts` field names — adjust if the current DTO uses different field identifiers. `s.GetSyncLog("garmin")` already loads the row here; `recoveryDays` is still emitted in `Counts`, only its use as the connected proxy is dropped.)
  - DELETE `stravaConnect` (anchor `func (h *handlers) stravaConnect`), `randomState` (anchor `func randomState`), `stravaCallback` (anchor `func (h *handlers) stravaCallback`), `writeHTML` (anchor `func writeHTML`).
  - `sync()` (anchor `func (h *handlers) sync`):
```go
func (h *handlers) sync(w http.ResponseWriter, r *http.Request) {
	gs, gsn, gErr := h.d.SyncFunc(r.Context())
	writeJSON(w, http.StatusOK, syncResp{
		Garmin: syncSourceResult{Status: gs, Synced: gsn, Error: gErr},
	})
}
```
  (`activities()` already re-keyed in CORE Task 2.)

- [ ] **Step 5: Rewrite `stream_handlers.go`.**
  - Imports: remove `"help-my-run/backend/internal/strava"`. Keep `errors`, `store`, `streams`, `chi`, `net/http`, `strconv`.
  - `fetchStream` (anchor `// 429 on Strava rate limit; 500 otherwise.` + `var rl *strava.ErrRateLimited`):
```go
// fetchStream serves POST /api/activities/{id}/stream/fetch — fetch-if-missing,
// compute, cache, return. 500 on fetch error.
func (h *handlers) fetchStream(w http.ResponseWriter, r *http.Request) {
	id, ok := parseActivityID(w, r)
	if !ok {
		return
	}
	a, err := h.d.Streams.FetchAndAnalyze(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, streamAnalysisToDTO(a))
}
```
  (Match existing helper names `parseActivityID`/`streamAnalysisToDTO`.)

- [ ] **Step 6: Rewrite `main.go` (`Wire` + `main`).**
  - Remove import `"help-my-run/backend/internal/strava"`.
  - `App` struct: delete `Strava   *strava.Client` field.
  - `Wire`: delete `stravaClient := strava.New(cfg.StravaClientID, cfg.StravaClientSecret, cfg.StravaRedirectURL)`; `streams.New(s, stravaClient, runner, extraEnv, cfg.GarminMatchToleranceS)` → `streams.New(s, runner, extraEnv)`; `syncFunc` adapter (anchor `res := syncpkg.SyncAll(ctx, s, stravaClient, runner, extraEnv, streamTrickle(cfg, streamsEngine))`):
```go
	syncFunc := func(ctx context.Context) (string, int, *string) {
		res := syncpkg.SyncAll(ctx, s, runner, extraEnv, streamTrickle(cfg, streamsEngine))
		return res.Garmin.Status, res.Garmin.Synced, res.Garmin.Error
	}
```
    `agent.NewRealSyncer(s, stravaClient, runner, extraEnv)` → `agent.NewRealSyncer(s, runner, extraEnv)`; remove `Strava:   stravaClient,` from `api.Deps{...}` and from `App{...}`; `Wire` doc `constructs the Strava client and Garmin runner` → `constructs the Garmin runner`.
  - `main()`: remove `stravaClient := app.Strava`; `syncOnce` (anchor `res := syncpkg.SyncAll(c, app.Store, stravaClient, runner, extraEnv, streamTrickle(cfg, app.Streams))`):
```go
	syncOnce := func(c context.Context) {
		res := syncpkg.SyncAll(c, app.Store, runner, extraEnv, streamTrickle(cfg, app.Streams))
		log.Printf("sync: garmin=%s/%d", res.Garmin.Status, res.Garmin.Synced)
	}
```
  (`runSyncOnBoot`, `RunTicker`, scheduler wiring, `garminEnv` unchanged.)

- [ ] **Step 7: Build the whole module — first whole-tree green.** Command: `cd /home/jake/project/help-my-run/backend && go build ./...`. Expected: no output. If any non-test file still references `strava`, it errors here — fix before proceeding.

- [ ] **Step 8: Rewrite the api test Deps + Strava assertions.** Five+ api test files set `Deps{... Strava: ..., SyncFunc: 6-tuple ...}`:
  - `handlers_test.go`: remove strava import; drop `Strava:` from each `Deps{...}`; change every `SyncFunc` to the 3-tuple form `func(ctx context.Context) (string, int, *string) { return ... }`. DELETE the Strava status/token assertions (the `SaveStravaTokens` seed + `body.Strava.*` checks) — rewrite the status test to assert only Garmin + counts. DELETE the connect test (`/api/strava/connect`/`connectResp`/`AuthorizeURL`), `TestStravaCallbackExchangesAndPersists`, `TestStravaCallbackAccessDenied`. Re-key `activities()` test literals if not already (`StravaID:`→`ActivityID:`, `.StravaID`→`.ActivityID`). Rewrite the sync test to assert `body.Garmin.Status=="ok"`/`Synced` via a 3-tuple SyncFunc.
    Status assertion (M4: `connected` is derived from the garmin `sync_log` status, NOT recovery-data presence — so seed a SUCCESSFUL garmin sync_log row; optionally seed a recovery day too for the non-zero `recovery_days` count):
```go
	now := "2026-06-23T05:00:00Z"
	_ = s.UpdateSyncLog(store.SyncLog{
		Source: "garmin", LastSyncedAt: &now, LastRunAt: &now, Status: "ok", Error: nil,
	})
	_ = s.UpsertSleep(store.SleepRow{Date: "2026-06-14", RawJSON: "{}"}) // non-zero recovery_days count
	// ... fetch /api/status ...
	if !body.Garmin.Connected {
		t.Errorf("garmin = %+v, want connected (sync_log status ok)", body.Garmin)
	}
	if body.Garmin.Status != "ok" {
		t.Errorf("garmin.status = %q, want ok", body.Garmin.Status)
	}
```
  (`store` import stays in the test for `store.SyncLog`/`store.SleepRow`; that is the TEST file, unaffected by trimming the `store` import from the handlers production file. Match `UpdateSyncLog`/`SyncLog` field names to the existing store API — verify with the helpers `sync.go`/`okResult` use.)
    Removed-routes 404 test (use the existing server-builder helper — verify its name):
```go
func TestRemovedStravaRoutesReturn404(t *testing.T) {
	h := newServer(t)
	for _, p := range []string{"/api/strava/connect", "/api/strava/callback"} {
		rec := do(t, h, http.MethodGet, p, testToken)
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404 (route removed)", p, rec.Code)
		}
	}
}
```
  - `stream_handlers_test.go`: remove strava import; drop `Strava:` from the test `Deps`; 3-tuple `SyncFunc`; DELETE `TestFetchStreamRateLimited` (anchor `fs := &fakeStreams{fetchErr: &strava.ErrRateLimited{}}`) — keep/use `TestFetchStreamOtherError` (500 path). `fakeStreams.fetchErr` is already a plain `error` — no change.
  - `image_security_test.go`, `chat_handlers_test.go`, `progress_handlers_test.go`, `m2_handlers_test.go`: remove strava import where present; drop the `Strava:` Deps line; 3-tuple `SyncFunc` `func(ctx context.Context) (string, int, *string) { return "ok", 0, nil }`.
  - `config_test.go`: the Strava-var + `GarminMatchToleranceS` test edits are done in CONFIG Task 9 — if Task 9 has not yet run, `go test ./internal/config/` may fail until it does; sequence Task 9 around here or run `go build ./...` only as this task's gate.

- [ ] **Step 9: Run the full api suite — expect PASS.** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/api/`. Expected: `ok`.

- [ ] **Step 10: Full module build + gofmt.** Command: `cd /home/jake/project/help-my-run/backend && gofmt -l . && go build ./...`. Expected: `gofmt -l .` empty; build silent.

- [ ] **Step 11: Commit.** `git add -A && git commit` message: `M4 STRIP: api Garmin-only — remove Strava OAuth routes/handlers, status/sync DTO, SyncFunc 3-tuple; main wiring + config field removal`.

---

### Task 8: Delete the `strava` package + dead store fns/tables; build/grep gate

**Files:**
- Delete: `backend/internal/strava/` (whole dir: `client.go`, `types.go`, `client_test.go`, `testdata/*.json`, `testdata/.gitkeep`)
- Delete: `backend/internal/store/oauth_state.go`
- Modify: `backend/internal/store/tokens.go` (delete `StravaTokens`/`GetStravaTokens`/`SaveStravaTokens`; KEEP `ErrNotFound`; trim imports)
- Modify: `backend/internal/store/garmin.go` (delete `GarminActivityRow`/`UpsertGarminActivity`/`GarminActivityCandidate`/`FindGarminActivitiesNear`)
- Optional: ADD `backend/internal/store/migration_00009_test.go` (if Task 1's `TestMigration00009RekeysAndDrops` does not already cover the drops — it does; this is verify-only)
- Test: `go build ./... && go test ./...`

- [ ] **Step 1: Confirm no remaining non-deleted-file references.** Command:
```
cd /home/jake/project/help-my-run/backend && grep -rn 'internal/strava' --include='*.go' . | grep -v 'internal/strava/' ; echo "EXIT:$?"
```
Expected: no output and `EXIT:1`. Then confirm the dead store fns are unreferenced:
```
grep -rn 'GetStravaTokens\|SaveStravaTokens\|StravaTokens\|SaveOAuthState\|ConsumeOAuthState\|UpsertGarminActivity\|FindGarminActivitiesNear\|GarminActivityRow\|GarminActivityCandidate' --include='*.go' . | grep -v '_test.go' | grep -v 'store/tokens.go\|store/oauth_state.go\|store/garmin.go'
```
Expected: no output. Any `_test.go` hit must already be gone from Tasks 5-7 (else it FAILS to compile in Step 6 — fix the test).

- [ ] **Step 2: Delete the `strava` package + `oauth_state.go`.** (`oauth_state_test.go` was already `git rm`'d in CORE Task 2 Step 4 — only the production file remains to delete here.) Command:
```
cd /home/jake/project/help-my-run/backend && git rm -r internal/strava && git rm internal/store/oauth_state.go
```

- [ ] **Step 3: Trim `store/tokens.go` to just `ErrNotFound`.** Replace the whole file:
```go
package store

import "errors"

// ErrNotFound is returned by getters when no matching row exists.
var ErrNotFound = errors.New("store: not found")
```

- [ ] **Step 4: Delete the 4 dead Garmin-activity store symbols from `store/garmin.go`.** DELETE (entire blocks, by anchor): `type GarminActivityRow struct` (anchor `// GarminActivityRow maps to garmin_activities.`), `func (s *Store) UpsertGarminActivity`, `type GarminActivityCandidate struct` (anchor `// GarminActivityCandidate is one run-type garmin_activities row`), `func (s *Store) FindGarminActivitiesNear`. KEEP all sleep/hrv/bb/rhr/vo2max/recovery fns. Trim any now-unused imports (`math`/`time`) if they were only used by `FindGarminActivitiesNear` — verify with `go build`.

- [ ] **Step 5: Verify migration 00009 drops are present (verify-only).** Command:
```
cd /home/jake/project/help-my-run && grep -c 'DROP TABLE' backend/internal/store/migrations/00009_m4_garmin_only.sql
```
Expected: `3` (Task 1 wrote the full file with drops). If `0`, append the drops to Up + the recreate to Down per **Contract A**. The 00009 drop/round-trip coverage is `TestMigration00009RekeysAndDrops` (Task 1) — no new test needed.

- [ ] **Step 6: Build + full test suite — expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go build ./... && go test ./...
```
Expected: build silent; every package `ok` (no `FAIL`, no `[build failed]`).

- [ ] **Step 7: THE BUILD/GREP GATE — `grep -rn internal/strava backend/` returns nothing.** Command:
```
cd /home/jake/project/help-my-run && grep -rn 'internal/strava' backend/ ; echo "GREP_EXIT:$?"
```
Expected EXACT: `GREP_EXIT:1` (zero lines). Then the combined gate:
```
cd /home/jake/project/help-my-run/backend && gofmt -l . ; go build ./... && go test ./...
```
Expected: `gofmt -l .` empty; build silent; all `ok`.
> The `stream_fetch_log.source='strava'` literal (Decision #2) survives in `store/streams.go` + `streams_sync.go` as an opaque DB key — NOT an `internal/strava` import, so this gate passes. A separate `grep -rni strava backend/internal/store/streams.go` would still hit the literal — expected and acceptable.

- [ ] **Step 8: Commit.** `git add -A && git commit` message: `M4 STRIP: delete strava package + dead store fns/tables; internal/strava grep clean`.

---

### Task 9: Remove STRAVA_* + GARMIN_MATCH_TOLERANCE_S from config_test (TDD)

> The `config.go` field removal itself is in Task 7 Step 1 (so the whole-module build stays green there). This task brings `config_test.go` in line. If you prefer one commit, fold this test edit into Task 7. Either way, `go test ./internal/config/` must be green before the suite gate.

**Files:**
- Modify: `backend/internal/config/config_test.go`
- Test: `go test ./internal/config/`

- [ ] **Step 1: Update `config_test.go` to the Garmin-only contract.**
  (a) In `setEnv`'s hermetic `all` slice, drop the 3 Strava keys:
```go
	all := []string{
		"API_TOKEN", "DB_PATH", "PORT",
		"GARMIN_EMAIL", "GARMIN_PASSWORD", "GARMIN_TOKENSTORE",
		"PYTHON_BIN", "WORKER_SCRIPT", "ANTHROPIC_API_KEY",
		"CLAUDE_BIN", "CLAUDE_MODEL", "IMAGE_DIR",
		"STREAM_RECENT_WEEKS", "STREAM_FETCH_BUDGET",
		"CHAT_HISTORY_TURNS",
	}
```
  (b) `requiredEnv` → only `API_TOKEN`:
```go
func requiredEnv() map[string]string {
	return map[string]string{
		"API_TOKEN": "tok",
	}
}
```
  (c) In `TestLoadDefaults`, delete the `StravaClientID` assertion (the `if cfg.StravaClientID != "123456" {...}` block).
  (d) In `TestM2ConfigDefaults`, drop the 3 Strava `t.Setenv` lines (keep `t.Setenv("API_TOKEN", "tok")`).
  (e) In `TestM2ConfigOverrides`, drop the 3 Strava `t.Setenv` lines.
  (f) DELETE `TestLoadGarminMatchToleranceDefault` and `TestLoadGarminMatchToleranceOverride` in full.
  (g) Keep `import "os"` (still used by `setEnv`'s `os.Unsetenv`) and `envconfig` (used by `TestM2Config*`).

- [ ] **Step 2: Run — expect PASS (config.go fields already removed in Task 7).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/config/`. Expected: `ok`.
> If running BEFORE Task 7's config.go edit, the test fails on `required key STRAVA_CLIENT_ID missing` (red) — then Task 7 Step 1 makes it green. Sequence accordingly.

- [ ] **Step 3: gofmt + commit (if not folded into Task 7).** Command: `cd /home/jake/project/help-my-run/backend && gofmt -l internal/config/`. Expected: empty. Commit: `M4: config_test Garmin-only (drop STRAVA_* + GARMIN_MATCH_TOLERANCE_S)`.

---

### Task 10: Strip STRAVA_* + GARMIN_MATCH_TOLERANCE_S from .env.example

> Pure docs edit — no build gate. Final key set per **Contract F**.

**Files:** Modify `.env.example`.

- [ ] **Step 1: Remove the Strava OAuth block** (anchor `# --- Strava OAuth (required) ---` through `STRAVA_REDIRECT_URL=...`):
```
# --- Strava OAuth (required) ---
# Create a Strava API app at https://www.strava.com/settings/api
STRAVA_CLIENT_ID=123456
STRAVA_CLIENT_SECRET=replace-with-your-strava-client-secret
# Must EXACTLY match the callback registered in the Strava app settings
# and the redirect_uri the backend builds. Points at /api/strava/callback.
STRAVA_REDIRECT_URL=http://localhost:8080/api/strava/callback

```

- [ ] **Step 2: Remove the GARMIN_MATCH_TOLERANCE_S line + comment:**
```
# M3.2.1: Garmin .FIT fallback start-time match tolerance (seconds).
GARMIN_MATCH_TOLERANCE_S=120
```
  (Keep `GARMIN_EMAIL`/`GARMIN_PASSWORD`/`GARMIN_TOKENSTORE`.)

- [ ] **Step 3: Verify.** Command: `grep -n "STRAVA" /home/jake/project/help-my-run/.env.example; grep -n "GARMIN_MATCH_TOLERANCE_S" /home/jake/project/help-my-run/.env.example`. Expected: no output. Confirm notes retained: `grep -n "LEAVE THIS UNSET" /home/jake/project/help-my-run/.env.example` matches; `grep -n "ABSOLUTE" /home/jake/project/help-my-run/.env.example` matches.

- [ ] **Step 4: Commit.** `M4: remove STRAVA_* + GARMIN_MATCH_TOLERANCE_S from .env.example`.

---

### Task 11: Garmin-only README rewrite

> Pure docs edit — no build gate. Keep ANTHROPIC-key-unset + absolute-path callouts.

**Files:** Modify `README.md` (anchors at lines 3, 13, 28, 84, 100).

- [ ] **Step 1: Line 3 intro — drop Strava.** Replace:
```
A self-hostable, single-user AI running coach. It pulls your runs from **Strava** and your recovery data (sleep, HRV, Body Battery, resting HR) from **Garmin Connect** into a local database, then (in a later milestone) uses Claude to coach you. M0 delivers the data foundation: connect Strava, log in to Garmin once, sync, and view your runs + recovery in a small Expo app.
```
with:
```
A self-hostable, single-user AI running coach. It pulls your runs **and** your recovery data (sleep, HRV, Body Battery, resting HR) from **Garmin Connect** into a local database, then uses Claude to coach you. It delivers the data foundation: log in to Garmin once, sync, and view your runs + recovery in a small Expo app.
```

- [ ] **Step 2: Line 13 prerequisite — delete the Strava-app bullet** (anchor `- **A Strava API application.**` through the end of that bullet). Keep the Garmin-account, No-Anthropic-key, Go/Python/Node bullets, and the absolute-path callout.

- [ ] **Step 3: Line 28 setup comment — re-target env vars.** Replace:
```
# edit .env and fill in STRAVA_CLIENT_ID, STRAVA_CLIENT_SECRET, STRAVA_REDIRECT_URL,
# API_TOKEN, GARMIN_EMAIL, GARMIN_PASSWORD (and any optional overrides)
```
with:
```
# edit .env and fill in API_TOKEN, GARMIN_EMAIL, GARMIN_PASSWORD, and the absolute
# PYTHON_BIN / WORKER_SCRIPT / DB_PATH / IMAGE_DIR paths (and any optional overrides)
```

- [ ] **Step 4: Line 84 — reframe app connection as Sync.** Replace:
```
In the app's Settings screen, enter the backend URL (e.g. `http://<your-LAN-ip>:8080`) and your `API_TOKEN`, then connect Strava.
```
with:
```
In the app's Settings screen, enter the backend URL (e.g. `http://<your-LAN-ip>:8080`) and your `API_TOKEN`, then tap **Sync now**. (Garmin connection is the one-time `make garmin-login` above; Settings shows the Garmin connection status.)
```

- [ ] **Step 5: Line 100 security note — drop "Strava secret".** Replace:
```
All secrets live in `.env`, which is **gitignored**. Never commit credentials (Strava secret, API token, Garmin password) or your Garmin token directory. Review `.gitignore` before pushing.
```
with:
```
All secrets live in `.env`, which is **gitignored**. Never commit credentials (API token, Garmin password) or your Garmin token directory. Review `.gitignore` before pushing.
```

- [ ] **Step 6: Verify.** Command: `grep -ni "strava" /home/jake/project/help-my-run/README.md`. Expected: no output. Confirm: `grep -ni "Leave .ANTHROPIC_API_KEY. UNSET" /home/jake/project/help-my-run/README.md` matches; `grep -n "Use absolute paths" /home/jake/project/help-my-run/README.md` matches.

- [ ] **Step 7: Commit.** `M4: README Garmin-only (remove Strava prerequisites/steps)`.

---

### Task 12: App — Garmin-only types/hooks/screens + jest tests

> ZERO Go-build coupling — gates `jest` + `tsc` only. Land the sub-steps as a cohesive sequence (types → hooks → screens → tests) so `tsc`/`jest` stay green; gate once at the end. The DTO field names the app expects (`activity_id`, Garmin-only `status`/`sync`) match Task 7's `dto.go`.

**Files:**
- Modify: `app/src/api/types.ts`, `app/src/api/hooks.ts`, `app/app/settings.tsx`, `app/app/index.tsx`
- Test: `app/app/__tests__/{settings,index,run-detail}.test.tsx`, `app/src/api/__tests__/{types,client,hooks,hooks-streams}.test.ts(x)`

- [ ] **Step 1: `types.ts`.**
  - `Status` (anchor `strava: SourceStatus & { athlete_id: number };`):
```ts
export interface Status {
  garmin: SourceStatus;
  counts: { activities: number; recovery_days: number };
}
```
  - DELETE `export interface ConnectResponse { authorizeUrl: string; }` (anchor `export interface ConnectResponse`).
  - `SyncResponse` (anchor `export interface SyncResponse`): → `export interface SyncResponse { garmin: SyncSourceResult; }`.
  - `Activity` (anchor `strava_id: number;`): → `activity_id: number;`.
  - `StreamAnalysis.source` (anchor `source: 'strava' | 'garmin' | '';`): → `source: 'garmin' | '';`.
  - Verify: `grep -ni "strava" /home/jake/project/help-my-run/app/src/api/types.ts` → no output.

- [ ] **Step 2: `hooks.ts`.**
  - Remove `import * as WebBrowser from 'expo-web-browser';` (anchor).
  - Remove `ConnectResponse,` from the `import type { ... } from './types';` block.
  - DELETE the `useConnectStrava` hook in full (anchor `export function useConnectStrava`). Keep `Status` in the type import (used by `useStatus`); keep all other hooks.
  - Verify: `grep -nE "strava|WebBrowser|ConnectResponse" /home/jake/project/help-my-run/app/src/api/hooks.ts` → no output.

- [ ] **Step 3: `settings.tsx`.**
  - Import (anchor `import { useStatus, useSync, useConnectStrava, useProfile, useUpdateProfile }`): drop `useConnectStrava`.
  - Remove `const connectStrava = useConnectStrava();` and `const stravaConnected = status.data?.strava.connected ?? false;`.
  - Remove the entire Strava section JSX (anchor `<Text style={styles.heading}>Strava</Text>` through the closing `</Pressable>` with `testID="btn-strava-connect"` — heading, `testID="strava-status"`, `testID="btn-strava-connect"`). Keep the Garmin section + Sync.
  - `sync-result` line (anchor `Strava: {sync.data.strava.status} ({sync.data.strava.synced}) · Garmin:`):
```tsx
        <Text testID="sync-result" style={styles.statusLine}>
          Garmin: {sync.data.garmin.status} ({sync.data.garmin.synced})
        </Text>
```
  - Verify: `grep -ni "strava" /home/jake/project/help-my-run/app/app/settings.tsx` → no output.

- [ ] **Step 4: `index.tsx`.**
  - Remove `const strava = status.data?.strava;` (keep `const garmin = status.data?.garmin;`).
  - Remove the Strava connection line (anchor `<Text testID="home-strava-status" ...>` block). Keep `home-garmin-status`.
  - `keyExtractor` (anchor `(item: Activity) => String(item.strava_id)`) → `String(item.activity_id)`.
  - `Link` params (anchor `params: { id: String(item.strava_id) }`) → `String(item.activity_id)`.
  - `testID` (anchor `` testID={`run-row-${item.strava_id}`} ``) → `` run-row-${item.activity_id} ``.
  - Verify: `grep -ni "strava" /home/jake/project/help-my-run/app/app/index.tsx` → no output.

- [ ] **Step 5: Rewrite/re-key app jest tests.**
  - `settings.test.tsx`: remove `const mockConnectMutate = jest.fn();`; drop the `strava` member from the `useStatus` mock data; remove the `useConnectStrava: () => (...)` mock line; DELETE the two Strava-only tests ("starts Strava connect when Connect is pressed", "shows the Strava connected state"). Keep render-prefill/save/Garmin-not-connected/sync/daily-coach tests.
  - `index.test.tsx`: drop the `strava` member from `statusData`; re-key both `activitiesData` items `strava_id`→`activity_id` (anchors `strava_id: 14820001234`, `strava_id: 14820009999`); rewrite "renders connection status for both sources" → "renders the Garmin connection status" asserting only `home-garmin-status`. The `run-row-14820001234`/`run-row-14820009999` testID assertions stay valid (same numbers via `activity_id`).
  - `run-detail.test.tsx`: top-level `analysis` fixture `source: 'strava'`→`source: 'garmin'`; rewrite "does NOT show the source badge when source is strava" → "...when source is empty" with `source: ''`.
  - `types.test.ts`: remove `ConnectResponse,` import; rewrite the `Status` test (no `strava`; assert `status.garmin.connected`/`status.garmin.status`/`status.counts.recovery_days`); DELETE the `ConnectResponse` test; rewrite the `SyncResponse` test (Garmin-only); re-key the `Activity` test (`strava_id`→`activity_id`, `resp.activities[0].strava_id`→`.activity_id`). If `noUnusedLocals` flags `SyncSourceResult`, remove it from the import too (resolve at the tsc gate, Step 6).
  - `hooks.test.tsx`: drop `strava` from `useStatus` + `useSync` mock `data`.
  - `hooks-streams.test.tsx`: fixture `source: 'strava'`→`source: 'garmin'`.
  - `client.test.ts`: re-key the `/api/sync` example shape `strava`→`garmin` (both the mock `json` and the assertions/generic type).
  - Verify: `grep -rni "strava" /home/jake/project/help-my-run/app/app/__tests__ /home/jake/project/help-my-run/app/src/api/__tests__` → no output.

- [ ] **Step 6: App build-hygiene gate (jest + tsc + grep).** Commands:
```
cd /home/jake/project/help-my-run/app && npm test
cd /home/jake/project/help-my-run/app && npx tsc --noEmit
grep -rni strava /home/jake/project/help-my-run/app/src /home/jake/project/help-my-run/app/app 2>/dev/null
```
Expected: all jest suites pass; `tsc --noEmit` no errors (if it flags an unused `SyncSourceResult` import in `types.test.ts`, remove it and re-run); grep prints **no output** (satisfies spec §8 app gate `grep -rni strava app/src app/app = 0`).

- [ ] **Step 7: Commit.** `M4: app Garmin-only (drop Connect-Strava, re-key activity_id, narrow stream source)`.

---

### Task 13: Final full-suite + build-hygiene gate (whole repo)

> Verification only — confirms every spec §8 gate after all tasks land.

- [ ] **Step 1: Backend grep + build + test + fmt.** Commands:
```
cd /home/jake/project/help-my-run && grep -rn 'internal/strava' backend/ ; echo "GREP_EXIT:$?"
cd /home/jake/project/help-my-run/backend && gofmt -l . ; go build ./... && go test ./...
```
Expected: `GREP_EXIT:1` (no `internal/strava`); `gofmt -l .` empty; build silent; all packages `ok`.

- [ ] **Step 2: Worker pytest.** Command: `cd /home/jake/project/help-my-run/garmin-worker && python3 -m pytest -q`. Expected: all PASS.

- [ ] **Step 3: App jest + tsc + grep.** Commands:
```
cd /home/jake/project/help-my-run/app && npm test && npx tsc --noEmit
grep -rni strava /home/jake/project/help-my-run/app/src /home/jake/project/help-my-run/app/app 2>/dev/null
```
Expected: jest green; tsc clean; grep no output.

- [ ] **Step 4: Manual (operator).** `make garmin-login` (MFA-aware) → start server → `/api/sync` (or "Sync now") → assert real runs land in `activities` (`/api/activities`), removed routes `GET /api/strava/connect` + `/api/strava/callback` return 404, `/api/status` is Garmin-only, and Progress / streams (time-in-zone, decoupling) / a weekly plan / chat all work on real Garmin data.

---

## Definition of Done

Each M4 success criterion (spec §3) maps to its task(s) + the build-hygiene gates:

| # | Spec §3 criterion | Tasks | Verification |
|---|---|---|---|
| 1 | Runs come from Garmin, landing in `activities`; metrics/progress/streams/coach/chat unchanged | Task 1 (migration 00009 re-key), Task 2 (re-key struct + all consumers), Task 3 (worker enrich), Task 4 (`SyncGarmin`→`UpsertActivity`) | `TestSyncGarminUpsertsActivitiesAndRecovery` (2 runs in `activities`, `GetActivity(14820001234).Type=="running"`); `TestMigration00009RekeysAndDrops`; engines untouched (no metrics/progress/coach/chat logic edits) |
| 2 | Per-second streams from `.FIT` worker only; `resolveGarminID` returns the activity id directly | Task 5 (streams FIT-only, identity resolve), Task 4 (runner/types comments) | `TestResolveGarminIDIsIdentity`, `TestFetchAndAnalyzeFITOnly`, `TestFetchAndAnalyzeFITErrorPropagates`; `strava.go` deleted |
| 3 | No Strava remains: package, OAuth endpoints, Connect screen, `SyncStrava`, `STRAVA_*` env removed; Go build has no `strava` import | Task 5 (strava.go), Task 6 (`SyncStrava`/follow-ups), Task 7 (OAuth routes/handlers + config fields), Task 8 (delete `strava` package + dead store fns), Task 9 (config_test), Task 12 (Connect screen) | **Build-hygiene grep:** `grep -rn 'internal/strava' backend/` = 0 (`GREP_EXIT:1`, Task 8 Step 7 / Task 13 Step 1); `grep -rni strava app/src app/app` = 0 (Task 12 Step 6); `go build ./...` green |
| 4 | `/api/status` reports Garmin only; removed Strava routes 404 | Task 7 (`status()` Garmin-only, routes removed, DTO) | `TestRemovedStravaRoutesReturn404` (both routes → 404); status test asserts Garmin + counts only; `/api/status` shape per Contract E |
| 5 | App Connect/Settings shows Garmin status + "Sync now"; other screens unchanged | Task 12 (settings/index Garmin-only) | jest: settings render Garmin status + Sync; `index` Garmin status + `activity_id` rows; run-detail/plan/progress/chat smoke unaffected; `grep -rni strava app/src app/app` = 0 |
| 6 | Full Go/worker/jest suites green; real `make garmin-login` + sync lands real runs and downstream features work | Task 13 (full-suite gate) + all | **Suite gates:** `go build ./...` + `go test ./...` (Task 13 Step 1), worker `pytest` (Step 2), app `jest` + `tsc` (Step 3), `gofmt -l .` empty; **Manual** (Step 4): `make garmin-login` → sync → real runs in `activities` → Progress/streams/plan/chat on real data |

**Build hygiene (spec §8), per task:** `go build ./...` green after every commit (Tasks 1+2 commit together, first whole-tree green at end of Task 2; per-package green for Tasks 5/6 until Task 7 closes the wiring); `go test ./...` fully green after Task 8; worker `pytest` after Task 3; app `jest`+`tsc` after Task 12; `gofmt -l .` empty at each Go commit; `grep -rn internal/strava backend` = 0 (Task 8); `grep -rni strava app/src app/app` = 0 (Task 12); removed routes 404 (Task 7); manual `make garmin-login`→sync→real-runs check (Task 13).
