# Milestone 0 (Foundation) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove the data pipeline end-to-end — connect Strava and Garmin once, land recent runs (with HR, pace, distance, splits) and ~30 days of recovery (sleep, HRV, Body Battery, resting HR) into one local SQLite store, and surface connection status, last-sync times, recent runs, and recent recovery in the Expo app, with both a manual "Sync now" and an automatic periodic sync.

**Architecture:** A Go core owns the SQLite database (single writer via `modernc.org/sqlite`), the REST API (chi), Strava OAuth + sync, and a periodic sync scheduler. A thin, stateless Python worker handles the fragile Garmin path only — it logs in once (MFA-aware) and, on `fetch`, prints normalized JSON to stdout, which Go invokes via `os/exec` and writes to SQLite. The Expo app (expo-router + React Query) is the client, talking to the Go core over HTTPS with a single bearer token stored in `expo-secure-store`.

**Tech Stack:** Go + chi + `modernc.org/sqlite` + goose (backend core, API, DB, scheduler); Python + `garminconnect` (`curl_cffi` TLS impersonation) for the Garmin worker; Expo + expo-router + react-query (`@tanstack/react-query`) for the mobile client.

---

## Setup Prerequisites

Obtain these before running anything. Each value goes into a local `.env` (copied from `.env.example`); `.env` is git-ignored and must never hold real secrets in a commit.

### 1. Create a Strava API application
- Go to <https://www.strava.com/settings/api> and create an app.
- Note the **Client ID** and **Client Secret**.
- Set the app's **Authorization Callback Domain** to match your `STRAVA_REDIRECT_URL` host (for local dev: `localhost`).
- Scope used by the backend: `activity:read_all` (read all activities). The redirect URL points at `/api/strava/callback`.
- Put these in `.env`:
  - `STRAVA_CLIENT_ID=<your client id>`
  - `STRAVA_CLIENT_SECRET=<your client secret>`
  - `STRAVA_REDIRECT_URL=http://localhost:8080/api/strava/callback` (must EXACTLY match the value registered in the Strava app settings and the one the backend builds — three places, one value)

### 2. One-time Garmin login (`make garmin-login`)
- Garmin has no usable official API; the worker uses the unofficial `python-garminconnect` with `curl_cffi` TLS impersonation. Keep request volume low.
- Put your Garmin credentials in `.env`:
  - `GARMIN_EMAIL=you@example.com`
  - `GARMIN_PASSWORD=<your garmin password>`
  - `GARMIN_TOKENSTORE=~/.garminconnect` (directory where refreshed OAuth tokens persist; optional, this is the default)
- After the worker virtualenv exists, run the one-time interactive (MFA-aware) login:
  ```bash
  make garmin-login
  ```
  This persists refreshed OAuth tokens to `GARMIN_TOKENSTORE` so subsequent `fetch` calls are non-interactive. Re-run it if tokens expire/are revoked.

### 3. Get an Anthropic API key (NOT needed until M1)
- Create one at <https://console.anthropic.com/>.
- In M0 it is loaded but unused — the placeholder in `.env` is fine:
  - `ANTHROPIC_API_KEY=<your anthropic key>` (stub in M0)

### Other `.env` values
- `API_TOKEN=<long random string>` — the bearer token the Expo app sends on every protected endpoint. Generate with `openssl rand -hex 32`.
- `DB_PATH=./helpmyrun.db` — SQLite file path (optional, default shown).
- `PORT=8080` — HTTP listen port (optional, default shown).
- `PYTHON_BIN=garmin-worker/.venv/bin/python` and `WORKER_SCRIPT=garmin-worker/worker.py` — how Go invokes the worker (optional, defaults resolve the venv path).

---

## File Structure

```
help-my-run/
├── .env.example                    # documents all M0 env vars (see Shared Contracts §4)
├── .gitignore                      # excludes .env, *.db, .garminconnect/, .venv/, node_modules/, app build dirs
├── README.md                       # full self-host setup incl. one-time worker.py login
├── Makefile                        # run-backend, sync, garmin-login, run-app, test targets
│
├── backend/                        # Go core
│   ├── go.mod                      # module github.com/USER/help-my-run/backend
│   ├── go.sum                      # dependency checksums
│   ├── cmd/
│   │   └── server/
│   │       └── main.go             # loads config, runs migrations, starts chi server + periodic ticker
│   └── internal/
│       ├── config/
│       │   ├── config.go           # Config struct + Load() (godotenv + envconfig)
│       │   └── config_test.go      # defaults / required / explicit env tests
│       ├── store/
│       │   ├── store.go            # sql.Open("sqlite", dsn), SetMaxOpenConns(1), Open/Close
│       │   ├── migrate.go          # goose.SetBaseFS + SetDialect("sqlite3") + Up
│       │   ├── activities.go       # UpsertActivity, ListActivities, UpsertSplits
│       │   ├── garmin.go           # UpsertSleep/Hrv/BodyBattery/Rhr, CountRecoveryDays, ListRecovery
│       │   ├── tokens.go           # GetStravaTokens, SaveStravaTokens, ErrNotFound
│       │   ├── synclog.go          # GetSyncLog, UpdateSyncLog
│       │   ├── store_test.go       # open/migrate + all typed-query tests
│       │   └── migrations/
│       │       └── 00001_init.sql  # the schema in Shared Contracts §1 (embedded via //go:embed)
│       ├── api/
│       │   ├── router.go           # chi router, middleware, Deps, route mounting
│       │   ├── auth.go             # BearerAuth(token) middleware
│       │   ├── handlers.go         # health, status, connect, callback, sync, activities, recovery
│       │   ├── dto.go              # response structs matching Shared Contracts §3 JSON exactly
│       │   ├── auth_test.go        # bearer-auth table-driven tests
│       │   ├── handlers_test.go    # httptest handler tests
│       │   └── testdata/           # handler fixtures
│       ├── strava/
│       │   ├── client.go           # injectable baseURL; AuthorizeURL, Exchange, Refresh, ListActivities, ListLaps
│       │   ├── types.go            # SummaryActivity, Lap, TokenResponse, SummaryAthlete
│       │   ├── client_test.go      # httptest.NewServer + fixtures
│       │   └── testdata/
│       │       ├── strava_activities.json
│       │       ├── strava_laps.json
│       │       └── strava_token.json
│       ├── garmin/
│       │   ├── runner.go           # RunGarminFetch via os/exec.CommandContext
│       │   ├── types.go            # WorkerOutput, SleepDay, HrvDay, BodyBatteryDay, RhrDay (matches §2)
│       │   ├── runner_test.go      # fake worker script / fixture
│       │   └── testdata/
│       │       └── worker_output.json
│       └── sync/
│           ├── sync.go             # SyncStrava, SyncGarmin, SyncAll (used by POST /api/sync + ticker)
│           ├── ticker.go           # periodic RunTicker loop
│           └── sync_test.go        # sync + ticker tests
│
├── garmin-worker/                  # Python Garmin worker (thin, stateless)
│   ├── worker.py                   # thin entrypoint delegating to garmin_worker.cli.main
│   ├── requirements.txt            # garminconnect==0.3.6, curl_cffi, pytest
│   ├── pytest.ini                  # test discovery config
│   ├── garmin_worker/
│   │   ├── __init__.py             # package marker
│   │   ├── normalize.py            # pure raw->contract JSON normalizers (no I/O, no garminconnect)
│   │   ├── cli.py                  # argparse + --dry-run + login/fetch wiring
│   │   ├── client.py               # the ONLY module importing garminconnect (login/resume/data methods)
│   │   └── fetcher.py              # run_fetch: drive injected client over date range -> §2.1 dict
│   └── tests/
│       ├── __init__.py             # empty package marker
│       ├── test_normalize.py       # pure-function contract-shape tests
│       ├── test_cli.py             # parser + --dry-run tests
│       ├── test_client.py          # env/tokenstore plumbing + 1:1 delegation tests
│       ├── test_fetcher.py         # date iteration + mock-client tests
│       ├── test_fetch_cli.py       # live fetch branch (client/fetcher mocked)
│       └── fixtures/
│           ├── raw_sleep_2026-06-15.json
│           ├── raw_hrv_2026-06-15.json
│           ├── raw_body_battery_range.json
│           ├── raw_stats_2026-06-15.json
│           └── dry_run_expected.json
│
├── app/                            # Expo app (SDK 56, expo-router; create-expo-app@latest now ships SDK 56)
│   ├── app.json                    # expo.scheme "helpmyrun", name, slug
│   ├── package.json                # scripts.test=jest, jest.preset=jest-expo
│   ├── tsconfig.json               # TypeScript config
│   ├── app/                        # expo-router routes (flat, after `npm run reset-project` in Task 5 — no (tabs)/ group, no +not-found.tsx)
│   │   ├── _layout.tsx             # Stack + QueryClientProvider (overwrites reset-project's _layout.tsx)
│   │   ├── index.tsx               # Home/Status screen (overwrites reset-project's index.tsx)
│   │   ├── settings.tsx            # Connect/Settings screen (newly created)
│   │   └── __tests__/              # screen + layout tests
│   └── src/
│       └── api/
│           ├── client.ts           # typed fetch (apiGet/apiPost), ApiError, Bearer + base URL
│           ├── config.ts           # expo-secure-store: save/get baseUrl + token
│           ├── settings.ts         # useSettings hook over secure-store
│           ├── types.ts            # the §3.8 TypeScript interfaces
│           ├── queryClient.ts      # shared QueryClient instance
│           ├── hooks.ts            # useStatus, useActivities, useRecovery, useSync, useConnectStrava
│           └── __tests__/          # client/types/hooks/settings tests
│
└── docs/
    └── superpowers/
        ├── specs/
        │   └── 2026-06-19-help-my-run-m0-foundation-design.md   # the design spec
        └── plans/
            └── 2026-06-19-m0-foundation.md                       # this plan
```

---

## Shared Contracts

The Go core, Python worker, and Expo app MUST all conform to the names, types, shapes, and URLs below verbatim. A typo here is a cross-component bug.

**Conventions:**
- All dates are `YYYY-MM-DD` strings (UTC calendar date) unless suffixed `_time`/`_at`.
- All timestamps (`*_time`, `*_at`) are ISO-8601 UTC strings, e.g. `2026-06-15T06:30:00Z`.
- Durations are integer seconds (`*_s`); distances are float meters (`*_m`); speeds are float m/s.
- Heart rates are integers (bpm). HRV values are integers (ms).
- `raw_json` columns store the complete upstream payload as a TEXT string (verbatim JSON) so M1's metrics engine never loses fidelity.
- Nullable values: in JSON they are `null`; in SQLite the column is nullable (no `NOT NULL`).

### 1. SQLite schema

Single goose migration file: `backend/internal/store/migrations/00001_init.sql`.
Driver: `modernc.org/sqlite` (driver name `"sqlite"`); goose dialect `"sqlite3"`.
DSN: `file:<DB_PATH>?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)`.

```sql
-- +goose Up
-- +goose StatementBegin

-- ---------------------------------------------------------------------------
-- strava_tokens : single row (id always = 1). OAuth tokens for the one user.
-- ---------------------------------------------------------------------------
CREATE TABLE strava_tokens (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    access_token  TEXT    NOT NULL,
    refresh_token TEXT    NOT NULL,
    expires_at    INTEGER NOT NULL,              -- unix epoch seconds (Strava expires_at)
    scope         TEXT,                          -- e.g. "read,activity:read_all"
    athlete_id    INTEGER,                       -- Strava athlete.id (summary athlete)
    updated_at    TEXT    NOT NULL               -- ISO-8601 UTC, set by Go on write
);

-- ---------------------------------------------------------------------------
-- activities : one row per Strava run. strava_id is the natural PK.
-- ---------------------------------------------------------------------------
CREATE TABLE activities (
    strava_id        INTEGER PRIMARY KEY,        -- Strava SummaryActivity.id (long)
    name             TEXT    NOT NULL,
    type             TEXT    NOT NULL,           -- deprecated Strava "type", e.g. "Run"
    sport_type       TEXT,                       -- preferred Strava "sport_type", e.g. "Run","TrailRun"
    start_time       TEXT    NOT NULL,           -- ISO-8601 UTC (Strava start_date)
    start_time_local TEXT,                       -- ISO-8601 local (Strava start_date_local)
    distance_m       REAL    NOT NULL,           -- meters
    moving_time_s    INTEGER NOT NULL,           -- seconds
    elapsed_time_s   INTEGER NOT NULL,           -- seconds
    avg_hr           REAL,                        -- bpm, nullable (no HR sensor)
    max_hr           REAL,                        -- bpm, nullable
    avg_speed        REAL,                        -- m/s, nullable
    max_speed        REAL,                        -- m/s, nullable
    avg_cadence      REAL,                        -- Strava one-leg RPM, nullable
    elevation_gain_m REAL,                        -- meters, nullable
    raw_json         TEXT    NOT NULL,           -- full Strava SummaryActivity JSON
    synced_at        TEXT    NOT NULL            -- ISO-8601 UTC, set by Go on upsert
);

CREATE INDEX idx_activities_start_time ON activities (start_time DESC);

-- ---------------------------------------------------------------------------
-- activity_splits : Strava laps for an activity (lap_index -> idx).
-- ---------------------------------------------------------------------------
CREATE TABLE activity_splits (
    activity_id    INTEGER NOT NULL,             -- FK -> activities.strava_id
    idx            INTEGER NOT NULL,             -- Strava lap_index (1-based)
    distance_m     REAL    NOT NULL,             -- meters
    elapsed_time_s INTEGER NOT NULL,             -- seconds
    moving_time_s  INTEGER,                       -- seconds, nullable
    avg_hr         REAL,                          -- bpm, nullable
    max_hr         REAL,                          -- bpm, nullable
    avg_speed      REAL,                          -- m/s, nullable
    PRIMARY KEY (activity_id, idx),
    FOREIGN KEY (activity_id) REFERENCES activities (strava_id) ON DELETE CASCADE
);

-- ---------------------------------------------------------------------------
-- garmin_sleep : one row per calendar date. date is PK.
-- ---------------------------------------------------------------------------
CREATE TABLE garmin_sleep (
    date       TEXT    PRIMARY KEY,              -- YYYY-MM-DD
    duration_s INTEGER,                           -- sleepTimeSeconds
    deep_s     INTEGER,                           -- deepSleepSeconds
    light_s    INTEGER,                           -- lightSleepSeconds
    rem_s      INTEGER,                           -- remSleepSeconds
    awake_s    INTEGER,                           -- awakeSleepSeconds
    score      INTEGER,                           -- sleepScores.overall.value, nullable
    raw_json   TEXT    NOT NULL                  -- full get_sleep_data() dict
);

-- ---------------------------------------------------------------------------
-- garmin_hrv : one row per calendar date. date is PK.
-- ---------------------------------------------------------------------------
CREATE TABLE garmin_hrv (
    date              TEXT PRIMARY KEY,           -- YYYY-MM-DD
    last_night_avg_ms INTEGER,                    -- hrvSummary.lastNightAvg, nullable
    status            TEXT,                       -- hrvSummary.status, nullable
    raw_json          TEXT NOT NULL              -- full get_hrv_data() dict (or "null")
);

-- ---------------------------------------------------------------------------
-- garmin_body_battery : one row per calendar date. date is PK.
-- ---------------------------------------------------------------------------
CREATE TABLE garmin_body_battery (
    date     TEXT PRIMARY KEY,                    -- YYYY-MM-DD
    charged  INTEGER,                             -- charged over the day, nullable
    drained  INTEGER,                             -- drained over the day, nullable
    high     INTEGER,                             -- max of bodyBatteryValuesArray, nullable
    low      INTEGER,                             -- min of bodyBatteryValuesArray, nullable
    raw_json TEXT NOT NULL                       -- full per-day dict
);

-- ---------------------------------------------------------------------------
-- garmin_rhr : one row per calendar date. date is PK.
-- ---------------------------------------------------------------------------
CREATE TABLE garmin_rhr (
    date       TEXT    PRIMARY KEY,              -- YYYY-MM-DD
    resting_hr INTEGER,                           -- get_stats()["restingHeartRate"], nullable
    raw_json   TEXT    NOT NULL                  -- full get_stats() dict
);

-- ---------------------------------------------------------------------------
-- sync_log : one row per source. source is PK (upserted each sync).
-- ---------------------------------------------------------------------------
CREATE TABLE sync_log (
    source         TEXT PRIMARY KEY,             -- "strava" | "garmin"
    last_synced_at TEXT,                          -- ISO-8601 UTC of last SUCCESSFUL sync, nullable
    last_run_at    TEXT,                          -- ISO-8601 UTC of last attempt (success or fail)
    status         TEXT NOT NULL,                -- "ok" | "error" | "never"
    error          TEXT                          -- error message when status="error", else NULL
);

-- Seed the two sync_log rows so /api/status always has both.
INSERT INTO sync_log (source, last_synced_at, last_run_at, status, error)
VALUES ('strava', NULL, NULL, 'never', NULL),
       ('garmin', NULL, NULL, 'never', NULL);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE sync_log;
DROP TABLE garmin_rhr;
DROP TABLE garmin_body_battery;
DROP TABLE garmin_hrv;
DROP TABLE garmin_sleep;
DROP TABLE activity_splits;
DROP TABLE activities;
DROP TABLE strava_tokens;
-- +goose StatementEnd
```

**Type notes for Go scanning:**
- Nullable REAL/INTEGER columns (`avg_hr`, `max_hr`, `avg_speed`, `avg_cadence`, `elevation_gain_m`, all `garmin_*` metric columns, `last_synced_at`, `error`) MUST be scanned into `sql.Null*` types or pointers (`*float64`, `*int64`, `*string`).
- `expires_at` (strava_tokens) is unix epoch **seconds** (matches Strava `expires_at`), distinct from the ISO-8601 `*_at` text columns.

### 2. Garmin worker stdout JSON contract

**Command:** `python worker.py fetch --since YYYY-MM-DD`
**Output:** exactly ONE JSON object printed to stdout (no logs on stdout — all diagnostics go to stderr). Exit code 0 on success; non-zero on auth/connection failure (Go reads stderr into `sync_log.error`).

#### 2.1 Top-level shape

```jsonc
{
  "since": "2026-05-20",          // echo of --since arg (string YYYY-MM-DD)
  "until": "2026-06-19",          // last date fetched, inclusive (today, string YYYY-MM-DD)
  "fetched_at": "2026-06-19T05:00:12Z",  // ISO-8601 UTC when the worker ran
  "sleep":        [ SleepDay,        ... ],  // array, one entry per date with data
  "hrv":          [ HrvDay,          ... ],  // array; entries omitted when Garmin returned null
  "body_battery": [ BodyBatteryDay,  ... ],  // array, one entry per date
  "rhr":          [ RhrDay,          ... ]   // array, one entry per date with data
}
```

Each array is keyed by calendar `date`. Arrays MAY be shorter than the date range (Garmin may have no data for some days). Go upserts each entry by `date` into the matching `garmin_*` table. Empty arrays are valid (`[]`).

#### 2.2 Per-day object shapes

**`SleepDay`** (maps to `garmin_sleep`; source `get_sleep_data(date)`):
```jsonc
{
  "date":       "2026-06-15",   // string YYYY-MM-DD
  "duration_s": 27000,          // int|null  <- dailySleepDTO.sleepTimeSeconds
  "deep_s":     6300,           // int|null  <- dailySleepDTO.deepSleepSeconds
  "light_s":    14400,          // int|null  <- dailySleepDTO.lightSleepSeconds
  "rem_s":      5400,           // int|null  <- dailySleepDTO.remSleepSeconds
  "awake_s":    900,            // int|null  <- dailySleepDTO.awakeSleepSeconds
  "score":      82,             // int|null  <- dailySleepDTO.sleepScores.overall.value
  "raw_json":   { /* full get_sleep_data() dict, verbatim */ }
}
```

**`HrvDay`** (maps to `garmin_hrv`; source `get_hrv_data(date)`, which may be `None` — those dates are omitted from the array):
```jsonc
{
  "date":              "2026-06-15",  // string YYYY-MM-DD
  "last_night_avg_ms": 48,            // int|null  <- hrvSummary.lastNightAvg
  "status":            "BALANCED",    // string|null <- hrvSummary.status
  "raw_json":          { /* full get_hrv_data() dict, verbatim */ }
}
```

**`BodyBatteryDay`** (maps to `garmin_body_battery`; source `get_body_battery(since, until)` range call, indexed per day):
```jsonc
{
  "date":     "2026-06-15",  // string YYYY-MM-DD
  "charged":  62,            // int|null  <- per-day "charged" (fallback: sum positive deltas of bodyBatteryValuesArray)
  "drained":  78,            // int|null  <- per-day "drained" (fallback: sum negative deltas)
  "high":     91,            // int|null  <- max value in bodyBatteryValuesArray
  "low":      14,            // int|null  <- min value in bodyBatteryValuesArray
  "raw_json": { /* full per-day dict from get_body_battery()[i], verbatim */ }
}
```

**`RhrDay`** (maps to `garmin_rhr`; source `get_stats(date)["restingHeartRate"]` — the confirmed path, not `get_rhr_day`):
```jsonc
{
  "date":       "2026-06-15",  // string YYYY-MM-DD
  "resting_hr": 47,            // int|null  <- get_stats(date)["restingHeartRate"]
  "raw_json":   { /* full get_stats() dict, verbatim */ }
}
```

#### 2.3 Concrete full example

```json
{
  "since": "2026-06-14",
  "until": "2026-06-15",
  "fetched_at": "2026-06-15T05:00:12Z",
  "sleep": [
    {
      "date": "2026-06-14",
      "duration_s": 26400, "deep_s": 6000, "light_s": 14100,
      "rem_s": 5400, "awake_s": 900, "score": 79,
      "raw_json": {"dailySleepDTO": {"sleepTimeSeconds": 26400, "deepSleepSeconds": 6000, "lightSleepSeconds": 14100, "remSleepSeconds": 5400, "awakeSleepSeconds": 900, "sleepScores": {"overall": {"value": 79}}}}
    },
    {
      "date": "2026-06-15",
      "duration_s": 27000, "deep_s": 6300, "light_s": 14400,
      "rem_s": 5400, "awake_s": 900, "score": 82,
      "raw_json": {"dailySleepDTO": {"sleepTimeSeconds": 27000, "deepSleepSeconds": 6300, "lightSleepSeconds": 14400, "remSleepSeconds": 5400, "awakeSleepSeconds": 900, "sleepScores": {"overall": {"value": 82}}}}
    }
  ],
  "hrv": [
    {
      "date": "2026-06-15",
      "last_night_avg_ms": 48, "status": "BALANCED",
      "raw_json": {"hrvSummary": {"lastNightAvg": 48, "lastNight5MinHigh": 70, "weeklyAvg": 46, "status": "BALANCED"}, "hrvReadings": []}
    }
  ],
  "body_battery": [
    {
      "date": "2026-06-14", "charged": 60, "drained": 75, "high": 88, "low": 16,
      "raw_json": {"date": "2026-06-14", "charged": 60, "drained": 75, "bodyBatteryValuesArray": [[1718323200000, "ACTIVE", 88], [1718366400000, "ACTIVE", 16]]}
    },
    {
      "date": "2026-06-15", "charged": 62, "drained": 78, "high": 91, "low": 14,
      "raw_json": {"date": "2026-06-15", "charged": 62, "drained": 78, "bodyBatteryValuesArray": [[1718409600000, "ACTIVE", 91], [1718452800000, "ACTIVE", 14]]}
    }
  ],
  "rhr": [
    {
      "date": "2026-06-14", "resting_hr": 48,
      "raw_json": {"restingHeartRate": 48, "totalSteps": 9000}
    },
    {
      "date": "2026-06-15", "resting_hr": 47,
      "raw_json": {"restingHeartRate": 47, "totalSteps": 11000}
    }
  ]
}
```

#### 2.4 Error contract
On auth failure (`GarminConnectAuthenticationError`): exit non-zero, print to **stderr** a message containing the literal substring `re-run worker.py login`, print nothing parseable on stdout. On rate-limit (`GarminConnectTooManyRequestsError`) after retries, or connection error: exit non-zero with the exception message on stderr. Go writes the captured stderr into `sync_log.error` for source `garmin`.

### 3. REST API contract

Base URL: `http://<host>:<PORT>` (PORT default `8080`). Auth: `Authorization: Bearer <API_TOKEN>` on all endpoints **except** `GET /health` and `GET /api/strava/callback`. Missing/wrong token on a protected endpoint → `401` with body `{"error":"unauthorized"}`. Content-Type of all JSON responses: `application/json`.

#### 3.1 `GET /health`
- Auth: none. Query/body: none.
- `200`:
```json
{ "status": "ok" }
```

#### 3.2 `GET /api/status`
- Auth: bearer. Query/body: none.
- Returns connection + last-sync state for both sources plus row counts. Consumed by the Expo Home/Status screen and the OAuth completion poll.
- `200`:
```json
{
  "strava": {
    "connected": true,
    "athlete_id": 12345678,
    "last_synced_at": "2026-06-19T05:00:30Z",
    "last_run_at": "2026-06-19T05:00:30Z",
    "status": "ok",
    "error": null
  },
  "garmin": {
    "connected": true,
    "last_synced_at": "2026-06-19T05:00:42Z",
    "last_run_at": "2026-06-19T05:00:42Z",
    "status": "ok",
    "error": null
  },
  "counts": {
    "activities": 42,
    "recovery_days": 30
  }
}
```
- Field semantics: `strava.connected` = a row exists in `strava_tokens`. `garmin.connected` = `garmin_*` tables have at least one row (proxy for "worker.py login has run and produced data"). `status` ∈ `"ok" | "error" | "never"` (from `sync_log.status`). `error` is `null` unless `status == "error"`. `counts.recovery_days` = distinct dates present across `garmin_*` tables. `strava.athlete_id` is a non-null `number` in M0: a `strava_tokens` row only exists after a successful OAuth that includes an athlete, so a `Status` with `strava` data always carries a real `athlete_id`.

#### 3.3 `GET /api/strava/connect`
- Auth: bearer. Query/body: none.
- Builds the Strava authorize URL (server-side, using `STRAVA_CLIENT_ID`, `STRAVA_REDIRECT_URL`, `response_type=code`, `scope=activity:read_all`, `approval_prompt=auto`, and a generated `state`).
- `200`:
```json
{ "authorizeUrl": "https://www.strava.com/oauth/authorize?client_id=12345&redirect_uri=http%3A%2F%2Flocalhost%3A8080%2Fapi%2Fstrava%2Fcallback&response_type=code&scope=activity%3Aread_all&approval_prompt=auto&state=abc123" }
```
- JSON key is exactly `authorizeUrl` (camelCase) — the Expo `connectStrava()` reads `{ authorizeUrl }`.

#### 3.4 `GET /api/strava/callback`
- Auth: **none** (Strava's browser redirect hits this directly).
- Query params (from Strava): `code` (string), `scope` (string), `state` (string); or `error=access_denied` on denial.
- Behavior: exchange `code` at `POST https://www.strava.com/oauth/token` (`grant_type=authorization_code`), persist `access_token`/`refresh_token`/`expires_at`/`scope`/`athlete_id` into `strava_tokens`.
- Response: `200` with a minimal HTML page containing the text `You can close this tab.` (the Expo app uses Option B — open browser then poll `/api/status` — so no app-scheme redirect is required). On `error=access_denied` or exchange failure: `200` HTML page stating the connection failed (still closeable).

#### 3.5 `POST /api/sync`
- Auth: bearer. Query/body: none (optional body ignored).
- Behavior: runs both a Strava incremental sync and a Garmin worker fetch (incremental since each source's `last_synced_at`, or a ~30-day backfill if never synced). Updates `sync_log` for both sources. This is the `Sync now` button and the periodic ticker's action.
- `200` (per-source result; `synced` = rows upserted):
```json
{
  "strava": { "status": "ok", "synced": 3, "error": null },
  "garmin": { "status": "ok", "synced": 5, "error": null }
}
```
- On partial failure, the failing source has `status: "error"` and a non-null `error` string; the endpoint still returns `200` (the app shows per-source errors). Example:
```json
{
  "strava": { "status": "ok", "synced": 2, "error": null },
  "garmin": { "status": "error", "synced": 0, "error": "worker exit 1: re-run worker.py login" }
}
```

#### 3.6 `GET /api/activities`
- Auth: bearer. Query: `limit` (int, optional, default `30`, max `200`).
- Returns most-recent-first runs. Consumed by the Expo recent-runs list (`useActivities` hook).
- `200`:
```json
{
  "activities": [
    {
      "strava_id": 14820001234,
      "name": "Morning Run",
      "type": "Run",
      "sport_type": "Run",
      "start_time": "2026-06-18T06:12:00Z",
      "start_time_local": "2026-06-18T08:12:00",
      "distance_m": 10240.5,
      "moving_time_s": 3120,
      "elapsed_time_s": 3200,
      "avg_hr": 152.3,
      "max_hr": 171,
      "avg_speed": 3.28,
      "max_speed": 4.91,
      "avg_cadence": 86.5,
      "elevation_gain_m": 84.0
    }
  ]
}
```
- Nullable fields (`sport_type`, `start_time_local`, `avg_hr`, `max_hr`, `avg_speed`, `max_speed`, `avg_cadence`, `elevation_gain_m`) serialize as `null` when absent. `raw_json` is NOT included in this list response. Splits are NOT included in M0 (deferred to activity-detail in M1).

#### 3.7 `GET /api/recovery`
- Auth: bearer. Query: `days` (int, optional, default `30`, max `365`).
- Returns one merged record per calendar date (most-recent-first), joining the four `garmin_*` tables by `date`. Any source missing for a date yields `null` sub-fields. Consumed by the Expo recent-recovery list (`useRecovery` hook).
- `200`:
```json
{
  "recovery": [
    {
      "date": "2026-06-18",
      "sleep":        { "duration_s": 27000, "deep_s": 6300, "light_s": 14400, "rem_s": 5400, "awake_s": 900, "score": 82 },
      "hrv":          { "last_night_avg_ms": 48, "status": "BALANCED" },
      "body_battery": { "charged": 62, "drained": 78, "high": 91, "low": 14 },
      "rhr":          { "resting_hr": 47 }
    },
    {
      "date": "2026-06-17",
      "sleep":        { "duration_s": 25800, "deep_s": 5400, "light_s": 13800, "rem_s": 4800, "awake_s": 1800, "score": 71 },
      "hrv":          null,
      "body_battery": { "charged": 58, "drained": 80, "high": 86, "low": 12 },
      "rhr":          { "resting_hr": 49 }
    }
  ]
}
```
- Each of `sleep`/`hrv`/`body_battery`/`rhr` is either the object shown or `null` (no Garmin data for that source on that date). The inner field names match the `garmin_*` table columns exactly (minus `date`/`raw_json`).

#### 3.8 TypeScript types for the Expo client (`src/api/types.ts`)

```ts
export interface Health { status: string; }

export interface SourceStatus {
  connected: boolean;
  last_synced_at: string | null;
  last_run_at: string | null;
  status: 'ok' | 'error' | 'never';
  error: string | null;
}
export interface Status {
  strava: SourceStatus & { athlete_id: number }; // M0 always emits a non-null athlete_id: a strava_tokens row exists only after a successful OAuth that includes an athlete.
  garmin: SourceStatus;
  counts: { activities: number; recovery_days: number };
}

export interface ConnectResponse { authorizeUrl: string; }

export interface SyncSourceResult { status: 'ok' | 'error'; synced: number; error: string | null; }
export interface SyncResponse { strava: SyncSourceResult; garmin: SyncSourceResult; }

export interface Activity {
  strava_id: number;
  name: string;
  type: string;
  sport_type: string | null;
  start_time: string;
  start_time_local: string | null;
  distance_m: number;
  moving_time_s: number;
  elapsed_time_s: number;
  avg_hr: number | null;
  max_hr: number | null;
  avg_speed: number | null;
  max_speed: number | null;
  avg_cadence: number | null;
  elevation_gain_m: number | null;
}
export interface ActivitiesResponse { activities: Activity[]; }

export interface RecoveryDay {
  date: string;
  sleep: { duration_s: number | null; deep_s: number | null; light_s: number | null; rem_s: number | null; awake_s: number | null; score: number | null } | null;
  hrv: { last_night_avg_ms: number | null; status: string | null } | null;
  body_battery: { charged: number | null; drained: number | null; high: number | null; low: number | null } | null;
  rhr: { resting_hr: number | null } | null;
}
export interface RecoveryResponse { recovery: RecoveryDay[]; }
```

### 4. Environment variables

Documented in `.env.example`. Loaded by Go via `godotenv.Load()` + `envconfig.Process("", &cfg)`. The Python worker reads its subset directly via `os.getenv`. The Expo app reads NONE of these (its base URL + token are entered in-app and stored in `expo-secure-store`).

| Name | Purpose | Example value | Read by | Required |
|---|---|---|---|---|
| `STRAVA_CLIENT_ID` | Strava app ID for OAuth | `123456` | Go | yes |
| `STRAVA_CLIENT_SECRET` | Strava app secret for token exchange | `<secret>` | Go | yes |
| `STRAVA_REDIRECT_URL` | OAuth callback (must match Strava app settings; points at `/api/strava/callback`) | `http://localhost:8080/api/strava/callback` | Go | yes |
| `API_TOKEN` | Bearer token the Expo app sends; gates all protected endpoints | `<long-random-string>` | Go | yes |
| `DB_PATH` | SQLite file path | `./helpmyrun.db` | Go | no (default `./helpmyrun.db`) |
| `PORT` | HTTP listen port | `8080` | Go | no (default `8080`) |
| `GARMIN_EMAIL` | Garmin Connect login email (for one-time `worker.py login`) | `you@example.com` | Python worker (and Go, to pass through env) | yes (for Garmin) |
| `GARMIN_PASSWORD` | Garmin Connect password | `<secret>` | Python worker | yes (for Garmin) |
| `GARMIN_TOKENSTORE` | Directory for persisted Garmin OAuth tokens | `~/.garminconnect` | Python worker | no (default `~/.garminconnect`) |
| `PYTHON_BIN` | Python interpreter Go uses to invoke the worker | `garmin-worker/.venv/bin/python` | Go | no (default resolves the venv path) |
| `WORKER_SCRIPT` | Path to the worker entrypoint | `garmin-worker/worker.py` | Go | no (default `garmin-worker/worker.py`) |
| `ANTHROPIC_API_KEY` | Claude key — STUB in M0, unused until M1 | `<secret>` | Go (loaded, not used) | no |

Notes:
- `STRAVA_REDIRECT_URL` MUST equal the `redirect_uri` Go builds in `/api/strava/connect` and the one registered in the Strava app — three places, one value.
- Go passes `GARMIN_EMAIL`, `GARMIN_PASSWORD`, `GARMIN_TOKENSTORE` through to the worker subprocess env when invoking `fetch`.
- `.gitignore` excludes `.env`, the Garmin token dir (`.garminconnect/` and any `GARMIN_TOKENSTORE` path), and `*.db`.

### Cross-component consistency anchors (copy verbatim)

- **Sync source identifiers:** `"strava"`, `"garmin"` (used as `sync_log.source` PKs and as top-level keys in `/api/status` and `/api/sync` responses).
- **Status enum:** `"ok" | "error" | "never"` everywhere (`sync_log.status`, `/api/status`, `/api/sync`).
- **Worker JSON top-level keys:** `since`, `until`, `fetched_at`, `sleep`, `hrv`, `body_battery`, `rhr`.
- **Connect response key:** `authorizeUrl` (camelCase) — all other API JSON uses `snake_case`; this one is camelCase to match the Expo `connectStrava()` destructure.
- **Strava token endpoint:** `https://www.strava.com/oauth/token` (NOT `/api/v3/oauth/token`).
- **modernc driver name:** `"sqlite"`; **goose dialect:** `"sqlite3"`.
- **RHR source method:** `get_stats(date)["restingHeartRate"]` (confirmed path; not `get_rhr_day`).
- **OAuth completion strategy:** Option B — backend callback returns "You can close this tab." HTML; app polls `GET /api/status` until `strava.connected === true`. No app deep-link scheme dependency in M0.

---

## Tasks

### Task 1: Confirm git repo and .gitignore secret coverage

**Files:**
- Test: (verification command — `.gitignore`)
- Modify: `/home/jake/project/help-my-run/.gitignore` (only if a pattern is missing)

- [ ] **Step 1: Verify the git repo and clean tree.** Run the exact command and confirm output:
  ```bash
  git -C /home/jake/project/help-my-run status --porcelain && echo "TREE_CLEAN"
  ```
  Expected output: an empty line for porcelain (no changes) followed by `TREE_CLEAN`. (A git repo already exists.)

- [ ] **Step 2: Verify `.gitignore` already covers every secret/artifact the contracts require.** Run this verification command:
  ```bash
  for p in '.env' '!.env.example' '.garminconnect/' '*.db' '*.db-wal' '__pycache__/' '.venv/' 'node_modules/' '.expo/'; do grep -qxF "$p" .gitignore && echo "OK   $p" || echo "MISS $p"; done
  ```
  Expected output: `OK` on every line:
  ```
  OK   .env
  OK   !.env.example
  OK   .garminconnect/
  OK   *.db
  OK   *.db-wal
  OK   __pycache__/
  OK   .venv/
  OK   node_modules/
  OK   .expo/
  ```

- [ ] **Step 3 (only if any line printed `MISS`): append the missing pattern(s).** For each `MISS <pattern>` line, append it under the appropriate section of `.gitignore`. If Step 2 printed all `OK`, skip this step and commit nothing here.

- [ ] **Step 4: Commit only if `.gitignore` changed.** Run:
  ```bash
  cd /home/jake/project/help-my-run && git diff --quiet .gitignore || git commit -m "chore(scaffold): ensure .gitignore covers env, db, garmin token store, venv" -- .gitignore
  ```
  Expected output: nothing if unchanged; a commit summary line if a pattern was added.

---

### Task 2: Create the full directory tree from the contracts

**Files:**
- Create (dirs): all directories from the File Structure section
- Test: `find` verification command

- [ ] **Step 1: Create every directory in one command.** Run exactly:
  ```bash
  cd /home/jake/project/help-my-run && mkdir -p \
    backend/cmd/server \
    backend/internal/config \
    backend/internal/store/migrations \
    backend/internal/api/testdata \
    backend/internal/strava/testdata \
    backend/internal/garmin/testdata \
    backend/internal/sync \
    garmin-worker/tests/fixtures \
    app/src/api/__tests__
  ```
  Expected output: none (success is silent).

- [ ] **Step 2: Add `.gitkeep` to empty leaf dirs git would otherwise drop.** Run:
  ```bash
  cd /home/jake/project/help-my-run && touch \
    backend/internal/api/testdata/.gitkeep \
    backend/internal/strava/testdata/.gitkeep \
    backend/internal/garmin/testdata/.gitkeep \
    garmin-worker/tests/fixtures/.gitkeep \
    app/src/api/__tests__/.gitkeep
  ```
  Expected output: none.

- [ ] **Step 3: Verify the tree exists (the "test").** Run:
  ```bash
  cd /home/jake/project/help-my-run && for d in backend/cmd/server backend/internal/config backend/internal/store/migrations backend/internal/api/testdata backend/internal/strava/testdata backend/internal/garmin/testdata backend/internal/sync garmin-worker/tests/fixtures app/src/api/__tests__; do test -d "$d" && echo "OK   $d" || echo "MISS $d"; done
  ```
  Expected output: nine `OK` lines, no `MISS`:
  ```
  OK   backend/cmd/server
  OK   backend/internal/config
  OK   backend/internal/store/migrations
  OK   backend/internal/api/testdata
  OK   backend/internal/strava/testdata
  OK   backend/internal/garmin/testdata
  OK   backend/internal/sync
  OK   garmin-worker/tests/fixtures
  OK   app/src/api/__tests__
  ```

- [ ] **Step 4: Commit.** Run:
  ```bash
  cd /home/jake/project/help-my-run && git add -A && git commit -m "chore(scaffold): create repo directory tree from contracts"
  ```
  Expected output: a commit summary listing the `.gitkeep` files added.

---

### Task 3: Initialize the Go module and pin core dependencies

**Files:**
- Create: `/home/jake/project/help-my-run/backend/go.mod`
- Create: `/home/jake/project/help-my-run/backend/go.sum`
- Create: `/home/jake/project/help-my-run/backend/cmd/server/main.go` (temporary stub so `go build ./...` has a buildable package; replaced by Task 31 later)
- Test: `go build ./...` and `go list -m all`

- [ ] **Step 1: Init the module.** Run (the USER must later replace `USER` with their GitHub username):
  ```bash
  cd /home/jake/project/help-my-run/backend && go mod init github.com/USER/help-my-run/backend
  ```
  Expected output:
  ```
  go: creating new go.mod: module github.com/USER/help-my-run/backend
  ```

- [ ] **Step 2: Write a minimal compilable `main.go` stub.** Create `/home/jake/project/help-my-run/backend/cmd/server/main.go` with exactly:
  ```go
  // Package main is the help-my-run backend server entrypoint.
  // This is a scaffolding stub; the real server is implemented by the SERVER tasks.
  package main

  import "fmt"

  func main() {
  	fmt.Println("help-my-run backend (scaffold stub)")
  }
  ```

- [ ] **Step 3: Add the five core dependencies at the contract-specified versions.** Run:
  ```bash
  cd /home/jake/project/help-my-run/backend && \
    go get github.com/go-chi/chi/v5@v5.1.0 && \
    go get modernc.org/sqlite@v1.34.4 && \
    go get github.com/pressly/goose/v3@v3.24.1 && \
    go get github.com/joho/godotenv@v1.5.1 && \
    go get github.com/kelseyhightower/envconfig@v1.4.0
  ```
  Expected output: a `go: added github.com/go-chi/chi/v5 v5.1.0` line (and one per module) for each `go get`, plus transitive `go: downloading ...` lines. (If a pinned version errors as nonexistent at build time, drop the `@version` suffix to take the latest; the dependency identity, not the exact patch, is what matters.)

- [ ] **Step 4: Tidy and build (the "test").** Run:
  ```bash
  cd /home/jake/project/help-my-run/backend && go mod tidy && go build ./...
  ```
  Expected output: silence (exit 0). `go build ./...` prints nothing on success.

- [ ] **Step 5: Verify all five deps are recorded in `go.mod`.** Run:
  ```bash
  cd /home/jake/project/help-my-run/backend && for m in github.com/go-chi/chi/v5 modernc.org/sqlite github.com/pressly/goose/v3 github.com/joho/godotenv github.com/kelseyhightower/envconfig; do grep -q "$m" go.mod && echo "OK   $m" || echo "MISS $m"; done
  ```
  Expected output: five `OK` lines, no `MISS`:
  ```
  OK   github.com/go-chi/chi/v5
  OK   modernc.org/sqlite
  OK   github.com/pressly/goose/v3
  OK   github.com/joho/godotenv
  OK   github.com/kelseyhightower/envconfig
  ```

- [ ] **Step 6: Commit.** Run:
  ```bash
  cd /home/jake/project/help-my-run && git add backend/go.mod backend/go.sum backend/cmd/server/main.go && git commit -m "chore(scaffold): init Go module backend with chi, modernc sqlite, goose, godotenv, envconfig"
  ```
  Expected output: a commit summary listing `go.mod`, `go.sum`, and `main.go`.

---

### Task 4: Create the Garmin worker requirements.txt and virtualenv

**Files:**
- Create: `/home/jake/project/help-my-run/garmin-worker/requirements.txt`
- Create: `/home/jake/project/help-my-run/garmin-worker/.venv/` (git-ignored)
- Create: `/home/jake/project/help-my-run/garmin-worker/worker.py` (temporary import-smoke stub; replaced by Task 16 later)
- Test: venv import smoke + `python worker.py --help`

- [ ] **Step 1: Write `requirements.txt`.** Create `/home/jake/project/help-my-run/garmin-worker/requirements.txt` with exactly:
  ```
  garminconnect==0.3.6
  curl_cffi
  ```

- [ ] **Step 2: Create the virtualenv.** Run:
  ```bash
  cd /home/jake/project/help-my-run/garmin-worker && python3 -m venv .venv
  ```
  Expected output: none (success is silent). Verify with `test -x .venv/bin/python && echo VENV_OK` → `VENV_OK`.

- [ ] **Step 3: Install dependencies into the venv.** Run:
  ```bash
  cd /home/jake/project/help-my-run/garmin-worker && ./.venv/bin/python -m pip install --upgrade pip && ./.venv/bin/python -m pip install -r requirements.txt
  ```
  Expected output: ends with `Successfully installed ... garminconnect-0.3.6 ... curl-cffi-...` (plus transitive deps). (If `garminconnect==0.3.6` is unavailable, install the latest `garminconnect` instead — the library identity matters, not the patch — and update `requirements.txt` to the resolved version.)

- [ ] **Step 4: Write a minimal argparse stub `worker.py`.** Create `/home/jake/project/help-my-run/garmin-worker/worker.py` with exactly:
  ```python
  #!/usr/bin/env python3
  """help-my-run Garmin worker (scaffold stub).

  Real subcommands (`login`, `fetch --since YYYY-MM-DD`) are implemented by the
  WORKER tasks. This stub only establishes the argparse surface so scaffolding
  verification (`worker.py --help`) succeeds.
  """
  import argparse
  import sys


  def main() -> int:
      parser = argparse.ArgumentParser(prog="worker.py", description="help-my-run Garmin worker")
      sub = parser.add_subparsers(dest="command", required=True)
      sub.add_parser("login", help="interactive one-time Garmin login (MFA-aware)")
      fetch = sub.add_parser("fetch", help="fetch recovery data and print JSON to stdout")
      fetch.add_argument("--since", required=True, metavar="YYYY-MM-DD", help="inclusive start date")
      args = parser.parse_args()
      print(f"scaffold stub: command={args.command}", file=sys.stderr)
      return 0


  if __name__ == "__main__":
      raise SystemExit(main())
  ```

- [ ] **Step 5: Verify deps import and the CLI parses (the "test").** Run:
  ```bash
  cd /home/jake/project/help-my-run/garmin-worker && ./.venv/bin/python -c "import garminconnect, curl_cffi; print('IMPORTS_OK')" && ./.venv/bin/python worker.py --help | head -1
  ```
  Expected output:
  ```
  IMPORTS_OK
  usage: worker.py [-h] {login,fetch} ...
  ```

- [ ] **Step 6: Confirm the venv is git-ignored (must NOT be committed).** Run:
  ```bash
  cd /home/jake/project/help-my-run && git check-ignore garmin-worker/.venv/bin/python && echo "VENV_IGNORED"
  ```
  Expected output:
  ```
  garmin-worker/.venv/bin/python
  VENV_IGNORED
  ```

- [ ] **Step 7: Commit (requirements + worker stub only; venv excluded).** Run:
  ```bash
  cd /home/jake/project/help-my-run && git add garmin-worker/requirements.txt garmin-worker/worker.py && git commit -m "chore(scaffold): add garmin-worker requirements (garminconnect, curl_cffi) and CLI stub"
  ```
  Expected output: a commit summary listing `requirements.txt` and `worker.py`.

---

### Task 5: Create the Expo app via create-expo-app and add runtime + dev dependencies

**Files:**
- Create: `/home/jake/project/help-my-run/app/` (full Expo project — `package.json`, `app.json`, `tsconfig.json`, `app/` router dir, etc.)
- Test: `npx expo-doctor` and `ls` of generated files

- [ ] **Step 1: Scaffold the Expo app (TypeScript + expo-router default template) into `app/`.** Run from the repo root so the project lands at `help-my-run/app/`:
  ```bash
  cd /home/jake/project/help-my-run && npx --yes create-expo-app@latest app --template default --no-install
  ```
  Expected output: ends with a "Your project is ready!" message. The `default` template is TypeScript + expo-router. `--no-install` defers npm install so the next step installs everything in one pass.

- [ ] **Step 2: Flatten the router tree to a known minimal structure.** The `default` template ships an `app/(tabs)/` route group (`index.tsx`, `explore.tsx`), an `app/_layout.tsx`, and `app/+not-found.tsx` — but this plan assumes a FLAT `app/app/` with just `_layout.tsx`, `index.tsx`, and (later) `settings.tsx`. Use the template's official reset script, which moves the starter into `app-example/` and writes a minimal flat `app/` (`_layout.tsx` + `index.tsx`), then delete the leftover example dir. Run:
  ```bash
  cd /home/jake/project/help-my-run/app && npm run reset-project && rm -rf app-example
  ```
  Chosen approach: `reset-project` (official). Expected output: the script prints that it moved the existing `app/` into `app-example/` and created a fresh minimal `app/`. **Fallback** if `reset-project` is unavailable on your template version: `rm -rf "app/(tabs)" app/+not-found.tsx` and ensure a flat `app/_layout.tsx` and `app/index.tsx` remain (create minimal ones if the reset removed them).

- [ ] **Step 3: Verify the flattened `app/` structure.** Run:
  ```bash
  cd /home/jake/project/help-my-run/app && ls app && test ! -d "app/(tabs)" && test ! -f app/+not-found.tsx && test -f app/_layout.tsx && test -f app/index.tsx && echo "FLAT_OK"
  ```
  Expected output: a listing showing exactly `_layout.tsx` and `index.tsx` (no `(tabs)/` group, no `+not-found.tsx`), followed by `FLAT_OK`.

- [ ] **Step 4: Install the base template deps plus the four runtime dependencies.** Run:
  ```bash
  cd /home/jake/project/help-my-run/app && npm install && npx --yes expo install @tanstack/react-query expo-secure-store expo-web-browser
  ```
  Expected output: `npm install` finishes with an `added N packages` line; `expo install` prints `› Installing N SDK 56.0.0 compatible native modules` and the four packages resolved. (`expo install` pins SDK-compatible versions, unlike plain `npm install`.) NOTE: `create-expo-app@latest` now ships **Expo SDK 56** (React 19.2.x, RNTL v14); references to "SDK 55" elsewhere in this plan predate that bump — the literal `@latest` command is authoritative and SDK 56 is correct.

- [ ] **Step 5: Add the dev dependencies for testing (jest-expo + Testing Library).** Run:
  ```bash
  cd /home/jake/project/help-my-run/app && npx --yes expo install --dev jest-expo jest @testing-library/react-native react-test-renderer test-renderer @types/jest
  ```
  Expected output: an `added N packages` line listing `jest-expo`, `@testing-library/react-native`, and the rest under devDependencies. NOTES on dev deps under SDK 56 / React 19 (reconciled with reality):
  - **`@testing-library/react-hooks` is intentionally omitted.** That standalone package was deprecated and its last release (`8.0.1`) peer-requires `react@^16.9.0 || ^17.0.0`, so it is uninstallable and unusable on React 19. Its `renderHook` has been merged into `@testing-library/react-native` v14 (`import { renderHook } from '@testing-library/react-native'`), which is the official replacement. Use that for hook tests.
  - **`test-renderer` (`~1.2.0`) is required** because `@testing-library/react-native` v14 declares `test-renderer@^1.0.0` (the universal renderer) as a *peerDependency* — npm does not auto-install peers, so it must be added explicitly or RNTL's `render`/`screen` fail with "Cannot find module 'test-renderer'".
  - **`react-test-renderer` (pin to the project's React version, e.g. `react-test-renderer@19.2.3`)** is `jest-expo`'s own dependency and is referenced by the jest-expo preset; pin it to match `react` exactly (the bare `react-test-renderer@*` that `expo install` would otherwise resolve picks a newer patch that conflicts with the SDK-pinned `react`).

- [ ] **Step 6: Set the app scheme/name/slug and wire the `test` script.** Read `/home/jake/project/help-my-run/app/app.json` and ensure `expo.scheme` is `"helpmyrun"`, `expo.name` is `"help-my-run"`, `expo.slug` is `"help-my-run"` (edit the values create-expo-app generated). Then read `/home/jake/project/help-my-run/app/package.json` and add to the `"scripts"` object:
  ```json
  "test": "jest"
  ```
  and add a top-level `"jest"` key:
  ```json
  "jest": { "preset": "jest-expo" }
  ```
  (Use the `Edit` tool against the exact existing JSON; do not hand-rewrite the whole file.)

- [ ] **Step 7: Verify scheme, name, and the test script landed.** Run:
  ```bash
  cd /home/jake/project/help-my-run/app && node -e "const a=require('./app.json').expo; const p=require('./package.json'); console.log('scheme='+a.scheme, 'name='+a.name, 'slug='+a.slug, 'test='+p.scripts.test, 'preset='+(p.jest&&p.jest.preset))"
  ```
  Expected output:
  ```
  scheme=helpmyrun name=help-my-run slug=help-my-run test=jest preset=jest-expo
  ```

- [ ] **Step 8: Run expo-doctor (the "test").** Run:
  ```bash
  cd /home/jake/project/help-my-run/app && npx --yes expo-doctor
  ```
  Expected output: ends with `15/15 checks passed. No issues detected!` (count may vary by expo-doctor version; the required signal is `No issues detected!` and a zero exit code). If a check flags a dependency-version mismatch, run the `npx expo install --check` fix it suggests, then re-run expo-doctor until clean.

- [ ] **Step 9: Confirm the four runtime deps and key dev deps are present in `package.json`.** Run:
  ```bash
  cd /home/jake/project/help-my-run/app && node -e "const p=require('./package.json'); const d={...p.dependencies,...p.devDependencies}; ['@tanstack/react-query','expo-secure-store','expo-web-browser','jest-expo','@testing-library/react-native'].forEach(k=>console.log((d[k]?'OK  ':'MISS ')+k))"
  ```
  Expected output: five `OK` lines, no `MISS`:
  ```
  OK  @tanstack/react-query
  OK  expo-secure-store
  OK  expo-web-browser
  OK  jest-expo
  OK  @testing-library/react-native
  ```

- [ ] **Step 10: Commit (node_modules is git-ignored).** Run:
  ```bash
  cd /home/jake/project/help-my-run && git add app && git commit -m "chore(scaffold): create Expo app (TS, expo-router) with react-query, secure-store, web-browser, jest-expo"
  ```
  Expected output: a commit summary; verify `node_modules` was excluded with `git -C /home/jake/project/help-my-run show --stat HEAD | grep -c node_modules` → `0`.

---

### Task 6: Create the root .env.example from the contracts env list

**Files:**
- Create: `/home/jake/project/help-my-run/.env.example`
- Test: presence-of-keys verification command

- [ ] **Step 1: Write `.env.example`.** Create `/home/jake/project/help-my-run/.env.example` with exactly (keys and order from Shared Contracts §4):
  ```dotenv
  # help-my-run — environment configuration.
  # Copy to .env and fill in. .env is git-ignored; never commit real secrets.

  # --- Strava OAuth (required) ---
  # Create a Strava API app at https://www.strava.com/settings/api
  STRAVA_CLIENT_ID=123456
  STRAVA_CLIENT_SECRET=replace-with-your-strava-client-secret
  # Must EXACTLY match the callback registered in the Strava app settings
  # and the redirect_uri the backend builds. Points at /api/strava/callback.
  STRAVA_REDIRECT_URL=http://localhost:8080/api/strava/callback

  # --- App <-> backend auth (required) ---
  # Bearer token the Expo app sends on every protected endpoint.
  # Generate a long random value, e.g. `openssl rand -hex 32`.
  API_TOKEN=replace-with-a-long-random-string

  # --- Backend storage / server (optional, defaults shown) ---
  DB_PATH=./helpmyrun.db
  PORT=8080

  # --- Garmin Connect (required for Garmin recovery data) ---
  # Used by the one-time `worker.py login`; passed through to the worker by Go.
  GARMIN_EMAIL=you@example.com
  GARMIN_PASSWORD=replace-with-your-garmin-password
  # Directory where the worker persists refreshed Garmin OAuth tokens.
  GARMIN_TOKENSTORE=~/.garminconnect

  # --- Worker invocation (optional, defaults resolve the venv) ---
  PYTHON_BIN=garmin-worker/.venv/bin/python
  WORKER_SCRIPT=garmin-worker/worker.py

  # --- Claude / Anthropic (STUB in M0; unused until M1) ---
  ANTHROPIC_API_KEY=replace-with-your-anthropic-key
  ```

- [ ] **Step 2: Verify every contract env key is documented (the "test").** Run:
  ```bash
  cd /home/jake/project/help-my-run && for k in STRAVA_CLIENT_ID STRAVA_CLIENT_SECRET STRAVA_REDIRECT_URL API_TOKEN DB_PATH PORT GARMIN_EMAIL GARMIN_PASSWORD GARMIN_TOKENSTORE PYTHON_BIN WORKER_SCRIPT ANTHROPIC_API_KEY; do grep -q "^$k=" .env.example && echo "OK   $k" || echo "MISS $k"; done
  ```
  Expected output: twelve `OK` lines, no `MISS`:
  ```
  OK   STRAVA_CLIENT_ID
  OK   STRAVA_CLIENT_SECRET
  OK   STRAVA_REDIRECT_URL
  OK   API_TOKEN
  OK   DB_PATH
  OK   PORT
  OK   GARMIN_EMAIL
  OK   GARMIN_PASSWORD
  OK   GARMIN_TOKENSTORE
  OK   PYTHON_BIN
  OK   WORKER_SCRIPT
  OK   ANTHROPIC_API_KEY
  ```

- [ ] **Step 3: Confirm `.env.example` is NOT ignored (it must be committed).** Run:
  ```bash
  cd /home/jake/project/help-my-run && git check-ignore -v .env.example; echo "exit=$?"
  ```
  Expected output: no path printed and `exit=1` (the `!.env.example` negation in `.gitignore` un-ignores it).

- [ ] **Step 4: Commit.** Run:
  ```bash
  cd /home/jake/project/help-my-run && git add .env.example && git commit -m "chore(scaffold): add .env.example documenting all M0 env vars"
  ```
  Expected output: a commit summary listing `.env.example`.

---
### Task 7: Create README + Makefile, then final scaffolding gate

**Files:**
- Create: `/home/jake/project/help-my-run/README.md`
- Create: `/home/jake/project/help-my-run/Makefile`
- Test: aggregate verification across all scaffolded components

- [ ] **Step 0a: Create the self-host `README.md`.** Write `/home/jake/project/help-my-run/README.md` with exactly:
````markdown
# help-my-run

A self-hostable, single-user AI running coach. It pulls your runs from **Strava** and your recovery data (sleep, HRV, Body Battery, resting HR) from **Garmin Connect** into a local database, then (in a later milestone) uses Claude to coach you. M0 delivers the data foundation: connect Strava, log in to Garmin once, sync, and view your runs + recovery in a small Expo app.

## Architecture

- **Go core** (`backend/`) owns the SQLite database, the REST API, and the periodic sync scheduler. It is the single source of truth.
- **Python Garmin worker** (`garmin-worker/`) is a thin, stateless subprocess that the Go core invokes to fetch Garmin data and print one JSON object to stdout. It is the only component that talks to Garmin.
- **Expo app** (`app/`) is the client. It stores your backend URL + API token in `expo-secure-store` and reads/writes the Go API over HTTP.

## Prerequisites

- **A Strava API application.** Create one at <https://www.strava.com/settings/api>. Copy the **Client ID** and **Client Secret** into `.env` (`STRAVA_CLIENT_ID`, `STRAVA_CLIENT_SECRET`). Set the application's **Authorization Callback Domain** to match `STRAVA_REDIRECT_URL` (e.g. `localhost`); the redirect URL must point at `/api/strava/callback`.
- **A Garmin Connect account** (email + password) for the one-time `worker.py login`.
- **An Anthropic API key** is **not needed for M0** — it is loaded but unused until M1. You can leave `ANTHROPIC_API_KEY` blank for now.
- Go 1.22+, Python 3.11+, and Node.js 18+ installed.

## Setup

```bash
git clone <your-fork-url> help-my-run
cd help-my-run

# 1. Configure secrets
cp .env.example .env
# edit .env and fill in STRAVA_CLIENT_ID, STRAVA_CLIENT_SECRET, STRAVA_REDIRECT_URL,
# API_TOKEN, GARMIN_EMAIL, GARMIN_PASSWORD (and any optional overrides)

# 2. Backend deps
cd backend && go mod download && cd ..

# 3. Garmin worker deps
cd garmin-worker && python -m venv .venv && . .venv/bin/activate && pip install -r requirements.txt && deactivate && cd ..

# 4. App deps
cd app && npm install && cd ..
```

## One-time Garmin login

Run the interactive login once. It will prompt for an **MFA code** if your Garmin account has multi-factor auth enabled. On success it persists OAuth tokens to `GARMIN_TOKENSTORE` (default `~/.garminconnect`) so subsequent syncs run non-interactively.

```bash
make garmin-login
```

## Running

```bash
make run-backend   # starts the Go API + periodic sync ticker on $PORT (default 8080)
make run-app       # starts the Expo dev server (open in Expo Go or a dev build)
```

In the app's Settings screen, enter the backend URL (e.g. `http://<your-LAN-ip>:8080`) and your `API_TOKEN`, then connect Strava.

## Syncing

```bash
make sync          # POSTs /api/sync (the backend must be running)
```

## Testing

```bash
make test          # runs the Go, Python worker, and Expo app test suites
```

## Security note

All secrets live in `.env`, which is **gitignored**. Never commit credentials (Strava secret, API token, Garmin password) or your Garmin token directory. Review `.gitignore` before pushing.

## Disclaimer

Garmin access uses the unofficial [`python-garminconnect`](https://github.com/cyberjunky/python-garminconnect) library — Garmin provides no public API for this data. Use it only for **personal access to your own account**. It may break at any time if Garmin changes their site, and you are responsible for complying with Garmin's terms of service.
````

- [ ] **Step 0b: Create the `Makefile`.** Write `/home/jake/project/help-my-run/Makefile` with exactly:
```makefile
# Load .env so targets can read PORT, API_TOKEN, etc.
-include .env
export

.PHONY: run-backend garmin-login sync run-app test

run-backend:
	cd backend && go run ./cmd/server

# One-time interactive Garmin login (MFA-aware). Persists tokens to GARMIN_TOKENSTORE.
garmin-login:
	cd garmin-worker && . .venv/bin/activate && python worker.py login

# Trigger a sync against the running backend (the backend must be running).
sync:
	curl -fsS -X POST -H "Authorization: Bearer $(API_TOKEN)" http://localhost:$(PORT)/api/sync
	@echo

run-app:
	cd app && npx expo start

# Run all three test suites: Go core, Python worker, Expo app.
test:
	cd backend && go test ./...
	cd garmin-worker && . .venv/bin/activate && python -m pytest tests -q
	cd app && npm test -- --watchAll=false
```

- [ ] **Step 1: Verify the Go backend still builds.** Run:
  ```bash
  cd /home/jake/project/help-my-run/backend && go build ./... && echo "GO_BUILD_OK"
  ```
  Expected output:
  ```
  GO_BUILD_OK
  ```

- [ ] **Step 2: Verify the Python worker CLI parses.** Run:
  ```bash
  cd /home/jake/project/help-my-run/garmin-worker && ./.venv/bin/python worker.py --help >/dev/null && echo "WORKER_CLI_OK"
  ```
  Expected output:
  ```
  WORKER_CLI_OK
  ```

- [ ] **Step 3: Verify the Expo app passes expo-doctor.** Run:
  ```bash
  cd /home/jake/project/help-my-run/app && npx --yes expo-doctor >/dev/null 2>&1 && echo "EXPO_DOCTOR_OK" || echo "EXPO_DOCTOR_ISSUES"
  ```
  Expected output:
  ```
  EXPO_DOCTOR_OK
  ```

- [ ] **Step 4: Verify all root scaffolding files exist.** Run:
  ```bash
  for f in .env.example .gitignore Makefile README.md backend/go.mod garmin-worker/requirements.txt app/package.json; do test -f "$f" && echo "OK   $f" || echo "MISS $f"; done
  ```
  Expected output: seven `OK` lines, no `MISS`:
  ```
  OK   .env.example
  OK   .gitignore
  OK   Makefile
  OK   README.md
  OK   backend/go.mod
  OK   garmin-worker/requirements.txt
  OK   app/package.json
  ```

- [ ] **Step 5: Verify no secrets or heavy artifacts were committed.** Run:
  ```bash
  cd /home/jake/project/help-my-run && git ls-files | grep -E '(^|/)\.env$|\.db$|/node_modules/|/\.venv/|/\.garminconnect/' | head; echo "exit=$?"
  ```
  Expected output: no file paths printed and `exit=1` (grep found no matches — nothing leaked).

- [ ] **Step 6: Commit the README and Makefile.** Run:
  ```bash
  cd /home/jake/project/help-my-run && git add README.md Makefile && git commit -m "docs: add README and Makefile"
  ```
  Expected output: a commit summary listing `README.md` and `Makefile`.

- [ ] **Step 7: Confirm the working tree is clean (all scaffolding committed).** Run:
  ```bash
  cd /home/jake/project/help-my-run && git status --porcelain && echo "CLEAN"
  ```
  Expected output: just `CLEAN` (porcelain prints nothing when clean). If anything is uncommitted, return to the owning task and commit it.

**Scaffolding notes for downstream tasks:**
- The temporary stubs created in scaffolding (`backend/cmd/server/main.go`, `garmin-worker/worker.py`) exist ONLY so `go build ./...` and `worker.py --help` pass during scaffolding. Task 29 (`cmd/server/main.go`) and Tasks 14–16 (the Python worker) OVERWRITE them with the real implementations from the contracts; they are not additive.
- Module path uses the literal placeholder `USER` (`github.com/USER/help-my-run/backend`); the human self-hoster replaces `USER` with their GitHub username after forking. The Go tasks below use the module path `help-my-run/backend` for internal imports; if the SCAFFOLD-chosen `go.mod` module line differs, adjust all `help-my-run/backend/internal/...` import prefixes to match the actual module line.
- Pinned dependency versions are best-known-good as of the spec date; if a pin fails to resolve, fall back to latest of the same library and record the resolved version — library identity is the contract, not the exact patch.

---

### Task 8: Config loading (envconfig + godotenv)

**Files:**
- Create: `backend/internal/config/config.go`
- Test: `backend/internal/config/config_test.go`

- [ ] **Step 1: Write the failing test** for `config.Load()` covering required-var enforcement, defaults, and explicit values. Create `backend/internal/config/config_test.go`:
```go
package config

import (
	"os"
	"testing"
)

// setEnv sets the given env vars for the duration of the test and TRULY UNSETS
// any others that Load reads, so tests are hermetic. We must Unsetenv (not set
// to "") because envconfig's required check uses os.LookupEnv: a set-but-empty
// var counts as PRESENT and would defeat TestLoadMissingRequired.
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	all := []string{
		"STRAVA_CLIENT_ID", "STRAVA_CLIENT_SECRET", "STRAVA_REDIRECT_URL",
		"API_TOKEN", "DB_PATH", "PORT",
		"GARMIN_EMAIL", "GARMIN_PASSWORD", "GARMIN_TOKENSTORE",
		"PYTHON_BIN", "WORKER_SCRIPT", "ANTHROPIC_API_KEY",
	}
	for _, k := range all {
		// t.Setenv first to register restoration of the original value on
		// cleanup, then Unsetenv to actually clear it for this test.
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func requiredEnv() map[string]string {
	return map[string]string{
		"STRAVA_CLIENT_ID":     "123456",
		"STRAVA_CLIENT_SECRET": "secret",
		"STRAVA_REDIRECT_URL":  "http://localhost:8080/api/strava/callback",
		"API_TOKEN":            "tok",
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.StravaClientID != "123456" {
		t.Errorf("StravaClientID = %q, want %q", cfg.StravaClientID, "123456")
	}
	if cfg.DBPath != "./helpmyrun.db" {
		t.Errorf("DBPath = %q, want default %q", cfg.DBPath, "./helpmyrun.db")
	}
	if cfg.Port != "8080" {
		t.Errorf("Port = %q, want default %q", cfg.Port, "8080")
	}
	if cfg.GarminTokenstore != "~/.garminconnect" {
		t.Errorf("GarminTokenstore = %q, want default %q", cfg.GarminTokenstore, "~/.garminconnect")
	}
	if cfg.PythonBin != "garmin-worker/.venv/bin/python" {
		t.Errorf("PythonBin = %q, want default %q", cfg.PythonBin, "garmin-worker/.venv/bin/python")
	}
	if cfg.WorkerScript != "garmin-worker/worker.py" {
		t.Errorf("WorkerScript = %q, want default %q", cfg.WorkerScript, "garmin-worker/worker.py")
	}
}

func TestLoadExplicit(t *testing.T) {
	env := requiredEnv()
	env["DB_PATH"] = "/tmp/x.db"
	env["PORT"] = "9090"
	env["GARMIN_EMAIL"] = "you@example.com"
	env["GARMIN_PASSWORD"] = "pw"
	env["GARMIN_TOKENSTORE"] = "/tmp/gc"
	env["PYTHON_BIN"] = "/usr/bin/python3"
	env["WORKER_SCRIPT"] = "/srv/worker.py"
	env["ANTHROPIC_API_KEY"] = "sk-ant"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.DBPath != "/tmp/x.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/tmp/x.db")
	}
	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want %q", cfg.Port, "9090")
	}
	if cfg.GarminEmail != "you@example.com" {
		t.Errorf("GarminEmail = %q, want %q", cfg.GarminEmail, "you@example.com")
	}
	if cfg.PythonBin != "/usr/bin/python3" {
		t.Errorf("PythonBin = %q, want %q", cfg.PythonBin, "/usr/bin/python3")
	}
	if cfg.AnthropicAPIKey != "sk-ant" {
		t.Errorf("AnthropicAPIKey = %q, want %q", cfg.AnthropicAPIKey, "sk-ant")
	}
}

func TestLoadMissingRequired(t *testing.T) {
	env := requiredEnv()
	delete(env, "API_TOKEN")
	setEnv(t, env)

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for missing API_TOKEN")
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/config/`. Expected: compile failure `undefined: Load` / `undefined: Config` (package has no non-test source), printed as `FAIL backend/internal/config [build failed]`.

- [ ] **Step 3: Write the implementation.** Create `backend/internal/config/config.go`:
```go
// Package config loads and validates process configuration from the
// environment (optionally seeded from a .env file).
package config

import (
	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

// Config holds all runtime configuration. Field tags map to the env var names
// defined in the M0 contracts (§4).
type Config struct {
	StravaClientID     string `envconfig:"STRAVA_CLIENT_ID" required:"true"`
	StravaClientSecret string `envconfig:"STRAVA_CLIENT_SECRET" required:"true"`
	StravaRedirectURL  string `envconfig:"STRAVA_REDIRECT_URL" required:"true"`
	APIToken           string `envconfig:"API_TOKEN" required:"true"`

	DBPath string `envconfig:"DB_PATH" default:"./helpmyrun.db"`
	Port   string `envconfig:"PORT" default:"8080"`

	GarminEmail      string `envconfig:"GARMIN_EMAIL"`
	GarminPassword   string `envconfig:"GARMIN_PASSWORD"`
	GarminTokenstore string `envconfig:"GARMIN_TOKENSTORE" default:"~/.garminconnect"`

	PythonBin    string `envconfig:"PYTHON_BIN" default:"garmin-worker/.venv/bin/python"`
	WorkerScript string `envconfig:"WORKER_SCRIPT" default:"garmin-worker/worker.py"`

	AnthropicAPIKey string `envconfig:"ANTHROPIC_API_KEY"` // stub until M1
}

// Load reads .env (if present) into the process environment, then maps env
// vars into a Config. Missing required vars return an error.
func Load() (*Config, error) {
	_ = godotenv.Load() // no error if .env absent; real env still used
	var c Config
	if err := envconfig.Process("", &c); err != nil {
		return nil, err
	}
	return &c, nil
}
```

- [ ] **Step 4: Run the test, expect PASS.** Command: `cd backend && go test ./internal/config/`. Expected: `ok  	help-my-run/backend/internal/config  0.00Ns` (all three tests `--- PASS`).

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/config && git commit -m "feat(config): add envconfig-based Config.Load with defaults and required vars"
```

---
### Task 9: Store — open SQLite (modernc, WAL) + embedded goose migrations

**Files:**
- Create: `backend/internal/store/store.go`
- Create: `backend/internal/store/migrate.go`
- Create: `backend/internal/store/migrations/00001_init.sql`
- Test: `backend/internal/store/store_test.go`

- [ ] **Step 1: Write the migration file** (the canonical Shared Contracts §1 schema). Create `backend/internal/store/migrations/00001_init.sql`:
```sql
-- +goose Up
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

CREATE TABLE activities (
    strava_id        INTEGER PRIMARY KEY,
    name             TEXT    NOT NULL,
    type             TEXT    NOT NULL,
    sport_type       TEXT,
    start_time       TEXT    NOT NULL,
    start_time_local TEXT,
    distance_m       REAL    NOT NULL,
    moving_time_s    INTEGER NOT NULL,
    elapsed_time_s   INTEGER NOT NULL,
    avg_hr           REAL,
    max_hr           REAL,
    avg_speed        REAL,
    max_speed        REAL,
    avg_cadence      REAL,
    elevation_gain_m REAL,
    raw_json         TEXT    NOT NULL,
    synced_at        TEXT    NOT NULL
);

CREATE INDEX idx_activities_start_time ON activities (start_time DESC);

CREATE TABLE activity_splits (
    activity_id    INTEGER NOT NULL,
    idx            INTEGER NOT NULL,
    distance_m     REAL    NOT NULL,
    elapsed_time_s INTEGER NOT NULL,
    moving_time_s  INTEGER,
    avg_hr         REAL,
    max_hr         REAL,
    avg_speed      REAL,
    PRIMARY KEY (activity_id, idx),
    FOREIGN KEY (activity_id) REFERENCES activities (strava_id) ON DELETE CASCADE
);

CREATE TABLE garmin_sleep (
    date       TEXT    PRIMARY KEY,
    duration_s INTEGER,
    deep_s     INTEGER,
    light_s    INTEGER,
    rem_s      INTEGER,
    awake_s    INTEGER,
    score      INTEGER,
    raw_json   TEXT    NOT NULL
);

CREATE TABLE garmin_hrv (
    date              TEXT PRIMARY KEY,
    last_night_avg_ms INTEGER,
    status            TEXT,
    raw_json          TEXT NOT NULL
);

CREATE TABLE garmin_body_battery (
    date     TEXT PRIMARY KEY,
    charged  INTEGER,
    drained  INTEGER,
    high     INTEGER,
    low      INTEGER,
    raw_json TEXT NOT NULL
);

CREATE TABLE garmin_rhr (
    date       TEXT    PRIMARY KEY,
    resting_hr INTEGER,
    raw_json   TEXT    NOT NULL
);

CREATE TABLE sync_log (
    source         TEXT PRIMARY KEY,
    last_synced_at TEXT,
    last_run_at    TEXT,
    status         TEXT NOT NULL,
    error          TEXT
);

INSERT INTO sync_log (source, last_synced_at, last_run_at, status, error)
VALUES ('strava', NULL, NULL, 'never', NULL),
       ('garmin', NULL, NULL, 'never', NULL);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE sync_log;
DROP TABLE garmin_rhr;
DROP TABLE garmin_body_battery;
DROP TABLE garmin_hrv;
DROP TABLE garmin_sleep;
DROP TABLE activity_splits;
DROP TABLE activities;
DROP TABLE strava_tokens;
-- +goose StatementEnd
```

- [ ] **Step 2: Write the failing test** for opening + migrating a temp DB and asserting the schema landed (tables present, sync_log seeded). Create `backend/internal/store/store_test.go`:
```go
package store

import (
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
		"sync_log",
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
```

- [ ] **Step 3: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/store/`. Expected: build failure `undefined: Open` / `undefined: Store` → `FAIL backend/internal/store [build failed]`.

- [ ] **Step 4: Write `store.go`** (open the modernc driver with WAL DSN, single writer). Create `backend/internal/store/store.go`:
```go
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
```

- [ ] **Step 5: Write `migrate.go`** (embed migrations, run goose Up). Create `backend/internal/store/migrate.go`:
```go
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
```

- [ ] **Step 6: Run the test, expect PASS.** Command: `cd backend && go test ./internal/store/`. Expected: `ok  	help-my-run/backend/internal/store  0.0Ns` (`TestOpenAndMigrate`, `TestMigrateSeedsSyncLog`, `TestMigrateIdempotent` all `--- PASS`).

- [ ] **Step 7: Commit.** Command:
```
git add backend/internal/store && git commit -m "feat(store): open modernc SQLite (WAL) and run embedded goose migrations"
```

---
### Task 10: Store — strava_tokens get/set + sync_log get/update

**Files:**
- Create: `backend/internal/store/tokens.go`
- Create: `backend/internal/store/synclog.go`
- Modify: `backend/internal/store/store_test.go`

- [ ] **Step 1: Write the failing test** for `SaveStravaTokens`/`GetStravaTokens` (incl. not-found) and `GetSyncLog`/`UpdateSyncLog`. Append to `backend/internal/store/store_test.go`:
```go

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
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/store/ -run 'StravaTokens|SyncLog'`. Expected: build failure `undefined: StravaTokens`, `undefined: ErrNotFound`, `undefined: SyncLog` → `FAIL backend/internal/store [build failed]`.

- [ ] **Step 3: Write `tokens.go`.** Create `backend/internal/store/tokens.go`:
```go
package store

import (
	"database/sql"
	"errors"
	"time"
)

// ErrNotFound is returned by getters when no matching row exists.
var ErrNotFound = errors.New("store: not found")

// StravaTokens is the persisted OAuth token set (single row, id=1).
type StravaTokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    int64 // unix epoch seconds (Strava expires_at)
	Scope        string
	AthleteID    int64
}

// GetStravaTokens returns the single stored token row, or ErrNotFound.
func (s *Store) GetStravaTokens() (StravaTokens, error) {
	var t StravaTokens
	var scope sql.NullString
	var athleteID sql.NullInt64
	err := s.DB.QueryRow(`
		SELECT access_token, refresh_token, expires_at, scope, athlete_id
		FROM strava_tokens WHERE id = 1`).
		Scan(&t.AccessToken, &t.RefreshToken, &t.ExpiresAt, &scope, &athleteID)
	if errors.Is(err, sql.ErrNoRows) {
		return StravaTokens{}, ErrNotFound
	}
	if err != nil {
		return StravaTokens{}, err
	}
	t.Scope = scope.String
	t.AthleteID = athleteID.Int64
	return t, nil
}

// SaveStravaTokens upserts the single token row (id always 1).
func (s *Store) SaveStravaTokens(t StravaTokens) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO strava_tokens
			(id, access_token, refresh_token, expires_at, scope, athlete_id, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			access_token  = excluded.access_token,
			refresh_token = excluded.refresh_token,
			expires_at    = excluded.expires_at,
			scope         = excluded.scope,
			athlete_id    = excluded.athlete_id,
			updated_at    = excluded.updated_at`,
		t.AccessToken, t.RefreshToken, t.ExpiresAt, t.Scope, t.AthleteID, now)
	return err
}
```

- [ ] **Step 4: Write `synclog.go`.** Create `backend/internal/store/synclog.go`:
```go
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
```

- [ ] **Step 5: Run the test, expect PASS.** Command: `cd backend && go test ./internal/store/ -run 'StravaTokens|SyncLog'`. Expected: `ok  	help-my-run/backend/internal/store` (`TestStravaTokensRoundTrip`, `TestSyncLogGetAndUpdate` `--- PASS`).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/store && git commit -m "feat(store): add strava_tokens and sync_log typed get/upsert functions"
```

---
### Task 11: Store — activities upsert/list + splits upsert

**Files:**
- Create: `backend/internal/store/activities.go`
- Modify: `backend/internal/store/store_test.go`

- [ ] **Step 1: Write the failing test** for `UpsertActivity` (insert + update via re-upsert), `ListActivities` (order + limit), and `UpsertSplits`. Append to `backend/internal/store/store_test.go`:
```go

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
```

- [ ] **Step 2: Add shared test helpers** `strp` and `i64p` (used across store tests). Append to `backend/internal/store/store_test.go`:
```go

func strp(v string) *string { return &v }
func i64p(v int64) *int64    { return &v }
```

- [ ] **Step 3: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/store/ -run 'Activities|Splits'`. Expected: build failure `undefined: Activity`, `undefined: Split` → `FAIL backend/internal/store [build failed]`.

- [ ] **Step 4: Write `activities.go`.** Create `backend/internal/store/activities.go`:
```go
package store

import "time"

// Activity is a normalized Strava run (one row in activities).
type Activity struct {
	StravaID       int64
	Name           string
	Type           string
	SportType      *string
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

// Split is one Strava lap mapped into activity_splits.
type Split struct {
	ActivityID   int64
	Idx          int64
	DistanceM    float64
	ElapsedTimeS int64
	MovingTimeS  *int64
	AvgHR        *float64
	MaxHR        *float64
	AvgSpeed     *float64
}

// UpsertActivity inserts or updates an activity by strava_id.
func (s *Store) UpsertActivity(a Activity) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO activities (
			strava_id, name, type, sport_type, start_time, start_time_local,
			distance_m, moving_time_s, elapsed_time_s,
			avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m,
			raw_json, synced_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(strava_id) DO UPDATE SET
			name=excluded.name, type=excluded.type, sport_type=excluded.sport_type,
			start_time=excluded.start_time, start_time_local=excluded.start_time_local,
			distance_m=excluded.distance_m, moving_time_s=excluded.moving_time_s,
			elapsed_time_s=excluded.elapsed_time_s,
			avg_hr=excluded.avg_hr, max_hr=excluded.max_hr, avg_speed=excluded.avg_speed,
			max_speed=excluded.max_speed, avg_cadence=excluded.avg_cadence,
			elevation_gain_m=excluded.elevation_gain_m,
			raw_json=excluded.raw_json, synced_at=excluded.synced_at`,
		a.StravaID, a.Name, a.Type, a.SportType, a.StartTime, a.StartTimeLocal,
		a.DistanceM, a.MovingTimeS, a.ElapsedTimeS,
		a.AvgHR, a.MaxHR, a.AvgSpeed, a.MaxSpeed, a.AvgCadence, a.ElevationGainM,
		a.RawJSON, now)
	return err
}

// ListActivities returns up to limit activities, most-recent-first by start_time.
// raw_json is intentionally not loaded (not needed by the list response).
func (s *Store) ListActivities(limit int) ([]Activity, error) {
	rows, err := s.DB.Query(`
		SELECT strava_id, name, type, sport_type, start_time, start_time_local,
		       distance_m, moving_time_s, elapsed_time_s,
		       avg_hr, max_hr, avg_speed, max_speed, avg_cadence, elevation_gain_m
		FROM activities
		ORDER BY start_time DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(
			&a.StravaID, &a.Name, &a.Type, &a.SportType, &a.StartTime, &a.StartTimeLocal,
			&a.DistanceM, &a.MovingTimeS, &a.ElapsedTimeS,
			&a.AvgHR, &a.MaxHR, &a.AvgSpeed, &a.MaxSpeed, &a.AvgCadence, &a.ElevationGainM,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpsertSplits upserts all splits for an activity (by activity_id+idx PK).
func (s *Store) UpsertSplits(activityID int64, splits []Split) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO activity_splits (
			activity_id, idx, distance_m, elapsed_time_s, moving_time_s,
			avg_hr, max_hr, avg_speed
		) VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(activity_id, idx) DO UPDATE SET
			distance_m=excluded.distance_m, elapsed_time_s=excluded.elapsed_time_s,
			moving_time_s=excluded.moving_time_s, avg_hr=excluded.avg_hr,
			max_hr=excluded.max_hr, avg_speed=excluded.avg_speed`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sp := range splits {
		if _, err := stmt.Exec(
			sp.ActivityID, sp.Idx, sp.DistanceM, sp.ElapsedTimeS, sp.MovingTimeS,
			sp.AvgHR, sp.MaxHR, sp.AvgSpeed,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
```

- [ ] **Step 5: Run the test, expect PASS.** Command: `cd backend && go test ./internal/store/ -run 'Activities|Splits'`. Expected: `ok  	help-my-run/backend/internal/store` (`TestUpsertAndListActivities`, `TestUpsertSplits` `--- PASS`).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/store && git commit -m "feat(store): add activities upsert/list and splits upsert"
```

---
### Task 12: Store — four garmin_* upserts + ListRecovery merge

**Files:**
- Create: `backend/internal/store/garmin.go`
- Modify: `backend/internal/store/store_test.go`

- [ ] **Step 1: Write the failing test** for `UpsertSleep/Hrv/BodyBattery/Rhr`, the distinct-dates count used by `/api/status`, and `ListRecovery` merging the four tables by date with nulls for missing sources. Append to `backend/internal/store/store_test.go`:
```go

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
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/store/ -run 'GarminAndListRecovery'`. Expected: build failure `undefined: SleepRow`, `undefined: HrvRow`, `undefined: BodyBatteryRow`, `undefined: RhrRow` → `FAIL backend/internal/store [build failed]`.

- [ ] **Step 3: Write `garmin.go`.** Create `backend/internal/store/garmin.go`:
```go
package store

// SleepRow maps to garmin_sleep.
type SleepRow struct {
	Date      string
	DurationS *int64
	DeepS     *int64
	LightS    *int64
	RemS      *int64
	AwakeS    *int64
	Score     *int64
	RawJSON   string
}

// HrvRow maps to garmin_hrv.
type HrvRow struct {
	Date           string
	LastNightAvgMs *int64
	Status         *string
	RawJSON        string
}

// BodyBatteryRow maps to garmin_body_battery.
type BodyBatteryRow struct {
	Date    string
	Charged *int64
	Drained *int64
	High    *int64
	Low     *int64
	RawJSON string
}

// RhrRow maps to garmin_rhr.
type RhrRow struct {
	Date      string
	RestingHR *int64
	RawJSON   string
}

// SleepFields is the recovery sub-record for sleep (no date/raw_json).
type SleepFields struct {
	DurationS *int64
	DeepS     *int64
	LightS    *int64
	RemS      *int64
	AwakeS    *int64
	Score     *int64
}

// HrvFields is the recovery sub-record for hrv.
type HrvFields struct {
	LastNightAvgMs *int64
	Status         *string
}

// BodyBatteryFields is the recovery sub-record for body battery.
type BodyBatteryFields struct {
	Charged *int64
	Drained *int64
	High    *int64
	Low     *int64
}

// RhrFields is the recovery sub-record for resting HR.
type RhrFields struct {
	RestingHR *int64
}

// RecoveryDay is one merged calendar date across the four garmin_* tables.
// Any sub-record is nil when that source has no data for the date.
type RecoveryDay struct {
	Date        string
	Sleep       *SleepFields
	HRV         *HrvFields
	BodyBattery *BodyBatteryFields
	RHR         *RhrFields
}

// UpsertSleep upserts one garmin_sleep row by date.
func (s *Store) UpsertSleep(r SleepRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_sleep (date, duration_s, deep_s, light_s, rem_s, awake_s, score, raw_json)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			duration_s=excluded.duration_s, deep_s=excluded.deep_s, light_s=excluded.light_s,
			rem_s=excluded.rem_s, awake_s=excluded.awake_s, score=excluded.score,
			raw_json=excluded.raw_json`,
		r.Date, r.DurationS, r.DeepS, r.LightS, r.RemS, r.AwakeS, r.Score, r.RawJSON)
	return err
}

// UpsertHrv upserts one garmin_hrv row by date.
func (s *Store) UpsertHrv(r HrvRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_hrv (date, last_night_avg_ms, status, raw_json)
		VALUES (?,?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			last_night_avg_ms=excluded.last_night_avg_ms, status=excluded.status,
			raw_json=excluded.raw_json`,
		r.Date, r.LastNightAvgMs, r.Status, r.RawJSON)
	return err
}

// UpsertBodyBattery upserts one garmin_body_battery row by date.
func (s *Store) UpsertBodyBattery(r BodyBatteryRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_body_battery (date, charged, drained, high, low, raw_json)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			charged=excluded.charged, drained=excluded.drained, high=excluded.high,
			low=excluded.low, raw_json=excluded.raw_json`,
		r.Date, r.Charged, r.Drained, r.High, r.Low, r.RawJSON)
	return err
}

// UpsertRhr upserts one garmin_rhr row by date.
func (s *Store) UpsertRhr(r RhrRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_rhr (date, resting_hr, raw_json)
		VALUES (?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			resting_hr=excluded.resting_hr, raw_json=excluded.raw_json`,
		r.Date, r.RestingHR, r.RawJSON)
	return err
}

// CountRecoveryDays returns the number of distinct calendar dates present
// across all four garmin_* tables.
func (s *Store) CountRecoveryDays() (int, error) {
	var n int
	err := s.DB.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT date FROM garmin_sleep
			UNION SELECT date FROM garmin_hrv
			UNION SELECT date FROM garmin_body_battery
			UNION SELECT date FROM garmin_rhr
		)`).Scan(&n)
	return n, err
}

// ListRecovery returns up to `days` merged recovery records, most-recent-first
// by date, full-outer-joining the four garmin_* tables on date.
func (s *Store) ListRecovery(days int) ([]RecoveryDay, error) {
	rows, err := s.DB.Query(`
		WITH dates AS (
			SELECT date FROM garmin_sleep
			UNION SELECT date FROM garmin_hrv
			UNION SELECT date FROM garmin_body_battery
			UNION SELECT date FROM garmin_rhr
		)
		SELECT d.date,
			s.duration_s, s.deep_s, s.light_s, s.rem_s, s.awake_s, s.score,
			h.last_night_avg_ms, h.status,
			b.charged, b.drained, b.high, b.low,
			r.resting_hr,
			(s.date IS NOT NULL) AS has_sleep,
			(h.date IS NOT NULL) AS has_hrv,
			(b.date IS NOT NULL) AS has_bb,
			(r.date IS NOT NULL) AS has_rhr
		FROM dates d
		LEFT JOIN garmin_sleep        s ON s.date = d.date
		LEFT JOIN garmin_hrv          h ON h.date = d.date
		LEFT JOIN garmin_body_battery b ON b.date = d.date
		LEFT JOIN garmin_rhr          r ON r.date = d.date
		ORDER BY d.date DESC
		LIMIT ?`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []RecoveryDay
	for rows.Next() {
		var (
			rd                                       RecoveryDay
			durationS, deepS, lightS, remS, awakeS   *int64
			score, lastNightAvg, charged, drained    *int64
			high, low, restingHR                     *int64
			status                                   *string
			hasSleep, hasHrv, hasBB, hasRhr          bool
		)
		if err := rows.Scan(
			&rd.Date,
			&durationS, &deepS, &lightS, &remS, &awakeS, &score,
			&lastNightAvg, &status,
			&charged, &drained, &high, &low,
			&restingHR,
			&hasSleep, &hasHrv, &hasBB, &hasRhr,
		); err != nil {
			return nil, err
		}
		if hasSleep {
			rd.Sleep = &SleepFields{
				DurationS: durationS, DeepS: deepS, LightS: lightS,
				RemS: remS, AwakeS: awakeS, Score: score,
			}
		}
		if hasHrv {
			rd.HRV = &HrvFields{LastNightAvgMs: lastNightAvg, Status: status}
		}
		if hasBB {
			rd.BodyBattery = &BodyBatteryFields{Charged: charged, Drained: drained, High: high, Low: low}
		}
		if hasRhr {
			rd.RHR = &RhrFields{RestingHR: restingHR}
		}
		out = append(out, rd)
	}
	return out, rows.Err()
}
```
Note: the grouped `var (...)` block above is illustrative; column alignment may not be gofmt-canonical. After writing the file, run `gofmt -w backend/internal/store/garmin.go` to normalize alignment (cosmetic; the build/tests do not depend on it).

- [ ] **Step 4: Run the test, expect PASS.** Command: `cd backend && go test ./internal/store/ -run 'GarminAndListRecovery'`. Expected: `ok  	help-my-run/backend/internal/store` (`TestUpsertGarminAndListRecovery --- PASS`).

- [ ] **Step 5: Run the full store package** to confirm no regressions. Command: `cd backend && go test ./internal/store/`. Expected: `ok  	help-my-run/backend/internal/store` (all store tests PASS).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/store && git commit -m "feat(store): add garmin_* upserts, CountRecoveryDays, and ListRecovery merge"
```

---
### Task 13: Python worker — project layout + pure `normalize()` functions + fixture contract-shape tests

**Files:**
- Create: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/__init__.py`
- Create: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/normalize.py`
- Create: `/home/jake/project/help-my-run/garmin-worker/tests/__init__.py`
- Create: `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_sleep_2026-06-15.json`
- Create: `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_hrv_2026-06-15.json`
- Create: `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_body_battery_range.json`
- Create: `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_stats_2026-06-15.json`
- Create: `/home/jake/project/help-my-run/garmin-worker/pytest.ini`
- Test: `/home/jake/project/help-my-run/garmin-worker/tests/test_normalize.py`

Design constraint: every function in `normalize.py` is **pure** (input dict/list/str → output dict/list), no I/O, no network, no `garminconnect` import. This is the unit-testable core. The live client (Task 15) and CLI (Tasks 14/16) call these functions but the functions never call the client.

- [ ] **Step 1: Add `pytest` to requirements and install.** Append to `/home/jake/project/help-my-run/garmin-worker/requirements.txt` so the file reads exactly:
```
garminconnect==0.3.6
curl_cffi
pytest
```
Then run (exact command):
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/pip install -r /home/jake/project/help-my-run/garmin-worker/requirements.txt
```
Expected output ends with a line containing `Successfully installed` and `pytest` (or `Requirement already satisfied` for each).

- [ ] **Step 2: Create the pytest config so tests are discovered.** Write `/home/jake/project/help-my-run/garmin-worker/pytest.ini` with exactly:
```ini
[pytest]
testpaths = tests
python_files = test_*.py
addopts = -ra
```

- [ ] **Step 3: Create the four raw-response fixtures (recorded `garminconnect` shapes).** These mirror the verified upstream shapes in Shared Contracts §2.3. Write each file exactly.

`/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_sleep_2026-06-15.json`:
```json
{
  "dailySleepDTO": {
    "sleepTimeSeconds": 27000,
    "deepSleepSeconds": 6300,
    "lightSleepSeconds": 14400,
    "remSleepSeconds": 5400,
    "awakeSleepSeconds": 900,
    "sleepEndTimestampGMT": 1718434800000,
    "sleepScores": {"overall": {"value": 82}}
  }
}
```

`/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_hrv_2026-06-15.json`:
```json
{
  "hrvSummary": {
    "lastNightAvg": 48,
    "lastNight5MinHigh": 70,
    "weeklyAvg": 46,
    "status": "BALANCED"
  },
  "hrvReadings": []
}
```

`/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_body_battery_range.json`:
```json
[
  {
    "date": "2026-06-14",
    "charged": 60,
    "drained": 75,
    "bodyBatteryValuesArray": [[1718323200000, "ACTIVE", 88], [1718366400000, "ACTIVE", 16]]
  },
  {
    "date": "2026-06-15",
    "charged": 62,
    "drained": 78,
    "bodyBatteryValuesArray": [[1718409600000, "ACTIVE", 91], [1718452800000, "ACTIVE", 14]]
  }
]
```

`/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_stats_2026-06-15.json`:
```json
{
  "restingHeartRate": 47,
  "totalSteps": 11000
}
```

- [ ] **Step 4: Create empty package markers.** Write `/home/jake/project/help-my-run/garmin-worker/garmin_worker/__init__.py` with exactly (single line):
```python
"""help-my-run Garmin worker package."""
```
Write `/home/jake/project/help-my-run/garmin-worker/tests/__init__.py` as an empty file (zero bytes).

- [ ] **Step 5: Write the FAILING tests for `normalize.py`.** Write `/home/jake/project/help-my-run/garmin-worker/tests/test_normalize.py` with exactly:
```python
import json
import os

import pytest

from garmin_worker import normalize

FIXTURES = os.path.join(os.path.dirname(__file__), "fixtures")


def load(name):
    with open(os.path.join(FIXTURES, name), encoding="utf-8") as fh:
        return json.load(fh)


# --------------------------------------------------------------------------
# normalize_sleep_day
# --------------------------------------------------------------------------
def test_normalize_sleep_day_full_shape():
    raw = load("raw_sleep_2026-06-15.json")
    out = normalize.normalize_sleep_day("2026-06-15", raw)
    assert out == {
        "date": "2026-06-15",
        "duration_s": 27000,
        "deep_s": 6300,
        "light_s": 14400,
        "rem_s": 5400,
        "awake_s": 900,
        "score": 82,
        "raw_json": raw,
    }
    assert list(out.keys()) == [
        "date", "duration_s", "deep_s", "light_s",
        "rem_s", "awake_s", "score", "raw_json",
    ]


def test_normalize_sleep_day_missing_fields_become_null():
    raw = {"dailySleepDTO": {"sleepTimeSeconds": 26400}}
    out = normalize.normalize_sleep_day("2026-06-15", raw)
    assert out["date"] == "2026-06-15"
    assert out["duration_s"] == 26400
    assert out["deep_s"] is None
    assert out["light_s"] is None
    assert out["rem_s"] is None
    assert out["awake_s"] is None
    assert out["score"] is None
    assert out["raw_json"] == raw


def test_normalize_sleep_day_no_dto_all_null():
    raw = {}
    out = normalize.normalize_sleep_day("2026-06-15", raw)
    assert out["duration_s"] is None
    assert out["score"] is None
    assert out["raw_json"] == {}


# --------------------------------------------------------------------------
# normalize_hrv_day  (get_hrv_data may return None -> caller omits;
# normalizer is only called for non-None payloads)
# --------------------------------------------------------------------------
def test_normalize_hrv_day_full_shape():
    raw = load("raw_hrv_2026-06-15.json")
    out = normalize.normalize_hrv_day("2026-06-15", raw)
    assert out == {
        "date": "2026-06-15",
        "last_night_avg_ms": 48,
        "status": "BALANCED",
        "raw_json": raw,
    }
    assert list(out.keys()) == ["date", "last_night_avg_ms", "status", "raw_json"]


def test_normalize_hrv_day_missing_summary_null():
    raw = {"hrvReadings": []}
    out = normalize.normalize_hrv_day("2026-06-15", raw)
    assert out["last_night_avg_ms"] is None
    assert out["status"] is None
    assert out["raw_json"] == raw


# --------------------------------------------------------------------------
# normalize_body_battery_day  (one entry of the range list)
# --------------------------------------------------------------------------
def test_normalize_body_battery_day_full_shape():
    entry = load("raw_body_battery_range.json")[1]  # 2026-06-15
    out = normalize.normalize_body_battery_day("2026-06-15", entry)
    assert out == {
        "date": "2026-06-15",
        "charged": 62,
        "drained": 78,
        "high": 91,
        "low": 14,
        "raw_json": entry,
    }
    assert list(out.keys()) == [
        "date", "charged", "drained", "high", "low", "raw_json",
    ]


def test_normalize_body_battery_high_low_from_values_array():
    entry = {
        "date": "2026-06-15",
        "charged": 30,
        "drained": 40,
        "bodyBatteryValuesArray": [
            [1, "ACTIVE", 55], [2, "ACTIVE", 12], [3, "ACTIVE", 80],
        ],
    }
    out = normalize.normalize_body_battery_day("2026-06-15", entry)
    assert out["high"] == 80
    assert out["low"] == 12


def test_normalize_body_battery_empty_array_high_low_null():
    entry = {"date": "2026-06-15", "charged": None, "drained": None,
             "bodyBatteryValuesArray": []}
    out = normalize.normalize_body_battery_day("2026-06-15", entry)
    assert out["high"] is None
    assert out["low"] is None
    assert out["charged"] is None
    assert out["drained"] is None


def test_normalize_body_battery_missing_charged_drained_fallback_from_deltas():
    # No "charged"/"drained" keys -> derive from value deltas.
    entry = {
        "date": "2026-06-15",
        "bodyBatteryValuesArray": [
            [1, "ACTIVE", 50], [2, "ACTIVE", 60], [3, "ACTIVE", 45],
            [4, "ACTIVE", 70],
        ],
    }
    out = normalize.normalize_body_battery_day("2026-06-15", entry)
    # positive deltas: +10, +25 = 35 ; negative deltas: -15 = -15 -> drained 15
    assert out["charged"] == 35
    assert out["drained"] == 15
    assert out["high"] == 70
    assert out["low"] == 45


# --------------------------------------------------------------------------
# normalize_rhr_day  (source: get_stats(date)["restingHeartRate"])
# --------------------------------------------------------------------------
def test_normalize_rhr_day_full_shape():
    raw = load("raw_stats_2026-06-15.json")
    out = normalize.normalize_rhr_day("2026-06-15", raw)
    assert out == {
        "date": "2026-06-15",
        "resting_hr": 47,
        "raw_json": raw,
    }
    assert list(out.keys()) == ["date", "resting_hr", "raw_json"]


def test_normalize_rhr_day_missing_rhr_null():
    raw = {"totalSteps": 9000}
    out = normalize.normalize_rhr_day("2026-06-15", raw)
    assert out["resting_hr"] is None
    assert out["raw_json"] == raw


def test_normalize_rhr_day_none_raw_yields_null():
    out = normalize.normalize_rhr_day("2026-06-15", None)
    assert out["resting_hr"] is None
    assert out["raw_json"] is None


# --------------------------------------------------------------------------
# build_output  (assembles the full §2.1 top-level object)
# --------------------------------------------------------------------------
def test_build_output_top_level_shape():
    out = normalize.build_output(
        since="2026-06-14",
        until="2026-06-15",
        fetched_at="2026-06-15T05:00:12Z",
        sleep=[{"date": "2026-06-14"}],
        hrv=[],
        body_battery=[{"date": "2026-06-14"}, {"date": "2026-06-15"}],
        rhr=[{"date": "2026-06-15"}],
    )
    assert list(out.keys()) == [
        "since", "until", "fetched_at",
        "sleep", "hrv", "body_battery", "rhr",
    ]
    assert out["since"] == "2026-06-14"
    assert out["until"] == "2026-06-15"
    assert out["fetched_at"] == "2026-06-15T05:00:12Z"
    assert out["hrv"] == []
    assert len(out["body_battery"]) == 2
    assert out["sleep"][0]["date"] == "2026-06-14"
    assert out["rhr"][0]["date"] == "2026-06-15"


def test_build_output_full_serializes_to_json():
    out = normalize.build_output(
        since="2026-06-15", until="2026-06-15",
        fetched_at="2026-06-15T05:00:12Z",
        sleep=[], hrv=[], body_battery=[], rhr=[],
    )
    # must be JSON-serializable (no datetime / non-primitive leaks)
    text = json.dumps(out)
    again = json.loads(text)
    assert again["since"] == "2026-06-15"
```

- [ ] **Step 6: Run the tests and confirm they FAIL (no implementation yet).** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_normalize.py
```
Expected: collection error / failure with `ModuleNotFoundError: No module named 'garmin_worker.normalize'` (or `ImportError`), summary line ending in `errors` / `1 error`. No tests pass.

- [ ] **Step 7: Write the minimal implementation `normalize.py`.** Write `/home/jake/project/help-my-run/garmin-worker/garmin_worker/normalize.py` with exactly:
```python
"""Pure normalization functions: raw python-garminconnect responses -> contract JSON.

These functions perform NO I/O and import NO garminconnect symbols. They are the
unit-testable core of the worker (CONTRACTS §2.2). The live client (client.py) and
the CLI (worker.py) call these, never the reverse.
"""
from __future__ import annotations

from typing import Any, Optional


def _get(d: Optional[dict], *path: str) -> Any:
    """Safely walk a nested dict by keys; return None on any miss / non-dict."""
    cur: Any = d
    for key in path:
        if not isinstance(cur, dict):
            return None
        cur = cur.get(key)
    return cur


def normalize_sleep_day(date: str, raw: Optional[dict]) -> dict:
    """Map get_sleep_data(date) -> SleepDay (CONTRACTS §2.2)."""
    dto = _get(raw, "dailySleepDTO") or {}
    return {
        "date": date,
        "duration_s": dto.get("sleepTimeSeconds"),
        "deep_s": dto.get("deepSleepSeconds"),
        "light_s": dto.get("lightSleepSeconds"),
        "rem_s": dto.get("remSleepSeconds"),
        "awake_s": dto.get("awakeSleepSeconds"),
        "score": _get(dto, "sleepScores", "overall", "value"),
        "raw_json": raw if raw is not None else {},
    }


def normalize_hrv_day(date: str, raw: Optional[dict]) -> dict:
    """Map get_hrv_data(date) -> HrvDay (CONTRACTS §2.2).

    Caller is responsible for OMITTING dates where get_hrv_data returned None;
    this function is only invoked for non-None payloads.
    """
    summary = _get(raw, "hrvSummary") or {}
    return {
        "date": date,
        "last_night_avg_ms": summary.get("lastNightAvg"),
        "status": summary.get("status"),
        "raw_json": raw if raw is not None else {},
    }


def normalize_body_battery_day(date: str, entry: Optional[dict]) -> dict:
    """Map one entry of get_body_battery(since, until) -> BodyBatteryDay (CONTRACTS §2.2).

    high/low = max/min of bodyBatteryValuesArray values.
    charged/drained: direct keys if present, else derived from value deltas
    (sum of positive deltas = charged; abs(sum of negative deltas) = drained).
    """
    entry = entry or {}
    values = [
        row[2]
        for row in entry.get("bodyBatteryValuesArray") or []
        if isinstance(row, (list, tuple)) and len(row) >= 3 and row[2] is not None
    ]
    high = max(values) if values else None
    low = min(values) if values else None

    charged = entry.get("charged")
    drained = entry.get("drained")
    if charged is None and drained is None and len(values) >= 2:
        pos = 0
        neg = 0
        for prev, nxt in zip(values, values[1:]):
            delta = nxt - prev
            if delta > 0:
                pos += delta
            elif delta < 0:
                neg += delta
        charged = pos
        drained = -neg

    return {
        "date": date,
        "charged": charged,
        "drained": drained,
        "high": high,
        "low": low,
        "raw_json": entry,
    }


def normalize_rhr_day(date: str, raw: Optional[dict]) -> dict:
    """Map get_stats(date) -> RhrDay (CONTRACTS §2.2).

    Source path: get_stats(date)["restingHeartRate"] (confirmed; not get_rhr_day).
    """
    rhr = raw.get("restingHeartRate") if isinstance(raw, dict) else None
    return {
        "date": date,
        "resting_hr": rhr,
        "raw_json": raw,
    }


def build_output(
    *,
    since: str,
    until: str,
    fetched_at: str,
    sleep: list,
    hrv: list,
    body_battery: list,
    rhr: list,
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
    }
```

- [ ] **Step 8: Run the tests and confirm they PASS.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_normalize.py -q
```
Expected output ends with: `14 passed` (last line e.g. `14 passed in 0.0Xs`), exit code 0.

- [ ] **Step 9: Commit.** Exact command:
```bash
git -C /home/jake/project/help-my-run add garmin-worker/garmin_worker/__init__.py garmin-worker/garmin_worker/normalize.py garmin-worker/tests/__init__.py garmin-worker/tests/test_normalize.py garmin-worker/tests/fixtures/raw_sleep_2026-06-15.json garmin-worker/tests/fixtures/raw_hrv_2026-06-15.json garmin-worker/tests/fixtures/raw_body_battery_range.json garmin-worker/tests/fixtures/raw_stats_2026-06-15.json garmin-worker/pytest.ini garmin-worker/requirements.txt && git -C /home/jake/project/help-my-run commit -m "feat(worker): pure normalize() functions + fixture contract-shape tests"
```
Expected: a commit summary line showing 10 files changed.

---
### Task 14: Python worker — CLI arg parsing (argparse) + `--dry-run` mode + tests

**Files:**
- Create: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/cli.py`
- Create: `/home/jake/project/help-my-run/garmin-worker/worker.py`
- Create: `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/dry_run_expected.json`
- Test: `/home/jake/project/help-my-run/garmin-worker/tests/test_cli.py`

Design: `cli.py` owns argparse + the `--dry-run` path (which calls the pure functions on fixture-style synthetic data and `build_output`, no Garmin). `worker.py` is the executable thin entrypoint that delegates to `cli.main`. The live `fetch`/`login` wiring is added in Tasks 15 & 16; this task only proves the parser, the `--dry-run` JSON shape, and the stdout/stderr/exit-code discipline. (This `worker.py` OVERWRITES the scaffold stub from Task 4.)

- [ ] **Step 1: Write the expected `--dry-run` output fixture.** Write `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/dry_run_expected.json` with exactly:
```json
{
  "since": "2026-06-14",
  "until": "2026-06-15",
  "fetched_at": "2026-06-15T05:00:12Z",
  "sleep": [
    {"date": "2026-06-14", "duration_s": 26400, "deep_s": 6000, "light_s": 14100, "rem_s": 5400, "awake_s": 900, "score": 79, "raw_json": {"dailySleepDTO": {"sleepTimeSeconds": 26400, "deepSleepSeconds": 6000, "lightSleepSeconds": 14100, "remSleepSeconds": 5400, "awakeSleepSeconds": 900, "sleepScores": {"overall": {"value": 79}}}}},
    {"date": "2026-06-15", "duration_s": 27000, "deep_s": 6300, "light_s": 14400, "rem_s": 5400, "awake_s": 900, "score": 82, "raw_json": {"dailySleepDTO": {"sleepTimeSeconds": 27000, "deepSleepSeconds": 6300, "lightSleepSeconds": 14400, "remSleepSeconds": 5400, "awakeSleepSeconds": 900, "sleepScores": {"overall": {"value": 82}}}}}
  ],
  "hrv": [
    {"date": "2026-06-15", "last_night_avg_ms": 48, "status": "BALANCED", "raw_json": {"hrvSummary": {"lastNightAvg": 48, "lastNight5MinHigh": 70, "weeklyAvg": 46, "status": "BALANCED"}, "hrvReadings": []}}
  ],
  "body_battery": [
    {"date": "2026-06-14", "charged": 60, "drained": 75, "high": 88, "low": 16, "raw_json": {"date": "2026-06-14", "charged": 60, "drained": 75, "bodyBatteryValuesArray": [[1718323200000, "ACTIVE", 88], [1718366400000, "ACTIVE", 16]]}},
    {"date": "2026-06-15", "charged": 62, "drained": 78, "high": 91, "low": 14, "raw_json": {"date": "2026-06-15", "charged": 62, "drained": 78, "bodyBatteryValuesArray": [[1718409600000, "ACTIVE", 91], [1718452800000, "ACTIVE", 14]]}}
  ],
  "rhr": [
    {"date": "2026-06-14", "resting_hr": 48, "raw_json": {"restingHeartRate": 48, "totalSteps": 9000}},
    {"date": "2026-06-15", "resting_hr": 47, "raw_json": {"restingHeartRate": 47, "totalSteps": 11000}}
  ]
}
```

- [ ] **Step 2: Write the FAILING tests for the CLI.** Write `/home/jake/project/help-my-run/garmin-worker/tests/test_cli.py` with exactly:
```python
import io
import json
import os

import pytest

from garmin_worker import cli

FIXTURES = os.path.join(os.path.dirname(__file__), "fixtures")


def load(name):
    with open(os.path.join(FIXTURES, name), encoding="utf-8") as fh:
        return json.load(fh)


# --------------------------------------------------------------------------
# build_parser
# --------------------------------------------------------------------------
def test_parser_fetch_with_since():
    p = cli.build_parser()
    args = p.parse_args(["fetch", "--since", "2026-06-14"])
    assert args.command == "fetch"
    assert args.since == "2026-06-14"
    assert args.until is None
    assert args.dry_run is False


def test_parser_fetch_with_since_until_and_dry_run():
    p = cli.build_parser()
    args = p.parse_args(
        ["fetch", "--since", "2026-06-14", "--until", "2026-06-15", "--dry-run"]
    )
    assert args.command == "fetch"
    assert args.since == "2026-06-14"
    assert args.until == "2026-06-15"
    assert args.dry_run is True


def test_parser_login_command():
    p = cli.build_parser()
    args = p.parse_args(["login"])
    assert args.command == "login"


def test_parser_fetch_requires_since():
    p = cli.build_parser()
    with pytest.raises(SystemExit):
        p.parse_args(["fetch"])


def test_parser_no_command_errors():
    p = cli.build_parser()
    with pytest.raises(SystemExit):
        p.parse_args([])


# --------------------------------------------------------------------------
# validate_date
# --------------------------------------------------------------------------
def test_validate_date_ok():
    assert cli.validate_date("2026-06-14") == "2026-06-14"


@pytest.mark.parametrize("bad", ["2026-6-14", "06-14-2026", "2026/06/14", "nope", ""])
def test_validate_date_rejects_bad(bad):
    with pytest.raises(ValueError):
        cli.validate_date(bad)


# --------------------------------------------------------------------------
# --dry-run path: deterministic JSON to stdout, exit 0, nothing on stderr
# --------------------------------------------------------------------------
def test_main_dry_run_prints_contract_json(capsys):
    rc = cli.main(["fetch", "--since", "2026-06-14", "--until", "2026-06-15", "--dry-run"])
    assert rc == 0
    captured = capsys.readouterr()
    assert captured.err == ""
    out = json.loads(captured.out)  # must be exactly one parseable JSON object
    expected = load("dry_run_expected.json")
    assert out == expected


def test_main_dry_run_stdout_is_single_json_object(capsys):
    cli.main(["fetch", "--since", "2026-06-14", "--until", "2026-06-15", "--dry-run"])
    out = capsys.readouterr().out
    # exactly one top-level JSON value: json.loads on the whole buffer works,
    # and there is no trailing non-whitespace second document.
    decoder = json.JSONDecoder()
    obj, end = decoder.raw_decode(out.lstrip())
    assert out.lstrip()[end:].strip() == ""
    assert set(obj.keys()) == {
        "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr",
    }


def test_main_dry_run_bad_date_exits_nonzero_with_stderr(capsys):
    rc = cli.main(["fetch", "--since", "2026/06/14", "--dry-run"])
    assert rc != 0
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "2026/06/14" in captured.err or "date" in captured.err.lower()
```

- [ ] **Step 3: Run the tests and confirm they FAIL.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_cli.py
```
Expected: `ModuleNotFoundError: No module named 'garmin_worker.cli'`, summary `1 error` (collection error). No tests pass.

- [ ] **Step 4: Write the minimal `cli.py` implementing parser + `--dry-run`.** Write `/home/jake/project/help-my-run/garmin-worker/garmin_worker/cli.py` with exactly:
```python
"""CLI for the help-my-run Garmin worker (CONTRACTS §2).

Subcommands:
  login                       interactive one-time SSO; persists OAuth tokens
  fetch --since YYYY-MM-DD [--until YYYY-MM-DD] [--dry-run]
                              non-interactive; prints §2.1 JSON to stdout

Discipline (CONTRACTS §2 / §2.4):
  - stdout carries ONLY the single JSON object (or nothing on failure)
  - all diagnostics go to stderr
  - exit 0 on success; non-zero on auth/connection/validation failure
"""
from __future__ import annotations

import argparse
import datetime as _dt
import json
import sys
from typing import Optional, Sequence

from . import normalize

PROG = "worker.py"


def validate_date(value: str) -> str:
    """Accept exactly YYYY-MM-DD; raise ValueError otherwise."""
    try:
        parsed = _dt.datetime.strptime(value, "%Y-%m-%d").date()
    except (ValueError, TypeError) as exc:
        raise ValueError(f"invalid date {value!r}; expected YYYY-MM-DD") from exc
    return parsed.isoformat()


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog=PROG, description="help-my-run Garmin worker")
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("login", help="interactive one-time Garmin SSO login")

    fetch = sub.add_parser("fetch", help="fetch recovery metrics; print JSON to stdout")
    fetch.add_argument("--since", required=True, help="inclusive start date YYYY-MM-DD")
    fetch.add_argument("--until", default=None, help="inclusive end date YYYY-MM-DD (default: today)")
    fetch.add_argument(
        "--dry-run",
        action="store_true",
        help="emit synthetic contract JSON without contacting Garmin",
    )
    return parser


# ---- synthetic data for --dry-run (mirrors CONTRACTS §2.3) -----------------
_DRY_SLEEP_RAW = {
    "2026-06-14": {"dailySleepDTO": {"sleepTimeSeconds": 26400, "deepSleepSeconds": 6000, "lightSleepSeconds": 14100, "remSleepSeconds": 5400, "awakeSleepSeconds": 900, "sleepScores": {"overall": {"value": 79}}}},
    "2026-06-15": {"dailySleepDTO": {"sleepTimeSeconds": 27000, "deepSleepSeconds": 6300, "lightSleepSeconds": 14400, "remSleepSeconds": 5400, "awakeSleepSeconds": 900, "sleepScores": {"overall": {"value": 82}}}},
}
_DRY_HRV_RAW = {
    "2026-06-15": {"hrvSummary": {"lastNightAvg": 48, "lastNight5MinHigh": 70, "weeklyAvg": 46, "status": "BALANCED"}, "hrvReadings": []},
}
_DRY_BB_RANGE = [
    {"date": "2026-06-14", "charged": 60, "drained": 75, "bodyBatteryValuesArray": [[1718323200000, "ACTIVE", 88], [1718366400000, "ACTIVE", 16]]},
    {"date": "2026-06-15", "charged": 62, "drained": 78, "bodyBatteryValuesArray": [[1718409600000, "ACTIVE", 91], [1718452800000, "ACTIVE", 14]]},
]
_DRY_STATS_RAW = {
    "2026-06-14": {"restingHeartRate": 48, "totalSteps": 9000},
    "2026-06-15": {"restingHeartRate": 47, "totalSteps": 11000},
}


def _run_dry_fetch(since: str, until: str) -> dict:
    """Build the §2.1 object from baked-in synthetic data (no Garmin)."""
    sleep = [normalize.normalize_sleep_day(d, raw) for d, raw in sorted(_DRY_SLEEP_RAW.items())]
    hrv = [normalize.normalize_hrv_day(d, raw) for d, raw in sorted(_DRY_HRV_RAW.items())]
    body_battery = [normalize.normalize_body_battery_day(e["date"], e) for e in _DRY_BB_RANGE]
    rhr = [normalize.normalize_rhr_day(d, raw) for d, raw in sorted(_DRY_STATS_RAW.items())]
    return normalize.build_output(
        since=since,
        until=until,
        fetched_at="2026-06-15T05:00:12Z",
        sleep=sleep,
        hrv=hrv,
        body_battery=body_battery,
        rhr=rhr,
    )


def main(argv: Optional[Sequence[str]] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if args.command == "login":
        # Live login wiring is added in Task 15.
        print("login is not wired yet", file=sys.stderr)
        return 1

    # command == "fetch"
    try:
        since = validate_date(args.since)
        until = validate_date(args.until) if args.until else _dt.date.today().isoformat()
    except ValueError as exc:
        print(str(exc), file=sys.stderr)
        return 2

    if args.dry_run:
        output = _run_dry_fetch(since, until)
        json.dump(output, sys.stdout)
        sys.stdout.write("\n")
        return 0

    # Live fetch wiring is added in Task 16.
    print("live fetch is not wired yet", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 5: Write the executable entrypoint `worker.py`.** Write `/home/jake/project/help-my-run/garmin-worker/worker.py` with exactly:
```python
#!/usr/bin/env python3
"""help-my-run Garmin worker entrypoint (CONTRACTS §2).

Usage:
  python worker.py login
  python worker.py fetch --since YYYY-MM-DD [--until YYYY-MM-DD] [--dry-run]
"""
import sys

from garmin_worker.cli import main

if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 6: Run the CLI tests and confirm they PASS.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_cli.py -q
```
Expected output ends with: `13 passed` (last line e.g. `13 passed in 0.0Xs`), exit code 0.

- [ ] **Step 7: Smoke-test the executable produces parseable JSON.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python /home/jake/project/help-my-run/garmin-worker/worker.py fetch --since 2026-06-14 --until 2026-06-15 --dry-run | /home/jake/project/help-my-run/garmin-worker/.venv/bin/python -c "import sys,json; o=json.load(sys.stdin); print(sorted(o.keys()))"
```
Expected stdout exactly:
```
['body_battery', 'fetched_at', 'hrv', 'rhr', 'since', 'sleep', 'until']
```

- [ ] **Step 8: Commit.** Exact command:
```bash
git -C /home/jake/project/help-my-run add garmin-worker/garmin_worker/cli.py garmin-worker/worker.py garmin-worker/tests/test_cli.py garmin-worker/tests/fixtures/dry_run_expected.json && git -C /home/jake/project/help-my-run commit -m "feat(worker): argparse CLI + --dry-run mode emitting contract JSON"
```
Expected: commit summary showing 4 files changed.

---
### Task 15: Python worker — live Garmin client wrapper + `login` command

**Files:**
- Create: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/client.py`
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/cli.py`
- Test: `/home/jake/project/help-my-run/garmin-worker/tests/test_client.py`

Design: `client.py` is the ONLY module that imports `garminconnect`. It exposes a thin `GarminClient` class wrapping the verified method names (`get_sleep_data`, `get_hrv_data`, `get_body_battery`, `get_stats`) plus `login_interactive()` and `resume()`. The wrapper delegates 1:1 (no normalization here — normalization stays pure in Task 13). Unit tests cover only the env/tokenstore plumbing and the import surface; the actual SSO/HTTP call is explicitly NOT unit-tested (documented) since it requires live Garmin. The `login` command in `cli.py` is rewired to call `client.login_interactive()`.

- [ ] **Step 1: Write the FAILING tests for the client wrapper plumbing.** Write `/home/jake/project/help-my-run/garmin-worker/tests/test_client.py` with exactly:
```python
import os
import sys
import types

import pytest

from garmin_worker import client


# --------------------------------------------------------------------------
# tokenstore_path: expands ~ and honors GARMIN_TOKENSTORE (CONTRACTS §4 env)
# --------------------------------------------------------------------------
def test_tokenstore_path_default(monkeypatch):
    monkeypatch.delenv("GARMIN_TOKENSTORE", raising=False)
    monkeypatch.setenv("HOME", "/home/tester")
    assert client.tokenstore_path() == "/home/tester/.garminconnect"


def test_tokenstore_path_from_env(monkeypatch):
    monkeypatch.setenv("GARMIN_TOKENSTORE", "~/customtokens")
    monkeypatch.setenv("HOME", "/home/tester")
    assert client.tokenstore_path() == "/home/tester/customtokens"


# --------------------------------------------------------------------------
# GarminClient wraps the verified method names and delegates 1:1.
# We inject a fake underlying garmin object (no garminconnect / no network).
# --------------------------------------------------------------------------
class _FakeGarmin:
    def __init__(self):
        self.calls = []

    def get_sleep_data(self, cdate):
        self.calls.append(("get_sleep_data", cdate))
        return {"dailySleepDTO": {"sleepTimeSeconds": 100}}

    def get_hrv_data(self, cdate):
        self.calls.append(("get_hrv_data", cdate))
        return None  # exercises the None path

    def get_body_battery(self, startdate, enddate=None):
        self.calls.append(("get_body_battery", startdate, enddate))
        return [{"date": startdate, "charged": 1, "drained": 2, "bodyBatteryValuesArray": []}]

    def get_stats(self, cdate):
        self.calls.append(("get_stats", cdate))
        return {"restingHeartRate": 47}


def test_client_delegates_sleep():
    fake = _FakeGarmin()
    c = client.GarminClient(garmin=fake)
    assert c.get_sleep_data("2026-06-15") == {"dailySleepDTO": {"sleepTimeSeconds": 100}}
    assert ("get_sleep_data", "2026-06-15") in fake.calls


def test_client_delegates_hrv_none():
    fake = _FakeGarmin()
    c = client.GarminClient(garmin=fake)
    assert c.get_hrv_data("2026-06-15") is None
    assert ("get_hrv_data", "2026-06-15") in fake.calls


def test_client_delegates_body_battery_range():
    fake = _FakeGarmin()
    c = client.GarminClient(garmin=fake)
    out = c.get_body_battery("2026-06-14", "2026-06-15")
    assert isinstance(out, list)
    assert ("get_body_battery", "2026-06-14", "2026-06-15") in fake.calls


def test_client_delegates_stats():
    fake = _FakeGarmin()
    c = client.GarminClient(garmin=fake)
    assert c.get_stats("2026-06-15") == {"restingHeartRate": 47}
    assert ("get_stats", "2026-06-15") in fake.calls


# --------------------------------------------------------------------------
# resume() builds a credential-less Garmin and logs in from the token dir.
# We monkeypatch the factory to avoid importing garminconnect / hitting net.
# --------------------------------------------------------------------------
def test_resume_uses_tokenstore(monkeypatch, tmp_path):
    created = {}

    class _ResumeGarmin:
        def __init__(self, *a, **k):
            created["init_args"] = (a, k)

        def login(self, tokenstore=None):
            created["login_tokenstore"] = tokenstore
            return ("oauth1", "oauth2")

    monkeypatch.setattr(client, "_new_garmin", lambda **kw: _ResumeGarmin(**kw))
    monkeypatch.setenv("GARMIN_TOKENSTORE", str(tmp_path))

    c = client.GarminClient.resume()
    assert isinstance(c, client.GarminClient)
    # resume() passes NO credentials (token-only) per Garmin research §2.
    assert created["init_args"] == ((), {})
    assert created["login_tokenstore"] == str(tmp_path)


# --------------------------------------------------------------------------
# login_interactive() passes creds + prompt_mfa and writes tokenstore.
# --------------------------------------------------------------------------
def test_login_interactive_passes_creds_and_mfa(monkeypatch, tmp_path):
    captured = {}

    class _LoginGarmin:
        def __init__(self, email=None, password=None, prompt_mfa=None, **k):
            captured["email"] = email
            captured["password"] = password
            captured["prompt_mfa"] = prompt_mfa

        def login(self, tokenstore=None):
            captured["login_tokenstore"] = tokenstore
            return ("oauth1", "oauth2")

    monkeypatch.setattr(client, "_new_garmin", lambda **kw: _LoginGarmin(**kw))
    monkeypatch.setenv("GARMIN_EMAIL", "you@example.com")
    monkeypatch.setenv("GARMIN_PASSWORD", "s3cret")
    monkeypatch.setenv("GARMIN_TOKENSTORE", str(tmp_path))

    prompt = lambda: "000000"
    c = client.GarminClient.login_interactive(prompt_mfa=prompt)
    assert isinstance(c, client.GarminClient)
    assert captured["email"] == "you@example.com"
    assert captured["password"] == "s3cret"
    assert captured["prompt_mfa"] is prompt
    assert captured["login_tokenstore"] == str(tmp_path)


def test_login_interactive_missing_creds_raises(monkeypatch, tmp_path):
    monkeypatch.delenv("GARMIN_EMAIL", raising=False)
    monkeypatch.delenv("GARMIN_PASSWORD", raising=False)
    monkeypatch.setenv("GARMIN_TOKENSTORE", str(tmp_path))
    with pytest.raises(ValueError):
        client.GarminClient.login_interactive(prompt_mfa=lambda: "0")
```

- [ ] **Step 2: Run the tests and confirm they FAIL.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_client.py
```
Expected: `ModuleNotFoundError: No module named 'garmin_worker.client'`, summary `1 error`. No tests pass.

- [ ] **Step 3: Write `client.py` (the only module importing `garminconnect`).** Write `/home/jake/project/help-my-run/garmin-worker/garmin_worker/client.py` with exactly:
```python
"""Live Garmin client wrapper (the ONLY module that imports garminconnect).

Isolates all network/SSO behind a thin class so the rest of the worker
(normalize.py, cli.py) stays pure and unit-testable. Wraps the VERIFIED
python-garminconnect method names:
    get_sleep_data(cdate)
    get_hrv_data(cdate)            -> dict | None
    get_body_battery(start, end)  -> list[dict]   (range-native)
    get_stats(cdate)              -> dict (flat; restingHeartRate top-level)

Login strategy (Garmin research §2): the widget+cffi multi-strategy login is
BUILT INTO garminconnect's resilient login(); there is NO login_strategy=
argument. We simply call login(tokenstore). curl_cffi (in requirements.txt)
provides the TLS impersonation that defeats the March-2026 Cloudflare block.

The live SSO/HTTP call is deliberately NOT unit-tested (it requires a real
Garmin account). Tests cover env/tokenstore plumbing and 1:1 delegation only,
by injecting a fake underlying object or monkeypatching `_new_garmin`.
"""
from __future__ import annotations

import os
from typing import Any, Callable, Optional


def tokenstore_path() -> str:
    """Resolve the OAuth token directory (CONTRACTS §4: GARMIN_TOKENSTORE).

    Default ~/.garminconnect; ~ is expanded.
    """
    raw = os.getenv("GARMIN_TOKENSTORE", "~/.garminconnect")
    return os.path.expanduser(raw)


def _new_garmin(**kwargs: Any):
    """Factory for the underlying garminconnect.Garmin.

    Imported lazily and isolated here so tests can monkeypatch this function
    without importing garminconnect or touching the network.
    """
    from garminconnect import Garmin  # noqa: WPS433 (intentional local import)

    return Garmin(**kwargs)


class GarminClient:
    """Thin wrapper delegating 1:1 to a logged-in garminconnect.Garmin."""

    def __init__(self, garmin: Any):
        self._g = garmin

    # ---- construction --------------------------------------------------
    @classmethod
    def resume(cls) -> "GarminClient":
        """Non-interactive: build a credential-less Garmin and resume tokens.

        Per Garmin research §2: no creds needed when resuming; login(tokenstore)
        loads tokens from the dir and auto-refreshes per request. If tokens are
        stale/revoked this raises GarminConnectAuthenticationError (handled by
        the fetch command in Task 16).
        """
        g = _new_garmin()
        g.login(tokenstore_path())
        return cls(g)

    @classmethod
    def login_interactive(cls, prompt_mfa: Callable[[], str]) -> "GarminClient":
        """One-time interactive SSO; persists OAuth tokens to the token dir.

        Reads GARMIN_EMAIL / GARMIN_PASSWORD from env (CONTRACTS §4). The
        widget+cffi strategy is automatic inside login(); MFA is handled via
        the prompt_mfa callback.
        """
        email = os.getenv("GARMIN_EMAIL")
        password = os.getenv("GARMIN_PASSWORD")
        if not email or not password:
            raise ValueError(
                "GARMIN_EMAIL and GARMIN_PASSWORD must be set for interactive login"
            )
        g = _new_garmin(email=email, password=password, prompt_mfa=prompt_mfa)
        g.login(tokenstore_path())
        return cls(g)

    # ---- verified data methods (1:1 delegation) ------------------------
    def get_sleep_data(self, cdate: str) -> dict:
        return self._g.get_sleep_data(cdate)

    def get_hrv_data(self, cdate: str) -> Optional[dict]:
        return self._g.get_hrv_data(cdate)

    def get_body_battery(self, startdate: str, enddate: Optional[str] = None) -> list:
        return self._g.get_body_battery(startdate, enddate)

    def get_stats(self, cdate: str) -> dict:
        return self._g.get_stats(cdate)
```

- [ ] **Step 4: Rewire the `login` command in `cli.py` to call the client.** In `/home/jake/project/help-my-run/garmin-worker/garmin_worker/cli.py`, replace the import block and the `login` branch.

First, change the imports near the top from:
```python
from . import normalize
```
to:
```python
from . import client, normalize
```

Then replace the `login` branch in `main()`. Replace:
```python
    if args.command == "login":
        # Live login wiring is added in Task 15.
        print("login is not wired yet", file=sys.stderr)
        return 1
```
with:
```python
    if args.command == "login":
        try:
            client.GarminClient.login_interactive(prompt_mfa=lambda: input("MFA code: "))
        except ValueError as exc:
            print(str(exc), file=sys.stderr)
            return 2
        except Exception as exc:  # garminconnect auth/connection errors
            print(f"login failed: {exc}", file=sys.stderr)
            return 1
        print(f"login ok; tokens saved to {client.tokenstore_path()}", file=sys.stderr)
        return 0
```

- [ ] **Step 5: Run the client tests and confirm they PASS.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_client.py -q
```
Expected output ends with: `9 passed`, exit code 0.

- [ ] **Step 6: Confirm the CLI tests still pass after the `login` rewire (no regression).** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_cli.py -q
```
Expected output ends with: `13 passed`, exit code 0.

- [ ] **Step 7: Commit.** Exact command:
```bash
git -C /home/jake/project/help-my-run add garmin-worker/garmin_worker/client.py garmin-worker/garmin_worker/cli.py garmin-worker/tests/test_client.py && git -C /home/jake/project/help-my-run commit -m "feat(worker): live GarminClient wrapper + wire login command"
```
Expected: commit summary showing 3 files changed.

---
### Task 16: Python worker — `fetch` command wiring (date iteration, error handling, exit codes) + tests with client mocked

**Files:**
- Create: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/fetcher.py`
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/cli.py`
- Test: `/home/jake/project/help-my-run/garmin-worker/tests/test_fetcher.py`
- Test: `/home/jake/project/help-my-run/garmin-worker/tests/test_fetch_cli.py`

Design: `fetcher.py` contains `run_fetch(client, since, until, fetched_at, sleep_fn=time.sleep)` — a function that takes an ALREADY-CONSTRUCTED client (so tests inject a mock, no Garmin), iterates dates, calls the verified methods, normalizes via Task 13's pure functions, and returns the `build_output` dict. HRV `None` days are omitted (CONTRACTS §2.2). Body Battery is a single range call. `cli.py`'s live `fetch` branch is rewired to `client.GarminClient.resume()` → `run_fetch(...)` → print JSON, with `GarminConnectAuthenticationError` → stderr containing `re-run worker.py login` + non-zero exit, and other Garmin errors → stderr + non-zero (CONTRACTS §2.4).

- [ ] **Step 1: Write the FAILING tests for `fetcher.run_fetch` (client mocked).** Write `/home/jake/project/help-my-run/garmin-worker/tests/test_fetcher.py` with exactly:
```python
import pytest

from garmin_worker import fetcher


class _MockClient:
    """Mock GarminClient: deterministic per-date data, records calls."""

    def __init__(self, hrv_map=None, raise_on=None):
        self.calls = []
        self._hrv_map = hrv_map or {}
        self._raise_on = raise_on  # (method_name, exception) to raise

    def _maybe_raise(self, method):
        if self._raise_on and self._raise_on[0] == method:
            raise self._raise_on[1]

    def get_sleep_data(self, cdate):
        self.calls.append(("sleep", cdate))
        self._maybe_raise("get_sleep_data")
        return {"dailySleepDTO": {"sleepTimeSeconds": 100, "sleepScores": {"overall": {"value": 70}}}}

    def get_hrv_data(self, cdate):
        self.calls.append(("hrv", cdate))
        self._maybe_raise("get_hrv_data")
        return self._hrv_map.get(cdate)  # None unless provided

    def get_body_battery(self, startdate, enddate=None):
        self.calls.append(("bb", startdate, enddate))
        self._maybe_raise("get_body_battery")
        return [
            {"date": startdate, "charged": 10, "drained": 20, "bodyBatteryValuesArray": [[1, "ACTIVE", 80], [2, "ACTIVE", 5]]},
            {"date": enddate, "charged": 11, "drained": 22, "bodyBatteryValuesArray": [[3, "ACTIVE", 90], [4, "ACTIVE", 7]]},
        ]

    def get_stats(self, cdate):
        self.calls.append(("stats", cdate))
        self._maybe_raise("get_stats")
        return {"restingHeartRate": 50}


def _noop_sleep(_):
    return None


def test_run_fetch_top_level_shape_and_echo():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="2026-06-15T05:00:12Z", sleep_fn=_noop_sleep,
    )
    assert list(out.keys()) == [
        "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr",
    ]
    assert out["since"] == "2026-06-14"
    assert out["until"] == "2026-06-15"
    assert out["fetched_at"] == "2026-06-15T05:00:12Z"


def test_run_fetch_iterates_each_day_for_per_day_sources():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    # 2 days -> 2 sleep, 2 rhr entries
    assert [s["date"] for s in out["sleep"]] == ["2026-06-14", "2026-06-15"]
    assert [r["date"] for r in out["rhr"]] == ["2026-06-14", "2026-06-15"]
    assert out["sleep"][0]["duration_s"] == 100
    assert out["sleep"][0]["score"] == 70
    assert out["rhr"][0]["resting_hr"] == 50


def test_run_fetch_body_battery_single_range_call():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    bb_calls = [c for c in mc.calls if c[0] == "bb"]
    assert bb_calls == [("bb", "2026-06-14", "2026-06-15")]  # exactly one range call
    assert [b["date"] for b in out["body_battery"]] == ["2026-06-14", "2026-06-15"]
    assert out["body_battery"][0]["high"] == 80
    assert out["body_battery"][0]["low"] == 5


def test_run_fetch_omits_hrv_none_days():
    # HRV present only for 2026-06-15
    mc = _MockClient(hrv_map={
        "2026-06-15": {"hrvSummary": {"lastNightAvg": 48, "status": "BALANCED"}, "hrvReadings": []},
    })
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert [h["date"] for h in out["hrv"]] == ["2026-06-15"]  # 06-14 omitted (None)
    assert out["hrv"][0]["last_night_avg_ms"] == 48
    assert out["hrv"][0]["status"] == "BALANCED"


def test_run_fetch_single_day_range():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-15", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert len(out["sleep"]) == 1
    assert out["sleep"][0]["date"] == "2026-06-15"


def test_run_fetch_body_battery_failure_propagates():
    err = RuntimeError("bb boom")
    mc = _MockClient(raise_on=("get_body_battery", err))
    with pytest.raises(RuntimeError, match="bb boom"):
        fetcher.run_fetch(
            mc, since="2026-06-14", until="2026-06-15",
            fetched_at="t", sleep_fn=_noop_sleep,
        )


def test_run_fetch_output_is_json_serializable():
    import json
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    json.loads(json.dumps(out))  # must not raise
```

- [ ] **Step 2: Run the tests and confirm they FAIL.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_fetcher.py
```
Expected: `ModuleNotFoundError: No module named 'garmin_worker.fetcher'`, summary `1 error`. No tests pass.

- [ ] **Step 3: Write `fetcher.py` (pure iteration over an injected client).** Write `/home/jake/project/help-my-run/garmin-worker/garmin_worker/fetcher.py` with exactly:
```python
"""Fetch orchestration: drive an injected GarminClient over a date range and
assemble the §2.1 contract object via the pure normalizers.

run_fetch() takes an ALREADY-CONSTRUCTED client so it is fully unit-testable
with a mock (no Garmin login). The CLI (cli.py) constructs the live client via
GarminClient.resume() and passes it here.

Per-day sources (sleep, HRV, RHR) are looped one date at a time.
Body Battery is a single range-native call (Garmin research §3/§4).
HRV None days are OMITTED from the output array (CONTRACTS §2.2).
"""
from __future__ import annotations

import datetime as _dt
import time
from typing import Callable

from . import normalize

# Small politeness delay between per-day calls (Garmin research §4: keep
# request volume low). Overridable in tests via sleep_fn.
_PER_DAY_DELAY_S = 0.2


def _date_range(since: str, until: str):
    start = _dt.date.fromisoformat(since)
    end = _dt.date.fromisoformat(until)
    cur = start
    while cur <= end:
        yield cur.isoformat()
        cur += _dt.timedelta(days=1)


def run_fetch(
    client,
    *,
    since: str,
    until: str,
    fetched_at: str,
    sleep_fn: Callable[[float], None] = time.sleep,
) -> dict:
    """Fetch + normalize the whole window; return the §2.1 dict."""
    sleep = []
    hrv = []
    rhr = []

    # Body Battery: one range call for the whole window.
    bb_entries = client.get_body_battery(since, until) or []
    body_battery = [
        normalize.normalize_body_battery_day(entry.get("date"), entry)
        for entry in bb_entries
        if isinstance(entry, dict)
    ]

    # Per-day sources.
    dates = list(_date_range(since, until))
    for i, cdate in enumerate(dates):
        sleep.append(normalize.normalize_sleep_day(cdate, client.get_sleep_data(cdate)))

        hrv_raw = client.get_hrv_data(cdate)
        if hrv_raw is not None:  # CONTRACTS §2.2: omit None HRV days
            hrv.append(normalize.normalize_hrv_day(cdate, hrv_raw))

        rhr.append(normalize.normalize_rhr_day(cdate, client.get_stats(cdate)))

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
    )
```

- [ ] **Step 4: Run the fetcher tests and confirm they PASS.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_fetcher.py -q
```
Expected output ends with: `7 passed`, exit code 0.

- [ ] **Step 5: Write the FAILING tests for the live `fetch` CLI branch (client + fetcher mocked).** Write `/home/jake/project/help-my-run/garmin-worker/tests/test_fetch_cli.py` with exactly:
```python
import json

import pytest

from garmin_worker import cli, client


class _AuthError(Exception):
    """Stand-in for GarminConnectAuthenticationError in tests."""


def test_fetch_live_success_prints_json(monkeypatch, capsys):
    # GarminClient.resume() returns a sentinel; run_fetch returns a fixed dict.
    sentinel = object()
    monkeypatch.setattr(client.GarminClient, "resume", classmethod(lambda cls: sentinel))

    def fake_run_fetch(c, *, since, until, fetched_at, **kw):
        assert c is sentinel
        return {
            "since": since, "until": until, "fetched_at": fetched_at,
            "sleep": [], "hrv": [], "body_battery": [], "rhr": [],
        }

    monkeypatch.setattr(cli, "run_fetch", fake_run_fetch)

    rc = cli.main(["fetch", "--since", "2026-06-14", "--until", "2026-06-15"])
    assert rc == 0
    captured = capsys.readouterr()
    assert captured.err == ""
    out = json.loads(captured.out)
    assert out["since"] == "2026-06-14"
    assert out["until"] == "2026-06-15"
    assert set(out.keys()) == {
        "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr",
    }


def test_fetch_live_auth_error_exits_nonzero_with_relogin_hint(monkeypatch, capsys):
    # Point the worker's auth-error type at our stand-in, and make resume() raise it.
    monkeypatch.setattr(cli, "GarminConnectAuthenticationError", _AuthError, raising=False)

    def boom(cls):
        raise _AuthError("token expired")

    monkeypatch.setattr(client.GarminClient, "resume", classmethod(boom))

    rc = cli.main(["fetch", "--since", "2026-06-14"])
    assert rc != 0
    captured = capsys.readouterr()
    assert captured.out == ""  # nothing parseable on stdout
    assert "re-run worker.py login" in captured.err  # CONTRACTS §2.4 literal substring


def test_fetch_live_generic_error_exits_nonzero_with_message(monkeypatch, capsys):
    def boom(cls):
        raise RuntimeError("connection reset")

    monkeypatch.setattr(client.GarminClient, "resume", classmethod(boom))

    rc = cli.main(["fetch", "--since", "2026-06-14"])
    assert rc != 0
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "connection reset" in captured.err


def test_fetch_live_run_fetch_error_exits_nonzero(monkeypatch, capsys):
    sentinel = object()
    monkeypatch.setattr(client.GarminClient, "resume", classmethod(lambda cls: sentinel))

    def boom(c, **kw):
        raise RuntimeError("rate limited")

    monkeypatch.setattr(cli, "run_fetch", boom)

    rc = cli.main(["fetch", "--since", "2026-06-14"])
    assert rc != 0
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "rate limited" in captured.err
```

- [ ] **Step 6: Run these tests and confirm they FAIL.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_fetch_cli.py
```
Expected: failures — `AttributeError: <module 'garmin_worker.cli'> does not have the attribute 'run_fetch'` (and/or `GarminConnectAuthenticationError`), because the live `fetch` branch and these names are not yet wired into `cli.py`. Summary shows `4 failed` (or errors). No tests pass.

- [ ] **Step 7: Wire the live `fetch` branch into `cli.py`.** In `/home/jake/project/help-my-run/garmin-worker/garmin_worker/cli.py`:

Add the new imports. Change:
```python
import argparse
import datetime as _dt
import json
import sys
from typing import Optional, Sequence

from . import client, normalize
```
to:
```python
import argparse
import datetime as _dt
import json
import sys
from typing import Optional, Sequence

from . import client, normalize
from .fetcher import run_fetch

try:  # re-exported from package root (Garmin research §5)
    from garminconnect import GarminConnectAuthenticationError
except Exception:  # pragma: no cover - import guard for environments w/o lib

    class GarminConnectAuthenticationError(Exception):
        """Fallback when garminconnect is unavailable (e.g. unit tests)."""
```

Then replace the live-fetch placeholder. Replace:
```python
    # Live fetch wiring is added in Task 16.
    print("live fetch is not wired yet", file=sys.stderr)
    return 1
```
with:
```python
    fetched_at = (
        _dt.datetime.now(_dt.timezone.utc)
        .replace(microsecond=0)
        .strftime("%Y-%m-%dT%H:%M:%SZ")
    )
    try:
        live = client.GarminClient.resume()
        output = run_fetch(live, since=since, until=until, fetched_at=fetched_at)
    except GarminConnectAuthenticationError as exc:
        print(
            f"garmin authentication failed ({exc}); re-run worker.py login",
            file=sys.stderr,
        )
        return 1
    except Exception as exc:  # connection / rate-limit / unexpected
        print(f"fetch failed: {exc}", file=sys.stderr)
        return 1

    json.dump(output, sys.stdout)
    sys.stdout.write("\n")
    return 0
```

- [ ] **Step 8: Run the live-fetch CLI tests and confirm they PASS.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests/test_fetch_cli.py -q
```
Expected output ends with: `4 passed`, exit code 0.

- [ ] **Step 9: Run the FULL suite to confirm no regressions.** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python -m pytest /home/jake/project/help-my-run/garmin-worker/tests -q
```
Expected output ends with: `47 passed` (14 normalize + 13 cli + 9 client + 7 fetcher + 4 fetch_cli), exit code 0.

- [ ] **Step 10: Smoke-test that `--dry-run` still emits valid JSON end-to-end (no Garmin).** Exact command:
```bash
/home/jake/project/help-my-run/garmin-worker/.venv/bin/python /home/jake/project/help-my-run/garmin-worker/worker.py fetch --since 2026-06-14 --until 2026-06-15 --dry-run | /home/jake/project/help-my-run/garmin-worker/.venv/bin/python -c "import sys,json; print('ok' if json.load(sys.stdin)['since']=='2026-06-14' else 'bad')"
```
Expected stdout exactly:
```
ok
```

- [ ] **Step 11: Commit.** Exact command:
```bash
git -C /home/jake/project/help-my-run add garmin-worker/garmin_worker/fetcher.py garmin-worker/garmin_worker/cli.py garmin-worker/tests/test_fetcher.py garmin-worker/tests/test_fetch_cli.py && git -C /home/jake/project/help-my-run commit -m "feat(worker): wire live fetch command with date iteration + error handling"
```
Expected: commit summary showing 4 files changed.

---
### Task 17: Strava client — types + AuthorizeURL

**Files:**
- Create: `backend/internal/strava/types.go`
- Create: `backend/internal/strava/client.go`
- Test: `backend/internal/strava/client_test.go`

- [ ] **Step 1: Write the failing test** for `New` + `AuthorizeURL` (exact query params + encoding from Shared Contracts §3.3). Create `backend/internal/strava/client_test.go`:
```go
package strava

import (
	"net/url"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	c := New("12345", "secret", "http://localhost:8080/api/strava/callback")
	got := c.AuthorizeURL("abc123")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("AuthorizeURL parse error = %v", err)
	}
	if u.Scheme != "https" || u.Host != "www.strava.com" || u.Path != "/oauth/authorize" {
		t.Errorf("base = %s://%s%s, want https://www.strava.com/oauth/authorize", u.Scheme, u.Host, u.Path)
	}
	q := u.Query()
	checks := map[string]string{
		"client_id":       "12345",
		"redirect_uri":    "http://localhost:8080/api/strava/callback",
		"response_type":   "code",
		"scope":           "activity:read_all",
		"approval_prompt": "auto",
		"state":           "abc123",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("query[%q] = %q, want %q", k, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/strava/ -run AuthorizeURL`. Expected: build failure `undefined: New` → `FAIL backend/internal/strava [build failed]`.

- [ ] **Step 3: Write `types.go`** (Strava response shapes used by the client). Create `backend/internal/strava/types.go`:
```go
// Package strava is a small, base-URL-injectable client for the Strava API
// (OAuth + activities + laps) used by the sync layer.
package strava

// TokenResponse is the Strava /oauth/token reply (exchange + refresh).
type TokenResponse struct {
	TokenType    string         `json:"token_type"`
	AccessToken  string         `json:"access_token"`
	RefreshToken string         `json:"refresh_token"`
	ExpiresAt    int64          `json:"expires_at"` // unix seconds
	ExpiresIn    int64          `json:"expires_in"`
	Scope        string         `json:"scope"`
	Athlete      *SummaryAthlete `json:"athlete"`
}

// SummaryAthlete is the minimal athlete sub-object on a token response.
type SummaryAthlete struct {
	ID int64 `json:"id"`
}

// SummaryActivity is a Strava activity (run). HR/speed/cadence are pointers
// because they are absent when no sensor was present.
type SummaryActivity struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Type               string   `json:"type"`
	SportType          string   `json:"sport_type"`
	StartDate          string   `json:"start_date"`
	StartDateLocal     string   `json:"start_date_local"`
	Distance           float64  `json:"distance"`
	MovingTime         int64    `json:"moving_time"`
	ElapsedTime        int64    `json:"elapsed_time"`
	AverageHeartrate   *float64 `json:"average_heartrate"`
	MaxHeartrate       *float64 `json:"max_heartrate"`
	AverageSpeed       *float64 `json:"average_speed"`
	MaxSpeed           *float64 `json:"max_speed"`
	AverageCadence     *float64 `json:"average_cadence"`
	TotalElevationGain *float64 `json:"total_elevation_gain"`
}

// Lap is a Strava lap (mapped to activity_splits).
type Lap struct {
	LapIndex         int64    `json:"lap_index"`
	Distance         float64  `json:"distance"`
	ElapsedTime      int64    `json:"elapsed_time"`
	MovingTime       *int64   `json:"moving_time"`
	AverageHeartrate *float64 `json:"average_heartrate"`
	MaxHeartrate     *float64 `json:"max_heartrate"`
	AverageSpeed     *float64 `json:"average_speed"`
}
```

- [ ] **Step 4: Write `client.go`** with the injectable base URL and `AuthorizeURL`. Create `backend/internal/strava/client.go`:
```go
package strava

import (
	"net/http"
	"net/url"
	"time"
)

// defaultBaseURL is the real Strava host. Tests override it via NewWithBase.
const defaultBaseURL = "https://www.strava.com"

// Client talks to the Strava API. baseURL is injectable so tests can point it
// at an httptest server.
type Client struct {
	clientID     string
	clientSecret string
	redirectURL  string
	baseURL      string
	http         *http.Client
}

// New builds a Client against the real Strava base URL.
func New(clientID, clientSecret, redirectURL string) *Client {
	return NewWithBase(clientID, clientSecret, redirectURL, defaultBaseURL)
}

// NewWithBase builds a Client against an explicit base URL (for tests).
func NewWithBase(clientID, clientSecret, redirectURL, baseURL string) *Client {
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		baseURL:      baseURL,
		http:         &http.Client{Timeout: 30 * time.Second},
	}
}

// AuthorizeURL builds the Strava OAuth authorize URL with the given CSRF state.
func (c *Client) AuthorizeURL(state string) string {
	q := url.Values{}
	q.Set("client_id", c.clientID)
	q.Set("redirect_uri", c.redirectURL)
	q.Set("response_type", "code")
	q.Set("scope", "activity:read_all")
	q.Set("approval_prompt", "auto")
	q.Set("state", state)
	return c.baseURL + "/oauth/authorize?" + q.Encode()
}
```

- [ ] **Step 5: Run the test, expect PASS.** Command: `cd backend && go test ./internal/strava/ -run AuthorizeURL`. Expected: `ok  	help-my-run/backend/internal/strava` (`TestAuthorizeURL --- PASS`).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/strava && git commit -m "feat(strava): add client types and AuthorizeURL builder"
```

---

### Task 18: Strava client — token Exchange + Refresh

**Files:**
- Modify: `backend/internal/strava/client.go`
- Create: `backend/internal/strava/testdata/strava_token.json`
- Modify: `backend/internal/strava/client_test.go`

- [ ] **Step 1: Write the token fixture.** Create `backend/internal/strava/testdata/strava_token.json`:
```json
{
  "token_type": "Bearer",
  "access_token": "new-access",
  "refresh_token": "new-refresh",
  "expires_at": 1737000000,
  "expires_in": 21600,
  "scope": "read,activity:read_all",
  "athlete": { "id": 12345678 }
}
```

- [ ] **Step 2: Write the failing test** that fakes `POST /oauth/token` and asserts both `Exchange` and `Refresh` send the right grant + parse the response. Append to `backend/internal/strava/client_test.go`:
```go

func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := NewWithBase("12345", "secret", "http://localhost:8080/api/strava/callback", srv.URL)
	return c, srv
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return b
}

func TestExchange(t *testing.T) {
	var gotGrant, gotCode string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/token" {
			t.Errorf("got %s %s, want POST /oauth/token", r.Method, r.URL.Path)
		}
		_ = r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotCode = r.Form.Get("code")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(loadFixture(t, "strava_token.json"))
	})

	tok, err := c.Exchange(context.Background(), "the-code")
	if err != nil {
		t.Fatalf("Exchange error = %v", err)
	}
	if gotGrant != "authorization_code" {
		t.Errorf("grant_type = %q, want authorization_code", gotGrant)
	}
	if gotCode != "the-code" {
		t.Errorf("code = %q, want the-code", gotCode)
	}
	if tok.AccessToken != "new-access" || tok.RefreshToken != "new-refresh" || tok.ExpiresAt != 1737000000 {
		t.Errorf("token = %+v, want access=new-access refresh=new-refresh exp=1737000000", tok)
	}
	if tok.Athlete == nil || tok.Athlete.ID != 12345678 {
		t.Errorf("athlete = %+v, want id 12345678", tok.Athlete)
	}
}

func TestRefresh(t *testing.T) {
	var gotGrant, gotRefresh string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotGrant = r.Form.Get("grant_type")
		gotRefresh = r.Form.Get("refresh_token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(loadFixture(t, "strava_token.json"))
	})

	tok, err := c.Refresh(context.Background(), "old-refresh")
	if err != nil {
		t.Fatalf("Refresh error = %v", err)
	}
	if gotGrant != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", gotGrant)
	}
	if gotRefresh != "old-refresh" {
		t.Errorf("refresh_token sent = %q, want old-refresh", gotRefresh)
	}
	if tok.AccessToken != "new-access" {
		t.Errorf("AccessToken = %q, want new-access", tok.AccessToken)
	}
}

func TestExchangeNon200(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"Bad Request"}`))
	})
	if _, err := c.Exchange(context.Background(), "x"); err == nil {
		t.Fatal("Exchange on 400 error = nil, want error")
	}
}
```

- [ ] **Step 3: Add the test imports.** Replace the import block at the top of `backend/internal/strava/client_test.go` with:
```go
import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)
```

- [ ] **Step 4: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/strava/ -run 'Exchange|Refresh'`. Expected: build failure `c.Exchange undefined`, `c.Refresh undefined` → `FAIL backend/internal/strava [build failed]`.

- [ ] **Step 5: Add `Exchange` + `Refresh` + a shared `postToken` helper.** FIRST, merge the new imports into the SINGLE existing `import (...)` block at the top of `backend/internal/strava/client.go` — do NOT add a second import block (Go forbids imports after other declarations). Add these paths to the existing block: `context`, `encoding/json`, `fmt`, `io`, `strings` (alongside the already-present `net/http`, `net/url`, `time`), so the final import block is exactly `context`, `encoding/json`, `fmt`, `io`, `net/http`, `net/url`, `strings`, `time`. THEN append only the following function bodies (no import block) to `client.go`:
```go

// tokenURL is the Strava token endpoint (NOT /api/v3/oauth/token).
func (c *Client) tokenURL() string { return c.baseURL + "/oauth/token" }

// Exchange swaps an authorization code for tokens (grant_type=authorization_code).
func (c *Client) Exchange(ctx context.Context, code string) (*TokenResponse, error) {
	return c.postToken(ctx, url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	})
}

// Refresh exchanges a refresh token for a new access token
// (grant_type=refresh_token). Always persist the returned refresh_token.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	return c.postToken(ctx, url.Values{
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

func (c *Client) postToken(ctx context.Context, form url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL(),
		strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("strava token endpoint: status %d: %s", resp.StatusCode, string(body))
	}

	var tok TokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("strava token parse: %w", err)
	}
	return &tok, nil
}
```

- [ ] **Step 6: Run the test, expect PASS.** Command: `cd backend && go test ./internal/strava/ -run 'Exchange|Refresh'`. Expected: `ok  	help-my-run/backend/internal/strava` (`TestExchange`, `TestRefresh`, `TestExchangeNon200` `--- PASS`).

- [ ] **Step 7: Commit.** Command:
```
git add backend/internal/strava && git commit -m "feat(strava): add token Exchange and Refresh against /oauth/token"
```

---
### Task 19: Strava client — ListActivities + ListLaps (paginated, fixtures)

**Files:**
- Modify: `backend/internal/strava/client.go`
- Create: `backend/internal/strava/testdata/strava_activities.json`
- Create: `backend/internal/strava/testdata/strava_laps.json`
- Modify: `backend/internal/strava/client_test.go`

- [ ] **Step 1: Write the activities fixture** (page 1; one run with HR, one without). Create `backend/internal/strava/testdata/strava_activities.json`:
```json
[
  {
    "id": 14820001234,
    "name": "Morning Run",
    "type": "Run",
    "sport_type": "Run",
    "start_date": "2026-06-18T06:12:00Z",
    "start_date_local": "2026-06-18T08:12:00Z",
    "distance": 10240.5,
    "moving_time": 3120,
    "elapsed_time": 3200,
    "average_heartrate": 152.3,
    "max_heartrate": 171,
    "average_speed": 3.28,
    "max_speed": 4.91,
    "average_cadence": 86.5,
    "total_elevation_gain": 84.0
  },
  {
    "id": 14820005678,
    "name": "No HR Run",
    "type": "Run",
    "sport_type": "TrailRun",
    "start_date": "2026-06-17T18:00:00Z",
    "start_date_local": "2026-06-17T20:00:00Z",
    "distance": 5000.0,
    "moving_time": 1500,
    "elapsed_time": 1500
  }
]
```

- [ ] **Step 2: Write the laps fixture.** Create `backend/internal/strava/testdata/strava_laps.json`:
```json
[
  {
    "lap_index": 1,
    "distance": 1000.0,
    "elapsed_time": 300,
    "moving_time": 295,
    "average_heartrate": 148.0,
    "max_heartrate": 158.0,
    "average_speed": 3.33
  },
  {
    "lap_index": 2,
    "distance": 1000.0,
    "elapsed_time": 305,
    "average_speed": 3.27
  }
]
```

- [ ] **Step 3: Write the failing test** for `ListActivities` (sends bearer + `after`/`page`/`per_page`, paginates until empty) and `ListLaps`. Append to `backend/internal/strava/client_test.go`:
```go

func TestListActivitiesPaginates(t *testing.T) {
	var sawAuth string
	var afterParam string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/athlete/activities" {
			t.Errorf("path = %s, want /api/v3/athlete/activities", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization")
		afterParam = r.URL.Query().Get("after")
		w.Header().Set("Content-Type", "application/json")
		// Page 1 -> fixture (2 activities); page 2+ -> empty array (stop).
		if r.URL.Query().Get("page") == "1" {
			_, _ = w.Write(loadFixture(t, "strava_activities.json"))
		} else {
			_, _ = w.Write([]byte(`[]`))
		}
	})

	acts, err := c.ListActivities(context.Background(), "access-tok", 1718600000)
	if err != nil {
		t.Fatalf("ListActivities error = %v", err)
	}
	if sawAuth != "Bearer access-tok" {
		t.Errorf("Authorization = %q, want %q", sawAuth, "Bearer access-tok")
	}
	if afterParam != "1718600000" {
		t.Errorf("after = %q, want 1718600000", afterParam)
	}
	if len(acts) != 2 {
		t.Fatalf("activities len = %d, want 2", len(acts))
	}
	if acts[0].ID != 14820001234 || acts[0].SportType != "Run" {
		t.Errorf("act0 = id %d sport %q, want 14820001234 Run", acts[0].ID, acts[0].SportType)
	}
	if acts[0].AverageHeartrate == nil || *acts[0].AverageHeartrate != 152.3 {
		t.Errorf("act0.AverageHeartrate = %v, want 152.3", acts[0].AverageHeartrate)
	}
	// Second run has no HR -> pointer nil.
	if acts[1].AverageHeartrate != nil {
		t.Errorf("act1.AverageHeartrate = %v, want nil", acts[1].AverageHeartrate)
	}
}

func TestListLaps(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/activities/14820001234/laps" {
			t.Errorf("path = %s, want /api/v3/activities/14820001234/laps", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer access-tok" {
			t.Errorf("Authorization = %q, want Bearer access-tok", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(loadFixture(t, "strava_laps.json"))
	})

	laps, err := c.ListLaps(context.Background(), "access-tok", 14820001234)
	if err != nil {
		t.Fatalf("ListLaps error = %v", err)
	}
	if len(laps) != 2 {
		t.Fatalf("laps len = %d, want 2", len(laps))
	}
	if laps[0].LapIndex != 1 || laps[0].Distance != 1000.0 {
		t.Errorf("lap0 = idx %d dist %v, want 1 1000", laps[0].LapIndex, laps[0].Distance)
	}
	if laps[1].AverageHeartrate != nil {
		t.Errorf("lap1.AverageHeartrate = %v, want nil", laps[1].AverageHeartrate)
	}
}
```

- [ ] **Step 4: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/strava/ -run 'ListActivities|ListLaps'`. Expected: build failure `c.ListActivities undefined`, `c.ListLaps undefined` → `FAIL backend/internal/strava [build failed]`.

- [ ] **Step 5: Add `ListActivities` + `ListLaps` + a shared authed GET helper.** FIRST, merge `strconv` into the SINGLE existing `import (...)` block at the top of `backend/internal/strava/client.go` — do NOT add a second `import` statement (Go forbids imports after other declarations). After merging, the import block is exactly `context`, `encoding/json`, `fmt`, `io`, `net/http`, `net/url`, `strconv`, `strings`, `time`. THEN append only the following declarations (no import statement) to `client.go`:
```go

const perPage = 200

// ListActivities returns all activities after the given unix-second timestamp,
// paginating until Strava returns an empty page.
func (c *Client) ListActivities(ctx context.Context, accessToken string, after int64) ([]SummaryActivity, error) {
	var all []SummaryActivity
	for page := 1; ; page++ {
		q := url.Values{}
		if after > 0 {
			q.Set("after", strconv.FormatInt(after, 10))
		}
		q.Set("page", strconv.Itoa(page))
		q.Set("per_page", strconv.Itoa(perPage))

		var batch []SummaryActivity
		if err := c.getJSON(ctx, accessToken,
			"/api/v3/athlete/activities?"+q.Encode(), &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
	}
	return all, nil
}

// ListLaps returns the laps for an activity.
func (c *Client) ListLaps(ctx context.Context, accessToken string, activityID int64) ([]Lap, error) {
	var laps []Lap
	path := "/api/v3/activities/" + strconv.FormatInt(activityID, 10) + "/laps"
	if err := c.getJSON(ctx, accessToken, path, &laps); err != nil {
		return nil, err
	}
	return laps, nil
}

// getJSON performs an authenticated GET and unmarshals the JSON body into dst.
func (c *Client) getJSON(ctx context.Context, accessToken, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("strava GET %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, dst)
}
```

- [ ] **Step 6: Run the test, expect PASS.** Command: `cd backend && go test ./internal/strava/`. Expected: `ok  	help-my-run/backend/internal/strava` (all strava tests, incl. `TestListActivitiesPaginates`, `TestListLaps`, `--- PASS`).

- [ ] **Step 7: Commit.** Command:
```
git add backend/internal/strava && git commit -m "feat(strava): add paginated ListActivities and ListLaps"
```

---

### Task 20: Garmin runner — types + RunGarminFetch (os/exec, injectable command)

**Files:**
- Create: `backend/internal/garmin/types.go`
- Create: `backend/internal/garmin/runner.go`
- Create: `backend/internal/garmin/testdata/worker_output.json`
- Test: `backend/internal/garmin/runner_test.go`

- [ ] **Step 1: Write the worker-output fixture** (the Shared Contracts §2.3 example). Create `backend/internal/garmin/testdata/worker_output.json`:
```json
{
  "since": "2026-06-14",
  "until": "2026-06-15",
  "fetched_at": "2026-06-15T05:00:12Z",
  "sleep": [
    { "date": "2026-06-14", "duration_s": 26400, "deep_s": 6000, "light_s": 14100, "rem_s": 5400, "awake_s": 900, "score": 79, "raw_json": {"dailySleepDTO": {"sleepTimeSeconds": 26400}} },
    { "date": "2026-06-15", "duration_s": 27000, "deep_s": 6300, "light_s": 14400, "rem_s": 5400, "awake_s": 900, "score": 82, "raw_json": {"dailySleepDTO": {"sleepTimeSeconds": 27000}} }
  ],
  "hrv": [
    { "date": "2026-06-15", "last_night_avg_ms": 48, "status": "BALANCED", "raw_json": {"hrvSummary": {"lastNightAvg": 48, "status": "BALANCED"}} }
  ],
  "body_battery": [
    { "date": "2026-06-14", "charged": 60, "drained": 75, "high": 88, "low": 16, "raw_json": {"date": "2026-06-14", "charged": 60} },
    { "date": "2026-06-15", "charged": 62, "drained": 78, "high": 91, "low": 14, "raw_json": {"date": "2026-06-15", "charged": 62} }
  ],
  "rhr": [
    { "date": "2026-06-14", "resting_hr": 48, "raw_json": {"restingHeartRate": 48} },
    { "date": "2026-06-15", "resting_hr": 47, "raw_json": {"restingHeartRate": 47} }
  ]
}
```

- [ ] **Step 2: Write the failing test** that runs a real stub command (`cat` of the fixture) and a failing stub (`false` / a script exiting non-zero with stderr), asserting parse + error surfacing. Create `backend/internal/garmin/runner_test.go`:
```go
package garmin

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunGarminFetchParsesOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/cat")
	}
	fixture, err := filepath.Abs(filepath.Join("testdata", "worker_output.json"))
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	// Stub "worker": `cat <fixture>` ignores the fetch args and prints JSON.
	r := Runner{Python: "/bin/cat", Script: fixture}

	out, err := r.RunGarminFetch(context.Background(), "2026-06-14", nil)
	if err != nil {
		t.Fatalf("RunGarminFetch error = %v", err)
	}
	if out.Since != "2026-06-14" || out.Until != "2026-06-15" {
		t.Errorf("since/until = %q/%q, want 2026-06-14/2026-06-15", out.Since, out.Until)
	}
	if len(out.Sleep) != 2 || out.Sleep[0].Date != "2026-06-14" || *out.Sleep[0].DurationS != 26400 {
		t.Errorf("sleep parse wrong: %+v", out.Sleep)
	}
	if len(out.HRV) != 1 || out.HRV[0].Status == nil || *out.HRV[0].Status != "BALANCED" {
		t.Errorf("hrv parse wrong: %+v", out.HRV)
	}
	if len(out.BodyBattery) != 2 || *out.BodyBattery[1].High != 91 {
		t.Errorf("body_battery parse wrong: %+v", out.BodyBattery)
	}
	if len(out.RHR) != 2 || *out.RHR[1].RestingHR != 47 {
		t.Errorf("rhr parse wrong: %+v", out.RHR)
	}
	// raw_json must be preserved as a JSON string for the store.
	if !strings.Contains(string(out.Sleep[0].RawJSON), "dailySleepDTO") {
		t.Errorf("sleep raw_json missing: %s", out.Sleep[0].RawJSON)
	}
}

func TestRunGarminFetchSurfacesStderrOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	// Stub script: print the login hint to stderr and exit 1.
	script := filepath.Join(t.TempDir(), "fail.sh")
	body := "#!/bin/sh\necho 're-run worker.py login' 1>&2\nexit 1\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := Runner{Python: "/bin/sh", Script: script}

	_, err := r.RunGarminFetch(context.Background(), "2026-06-14", nil)
	if err == nil {
		t.Fatal("RunGarminFetch error = nil, want non-nil on exit 1")
	}
	if !strings.Contains(err.Error(), "re-run worker.py login") {
		t.Errorf("error = %q, want it to contain stderr 're-run worker.py login'", err.Error())
	}
}
```

- [ ] **Step 3: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/garmin/`. Expected: build failure `undefined: Runner`, `undefined: WorkerOutput` → `FAIL backend/internal/garmin [build failed]`.

- [ ] **Step 4: Write `types.go`.** Create `backend/internal/garmin/types.go`:
```go
// Package garmin invokes the Python worker via os/exec and parses its JSON
// (the contracts §2 shape).
package garmin

import "encoding/json"

// WorkerOutput is the top-level worker stdout JSON.
type WorkerOutput struct {
	Since       string           `json:"since"`
	Until       string           `json:"until"`
	FetchedAt   string           `json:"fetched_at"`
	Sleep       []SleepDay       `json:"sleep"`
	HRV         []HrvDay         `json:"hrv"`
	BodyBattery []BodyBatteryDay `json:"body_battery"`
	RHR         []RhrDay         `json:"rhr"`
}

// SleepDay is one per-day sleep entry. RawJSON is kept verbatim for the store.
type SleepDay struct {
	Date      string          `json:"date"`
	DurationS *int64          `json:"duration_s"`
	DeepS     *int64          `json:"deep_s"`
	LightS    *int64          `json:"light_s"`
	RemS      *int64          `json:"rem_s"`
	AwakeS    *int64          `json:"awake_s"`
	Score     *int64          `json:"score"`
	RawJSON   json.RawMessage `json:"raw_json"`
}

// HrvDay is one per-day HRV entry.
type HrvDay struct {
	Date           string          `json:"date"`
	LastNightAvgMs *int64          `json:"last_night_avg_ms"`
	Status         *string         `json:"status"`
	RawJSON        json.RawMessage `json:"raw_json"`
}

// BodyBatteryDay is one per-day Body Battery entry.
type BodyBatteryDay struct {
	Date    string          `json:"date"`
	Charged *int64          `json:"charged"`
	Drained *int64          `json:"drained"`
	High    *int64          `json:"high"`
	Low     *int64          `json:"low"`
	RawJSON json.RawMessage `json:"raw_json"`
}

// RhrDay is one per-day resting-HR entry.
type RhrDay struct {
	Date      string          `json:"date"`
	RestingHR *int64          `json:"resting_hr"`
	RawJSON   json.RawMessage `json:"raw_json"`
}
```

- [ ] **Step 5: Write `runner.go`.** Create `backend/internal/garmin/runner.go`:
```go
package garmin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
)

// Runner invokes the Python Garmin worker. Python and Script are injectable so
// tests can substitute a stub command (e.g. /bin/cat of a fixture).
type Runner struct {
	Python string
	Script string
}

// RunGarminFetch runs `<python> <script> fetch --since <since>` with extraEnv
// appended to the current environment, parses the worker's stdout JSON, and
// surfaces captured stderr in the error on non-zero exit.
func (r Runner) RunGarminFetch(ctx context.Context, since string, extraEnv []string) (*WorkerOutput, error) {
	cmd := exec.CommandContext(ctx, r.Python, r.Script, "fetch", "--since", since)
	if len(extraEnv) > 0 {
		cmd.Env = append(cmd.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("worker exit %d: %s", ee.ExitCode(), stderr.String())
		}
		return nil, fmt.Errorf("worker start failed: %w (stderr: %s)", err, stderr.String())
	}

	var out WorkerOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("worker JSON parse: %w (stdout: %.200s)", err, stdout.String())
	}
	return &out, nil
}
```

- [ ] **Step 6: Run the test, expect PASS.** Command: `cd backend && go test ./internal/garmin/`. Expected: `ok  	help-my-run/backend/internal/garmin` (`TestRunGarminFetchParsesOutput`, `TestRunGarminFetchSurfacesStderrOnFailure` `--- PASS`).

- [ ] **Step 7: Commit.** Command:
```
git add backend/internal/garmin && git commit -m "feat(garmin): add worker output types and RunGarminFetch via os/exec"
```

---
### Task 21: Sync — SyncStrava (refresh-if-expired, map, upsert, sync_log)

**Files:**
- Create: `backend/internal/sync/sync.go`
- Test: `backend/internal/sync/sync_test.go`

- [ ] **Step 1: Write the failing test** for `SyncStrava` using a real `strava.Client` pointed at an httptest server (token refresh when expired + activities + laps), asserting upserts and `sync_log` update. Create `backend/internal/sync/sync_test.go`:
```go
package sync

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
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

func TestSyncStravaRefreshesAndUpserts(t *testing.T) {
	s := newStore(t)
	// Store an EXPIRED token so the sync must refresh first.
	if err := s.SaveStravaTokens(store.StravaTokens{
		AccessToken: "old-acc", RefreshToken: "old-ref",
		ExpiresAt: time.Now().Add(-time.Hour).Unix(), Scope: "activity:read_all", AthleteID: 1,
	}); err != nil {
		t.Fatalf("seed tokens: %v", err)
	}

	var refreshed bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/oauth/token":
			refreshed = true
			_, _ = w.Write([]byte(`{"token_type":"Bearer","access_token":"fresh","refresh_token":"fresh-ref","expires_at":4102444800,"expires_in":21600,"scope":"activity:read_all","athlete":{"id":1}}`))
		case r.URL.Path == "/api/v3/athlete/activities":
			if r.URL.Query().Get("page") == "1" {
				_, _ = w.Write([]byte(`[{"id":900,"name":"Run","type":"Run","sport_type":"Run","start_date":"2026-06-18T06:00:00Z","start_date_local":"2026-06-18T08:00:00Z","distance":10000,"moving_time":3000,"elapsed_time":3050,"average_heartrate":150,"max_heartrate":170,"average_speed":3.3,"max_speed":4.5,"average_cadence":85,"total_elevation_gain":50}]`))
			} else {
				_, _ = w.Write([]byte(`[]`))
			}
		case r.URL.Path == "/api/v3/activities/900/laps":
			_, _ = w.Write([]byte(`[{"lap_index":1,"distance":5000,"elapsed_time":1500,"moving_time":1490,"average_heartrate":148,"max_heartrate":160,"average_speed":3.3}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := strava.NewWithBase("123", "secret", "http://cb", srv.URL)
	res := SyncStrava(context.Background(), s, client)

	if res.Status != "ok" || res.Error != nil {
		t.Fatalf("result = %+v, want ok/no-error", res)
	}
	if res.Synced != 1 {
		t.Errorf("synced = %d, want 1", res.Synced)
	}
	if !refreshed {
		t.Error("expected token refresh on expired token")
	}
	// Fresh token persisted.
	tok, _ := s.GetStravaTokens()
	if tok.AccessToken != "fresh" || tok.RefreshToken != "fresh-ref" {
		t.Errorf("tokens = %+v, want fresh/fresh-ref", tok)
	}
	// Activity + lap upserted.
	acts, _ := s.ListActivities(30)
	if len(acts) != 1 || acts[0].StravaID != 900 {
		t.Fatalf("activities = %+v, want one id=900", acts)
	}
	var nLaps int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activity_splits WHERE activity_id=900`).Scan(&nLaps)
	if nLaps != 1 {
		t.Errorf("laps = %d, want 1", nLaps)
	}
	// sync_log updated.
	sl, _ := s.GetSyncLog("strava")
	if sl.Status != "ok" || sl.LastSyncedAt == nil {
		t.Errorf("sync_log = %+v, want ok with last_synced_at", sl)
	}
}

func TestSyncStravaNotConnected(t *testing.T) {
	s := newStore(t)
	client := strava.NewWithBase("123", "secret", "http://cb", "http://unused")
	res := SyncStrava(context.Background(), s, client)
	if res.Status != "error" || res.Error == nil {
		t.Fatalf("result = %+v, want error when not connected", res)
	}
	sl, _ := s.GetSyncLog("strava")
	if sl.Status != "error" {
		t.Errorf("sync_log status = %q, want error", sl.Status)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/sync/ -run SyncStrava`. Expected: build failure `undefined: SyncStrava`, `undefined: SourceResult` → `FAIL backend/internal/sync [build failed]`.

- [ ] **Step 3: Write `sync.go`** with the result type and `SyncStrava`. Create `backend/internal/sync/sync.go`:
```go
// Package sync orchestrates Strava + Garmin ingestion and records sync_log.
package sync

import (
	"context"
	"encoding/json"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

// refreshBuffer refreshes the Strava token if it expires within this window.
const refreshBuffer = 60 * time.Second

// SourceResult is the per-source sync outcome (matches the /api/sync contract).
type SourceResult struct {
	Status string  // "ok" | "error"
	Synced int     // rows upserted
	Error  *string // non-nil when Status=="error"
}

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

func errResult(s *store.Store, source string, err error) SourceResult {
	msg := err.Error()
	now := nowUTC()
	_ = s.UpdateSyncLog(store.SyncLog{
		Source: source, LastSyncedAt: prevSynced(s, source), LastRunAt: &now,
		Status: "error", Error: &msg,
	})
	return SourceResult{Status: "error", Synced: 0, Error: &msg}
}

// prevSynced preserves the prior last_synced_at on an errored run.
func prevSynced(s *store.Store, source string) *string {
	if sl, err := s.GetSyncLog(source); err == nil {
		return sl.LastSyncedAt
	}
	return nil
}

func okResult(s *store.Store, source string, synced int) SourceResult {
	now := nowUTC()
	_ = s.UpdateSyncLog(store.SyncLog{
		Source: source, LastSyncedAt: &now, LastRunAt: &now,
		Status: "ok", Error: nil,
	})
	return SourceResult{Status: "ok", Synced: synced, Error: nil}
}

// SyncStrava refreshes the access token if needed, pulls activities since the
// last successful sync (or a ~30-day backfill), upserts activities + laps, and
// records sync_log. Returns the per-source result.
func SyncStrava(ctx context.Context, s *store.Store, client *strava.Client) SourceResult {
	const source = "strava"

	tok, err := s.GetStravaTokens()
	if err != nil {
		return errResult(s, source, err)
	}

	// Refresh if expired (or within the buffer).
	if tok.ExpiresAt <= time.Now().Add(refreshBuffer).Unix() {
		tr, err := client.Refresh(ctx, tok.RefreshToken)
		if err != nil {
			return errResult(s, source, err)
		}
		tok.AccessToken = tr.AccessToken
		tok.RefreshToken = tr.RefreshToken
		tok.ExpiresAt = tr.ExpiresAt
		if tr.Scope != "" {
			tok.Scope = tr.Scope
		}
		if tr.Athlete != nil {
			tok.AthleteID = tr.Athlete.ID
		}
		if err := s.SaveStravaTokens(tok); err != nil {
			return errResult(s, source, err)
		}
	}

	// Incremental window: since last successful sync, else ~30-day backfill.
	after := time.Now().AddDate(0, 0, -30).Unix()
	if sl, err := s.GetSyncLog(source); err == nil && sl.LastSyncedAt != nil {
		if ts, perr := time.Parse(time.RFC3339, *sl.LastSyncedAt); perr == nil {
			after = ts.Unix()
		}
	}

	acts, err := client.ListActivities(ctx, tok.AccessToken, after)
	if err != nil {
		return errResult(s, source, err)
	}

	synced := 0
	for _, a := range acts {
		raw, _ := json.Marshal(a)
		if err := s.UpsertActivity(mapActivity(a, string(raw))); err != nil {
			return errResult(s, source, err)
		}
		laps, err := client.ListLaps(ctx, tok.AccessToken, a.ID)
		if err != nil {
			return errResult(s, source, err)
		}
		if err := s.UpsertSplits(a.ID, mapLaps(a.ID, laps)); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	return okResult(s, source, synced)
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mapActivity(a strava.SummaryActivity, raw string) store.Activity {
	return store.Activity{
		StravaID:       a.ID,
		Name:           a.Name,
		Type:           a.Type,
		SportType:      strPtr(a.SportType),
		StartTime:      a.StartDate,
		StartTimeLocal: strPtr(a.StartDateLocal),
		DistanceM:      a.Distance,
		MovingTimeS:    a.MovingTime,
		ElapsedTimeS:   a.ElapsedTime,
		AvgHR:          a.AverageHeartrate,
		MaxHR:          a.MaxHeartrate,
		AvgSpeed:       a.AverageSpeed,
		MaxSpeed:       a.MaxSpeed,
		AvgCadence:     a.AverageCadence,
		ElevationGainM: a.TotalElevationGain,
		RawJSON:        raw,
	}
}

func mapLaps(activityID int64, laps []strava.Lap) []store.Split {
	out := make([]store.Split, 0, len(laps))
	for _, l := range laps {
		out = append(out, store.Split{
			ActivityID:   activityID,
			Idx:          l.LapIndex,
			DistanceM:    l.Distance,
			ElapsedTimeS: l.ElapsedTime,
			MovingTimeS:  l.MovingTime,
			AvgHR:        l.AverageHeartrate,
			MaxHR:        l.MaxHeartrate,
			AvgSpeed:     l.AverageSpeed,
		})
	}
	return out
}
```

- [ ] **Step 4: Run the test, expect PASS.** Command: `cd backend && go test ./internal/sync/ -run SyncStrava`. Expected: `ok  	help-my-run/backend/internal/sync` (`TestSyncStravaRefreshesAndUpserts`, `TestSyncStravaNotConnected` `--- PASS`).

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/sync && git commit -m "feat(sync): add SyncStrava with token refresh, mapping, and sync_log"
```

---
### Task 22: Sync — SyncGarmin (run worker, map, upsert, sync_log)

**Files:**
- Modify: `backend/internal/sync/sync.go`
- Modify: `backend/internal/sync/sync_test.go`

- [ ] **Step 1: Write the failing test** for `SyncGarmin` using a real stub worker (`/bin/cat` of the garmin fixture) and a failing stub, asserting the four tables get upserted and `sync_log` reflects success/error. Append to `backend/internal/sync/sync_test.go`:
```go

func TestSyncGarminUpsertsAllTables(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/cat")
	}
	s := newStore(t)

	fixture, err := filepath.Abs(filepath.Join("..", "garmin", "testdata", "worker_output.json"))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	r := garmin.Runner{Python: "/bin/cat", Script: fixture}

	res := SyncGarmin(context.Background(), s, r, nil)
	if res.Status != "ok" || res.Error != nil {
		t.Fatalf("result = %+v, want ok", res)
	}
	// Fixture has 2 sleep + 1 hrv + 2 bb + 2 rhr = 7 upserts.
	if res.Synced != 7 {
		t.Errorf("synced = %d, want 7", res.Synced)
	}

	counts := map[string]int{
		"garmin_sleep": 0, "garmin_hrv": 0, "garmin_body_battery": 0, "garmin_rhr": 0,
	}
	for tbl := range counts {
		var n int
		if err := s.DB.QueryRow(`SELECT COUNT(*) FROM ` + tbl).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		counts[tbl] = n
	}
	if counts["garmin_sleep"] != 2 || counts["garmin_hrv"] != 1 ||
		counts["garmin_body_battery"] != 2 || counts["garmin_rhr"] != 2 {
		t.Errorf("counts = %+v, want sleep2 hrv1 bb2 rhr2", counts)
	}

	// raw_json persisted from the worker.
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
```

- [ ] **Step 2: Add the new test imports.** Replace the import block at the top of `backend/internal/sync/sync_test.go` with:
```go
import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)
```

- [ ] **Step 3: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/sync/ -run SyncGarmin`. Expected: build failure `undefined: SyncGarmin` → `FAIL backend/internal/sync [build failed]`.

- [ ] **Step 4: Add `SyncGarmin` to `sync.go`.** Append to `backend/internal/sync/sync.go`:
```go

// garminBackfillDays is the default look-back when Garmin has never synced.
const garminBackfillDays = 30

// SyncGarmin runs the Python worker since the last successful Garmin sync (or a
// ~30-day backfill), upserts the four garmin_* tables, and records sync_log.
func SyncGarmin(ctx context.Context, s *store.Store, r garmin.Runner, extraEnv []string) SourceResult {
	const source = "garmin"

	since := time.Now().AddDate(0, 0, -garminBackfillDays).Format("2006-01-02")
	if sl, err := s.GetSyncLog(source); err == nil && sl.LastSyncedAt != nil {
		if ts, perr := time.Parse(time.RFC3339, *sl.LastSyncedAt); perr == nil {
			since = ts.Format("2006-01-02")
		}
	}

	out, err := r.RunGarminFetch(ctx, since, extraEnv)
	if err != nil {
		return errResult(s, source, err)
	}

	synced := 0
	for _, d := range out.Sleep {
		if err := s.UpsertSleep(store.SleepRow{
			Date: d.Date, DurationS: d.DurationS, DeepS: d.DeepS, LightS: d.LightS,
			RemS: d.RemS, AwakeS: d.AwakeS, Score: d.Score, RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	for _, d := range out.HRV {
		if err := s.UpsertHrv(store.HrvRow{
			Date: d.Date, LastNightAvgMs: d.LastNightAvgMs, Status: d.Status,
			RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	for _, d := range out.BodyBattery {
		if err := s.UpsertBodyBattery(store.BodyBatteryRow{
			Date: d.Date, Charged: d.Charged, Drained: d.Drained, High: d.High, Low: d.Low,
			RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	for _, d := range out.RHR {
		if err := s.UpsertRhr(store.RhrRow{
			Date: d.Date, RestingHR: d.RestingHR, RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
	return okResult(s, source, synced)
}

// rawString renders a json.RawMessage to a string for the raw_json column,
// defaulting to "null" when empty.
func rawString(m json.RawMessage) string {
	if len(m) == 0 {
		return "null"
	}
	return string(m)
}
```

- [ ] **Step 5: Run the test, expect PASS.** Command: `cd backend && go test ./internal/sync/ -run SyncGarmin`. Expected: `ok  	help-my-run/backend/internal/sync` (`TestSyncGarminUpsertsAllTables`, `TestSyncGarminError` `--- PASS`).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/sync && git commit -m "feat(sync): add SyncGarmin running the worker and upserting garmin_* tables"
```

---

### Task 23: Sync — SyncAll orchestration

**Files:**
- Modify: `backend/internal/sync/sync.go`
- Modify: `backend/internal/sync/sync_test.go`

- [ ] **Step 1: Write the failing test** for `SyncAll` returning both per-source results, where Strava succeeds and Garmin fails (partial failure must still return both). Append to `backend/internal/sync/sync_test.go`:
```go

func TestSyncAllPartialFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)
	if err := s.SaveStravaTokens(store.StravaTokens{
		AccessToken: "acc", RefreshToken: "ref",
		ExpiresAt: time.Now().Add(time.Hour).Unix(), Scope: "activity:read_all", AthleteID: 1,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// No activities -> 0 synced, status ok.
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	client := strava.NewWithBase("123", "secret", "http://cb", srv.URL)

	// Garmin worker fails.
	script := filepath.Join(t.TempDir(), "fail.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho boom 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	r := garmin.Runner{Python: "/bin/sh", Script: script}

	out := SyncAll(context.Background(), s, client, r, nil)
	if out.Strava.Status != "ok" {
		t.Errorf("strava = %+v, want ok", out.Strava)
	}
	if out.Garmin.Status != "error" || out.Garmin.Error == nil {
		t.Errorf("garmin = %+v, want error", out.Garmin)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/sync/ -run SyncAll`. Expected: build failure `undefined: SyncAll`, `undefined: AllResult` → `FAIL backend/internal/sync [build failed]`.

- [ ] **Step 3: Add `SyncAll` + `AllResult` to `sync.go`.** Append to `backend/internal/sync/sync.go`:
```go

// AllResult is the combined sync outcome for both sources (the /api/sync body).
type AllResult struct {
	Strava SourceResult
	Garmin SourceResult
}

// SyncAll runs both syncs sequentially (single SQLite writer) and returns both
// results. A failure in one source never aborts the other.
func SyncAll(ctx context.Context, s *store.Store, client *strava.Client, r garmin.Runner, extraEnv []string) AllResult {
	return AllResult{
		Strava: SyncStrava(ctx, s, client),
		Garmin: SyncGarmin(ctx, s, r, extraEnv),
	}
}
```

- [ ] **Step 4: Run the test, expect PASS.** Command: `cd backend && go test ./internal/sync/ -run SyncAll`. Expected: `ok  	help-my-run/backend/internal/sync` (`TestSyncAllPartialFailure --- PASS`).

- [ ] **Step 5: Run the whole sync package** for regressions. Command: `cd backend && go test ./internal/sync/`. Expected: `ok  	help-my-run/backend/internal/sync` (all sync tests PASS).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/sync && git commit -m "feat(sync): add SyncAll combining Strava and Garmin with partial-failure tolerance"
```

---
### Task 24: Sync — periodic ticker

**Files:**
- Create: `backend/internal/sync/ticker.go`
- Modify: `backend/internal/sync/sync_test.go`

- [ ] **Step 1: Write the failing test** that runs `RunTicker` with a short interval and a context cancelled after a couple ticks, asserting the supplied sync function was called at least once and the loop stops on cancel. Append to `backend/internal/sync/sync_test.go`:
```go

func TestRunTickerCallsAndStops(t *testing.T) {
	var calls int32
	fn := func(ctx context.Context) { atomic.AddInt32(&calls, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunTicker(ctx, 10*time.Millisecond, fn)
		close(done)
	}()

	// Let a few ticks happen, then cancel.
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

- [ ] **Step 2: Add the new test imports.** Update the `backend/internal/sync/sync_test.go` import block to also include `sync/atomic` (final list: `context`, `net/http`, `net/http/httptest`, `os`, `path/filepath`, `runtime`, `strings`, `sync/atomic`, `testing`, `time`, plus the three internal packages):
```go
import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)
```

- [ ] **Step 3: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/sync/ -run RunTicker`. Expected: build failure `undefined: RunTicker` → `FAIL backend/internal/sync [build failed]`.

- [ ] **Step 4: Write `ticker.go`.** Create `backend/internal/sync/ticker.go`:
```go
package sync

import (
	"context"
	"time"
)

// RunTicker calls fn every interval until ctx is cancelled. fn receives the
// same ctx so a cancellation also aborts an in-flight sync. RunTicker does not
// run fn immediately on start; the first call happens after one interval.
func RunTicker(ctx context.Context, interval time.Duration, fn func(context.Context)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}
```

- [ ] **Step 5: Run the test, expect PASS.** Command: `cd backend && go test ./internal/sync/ -run RunTicker`. Expected: `ok  	help-my-run/backend/internal/sync` (`TestRunTickerCallsAndStops --- PASS`).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/sync && git commit -m "feat(sync): add periodic RunTicker loop with context cancellation"
```

---

### Task 25: API — DTOs + bearer-auth middleware

**Files:**
- Create: `backend/internal/api/dto.go`
- Create: `backend/internal/api/auth.go`
- Create: `backend/internal/api/auth_test.go`

- [ ] **Step 1: Write the failing test** for `BearerAuth(token)` middleware: accepts the right token, rejects missing/wrong with `401` and body `{"error":"unauthorized"}`. Create `backend/internal/api/auth_test.go`:
```go
package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBearerAuth(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("protected"))
	})
	mw := BearerAuth("secret-token")
	h := mw(okHandler)

	tests := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{"valid", "Bearer secret-token", http.StatusOK},
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "Bearer nope", http.StatusUnauthorized},
		{"no-prefix", "secret-token", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusUnauthorized {
				if !strings.Contains(rec.Body.String(), `"error":"unauthorized"`) {
					t.Errorf("body = %q, want it to contain {\"error\":\"unauthorized\"}", rec.Body.String())
				}
				if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/api/ -run BearerAuth`. Expected: build failure `undefined: BearerAuth` → `FAIL backend/internal/api [build failed]`.

- [ ] **Step 3: Write `auth.go`.** Create `backend/internal/api/auth.go`:
```go
package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

// BearerAuth returns middleware that requires Authorization: Bearer <token>.
// Comparison is constant-time. Failures return 401 with {"error":"unauthorized"}.
func BearerAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if token == "" || !strings.HasPrefix(h, bearerPrefix) ||
				subtle.ConstantTimeCompare(
					[]byte(strings.TrimPrefix(h, bearerPrefix)), []byte(token)) != 1 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Write `dto.go`** with the response structs (exact contract JSON keys) and the `writeJSON` helper. Create `backend/internal/api/dto.go`:
```go
package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON writes v as application/json with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- /health ---
type healthResp struct {
	Status string `json:"status"`
}

// --- /api/status ---
type sourceStatus struct {
	Connected    bool    `json:"connected"`
	LastSyncedAt *string `json:"last_synced_at"`
	LastRunAt    *string `json:"last_run_at"`
	Status       string  `json:"status"`
	Error        *string `json:"error"`
}
type stravaStatus struct {
	sourceStatus
	AthleteID *int64 `json:"athlete_id"`
}
type statusCounts struct {
	Activities   int `json:"activities"`
	RecoveryDays int `json:"recovery_days"`
}
type statusResp struct {
	Strava stravaStatus `json:"strava"`
	Garmin sourceStatus `json:"garmin"`
	Counts statusCounts `json:"counts"`
}

// --- /api/strava/connect ---
type connectResp struct {
	AuthorizeURL string `json:"authorizeUrl"`
}

// --- /api/sync ---
type syncSourceResult struct {
	Status string  `json:"status"`
	Synced int     `json:"synced"`
	Error  *string `json:"error"`
}
type syncResp struct {
	Strava syncSourceResult `json:"strava"`
	Garmin syncSourceResult `json:"garmin"`
}

// --- /api/activities ---
type activityDTO struct {
	StravaID       int64    `json:"strava_id"`
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	SportType      *string  `json:"sport_type"`
	StartTime      string   `json:"start_time"`
	StartTimeLocal *string  `json:"start_time_local"`
	DistanceM      float64  `json:"distance_m"`
	MovingTimeS    int64    `json:"moving_time_s"`
	ElapsedTimeS   int64    `json:"elapsed_time_s"`
	AvgHR          *float64 `json:"avg_hr"`
	MaxHR          *float64 `json:"max_hr"`
	AvgSpeed       *float64 `json:"avg_speed"`
	MaxSpeed       *float64 `json:"max_speed"`
	AvgCadence     *float64 `json:"avg_cadence"`
	ElevationGainM *float64 `json:"elevation_gain_m"`
}
type activitiesResp struct {
	Activities []activityDTO `json:"activities"`
}

// --- /api/recovery ---
type sleepDTO struct {
	DurationS *int64 `json:"duration_s"`
	DeepS     *int64 `json:"deep_s"`
	LightS    *int64 `json:"light_s"`
	RemS      *int64 `json:"rem_s"`
	AwakeS    *int64 `json:"awake_s"`
	Score     *int64 `json:"score"`
}
type hrvDTO struct {
	LastNightAvgMs *int64  `json:"last_night_avg_ms"`
	Status         *string `json:"status"`
}
type bodyBatteryDTO struct {
	Charged *int64 `json:"charged"`
	Drained *int64 `json:"drained"`
	High    *int64 `json:"high"`
	Low     *int64 `json:"low"`
}
type rhrDTO struct {
	RestingHR *int64 `json:"resting_hr"`
}
type recoveryDayDTO struct {
	Date        string          `json:"date"`
	Sleep       *sleepDTO       `json:"sleep"`
	HRV         *hrvDTO         `json:"hrv"`
	BodyBattery *bodyBatteryDTO `json:"body_battery"`
	RHR         *rhrDTO         `json:"rhr"`
}
type recoveryResp struct {
	Recovery []recoveryDayDTO `json:"recovery"`
}
```

- [ ] **Step 5: Run the test, expect PASS.** Command: `cd backend && go test ./internal/api/ -run BearerAuth`. Expected: `ok  	help-my-run/backend/internal/api` (`TestBearerAuth` with all four subtests `--- PASS`).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/api && git commit -m "feat(api): add response DTOs and constant-time bearer-auth middleware"
```

---
### Task 26: API — router + health/status/connect handlers

**Files:**
- Create: `backend/internal/api/router.go`
- Create: `backend/internal/api/handlers.go`
- Create: `backend/internal/api/handlers_test.go`

- [ ] **Step 1: Write the failing test** for the router wiring `/health` (no auth), and auth-gated `/api/status` + `/api/strava/connect`, including auth rejection. Create `backend/internal/api/handlers_test.go`:
```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

const testToken = "test-token"

func newTestServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	deps := Deps{
		Store:        s,
		Strava:       strava.NewWithBase("12345", "secret", "http://localhost:8080/api/strava/callback", "https://www.strava.com"),
		APIToken:     testToken,
		SyncFunc:     func(ctx context.Context) (string, int, *string, string, int, *string) { return "ok", 0, nil, "ok", 0, nil },
	}
	return NewRouter(deps), s
}

func do(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthNoAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body healthResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestStatusRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/status", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"unauthorized"`) {
		t.Errorf("body = %q, want unauthorized", rec.Body.String())
	}
}

func TestStatusOK(t *testing.T) {
	h, s := newTestServer(t)
	// Connect Strava + add one recovery day so counts are non-zero.
	_ = s.SaveStravaTokens(store.StravaTokens{
		AccessToken: "a", RefreshToken: "r", ExpiresAt: 4102444800, Scope: "activity:read_all", AthleteID: 999,
	})
	_ = s.UpsertRhr(store.RhrRow{Date: "2026-06-18", RestingHR: i64(47), RawJSON: "{}"})

	rec := do(t, h, http.MethodGet, "/api/status", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body statusResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Strava.Connected || body.Strava.AthleteID == nil || *body.Strava.AthleteID != 999 {
		t.Errorf("strava = %+v, want connected athlete 999", body.Strava)
	}
	if body.Strava.Status != "never" {
		t.Errorf("strava.status = %q, want never (seeded)", body.Strava.Status)
	}
	if !body.Garmin.Connected {
		t.Errorf("garmin.connected = %v, want true (one rhr row)", body.Garmin.Connected)
	}
	if body.Counts.RecoveryDays != 1 {
		t.Errorf("counts.recovery_days = %d, want 1", body.Counts.RecoveryDays)
	}
}

func TestConnectURL(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/strava/connect", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body connectResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(body.AuthorizeURL, "https://www.strava.com/oauth/authorize?") {
		t.Errorf("authorizeUrl = %q, want strava authorize URL", body.AuthorizeURL)
	}
	if !strings.Contains(body.AuthorizeURL, "scope=activity%3Aread_all") {
		t.Errorf("authorizeUrl = %q, want scope activity:read_all", body.AuthorizeURL)
	}
}

func i64(v int64) *int64 { return &v }
func ptrTime() string    { return time.Now().UTC().Format(time.RFC3339) }
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/api/ -run 'Health|Status|Connect'`. Expected: build failure `undefined: Deps`, `undefined: NewRouter` → `FAIL backend/internal/api [build failed]`.

- [ ] **Step 3: Write `router.go`** with `Deps`, route wiring, and chi middleware. Create `backend/internal/api/router.go`:
```go
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

// SyncFunc runs both syncs and returns flattened per-source results:
// (stravaStatus, stravaSynced, stravaErr, garminStatus, garminSynced, garminErr).
// Wiring (main.go) adapts the sync package to this signature so the api package
// does not import sync (avoids an import cycle and keeps handlers testable).
type SyncFunc func(ctx context.Context) (string, int, *string, string, int, *string)

// Deps are the handler dependencies injected by main.go (and tests).
type Deps struct {
	Store    *store.Store
	Strava   *strava.Client
	APIToken string
	SyncFunc SyncFunc
}

// NewRouter builds the chi router with public + bearer-protected routes.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(120 * time.Second))

	h := &handlers{d: d}

	// Public (no auth).
	r.Get("/health", h.health)
	r.Get("/api/strava/callback", h.stravaCallback)

	// Protected.
	r.Group(func(r chi.Router) {
		r.Use(BearerAuth(d.APIToken))
		r.Get("/api/status", h.status)
		r.Get("/api/strava/connect", h.stravaConnect)
		r.Post("/api/sync", h.sync)
		r.Get("/api/activities", h.activities)
		r.Get("/api/recovery", h.recovery)
	})

	return r
}
```

- [ ] **Step 4: Write `handlers.go`** with health/status/connect (the other three are added in the next tasks; include stubs that return 200-shaped empties for `sync`/`activities`/`recovery` so the package compiles now). Create `backend/internal/api/handlers.go`:
```go
package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"help-my-run/backend/internal/store"
)

type handlers struct {
	d Deps
}

func (h *handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, healthResp{Status: "ok"})
}

func (h *handlers) status(w http.ResponseWriter, r *http.Request) {
	s := h.d.Store

	// Strava connection = a token row exists.
	var stravaConn bool
	var athleteID *int64
	if tok, err := s.GetStravaTokens(); err == nil {
		stravaConn = true
		id := tok.AthleteID
		athleteID = &id
	}

	recoveryDays, _ := s.CountRecoveryDays()
	garminConn := recoveryDays > 0

	var activitiesCount int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM activities`).Scan(&activitiesCount)

	stravaLog, _ := s.GetSyncLog("strava")
	garminLog, _ := s.GetSyncLog("garmin")

	resp := statusResp{
		Strava: stravaStatus{
			sourceStatus: sourceStatus{
				Connected:    stravaConn,
				LastSyncedAt: stravaLog.LastSyncedAt,
				LastRunAt:    stravaLog.LastRunAt,
				Status:       stravaLog.Status,
				Error:        stravaLog.Error,
			},
			AthleteID: athleteID,
		},
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

func (h *handlers) stravaConnect(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	url := h.d.Strava.AuthorizeURL(state)
	writeJSON(w, http.StatusOK, connectResp{AuthorizeURL: url})
}

func randomState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "state"
	}
	return hex.EncodeToString(b)
}

// --- handlers completed in the next tasks; minimal compiling stubs for now ---

func (h *handlers) stravaCallback(w http.ResponseWriter, r *http.Request) {
	writeHTML(w, "You can close this tab.")
}

func (h *handlers) sync(w http.ResponseWriter, r *http.Request) {
	ss, ssn, sErr, gs, gsn, gErr := h.d.SyncFunc(r.Context())
	writeJSON(w, http.StatusOK, syncResp{
		Strava: syncSourceResult{Status: ss, Synced: ssn, Error: sErr},
		Garmin: syncSourceResult{Status: gs, Synced: gsn, Error: gErr},
	})
}

func (h *handlers) activities(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, activitiesResp{Activities: []activityDTO{}})
}

func (h *handlers) recovery(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, recoveryResp{Recovery: []recoveryDayDTO{}})
}

func writeHTML(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("<!doctype html><html><body><p>" + msg + "</p></body></html>"))
}

var _ = store.ErrNotFound // store import kept for upcoming handlers
```

- [ ] **Step 5: Run the test, expect PASS.** Command: `cd backend && go test ./internal/api/ -run 'Health|Status|Connect'`. Expected: `ok  	help-my-run/backend/internal/api` (`TestHealthNoAuth`, `TestStatusRequiresAuth`, `TestStatusOK`, `TestConnectURL` `--- PASS`).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/api && git commit -m "feat(api): add chi router with health, status, and strava/connect handlers"
```

---
### Task 27: API — activities + recovery handlers (limits + mapping)

**Files:**
- Modify: `backend/internal/api/handlers.go`
- Modify: `backend/internal/api/handlers_test.go`

- [ ] **Step 1: Write the failing test** for `/api/activities` (default + `limit` clamping + mapping) and `/api/recovery` (default + `days` + null sub-records). Append to `backend/internal/api/handlers_test.go`:
```go

func TestActivitiesHandler(t *testing.T) {
	h, s := newTestServer(t)
	_ = s.UpsertActivity(store.Activity{
		StravaID: 11, Name: "A", Type: "Run", SportType: sp("Run"),
		StartTime: "2026-06-18T06:00:00Z", StartTimeLocal: sp("2026-06-18T08:00:00"),
		DistanceM: 10000, MovingTimeS: 3000, ElapsedTimeS: 3050,
		AvgHR: fp(150), RawJSON: "{}",
	})
	_ = s.UpsertActivity(store.Activity{
		StravaID: 12, Name: "B", Type: "Run", SportType: nil,
		StartTime: "2026-06-17T06:00:00Z", DistanceM: 5000,
		MovingTimeS: 1500, ElapsedTimeS: 1500, RawJSON: "{}",
	})

	rec := do(t, h, http.MethodGet, "/api/activities", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body activitiesResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Activities) != 2 {
		t.Fatalf("len = %d, want 2", len(body.Activities))
	}
	// Most-recent-first.
	if body.Activities[0].StravaID != 11 {
		t.Errorf("first id = %d, want 11", body.Activities[0].StravaID)
	}
	if body.Activities[0].AvgHR == nil || *body.Activities[0].AvgHR != 150 {
		t.Errorf("avg_hr = %v, want 150", body.Activities[0].AvgHR)
	}
	if body.Activities[1].SportType != nil {
		t.Errorf("sport_type = %v, want null", body.Activities[1].SportType)
	}

	// limit=1 clamps.
	rec = do(t, h, http.MethodGet, "/api/activities?limit=1", testToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Activities) != 1 {
		t.Errorf("limit=1 len = %d, want 1", len(body.Activities))
	}
}

func TestRecoveryHandler(t *testing.T) {
	h, s := newTestServer(t)
	_ = s.UpsertSleep(store.SleepRow{Date: "2026-06-18", DurationS: i64(27000), Score: i64(82), RawJSON: "{}"})
	_ = s.UpsertRhr(store.RhrRow{Date: "2026-06-18", RestingHR: i64(47), RawJSON: "{}"})
	// 06-17: rhr only.
	_ = s.UpsertRhr(store.RhrRow{Date: "2026-06-17", RestingHR: i64(49), RawJSON: "{}"})

	rec := do(t, h, http.MethodGet, "/api/recovery", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body recoveryResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Recovery) != 2 {
		t.Fatalf("len = %d, want 2", len(body.Recovery))
	}
	d18 := body.Recovery[0]
	if d18.Date != "2026-06-18" || d18.Sleep == nil || d18.Sleep.Score == nil || *d18.Sleep.Score != 82 {
		t.Errorf("06-18 sleep wrong: %+v", d18)
	}
	if d18.HRV != nil || d18.BodyBattery != nil {
		t.Errorf("06-18 hrv/bb = %v/%v, want both null", d18.HRV, d18.BodyBattery)
	}
	if d18.RHR == nil || *d18.RHR.RestingHR != 47 {
		t.Errorf("06-18 rhr wrong: %+v", d18.RHR)
	}
	// days=1 clamps to the most-recent date.
	rec = do(t, h, http.MethodGet, "/api/recovery?days=1", testToken)
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if len(body.Recovery) != 1 || body.Recovery[0].Date != "2026-06-18" {
		t.Errorf("days=1 = %+v, want single 2026-06-18", body.Recovery)
	}
}

func sp(v string) *string  { return &v }
func fp(v float64) *float64 { return &v }
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/api/ -run 'ActivitiesHandler|RecoveryHandler'`. Expected: the stubs return empty arrays → `TestActivitiesHandler` fails with `len = 0, want 2` and `TestRecoveryHandler` fails with `len = 0, want 2` → `FAIL`.

- [ ] **Step 3: Replace the `activities` and `recovery` stubs** with real implementations (query-param parsing + clamping + mapping). In `backend/internal/api/handlers.go`, replace the two stub funcs:
```go
func (h *handlers) activities(w http.ResponseWriter, r *http.Request) {
	limit := clampQuery(r, "limit", 30, 1, 200)
	rows, err := h.d.Store.ListActivities(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]activityDTO, 0, len(rows))
	for _, a := range rows {
		out = append(out, activityDTO{
			StravaID: a.StravaID, Name: a.Name, Type: a.Type, SportType: a.SportType,
			StartTime: a.StartTime, StartTimeLocal: a.StartTimeLocal,
			DistanceM: a.DistanceM, MovingTimeS: a.MovingTimeS, ElapsedTimeS: a.ElapsedTimeS,
			AvgHR: a.AvgHR, MaxHR: a.MaxHR, AvgSpeed: a.AvgSpeed, MaxSpeed: a.MaxSpeed,
			AvgCadence: a.AvgCadence, ElevationGainM: a.ElevationGainM,
		})
	}
	writeJSON(w, http.StatusOK, activitiesResp{Activities: out})
}

func (h *handlers) recovery(w http.ResponseWriter, r *http.Request) {
	days := clampQuery(r, "days", 30, 1, 365)
	rows, err := h.d.Store.ListRecovery(days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]recoveryDayDTO, 0, len(rows))
	for _, d := range rows {
		rd := recoveryDayDTO{Date: d.Date}
		if d.Sleep != nil {
			rd.Sleep = &sleepDTO{
				DurationS: d.Sleep.DurationS, DeepS: d.Sleep.DeepS, LightS: d.Sleep.LightS,
				RemS: d.Sleep.RemS, AwakeS: d.Sleep.AwakeS, Score: d.Sleep.Score,
			}
		}
		if d.HRV != nil {
			rd.HRV = &hrvDTO{LastNightAvgMs: d.HRV.LastNightAvgMs, Status: d.HRV.Status}
		}
		if d.BodyBattery != nil {
			rd.BodyBattery = &bodyBatteryDTO{
				Charged: d.BodyBattery.Charged, Drained: d.BodyBattery.Drained,
				High: d.BodyBattery.High, Low: d.BodyBattery.Low,
			}
		}
		if d.RHR != nil {
			rd.RHR = &rhrDTO{RestingHR: d.RHR.RestingHR}
		}
		out = append(out, rd)
	}
	writeJSON(w, http.StatusOK, recoveryResp{Recovery: out})
}

// clampQuery parses an int query param, applying a default and [min,max] clamp.
func clampQuery(r *http.Request, key string, def, min, max int) int {
	v := def
	if raw := r.URL.Query().Get(key); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			v = n
		}
	}
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return v
}
```

- [ ] **Step 4: Add the `strconv` import** to `handlers.go`. Update the import block in `backend/internal/api/handlers.go` to:
```go
import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"

	"help-my-run/backend/internal/store"
)
```

- [ ] **Step 5: Run the test, expect PASS.** Command: `cd backend && go test ./internal/api/ -run 'ActivitiesHandler|RecoveryHandler'`. Expected: `ok  	help-my-run/backend/internal/api` (`TestActivitiesHandler`, `TestRecoveryHandler` `--- PASS`).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/api && git commit -m "feat(api): implement activities and recovery handlers with limit clamping"
```

---

### Task 28: API — strava/callback (exchange + persist) + sync handler

**Files:**
- Modify: `backend/internal/api/handlers.go`
- Modify: `backend/internal/api/handlers_test.go`

- [ ] **Step 1: Write the failing test** for `/api/strava/callback` (no auth; exchanges code via a base-URL-injected Strava client pointed at an httptest server; persists tokens; returns the closeable HTML), the `error=access_denied` path, and `/api/sync` returning both per-source results from the injected `SyncFunc`. Append to `backend/internal/api/handlers_test.go`:
```go

func TestStravaCallbackExchangesAndPersists(t *testing.T) {
	// Fake Strava token endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token" {
			t.Errorf("path = %s, want /oauth/token", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token_type":"Bearer","access_token":"acc","refresh_token":"ref","expires_at":4102444800,"expires_in":21600,"scope":"read,activity:read_all","athlete":{"id":777}}`))
	}))
	defer srv.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "cb.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	deps := Deps{
		Store:    s,
		Strava:   strava.NewWithBase("12345", "secret", "http://localhost:8080/api/strava/callback", srv.URL),
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) { return "ok", 0, nil, "ok", 0, nil },
	}
	h := NewRouter(deps)

	// Callback has NO auth header.
	rec := do(t, h, http.MethodGet, "/api/strava/callback?code=the-code&scope=read,activity:read_all&state=xyz", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "You can close this tab.") {
		t.Errorf("body = %q, want close-tab text", rec.Body.String())
	}
	tok, err := s.GetStravaTokens()
	if err != nil {
		t.Fatalf("tokens not persisted: %v", err)
	}
	if tok.AccessToken != "acc" || tok.RefreshToken != "ref" || tok.AthleteID != 777 {
		t.Errorf("tokens = %+v, want acc/ref/777", tok)
	}
}

func TestStravaCallbackAccessDenied(t *testing.T) {
	h, s := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/strava/callback?error=access_denied", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "failed") {
		t.Errorf("body = %q, want failure text", rec.Body.String())
	}
	// No tokens stored.
	if _, err := s.GetStravaTokens(); err != store.ErrNotFound {
		t.Errorf("GetStravaTokens err = %v, want ErrNotFound", err)
	}
}

func TestSyncHandler(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	gerr := "worker exit 1: re-run worker.py login"
	deps := Deps{
		Store:    s,
		Strava:   strava.NewWithBase("1", "x", "http://cb", "http://unused"),
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) {
			return "ok", 3, nil, "error", 0, &gerr
		},
	}
	h := NewRouter(deps)

	rec := do(t, h, http.MethodPost, "/api/sync", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body syncResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Strava.Status != "ok" || body.Strava.Synced != 3 {
		t.Errorf("strava = %+v, want ok/3", body.Strava)
	}
	if body.Garmin.Status != "error" || body.Garmin.Error == nil || *body.Garmin.Error != gerr {
		t.Errorf("garmin = %+v, want error with %q", body.Garmin, gerr)
	}
}

func TestSyncRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodPost, "/api/sync", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./internal/api/ -run 'Callback|SyncHandler'`. Expected: `TestStravaCallbackExchangesAndPersists` fails at `tokens not persisted: store: not found` (the stub callback never exchanges), and `TestStravaCallbackAccessDenied` fails at body-not-"failed" → `FAIL`.

- [ ] **Step 3: Replace the `stravaCallback` stub** with the real exchange-and-persist implementation. In `backend/internal/api/handlers.go`, replace the `stravaCallback` func:
```go
func (h *handlers) stravaCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		writeHTML(w, "Strava connection failed. You can close this tab.")
		return
	}
	code := r.URL.Query().Get("code")
	tok, err := h.d.Strava.Exchange(r.Context(), code)
	if err != nil {
		writeHTML(w, "Strava connection failed. You can close this tab.")
		return
	}
	st := store.StravaTokens{
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		ExpiresAt:    tok.ExpiresAt,
		Scope:        tok.Scope,
	}
	if tok.Athlete != nil {
		st.AthleteID = tok.Athlete.ID
	}
	if err := h.d.Store.SaveStravaTokens(st); err != nil {
		writeHTML(w, "Strava connection failed. You can close this tab.")
		return
	}
	writeHTML(w, "Strava connected. You can close this tab.")
}
```

- [ ] **Step 4: Run the test, expect PASS.** Command: `cd backend && go test ./internal/api/ -run 'Callback|SyncHandler'`. Expected: `ok  	help-my-run/backend/internal/api` (`TestStravaCallbackExchangesAndPersists`, `TestStravaCallbackAccessDenied`, `TestSyncHandler`, `TestSyncRequiresAuth` `--- PASS`).

- [ ] **Step 5: Run the whole api package** for regressions. Command: `cd backend && go test ./internal/api/`. Expected: `ok  	help-my-run/backend/internal/api` (all api tests PASS).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/api && git commit -m "feat(api): implement strava callback exchange and wired sync handler"
```

---
### Task 29: cmd/server/main.go — wiring + ticker + build/run verification

**Files:**
- Create: `backend/cmd/server/main.go` (OVERWRITES the scaffold stub from Task 3)
- Test: `backend/cmd/server/main_test.go`

- [ ] **Step 1: Write the failing test** that builds `Wire(cfg)` (the testable wiring: store + migrate + router + adapted SyncFunc) and asserts `/health` works and `/api/status` is auth-gated, using a temp DB path. Create `backend/cmd/server/main_test.go`:
```go
package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/config"
)

func TestWireServesHealthAndAuth(t *testing.T) {
	cfg := &config.Config{
		StravaClientID:     "12345",
		StravaClientSecret: "secret",
		StravaRedirectURL:  "http://localhost:8080/api/strava/callback",
		APIToken:           "tok",
		DBPath:             filepath.Join(t.TempDir(), "wire.db"),
		Port:               "8080",
		PythonBin:          "/bin/cat",
		WorkerScript:       "/dev/null",
	}

	app, err := Wire(cfg)
	if err != nil {
		t.Fatalf("Wire error = %v", err)
	}
	t.Cleanup(func() { _ = app.Store.Close() })

	// /health: no auth, 200.
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/health = %d, want 200", rec.Code)
	}

	// /api/status without token: 401.
	rec = httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/status (no auth) = %d, want 401", rec.Code)
	}

	// /api/status with token: 200 (DB migrated, sync_log seeded).
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec = httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/status (auth) = %d, want 200", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.** Command: `cd backend && go test ./cmd/server/`. Expected: build failure `undefined: Wire`, `undefined: App` → `FAIL backend/cmd/server [build failed]`.

- [ ] **Step 3: Write `main.go`** with a testable `Wire` (returns `App`) plus `main()` that loads config, wires, starts the ticker, and listens. Create `backend/cmd/server/main.go`:
```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"help-my-run/backend/internal/api"
	"help-my-run/backend/internal/config"
	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
	syncpkg "help-my-run/backend/internal/sync"
)

// syncInterval is how often the periodic sync ticker fires.
const syncInterval = 6 * time.Hour

// App is the wired application graph (returned by Wire so tests can drive it).
type App struct {
	Store   *store.Store
	Handler http.Handler
	Strava  *strava.Client
	Runner  garmin.Runner
	Cfg     *config.Config
}

// Wire builds the full application graph from config: opens + migrates the
// store, constructs the Strava client and Garmin runner, and builds the router
// with a SyncFunc adapter that runs SyncAll.
func Wire(cfg *config.Config) (*App, error) {
	s, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	if err := s.Migrate(); err != nil {
		_ = s.Close()
		return nil, err
	}

	stravaClient := strava.New(cfg.StravaClientID, cfg.StravaClientSecret, cfg.StravaRedirectURL)
	runner := garmin.Runner{Python: cfg.PythonBin, Script: cfg.WorkerScript}
	extraEnv := garminEnv(cfg)

	syncFunc := func(ctx context.Context) (string, int, *string, string, int, *string) {
		res := syncpkg.SyncAll(ctx, s, stravaClient, runner, extraEnv)
		return res.Strava.Status, res.Strava.Synced, res.Strava.Error,
			res.Garmin.Status, res.Garmin.Synced, res.Garmin.Error
	}

	handler := api.NewRouter(api.Deps{
		Store:    s,
		Strava:   stravaClient,
		APIToken: cfg.APIToken,
		SyncFunc: syncFunc,
	})

	return &App{Store: s, Handler: handler, Strava: stravaClient, Runner: runner, Cfg: cfg}, nil
}

// garminEnv builds the env passed through to the worker subprocess.
func garminEnv(cfg *config.Config) []string {
	return []string{
		"GARMIN_EMAIL=" + cfg.GarminEmail,
		"GARMIN_PASSWORD=" + cfg.GarminPassword,
		"GARMIN_TOKENSTORE=" + cfg.GarminTokenstore,
	}
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app, err := Wire(cfg)
	if err != nil {
		log.Fatalf("wire: %v", err)
	}
	defer func() { _ = app.Store.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Periodic sync ticker (the agentic schedule is M2; this is plain periodic).
	stravaClient := app.Strava
	runner := app.Runner
	extraEnv := garminEnv(cfg)
	go syncpkg.RunTicker(ctx, syncInterval, func(c context.Context) {
		res := syncpkg.SyncAll(c, app.Store, stravaClient, runner, extraEnv)
		log.Printf("periodic sync: strava=%s/%d garmin=%s/%d",
			res.Strava.Status, res.Strava.Synced, res.Garmin.Status, res.Garmin.Synced)
	})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	_ = os.Stdout.Sync()
}
```

- [ ] **Step 4: Run the test, expect PASS.** Command: `cd backend && go test ./cmd/server/`. Expected: `ok  	help-my-run/backend/cmd/server` (`TestWireServesHealthAndAuth --- PASS`).

- [ ] **Step 5: Build the whole backend** to verify it compiles end-to-end. Command: `cd backend && go build ./...`. Expected: no output, exit code 0 (a clean build of all packages incl. `cmd/server`).

- [ ] **Step 6: Run the full backend test suite** to confirm everything is green together. Command: `cd backend && go test ./...`. Expected: every package reports `ok` (`config`, `store`, `strava`, `garmin`, `sync`, `api`, `cmd/server`) with no `FAIL`.

- [ ] **Step 7: Verify the binary runs and serves /health.** Command:
```
cd backend && STRAVA_CLIENT_ID=1 STRAVA_CLIENT_SECRET=x STRAVA_REDIRECT_URL=http://localhost:8080/api/strava/callback API_TOKEN=tok DB_PATH=$(mktemp -d)/run.db PORT=8099 go run ./cmd/server & SRV=$!; sleep 2; curl -s http://localhost:8099/health; kill $SRV
```
Expected: prints `{"status":"ok"}` (the server logs `listening on :8099`, then is killed).

- [ ] **Step 8: Commit.** Command:
```
git add backend/cmd && git commit -m "feat(server): wire config, store, router, and periodic sync ticker in main"
```

---
### Task 30: App — typed API client (`client.ts`) with Bearer + base URL + ApiError

**Files:**
- Create: `app/src/api/config.ts`
- Create: `app/src/api/client.ts`
- Test: `app/src/api/__tests__/client.test.ts`

- [ ] **Step 1: Create the secure-store config helpers (dependency of the client) at `app/src/api/config.ts`.** The client reads `getBaseUrl`/`getToken` from here; write it first so the client compiles. Full code:
```ts
import * as SecureStore from 'expo-secure-store';

const BASE_URL_KEY = 'hmr.baseUrl';
const TOKEN_KEY = 'hmr.token';

export async function saveConfig(baseUrl: string, token: string): Promise<void> {
  await SecureStore.setItemAsync(BASE_URL_KEY, baseUrl);
  await SecureStore.setItemAsync(TOKEN_KEY, token);
}

export async function getBaseUrl(): Promise<string | null> {
  return SecureStore.getItemAsync(BASE_URL_KEY);
}

export async function getToken(): Promise<string | null> {
  return SecureStore.getItemAsync(TOKEN_KEY);
}

export async function clearConfig(): Promise<void> {
  await SecureStore.deleteItemAsync(BASE_URL_KEY);
  await SecureStore.deleteItemAsync(TOKEN_KEY);
}
```

- [ ] **Step 2: Write the failing test for the API client at `app/src/api/__tests__/client.test.ts`.** It mocks `./config` (so `getBaseUrl`/`getToken` are deterministic) and mocks global `fetch`. It asserts: base URL is prepended, `Authorization: Bearer <token>` and `Content-Type: application/json` headers are set, GET returns parsed JSON, POST serializes a body and uses method POST, a non-ok response throws `ApiError` with the right `status`, a missing base URL throws `ApiError(0, ...)`, and a 204 returns `undefined`. Full test code:
```ts
import { apiGet, apiPost, ApiError } from '../client';

jest.mock('../config', () => ({
  getBaseUrl: jest.fn(),
  getToken: jest.fn(),
}));

import { getBaseUrl, getToken } from '../config';

const mockGetBaseUrl = getBaseUrl as jest.MockedFunction<typeof getBaseUrl>;
const mockGetToken = getToken as jest.MockedFunction<typeof getToken>;

function mockFetchOnce(opts: { ok: boolean; status: number; json?: unknown }) {
  (global.fetch as jest.Mock).mockResolvedValueOnce({
    ok: opts.ok,
    status: opts.status,
    json: async () => opts.json,
  });
}

beforeEach(() => {
  global.fetch = jest.fn() as jest.Mock;
  mockGetBaseUrl.mockResolvedValue('http://localhost:8080');
  mockGetToken.mockResolvedValue('test-token');
});

afterEach(() => {
  jest.clearAllMocks();
});

describe('apiGet', () => {
  it('prepends base URL and sends bearer + content-type headers', async () => {
    mockFetchOnce({ ok: true, status: 200, json: { status: 'ok' } });

    const data = await apiGet<{ status: string }>('/health');

    expect(global.fetch).toHaveBeenCalledTimes(1);
    const [url, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(url).toBe('http://localhost:8080/health');
    expect(init.headers).toMatchObject({
      'Content-Type': 'application/json',
      Authorization: 'Bearer test-token',
    });
    expect(data).toEqual({ status: 'ok' });
  });

  it('omits Authorization header when no token is stored', async () => {
    mockGetToken.mockResolvedValue(null);
    mockFetchOnce({ ok: true, status: 200, json: {} });

    await apiGet('/api/status');

    const [, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(init.headers.Authorization).toBeUndefined();
  });

  it('throws ApiError with the response status on non-ok response', async () => {
    mockFetchOnce({ ok: false, status: 401, json: { error: 'unauthorized' } });

    // Capture the rejection ONCE (only one mock is queued) and assert both
    // its shape and its instanceof on that single error.
    const err = await apiGet('/api/status').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ name: 'ApiError', status: 401 });
  });

  it('throws ApiError(0) when base URL is not configured', async () => {
    mockGetBaseUrl.mockResolvedValue(null);

    await expect(apiGet('/api/status')).rejects.toMatchObject({
      status: 0,
      message: 'Backend URL not configured',
    });
    expect(global.fetch).not.toHaveBeenCalled();
  });
});

describe('apiPost', () => {
  it('uses POST and serializes the body', async () => {
    mockFetchOnce({
      ok: true,
      status: 200,
      json: { strava: { status: 'ok', synced: 1, error: null } },
    });

    const data = await apiPost<{ strava: { status: string } }>('/api/sync', { foo: 1 });

    const [url, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(url).toBe('http://localhost:8080/api/sync');
    expect(init.method).toBe('POST');
    expect(init.body).toBe(JSON.stringify({ foo: 1 }));
    expect(data).toEqual({ strava: { status: 'ok', synced: 1, error: null } });
  });

  it('sends no body when none is provided', async () => {
    mockFetchOnce({ ok: true, status: 200, json: {} });

    await apiPost('/api/sync');

    const [, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(init.method).toBe('POST');
    expect(init.body).toBeUndefined();
  });

  it('returns undefined for a 204 No Content response', async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      status: 204,
      json: async () => {
        throw new Error('should not parse json on 204');
      },
    });

    const data = await apiPost('/api/sync');
    expect(data).toBeUndefined();
  });
});
```

- [ ] **Step 3: Run the test and confirm it FAILS (no implementation yet).** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/client.test.ts
```
Expected FAIL output (module does not exist):
```
Cannot find module '../client' from 'src/api/__tests__/client.test.ts'
```
(Jest reports the suite as failed to run; `Tests: 0 total`, `Test Suites: 1 failed`.)

- [ ] **Step 4: Write the minimal implementation at `app/src/api/client.ts`.** Full code:
```ts
import { getBaseUrl, getToken } from './config';

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const baseUrl = await getBaseUrl();
  const token = await getToken();
  if (!baseUrl) throw new ApiError(0, 'Backend URL not configured');

  const res = await fetch(`${baseUrl}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init.headers,
    },
  });

  if (!res.ok) {
    throw new ApiError(res.status, `${init.method ?? 'GET'} ${path} failed: ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const apiGet = <T>(path: string) => request<T>(path);
export const apiPost = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: 'POST', body: body ? JSON.stringify(body) : undefined });
```

- [ ] **Step 5: Run the test and confirm it PASSES.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/client.test.ts
```
Expected PASS output: `Test Suites: 1 passed, 1 total`, `Tests: 7 passed, 7 total`.

- [ ] **Step 6: Commit.** Command:
```bash
git add app/src/api/config.ts app/src/api/client.ts app/src/api/__tests__/client.test.ts && git commit -m "feat(app): typed API client with bearer auth + secure-store config"
```

---

### Task 31: App — shared API TypeScript types (`types.ts`)

**Files:**
- Create: `app/src/api/types.ts`
- Test: `app/src/api/__tests__/types.test.ts`

- [ ] **Step 1: Write the failing compile/shape test at `app/src/api/__tests__/types.test.ts`.** Types have no runtime; the test constructs fully-typed sample objects (matching Shared Contracts §3.8 exactly) and asserts on their fields, so a missing/renamed field is a compile error and a wrong value is a runtime failure. Full test code:
```ts
import type {
  Health,
  SourceStatus,
  Status,
  ConnectResponse,
  SyncSourceResult,
  SyncResponse,
  Activity,
  ActivitiesResponse,
  RecoveryDay,
  RecoveryResponse,
} from '../types';

describe('api types', () => {
  it('Status matches the /api/status contract', () => {
    const status: Status = {
      strava: {
        connected: true,
        athlete_id: 12345678,
        last_synced_at: '2026-06-19T05:00:30Z',
        last_run_at: '2026-06-19T05:00:30Z',
        status: 'ok',
        error: null,
      },
      garmin: {
        connected: true,
        last_synced_at: '2026-06-19T05:00:42Z',
        last_run_at: '2026-06-19T05:00:42Z',
        status: 'ok',
        error: null,
      },
      counts: { activities: 42, recovery_days: 30 },
    };
    expect(status.strava.athlete_id).toBe(12345678);
    expect(status.strava.status).toBe('ok');
    expect(status.counts.recovery_days).toBe(30);
  });

  it('SourceStatus supports null + never/error states', () => {
    const s: SourceStatus = {
      connected: false,
      last_synced_at: null,
      last_run_at: null,
      status: 'never',
      error: null,
    };
    const e: SourceStatus = {
      connected: false,
      last_synced_at: null,
      last_run_at: '2026-06-19T05:00:42Z',
      status: 'error',
      error: 'worker exit 1: re-run worker.py login',
    };
    expect(s.status).toBe('never');
    expect(e.error).toContain('re-run worker.py login');
  });

  it('ConnectResponse uses camelCase authorizeUrl', () => {
    const c: ConnectResponse = { authorizeUrl: 'https://www.strava.com/oauth/authorize?x=1' };
    expect(c.authorizeUrl).toContain('strava.com/oauth/authorize');
  });

  it('SyncResponse has per-source results', () => {
    const ok: SyncSourceResult = { status: 'ok', synced: 3, error: null };
    const sync: SyncResponse = {
      strava: ok,
      garmin: { status: 'error', synced: 0, error: 'worker exit 1: re-run worker.py login' },
    };
    expect(sync.strava.synced).toBe(3);
    expect(sync.garmin.status).toBe('error');
  });

  it('Activity allows null optional fields', () => {
    const a: Activity = {
      strava_id: 14820001234,
      name: 'Morning Run',
      type: 'Run',
      sport_type: null,
      start_time: '2026-06-18T06:12:00Z',
      start_time_local: null,
      distance_m: 10240.5,
      moving_time_s: 3120,
      elapsed_time_s: 3200,
      avg_hr: null,
      max_hr: null,
      avg_speed: null,
      max_speed: null,
      avg_cadence: null,
      elevation_gain_m: null,
    };
    const resp: ActivitiesResponse = { activities: [a] };
    expect(resp.activities[0].strava_id).toBe(14820001234);
  });

  it('RecoveryDay allows null sub-objects', () => {
    const day: RecoveryDay = {
      date: '2026-06-17',
      sleep: { duration_s: 25800, deep_s: 5400, light_s: 13800, rem_s: 4800, awake_s: 1800, score: 71 },
      hrv: null,
      body_battery: { charged: 58, drained: 80, high: 86, low: 12 },
      rhr: { resting_hr: 49 },
    };
    const resp: RecoveryResponse = { recovery: [day] };
    expect(resp.recovery[0].hrv).toBeNull();
    expect(resp.recovery[0].rhr?.resting_hr).toBe(49);
  });

  it('Health is a simple status object', () => {
    const h: Health = { status: 'ok' };
    expect(h.status).toBe('ok');
  });
});
```

- [ ] **Step 2: Run the test and confirm it FAILS.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/types.test.ts
```
Expected FAIL output: `Cannot find module '../types' from 'src/api/__tests__/types.test.ts'` (`Test Suites: 1 failed, 1 total`).

- [ ] **Step 3: Write the implementation at `app/src/api/types.ts`** (verbatim from Shared Contracts §3.8). Full code:
```ts
export interface Health { status: string; }

export interface SourceStatus {
  connected: boolean;
  last_synced_at: string | null;
  last_run_at: string | null;
  status: 'ok' | 'error' | 'never';
  error: string | null;
}
export interface Status {
  strava: SourceStatus & { athlete_id: number }; // M0 always emits a non-null athlete_id: a strava_tokens row exists only after a successful OAuth that includes an athlete.
  garmin: SourceStatus;
  counts: { activities: number; recovery_days: number };
}

export interface ConnectResponse { authorizeUrl: string; }

export interface SyncSourceResult { status: 'ok' | 'error'; synced: number; error: string | null; }
export interface SyncResponse { strava: SyncSourceResult; garmin: SyncSourceResult; }

export interface Activity {
  strava_id: number;
  name: string;
  type: string;
  sport_type: string | null;
  start_time: string;
  start_time_local: string | null;
  distance_m: number;
  moving_time_s: number;
  elapsed_time_s: number;
  avg_hr: number | null;
  max_hr: number | null;
  avg_speed: number | null;
  max_speed: number | null;
  avg_cadence: number | null;
  elevation_gain_m: number | null;
}
export interface ActivitiesResponse { activities: Activity[]; }

export interface RecoveryDay {
  date: string;
  sleep: { duration_s: number | null; deep_s: number | null; light_s: number | null; rem_s: number | null; awake_s: number | null; score: number | null } | null;
  hrv: { last_night_avg_ms: number | null; status: string | null } | null;
  body_battery: { charged: number | null; drained: number | null; high: number | null; low: number | null } | null;
  rhr: { resting_hr: number | null } | null;
}
export interface RecoveryResponse { recovery: RecoveryDay[]; }
```

- [ ] **Step 4: Run the test and confirm it PASSES.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/types.test.ts
```
Expected PASS output: `Test Suites: 1 passed, 1 total`, `Tests: 7 passed, 7 total`.

- [ ] **Step 5: Commit.** Command:
```bash
cd /home/jake/project/help-my-run && git add app/src/api/types.ts app/src/api/__tests__/types.test.ts && git commit -m "feat(app): shared API TypeScript types matching M0 contract"
```

---
### Task 32: App — React Query hooks (`useStatus`, `useActivities`, `useRecovery`, `useSync`, `useConnectStrava`)

**Files:**
- Create: `app/src/api/hooks.ts`
- Test: `app/src/api/__tests__/hooks.test.tsx`

- [ ] **Step 1: Write the failing test for the read hooks at `app/src/api/__tests__/hooks.test.tsx`.** It mocks `../client` (`apiGet`/`apiPost`) and renders each hook through a `QueryClientProvider` wrapper with `retry: false`. It asserts each hook hits the correct contract endpoint with the correct query params and returns the mocked data. Full test code:
```tsx
import React from 'react';
import { renderHook, waitFor, act } from '@testing-library/react-native';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

jest.mock('../client', () => ({
  apiGet: jest.fn(),
  apiPost: jest.fn(),
}));

import { apiGet, apiPost } from '../client';
import { useStatus, useActivities, useRecovery, useSync } from '../hooks';
import type { Status, ActivitiesResponse, RecoveryResponse, SyncResponse } from '../types';

const mockApiGet = apiGet as jest.MockedFunction<typeof apiGet>;
const mockApiPost = apiPost as jest.MockedFunction<typeof apiPost>;

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

afterEach(() => {
  jest.clearAllMocks();
});

describe('useStatus', () => {
  it('fetches /api/status', async () => {
    const data: Status = {
      strava: { connected: true, athlete_id: 1, last_synced_at: null, last_run_at: null, status: 'ok', error: null },
      garmin: { connected: false, last_synced_at: null, last_run_at: null, status: 'never', error: null },
      counts: { activities: 5, recovery_days: 3 },
    };
    mockApiGet.mockResolvedValue(data);

    const { result } = renderHook(() => useStatus(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/status');
    expect(result.current.data).toEqual(data);
  });
});

describe('useActivities', () => {
  it('fetches /api/activities with default limit 30', async () => {
    const data: ActivitiesResponse = { activities: [] };
    mockApiGet.mockResolvedValue(data);

    const { result } = renderHook(() => useActivities(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/activities?limit=30');
    expect(result.current.data).toEqual(data);
  });

  it('fetches /api/activities with an explicit limit', async () => {
    mockApiGet.mockResolvedValue({ activities: [] });

    const { result } = renderHook(() => useActivities(10), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/activities?limit=10');
  });
});

describe('useRecovery', () => {
  it('fetches /api/recovery with default days 30', async () => {
    const data: RecoveryResponse = { recovery: [] };
    mockApiGet.mockResolvedValue(data);

    const { result } = renderHook(() => useRecovery(), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/recovery?days=30');
    expect(result.current.data).toEqual(data);
  });

  it('fetches /api/recovery with an explicit days value', async () => {
    mockApiGet.mockResolvedValue({ recovery: [] });

    const { result } = renderHook(() => useRecovery(7), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/recovery?days=7');
  });
});

describe('useSync', () => {
  it('POSTs /api/sync and returns per-source results', async () => {
    const data: SyncResponse = {
      strava: { status: 'ok', synced: 3, error: null },
      garmin: { status: 'ok', synced: 5, error: null },
    };
    mockApiPost.mockResolvedValue(data);

    const { result } = renderHook(() => useSync(), { wrapper: createWrapper() });

    act(() => {
      result.current.mutate();
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/sync');
    expect(result.current.data).toEqual(data);
  });
});
```

- [ ] **Step 2: Run the test and confirm it FAILS.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/hooks.test.tsx
```
Expected FAIL output: `Cannot find module '../hooks' from 'src/api/__tests__/hooks.test.tsx'` (`Test Suites: 1 failed, 1 total`).

- [ ] **Step 3: Write the implementation at `app/src/api/hooks.ts`.** `useConnectStrava` uses Option B (open browser, poll `/api/status` until `strava.connected`, then invalidate). Full code:
```ts
import {
  useQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query';
import * as WebBrowser from 'expo-web-browser';
import { apiGet, apiPost } from './client';
import type {
  Status,
  ActivitiesResponse,
  RecoveryResponse,
  SyncResponse,
  ConnectResponse,
} from './types';

export function useStatus() {
  return useQuery({
    queryKey: ['status'],
    queryFn: () => apiGet<Status>('/api/status'),
  });
}

export function useActivities(limit = 30) {
  return useQuery({
    queryKey: ['activities', limit],
    queryFn: () => apiGet<ActivitiesResponse>(`/api/activities?limit=${limit}`),
  });
}

export function useRecovery(days = 30) {
  return useQuery({
    queryKey: ['recovery', days],
    queryFn: () => apiGet<RecoveryResponse>(`/api/recovery?days=${days}`),
  });
}

export function useSync() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<SyncResponse>('/api/sync'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['status'] });
      queryClient.invalidateQueries({ queryKey: ['activities'] });
      queryClient.invalidateQueries({ queryKey: ['recovery'] });
    },
  });
}

export function useConnectStrava() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const { authorizeUrl } = await apiGet<ConnectResponse>('/api/strava/connect');
      await WebBrowser.openAuthSessionAsync(authorizeUrl);
      for (let i = 0; i < 30; i++) {
        const s = await apiGet<Status>('/api/status');
        if (s.strava?.connected) return s;
        await new Promise((r) => setTimeout(r, 2000));
      }
      return apiGet<Status>('/api/status');
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['status'] });
    },
  });
}
```

- [ ] **Step 4: Run the test and confirm it PASSES.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/hooks.test.tsx
```
Expected PASS output: `Test Suites: 1 passed, 1 total`, `Tests: 6 passed, 6 total`.

- [ ] **Step 5: Commit.** Command:
```bash
git add app/src/api/hooks.ts app/src/api/__tests__/hooks.test.tsx && git commit -m "feat(app): react-query hooks for status, activities, recovery, sync, connect"
```

---

### Task 33: App — settings store hook (`useSettings`) over secure-store

**Files:**
- Create: `app/src/api/settings.ts`
- Test: `app/src/api/__tests__/settings.test.tsx`

- [ ] **Step 1: Write the failing test for `useSettings` at `app/src/api/__tests__/settings.test.tsx`.** It mocks `./config` so the hook's persistence is deterministic, and asserts: on mount it loads `baseUrl`/`token` (via `getBaseUrl`/`getToken`) into state with a `loading` flag, and `save(baseUrl, token)` calls `saveConfig` then updates state. Full test code:
```tsx
import React from 'react';
import { renderHook, waitFor, act } from '@testing-library/react-native';

jest.mock('../config', () => ({
  getBaseUrl: jest.fn(),
  getToken: jest.fn(),
  saveConfig: jest.fn(),
  clearConfig: jest.fn(),
}));

import { getBaseUrl, getToken, saveConfig } from '../config';
import { useSettings } from '../settings';

const mockGetBaseUrl = getBaseUrl as jest.MockedFunction<typeof getBaseUrl>;
const mockGetToken = getToken as jest.MockedFunction<typeof getToken>;
const mockSaveConfig = saveConfig as jest.MockedFunction<typeof saveConfig>;

afterEach(() => {
  jest.clearAllMocks();
});

describe('useSettings', () => {
  it('loads stored baseUrl + token on mount', async () => {
    mockGetBaseUrl.mockResolvedValue('http://localhost:8080');
    mockGetToken.mockResolvedValue('stored-token');

    const { result } = renderHook(() => useSettings());

    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.baseUrl).toBe('http://localhost:8080');
    expect(result.current.token).toBe('stored-token');
  });

  it('defaults to empty strings when nothing is stored', async () => {
    mockGetBaseUrl.mockResolvedValue(null);
    mockGetToken.mockResolvedValue(null);

    const { result } = renderHook(() => useSettings());

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.baseUrl).toBe('');
    expect(result.current.token).toBe('');
  });

  it('save persists via saveConfig and updates state', async () => {
    mockGetBaseUrl.mockResolvedValue(null);
    mockGetToken.mockResolvedValue(null);
    mockSaveConfig.mockResolvedValue(undefined);

    const { result } = renderHook(() => useSettings());
    await waitFor(() => expect(result.current.loading).toBe(false));

    await act(async () => {
      await result.current.save('http://10.0.0.5:8080', 'new-token');
    });

    expect(mockSaveConfig).toHaveBeenCalledWith('http://10.0.0.5:8080', 'new-token');
    expect(result.current.baseUrl).toBe('http://10.0.0.5:8080');
    expect(result.current.token).toBe('new-token');
  });
});
```

- [ ] **Step 2: Run the test and confirm it FAILS.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/settings.test.tsx
```
Expected FAIL output: `Cannot find module '../settings' from 'src/api/__tests__/settings.test.tsx'` (`Test Suites: 1 failed, 1 total`).

- [ ] **Step 3: Write the implementation at `app/src/api/settings.ts`.** Full code:
```ts
import { useState, useEffect, useCallback } from 'react';
import { getBaseUrl, getToken, saveConfig } from './config';

export interface Settings {
  baseUrl: string;
  token: string;
  loading: boolean;
  save: (baseUrl: string, token: string) => Promise<void>;
}

export function useSettings(): Settings {
  const [baseUrl, setBaseUrl] = useState('');
  const [token, setToken] = useState('');
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let mounted = true;
    (async () => {
      const [storedUrl, storedToken] = await Promise.all([getBaseUrl(), getToken()]);
      if (!mounted) return;
      setBaseUrl(storedUrl ?? '');
      setToken(storedToken ?? '');
      setLoading(false);
    })();
    return () => {
      mounted = false;
    };
  }, []);

  const save = useCallback(async (newBaseUrl: string, newToken: string) => {
    await saveConfig(newBaseUrl, newToken);
    setBaseUrl(newBaseUrl);
    setToken(newToken);
  }, []);

  return { baseUrl, token, loading, save };
}
```

- [ ] **Step 4: Run the test and confirm it PASSES.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/settings.test.tsx
```
Expected PASS output: `Test Suites: 1 passed, 1 total`, `Tests: 3 passed, 3 total`.

- [ ] **Step 5: Commit.** Command:
```bash
cd /home/jake/project/help-my-run && git add app/src/api/settings.ts app/src/api/__tests__/settings.test.tsx && git commit -m "feat(app): useSettings hook persisting backend URL + token via secure-store"
```

---
### Task 34: App — root layout `_layout.tsx` (QueryClientProvider + Stack)

**Files:**
- Create: `app/src/api/queryClient.ts`
- Modify: `app/app/_layout.tsx`
- Test: `app/app/__tests__/_layout.test.tsx`

- [ ] **Step 1: Create the shared `QueryClient` instance at `app/src/api/queryClient.ts`** (extracted so screens and tests can import the same configured client). Full code:
```ts
import { QueryClient } from '@tanstack/react-query';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 30_000,
    },
  },
});
```

- [ ] **Step 2: Write the failing test for the root layout at `app/app/__tests__/_layout.test.tsx`.** It mocks `expo-router` (`Stack` and `Stack.Screen`) so the layout renders without the router runtime, then asserts the layout renders a `QueryClientProvider` wrapping the `Stack` (verified by rendering a child hook that needs a QueryClient and confirming no "No QueryClient set" throw). Full test code:
```tsx
import React from 'react';
import { render } from '@testing-library/react-native';
import { Text } from 'react-native';
import { useQueryClient } from '@tanstack/react-query';

jest.mock('expo-router', () => {
  const React = require('react');
  const Stack = ({ children }: { children?: React.ReactNode }) =>
    React.createElement(React.Fragment, null, children);
  Stack.Screen = () => null;
  return { Stack };
});

import RootLayout from '../_layout';

// A probe that throws (caught by RTL) if no QueryClient is in context.
function QueryClientProbe() {
  const client = useQueryClient();
  return <Text testID="probe">{client ? 'has-client' : 'no-client'}</Text>;
}

describe('RootLayout', () => {
  it('renders without crashing', () => {
    const { toJSON } = render(<RootLayout />);
    expect(toJSON()).not.toBeNull();
  });

  it('provides a QueryClient to its subtree', () => {
    // Render the same provider the layout uses by mounting the layout's
    // QueryClientProvider via the exported queryClient.
    const { queryClient } = require('../../src/api/queryClient');
    const { QueryClientProvider } = require('@tanstack/react-query');
    const { getByTestId } = render(
      <QueryClientProvider client={queryClient}>
        <QueryClientProbe />
      </QueryClientProvider>
    );
    expect(getByTestId('probe').props.children).toBe('has-client');
  });
});
```

- [ ] **Step 3: Run the test and confirm it FAILS** (the current `_layout.tsx` from scaffolding has no `QueryClientProvider`, and `../../src/api/queryClient` is freshly added but the layout does not yet use it). Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/_layout.test.tsx
```
Expected FAIL output (the first render test will fail or the probe test depends on a layout that has no provider — initial run before edit):
```
FAIL  app/__tests__/_layout.test.tsx
  RootLayout
    ✕ renders without crashing
```
(If the scaffold's default `_layout.tsx` renders fine, the failing assertion is the provider wiring added in Step 4; in either case the suite is RED until Step 4 lands.)

- [ ] **Step 4: Write the implementation by replacing `app/app/_layout.tsx`.** It wraps the `Stack` in `QueryClientProvider` using the shared `queryClient`. Full code:
```tsx
import { Stack } from 'expo-router';
import { QueryClientProvider } from '@tanstack/react-query';
import { queryClient } from '../src/api/queryClient';

export default function RootLayout() {
  return (
    <QueryClientProvider client={queryClient}>
      <Stack>
        <Stack.Screen name="index" options={{ title: 'help-my-run' }} />
        <Stack.Screen name="settings" options={{ title: 'Settings' }} />
      </Stack>
    </QueryClientProvider>
  );
}
```

- [ ] **Step 5: Run the test and confirm it PASSES.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/_layout.test.tsx
```
Expected PASS output: `Test Suites: 1 passed, 1 total`, `Tests: 2 passed, 2 total`.

- [ ] **Step 6: Commit.** Command:
```bash
git add app/src/api/queryClient.ts app/app/_layout.tsx app/app/__tests__/_layout.test.tsx && git commit -m "feat(app): root layout with QueryClientProvider + shared queryClient"
```

---

### Task 35: App — Connect/Settings screen (`settings.tsx`)

**Files:**
- Create: `app/app/settings.tsx`
- Test: `app/app/__tests__/settings.test.tsx`

- [ ] **Step 1: Write the failing component test at `app/app/__tests__/settings.test.tsx`.** It mocks the api layer (`../../src/api/settings` for `useSettings`, `../../src/api/hooks` for `useStatus`/`useSync`/`useConnectStrava`) and `expo-router`. It asserts: the backend-URL and token `TextInput`s render with stored values, the `Save` button calls `save`, the `Strava Connect` button calls the connect mutation, the Garmin status text renders, and the `Sync now` button calls the sync mutation. Full test code:
```tsx
import React from 'react';
import { render, screen, fireEvent } from '@testing-library/react-native';

const mockSave = jest.fn();
const mockConnectMutate = jest.fn();
const mockSyncMutate = jest.fn();

jest.mock('expo-router', () => ({ Stack: { Screen: () => null } }));

jest.mock('../../src/api/settings', () => ({
  useSettings: () => ({
    baseUrl: 'http://localhost:8080',
    token: 'stored-token',
    loading: false,
    save: mockSave,
  }),
}));

jest.mock('../../src/api/hooks', () => ({
  useStatus: () => ({
    data: {
      strava: { connected: true, athlete_id: 1, last_synced_at: null, last_run_at: null, status: 'ok', error: null },
      garmin: { connected: false, last_synced_at: null, last_run_at: null, status: 'never', error: null },
      counts: { activities: 0, recovery_days: 0 },
    },
    isPending: false,
    isError: false,
  }),
  useSync: () => ({ mutate: mockSyncMutate, isPending: false }),
  useConnectStrava: () => ({ mutate: mockConnectMutate, isPending: false }),
}));

import SettingsScreen from '../settings';

afterEach(() => {
  jest.clearAllMocks();
});

describe('SettingsScreen', () => {
  it('renders inputs prefilled with stored config', () => {
    render(<SettingsScreen />);
    expect(screen.getByTestId('input-base-url').props.value).toBe('http://localhost:8080');
    expect(screen.getByTestId('input-token').props.value).toBe('stored-token');
  });

  it('saves edited config when Save is pressed', () => {
    render(<SettingsScreen />);
    fireEvent.changeText(screen.getByTestId('input-base-url'), 'http://10.0.0.5:8080');
    fireEvent.changeText(screen.getByTestId('input-token'), 'new-token');
    fireEvent.press(screen.getByTestId('btn-save'));
    expect(mockSave).toHaveBeenCalledWith('http://10.0.0.5:8080', 'new-token');
  });

  it('starts Strava connect when Connect is pressed', () => {
    render(<SettingsScreen />);
    fireEvent.press(screen.getByTestId('btn-strava-connect'));
    expect(mockConnectMutate).toHaveBeenCalledTimes(1);
  });

  it('shows the Strava connected state', () => {
    render(<SettingsScreen />);
    expect(screen.getByTestId('strava-status').props.children).toContain('Connected');
  });

  it('shows the Garmin not-connected state', () => {
    render(<SettingsScreen />);
    expect(screen.getByTestId('garmin-status').props.children).toContain('Not connected');
  });

  it('triggers a sync when Sync now is pressed', () => {
    render(<SettingsScreen />);
    fireEvent.press(screen.getByTestId('btn-sync'));
    expect(mockSyncMutate).toHaveBeenCalledTimes(1);
  });
});
```

- [ ] **Step 2: Run the test and confirm it FAILS.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/settings.test.tsx
```
Expected FAIL output: `Cannot find module '../settings' from 'app/__tests__/settings.test.tsx'` (`Test Suites: 1 failed, 1 total`).

- [ ] **Step 3: Write the implementation at `app/app/settings.tsx`.** Full code:
```tsx
import React, { useEffect, useState } from 'react';
import { View, Text, TextInput, Pressable, ScrollView, StyleSheet } from 'react-native';
import { useSettings } from '../src/api/settings';
import { useStatus, useSync, useConnectStrava } from '../src/api/hooks';

export default function SettingsScreen() {
  const settings = useSettings();
  const status = useStatus();
  const sync = useSync();
  const connectStrava = useConnectStrava();

  const [baseUrl, setBaseUrl] = useState('');
  const [token, setToken] = useState('');

  useEffect(() => {
    if (!settings.loading) {
      setBaseUrl(settings.baseUrl);
      setToken(settings.token);
    }
  }, [settings.loading, settings.baseUrl, settings.token]);

  const stravaConnected = status.data?.strava.connected ?? false;
  const garminConnected = status.data?.garmin.connected ?? false;

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Backend</Text>
      <Text style={styles.label}>Backend URL</Text>
      <TextInput
        testID="input-base-url"
        style={styles.input}
        autoCapitalize="none"
        autoCorrect={false}
        placeholder="http://localhost:8080"
        value={baseUrl}
        onChangeText={setBaseUrl}
      />
      <Text style={styles.label}>API token</Text>
      <TextInput
        testID="input-token"
        style={styles.input}
        autoCapitalize="none"
        autoCorrect={false}
        secureTextEntry
        placeholder="API_TOKEN"
        value={token}
        onChangeText={setToken}
      />
      <Pressable
        testID="btn-save"
        style={styles.button}
        onPress={() => settings.save(baseUrl, token)}
      >
        <Text style={styles.buttonText}>Save</Text>
      </Pressable>

      <Text style={styles.heading}>Strava</Text>
      <Text testID="strava-status" style={styles.statusLine}>
        {stravaConnected ? 'Connected' : 'Not connected'}
      </Text>
      <Pressable
        testID="btn-strava-connect"
        style={styles.button}
        disabled={connectStrava.isPending}
        onPress={() => connectStrava.mutate()}
      >
        <Text style={styles.buttonText}>
          {connectStrava.isPending ? 'Connecting…' : 'Strava Connect'}
        </Text>
      </Pressable>

      <Text style={styles.heading}>Garmin</Text>
      <Text testID="garmin-status" style={styles.statusLine}>
        {garminConnected ? 'Connected' : 'Not connected'}
      </Text>

      <Text style={styles.heading}>Sync</Text>
      <Pressable
        testID="btn-sync"
        style={styles.button}
        disabled={sync.isPending}
        onPress={() => sync.mutate()}
      >
        <Text style={styles.buttonText}>{sync.isPending ? 'Syncing…' : 'Sync now'}</Text>
      </Pressable>
      {sync.data ? (
        <Text testID="sync-result" style={styles.statusLine}>
          Strava: {sync.data.strava.status} ({sync.data.strava.synced}) · Garmin:{' '}
          {sync.data.garmin.status} ({sync.data.garmin.synced})
        </Text>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 8 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  label: { fontSize: 14, color: '#444', marginTop: 8 },
  input: {
    borderWidth: 1,
    borderColor: '#ccc',
    borderRadius: 8,
    paddingHorizontal: 12,
    paddingVertical: 10,
    fontSize: 16,
  },
  button: {
    backgroundColor: '#fc4c02',
    borderRadius: 8,
    paddingVertical: 12,
    alignItems: 'center',
    marginTop: 8,
  },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
  statusLine: { fontSize: 15, color: '#222' },
});
```

- [ ] **Step 4: Run the test and confirm it PASSES.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/settings.test.tsx
```
Expected PASS output: `Test Suites: 1 passed, 1 total`, `Tests: 6 passed, 6 total`.

- [ ] **Step 5: Commit.** Command:
```bash
cd /home/jake/project/help-my-run && git add app/app/settings.tsx app/app/__tests__/settings.test.tsx && git commit -m "feat(app): Connect/Settings screen (backend config, Strava connect, Garmin status, sync now)"
```

---
### Task 36: App — Home/Status screen (`index.tsx`)

**Files:**
- Modify: `app/app/index.tsx` (overwrite the minimal `index.tsx` left by `reset-project` in Task 5)
- Test: `app/app/__tests__/index.test.tsx`

- [ ] **Step 1: Write the failing component test at `app/app/__tests__/index.test.tsx`.** It mocks `../../src/api/hooks` (`useStatus`, `useActivities`, `useRecovery`) with concrete contract data and `expo-router` (`Link`). It asserts: connection + last-sync + counts render, the recent-runs list renders one row per activity (by name), and the recent-recovery list renders one row per day (by date). Full test code:
```tsx
import React from 'react';
import { render, screen } from '@testing-library/react-native';
import type { Status, ActivitiesResponse, RecoveryResponse } from '../../src/api/types';

jest.mock('expo-router', () => ({
  Link: ({ children }: { children: React.ReactNode }) => children,
  Stack: { Screen: () => null },
}));

const statusData: Status = {
  strava: { connected: true, athlete_id: 1, last_synced_at: '2026-06-19T05:00:30Z', last_run_at: '2026-06-19T05:00:30Z', status: 'ok', error: null },
  garmin: { connected: true, last_synced_at: '2026-06-19T05:00:42Z', last_run_at: '2026-06-19T05:00:42Z', status: 'ok', error: null },
  counts: { activities: 42, recovery_days: 30 },
};

const activitiesData: ActivitiesResponse = {
  activities: [
    {
      strava_id: 14820001234, name: 'Morning Run', type: 'Run', sport_type: 'Run',
      start_time: '2026-06-18T06:12:00Z', start_time_local: '2026-06-18T08:12:00',
      distance_m: 10240.5, moving_time_s: 3120, elapsed_time_s: 3200,
      avg_hr: 152.3, max_hr: 171, avg_speed: 3.28, max_speed: 4.91,
      avg_cadence: 86.5, elevation_gain_m: 84.0,
    },
    {
      strava_id: 14820009999, name: 'Evening Jog', type: 'Run', sport_type: 'Run',
      start_time: '2026-06-17T18:00:00Z', start_time_local: '2026-06-17T20:00:00',
      distance_m: 5000, moving_time_s: 1500, elapsed_time_s: 1520,
      avg_hr: null, max_hr: null, avg_speed: null, max_speed: null,
      avg_cadence: null, elevation_gain_m: null,
    },
  ],
};

const recoveryData: RecoveryResponse = {
  recovery: [
    {
      date: '2026-06-18',
      sleep: { duration_s: 27000, deep_s: 6300, light_s: 14400, rem_s: 5400, awake_s: 900, score: 82 },
      hrv: { last_night_avg_ms: 48, status: 'BALANCED' },
      body_battery: { charged: 62, drained: 78, high: 91, low: 14 },
      rhr: { resting_hr: 47 },
    },
    {
      date: '2026-06-17',
      sleep: { duration_s: 25800, deep_s: 5400, light_s: 13800, rem_s: 4800, awake_s: 1800, score: 71 },
      hrv: null,
      body_battery: { charged: 58, drained: 80, high: 86, low: 12 },
      rhr: { resting_hr: 49 },
    },
  ],
};

jest.mock('../../src/api/hooks', () => ({
  useStatus: () => ({ data: statusData, isPending: false, isError: false }),
  useActivities: () => ({ data: activitiesData, isPending: false, isError: false }),
  useRecovery: () => ({ data: recoveryData, isPending: false, isError: false }),
}));

import HomeScreen from '../index';

describe('HomeScreen', () => {
  it('renders connection status for both sources', () => {
    render(<HomeScreen />);
    expect(screen.getByTestId('home-strava-status').props.children).toContain('Connected');
    expect(screen.getByTestId('home-garmin-status').props.children).toContain('Connected');
  });

  it('renders the activity + recovery counts', () => {
    render(<HomeScreen />);
    expect(screen.getByTestId('count-activities').props.children).toContain(42);
    expect(screen.getByTestId('count-recovery').props.children).toContain(30);
  });

  it('renders one row per recent run by name', () => {
    render(<HomeScreen />);
    expect(screen.getByText('Morning Run')).toBeTruthy();
    expect(screen.getByText('Evening Jog')).toBeTruthy();
  });

  it('renders one row per recent recovery day by date', () => {
    render(<HomeScreen />);
    expect(screen.getByText('2026-06-18')).toBeTruthy();
    expect(screen.getByText('2026-06-17')).toBeTruthy();
  });
});
```

- [ ] **Step 2: Run the test and confirm it FAILS.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/index.test.tsx
```
Expected FAIL output: the suite is RED — the placeholder `index.tsx` left by `reset-project` renders none of the expected `testID`s, so the assertions fail (e.g. `Unable to find an element with testID: home-strava-status`). `Test Suites: 1 failed, 1 total`.

- [ ] **Step 3: Write the implementation by OVERWRITING `app/app/index.tsx`** (replacing the minimal placeholder from `reset-project`). It uses the three read hooks and renders status, counts, the runs list, and the recovery list. Full code:
```tsx
import React from 'react';
import { View, Text, FlatList, ScrollView, StyleSheet } from 'react-native';
import { Link } from 'expo-router';
import { useStatus, useActivities, useRecovery } from '../src/api/hooks';
import type { Activity, RecoveryDay } from '../src/api/types';

function fmtKm(distanceM: number): string {
  return (distanceM / 1000).toFixed(2) + ' km';
}

function fmtSyncTime(iso: string | null): string {
  return iso ?? 'never';
}

export default function HomeScreen() {
  const status = useStatus();
  const activities = useActivities();
  const recovery = useRecovery();

  const strava = status.data?.strava;
  const garmin = status.data?.garmin;

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Connection</Text>
      <Text testID="home-strava-status" style={styles.statusLine}>
        Strava: {strava?.connected ? 'Connected' : 'Not connected'} · last sync{' '}
        {fmtSyncTime(strava?.last_synced_at ?? null)}
      </Text>
      <Text testID="home-garmin-status" style={styles.statusLine}>
        Garmin: {garmin?.connected ? 'Connected' : 'Not connected'} · last sync{' '}
        {fmtSyncTime(garmin?.last_synced_at ?? null)}
      </Text>
      <Text testID="count-activities" style={styles.statusLine}>
        Activities: {status.data?.counts.activities ?? 0}
      </Text>
      <Text testID="count-recovery" style={styles.statusLine}>
        Recovery days: {status.data?.counts.recovery_days ?? 0}
      </Text>
      <Link href="/settings" style={styles.link}>
        Settings
      </Link>

      <Text style={styles.heading}>Recent runs</Text>
      <FlatList
        scrollEnabled={false}
        data={activities.data?.activities ?? []}
        keyExtractor={(item: Activity) => String(item.strava_id)}
        ListEmptyComponent={<Text style={styles.empty}>No runs yet</Text>}
        renderItem={({ item }: { item: Activity }) => (
          <View style={styles.row}>
            <Text style={styles.rowTitle}>{item.name}</Text>
            <Text style={styles.rowSub}>
              {fmtKm(item.distance_m)} · {Math.round(item.moving_time_s / 60)} min
              {item.avg_hr != null ? ` · ${Math.round(item.avg_hr)} bpm` : ''}
            </Text>
          </View>
        )}
      />

      <Text style={styles.heading}>Recent recovery</Text>
      <FlatList
        scrollEnabled={false}
        data={recovery.data?.recovery ?? []}
        keyExtractor={(item: RecoveryDay) => item.date}
        ListEmptyComponent={<Text style={styles.empty}>No recovery data yet</Text>}
        renderItem={({ item }: { item: RecoveryDay }) => (
          <View style={styles.row}>
            <Text style={styles.rowTitle}>{item.date}</Text>
            <Text style={styles.rowSub}>
              {item.sleep?.score != null ? `Sleep ${item.sleep.score}` : 'Sleep —'} ·{' '}
              {item.hrv?.last_night_avg_ms != null ? `HRV ${item.hrv.last_night_avg_ms}ms` : 'HRV —'}{' '}
              · {item.rhr?.resting_hr != null ? `RHR ${item.rhr.resting_hr}` : 'RHR —'}
            </Text>
          </View>
        )}
      />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 6 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  statusLine: { fontSize: 15, color: '#222' },
  link: { fontSize: 15, color: '#fc4c02', marginTop: 8 },
  row: { paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: '#ddd' },
  rowTitle: { fontSize: 16, fontWeight: '500' },
  rowSub: { fontSize: 13, color: '#666' },
  empty: { fontSize: 14, color: '#999', paddingVertical: 8 },
});
```

- [ ] **Step 4: Run the test and confirm it PASSES.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/index.test.tsx
```
Expected PASS output: `Test Suites: 1 passed, 1 total`, `Tests: 4 passed, 4 total`.

- [ ] **Step 5: Commit.** Command:
```bash
git add app/app/index.tsx app/app/__tests__/index.test.tsx && git commit -m "feat(app): Home/Status screen (connection, counts, recent runs, recent recovery)"
```

---

### Task 37: App — full app test suite green + app.json scheme

**Files:**
- Modify: `app/app.json`
- Modify: `app/package.json`

- [ ] **Step 1: Ensure `app/package.json` has the test script + jest-expo preset (verify the scaffold set these; if not, set them).** The relevant keys MUST be exactly:
```json
{
  "scripts": { "test": "jest" },
  "jest": { "preset": "jest-expo" }
}
```
(If the scaffold already wrote `"test": "jest --watchAll"`, change it to `"jest"` so CI runs once and exits.)

- [ ] **Step 2: Set the deep-link scheme in `app/app.json`** (required by `expo-web-browser` / future Option A; harmless for Option B). Ensure the `expo` object contains:
```json
{ "expo": { "scheme": "helpmyrun", "name": "help-my-run", "slug": "help-my-run" } }
```

- [ ] **Step 3: Run the COMPLETE test suite and confirm all suites PASS.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx jest
```
Expected PASS output (7 suites: client, types, hooks, settings hook, _layout, settings screen, index screen):
```
Test Suites: 7 passed, 7 total
Tests:       35 passed, 35 total
Snapshots:   0 total
```
(Suite count is 7 files: `client.test.ts`, `types.test.ts`, `hooks.test.tsx`, `settings.test.tsx` (hook, under `src/api/__tests__/`), `_layout.test.tsx`, `settings.test.tsx` (screen, under `app/__tests__/`), `index.test.tsx`.)

- [ ] **Step 4: Run the TypeScript compiler to confirm no type errors across the app.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx tsc --noEmit
```
Expected output: no output, exit code 0 (clean compile).

- [ ] **Step 5: Commit.** Command:
```bash
cd /home/jake/project/help-my-run && git add app/app.json app/package.json && git commit -m "chore(app): jest-expo test script + helpmyrun deep-link scheme"
```

---

### Task 38: MANUAL verification — Strava OAuth browser flow (cannot be unit-tested)

**Files:** none (manual integration verification; the real `WebBrowser.openAuthSessionAsync` → Strava authorize → backend `/api/strava/callback` round-trip is integration-only per Shared Contracts §3.4 + spec §5.7).

- [ ] **Step 1: Start the Go backend with valid Strava env vars.** From repo root:
```bash
cd /home/jake/project/help-my-run && make run-backend
```
Expected: server logs `listening on :8080` (or the configured `PORT`); `curl http://localhost:8080/health` returns `{"status":"ok"}`.

- [ ] **Step 2: Start the Expo app on a device/simulator.** Command:
```bash
cd /home/jake/project/help-my-run/app && npx expo start
```
Expected: Metro bundler starts; open the app in Expo Go or a dev build on a phone that can reach the backend host.

- [ ] **Step 3: In the app, open Settings, enter the backend URL and API token, press Save.** Actions: type `http://<your-LAN-ip>:8080` into the Backend URL field, paste the value of `API_TOKEN` from `.env` into the API token field, press `Save`.
  Expected on-screen: no error; pressing back to Home then returning shows the values persisted (secure-store round-trip).

- [ ] **Step 4: Press `Strava Connect` and complete the OAuth flow in the system browser.** Actions: tap `Strava Connect`; the system browser opens the Strava authorize page; log in if needed and tap `Authorize`.
  Expected on-screen: the browser shows the backend callback page containing the text `You can close this tab.` Close the browser tab.

- [ ] **Step 5: Confirm the app reflects the connection.** Actions: return to the app; within ~2–60 seconds the `useConnectStrava` poll detects `strava.connected === true` and invalidates `['status']`.
  Expected on-screen: the Settings `Strava` status flips to `Connected`; navigating to Home shows `Strava: Connected` with a non-`never` last-sync after the next sync.

- [ ] **Step 6: Press `Sync now` and confirm data lands.** Actions: on Settings, tap `Sync now`.
  Expected on-screen: a sync result line like `Strava: ok (N) · Garmin: ok (M)` (Garmin may show `error` with `re-run worker.py login` if the worker has not logged in — that is the expected degraded state per Shared Contracts §3.5). Navigate to Home: the `Recent runs` list now shows real runs and `Activities` count is non-zero.

- [ ] **Step 7: Record the result.** Confirm and note: (a) `{"status":"ok"}` from `/health`; (b) browser reached `You can close this tab.`; (c) Home shows `Strava: Connected` and a non-zero activity count; (d) `Sync now` returned per-source `ok`/`error` lines. If any step fails, capture the on-screen error and the backend `sync_log` row (`error` column) for the failing source before reporting blocked.

---
## Definition of Done

Each Milestone 0 success criterion from the spec (§5.1) maps to the task(s) that satisfy it. M0 is done when every criterion below is met and verified.

| # | Success criterion (spec §5.1) | Satisfied by tasks |
|---|---|---|
| 1 | **Connect Strava once → recent runs land in the DB with HR, pace, distance, splits.** | Task 9 (schema: `activities` + `activity_splits` + WAL store), Task 11 (activities + splits upsert/list), Task 17–19 (Strava client: AuthorizeURL, Exchange/Refresh, paginated ListActivities + ListLaps), Task 21 (SyncStrava maps + upserts activities and laps), Task 25–26 (DTOs + router + `/api/strava/connect`), Task 28 (`/api/strava/callback` exchanges code and persists tokens), Task 30–32 (app client + types + `useConnectStrava`/`useActivities`), Task 35 (Settings screen `Strava Connect`), Task 38 (manual OAuth round-trip verified end-to-end). |
| 2 | **Garmin worker logs in once (MFA-aware) → last ~30 days of sleep / HRV / Body Battery / resting HR land in the DB; daily incremental works non-interactively afterward.** | Task 4 (worker venv + `garminconnect`/`curl_cffi`), Task 9 (schema: four `garmin_*` tables), Task 12 (four garmin upserts), Task 13 (pure normalizers for all four sources), Task 14 (`fetch` CLI + `--dry-run` JSON), Task 15 (`GarminClient.login_interactive` MFA-aware + `resume` token reuse + `login` command), Task 16 (`run_fetch` date iteration over `--since`, RHR via `get_stats`, HRV-None omitted, error contract), Task 20 (Go runner invokes worker via `os/exec`, parses JSON, surfaces stderr), Task 22 (SyncGarmin upserts four tables, ~30-day backfill when never synced, incremental since `last_synced_at`), Task 8 (config passes `GARMIN_*` env through), Task 29 (`Wire` passes `garminEnv` to the runner). One-time login operationalized by Task 7's Makefile `garmin-login` target and the README. |
| 3 | **The Expo app shows connection status, last-sync times, a recent-runs list, and recent recovery days.** | Task 12 (`CountRecoveryDays` + `ListRecovery` merge), Task 25–27 (`/api/status` with connection/last-sync/counts, `/api/activities`, `/api/recovery` handlers), Task 31 (types), Task 32 (`useStatus`/`useActivities`/`useRecovery`), Task 34 (root layout `QueryClientProvider`), Task 36 (Home/Status screen: connection status, last-sync, counts, recent-runs list, recent-recovery list), Task 37 (full suite green + tsc clean). |
| 4 | **A `Sync now` button and an automatic periodic sync both work.** | Task 21–23 (SyncStrava + SyncGarmin + SyncAll partial-failure tolerant), Task 24 (`RunTicker` periodic loop), Task 28 (`POST /api/sync` handler returns per-source results), Task 29 (`main` starts `RunTicker` on `syncInterval` + wires `SyncAll` into the handler), Task 32 (`useSync` mutation invalidates queries), Task 35 (Settings `Sync now` button), Task 38 (manual `Sync now` verified). |
| 5 | **A fresh clone + `.env` + documented steps gets someone else running their own instance.** | Task 1 (`.gitignore` secret coverage), Task 6 (`.env.example` documenting all env vars), Task 7 **creates** the `Makefile` (with the `run-backend`, `garmin-login`, `sync`, `run-app`, `test` targets) and the `README.md` self-host guide (Prerequisites, Setup, one-time `worker.py login`, Running, Syncing, Testing, Security note, Disclaimer). The whole-suite verification gates — Task 7 (final scaffold gate after creating README + Makefile), Task 29 Step 6–7 (full backend `go test ./...` + binary serves `/health`), Task 16 Step 9 (full worker pytest suite), Task 37 (full app jest suite + `tsc --noEmit`) — confirm a clean checkout builds and passes end-to-end. |

**Out of scope for M0 (deferred, per spec §5.9):** activity streams, the metrics engine (zones/decoupling/load), any Claude calls (the `ANTHROPIC_API_KEY` is loaded but unused — Task 8), the agentic nightly decision loop, push notifications, race/taper features.
