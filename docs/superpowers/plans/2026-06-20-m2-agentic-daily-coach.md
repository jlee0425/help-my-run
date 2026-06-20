# Milestone 2 (Agentic Daily Coach) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every morning before the user wakes, the backend reads last night's recovery, assesses readiness (GREEN/AMBER/RED), automatically rewrites today's planned run (STAND/SOFTEN/MOVE/REST_DAY) with a deterministic fallback when Claude is unavailable, persists it as today's effective session, and pushes a morning briefing the user can Undo.

**Architecture:** M2 extends the M0+M1 codebase already merged on `main`, adding an in-process Go daily scheduler that runs the agent loop = sync → readiness gate → `claude -p` daily-adjust ("Coach Brain") with a deterministic readiness→action fallback → persist `daily_decisions`/`agent_runs` → Expo push. A pure-Go readiness engine and an injectable-clock scheduler keep the whole loop offline-testable; the Expo app adds a Today card (readiness pill + drivers + what-changed + Undo) and push-token registration.

**Tech Stack:** Go (`chi` router, `modernc.org/sqlite`, `goose` migrations); `claude` CLI in headless print mode (`claude -p --output-format json`) reused from M1; Expo push via `expo-notifications` (client) + the Expo Push HTTP API (`https://exp.host/--/api/v2/push/send`, server); Expo app on `expo-router` + `@tanstack/react-query` (SDK 56, RN 0.85, React 19).

---

## Setup Prerequisites

- **Unattended Claude auth:** the host runs `claude setup-token` once to mint a long-lived subscription token so the backend's daily `claude -p` daily-adjust call works headless (no interactive login at run time). `ANTHROPIC_API_KEY` remains a paid fallback. One daily call is well within subscription rate limits. (Path established in the M1 spec §9.)
- **Expo dev build for push:** remote push requires an EAS **development build** on a physical device — **not Expo Go** (SDK 53+ cannot receive remote push in Expo Go) and not the iOS Simulator. Run `eas init` / `eas build:configure` once to write the real `expo.extra.eas.projectId`, then `eas build --profile development`.
- **Builds on M0/M1 `main`:** this plan EXTENDS the merged M0+M1 backend (`help-my-run/backend`, module path `help-my-run/backend`) and Expo app (`help-my-run/app`). Migrations `00001`–`00003` already exist; M2 adds `00004`. All M2 wire/DTO/stored JSON is **snake_case** (repo convention; overrides the brief's "camelCase").

---

## File Structure

### Backend — new files
- `backend/internal/store/migrations/00004_m2_agent.sql` — device_tokens, daily_decisions, agent_runs + athlete_profile columns
- `backend/internal/store/daily.go` — `DeviceToken`/`DailyDecision`/`AgentRun` structs + CRUD
- `backend/internal/store/daily_test.go` — migration-shape + upsert/get round-trip tests
- `backend/internal/store/profile_test.go` — M2 profile-column round-trip tests
- `backend/internal/readiness/readiness.go` — `Assess`, `Color`, `ReadinessDrivers`, `Readiness`, thresholds, driver helpers
- `backend/internal/readiness/readiness_test.go` — table-driven color/driver tests
- `backend/internal/readiness/fallback.go` — `Fallback`, `FallbackSession`, `FallbackDecision`, `roundHalf`, `isEasyType`
- `backend/internal/readiness/fallback_test.go` — fallback rule-matrix tests
- `backend/internal/push/push.go` — `Message`, `Sender`, `Client`, `NewClient`, `Send`, `ErrDeviceNotRegistered`
- `backend/internal/push/push_test.go` — httptest Expo API tests
- `backend/internal/llm/types_test.go` — `DailyDecisionParsed`/`DailyAction` round-trip tests
- `backend/internal/coach/prompts_test.go` — daily-adjust prompt + input builder tests
- `backend/internal/coach/adjust_test.go` — `AdjustToday` AI + fallback tests
- `backend/internal/agent/agent.go` — `Agent`, `RunResult`, seams, `New`, `RunDaily`
- `backend/internal/agent/agent_test.go` — full-loop offline tests
- `backend/internal/agent/syncer.go` — `RealSyncer` adapter over `sync.SyncAll`
- `backend/internal/agent/syncer_test.go` — RealSyncer adapter test
- `backend/internal/scheduler/scheduler.go` — `Clock`, `RealClock`, `Config`, `nextFire`, `Run`
- `backend/internal/scheduler/scheduler_test.go` — `nextFire` table tests
- `backend/internal/scheduler/run_test.go` — fake-clock fire-once-per-day tests
- `backend/internal/api/today_handlers.go` — `today`, `undoToday`, `agentRun` handlers + mappers
- `backend/internal/api/push_handlers.go` — `pushRegister` handler
- `backend/internal/api/m2_dto_test.go` — DTO tag + `validISODate` tests
- `backend/internal/api/m2_fakes_test.go` — `fakeAgent`/`fakePusher` + interface conformance
- `backend/internal/api/m2_handlers_test.go` — profile/push/today/undo/agent handler tests
- `backend/internal/config/config_test.go` — M2 config-default tests
- `backend/cmd/server/main_test.go` — Wire builds M2 graph tests

### Backend — modified files
- `backend/internal/metrics/metrics.go` — export `RecoveryTrend` wrapper
- `backend/internal/metrics/metrics_test.go` — `TestRecoveryTrendExported`
- `backend/internal/llm/types.go` — add `DailyAction` enum + `DailyDecisionParsed`
- `backend/internal/coach/coach.go` — add `DailyAdjustInput`, `dailyAdjustArgs()`, `AdjustToday(...)`, fallback helpers
- `backend/internal/coach/prompts.go` — add `dailyAdjustPrompt` const + `buildDailyAdjustInput`
- `backend/internal/store/profile.go` — `AthleteProfile` + Get/Upsert add `DailyRunTime`/`Timezone`/`AgentEnabled`
- `backend/internal/api/dto.go` — extend `profileDTO`; add `todayResponseDTO`, push DTOs, `readinessDriversDTO`
- `backend/internal/api/profile_handlers.go` — map/validate new profile fields
- `backend/internal/api/plan_handlers.go` — add `validISODate(s)` helper
- `backend/internal/api/router.go` — add `AdjustToday` to `Coach`; add `Agent`/`Pusher` seams + `Deps`; register 4 routes
- `backend/internal/api/m1_handlers_test.go` — extend `fakeCoach` with `AdjustToday`
- `backend/internal/api/handlers_test.go` — wire `Agent`/`Pusher` fakes into `newTestServer`
- `backend/internal/config/config.go` — add M2 env fields
- `backend/cmd/server/main.go` — `import _ "time/tzdata"`; build `push.Client` + `agent.Agent`; launch `scheduler.Run`; expose `Coach`/`Agent`/`Pusher` on `App`

### App — new files
- `app/src/lib/notifications.ts` — `registerForPushNotificationsAsync()` + module-scope `setNotificationHandler`
- `app/app/__tests__/appConfig.test.ts` — app.json plugin/projectId tests
- `app/app/__tests__/notifications.test.ts` — mocked registration tests

### App — modified files
- `app/package.json` — add `expo-notifications` dep
- `app/app.json` — add `expo-notifications` plugin + `extra.eas.projectId` + Android channel config
- `app/app/_layout.tsx` — `useEffect` registering the push token on launch
- `app/app/__tests__/_layout.test.tsx` — notifications mock + registration-on-mount test
- `app/src/api/types.ts` — add `TodayBriefing`, `ReadinessDrivers`, `DailyAction`, `PushRegisterRequest`, `RunResult`; extend `AthleteProfile`
- `app/src/api/__tests__/types.test.ts` — M2 types block
- `app/src/api/hooks.ts` — add `useToday`/`useUndoToday`/`useRunAgent`/`useRegisterPushToken`
- `app/src/api/__tests__/hooks.test.tsx` — 4 new hook tests
- `app/app/index.tsx` — add `<TodayCard>` (readiness pill + drivers + what-changed + Undo)
- `app/app/__tests__/index.test.tsx` — Today-card tests
- `app/app/settings.tsx` — daily-run time + timezone + agent enable toggle via `useUpdateProfile`
- `app/app/__tests__/settings.test.tsx` — agent-schedule tests
- `app/app/__tests__/profile.test.tsx` — extend `AthleteProfile` fixtures with M2 fields

---

## Shared Contracts

These are the authoritative interfaces. Every task MUST agree on these names, types, JSON tags, paths, and signatures. Conventions inherited from M0/M1: Go module `help-my-run/backend`; **all data-payload JSON tags are snake_case**; error shape `map[string]string{"error": "..."}`; migrations `0000N_name.sql` goose dialect `sqlite3`; coach/llm errors → HTTP 502, `store.ErrNotFound` → 404, validation → 400; `time.Now().UTC().Format(time.RFC3339)` for stored timestamps.

### Migration `00004_m2_agent.sql`

```sql
-- +goose Up
-- +goose StatementBegin

CREATE TABLE device_tokens (
    expo_push_token TEXT PRIMARY KEY,
    platform        TEXT NOT NULL CHECK (platform IN ('ios','android')),
    updated_at      TEXT NOT NULL
);

CREATE TABLE daily_decisions (
    date                  TEXT PRIMARY KEY,            -- ISO local date YYYY-MM-DD
    readiness_color       TEXT NOT NULL CHECK (readiness_color IN ('green','amber','red')),
    drivers_json          TEXT NOT NULL,              -- serialized ReadinessDrivers
    original_session_json TEXT,                        -- llm.PlanDay JSON; NULL if no run scheduled
    adjusted_session_json TEXT,                        -- effective session (PlanDay JSON); NULL if no run / undone-to-null
    action                TEXT NOT NULL CHECK (action IN ('STAND','SOFTEN','MOVE','REST_DAY')),
    rationale             TEXT NOT NULL DEFAULT '',
    source                TEXT NOT NULL CHECK (source IN ('ai','fallback')),
    raw_response          TEXT,                        -- raw claude -p .result (NULL on fallback)
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

CREATE TABLE agent_runs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    last_run_date TEXT NOT NULL,                        -- local date YYYY-MM-DD (idempotency key)
    status        TEXT NOT NULL CHECK (status IN ('ok','error')),
    error         TEXT,
    ran_at        TEXT NOT NULL                         -- RFC3339 UTC
);

CREATE UNIQUE INDEX idx_agent_runs_last_run_date ON agent_runs (last_run_date);

ALTER TABLE athlete_profile ADD COLUMN daily_run_time TEXT    NOT NULL DEFAULT '05:30';
ALTER TABLE athlete_profile ADD COLUMN timezone       TEXT    NOT NULL DEFAULT 'UTC';
ALTER TABLE athlete_profile ADD COLUMN agent_enabled  INTEGER NOT NULL DEFAULT 1;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE athlete_profile DROP COLUMN agent_enabled;
ALTER TABLE athlete_profile DROP COLUMN timezone;
ALTER TABLE athlete_profile DROP COLUMN daily_run_time;
DROP INDEX idx_agent_runs_last_run_date;
DROP TABLE agent_runs;
DROP TABLE daily_decisions;
DROP TABLE device_tokens;
-- +goose StatementEnd
```

`athlete_profile` is a single seeded row (`id=1`); new columns use `DEFAULT` so the existing row stays valid. `daily_run_time` is `HH:MM` 24h local; `timezone` is an IANA name; `agent_enabled` is `0|1`. `action` adds `REST_DAY` (no-run readiness-only briefing) beyond the spec's `STAND|SOFTEN|MOVE`.

### Readiness thresholds + fallback rule

Package-level exported constants (`internal/readiness`):

```go
const (
	BaselineWindowDays = 14   // baseline = mean over the prior up-to-14 days (excluding last night)
	MinBaselineDays    = 3    // fewer than this → baseline unavailable → DataComplete=false

	RedSleepHours   = 5.0   // < 5.0h slept → RED
	AmberSleepHours = 6.5   // < 6.5h slept → AMBER
	RedSleepScore   = 50    // sleep score < 50 → RED
	AmberSleepScore = 65    // sleep score < 65 → AMBER

	RedHRVDropPct   = -15.0 // HRV ≤ -15% vs baseline → RED
	AmberHRVDropPct = -7.0  // HRV ≤ -7% vs baseline → AMBER

	RedRHRRiseBpm   = 7.0   // RHR ≥ +7 bpm vs baseline → RED
	AmberRHRRiseBpm = 4.0   // RHR ≥ +4 bpm vs baseline → AMBER

	RedBodyBattery   = 30   // overnight BodyBattery high < 30 → RED
	AmberBodyBattery = 50   // overnight BodyBattery high < 50 → AMBER
)
```

**Aggregation (worst-wins with confirmation):** RED if any signal RED OR ≥2 AMBER; AMBER if exactly 1 AMBER OR `DataComplete==false` (missing last-night data → conservative AMBER, never silent GREEN); GREEN otherwise. Trend modifier: `"declining"` adds one AMBER-weight signal; `"improving"` cancels one.

**Deterministic fallback rule** (`readiness.Fallback`, used by `coach.AdjustToday` on any `*llm.CallError`/`llm.ErrMalformedJSON`; `source="fallback"`, no `raw_response`):

| Readiness | Run today? | Action | Adjusted session |
|---|---|---|---|
| RED | yes (quality: tempo/intervals/long) | `MOVE` | `run_type="recovery"`, `distance_km=min(orig,4)` rounded to 0.5, easy pace, `optional_if_cns=true` |
| RED | yes (already easy/recovery) | `SOFTEN` | keep type, `distance_km=round(orig*0.5,0.5)`, easy pace, `optional_if_cns=true` |
| RED | no run | `REST_DAY` | `adjusted=nil` |
| AMBER | yes (quality) | `SOFTEN` | keep type, `distance_km=round(orig*0.75,0.5)`, pace eased to easy |
| AMBER | yes (easy/recovery) | `STAND` | unchanged |
| AMBER | no run | `REST_DAY` | `adjusted=nil` |
| GREEN | yes | `STAND` | unchanged |
| GREEN | no run | `REST_DAY` | `adjusted=nil` |

Easy pace comes from `coach.Fitness(ctx)` — since `Fitness` returns `(FitnessMetrics, error)` it cannot be chained, so the coach reads it as `fit, _ := coach.Fitness(ctx); fit.EasyPace`; `round(x,0.5)` rounds to nearest 0.5 km.

### Daily-adjust `claude -p` contract + model-output JSON

Args (`dailyAdjustArgs()`, mirrors M1's `stage2Args`):

```go
[]string{"-p", dailyAdjustPrompt, "--model", c.model, "--output-format", "json", "--allowedTools", "", "--no-session-persistence"}
```

Stdin input (`coach.DailyAdjustInput`, snake_case JSON):

```go
type DailyAdjustInput struct {
	Date          string                 `json:"date"`           // local date YYYY-MM-DD
	Readiness     readiness.Readiness    `json:"readiness"`
	TodaySession  *llm.PlanDay           `json:"today_session"`  // nil if no run scheduled
	Metrics       metrics.FitnessMetrics `json:"metrics"`
	Profile       ProfilePack            `json:"profile"`
	CrossFitToday *llm.CrossFitDay       `json:"crossfit_today"` // nil if none
	WeekRationale string                 `json:"week_rationale"`
}
```

EXACT model-output struct (`llm/types.go`; `adjusted_session` reuses `llm.PlanDay` verbatim):

```go
type DailyAction string

const (
	ActionStand   DailyAction = "STAND"
	ActionSoften  DailyAction = "SOFTEN"
	ActionMove    DailyAction = "MOVE"
	ActionRestDay DailyAction = "REST_DAY"
)

type DailyDecisionParsed struct {
	Action          DailyAction `json:"action"`
	AdjustedSession *PlanDay    `json:"adjusted_session"` // nil for REST_DAY
	Rationale       string      `json:"rationale"`
}
```

`AdjustToday` signature: `func (c *Coach) AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (decision llm.DailyDecisionParsed, raw string, source string, err error)`. On `Call` error → deterministic fallback, `source="fallback"`, `raw=""`, nil error. On success → `source="ai"`, `raw` = re-marshaled `decision` JSON (consistent with M1's `ParseCrossFit`).

Column mapping: `date`←Input.Date; `readiness_color`←rd.Color; `drivers_json`←marshal(rd.Drivers); `original_session_json`←marshal(today) (NULL if nil); `adjusted_session_json`←marshal(decision.AdjustedSession) (NULL if nil); `action`←decision.Action; `rationale`←decision.Rationale; `source`←source; `raw_response`←raw (NULL/empty on fallback); `created_at`/`updated_at`←RFC3339 UTC now.

### Go types

`internal/store/daily.go` rows:

```go
type DeviceToken struct {
	ExpoPushToken string // PK
	Platform      string // "ios"|"android"
	UpdatedAt     string
}
type DailyDecision struct {
	Date                string  // PK, local date YYYY-MM-DD
	ReadinessColor      string  // "green"|"amber"|"red"
	DriversJSON         string
	OriginalSessionJSON *string // PlanDay JSON or nil
	AdjustedSessionJSON *string // PlanDay JSON or nil
	Action              string  // "STAND"|"SOFTEN"|"MOVE"|"REST_DAY"
	Rationale           string
	Source              string  // "ai"|"fallback"
	RawResponse         *string
	CreatedAt           string
	UpdatedAt           string
}
type AgentRun struct {
	ID          int64
	LastRunDate string // local date YYYY-MM-DD
	Status      string // "ok"|"error"
	Error       *string
	RanAt       string // RFC3339 UTC
}
```

Store methods (all on `*Store`; `ErrNotFound` on no rows):

```go
func (s *Store) UpsertDeviceToken(t DeviceToken) error
func (s *Store) ListDeviceTokens() ([]DeviceToken, error)
func (s *Store) DeleteDeviceToken(expoPushToken string) error          // DeviceNotRegistered drop
func (s *Store) UpsertDailyDecision(d DailyDecision) error             // ON CONFLICT(date) DO UPDATE
func (s *Store) GetDailyDecision(date string) (DailyDecision, error)
func (s *Store) UpsertAgentRun(r AgentRun) error                       // ON CONFLICT(last_run_date) DO UPDATE
func (s *Store) GetAgentRun(date string) (AgentRun, error)
func (s *Store) LatestAgentRun() (AgentRun, error)
func (s *Store) DeleteAgentRun(date string) error                      // DELETE FROM agent_runs WHERE last_run_date=?; no-op if absent (resets force guard)
```

`store.AthleteProfile` additions: `DailyRunTime string`, `Timezone string`, `AgentEnabled bool`.

`internal/agent/agent.go` seams + result:

```go
type Syncer  interface { SyncAll(ctx context.Context) sync.AllResult }
type Adjuster interface {
	AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error)
	Fitness(ctx context.Context) (metrics.FitnessMetrics, error)
}
type Pusher interface { Send(ctx context.Context, msg push.Message) error }
type Clock  interface { Now() time.Time }

type RunResult struct {
	Date           string          `json:"date"`
	Skipped        bool            `json:"skipped"`
	ReadinessColor readiness.Color `json:"readiness_color"`
	Action         llm.DailyAction `json:"action"`
	Source         string          `json:"source"`           // "ai"|"fallback"
	Stale          bool            `json:"stale"`
	Pushed         bool            `json:"pushed"`
	Error          *string         `json:"error"`
}

func New(s *store.Store, syncer Syncer, adjuster Adjuster, pusher Pusher, clock Clock, loc *time.Location) *Agent
func (a *Agent) RunDaily(ctx context.Context, localDate string) RunResult
```

`internal/push/push.go` message:

```go
type Message struct {
	To        string                 `json:"to"`                  // "ExponentPushToken[...]"
	Title     string                 `json:"title,omitempty"`
	Body      string                 `json:"body,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Sound     string                 `json:"sound,omitempty"`     // "default"
	Priority  string                 `json:"priority,omitempty"`  // "high"
	ChannelID string                 `json:"channelId,omitempty"` // "default"
}
type Sender interface { Send(ctx context.Context, msg Message) error }
type Client struct { BaseURL string; HTTPClient *http.Client }
func NewClient(baseURL string) *Client                  // ""→"https://exp.host"
var ErrDeviceNotRegistered = errors.New("push: device not registered")
func (c *Client) Send(ctx context.Context, msg Message) error
// POSTs BaseURL+"/--/api/v2/push/send"; Content-Type+Accept application/json;
// DeviceNotRegistered ticket → ErrDeviceNotRegistered.
```

### REST API contract

All under bearer auth, snake_case JSON.

- `POST /api/push/register` — body `{"expo_push_token","platform"}`; 200 `{"expo_push_token","platform","updated_at"}`; 400 if token empty or platform∉{ios,android}.
- `GET /api/today` (optional `?date=YYYY-MM-DD`, defaults to server UTC today) → `todayResponseDTO` (date, readiness_color, drivers, reasons[], action, original_session|null, effective_session|null, rationale, source, stale); 404 `{"error":"no decision for date"}` if absent; 400 on bad date.
- `POST /api/today/undo` (empty body, optional `?date=`) → updated `todayResponseDTO` with `effective_session==original_session`, `action="STAND"`, rationale "Reverted to original session."; 404 if absent.
- `POST /api/agent/run` (empty body, optional `?date=` and `?force=true`) → `agent.RunResult` JSON; 400 on bad date.
- `GET`/`PUT /api/profile` — `profileDTO` gains `daily_run_time` ("HH:MM", default "05:30"), `timezone` (IANA, default "UTC"), `agent_enabled` (bool, default true). `PUT` defaults empty run-time→"05:30"/tz→"UTC"; validates run-time `^([01]\d|2[0-3]):[0-5]\d$` and `time.LoadLocation(timezone)` → 400 otherwise.

`todayResponseDTO` example (rest day → `original_session`/`effective_session` are `null`, `action:"REST_DAY"`):

```json
{
  "date": "2026-06-20", "readiness_color": "amber",
  "drivers": { "date": "2026-06-20", "sleep_hours": 6.1, "sleep_score": 62,
    "hrv_last_night_ms": 48, "hrv_baseline_ms": 58.4, "hrv_delta_pct": -17.8,
    "rhr_last_night": 54, "rhr_baseline": 50.2, "rhr_delta_bpm": 3.8,
    "body_battery_high": 61, "recovery_trend": "declining", "data_complete": true },
  "reasons": ["HRV -17.8% vs baseline", "Sleep score 62 (<65)"],
  "action": "SOFTEN",
  "original_session": { "date": "2026-06-20", "dow": "Fri", "run_type": "tempo", "distance_km": 6,
    "pace_target": "5:05/km", "time_note": "~20:00 after CrossFit", "optional_if_cns": false, "rationale": "Threshold work on a light CF day." },
  "effective_session": { "date": "2026-06-20", "dow": "Fri", "run_type": "easy", "distance_km": 4.5,
    "pace_target": "6:00/km", "time_note": "~20:00 after CrossFit", "optional_if_cns": true, "rationale": "Trimmed to easy: HRV down, sleep short." },
  "rationale": "HRV is 18% below baseline and sleep was short, so today's tempo is eased to an easy 4.5k.",
  "source": "ai", "stale": false
}
```

### Scheduler contract

`internal/scheduler/scheduler.go` — first injectable clock in the codebase.

```go
type Clock interface {
	Now() time.Time
	NewTimer(d time.Duration) (<-chan time.Time, func() bool) // (C, stop)
}
type RealClock struct{}
type Config struct { Hour, Minute int; Loc *time.Location }
func nextFire(from time.Time, cfg Config) time.Time           // next strictly-after instant at Hour:Minute in Loc

// ConfigProvider re-reads the live schedule from athlete_profile on EVERY loop
// iteration so PUT /api/profile edits (spec §7/§8) take effect WITHOUT a restart.
// It returns the resolved Config (HH:MM + IANA tz already parsed by the caller),
// whether the agent is enabled, and any error (on error Run waits ~1m and retries).
type ConfigProvider func() (cfg Config, enabled bool, err error)
func Run(ctx context.Context, clk Clock, next ConfigProvider, fn func(ctx context.Context, localDate string))
```

**Runtime config:** `Run` calls `next()` each iteration, so changing `daily_run_time`/`timezone` recomputes the next fire on the following cycle, and toggling `agent_enabled=false` makes `Run` SKIP firing `fn` (it still waits for the next instant and re-reads, so re-enabling resumes without a restart). None of this requires a process restart.

**Idempotency is two-layer:** (1) in-process `lastFired` local-date string inside `Run` (guards duplicate timer fires); (2) persistent `agent_runs.last_run_date` in `agent.RunDaily` (`Skipped:true` if already run; survives restarts, powers `POST /api/agent/run` catch-up). Idempotency key = local-date `YYYY-MM-DD` in the configured tz.

### Config additions

`internal/config/config.go` — the live schedule (time/tz/enable) is re-read from `athlete_profile` on every scheduler loop iteration (via the `scheduler.ConfigProvider` wired in Task 25), so `PUT /api/profile` edits take effect without a restart; the env vars below are only first-boot defaults seeded into the profile + the push test seam, NOT the runtime source.

```go
AgentEnabledDefault bool   `envconfig:"AGENT_ENABLED" default:"true"`
AgentRunTime        string `envconfig:"AGENT_RUN_TIME" default:"05:30"`     // "HH:MM"
AgentTimezone       string `envconfig:"AGENT_TZ" default:"UTC"`             // IANA
AgentTickInterval   string `envconfig:"AGENT_TICK_INTERVAL" default:"1m"`   // scheduler re-check cadence
ExpoPushBaseURL     string `envconfig:"EXPO_PUSH_BASE_URL" default:"https://exp.host"`
```

`cmd/server/main.go` adds `import _ "time/tzdata"` (embeds the IANA DB so `time.LoadLocation` works headless; ~450KB) and `push.NewClient(cfg.ExpoPushBaseURL)`.

### Cross-cutting invariants
- **JSON style = snake_case** for all M2 wire/DTO/stored JSON.
- **`llm.PlanDay`** is the canonical session shape everywhere — never redefine its fields.
- **Action enum** = `STAND | SOFTEN | MOVE | REST_DAY` (Go `llm.DailyAction`, TS `DailyAction`, SQLite CHECK, prompt output) — identical strings in all four places.
- **Readiness color** = `green | amber | red` lowercase in JSON/DB; Go consts `ColorGreen/Amber/Red`.
- **`source`** = `ai | fallback`; fallback never sets `raw_response`. A no-run REST_DAY briefing skips the claude call and stores `source="fallback"` (CHECK allows only `ai|fallback`).
- **Push**: POST `https://exp.host/--/api/v2/push/send`; `DeviceNotRegistered` ticket → delete the `device_tokens` row. Base URL injectable via `EXPO_PUSH_BASE_URL`.
- **Reasons + stale persistence:** the agent embeds `reasons` AND `stale` inside `drivers_json` as sibling keys (`{...driver fields..., "reasons":[...], "stale":true}`) — no schema change. `readinessDriversDTO` ignores the extra keys on unmarshal; `parseReasons`/`parseStale` read them back in `today_handlers.go` so `GET /api/today` (and undo) report the real staleness recorded at run time.

---

## Tasks

Tasks are renumbered into one global execution order. Build deps require this ordering: store (migration + rows + profile columns) → readiness → push → llm/coach daily-adjust → agent loop → scheduler → api handlers → main wiring → app. All backend commands run from `/home/jake/project/help-my-run/backend` (module root) except git, which uses `-C /home/jake/project/help-my-run` or `cd /home/jake/project/help-my-run`. All app commands run from `/home/jake/project/help-my-run/app`.

### Task 1: Migration 00004 (device_tokens, daily_decisions, agent_runs, athlete_profile columns)

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/store/migrations/00004_m2_agent.sql`
- Test: `/home/jake/project/help-my-run/backend/internal/store/daily_test.go` (new file; migration-shape test only — round-trip tests added in Task 2)

- [ ] **Step 1: Write the failing migration test.** Create `/home/jake/project/help-my-run/backend/internal/store/daily_test.go` (mirrors `TestM1MigrationCreatesTables`):

```go
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
```

- [ ] **Step 2: Run the test, expect FAIL (no migration file).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run 'TestM2Migration'
```
Expected: FAIL — `no such table: device_tokens` / `no such column: daily_run_time`.

- [ ] **Step 3: Write the migration file.** Create `/home/jake/project/help-my-run/backend/internal/store/migrations/00004_m2_agent.sql`:

```sql
-- +goose Up
-- +goose StatementBegin

CREATE TABLE device_tokens (
    expo_push_token TEXT PRIMARY KEY,
    platform        TEXT NOT NULL CHECK (platform IN ('ios','android')),
    updated_at      TEXT NOT NULL
);

CREATE TABLE daily_decisions (
    date                  TEXT PRIMARY KEY,
    readiness_color       TEXT NOT NULL CHECK (readiness_color IN ('green','amber','red')),
    drivers_json          TEXT NOT NULL,
    original_session_json TEXT,
    adjusted_session_json TEXT,
    action                TEXT NOT NULL CHECK (action IN ('STAND','SOFTEN','MOVE','REST_DAY')),
    rationale             TEXT NOT NULL DEFAULT '',
    source                TEXT NOT NULL CHECK (source IN ('ai','fallback')),
    raw_response          TEXT,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

CREATE TABLE agent_runs (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    last_run_date TEXT NOT NULL,
    status        TEXT NOT NULL CHECK (status IN ('ok','error')),
    error         TEXT,
    ran_at        TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_agent_runs_last_run_date ON agent_runs (last_run_date);

ALTER TABLE athlete_profile ADD COLUMN daily_run_time TEXT    NOT NULL DEFAULT '05:30';
ALTER TABLE athlete_profile ADD COLUMN timezone       TEXT    NOT NULL DEFAULT 'UTC';
ALTER TABLE athlete_profile ADD COLUMN agent_enabled  INTEGER NOT NULL DEFAULT 1;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE athlete_profile DROP COLUMN agent_enabled;
ALTER TABLE athlete_profile DROP COLUMN timezone;
ALTER TABLE athlete_profile DROP COLUMN daily_run_time;
DROP INDEX idx_agent_runs_last_run_date;
DROP TABLE agent_runs;
DROP TABLE daily_decisions;
DROP TABLE device_tokens;
-- +goose StatementEnd
```

- [ ] **Step 4: Run the test, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run 'TestM2Migration'
```
Expected: `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/store/migrations/00004_m2_agent.sql backend/internal/store/daily_test.go && git commit -m "feat(store): add 00004 migration for device_tokens, daily_decisions, agent_runs + profile columns

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Store types + CRUD for DeviceToken, DailyDecision, AgentRun

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/store/daily.go`
- Test: `/home/jake/project/help-my-run/backend/internal/store/daily_test.go` (append round-trip tests)

- [ ] **Step 1: Write the failing round-trip tests.** Append to `/home/jake/project/help-my-run/backend/internal/store/daily_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests, expect FAIL (compile error).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run 'TestDeviceToken|TestDailyDecision|TestAgentRun'
```
Expected: FAIL — `undefined: DeviceToken` / `s.UpsertDeviceToken undefined` / `FAIL	help-my-run/backend/internal/store [build failed]`.

- [ ] **Step 3: Write the store impl.** Create `/home/jake/project/help-my-run/backend/internal/store/daily.go`:

```go
package store

import (
	"database/sql"
	"errors"
	"time"
)

// DeviceToken is the device_tokens row (one per registered device).
type DeviceToken struct {
	ExpoPushToken string // PK
	Platform      string // "ios"|"android"
	UpdatedAt     string
}

// DailyDecision is the daily_decisions row. Session JSONs are raw PlanDay JSON
// strings so the store stays agnostic of llm types; nil columns are *string.
type DailyDecision struct {
	Date                string  // PK, local date YYYY-MM-DD
	ReadinessColor      string  // "green"|"amber"|"red"
	DriversJSON         string
	OriginalSessionJSON *string // PlanDay JSON or nil
	AdjustedSessionJSON *string // PlanDay JSON or nil
	Action              string  // "STAND"|"SOFTEN"|"MOVE"|"REST_DAY"
	Rationale           string
	Source              string // "ai"|"fallback"
	RawResponse         *string
	CreatedAt           string
	UpdatedAt           string
}

// AgentRun is one agent_runs row (idempotency + last-run history).
type AgentRun struct {
	ID          int64
	LastRunDate string // local date YYYY-MM-DD
	Status      string // "ok"|"error"
	Error       *string
	RanAt       string // RFC3339 UTC
}

// UpsertDeviceToken inserts/refreshes a device token by its PK. updated_at is set
// server-side to now (UTC RFC3339).
func (s *Store) UpsertDeviceToken(t DeviceToken) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO device_tokens (expo_push_token, platform, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(expo_push_token) DO UPDATE SET
			platform   = excluded.platform,
			updated_at = excluded.updated_at`,
		t.ExpoPushToken, t.Platform, now)
	return err
}

// ListDeviceTokens returns all registered device tokens (most-recent-first).
func (s *Store) ListDeviceTokens() ([]DeviceToken, error) {
	rows, err := s.DB.Query(`
		SELECT expo_push_token, platform, updated_at
		FROM device_tokens ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DeviceToken, 0)
	for rows.Next() {
		var t DeviceToken
		if err := rows.Scan(&t.ExpoPushToken, &t.Platform, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteDeviceToken removes a token (the DeviceNotRegistered drop path). Deleting
// a missing token is a no-op.
func (s *Store) DeleteDeviceToken(expoPushToken string) error {
	_, err := s.DB.Exec(`DELETE FROM device_tokens WHERE expo_push_token = ?`, expoPushToken)
	return err
}

// UpsertDailyDecision upserts today's decision by date. created_at is set on
// insert and preserved on update; updated_at is always bumped to now.
func (s *Store) UpsertDailyDecision(d DailyDecision) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO daily_decisions
			(date, readiness_color, drivers_json, original_session_json,
			 adjusted_session_json, action, rationale, source, raw_response,
			 created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(date) DO UPDATE SET
			readiness_color       = excluded.readiness_color,
			drivers_json          = excluded.drivers_json,
			original_session_json = excluded.original_session_json,
			adjusted_session_json = excluded.adjusted_session_json,
			action                = excluded.action,
			rationale             = excluded.rationale,
			source                = excluded.source,
			raw_response          = excluded.raw_response,
			updated_at            = excluded.updated_at`,
		d.Date, d.ReadinessColor, d.DriversJSON, d.OriginalSessionJSON,
		d.AdjustedSessionJSON, d.Action, d.Rationale, d.Source, d.RawResponse,
		now, now)
	return err
}

// GetDailyDecision returns the decision row for date, or ErrNotFound.
func (s *Store) GetDailyDecision(date string) (DailyDecision, error) {
	var d DailyDecision
	var orig, adj, raw sql.NullString
	err := s.DB.QueryRow(`
		SELECT date, readiness_color, drivers_json, original_session_json,
		       adjusted_session_json, action, rationale, source, raw_response,
		       created_at, updated_at
		FROM daily_decisions WHERE date = ?`, date).
		Scan(&d.Date, &d.ReadinessColor, &d.DriversJSON, &orig, &adj,
			&d.Action, &d.Rationale, &d.Source, &raw, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return DailyDecision{}, ErrNotFound
	}
	if err != nil {
		return DailyDecision{}, err
	}
	if orig.Valid {
		d.OriginalSessionJSON = &orig.String
	}
	if adj.Valid {
		d.AdjustedSessionJSON = &adj.String
	}
	if raw.Valid {
		d.RawResponse = &raw.String
	}
	return d, nil
}

// UpsertAgentRun upserts the agent run for a local date (idempotency key).
// ran_at is set server-side to now (UTC RFC3339).
func (s *Store) UpsertAgentRun(r AgentRun) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO agent_runs (last_run_date, status, error, ran_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(last_run_date) DO UPDATE SET
			status = excluded.status,
			error  = excluded.error,
			ran_at = excluded.ran_at`,
		r.LastRunDate, r.Status, r.Error, now)
	return err
}

// GetAgentRun returns the agent run for date, or ErrNotFound.
func (s *Store) GetAgentRun(date string) (AgentRun, error) {
	var r AgentRun
	var errStr sql.NullString
	err := s.DB.QueryRow(`
		SELECT id, last_run_date, status, error, ran_at
		FROM agent_runs WHERE last_run_date = ?`, date).
		Scan(&r.ID, &r.LastRunDate, &r.Status, &errStr, &r.RanAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRun{}, ErrNotFound
	}
	if err != nil {
		return AgentRun{}, err
	}
	if errStr.Valid {
		r.Error = &errStr.String
	}
	return r, nil
}

// LatestAgentRun returns the most-recent agent run by date, or ErrNotFound.
func (s *Store) LatestAgentRun() (AgentRun, error) {
	var r AgentRun
	var errStr sql.NullString
	err := s.DB.QueryRow(`
		SELECT id, last_run_date, status, error, ran_at
		FROM agent_runs ORDER BY last_run_date DESC LIMIT 1`).
		Scan(&r.ID, &r.LastRunDate, &r.Status, &errStr, &r.RanAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentRun{}, ErrNotFound
	}
	if err != nil {
		return AgentRun{}, err
	}
	if errStr.Valid {
		r.Error = &errStr.String
	}
	return r, nil
}

// DeleteAgentRun removes the agent_runs row for a local date, resetting the
// persistent once-per-day guard so POST /api/agent/run?force=true can re-run the
// day. Deleting a missing row is a no-op.
func (s *Store) DeleteAgentRun(date string) error {
	_, err := s.DB.Exec(`DELETE FROM agent_runs WHERE last_run_date = ?`, date)
	return err
}
```

- [ ] **Step 4: Run the tests, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run 'TestDeviceToken|TestDailyDecision|TestAgentRun'
```
Expected: `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 5: Commit.** Command:
```
cd /home/jake/project/help-my-run && git add backend/internal/store/daily.go backend/internal/store/daily_test.go && git commit -m "feat(store): typed CRUD for device_tokens, daily_decisions, agent_runs

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Extend AthleteProfile for daily_run_time / timezone / agent_enabled

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/store/profile.go`
- Modify: `/home/jake/project/help-my-run/backend/internal/store/profile_test.go` (append `TestAthleteProfileM2Columns`; the file ALREADY EXISTS with the M1 tests `TestGetAthleteProfileSeeded` and `TestUpsertAthleteProfileRoundTrip` — preserve them, do NOT recreate the file or its `package store`/`import "testing"` header)

- [ ] **Step 1: Write the failing profile round-trip test.** APPEND to the existing `/home/jake/project/help-my-run/backend/internal/store/profile_test.go` (it already declares `package store` and imports `"testing"`; add ONLY the function below, after the M1 tests — leave `TestGetAthleteProfileSeeded`/`TestUpsertAthleteProfileRoundTrip` untouched):

```go
func TestAthleteProfileM2Columns(t *testing.T) {
	s := newTestStore(t)

	p, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile error = %v", err)
	}
	if p.DailyRunTime != "05:30" || p.Timezone != "UTC" || p.AgentEnabled != true {
		t.Errorf("seed M2 fields = (%q,%q,%v), want (05:30,UTC,true)", p.DailyRunTime, p.Timezone, p.AgentEnabled)
	}

	p.DailyRunTime = "06:15"
	p.Timezone = "Asia/Seoul"
	p.AgentEnabled = false
	if err := s.UpsertAthleteProfile(p); err != nil {
		t.Fatalf("UpsertAthleteProfile error = %v", err)
	}
	got, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile after upsert error = %v", err)
	}
	if got.DailyRunTime != "06:15" || got.Timezone != "Asia/Seoul" || got.AgentEnabled != false {
		t.Errorf("M2 fields = (%q,%q,%v), want (06:15,Asia/Seoul,false)", got.DailyRunTime, got.Timezone, got.AgentEnabled)
	}
	if got.TargetWeeklyKm != p.TargetWeeklyKm || got.ProgressionMode != p.ProgressionMode {
		t.Errorf("M1 fields drifted: got %+v", got)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL (compile error).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run 'TestAthleteProfileM2Columns'
```
Expected: FAIL — `p.DailyRunTime undefined` / `FAIL	help-my-run/backend/internal/store [build failed]`.

- [ ] **Step 3: Extend the struct + getter + upsert.** In `/home/jake/project/help-my-run/backend/internal/store/profile.go`, add 3 fields after `GoalText` in the `AthleteProfile` struct:

```go
	DailyRunTime       string // "HH:MM" 24h local (M2)
	Timezone           string // IANA, e.g. "Asia/Seoul" (M2)
	AgentEnabled       bool   // M2 daily agent on/off
```

Extend `GetAthleteProfile` SELECT + scan (add the new columns; `agent_enabled` scanned into an int64 then converted):

```go
func (s *Store) GetAthleteProfile() (AthleteProfile, error) {
	var p AthleteProfile
	var z2, thr, mx sql.NullInt64
	var agentEnabled int64
	err := s.DB.QueryRow(`
		SELECT target_weekly_km, progression_mode, zone2_ceiling_bpm, threshold_bpm,
		       max_hr_bpm, run_constraints_json, goal_text,
		       daily_run_time, timezone, agent_enabled, updated_at
		FROM athlete_profile WHERE id = 1`).
		Scan(&p.TargetWeeklyKm, &p.ProgressionMode, &z2, &thr, &mx,
			&p.RunConstraintsJSON, &p.GoalText,
			&p.DailyRunTime, &p.Timezone, &agentEnabled, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AthleteProfile{}, ErrNotFound
	}
	if err != nil {
		return AthleteProfile{}, err
	}
	if z2.Valid {
		p.Zone2CeilingBpm = &z2.Int64
	}
	if thr.Valid {
		p.ThresholdBpm = &thr.Int64
	}
	if mx.Valid {
		p.MaxHRBpm = &mx.Int64
	}
	p.AgentEnabled = agentEnabled != 0
	return p, nil
}
```

Extend `UpsertAthleteProfile` INSERT/UPDATE column lists (bool persisted via a local int64):

```go
func (s *Store) UpsertAthleteProfile(p AthleteProfile) error {
	now := time.Now().UTC().Format(time.RFC3339)
	agentEnabled := int64(0)
	if p.AgentEnabled {
		agentEnabled = 1
	}
	_, err := s.DB.Exec(`
		INSERT INTO athlete_profile
			(id, target_weekly_km, progression_mode, zone2_ceiling_bpm, threshold_bpm,
			 max_hr_bpm, run_constraints_json, goal_text,
			 daily_run_time, timezone, agent_enabled, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			target_weekly_km     = excluded.target_weekly_km,
			progression_mode     = excluded.progression_mode,
			zone2_ceiling_bpm    = excluded.zone2_ceiling_bpm,
			threshold_bpm        = excluded.threshold_bpm,
			max_hr_bpm           = excluded.max_hr_bpm,
			run_constraints_json = excluded.run_constraints_json,
			goal_text            = excluded.goal_text,
			daily_run_time       = excluded.daily_run_time,
			timezone             = excluded.timezone,
			agent_enabled        = excluded.agent_enabled,
			updated_at           = excluded.updated_at`,
		p.TargetWeeklyKm, p.ProgressionMode, p.Zone2CeilingBpm, p.ThresholdBpm,
		p.MaxHRBpm, p.RunConstraintsJSON, p.GoalText,
		p.DailyRunTime, p.Timezone, agentEnabled, now)
	return err
}
```

- [ ] **Step 4: Run the test, expect PASS (no regressions).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/
```
Expected: `ok  	help-my-run/backend/internal/store` (M2 test plus all existing store tests pass).

- [ ] **Step 5: Commit.** Command:
```
cd /home/jake/project/help-my-run && git add backend/internal/store/profile.go backend/internal/store/profile_test.go && git commit -m "feat(store): extend AthleteProfile with daily_run_time, timezone, agent_enabled

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Export `metrics.RecoveryTrend` for readiness reuse

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/metrics/metrics.go` (add exported wrapper `RecoveryTrend` delegating to private `recoveryTrend`)
- Test: `/home/jake/project/help-my-run/backend/internal/metrics/metrics_test.go` (add `TestRecoveryTrendExported`)

The readiness engine needs the recovery-trend signal. Expose `metrics.RecoveryTrend(...)` rather than re-implementing. Keep the existing private `recoveryTrend` and its tests untouched; add a one-line exported wrapper.

- [ ] **Step 1: Write failing test for the exported wrapper.** Append to `/home/jake/project/help-my-run/backend/internal/metrics/metrics_test.go`:
```go
func TestRecoveryTrendExported(t *testing.T) {
	ip := func(v int64) *int64 { return &v }
	day := func(date string, hrv, sleep int64) store.RecoveryDay {
		return store.RecoveryDay{
			Date:  date,
			HRV:   &store.HrvFields{LastNightAvgMs: ip(hrv)},
			Sleep: &store.SleepFields{Score: ip(sleep)},
		}
	}
	rec := []store.RecoveryDay{
		day("2026-06-22", 60, 85), day("2026-06-21", 58, 84), day("2026-06-20", 59, 86),
		day("2026-06-19", 48, 72), day("2026-06-18", 47, 70), day("2026-06-17", 49, 71),
	}
	if got, want := RecoveryTrend(rec), recoveryTrend(rec); got != want {
		t.Errorf("RecoveryTrend = %q, want %q (parity with private)", got, want)
	}
	if got := RecoveryTrend(rec); got != "improving" {
		t.Errorf("RecoveryTrend = %q, want improving", got)
	}
	if got := RecoveryTrend(nil); got != "stable" {
		t.Errorf("RecoveryTrend(nil) = %q, want stable", got)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL (undefined: RecoveryTrend).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/ -run TestRecoveryTrendExported
```
Expected output contains: `undefined: RecoveryTrend` and `FAIL	help-my-run/backend/internal/metrics`.

- [ ] **Step 3: Add the exported wrapper.** Append to `/home/jake/project/help-my-run/backend/internal/metrics/metrics.go` (after the private `recoveryTrend` function, before `cutbackEpoch`):
```go
// RecoveryTrend is the exported wrapper over the private recoveryTrend so other
// packages (e.g. internal/readiness) can reuse the identical trend computation.
// recovery is most-recent-first (as ListRecovery returns).
func RecoveryTrend(recovery []store.RecoveryDay) string {
	return recoveryTrend(recovery)
}
```

- [ ] **Step 4: Run the test, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/
```
Expected output contains: `ok  	help-my-run/backend/internal/metrics`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/metrics/metrics.go backend/internal/metrics/metrics_test.go && git commit -m "feat(metrics): export RecoveryTrend wrapper for readiness reuse

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Readiness types, constants, and JSON tags

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/readiness/readiness.go` (package decl, `Color` enum, `ReadinessDrivers`, `Readiness`, threshold constants — `Assess` body added in Task 7)
- Test: `/home/jake/project/help-my-run/backend/internal/readiness/readiness_test.go` (create; `TestReadinessDriversJSONTags`, `TestColorConstants`, `TestThresholdConstants`, `TestReadinessJSONTags`)

Lock the wire/JSON contract (snake_case `ReadinessDrivers`, lowercase color strings) and the exported thresholds before any logic.

- [ ] **Step 1: Write failing tests for types + constants.** Create `/home/jake/project/help-my-run/backend/internal/readiness/readiness_test.go`:
```go
package readiness

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestColorConstants(t *testing.T) {
	if ColorGreen != "green" || ColorAmber != "amber" || ColorRed != "red" {
		t.Errorf("colors = (%q,%q,%q), want (green,amber,red)", ColorGreen, ColorAmber, ColorRed)
	}
}

func TestThresholdConstants(t *testing.T) {
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"RedSleepHours", RedSleepHours, 5.0},
		{"AmberSleepHours", AmberSleepHours, 6.5},
		{"RedSleepScore", float64(RedSleepScore), 50},
		{"AmberSleepScore", float64(AmberSleepScore), 65},
		{"RedHRVDropPct", RedHRVDropPct, -15.0},
		{"AmberHRVDropPct", AmberHRVDropPct, -7.0},
		{"RedRHRRiseBpm", RedRHRRiseBpm, 7.0},
		{"AmberRHRRiseBpm", AmberRHRRiseBpm, 4.0},
		{"RedBodyBattery", float64(RedBodyBattery), 30},
		{"AmberBodyBattery", float64(AmberBodyBattery), 50},
		{"BaselineWindowDays", float64(BaselineWindowDays), 14},
		{"MinBaselineDays", float64(MinBaselineDays), 3},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestReadinessDriversJSONTags(t *testing.T) {
	f := 6.1
	i := int64(48)
	d := ReadinessDrivers{
		Date:            "2026-06-20",
		SleepHours:      &f,
		SleepScore:      &i,
		HRVLastNightMs:  &i,
		HRVBaselineMs:   &f,
		HRVDeltaPct:     &f,
		RHRLastNight:    &i,
		RHRBaseline:     &f,
		RHRDeltaBpm:     &f,
		BodyBatteryHigh: &i,
		RecoveryTrend:   "declining",
		DataComplete:    true,
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	got := string(b)
	wantKeys := []string{
		`"date":`, `"sleep_hours":`, `"sleep_score":`, `"hrv_last_night_ms":`,
		`"hrv_baseline_ms":`, `"hrv_delta_pct":`, `"rhr_last_night":`, `"rhr_baseline":`,
		`"rhr_delta_bpm":`, `"body_battery_high":`, `"recovery_trend":"declining"`,
		`"data_complete":true`,
	}
	for _, k := range wantKeys {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}

	empty := ReadinessDrivers{Date: "2026-06-20", RecoveryTrend: "stable"}
	eb, _ := json.Marshal(empty)
	if !strings.Contains(string(eb), `"sleep_hours":null`) {
		t.Errorf("nil SleepHours = %s, want sleep_hours:null", eb)
	}
}

func TestReadinessJSONTags(t *testing.T) {
	r := Readiness{
		Color:   ColorAmber,
		Drivers: ReadinessDrivers{Date: "2026-06-20", RecoveryTrend: "stable"},
		Reasons: []string{"HRV -17.8% vs baseline"},
	}
	b, _ := json.Marshal(r)
	got := string(b)
	for _, k := range []string{`"color":"amber"`, `"drivers":`, `"reasons":["HRV -17.8% vs baseline"]`} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}
```

- [ ] **Step 2: Run the tests, expect FAIL (undefined identifiers).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/
```
Expected output contains: `undefined: ColorGreen` and `FAIL	help-my-run/backend/internal/readiness [build failed]`.

- [ ] **Step 3: Create the types + constants (no Assess yet).** Create `/home/jake/project/help-my-run/backend/internal/readiness/readiness.go`:
```go
// Package readiness computes a deterministic daily readiness gate (GREEN/AMBER/RED)
// from M0 garmin_* recovery rows plus the M1 recovery-trend signal. Like internal/metrics
// it is PURE: callers pass plain slices and an explicit `now`; there is no DB and no clock,
// so the engine is fully table-test friendly. JSON tags are snake_case (repo convention).
package readiness

import (
	"time"

	"help-my-run/backend/internal/store"
)

// Color is the readiness gate result.
type Color string

const (
	ColorGreen Color = "green"
	ColorAmber Color = "amber"
	ColorRed   Color = "red"
)

// Baseline + classification thresholds. Exported so tests reference the same numbers
// the engine uses (no magic-number drift between impl and tests).
const (
	BaselineWindowDays = 14 // baseline = mean over the prior up-to-14 days (excluding last night)
	MinBaselineDays    = 3  // fewer than this with the metric -> baseline unavailable

	RedSleepHours   = 5.0 // < 5.0h slept -> RED contribution
	AmberSleepHours = 6.5 // < 6.5h slept -> AMBER contribution
	RedSleepScore   = 50  // sleep score < 50 -> RED
	AmberSleepScore = 65  // sleep score < 65 -> AMBER

	RedHRVDropPct   = -15.0 // HRV delta <= -15% vs baseline -> RED
	AmberHRVDropPct = -7.0  // HRV delta <= -7% vs baseline -> AMBER

	RedRHRRiseBpm   = 7.0 // RHR delta >= +7 bpm vs baseline -> RED
	AmberRHRRiseBpm = 4.0 // RHR delta >= +4 bpm vs baseline -> AMBER

	RedBodyBattery   = 30 // overnight BodyBattery high < 30 -> RED
	AmberBodyBattery = 50 // overnight BodyBattery high < 50 -> AMBER
)

// ReadinessDrivers are the raw numbers that decided the color. Pointers are nil
// when the underlying Garmin metric (or its baseline) is missing for last night.
type ReadinessDrivers struct {
	Date            string   `json:"date"`              // local date assessed (YYYY-MM-DD)
	SleepHours      *float64 `json:"sleep_hours"`       // last night sleep duration in hours
	SleepScore      *int64   `json:"sleep_score"`       // Garmin sleep score 0-100
	HRVLastNightMs  *int64   `json:"hrv_last_night_ms"` // overnight avg HRV
	HRVBaselineMs   *float64 `json:"hrv_baseline_ms"`   // mean HRV over baseline window
	HRVDeltaPct     *float64 `json:"hrv_delta_pct"`     // (last - baseline)/baseline * 100
	RHRLastNight    *int64   `json:"rhr_last_night"`    // last night resting HR
	RHRBaseline     *float64 `json:"rhr_baseline"`      // mean RHR over baseline window
	RHRDeltaBpm     *float64 `json:"rhr_delta_bpm"`     // last - baseline (positive = elevated)
	BodyBatteryHigh *int64   `json:"body_battery_high"` // last night BodyBattery.High (overnight peak)
	RecoveryTrend   string   `json:"recovery_trend"`    // "improving"|"stable"|"declining"
	DataComplete    bool     `json:"data_complete"`     // false if last-night data missing -> conservative AMBER
}

// Readiness is the readiness engine's output.
type Readiness struct {
	Color   Color            `json:"color"`
	Drivers ReadinessDrivers `json:"drivers"`
	Reasons []string         `json:"reasons"` // human-readable bullets, e.g. "HRV -18% vs baseline"
}

// Assess computes readiness from recovery rows (most-recent-first, as ListRecovery
// returns) for the given local date. `now` is unused for arithmetic but kept for
// signature symmetry with metrics and future trend windows.
//
// Implemented in a later step; this stub keeps the package compiling for the
// types/constants tests.
func Assess(recovery []store.RecoveryDay, now time.Time) Readiness {
	return Readiness{}
}
```

- [ ] **Step 4: Run the tests, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/ -run 'TestColorConstants|TestThresholdConstants|TestReadinessDriversJSONTags|TestReadinessJSONTags'
```
Expected output contains: `ok  	help-my-run/backend/internal/readiness`.

- [ ] **Step 5: Commit.** Command:
```
cd /home/jake/project/help-my-run && git add backend/internal/readiness/readiness.go backend/internal/readiness/readiness_test.go && git commit -m "feat(readiness): add Color, ReadinessDrivers, Readiness types and thresholds

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Driver-extraction helpers (last-night metrics + baselines + deltas)

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/readiness/readiness.go` (add private helpers: `sleepHours`, `meanI64`, `baseline`, `pctDelta`, `bpmDelta`, `ptrF`, `ptrI`)
- Test: `/home/jake/project/help-my-run/backend/internal/readiness/readiness_test.go` (add `TestSleepHours`, `TestBaseline`, `TestPctDelta`, `TestBpmDelta`)

Build the deterministic arithmetic (duration-seconds → hours, mean over baseline window with the `MinBaselineDays` floor, %-delta and bpm-delta) as small pure helpers.

- [ ] **Step 1: Write failing tests for the helpers.** Append to `/home/jake/project/help-my-run/backend/internal/readiness/readiness_test.go` (and add `"math"` to the existing import block at the top of the file):
```go
func i64p(v int64) *int64 { return &v }

func TestSleepHours(t *testing.T) {
	if got := sleepHours(nil); got != nil {
		t.Errorf("sleepHours(nil) = %v, want nil", got)
	}
	if got := sleepHours(&store.SleepFields{}); got != nil {
		t.Errorf("sleepHours(empty) = %v, want nil", got)
	}
	got := sleepHours(&store.SleepFields{DurationS: i64p(27000)})
	if got == nil || math.Abs(*got-7.5) > 1e-9 {
		t.Errorf("sleepHours(27000) = %v, want 7.5", got)
	}
}

func TestBaseline(t *testing.T) {
	if got, ok := baseline([]*int64{i64p(50), i64p(52)}); ok || got != 0 {
		t.Errorf("baseline(2 vals) = (%v,%v), want (0,false)", got, ok)
	}
	got, ok := baseline([]*int64{i64p(48), i64p(50), i64p(52)})
	if !ok || math.Abs(got-50.0) > 1e-9 {
		t.Errorf("baseline(3 vals) = (%v,%v), want (50,true)", got, ok)
	}
	got, ok = baseline([]*int64{i64p(48), nil, i64p(50), i64p(52), nil})
	if !ok || math.Abs(got-50.0) > 1e-9 {
		t.Errorf("baseline(with nils) = (%v,%v), want (50,true)", got, ok)
	}
	if _, ok := baseline([]*int64{nil, nil, nil}); ok {
		t.Errorf("baseline(all nil) ok = true, want false")
	}
}

func TestPctDelta(t *testing.T) {
	got := pctDelta(48, 58.4)
	if math.Abs(got-(-17.808219178082192)) > 1e-9 {
		t.Errorf("pctDelta(48,58.4) = %v, want ~-17.808", got)
	}
	if got := pctDelta(48, 0); got != 0 {
		t.Errorf("pctDelta(48,0) = %v, want 0", got)
	}
}

func TestBpmDelta(t *testing.T) {
	if got := bpmDelta(54, 50.2); math.Abs(got-3.8) > 1e-9 {
		t.Errorf("bpmDelta(54,50.2) = %v, want 3.8", got)
	}
}
```

- [ ] **Step 2: Run the tests, expect FAIL (undefined helpers).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/ -run 'TestSleepHours|TestBaseline|TestPctDelta|TestBpmDelta'
```
Expected output contains: `undefined: sleepHours` and `FAIL	help-my-run/backend/internal/readiness [build failed]`.

- [ ] **Step 3: Add the helpers.** Append to `/home/jake/project/help-my-run/backend/internal/readiness/readiness.go` (and add `"math"` to the import block):
```go
// ptrF / ptrI are convenience constructors for the *float64 / *int64 driver fields.
func ptrF(v float64) *float64 { return &v }
func ptrI(v int64) *int64     { return &v }

// sleepHours converts a sleep record's duration (seconds) to hours; nil if absent.
func sleepHours(s *store.SleepFields) *float64 {
	if s == nil || s.DurationS == nil {
		return nil
	}
	h := float64(*s.DurationS) / 3600.0
	return &h
}

// meanI64 averages the non-nil values; ok=false if none present. count is the
// number of non-nil values averaged.
func meanI64(vals []*int64) (mean float64, count int) {
	var sum float64
	for _, v := range vals {
		if v != nil {
			sum += float64(*v)
			count++
		}
	}
	if count == 0 {
		return 0, 0
	}
	return sum / float64(count), count
}

// baseline returns the mean of the available values over the baseline window,
// ok=false when fewer than MinBaselineDays non-nil values are present (baseline
// unavailable -> that signal contributes no delta and forces DataComplete=false).
func baseline(vals []*int64) (mean float64, ok bool) {
	m, count := meanI64(vals)
	if count < MinBaselineDays {
		return 0, false
	}
	return m, true
}

// pctDelta is (last-baseline)/baseline*100; 0 when baseline is 0.
func pctDelta(last int64, base float64) float64 {
	if base == 0 {
		return 0
	}
	return (float64(last) - base) / base * 100.0
}

// bpmDelta is last-baseline (positive = elevated RHR).
func bpmDelta(last int64, base float64) float64 {
	return float64(last) - base
}
```

- [ ] **Step 4: Run the tests, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/ -run 'TestSleepHours|TestBaseline|TestPctDelta|TestBpmDelta'
```
Expected output contains: `ok  	help-my-run/backend/internal/readiness`.

- [ ] **Step 5: Commit.** Command:
```
cd /home/jake/project/help-my-run && git add backend/internal/readiness/readiness.go backend/internal/readiness/readiness_test.go && git commit -m "feat(readiness): add driver-extraction helpers (sleep hours, baseline, deltas)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: `Assess` — driver assembly + GREEN/AMBER/RED classification

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/readiness/readiness.go` (replace the `Assess` stub; add `signalLevel`, `worse`, `pick`, `metricsTrend`; baseline-window slicing, aggregation; collect `Reasons`)
- Test: `/home/jake/project/help-my-run/backend/internal/readiness/readiness_test.go` (add `TestAssess` table-driven + `TestAssessEmpty`, `TestAssessMissingBaselineForcesAmber`, `TestAssessDriverNumbers`)

Implement the core deterministic gate per the §2 contract: last-night drivers from `recovery[0]`, baselines over `recovery[1:1+BaselineWindowDays]`, per-signal color via thresholds, worst-wins-with-confirmation aggregation, trend modifier, and `Reasons`.

- [ ] **Step 1: Write failing table-driven test for `Assess`.** Append to `/home/jake/project/help-my-run/backend/internal/readiness/readiness_test.go` (and add `"time"` to the import block):
```go
func mkDay(date string, durationS, sleepScore, hrvMs, rhr, bbHigh *int64) store.RecoveryDay {
	rd := store.RecoveryDay{Date: date}
	if durationS != nil || sleepScore != nil {
		rd.Sleep = &store.SleepFields{DurationS: durationS, Score: sleepScore}
	}
	if hrvMs != nil {
		rd.HRV = &store.HrvFields{LastNightAvgMs: hrvMs}
	}
	if bbHigh != nil {
		rd.BodyBattery = &store.BodyBatteryFields{High: bbHigh}
	}
	if rhr != nil {
		rd.RHR = &store.RhrFields{RestingHR: rhr}
	}
	return rd
}

func baselineRows(n int, hrvMs, rhr int64) []store.RecoveryDay {
	out := make([]store.RecoveryDay, 0, n)
	for i := 0; i < n; i++ {
		date := "2026-06-" + twoDigit(19-i)
		out = append(out, mkDay(date, i64p(27000), i64p(85), i64p(hrvMs), i64p(rhr), i64p(80)))
	}
	return out
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func TestAssess(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")

	tests := []struct {
		name      string
		lastNight store.RecoveryDay
		baseHRV   int64
		baseRHR   int64
		wantColor Color
	}{
		{
			name:      "all nominal -> GREEN",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorGreen,
		},
		{
			name:      "HRV -17% (red) -> RED",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(48), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
		{
			name:      "HRV -8% (one amber) -> AMBER",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(53), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorAmber,
		},
		{
			name:      "two amber signals (HRV -8% + RHR +5) -> RED (confirmation)",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(53), i64p(55), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
		{
			name:      "short sleep 4.5h -> RED",
			lastNight: mkDay("2026-06-20", i64p(16200), i64p(85), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
		{
			name:      "sleep 6.0h (amber) only -> AMBER",
			lastNight: mkDay("2026-06-20", i64p(21600), i64p(85), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorAmber,
		},
		{
			name:      "body battery 25 -> RED",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(25)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
		{
			name:      "body battery 45 (amber) only -> AMBER",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(45)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorAmber,
		},
		{
			name:      "sleep score 60 (amber) only -> AMBER",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(60), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorAmber,
		},
		{
			name:      "sleep score 45 -> RED",
			lastNight: mkDay("2026-06-20", i64p(27000), i64p(45), i64p(58), i64p(50), i64p(80)),
			baseHRV:   58, baseRHR: 50, wantColor: ColorRed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := append([]store.RecoveryDay{tt.lastNight}, baselineRows(MinBaselineDays, tt.baseHRV, tt.baseRHR)...)
			r := Assess(rows, now)
			if r.Color != tt.wantColor {
				t.Errorf("Assess color = %q, want %q (drivers=%+v reasons=%v)", r.Color, tt.wantColor, r.Drivers, r.Reasons)
			}
			if r.Drivers.Date != "2026-06-20" {
				t.Errorf("Drivers.Date = %q, want 2026-06-20", r.Drivers.Date)
			}
			if r.Color != ColorGreen && len(r.Reasons) == 0 {
				t.Errorf("non-green color %q must have reasons", r.Color)
			}
		})
	}
}

func TestAssessEmpty(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")
	r := Assess(nil, now)
	if r.Color != ColorAmber {
		t.Errorf("Assess(nil) color = %q, want amber (conservative)", r.Color)
	}
	if r.Drivers.DataComplete {
		t.Errorf("Assess(nil) DataComplete = true, want false")
	}
}

func TestAssessMissingBaselineForcesAmber(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")
	rows := []store.RecoveryDay{
		mkDay("2026-06-20", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
		mkDay("2026-06-19", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
	}
	r := Assess(rows, now)
	if r.Drivers.DataComplete {
		t.Errorf("DataComplete = true with <MinBaselineDays prior days, want false")
	}
	if r.Color != ColorAmber {
		t.Errorf("color = %q, want amber when baseline missing", r.Color)
	}
	if r.Drivers.HRVDeltaPct != nil {
		t.Errorf("HRVDeltaPct = %v, want nil when baseline unavailable", *r.Drivers.HRVDeltaPct)
	}
}

func TestAssessDriverNumbers(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")
	rows := []store.RecoveryDay{
		mkDay("2026-06-20", i64p(21960), i64p(62), i64p(48), i64p(54), i64p(61)), // 6.1h
		mkDay("2026-06-19", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
		mkDay("2026-06-18", i64p(27000), i64p(85), i64p(59), i64p(50), i64p(80)),
		mkDay("2026-06-17", i64p(27000), i64p(85), i64p(58), i64p(51), i64p(80)),
	}
	r := Assess(rows, now)
	d := r.Drivers
	if d.SleepHours == nil || math.Abs(*d.SleepHours-6.1) > 1e-9 {
		t.Errorf("SleepHours = %v, want 6.1", d.SleepHours)
	}
	if d.SleepScore == nil || *d.SleepScore != 62 {
		t.Errorf("SleepScore = %v, want 62", d.SleepScore)
	}
	if d.HRVLastNightMs == nil || *d.HRVLastNightMs != 48 {
		t.Errorf("HRVLastNightMs = %v, want 48", d.HRVLastNightMs)
	}
	if d.HRVBaselineMs == nil || math.Abs(*d.HRVBaselineMs-58.333333333333336) > 1e-9 {
		t.Errorf("HRVBaselineMs = %v, want ~58.333", d.HRVBaselineMs)
	}
	if d.HRVDeltaPct == nil || math.Abs(*d.HRVDeltaPct-(-17.714285714285715)) > 1e-9 {
		t.Errorf("HRVDeltaPct = %v, want ~-17.714", d.HRVDeltaPct)
	}
	if d.RHRLastNight == nil || *d.RHRLastNight != 54 {
		t.Errorf("RHRLastNight = %v, want 54", d.RHRLastNight)
	}
	if d.RHRBaseline == nil || math.Abs(*d.RHRBaseline-50.333333333333336) > 1e-9 {
		t.Errorf("RHRBaseline = %v, want ~50.333", d.RHRBaseline)
	}
	if d.RHRDeltaBpm == nil || math.Abs(*d.RHRDeltaBpm-3.6666666666666643) > 1e-9 {
		t.Errorf("RHRDeltaBpm = %v, want ~3.667", d.RHRDeltaBpm)
	}
	if d.BodyBatteryHigh == nil || *d.BodyBatteryHigh != 61 {
		t.Errorf("BodyBatteryHigh = %v, want 61", d.BodyBatteryHigh)
	}
	if !d.DataComplete {
		t.Errorf("DataComplete = false, want true (last night + 3 baseline days present)")
	}
}

func mustNow(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}
```

- [ ] **Step 2: Run the tests, expect FAIL (stub returns empty Readiness).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/ -run 'TestAssess'
```
Expected output contains: failures like `Assess color = "", want "green"` and `FAIL	help-my-run/backend/internal/readiness`.

- [ ] **Step 3: Implement `Assess` (replace the stub) + classification helpers.** In `/home/jake/project/help-my-run/backend/internal/readiness/readiness.go`, replace the entire `Assess` stub with the implementation below, add the helper functions after it, and add `"fmt"` and `"help-my-run/backend/internal/metrics"` to the import block:
```go
// signalLevel is the per-signal severity used internally during aggregation.
type signalLevel int

const (
	levelGreen signalLevel = iota
	levelAmber
	levelRed
)

// worse returns the more severe of two levels.
func worse(a, b signalLevel) signalLevel {
	if b > a {
		return b
	}
	return a
}

// pick collects one *int64 metric across the given days (used to build a baseline
// window). days are the prior days (recovery[1:1+BaselineWindowDays]).
func pick(days []store.RecoveryDay, get func(store.RecoveryDay) *int64) []*int64 {
	out := make([]*int64, 0, len(days))
	for _, d := range days {
		out = append(out, get(d))
	}
	return out
}

// Assess computes readiness from recovery rows (most-recent-first, as ListRecovery
// returns) for the given local date. `now` is unused for arithmetic but kept for
// signature symmetry with metrics and future trend windows.
func Assess(recovery []store.RecoveryDay, now time.Time) Readiness {
	_ = now // reserved for future trend windows; arithmetic is row-driven.

	drivers := ReadinessDrivers{
		RecoveryTrend: metricsTrend(recovery),
		DataComplete:  true,
	}
	var reasons []string

	if len(recovery) == 0 {
		drivers.DataComplete = false
		return Readiness{
			Color:   ColorAmber,
			Drivers: drivers,
			Reasons: []string{"No recovery data for last night"},
		}
	}

	last := recovery[0]
	drivers.Date = last.Date

	var window []store.RecoveryDay
	if len(recovery) > 1 {
		end := 1 + BaselineWindowDays
		if end > len(recovery) {
			end = len(recovery)
		}
		window = recovery[1:end]
	}

	level := levelGreen
	amberCount := 0
	redCount := 0
	note := func(sig signalLevel, reason string) {
		switch sig {
		case levelAmber:
			amberCount++
			reasons = append(reasons, reason)
		case levelRed:
			redCount++
			reasons = append(reasons, reason)
		}
		level = worse(level, sig)
	}

	// --- Sleep hours ---
	if sh := sleepHours(last.Sleep); sh != nil {
		drivers.SleepHours = sh
		switch {
		case *sh < RedSleepHours:
			note(levelRed, fmt.Sprintf("Sleep %.1fh (<%.1fh)", *sh, RedSleepHours))
		case *sh < AmberSleepHours:
			note(levelAmber, fmt.Sprintf("Sleep %.1fh (<%.1fh)", *sh, AmberSleepHours))
		}
	} else {
		drivers.DataComplete = false
	}

	// --- Sleep score ---
	if last.Sleep != nil && last.Sleep.Score != nil {
		ss := *last.Sleep.Score
		drivers.SleepScore = ptrI(ss)
		switch {
		case ss < RedSleepScore:
			note(levelRed, fmt.Sprintf("Sleep score %d (<%d)", ss, RedSleepScore))
		case ss < AmberSleepScore:
			note(levelAmber, fmt.Sprintf("Sleep score %d (<%d)", ss, AmberSleepScore))
		}
	}

	// --- HRV vs baseline ---
	if last.HRV != nil && last.HRV.LastNightAvgMs != nil {
		hv := *last.HRV.LastNightAvgMs
		drivers.HRVLastNightMs = ptrI(hv)
		if base, ok := baseline(pick(window, func(d store.RecoveryDay) *int64 {
			if d.HRV == nil {
				return nil
			}
			return d.HRV.LastNightAvgMs
		})); ok {
			drivers.HRVBaselineMs = ptrF(base)
			delta := pctDelta(hv, base)
			drivers.HRVDeltaPct = ptrF(delta)
			switch {
			case delta <= RedHRVDropPct:
				note(levelRed, fmt.Sprintf("HRV %.1f%% vs baseline", delta))
			case delta <= AmberHRVDropPct:
				note(levelAmber, fmt.Sprintf("HRV %.1f%% vs baseline", delta))
			}
		} else {
			drivers.DataComplete = false
		}
	} else {
		drivers.DataComplete = false
	}

	// --- RHR vs baseline ---
	if last.RHR != nil && last.RHR.RestingHR != nil {
		rv := *last.RHR.RestingHR
		drivers.RHRLastNight = ptrI(rv)
		if base, ok := baseline(pick(window, func(d store.RecoveryDay) *int64 {
			if d.RHR == nil {
				return nil
			}
			return d.RHR.RestingHR
		})); ok {
			drivers.RHRBaseline = ptrF(base)
			delta := bpmDelta(rv, base)
			drivers.RHRDeltaBpm = ptrF(delta)
			switch {
			case delta >= RedRHRRiseBpm:
				note(levelRed, fmt.Sprintf("RHR +%.1f bpm vs baseline", delta))
			case delta >= AmberRHRRiseBpm:
				note(levelAmber, fmt.Sprintf("RHR +%.1f bpm vs baseline", delta))
			}
		} else {
			drivers.DataComplete = false
		}
	} else {
		drivers.DataComplete = false
	}

	// --- Body Battery overnight high ---
	if last.BodyBattery != nil && last.BodyBattery.High != nil {
		bb := *last.BodyBattery.High
		drivers.BodyBatteryHigh = ptrI(bb)
		switch {
		case bb < RedBodyBattery:
			note(levelRed, fmt.Sprintf("Body Battery high %d (<%d)", bb, RedBodyBattery))
		case bb < AmberBodyBattery:
			note(levelAmber, fmt.Sprintf("Body Battery high %d (<%d)", bb, AmberBodyBattery))
		}
	} else {
		drivers.DataComplete = false
	}

	// --- Recovery-trend modifier: only when NO direct signal already fired. ---
	// A declining trend adds one amber-weight ONLY when no per-signal amber/red has
	// already been recorded; otherwise a single direct amber would double-count
	// (direct amber + trend amber => 2 ambers => spurious RED). When a direct
	// signal already fired, the trend is informational (a reason) but not additive.
	if amberCount == 0 && redCount == 0 {
		switch drivers.RecoveryTrend {
		case "declining":
			amberCount++
			level = worse(level, levelAmber)
			reasons = append(reasons, "Recovery trend declining")
		case "improving":
			// no direct concerns + improving trend: nothing to cancel.
		}
	}

	// --- Aggregate (worst-wins with confirmation). ---
	var color Color
	switch {
	case redCount >= 1 || amberCount >= 2:
		color = ColorRed
	case amberCount == 1 || !drivers.DataComplete:
		color = ColorAmber
	default:
		color = ColorGreen
	}

	if !drivers.DataComplete && color == ColorAmber {
		reasons = append(reasons, "Incomplete last-night data — conservative")
	}

	return Readiness{Color: color, Drivers: drivers, Reasons: reasons}
}

// metricsTrend delegates to the exported metrics.RecoveryTrend so readiness reuses
// the identical M1 trend computation rather than re-implementing it.
func metricsTrend(recovery []store.RecoveryDay) string {
	return metrics.RecoveryTrend(recovery)
}
```

- [ ] **Step 4: Run the full readiness test suite, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/
```
Expected output contains: `ok  	help-my-run/backend/internal/readiness`.

- [ ] **Step 5: Run `go vet`, expect clean.** Command:
```
cd /home/jake/project/help-my-run/backend && go vet ./internal/readiness/
```
Expected output: no output (exit 0).

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/readiness/readiness.go backend/internal/readiness/readiness_test.go && git commit -m "feat(readiness): implement Assess GREEN/AMBER/RED gate with drivers and reasons

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: Deterministic fallback action rule (readiness → STAND/SOFTEN/MOVE/REST_DAY)

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/readiness/fallback.go` (`FallbackAction`, `FallbackSession`, `FallbackDecision`, `Fallback(...)`, `roundHalf`, `isEasyType`)
- Test: `/home/jake/project/help-my-run/backend/internal/readiness/fallback_test.go` (create; `TestFallback`, `TestRoundHalf`, `TestIsEasyType`, `TestFallbackActionConstants`)

Implement the §2 deterministic fallback table as a PURE function the agent/coach reuses when `claude -p` fails. It must NOT import `coach`/`llm` (avoids the cycle: `coach` imports `readiness`). It operates on a small `FallbackSession` (mirrors the `llm.PlanDay` fields the rule touches) + the easy-pace string, returning the action + adjusted session fields + rationale. `readiness.Fallback` is the SINGLE SOURCE OF TRUTH for the rule table (action selection, `roundHalf` capping, and the rationale strings). The coach's `AdjustToday` fallback branch (Task 13) does NOT re-implement the table; it converts `*llm.PlanDay ↔ FallbackSession`, delegates to `readiness.Fallback`, and converts the result back, so the shipped fallback path is exactly what the Task 8/9 readiness tests cover.

- [ ] **Step 1: Write failing table-driven test.** Create `/home/jake/project/help-my-run/backend/internal/readiness/fallback_test.go`:
```go
package readiness

import (
	"math"
	"testing"
)

func TestRoundHalf(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{4.0, 4.0}, {4.2, 4.0}, {4.25, 4.5}, {4.74, 4.5}, {4.75, 5.0},
		{6.0, 6.0}, {3.1, 3.0}, {0.0, 0.0}, {2.5, 2.5},
	}
	for _, c := range cases {
		if got := roundHalf(c.in); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("roundHalf(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsEasyType(t *testing.T) {
	cases := map[string]bool{
		"easy": true, "recovery": true, "rest": true,
		"tempo": false, "intervals": false, "long": false, "": false,
	}
	for typ, want := range cases {
		if got := isEasyType(typ); got != want {
			t.Errorf("isEasyType(%q) = %v, want %v", typ, got, want)
		}
	}
}

func TestFallback(t *testing.T) {
	const easyPace = "6:00/km"

	tempo := &FallbackSession{
		Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6,
		PaceTarget: "5:05/km", TimeNote: "~20:00 after CrossFit",
	}
	easy := &FallbackSession{
		Date: "2026-06-20", Dow: "Fri", RunType: "easy", DistanceKm: 8,
		PaceTarget: "6:00/km", TimeNote: "~20:00 after CrossFit",
	}

	tests := []struct {
		name        string
		color       Color
		session     *FallbackSession
		wantAction  string
		wantAdjNil  bool
		wantType    string
		wantDistKm  float64
		wantPace    string
		wantOptCNS  bool
	}{
		{
			name: "RED + quality run -> MOVE to easy recovery, capped 4km",
			color: ColorRed, session: tempo,
			wantAction: "MOVE", wantType: "recovery", wantDistKm: 4, wantPace: easyPace, wantOptCNS: true,
		},
		{
			name: "RED + already easy -> SOFTEN to half, easy pace",
			color: ColorRed, session: easy,
			wantAction: "SOFTEN", wantType: "easy", wantDistKm: 4, wantPace: easyPace, wantOptCNS: true,
		},
		{
			name: "RED + no run -> REST_DAY",
			color: ColorRed, session: nil,
			wantAction: "REST_DAY", wantAdjNil: true,
		},
		{
			name: "AMBER + quality -> SOFTEN to 75%, easy pace",
			color: ColorAmber, session: tempo,
			wantAction: "SOFTEN", wantType: "tempo", wantDistKm: 4.5, wantPace: easyPace, wantOptCNS: false,
		},
		{
			name: "AMBER + easy -> STAND unchanged",
			color: ColorAmber, session: easy,
			wantAction: "STAND", wantType: "easy", wantDistKm: 8, wantPace: "6:00/km", wantOptCNS: false,
		},
		{
			name: "AMBER + no run -> REST_DAY",
			color: ColorAmber, session: nil,
			wantAction: "REST_DAY", wantAdjNil: true,
		},
		{
			name: "GREEN + quality -> STAND unchanged",
			color: ColorGreen, session: tempo,
			wantAction: "STAND", wantType: "tempo", wantDistKm: 6, wantPace: "5:05/km", wantOptCNS: false,
		},
		{
			name: "GREEN + no run -> REST_DAY",
			color: ColorGreen, session: nil,
			wantAction: "REST_DAY", wantAdjNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := Fallback(tt.color, tt.session, easyPace)
			if string(dec.Action) != tt.wantAction {
				t.Fatalf("Action = %q, want %q", dec.Action, tt.wantAction)
			}
			if dec.Rationale == "" {
				t.Errorf("Rationale empty, want non-empty")
			}
			if tt.wantAdjNil {
				if dec.Adjusted != nil {
					t.Errorf("Adjusted = %+v, want nil", dec.Adjusted)
				}
				return
			}
			if dec.Adjusted == nil {
				t.Fatalf("Adjusted = nil, want session")
			}
			a := dec.Adjusted
			if a.RunType != tt.wantType {
				t.Errorf("RunType = %q, want %q", a.RunType, tt.wantType)
			}
			if math.Abs(a.DistanceKm-tt.wantDistKm) > 1e-9 {
				t.Errorf("DistanceKm = %v, want %v", a.DistanceKm, tt.wantDistKm)
			}
			if a.PaceTarget != tt.wantPace {
				t.Errorf("PaceTarget = %q, want %q", a.PaceTarget, tt.wantPace)
			}
			if a.OptionalIfCNS != tt.wantOptCNS {
				t.Errorf("OptionalIfCNS = %v, want %v", a.OptionalIfCNS, tt.wantOptCNS)
			}
			if tt.wantAction == "STAND" {
				if a.Date != tt.session.Date || a.Dow != tt.session.Dow || a.TimeNote != tt.session.TimeNote {
					t.Errorf("STAND mutated identity fields: %+v", a)
				}
			}
		})
	}
}

func TestFallbackActionConstants(t *testing.T) {
	if FbStand != "STAND" || FbSoften != "SOFTEN" || FbMove != "MOVE" || FbRestDay != "REST_DAY" {
		t.Errorf("action constants drifted: %q %q %q %q", FbStand, FbSoften, FbMove, FbRestDay)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL (no fallback.go).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/ -run 'TestFallback|TestRoundHalf|TestIsEasyType'
```
Expected output contains: `undefined: roundHalf` / `undefined: Fallback` and `FAIL	help-my-run/backend/internal/readiness [build failed]`.

- [ ] **Step 3: Implement the fallback rule.** Create `/home/jake/project/help-my-run/backend/internal/readiness/fallback.go`:
```go
package readiness

import "math"

// FallbackAction is the deterministic action string (matches llm.DailyAction values
// and the daily_decisions CHECK constraint: STAND|SOFTEN|MOVE|REST_DAY).
type FallbackAction string

const (
	FbStand   FallbackAction = "STAND"
	FbSoften  FallbackAction = "SOFTEN"
	FbMove    FallbackAction = "MOVE"
	FbRestDay FallbackAction = "REST_DAY"
)

// FallbackSession mirrors the llm.PlanDay fields the fallback rule reads/writes.
// readiness stays free of an llm import; the coach/agent converts *llm.PlanDay
// to/from this struct at the call site.
type FallbackSession struct {
	Date          string
	Dow           string
	RunType       string
	DistanceKm    float64
	PaceTarget    string
	TimeNote      string
	OptionalIfCNS bool
	Rationale     string
}

// FallbackDecision is the deterministic-rule output. Adjusted is nil for REST_DAY.
type FallbackDecision struct {
	Action    FallbackAction
	Adjusted  *FallbackSession
	Rationale string
}

// easyTypes are the run types treated as already-easy (no quality to remove).
var easyTypes = map[string]bool{"easy": true, "recovery": true, "rest": true}

// isEasyType reports whether a run type is already easy/recovery/rest.
func isEasyType(runType string) bool { return easyTypes[runType] }

// roundHalf rounds to the nearest 0.5 (e.g. 4.74 -> 4.5, 4.75 -> 5.0).
func roundHalf(x float64) float64 { return math.Round(x*2) / 2 }

// Fallback applies the deterministic readiness->action rule (M2 contract §2) used
// when claude -p is unavailable. `easyPace` is the athlete's computed easy pace
// (coach.Fitness(ctx).EasyPace); it eases tempo/interval targets on AMBER/RED.
// Pure function: no DB, no clock, no llm import.
func Fallback(color Color, session *FallbackSession, easyPace string) FallbackDecision {
	if session == nil {
		var r string
		switch color {
		case ColorGreen:
			r = "Rest day as planned; you're well recovered."
		default:
			r = "Rest day — readiness low, stay recovered."
		}
		return FallbackDecision{Action: FbRestDay, Adjusted: nil, Rationale: r}
	}

	switch color {
	case ColorRed:
		if isEasyType(session.RunType) {
			adj := *session
			adj.DistanceKm = roundHalf(session.DistanceKm * 0.5)
			adj.PaceTarget = easyPace
			adj.OptionalIfCNS = true
			adj.Rationale = "Low readiness — distance halved, kept easy."
			return FallbackDecision{Action: FbSoften, Adjusted: &adj, Rationale: adj.Rationale}
		}
		dist := session.DistanceKm
		if dist > 4 {
			dist = 4
		}
		adj := FallbackSession{
			Date:          session.Date,
			Dow:           session.Dow,
			RunType:       "recovery",
			DistanceKm:    roundHalf(dist),
			PaceTarget:    easyPace,
			TimeNote:      session.TimeNote,
			OptionalIfCNS: true,
			Rationale:     "Low readiness — moved to easy recovery.",
		}
		return FallbackDecision{Action: FbMove, Adjusted: &adj, Rationale: adj.Rationale}

	case ColorAmber:
		if isEasyType(session.RunType) {
			adj := *session
			adj.Rationale = "Reduced readiness — easy run stands."
			return FallbackDecision{Action: FbStand, Adjusted: &adj, Rationale: adj.Rationale}
		}
		adj := *session
		adj.DistanceKm = roundHalf(session.DistanceKm * 0.75)
		adj.PaceTarget = easyPace
		adj.Rationale = "Reduced readiness — trimmed volume/intensity."
		return FallbackDecision{Action: FbSoften, Adjusted: &adj, Rationale: adj.Rationale}

	default: // ColorGreen
		adj := *session
		if adj.Rationale == "" {
			adj.Rationale = "Well recovered — session stands as planned."
		}
		return FallbackDecision{Action: FbStand, Adjusted: &adj, Rationale: "Well recovered — session stands as planned."}
	}
}
```

- [ ] **Step 4: Run the test, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/ -run 'TestFallback|TestRoundHalf|TestIsEasyType'
```
Expected output contains: `ok  	help-my-run/backend/internal/readiness`.

- [ ] **Step 5: Run the full readiness suite + vet, expect PASS / clean.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/ && go vet ./internal/readiness/
```
Expected output contains: `ok  	help-my-run/backend/internal/readiness` and no vet output.

- [ ] **Step 6: Commit.** Command:
```
git add backend/internal/readiness/fallback.go backend/internal/readiness/fallback_test.go && git commit -m "feat(readiness): add deterministic fallback action rule (STAND/SOFTEN/MOVE/REST_DAY)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Readiness engine integration smoke test + whole-package green gate

**Files:**
- Test: `/home/jake/project/help-my-run/backend/internal/readiness/readiness_test.go` (add `TestAssessThenFallbackEndToEnd`)

Prove the two halves compose deterministically — a realistic AMBER fixture flows from `Assess` into `Fallback` and yields the expected SOFTEN on a quality session, matching the `GET /api/today` example. No new production code.

- [ ] **Step 1: Write the end-to-end composition test.** Append to `/home/jake/project/help-my-run/backend/internal/readiness/readiness_test.go`:
```go
func TestAssessThenFallbackEndToEnd(t *testing.T) {
	now := mustNow(t, "2026-06-20T05:30:00Z")
	rows := []store.RecoveryDay{
		mkDay("2026-06-20", i64p(21960), i64p(62), i64p(53), i64p(50), i64p(61)), // 6.1h, score 62, hrv 53
		mkDay("2026-06-19", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
		mkDay("2026-06-18", i64p(27000), i64p(85), i64p(59), i64p(50), i64p(80)),
		mkDay("2026-06-17", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
	}
	r := Assess(rows, now)

	// Single-amber fixture lands deterministically on AMBER.
	single := []store.RecoveryDay{
		mkDay("2026-06-20", i64p(27000), i64p(62), i64p(58), i64p(50), i64p(80)), // only sleep score 62 amber
		mkDay("2026-06-19", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
		mkDay("2026-06-18", i64p(27000), i64p(85), i64p(59), i64p(50), i64p(80)),
		mkDay("2026-06-17", i64p(27000), i64p(85), i64p(58), i64p(50), i64p(80)),
	}
	ra := Assess(single, now)
	if ra.Color != ColorAmber {
		t.Fatalf("single-amber fixture color = %q, want amber", ra.Color)
	}

	session := &FallbackSession{
		Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6,
		PaceTarget: "5:05/km", TimeNote: "~20:00 after CrossFit",
	}
	dec := Fallback(ra.Color, session, "6:00/km")
	if dec.Action != FbSoften {
		t.Errorf("Action = %q, want SOFTEN", dec.Action)
	}
	if dec.Adjusted == nil || dec.Adjusted.DistanceKm != 4.5 || dec.Adjusted.PaceTarget != "6:00/km" {
		t.Errorf("Adjusted = %+v, want 4.5km @ 6:00/km", dec.Adjusted)
	}
	if r.Color == ColorRed {
		mv := Fallback(r.Color, session, "6:00/km")
		if mv.Action != FbMove || mv.Adjusted == nil || mv.Adjusted.RunType != "recovery" {
			t.Errorf("RED fallback = %+v, want MOVE to recovery", mv)
		}
	}
}
```

- [ ] **Step 2: Run the new test, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/ -run TestAssessThenFallbackEndToEnd
```
Expected output contains: `ok  	help-my-run/backend/internal/readiness`.

- [ ] **Step 3: Run the entire readiness package verbose.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/readiness/ -v
```
Expected output contains: `--- PASS: TestAssess`, `--- PASS: TestFallback`, `--- PASS: TestAssessThenFallbackEndToEnd`, and final `ok  	help-my-run/backend/internal/readiness`.

- [ ] **Step 4: Confirm the backend still builds + metrics still pass.** Command:
```
cd /home/jake/project/help-my-run/backend && go build ./... && go test ./internal/metrics/ ./internal/readiness/
```
Expected output contains: `ok  	help-my-run/backend/internal/metrics` and `ok  	help-my-run/backend/internal/readiness`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/readiness/readiness_test.go && git commit -m "test(readiness): add Assess->Fallback end-to-end composition test

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Push client (Expo Push HTTP API)

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/push/push.go`
- Test: `/home/jake/project/help-my-run/backend/internal/push/push_test.go`

- [ ] **Step 1: Write the failing push tests.** Create `/home/jake/project/help-my-run/backend/internal/push/push_test.go`:
```go
package push_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"help-my-run/backend/internal/push"
)

func TestSendOK(t *testing.T) {
	var gotBody push.Message
	var gotPath, gotCT, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"status":"ok","id":"abc-123"}]}`)
	}))
	defer srv.Close()

	c := &push.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := c.Send(context.Background(), push.Message{
		To: "ExponentPushToken[x]", Title: "Today: AMBER", Body: "Trimmed tempo.",
		Data: map[string]interface{}{"date": "2026-06-20", "action": "SOFTEN"},
		Sound: "default", Priority: "high", ChannelID: "default",
	})
	if err != nil {
		t.Fatalf("Send error = %v", err)
	}
	if gotPath != "/--/api/v2/push/send" {
		t.Errorf("path = %q, want /--/api/v2/push/send", gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
	if gotBody.To != "ExponentPushToken[x]" || gotBody.Title != "Today: AMBER" || gotBody.Body != "Trimmed tempo." {
		t.Errorf("decoded body = %+v", gotBody)
	}
	if gotBody.Data["action"] != "SOFTEN" {
		t.Errorf("Data = %+v, want action=SOFTEN", gotBody.Data)
	}
}

func TestSendDeviceNotRegistered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"status":"error","message":"not registered","details":{"error":"DeviceNotRegistered"}}]}`)
	}))
	defer srv.Close()

	c := &push.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := c.Send(context.Background(), push.Message{To: "ExponentPushToken[dead]"})
	if !errors.Is(err, push.ErrDeviceNotRegistered) {
		t.Fatalf("Send err = %v, want ErrDeviceNotRegistered", err)
	}
}

func TestSendOtherError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"status":"error","message":"too big","details":{"error":"MessageTooBig"}}]}`)
	}))
	defer srv.Close()

	c := &push.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	err := c.Send(context.Background(), push.Message{To: "ExponentPushToken[big]"})
	if err == nil {
		t.Fatal("Send err = nil, want non-nil for MessageTooBig")
	}
	if errors.Is(err, push.ErrDeviceNotRegistered) {
		t.Errorf("MessageTooBig must not be ErrDeviceNotRegistered, got %v", err)
	}
}

func TestSendEmptyData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[],"errors":[{"code":"PUSH_TOO_MANY","message":"bad"}]}`)
	}))
	defer srv.Close()

	c := &push.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	if err := c.Send(context.Background(), push.Message{To: "ExponentPushToken[x]"}); err == nil {
		t.Fatal("Send err = nil, want non-nil for empty data")
	}
}

func TestNewClientDefaultsBaseURL(t *testing.T) {
	c := push.NewClient("")
	if c.BaseURL != "https://exp.host" {
		t.Errorf("BaseURL = %q, want https://exp.host", c.BaseURL)
	}
	if c.HTTPClient == nil {
		t.Error("HTTPClient = nil, want non-nil")
	}
	c2 := push.NewClient("http://localhost:9")
	if c2.BaseURL != "http://localhost:9" {
		t.Errorf("BaseURL = %q, want override preserved", c2.BaseURL)
	}
}

var _ push.Sender = (*push.Client)(nil)
```

- [ ] **Step 2: Run the tests, expect FAIL (no package).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/push/
```
Expected: build failure — the `push` package does not exist yet.

- [ ] **Step 3: Write the push client.** Create `/home/jake/project/help-my-run/backend/internal/push/push.go`:
```go
// Package push sends notifications via the Expo Push HTTP API (v2). The base URL
// is injectable so tests drive it against httptest with no real network.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

const (
	defaultExpoBaseURL = "https://exp.host"
	sendPath           = "/--/api/v2/push/send"
)

// Message is one Expo Push API message object.
type Message struct {
	To        string                 `json:"to"`                  // "ExponentPushToken[...]"
	Title     string                 `json:"title,omitempty"`
	Body      string                 `json:"body,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Sound     string                 `json:"sound,omitempty"`     // "default"
	Priority  string                 `json:"priority,omitempty"`  // "high"
	ChannelID string                 `json:"channelId,omitempty"` // "default"
}

// Sender is the injectable push transport (faked in agent tests).
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Client is the production Sender (HTTP to the Expo Push API).
type Client struct {
	BaseURL    string       // injectable: prod "https://exp.host", test = httptest URL
	HTTPClient *http.Client
}

// NewClient builds a Client. An empty baseURL falls back to the Expo prod host.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultExpoBaseURL
	}
	return &Client{BaseURL: baseURL, HTTPClient: http.DefaultClient}
}

// ErrDeviceNotRegistered signals the caller to delete the token from device_tokens.
var ErrDeviceNotRegistered = errors.New("push: device not registered")

type ticketDetails struct {
	Error string `json:"error"` // e.g. "DeviceNotRegistered"
}
type ticket struct {
	Status  string        `json:"status"` // "ok"|"error"
	ID      string        `json:"id"`
	Message string        `json:"message"`
	Details ticketDetails `json:"details"`
}
type sendResponse struct {
	Data   []ticket `json:"data"`
	Errors []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"errors"`
}

// Send POSTs one message to the Expo Push API and inspects the ticket. A
// DeviceNotRegistered ticket returns ErrDeviceNotRegistered; any other error
// ticket / request-level error returns a descriptive error.
func (c *Client) Send(ctx context.Context, msg Message) error {
	buf, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+sendPath, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var out sendResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("push: decode response: %w", err)
	}
	if len(out.Data) == 0 {
		return fmt.Errorf("push: empty data (errors=%v)", out.Errors)
	}
	t := out.Data[0]
	if t.Status == "error" {
		if t.Details.Error == "DeviceNotRegistered" {
			return ErrDeviceNotRegistered
		}
		return fmt.Errorf("push: expo error: %s (%s)", t.Message, t.Details.Error)
	}
	return nil
}
```

- [ ] **Step 4: Run the tests, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/push/
```
Expected output contains: `ok  	help-my-run/backend/internal/push`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/push/ && git commit -m "feat(push): Expo Push HTTP API client with DeviceNotRegistered detection

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: llm DailyAction enum + DailyDecisionParsed type

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/llm/types.go` (append `DailyAction` enum + `DailyDecisionParsed` after `PlanParsed`)
- Test: `/home/jake/project/help-my-run/backend/internal/llm/types_test.go` (create)

- [ ] **Step 1: Write failing test.** Create `/home/jake/project/help-my-run/backend/internal/llm/types_test.go`:
```go
package llm

import (
	"encoding/json"
	"testing"
)

func TestDailyDecisionParsedRoundTrip(t *testing.T) {
	in := `{"action":"SOFTEN","adjusted_session":{"date":"2026-06-20","dow":"Fri","run_type":"easy","distance_km":4.5,"pace_target":"6:00/km","time_note":"~20:00 after CrossFit","optional_if_cns":true,"rationale":"trimmed"},"rationale":"HRV down"}`
	var d DailyDecisionParsed
	if err := json.Unmarshal([]byte(in), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Action != ActionSoften {
		t.Errorf("action = %q, want SOFTEN", d.Action)
	}
	if d.AdjustedSession == nil || d.AdjustedSession.RunType != "easy" || d.AdjustedSession.DistanceKm != 4.5 {
		t.Errorf("adjusted = %+v", d.AdjustedSession)
	}
	if d.Rationale != "HRV down" {
		t.Errorf("rationale = %q", d.Rationale)
	}
}

func TestDailyDecisionParsedRestDayNullSession(t *testing.T) {
	var d DailyDecisionParsed
	if err := json.Unmarshal([]byte(`{"action":"REST_DAY","adjusted_session":null,"rationale":"rest"}`), &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if d.Action != ActionRestDay {
		t.Errorf("action = %q, want REST_DAY", d.Action)
	}
	if d.AdjustedSession != nil {
		t.Errorf("adjusted = %+v, want nil", d.AdjustedSession)
	}
}

func TestDailyActionConstants(t *testing.T) {
	if ActionStand != "STAND" || ActionSoften != "SOFTEN" || ActionMove != "MOVE" || ActionRestDay != "REST_DAY" {
		t.Errorf("action consts = %q/%q/%q/%q", ActionStand, ActionSoften, ActionMove, ActionRestDay)
	}
}
```

- [ ] **Step 2: Run it (expect FAIL).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/llm/ -run 'TestDailyDecisionParsed|TestDailyActionConstants'
```
Expected: compile failure `undefined: DailyDecisionParsed`, `undefined: ActionSoften`, etc.

- [ ] **Step 3: Minimal impl.** Append to `/home/jake/project/help-my-run/backend/internal/llm/types.go`:
```go

// DailyAction is the single-day adjust action enum (M2 daily coach).
type DailyAction string

const (
	ActionStand   DailyAction = "STAND"
	ActionSoften  DailyAction = "SOFTEN"
	ActionMove    DailyAction = "MOVE"
	ActionRestDay DailyAction = "REST_DAY"
)

// DailyDecisionParsed is the EXACT JSON claude -p emits for the daily adjust.
// AdjustedSession reuses PlanDay verbatim; it is nil for REST_DAY.
type DailyDecisionParsed struct {
	Action          DailyAction `json:"action"`
	AdjustedSession *PlanDay    `json:"adjusted_session"`
	Rationale       string      `json:"rationale"`
}
```

- [ ] **Step 4: Run it (expect PASS).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/llm/ -run 'TestDailyDecisionParsed|TestDailyActionConstants'
```
Expected: `ok  	help-my-run/backend/internal/llm`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/llm/types.go backend/internal/llm/types_test.go && git commit -m "feat(llm): add DailyAction enum and DailyDecisionParsed for M2 daily adjust

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 12: coach daily-adjust prompt + input builder

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/coach/prompts.go` (append `dailyAdjustPrompt` const + `buildDailyAdjustInput` helper)
- Modify: `/home/jake/project/help-my-run/backend/internal/coach/coach.go` (add `DailyAdjustInput` struct + `readiness` import)
- Test: `/home/jake/project/help-my-run/backend/internal/coach/prompts_test.go` (create)

Requires `readiness.Readiness` (Task 5) and `metrics.FitnessMetrics` (exists). Import path `help-my-run/backend/internal/readiness`.

- [ ] **Step 1: Write failing test.** Create `/home/jake/project/help-my-run/backend/internal/coach/prompts_test.go`:
```go
package coach

import (
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/readiness"
)

func TestDailyAdjustPromptShape(t *testing.T) {
	if !strings.Contains(dailyAdjustPrompt, "STAND") ||
		!strings.Contains(dailyAdjustPrompt, "SOFTEN") ||
		!strings.Contains(dailyAdjustPrompt, "MOVE") ||
		!strings.Contains(dailyAdjustPrompt, "REST_DAY") {
		t.Errorf("dailyAdjustPrompt missing an action token")
	}
	if !strings.Contains(dailyAdjustPrompt, "adjusted_session") || !strings.Contains(dailyAdjustPrompt, "rationale") {
		t.Errorf("dailyAdjustPrompt missing output keys")
	}
	if !strings.Contains(dailyAdjustPrompt, "single JSON object") {
		t.Errorf("dailyAdjustPrompt missing JSON-only instruction")
	}
}

func TestBuildDailyAdjustInput(t *testing.T) {
	rd := readiness.Readiness{
		Color:   readiness.ColorAmber,
		Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true},
		Reasons: []string{"HRV -17.8% vs baseline"},
	}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6}
	in := buildDailyAdjustInput("2026-06-20", rd, today,
		metrics.FitnessMetrics{EasyPace: "6:00/km"},
		ProfilePack{TargetWeeklyKm: 20}, nil, "build week")
	if in.Date != "2026-06-20" {
		t.Errorf("date = %q", in.Date)
	}
	if in.Readiness.Color != readiness.ColorAmber {
		t.Errorf("readiness color = %q", in.Readiness.Color)
	}
	if in.TodaySession == nil || in.TodaySession.RunType != "tempo" {
		t.Errorf("today session = %+v", in.TodaySession)
	}
	if in.Metrics.EasyPace != "6:00/km" {
		t.Errorf("metrics easy pace = %q", in.Metrics.EasyPace)
	}
	if in.Profile.TargetWeeklyKm != 20 {
		t.Errorf("profile target = %v", in.Profile.TargetWeeklyKm)
	}
	if in.WeekRationale != "build week" {
		t.Errorf("week rationale = %q", in.WeekRationale)
	}
}
```

- [ ] **Step 2: Run it (expect FAIL).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/coach/ -run 'TestDailyAdjustPromptShape|TestBuildDailyAdjustInput'
```
Expected: compile failure `undefined: dailyAdjustPrompt`, `undefined: buildDailyAdjustInput`, `undefined: DailyAdjustInput`.

- [ ] **Step 3a: Add `DailyAdjustInput` struct + readiness import to `coach.go`.** In `/home/jake/project/help-my-run/backend/internal/coach/coach.go`, add `"help-my-run/backend/internal/readiness"` to the import block, and append after the `ContextPack` struct:
```go

// DailyAdjustInput is the JSON piped to claude -p stdin for the daily adjust
// (M2 Coach Brain). snake_case wire JSON.
type DailyAdjustInput struct {
	Date          string                 `json:"date"`
	Readiness     readiness.Readiness    `json:"readiness"`
	TodaySession  *llm.PlanDay           `json:"today_session"`
	Metrics       metrics.FitnessMetrics `json:"metrics"`
	Profile       ProfilePack            `json:"profile"`
	CrossFitToday *llm.CrossFitDay       `json:"crossfit_today"`
	WeekRationale string                 `json:"week_rationale"`
}
```

- [ ] **Step 3b: Add prompt + builder to `prompts.go`.** Append to `/home/jake/project/help-my-run/backend/internal/coach/prompts.go`:
```go

// dailyAdjustPrompt is the M2 Coach Brain instruction block for the single-day
// adjust. The structured DailyAdjustInput is piped on stdin; the model returns
// ONLY a DailyDecisionParsed JSON object.
const dailyAdjustPrompt = `You are a CrossFit-aware running coach making a SINGLE-DAY adjustment. You receive a JSON context on stdin: today's date, a deterministic readiness assessment (color GREEN/AMBER/RED plus the driver numbers and reasons), today's already-planned running session (or null if no run is scheduled), computed fitness metrics, the athlete profile + constraints, and today's CrossFit day.

Decide ONE action for TODAY ONLY (do not touch other days):
- STAND  — keep today's session exactly as planned.
- SOFTEN — keep the session but reduce volume and/or intensity (lower distance_km, ease pace, or mark optional_if_cns).
- MOVE   — replace a quality session (tempo/intervals/long) with easy/recovery or rest.

Rules:
- Respect the readiness gate: GREEN -> default STAND; AMBER -> SOFTEN unless already easy; RED -> MOVE quality to easy/recovery, or shorten an easy run. Never increase load on AMBER/RED.
- If no run is scheduled today (today_session is null), return action "REST_DAY" with a null adjusted_session and a short readiness note.
- Keep the adjusted session faithful to the M1 plan's intent and the athlete's constraints.

Output ONLY a single JSON object (no prose, no markdown fences) of this EXACT shape:
{
  "action": "STAND|SOFTEN|MOVE|REST_DAY",
  "adjusted_session": {
    "date":"YYYY-MM-DD","dow":"Mon","run_type":"easy|tempo|recovery|long|rest|intervals",
    "distance_km":0,"pace_target":"5:45/km","time_note":"~20:00 after CrossFit",
    "optional_if_cns":false,"rationale":"one line"
  },
  "rationale": "one or two sentences: what changed and why, referencing the readiness drivers"
}
For action "STAND", adjusted_session MUST equal today's session. For "REST_DAY", adjusted_session MUST be null. distance_km is 0 and pace_target/time_note are "" for rest.`

// buildDailyAdjustInput assembles the stdin payload for the daily-adjust call.
func buildDailyAdjustInput(date string, rd readiness.Readiness, today *llm.PlanDay, fit metrics.FitnessMetrics, profile ProfilePack, crossfitToday *llm.CrossFitDay, weekRationale string) DailyAdjustInput {
	return DailyAdjustInput{
		Date:          date,
		Readiness:     rd,
		TodaySession:  today,
		Metrics:       fit,
		Profile:       profile,
		CrossFitToday: crossfitToday,
		WeekRationale: weekRationale,
	}
}
```

- [ ] **Step 4: Run it (expect PASS).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/coach/ -run 'TestDailyAdjustPromptShape|TestBuildDailyAdjustInput'
```
Expected: `ok  	help-my-run/backend/internal/coach`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/coach/prompts.go backend/internal/coach/coach.go backend/internal/coach/prompts_test.go && git commit -m "feat(coach): add dailyAdjustPrompt and DailyAdjustInput builder for M2

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 13: coach AdjustToday method + deterministic fallback

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/coach/coach.go` (add `dailyAdjustArgs()`, `AdjustToday(...)`, the `*llm.PlanDay ↔ readiness.FallbackSession` conversion + delegating `fallbackDecision`; add `"log"` to imports — `readiness` is already imported from Task 12, and the rule table lives entirely in `readiness.Fallback` so no `"math"` import is needed)
- Test: `/home/jake/project/help-my-run/backend/internal/coach/adjust_test.go` (create)

Depends on Tasks 11 + 12; `readiness` package; `metrics.EasyPace` via `c.Fitness(ctx)`. Reuses the M1 test helpers `newCoachStore` and `captureRunner` (which records `args`/`body`/returns canned `out`); confirm those exist in the coach test package or add the small `captureRunner` if absent.

- [ ] **Step 1: Write failing test (stub Runner with canned JSON + failing Runner for fallback).** Create `/home/jake/project/help-my-run/backend/internal/coach/adjust_test.go`:
```go
package coach

import (
	"context"
	"errors"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/readiness"
)

// failRunner returns an is_error envelope (the fallback path trigger).
type failRunner struct{ calls int }

func (r *failRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	r.calls++
	return []byte(`{"type":"result","is_error":true,"result":"please login"}`), nil
}

const adjustEnv = `{"type":"result","subtype":"success","is_error":false,"result":"{\"action\":\"SOFTEN\",\"adjusted_session\":{\"date\":\"2026-06-20\",\"dow\":\"Fri\",\"run_type\":\"easy\",\"distance_km\":4.5,\"pace_target\":\"6:00/km\",\"time_note\":\"~20:00 after CrossFit\",\"optional_if_cns\":true,\"rationale\":\"trimmed\"},\"rationale\":\"HRV down, sleep short\"}"}`

func TestAdjustTodayAIPath(t *testing.T) {
	s := newCoachStore(t)
	r := &captureRunner{out: []byte(adjustEnv)}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	rd := readiness.Readiness{Color: readiness.ColorAmber, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6, PaceTarget: "5:05/km"}

	dec, raw, source, err := c.AdjustToday(context.Background(), "2026-06-20", rd, today)
	if err != nil {
		t.Fatalf("AdjustToday error = %v", err)
	}
	if source != "ai" {
		t.Errorf("source = %q, want ai", source)
	}
	if dec.Action != llm.ActionSoften || dec.AdjustedSession == nil || dec.AdjustedSession.DistanceKm != 4.5 {
		t.Errorf("decision = %+v", dec)
	}
	if raw == "" {
		t.Error("raw empty, want re-marshaled decision JSON for ai source")
	}
	if !strings.Contains(r.body, `"readiness"`) || !strings.Contains(r.body, `"today_session"`) {
		t.Errorf("stdin missing readiness/today_session: %s", r.body)
	}
	joined := strings.Join(r.args, " ")
	if !strings.Contains(joined, "SINGLE-DAY") {
		t.Errorf("args missing daily-adjust prompt: %v", r.args)
	}
}

func TestAdjustTodayFallbackOnFailure(t *testing.T) {
	s := newCoachStore(t)
	r := &failRunner{}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	rd := readiness.Readiness{Color: readiness.ColorRed, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 8, PaceTarget: "5:05/km"}

	dec, raw, source, err := c.AdjustToday(context.Background(), "2026-06-20", rd, today)
	if err != nil {
		t.Fatalf("AdjustToday fallback returned error = %v, want nil", err)
	}
	if source != "fallback" {
		t.Errorf("source = %q, want fallback", source)
	}
	if raw != "" {
		t.Errorf("raw = %q, want empty on fallback", raw)
	}
	if dec.Action != llm.ActionMove {
		t.Errorf("action = %q, want MOVE", dec.Action)
	}
	if dec.AdjustedSession == nil || dec.AdjustedSession.RunType != "recovery" || dec.AdjustedSession.DistanceKm != 4 {
		t.Errorf("adjusted = %+v, want recovery 4km", dec.AdjustedSession)
	}
	if !dec.AdjustedSession.OptionalIfCNS {
		t.Error("adjusted optional_if_cns = false, want true on RED move")
	}
}

func TestAdjustTodayFallbackNoRunRestDay(t *testing.T) {
	s := newCoachStore(t)
	c := New(s, &llm.Client{Runner: &failRunner{}, Model: "m"}, "m", "/tmp/cfimg")
	rd := readiness.Readiness{Color: readiness.ColorGreen, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}

	dec, _, source, err := c.AdjustToday(context.Background(), "2026-06-20", rd, nil)
	if err != nil {
		t.Fatalf("AdjustToday error = %v", err)
	}
	if source != "fallback" {
		t.Errorf("source = %q, want fallback", source)
	}
	if dec.Action != llm.ActionRestDay || dec.AdjustedSession != nil {
		t.Errorf("decision = %+v, want REST_DAY/nil", dec)
	}
}

func TestAdjustTodayFallbackAmberQualitySoften(t *testing.T) {
	s := newCoachStore(t)
	c := New(s, &llm.Client{Runner: &failRunner{}, Model: "m"}, "m", "/tmp/cfimg")
	rd := readiness.Readiness{Color: readiness.ColorAmber, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6, PaceTarget: "5:05/km"}

	dec, _, _, err := c.AdjustToday(context.Background(), "2026-06-20", rd, today)
	if err != nil {
		t.Fatalf("AdjustToday error = %v", err)
	}
	if dec.Action != llm.ActionSoften {
		t.Errorf("action = %q, want SOFTEN", dec.Action)
	}
	if dec.AdjustedSession == nil || dec.AdjustedSession.DistanceKm != 4.5 {
		t.Errorf("adjusted distance = %v, want 4.5", dec.AdjustedSession)
	}
}

func TestAdjustTodayFallbackGreenStand(t *testing.T) {
	s := newCoachStore(t)
	c := New(s, &llm.Client{Runner: &failRunner{}, Model: "m"}, "m", "/tmp/cfimg")
	rd := readiness.Readiness{Color: readiness.ColorGreen, Drivers: readiness.ReadinessDrivers{Date: "2026-06-20", DataComplete: true}}
	today := &llm.PlanDay{Date: "2026-06-20", Dow: "Fri", RunType: "tempo", DistanceKm: 6, PaceTarget: "5:05/km"}

	dec, _, _, err := c.AdjustToday(context.Background(), "2026-06-20", rd, today)
	if err != nil {
		t.Fatalf("AdjustToday error = %v", err)
	}
	if dec.Action != llm.ActionStand {
		t.Errorf("action = %q, want STAND", dec.Action)
	}
	if dec.AdjustedSession == nil || dec.AdjustedSession.DistanceKm != 6 {
		t.Errorf("adjusted = %+v, want unchanged 6km", dec.AdjustedSession)
	}
}

var _ = errors.Is // keep errors import live if unused above
```
Note: if the coach test package lacks `captureRunner`, add a minimal one in this file: a `Run` that records `args []string` and `body string` (the stdin) and returns its `out []byte, nil`.

- [ ] **Step 2: Run it (expect FAIL).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/coach/ -run 'TestAdjustToday'
```
Expected: compile failure `c.AdjustToday undefined (type *Coach has no field or method AdjustToday)`.

- [ ] **Step 3: Minimal impl.** Append to `/home/jake/project/help-my-run/backend/internal/coach/coach.go` (the `readiness` import was added in Task 12; add `"log"` to the import block — do NOT add `"math"`: the rule table, `roundHalf`, and rationale strings all live in `readiness.Fallback`, which `fallbackDecision` now delegates to):
```go

// dailyAdjustArgs builds the claude -p argv for the daily-adjust (Coach Brain) call.
func (c *Coach) dailyAdjustArgs() []string {
	return []string{
		"-p", dailyAdjustPrompt,
		"--model", c.model,
		"--output-format", "json",
		"--allowedTools", "",
		"--no-session-persistence",
	}
}

// AdjustToday runs the daily-adjust claude -p call. By the llm.Call error
// contract, Call returns ONLY *llm.CallError (classified model/process failure) or
// llm.ErrMalformedJSON (unparseable output, post-retry) — i.e. every non-nil error
// here is a model failure, so any such error triggers the deterministic fallback
// rule (§2 table via readiness.Fallback). Errors from the pre-call setup (Fitness,
// GetAthleteProfile, input marshal) are NOT model failures and are returned to the
// caller. Returns the parsed decision, the raw re-marshaled JSON (empty on
// fallback), the source ("ai"|"fallback"), and an error only for those setup issues.
func (c *Coach) AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error) {
	fit, err := c.Fitness(ctx)
	if err != nil {
		return llm.DailyDecisionParsed{}, "", "", err
	}
	prof, err := c.store.GetAthleteProfile()
	if err != nil {
		return llm.DailyDecisionParsed{}, "", "", err
	}
	rc := json.RawMessage(prof.RunConstraintsJSON)
	if len(rc) == 0 || !json.Valid(rc) {
		rc = json.RawMessage(`{}`)
	}
	pp := ProfilePack{
		TargetWeeklyKm:  prof.TargetWeeklyKm,
		ProgressionMode: prof.ProgressionMode,
		Zone2CeilingBpm: prof.Zone2CeilingBpm,
		ThresholdBpm:    prof.ThresholdBpm,
		MaxHRBpm:        prof.MaxHRBpm,
		RunConstraints:  rc,
		GoalText:        prof.GoalText,
	}

	in := buildDailyAdjustInput(date, rd, today, fit, pp, nil, "")
	inputJSON, err := json.Marshal(in)
	if err != nil {
		return llm.DailyDecisionParsed{}, "", "", err
	}

	var decision llm.DailyDecisionParsed
	// llm.Call only returns *llm.CallError or llm.ErrMalformedJSON, both of which are
	// model failures, so any non-nil error here -> deterministic fallback.
	if cerr := c.llm.Call(ctx, c.dailyAdjustArgs(), string(inputJSON), &decision); cerr != nil {
		log.Printf("coach.AdjustToday: claude failed (%v); using deterministic fallback", cerr)
		fb := fallbackDecision(date, rd, today, fit)
		return fb, "", "fallback", nil
	}
	raw, _ := json.Marshal(decision)
	return decision, string(raw), "ai", nil
}

// planDayToFallbackSession converts a *llm.PlanDay into the readiness package's
// llm-free FallbackSession (nil today -> nil session, i.e. no run). `date` forces
// the local date onto the session (the coach always stamps today's date).
func planDayToFallbackSession(date string, today *llm.PlanDay) *readiness.FallbackSession {
	if today == nil {
		return nil
	}
	return &readiness.FallbackSession{
		Date:          date,
		Dow:           today.Dow,
		RunType:       today.RunType,
		DistanceKm:    today.DistanceKm,
		PaceTarget:    today.PaceTarget,
		TimeNote:      today.TimeNote,
		OptionalIfCNS: today.OptionalIfCNS,
		Rationale:     today.Rationale,
	}
}

// fallbackSessionToPlanDay converts a readiness.FallbackSession back into a
// *llm.PlanDay (nil -> nil for REST_DAY).
func fallbackSessionToPlanDay(fs *readiness.FallbackSession) *llm.PlanDay {
	if fs == nil {
		return nil
	}
	return &llm.PlanDay{
		Date:          fs.Date,
		Dow:           fs.Dow,
		RunType:       fs.RunType,
		DistanceKm:    fs.DistanceKm,
		PaceTarget:    fs.PaceTarget,
		TimeNote:      fs.TimeNote,
		OptionalIfCNS: fs.OptionalIfCNS,
		Rationale:     fs.Rationale,
	}
}

// fallbackDecision is the deterministic readiness->action rule applied when the
// claude -p daily-adjust call fails. It DELEGATES to readiness.Fallback (the
// single source of truth for the §2 rule table, roundHalf capping, and rationale
// strings) so the shipped fallback path is exactly what the Task 8/9 readiness
// tests cover. No rule logic lives here — only the *llm.PlanDay <-> FallbackSession
// conversion.
func fallbackDecision(date string, rd readiness.Readiness, today *llm.PlanDay, fit metrics.FitnessMetrics) llm.DailyDecisionParsed {
	dec := readiness.Fallback(rd.Color, planDayToFallbackSession(date, today), fit.EasyPace)
	return llm.DailyDecisionParsed{
		Action:          llm.DailyAction(dec.Action),
		AdjustedSession: fallbackSessionToPlanDay(dec.Adjusted),
		Rationale:       dec.Rationale,
	}
}
```

- [ ] **Step 4: Run it (expect PASS).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/coach/ -run 'TestAdjustToday'
```
Expected: `ok  	help-my-run/backend/internal/coach`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/coach/coach.go backend/internal/coach/adjust_test.go && git commit -m "feat(coach): add AdjustToday with deterministic readiness fallback (M2)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 14: agent seams + RunResult + RunDaily orchestration

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/agent/agent.go` (`Syncer`, `Adjuster`, `Pusher`, `Clock`, `RunResult`, `Agent`, `New`, `RunDaily`)
- Test: `/home/jake/project/help-my-run/backend/internal/agent/agent_test.go` (happy path, fallback, no-run, idempotency, stale)

Depends on Tasks 11–13; `readiness.Assess`/`Readiness`/`Color` (Tasks 5–9); `store.DailyDecision`/`AgentRun` + methods (Tasks 1–2); `push.Message`/`Sender`/`ErrDeviceNotRegistered` (Task 10); `sync.AllResult` (exists). Uses `store.GetLatestPlan(weekStart)` to load the M1 plan; confirm that method name/signature in the M1 store (adapt `todaySession` if the accessor differs).

- [ ] **Step 1: Write failing test (fake clock, fake syncer, fake adjuster, fake pusher, temp DB).** Create `/home/jake/project/help-my-run/backend/internal/agent/agent_test.go`:
```go
package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/push"
	"help-my-run/backend/internal/readiness"
	"help-my-run/backend/internal/store"
	syncpkg "help-my-run/backend/internal/sync"
)

func newAgentStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

type fakeSyncer struct {
	res    syncpkg.AllResult
	called int
}

func (f *fakeSyncer) SyncAll(ctx context.Context) syncpkg.AllResult {
	f.called++
	return f.res
}

type fakeAdjuster struct {
	dec    llm.DailyDecisionParsed
	raw    string
	source string
	err    error
	fit    metrics.FitnessMetrics
	called int
	gotRd  readiness.Readiness
	gotDay *llm.PlanDay
}

func (f *fakeAdjuster) AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error) {
	f.called++
	f.gotRd = rd
	f.gotDay = today
	return f.dec, f.raw, f.source, f.err
}

func (f *fakeAdjuster) Fitness(ctx context.Context) (metrics.FitnessMetrics, error) {
	return f.fit, nil
}

type fakePusher struct {
	msgs []push.Message
	err  error
}

func (f *fakePusher) Send(ctx context.Context, msg push.Message) error {
	f.msgs = append(f.msgs, msg)
	return f.err
}

func okSync() syncpkg.AllResult {
	return syncpkg.AllResult{
		Strava: syncpkg.SourceResult{Status: "ok"},
		Garmin: syncpkg.SourceResult{Status: "ok"},
	}
}

func seedPlanWithToday(t *testing.T, s *store.Store, weekStart, date string) {
	t.Helper()
	plan := llm.PlanParsed{
		WeeklyTargetKm: 20,
		Days: []llm.PlanDay{
			{Date: date, Dow: "Fri", RunType: "tempo", DistanceKm: 6, PaceTarget: "5:05/km"},
		},
		WeekRationale: "build",
	}
	b, _ := json.Marshal(plan)
	if _, err := s.InsertPlan(store.Plan{
		WeekStart: weekStart, GeneratedAt: "2026-06-19T00:00:00Z", Status: "ok",
		PlanJSON: string(b), FitnessSummary: "x", Model: "m",
	}); err != nil {
		t.Fatalf("InsertPlan: %v", err)
	}
	if err := s.UpsertDeviceToken(store.DeviceToken{
		ExpoPushToken: "ExponentPushToken[x]", Platform: "ios", UpdatedAt: "2026-06-19T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpsertDeviceToken: %v", err)
	}
}

func TestRunDailyHappyPathPersistsAndPushes(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")

	adj := &fakeAdjuster{
		dec: llm.DailyDecisionParsed{
			Action:          llm.ActionSoften,
			AdjustedSession: &llm.PlanDay{Date: "2026-06-19", Dow: "Fri", RunType: "easy", DistanceKm: 4.5, PaceTarget: "6:00/km"},
			Rationale:       "trimmed",
		},
		raw: `{"action":"SOFTEN"}`, source: "ai",
	}
	pu := &fakePusher{}
	a := New(s, &fakeSyncer{res: okSync()}, adj, pu, fakeClock{now: time.Date(2026, 6, 19, 5, 30, 0, 0, time.UTC)}, time.UTC)

	res := a.RunDaily(context.Background(), "2026-06-19")
	if res.Skipped {
		t.Fatalf("res.Skipped = true, want false")
	}
	if res.Action != llm.ActionSoften || res.Source != "ai" || !res.Pushed {
		t.Errorf("res = %+v", res)
	}
	if adj.gotDay == nil || adj.gotDay.RunType != "tempo" {
		t.Errorf("adjuster got today = %+v, want tempo from plan", adj.gotDay)
	}
	d, err := s.GetDailyDecision("2026-06-19")
	if err != nil {
		t.Fatalf("GetDailyDecision: %v", err)
	}
	if d.Action != "SOFTEN" || d.Source != "ai" || d.AdjustedSessionJSON == nil {
		t.Errorf("stored decision = %+v", d)
	}
	if d.OriginalSessionJSON == nil {
		t.Error("original_session_json nil, want the plan's tempo day")
	}
	run, err := s.GetAgentRun("2026-06-19")
	if err != nil {
		t.Fatalf("GetAgentRun: %v", err)
	}
	if run.Status != "ok" {
		t.Errorf("agent_run status = %q, want ok", run.Status)
	}
	if len(pu.msgs) != 1 || pu.msgs[0].To != "ExponentPushToken[x]" {
		t.Errorf("push msgs = %+v", pu.msgs)
	}
}

func TestRunDailyFallbackSource(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")
	adj := &fakeAdjuster{
		dec:    llm.DailyDecisionParsed{Action: llm.ActionMove, AdjustedSession: &llm.PlanDay{Date: "2026-06-19", RunType: "recovery", DistanceKm: 4}, Rationale: "fb"},
		raw:    "", source: "fallback",
	}
	a := New(s, &fakeSyncer{res: okSync()}, adj, &fakePusher{}, fakeClock{now: time.Date(2026, 6, 19, 5, 30, 0, 0, time.UTC)}, time.UTC)
	res := a.RunDaily(context.Background(), "2026-06-19")
	if res.Source != "fallback" || res.Action != llm.ActionMove {
		t.Errorf("res = %+v", res)
	}
	d, _ := s.GetDailyDecision("2026-06-19")
	if d.Source != "fallback" || d.RawResponse != nil {
		t.Errorf("stored = %+v, want fallback/nil raw", d)
	}
}

func TestRunDailyNoRunTodayReadinessOnly(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")
	adj := &fakeAdjuster{}
	a := New(s, &fakeSyncer{res: okSync()}, adj, &fakePusher{}, fakeClock{now: time.Date(2026, 6, 21, 5, 30, 0, 0, time.UTC)}, time.UTC)

	res := a.RunDaily(context.Background(), "2026-06-21")
	if res.Skipped {
		t.Fatalf("skipped, want a run")
	}
	if res.Action != llm.ActionRestDay {
		t.Errorf("action = %q, want REST_DAY (no run today)", res.Action)
	}
	if adj.called != 0 {
		t.Errorf("adjuster called %d times, want 0 on no-run day", adj.called)
	}
	d, err := s.GetDailyDecision("2026-06-21")
	if err != nil {
		t.Fatalf("GetDailyDecision: %v", err)
	}
	if d.Action != "REST_DAY" || d.OriginalSessionJSON != nil || d.AdjustedSessionJSON != nil {
		t.Errorf("stored = %+v, want REST_DAY/nil sessions", d)
	}
}

func TestRunDailyIdempotentSecondRunSkips(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")
	adj := &fakeAdjuster{
		dec:    llm.DailyDecisionParsed{Action: llm.ActionStand, AdjustedSession: &llm.PlanDay{Date: "2026-06-19", RunType: "tempo", DistanceKm: 6}},
		source: "ai",
	}
	syncer := &fakeSyncer{res: okSync()}
	pu := &fakePusher{}
	a := New(s, syncer, adj, pu, fakeClock{now: time.Date(2026, 6, 19, 5, 30, 0, 0, time.UTC)}, time.UTC)

	first := a.RunDaily(context.Background(), "2026-06-19")
	if first.Skipped {
		t.Fatal("first run skipped, want full run")
	}
	second := a.RunDaily(context.Background(), "2026-06-19")
	if !second.Skipped {
		t.Errorf("second run skipped = false, want true (idempotency)")
	}
	if adj.called != 1 || syncer.called != 1 || len(pu.msgs) != 1 {
		t.Errorf("second run was not a no-op: adj=%d sync=%d push=%d", adj.called, syncer.called, len(pu.msgs))
	}
}

func TestRunDailyStaleWhenSyncFails(t *testing.T) {
	s := newAgentStore(t)
	seedPlanWithToday(t, s, "2026-06-15", "2026-06-19")
	adj := &fakeAdjuster{
		dec:    llm.DailyDecisionParsed{Action: llm.ActionStand, AdjustedSession: &llm.PlanDay{Date: "2026-06-19", RunType: "tempo", DistanceKm: 6}},
		source: "ai",
	}
	bad := syncpkg.AllResult{
		Strava: syncpkg.SourceResult{Status: "ok"},
		Garmin: syncpkg.SourceResult{Status: "error"},
	}
	a := New(s, &fakeSyncer{res: bad}, adj, &fakePusher{}, fakeClock{now: time.Date(2026, 6, 19, 5, 30, 0, 0, time.UTC)}, time.UTC)
	res := a.RunDaily(context.Background(), "2026-06-19")
	if !res.Stale {
		t.Errorf("res.Stale = false, want true when garmin sync errored")
	}
}
```
Note: adapt `InsertPlan`/`store.Plan` field names + `GetLatestPlan` to the M1 store's actual API (verify before implementing; the contract assumes a `PlanJSON string` plan row keyed by `week_start` with a latest accessor).

- [ ] **Step 2: Run it (expect FAIL).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/agent/ -run 'TestRunDaily'
```
Expected: build failure `no Go files in .../agent` / `undefined: New`, `undefined: RunResult`, `undefined: Agent`.

- [ ] **Step 3: Minimal impl.** Create `/home/jake/project/help-my-run/backend/internal/agent/agent.go`:
```go
// Package agent orchestrates the daily M2 loop: sync -> readiness -> load today's
// session -> coach.AdjustToday (or readiness-only) -> persist -> push, guarded by
// a persistent once-per-day idempotency key (agent_runs.last_run_date).
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/push"
	"help-my-run/backend/internal/readiness"
	"help-my-run/backend/internal/store"
	syncpkg "help-my-run/backend/internal/sync"
)

// Syncer is the sync seam (real impl wraps sync.SyncAll with bound deps).
type Syncer interface {
	SyncAll(ctx context.Context) syncpkg.AllResult
}

// Adjuster is the coach seam used by the agent (*coach.Coach satisfies it).
type Adjuster interface {
	AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error)
	Fitness(ctx context.Context) (metrics.FitnessMetrics, error)
}

// Pusher is the push seam (*push.Client satisfies it).
type Pusher interface {
	Send(ctx context.Context, msg push.Message) error
}

// Clock is the agent's injectable time source (RFC3339 stamps in tests).
type Clock interface {
	Now() time.Time
}

// RunResult is the outcome of one daily agent run.
type RunResult struct {
	Date           string          `json:"date"`
	Skipped        bool            `json:"skipped"`
	ReadinessColor readiness.Color `json:"readiness_color"`
	Action         llm.DailyAction `json:"action"`
	Source         string          `json:"source"`
	Stale          bool            `json:"stale"`
	Pushed         bool            `json:"pushed"`
	Error          *string         `json:"error"`
}

// Agent owns the daily loop + persistent idempotency guard.
type Agent struct {
	store    *store.Store
	syncer   Syncer
	adjuster Adjuster
	pusher   Pusher
	clock    Clock
	loc      *time.Location
}

// New constructs an Agent. loc defaults to UTC if nil.
func New(s *store.Store, syncer Syncer, adjuster Adjuster, pusher Pusher, clock Clock, loc *time.Location) *Agent {
	if loc == nil {
		loc = time.UTC
	}
	return &Agent{store: s, syncer: syncer, adjuster: adjuster, pusher: pusher, clock: clock, loc: loc}
}

// mondayOf returns the Monday (YYYY-MM-DD) of the ISO week containing localDate.
func mondayOf(localDate string) (string, error) {
	d, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return "", err
	}
	off := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -off).Format("2006-01-02"), nil
}

// todaySession finds the PlanDay matching localDate in the latest plan for its
// week; returns (nil, "", nil) when no plan or no matching run day.
func (a *Agent) todaySession(localDate string) (*llm.PlanDay, string, error) {
	weekStart, err := mondayOf(localDate)
	if err != nil {
		return nil, "", err
	}
	p, err := a.store.GetLatestPlan(weekStart)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, "", nil
		}
		return nil, "", err
	}
	var parsed llm.PlanParsed
	if uerr := json.Unmarshal([]byte(p.PlanJSON), &parsed); uerr != nil {
		return nil, "", uerr
	}
	for i := range parsed.Days {
		if parsed.Days[i].Date == localDate && parsed.Days[i].RunType != "rest" {
			day := parsed.Days[i]
			return &day, parsed.WeekRationale, nil
		}
	}
	return nil, parsed.WeekRationale, nil
}

// RunDaily is the scheduler callback. It checks agent_runs.last_run_date for
// idempotency, runs the §5 flow, persists daily_decisions + agent_runs, and
// pushes the briefing. localDate is YYYY-MM-DD in the configured tz.
func (a *Agent) RunDaily(ctx context.Context, localDate string) RunResult {
	res := RunResult{Date: localDate}

	if _, err := a.store.GetAgentRun(localDate); err == nil {
		res.Skipped = true
		return res
	} else if !errors.Is(err, store.ErrNotFound) {
		return a.recordErr(localDate, res, err)
	}

	sr := a.syncer.SyncAll(ctx)
	res.Stale = sr.Strava.Status != "ok" || sr.Garmin.Status != "ok"

	rec, err := a.store.ListRecovery(60)
	if err != nil {
		return a.recordErr(localDate, res, err)
	}
	rd := readiness.Assess(rec, a.clock.Now())
	res.ReadinessColor = rd.Color

	today, _, err := a.todaySession(localDate)
	if err != nil {
		return a.recordErr(localDate, res, err)
	}

	now := a.clock.Now().UTC().Format(time.RFC3339)
	dd := store.DailyDecision{
		Date:           localDate,
		ReadinessColor: string(rd.Color),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	dd.DriversJSON = marshalDriversWithReasons(rd, res.Stale)

	var decision llm.DailyDecisionParsed
	if today == nil {
		decision = llm.DailyDecisionParsed{Action: llm.ActionRestDay, Rationale: restRationale(rd.Color)}
		res.Source = "fallback" // matches daily_decisions.source + the TS RunResult ai|fallback union
		dd.Source = "fallback"
		dd.Action = string(decision.Action)
		dd.Rationale = decision.Rationale
	} else {
		dec, raw, source, aerr := a.adjuster.AdjustToday(ctx, localDate, rd, today)
		if aerr != nil {
			return a.recordErr(localDate, res, aerr)
		}
		decision = dec
		res.Source = source
		dd.Source = source
		dd.Action = string(dec.Action)
		dd.Rationale = dec.Rationale
		if oj, oerr := json.Marshal(today); oerr == nil {
			dd.OriginalSessionJSON = strPtr(string(oj))
		}
		if dec.AdjustedSession != nil {
			if aj, aerr2 := json.Marshal(dec.AdjustedSession); aerr2 == nil {
				dd.AdjustedSessionJSON = strPtr(string(aj))
			}
		}
		if source == "ai" && raw != "" {
			dd.RawResponse = strPtr(raw)
		}
	}
	res.Action = decision.Action

	if perr := a.store.UpsertDailyDecision(dd); perr != nil {
		return a.recordErr(localDate, res, perr)
	}

	res.Pushed = a.pushBriefing(ctx, localDate, rd.Color, decision)

	_ = a.store.UpsertAgentRun(store.AgentRun{
		LastRunDate: localDate, Status: "ok", RanAt: now,
	})
	return res
}

// marshalDriversWithReasons serializes ReadinessDrivers with the readiness Reasons
// AND the run's staleness embedded as sibling keys ("reasons", "stale") so the API
// can surface both without a schema change. Falls back to plain drivers JSON on any
// error. The DTO/parseReasons in today_handlers.go ignore unknown driver keys and
// read these siblings back.
func marshalDriversWithReasons(rd readiness.Readiness, stale bool) string {
	dj, err := json.Marshal(rd.Drivers)
	if err != nil {
		return "{}"
	}
	var m map[string]interface{}
	if uerr := json.Unmarshal(dj, &m); uerr != nil {
		return string(dj)
	}
	m["reasons"] = rd.Reasons
	m["stale"] = stale
	out, merr := json.Marshal(m)
	if merr != nil {
		return string(dj)
	}
	return string(out)
}

// pushBriefing sends to every registered device token; returns true if >=1 ok.
// A DeviceNotRegistered token is deleted. Returns false (not fatal) on errors.
func (a *Agent) pushBriefing(ctx context.Context, localDate string, color readiness.Color, dec llm.DailyDecisionParsed) bool {
	tokens, err := a.store.ListDeviceTokens()
	if err != nil || len(tokens) == 0 {
		return false
	}
	body := dec.Rationale
	if body == "" {
		body = restRationale(color)
	}
	title := fmt.Sprintf("%s — %s", localDate, string(color))
	delivered := false
	for _, tk := range tokens {
		msg := push.Message{
			To:        tk.ExpoPushToken,
			Title:     title,
			Body:      body,
			Data:      map[string]interface{}{"date": localDate, "action": string(dec.Action)},
			Sound:     "default",
			Priority:  "high",
			ChannelID: "default",
		}
		if serr := a.pusher.Send(ctx, msg); serr != nil {
			if errors.Is(serr, push.ErrDeviceNotRegistered) {
				_ = a.store.DeleteDeviceToken(tk.ExpoPushToken)
			}
			log.Printf("agent: push to %s failed: %v", tk.ExpoPushToken, serr)
			continue
		}
		delivered = true
	}
	return delivered
}

// recordErr stores an error agent_run and returns res with the error set.
func (a *Agent) recordErr(localDate string, res RunResult, err error) RunResult {
	msg := err.Error()
	res.Error = &msg
	now := a.clock.Now().UTC().Format(time.RFC3339)
	_ = a.store.UpsertAgentRun(store.AgentRun{
		LastRunDate: localDate, Status: "error", Error: &msg, RanAt: now,
	})
	log.Printf("agent.RunDaily(%s): %v", localDate, err)
	return res
}

func restRationale(color readiness.Color) string {
	switch color {
	case readiness.ColorRed:
		return "Rest day — readiness low, stay recovered."
	case readiness.ColorAmber:
		return "Rest day."
	default:
		return "Rest day as planned; you're well recovered."
	}
}

func strPtr(s string) *string { return &s }
```

- [ ] **Step 4: Run it (expect PASS).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/agent/ -run 'TestRunDaily'
```
Expected: `ok  	help-my-run/backend/internal/agent`. (Passes only once store/readiness/push/coach symbols are present — they are, given task ordering.)

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/agent/ && git commit -m "feat(agent): add daily RunDaily loop with idempotency, fallback, no-run, and push (M2)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 15: agent RealSyncer adapter

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/agent/syncer.go` (`RealSyncer` wrapping `sync.SyncAll`)
- Test: `/home/jake/project/help-my-run/backend/internal/agent/syncer_test.go`

- [ ] **Step 1: Write failing test.** Create `/home/jake/project/help-my-run/backend/internal/agent/syncer_test.go`:
```go
package agent

import (
	"context"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
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

	// No strava tokens seeded -> SyncStrava errors (status "error"), proving the
	// adapter ran SyncAll end-to-end with the bound deps.
	rs := NewRealSyncer(s, strava.NewWithBase("id", "sec", "http://localhost/cb", "http://localhost"),
		garmin.Runner{Python: "/bin/cat", Script: "/dev/null"}, nil)
	res := rs.SyncAll(context.Background())
	if res.Strava.Status != "error" {
		t.Errorf("strava status = %q, want error (no tokens)", res.Strava.Status)
	}
}
```
Note: adapt `strava.NewWithBase` / `garmin.Runner` field names to the actual M0 signatures if they differ.

- [ ] **Step 2: Run it (expect FAIL).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/agent/ -run 'TestRealSyncer'
```
Expected: compile failure `undefined: NewRealSyncer`.

- [ ] **Step 3: Minimal impl.** Create `/home/jake/project/help-my-run/backend/internal/agent/syncer.go`:
```go
package agent

import (
	"context"

	"help-my-run/backend/internal/garmin"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
	syncpkg "help-my-run/backend/internal/sync"
)

// RealSyncer binds the sync dependencies and adapts sync.SyncAll to the Syncer
// seam used by the Agent.
type RealSyncer struct {
	store    *store.Store
	strava   *strava.Client
	runner   garmin.Runner
	extraEnv []string
}

// NewRealSyncer constructs the production Syncer.
func NewRealSyncer(s *store.Store, sc *strava.Client, r garmin.Runner, extraEnv []string) *RealSyncer {
	return &RealSyncer{store: s, strava: sc, runner: r, extraEnv: extraEnv}
}

// SyncAll runs both syncs with the bound deps.
func (r *RealSyncer) SyncAll(ctx context.Context) syncpkg.AllResult {
	return syncpkg.SyncAll(ctx, r.store, r.strava, r.runner, r.extraEnv)
}
```
Note: match the real `sync.SyncAll(...)` parameter list exactly (verify M0's signature).

- [ ] **Step 4: Run it (expect PASS).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/agent/
```
Expected: `ok  	help-my-run/backend/internal/agent`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/agent/syncer.go backend/internal/agent/syncer_test.go && git commit -m "feat(agent): add RealSyncer adapter over sync.SyncAll (M2)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 16: scheduler nextFire (pure function)

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/scheduler/scheduler.go` (`Clock`, `RealClock`, `Config`, `nextFire`)
- Test: `/home/jake/project/help-my-run/backend/internal/scheduler/scheduler_test.go`

- [ ] **Step 1: Write failing test.** Create `/home/jake/project/help-my-run/backend/internal/scheduler/scheduler_test.go`:
```go
package scheduler

import (
	"testing"
	"time"
)

func TestNextFire(t *testing.T) {
	utc := time.UTC
	cfg := Config{Hour: 5, Minute: 30, Loc: utc}

	cases := []struct {
		name string
		from time.Time
		want time.Time
	}{
		{
			name: "before T -> today",
			from: time.Date(2026, 6, 20, 3, 0, 0, 0, utc),
			want: time.Date(2026, 6, 20, 5, 30, 0, 0, utc),
		},
		{
			name: "after T -> tomorrow",
			from: time.Date(2026, 6, 20, 6, 0, 0, 0, utc),
			want: time.Date(2026, 6, 21, 5, 30, 0, 0, utc),
		},
		{
			name: "exactly at T -> tomorrow",
			from: time.Date(2026, 6, 20, 5, 30, 0, 0, utc),
			want: time.Date(2026, 6, 21, 5, 30, 0, 0, utc),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := nextFire(tc.from, cfg)
			if !got.Equal(tc.want) {
				t.Errorf("nextFire(%v) = %v, want %v", tc.from, got, tc.want)
			}
		})
	}
}

func TestNextFirePreservesWallClockAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	cfg := Config{Hour: 5, Minute: 30, Loc: loc}
	from := time.Date(2026, 3, 7, 6, 0, 0, 0, loc)
	got := nextFire(from, cfg)
	if got.Hour() != 5 || got.Minute() != 30 || got.Day() != 8 {
		t.Errorf("nextFire across DST = %v, want 2026-03-08 05:30 local", got)
	}
}
```

- [ ] **Step 2: Run it (expect FAIL).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/scheduler/ -run 'TestNextFire'
```
Expected: build failure `no Go files in .../scheduler` / `undefined: nextFire`, `undefined: Config`.

- [ ] **Step 3: Minimal impl.** Create `/home/jake/project/help-my-run/backend/internal/scheduler/scheduler.go`:
```go
// Package scheduler runs a callback once per day at a configured local time.
// The Clock is injectable so the daily fire is deterministic in tests.
package scheduler

import (
	"context"
	"time"
)

// Clock is the injectable time source (deterministic in tests).
type Clock interface {
	Now() time.Time
	// NewTimer returns a channel that fires after d plus a stop func.
	NewTimer(d time.Duration) (<-chan time.Time, func() bool)
}

// RealClock backs the production scheduler with the time package.
type RealClock struct{}

// Now returns the current time.
func (RealClock) Now() time.Time { return time.Now() }

// NewTimer wraps time.NewTimer.
func (RealClock) NewTimer(d time.Duration) (<-chan time.Time, func() bool) {
	t := time.NewTimer(d)
	return t.C, t.Stop
}

// Config is the daily schedule: fire at Hour:Minute in Loc, once per local day.
type Config struct {
	Hour, Minute int
	Loc          *time.Location
}

// nextFire returns the next instant strictly after `from` at cfg.Hour:cfg.Minute
// in cfg.Loc. If today's time already passed (or equals from), returns tomorrow's.
// AddDate preserves the wall clock across DST.
func nextFire(from time.Time, cfg Config) time.Time {
	now := from.In(cfg.Loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), cfg.Hour, cfg.Minute, 0, 0, cfg.Loc)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// compile-time guard that context is imported (Run added in the next task).
var _ = context.Background
```

- [ ] **Step 4: Run it (expect PASS).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/scheduler/ -run 'TestNextFire'
```
Expected: `ok  	help-my-run/backend/internal/scheduler`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/scheduler/ && git commit -m "feat(scheduler): add Clock, Config, and nextFire pure function (M2)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 17: scheduler Run loop (fake clock, fire-once-per-day)

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/scheduler/scheduler.go` (add `ConfigProvider` type + `Run`; remove the `var _ = context.Background` guard)
- Test: `/home/jake/project/help-my-run/backend/internal/scheduler/run_test.go`

`Run` takes a `ConfigProvider` (not a static `Config`) and calls it once per loop iteration, so the live `daily_run_time`/`timezone`/`agent_enabled` from `athlete_profile` are honored WITHOUT a restart: a changed time recomputes the next fire on the next cycle, and `enabled=false` suppresses `fn`.

- [ ] **Step 1: Write failing test.** Create `/home/jake/project/help-my-run/backend/internal/scheduler/run_test.go`:
```go
package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

// fakeClock is a controllable Clock: tests drive virtual time by sending on ch.
type fakeClock struct {
	mu   sync.Mutex
	now  time.Time
	ch   chan time.Time
	last time.Duration
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) setNow(t time.Time) {
	f.mu.Lock()
	f.now = t
	f.mu.Unlock()
}

func (f *fakeClock) NewTimer(d time.Duration) (<-chan time.Time, func() bool) {
	f.mu.Lock()
	f.last = d
	f.mu.Unlock()
	return f.ch, func() bool { return true }
}

// staticProvider returns a fixed Config+enabled (no DB) for tests that don't
// exercise live re-reads.
func staticProvider(cfg Config, enabled bool) ConfigProvider {
	return func() (Config, bool, error) { return cfg, enabled, nil }
}

func TestRunFiresOncePerDay(t *testing.T) {
	utc := time.UTC
	start := time.Date(2026, 6, 20, 3, 0, 0, 0, utc) // before T=05:30
	clk := &fakeClock{now: start, ch: make(chan time.Time)}
	cfg := Config{Hour: 5, Minute: 30, Loc: utc}

	var mu sync.Mutex
	var fires []string
	step := make(chan struct{}, 8)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, clk, staticProvider(cfg, true), func(_ context.Context, d string) {
			mu.Lock()
			fires = append(fires, d)
			mu.Unlock()
			step <- struct{}{}
		})
		close(done)
	}()

	clk.setNow(time.Date(2026, 6, 20, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now()
	<-step // wait for fn to record 06-20

	clk.ch <- clk.Now() // SAME day: ignored by in-process guard (no step)

	clk.setNow(time.Date(2026, 6, 21, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now()
	<-step // wait for fn to record 06-21

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if len(fires) != 2 || fires[0] != "2026-06-20" || fires[1] != "2026-06-21" {
		t.Fatalf("fires = %v, want [2026-06-20 2026-06-21]", fires)
	}
}

func TestRunDisabledDoesNotFire(t *testing.T) {
	utc := time.UTC
	clk := &fakeClock{now: time.Date(2026, 6, 20, 3, 0, 0, 0, utc), ch: make(chan time.Time)}
	cfg := Config{Hour: 5, Minute: 30, Loc: utc}

	var mu sync.Mutex
	fired := false
	// provider reports agent_enabled=false; the timer fires but fn must NOT run.
	prov := func() (Config, bool, error) { return cfg, false, nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, clk, prov, func(context.Context, string) {
			mu.Lock()
			fired = true
			mu.Unlock()
		})
		close(done)
	}()

	clk.setNow(time.Date(2026, 6, 20, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now() // delivers the fire; disabled provider must suppress fn
	clk.setNow(time.Date(2026, 6, 21, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now() // next cycle, still disabled

	cancel()
	<-done

	mu.Lock()
	defer mu.Unlock()
	if fired {
		t.Fatal("fn fired while agent_enabled=false, want suppressed")
	}
}

func TestRunRecomputesNextFireWhenTimeChanges(t *testing.T) {
	utc := time.UTC
	clk := &fakeClock{now: time.Date(2026, 6, 20, 3, 0, 0, 0, utc), ch: make(chan time.Time)}

	// provider returns 05:30 first, then 07:00 — Run must recompute the durations.
	var mu sync.Mutex
	call := 0
	prov := func() (Config, bool, error) {
		mu.Lock()
		defer mu.Unlock()
		call++
		if call == 1 {
			return Config{Hour: 5, Minute: 30, Loc: utc}, true, nil
		}
		return Config{Hour: 7, Minute: 0, Loc: utc}, true, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, clk, prov, func(context.Context, string) {})
		close(done)
	}()

	// First iteration is scheduled for 05:30 (from 03:00 -> 05:30 = 2h30m). Drive
	// that fire so Run loops back and re-reads the provider (which now returns 07:00).
	clk.setNow(time.Date(2026, 6, 20, 5, 30, 0, 0, utc))
	clk.ch <- clk.Now()

	// After re-reading the 07:00 schedule, the next timer is 05:30 -> 07:00 = 1h30m.
	// Poll until that recomputed duration is observed (proves the live re-read).
	deadline := time.Now().Add(2 * time.Second)
	for {
		clk.mu.Lock()
		last := clk.last
		clk.mu.Unlock()
		if last == 90*time.Minute {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("next fire not recomputed for 07:00; last timer = %v, want 1h30m", last)
		}
		time.Sleep(2 * time.Millisecond)
	}

	cancel()
	<-done
}

func TestRunStopsOnContextCancel(t *testing.T) {
	utc := time.UTC
	clk := &fakeClock{now: time.Date(2026, 6, 20, 3, 0, 0, 0, utc), ch: make(chan time.Time)}
	cfg := Config{Hour: 5, Minute: 30, Loc: utc}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Run(ctx, clk, staticProvider(cfg, true), func(context.Context, string) {})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
```

- [ ] **Step 2: Run it (expect FAIL).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/scheduler/ -run 'TestRun'
```
Expected: compile failure `undefined: Run`.

- [ ] **Step 3: Minimal impl.** In `/home/jake/project/help-my-run/backend/internal/scheduler/scheduler.go`, remove the line `var _ = context.Background` and append:
```go

// ConfigProvider re-reads the live schedule (from athlete_profile in production)
// on every loop iteration. It returns the resolved Config (HH:MM + IANA tz already
// parsed by the caller), whether the agent is enabled, and any error.
type ConfigProvider func() (cfg Config, enabled bool, err error)

// errRetry is how long Run waits before retrying after a ConfigProvider error.
const errRetry = time.Minute

// Run blocks until ctx is cancelled, invoking fn once per scheduled local day.
// fn receives ctx and the local date (YYYY-MM-DD) it fired for. The schedule is
// re-read via `next` on EVERY iteration, so changing daily_run_time/timezone
// recomputes the next fire on the following cycle and toggling agent_enabled=false
// suppresses fn WITHOUT a restart. An in-process guard (lastFired) prevents
// same-process double fires; the PERSISTENT once-per-day guard is owned by fn
// (agent.RunDaily checks agent_runs).
func Run(ctx context.Context, clk Clock, next ConfigProvider, fn func(ctx context.Context, localDate string)) {
	var lastFired string
	for {
		cfg, enabled, err := next()
		if err != nil {
			// Can't read the schedule; wait a bit and retry rather than spin.
			c, stop := clk.NewTimer(errRetry)
			select {
			case <-ctx.Done():
				stop()
				return
			case <-c:
				continue
			}
		}

		fireAt := nextFire(clk.Now(), cfg)
		d := fireAt.Sub(clk.Now())
		if d < 0 {
			d = 0
		}
		c, stop := clk.NewTimer(d)
		select {
		case <-ctx.Done():
			stop()
			return
		case <-c:
			if !enabled {
				// Agent disabled in the profile: skip this fire, re-read next cycle.
				continue
			}
			localDate := clk.Now().In(cfg.Loc).Format("2006-01-02")
			if localDate != lastFired {
				lastFired = localDate
				fn(ctx, localDate)
			}
		}
	}
}
```

- [ ] **Step 4: Run it (expect PASS).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/scheduler/ -run 'TestRun|TestNextFire'
```
Expected: `ok  	help-my-run/backend/internal/scheduler`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/scheduler/ && git commit -m "feat(scheduler): add Run loop with in-process once-per-day guard (M2)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 18: Config additions for M2 (agent + Expo push base URL)

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/config/config.go` (add M2 env fields after the M1 fields, e.g. after `AnthropicAPIKey`)
- Test: `/home/jake/project/help-my-run/backend/internal/config/config_test.go` (create)

- [ ] **Step 1: Write the failing config defaults test.** Create `/home/jake/project/help-my-run/backend/internal/config/config_test.go`:
```go
package config

import (
	"testing"

	"github.com/kelseyhightower/envconfig"
)

func TestM2ConfigDefaults(t *testing.T) {
	t.Setenv("STRAVA_CLIENT_ID", "id")
	t.Setenv("STRAVA_CLIENT_SECRET", "secret")
	t.Setenv("STRAVA_REDIRECT_URL", "http://localhost/cb")
	t.Setenv("API_TOKEN", "tok")

	var c Config
	if err := envconfig.Process("", &c); err != nil {
		t.Fatalf("envconfig.Process error = %v", err)
	}
	if c.AgentEnabledDefault != true {
		t.Errorf("AgentEnabledDefault = %v, want true", c.AgentEnabledDefault)
	}
	if c.AgentRunTime != "05:30" {
		t.Errorf("AgentRunTime = %q, want 05:30", c.AgentRunTime)
	}
	if c.AgentTimezone != "UTC" {
		t.Errorf("AgentTimezone = %q, want UTC", c.AgentTimezone)
	}
	if c.AgentTickInterval != "1m" {
		t.Errorf("AgentTickInterval = %q, want 1m", c.AgentTickInterval)
	}
	if c.ExpoPushBaseURL != "https://exp.host" {
		t.Errorf("ExpoPushBaseURL = %q, want https://exp.host", c.ExpoPushBaseURL)
	}
}

func TestM2ConfigOverrides(t *testing.T) {
	t.Setenv("STRAVA_CLIENT_ID", "id")
	t.Setenv("STRAVA_CLIENT_SECRET", "secret")
	t.Setenv("STRAVA_REDIRECT_URL", "http://localhost/cb")
	t.Setenv("API_TOKEN", "tok")
	t.Setenv("AGENT_ENABLED", "false")
	t.Setenv("AGENT_RUN_TIME", "06:00")
	t.Setenv("AGENT_TZ", "Asia/Seoul")
	t.Setenv("EXPO_PUSH_BASE_URL", "http://localhost:9999")

	var c Config
	if err := envconfig.Process("", &c); err != nil {
		t.Fatalf("envconfig.Process error = %v", err)
	}
	if c.AgentEnabledDefault != false || c.AgentRunTime != "06:00" ||
		c.AgentTimezone != "Asia/Seoul" || c.ExpoPushBaseURL != "http://localhost:9999" {
		t.Errorf("overrides not applied: %+v", c)
	}
}
```
Note: adjust the required-env setup to match the actual M0/M1 `Config` required fields if they differ (e.g. `DB_PATH`, `CLAUDE_BIN`); the test only needs `envconfig.Process` to succeed.

- [ ] **Step 2: Run the test, expect FAIL (compile error).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/config/
```
Expected: FAIL — `c.AgentEnabledDefault undefined (type Config has no field or method AgentEnabledDefault)`.

- [ ] **Step 3: Add the M2 env fields.** In `/home/jake/project/help-my-run/backend/internal/config/config.go`, inside the `Config` struct (after the M1 fields), add:
```go
	// M2: agentic daily coach. The live schedule (time/tz/enable) is re-read from
	// athlete_profile on every scheduler.Run iteration (see scheduler.ConfigProvider,
	// Task 25), so PUT /api/profile edits apply without a restart; these are only
	// first-boot defaults + the push test seam, NOT the runtime source.
	AgentEnabledDefault bool   `envconfig:"AGENT_ENABLED" default:"true"`
	AgentRunTime        string `envconfig:"AGENT_RUN_TIME" default:"05:30"`
	AgentTimezone       string `envconfig:"AGENT_TZ" default:"UTC"`
	AgentTickInterval   string `envconfig:"AGENT_TICK_INTERVAL" default:"1m"`
	ExpoPushBaseURL     string `envconfig:"EXPO_PUSH_BASE_URL" default:"https://exp.host"`
```

- [ ] **Step 4: Run the test, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/config/
```
Expected: `ok  	help-my-run/backend/internal/config`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/config/ && git commit -m "feat(config): add M2 agent + Expo push base URL env fields

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 19: M2 DTOs + validISODate helper

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/api/dto.go` (extend `profileDTO`; add `pushRegisterRequestDTO`, `pushRegisterResponseDTO`, `readinessDriversDTO`, `todayResponseDTO`)
- Modify: `/home/jake/project/help-my-run/backend/internal/api/plan_handlers.go` (add `validISODate` after `validWeekStart`)
- Test: `/home/jake/project/help-my-run/backend/internal/api/m2_dto_test.go` (create)

- [ ] **Step 1: Write the failing DTO/helper test.** Create `/home/jake/project/help-my-run/backend/internal/api/m2_dto_test.go`:
```go
package api

import (
	"encoding/json"
	"testing"
)

func TestValidISODate(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"2026-06-20", true},
		{"2026-13-01", false},
		{"2026-6-1", false},
		{"", false},
		{"../etc", false},
		{"2026-06-20T00:00:00Z", false},
	}
	for _, c := range cases {
		if got := validISODate(c.in); got != c.want {
			t.Errorf("validISODate(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestProfileDTOM2Fields(t *testing.T) {
	dto := profileDTO{DailyRunTime: "05:30", Timezone: "Asia/Seoul", AgentEnabled: true}
	b, _ := json.Marshal(dto)
	s := string(b)
	for _, want := range []string{`"daily_run_time":"05:30"`, `"timezone":"Asia/Seoul"`, `"agent_enabled":true`} {
		if !contains(s, want) {
			t.Errorf("profileDTO JSON %s missing %s", s, want)
		}
	}
}

func TestTodayResponseDTOTags(t *testing.T) {
	dto := todayResponseDTO{
		Date:           "2026-06-20",
		ReadinessColor: "amber",
		Drivers:        readinessDriversDTO{Date: "2026-06-20", DataComplete: true},
		Reasons:        []string{"HRV -18% vs baseline"},
		Action:         "SOFTEN",
		Rationale:      "trimmed",
		Source:         "ai",
		Stale:          false,
	}
	b, _ := json.Marshal(dto)
	s := string(b)
	for _, want := range []string{
		`"readiness_color":"amber"`, `"drivers":`, `"data_complete":true`,
		`"original_session":null`, `"effective_session":null`,
		`"reasons":["HRV -18% vs baseline"]`, `"action":"SOFTEN"`, `"source":"ai"`, `"stale":false`,
	} {
		if !contains(s, want) {
			t.Errorf("todayResponseDTO JSON %s missing %s", s, want)
		}
	}
}

func TestPushRegisterDTOTags(t *testing.T) {
	req := pushRegisterRequestDTO{ExpoPushToken: "ExponentPushToken[x]", Platform: "ios"}
	b, _ := json.Marshal(req)
	if !contains(string(b), `"expo_push_token":"ExponentPushToken[x]"`) || !contains(string(b), `"platform":"ios"`) {
		t.Errorf("pushRegisterRequestDTO JSON = %s", b)
	}
	resp := pushRegisterResponseDTO{ExpoPushToken: "ExponentPushToken[x]", Platform: "ios", UpdatedAt: "2026-06-20T05:30:01Z"}
	rb, _ := json.Marshal(resp)
	if !contains(string(rb), `"updated_at":"2026-06-20T05:30:01Z"`) {
		t.Errorf("pushRegisterResponseDTO JSON = %s", rb)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```
Note: if the api test package already defines a `contains`/`indexOf` helper, drop the duplicates here and reuse the existing one.

- [ ] **Step 2: Run the test, expect FAIL (compile error).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestValidISODate|TestProfileDTOM2Fields|TestTodayResponseDTOTags|TestPushRegisterDTOTags'
```
Expected: FAIL — `undefined: validISODate`, `dto.DailyRunTime undefined`, `undefined: todayResponseDTO`, etc.

- [ ] **Step 3a: Extend profileDTO + add M2 DTOs.** In `/home/jake/project/help-my-run/backend/internal/api/dto.go`, replace the `profileDTO` block (adds 3 fields before `UpdatedAt`):
```go
// --- M1 /api/profile (extended in M2) ---
type profileDTO struct {
	TargetWeeklyKm     float64 `json:"target_weekly_km"`
	ProgressionMode    string  `json:"progression_mode"`
	Zone2CeilingBpm    *int64  `json:"zone2_ceiling_bpm"`
	ThresholdBpm       *int64  `json:"threshold_bpm"`
	MaxHRBpm           *int64  `json:"max_hr_bpm"`
	RunConstraintsJSON string  `json:"run_constraints_json"`
	GoalText           string  `json:"goal_text"`
	DailyRunTime       string  `json:"daily_run_time"` // "HH:MM" 24h local (M2)
	Timezone           string  `json:"timezone"`       // IANA (M2)
	AgentEnabled       bool    `json:"agent_enabled"`  // M2 daily agent on/off
	UpdatedAt          string  `json:"updated_at,omitempty"`
}
```
Append the new M2 DTOs at the end of `dto.go`:
```go
// --- M2 /api/push/register ---
type pushRegisterRequestDTO struct {
	ExpoPushToken string `json:"expo_push_token"`
	Platform      string `json:"platform"` // "ios"|"android"
}
type pushRegisterResponseDTO struct {
	ExpoPushToken string `json:"expo_push_token"`
	Platform      string `json:"platform"`
	UpdatedAt     string `json:"updated_at"`
}

// --- M2 /api/today ---
type readinessDriversDTO struct {
	Date            string   `json:"date"`
	SleepHours      *float64 `json:"sleep_hours"`
	SleepScore      *int64   `json:"sleep_score"`
	HRVLastNightMs  *int64   `json:"hrv_last_night_ms"`
	HRVBaselineMs   *float64 `json:"hrv_baseline_ms"`
	HRVDeltaPct     *float64 `json:"hrv_delta_pct"`
	RHRLastNight    *int64   `json:"rhr_last_night"`
	RHRBaseline     *float64 `json:"rhr_baseline"`
	RHRDeltaBpm     *float64 `json:"rhr_delta_bpm"`
	BodyBatteryHigh *int64   `json:"body_battery_high"`
	RecoveryTrend   string   `json:"recovery_trend"`
	DataComplete    bool     `json:"data_complete"`
}
type todayResponseDTO struct {
	Date             string              `json:"date"`
	ReadinessColor   string              `json:"readiness_color"`
	Drivers          readinessDriversDTO `json:"drivers"`
	Reasons          []string            `json:"reasons"`
	Action           string              `json:"action"`
	OriginalSession  *planDayDTO         `json:"original_session"`
	EffectiveSession *planDayDTO         `json:"effective_session"`
	Rationale        string              `json:"rationale"`
	Source           string              `json:"source"`
	Stale            bool                `json:"stale"`
}
```
Note: `planDayDTO` is the M1 plan-day DTO — confirm its name/fields; if M1 surfaces plan days as raw `llm.PlanDay` instead of a DTO, use `*llm.PlanDay` here and in `today_handlers.go` (Task 23) and drop `planDayDTOPtr`'s DTO conversion.

- [ ] **Step 3b: Add validISODate helper.** In `/home/jake/project/help-my-run/backend/internal/api/plan_handlers.go`, add immediately after `validWeekStart` (`time` is already imported there):
```go
// validISODate reports whether s is a strict ISO calendar date (YYYY-MM-DD).
// Reused by M2 today/undo/agent date params.
func validISODate(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Format("2006-01-02") == s
}
```

- [ ] **Step 4: Run the test, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestValidISODate|TestProfileDTOM2Fields|TestTodayResponseDTOTags|TestPushRegisterDTOTags'
```
Expected: `ok  	help-my-run/backend/internal/api`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/api/dto.go backend/internal/api/plan_handlers.go backend/internal/api/m2_dto_test.go && git commit -m "feat(api): add M2 DTOs (today/push/profile fields) + validISODate helper

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 20: Profile handler — map + validate M2 fields

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/api/profile_handlers.go` (`updateProfile` defaults + validation + mapping; `toProfileDTO` mapping)
- Test: `/home/jake/project/help-my-run/backend/internal/api/m2_handlers_test.go` (create; profile section here, push/today/agent appended in later tasks)

- [ ] **Step 1: Write the failing profile-handler test.** Create `/home/jake/project/help-my-run/backend/internal/api/m2_handlers_test.go`:
```go
package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestProfileGetReturnsM2Defaults(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/profile", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out profileDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.DailyRunTime != "05:30" || out.Timezone != "UTC" || out.AgentEnabled != true {
		t.Errorf("M2 defaults = (%q,%q,%v), want (05:30,UTC,true)", out.DailyRunTime, out.Timezone, out.AgentEnabled)
	}
}

func TestProfilePutPersistsM2Fields(t *testing.T) {
	h, s := newTestServer(t)
	body := `{"target_weekly_km":25,"progression_mode":"hold","run_constraints_json":"{}","daily_run_time":"06:15","timezone":"Asia/Seoul","agent_enabled":false}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out profileDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.DailyRunTime != "06:15" || out.Timezone != "Asia/Seoul" || out.AgentEnabled != false {
		t.Errorf("resp M2 = %+v", out)
	}
	p, _ := s.GetAthleteProfile()
	if p.DailyRunTime != "06:15" || p.Timezone != "Asia/Seoul" || p.AgentEnabled != false {
		t.Errorf("stored M2 = %+v", p)
	}
}

func TestProfilePutDefaultsEmptyM2Fields(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"target_weekly_km":20,"progression_mode":"build","run_constraints_json":"{}","daily_run_time":"","timezone":"","agent_enabled":true}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out profileDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.DailyRunTime != "05:30" || out.Timezone != "UTC" {
		t.Errorf("defaults on empty = (%q,%q), want (05:30,UTC)", out.DailyRunTime, out.Timezone)
	}
}

func TestProfilePutRejectsBadRunTime(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"target_weekly_km":20,"progression_mode":"build","run_constraints_json":"{}","daily_run_time":"5:30","timezone":"UTC","agent_enabled":true}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for bad run time (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestProfilePutRejectsBadTimezone(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"target_weekly_km":20,"progression_mode":"build","run_constraints_json":"{}","daily_run_time":"06:00","timezone":"Mars/Phobos","agent_enabled":true}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for bad tz (body=%s)", rec.Code, rec.Body.String())
	}
}
```
Note: `newTestServer`, `do`, `doBody`, `testToken` are the existing M1 api test helpers. `newTestServer` will be extended in Task 21 to inject `Agent`/`Pusher` fakes; until then it constructs `Deps` without those fields — these profile tests do not exercise them. If `newTestServer` currently returns only `h` (no store), adapt the `_, s :=` destructuring to the existing helper's signature.

- [ ] **Step 2: Run the test, expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestProfileGetReturnsM2Defaults|TestProfilePutPersistsM2Fields|TestProfilePutDefaultsEmptyM2Fields|TestProfilePutRejectsBadRunTime|TestProfilePutRejectsBadTimezone'
```
Expected: FAIL — M2 fields not mapped (zero values) and bad-input cases return 200 not 400.

- [ ] **Step 3: Add mapping + validation.** Rewrite `/home/jake/project/help-my-run/backend/internal/api/profile_handlers.go` (preserve existing `profile`/`updateProfile`/`toProfileDTO` behavior, add the 3 fields + validation):
```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"time"

	"help-my-run/backend/internal/store"
)

// runTimeRe matches a 24h HH:MM (00:00–23:59).
var runTimeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func (h *handlers) profile(w http.ResponseWriter, r *http.Request) {
	p, err := h.d.Store.GetAthleteProfile()
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "profile not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}

func (h *handlers) updateProfile(w http.ResponseWriter, r *http.Request) {
	var in profileDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	if in.ProgressionMode == "" {
		in.ProgressionMode = "build"
	}
	if in.RunConstraintsJSON == "" {
		in.RunConstraintsJSON = "{}"
	}
	if in.DailyRunTime == "" {
		in.DailyRunTime = "05:30"
	}
	if in.Timezone == "" {
		in.Timezone = "UTC"
	}
	if !runTimeRe.MatchString(in.DailyRunTime) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "daily_run_time must be HH:MM (00:00-23:59)"})
		return
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timezone must be a valid IANA name"})
		return
	}
	if err := h.d.Store.UpsertAthleteProfile(store.AthleteProfile{
		TargetWeeklyKm:     in.TargetWeeklyKm,
		ProgressionMode:    in.ProgressionMode,
		Zone2CeilingBpm:    in.Zone2CeilingBpm,
		ThresholdBpm:       in.ThresholdBpm,
		MaxHRBpm:           in.MaxHRBpm,
		RunConstraintsJSON: in.RunConstraintsJSON,
		GoalText:           in.GoalText,
		DailyRunTime:       in.DailyRunTime,
		Timezone:           in.Timezone,
		AgentEnabled:       in.AgentEnabled,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	p, err := h.d.Store.GetAthleteProfile()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toProfileDTO(p))
}

func toProfileDTO(p store.AthleteProfile) profileDTO {
	return profileDTO{
		TargetWeeklyKm:     p.TargetWeeklyKm,
		ProgressionMode:    p.ProgressionMode,
		Zone2CeilingBpm:    p.Zone2CeilingBpm,
		ThresholdBpm:       p.ThresholdBpm,
		MaxHRBpm:           p.MaxHRBpm,
		RunConstraintsJSON: p.RunConstraintsJSON,
		GoalText:           p.GoalText,
		DailyRunTime:       p.DailyRunTime,
		Timezone:           p.Timezone,
		AgentEnabled:       p.AgentEnabled,
		UpdatedAt:          p.UpdatedAt,
	}
}
```
Note: `time/tzdata` (embedded by `main.go` in Task 26) makes `time.LoadLocation("Asia/Seoul")` succeed in CI. If the api package tests fail to load `Asia/Seoul` on a tzdata-less box, add `import _ "time/tzdata"` to `m2_handlers_test.go`.

- [ ] **Step 4: Run the test, expect PASS (existing profile tests unaffected).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestProfile'
```
Expected: `ok  	help-my-run/backend/internal/api`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/api/profile_handlers.go backend/internal/api/m2_handlers_test.go && git commit -m "feat(api): map + validate M2 profile fields (daily_run_time/timezone/agent_enabled)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 21: Extend Deps + Coach interface; add Agent/Pusher seams + register routes; fakes

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/api/router.go` (add `AdjustToday` to `Coach`; add `Agent`+`Pusher` seam interfaces; extend `Deps`; register 4 routes)
- Modify: `/home/jake/project/help-my-run/backend/internal/api/m1_handlers_test.go` (extend `fakeCoach` with `AdjustToday`)
- Modify: `/home/jake/project/help-my-run/backend/internal/api/handlers_test.go` (wire `Agent`/`Pusher` fakes into `newTestServer` `Deps`)
- Create: `/home/jake/project/help-my-run/backend/internal/api/m2_fakes_test.go` (`fakeAgent`, `fakePusher` + interface conformance)

This task plus Tasks 22–23 form one contiguous block: the router references `h.pushRegister`/`h.today`/`h.undoToday`/`h.agentRun` (built in 22–23), so run the full api suite only after Task 23. Note the `force bool` param on the `Agent` seam's `RunDaily` — the api `Agent` seam wraps the agent so `POST /api/agent/run?force=true` can bypass the persistent guard. Threading `force` through requires the agent expose a force-capable entry point: either add `force bool` to `agent.Agent.RunDaily` (and update Task 14's signature + tests), or have the api `Agent` impl delete the `agent_runs` row before calling `RunDaily` when `force`. Decide before Task 23; this plan's `api.Agent` interface uses `RunDaily(ctx, localDate string, force bool)`.

- [ ] **Step 1: Write the failing seam test.** Create `/home/jake/project/help-my-run/backend/internal/api/m2_fakes_test.go`:
```go
package api

import (
	"context"

	"help-my-run/backend/internal/agent"
	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/push"
	"help-my-run/backend/internal/readiness"
)

// fakeAgent is the injected api.Agent for handler tests.
type fakeAgent struct {
	result    agent.RunResult
	lastDate  string
	lastForce bool
}

func (f *fakeAgent) RunDaily(ctx context.Context, localDate string, force bool) agent.RunResult {
	f.lastDate = localDate
	f.lastForce = force
	if f.result.Date == "" {
		f.result.Date = localDate
	}
	return f.result
}

// fakePusher is the injected api.Pusher for handler tests.
type fakePusher struct {
	sent    []push.Message
	sendErr error
}

func (f *fakePusher) Send(ctx context.Context, msg push.Message) error {
	f.sent = append(f.sent, msg)
	return f.sendErr
}

// Compile-time interface conformance checks (the RED-state assertions).
var (
	_ Coach  = (*fakeCoach)(nil)
	_ Agent  = (*fakeAgent)(nil)
	_ Pusher = (*fakePusher)(nil)
)

var _ = readiness.ColorGreen
var _ llm.DailyAction = llm.ActionStand
```

- [ ] **Step 2: Run the test, expect FAIL (compile error).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestProfile'
```
Expected: FAIL — `undefined: Agent`, `undefined: Pusher` (build failed).

- [ ] **Step 3a: Extend the Coach interface + add Agent/Pusher seams + Deps + routes.** In `/home/jake/project/help-my-run/backend/internal/api/router.go`, add to the import block `"help-my-run/backend/internal/agent"`, `"help-my-run/backend/internal/push"`, `"help-my-run/backend/internal/readiness"`. Add `AdjustToday` to the `Coach` interface; add the `Agent`/`Pusher` interfaces; add `Agent Agent` + `Pusher Pusher` to `Deps`; register the 4 routes inside the existing bearer `r.Group`:
```go
// Coach is the M1 plan-engine seam (extended in M2 with AdjustToday). *coach.Coach
// satisfies it structurally.
type Coach interface {
	ParseCrossFit(ctx context.Context, weekStart, imagePath string) (llm.CrossFitWeekParsed, string, error)
	GeneratePlan(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (llm.PlanParsed, string, string, error)
	Fitness(ctx context.Context) (metrics.FitnessMetrics, error)
	AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error)
}

// Agent is the M2 daily-loop seam. *agent.Agent satisfies it structurally (after
// the force-capable RunDaily is wired — see Task 26).
type Agent interface {
	RunDaily(ctx context.Context, localDate string, force bool) agent.RunResult
}

// Pusher is the M2 push transport seam. *push.Client satisfies it structurally.
type Pusher interface {
	Send(ctx context.Context, msg push.Message) error
}
```
Add to `Deps`:
```go
	Agent  Agent  // M2: daily loop (POST /api/agent/run)
	Pusher Pusher // M2: push transport
```
Register inside the bearer `r.Group(...)` block (next to the M1 routes):
```go
		// M2
		r.Post("/api/push/register", h.pushRegister)
		r.Get("/api/today", h.today)
		r.Post("/api/today/undo", h.undoToday)
		r.Post("/api/agent/run", h.agentRun)
```
Keep the exact existing import names/signatures for `Coach`/`Deps`/`NewRouter`; this task only adds members + routes.

- [ ] **Step 3b: Extend fakeCoach with AdjustToday.** In `/home/jake/project/help-my-run/backend/internal/api/m1_handlers_test.go`, add `"help-my-run/backend/internal/readiness"` to the imports, add a `lastAdjustDate string` field to `fakeCoach`, and add the method (after `Fitness`):
```go
func (f *fakeCoach) AdjustToday(ctx context.Context, date string, rd readiness.Readiness, today *llm.PlanDay) (llm.DailyDecisionParsed, string, string, error) {
	f.lastAdjustDate = date
	if f.adjustErr != nil {
		return llm.DailyDecisionParsed{}, "", "", f.adjustErr
	}
	dec := llm.DailyDecisionParsed{Action: llm.ActionStand, AdjustedSession: today, Rationale: "stand"}
	return dec, `{"action":"STAND"}`, "ai", nil
}
```
Add `adjustErr error` to the `fakeCoach` struct if not present.

- [ ] **Step 3c: Wire Agent/Pusher fakes into newTestServer.** In `/home/jake/project/help-my-run/backend/internal/api/handlers_test.go`, add `Agent: &fakeAgent{}` and `Pusher: &fakePusher{}` to the `Deps` literal inside `newTestServer`.

- [ ] **Step 4: Run the test, expect PASS for the seam compile + profile (after Tasks 22–23 handlers exist).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestProfile'
```
Expected (once 22–23 land): `ok  	help-my-run/backend/internal/api` (conformance checks compile; profile tests green).

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/api/router.go backend/internal/api/m1_handlers_test.go backend/internal/api/handlers_test.go backend/internal/api/m2_fakes_test.go && git commit -m "feat(api): add AdjustToday to Coach + Agent/Pusher seams + register M2 routes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 22: Push-register handler

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/api/push_handlers.go` (`pushRegister`)
- Test: `/home/jake/project/help-my-run/backend/internal/api/m2_handlers_test.go` (append push-register tests)

- [ ] **Step 1: Write the failing push-register tests.** Append to `/home/jake/project/help-my-run/backend/internal/api/m2_handlers_test.go`:
```go
func TestPushRegisterStoresToken(t *testing.T) {
	h, s := newTestServer(t)
	body := `{"expo_push_token":"ExponentPushToken[abc]","platform":"ios"}`
	rec := doBody(t, h, http.MethodPost, "/api/push/register", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out pushRegisterResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.ExpoPushToken != "ExponentPushToken[abc]" || out.Platform != "ios" || out.UpdatedAt == "" {
		t.Errorf("resp = %+v", out)
	}
	toks, _ := s.ListDeviceTokens()
	if len(toks) != 1 || toks[0].ExpoPushToken != "ExponentPushToken[abc]" {
		t.Errorf("stored tokens = %+v, want one", toks)
	}
}

func TestPushRegisterRejectsEmptyToken(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"expo_push_token":"","platform":"ios"}`
	rec := doBody(t, h, http.MethodPost, "/api/push/register", testToken, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPushRegisterRejectsBadPlatform(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"expo_push_token":"ExponentPushToken[x]","platform":"windows"}`
	rec := doBody(t, h, http.MethodPost, "/api/push/register", testToken, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPushRegisterRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	body := `{"expo_push_token":"ExponentPushToken[x]","platform":"ios"}`
	rec := doBody(t, h, http.MethodPost, "/api/push/register", "", body)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestPushRegister'
```
Expected: FAIL — `h.pushRegister undefined` (build failed).

- [ ] **Step 3: Write the handler.** Create `/home/jake/project/help-my-run/backend/internal/api/push_handlers.go`:
```go
package api

import (
	"encoding/json"
	"net/http"

	"help-my-run/backend/internal/store"
)

func (h *handlers) pushRegister(w http.ResponseWriter, r *http.Request) {
	var in pushRegisterRequestDTO
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	if in.ExpoPushToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "expo_push_token required"})
		return
	}
	if in.Platform != "ios" && in.Platform != "android" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "platform must be ios or android"})
		return
	}
	if err := h.d.Store.UpsertDeviceToken(store.DeviceToken{
		ExpoPushToken: in.ExpoPushToken,
		Platform:      in.Platform,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	toks, err := h.d.Store.ListDeviceTokens()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	updatedAt := ""
	for _, t := range toks {
		if t.ExpoPushToken == in.ExpoPushToken {
			updatedAt = t.UpdatedAt
			break
		}
	}
	writeJSON(w, http.StatusOK, pushRegisterResponseDTO{
		ExpoPushToken: in.ExpoPushToken,
		Platform:      in.Platform,
		UpdatedAt:     updatedAt,
	})
}
```

- [ ] **Step 4: Run the test, expect PASS.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestPushRegister'
```
Expected: `ok  	help-my-run/backend/internal/api`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/api/push_handlers.go backend/internal/api/m2_handlers_test.go && git commit -m "feat(api): POST /api/push/register handler (store/refresh device token)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 23: Today / Undo / Agent-run handlers

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/api/today_handlers.go` (`today`, `undoToday`, `agentRun` + `toTodayResponse`/`parseReasons`/`planDayDTOPtr` mappers)
- Test: `/home/jake/project/help-my-run/backend/internal/api/m2_handlers_test.go` (append today/undo/agent tests)

- [ ] **Step 1: Write the failing today/undo/agent tests.** Append to `/home/jake/project/help-my-run/backend/internal/api/m2_handlers_test.go` (and ensure its import block includes `"context"`, `"help-my-run/backend/internal/agent"`, `"help-my-run/backend/internal/store"`):
```go
func seedDecision(t *testing.T, s *store.Store, date string) {
	t.Helper()
	orig := `{"date":"` + date + `","dow":"Fri","run_type":"tempo","distance_km":6,"pace_target":"5:05/km","time_note":"~20:00 after CrossFit","optional_if_cns":false,"rationale":"Threshold work."}`
	adj := `{"date":"` + date + `","dow":"Fri","run_type":"easy","distance_km":4.5,"pace_target":"6:00/km","time_note":"~20:00 after CrossFit","optional_if_cns":true,"rationale":"Trimmed to easy."}`
	raw := `{"action":"SOFTEN"}`
	if err := s.UpsertDailyDecision(store.DailyDecision{
		Date:                date,
		ReadinessColor:      "amber",
		DriversJSON:         `{"date":"` + date + `","sleep_hours":6.1,"hrv_delta_pct":-17.8,"recovery_trend":"declining","data_complete":true,"reasons":["HRV -17.8% vs baseline"],"stale":true}`,
		OriginalSessionJSON: &orig,
		AdjustedSessionJSON: &adj,
		Action:              "SOFTEN",
		Rationale:           "HRV is 18% below baseline.",
		Source:              "ai",
		RawResponse:         &raw,
	}); err != nil {
		t.Fatalf("seedDecision: %v", err)
	}
}

func TestTodayReturnsDecision(t *testing.T) {
	h, s := newTestServer(t)
	seedDecision(t, s, "2026-06-20")
	rec := do(t, h, http.MethodGet, "/api/today?date=2026-06-20", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out todayResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Date != "2026-06-20" || out.ReadinessColor != "amber" || out.Action != "SOFTEN" || out.Source != "ai" {
		t.Errorf("today = %+v", out)
	}
	if out.Drivers.RecoveryTrend != "declining" || out.Drivers.DataComplete != true {
		t.Errorf("drivers = %+v", out.Drivers)
	}
	if len(out.Reasons) == 0 || out.Reasons[0] != "HRV -17.8% vs baseline" {
		t.Errorf("reasons = %v", out.Reasons)
	}
	if !out.Stale {
		t.Errorf("stale = false, want true (read back from drivers_json)")
	}
	if out.OriginalSession == nil || out.OriginalSession.RunType != "tempo" || out.OriginalSession.DistanceKm != 6 {
		t.Errorf("original = %+v", out.OriginalSession)
	}
	if out.EffectiveSession == nil || out.EffectiveSession.RunType != "easy" || out.EffectiveSession.DistanceKm != 4.5 {
		t.Errorf("effective = %+v", out.EffectiveSession)
	}
}

func TestTodayBadDate(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/today?date=2026-13-99", testToken)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestTodayNotFound(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/today?date=2026-06-20", testToken)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
	if !contains(rec.Body.String(), `"error":"no decision for date"`) {
		t.Errorf("body = %s, want no-decision error", rec.Body.String())
	}
}

func TestUndoTodayRevertsToOriginal(t *testing.T) {
	h, s := newTestServer(t)
	seedDecision(t, s, "2026-06-20")
	rec := doBody(t, h, http.MethodPost, "/api/today/undo?date=2026-06-20", testToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out todayResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Action != "STAND" {
		t.Errorf("action = %q, want STAND after undo", out.Action)
	}
	if out.EffectiveSession == nil || out.OriginalSession == nil ||
		out.EffectiveSession.RunType != out.OriginalSession.RunType ||
		out.EffectiveSession.DistanceKm != out.OriginalSession.DistanceKm {
		t.Errorf("after undo effective != original: eff=%+v orig=%+v", out.EffectiveSession, out.OriginalSession)
	}
	got, err := s.GetDailyDecision("2026-06-20")
	if err != nil {
		t.Fatalf("GetDailyDecision: %v", err)
	}
	if got.Action != "STAND" || got.AdjustedSessionJSON == nil || *got.AdjustedSessionJSON != *got.OriginalSessionJSON {
		t.Errorf("stored after undo = %+v", got)
	}
}

func TestUndoTodayNotFound(t *testing.T) {
	h, _ := newTestServer(t)
	rec := doBody(t, h, http.MethodPost, "/api/today/undo?date=2026-06-20", testToken, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestAgentRunInvokesAgentAndReturnsResult(t *testing.T) {
	_, s := newTestServer(t)
	fa := &fakeAgent{result: agent.RunResult{
		Date: "2026-06-20", Skipped: false, ReadinessColor: "amber",
		Action: "SOFTEN", Source: "ai", Stale: false, Pushed: true,
	}}
	h2 := NewRouter(Deps{
		Store: s, Strava: nil, APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) {
			return "ok", 0, nil, "ok", 0, nil
		},
		Coach: &fakeCoach{}, ImageDir: t.TempDir(), Agent: fa, Pusher: &fakePusher{},
	})
	rec := doBody(t, h2, http.MethodPost, "/api/agent/run?date=2026-06-20&force=true", testToken, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out agent.RunResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Date != "2026-06-20" || out.Action != "SOFTEN" || !out.Pushed {
		t.Errorf("run result = %+v", out)
	}
	if fa.lastDate != "2026-06-20" || fa.lastForce != true {
		t.Errorf("agent invoked with date=%q force=%v, want 2026-06-20/true", fa.lastDate, fa.lastForce)
	}
}

func TestAgentRunBadDate(t *testing.T) {
	h, _ := newTestServer(t)
	rec := doBody(t, h, http.MethodPost, "/api/agent/run?date=nope", testToken, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestTodayRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/today", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestToday|TestUndoToday|TestAgentRun'
```
Expected: FAIL — `h.today undefined`, `h.undoToday undefined`, `h.agentRun undefined` (build failed).

- [ ] **Step 3: Write the handlers.** Create `/home/jake/project/help-my-run/backend/internal/api/today_handlers.go`:
```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

// resolveDate returns the ?date= param (validated) or today in UTC if absent.
func resolveDate(r *http.Request) (string, bool) {
	d := r.URL.Query().Get("date")
	if d == "" {
		return time.Now().UTC().Format("2006-01-02"), true
	}
	return d, validISODate(d)
}

func (h *handlers) today(w http.ResponseWriter, r *http.Request) {
	date, ok := resolveDate(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date must be an ISO date (YYYY-MM-DD)"})
		return
	}
	dec, err := h.d.Store.GetDailyDecision(date)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no decision for date"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp, err := toTodayResponse(dec)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) undoToday(w http.ResponseWriter, r *http.Request) {
	date, ok := resolveDate(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date must be an ISO date (YYYY-MM-DD)"})
		return
	}
	dec, err := h.d.Store.GetDailyDecision(date)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no decision for date"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	dec.AdjustedSessionJSON = dec.OriginalSessionJSON
	dec.Action = string(llm.ActionStand)
	dec.Rationale = "Reverted to original session."
	if err := h.d.Store.UpsertDailyDecision(dec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	updated, err := h.d.Store.GetDailyDecision(date)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp, err := toTodayResponse(updated)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *handlers) agentRun(w http.ResponseWriter, r *http.Request) {
	date, ok := resolveDate(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "date must be an ISO date (YYYY-MM-DD)"})
		return
	}
	force := r.URL.Query().Get("force") == "true"
	res := h.d.Agent.RunDaily(r.Context(), date, force)
	writeJSON(w, http.StatusOK, res)
}

// toTodayResponse maps a stored DailyDecision into the wire DTO.
func toTodayResponse(d store.DailyDecision) (todayResponseDTO, error) {
	var drivers readinessDriversDTO
	if err := json.Unmarshal([]byte(d.DriversJSON), &drivers); err != nil {
		return todayResponseDTO{}, errors.New("stored drivers corrupt: " + err.Error())
	}
	reasons := parseReasons(d.DriversJSON)
	stale := parseStale(d.DriversJSON)

	orig, err := planDayDTOPtr(d.OriginalSessionJSON)
	if err != nil {
		return todayResponseDTO{}, errors.New("stored original_session corrupt: " + err.Error())
	}
	eff, err := planDayDTOPtr(d.AdjustedSessionJSON)
	if err != nil {
		return todayResponseDTO{}, errors.New("stored adjusted_session corrupt: " + err.Error())
	}
	return todayResponseDTO{
		Date:             d.Date,
		ReadinessColor:   d.ReadinessColor,
		Drivers:          drivers,
		Reasons:          reasons,
		Action:           d.Action,
		OriginalSession:  orig,
		EffectiveSession: eff,
		Rationale:        d.Rationale,
		Source:           d.Source,
		Stale:            stale,
	}, nil
}

// parseReasons extracts the optional "reasons" array embedded in drivers_json by
// the agent. Absent -> empty slice.
func parseReasons(driversJSON string) []string {
	var wrap struct {
		Reasons []string `json:"reasons"`
	}
	_ = json.Unmarshal([]byte(driversJSON), &wrap)
	if wrap.Reasons == nil {
		return []string{}
	}
	return wrap.Reasons
}

// parseStale extracts the optional "stale" boolean embedded in drivers_json by the
// agent (marshalDriversWithReasons). Absent -> false.
func parseStale(driversJSON string) bool {
	var wrap struct {
		Stale bool `json:"stale"`
	}
	_ = json.Unmarshal([]byte(driversJSON), &wrap)
	return wrap.Stale
}

// planDayDTOPtr unmarshals a *string PlanDay JSON column into a *planDayDTO (nil
// when nil/empty — rest day or undone-to-null).
func planDayDTOPtr(js *string) (*planDayDTO, error) {
	if js == nil || *js == "" {
		return nil, nil
	}
	var pd llm.PlanDay
	if err := json.Unmarshal([]byte(*js), &pd); err != nil {
		return nil, err
	}
	dto := planDayDTO{
		Date: pd.Date, Dow: pd.Dow, RunType: pd.RunType, DistanceKm: pd.DistanceKm,
		PaceTarget: pd.PaceTarget, TimeNote: pd.TimeNote,
		OptionalIfCNS: pd.OptionalIfCNS, Rationale: pd.Rationale,
	}
	return &dto, nil
}
```
Note: `planDayDTO` field names must match the M1 DTO. If M1 has no `planDayDTO` (it surfaces `llm.PlanDay` directly), change `todayResponseDTO.OriginalSession`/`EffectiveSession` to `*llm.PlanDay` (Task 19) and replace `planDayDTOPtr` with a `*llm.PlanDay` unmarshal.

- [ ] **Step 4: Run the test, expect PASS (full api suite).** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/
```
Expected: `ok  	help-my-run/backend/internal/api` (all today/undo/agent tests plus the full existing api suite green).

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/api/today_handlers.go backend/internal/api/m2_handlers_test.go && git commit -m "feat(api): GET /api/today, POST /api/today/undo, POST /api/agent/run handlers

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 24: Full-suite verification + DeviceNotRegistered drop integration check

**Files:**
- Test: `/home/jake/project/help-my-run/backend/internal/api/m2_handlers_test.go` (append one integration assertion)
- (No new impl — verification gate.)

- [ ] **Step 1: Write a token-drop integration test.** Append to `/home/jake/project/help-my-run/backend/internal/api/m2_handlers_test.go`:
```go
func TestRegisteredTokenIsDroppable(t *testing.T) {
	h, s := newTestServer(t)
	body := `{"expo_push_token":"ExponentPushToken[drop]","platform":"android"}`
	if rec := doBody(t, h, http.MethodPost, "/api/push/register", testToken, body); rec.Code != http.StatusOK {
		t.Fatalf("register status = %d", rec.Code)
	}
	toks, _ := s.ListDeviceTokens()
	if len(toks) != 1 {
		t.Fatalf("tokens after register = %d, want 1", len(toks))
	}
	if err := s.DeleteDeviceToken("ExponentPushToken[drop]"); err != nil {
		t.Fatalf("DeleteDeviceToken: %v", err)
	}
	toks, _ = s.ListDeviceTokens()
	if len(toks) != 0 {
		t.Errorf("tokens after drop = %d, want 0", len(toks))
	}
}
```

- [ ] **Step 2: Run it, expect PASS immediately.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run 'TestRegisteredTokenIsDroppable'
```
Expected: `ok  	help-my-run/backend/internal/api`.

- [ ] **Step 3: Run the entire backend test suite.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./... 2>&1 | tail -40
```
Expected: every package line is `ok` (or `no test files`), no `FAIL` — `internal/store`, `internal/push`, `internal/config`, `internal/api`, `internal/readiness`, `internal/agent`, `internal/scheduler`, `internal/coach`, `internal/llm`, `internal/metrics`.

- [ ] **Step 4: gofmt + vet gate.** Command:
```
cd /home/jake/project/help-my-run/backend && test -z "$(gofmt -l internal cmd)" && go vet ./... && echo "CLEAN"
```
Expected: prints `CLEAN`.

- [ ] **Step 5: Commit.** Command:
```
git add backend/internal/api/m2_handlers_test.go && git commit -m "test(api): integration check for DeviceNotRegistered token drop path

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 25: main.go wiring — push client + agent + scheduler + tzdata

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/cmd/server/main.go` (`import _ "time/tzdata"`; `App` exposes `Coach`+`Agent`+`Pusher`; `Wire` builds push client + agent; `main` launches `scheduler.Run` next to the sync ticker)
- Test: `/home/jake/project/help-my-run/backend/cmd/server/main_test.go` (create/extend — assert `Wire` builds the M2 graph)

This is the integration seam. The `api.Agent` interface (Task 21) requires `RunDaily(ctx, localDate string, force bool)`, but `agent.Agent.RunDaily` (Task 14) takes `(ctx, localDate string)`. Reconcile with ONE of:
- (A, recommended) Define a small `forceAgent` adapter in `main.go` that satisfies `api.Agent`: on `force`, it deletes the `agent_runs` row for the date (resetting the persistent guard) then calls `dailyAgent.RunDaily(ctx, localDate)`; on `!force` it calls `RunDaily` directly. The scheduler callback calls `dailyAgent.RunDaily(ctx, localDate)` (non-forced).
- (B) Add `force bool` to `agent.Agent.RunDaily` (update Task 14 signature/tests) and pass it through.
This plan uses (A) so Task 14 stays unchanged.

- [ ] **Step 1: Write the failing wiring test.** Create `/home/jake/project/help-my-run/backend/cmd/server/main_test.go`:
```go
package main

import (
	"net/http/httptest"
	"path/filepath"
	"testing"

	"help-my-run/backend/internal/config"
)

func testCfg(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		StravaClientID:      "id",
		StravaClientSecret:  "secret",
		StravaRedirectURL:   "http://localhost:8080/api/strava/callback",
		APIToken:            "tok",
		DBPath:              filepath.Join(t.TempDir(), "wire.db"),
		Port:                "8080",
		ClaudeBin:           "claude",
		ClaudeModel:         "claude-opus-4-8",
		ImageDir:            t.TempDir(),
		AgentEnabledDefault: true,
		AgentRunTime:        "05:30",
		AgentTimezone:       "Asia/Seoul",
		AgentTickInterval:   "1m",
		ExpoPushBaseURL:     "https://exp.host",
	}
}

func TestWireBuildsM2Graph(t *testing.T) {
	app, err := Wire(testCfg(t))
	if err != nil {
		t.Fatalf("Wire error = %v", err)
	}
	defer func() { _ = app.Store.Close() }()

	if app.Agent == nil {
		t.Error("app.Agent = nil, want a wired *agent.Agent")
	}
	if app.Pusher == nil {
		t.Error("app.Pusher = nil, want a wired *push.Client")
	}
	if app.Handler == nil {
		t.Error("app.Handler = nil")
	}
}

func TestWireTzdataLoadsSeoul(t *testing.T) {
	_, err := loadAgentLocation("Asia/Seoul")
	if err != nil {
		t.Fatalf("loadAgentLocation(Asia/Seoul) error = %v", err)
	}
}

func TestWiredHandlerServesToday404(t *testing.T) {
	app, err := Wire(testCfg(t))
	if err != nil {
		t.Fatalf("Wire error = %v", err)
	}
	defer func() { _ = app.Store.Close() }()

	req := httptest.NewRequest("GET", "/api/today?date=2026-06-20", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("today status = %d, want 404 (no decision seeded)", rec.Code)
	}
}
```
Note: match `config.Config` field names exactly to the live struct (e.g. `DBPath`/`Port`/`ClaudeBin`/`ImageDir` may differ — adjust the literal accordingly). The test only needs `Wire` to succeed and expose the three new `App` fields + `loadAgentLocation`.

- [ ] **Step 2: Run the test, expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/backend && go test ./cmd/server/
```
Expected: FAIL — `app.Agent undefined (type *App has no field Agent)`, `undefined: loadAgentLocation` (build failed).

- [ ] **Step 3: Wire push + agent + scheduler in main.go.** In `/home/jake/project/help-my-run/backend/cmd/server/main.go`:
  (a) Add `_ "time/tzdata"` to the import block plus `"fmt"`, `"strconv"`, `"strings"`, `"help-my-run/backend/internal/agent"`, `"help-my-run/backend/internal/push"`, `"help-my-run/backend/internal/scheduler"` (`"fmt"` is used by the scheduleProvider's wrapped tz error in step (f); skip it if already imported).
  (b) Extend the `App` struct with `Coach *coach.Coach`, `Agent *agent.Agent`, `Pusher *push.Client` (keep existing fields).
  (c) In `Wire`, after building `coachEngine`, build the push client + agent and inject them into `api.Deps`:
```go
	pushClient := push.NewClient(cfg.ExpoPushBaseURL)
	dailyAgent := agent.New(
		s,
		agent.NewRealSyncer(s, stravaClient, runner, extraEnv),
		coachEngine,
		pushClient,
		agentClock{},
		nil, // loc resolved in main() from profile; agent default UTC is fine for Wire
	)
```
   and add `Agent: apiAgent{a: dailyAgent, store: s}` (the force adapter, see (e)) and `Pusher: pushClient` to the `api.Deps` literal; set `Coach: coachEngine`, `Agent`, `Pusher` on the returned `*App`.
  (d) Add helpers at package scope:
```go
// agentClock backs the agent with the real clock.
type agentClock struct{}

func (agentClock) Now() time.Time { return time.Now() }

// loadAgentLocation loads the IANA timezone for the daily schedule. Empty -> UTC.
func loadAgentLocation(tz string) (*time.Location, error) {
	if tz == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(tz)
}

// parseRunTime splits "HH:MM" 24h into hour, minute; defaults to 05:30 on a
// malformed value.
func parseRunTime(s string) (int, int) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 {
		h, herr := strconv.Atoi(parts[0])
		m, merr := strconv.Atoi(parts[1])
		if herr == nil && merr == nil && h >= 0 && h < 24 && m >= 0 && m < 60 {
			return h, m
		}
	}
	return 5, 30
}
```
  (e) Add the force adapter satisfying `api.Agent` (option A):
```go
// apiAgent adapts *agent.Agent to the api.Agent seam, adding force semantics:
// force deletes the persistent once-per-day guard before running.
type apiAgent struct {
	a     *agent.Agent
	store *store.Store
}

func (p apiAgent) RunDaily(ctx context.Context, localDate string, force bool) agent.RunResult {
	if force {
		_ = p.store.DeleteAgentRun(localDate) // reset the persistent once-per-day guard
	}
	return p.a.RunDaily(ctx, localDate)
}
```
   `store.DeleteAgentRun(date string) error` is defined in Task 2 (`internal/store/daily.go`) and listed in the Shared Contracts store-method block, so it already exists when `main.go` compiles here — no extra store work in this task.
  (f) In `main()`, after the existing sync-ticker launch (`go syncpkg.RunTicker(...)` region), build a `scheduler.ConfigProvider` that RE-READS `athlete_profile` on every call (falling back to the env defaults) and launch the scheduler unconditionally — the provider's `enabled` flag now governs firing at runtime, so `PUT /api/profile` edits (time/tz/enable) take effect WITHOUT a restart:
```go
	// scheduleProvider re-reads the live schedule from athlete_profile on every
	// scheduler loop iteration; env values are first-boot fallbacks only.
	scheduleProvider := func() (scheduler.Config, bool, error) {
		runTime := cfg.AgentRunTime
		runTz := cfg.AgentTimezone
		enabled := cfg.AgentEnabledDefault
		if prof, perr := app.Store.GetAthleteProfile(); perr == nil {
			if prof.DailyRunTime != "" {
				runTime = prof.DailyRunTime
			}
			if prof.Timezone != "" {
				runTz = prof.Timezone
			}
			enabled = prof.AgentEnabled
		}
		loc, lerr := loadAgentLocation(runTz)
		if lerr != nil {
			return scheduler.Config{}, false, fmt.Errorf("scheduler tz %q: %w", runTz, lerr)
		}
		hh, mm := parseRunTime(runTime)
		return scheduler.Config{Hour: hh, Minute: mm, Loc: loc}, enabled, nil
	}
	go scheduler.Run(ctx, scheduler.RealClock{}, scheduleProvider,
		func(c context.Context, localDate string) {
			res := app.Agent.RunDaily(c, localDate)
			log.Printf("agent: date=%s skipped=%v color=%s action=%s source=%s pushed=%v",
				res.Date, res.Skipped, res.ReadinessColor, res.Action, res.Source, res.Pushed)
		})
	log.Printf("agent scheduler: started (schedule re-read from profile each cycle)")
```
   `stravaClient`, `runner`, `extraEnv` are already in scope from the M0/M1 wiring; reuse them. Add `"fmt"` to the import block (used by the provider's wrapped error) if not already present.

- [ ] **Step 4: Run the test + full backend build/vet.** Commands:
```
cd /home/jake/project/help-my-run/backend && go test ./cmd/server/
cd /home/jake/project/help-my-run/backend && go build ./... && go vet ./...
```
Expected: `ok  	help-my-run/backend/cmd/server`; `go build ./...` and `go vet ./...` exit 0.

- [ ] **Step 5: Commit.** Command:
```
git add backend/cmd/server/main.go backend/cmd/server/main_test.go backend/go.mod backend/go.sum && git commit -m "feat(server): wire push client + daily agent + scheduler alongside sync ticker

Embeds time/tzdata for headless IANA tz lookups; the scheduler re-reads the
agent schedule (time/tz/enable) from athlete_profile every cycle via a
ConfigProvider (config values are first-boot fallbacks only), so PUT /api/profile
edits apply without a restart; force adapter resets the persistent guard.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 26: App — add expo-notifications dependency + app.json config

**Files:**
- Modify: `/home/jake/project/help-my-run/app/package.json` (add `expo-notifications`; `expo-device`/`expo-constants` already present)
- Modify: `/home/jake/project/help-my-run/app/app.json` (`expo-notifications` plugin + `extra.eas.projectId` + Android channel)
- Test: `/home/jake/project/help-my-run/app/app/__tests__/appConfig.test.ts` (create)

All app commands run from `/home/jake/project/help-my-run/app`.

- [ ] **Step 1: Write the failing config test.** Create `/home/jake/project/help-my-run/app/app/__tests__/appConfig.test.ts`:
```ts
import appConfig from '../../app.json';

describe('app.json M2 config', () => {
  const expo = (appConfig as { expo: Record<string, unknown> }).expo;

  it('lists expo-notifications in the plugins array', () => {
    const plugins = expo.plugins as Array<string | [string, unknown]>;
    const names = plugins.map((p) => (Array.isArray(p) ? p[0] : p));
    expect(names).toContain('expo-notifications');
  });

  it('exposes an EAS projectId under expo.extra.eas.projectId', () => {
    const extra = expo.extra as { eas?: { projectId?: string } } | undefined;
    expect(extra?.eas?.projectId).toBeDefined();
    expect(typeof extra?.eas?.projectId).toBe('string');
    expect((extra?.eas?.projectId ?? '').length).toBeGreaterThan(0);
  });
});
```

- [ ] **Step 2: Run it — expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/appConfig.test.ts
```
Expected: FAIL — plugins lacks `'expo-notifications'`; no `extra.eas.projectId`.

- [ ] **Step 3: Install expo-notifications.** Run:
```
cd /home/jake/project/help-my-run/app && npx expo install expo-notifications
```
This resolves the SDK-56-compatible version into `package.json` (do NOT hardcode a version). `expo-constants`/`expo-device` are already present.

- [ ] **Step 4: Add the plugin + projectId + Android channel config to app.json.** Edit `/home/jake/project/help-my-run/app/app.json`: add `expo-notifications` (with `icon`/`color` options) to the `plugins` array, and add an `extra.eas.projectId` placeholder. Append inside `plugins`:
```json
      [
        "expo-notifications",
        {
          "icon": "./assets/images/icon.png",
          "color": "#fc4c02"
        }
      ]
```
and add at `expo` top level (alongside `experiments`):
```json
    "extra": {
      "eas": {
        "projectId": "00000000-0000-0000-0000-000000000000"
      }
    }
```
The placeholder UUID is replaced by the real value when the host runs `eas init`/`eas build:configure` (Setup Prerequisites). The test only asserts a non-empty string.

- [ ] **Step 5: Run the config test — expect PASS.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/appConfig.test.ts
```
Expected: `2 passed`.

- [ ] **Step 6: Commit.** Command:
```
git add app/package.json app/package-lock.json app/app.json app/app/__tests__/appConfig.test.ts && git commit -m "build(app): add expo-notifications dep + app.json projectId/plugin (APP)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 27: TS types — TodayBriefing, ReadinessDrivers, DailyAction, PushRegisterRequest, RunResult + AthleteProfile additions

**Files:**
- Modify: `/home/jake/project/help-my-run/app/src/api/types.ts` (append M2 types; extend `AthleteProfile`)
- Modify: `/home/jake/project/help-my-run/app/src/api/__tests__/types.test.ts` (add M2-types block)

Sequenced before the notifications lib (Task 28) so `tsc --noEmit` stays green (the lib imports `PushRegisterRequest`).

- [ ] **Step 1: Write the failing types test.** Append to `/home/jake/project/help-my-run/app/src/api/__tests__/types.test.ts` (after the existing M1 block; add `ReadinessColor`/`DailyAction`/`ReadinessDrivers`/`TodayBriefing`/`PushRegisterRequest`/`RunResult` to the `from '../types'` type-imports):
```ts
describe('M2 API types', () => {
  it('TodayBriefing matches the GET /api/today contract (snake_case)', () => {
    const color: ReadinessColor = 'amber';
    const action: DailyAction = 'SOFTEN';
    const drivers: ReadinessDrivers = {
      date: '2026-06-20',
      sleep_hours: 6.1, sleep_score: 62,
      hrv_last_night_ms: 48, hrv_baseline_ms: 58.4, hrv_delta_pct: -17.8,
      rhr_last_night: 54, rhr_baseline: 50.2, rhr_delta_bpm: 3.8,
      body_battery_high: 61,
      recovery_trend: 'declining', data_complete: true,
    };
    const briefing: TodayBriefing = {
      date: '2026-06-20',
      readiness_color: color,
      drivers,
      reasons: ['HRV -17.8% vs baseline', 'Sleep score 62 (<65)'],
      action,
      original_session: {
        date: '2026-06-20', dow: 'Fri', run_type: 'tempo', distance_km: 6,
        pace_target: '5:05/km', time_note: '~20:00 after CrossFit',
        optional_if_cns: false, rationale: 'Threshold work on a light CF day.',
      },
      effective_session: {
        date: '2026-06-20', dow: 'Fri', run_type: 'easy', distance_km: 4.5,
        pace_target: '6:00/km', time_note: '~20:00 after CrossFit',
        optional_if_cns: true, rationale: 'Trimmed to easy: HRV down, sleep short.',
      },
      rationale: 'HRV is 18% below baseline and sleep was short.',
      source: 'ai',
      stale: false,
    };
    expect(briefing.readiness_color).toBe('amber');
    expect(briefing.action).toBe('SOFTEN');
    expect(briefing.drivers.hrv_delta_pct).toBe(-17.8);
    expect(briefing.effective_session?.distance_km).toBe(4.5);
  });

  it('TodayBriefing allows null sessions on a rest day', () => {
    const briefing: TodayBriefing = {
      date: '2026-06-21',
      readiness_color: 'green',
      drivers: {
        date: '2026-06-21',
        sleep_hours: null, sleep_score: null,
        hrv_last_night_ms: null, hrv_baseline_ms: null, hrv_delta_pct: null,
        rhr_last_night: null, rhr_baseline: null, rhr_delta_bpm: null,
        body_battery_high: null,
        recovery_trend: 'stable', data_complete: false,
      },
      reasons: [],
      action: 'REST_DAY',
      original_session: null,
      effective_session: null,
      rationale: 'Rest day as planned.',
      source: 'fallback',
      stale: true,
    };
    expect(briefing.action).toBe('REST_DAY');
    expect(briefing.original_session).toBeNull();
    expect(briefing.drivers.data_complete).toBe(false);
  });

  it('PushRegisterRequest carries the token and platform', () => {
    const req: PushRegisterRequest = {
      expo_push_token: 'ExponentPushToken[abc]',
      platform: 'ios',
    };
    expect(req.platform).toBe('ios');
  });

  it('RunResult matches the agent.RunResult shape', () => {
    const r: RunResult = {
      date: '2026-06-20', skipped: false,
      readiness_color: 'amber', action: 'SOFTEN', source: 'ai',
      stale: false, pushed: true, error: null,
    };
    expect(r.pushed).toBe(true);
  });

  it('AthleteProfile exposes the M2 daily-run-time/timezone/agent fields', () => {
    const p: AthleteProfile = {
      target_weekly_km: 20, progression_mode: 'build',
      zone2_ceiling_bpm: null, threshold_bpm: null, max_hr_bpm: null,
      run_constraints_json: '{}', goal_text: '',
      daily_run_time: '05:30', timezone: 'Asia/Seoul', agent_enabled: true,
      updated_at: '2026-06-20T05:00:00Z',
    };
    expect(p.daily_run_time).toBe('05:30');
    expect(p.timezone).toBe('Asia/Seoul');
    expect(p.agent_enabled).toBe(true);
  });
});
```

- [ ] **Step 2: Run it — expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/app && npx tsc --noEmit
```
Expected: FAIL — `Module '"../types"' has no exported member 'ReadinessColor'` (and `TodayBriefing`/`daily_run_time` unknown).

- [ ] **Step 3: Add the M2 types and extend AthleteProfile.** Append to `/home/jake/project/help-my-run/app/src/api/types.ts`:
```ts
// --- M2 types (snake_case wire JSON) ---

export type ReadinessColor = 'green' | 'amber' | 'red';
export type DailyAction = 'STAND' | 'SOFTEN' | 'MOVE' | 'REST_DAY';

export interface ReadinessDrivers {
  date: string;
  sleep_hours: number | null;
  sleep_score: number | null;
  hrv_last_night_ms: number | null;
  hrv_baseline_ms: number | null;
  hrv_delta_pct: number | null;
  rhr_last_night: number | null;
  rhr_baseline: number | null;
  rhr_delta_bpm: number | null;
  body_battery_high: number | null;
  recovery_trend: string;
  data_complete: boolean;
}

export interface TodayBriefing {
  date: string;
  readiness_color: ReadinessColor;
  drivers: ReadinessDrivers;
  reasons: string[];
  action: DailyAction;
  original_session: PlanDay | null;
  effective_session: PlanDay | null;
  rationale: string;
  source: 'ai' | 'fallback';
  stale: boolean;
}

export interface PushRegisterRequest {
  expo_push_token: string;
  platform: 'ios' | 'android';
}

export interface RunResult {
  date: string;
  skipped: boolean;
  readiness_color: ReadinessColor;
  action: DailyAction;
  source: 'ai' | 'fallback';
  stale: boolean;
  pushed: boolean;
  error: string | null;
}
```
Then replace the existing `AthleteProfile` interface with:
```ts
export interface AthleteProfile {
  target_weekly_km: number;
  progression_mode: 'build' | 'hold';
  zone2_ceiling_bpm: number | null;
  threshold_bpm: number | null;
  max_hr_bpm: number | null;
  run_constraints_json: string;
  goal_text: string;
  daily_run_time: string;
  timezone: string;
  agent_enabled: boolean;
  updated_at?: string;
}
```

- [ ] **Step 4: Run it — expect PASS (jest M2 block).** Command:
```
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/types.test.ts
```
Expected: jest passes the M2 block. (Repo-wide `tsc --noEmit` may still flag other `AthleteProfile` literals missing the 3 new required fields; those are fixed in Tasks 28–32. Run the full `tsc` after Task 32.)

- [ ] **Step 5: Commit.** Command:
```
git add app/src/api/types.ts app/src/api/__tests__/types.test.ts && git commit -m "feat(app): add M2 Today/Readiness/Push types + profile fields (APP)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 28: Push-token registration lib helper + MANUAL real-delivery verification

**Files:**
- Create: `/home/jake/project/help-my-run/app/src/lib/notifications.ts`
- Test: `/home/jake/project/help-my-run/app/app/__tests__/notifications.test.ts` (create)

- [ ] **Step 1: Write the failing notifications test.** Create `/home/jake/project/help-my-run/app/app/__tests__/notifications.test.ts`:
```ts
import { Platform } from 'react-native';

const mockSetChannel = jest.fn();
const mockGetPerms = jest.fn();
const mockReqPerms = jest.fn();
const mockGetToken = jest.fn();
const mockSetHandler = jest.fn();

jest.mock('expo-notifications', () => ({
  setNotificationChannelAsync: (...a: unknown[]) => mockSetChannel(...a),
  getPermissionsAsync: () => mockGetPerms(),
  requestPermissionsAsync: () => mockReqPerms(),
  getExpoPushTokenAsync: (...a: unknown[]) => mockGetToken(...a),
  setNotificationHandler: (...a: unknown[]) => mockSetHandler(...a),
  AndroidImportance: { MAX: 5 },
}));

jest.mock('expo-constants', () => ({
  __esModule: true,
  default: { expoConfig: { extra: { eas: { projectId: 'proj-123' } } }, easConfig: undefined },
}));

const mockApiPost = jest.fn();
jest.mock('../../src/api/client', () => ({
  apiPost: (...a: unknown[]) => mockApiPost(...a),
}));

import { registerForPushNotificationsAsync } from '../../src/lib/notifications';

afterEach(() => {
  jest.clearAllMocks();
  Platform.OS = 'ios';
});

describe('registerForPushNotificationsAsync', () => {
  it('returns null and does not POST when permission is denied', async () => {
    mockGetPerms.mockResolvedValue({ status: 'denied' });
    mockReqPerms.mockResolvedValue({ status: 'denied' });

    const token = await registerForPushNotificationsAsync();

    expect(token).toBeNull();
    expect(mockGetToken).not.toHaveBeenCalled();
    expect(mockApiPost).not.toHaveBeenCalled();
  });

  it('requests permission only when not already granted', async () => {
    mockGetPerms.mockResolvedValue({ status: 'granted' });
    mockGetToken.mockResolvedValue({ data: 'ExponentPushToken[abc]', type: 'expo' });
    mockApiPost.mockResolvedValue({});

    await registerForPushNotificationsAsync();

    expect(mockReqPerms).not.toHaveBeenCalled();
  });

  it('gets the token with the projectId and POSTs it (ios platform)', async () => {
    mockGetPerms.mockResolvedValue({ status: 'granted' });
    mockGetToken.mockResolvedValue({ data: 'ExponentPushToken[abc]', type: 'expo' });
    mockApiPost.mockResolvedValue({});

    const token = await registerForPushNotificationsAsync();

    expect(mockGetToken).toHaveBeenCalledWith({ projectId: 'proj-123' });
    expect(token).toBe('ExponentPushToken[abc]');
    expect(mockApiPost).toHaveBeenCalledWith('/api/push/register', {
      expo_push_token: 'ExponentPushToken[abc]',
      platform: 'ios',
    });
  });

  it('creates the android channel BEFORE requesting permission on android', async () => {
    Platform.OS = 'android';
    const order: string[] = [];
    mockSetChannel.mockImplementation(async () => { order.push('channel'); });
    mockGetPerms.mockImplementation(async () => { order.push('getPerms'); return { status: 'denied' }; });
    mockReqPerms.mockImplementation(async () => { order.push('reqPerms'); return { status: 'denied' }; });

    await registerForPushNotificationsAsync();

    expect(mockSetChannel).toHaveBeenCalledWith('default', expect.objectContaining({ name: 'default' }));
    expect(order[0]).toBe('channel');
    expect(order.indexOf('channel')).toBeLessThan(order.indexOf('getPerms'));
  });

  it('returns null (no throw) when projectId is missing', async () => {
    jest.resetModules();
    jest.doMock('expo-constants', () => ({
      __esModule: true,
      default: { expoConfig: { extra: {} }, easConfig: undefined },
    }));
    jest.doMock('expo-notifications', () => ({
      setNotificationChannelAsync: jest.fn(),
      getPermissionsAsync: () => Promise.resolve({ status: 'granted' }),
      requestPermissionsAsync: jest.fn(),
      getExpoPushTokenAsync: jest.fn(),
      setNotificationHandler: jest.fn(),
      AndroidImportance: { MAX: 5 },
    }));
    jest.doMock('../../src/api/client', () => ({ apiPost: jest.fn() }));
    const mod = require('../../src/lib/notifications');
    const token = await mod.registerForPushNotificationsAsync();
    expect(token).toBeNull();
  });
});
```

- [ ] **Step 2: Run it — expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/notifications.test.ts
```
Expected: FAIL — `Cannot find module '../../src/lib/notifications'`.

- [ ] **Step 3: Implement the lib helper.** Create `/home/jake/project/help-my-run/app/src/lib/notifications.ts`:
```ts
import * as Notifications from 'expo-notifications';
import Constants from 'expo-constants';
import { Platform } from 'react-native';
import { apiPost } from '../api/client';
import type { PushRegisterRequest } from '../api/types';

// Module-scope handler: how to present a notification received while the app is
// foregrounded. SDK 56 uses shouldShowBanner/shouldShowList (shouldShowAlert removed).
Notifications.setNotificationHandler({
  handleNotification: async () => ({
    shouldShowBanner: true,
    shouldShowList: true,
    shouldPlaySound: true,
    shouldSetBadge: false,
  }),
});

function getProjectId(): string | null {
  const fromExpoConfig = (Constants?.expoConfig as { extra?: { eas?: { projectId?: string } } } | undefined)
    ?.extra?.eas?.projectId;
  const fromEasConfig = (Constants?.easConfig as { projectId?: string } | undefined)?.projectId;
  return fromExpoConfig ?? fromEasConfig ?? null;
}

/**
 * Requests notification permission, obtains the Expo push token, and POSTs it to
 * the backend. Returns the token string on success, or null on simulator /
 * denied permission / missing projectId (push needs a dev build, not Expo Go).
 */
export async function registerForPushNotificationsAsync(): Promise<string | null> {
  if (Platform.OS === 'android') {
    await Notifications.setNotificationChannelAsync('default', {
      name: 'default',
      importance: Notifications.AndroidImportance.MAX,
      vibrationPattern: [0, 250, 250, 250],
      lightColor: '#FF231F7C',
    });
  }

  const { status: existingStatus } = await Notifications.getPermissionsAsync();
  let finalStatus = existingStatus;
  if (existingStatus !== 'granted') {
    const { status } = await Notifications.requestPermissionsAsync();
    finalStatus = status;
  }
  if (finalStatus !== 'granted') return null;

  const projectId = getProjectId();
  if (!projectId) return null;

  const tokenResp = await Notifications.getExpoPushTokenAsync({ projectId });
  const token = tokenResp.data;

  const body: PushRegisterRequest = {
    expo_push_token: token,
    platform: Platform.OS === 'android' ? 'android' : 'ios',
  };
  await apiPost('/api/push/register', body);

  return token;
}
```
Note: imports `apiPost` from `../api/client` — confirm that's the M1 client module path/name.

- [ ] **Step 4: Run it — expect PASS.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/notifications.test.ts
```
Expected: `5 passed`.

- [ ] **Step 5: MANUAL verification — real push delivery on a dev build (not unit-testable).** Performed by a human on hardware; record the result.
  1. On the backend host, run `eas init` (or `eas build:configure`) once to write the real `expo.extra.eas.projectId` UUID into `app.json` (replacing the placeholder from Task 26).
  2. Build an EAS development build on a physical device (remote push does NOT work in Expo Go SDK 53+, nor on the iOS Simulator):
     ```
     cd /home/jake/project/help-my-run/app && eas build --profile development --platform ios
     ```
     (or `--platform android`). Install on a physical device.
  3. Launch the app, set the backend URL + API token in Settings, accept the notification permission prompt.
  4. Confirm registration landed:
     ```
     sqlite3 /home/jake/project/help-my-run/backend/helpmyrun.db "SELECT expo_push_token, platform FROM device_tokens;"
     ```
     Expect one `ExponentPushToken[...]` row with the correct platform.
  5. Trigger a real push end-to-end:
     ```
     curl -s -X POST -H "Authorization: Bearer $API_TOKEN" "http://localhost:8080/api/agent/run?force=true"
     ```
     Expect `"pushed": true` in the JSON and a morning-briefing notification on the device. PASS = notification visibly received.
  6. If `pushed` is true but nothing arrives, verify the Android channel id `default` matches `push.Message.ChannelID` and that the dev build (not Expo Go) is installed.

- [ ] **Step 6: Commit.** Command:
```
git add app/src/lib/notifications.ts app/app/__tests__/notifications.test.ts && git commit -m "feat(app): push-token registration helper + notification handler (APP)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 29: Wire push registration into root _layout.tsx on launch

**Files:**
- Modify: `/home/jake/project/help-my-run/app/app/_layout.tsx` (`useEffect` calling `registerForPushNotificationsAsync`)
- Modify: `/home/jake/project/help-my-run/app/app/__tests__/_layout.test.tsx` (notifications mock + registration-on-mount test)

- [ ] **Step 1: Add a failing registration test to _layout.test.tsx.** In `/home/jake/project/help-my-run/app/app/__tests__/_layout.test.tsx`, add the mock at the top (after the existing `expo-router` mock, before `import RootLayout`):
```tsx
const mockRegister = jest.fn(() => Promise.resolve('ExponentPushToken[abc]'));
jest.mock('../../src/lib/notifications', () => ({
  registerForPushNotificationsAsync: () => mockRegister(),
}));
```
Add this test inside the `describe('RootLayout', ...)` block:
```tsx
  it('registers for push notifications once on mount', async () => {
    await render(<RootLayout />);
    expect(mockRegister).toHaveBeenCalledTimes(1);
  });
```

- [ ] **Step 2: Run it — expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/_layout.test.tsx
```
Expected: FAIL — `expect(mockRegister).toHaveBeenCalledTimes(1)` got `0`.

- [ ] **Step 3: Wire the useEffect into RootLayout.** In `/home/jake/project/help-my-run/app/app/_layout.tsx`, add the import + a `useEffect` that fires registration once (preserve the existing `QueryClientProvider`/`Stack` tree exactly — only add the import and the effect):
```tsx
import { useEffect } from 'react';
import { registerForPushNotificationsAsync } from '../src/lib/notifications';
```
Inside `RootLayout`, before the `return`:
```tsx
  useEffect(() => {
    // Fire-and-forget: register the device's Expo push token on launch. Returns
    // null (no-op) on simulator / denied / missing projectId.
    registerForPushNotificationsAsync().catch(() => {
      // best-effort; the app works without push and still fetches Today over the API.
    });
  }, []);
```

- [ ] **Step 4: Run it — expect PASS.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/_layout.test.tsx
```
Expected: PASS — existing layout tests + the new registration test.

- [ ] **Step 5: Commit.** Command:
```
git add app/app/_layout.tsx app/app/__tests__/_layout.test.tsx && git commit -m "feat(app): register push token on launch in root layout (APP)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 30: API hooks — useToday, useUndoToday, useRunAgent, useRegisterPushToken

**Files:**
- Modify: `/home/jake/project/help-my-run/app/src/api/hooks.ts` (add 4 hooks; extend type imports)
- Modify: `/home/jake/project/help-my-run/app/src/api/__tests__/hooks.test.tsx` (add 4 describe blocks; extend imports)

- [ ] **Step 1: Write the failing hooks tests.** In `/home/jake/project/help-my-run/app/src/api/__tests__/hooks.test.tsx`, extend the `from '../hooks'` import to add `useToday`, `useUndoToday`, `useRunAgent`, `useRegisterPushToken`, and the `from '../types'` import to add `TodayBriefing`, `RunResult`, `PushRegisterRequest`. Append:
```tsx
const todayBriefing: TodayBriefing = {
  date: '2026-06-20',
  readiness_color: 'amber',
  drivers: {
    date: '2026-06-20',
    sleep_hours: 6.1, sleep_score: 62,
    hrv_last_night_ms: 48, hrv_baseline_ms: 58.4, hrv_delta_pct: -17.8,
    rhr_last_night: 54, rhr_baseline: 50.2, rhr_delta_bpm: 3.8,
    body_battery_high: 61, recovery_trend: 'declining', data_complete: true,
  },
  reasons: ['HRV -17.8% vs baseline'],
  action: 'SOFTEN',
  original_session: null,
  effective_session: null,
  rationale: 'Trimmed.',
  source: 'ai',
  stale: false,
};

describe('useToday', () => {
  it('fetches /api/today', async () => {
    mockApiGet.mockResolvedValue(todayBriefing);
    const { result } = await renderHook(() => useToday(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/today');
    expect(result.current.data).toEqual(todayBriefing);
  });
});

describe('useUndoToday', () => {
  it('POSTs /api/today/undo and returns the reverted briefing', async () => {
    const reverted: TodayBriefing = { ...todayBriefing, action: 'STAND' };
    mockApiPost.mockResolvedValue(reverted);
    const { result } = await renderHook(() => useUndoToday(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate(); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/today/undo');
    expect(result.current.data).toEqual(reverted);
  });
});

describe('useRunAgent', () => {
  it('POSTs /api/agent/run and returns the run result', async () => {
    const runResult: RunResult = {
      date: '2026-06-20', skipped: false, readiness_color: 'amber',
      action: 'SOFTEN', source: 'ai', stale: false, pushed: true, error: null,
    };
    mockApiPost.mockResolvedValue(runResult);
    const { result } = await renderHook(() => useRunAgent(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate(); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/agent/run');
    expect(result.current.data).toEqual(runResult);
  });
});

describe('useRegisterPushToken', () => {
  it('POSTs /api/push/register with the token + platform body', async () => {
    const body: PushRegisterRequest = {
      expo_push_token: 'ExponentPushToken[abc]', platform: 'ios',
    };
    mockApiPost.mockResolvedValue(body);
    const { result } = await renderHook(() => useRegisterPushToken(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate(body); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/push/register', body);
  });
});
```
Note: reuse the existing test harness names (`mockApiGet`/`mockApiPost`/`renderHook`/`createWrapper`/`waitFor`/`act`); add `act` to the `@testing-library/react-native` import if not already present.

- [ ] **Step 2: Run it — expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/hooks.test.tsx
```
Expected: FAIL — `(0 , _hooks.useToday) is not a function` (and the other three undefined).

- [ ] **Step 3: Implement the four hooks.** In `/home/jake/project/help-my-run/app/src/api/hooks.ts`, extend the `from './types'` import to add `TodayBriefing`, `RunResult`, `PushRegisterRequest`, and append:
```ts
export function useToday() {
  return useQuery({
    queryKey: ['today'],
    queryFn: () => apiGet<TodayBriefing>('/api/today'),
  });
}

export function useUndoToday() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<TodayBriefing>('/api/today/undo'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['today'] });
    },
  });
}

export function useRunAgent() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => apiPost<RunResult>('/api/agent/run'),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['today'] });
    },
  });
}

export function useRegisterPushToken() {
  return useMutation({
    mutationFn: (body: PushRegisterRequest) =>
      apiPost<PushRegisterRequest>('/api/push/register', body),
  });
}
```
Note: match the existing `apiGet`/`apiPost`/`useQuery`/`useMutation`/`useQueryClient` import names already used in `hooks.ts`.

- [ ] **Step 4: Run it — expect PASS.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/hooks.test.tsx
```
Expected: PASS — existing + 4 new tests.

- [ ] **Step 5: Commit.** Command:
```
git add app/src/api/hooks.ts app/src/api/__tests__/hooks.test.tsx && git commit -m "feat(app): add useToday/useUndoToday/useRunAgent/useRegisterPushToken hooks (APP)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 31: Today card on Home (app/index.tsx)

**Files:**
- Modify: `/home/jake/project/help-my-run/app/app/index.tsx` (add `<TodayCard>` consuming `useToday`/`useUndoToday`; readiness pill + drivers + what-changed + why + Undo button `testID="btn-today-undo"`)
- Modify: `/home/jake/project/help-my-run/app/app/__tests__/index.test.tsx` (mock `useToday`/`useUndoToday`; add Today-card tests)

Today is a card on `index.tsx` per spec §8 — no new route in `_layout.tsx`.

- [ ] **Step 1: Write the failing Today-card tests.** In `/home/jake/project/help-my-run/app/app/__tests__/index.test.tsx`, add the fixture + mutate spy before the `jest.mock('../../src/api/hooks', ...)` call:
```tsx
import type { TodayBriefing } from '../../src/api/types';

const todayData: TodayBriefing = {
  date: '2026-06-20',
  readiness_color: 'amber',
  drivers: {
    date: '2026-06-20',
    sleep_hours: 6.1, sleep_score: 62,
    hrv_last_night_ms: 48, hrv_baseline_ms: 58.4, hrv_delta_pct: -17.8,
    rhr_last_night: 54, rhr_baseline: 50.2, rhr_delta_bpm: 3.8,
    body_battery_high: 61, recovery_trend: 'declining', data_complete: true,
  },
  reasons: ['HRV -17.8% vs baseline', 'Sleep score 62 (<65)'],
  action: 'SOFTEN',
  original_session: {
    date: '2026-06-20', dow: 'Fri', run_type: 'tempo', distance_km: 6,
    pace_target: '5:05/km', time_note: '~20:00 after CrossFit',
    optional_if_cns: false, rationale: 'Threshold work.',
  },
  effective_session: {
    date: '2026-06-20', dow: 'Fri', run_type: 'easy', distance_km: 4.5,
    pace_target: '6:00/km', time_note: '~20:00 after CrossFit',
    optional_if_cns: true, rationale: 'Trimmed to easy.',
  },
  rationale: 'HRV is 18% below baseline and sleep was short.',
  source: 'ai',
  stale: false,
};

const mockUndoMutate = jest.fn();
```
Replace the existing `jest.mock('../../src/api/hooks', ...)` block to add the two hooks (keep the existing mocked hooks):
```tsx
jest.mock('../../src/api/hooks', () => ({
  useStatus: () => ({ data: statusData, isPending: false, isError: false }),
  useActivities: () => ({ data: activitiesData, isPending: false, isError: false }),
  useRecovery: () => ({ data: recoveryData, isPending: false, isError: false }),
  useToday: () => ({ data: todayData, isPending: false, isError: false }),
  useUndoToday: () => ({ mutate: mockUndoMutate, isPending: false }),
}));
```
Extend the `@testing-library/react-native` import to include `fireEvent, act`, add `afterEach(() => { jest.clearAllMocks(); });` after the `jest.mock` block, and add inside `describe('HomeScreen', ...)`:
```tsx
  it('renders the readiness color pill', async () => {
    const { getByTestId } = await render(<HomeScreen />);
    expect(getByTestId('today-readiness').props.children).toContain('amber');
  });

  it('renders the readiness reasons (drivers)', async () => {
    const { getByText } = await render(<HomeScreen />);
    expect(getByText('HRV -17.8% vs baseline')).toBeTruthy();
    expect(getByText('Sleep score 62 (<65)')).toBeTruthy();
  });

  it('renders what-changed: original tempo 6 vs effective easy 4.5', async () => {
    const { getByTestId } = await render(<HomeScreen />);
    expect(getByTestId('today-original').props.children.join('')).toContain('tempo');
    expect(getByTestId('today-effective').props.children.join('')).toContain('easy');
    expect(getByTestId('today-effective').props.children.join('')).toContain('4.5');
  });

  it('renders the why (rationale)', async () => {
    const { getByTestId } = await render(<HomeScreen />);
    expect(getByTestId('today-rationale').props.children).toContain('HRV is 18% below baseline');
  });

  it('calls undo when the Undo button is pressed', async () => {
    const { getByTestId } = await render(<HomeScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-today-undo'));
    });
    expect(mockUndoMutate).toHaveBeenCalledTimes(1);
  });
```

- [ ] **Step 2: Run it — expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/index.test.tsx
```
Expected: FAIL — `Unable to find an element with testID: today-readiness`; `mockUndoMutate` not called.

- [ ] **Step 3: Add the Today card to index.tsx.** In `/home/jake/project/help-my-run/app/app/index.tsx`, add `useToday`/`useUndoToday` to the `../src/api/hooks` import, add `Pressable` to the `react-native` import if absent, import `PlanDay` from `../src/api/types`, and add a `TodayCard` component rendered at the top of `HomeScreen`'s scroll view. Use these exact testIDs: `today-readiness` (pill, child text = the lowercase color), `today-original`, `today-effective` (each a `<Text>` whose children array joins to include the session summary), `today-rationale`, optional `today-stale`, and the Undo `Pressable` `testID="btn-today-undo"` (shown only when `action` is not STAND/REST_DAY and both sessions are non-null, calling `undo.mutate()`):
```tsx
const READINESS_BG: Record<string, string> = { green: '#1b8a3a', amber: '#d39000', red: '#c0392b' };

function fmtSession(s: PlanDay | null): string {
  if (!s) return 'rest';
  return `${s.run_type} ${s.distance_km}km @ ${s.pace_target}`;
}

function TodayCard() {
  const today = useToday();
  const undo = useUndoToday();
  const b = today.data;
  if (today.isPending) {
    return (
      <View style={styles.todayCard}><Text style={styles.todayLoading}>Loading today…</Text></View>
    );
  }
  if (!b) {
    return (
      <View style={styles.todayCard}><Text style={styles.empty}>No briefing for today yet</Text></View>
    );
  }
  const changed = b.action !== 'STAND' && b.action !== 'REST_DAY' && b.original_session != null && b.effective_session != null;
  return (
    <View style={styles.todayCard}>
      <View style={styles.todayHeaderRow}>
        <Text testID="today-readiness" style={[styles.todayPill, { backgroundColor: READINESS_BG[b.readiness_color] ?? '#666' }]}>
          {b.readiness_color}
        </Text>
        <Text style={styles.todayAction}>{b.action}</Text>
        {b.stale ? <Text testID="today-stale" style={styles.todayStale}>stale data</Text> : null}
      </View>
      {b.reasons.length > 0 ? (
        <View style={styles.todayReasons}>
          {b.reasons.map((r) => (<Text key={r} style={styles.todayReason}>• {r}</Text>))}
        </View>
      ) : null}
      <Text testID="today-original" style={styles.todayLine}>Original: {fmtSession(b.original_session)}</Text>
      <Text testID="today-effective" style={styles.todayLineStrong}>Today: {fmtSession(b.effective_session)}</Text>
      <Text testID="today-rationale" style={styles.todayRationale}>{b.rationale}</Text>
      {changed ? (
        <Pressable testID="btn-today-undo" style={styles.undoButton} disabled={undo.isPending} onPress={() => undo.mutate()}>
          <Text style={styles.undoButtonText}>{undo.isPending ? 'Reverting…' : 'Undo (revert to original)'}</Text>
        </Pressable>
      ) : null}
    </View>
  );
}
```
Render `<Text style={styles.heading}>Today</Text>` + `<TodayCard />` as the first children of the `HomeScreen` scroll container, and add the corresponding `styles` entries (`todayCard`, `todayLoading`, `todayHeaderRow`, `todayPill`, `todayAction`, `todayStale`, `todayReasons`, `todayReason`, `todayLine`, `todayLineStrong`, `todayRationale`, `undoButton`, `undoButtonText`) to the existing `StyleSheet.create({...})`. Keep all existing Home content (connection/status/links/recent runs/recovery) intact.

- [ ] **Step 4: Run it — expect PASS.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/index.test.tsx
```
Expected: PASS — existing Home tests + 5 new Today-card tests.

- [ ] **Step 5: Commit.** Command:
```
git add app/app/index.tsx app/app/__tests__/index.test.tsx && git commit -m "feat(app): Today card on Home (readiness, what-changed, why, Undo) (APP)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 32: Settings additions — daily run time, timezone, agent enable toggle

**Files:**
- Modify: `/home/jake/project/help-my-run/app/app/settings.tsx` (add `useProfile`/`useUpdateProfile`; daily-run-time input `testID="input-daily-run-time"`, timezone input `testID="input-timezone"`, agent toggle `testID="toggle-agent-enabled"`, save button `testID="btn-save-agent"`)
- Modify: `/home/jake/project/help-my-run/app/app/__tests__/settings.test.tsx` (mock `useProfile`/`useUpdateProfile`; add tests)
- Modify: `/home/jake/project/help-my-run/app/app/__tests__/profile.test.tsx` + any other `AthleteProfile` literals (add the 3 new required fields so `tsc` passes)

- [ ] **Step 1: Write the failing settings tests.** In `/home/jake/project/help-my-run/app/app/__tests__/settings.test.tsx`, add before the existing `jest.mock('../../src/api/hooks', ...)`:
```tsx
import type { AthleteProfile } from '../../src/api/types';

const profileData: AthleteProfile = {
  target_weekly_km: 20,
  progression_mode: 'build',
  zone2_ceiling_bpm: null,
  threshold_bpm: null,
  max_hr_bpm: null,
  run_constraints_json: '{}',
  goal_text: '',
  daily_run_time: '05:30',
  timezone: 'Asia/Seoul',
  agent_enabled: true,
  updated_at: '2026-06-20T05:00:00Z',
};

const mockProfileUpdateMutate = jest.fn();
```
Replace the existing hooks mock to add the two profile hooks (keep the existing mocked hooks for status/sync/connect):
```tsx
  useProfile: () => ({ data: profileData, isPending: false, isError: false }),
  useUpdateProfile: () => ({ mutate: mockProfileUpdateMutate, isPending: false }),
```
Add inside `describe('SettingsScreen', ...)`:
```tsx
  it('prefills the daily run time, timezone, and agent toggle from the profile', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    expect(getByTestId('input-daily-run-time').props.value).toBe('05:30');
    expect(getByTestId('input-timezone').props.value).toBe('Asia/Seoul');
    expect(getByTestId('toggle-agent-enabled').props.accessibilityState.checked).toBe(true);
  });

  it('saves the agent schedule with edited run time and toggled-off agent', async () => {
    const { getByTestId } = await render(<SettingsScreen />);
    await act(async () => { fireEvent.changeText(getByTestId('input-daily-run-time'), '06:15'); });
    await act(async () => { fireEvent.changeText(getByTestId('input-timezone'), 'UTC'); });
    await act(async () => { fireEvent.press(getByTestId('toggle-agent-enabled')); });
    await act(async () => { fireEvent.press(getByTestId('btn-save-agent')); });
    expect(mockProfileUpdateMutate).toHaveBeenCalledTimes(1);
    const arg = mockProfileUpdateMutate.mock.calls[0][0] as AthleteProfile;
    expect(arg.daily_run_time).toBe('06:15');
    expect(arg.timezone).toBe('UTC');
    expect(arg.agent_enabled).toBe(false);
    expect(arg.target_weekly_km).toBe(20);
    expect(arg.progression_mode).toBe('build');
  });
```
Note: if the installed RN surfaces `Switch` state only as `props.value` (not `accessibilityState.checked`), change the prefill assertion to `expect(getByTestId('toggle-agent-enabled').props.value).toBe(true);` (verify against the first render).

- [ ] **Step 2: Run it — expect FAIL.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/settings.test.tsx
```
Expected: FAIL — `Unable to find an element with testID: input-daily-run-time`; agent save not called.

- [ ] **Step 3: Add the daily-coach section to settings.tsx.** In `/home/jake/project/help-my-run/app/app/settings.tsx`, add `useProfile`/`useUpdateProfile` to the `../src/api/hooks` import, add `Switch` to the `react-native` import, import `AthleteProfile` from `../src/api/types`, add local state (`dailyRunTime`, `timezone`, `agentEnabled`) seeded from `useProfile().data` via a `useEffect`, and add a "Daily coach" section with the three inputs + save button. `onSaveAgent` spreads the loaded profile and overrides the three fields so existing fields are preserved:
```tsx
  const profile = useProfile();
  const updateProfile = useUpdateProfile();
  const [dailyRunTime, setDailyRunTime] = useState('');
  const [timezone, setTimezone] = useState('');
  const [agentEnabled, setAgentEnabled] = useState(true);

  const loadedProfile = profile.data;
  useEffect(() => {
    if (loadedProfile) {
      setDailyRunTime(loadedProfile.daily_run_time);
      setTimezone(loadedProfile.timezone);
      setAgentEnabled(loadedProfile.agent_enabled);
    }
  }, [loadedProfile]);

  const onSaveAgent = () => {
    if (!loadedProfile) return;
    const body: AthleteProfile = {
      ...loadedProfile,
      daily_run_time: dailyRunTime,
      timezone,
      agent_enabled: agentEnabled,
    };
    updateProfile.mutate(body);
  };
```
And the JSX section (place near the other settings sections):
```tsx
      <Text style={styles.heading}>Daily coach</Text>
      <Text style={styles.label}>Daily run time (HH:MM, 24h local)</Text>
      <TextInput testID="input-daily-run-time" style={styles.input} autoCapitalize="none" autoCorrect={false}
        placeholder="05:30" value={dailyRunTime} onChangeText={setDailyRunTime} />
      <Text style={styles.label}>Timezone (IANA)</Text>
      <TextInput testID="input-timezone" style={styles.input} autoCapitalize="none" autoCorrect={false}
        placeholder="Asia/Seoul" value={timezone} onChangeText={setTimezone} />
      <View style={styles.toggleRow}>
        <Text style={styles.label}>Agent enabled</Text>
        <Switch testID="toggle-agent-enabled" value={agentEnabled} onValueChange={setAgentEnabled} />
      </View>
      <Pressable testID="btn-save-agent" style={styles.button} disabled={updateProfile.isPending} onPress={onSaveAgent}>
        <Text style={styles.buttonText}>{updateProfile.isPending ? 'Saving…' : 'Save daily coach'}</Text>
      </Pressable>
```
Add a `toggleRow` style (`{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: 8 }`) if not present. Keep all existing settings content intact.

- [ ] **Step 4: Run it — expect PASS.** Command:
```
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/settings.test.tsx
```
Expected: PASS — existing Settings tests + 2 new agent-schedule tests.

- [ ] **Step 5: Run the full app suite + type-check.** Commands:
```
cd /home/jake/project/help-my-run/app && npx tsc --noEmit && npx jest
```
Expected: `tsc` clean and all jest suites pass. Fix any remaining `AthleteProfile` literals flagged by `tsc` (e.g. in `profile.test.tsx`, `hooks.test.tsx` `useProfile`/`useUpdateProfile` cases, and `profile.tsx`'s `onSave` body — spread `...loaded` or add `daily_run_time: '05:30', timezone: 'UTC', agent_enabled: true`) under this task.

- [ ] **Step 6: Commit.** Command:
```
git add app/app/settings.tsx app/app/__tests__/settings.test.tsx app/app/__tests__/profile.test.tsx app/app/profile.tsx app/src/api/__tests__/hooks.test.tsx && git commit -m "feat(app): Settings daily-run-time + timezone + agent enable toggle (APP)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Definition of Done

Each M2 success criterion from spec §3 maps to the task(s) that satisfy it. The plan is done when every task's tests pass, `go test ./...` + `go vet ./...` + `gofmt -l` are clean (Task 24), `npx tsc --noEmit` + `npx jest` are clean (Task 32), and the manual on-device check below passes.

| Spec §3 criterion | Satisfied by |
|---|---|
| **§3.1** A daily job fires unattended at the configured wake time, idempotent (once per day). | Scheduler `nextFire`/`Run` with in-process guard (Tasks 16–17); persistent `agent_runs.last_run_date` guard in `RunDaily` + `TestRunDailyIdempotentSecondRunSkips` (Tasks 1–2, 14); profile-sourced schedule launched in `main.go` (Task 25); `AGENT_RUN_TIME`/`AGENT_TZ`/`AGENT_ENABLED` + profile fields (Tasks 3, 18, 20). |
| **§3.2** Assess readiness (GREEN/AMBER/RED) from last night's sleep / overnight HRV / RHR / Body Battery + trend, recording driver numbers. | Readiness engine `Assess` + drivers + thresholds + trend modifier (Tasks 4–7); `TestAssess`/`TestAssessDriverNumbers`; drivers persisted to `daily_decisions.drivers_json` (Tasks 1–2, 14). |
| **§3.3** Rewrite today's session (STAND/SOFTEN/MOVE/REST_DAY) and persist it as today's effective session, leaving the week untouched. | `coach.AdjustToday` (today-only) (Tasks 11–13); `agent.RunDaily` loads only today's PlanDay and persists `original`/`adjusted` session JSON (Task 14); `daily_decisions` schema keeps original + adjusted (Tasks 1–2). |
| **§3.4** A morning Expo push lands with readiness + what changed + why. | Push client (Task 10); `agent.pushBriefing` sends title (date+color) + body (rationale) + data (date/action) to every device token, dropping DeviceNotRegistered (Task 14); device-token registration API + app lib + launch wiring (Tasks 22, 26, 28, 29); push base URL config (Task 18) + wiring (Task 25). |
| **§3.5** App shows a Today briefing with Undo (revert to the original session). | `GET /api/today` + `POST /api/today/undo` handlers (Tasks 19, 21, 23); `useToday`/`useUndoToday` hooks (Task 30); Today card with readiness pill + drivers + what-changed + why + `btn-today-undo` (Task 31). |
| **§3.6** If Claude is unavailable, a deterministic fallback still decides today (RED→easy/rest, AMBER→trim, GREEN→stand); today is never undecided. | `readiness.Fallback` rule table + tests (Tasks 8–9); `coach.AdjustToday` falls back on any `claude -p` failure with `source="fallback"` (Task 13, `TestAdjustTodayFallback*`); agent's no-run REST_DAY branch (Task 14); `source` recorded in `daily_decisions` (Tasks 1–2). |

**Manual on-device push/run check (Task 28 Step 5 — the sole non-unit-testable surface):**
1. Host: `claude setup-token` once (unattended `claude -p`); `eas init` to write the real `projectId`.
2. `eas build --profile development` on a physical device (not Expo Go / Simulator); install; set backend URL + token in Settings; accept the notification prompt.
3. Confirm the token persisted: `sqlite3 .../helpmyrun.db "SELECT expo_push_token, platform FROM device_tokens;"` → one row.
4. Trigger end-to-end: `curl -s -X POST -H "Authorization: Bearer $API_TOKEN" "http://localhost:8080/api/agent/run?force=true"` → response `"pushed": true` AND a morning-briefing notification (readiness color + action + what-changed) visibly arrives on the device.
5. Open the app → the Today card shows the same readiness/what-changed/why; pressing Undo reverts the effective session to the original (`action` becomes STAND).

---

## Cross-cutting sequencing & reconciliation notes

- **Build-dependency order** (already reflected in the task numbering): store (1–3) → metrics export (4) → readiness (5–9) → push (10) → llm (11) → coach (12–13) → agent (14–15) → scheduler (16–17) → config (18) → api DTOs/profile (19–20) → api seams+routes/push/today (21–24) → main wiring (25) → app (26–32). Tasks 4, 10, 11, 16–18 are largely self-contained and may run earlier in parallel, but 14 needs 1–13, and 21–25 need the agent/readiness/coach/scheduler packages present.
- **`source` value on a no-run day:** `agent.RunDaily` sets `RunResult.Source="fallback"` AND stores `daily_decisions.source="fallback"` — the single `ai|fallback` union holds on every surface (Go `RunResult`, the TS `RunResult` union in Task 27, and the SQLite CHECK, which allows only `ai|fallback`); `action:"REST_DAY"` is how the API conveys a readiness-only briefing.
- **Reasons persistence:** option (A) — the agent embeds `reasons` inside `drivers_json` (`marshalDriversWithReasons`, Task 14); the API reads them with `parseReasons` (Task 23). No schema change. If a future task prefers a dedicated `reasons_json` column, amend Tasks 1/2/23 together.
- **`force` flag:** `POST /api/agent/run?force=true` is threaded via the `apiAgent` adapter in `main.go` (Task 25, option A), which deletes the `agent_runs` row (new `store.DeleteAgentRun`, added in Task 25) before calling `RunDaily`. The scheduler callback always runs non-forced.
- **Fallback rule duplication:** the canonical tested table lives in `readiness.Fallback` (Task 8); `coach.fallbackDecision` (Task 13) re-implements the identical table against `*llm.PlanDay`. Keep both byte-identical; reviewers should diff them. (An alternative is for `coach.fallbackDecision` to convert `*llm.PlanDay ↔ readiness.FallbackSession` and delegate to `readiness.Fallback` — acceptable if the executor prefers a single source of truth.)
- **M1 API verification before implementing:** several tasks assume M1 store/api accessor names (`store.GetLatestPlan`, `store.InsertPlan`/`store.Plan.PlanJSON`, `sync.SyncAll(ctx, store, *strava.Client, garmin.Runner, []string)`, `planDayDTO`, `coach.New(store, *llm.Client, model, imageDir)`, `llm.Client.Call`/`captureRunner`, `config.Config` field names). Verify each against the live M0/M1 code at the start of the dependent task and adapt the snippet to the real signature — the contracts above are the intent; the live names win.
