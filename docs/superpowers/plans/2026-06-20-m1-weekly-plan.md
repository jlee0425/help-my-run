# Milestone 1 (CrossFit-aware Weekly Run-Plan Generator) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upload the week's CrossFit box-programming photo and get a dated, progressive 7-day running plan that places runs intelligently around CrossFit load, with pace targets grounded in real computed fitness.

**Architecture:** M1 extends the M0 codebase on `main`. A deterministic Go metrics engine computes the trustworthy numbers (volume, acute:chronic load, paces, recovery trend, safe weekly target), while a coach engine shells out to the `claude -p` CLI in headless print mode — running under the user's Claude subscription (no API key) — for Stage-1 image→CrossFit-week vision and Stage-2 context-pack→plan generation. New Expo screens drive the plan/profile flow on top of M0's existing app.

**Tech Stack:** Go (chi router, `modernc.org/sqlite`, goose migrations); the `claude` CLI invoked headlessly via `os/exec` (`claude -p --output-format json`, subscription OAuth, no `ANTHROPIC_API_KEY`); Expo (expo-router, react-query, expo-image-picker).

---

## Setup Prerequisites

M1 builds on the **M0 `main` branch** already in this repo (`/home/jake/project/help-my-run`). Before running the M1 plan-generation flow on a host:

1. Install the Claude Code CLI on the host.
2. Log in **once** (interactive, opens a browser) under your Pro/Max subscription:

   ```bash
   claude auth login
   ```

   Do **NOT** use `claude auth login --console` — that switches to the metered API-key path. M1 deliberately uses the subscription OAuth credentials at `~/.claude/.credentials.json`.
3. Confirm the CLI is installed and reachable:

   ```bash
   claude --version
   ```

4. **No `ANTHROPIC_API_KEY` is required.** The M0 `AnthropicAPIKey` config field stays a stub (subscription path, unused). All Claude access in M1 goes through the logged-in `claude` CLI subprocess.

> **Headless / remote-host caveat.** Subscription-based `claude -p` works on a host where `claude auth login` has already been run interactively (e.g. your own dev workstation/laptop): `claude -p` reads the OAuth credentials at `~/.claude/.credentials.json`. On a **headless or remote/VPS/CI host** (no browser to complete the OAuth flow), run **`claude setup-token`** once — interactively, on a machine that *does* have a browser, under your Claude subscription — to mint a long-lived subscription token, then make that token available in the backend host's environment so `claude -p` can authenticate non-interactively. `ANTHROPIC_API_KEY` remains a paid (metered) fallback only. None of this affects `go test`: the CI test suite never invokes the real `claude` binary (it injects a stub `llm.Runner`).

All Go/CLI tests inject a stub `llm.Runner` and never invoke the real `claude` binary or network — `claude auth login` is only needed for the manual end-to-end check (see Definition of Done).

---

## File Structure

### Backend — new files
- `backend/internal/store/migrations/00002_m1_plan.sql` — M1 migration (athlete_profile, crossfit_weeks, plans)
- `backend/internal/store/migrations/00003_oauth_state.sql` — oauth_states table (M0 follow-up #1)
- `backend/internal/store/profile.go` — `AthleteProfile` struct + `Get/UpsertAthleteProfile`
- `backend/internal/store/profile_test.go` — temp-DB upsert/get tests
- `backend/internal/store/crossfit.go` — `CrossFitWeek` struct + `Get/UpsertCrossFitWeek`
- `backend/internal/store/crossfit_test.go` — temp-DB tests
- `backend/internal/store/plans.go` — `Plan` struct + `InsertPlan`/`GetLatestPlan`
- `backend/internal/store/plans_test.go` — temp-DB tests
- `backend/internal/store/activities_test.go` — `LatestActivityStartTime` tests
- `backend/internal/store/oauth_state.go` — `SaveOAuthState`/`ConsumeOAuthState`
- `backend/internal/store/oauth_state_test.go` — save/consume tests
- `backend/internal/llm/client.go` — `Runner`, `ExecRunner`, `Client`, `Envelope`, `ParseEnvelope`, `ExtractJSON`, `ClassifyFailure`, `Call`, error types
- `backend/internal/llm/types.go` — Stage-1/Stage-2 parsed structs + `Load` enum
- `backend/internal/llm/client_test.go` — envelope/extract/classify/ExecRunner stub tests
- `backend/internal/llm/call_test.go` — `Client.Call` success/failure + one-retry tests
- `backend/internal/llm/types_test.go` — parsed-struct round-trip tests
- `backend/internal/llm/testdata/stage1_envelope.json` — canned Stage-1 envelope (fenced JSON result)
- `backend/internal/llm/testdata/stage2_envelope.json` — canned Stage-2 envelope (prose+JSON result)
- `backend/internal/llm/testdata/malformed_envelope.json` — `.result` with bad JSON (retry test)
- `backend/internal/llm/testdata/not_logged_in_envelope.json` — `subtype:"success"` + `is_error:true`
- `backend/internal/metrics/metrics.go` — `FitnessMetrics` + `ComputeFitness` pure funcs + helpers
- `backend/internal/metrics/metrics_test.go` — table-driven fixture tests
- `backend/internal/coach/coach.go` — `ContextPack`, `ProfilePack`, `Coach` impl + arg builders
- `backend/internal/coach/prompts.go` — Stage-1 template + Coach Brain block
- `backend/internal/coach/prompts_test.go` — prompt-builder tests
- `backend/internal/coach/coach_test.go` — stubbed-runner orchestration tests
- `backend/internal/api/images.go` — `saveUploadedImage` multipart helper
- `backend/internal/api/images_test.go` — image-save tests
- `backend/internal/api/profile_handlers.go` — `GET/PUT /api/profile` handlers
- `backend/internal/api/plan_handlers.go` — crossfit/parse, plan/generate, plan, fitness handlers
- `backend/internal/api/m1_handlers_test.go` — httptest for the M1 endpoints + fakeCoach
- `app/app/plan.tsx` — Plan-my-week screen (pick/take photo → parse → edit CF week → generate)
- `app/app/plan-view.tsx` — Weekly plan view (per-day cards + rationale + regenerate)
- `app/app/profile.tsx` — Profile/settings form
- `app/app/__tests__/plan.test.tsx` — parse→edit→generate flow test
- `app/app/__tests__/plan-view.test.tsx` — plan-view render test
- `app/app/__tests__/profile.test.tsx` — profile-form test
- `app/src/api/__tests__/types.test.ts` — M1 type-shape conformance test
- `app/src/lib/imagePicker.ts` — `pickFromLibrary()`/`takePhoto()`/`toUploadFile()` wrappers
- `app/src/lib/__tests__/imagePicker.test.ts` — wrapper tests

### Backend — modified M0 files
- `backend/internal/config/config.go` — add `ClaudeBin`, `ClaudeModel`, `ImageDir`
- `backend/internal/config/config_test.go` — cover new defaults/explicit
- `backend/internal/store/activities.go` — add `LatestActivityStartTime()` (M0 follow-up #3)
- `backend/internal/store/store_test.go` — M1 migration table/seed tests
- `backend/internal/api/router.go` — extend `Deps` with `Coach`/`ImageDir`; `Coach` interface; register 6 M1 routes
- `backend/internal/api/dto.go` — add M1 response DTOs (snake_case)
- `backend/internal/api/handlers.go` — Strava OAuth state persist/validate (M0 follow-up #1)
- `backend/internal/api/handlers_test.go` — inject fakeCoach/ImageDir in `newTestServer`; state in callback test
- `backend/internal/sync/sync.go` — Strava cursor from `LatestActivityStartTime()` (M0 follow-up #3)
- `backend/internal/sync/sync_test.go` — cursor test
- `backend/cmd/server/main.go` — wire `llm.Client` + `coach`; sync-once-on-boot (M0 follow-up #2)
- `backend/cmd/server/main_test.go` — coach-wired + sync-on-boot tests
- `README.md` — document `claude auth login`, `CLAUDE_BIN`/`CLAUDE_MODEL`/`IMAGE_DIR`
- `.env.example` — Claude/image env additions
- `.gitignore` — add `data/`
- `app/package.json` — add `expo-image-picker`
- `app/app.json` — add `expo-image-picker` plugin (photos/camera permission strings)
- `app/app/_layout.tsx` — register `plan`/`profile`/`plan-view` `<Stack.Screen>`s
- `app/app/index.tsx` — add nav links to Plan/Profile
- `app/app/__tests__/index.test.tsx` — assert new nav links
- `app/src/api/client.ts` — add `apiUpload<T>` (multipart) + `apiPut<T>`
- `app/src/api/__tests__/client.test.ts` — `apiUpload`/`apiPut` tests
- `app/src/api/types.ts` — add `Load`, `CrossFitDay`, `CrossFitWeek`, `PlanDay`, `Plan`, `AthleteProfile`, `Fitness`
- `app/src/api/hooks.ts` — add `useProfile`, `useUpdateProfile`, `useFitness`, `usePlan`, `useParseCrossfit`, `useGeneratePlan`
- `app/src/api/__tests__/hooks.test.tsx` — extend client mock; cover new hooks

---

## Shared Contracts

> Authoritative interface definitions. Every Go struct field name, json tag, SQL column, CLI flag, prompt JSON key, REST path, and TS type below is canonical — copy verbatim. Consistent with the verified M0 codebase.
>
> **M0 DTO casing (important):** M0 DTOs use **snake_case** json tags (`strava_id`, `last_synced_at`, `body_battery`), NOT camelCase. The *only* camelCase tag in M0 is `authorizeUrl`. M1 DTOs therefore use **snake_case** to match the dominant M0 convention.

### Migration DDL — `backend/internal/store/migrations/00002_m1_plan.sql`

Next sequential prefix after `00001_init.sql`; picked up by the `//go:embed migrations/*.sql` glob. Dialect `sqlite3`, goose. Follow the exact `-- +goose` block structure of `00001_init.sql`, including the single `INSERT` seed for the single-row `athlete_profile`.

```sql
-- +goose Up
-- +goose StatementBegin

CREATE TABLE athlete_profile (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    target_weekly_km    REAL    NOT NULL DEFAULT 20,
    progression_mode    TEXT    NOT NULL DEFAULT 'build',
    zone2_ceiling_bpm   INTEGER,
    threshold_bpm       INTEGER,
    max_hr_bpm          INTEGER,
    run_constraints_json TEXT   NOT NULL DEFAULT '{}',
    goal_text           TEXT    NOT NULL DEFAULT '',
    updated_at          TEXT    NOT NULL
);

CREATE TABLE crossfit_weeks (
    week_start   TEXT PRIMARY KEY,
    image_path   TEXT,
    parsed_json  TEXT NOT NULL,
    raw_response TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE plans (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    week_start        TEXT    NOT NULL,
    generated_at      TEXT    NOT NULL,
    status            TEXT    NOT NULL DEFAULT 'generated',
    plan_json         TEXT    NOT NULL,
    fitness_summary   TEXT    NOT NULL DEFAULT '',
    context_pack_json TEXT,
    model             TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX idx_plans_week_start ON plans (week_start, generated_at DESC);

INSERT INTO athlete_profile
    (id, target_weekly_km, progression_mode, zone2_ceiling_bpm, threshold_bpm,
     max_hr_bpm, run_constraints_json, goal_text, updated_at)
VALUES
    (1, 20, 'build', NULL, NULL, NULL, '{}', '',
     strftime('%Y-%m-%dT%H:%M:%SZ', 'now'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE plans;
DROP TABLE crossfit_weeks;
DROP TABLE athlete_profile;
-- +goose StatementEnd
```

**Column semantics:**
- `athlete_profile`: single row enforced by `CHECK (id = 1)` (same idiom as `strava_tokens`). `progression_mode` ∈ `{'build','hold'}`. Nullable HR markers → Claude estimates from data. `run_constraints_json` is a JSON string (shape below).
- `crossfit_weeks`: `week_start` = ISO date of the Monday (`YYYY-MM-DD`), PK. `parsed_json` holds the Stage-1 CrossFit-week object. `raw_response` = the parsed Stage-1 week **re-marshaled to canonical JSON** (`json.Marshal(week)` from `ParseCrossFit`; it is NOT the byte-for-byte `claude -p` `.result`, since `Client.Call` extracts/validates the inner JSON and does not surface the raw envelope string to the coach). Nullable. `image_path` nullable.
- `plans`: `AUTOINCREMENT` id; multiple plans per `week_start` (regenerate appends; "latest" = `ORDER BY generated_at DESC LIMIT 1`). `plan_json` holds the Stage-2 plan object. `context_pack_json` = serialized context pack (nullable). `model` = `claude-opus-4-8`. `status` ∈ `{'generated','error'}`.

A second migration `00003_oauth_state.sql` (M0 follow-up #1) adds the `oauth_states` table:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE oauth_states (
    state      TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE oauth_states;
-- +goose StatementEnd
```

### `claude -p` invocation contract (`internal/llm`)

**Go client interface (`backend/internal/llm/client.go`):**

```go
package llm

// Runner executes one claude -p call. Injectable so tests stub it (mirrors
// garmin.Runner). The real impl shells out via os/exec; tests return canned envelopes.
type Runner interface {
    Run(ctx context.Context, args []string, stdin string) (stdout []byte, err error)
}

// ExecRunner is the production Runner backed by os/exec.
type ExecRunner struct {
    Bin string // claude binary path, from Config.ClaudeBin
}

// Client wraps a Runner with model/flag defaults and JSON extraction.
type Client struct {
    Runner  Runner
    Model   string // Config.ClaudeModel, e.g. "claude-opus-4-8"
    Timeout time.Duration // default 120s (matches chi middleware.Timeout)
}

// Envelope is the parsed `--output-format json` result envelope.
type Envelope struct {
    Type           string          `json:"type"`
    Subtype        string          `json:"subtype"`
    IsError        bool            `json:"is_error"`
    APIErrorStatus *int            `json:"api_error_status"`
    Result         string          `json:"result"`
    StopReason     string          `json:"stop_reason"`
    SessionID      string          `json:"session_id"`
    NumTurns       int             `json:"num_turns"`
    DurationMs     int             `json:"duration_ms"`
    TotalCostUSD   float64         `json:"total_cost_usd"`
}
```

**Stage 1 — image → CrossFit week:**

```bash
claude -p "<STAGE1_PROMPT>" \
  --model claude-opus-4-8 \
  --output-format json \
  --allowedTools "Read" \
  --add-dir "<IMAGE_DIR>" \
  --no-session-persistence
```
- Prompt passed as the **positional arg** (`exec.Command(bin, "-p", prompt, ...)`). The prompt instructs Claude to read the absolute image path (under `IMAGE_DIR`) and emit ONLY the Stage-1 JSON object.
- `--add-dir <IMAGE_DIR>` grants file-read access. `--allowedTools "Read"` auto-approves the `Read` tool so the call is non-interactive.
- Pass the **absolute** image path in the prompt (subprocess working dir is arbitrary).

**Stage 2 — context pack → plan:**

```bash
printf '%s' "<CONTEXT_PACK_JSON>" | claude -p "<COACH_BRAIN_PROMPT>" \
  --model claude-opus-4-8 \
  --output-format json \
  --allowedTools "" \
  --no-session-persistence
```
- Context pack (serialized JSON) is piped via **stdin**. The Coach Brain instruction block + "emit ONLY the plan JSON" is the **positional-arg** prompt. No file/tool access (`--allowedTools ""` auto-approves nothing).

> **Flags verified against `claude` CLI v2.1.x** (`--model`, `--output-format`, `--allowedTools`/`--allowed-tools`, `--add-dir`, `--no-session-persistence`, `-p`/`--print` all exist; **`--max-turns` does NOT exist in v2.1.x** and is omitted; `--tools` exists but is redundant with `--allowedTools` for our gating purpose, so we use `--allowedTools` only). Re-run `claude -p --help` if the CLI is upgraded.

**Hard rules (both stages):**
- **Do NOT** use `--bare` (ignores subscription OAuth, demands `ANTHROPIC_API_KEY`).
- **Do NOT** set `ANTHROPIC_API_KEY`, `--dangerously-skip-permissions`, or `--permission-mode bypassPermissions`.
- Auth is subscription OAuth at `~/.claude/.credentials.json` (`claude auth login`, NOT `claude auth login --console`).
- M1's canonical path is prompt-for-JSON + extract from `.result`. `--json-schema` is NOT part of the canonical contract.

**Success / failure determination:** Treat as **failure** when `exitCode != 0` **OR** `envelope.IsError == true`. (The "not logged in" case returns `subtype:"success"` with `is_error:true` — branch on `IsError`, not `subtype`.) Classify by substring-matching `.result`:
- contains `"login"` / `"Not logged in"` → "Claude not logged in — run `claude auth login`."
- `api_error_status` set (e.g. 429) or rate/usage-limit text → "Claude rate/usage limit hit — try later."
- binary not found (`exec.ErrNotFound`) → "`claude` CLI not installed."
- otherwise → generic error with the raw `.result` logged.

**Inner-JSON extraction rule (canonical, both stages):** On success, extract the model's JSON from `envelope.Result`:
1. `s := strings.TrimSpace(envelope.Result)`.
2. If `s` starts with a fenced block (` ```json ` or ` ``` `), strip the fence (content between the first newline after the opening fence and the closing ` ``` `).
3. Else if `s` does not start with `{`, take the substring from the first `{` to the last `}` (inclusive).
4. `json.Unmarshal([]byte(extracted), &target)`.
5. **On any extraction-or-unmarshal failure: retry the entire `claude -p` call exactly ONCE** (same args/prompt), then re-run steps 1–4. If the second attempt also fails → return `ErrMalformedJSON` (handler responds 502).

Exported helpers (coach + tests share them):
```go
func ExtractJSON(result string, v any) error // steps 1–4; no retry (caller owns retry loop)
func (c *Client) Call(ctx context.Context, args []string, stdin string, v any) error // full call + 1 retry on malformed JSON
```

### Stage 1 model-output JSON shape

| key | type | notes |
|---|---|---|
| `week_start` | string | ISO date of Monday, `YYYY-MM-DD` |
| `days` | array | exactly 7 objects, Mon→Sun |
| `days[].date` | string | `YYYY-MM-DD` |
| `days[].dow` | string | `"Mon"`,…,`"Sun"` |
| `days[].has_crossfit` | bool | |
| `days[].focus` | string | session focus; `""` if rest |
| `days[].cns_load` | enum string | `"low"`\|`"med"`\|`"high"` |
| `days[].leg_load` | enum string | `"low"`\|`"med"`\|`"high"` |
| `days[].notes` | string | free text; `""` if none |

```json
{
  "week_start": "2026-06-22",
  "days": [
    {"date":"2026-06-22","dow":"Mon","has_crossfit":true,"focus":"Back squat 5x5 + short metcon","cns_load":"high","leg_load":"high","notes":"Heavy legs"},
    {"date":"2026-06-23","dow":"Tue","has_crossfit":true,"focus":"Gymnastics + row intervals","cns_load":"med","leg_load":"low","notes":""},
    {"date":"2026-06-24","dow":"Wed","has_crossfit":true,"focus":"OHP + upper accessory","cns_load":"med","leg_load":"low","notes":""},
    {"date":"2026-06-25","dow":"Thu","has_crossfit":true,"focus":"Barbell skill / technique","cns_load":"low","leg_load":"low","notes":"Skill day, lighter"},
    {"date":"2026-06-26","dow":"Fri","has_crossfit":true,"focus":"Deadlift + conditioning","cns_load":"high","leg_load":"high","notes":""},
    {"date":"2026-06-27","dow":"Sat","has_crossfit":false,"focus":"","cns_load":"low","leg_load":"low","notes":"Rest"},
    {"date":"2026-06-28","dow":"Sun","has_crossfit":false,"focus":"","cns_load":"low","leg_load":"low","notes":"Rest"}
  ]
}
```

### Stage 2 model-output JSON shape

| key | type | notes |
|---|---|---|
| `fitness_summary` | string | 1–2 sentence read of current fitness |
| `weekly_target_km` | number | safe target for the week |
| `days` | array | exactly 7 objects, Mon→Sun |
| `days[].date` | string | `YYYY-MM-DD` |
| `days[].dow` | string | `"Mon"`…`"Sun"` |
| `days[].run_type` | string | e.g. `"easy"`,`"tempo"`,`"recovery"`,`"long"`,`"rest"`,`"intervals"` |
| `days[].distance_km` | number | `0` for rest |
| `days[].pace_target` | string | e.g. `"5:45/km"` or `"easy"`; `""` for rest |
| `days[].time_note` | string | evening-double note; `""` if none |
| `days[].optional_if_cns` | bool | true → skippable after a high-CNS day |
| `days[].rationale` | string | one-line why |
| `week_rationale` | string | paragraph: run placement + progression |
| `one_flag` | string | the single most important caution |

```json
{
  "fitness_summary": "~18 km/week over the last month, acute:chronic 1.05 (balanced). Easy ~6:00/km, threshold ~5:05/km. HRV and sleep trending up.",
  "weekly_target_km": 20,
  "days": [
    {"date":"2026-06-22","dow":"Mon","run_type":"rest","distance_km":0,"pace_target":"","time_note":"","optional_if_cns":false,"rationale":"Heavy squat day; no run."},
    {"date":"2026-06-23","dow":"Tue","run_type":"easy","distance_km":5,"pace_target":"6:00/km","time_note":"~20:00 after CrossFit","optional_if_cns":false,"rationale":"Low leg load; easy aerobic double."},
    {"date":"2026-06-24","dow":"Wed","run_type":"recovery","distance_km":4,"pace_target":"6:20/km","time_note":"~20:00 after CrossFit","optional_if_cns":true,"rationale":"Keep it light mid-week."},
    {"date":"2026-06-25","dow":"Thu","run_type":"tempo","distance_km":5,"pace_target":"5:05/km","time_note":"~20:00 after skill work","optional_if_cns":false,"rationale":"Thu is light CNS/legs — best quality slot."},
    {"date":"2026-06-26","dow":"Fri","run_type":"rest","distance_km":0,"pace_target":"","time_note":"","optional_if_cns":false,"rationale":"Heavy deadlift day."},
    {"date":"2026-06-27","dow":"Sat","run_type":"long","distance_km":8,"pace_target":"6:10/km","time_note":"Morning, fresh","optional_if_cns":false,"rationale":"Weekend long run, no CrossFit conflict."},
    {"date":"2026-06-28","dow":"Sun","run_type":"easy","distance_km":3,"pace_target":"6:15/km","time_note":"Morning","optional_if_cns":false,"rationale":"Easy shakeout to hit 25 km total."}
  ],
  "week_rationale": "Quality (tempo) placed Thursday (barbell-skill day, low CNS/legs) and the long run Saturday (no CrossFit). Hard runs kept off Mon/Fri heavy-leg days. Total ~25 km vs ~18 km baseline is a modest build within the 10% guideline given upward recovery trend.",
  "one_flag": "If Thursday skill work runs heavy, downgrade the tempo to easy."
}
```

### Go types

**Store structs** (`profile.go`, `crossfit.go`, `plans.go`). Pointers for nullable cols, `*Json string` for JSON columns, `Upsert*` via `ON CONFLICT ... DO UPDATE`, `Get*`/`List*` returning `ErrNotFound`.

```go
// profile.go
type AthleteProfile struct {
    TargetWeeklyKm     float64
    ProgressionMode    string  // "build" | "hold"
    Zone2CeilingBpm    *int64
    ThresholdBpm       *int64
    MaxHRBpm           *int64
    RunConstraintsJSON string  // JSON string
    GoalText           string
    UpdatedAt          string
}
func (s *Store) GetAthleteProfile() (AthleteProfile, error)      // id=1; ErrNotFound if missing
func (s *Store) UpsertAthleteProfile(p AthleteProfile) error     // INSERT id=1 ON CONFLICT(id) DO UPDATE

// crossfit.go
type CrossFitWeek struct {
    WeekStart   string
    ImagePath   *string
    ParsedJSON  string   // Stage-1 object verbatim
    RawResponse *string
    CreatedAt   string
    UpdatedAt   string
}
func (s *Store) GetCrossFitWeek(weekStart string) (CrossFitWeek, error) // ErrNotFound if none
func (s *Store) UpsertCrossFitWeek(w CrossFitWeek) error                // ON CONFLICT(week_start)

// plans.go
type Plan struct {
    ID              int64
    WeekStart       string
    GeneratedAt     string
    Status          string
    PlanJSON        string   // Stage-2 object verbatim
    FitnessSummary  string
    ContextPackJSON *string
    Model           string
}
func (s *Store) InsertPlan(p Plan) (int64, error)              // returns new id (AUTOINCREMENT)
func (s *Store) GetLatestPlan(weekStart string) (Plan, error)  // ORDER BY generated_at DESC LIMIT 1; ErrNotFound

// activities.go (NEW getter for M0 follow-up #3 — Strava cursor)
func (s *Store) LatestActivityStartTime() (string, error)      // max(start_time); ErrNotFound if no rows
```

**Stage 1 / Stage 2 parsed outputs** (`backend/internal/llm/types.go`). json tags MUST match the model keys exactly.

```go
type Load string // "low" | "med" | "high"
const (
    LoadLow  Load = "low"
    LoadMed  Load = "med"
    LoadHigh Load = "high"
)

// Stage 1
type CrossFitDay struct {
    Date        string `json:"date"`
    Dow         string `json:"dow"`
    HasCrossFit bool   `json:"has_crossfit"`
    Focus       string `json:"focus"`
    CNSLoad     Load   `json:"cns_load"`
    LegLoad     Load   `json:"leg_load"`
    Notes       string `json:"notes"`
}
type CrossFitWeekParsed struct {
    WeekStart string        `json:"week_start"`
    Days      []CrossFitDay `json:"days"`
}

// Stage 2
type PlanDay struct {
    Date         string  `json:"date"`
    Dow          string  `json:"dow"`
    RunType      string  `json:"run_type"`
    DistanceKm   float64 `json:"distance_km"`
    PaceTarget   string  `json:"pace_target"`
    TimeNote     string  `json:"time_note"`
    OptionalIfCNS bool   `json:"optional_if_cns"`
    Rationale    string  `json:"rationale"`
}
type PlanParsed struct {
    FitnessSummary string    `json:"fitness_summary"`
    WeeklyTargetKm float64   `json:"weekly_target_km"`
    Days           []PlanDay `json:"days"`
    WeekRationale  string    `json:"week_rationale"`
    OneFlag        string    `json:"one_flag"`
}
```

**Metrics output** (`backend/internal/metrics/metrics.go`):

```go
type FitnessMetrics struct {
    WeeklyVolumeKm     float64 `json:"weekly_volume_km"`     // recent (last 7-day) running km
    FourWeekAvgKm      float64 `json:"four_week_avg_km"`     // mean weekly km over last 4 wks
    AcuteChronicRatio  float64 `json:"acute_chronic_ratio"`  // 7-day vs 28-day load ratio
    EasyPace           string  `json:"easy_pace"`            // "6:00/km"
    ThresholdPace      string  `json:"threshold_pace"`       // "5:05/km"
    RecoveryTrend      string  `json:"recovery_trend"`       // "improving" | "stable" | "declining"
    SafeWeeklyTargetKm float64 `json:"safe_weekly_target_km"`// baseline × progression, ≤~10% ramp, cutback every 4th wk
    IsCutbackWeek      bool    `json:"is_cutback_week"`
}

// Pure functions over store rows (deterministic, table-driven tests).
func ComputeFitness(acts []store.Activity, recovery []store.RecoveryDay, profile store.AthleteProfile, now time.Time) FitnessMetrics
```

**Context pack** (`backend/internal/coach/coach.go`). Serialized to JSON and (a) piped to Stage 2 via stdin, (b) stored in `plans.context_pack_json`.

```go
type ContextPack struct {
    GeneratedAt   string                  `json:"generated_at"`   // RFC3339
    WeekStart     string                  `json:"week_start"`     // ISO Monday
    Metrics       metrics.FitnessMetrics  `json:"metrics"`
    Profile       ProfilePack             `json:"profile"`
    CrossFitWeek  llm.CrossFitWeekParsed  `json:"crossfit_week"`  // possibly user-edited
    LastWeekPlan  *llm.PlanParsed         `json:"last_week_plan"` // nil if none
}

type ProfilePack struct {
    TargetWeeklyKm   float64 `json:"target_weekly_km"`
    ProgressionMode  string  `json:"progression_mode"`
    Zone2CeilingBpm  *int64  `json:"zone2_ceiling_bpm"`
    ThresholdBpm     *int64  `json:"threshold_bpm"`
    MaxHRBpm         *int64  `json:"max_hr_bpm"`
    RunConstraints   json.RawMessage `json:"run_constraints"` // parsed from run_constraints_json
    GoalText         string  `json:"goal_text"`
}
```

**`run_constraints` JSON shape** (stored in `athlete_profile.run_constraints_json`; canonical keys):
```json
{
  "crossfit_days": ["Mon","Tue","Wed","Thu","Fri"],
  "crossfit_time": "18:15-19:15",
  "doubles_pref": "evening ~20:00 after CrossFit",
  "cns_rule": "skip or delay run ~2h after high-CNS days",
  "thursday_note": "barbell skill day, lighter legs/CNS"
}
```

**Coach orchestration interface** (injected into `api.Deps` — declared in `api` to avoid an import cycle; `*coach.Coach` satisfies it structurally):

```go
type Coach interface {
    ParseCrossFit(ctx context.Context, weekStart, imagePath string) (llm.CrossFitWeekParsed, rawResponse string, err error)
    GeneratePlan(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (llm.PlanParsed, contextPackJSON string, model string, err error)
    Fitness(ctx context.Context) (metrics.FitnessMetrics, error)
}
```

### REST API contract

All new endpoints live inside the bearer-protected `r.Group` in `NewRouter` (existing `middleware.Timeout(120s)` covers the `claude -p` budget). Auth: `Authorization: Bearer <API_TOKEN>`; missing/wrong → `401 {"error":"unauthorized"}`. Errors: `writeJSON(w, status, map[string]string{"error": ...})`. **All response json tags are snake_case.**

**`GET /api/profile`** — bearer, no params. `200`:
```json
{
  "target_weekly_km": 20,
  "progression_mode": "build",
  "zone2_ceiling_bpm": null,
  "threshold_bpm": null,
  "max_hr_bpm": null,
  "run_constraints_json": "{\"crossfit_days\":[\"Mon\",\"Tue\",\"Wed\",\"Thu\",\"Fri\"],\"crossfit_time\":\"18:15-19:15\",\"doubles_pref\":\"evening ~20:00 after CrossFit\",\"cns_rule\":\"skip or delay run ~2h after high-CNS days\",\"thursday_note\":\"barbell skill day, lighter legs/CNS\"}",
  "goal_text": "Build cardio over time",
  "updated_at": "2026-06-20T08:00:00Z"
}
```

**`PUT /api/profile`** — bearer. Body = same shape as `GET` **minus** `updated_at` (server sets it). `run_constraints_json` is a JSON **string**. Decoded with `json.NewDecoder(r.Body).Decode(...)`. `200`: the full updated profile. `400 {"error":"..."}` on bad body.

**`POST /api/crossfit/parse`** (multipart) — bearer. `Content-Type: multipart/form-data`:
- `image` (**file**, required) — read via `r.ParseMultipartForm` + `r.FormFile("image")`, saved to `IMAGE_DIR/<week_start>.<ext>`.
- `week_start` (text, required) — ISO Monday.
Runs Stage 1, upserts `crossfit_weeks`. `200`: the parsed CrossFit week (Stage-1 shape):
```json
{"week_start":"2026-06-22","days":[{"date":"2026-06-22","dow":"Mon","has_crossfit":true,"focus":"Back squat 5x5 + short metcon","cns_load":"high","leg_load":"high","notes":"Heavy legs"}, "...6 more"]}
```
`400` missing file/week_start; `502 {"error":"..."}` on Claude failure / malformed JSON (after the one retry); `500` on store error.

**`POST /api/plan/generate`** — bearer. `Content-Type: application/json`. Body:
```json
{
  "week_start": "2026-06-22",
  "crossfit_week": { "...optional edited Stage-1 object..." }
}
```
`week_start` required. `crossfit_week` optional — if present, overrides the stored one; if absent, the handler loads `crossfit_weeks[week_start]`. Builds context pack, runs Stage 2, inserts a new `plans` row. `200`: the plan (Stage-2 shape) plus server-set `id`, `week_start`, `generated_at`:
```json
{
  "id": 7,
  "week_start": "2026-06-22",
  "generated_at": "2026-06-20T08:05:12Z",
  "fitness_summary": "~18 km/week over the last month, acute:chronic 1.05 ...",
  "weekly_target_km": 20,
  "days": [ {"date":"2026-06-22","dow":"Mon","run_type":"rest","distance_km":0,"pace_target":"","time_note":"","optional_if_cns":false,"rationale":"Heavy squat day; no run."}, "...6 more" ],
  "week_rationale": "Quality placed Thursday ...",
  "one_flag": "If Thursday skill work runs heavy, downgrade the tempo to easy."
}
```
`400` missing/invalid `week_start`; `404` if no CrossFit week available and none supplied; `502` Claude failure/malformed JSON; `500` store error.

**`GET /api/plan?week=YYYY-MM-DD`** — bearer. Query param `week` (required, ISO Monday) → latest plan. `200`: same shape as `/api/plan/generate`. `400` missing/invalid `week`; `404 {"error":"no plan for week"}` if none.

**`GET /api/fitness`** — bearer, no params. Computes metrics from local store (works without Claude). `200` (FitnessMetrics):
```json
{
  "weekly_volume_km": 18.2,
  "four_week_avg_km": 17.4,
  "acute_chronic_ratio": 1.05,
  "easy_pace": "6:00/km",
  "threshold_pace": "5:05/km",
  "recovery_trend": "improving",
  "safe_weekly_target_km": 20.0,
  "is_cutback_week": false
}
```
`500` on store/compute error.

### Config / env additions

Add to `backend/internal/config/config.go` `Config` struct (envconfig tags, M0 conventions). `AnthropicAPIKey` stays a stub.

```go
ClaudeBin   string `envconfig:"CLAUDE_BIN" default:"claude"`
ClaudeModel string `envconfig:"CLAUDE_MODEL" default:"claude-opus-4-8"`
ImageDir    string `envconfig:"IMAGE_DIR" default:"./data/crossfit"`
```

`.env.example` additions (next to the existing Claude/Anthropic stub block):
```bash
# --- Claude Code headless (M1) ---
# Path to the `claude` CLI. Host must be logged in once: `claude auth login` (NOT --console).
# Headless/remote host with no browser? Run `claude setup-token` once on a machine with
# a browser and expose that token to this host's env; ANTHROPIC_API_KEY is a paid fallback.
CLAUDE_BIN=claude
# Model used for vision + planning.
CLAUDE_MODEL=claude-opus-4-8
# Directory where uploaded CrossFit schedule images are saved (read by `claude -p`).
IMAGE_DIR=./data/crossfit
```

`.gitignore`: add `data/` (uploaded images); ensure `*.db` already excluded.

### TS types (`app/src/api/types.ts`) — canonical (snake_case)

```ts
export type Load = 'low' | 'med' | 'high';

export interface CrossFitDay {
  date: string; dow: string; has_crossfit: boolean;
  focus: string; cns_load: Load; leg_load: Load; notes: string;
}
export interface CrossFitWeek { week_start: string; days: CrossFitDay[]; }

export interface PlanDay {
  date: string; dow: string; run_type: string; distance_km: number;
  pace_target: string; time_note: string; optional_if_cns: boolean; rationale: string;
}
export interface Plan {
  id?: number; week_start: string; generated_at?: string;
  fitness_summary: string; weekly_target_km: number;
  days: PlanDay[]; week_rationale: string; one_flag: string;
}

export interface AthleteProfile {
  target_weekly_km: number;
  progression_mode: 'build' | 'hold';
  zone2_ceiling_bpm: number | null;
  threshold_bpm: number | null;
  max_hr_bpm: number | null;
  run_constraints_json: string;
  goal_text: string;
  updated_at?: string;
}

export interface Fitness {
  weekly_volume_km: number;
  four_week_avg_km: number;
  acute_chronic_ratio: number;
  easy_pace: string;
  threshold_pace: string;
  recovery_trend: string;
  safe_weekly_target_km: number;
  is_cutback_week: boolean;
}
```

### Key cross-cutting notes
- **Casing:** wire JSON is **snake_case** everywhere. Go store struct field names are PascalCase; only the `llm` parsed structs, `metrics.FitnessMetrics`, and DTOs carry json tags.
- **Enum:** `Load` = exactly `"low"|"med"|"high"` across SQL, Go, model JSON, TS.
- **Multipart field name `image`** must match between `apiUpload` and Go `r.FormFile("image")`.
- **Injectable `llm.Runner`** is the test seam (copy `garmin/runner_test.go` shell-stub pattern); no real `claude`/network in CI.
- **Failure branching** keys off `exitCode != 0 || envelope.IsError` (NOT `subtype`); `502` to the app with a "logged in? limit hit?" classification.
- **One-retry rule** lives in `llm.Client.Call`; both stages go through it.
- **M0 follow-ups** (#1 OAuth state, #2 sync-on-boot, #3 latest-start_time cursor) are folded into the tasks below.

---

## Tasks

> **Global ordering note.** Tasks are renumbered into one sequence. Backend store/llm/coach/api/wiring follow the canonical API-draft implementations (consistent end-to-end with the contracts); the metrics engine uses the rigorous table-driven METRICS-draft tasks. The three M0 follow-ups are folded in after the core M1 backend. App tasks land last. Two migrations are introduced: `00002_m1_plan.sql` (Task 1) and `00003_oauth_state.sql` (Task 8).

### Task 1: Config additions (CLAUDE_BIN, CLAUDE_MODEL, IMAGE_DIR)

**Files:**
- Modify: `backend/internal/config/config.go` (add three fields to `Config` struct, after the `PythonBin`/`WorkerScript` block, before `AnthropicAPIKey`)
- Test: `backend/internal/config/config_test.go` (extend `setEnv` allow-list, add `TestLoadM1Defaults`/`TestLoadM1Explicit`)

- [ ] **Step 1: Write the failing test.** Edit `backend/internal/config/config_test.go`. In `setEnv`, append the three new keys to the `all` slice so they are cleared between tests; then add a new test and extend `TestLoadExplicit`.

Append to the `all` slice in `setEnv` (replace the existing slice literal's last line `"PYTHON_BIN", "WORKER_SCRIPT", "ANTHROPIC_API_KEY",` with):
```go
		"PYTHON_BIN", "WORKER_SCRIPT", "ANTHROPIC_API_KEY",
		"CLAUDE_BIN", "CLAUDE_MODEL", "IMAGE_DIR",
```

Add this new test function at the end of the file:
```go
func TestLoadM1Defaults(t *testing.T) {
	setEnv(t, requiredEnv())

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.ClaudeBin != "claude" {
		t.Errorf("ClaudeBin = %q, want default %q", cfg.ClaudeBin, "claude")
	}
	if cfg.ClaudeModel != "claude-opus-4-8" {
		t.Errorf("ClaudeModel = %q, want default %q", cfg.ClaudeModel, "claude-opus-4-8")
	}
	if cfg.ImageDir != "./data/crossfit" {
		t.Errorf("ImageDir = %q, want default %q", cfg.ImageDir, "./data/crossfit")
	}
}

func TestLoadM1Explicit(t *testing.T) {
	env := requiredEnv()
	env["CLAUDE_BIN"] = "/usr/local/bin/claude"
	env["CLAUDE_MODEL"] = "claude-opus-4-8"
	env["IMAGE_DIR"] = "/srv/data/cf"
	setEnv(t, env)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.ClaudeBin != "/usr/local/bin/claude" {
		t.Errorf("ClaudeBin = %q, want %q", cfg.ClaudeBin, "/usr/local/bin/claude")
	}
	if cfg.ImageDir != "/srv/data/cf" {
		t.Errorf("ImageDir = %q, want %q", cfg.ImageDir, "/srv/data/cf")
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/config/ -run TestLoadM1
```
Expected: FAIL — compile error `cfg.ClaudeBin undefined (type config.Config has no field or method ClaudeBin)`.

- [ ] **Step 3: Minimal implementation.** Edit `backend/internal/config/config.go`. Replace the `AnthropicAPIKey` line with the new fields above it:
```go
	PythonBin    string `envconfig:"PYTHON_BIN" default:"garmin-worker/.venv/bin/python"`
	WorkerScript string `envconfig:"WORKER_SCRIPT" default:"garmin-worker/worker.py"`

	// M1: Claude Code headless + image storage.
	ClaudeBin   string `envconfig:"CLAUDE_BIN" default:"claude"`
	ClaudeModel string `envconfig:"CLAUDE_MODEL" default:"claude-opus-4-8"`
	ImageDir    string `envconfig:"IMAGE_DIR" default:"./data/crossfit"`

	AnthropicAPIKey string `envconfig:"ANTHROPIC_API_KEY"` // stub (subscription path; unused)
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/config/
```
Expected: `ok  	help-my-run/backend/internal/config`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/config/config.go backend/internal/config/config_test.go && git commit -m "feat(config): add CLAUDE_BIN, CLAUDE_MODEL, IMAGE_DIR for M1

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: M1 migration (athlete_profile, crossfit_weeks, plans)

**Files:**
- Create: `backend/internal/store/migrations/00002_m1_plan.sql`
- Test: `backend/internal/store/store_test.go` (add `TestM1MigrationCreatesTables`, `TestM1MigrationSeedsProfile`)

- [ ] **Step 1: Write the failing test.** Append to `backend/internal/store/store_test.go`:
```go
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
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestM1Migration
```
Expected: FAIL — `table "athlete_profile" not found after migrate: sql: no rows in result set`.

- [ ] **Step 3: Create the migration.** Write `backend/internal/store/migrations/00002_m1_plan.sql`:
```sql
-- +goose Up
-- +goose StatementBegin

CREATE TABLE athlete_profile (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    target_weekly_km    REAL    NOT NULL DEFAULT 20,
    progression_mode    TEXT    NOT NULL DEFAULT 'build',
    zone2_ceiling_bpm   INTEGER,
    threshold_bpm       INTEGER,
    max_hr_bpm          INTEGER,
    run_constraints_json TEXT   NOT NULL DEFAULT '{}',
    goal_text           TEXT    NOT NULL DEFAULT '',
    updated_at          TEXT    NOT NULL
);

CREATE TABLE crossfit_weeks (
    week_start   TEXT PRIMARY KEY,
    image_path   TEXT,
    parsed_json  TEXT NOT NULL,
    raw_response TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE plans (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    week_start        TEXT    NOT NULL,
    generated_at      TEXT    NOT NULL,
    status            TEXT    NOT NULL DEFAULT 'generated',
    plan_json         TEXT    NOT NULL,
    fitness_summary   TEXT    NOT NULL DEFAULT '',
    context_pack_json TEXT,
    model             TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX idx_plans_week_start ON plans (week_start, generated_at DESC);

INSERT INTO athlete_profile
    (id, target_weekly_km, progression_mode, zone2_ceiling_bpm, threshold_bpm,
     max_hr_bpm, run_constraints_json, goal_text, updated_at)
VALUES
    (1, 20, 'build', NULL, NULL, NULL, '{}', '',
     strftime('%Y-%m-%dT%H:%M:%SZ', 'now'));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE plans;
DROP TABLE crossfit_weeks;
DROP TABLE athlete_profile;
-- +goose StatementEnd
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run "TestM1Migration|TestOpenAndMigrate|TestMigrate"
```
Expected: `ok  	help-my-run/backend/internal/store` (existing migration tests still pass; new ones pass).

- [ ] **Step 5: Commit.**
```
git add backend/internal/store/migrations/00002_m1_plan.sql backend/internal/store/store_test.go && git commit -m "feat(store): add 00002 migration for athlete_profile, crossfit_weeks, plans

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Store — athlete_profile (Get/Upsert)

**Files:**
- Create: `backend/internal/store/profile.go`
- Test: `backend/internal/store/profile_test.go`

- [ ] **Step 1: Write the failing test.** Write `backend/internal/store/profile_test.go`:
```go
package store

import "testing"

func TestGetAthleteProfileSeeded(t *testing.T) {
	s := newTestStore(t)

	p, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile() error = %v, want seeded row", err)
	}
	if p.TargetWeeklyKm != 20 || p.ProgressionMode != "build" {
		t.Errorf("seed = %+v, want target 20 mode build", p)
	}
	if p.RunConstraintsJSON != "{}" || p.GoalText != "" {
		t.Errorf("seed constraints/goal = %q/%q, want {}/empty", p.RunConstraintsJSON, p.GoalText)
	}
	if p.Zone2CeilingBpm != nil || p.ThresholdBpm != nil || p.MaxHRBpm != nil {
		t.Errorf("seed HR markers = %v/%v/%v, want all nil", p.Zone2CeilingBpm, p.ThresholdBpm, p.MaxHRBpm)
	}
	if p.UpdatedAt == "" {
		t.Error("seed UpdatedAt is empty, want non-empty")
	}
}

func TestUpsertAthleteProfileRoundTrip(t *testing.T) {
	s := newTestStore(t)

	z2 := int64(140)
	thr := int64(165)
	mx := int64(190)
	in := AthleteProfile{
		TargetWeeklyKm:     25,
		ProgressionMode:    "hold",
		Zone2CeilingBpm:    &z2,
		ThresholdBpm:       &thr,
		MaxHRBpm:           &mx,
		RunConstraintsJSON: `{"crossfit_days":["Mon","Tue"]}`,
		GoalText:           "Build cardio over time",
		UpdatedAt:          "ignored-server-sets-it",
	}
	if err := s.UpsertAthleteProfile(in); err != nil {
		t.Fatalf("UpsertAthleteProfile() error = %v", err)
	}

	got, err := s.GetAthleteProfile()
	if err != nil {
		t.Fatalf("GetAthleteProfile() error = %v", err)
	}
	if got.TargetWeeklyKm != 25 || got.ProgressionMode != "hold" {
		t.Errorf("got = %+v, want target 25 mode hold", got)
	}
	if got.Zone2CeilingBpm == nil || *got.Zone2CeilingBpm != 140 {
		t.Errorf("zone2 = %v, want 140", got.Zone2CeilingBpm)
	}
	if got.ThresholdBpm == nil || *got.ThresholdBpm != 165 || got.MaxHRBpm == nil || *got.MaxHRBpm != 190 {
		t.Errorf("thr/max = %v/%v, want 165/190", got.ThresholdBpm, got.MaxHRBpm)
	}
	if got.RunConstraintsJSON != `{"crossfit_days":["Mon","Tue"]}` || got.GoalText != "Build cardio over time" {
		t.Errorf("constraints/goal = %q/%q", got.RunConstraintsJSON, got.GoalText)
	}
	if got.UpdatedAt == "ignored-server-sets-it" || got.UpdatedAt == "" {
		t.Errorf("UpdatedAt = %q, want server-set RFC3339", got.UpdatedAt)
	}

	// Upsert again -> still single row (id=1).
	in.TargetWeeklyKm = 30
	in.Zone2CeilingBpm = nil
	if err := s.UpsertAthleteProfile(in); err != nil {
		t.Fatalf("second UpsertAthleteProfile() error = %v", err)
	}
	got, _ = s.GetAthleteProfile()
	if got.TargetWeeklyKm != 30 || got.Zone2CeilingBpm != nil {
		t.Errorf("after re-upsert = %+v, want target 30 zone2 nil", got)
	}
	var rows int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM athlete_profile`).Scan(&rows); err != nil {
		t.Fatalf("count: %v", err)
	}
	if rows != 1 {
		t.Errorf("athlete_profile row count = %d, want 1", rows)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestUpsertAthleteProfile
```
Expected: FAIL — compile error `undefined: AthleteProfile` / `s.GetAthleteProfile undefined`.

- [ ] **Step 3: Minimal implementation.** Write `backend/internal/store/profile.go`:
```go
package store

import (
	"database/sql"
	"errors"
	"time"
)

// AthleteProfile is the single-row athlete_profile record (id=1). Nullable HR
// markers are pointers; run_constraints_json is a raw JSON string.
type AthleteProfile struct {
	TargetWeeklyKm     float64
	ProgressionMode    string // "build" | "hold"
	Zone2CeilingBpm    *int64
	ThresholdBpm       *int64
	MaxHRBpm           *int64
	RunConstraintsJSON string
	GoalText           string
	UpdatedAt          string
}

// GetAthleteProfile returns the single profile row (id=1), or ErrNotFound.
func (s *Store) GetAthleteProfile() (AthleteProfile, error) {
	var p AthleteProfile
	var z2, thr, mx sql.NullInt64
	err := s.DB.QueryRow(`
		SELECT target_weekly_km, progression_mode, zone2_ceiling_bpm, threshold_bpm,
		       max_hr_bpm, run_constraints_json, goal_text, updated_at
		FROM athlete_profile WHERE id = 1`).
		Scan(&p.TargetWeeklyKm, &p.ProgressionMode, &z2, &thr, &mx,
			&p.RunConstraintsJSON, &p.GoalText, &p.UpdatedAt)
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
	return p, nil
}

// UpsertAthleteProfile upserts the single profile row (id always 1). updated_at
// is set server-side to now (UTC RFC3339).
func (s *Store) UpsertAthleteProfile(p AthleteProfile) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO athlete_profile
			(id, target_weekly_km, progression_mode, zone2_ceiling_bpm, threshold_bpm,
			 max_hr_bpm, run_constraints_json, goal_text, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			target_weekly_km     = excluded.target_weekly_km,
			progression_mode     = excluded.progression_mode,
			zone2_ceiling_bpm    = excluded.zone2_ceiling_bpm,
			threshold_bpm        = excluded.threshold_bpm,
			max_hr_bpm           = excluded.max_hr_bpm,
			run_constraints_json = excluded.run_constraints_json,
			goal_text            = excluded.goal_text,
			updated_at           = excluded.updated_at`,
		p.TargetWeeklyKm, p.ProgressionMode, p.Zone2CeilingBpm, p.ThresholdBpm,
		p.MaxHRBpm, p.RunConstraintsJSON, p.GoalText, now)
	return err
}
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run "TestGetAthleteProfile|TestUpsertAthleteProfile"
```
Expected: `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/store/profile.go backend/internal/store/profile_test.go && git commit -m "feat(store): athlete_profile Get/Upsert

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---
### Task 4: Store — crossfit_weeks (Get/Upsert)

**Files:**
- Create: `backend/internal/store/crossfit.go`
- Test: `backend/internal/store/crossfit_test.go`

- [ ] **Step 1: Write the failing test.** Write `backend/internal/store/crossfit_test.go`:
```go
package store

import "testing"

func TestGetCrossFitWeekNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetCrossFitWeek("2026-06-22"); err != ErrNotFound {
		t.Fatalf("GetCrossFitWeek on empty = %v, want ErrNotFound", err)
	}
}

func TestUpsertCrossFitWeekRoundTrip(t *testing.T) {
	s := newTestStore(t)

	img := "/data/crossfit/2026-06-22.jpg"
	raw := "```json\n{...}\n```"
	in := CrossFitWeek{
		WeekStart:   "2026-06-22",
		ImagePath:   &img,
		ParsedJSON:  `{"week_start":"2026-06-22","days":[]}`,
		RawResponse: &raw,
		CreatedAt:   "set-by-store",
		UpdatedAt:   "set-by-store",
	}
	if err := s.UpsertCrossFitWeek(in); err != nil {
		t.Fatalf("UpsertCrossFitWeek() error = %v", err)
	}

	got, err := s.GetCrossFitWeek("2026-06-22")
	if err != nil {
		t.Fatalf("GetCrossFitWeek() error = %v", err)
	}
	if got.WeekStart != "2026-06-22" {
		t.Errorf("WeekStart = %q, want 2026-06-22", got.WeekStart)
	}
	if got.ImagePath == nil || *got.ImagePath != img {
		t.Errorf("ImagePath = %v, want %q", got.ImagePath, img)
	}
	if got.ParsedJSON != `{"week_start":"2026-06-22","days":[]}` {
		t.Errorf("ParsedJSON = %q", got.ParsedJSON)
	}
	if got.RawResponse == nil || *got.RawResponse != raw {
		t.Errorf("RawResponse = %v, want %q", got.RawResponse, raw)
	}
	if got.CreatedAt == "" || got.CreatedAt == "set-by-store" {
		t.Errorf("CreatedAt = %q, want server-set", got.CreatedAt)
	}
	createdFirst := got.CreatedAt

	// Re-upsert (same PK week_start): updates parsed_json, preserves created_at.
	in.ParsedJSON = `{"week_start":"2026-06-22","days":[{"dow":"Mon"}]}`
	in.ImagePath = nil
	if err := s.UpsertCrossFitWeek(in); err != nil {
		t.Fatalf("second UpsertCrossFitWeek() error = %v", err)
	}
	got, _ = s.GetCrossFitWeek("2026-06-22")
	if got.ParsedJSON != `{"week_start":"2026-06-22","days":[{"dow":"Mon"}]}` {
		t.Errorf("after re-upsert ParsedJSON = %q", got.ParsedJSON)
	}
	if got.ImagePath != nil {
		t.Errorf("ImagePath = %v, want nil after re-upsert", got.ImagePath)
	}
	if got.CreatedAt != createdFirst {
		t.Errorf("CreatedAt changed on update: %q -> %q, want preserved", createdFirst, got.CreatedAt)
	}
	var rows int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM crossfit_weeks`).Scan(&rows)
	if rows != 1 {
		t.Errorf("row count = %d, want 1 (same PK)", rows)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestUpsertCrossFitWeek
```
Expected: FAIL — `undefined: CrossFitWeek` / `s.GetCrossFitWeek undefined`.

- [ ] **Step 3: Minimal implementation.** Write `backend/internal/store/crossfit.go`:
```go
package store

import (
	"database/sql"
	"errors"
	"time"
)

// CrossFitWeek maps to crossfit_weeks (one row per Monday week_start).
type CrossFitWeek struct {
	WeekStart   string
	ImagePath   *string
	ParsedJSON  string // Stage-1 parsed object, verbatim
	RawResponse *string
	CreatedAt   string
	UpdatedAt   string
}

// GetCrossFitWeek returns the row for weekStart, or ErrNotFound.
func (s *Store) GetCrossFitWeek(weekStart string) (CrossFitWeek, error) {
	var w CrossFitWeek
	var img, raw sql.NullString
	err := s.DB.QueryRow(`
		SELECT week_start, image_path, parsed_json, raw_response, created_at, updated_at
		FROM crossfit_weeks WHERE week_start = ?`, weekStart).
		Scan(&w.WeekStart, &img, &w.ParsedJSON, &raw, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CrossFitWeek{}, ErrNotFound
	}
	if err != nil {
		return CrossFitWeek{}, err
	}
	if img.Valid {
		w.ImagePath = &img.String
	}
	if raw.Valid {
		w.RawResponse = &raw.String
	}
	return w, nil
}

// UpsertCrossFitWeek upserts a parsed CrossFit week by week_start. created_at is
// set on insert and preserved on update; updated_at is always set to now.
func (s *Store) UpsertCrossFitWeek(w CrossFitWeek) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO crossfit_weeks
			(week_start, image_path, parsed_json, raw_response, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(week_start) DO UPDATE SET
			image_path   = excluded.image_path,
			parsed_json  = excluded.parsed_json,
			raw_response = excluded.raw_response,
			updated_at   = excluded.updated_at`,
		w.WeekStart, w.ImagePath, w.ParsedJSON, w.RawResponse, now, now)
	return err
}
```
Note: `created_at` is intentionally omitted from the `DO UPDATE SET` clause so the original insert timestamp is preserved on conflict.

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run "TestGetCrossFitWeek|TestUpsertCrossFitWeek"
```
Expected: `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/store/crossfit.go backend/internal/store/crossfit_test.go && git commit -m "feat(store): crossfit_weeks Get/Upsert

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Store — plans (Insert/GetLatest)

**Files:**
- Create: `backend/internal/store/plans.go`
- Test: `backend/internal/store/plans_test.go`

- [ ] **Step 1: Write the failing test.** Write `backend/internal/store/plans_test.go`:
```go
package store

import "testing"

func TestGetLatestPlanNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.GetLatestPlan("2026-06-22"); err != ErrNotFound {
		t.Fatalf("GetLatestPlan on empty = %v, want ErrNotFound", err)
	}
}

func TestInsertAndGetLatestPlan(t *testing.T) {
	s := newTestStore(t)

	ctx := `{"metrics":{}}`
	p1 := Plan{
		WeekStart:       "2026-06-22",
		GeneratedAt:     "2026-06-20T08:00:00Z",
		Status:          "generated",
		PlanJSON:        `{"weekly_target_km":20}`,
		FitnessSummary:  "first read",
		ContextPackJSON: &ctx,
		Model:           "claude-opus-4-8",
	}
	id1, err := s.InsertPlan(p1)
	if err != nil {
		t.Fatalf("InsertPlan(p1) error = %v", err)
	}
	if id1 <= 0 {
		t.Errorf("id1 = %d, want positive AUTOINCREMENT id", id1)
	}

	// Second plan, same week, later generated_at -> becomes the latest.
	p2 := Plan{
		WeekStart:      "2026-06-22",
		GeneratedAt:    "2026-06-20T09:30:00Z",
		Status:         "generated",
		PlanJSON:       `{"weekly_target_km":22}`,
		FitnessSummary: "second read",
		Model:          "claude-opus-4-8",
	}
	id2, err := s.InsertPlan(p2)
	if err != nil {
		t.Fatalf("InsertPlan(p2) error = %v", err)
	}
	if id2 == id1 {
		t.Errorf("id2 = %d equals id1, want distinct AUTOINCREMENT", id2)
	}

	got, err := s.GetLatestPlan("2026-06-22")
	if err != nil {
		t.Fatalf("GetLatestPlan() error = %v", err)
	}
	if got.ID != id2 {
		t.Errorf("latest ID = %d, want %d (most recent generated_at)", got.ID, id2)
	}
	if got.PlanJSON != `{"weekly_target_km":22}` || got.FitnessSummary != "second read" {
		t.Errorf("latest = %+v, want second plan", got)
	}
	if got.ContextPackJSON != nil {
		t.Errorf("p2 ContextPackJSON = %v, want nil", got.ContextPackJSON)
	}

	// A different week with no plan -> ErrNotFound.
	if _, err := s.GetLatestPlan("2026-06-29"); err != ErrNotFound {
		t.Errorf("GetLatestPlan(other week) = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestInsertAndGetLatestPlan
```
Expected: FAIL — `undefined: Plan` / `s.InsertPlan undefined`.

- [ ] **Step 3: Minimal implementation.** Write `backend/internal/store/plans.go`:
```go
package store

import (
	"database/sql"
	"errors"
)

// Plan maps to one plans row. Multiple plans per week_start are allowed
// (regenerate appends); "latest" is the most recent generated_at.
type Plan struct {
	ID              int64
	WeekStart       string
	GeneratedAt     string
	Status          string
	PlanJSON        string // Stage-2 parsed object, verbatim
	FitnessSummary  string
	ContextPackJSON *string
	Model           string
}

// InsertPlan inserts a new plan row and returns its AUTOINCREMENT id.
func (s *Store) InsertPlan(p Plan) (int64, error) {
	res, err := s.DB.Exec(`
		INSERT INTO plans
			(week_start, generated_at, status, plan_json, fitness_summary,
			 context_pack_json, model)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.WeekStart, p.GeneratedAt, p.Status, p.PlanJSON, p.FitnessSummary,
		p.ContextPackJSON, p.Model)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetLatestPlan returns the most recent plan for weekStart, or ErrNotFound.
func (s *Store) GetLatestPlan(weekStart string) (Plan, error) {
	var p Plan
	var ctx sql.NullString
	err := s.DB.QueryRow(`
		SELECT id, week_start, generated_at, status, plan_json, fitness_summary,
		       context_pack_json, model
		FROM plans
		WHERE week_start = ?
		ORDER BY generated_at DESC, id DESC
		LIMIT 1`, weekStart).
		Scan(&p.ID, &p.WeekStart, &p.GeneratedAt, &p.Status, &p.PlanJSON,
			&p.FitnessSummary, &ctx, &p.Model)
	if errors.Is(err, sql.ErrNoRows) {
		return Plan{}, ErrNotFound
	}
	if err != nil {
		return Plan{}, err
	}
	if ctx.Valid {
		p.ContextPackJSON = &ctx.String
	}
	return p, nil
}
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run "TestGetLatestPlan|TestInsertAndGetLatestPlan"
```
Expected: `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/store/plans.go backend/internal/store/plans_test.go && git commit -m "feat(store): plans Insert/GetLatest

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Store — LatestActivityStartTime (M0 follow-up #3 getter)

**Files:**
- Modify: `backend/internal/store/activities.go` (add `LatestActivityStartTime` after `ListActivities`)
- Test: `backend/internal/store/activities_test.go` (new file)

This getter is consumed by M0 follow-up #3 in Task 23 (sync cursor). Currently `SyncStrava` derives the cursor from `sync_log.last_synced_at`.

- [ ] **Step 1: Write the failing test.** Write `backend/internal/store/activities_test.go`:
```go
package store

import "testing"

func TestLatestActivityStartTimeEmpty(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.LatestActivityStartTime(); err != ErrNotFound {
		t.Fatalf("LatestActivityStartTime on empty = %v, want ErrNotFound", err)
	}
}

func TestLatestActivityStartTime(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertActivity(Activity{
		StravaID: 1, Name: "older", Type: "Run", StartTime: "2026-06-10T06:00:00Z",
		DistanceM: 5000, MovingTimeS: 1500, ElapsedTimeS: 1500, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert older: %v", err)
	}
	if err := s.UpsertActivity(Activity{
		StravaID: 2, Name: "newer", Type: "Run", StartTime: "2026-06-18T18:00:00Z",
		DistanceM: 8000, MovingTimeS: 2400, ElapsedTimeS: 2400, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("upsert newer: %v", err)
	}

	got, err := s.LatestActivityStartTime()
	if err != nil {
		t.Fatalf("LatestActivityStartTime() error = %v", err)
	}
	if got != "2026-06-18T18:00:00Z" {
		t.Errorf("LatestActivityStartTime() = %q, want 2026-06-18T18:00:00Z", got)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestLatestActivityStartTime
```
Expected: FAIL — `s.LatestActivityStartTime undefined`.

- [ ] **Step 3: Minimal implementation.** Append to `backend/internal/store/activities.go` (after `ListActivities`, before `UpsertSplits`):
```go
// LatestActivityStartTime returns the max start_time across all activities (ISO
// string), or ErrNotFound when there are no activities. Used as the Strava
// incremental sync cursor.
func (s *Store) LatestActivityStartTime() (string, error) {
	var t sql.NullString
	err := s.DB.QueryRow(`SELECT MAX(start_time) FROM activities`).Scan(&t)
	if err != nil {
		return "", err
	}
	if !t.Valid {
		return "", ErrNotFound
	}
	return t.String, nil
}
```
Add the `database/sql` import to `activities.go` (currently it only imports `time`):
```go
import (
	"database/sql"
	"time"
)
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestLatestActivityStartTime
```
Expected: `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/store/activities.go backend/internal/store/activities_test.go && git commit -m "feat(store): LatestActivityStartTime cursor getter

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Store — OAuth state (M0 follow-up #1 persistence)

**Files:**
- Create: `backend/internal/store/oauth_state.go` (`SaveOAuthState`, `ConsumeOAuthState`)
- Create: `backend/internal/store/migrations/00003_oauth_state.sql`
- Test: `backend/internal/store/oauth_state_test.go`

A dedicated table is the simplest persistence consistent with the M0 single-writer store. Consumed by Task 22 (callback validation).

- [ ] **Step 1: Write the failing test.** Write `backend/internal/store/oauth_state_test.go`:
```go
package store

import "testing"

func TestOAuthStateSaveAndConsume(t *testing.T) {
	s := newTestStore(t)

	if err := s.SaveOAuthState("abc123"); err != nil {
		t.Fatalf("SaveOAuthState() error = %v", err)
	}

	// Consume succeeds once.
	if err := s.ConsumeOAuthState("abc123"); err != nil {
		t.Fatalf("ConsumeOAuthState(abc123) error = %v, want nil", err)
	}
	// Second consume of the same state fails (single-use).
	if err := s.ConsumeOAuthState("abc123"); err != ErrNotFound {
		t.Errorf("second ConsumeOAuthState = %v, want ErrNotFound", err)
	}
}

func TestConsumeUnknownOAuthState(t *testing.T) {
	s := newTestStore(t)
	if err := s.ConsumeOAuthState("never-saved"); err != ErrNotFound {
		t.Errorf("ConsumeOAuthState(unknown) = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run "OAuthState"
```
Expected: FAIL — `s.SaveOAuthState undefined` / `s.ConsumeOAuthState undefined`.

- [ ] **Step 3a: Create the migration.** Write `backend/internal/store/migrations/00003_oauth_state.sql`:
```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE oauth_states (
    state      TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE oauth_states;
-- +goose StatementEnd
```

- [ ] **Step 3b: Minimal implementation.** Write `backend/internal/store/oauth_state.go`:
```go
package store

import "time"

// SaveOAuthState persists a generated CSRF state for later one-time validation
// in the Strava callback.
func (s *Store) SaveOAuthState(state string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`
		INSERT INTO oauth_states (state, created_at) VALUES (?, ?)
		ON CONFLICT(state) DO UPDATE SET created_at = excluded.created_at`,
		state, now)
	return err
}

// ConsumeOAuthState deletes the stored state, returning ErrNotFound if it was
// never saved (or already consumed). Single-use: a second consume fails.
func (s *Store) ConsumeOAuthState(state string) error {
	res, err := s.DB.Exec(`DELETE FROM oauth_states WHERE state = ?`, state)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run "OAuthState"
```
Expected: `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/store/oauth_state.go backend/internal/store/oauth_state_test.go backend/internal/store/migrations/00003_oauth_state.sql && git commit -m "feat(store): persistent single-use OAuth state for CSRF validation

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---
### Task 8: Metrics — package scaffold, `FitnessMetrics` struct, and pace helper

**Files:**
- Create: `backend/internal/metrics/metrics.go`
- Test: `backend/internal/metrics/metrics_test.go`

> **Dependency note:** `ComputeFitness` (Task 15) takes a `store.AthleteProfile` argument (Task 3). Tasks 8–13 here only use `store.Activity` / `store.RecoveryDay` (already on `main`), so they are unblocked. Task 15 must land after `store.AthleteProfile` exists.

- [ ] **Step 1: Write the failing test for `FitnessMetrics` JSON tags + `formatPace` helper.** Create `backend/internal/metrics/metrics_test.go`:

```go
package metrics

import (
	"encoding/json"
	"testing"
)

func TestFitnessMetricsJSONTags(t *testing.T) {
	m := FitnessMetrics{
		WeeklyVolumeKm:     18.2,
		FourWeekAvgKm:      17.4,
		AcuteChronicRatio:  1.05,
		EasyPace:           "6:00/km",
		ThresholdPace:      "5:05/km",
		RecoveryTrend:      "improving",
		SafeWeeklyTargetKm: 20.0,
		IsCutbackWeek:      false,
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	got := string(b)
	wantKeys := []string{
		`"weekly_volume_km":18.2`,
		`"four_week_avg_km":17.4`,
		`"acute_chronic_ratio":1.05`,
		`"easy_pace":"6:00/km"`,
		`"threshold_pace":"5:05/km"`,
		`"recovery_trend":"improving"`,
		`"safe_weekly_target_km":20`,
		`"is_cutback_week":false`,
	}
	for _, k := range wantKeys {
		if !contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestFormatPace(t *testing.T) {
	tests := []struct {
		name      string
		secPerKm  float64
		want      string
	}{
		{"zero -> empty", 0, ""},
		{"negative -> empty", -5, ""},
		{"6:00", 360, "6:00/km"},
		{"5:05", 305, "5:05/km"},
		{"rounds to nearest second", 305.4, "5:05/km"},
		{"rounds up", 305.6, "5:06/km"},
		{"single-digit seconds zero-padded", 363, "6:03/km"},
		{"carry to next minute", 359.6, "6:00/km"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPace(tt.secPerKm); got != tt.want {
				t.Errorf("formatPace(%v) = %q, want %q", tt.secPerKm, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test, expect a COMPILE FAILURE (red).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output contains `undefined: FitnessMetrics` and `undefined: formatPace` (`FAIL help-my-run/backend/internal/metrics [build failed]`).

- [ ] **Step 3: Write minimal implementation.** Create `backend/internal/metrics/metrics.go`:

```go
// Package metrics computes deterministic fitness numbers from M0 store rows.
// All functions here are pure (no DB, no clock) so they are table-test friendly:
// callers pass plain slices and an explicit `now`.
package metrics

import (
	"fmt"
	"math"
)

// FitnessMetrics is the computed fitness read returned by ComputeFitness and
// served at GET /api/fitness. JSON tags are snake_case (matches M0 dto.go).
type FitnessMetrics struct {
	WeeklyVolumeKm     float64 `json:"weekly_volume_km"`      // recent (last 7-day) running km
	FourWeekAvgKm      float64 `json:"four_week_avg_km"`      // mean weekly km over last 4 wks
	AcuteChronicRatio  float64 `json:"acute_chronic_ratio"`   // 7-day vs 28-day load ratio
	EasyPace           string  `json:"easy_pace"`             // "6:00/km"
	ThresholdPace      string  `json:"threshold_pace"`        // "5:05/km"
	RecoveryTrend      string  `json:"recovery_trend"`        // "improving" | "stable" | "declining"
	SafeWeeklyTargetKm float64 `json:"safe_weekly_target_km"` // baseline × progression, ≤~10% ramp
	IsCutbackWeek      bool    `json:"is_cutback_week"`
}

// formatPace renders seconds-per-km as "M:SS/km" (zero-padded seconds).
// Returns "" for non-positive input (no data).
func formatPace(secPerKm float64) string {
	if secPerKm <= 0 {
		return ""
	}
	total := int(math.Round(secPerKm))
	min := total / 60
	sec := total % 60
	return fmt.Sprintf("%d:%02d/km", min, sec)
}
```

- [ ] **Step 4: Run the test, expect PASS (green).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output: `ok  	help-my-run/backend/internal/metrics`.

- [ ] **Step 5: Commit.** Command: `git add backend/internal/metrics/metrics.go backend/internal/metrics/metrics_test.go && git commit -m "feat(metrics): add FitnessMetrics struct and pace formatter"`.

---

### Task 9: Metrics — run-activity filtering and weekly km bucketing helpers

**Files:**
- Modify: `backend/internal/metrics/metrics.go` (add `isRun`, `parseStart`, `distanceKmInWindow` helpers)
- Test: `backend/internal/metrics/metrics_test.go` (add `TestIsRun`, `TestDistanceKmInWindow`)

> Only run types count toward volume/load: `"Run"`, `"TrailRun"`, `"VirtualRun"`. `start_time` is RFC3339 UTC; unparseable rows are skipped (defensive).

- [ ] **Step 1: Write the failing test.** Append to `backend/internal/metrics/metrics_test.go`:

```go
func TestIsRun(t *testing.T) {
	tests := []struct {
		typ  string
		want bool
	}{
		{"Run", true},
		{"TrailRun", true},
		{"VirtualRun", true},
		{"Ride", false},
		{"Workout", false},
		{"WeightTraining", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isRun(tt.typ); got != tt.want {
			t.Errorf("isRun(%q) = %v, want %v", tt.typ, got, tt.want)
		}
	}
}

func TestDistanceKmInWindow(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z") // Monday
	acts := []store.Activity{
		// 2 days ago, run, 10 km -> in 7-day window.
		{StravaID: 1, Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000},
		// 6 days ago, trail run, 5 km -> in 7-day window.
		{StravaID: 2, Type: "TrailRun", StartTime: "2026-06-16T18:00:00Z", DistanceM: 5000},
		// 10 days ago, run, 8 km -> outside 7-day, inside 28-day.
		{StravaID: 3, Type: "Run", StartTime: "2026-06-12T06:00:00Z", DistanceM: 8000},
		// 2 days ago but a Ride -> excluded (not a run).
		{StravaID: 4, Type: "Ride", StartTime: "2026-06-20T07:00:00Z", DistanceM: 40000},
		// unparseable start -> skipped.
		{StravaID: 5, Type: "Run", StartTime: "not-a-time", DistanceM: 99000},
	}
	// 7-day window: [now-7d, now] -> acts 1 (10) + 2 (5) = 15 km.
	if got := distanceKmInWindow(acts, now.AddDate(0, 0, -7), now); got != 15 {
		t.Errorf("7-day distance = %v, want 15", got)
	}
	// 28-day window: acts 1+2+3 = 23 km.
	if got := distanceKmInWindow(acts, now.AddDate(0, 0, -28), now); got != 23 {
		t.Errorf("28-day distance = %v, want 23", got)
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}
```

Also add the imports to the top of the test file (replace the existing import block):

```go
import (
	"encoding/json"
	"testing"
	"time"

	"help-my-run/backend/internal/store"
)
```

- [ ] **Step 2: Run the test, expect a COMPILE FAILURE (red).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output contains `undefined: isRun` and `undefined: distanceKmInWindow` → `FAIL ... [build failed]`.

- [ ] **Step 3: Write minimal implementation.** Replace the import block in `backend/internal/metrics/metrics.go`:

```go
import (
	"fmt"
	"math"
	"time"

	"help-my-run/backend/internal/store"
)
```

Append these helpers:

```go
// runTypes are the Strava activity types counted as runs for volume/load.
var runTypes = map[string]bool{
	"Run":        true,
	"TrailRun":   true,
	"VirtualRun": true,
}

// isRun reports whether a Strava activity type counts toward running volume.
func isRun(typ string) bool { return runTypes[typ] }

// parseStart parses an activity StartTime (RFC3339 UTC). ok=false if unparseable.
func parseStart(startTime string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// distanceKmInWindow sums run distance (km) for activities whose start time is
// within (from, to] — from exclusive, to inclusive. Non-runs and unparseable
// rows are skipped.
func distanceKmInWindow(acts []store.Activity, from, to time.Time) float64 {
	var km float64
	for _, a := range acts {
		if !isRun(a.Type) {
			continue
		}
		t, ok := parseStart(a.StartTime)
		if !ok {
			continue
		}
		if t.After(from) && !t.After(to) {
			km += a.DistanceM / 1000.0
		}
	}
	return km
}
```

- [ ] **Step 4: Run the test, expect PASS (green).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output: `ok  	help-my-run/backend/internal/metrics`.

- [ ] **Step 5: Commit.** Command: `cd /home/jake/project/help-my-run && git add backend/internal/metrics/metrics.go backend/internal/metrics/metrics_test.go && git commit -m "feat(metrics): add run filtering and windowed distance helpers"`.

---

### Task 10: Metrics — recent weekly volume + 4-week average

**Files:**
- Modify: `backend/internal/metrics/metrics.go` (add `weeklyVolumeKm`, `fourWeekAvgKm`)
- Test: `backend/internal/metrics/metrics_test.go` (add `TestWeeklyVolumeKm`, `TestFourWeekAvgKm`)

> `weeklyVolumeKm` = run km in the last 7 days (now-7d, now]. `fourWeekAvgKm` = total run km in the last 28 days ÷ 4.

- [ ] **Step 1: Write the failing test.** Add `"math"` to the `metrics_test.go` import block (it currently imports `"testing"` and `"time"`; the result should be `import ( "math"; "testing"; "time"; "help-my-run/backend/internal/store" )`), then append:

```go
// nearlyEqual compares accumulated-division float64s with an epsilon to avoid
// brittle exact-equality failures (e.g. 18200/1000 vs 18.2). Defined once here;
// reused by the volume/average assertions below.
func nearlyEqual(got, want float64) bool { return math.Abs(got-want) <= 1e-9 }

func TestWeeklyVolumeKm(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")
	acts := []store.Activity{
		{StravaID: 1, Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000},
		{StravaID: 2, Type: "Run", StartTime: "2026-06-17T06:00:00Z", DistanceM: 8200},
		{StravaID: 3, Type: "Run", StartTime: "2026-06-10T06:00:00Z", DistanceM: 6000}, // >7d ago
	}
	if got := weeklyVolumeKm(acts, now); !nearlyEqual(got, 18.2) {
		t.Errorf("weeklyVolumeKm = %v, want 18.2", got)
	}
	// No runs in window -> 0.
	if got := weeklyVolumeKm(nil, now); !nearlyEqual(got, 0) {
		t.Errorf("weeklyVolumeKm(nil) = %v, want 0", got)
	}
}

func TestFourWeekAvgKm(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")
	acts := []store.Activity{
		{StravaID: 1, Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000}, // wk0
		{StravaID: 2, Type: "Run", StartTime: "2026-06-14T06:00:00Z", DistanceM: 6000},  // wk1
		{StravaID: 3, Type: "Run", StartTime: "2026-06-07T06:00:00Z", DistanceM: 8000},  // wk2
		{StravaID: 4, Type: "Run", StartTime: "2026-05-30T06:00:00Z", DistanceM: 8000},  // wk3 (23 days ago - in)
		{StravaID: 5, Type: "Run", StartTime: "2026-05-10T06:00:00Z", DistanceM: 50000}, // >28d ago, excluded
	}
	// 28-day total = 10+6+8+8 = 32 km; /4 = 8.0.
	if got := fourWeekAvgKm(acts, now); !nearlyEqual(got, 8.0) {
		t.Errorf("fourWeekAvgKm = %v, want 8.0", got)
	}
}
```
(`"math"` may already be imported by an earlier task's implementation file, but this is the **test** file (`metrics_test.go`) — add the import there. If a later edit reports `"math" imported and not used` it means the import already exists; keep a single copy.)

- [ ] **Step 2: Run the test, expect a COMPILE FAILURE (red).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output contains `undefined: weeklyVolumeKm` and `undefined: fourWeekAvgKm` → `FAIL ... [build failed]`.

- [ ] **Step 3: Write minimal implementation.** Append to `backend/internal/metrics/metrics.go`:

```go
// weeklyVolumeKm is run km over the last 7 days (now-7d, now].
func weeklyVolumeKm(acts []store.Activity, now time.Time) float64 {
	return distanceKmInWindow(acts, now.AddDate(0, 0, -7), now)
}

// fourWeekAvgKm is mean weekly run km over the last 28 days.
func fourWeekAvgKm(acts []store.Activity, now time.Time) float64 {
	total := distanceKmInWindow(acts, now.AddDate(0, 0, -28), now)
	return total / 4.0
}
```

- [ ] **Step 4: Run the test, expect PASS (green).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output: `ok  	help-my-run/backend/internal/metrics`.

- [ ] **Step 5: Commit.** Command: `cd /home/jake/project/help-my-run && git add backend/internal/metrics/metrics.go backend/internal/metrics/metrics_test.go && git commit -m "feat(metrics): compute weekly volume and 4-week average km"`.

---

### Task 11: Metrics — acute:chronic load ratio (7-day vs 28-day)

**Files:**
- Modify: `backend/internal/metrics/metrics.go` (add `round2`, `acuteChronicRatio`)
- Test: `backend/internal/metrics/metrics_test.go` (add `TestAcuteChronicRatio`)

> Load proxy = run km. Acute = 7-day km. Chronic = 28-day km ÷ 4 (mean weekly). Ratio = acute / chronic, rounded to 2 decimals. Guard: chronic == 0 → ratio 0. Balanced is ~0.8–1.3.

- [ ] **Step 1: Write the failing test.** Append to `backend/internal/metrics/metrics_test.go`:

```go
func TestAcuteChronicRatio(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")
	tests := []struct {
		name string
		acts []store.Activity
		want float64
	}{
		{
			name: "balanced ~1.05",
			acts: []store.Activity{
				{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000},
				{Type: "Run", StartTime: "2026-06-17T06:00:00Z", DistanceM: 8000},
				{Type: "Run", StartTime: "2026-06-13T06:00:00Z", DistanceM: 17400},
				{Type: "Run", StartTime: "2026-06-06T06:00:00Z", DistanceM: 17000},
				{Type: "Run", StartTime: "2026-05-30T06:00:00Z", DistanceM: 16000},
			},
			// acute=18; 28d total=68.4; chronic=17.1; 18/17.1=1.0526 -> 1.05.
			want: 1.05,
		},
		{
			name: "no chronic baseline -> 0",
			acts: nil,
			want: 0,
		},
		{
			name: "spike 2.0",
			acts: []store.Activity{
				{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 20000},
				{Type: "Run", StartTime: "2026-06-05T06:00:00Z", DistanceM: 20000},
			},
			want: 2.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := acuteChronicRatio(tt.acts, now); got != tt.want {
				t.Errorf("acuteChronicRatio = %v, want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test, expect a COMPILE FAILURE (red).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output contains `undefined: acuteChronicRatio` → `FAIL ... [build failed]`.

- [ ] **Step 3: Write minimal implementation.** Append to `backend/internal/metrics/metrics.go`:

```go
// round2 rounds to 2 decimal places.
func round2(v float64) float64 { return math.Round(v*100) / 100 }

// acuteChronicRatio is the 7-day load divided by the 28-day mean weekly load,
// both using run km as the load proxy. Returns 0 when there is no chronic
// baseline (28-day load is 0). Balanced is roughly 0.8–1.3.
func acuteChronicRatio(acts []store.Activity, now time.Time) float64 {
	acute := distanceKmInWindow(acts, now.AddDate(0, 0, -7), now)
	chronic := distanceKmInWindow(acts, now.AddDate(0, 0, -28), now) / 4.0
	if chronic == 0 {
		return 0
	}
	return round2(acute / chronic)
}
```

- [ ] **Step 4: Run the test, expect PASS (green).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output: `ok  	help-my-run/backend/internal/metrics`.

- [ ] **Step 5: Commit.** Command: `cd /home/jake/project/help-my-run && git add backend/internal/metrics/metrics.go backend/internal/metrics/metrics_test.go && git commit -m "feat(metrics): compute acute:chronic load ratio"`.

---
### Task 12: Metrics — easy-pace and threshold-pace estimates from activity summaries

**Files:**
- Modify: `backend/internal/metrics/metrics.go` (add `median`, `runPacesSecPerKm`, `paceEstimates`)
- Test: `backend/internal/metrics/metrics_test.go` (add `TestPaceEstimates`)

> Estimates come from activity *summaries* only (no streams — deferred per spec §11). For each run with positive distance and moving time, compute sec/km = `MovingTimeS / (DistanceM/1000)`. Use runs from the last 28 days.
> - **Easy pace** = median sec/km of all qualifying runs (robust; long easy runs dominate volume).
> - **Threshold pace** = median sec/km of the *fastest 25%* of runs, but never slower than easy.
> Returns `("", "")` when there are no qualifying runs.

- [ ] **Step 1: Write the failing test.** Append to `backend/internal/metrics/metrics_test.go`:

```go
func TestPaceEstimates(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")

	t.Run("no runs -> empty", func(t *testing.T) {
		easy, thr := paceEstimates(nil, now)
		if easy != "" || thr != "" {
			t.Errorf("paceEstimates(nil) = (%q,%q), want empty", easy, thr)
		}
	})

	t.Run("mixed runs", func(t *testing.T) {
		acts := []store.Activity{
			// easy ~6:00/km (360 s/km): 5km in 1800s.
			{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 5000, MovingTimeS: 1800},
			// easy ~6:20/km (380 s/km): 5km in 1900s.
			{Type: "Run", StartTime: "2026-06-18T06:00:00Z", DistanceM: 5000, MovingTimeS: 1900},
			// recovery ~6:40/km (400 s/km): 5km in 2000s.
			{Type: "Run", StartTime: "2026-06-16T06:00:00Z", DistanceM: 5000, MovingTimeS: 2000},
			// tempo ~5:05/km (305 s/km): 5km in 1525s. -> fastest.
			{Type: "Run", StartTime: "2026-06-14T06:00:00Z", DistanceM: 5000, MovingTimeS: 1525},
			// a Ride -> excluded.
			{Type: "Ride", StartTime: "2026-06-19T06:00:00Z", DistanceM: 20000, MovingTimeS: 3000},
			// zero distance -> skipped.
			{Type: "Run", StartTime: "2026-06-15T06:00:00Z", DistanceM: 0, MovingTimeS: 100},
		}
		// Qualifying sec/km sorted: [305, 360, 380, 400]. median = (360+380)/2 = 370 -> 6:10/km.
		// fastest 25% = ceil(4*0.25)=1 -> [305] -> median 305 -> 5:05/km.
		easy, thr := paceEstimates(acts, now)
		if easy != "6:10/km" {
			t.Errorf("easy = %q, want 6:10/km", easy)
		}
		if thr != "5:05/km" {
			t.Errorf("threshold = %q, want 5:05/km", thr)
		}
	})

	t.Run("threshold never slower than easy", func(t *testing.T) {
		acts := []store.Activity{
			{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 5000, MovingTimeS: 1800},
			{Type: "Run", StartTime: "2026-06-18T06:00:00Z", DistanceM: 5000, MovingTimeS: 1800},
		}
		easy, thr := paceEstimates(acts, now)
		if easy != "6:00/km" || thr != "6:00/km" {
			t.Errorf("got (easy=%q,thr=%q), want both 6:00/km", easy, thr)
		}
	})
}
```

- [ ] **Step 2: Run the test, expect a COMPILE FAILURE (red).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output contains `undefined: paceEstimates` → `FAIL ... [build failed]`.

- [ ] **Step 3: Write minimal implementation.** Add `"sort"` to the import block:

```go
import (
	"fmt"
	"math"
	"sort"
	"time"

	"help-my-run/backend/internal/store"
)
```

Append:

```go
// median returns the median of a sorted, non-empty slice.
func median(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2.0
}

// runPacesSecPerKm returns sorted (ascending) sec/km for qualifying runs in the
// last 28 days: runs with positive distance and moving time.
func runPacesSecPerKm(acts []store.Activity, now time.Time) []float64 {
	from := now.AddDate(0, 0, -28)
	var paces []float64
	for _, a := range acts {
		if !isRun(a.Type) || a.DistanceM <= 0 || a.MovingTimeS <= 0 {
			continue
		}
		t, ok := parseStart(a.StartTime)
		if !ok || !t.After(from) || t.After(now) {
			continue
		}
		secPerKm := float64(a.MovingTimeS) / (a.DistanceM / 1000.0)
		paces = append(paces, secPerKm)
	}
	sort.Float64s(paces)
	return paces
}

// paceEstimates returns (easyPace, thresholdPace) formatted as "M:SS/km" from
// activity summaries over the last 28 days. Easy = median of all qualifying
// runs; threshold = median of the fastest 25% (never slower than easy). Returns
// ("","") when there are no qualifying runs.
func paceEstimates(acts []store.Activity, now time.Time) (string, string) {
	paces := runPacesSecPerKm(acts, now)
	if len(paces) == 0 {
		return "", ""
	}
	easySec := median(paces)

	// Fastest 25% (at least 1). paces is ascending, so fastest are at the front.
	k := int(math.Ceil(float64(len(paces)) * 0.25))
	if k < 1 {
		k = 1
	}
	thrSec := median(paces[:k])
	if thrSec > easySec {
		thrSec = easySec
	}
	return formatPace(easySec), formatPace(thrSec)
}
```

- [ ] **Step 4: Run the test, expect PASS (green).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output: `ok  	help-my-run/backend/internal/metrics`.

- [ ] **Step 5: Commit.** Command: `git add backend/internal/metrics/metrics.go backend/internal/metrics/metrics_test.go && git commit -m "feat(metrics): estimate easy and threshold paces from summaries"`.

---

### Task 13: Metrics — recovery trend (HRV / sleep score / Body Battery over ~14 days)

**Files:**
- Modify: `backend/internal/metrics/metrics.go` (add `recoveryDeadband`, `avgPtr`, `signalVote`, `recoveryTrend`)
- Test: `backend/internal/metrics/metrics_test.go` (add `TestRecoveryTrend`)

> Compares the recent half vs the older half of the last 14 recovery days. Signals (each averaged, only present values count): HRV `LastNightAvgMs`, Sleep `Score`, Body Battery daily net (`Charged - Drained`). For each signal with data in both halves, classify up/down using a ±3% relative deadband. Majority vote → `"improving"` / `"declining"`; ties or no data → `"stable"`. Input is `[]store.RecoveryDay` most-recent-first (as `ListRecovery` returns); no clock argument.

- [ ] **Step 1: Write the failing test.** Append to `backend/internal/metrics/metrics_test.go`:

```go
func TestRecoveryTrend(t *testing.T) {
	day := func(date string, hrv, sleep, charged, drained *int64) store.RecoveryDay {
		rd := store.RecoveryDay{Date: date}
		if hrv != nil {
			rd.HRV = &store.HrvFields{LastNightAvgMs: hrv}
		}
		if sleep != nil {
			rd.Sleep = &store.SleepFields{Score: sleep}
		}
		if charged != nil || drained != nil {
			rd.BodyBattery = &store.BodyBatteryFields{Charged: charged, Drained: drained}
		}
		return rd
	}
	ip := func(v int64) *int64 { return &v }

	t.Run("no data -> stable", func(t *testing.T) {
		if got := recoveryTrend(nil); got != "stable" {
			t.Errorf("recoveryTrend(nil) = %q, want stable", got)
		}
	})

	t.Run("improving HRV and sleep", func(t *testing.T) {
		rec := []store.RecoveryDay{
			day("2026-06-22", ip(60), ip(85), ip(80), ip(40)), // recent
			day("2026-06-21", ip(58), ip(84), ip(78), ip(42)),
			day("2026-06-20", ip(59), ip(86), ip(82), ip(38)),
			day("2026-06-19", ip(48), ip(72), ip(60), ip(55)), // older
			day("2026-06-18", ip(47), ip(70), ip(58), ip(57)),
			day("2026-06-17", ip(49), ip(71), ip(62), ip(53)),
		}
		if got := recoveryTrend(rec); got != "improving" {
			t.Errorf("recoveryTrend = %q, want improving", got)
		}
	})

	t.Run("declining", func(t *testing.T) {
		rec := []store.RecoveryDay{
			day("2026-06-22", ip(45), ip(65), ip(50), ip(60)), // recent (worse)
			day("2026-06-21", ip(44), ip(64), ip(48), ip(62)),
			day("2026-06-20", ip(46), ip(66), ip(52), ip(58)),
			day("2026-06-19", ip(58), ip(82), ip(78), ip(40)), // older (better)
			day("2026-06-18", ip(57), ip(81), ip(76), ip(42)),
			day("2026-06-17", ip(59), ip(83), ip(80), ip(38)),
		}
		if got := recoveryTrend(rec); got != "declining" {
			t.Errorf("recoveryTrend = %q, want declining", got)
		}
	})

	t.Run("flat within deadband -> stable", func(t *testing.T) {
		rec := []store.RecoveryDay{
			day("2026-06-22", ip(50), ip(80), ip(70), ip(50)),
			day("2026-06-21", ip(50), ip(80), ip(70), ip(50)),
			day("2026-06-20", ip(50), ip(80), ip(70), ip(50)),
			day("2026-06-19", ip(50), ip(80), ip(70), ip(50)),
		}
		if got := recoveryTrend(rec); got != "stable" {
			t.Errorf("recoveryTrend = %q, want stable", got)
		}
	})

	t.Run("single day -> stable (cannot split halves)", func(t *testing.T) {
		rec := []store.RecoveryDay{day("2026-06-22", ip(50), ip(80), ip(70), ip(50))}
		if got := recoveryTrend(rec); got != "stable" {
			t.Errorf("recoveryTrend(1 day) = %q, want stable", got)
		}
	})
}
```

- [ ] **Step 2: Run the test, expect a COMPILE FAILURE (red).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output contains `undefined: recoveryTrend` → `FAIL ... [build failed]`.

- [ ] **Step 3: Write minimal implementation.** Append to `backend/internal/metrics/metrics.go`:

```go
// recoveryDeadband is the relative change (fraction) below which a signal's
// recent-vs-older difference is treated as flat.
const recoveryDeadband = 0.03

// avgPtr averages the non-nil int64 values; ok=false if none present.
func avgPtr(vals []*int64) (float64, bool) {
	var sum float64
	var n int
	for _, v := range vals {
		if v != nil {
			sum += float64(*v)
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

// signalVote compares recent vs older averages and returns +1 (improving),
// -1 (declining), or 0 (flat/absent). higherIsBetter inverts the polarity for
// signals where a lower value is better.
func signalVote(recent, older []*int64, higherIsBetter bool) int {
	r, okR := avgPtr(recent)
	o, okO := avgPtr(older)
	if !okR || !okO || o == 0 {
		return 0
	}
	change := (r - o) / o
	if change > recoveryDeadband {
		if higherIsBetter {
			return 1
		}
		return -1
	}
	if change < -recoveryDeadband {
		if higherIsBetter {
			return -1
		}
		return 1
	}
	return 0
}

// recoveryTrend classifies the recent ~14-day recovery direction as
// "improving" | "stable" | "declining" by majority vote across HRV
// (last-night avg ms), sleep score, and Body Battery net (charged-drained).
// recovery is most-recent-first (as ListRecovery returns). Needs >= 2 days.
func recoveryTrend(recovery []store.RecoveryDay) string {
	rows := recovery
	if len(rows) > 14 {
		rows = rows[:14]
	}
	if len(rows) < 2 {
		return "stable"
	}
	half := len(rows) / 2
	recent := rows[:half]
	older := rows[half:]

	collect := func(days []store.RecoveryDay, pick func(store.RecoveryDay) *int64) []*int64 {
		out := make([]*int64, 0, len(days))
		for _, d := range days {
			out = append(out, pick(d))
		}
		return out
	}

	hrv := func(d store.RecoveryDay) *int64 {
		if d.HRV == nil {
			return nil
		}
		return d.HRV.LastNightAvgMs
	}
	sleep := func(d store.RecoveryDay) *int64 {
		if d.Sleep == nil {
			return nil
		}
		return d.Sleep.Score
	}
	bbNet := func(d store.RecoveryDay) *int64 {
		if d.BodyBattery == nil || d.BodyBattery.Charged == nil || d.BodyBattery.Drained == nil {
			return nil
		}
		net := *d.BodyBattery.Charged - *d.BodyBattery.Drained
		return &net
	}

	score := 0
	score += signalVote(collect(recent, hrv), collect(older, hrv), true)
	score += signalVote(collect(recent, sleep), collect(older, sleep), true)
	score += signalVote(collect(recent, bbNet), collect(older, bbNet), true)

	switch {
	case score > 0:
		return "improving"
	case score < 0:
		return "declining"
	default:
		return "stable"
	}
}
```

- [ ] **Step 4: Run the test, expect PASS (green).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output: `ok  	help-my-run/backend/internal/metrics`.

- [ ] **Step 5: Commit.** Command: `cd /home/jake/project/help-my-run && git add backend/internal/metrics/metrics.go backend/internal/metrics/metrics_test.go && git commit -m "feat(metrics): compute recovery trend from HRV/sleep/body battery"`.

---
### Task 14: Metrics — safe next-week volume target + cutback-week detection

**Files:**
- Modify: `backend/internal/metrics/metrics.go` (add `cutbackEpoch`, `weekIndexSince`, `isCutbackWeek`, `round1`, `safeWeeklyTarget`)
- Test: `backend/internal/metrics/metrics_test.go` (add `TestIsCutbackWeek`, `TestSafeWeeklyTarget`)

> **Baseline** for progression = `max(weeklyVolumeKm, fourWeekAvgKm)`; if there is no history (baseline 0) fall back to `profile.TargetWeeklyKm`.
> **Cutback week**: every 4th week (week index since epoch Monday `2026-01-05`, `weekIndex % 4 == 3`). On a cutback week, target = 80% of baseline. Otherwise:
> - `progression_mode == "build"` → baseline × 1.10, capped so it never exceeds `profile.TargetWeeklyKm × 1.5`, never reduced below baseline.
> - `progression_mode == "hold"` → baseline (no ramp).
> All targets rounded to 1 decimal.

- [ ] **Step 1: Write the failing test.** Append to `backend/internal/metrics/metrics_test.go`:

```go
func TestIsCutbackWeek(t *testing.T) {
	tests := []struct {
		name string
		now  string
		want bool
	}{
		{"epoch week (idx 0)", "2026-01-05T12:00:00Z", false},
		{"idx 1", "2026-01-12T12:00:00Z", false},
		{"idx 2", "2026-01-19T12:00:00Z", false},
		{"idx 3 -> cutback", "2026-01-26T12:00:00Z", true},
		{"idx 4", "2026-02-02T12:00:00Z", false},
		{"idx 7 -> cutback", "2026-02-23T12:00:00Z", true},
		{"mid-week still counts by week", "2026-01-28T23:00:00Z", true}, // within idx-3 week
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := mustTime(t, tt.now)
			if got := isCutbackWeek(now); got != tt.want {
				t.Errorf("isCutbackWeek(%s) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestSafeWeeklyTarget(t *testing.T) {
	prof := func(target float64, mode string) store.AthleteProfile {
		return store.AthleteProfile{TargetWeeklyKm: target, ProgressionMode: mode}
	}

	tests := []struct {
		name     string
		baseline float64
		profile  store.AthleteProfile
		cutback  bool
		want     float64
	}{
		{"build ramps 10%", 20, prof(40, "build"), false, 22.0},
		{"hold stays flat", 20, prof(40, "hold"), false, 20.0},
		{"cutback = 80% of baseline", 20, prof(40, "build"), true, 16.0},
		{"build capped at 1.5x stated target", 20, prof(20, "build"), false, 22.0}, // 22 < 30 cap, ok
		{"build cap binds", 25, prof(20, "build"), false, 27.5},                    // 27.5 < 30 cap
		{"build hard cap", 28, prof(20, "build"), false, 30.0},                     // 30.8 -> capped to 30
		{"no history falls back to profile target", 0, prof(20, "build"), false, 22.0},
		{"rounds to 1 decimal", 18.16, prof(40, "build"), false, 20.0},             // 18.16*1.1=19.976 -> 20.0
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeWeeklyTarget(tt.baseline, tt.profile, tt.cutback)
			if got != tt.want {
				t.Errorf("safeWeeklyTarget(%v, %+v, cutback=%v) = %v, want %v",
					tt.baseline, tt.profile, tt.cutback, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test, expect a COMPILE FAILURE (red).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output contains `undefined: isCutbackWeek` and `undefined: safeWeeklyTarget` → `FAIL ... [build failed]`. (If `store.AthleteProfile` is not yet defined the failure additionally includes `undefined: store.AthleteProfile` — sequence this after Task 3.)

- [ ] **Step 3: Write minimal implementation.** Append to `backend/internal/metrics/metrics.go`:

```go
// cutbackEpoch is an anchor Monday used to index weeks for the cutback cadence.
var cutbackEpoch = time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)

// weekIndexSince returns the number of whole 7-day weeks from cutbackEpoch to t.
func weekIndexSince(t time.Time) int {
	days := int(t.UTC().Sub(cutbackEpoch).Hours() / 24)
	return days / 7
}

// isCutbackWeek reports whether the week containing `now` is a cutback week
// (every 4th week: weekIndex % 4 == 3).
func isCutbackWeek(now time.Time) bool {
	return weekIndexSince(now)%4 == 3
}

// round1 rounds to 1 decimal place.
func round1(v float64) float64 { return math.Round(v*10) / 10 }

// safeWeeklyTarget computes the next-week volume target from a baseline (recent
// run km) and the athlete profile. Cutback weeks pull back to 80% of baseline.
// "build" mode ramps +10% (capped at 1.5× the profile's stated target, floored
// at baseline); "hold" holds baseline. With no history (baseline <= 0) it falls
// back to the profile's stated target. Rounded to 1 decimal.
func safeWeeklyTarget(baseline float64, profile store.AthleteProfile, cutback bool) float64 {
	if baseline <= 0 {
		baseline = profile.TargetWeeklyKm
	}
	if cutback {
		return round1(baseline * 0.80)
	}
	if profile.ProgressionMode == "hold" {
		return round1(baseline)
	}
	// "build" (default): +10% ramp, capped at 1.5× stated target, floored at baseline.
	target := baseline * 1.10
	if cap := profile.TargetWeeklyKm * 1.5; cap > 0 && target > cap {
		target = cap
	}
	if target < baseline {
		target = baseline
	}
	return round1(target)
}
```

- [ ] **Step 4: Run the test, expect PASS (green).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output: `ok  	help-my-run/backend/internal/metrics`.

- [ ] **Step 5: Commit.** Command: `git add backend/internal/metrics/metrics.go backend/internal/metrics/metrics_test.go && git commit -m "feat(metrics): compute safe weekly target and cutback cadence"`.

---

### Task 15: Metrics — `ComputeFitness` top-level assembler (the contract entry point)

**Files:**
- Modify: `backend/internal/metrics/metrics.go` (add exported `ComputeFitness`)
- Test: `backend/internal/metrics/metrics_test.go` (add `TestComputeFitness`, `TestComputeFitnessEmpty`)

> Exact contract signature `ComputeFitness(acts []store.Activity, recovery []store.RecoveryDay, profile store.AthleteProfile, now time.Time) FitnessMetrics`. Baseline for the target = `max(weeklyVolumeKm, fourWeekAvgKm)`. This is the function `coach.Fitness` and `GET /api/fitness` call. **Requires `store.AthleteProfile` (Task 3) merged.**

- [ ] **Step 1: Write the failing test.** Append to `backend/internal/metrics/metrics_test.go`:

```go
func TestComputeFitness(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z") // Monday; weekIndex non-cutback

	acts := []store.Activity{
		// last 7 days: 10 + 8.2 = 18.2 km.
		{Type: "Run", StartTime: "2026-06-20T06:00:00Z", DistanceM: 10000, MovingTimeS: 3600}, // 6:00/km
		{Type: "Run", StartTime: "2026-06-17T06:00:00Z", DistanceM: 8200, MovingTimeS: 3000},  // ~6:06/km
		// 8-28 days: more volume for chronic/4-week avg + a tempo for threshold.
		{Type: "Run", StartTime: "2026-06-13T06:00:00Z", DistanceM: 8000, MovingTimeS: 2440}, // 5:05/km tempo
		{Type: "Run", StartTime: "2026-06-06T06:00:00Z", DistanceM: 16000, MovingTimeS: 6400},
		{Type: "Run", StartTime: "2026-05-30T06:00:00Z", DistanceM: 16000, MovingTimeS: 6400},
		{Type: "Ride", StartTime: "2026-06-19T06:00:00Z", DistanceM: 40000, MovingTimeS: 3600}, // excluded
	}
	ip := func(v int64) *int64 { return &v }
	recovery := []store.RecoveryDay{
		{Date: "2026-06-21", HRV: &store.HrvFields{LastNightAvgMs: ip(60)}, Sleep: &store.SleepFields{Score: ip(85)}},
		{Date: "2026-06-20", HRV: &store.HrvFields{LastNightAvgMs: ip(59)}, Sleep: &store.SleepFields{Score: ip(86)}},
		{Date: "2026-06-15", HRV: &store.HrvFields{LastNightAvgMs: ip(48)}, Sleep: &store.SleepFields{Score: ip(72)}},
		{Date: "2026-06-14", HRV: &store.HrvFields{LastNightAvgMs: ip(47)}, Sleep: &store.SleepFields{Score: ip(70)}},
	}
	profile := store.AthleteProfile{TargetWeeklyKm: 40, ProgressionMode: "build"}

	m := ComputeFitness(acts, recovery, profile, now)

	if m.WeeklyVolumeKm != 18.2 {
		t.Errorf("WeeklyVolumeKm = %v, want 18.2", m.WeeklyVolumeKm)
	}
	// 28-day total runs = 18.2 + 8 + 16 + 16 = 58.2 -> /4 = 14.55.
	if m.FourWeekAvgKm != 14.55 {
		t.Errorf("FourWeekAvgKm = %v, want 14.55", m.FourWeekAvgKm)
	}
	// acute=18.2; chronic=14.55; 18.2/14.55=1.2509 -> 1.25.
	if m.AcuteChronicRatio != 1.25 {
		t.Errorf("AcuteChronicRatio = %v, want 1.25", m.AcuteChronicRatio)
	}
	if m.EasyPace == "" || m.ThresholdPace == "" {
		t.Errorf("paces should be set, got easy=%q thr=%q", m.EasyPace, m.ThresholdPace)
	}
	if m.RecoveryTrend != "improving" {
		t.Errorf("RecoveryTrend = %q, want improving", m.RecoveryTrend)
	}
	// baseline = max(18.2, 14.55) = 18.2; non-cutback build -> 18.2*1.1=20.02 -> 20.0
	if !m.IsCutbackWeek && m.SafeWeeklyTargetKm != 20.0 {
		t.Errorf("SafeWeeklyTargetKm = %v, want 20.0 (non-cutback)", m.SafeWeeklyTargetKm)
	}
	if m.IsCutbackWeek != isCutbackWeek(now) {
		t.Errorf("IsCutbackWeek = %v, want %v", m.IsCutbackWeek, isCutbackWeek(now))
	}
}

func TestComputeFitnessEmpty(t *testing.T) {
	now := mustTime(t, "2026-06-22T12:00:00Z")
	profile := store.AthleteProfile{TargetWeeklyKm: 20, ProgressionMode: "build"}
	m := ComputeFitness(nil, nil, profile, now)

	if m.WeeklyVolumeKm != 0 || m.FourWeekAvgKm != 0 || m.AcuteChronicRatio != 0 {
		t.Errorf("empty volumes = %+v, want zeros", m)
	}
	if m.EasyPace != "" || m.ThresholdPace != "" {
		t.Errorf("empty paces = (%q,%q), want empty", m.EasyPace, m.ThresholdPace)
	}
	if m.RecoveryTrend != "stable" {
		t.Errorf("empty RecoveryTrend = %q, want stable", m.RecoveryTrend)
	}
	// baseline 0 -> fallback to profile target 20; non-cutback build -> 22.0.
	if !m.IsCutbackWeek && m.SafeWeeklyTargetKm != 22.0 {
		t.Errorf("empty SafeWeeklyTargetKm = %v, want 22.0", m.SafeWeeklyTargetKm)
	}
}
```

- [ ] **Step 2: Run the test, expect a COMPILE FAILURE (red).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output contains `undefined: ComputeFitness` → `FAIL ... [build failed]`.

- [ ] **Step 3: Write minimal implementation.** Append to `backend/internal/metrics/metrics.go`:

```go
// ComputeFitness assembles the full FitnessMetrics read from M0 store rows.
// Pure and deterministic: callers supply activities, recovery days
// (most-recent-first), the athlete profile, and an explicit `now`. This is the
// entry point used by coach.Fitness and GET /api/fitness.
func ComputeFitness(acts []store.Activity, recovery []store.RecoveryDay, profile store.AthleteProfile, now time.Time) FitnessMetrics {
	weekly := weeklyVolumeKm(acts, now)
	fourWeek := round2(fourWeekAvgKm(acts, now))
	easy, threshold := paceEstimates(acts, now)
	cutback := isCutbackWeek(now)

	baseline := weekly
	if fourWeek > baseline {
		baseline = fourWeek
	}

	return FitnessMetrics{
		WeeklyVolumeKm:     round2(weekly),
		FourWeekAvgKm:      fourWeek,
		AcuteChronicRatio:  acuteChronicRatio(acts, now),
		EasyPace:           easy,
		ThresholdPace:      threshold,
		RecoveryTrend:      recoveryTrend(recovery),
		SafeWeeklyTargetKm: safeWeeklyTarget(baseline, profile, cutback),
		IsCutbackWeek:      cutback,
	}
}
```

- [ ] **Step 4: Run the test, expect PASS (green).** Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`. Expected output: `ok  	help-my-run/backend/internal/metrics`. Then confirm no regression: `cd /home/jake/project/help-my-run/backend && go build ./... && go test ./...`.

- [ ] **Step 5: Commit.** Command: `cd /home/jake/project/help-my-run && git add backend/internal/metrics/metrics.go backend/internal/metrics/metrics_test.go && git commit -m "feat(metrics): add ComputeFitness assembler entry point"`.

---
### Task 16: LLM — Envelope, Runner, ExtractJSON, ClassifyFailure

**Files:**
- Create: `backend/internal/llm/client.go` (`Runner`, `ExecRunner`, `Client`, `Envelope`, `ParseEnvelope`, `ExtractJSON`, `ClassifyFailure`, error types)
- Create: `backend/internal/llm/testdata/stage1_envelope.json`, `stage2_envelope.json`, `malformed_envelope.json`, `not_logged_in_envelope.json`
- Test: `backend/internal/llm/client_test.go`

This task implements `ExtractJSON` (contracts §inner-JSON extraction) and the failure determination. `Call` (the 1-retry loop) is Task 17 — keeps each TDD step small.

- [ ] **Step 1: Write the failing test.** First create the fixtures.

`backend/internal/llm/testdata/stage1_envelope.json` (`.result` is a fenced JSON block):
```json
{
  "type": "result",
  "subtype": "success",
  "is_error": false,
  "api_error_status": null,
  "result": "```json\n{\"week_start\":\"2026-06-22\",\"days\":[{\"date\":\"2026-06-22\",\"dow\":\"Mon\",\"has_crossfit\":true,\"focus\":\"Back squat 5x5 + short metcon\",\"cns_load\":\"high\",\"leg_load\":\"high\",\"notes\":\"Heavy legs\"}]}\n```",
  "stop_reason": "end_turn",
  "session_id": "s1",
  "num_turns": 2,
  "duration_ms": 5300,
  "total_cost_usd": 0.0
}
```

`backend/internal/llm/testdata/stage2_envelope.json` (`.result` is bare JSON with surrounding prose):
```json
{
  "type": "result",
  "subtype": "success",
  "is_error": false,
  "api_error_status": null,
  "result": "Here is the plan:\n{\"fitness_summary\":\"~18 km/week\",\"weekly_target_km\":20,\"days\":[{\"date\":\"2026-06-22\",\"dow\":\"Mon\",\"run_type\":\"rest\",\"distance_km\":0,\"pace_target\":\"\",\"time_note\":\"\",\"optional_if_cns\":false,\"rationale\":\"Heavy squat day; no run.\"}],\"week_rationale\":\"Quality Thursday.\",\"one_flag\":\"Watch Thursday.\"}\nThat's it.",
  "stop_reason": "end_turn",
  "session_id": "s2",
  "num_turns": 1,
  "duration_ms": 4100,
  "total_cost_usd": 0.0
}
```

`backend/internal/llm/testdata/malformed_envelope.json` (success envelope, `.result` not valid JSON):
```json
{
  "type": "result",
  "subtype": "success",
  "is_error": false,
  "api_error_status": null,
  "result": "Sorry, I cannot produce JSON right now.",
  "stop_reason": "end_turn",
  "session_id": "s3",
  "num_turns": 1,
  "duration_ms": 900,
  "total_cost_usd": 0.0
}
```

`backend/internal/llm/testdata/not_logged_in_envelope.json` (the critical case: `subtype:"success"` but `is_error:true`):
```json
{
  "type": "result",
  "subtype": "success",
  "is_error": true,
  "api_error_status": null,
  "result": "Invalid API key · Please run /login. Not logged in.",
  "stop_reason": "error",
  "session_id": "s4",
  "num_turns": 0,
  "duration_ms": 120,
  "total_cost_usd": 0.0
}
```

Now write `backend/internal/llm/client_test.go`:
```go
package llm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return b
}

// envelopeResult unwraps a fixture file's .result via the Envelope type so the
// ExtractJSON tests operate on what the model actually emitted.
func envelopeResult(t *testing.T, name string) string {
	t.Helper()
	env, err := ParseEnvelope(loadFixture(t, name))
	if err != nil {
		t.Fatalf("ParseEnvelope(%s): %v", name, err)
	}
	return env.Result
}

func TestParseEnvelopeFlags(t *testing.T) {
	env, err := ParseEnvelope(loadFixture(t, "not_logged_in_envelope.json"))
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if env.Subtype != "success" {
		t.Errorf("subtype = %q, want success (the trap case)", env.Subtype)
	}
	if !env.IsError {
		t.Error("IsError = false, want true (branch on IsError not subtype)")
	}
}

func TestExtractJSONFencedBlock(t *testing.T) {
	type week struct {
		WeekStart string `json:"week_start"`
		Days      []struct {
			Dow     string `json:"dow"`
			CNSLoad string `json:"cns_load"`
		} `json:"days"`
	}
	var w week
	if err := ExtractJSON(envelopeResult(t, "stage1_envelope.json"), &w); err != nil {
		t.Fatalf("ExtractJSON(fenced) error = %v", err)
	}
	if w.WeekStart != "2026-06-22" || len(w.Days) != 1 || w.Days[0].Dow != "Mon" || w.Days[0].CNSLoad != "high" {
		t.Errorf("parsed = %+v, want week 2026-06-22 Mon high", w)
	}
}

func TestExtractJSONBareWithProse(t *testing.T) {
	type plan struct {
		WeeklyTargetKm float64 `json:"weekly_target_km"`
		OneFlag        string  `json:"one_flag"`
	}
	var p plan
	if err := ExtractJSON(envelopeResult(t, "stage2_envelope.json"), &p); err != nil {
		t.Fatalf("ExtractJSON(prose+json) error = %v", err)
	}
	if p.WeeklyTargetKm != 20 || p.OneFlag != "Watch Thursday." {
		t.Errorf("parsed = %+v, want target 20 flag 'Watch Thursday.'", p)
	}
}

func TestExtractJSONMalformed(t *testing.T) {
	var v map[string]any
	err := ExtractJSON(envelopeResult(t, "malformed_envelope.json"), &v)
	if err == nil {
		t.Fatal("ExtractJSON(malformed) error = nil, want error")
	}
}

func TestClassifyFailureNotLoggedIn(t *testing.T) {
	env, _ := ParseEnvelope(loadFixture(t, "not_logged_in_envelope.json"))
	msg := ClassifyFailure(env, nil)
	if msg == "" {
		t.Fatal("ClassifyFailure returned empty")
	}
	if !contains(msg, "logged in") {
		t.Errorf("classify = %q, want a 'logged in' hint", msg)
	}
}

func TestClassifyFailureBinaryNotFound(t *testing.T) {
	msg := ClassifyFailure(Envelope{}, errExecNotFound)
	if !contains(msg, "not installed") {
		t.Errorf("classify = %q, want a 'not installed' hint", msg)
	}
}

func TestClassifyFailureAuthKeywords(t *testing.T) {
	// Each of these .result texts should map to the not-logged-in message.
	for _, kw := range []string{
		"Please login first", "You are not logged in", "failed to authenticate",
		"401 Unauthorized", "no valid credential found",
	} {
		msg := ClassifyFailure(Envelope{Result: kw}, nil)
		if !contains(msg, "logged in") {
			t.Errorf("classify(%q) = %q, want a 'logged in' hint", kw, msg)
		}
	}
}

func TestClassifyFailureUsageLimitKeywords(t *testing.T) {
	// Each of these .result texts should map to the rate/usage-limit message.
	for _, kw := range []string{
		"usage limit reached", "rate limited", "out of credit", "monthly quota exceeded",
	} {
		msg := ClassifyFailure(Envelope{Result: kw}, nil)
		if !contains(msg, "limit") {
			t.Errorf("classify(%q) = %q, want a 'limit' hint", kw, msg)
		}
	}
}

func TestExecRunnerStubViaSh(t *testing.T) {
	// The ExecRunner shells out; stub it with /bin/sh printing a fixture.
	fixture, _ := filepath.Abs(filepath.Join("testdata", "stage1_envelope.json"))
	r := ExecRunner{Bin: "/bin/sh"}
	out, err := r.Run(context.Background(),
		[]string{"-c", "cat '" + fixture + "'"}, "")
	if err != nil {
		t.Fatalf("ExecRunner.Run error = %v", err)
	}
	env, err := ParseEnvelope(out)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if env.Subtype != "success" {
		t.Errorf("subtype = %q, want success", env.Subtype)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

var _ = errors.Is
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/llm/
```
Expected: FAIL — `undefined: ParseEnvelope`, `undefined: ExtractJSON`, `undefined: ExecRunner`, `undefined: Envelope`, `undefined: ClassifyFailure`, `undefined: errExecNotFound`.

- [ ] **Step 3: Minimal implementation.** Write `backend/internal/llm/client.go`:
```go
// Package llm invokes the Claude Code CLI (`claude -p`) headlessly and extracts
// the model's JSON from the result envelope. The Runner is injectable so tests
// use canned envelopes (no real claude/network).
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Envelope is the parsed `claude -p --output-format json` result envelope.
type Envelope struct {
	Type           string  `json:"type"`
	Subtype        string  `json:"subtype"`
	IsError        bool    `json:"is_error"`
	APIErrorStatus *int    `json:"api_error_status"`
	Result         string  `json:"result"`
	StopReason     string  `json:"stop_reason"`
	SessionID      string  `json:"session_id"`
	NumTurns       int     `json:"num_turns"`
	DurationMs     int     `json:"duration_ms"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
}

// ParseEnvelope unmarshals raw `claude -p` stdout into an Envelope.
func ParseEnvelope(b []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(b, &e); err != nil {
		return Envelope{}, fmt.Errorf("llm: parse envelope: %w", err)
	}
	return e, nil
}

// Runner executes one `claude -p` call. Injectable for tests.
type Runner interface {
	Run(ctx context.Context, args []string, stdin string) (stdout []byte, err error)
}

// errExecNotFound is the sentinel for a missing claude binary (matches exec.ErrNotFound).
var errExecNotFound = exec.ErrNotFound

// ExecRunner is the production Runner backed by os/exec.
type ExecRunner struct {
	Bin string // claude binary path (Config.ClaudeBin)
}

// Run executes Bin with args, writing stdin, capturing stdout. A non-zero exit
// surfaces stderr in the error; a missing binary surfaces exec.ErrNotFound.
func (r ExecRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return stdout.Bytes(), fmt.Errorf("claude exit %d: %s", ee.ExitCode(), stderr.String())
		}
		return nil, err // includes exec.ErrNotFound
	}
	return stdout.Bytes(), nil
}

// ErrMalformedJSON is returned when the model output cannot be extracted/parsed
// even after the one retry.
var ErrMalformedJSON = errors.New("llm: malformed JSON from claude -p")

// ExtractJSON pulls the JSON object out of a claude -p .result string and
// unmarshals it into v (inner-JSON extraction rule steps 1–4; no retry).
func ExtractJSON(result string, v any) error {
	s := strings.TrimSpace(result)
	if s == "" {
		return ErrMalformedJSON
	}
	// Strip a fenced ```json / ``` block.
	if strings.HasPrefix(s, "```") {
		nl := strings.IndexByte(s, '\n')
		if nl >= 0 {
			s = s[nl+1:]
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	} else if !strings.HasPrefix(s, "{") {
		// Take the first { .. last } span.
		first := strings.IndexByte(s, '{')
		last := strings.LastIndexByte(s, '}')
		if first < 0 || last < first {
			return ErrMalformedJSON
		}
		s = s[first : last+1]
	}
	if err := json.Unmarshal([]byte(s), v); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformedJSON, err)
	}
	return nil
}

// ClassifyFailure maps a failed call (exit error and/or is_error envelope) to a
// user-facing message (success/failure determination rules).
func ClassifyFailure(env Envelope, runErr error) string {
	if errors.Is(runErr, errExecNotFound) {
		return "`claude` CLI not installed."
	}
	low := strings.ToLower(env.Result)
	// Not-logged-in / auth-failure keywords.
	for _, kw := range []string{"login", "logged in", "authenticate", "unauthorized", "credential"} {
		if strings.Contains(low, kw) {
			return "Claude not logged in — run `claude auth login`."
		}
	}
	// Usage / rate / billing-limit keywords (or an explicit API error status).
	if env.APIErrorStatus != nil {
		return "Claude rate/usage limit hit — try later."
	}
	for _, kw := range []string{"limit", "rate", "credit", "quota"} {
		if strings.Contains(low, kw) {
			return "Claude rate/usage limit hit — try later."
		}
	}
	// Generic fallback: surface the raw .result (handler maps this to a 502-class error).
	if env.Result != "" {
		return "Claude error: " + env.Result
	}
	if runErr != nil {
		return "Claude error: " + runErr.Error()
	}
	return "Claude error (unknown)."
}

// Client wraps a Runner with model/flag defaults.
type Client struct {
	Runner  Runner
	Model   string
	Timeout time.Duration
}
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/llm/
```
Expected: `ok  	help-my-run/backend/internal/llm`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/llm/client.go backend/internal/llm/client_test.go backend/internal/llm/testdata/ && git commit -m "feat(llm): Envelope, ExecRunner, ExtractJSON, ClassifyFailure

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 17: LLM — Client.Call (success/failure + one-retry loop)

**Files:**
- Modify: `backend/internal/llm/client.go` (add `CallError`, `Client.Call`)
- Test: `backend/internal/llm/call_test.go`

`Call` runs the Runner, determines success via `exitCode != 0 || env.IsError`, extracts JSON via `ExtractJSON`, and retries the whole call exactly once on extraction/unmarshal failure.

- [ ] **Step 1: Write the failing test.** Write `backend/internal/llm/call_test.go`:
```go
package llm

import (
	"context"
	"errors"
	"testing"
)

// seqRunner returns a sequence of (out,err) per call; records call count.
type seqRunner struct {
	outs  [][]byte
	errs  []error
	calls int
}

func (r *seqRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	i := r.calls
	r.calls++
	var err error
	if i < len(r.errs) {
		err = r.errs[i]
	}
	return r.outs[i], err
}

func TestCallSuccess(t *testing.T) {
	r := &seqRunner{outs: [][]byte{loadFixture(t, "stage1_envelope.json")}, errs: []error{nil}}
	c := Client{Runner: r, Model: "claude-opus-4-8"}

	var w struct {
		WeekStart string `json:"week_start"`
	}
	if err := c.Call(context.Background(), []string{"-p", "x"}, "", &w); err != nil {
		t.Fatalf("Call() error = %v", err)
	}
	if w.WeekStart != "2026-06-22" {
		t.Errorf("week_start = %q, want 2026-06-22", w.WeekStart)
	}
	if r.calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on success)", r.calls)
	}
}

func TestCallRetriesOnceOnMalformed(t *testing.T) {
	r := &seqRunner{
		outs: [][]byte{loadFixture(t, "malformed_envelope.json"), loadFixture(t, "stage1_envelope.json")},
		errs: []error{nil, nil},
	}
	c := Client{Runner: r, Model: "claude-opus-4-8"}

	var w struct {
		WeekStart string `json:"week_start"`
	}
	if err := c.Call(context.Background(), []string{"-p", "x"}, "", &w); err != nil {
		t.Fatalf("Call() error = %v, want success on retry", err)
	}
	if r.calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry)", r.calls)
	}
	if w.WeekStart != "2026-06-22" {
		t.Errorf("week_start = %q after retry, want 2026-06-22", w.WeekStart)
	}
}

func TestCallMalformedTwiceReturnsErrMalformed(t *testing.T) {
	r := &seqRunner{
		outs: [][]byte{loadFixture(t, "malformed_envelope.json"), loadFixture(t, "malformed_envelope.json")},
		errs: []error{nil, nil},
	}
	c := Client{Runner: r, Model: "claude-opus-4-8"}

	var v map[string]any
	err := c.Call(context.Background(), []string{"-p", "x"}, "", &v)
	if !errors.Is(err, ErrMalformedJSON) {
		t.Errorf("err = %v, want ErrMalformedJSON", err)
	}
	if r.calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry then give up)", r.calls)
	}
}

func TestCallFailsOnIsErrorNoRetry(t *testing.T) {
	r := &seqRunner{outs: [][]byte{loadFixture(t, "not_logged_in_envelope.json")}, errs: []error{nil}}
	c := Client{Runner: r, Model: "claude-opus-4-8"}

	var v map[string]any
	err := c.Call(context.Background(), []string{"-p", "x"}, "", &v)
	if err == nil {
		t.Fatal("Call() error = nil, want failure on is_error")
	}
	if errors.Is(err, ErrMalformedJSON) {
		t.Errorf("err = %v, want a classified failure (not malformed)", err)
	}
	if r.calls != 1 {
		t.Errorf("calls = %d, want 1 (is_error is not retried)", r.calls)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/llm/ -run TestCall
```
Expected: FAIL — `c.Call undefined (type Client has no field or method Call)`.

- [ ] **Step 3: Minimal implementation.** Append to `backend/internal/llm/client.go`:
```go
// CallError carries a classified, user-facing failure message from a claude -p call.
type CallError struct {
	Msg string
	Err error
}

func (e *CallError) Error() string { return e.Msg }
func (e *CallError) Unwrap() error { return e.Err }

// Call runs one claude -p invocation (Runner), determines success
// (exitCode==0 && !IsError), extracts the model JSON into v, and retries the
// entire call exactly once on extraction/unmarshal failure. A classified
// (non-malformed) failure is returned immediately without retry.
func (c *Client) Call(ctx context.Context, args []string, stdin string, v any) error {
	attempt := func() (retryable bool, err error) {
		out, runErr := c.Runner.Run(ctx, args, stdin)

		var env Envelope
		if len(out) > 0 {
			env, _ = ParseEnvelope(out)
		}

		// Failure: non-zero exit (runErr) OR is_error envelope. Not retryable.
		if runErr != nil || env.IsError {
			return false, &CallError{Msg: ClassifyFailure(env, runErr), Err: runErr}
		}

		// Success path: extract JSON. Malformed -> retryable.
		if err := ExtractJSON(env.Result, v); err != nil {
			return true, err
		}
		return false, nil
	}

	retryable, err := attempt()
	if err == nil {
		return nil
	}
	if !retryable {
		return err
	}
	// One retry.
	_, err = attempt()
	return err
}
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/llm/
```
Expected: `ok  	help-my-run/backend/internal/llm`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/llm/client.go backend/internal/llm/call_test.go && git commit -m "feat(llm): Client.Call with success/failure branch and one-retry on malformed JSON

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---
### Task 18: LLM — types (Stage-1/Stage-2 parsed structs)

**Files:**
- Create: `backend/internal/llm/types.go` (`Load`, `CrossFitDay`, `CrossFitWeekParsed`, `PlanDay`, `PlanParsed`)
- Test: `backend/internal/llm/types_test.go`

- [ ] **Step 1: Write the failing test.** Write `backend/internal/llm/types_test.go`:
```go
package llm

import (
	"encoding/json"
	"testing"
)

func TestCrossFitWeekParsedRoundTrip(t *testing.T) {
	src := `{"week_start":"2026-06-22","days":[{"date":"2026-06-22","dow":"Mon","has_crossfit":true,"focus":"Back squat","cns_load":"high","leg_load":"high","notes":"Heavy"}]}`
	var w CrossFitWeekParsed
	if err := json.Unmarshal([]byte(src), &w); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if w.WeekStart != "2026-06-22" || len(w.Days) != 1 {
		t.Fatalf("parsed = %+v", w)
	}
	d := w.Days[0]
	if d.Dow != "Mon" || !d.HasCrossFit || d.CNSLoad != LoadHigh || d.LegLoad != LoadHigh {
		t.Errorf("day = %+v, want Mon/has/high/high", d)
	}
	b, _ := json.Marshal(w)
	if !json_contains(b, `"has_crossfit":true`) || !json_contains(b, `"cns_load":"high"`) {
		t.Errorf("marshal = %s, want snake_case keys", b)
	}
}

func TestPlanParsedRoundTrip(t *testing.T) {
	src := `{"fitness_summary":"ok","weekly_target_km":20,"days":[{"date":"2026-06-23","dow":"Tue","run_type":"easy","distance_km":5,"pace_target":"6:00/km","time_note":"~20:00","optional_if_cns":true,"rationale":"why"}],"week_rationale":"para","one_flag":"flag"}`
	var p PlanParsed
	if err := json.Unmarshal([]byte(src), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.WeeklyTargetKm != 20 || p.OneFlag != "flag" || len(p.Days) != 1 {
		t.Fatalf("parsed = %+v", p)
	}
	d := p.Days[0]
	if d.RunType != "easy" || d.DistanceKm != 5 || !d.OptionalIfCNS {
		t.Errorf("day = %+v, want easy/5/optional", d)
	}
	b, _ := json.Marshal(p)
	if !json_contains(b, `"optional_if_cns":true`) || !json_contains(b, `"weekly_target_km":20`) {
		t.Errorf("marshal = %s, want snake_case keys", b)
	}
}

func json_contains(b []byte, sub string) bool { return contains(string(b), sub) }
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/llm/ -run "Parsed"
```
Expected: FAIL — `undefined: CrossFitWeekParsed`, `undefined: PlanParsed`, `undefined: LoadHigh`.

- [ ] **Step 3: Minimal implementation.** Write `backend/internal/llm/types.go`:
```go
package llm

// Load is the CNS/leg load enum (exactly low|med|high).
type Load string

const (
	LoadLow  Load = "low"
	LoadMed  Load = "med"
	LoadHigh Load = "high"
)

// CrossFitDay is one day in the Stage-1 parsed week.
type CrossFitDay struct {
	Date        string `json:"date"`
	Dow         string `json:"dow"`
	HasCrossFit bool   `json:"has_crossfit"`
	Focus       string `json:"focus"`
	CNSLoad     Load   `json:"cns_load"`
	LegLoad     Load   `json:"leg_load"`
	Notes       string `json:"notes"`
}

// CrossFitWeekParsed is the Stage-1 model output.
type CrossFitWeekParsed struct {
	WeekStart string        `json:"week_start"`
	Days      []CrossFitDay `json:"days"`
}

// PlanDay is one day in the Stage-2 plan.
type PlanDay struct {
	Date          string  `json:"date"`
	Dow           string  `json:"dow"`
	RunType       string  `json:"run_type"`
	DistanceKm    float64 `json:"distance_km"`
	PaceTarget    string  `json:"pace_target"`
	TimeNote      string  `json:"time_note"`
	OptionalIfCNS bool    `json:"optional_if_cns"`
	Rationale     string  `json:"rationale"`
}

// PlanParsed is the Stage-2 model output.
type PlanParsed struct {
	FitnessSummary string    `json:"fitness_summary"`
	WeeklyTargetKm float64   `json:"weekly_target_km"`
	Days           []PlanDay `json:"days"`
	WeekRationale  string    `json:"week_rationale"`
	OneFlag        string    `json:"one_flag"`
}
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/llm/
```
Expected: `ok  	help-my-run/backend/internal/llm`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/llm/types.go backend/internal/llm/types_test.go && git commit -m "feat(llm): Stage-1/Stage-2 parsed structs + Load enum

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 19: Coach — prompts (Stage-1 + Coach Brain templates)

**Files:**
- Create: `backend/internal/coach/prompts.go` (`stage1Template`, `buildStage1Prompt`, `coachBrainPrompt`)
- Test: `backend/internal/coach/prompts_test.go`

- [ ] **Step 1: Write the failing test.** Write `backend/internal/coach/prompts_test.go`:
```go
package coach

import (
	"strings"
	"testing"
)

func TestBuildStage1PromptReferencesImage(t *testing.T) {
	p := buildStage1Prompt("/data/crossfit/2026-06-22.jpg", "2026-06-22")
	if !strings.Contains(p, "/data/crossfit/2026-06-22.jpg") {
		t.Errorf("prompt missing image path:\n%s", p)
	}
	if !strings.Contains(p, "2026-06-22") {
		t.Errorf("prompt missing week_start")
	}
	for _, k := range []string{"week_start", "has_crossfit", "cns_load", "leg_load"} {
		if !strings.Contains(p, k) {
			t.Errorf("prompt missing key hint %q", k)
		}
	}
	if !strings.Contains(strings.ToLower(p), "json") {
		t.Errorf("prompt does not ask for JSON")
	}
	if !strings.Contains(strings.ToLower(p), "read") {
		t.Errorf("prompt does not instruct read")
	}
}

func TestCoachBrainPromptHasGuidance(t *testing.T) {
	if !strings.Contains(strings.ToLower(coachBrainPrompt), "coach") {
		t.Errorf("coachBrainPrompt missing coach framing")
	}
	for _, k := range []string{"weekly_target_km", "run_type", "optional_if_cns", "week_rationale", "one_flag"} {
		if !strings.Contains(coachBrainPrompt, k) {
			t.Errorf("coachBrainPrompt missing output key %q", k)
		}
	}
	for _, want := range []string{"10%", "CNS", "Thursday", "JSON"} {
		if !strings.Contains(coachBrainPrompt, want) {
			t.Errorf("coachBrainPrompt missing rule mention %q", want)
		}
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/coach/
```
Expected: FAIL — `undefined: buildStage1Prompt`, `undefined: coachBrainPrompt`.

- [ ] **Step 3: Minimal implementation.** Write `backend/internal/coach/prompts.go`:
```go
package coach

import "fmt"

// stage1Template instructs claude -p to read the saved schedule image and emit
// ONLY the Stage-1 CrossFit-week JSON.
const stage1Template = `You are parsing a CrossFit box's weekly programming photo for a runner who also does CrossFit.

Read the image at this absolute path: %s

Known athlete pattern (hints): CrossFit Monday–Friday ~18:15–19:15; Thursday is a barbell-skill day (lighter legs/CNS); Saturday/Sunday usually rest.

The week starts on Monday %s.

Produce EXACTLY 7 day objects (Mon→Sun). Output ONLY a single JSON object (no prose, no markdown fences) of this shape:
{
  "week_start": "%s",
  "days": [
    {"date":"YYYY-MM-DD","dow":"Mon","has_crossfit":true,"focus":"...","cns_load":"low|med|high","leg_load":"low|med|high","notes":"..."}
  ]
}
Rules: cns_load and leg_load are exactly one of "low","med","high". focus and notes are "" when empty. has_crossfit is false on rest days.`

// buildStage1Prompt fills the Stage-1 template with the image path and week start.
func buildStage1Prompt(imagePath, weekStart string) string {
	return fmt.Sprintf(stage1Template, imagePath, weekStart, weekStart)
}

// coachBrainPrompt is the Stage-2 instruction block prepended to the context
// pack (piped on stdin). It asks for ONLY the Stage-2 plan JSON.
const coachBrainPrompt = `You are a CrossFit-aware running coach. Build a 7-day running plan for the upcoming week from the JSON context pack on stdin (computed fitness metrics, athlete profile + constraints, the parsed CrossFit week, and last week's plan if present).

Coaching rules:
- Periodize toward general aerobic improvement. Ramp weekly volume by no more than ~10% over baseline; include a cutback week when the metrics flag one.
- Place hard/quality runs (tempo, intervals, long) on low-CNS and low-leg CrossFit days and on weekends. Thursday is a barbell-skill day (lighter) — a good quality slot.
- Keep hard runs OFF heavy-leg / high-CNS CrossFit days. Easy stays easy.
- Evening doubles run ~20:00 after CrossFit; set time_note accordingly. Mark a run optional_if_cns:true when it follows a high-CNS day and could be skipped.
- Respect the athlete's run_constraints and weekly target; aim near safe_weekly_target_km.

Output ONLY a single JSON object (no prose, no markdown fences) of this shape:
{
  "fitness_summary": "...",
  "weekly_target_km": 0,
  "days": [
    {"date":"YYYY-MM-DD","dow":"Mon","run_type":"easy|tempo|recovery|long|rest|intervals","distance_km":0,"pace_target":"5:45/km","time_note":"~20:00 after CrossFit","optional_if_cns":false,"rationale":"one line"}
  ],
  "week_rationale": "paragraph on placement + progression",
  "one_flag": "the single most important caution"
}
Produce EXACTLY 7 day objects (Mon→Sun). distance_km is 0 and pace_target/time_note are "" for rest days.`
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/coach/
```
Expected: `ok  	help-my-run/backend/internal/coach`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/coach/prompts.go backend/internal/coach/prompts_test.go && git commit -m "feat(coach): Stage-1 image prompt + Coach Brain prompt

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---
### Task 20: Coach — ContextPack, ParseCrossFit, GeneratePlan, Fitness (orchestration)

**Files:**
- Create: `backend/internal/coach/coach.go` (`ContextPack`, `ProfilePack`, `Coach` struct + constructor `New`, the three methods, arg builders, `buildContextPack`)
- Test: `backend/internal/coach/coach_test.go`

The `Coach` struct holds `*store.Store`, an `*llm.Client`, the model string, and the image dir. It satisfies the `api.Coach` interface. `ParseCrossFit` builds Stage-1 args and calls `llm.Client.Call`; `GeneratePlan` builds the context pack, pipes it on stdin with Coach Brain args; `Fitness` calls `metrics.ComputeFitness`. **Requires Tasks 3–5 (store getters), 15 (ComputeFitness), 16–19 (llm + prompts).**

- [ ] **Step 1: Write the failing test.** Write `backend/internal/coach/coach_test.go`:
```go
package coach

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

func newCoachStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "coach.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// captureRunner records the args + stdin and returns a canned envelope.
type captureRunner struct {
	out  []byte
	args []string
	body string
}

func (r *captureRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	r.args = args
	r.body = stdin
	return r.out, nil
}

const stage1Env = `{"type":"result","subtype":"success","is_error":false,"result":"{\"week_start\":\"2026-06-22\",\"days\":[{\"date\":\"2026-06-22\",\"dow\":\"Mon\",\"has_crossfit\":true,\"focus\":\"Back squat\",\"cns_load\":\"high\",\"leg_load\":\"high\",\"notes\":\"\"}]}"}`

const stage2Env = `{"type":"result","subtype":"success","is_error":false,"result":"{\"fitness_summary\":\"ok\",\"weekly_target_km\":20,\"days\":[{\"date\":\"2026-06-22\",\"dow\":\"Mon\",\"run_type\":\"rest\",\"distance_km\":0,\"pace_target\":\"\",\"time_note\":\"\",\"optional_if_cns\":false,\"rationale\":\"rest\"}],\"week_rationale\":\"para\",\"one_flag\":\"flag\"}"}`

func TestParseCrossFitBuildsStage1AndParses(t *testing.T) {
	s := newCoachStore(t)
	r := &captureRunner{out: []byte(stage1Env)}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	week, raw, err := c.ParseCrossFit(context.Background(), "2026-06-22", "/tmp/cfimg/2026-06-22.jpg")
	if err != nil {
		t.Fatalf("ParseCrossFit error = %v", err)
	}
	if week.WeekStart != "2026-06-22" || len(week.Days) != 1 || week.Days[0].CNSLoad != llm.LoadHigh {
		t.Errorf("parsed week = %+v", week)
	}
	if raw == "" {
		t.Error("raw response empty, want canonical re-marshaled week JSON")
	}
	joined := strings.Join(r.args, " ")
	if !strings.Contains(joined, "/tmp/cfimg/2026-06-22.jpg") {
		t.Errorf("args missing image path: %v", r.args)
	}
	if !hasPair(r.args, "--output-format", "json") || !hasPair(r.args, "--model", "claude-opus-4-8") {
		t.Errorf("args missing model/output-format: %v", r.args)
	}
	if !hasFlag(r.args, "--add-dir") || !hasFlag(r.args, "-p") {
		t.Errorf("args missing --add-dir/-p: %v", r.args)
	}
}

func TestParseCrossFitMalformedThenValidRetriesOnce(t *testing.T) {
	s := newCoachStore(t)
	r := &seqCoachRunner{outs: [][]byte{
		[]byte(`{"type":"result","is_error":false,"result":"not json"}`),
		[]byte(stage1Env),
	}}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")
	week, _, err := c.ParseCrossFit(context.Background(), "2026-06-22", "/tmp/cfimg/2026-06-22.jpg")
	if err != nil {
		t.Fatalf("ParseCrossFit error = %v", err)
	}
	if r.calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry)", r.calls)
	}
	if week.WeekStart != "2026-06-22" {
		t.Errorf("parsed week wrong after retry: %+v", week)
	}
}

type seqCoachRunner struct {
	outs  [][]byte
	calls int
}

func (r *seqCoachRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	i := r.calls
	r.calls++
	if i < len(r.outs) {
		return r.outs[i], nil
	}
	return r.outs[len(r.outs)-1], nil
}

func TestGeneratePlanPipesContextPackAndParses(t *testing.T) {
	s := newCoachStore(t)
	_ = s.UpsertCrossFitWeek(store.CrossFitWeek{
		WeekStart:  "2026-06-22",
		ParsedJSON: `{"week_start":"2026-06-22","days":[{"date":"2026-06-22","dow":"Mon","has_crossfit":true,"focus":"x","cns_load":"high","leg_load":"high","notes":""}]}`,
	})
	r := &captureRunner{out: []byte(stage2Env)}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	plan, ctxPack, model, err := c.GeneratePlan(context.Background(), "2026-06-22", nil)
	if err != nil {
		t.Fatalf("GeneratePlan error = %v", err)
	}
	if plan.WeeklyTargetKm != 20 || plan.OneFlag != "flag" || len(plan.Days) != 1 {
		t.Errorf("plan = %+v", plan)
	}
	if model != "claude-opus-4-8" {
		t.Errorf("model = %q, want claude-opus-4-8", model)
	}
	if r.body == "" {
		t.Fatal("stdin empty, want context pack piped")
	}
	if !strings.Contains(r.body, `"crossfit_week"`) || !strings.Contains(r.body, `"metrics"`) {
		t.Errorf("piped context pack missing fields: %s", r.body)
	}
	if ctxPack != r.body {
		t.Errorf("returned context pack != piped stdin")
	}
	joined := strings.Join(r.args, " ")
	if !strings.Contains(joined, "running coach") {
		t.Errorf("Stage-2 args missing coach brain: %v", r.args)
	}
	if hasFlag(r.args, "--add-dir") {
		t.Errorf("Stage-2 must not pass --add-dir: %v", r.args)
	}
}

func TestGeneratePlanUsesEditedWeekOverStored(t *testing.T) {
	s := newCoachStore(t)
	_ = s.UpsertCrossFitWeek(store.CrossFitWeek{
		WeekStart:  "2026-06-22",
		ParsedJSON: `{"week_start":"2026-06-22","days":[]}`,
	})
	r := &captureRunner{out: []byte(stage2Env)}
	c := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8", "/tmp/cfimg")

	edited := &llm.CrossFitWeekParsed{
		WeekStart: "2026-06-22",
		Days:      []llm.CrossFitDay{{Date: "2026-06-22", Dow: "Mon", HasCrossFit: false, Focus: "EDITED", CNSLoad: "low", LegLoad: "low"}},
	}
	if _, _, _, err := c.GeneratePlan(context.Background(), "2026-06-22", edited); err != nil {
		t.Fatalf("GeneratePlan(edited) error = %v", err)
	}
	if !strings.Contains(r.body, "EDITED") {
		t.Errorf("context pack did not use edited week: %s", r.body)
	}
}

func TestFitnessComputesFromStore(t *testing.T) {
	s := newCoachStore(t)
	_ = s.UpsertActivity(store.Activity{
		StravaID: 1, Type: "Run", Name: "r", StartTime: "2026-06-19T06:00:00Z",
		DistanceM: 6000, MovingTimeS: 2160, ElapsedTimeS: 2160, RawJSON: "{}",
	})
	c := New(s, &llm.Client{Runner: &captureRunner{}, Model: "m"}, "m", "/tmp/cfimg")
	m, err := c.Fitness(context.Background())
	if err != nil {
		t.Fatalf("Fitness error = %v", err)
	}
	if m.SafeWeeklyTargetKm <= 0 {
		t.Errorf("SafeWeeklyTargetKm = %v, want > 0 (profile target fallback)", m.SafeWeeklyTargetKm)
	}
}

func hasFlag(args []string, f string) bool {
	for _, a := range args {
		if a == f {
			return true
		}
	}
	return false
}

func hasPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/coach/ -run "ParseCrossFit|GeneratePlan|Fitness"
```
Expected: FAIL — `undefined: New` (and the methods).

- [ ] **Step 3: Minimal implementation.** Write `backend/internal/coach/coach.go`:
```go
// Package coach assembles the context pack and orchestrates the two claude -p
// stages (image parse + plan generation) on top of store + metrics + llm.
package coach

import (
	"context"
	"encoding/json"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/store"
)

// Coach wires the store, the llm client, the model name, and the image dir.
type Coach struct {
	store    *store.Store
	llm      *llm.Client
	model    string
	imageDir string
}

// New constructs a Coach.
func New(s *store.Store, c *llm.Client, model, imageDir string) *Coach {
	return &Coach{store: s, llm: c, model: model, imageDir: imageDir}
}

// ProfilePack is the profile slice of the context pack.
type ProfilePack struct {
	TargetWeeklyKm  float64         `json:"target_weekly_km"`
	ProgressionMode string          `json:"progression_mode"`
	Zone2CeilingBpm *int64          `json:"zone2_ceiling_bpm"`
	ThresholdBpm    *int64          `json:"threshold_bpm"`
	MaxHRBpm        *int64          `json:"max_hr_bpm"`
	RunConstraints  json.RawMessage `json:"run_constraints"`
	GoalText        string          `json:"goal_text"`
}

// ContextPack is the Stage-2 input (piped to stdin; stored for reproducibility).
type ContextPack struct {
	GeneratedAt  string                 `json:"generated_at"`
	WeekStart    string                 `json:"week_start"`
	Metrics      metrics.FitnessMetrics `json:"metrics"`
	Profile      ProfilePack            `json:"profile"`
	CrossFitWeek llm.CrossFitWeekParsed `json:"crossfit_week"`
	LastWeekPlan *llm.PlanParsed        `json:"last_week_plan"`
}

// stage1Args builds the claude -p argv for Stage 1 (image → CrossFit week).
func (c *Coach) stage1Args(prompt string) []string {
	return []string{
		"-p", prompt,
		"--model", c.model,
		"--output-format", "json",
		"--allowedTools", "Read",
		"--add-dir", c.imageDir,
		"--no-session-persistence",
	}
}

// stage2Args builds the claude -p argv for Stage 2 (context pack → plan).
func (c *Coach) stage2Args() []string {
	return []string{
		"-p", coachBrainPrompt,
		"--model", c.model,
		"--output-format", "json",
		"--allowedTools", "",
		"--no-session-persistence",
	}
}

// ParseCrossFit runs Stage 1: reads the saved image, returns the parsed week and
// its canonical JSON re-marshaling (stored as crossfit_weeks.raw_response — NOT the
// byte-for-byte claude -p .result, which Call does not surface). Storage is the
// handler's responsibility.
func (c *Coach) ParseCrossFit(ctx context.Context, weekStart, imagePath string) (llm.CrossFitWeekParsed, string, error) {
	prompt := buildStage1Prompt(imagePath, weekStart)
	args := c.stage1Args(prompt)

	var week llm.CrossFitWeekParsed
	if err := c.llm.Call(ctx, args, "", &week); err != nil {
		return llm.CrossFitWeekParsed{}, "", err
	}
	raw, _ := json.Marshal(week)
	return week, string(raw), nil
}

// GeneratePlan runs Stage 2: builds the context pack (using edited if supplied,
// else the stored week), pipes it on stdin, returns the plan, the serialized
// context pack, and the model.
func (c *Coach) GeneratePlan(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (llm.PlanParsed, string, string, error) {
	pack, err := c.buildContextPack(ctx, weekStart, edited)
	if err != nil {
		return llm.PlanParsed{}, "", "", err
	}
	packJSON, err := json.Marshal(pack)
	if err != nil {
		return llm.PlanParsed{}, "", "", err
	}

	var plan llm.PlanParsed
	if err := c.llm.Call(ctx, c.stage2Args(), string(packJSON), &plan); err != nil {
		return llm.PlanParsed{}, "", "", err
	}
	return plan, string(packJSON), c.model, nil
}

// Fitness computes the current fitness read from the local store.
func (c *Coach) Fitness(ctx context.Context) (metrics.FitnessMetrics, error) {
	acts, err := c.store.ListActivities(200)
	if err != nil {
		return metrics.FitnessMetrics{}, err
	}
	rec, err := c.store.ListRecovery(60)
	if err != nil {
		return metrics.FitnessMetrics{}, err
	}
	prof, err := c.store.GetAthleteProfile()
	if err != nil {
		return metrics.FitnessMetrics{}, err
	}
	return metrics.ComputeFitness(acts, rec, prof, time.Now().UTC()), nil
}

// buildContextPack assembles metrics + profile + crossfit week + last plan.
func (c *Coach) buildContextPack(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (ContextPack, error) {
	prof, err := c.store.GetAthleteProfile()
	if err != nil {
		return ContextPack{}, err
	}
	fit, err := c.Fitness(ctx)
	if err != nil {
		return ContextPack{}, err
	}

	// CrossFit week: edited overrides stored.
	var week llm.CrossFitWeekParsed
	if edited != nil {
		week = *edited
	} else {
		stored, gerr := c.store.GetCrossFitWeek(weekStart)
		if gerr != nil {
			return ContextPack{}, gerr
		}
		if uerr := json.Unmarshal([]byte(stored.ParsedJSON), &week); uerr != nil {
			return ContextPack{}, uerr
		}
	}

	// Last week's plan (best-effort; nil if none).
	var last *llm.PlanParsed
	prevMonday, perr := time.Parse("2006-01-02", weekStart)
	if perr == nil {
		prev := prevMonday.AddDate(0, 0, -7).Format("2006-01-02")
		if lp, lerr := c.store.GetLatestPlan(prev); lerr == nil {
			var pp llm.PlanParsed
			if json.Unmarshal([]byte(lp.PlanJSON), &pp) == nil {
				last = &pp
			}
		}
	}

	rc := json.RawMessage(prof.RunConstraintsJSON)
	if len(rc) == 0 || !json.Valid(rc) {
		rc = json.RawMessage(`{}`)
	}
	return ContextPack{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		WeekStart:   weekStart,
		Metrics:     fit,
		Profile: ProfilePack{
			TargetWeeklyKm:  prof.TargetWeeklyKm,
			ProgressionMode: prof.ProgressionMode,
			Zone2CeilingBpm: prof.Zone2CeilingBpm,
			ThresholdBpm:    prof.ThresholdBpm,
			MaxHRBpm:        prof.MaxHRBpm,
			RunConstraints:  rc,
			GoalText:        prof.GoalText,
		},
		CrossFitWeek: week,
		LastWeekPlan: last,
	}, nil
}
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/coach/
```
Expected: `ok  	help-my-run/backend/internal/coach`.

- [ ] **Step 5: Commit.**
```
git add backend/internal/coach/coach.go backend/internal/coach/coach_test.go && git commit -m "feat(coach): ContextPack + ParseCrossFit/GeneratePlan/Fitness orchestration

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---
### Task 21: API — Coach interface, Deps extension, M1 DTOs, image save helper

**Files:**
- Modify: `backend/internal/api/router.go` (add `Coach` interface + `Coach`/`ImageDir` fields to `Deps`)
- Modify: `backend/internal/api/dto.go` (add M1 DTOs)
- Create: `backend/internal/api/images.go` (`saveUploadedImage`)
- Test: `backend/internal/api/images_test.go`

The `Coach` interface mirrors `coach.Coach`'s method set but is declared in `api` to avoid the cycle; `*coach.Coach` satisfies it structurally via wiring in main.go.

- [ ] **Step 1: Write the failing test.** Write `backend/internal/api/images_test.go`:
```go
package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// multipartReq builds a request with a file field "image" and text fields.
func multipartReq(t *testing.T, fieldFile, filename string, content []byte, textFields map[string]string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(fieldFile, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write file: %v", err)
	}
	for k, v := range textFields {
		_ = mw.WriteField(k, v)
	}
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/crossfit/parse", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestSaveUploadedImage(t *testing.T) {
	dir := t.TempDir()
	req := multipartReq(t, "image", "schedule.png", []byte("PNGDATA"), map[string]string{"week_start": "2026-06-22"})
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		t.Fatalf("ParseMultipartForm: %v", err)
	}
	file, hdr, err := req.FormFile("image")
	if err != nil {
		t.Fatalf("FormFile: %v", err)
	}
	defer file.Close()

	path, err := saveUploadedImage(dir, "2026-06-22", file, hdr)
	if err != nil {
		t.Fatalf("saveUploadedImage: %v", err)
	}
	if !strings.HasPrefix(path, dir) || !strings.Contains(path, "2026-06-22") {
		t.Errorf("path = %q, want under %q with week_start", path, dir)
	}
	if filepath.Ext(path) != ".png" {
		t.Errorf("ext = %q, want .png (preserved from upload)", filepath.Ext(path))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved: %v", err)
	}
	if string(got) != "PNGDATA" {
		t.Errorf("saved content = %q, want PNGDATA", got)
	}
}

func TestSaveUploadedImageCreatesDir(t *testing.T) {
	base := filepath.Join(t.TempDir(), "nested", "crossfit")
	req := multipartReq(t, "image", "x.jpg", []byte("J"), map[string]string{"week_start": "2026-06-22"})
	_ = req.ParseMultipartForm(10 << 20)
	file, hdr, _ := req.FormFile("image")
	defer file.Close()
	path, err := saveUploadedImage(base, "2026-06-22", file, hdr)
	if err != nil {
		t.Fatalf("saveUploadedImage (mkdir): %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("saved file missing: %v", err)
	}
}
```

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run TestSaveUploadedImage
```
Expected: FAIL — `undefined: saveUploadedImage`.

- [ ] **Step 3a: Image save helper.** Write `backend/internal/api/images.go`:
```go
package api

import (
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
)

// saveUploadedImage writes the uploaded file to <dir>/<weekStart><ext>, creating
// dir if needed, and returns the absolute saved path. The extension is taken
// from the original filename (default .jpg).
func saveUploadedImage(dir, weekStart string, file multipart.File, hdr *multipart.FileHeader) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(hdr.Filename))
	if ext == "" {
		ext = ".jpg"
	}
	dest := filepath.Join(dir, weekStart+ext)
	out, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dest)
	if err != nil {
		return dest, nil
	}
	return abs, nil
}
```

- [ ] **Step 3b: Coach interface + Deps + DTOs.** Edit `backend/internal/api/router.go` — add the imports and the interface, extend `Deps`.

Add to the import block:
```go
	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
```
Add the interface above `Deps` and the new fields:
```go
// Coach is the M1 plan-engine seam, injected from main.go (avoids an import
// cycle: api must not import coach). *coach.Coach satisfies it structurally.
type Coach interface {
	ParseCrossFit(ctx context.Context, weekStart, imagePath string) (llm.CrossFitWeekParsed, string, error)
	GeneratePlan(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (llm.PlanParsed, string, string, error)
	Fitness(ctx context.Context) (metrics.FitnessMetrics, error)
}

// Deps are the handler dependencies injected by main.go (and tests).
type Deps struct {
	Store    *store.Store
	Strava   *strava.Client
	APIToken string
	SyncFunc SyncFunc
	Coach    Coach  // M1
	ImageDir string // M1: where uploaded CrossFit images are saved
}
```
(Keep the existing M0 `Deps` fields; add `Coach`/`ImageDir` to the existing struct — do not delete `Store`/`Strava`/`APIToken`/`SyncFunc`.)

Append the M1 DTOs to `backend/internal/api/dto.go`:
```go
// --- M1 /api/profile ---
type profileDTO struct {
	TargetWeeklyKm     float64 `json:"target_weekly_km"`
	ProgressionMode    string  `json:"progression_mode"`
	Zone2CeilingBpm    *int64  `json:"zone2_ceiling_bpm"`
	ThresholdBpm       *int64  `json:"threshold_bpm"`
	MaxHRBpm           *int64  `json:"max_hr_bpm"`
	RunConstraintsJSON string  `json:"run_constraints_json"`
	GoalText           string  `json:"goal_text"`
	UpdatedAt          string  `json:"updated_at,omitempty"`
}

// --- M1 /api/plan/generate + /api/plan ---
type planResponseDTO struct {
	ID             int64        `json:"id"`
	WeekStart      string       `json:"week_start"`
	GeneratedAt    string       `json:"generated_at"`
	FitnessSummary string       `json:"fitness_summary"`
	WeeklyTargetKm float64      `json:"weekly_target_km"`
	Days           []planDayDTO `json:"days"`
	WeekRationale  string       `json:"week_rationale"`
	OneFlag        string       `json:"one_flag"`
}
type planDayDTO struct {
	Date          string  `json:"date"`
	Dow           string  `json:"dow"`
	RunType       string  `json:"run_type"`
	DistanceKm    float64 `json:"distance_km"`
	PaceTarget    string  `json:"pace_target"`
	TimeNote      string  `json:"time_note"`
	OptionalIfCNS bool    `json:"optional_if_cns"`
	Rationale     string  `json:"rationale"`
}
```
(`/api/fitness` returns `metrics.FitnessMetrics` directly; `/api/crossfit/parse` returns `llm.CrossFitWeekParsed` directly — their json tags ARE the contract.)

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run TestSaveUploadedImage && go build ./...
```
Expected: image tests pass; `go build ./...` succeeds. Existing handler tests still pass because `Coach`/`ImageDir` are optional fields (nil/empty default).

- [ ] **Step 5: Commit.**
```
git add backend/internal/api/router.go backend/internal/api/dto.go backend/internal/api/images.go backend/internal/api/images_test.go && git commit -m "feat(api): Coach interface, M1 Deps fields, DTOs, image save helper

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 22: API — handlers (profile, crossfit/parse, plan/generate, plan, fitness) + routes

**Files:**
- Create: `backend/internal/api/profile_handlers.go` (`profile`, `updateProfile`, `toProfileDTO`)
- Create: `backend/internal/api/plan_handlers.go` (`crossfitParse`, `planGenerate`, `plan`, `fitness`, `toPlanResponse`)
- Modify: `backend/internal/api/router.go` (register the 6 routes inside `r.Group`)
- Modify: `backend/internal/api/handlers_test.go` (extend `newTestServer` with a fake `Coach` + `ImageDir`)
- Create: `backend/internal/api/m1_handlers_test.go`

- [ ] **Step 1: Write the failing test.** First extend `newTestServer` in `backend/internal/api/handlers_test.go` to inject a fake coach and image dir. In the `Deps{...}` literal in `newTestServer`, add the two new fields (keep the existing M0 fields):
```go
		Coach:    &fakeCoach{},
		ImageDir: t.TempDir(),
```

Then write `backend/internal/api/m1_handlers_test.go`:
```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/metrics"
	"help-my-run/backend/internal/store"
)

// fakeCoach is the injected api.Coach for handler tests.
type fakeCoach struct {
	parseErr   error
	genErr     error
	fitnessErr error
	lastWeek   string
	lastEdited *llm.CrossFitWeekParsed
	lastImage  string
}

func (f *fakeCoach) ParseCrossFit(ctx context.Context, weekStart, imagePath string) (llm.CrossFitWeekParsed, string, error) {
	f.lastWeek = weekStart
	f.lastImage = imagePath
	if f.parseErr != nil {
		return llm.CrossFitWeekParsed{}, "", f.parseErr
	}
	return llm.CrossFitWeekParsed{
		WeekStart: weekStart,
		Days:      []llm.CrossFitDay{{Date: weekStart, Dow: "Mon", HasCrossFit: true, Focus: "Squat", CNSLoad: "high", LegLoad: "high"}},
	}, `{"week_start":"` + weekStart + `"}`, nil
}

func (f *fakeCoach) GeneratePlan(ctx context.Context, weekStart string, edited *llm.CrossFitWeekParsed) (llm.PlanParsed, string, string, error) {
	f.lastWeek = weekStart
	f.lastEdited = edited
	if f.genErr != nil {
		return llm.PlanParsed{}, "", "", f.genErr
	}
	return llm.PlanParsed{
		FitnessSummary: "ok read",
		WeeklyTargetKm: 20,
		Days:           []llm.PlanDay{{Date: weekStart, Dow: "Mon", RunType: "rest", DistanceKm: 0, OptionalIfCNS: false, Rationale: "rest"}},
		WeekRationale:  "para",
		OneFlag:        "flag",
	}, `{"context":"pack"}`, "claude-opus-4-8", nil
}

func (f *fakeCoach) Fitness(ctx context.Context) (metrics.FitnessMetrics, error) {
	if f.fitnessErr != nil {
		return metrics.FitnessMetrics{}, f.fitnessErr
	}
	return metrics.FitnessMetrics{WeeklyVolumeKm: 18.2, SafeWeeklyTargetKm: 20, RecoveryTrend: "improving"}, nil
}

func doBody(t *testing.T, h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProfileGetSeeded(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/profile", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body profileDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TargetWeeklyKm != 20 || body.ProgressionMode != "build" {
		t.Errorf("profile = %+v, want target 20 build", body)
	}
}

func TestProfileRequiresAuth(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/profile", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestProfilePut(t *testing.T) {
	h, s := newTestServer(t)
	body := `{"target_weekly_km":25,"progression_mode":"hold","zone2_ceiling_bpm":140,"threshold_bpm":165,"max_hr_bpm":190,"run_constraints_json":"{\"crossfit_days\":[\"Mon\"]}","goal_text":"Build cardio"}`
	rec := doBody(t, h, http.MethodPut, "/api/profile", testToken, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var out profileDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.TargetWeeklyKm != 25 || out.ProgressionMode != "hold" || out.UpdatedAt == "" {
		t.Errorf("put resp = %+v, want target 25 hold updated_at set", out)
	}
	p, _ := s.GetAthleteProfile()
	if p.TargetWeeklyKm != 25 || p.GoalText != "Build cardio" {
		t.Errorf("stored = %+v, want target 25 goal set", p)
	}
}

func TestCrossfitParse(t *testing.T) {
	h, s := newTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("image", "schedule.png")
	_, _ = fw.Write([]byte("PNG"))
	_ = mw.WriteField("week_start", "2026-06-22")
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/crossfit/parse", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var week llm.CrossFitWeekParsed
	if err := json.Unmarshal(rec.Body.Bytes(), &week); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if week.WeekStart != "2026-06-22" || len(week.Days) != 1 {
		t.Errorf("week = %+v, want 2026-06-22 1 day", week)
	}
	if _, err := s.GetCrossFitWeek("2026-06-22"); err != nil {
		t.Errorf("crossfit week not stored: %v", err)
	}
}

func TestCrossfitParseMissingFile(t *testing.T) {
	h, _ := newTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("week_start", "2026-06-22")
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/crossfit/parse", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (missing file)", rec.Code)
	}
}

func TestPlanGenerate(t *testing.T) {
	h, s := newTestServer(t)
	_ = s.UpsertCrossFitWeek(store.CrossFitWeek{
		WeekStart:  "2026-06-22",
		ParsedJSON: `{"week_start":"2026-06-22","days":[]}`,
	})
	rec := doBody(t, h, http.MethodPost, "/api/plan/generate", testToken, `{"week_start":"2026-06-22"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body planResponseDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID <= 0 || body.WeekStart != "2026-06-22" || body.GeneratedAt == "" {
		t.Errorf("plan resp = %+v, want id/week/generated_at set", body)
	}
	if body.WeeklyTargetKm != 20 || body.OneFlag != "flag" || len(body.Days) != 1 {
		t.Errorf("plan body = %+v", body)
	}
	got, err := s.GetLatestPlan("2026-06-22")
	if err != nil {
		t.Fatalf("plan not stored: %v", err)
	}
	if got.Model != "claude-opus-4-8" || got.ContextPackJSON == nil {
		t.Errorf("stored plan = %+v, want model + context pack", got)
	}
}

func TestPlanGenerateMissingWeek(t *testing.T) {
	h, _ := newTestServer(t)
	rec := doBody(t, h, http.MethodPost, "/api/plan/generate", testToken, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (missing week_start)", rec.Code)
	}
}

func TestPlanGenerateNoCrossFitWeek(t *testing.T) {
	// Cold start: a valid week_start but no stored CrossFit week and no edited
	// week in the body -> 404 (the user must parse a photo first).
	h, _ := newTestServer(t)
	rec := doBody(t, h, http.MethodPost, "/api/plan/generate", testToken, `{"week_start":"2026-06-29"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (no crossfit week stored), body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no crossfit week for that week") {
		t.Errorf("body = %q, want 'no crossfit week for that week'", rec.Body.String())
	}
}

func TestPlanGet(t *testing.T) {
	h, s := newTestServer(t)
	ctx := `{"c":"p"}`
	_, _ = s.InsertPlan(store.Plan{
		WeekStart: "2026-06-22", GeneratedAt: "2026-06-20T08:00:00Z", Status: "generated",
		PlanJSON: `{"fitness_summary":"f","weekly_target_km":20,"days":[{"date":"2026-06-22","dow":"Mon","run_type":"rest","distance_km":0,"pace_target":"","time_note":"","optional_if_cns":false,"rationale":"r"}],"week_rationale":"wr","one_flag":"of"}`,
		FitnessSummary: "f", ContextPackJSON: &ctx, Model: "claude-opus-4-8",
	})
	rec := do(t, h, http.MethodGet, "/api/plan?week=2026-06-22", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body planResponseDTO
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.WeekStart != "2026-06-22" || body.OneFlag != "of" || body.WeeklyTargetKm != 20 {
		t.Errorf("plan get = %+v", body)
	}
}

func TestPlanGetMissing(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/plan?week=2026-06-29", testToken)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no plan for week") {
		t.Errorf("body = %q, want 'no plan for week'", rec.Body.String())
	}
}

func TestFitnessHandler(t *testing.T) {
	h, _ := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/fitness", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body metrics.FitnessMetrics
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.WeeklyVolumeKm != 18.2 || body.RecoveryTrend != "improving" {
		t.Errorf("fitness = %+v", body)
	}
}
```

(Note: `do`, `newTestServer`, `testToken` are the existing M0 test helpers in `handlers_test.go` — reuse them; `doBody` is added here for request-body cases.)

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run "TestProfile|TestCrossfit|TestPlan|TestFitness"
```
Expected: FAIL — routes 404 (handlers/registration not present); e.g. `status = 404, want 200`.

- [ ] **Step 3a: Profile handlers.** Write `backend/internal/api/profile_handlers.go`:
```go
package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"help-my-run/backend/internal/store"
)

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
	if err := h.d.Store.UpsertAthleteProfile(store.AthleteProfile{
		TargetWeeklyKm:     in.TargetWeeklyKm,
		ProgressionMode:    in.ProgressionMode,
		Zone2CeilingBpm:    in.Zone2CeilingBpm,
		ThresholdBpm:       in.ThresholdBpm,
		MaxHRBpm:           in.MaxHRBpm,
		RunConstraintsJSON: in.RunConstraintsJSON,
		GoalText:           in.GoalText,
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
		UpdatedAt:          p.UpdatedAt,
	}
}
```
(Use the existing M0 handler-method receiver and `writeJSON` helper — confirm the receiver type name in `handlers.go`; the drafts assume `*handlers` with field `d Deps`. Match whatever M0 uses.)

- [ ] **Step 3b: Plan/crossfit/fitness handlers.** Write `backend/internal/api/plan_handlers.go`:
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

func (h *handlers) crossfitParse(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad multipart form"})
		return
	}
	weekStart := r.FormValue("week_start")
	if weekStart == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "week_start required"})
		return
	}
	file, hdr, err := r.FormFile("image")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image file required"})
		return
	}
	defer file.Close()

	imagePath, err := saveUploadedImage(h.d.ImageDir, weekStart, file, hdr)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save image: " + err.Error()})
		return
	}

	week, raw, err := h.d.Coach.ParseCrossFit(r.Context(), weekStart, imagePath)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	parsedJSON, _ := json.Marshal(week)
	if err := h.d.Store.UpsertCrossFitWeek(store.CrossFitWeek{
		WeekStart:   weekStart,
		ImagePath:   &imagePath,
		ParsedJSON:  string(parsedJSON),
		RawResponse: &raw,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, week)
}

type generateRequest struct {
	WeekStart    string                  `json:"week_start"`
	CrossFitWeek *llm.CrossFitWeekParsed `json:"crossfit_week"`
}

func (h *handlers) planGenerate(w http.ResponseWriter, r *http.Request) {
	var req generateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		return
	}
	if req.WeekStart == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "week_start required"})
		return
	}
	// If no edited week supplied, a stored week must exist.
	if req.CrossFitWeek == nil {
		if _, err := h.d.Store.GetCrossFitWeek(req.WeekStart); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "no crossfit week for that week"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	plan, ctxPack, model, err := h.d.Coach.GeneratePlan(r.Context(), req.WeekStart, req.CrossFitWeek)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}

	planJSON, _ := json.Marshal(plan)
	generatedAt := time.Now().UTC().Format(time.RFC3339)
	id, err := h.d.Store.InsertPlan(store.Plan{
		WeekStart:       req.WeekStart,
		GeneratedAt:     generatedAt,
		Status:          "generated",
		PlanJSON:        string(planJSON),
		FitnessSummary:  plan.FitnessSummary,
		ContextPackJSON: &ctxPack,
		Model:           model,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toPlanResponse(id, req.WeekStart, generatedAt, plan))
}

func (h *handlers) plan(w http.ResponseWriter, r *http.Request) {
	week := r.URL.Query().Get("week")
	if week == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "week required"})
		return
	}
	p, err := h.d.Store.GetLatestPlan(week)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no plan for week"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var parsed llm.PlanParsed
	if err := json.Unmarshal([]byte(p.PlanJSON), &parsed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stored plan corrupt: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, toPlanResponse(p.ID, p.WeekStart, p.GeneratedAt, parsed))
}

func (h *handlers) fitness(w http.ResponseWriter, r *http.Request) {
	m, err := h.d.Coach.Fitness(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func toPlanResponse(id int64, weekStart, generatedAt string, p llm.PlanParsed) planResponseDTO {
	days := make([]planDayDTO, 0, len(p.Days))
	for _, d := range p.Days {
		days = append(days, planDayDTO{
			Date: d.Date, Dow: d.Dow, RunType: d.RunType, DistanceKm: d.DistanceKm,
			PaceTarget: d.PaceTarget, TimeNote: d.TimeNote,
			OptionalIfCNS: d.OptionalIfCNS, Rationale: d.Rationale,
		})
	}
	return planResponseDTO{
		ID: id, WeekStart: weekStart, GeneratedAt: generatedAt,
		FitnessSummary: p.FitnessSummary, WeeklyTargetKm: p.WeeklyTargetKm,
		Days: days, WeekRationale: p.WeekRationale, OneFlag: p.OneFlag,
	}
}
```

- [ ] **Step 3c: Register routes.** In `backend/internal/api/router.go`, add the 6 routes inside the protected `r.Group` (after the existing `r.Get("/api/recovery", h.recovery)`):
```go
		// M1
		r.Get("/api/profile", h.profile)
		r.Put("/api/profile", h.updateProfile)
		r.Post("/api/crossfit/parse", h.crossfitParse)
		r.Post("/api/plan/generate", h.planGenerate)
		r.Get("/api/plan", h.plan)
		r.Get("/api/fitness", h.fitness)
```

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/
```
Expected: `ok  	help-my-run/backend/internal/api` (all M1 handler tests + existing M0 tests pass).

- [ ] **Step 5: Commit.**
```
git add backend/internal/api/profile_handlers.go backend/internal/api/plan_handlers.go backend/internal/api/router.go backend/internal/api/handlers_test.go backend/internal/api/m1_handlers_test.go && git commit -m "feat(api): profile/crossfit/plan/fitness handlers + routes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---
### Task 23: Wiring — main.go (llm + coach + image dir) and M0 follow-up #2 (sync on boot)

**Files:**
- Modify: `backend/cmd/server/main.go` (`Wire` constructs `llm.Client` + `coach`, injects into `Deps`; `main()` runs sync once before `RunTicker`)
- Modify: `backend/cmd/server/main_test.go` (assert coach wiring serves; sync-on-boot helper test)

`RunTicker` does not fire immediately; the ticker goroutine in `main()` only fires after `syncInterval` (6h). Fix: invoke the sync fn once on boot.

- [ ] **Step 1: Write the failing test.** Append to `backend/cmd/server/main_test.go`:
```go
func TestWireInjectsCoach(t *testing.T) {
	cfg := &config.Config{
		StravaClientID:     "12345",
		StravaClientSecret: "secret",
		StravaRedirectURL:  "http://localhost:8080/api/strava/callback",
		APIToken:           "tok",
		DBPath:             filepath.Join(t.TempDir(), "coach-wire.db"),
		Port:               "8080",
		PythonBin:          "/bin/cat",
		WorkerScript:       "/dev/null",
		ClaudeBin:          "claude",
		ClaudeModel:        "claude-opus-4-8",
		ImageDir:           filepath.Join(t.TempDir(), "cfimg"),
	}
	app, err := Wire(cfg)
	if err != nil {
		t.Fatalf("Wire error = %v", err)
	}
	t.Cleanup(func() { _ = app.Store.Close() })

	// /api/fitness is bearer-protected and served by the injected coach -> 200
	// (computes from an empty store, no claude needed).
	req := httptest.NewRequest(http.MethodGet, "/api/fitness", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/fitness = %d, want 200 (coach wired)", rec.Code)
	}
}

func TestRunSyncOnBoot(t *testing.T) {
	called := make(chan struct{}, 1)
	fn := func(ctx context.Context) {
		select {
		case called <- struct{}{}:
		default:
		}
	}
	runSyncOnBoot(context.Background(), fn)
	select {
	case <-called:
		// ok: sync ran exactly once on boot.
	default:
		t.Fatal("runSyncOnBoot did not invoke the sync fn")
	}
}
```
Add `"context"` to the test imports if not present. (Match the exact `config.Config` field names and the `Wire`/`App` struct shape that M0 uses — the literal above assumes M0's existing fields; adjust to the real names.)

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./cmd/server/ -run "TestWireInjectsCoach|TestRunSyncOnBoot"
```
Expected: FAIL — `undefined: runSyncOnBoot`; `TestWireInjectsCoach` fails `/api/fitness = 500` or panics on nil `Coach`.

- [ ] **Step 3: Minimal implementation.** Edit `backend/cmd/server/main.go`.

Add imports:
```go
	"help-my-run/backend/internal/coach"
	"help-my-run/backend/internal/llm"
```

In `Wire`, construct the llm client + coach, and add them to `Deps`:
```go
	llmClient := &llm.Client{
		Runner:  llm.ExecRunner{Bin: cfg.ClaudeBin},
		Model:   cfg.ClaudeModel,
		Timeout: 120 * time.Second,
	}
	coachEngine := coach.New(s, llmClient, cfg.ClaudeModel, cfg.ImageDir)
```
Then inject into the `api.Deps{...}` literal (keep the existing M0 fields):
```go
		Coach:    coachEngine,
		ImageDir: cfg.ImageDir,
```

Add the `runSyncOnBoot` helper:
```go
// runSyncOnBoot invokes the sync fn once immediately so a fresh instance pulls
// data without waiting a full ticker interval (M0 follow-up #2). It runs in a
// goroutine so server startup is not blocked.
func runSyncOnBoot(ctx context.Context, fn func(context.Context)) {
	go fn(ctx)
}
```

In `main()`, refactor the **inline** M0 ticker closure into a single named `syncOnce` var and hand the same value to both `runSyncOnBoot` (fires once at startup) and `RunTicker` (fires on the interval).

**Before (M0 — closure is inline in the `RunTicker` call):**
```go
	// Periodic sync ticker (the agentic schedule is M2; this is plain periodic).
	stravaClient := app.Strava
	runner := app.Runner
	extraEnv := garminEnv(cfg)
	go syncpkg.RunTicker(ctx, syncInterval, func(c context.Context) {
		res := syncpkg.SyncAll(c, app.Store, stravaClient, runner, extraEnv)
		log.Printf("periodic sync: strava=%s/%d garmin=%s/%d",
			res.Strava.Status, res.Strava.Synced, res.Garmin.Status, res.Garmin.Synced)
	})
```

**After (extract the closure to `syncOnce`, pass it to BOTH):**
```go
	// Periodic sync ticker (the agentic schedule is M2; this is plain periodic).
	stravaClient := app.Strava
	runner := app.Runner
	extraEnv := garminEnv(cfg)
	syncOnce := func(c context.Context) {
		res := syncpkg.SyncAll(c, app.Store, stravaClient, runner, extraEnv)
		log.Printf("sync: strava=%s/%d garmin=%s/%d",
			res.Strava.Status, res.Strava.Synced, res.Garmin.Status, res.Garmin.Synced)
	}
	// M0 follow-up #2: run once on boot, then on the interval.
	runSyncOnBoot(ctx, syncOnce)
	go syncpkg.RunTicker(ctx, syncInterval, syncOnce)
```
Do not change `syncInterval`. `syncOnce` is defined exactly once and reused; both call sites share the M0 `stravaClient`/`runner`/`extraEnv` already built in `main`.

> Note: `TestWireInjectsCoach` hits `/api/fitness`, whose handler reads the seeded `athlete_profile` row. The M1 migration (Task 2) seeds that single default row, so `ComputeFitness` over an otherwise-empty store returns `200` without invoking `claude` — the wiring test passes deterministically.

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./cmd/server/ && go build ./...
```
Expected: `ok  	help-my-run/backend/cmd/server`; build clean.

- [ ] **Step 5: Commit.**
```
git add backend/cmd/server/main.go backend/cmd/server/main_test.go && git commit -m "feat(server): wire llm+coach+image dir; run sync once on boot

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 24: M0 follow-up #1 — validate Strava OAuth state (CSRF)

**Files:**
- Modify: `backend/internal/api/handlers.go` (`stravaConnect` persists state; `stravaCallback` validates it)
- Modify: `backend/internal/api/handlers_test.go` (save state in the happy-path callback test)
- Modify: `backend/internal/api/m1_handlers_test.go` (add connect-persists + callback-rejects-bad-state tests)

Currently `randomState()` is generated in `stravaConnect` but never persisted, and `stravaCallback` ignores the `state` query param. Uses `Store.SaveOAuthState`/`ConsumeOAuthState` from Task 7.

- [ ] **Step 1: Write the failing test.** Append to `backend/internal/api/m1_handlers_test.go`:
```go
func TestStravaConnectPersistsState(t *testing.T) {
	h, s := newTestServer(t)
	rec := do(t, h, http.MethodGet, "/api/strava/connect", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body connectResp
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	u := body.AuthorizeURL
	idx := strings.Index(u, "state=")
	if idx < 0 {
		t.Fatalf("authorize URL has no state: %s", u)
	}
	state := u[idx+len("state="):]
	if amp := strings.IndexByte(state, '&'); amp >= 0 {
		state = state[:amp]
	}
	if state == "" {
		t.Fatal("empty state")
	}
	if err := s.ConsumeOAuthState(state); err != nil {
		t.Errorf("state %q not persisted by connect: %v", state, err)
	}
}

func TestStravaCallbackRejectsBadState(t *testing.T) {
	h, s := newTestServer(t)
	// No state was saved -> callback with an unknown state must NOT persist tokens.
	rec := do(t, h, http.MethodGet, "/api/strava/callback?code=the-code&state=forged", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (HTML page)", rec.Code)
	}
	if !strings.Contains(strings.ToLower(rec.Body.String()), "failed") {
		t.Errorf("body = %q, want failure text on bad state", rec.Body.String())
	}
	if _, err := s.GetStravaTokens(); err != store.ErrNotFound {
		t.Errorf("tokens persisted on forged state: %v", err)
	}
}
```
(Use the exact M0 `connectResp` DTO name and `GetStravaTokens` getter — adjust to whatever M0 calls them.)

Also update the existing happy-path callback test `TestStravaCallbackExchangesAndPersists` in `handlers_test.go` (~line 211, the one that exchanges + persists tokens). Once `stravaCallback` requires a valid state via `ConsumeOAuthState`, this test fails because it passes `&state=xyz` but never saved it. Save the state **after `s.Migrate()` and before the callback request is issued**. Concretely, in that test add the `SaveOAuthState("xyz")` call so the relevant region reads:
```go
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Persist the CSRF state the callback URL below carries (callback now validates it).
	if err := s.SaveOAuthState("xyz"); err != nil {
		t.Fatalf("SaveOAuthState: %v", err)
	}
	deps := Deps{
		Store:    s,
		Strava:   strava.NewWithBase("12345", "secret", "http://localhost:8080/api/strava/callback", srv.URL),
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) { return "ok", 0, nil, "ok", 0, nil },
	}
	h := NewRouter(deps)

	// Callback has NO auth header. The URL must carry &state=xyz (saved above).
	rec := do(t, h, http.MethodGet, "/api/strava/callback?code=the-code&scope=read,activity:read_all&state=xyz", "")
```
(`SaveOAuthState` takes a single `state string` — matching the store method signature in Task 7. Saving before `NewRouter`/the request is fine: `ConsumeOAuthState` reads it during the callback.)

> **`TestStravaCallbackAccessDenied` needs NO saved state.** It requests `/api/strava/callback?error=access_denied`, which returns via the **early `if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == ""` guard** (the new state check sits *after* that guard), so it never reaches `ConsumeOAuthState`. Leave that test unchanged.

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run "TestStravaConnectPersistsState|TestStravaCallbackRejectsBadState"
```
Expected: FAIL — `state ... not persisted by connect: store: not found`; and tokens ARE persisted on forged state (current code ignores state).

- [ ] **Step 3: Minimal implementation.** Edit `backend/internal/api/handlers.go`.

Make `stravaConnect` persist the state:
```go
func (h *handlers) stravaConnect(w http.ResponseWriter, r *http.Request) {
	state := randomState()
	if err := h.d.Store.SaveOAuthState(state); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	url := h.d.Strava.AuthorizeURL(state)
	writeJSON(w, http.StatusOK, connectResp{AuthorizeURL: url})
}
```
(Match the exact M0 function body / DTO field names — only add the `SaveOAuthState` call before building the URL.)

In `stravaCallback`, validate state **immediately after the existing `if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == ""` guard (`return`s on access-denied/missing-code) and before the `code := r.URL.Query().Get("code")` / `h.d.Strava.Exchange(...)` call.** Add:
```go
	state := r.URL.Query().Get("state")
	if state == "" || h.d.Store.ConsumeOAuthState(state) != nil {
		writeHTML(w, "Strava connection failed (invalid state). You can close this tab.")
		return
	}
```
So the top of `stravaCallback` reads:
```go
func (h *handlers) stravaCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		writeHTML(w, "Strava connection failed. You can close this tab.")
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" || h.d.Store.ConsumeOAuthState(state) != nil {
		writeHTML(w, "Strava connection failed (invalid state). You can close this tab.")
		return
	}
	code := r.URL.Query().Get("code")
	tok, err := h.d.Strava.Exchange(r.Context(), code)
	// ... unchanged ...
```
(`writeHTML` is M0's existing HTML-writer helper. The state check sits strictly between the error/missing-`code` guard and the token `Exchange` call — so `TestStravaCallbackAccessDenied`, which trips the first guard, never reaches it.)

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/
```
Expected: `ok  	help-my-run/backend/internal/api` (state tests pass; happy-path callback still persists because it now saves `xyz` first).

- [ ] **Step 5: Commit.**
```
git add backend/internal/api/handlers.go backend/internal/api/handlers_test.go backend/internal/api/m1_handlers_test.go && git commit -m "fix(api): validate Strava OAuth state (CSRF) in callback

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 25: M0 follow-up #3 — Strava cursor from latest stored activity start_time

**Files:**
- Modify: `backend/internal/sync/sync.go` (`SyncStrava` cursor logic)
- Test: `backend/internal/sync/sync_test.go` (add cursor test)

Currently the cursor is derived from `sync_log.last_synced_at`. Switch to `store.LatestActivityStartTime()` (added in Task 6), falling back to the 30-day backfill when there are no activities.

- [ ] **Step 1: Write the failing test.** First inspect existing sync test helpers to reuse the fake Strava server pattern:
```
cd /home/jake/project/help-my-run/backend && sed -n '1,60p' internal/sync/sync_test.go
```
Append a test to `backend/internal/sync/sync_test.go` that seeds an activity and asserts the `after` query param sent to the fake Strava `ListActivities` endpoint equals that activity's `start_time` unix:
```go
func TestSyncStravaCursorFromLatestActivity(t *testing.T) {
	var gotAfter string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth/token"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token_type":"Bearer","access_token":"acc","refresh_token":"ref","expires_at":4102444800,"expires_in":21600,"scope":"activity:read_all"}`))
		case strings.Contains(r.URL.Path, "/athlete/activities"):
			gotAfter = r.URL.Query().Get("after")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s, err := store.Open(filepath.Join(t.TempDir(), "cur.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_ = s.SaveStravaTokens(store.StravaTokens{AccessToken: "acc", RefreshToken: "ref", ExpiresAt: 4102444800})
	_ = s.UpsertActivity(store.Activity{
		StravaID: 1, Type: "Run", Name: "r", StartTime: "2026-06-18T18:00:00Z",
		DistanceM: 8000, MovingTimeS: 2400, ElapsedTimeS: 2400, RawJSON: "{}",
	})

	client := strava.NewWithBase("1", "x", "http://cb", srv.URL)
	res := SyncStrava(context.Background(), s, client)
	if res.Status != "ok" {
		t.Fatalf("sync status = %q (err=%v), want ok", res.Status, res.Error)
	}

	wantUnix := mustUnix(t, "2026-06-18T18:00:00Z")
	if gotAfter != wantUnix {
		t.Errorf("after = %q, want %q (latest stored activity start_time)", gotAfter, wantUnix)
	}
}

func mustUnix(t *testing.T, rfc string) string {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, rfc)
	if err != nil {
		t.Fatalf("parse %q: %v", rfc, err)
	}
	return strconv.FormatInt(ts.Unix(), 10)
}
```
Ensure the test file imports `net/http`, `net/http/httptest`, `path/filepath`, `strconv`, `strings`, `time`, `context`, plus `store` and `strava` (add any missing). Match the exact M0 names for `SyncStrava`/its result struct/`SaveStravaTokens`/`strava.NewWithBase`.

- [ ] **Step 2: Run the test, expect FAIL.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/sync/ -run TestSyncStravaCursorFromLatestActivity
```
Expected: FAIL — `after = "<30-day-backfill-unix>", want "<2026-06-18-unix>"` (cursor still from `sync_log`, which is empty so it backfills 30 days).

- [ ] **Step 3: Minimal implementation.** Edit `backend/internal/sync/sync.go`. Replace the cursor block (currently derived from `sync_log.last_synced_at`):
```go
	// Incremental window: since last successful sync, else ~30-day backfill.
	after := time.Now().AddDate(0, 0, -30).Unix()
	if sl, err := s.GetSyncLog(source); err == nil && sl.LastSyncedAt != nil {
		if ts, perr := time.Parse(time.RFC3339, *sl.LastSyncedAt); perr == nil {
			after = ts.Unix()
		}
	}
```
with:
```go
	// Incremental window: since the latest stored activity start_time, else a
	// ~30-day backfill on a fresh DB.
	after := time.Now().AddDate(0, 0, -30).Unix()
	if latest, err := s.LatestActivityStartTime(); err == nil {
		if ts, perr := time.Parse(time.RFC3339, latest); perr == nil {
			after = ts.Unix()
		}
	}
```
(Match the exact M0 cursor block text; the goal is to source the cursor from `LatestActivityStartTime()`.)

- [ ] **Step 4: Run the test, expect PASS.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/sync/
```
Expected: `ok  	help-my-run/backend/internal/sync` (new cursor test passes; existing sync tests still pass).

- [ ] **Step 5: Commit.**
```
git add backend/internal/sync/sync.go backend/internal/sync/sync_test.go && git commit -m "fix(sync): Strava cursor from latest stored activity start_time

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 26: Docs / env / gitignore (config surface for M1)

**Files:**
- Modify: `.env.example` (add the Claude/image block)
- Modify: `.gitignore` (add `data/`)
- Modify: `README.md` (document `claude auth login` + new env vars next to `make garmin-login`)

- [ ] **Step 1: Verify the current state (the "failing" check).**
```
grep -n "CLAUDE_BIN\|IMAGE_DIR\|claude auth login" .env.example README.md ; grep -n "^data/" .gitignore
```
Expected: no matches (the M1 entries are absent).

- [ ] **Step 2: Add to `.env.example`** (after the existing `ANTHROPIC_API_KEY` stub line):
```bash
# --- Claude Code headless (M1) ---
# Path to the `claude` CLI. Host must be logged in once: `claude auth login` (NOT --console).
# Headless/remote host with no browser? Run `claude setup-token` once on a machine with
# a browser and expose that token to this host's env; ANTHROPIC_API_KEY is a paid fallback.
CLAUDE_BIN=claude
# Model used for vision + planning.
CLAUDE_MODEL=claude-opus-4-8
# Directory where uploaded CrossFit schedule images are saved (read by `claude -p`).
IMAGE_DIR=./data/crossfit
```

- [ ] **Step 3: Add to `.gitignore`:**
```
# M1: uploaded CrossFit schedule images
data/
```

- [ ] **Step 4: Document in `README.md`** (an M1 setup subsection near `make garmin-login`):
```markdown
### Claude Code (M1 plan generation)

M1 generates plans via the `claude` CLI under your Claude subscription (no API key needed):

1. Install the Claude Code CLI on the host.
2. Log in once (interactive, opens a browser):

   ```bash
   claude auth login
   ```

   Do NOT use `claude auth login --console` (that switches to the metered API key path).

   **Headless / remote host?** `claude -p` reads `~/.claude/.credentials.json`, so it only
   works where `claude auth login` has been run interactively (e.g. your dev laptop). On a
   browser-less VPS/CI host, run `claude setup-token` once on a machine with a browser to mint
   a long-lived subscription token, then expose that token in this host's environment so
   `claude -p` authenticates non-interactively. `ANTHROPIC_API_KEY` remains a paid fallback.
3. Set the M1 env vars in `.env` (defaults shown):

   ```bash
   CLAUDE_BIN=claude
   CLAUDE_MODEL=claude-opus-4-8
   IMAGE_DIR=./data/crossfit
   ```

`IMAGE_DIR` must be writable by the backend (uploaded box-schedule photos are saved there and read by `claude -p`).
```

- [ ] **Step 5: Verify + commit.**
```
cd /home/jake/project/help-my-run && grep -n "CLAUDE_BIN\|IMAGE_DIR\|claude auth login" .env.example README.md && grep -n "^data/" .gitignore && (cd backend && go build ./...) && git add .env.example .gitignore README.md && git commit -m "docs(m1): document claude auth login, CLAUDE_BIN/CLAUDE_MODEL/IMAGE_DIR; gitignore data/

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```
Expected: all three greps match; build clean.

---

### Task 27: Full backend suite green (integration gate)

**Files:** (no new files) — runs the whole backend module to confirm M1 + M0 follow-ups integrate.

- [ ] **Step 1: Run the full backend test suite.**
```
cd /home/jake/project/help-my-run/backend && go test ./... 2>&1
```
Expected PASS: `ok` for every package — `internal/config`, `internal/store`, `internal/llm`, `internal/metrics`, `internal/coach`, `internal/api`, `internal/sync`, `internal/strava`, `internal/garmin`, `cmd/server`.

- [ ] **Step 2: Vet the module.**
```
cd /home/jake/project/help-my-run/backend && go vet ./... 2>&1
```
Expected: no output (clean).

- [ ] **Step 3: Confirm migrations apply in sequence on a fresh DB.**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run "TestM1Migration|TestOpenAndMigrate|OAuthState" -v 2>&1 | tail -20
```
Expected: all listed tests `--- PASS`.

- [ ] **Step 4: Commit (only if `go.mod`/`go.sum` changed by tidy).**
```
cd /home/jake/project/help-my-run/backend && go mod tidy && git -C /home/jake/project/help-my-run diff --quiet backend/go.mod backend/go.sum || (git -C /home/jake/project/help-my-run add backend/go.mod backend/go.sum && git -C /home/jake/project/help-my-run commit -m "chore(backend): go mod tidy after M1

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>")
```
Expected: either no change, or a tidy commit. No new runtime deps are introduced (M1 backend uses only stdlib + existing modules).

---
### Task 28: App — add expo-image-picker dependency + plugin config

**Files:**
- Modify: `app/package.json` (`dependencies` — adds `expo-image-picker`)
- Modify: `app/app.json` (`expo.plugins` — adds `expo-image-picker` plugin entry)

- [ ] **Step 1: Install expo-image-picker via the Expo resolver (exact command).** Use the SDK-aware installer so the version resolves to the SDK-56 line. Do NOT use plain `npm install`.
```bash
cd /home/jake/project/help-my-run/app && npx expo install expo-image-picker
```
Expected: `app/package.json` now contains an `expo-image-picker` entry under `dependencies` (the resolver chooses the version matching the installed Expo SDK — do not assume a specific version line).

- [ ] **Step 2: Verify the dependency landed (presence only — no version pin).**
```bash
cd /home/jake/project/help-my-run/app && node -e "const v=require('./package.json').dependencies['expo-image-picker']; if(!v){console.error('MISSING expo-image-picker dependency');process.exit(1)} console.log('expo-image-picker present:', v)"
```
Expected output: `expo-image-picker present: <version>` (exit 0) — the assertion only checks the dependency is present and non-empty; the exact version is whatever the Expo resolver picked.

- [ ] **Step 3: Add the `expo-image-picker` plugin config to `app.json`.** Edit `app/app.json` and replace the `plugins` array so the plugin (with permission strings) is appended after `"expo-web-browser"`:
```json
    "plugins": [
      "expo-router",
      [
        "expo-splash-screen",
        {
          "backgroundColor": "#208AEF",
          "android": {
            "image": "./assets/images/splash-icon.png",
            "imageWidth": 76
          }
        }
      ],
      "expo-secure-store",
      "expo-web-browser",
      [
        "expo-image-picker",
        {
          "photosPermission": "help-my-run accesses your photos so you can upload your CrossFit schedule.",
          "cameraPermission": "help-my-run uses the camera so you can photograph your CrossFit schedule."
        }
      ]
    ],
```
(Match the existing M0 `plugins` entries verbatim; only append the `expo-image-picker` block.)

- [ ] **Step 4: Validate `app.json` is still valid JSON and contains the plugin.**
```bash
cd /home/jake/project/help-my-run/app && node -e "const p=require('./app.json').expo.plugins; const has=p.some(x=>Array.isArray(x)&&x[0]==='expo-image-picker'); if(!has){console.error('MISSING expo-image-picker plugin');process.exit(1)} console.log('expo-image-picker plugin present')"
```
Expected output: `expo-image-picker plugin present` (exit 0).

- [ ] **Step 5: Commit.**
```bash
git add app/package.json app/package-lock.json app/app.json && git commit -m "build(app): add expo-image-picker dep + plugin config

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```
(Include `app/package-lock.json` only if it changed.)

---

### Task 29: App — API client additions (`apiUpload` + `apiPut`)

**Files:**
- Modify: `app/src/api/client.ts` (add exported `apiUpload<T>` and `apiPut<T>`; reuse `getBaseUrl`/`getToken`/`ApiError`/`request`)
- Test: `app/src/api/__tests__/client.test.ts` (add `describe('apiPut')` and `describe('apiUpload')`)

- [ ] **Step 1: Write the failing tests.** Update the import on line 1 of `app/src/api/__tests__/client.test.ts` to pull in the new functions:
```ts
import { apiGet, apiPost, apiPut, apiUpload, ApiError } from '../client';
```
Append at end of file:
```ts
describe('apiPut', () => {
  it('uses PUT and serializes the body', async () => {
    mockFetchOnce({ ok: true, status: 200, json: { goal_text: 'Build cardio' } });

    const data = await apiPut<{ goal_text: string }>('/api/profile', { goal_text: 'Build cardio' });

    const [url, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(url).toBe('http://localhost:8080/api/profile');
    expect(init.method).toBe('PUT');
    expect(init.body).toBe(JSON.stringify({ goal_text: 'Build cardio' }));
    expect(init.headers['Content-Type']).toBe('application/json');
    expect(data).toEqual({ goal_text: 'Build cardio' });
  });
});

describe('apiUpload', () => {
  it('POSTs multipart form-data without a JSON Content-Type header', async () => {
    mockFetchOnce({ ok: true, status: 200, json: { week_start: '2026-06-22', days: [] } });

    const data = await apiUpload<{ week_start: string }>('/api/crossfit/parse', {
      uri: 'file:///c.jpg',
      name: 'c.jpg',
      type: 'image/jpeg',
    });

    const [url, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(url).toBe('http://localhost:8080/api/crossfit/parse');
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
    // Critical: no JSON Content-Type — RN must set the multipart boundary itself.
    expect(init.headers['Content-Type']).toBeUndefined();
    expect(init.headers.Authorization).toBe('Bearer test-token');
    expect(data).toEqual({ week_start: '2026-06-22', days: [] });
  });

  it('omits Authorization header when no token is stored', async () => {
    mockGetToken.mockResolvedValue(null);
    mockFetchOnce({ ok: true, status: 200, json: {} });

    await apiUpload('/api/crossfit/parse', { uri: 'file:///c.jpg', name: 'c.jpg', type: 'image/jpeg' });

    const [, init] = (global.fetch as jest.Mock).mock.calls[0];
    expect(init.headers.Authorization).toBeUndefined();
  });

  it('throws ApiError on non-ok response', async () => {
    mockFetchOnce({ ok: false, status: 502, json: { error: 'claude failed' } });

    const err = await apiUpload('/api/crossfit/parse', {
      uri: 'file:///c.jpg', name: 'c.jpg', type: 'image/jpeg',
    }).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toMatchObject({ name: 'ApiError', status: 502 });
  });

  it('throws ApiError(0) when base URL is not configured', async () => {
    mockGetBaseUrl.mockResolvedValue(null);

    await expect(
      apiUpload('/api/crossfit/parse', { uri: 'file:///c.jpg', name: 'c.jpg', type: 'image/jpeg' }),
    ).rejects.toMatchObject({ status: 0, message: 'Backend URL not configured' });
    expect(global.fetch).not.toHaveBeenCalled();
  });
});
```
(Reuse the existing M0 test helpers `mockFetchOnce`, `mockGetToken`, `mockGetBaseUrl` already present in this file; match their exact names/`test-token` value to M0.)

- [ ] **Step 2: Run the test — expect FAIL.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/client.test.ts 2>&1 | tail -25
```
Expected: FAIL — `apiPut is not a function` / `apiUpload is not a function`.

- [ ] **Step 3: Implement `apiPut` and `apiUpload` in `app/src/api/client.ts`.** Keep the existing `apiGet`/`apiPost` exports; add:
```ts
export const apiPut = <T>(path: string, body?: unknown) =>
  request<T>(path, { method: 'PUT', body: body ? JSON.stringify(body) : undefined });

export async function apiUpload<T>(
  path: string,
  file: { uri: string; name: string; type: string },
  field = 'image',
): Promise<T> {
  const baseUrl = await getBaseUrl();
  const token = await getToken();
  if (!baseUrl) throw new ApiError(0, 'Backend URL not configured');

  const form = new FormData();
  // RN FormData accepts this object shape; cast to satisfy DOM lib types.
  form.append(field, { uri: file.uri, name: file.name, type: file.type } as unknown as Blob);

  const res = await fetch(`${baseUrl}${path}`, {
    method: 'POST',
    headers: {
      // No Content-Type: RN sets multipart/form-data + boundary automatically.
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: form,
  });

  if (!res.ok) {
    throw new ApiError(res.status, `POST ${path} failed: ${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
```
(Reuse M0's existing `getBaseUrl`/`getToken`/`ApiError`/`request`; match their exact signatures.)

- [ ] **Step 4: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/client.test.ts 2>&1 | tail -15
```
Expected: PASS — all `apiGet`/`apiPost`/`apiPut`/`apiUpload` describe blocks green.

- [ ] **Step 5: Commit.**
```bash
cd /home/jake/project/help-my-run && git add app/src/api/client.ts app/src/api/__tests__/client.test.ts && git commit -m "feat(app): add apiPut + multipart apiUpload to api client

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 30: App — API types (`Load`, `CrossFitDay`, `CrossFitWeek`, `PlanDay`, `Plan`, `AthleteProfile`, `Fitness`)

**Files:**
- Modify: `app/src/api/types.ts` (append M1 types; snake_case fields)
- Test: `app/src/api/__tests__/types.test.ts` (CREATE — compile-time shape assertions)

- [ ] **Step 1: Write the failing test.** Create `app/src/api/__tests__/types.test.ts`:
```ts
import type {
  Load,
  CrossFitDay,
  CrossFitWeek,
  PlanDay,
  Plan,
  AthleteProfile,
  Fitness,
} from '../types';

describe('M1 API types', () => {
  it('CrossFitWeek matches the Stage-1 shape (snake_case)', () => {
    const day: CrossFitDay = {
      date: '2026-06-22',
      dow: 'Mon',
      has_crossfit: true,
      focus: 'Back squat 5x5 + short metcon',
      cns_load: 'high',
      leg_load: 'high',
      notes: 'Heavy legs',
    };
    const load: Load = 'med';
    const week: CrossFitWeek = { week_start: '2026-06-22', days: [day] };
    expect(week.days[0].cns_load).toBe('high');
    expect(load).toBe('med');
    expect(week.week_start).toBe('2026-06-22');
  });

  it('Plan matches the Stage-2 shape (snake_case)', () => {
    const planDay: PlanDay = {
      date: '2026-06-23',
      dow: 'Tue',
      run_type: 'easy',
      distance_km: 5,
      pace_target: '6:00/km',
      time_note: '~20:00 after CrossFit',
      optional_if_cns: false,
      rationale: 'Low leg load; easy aerobic double.',
    };
    const plan: Plan = {
      id: 7,
      week_start: '2026-06-22',
      generated_at: '2026-06-20T08:05:12Z',
      fitness_summary: '~18 km/week, acute:chronic 1.05.',
      weekly_target_km: 20,
      days: [planDay],
      week_rationale: 'Quality placed Thursday.',
      one_flag: 'If Thursday skill work runs heavy, downgrade the tempo to easy.',
    };
    expect(plan.days[0].run_type).toBe('easy');
    expect(plan.weekly_target_km).toBe(20);
  });

  it('AthleteProfile and Fitness expose snake_case fields', () => {
    const profile: AthleteProfile = {
      target_weekly_km: 20,
      progression_mode: 'build',
      zone2_ceiling_bpm: null,
      threshold_bpm: null,
      max_hr_bpm: null,
      run_constraints_json: '{}',
      goal_text: 'Build cardio over time',
      updated_at: '2026-06-20T08:00:00Z',
    };
    const fitness: Fitness = {
      weekly_volume_km: 18.2,
      four_week_avg_km: 17.4,
      acute_chronic_ratio: 1.05,
      easy_pace: '6:00/km',
      threshold_pace: '5:05/km',
      recovery_trend: 'improving',
      safe_weekly_target_km: 20.0,
      is_cutback_week: false,
    };
    expect(profile.progression_mode).toBe('build');
    expect(fitness.recovery_trend).toBe('improving');
    expect(fitness.is_cutback_week).toBe(false);
  });
});
```

- [ ] **Step 2: Run the test — expect FAIL.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/types.test.ts 2>&1 | tail -25
```
Expected: FAIL — `Module '"../types"' has no exported member 'Load'` (and the rest).

- [ ] **Step 3: Append the M1 types to `app/src/api/types.ts`** (after the existing M0 interfaces) — exactly the canonical TS types from Shared Contracts (`Load`, `CrossFitDay`, `CrossFitWeek`, `PlanDay`, `Plan`, `AthleteProfile`, `Fitness`).

- [ ] **Step 4: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/types.test.ts 2>&1 | tail -12
```
Expected: PASS — `Tests: 3 passed`.

- [ ] **Step 5: Commit.**
```bash
cd /home/jake/project/help-my-run && git add app/src/api/types.ts app/src/api/__tests__/types.test.ts && git commit -m "feat(app): add M1 API types (CrossFit week, plan, profile, fitness)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---
### Task 31: App — React Query hooks (`useProfile`, `useUpdateProfile`, `useFitness`, `usePlan`, `useParseCrossfit`, `useGeneratePlan`)

**Files:**
- Modify: `app/src/api/hooks.ts` (add 6 hooks; import `apiPut`/`apiUpload` and the new types)
- Test: `app/src/api/__tests__/hooks.test.tsx` (extend client mock with `apiPut`/`apiUpload`; add 6 describe blocks)

- [ ] **Step 1: Write the failing tests.** In `app/src/api/__tests__/hooks.test.tsx`, extend the client mock + imports to include the new client funcs and hooks:
```ts
jest.mock('../client', () => ({
  apiGet: jest.fn(),
  apiPost: jest.fn(),
  apiPut: jest.fn(),
  apiUpload: jest.fn(),
}));

import { apiGet, apiPost, apiPut, apiUpload } from '../client';
import {
  useStatus,
  useActivities,
  useRecovery,
  useSync,
  useProfile,
  useUpdateProfile,
  useFitness,
  usePlan,
  useParseCrossfit,
  useGeneratePlan,
} from '../hooks';
import type {
  Status,
  ActivitiesResponse,
  RecoveryResponse,
  SyncResponse,
  AthleteProfile,
  Fitness,
  CrossFitWeek,
  Plan,
} from '../types';

const mockApiPut = apiPut as jest.MockedFunction<typeof apiPut>;
const mockApiUpload = apiUpload as jest.MockedFunction<typeof apiUpload>;
```
(Keep the existing M0 hook imports/mocks — `mockApiGet`/`mockApiPost` etc.; only add the new ones.)

Append these describe blocks at the END of the file:
```ts
describe('useProfile', () => {
  it('fetches /api/profile', async () => {
    const data: AthleteProfile = {
      target_weekly_km: 20, progression_mode: 'build',
      zone2_ceiling_bpm: null, threshold_bpm: null, max_hr_bpm: null,
      run_constraints_json: '{}', goal_text: 'Build cardio', updated_at: '2026-06-20T08:00:00Z',
    };
    mockApiGet.mockResolvedValue(data);
    const { result } = await renderHook(() => useProfile(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/profile');
    expect(result.current.data).toEqual(data);
  });
});

describe('useUpdateProfile', () => {
  it('PUTs /api/profile with the profile body', async () => {
    const profile: AthleteProfile = {
      target_weekly_km: 25, progression_mode: 'hold',
      zone2_ceiling_bpm: 150, threshold_bpm: 168, max_hr_bpm: 190,
      run_constraints_json: '{}', goal_text: 'Hold steady',
    };
    mockApiPut.mockResolvedValue(profile);
    const { result } = await renderHook(() => useUpdateProfile(), { wrapper: createWrapper() });
    act(() => { result.current.mutate(profile); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPut).toHaveBeenCalledWith('/api/profile', profile);
    expect(result.current.data).toEqual(profile);
  });
});

describe('useFitness', () => {
  it('fetches /api/fitness', async () => {
    const data: Fitness = {
      weekly_volume_km: 18.2, four_week_avg_km: 17.4, acute_chronic_ratio: 1.05,
      easy_pace: '6:00/km', threshold_pace: '5:05/km', recovery_trend: 'improving',
      safe_weekly_target_km: 20, is_cutback_week: false,
    };
    mockApiGet.mockResolvedValue(data);
    const { result } = await renderHook(() => useFitness(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/fitness');
    expect(result.current.data).toEqual(data);
  });
});

describe('usePlan', () => {
  it('fetches /api/plan with the week query param', async () => {
    const data: Plan = {
      week_start: '2026-06-22', fitness_summary: 's', weekly_target_km: 20,
      days: [], week_rationale: 'r', one_flag: 'f',
    };
    mockApiGet.mockResolvedValue(data);
    const { result } = await renderHook(() => usePlan('2026-06-22'), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/plan?week=2026-06-22');
    expect(result.current.data).toEqual(data);
  });

  it('is disabled (does not fetch) when week is empty', async () => {
    const { result } = await renderHook(() => usePlan(''), { wrapper: createWrapper() });
    expect(result.current.fetchStatus).toBe('idle');
    expect(mockApiGet).not.toHaveBeenCalled();
  });
});

describe('useParseCrossfit', () => {
  it('uploads the image and returns the parsed week', async () => {
    const week: CrossFitWeek = { week_start: '2026-06-22', days: [] };
    mockApiUpload.mockResolvedValue(week);
    const { result } = await renderHook(() => useParseCrossfit(), { wrapper: createWrapper() });
    act(() => {
      result.current.mutate({ uri: 'file:///x.jpg', name: 'x.jpg', type: 'image/jpeg' });
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiUpload).toHaveBeenCalledWith('/api/crossfit/parse', {
      uri: 'file:///x.jpg', name: 'x.jpg', type: 'image/jpeg',
    });
    expect(result.current.data).toEqual(week);
  });
});

describe('useGeneratePlan', () => {
  it('POSTs /api/plan/generate with week_start + crossfit_week', async () => {
    const week: CrossFitWeek = { week_start: '2026-06-22', days: [] };
    const plan: Plan = {
      id: 7, week_start: '2026-06-22', generated_at: '2026-06-20T08:05:12Z',
      fitness_summary: 's', weekly_target_km: 20, days: [], week_rationale: 'r', one_flag: 'f',
    };
    mockApiPost.mockResolvedValue(plan);
    const { result } = await renderHook(() => useGeneratePlan(), { wrapper: createWrapper() });
    act(() => {
      result.current.mutate({ week_start: '2026-06-22', crossfit_week: week });
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/plan/generate', {
      week_start: '2026-06-22', crossfit_week: week,
    });
    expect(result.current.data).toEqual(plan);
  });
});
```
(Reuse M0's `renderHook`, `createWrapper`, `waitFor`, `act`, `mockApiGet`, `mockApiPost` already in this test file.)

- [ ] **Step 2: Run the test — expect FAIL.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/hooks.test.tsx 2>&1 | tail -25
```
Expected: FAIL — `Module '"../hooks"' has no exported member 'useProfile'` (and the rest).

- [ ] **Step 3: Add the 6 hooks to `app/src/api/hooks.ts`.** Extend the import block to add `apiPut, apiUpload` and the new types (`AthleteProfile`, `Fitness`, `CrossFitWeek`, `Plan`), keeping existing M0 imports. Append at the END of the file:
```ts
export function useProfile() {
  return useQuery({
    queryKey: ['profile'],
    queryFn: () => apiGet<AthleteProfile>('/api/profile'),
  });
}

export function useUpdateProfile() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (profile: AthleteProfile) => apiPut<AthleteProfile>('/api/profile', profile),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['profile'] });
    },
  });
}

export function useFitness() {
  return useQuery({
    queryKey: ['fitness'],
    queryFn: () => apiGet<Fitness>('/api/fitness'),
  });
}

export function usePlan(week: string) {
  return useQuery({
    queryKey: ['plan', week],
    queryFn: () => apiGet<Plan>(`/api/plan?week=${week}`),
    enabled: !!week,
  });
}

export function useParseCrossfit() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (file: { uri: string; name: string; type: string }) =>
      apiUpload<CrossFitWeek>('/api/crossfit/parse', file),
    onSuccess: (week) => {
      queryClient.invalidateQueries({ queryKey: ['crossfit', week.week_start] });
    },
  });
}

export function useGeneratePlan() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { week_start: string; crossfit_week?: CrossFitWeek }) =>
      apiPost<Plan>('/api/plan/generate', body),
    onSuccess: (plan) => {
      queryClient.invalidateQueries({ queryKey: ['plan', plan.week_start] });
      queryClient.invalidateQueries({ queryKey: ['fitness'] });
    },
  });
}
```
(Use M0's existing `useQuery`/`useMutation`/`useQueryClient` imports — match how M0 hooks import them.)

- [ ] **Step 4: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/hooks.test.tsx 2>&1 | tail -15
```
Expected: PASS — original M0 hooks + 7 new tests green.

- [ ] **Step 5: Commit.**
```bash
git add app/src/api/hooks.ts app/src/api/__tests__/hooks.test.tsx && git commit -m "feat(app): add profile/fitness/plan/parse/generate react-query hooks

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 32: App — image picker wrappers (`pickFromLibrary` / `takePhoto`)

**Files:**
- Create: `app/src/lib/imagePicker.ts` (`pickFromLibrary()`, `takePhoto()`, `toUploadFile()`)
- Test: `app/src/lib/__tests__/imagePicker.test.ts` (CREATE — mock `expo-image-picker`)

- [ ] **Step 1: Write the failing test.** Create `app/src/lib/__tests__/imagePicker.test.ts`:
```ts
jest.mock('expo-image-picker', () => ({
  requestMediaLibraryPermissionsAsync: jest.fn(),
  requestCameraPermissionsAsync: jest.fn(),
  launchImageLibraryAsync: jest.fn(),
  launchCameraAsync: jest.fn(),
}));

import * as ImagePicker from 'expo-image-picker';
import { pickFromLibrary, takePhoto, toUploadFile } from '../imagePicker';

const mockReqLib = ImagePicker.requestMediaLibraryPermissionsAsync as jest.Mock;
const mockReqCam = ImagePicker.requestCameraPermissionsAsync as jest.Mock;
const mockLaunchLib = ImagePicker.launchImageLibraryAsync as jest.Mock;
const mockLaunchCam = ImagePicker.launchCameraAsync as jest.Mock;

afterEach(() => {
  jest.clearAllMocks();
});

describe('pickFromLibrary', () => {
  it('returns the first asset when permission granted and not cancelled', async () => {
    mockReqLib.mockResolvedValue({ granted: true });
    mockLaunchLib.mockResolvedValue({
      canceled: false,
      assets: [{ uri: 'file:///c.jpg', mimeType: 'image/jpeg', fileName: 'c.jpg', width: 1, height: 1 }],
    });
    const asset = await pickFromLibrary();
    expect(mockLaunchLib).toHaveBeenCalledWith({ mediaTypes: 'images', quality: 0.8, allowsEditing: false });
    expect(asset?.uri).toBe('file:///c.jpg');
  });

  it('returns null when permission denied', async () => {
    mockReqLib.mockResolvedValue({ granted: false });
    const asset = await pickFromLibrary();
    expect(asset).toBeNull();
    expect(mockLaunchLib).not.toHaveBeenCalled();
  });

  it('returns null when the picker is cancelled', async () => {
    mockReqLib.mockResolvedValue({ granted: true });
    mockLaunchLib.mockResolvedValue({ canceled: true, assets: null });
    const asset = await pickFromLibrary();
    expect(asset).toBeNull();
  });
});

describe('takePhoto', () => {
  it('returns the first asset from the camera', async () => {
    mockReqCam.mockResolvedValue({ granted: true });
    mockLaunchCam.mockResolvedValue({
      canceled: false,
      assets: [{ uri: 'file:///cam.jpg', mimeType: 'image/jpeg', fileName: 'cam.jpg', width: 1, height: 1 }],
    });
    const asset = await takePhoto();
    expect(mockLaunchCam).toHaveBeenCalledWith({ mediaTypes: 'images', quality: 0.8 });
    expect(asset?.uri).toBe('file:///cam.jpg');
  });

  it('returns null when camera permission denied', async () => {
    mockReqCam.mockResolvedValue({ granted: false });
    const asset = await takePhoto();
    expect(asset).toBeNull();
    expect(mockLaunchCam).not.toHaveBeenCalled();
  });
});

describe('toUploadFile', () => {
  it('derives uri/name/type from a full asset', () => {
    const f = toUploadFile({ uri: 'file:///c.png', mimeType: 'image/png', fileName: 'c.png' } as any);
    expect(f).toEqual({ uri: 'file:///c.png', name: 'c.png', type: 'image/png' });
  });

  it('falls back to jpeg type and a derived name when fields are missing', () => {
    const f = toUploadFile({ uri: 'file:///x', mimeType: undefined, fileName: null } as any);
    expect(f).toEqual({ uri: 'file:///x', name: 'crossfit.jpeg', type: 'image/jpeg' });
  });
});
```

- [ ] **Step 2: Run the test — expect FAIL.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/lib/__tests__/imagePicker.test.ts 2>&1 | tail -20
```
Expected: FAIL — `Cannot find module '../imagePicker'`.

- [ ] **Step 3: Implement `app/src/lib/imagePicker.ts`** (creates the new `app/src/lib/` dir):
```ts
import * as ImagePicker from 'expo-image-picker';

export type UploadFile = { uri: string; name: string; type: string };

const PICK_OPTIONS = { mediaTypes: 'images' as const, quality: 0.8, allowsEditing: false };

export async function pickFromLibrary(): Promise<ImagePicker.ImagePickerAsset | null> {
  const perm = await ImagePicker.requestMediaLibraryPermissionsAsync();
  if (!perm.granted) return null;
  const result = await ImagePicker.launchImageLibraryAsync(PICK_OPTIONS);
  if (result.canceled || !result.assets?.length) return null;
  return result.assets[0];
}

export async function takePhoto(): Promise<ImagePicker.ImagePickerAsset | null> {
  const perm = await ImagePicker.requestCameraPermissionsAsync();
  if (!perm.granted) return null;
  const result = await ImagePicker.launchCameraAsync({ mediaTypes: 'images', quality: 0.8 });
  if (result.canceled || !result.assets?.length) return null;
  return result.assets[0];
}

export function toUploadFile(asset: ImagePicker.ImagePickerAsset): UploadFile {
  const type = asset.mimeType ?? 'image/jpeg';
  const name = asset.fileName ?? `crossfit.${type.split('/')[1] ?? 'jpg'}`;
  return { uri: asset.uri, name, type };
}
```

- [ ] **Step 4: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/lib/__tests__/imagePicker.test.ts 2>&1 | tail -12
```
Expected: PASS — `Tests: 7 passed`.

- [ ] **Step 5: Commit.**
```bash
cd /home/jake/project/help-my-run && git add app/src/lib/imagePicker.ts app/src/lib/__tests__/imagePicker.test.ts && git commit -m "feat(app): add expo-image-picker wrappers (pickFromLibrary/takePhoto)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---
### Task 33: App — Plan-my-week screen (pick/take photo → parse → edit CF week → generate)

**Files:**
- Create: `app/app/plan.tsx` (PlanScreen)
- Test: `app/app/__tests__/plan.test.tsx` (CREATE — mock `../../src/lib/imagePicker`, `../../src/api/hooks`)
- Modify: `app/app/_layout.tsx` (register `<Stack.Screen name="plan" .../>` and `name="profile"`)

- [ ] **Step 1: Write the failing test.** Create `app/app/__tests__/plan.test.tsx`:
```tsx
import React from 'react';
import { render, fireEvent, act, waitFor } from '@testing-library/react-native';
import type { CrossFitWeek, Plan } from '../../src/api/types';

const parsedWeek: CrossFitWeek = {
  week_start: '2026-06-22',
  days: [
    { date: '2026-06-22', dow: 'Mon', has_crossfit: true, focus: 'Back squat 5x5', cns_load: 'high', leg_load: 'high', notes: 'Heavy legs' },
    { date: '2026-06-23', dow: 'Tue', has_crossfit: true, focus: 'Row intervals', cns_load: 'med', leg_load: 'low', notes: '' },
  ],
};

const generatedPlan: Plan = {
  id: 7, week_start: '2026-06-22', generated_at: '2026-06-20T08:05:12Z',
  fitness_summary: 's', weekly_target_km: 20,
  days: [
    { date: '2026-06-22', dow: 'Mon', run_type: 'rest', distance_km: 0, pace_target: '', time_note: '', optional_if_cns: false, rationale: 'Heavy squat day.' },
  ],
  week_rationale: 'r', one_flag: 'f',
};

const mockPickFromLibrary = jest.fn();
const mockTakePhoto = jest.fn();
const mockToUploadFile = jest.fn();
const mockParseMutateAsync = jest.fn();
const mockGenerateMutate = jest.fn();

// NOTE: plan.tsx imports nothing from expo-router, so no expo-router mock is needed.
jest.mock('../../src/lib/imagePicker', () => ({
  pickFromLibrary: (...a: unknown[]) => mockPickFromLibrary(...a),
  takePhoto: (...a: unknown[]) => mockTakePhoto(...a),
  toUploadFile: (...a: unknown[]) => mockToUploadFile(...a),
}));

jest.mock('../../src/api/hooks', () => ({
  useParseCrossfit: () => ({ mutateAsync: mockParseMutateAsync, isPending: false }),
  useGeneratePlan: () => ({ mutate: mockGenerateMutate, isPending: false, data: undefined }),
}));

import PlanScreen from '../plan';

afterEach(() => {
  jest.clearAllMocks();
});

describe('PlanScreen', () => {
  it('renders the pick/take photo buttons', async () => {
    const { getByTestId } = await render(<PlanScreen />);
    expect(getByTestId('btn-pick-photo')).toBeTruthy();
    expect(getByTestId('btn-take-photo')).toBeTruthy();
  });

  it('parses a picked photo and renders editable per-day cards', async () => {
    mockPickFromLibrary.mockResolvedValue({ uri: 'file:///c.jpg', mimeType: 'image/jpeg', fileName: 'c.jpg' });
    mockToUploadFile.mockReturnValue({ uri: 'file:///c.jpg', name: 'c.jpg', type: 'image/jpeg' });
    mockParseMutateAsync.mockResolvedValue(parsedWeek);

    const { getByTestId } = await render(<PlanScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-pick-photo'));
    });

    await waitFor(() => expect(getByTestId('cf-day-2026-06-22')).toBeTruthy());
    expect(mockParseMutateAsync).toHaveBeenCalledWith({ uri: 'file:///c.jpg', name: 'c.jpg', type: 'image/jpeg' });
    expect(getByTestId('cf-focus-2026-06-22').props.value).toBe('Back squat 5x5');
  });

  it('edits a day focus + CNS load and generates with the edited week', async () => {
    mockPickFromLibrary.mockResolvedValue({ uri: 'file:///c.jpg', mimeType: 'image/jpeg', fileName: 'c.jpg' });
    mockToUploadFile.mockReturnValue({ uri: 'file:///c.jpg', name: 'c.jpg', type: 'image/jpeg' });
    mockParseMutateAsync.mockResolvedValue(parsedWeek);
    mockGenerateMutate.mockReturnValue(undefined);

    const { getByTestId } = await render(<PlanScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-pick-photo'));
    });
    await waitFor(() => expect(getByTestId('cf-day-2026-06-22')).toBeTruthy());

    await act(async () => {
      fireEvent.changeText(getByTestId('cf-focus-2026-06-22'), 'Edited focus');
    });
    await act(async () => {
      fireEvent.press(getByTestId('cf-cns-2026-06-22-low'));
    });
    await act(async () => {
      fireEvent.press(getByTestId('btn-generate'));
    });

    expect(mockGenerateMutate).toHaveBeenCalledTimes(1);
    const arg = mockGenerateMutate.mock.calls[0][0];
    expect(arg.week_start).toBe('2026-06-22');
    expect(arg.crossfit_week.days[0].focus).toBe('Edited focus');
    expect(arg.crossfit_week.days[0].cns_load).toBe('low');
  });

  it('does not show the generate button before a week is parsed', async () => {
    const { queryByTestId } = await render(<PlanScreen />);
    expect(queryByTestId('btn-generate')).toBeNull();
  });
});
```

- [ ] **Step 2: Run the test — expect FAIL.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/plan.test.tsx 2>&1 | tail -20
```
Expected: FAIL — `Cannot find module '../plan'`.

- [ ] **Step 3: Implement `app/app/plan.tsx`.** FULL file:
```tsx
import React, { useState } from 'react';
import { View, Text, TextInput, Pressable, ScrollView, StyleSheet } from 'react-native';
import { useParseCrossfit, useGeneratePlan } from '../src/api/hooks';
import type { CrossFitWeek, CrossFitDay, Load } from '../src/api/types';
import {
  pickFromLibrary,
  takePhoto,
  toUploadFile,
} from '../src/lib/imagePicker';
import type { ImagePickerAsset } from 'expo-image-picker';

const LOADS: Load[] = ['low', 'med', 'high'];

export default function PlanScreen() {
  const parse = useParseCrossfit();
  const generate = useGeneratePlan();
  const [week, setWeek] = useState<CrossFitWeek | null>(null);

  const onPicked = async (asset: ImagePickerAsset | null) => {
    if (!asset) return;
    const file = toUploadFile(asset);
    const parsed = await parse.mutateAsync(file);
    setWeek(parsed);
  };

  const editDay = (i: number, patch: Partial<CrossFitDay>) =>
    setWeek((w) =>
      w ? { ...w, days: w.days.map((d, j) => (j === i ? { ...d, ...patch } : d)) } : w,
    );

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Pressable
        testID="btn-pick-photo"
        style={styles.button}
        disabled={parse.isPending}
        onPress={async () => onPicked(await pickFromLibrary())}
      >
        <Text style={styles.buttonText}>
          {parse.isPending ? 'Parsing…' : 'Pick schedule photo'}
        </Text>
      </Pressable>
      <Pressable
        testID="btn-take-photo"
        style={styles.button}
        disabled={parse.isPending}
        onPress={async () => onPicked(await takePhoto())}
      >
        <Text style={styles.buttonText}>Take photo</Text>
      </Pressable>

      {week ? <Text style={styles.heading}>Week of {week.week_start}</Text> : null}

      {week?.days.map((day, i) => (
        <View key={day.date} testID={`cf-day-${day.date}`} style={styles.card}>
          <Text style={styles.rowTitle}>
            {day.dow} · {day.date}
          </Text>
          <Text style={styles.label}>Focus</Text>
          <TextInput
            testID={`cf-focus-${day.date}`}
            style={styles.input}
            value={day.focus}
            onChangeText={(t) => editDay(i, { focus: t })}
          />
          <Text style={styles.label}>Notes</Text>
          <TextInput
            testID={`cf-notes-${day.date}`}
            style={styles.input}
            value={day.notes}
            onChangeText={(t) => editDay(i, { notes: t })}
          />
          <Pressable
            testID={`cf-hascf-${day.date}`}
            style={[styles.chip, day.has_crossfit && styles.chipOn]}
            onPress={() => editDay(i, { has_crossfit: !day.has_crossfit })}
          >
            <Text style={day.has_crossfit ? styles.chipTextOn : styles.chipText}>
              {day.has_crossfit ? 'CrossFit' : 'Rest'}
            </Text>
          </Pressable>
          <Text style={styles.label}>CNS load</Text>
          <View style={styles.chips}>
            {LOADS.map((lv) => (
              <Pressable
                key={lv}
                testID={`cf-cns-${day.date}-${lv}`}
                style={[styles.chip, day.cns_load === lv && styles.chipOn]}
                onPress={() => editDay(i, { cns_load: lv })}
              >
                <Text style={day.cns_load === lv ? styles.chipTextOn : styles.chipText}>{lv}</Text>
              </Pressable>
            ))}
          </View>
          <Text style={styles.label}>Leg load</Text>
          <View style={styles.chips}>
            {LOADS.map((lv) => (
              <Pressable
                key={lv}
                testID={`cf-leg-${day.date}-${lv}`}
                style={[styles.chip, day.leg_load === lv && styles.chipOn]}
                onPress={() => editDay(i, { leg_load: lv })}
              >
                <Text style={day.leg_load === lv ? styles.chipTextOn : styles.chipText}>{lv}</Text>
              </Pressable>
            ))}
          </View>
        </View>
      ))}

      {week ? (
        <Pressable
          testID="btn-generate"
          style={styles.button}
          disabled={generate.isPending}
          onPress={() => generate.mutate({ week_start: week.week_start, crossfit_week: week })}
        >
          <Text style={styles.buttonText}>
            {generate.isPending ? 'Generating…' : 'Generate plan'}
          </Text>
        </Pressable>
      ) : null}

      {generate.data ? (
        <Text testID="plan-generated" style={styles.statusLine}>
          Plan generated for {generate.data.week_start}
        </Text>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 8 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  label: { fontSize: 14, color: '#444', marginTop: 8 },
  rowTitle: { fontSize: 16, fontWeight: '500' },
  statusLine: { fontSize: 15, color: '#222' },
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
  card: {
    borderWidth: 1,
    borderColor: '#ddd',
    borderRadius: 8,
    padding: 12,
    marginTop: 8,
    gap: 4,
  },
  chips: { flexDirection: 'row', gap: 8, marginTop: 4 },
  chip: {
    borderWidth: 1,
    borderColor: '#ccc',
    borderRadius: 16,
    paddingHorizontal: 14,
    paddingVertical: 6,
    marginTop: 4,
    alignSelf: 'flex-start',
  },
  chipOn: { backgroundColor: '#fc4c02', borderColor: '#fc4c02' },
  chipText: { color: '#444', fontSize: 14 },
  chipTextOn: { color: '#fff', fontSize: 14, fontWeight: '600' },
});
```

- [ ] **Step 4: Register the routes in `app/app/_layout.tsx`.** Add the new screens inside `<Stack>` after the existing `settings` screen:
```tsx
        <Stack.Screen name="plan" options={{ title: 'Plan my week' }} />
        <Stack.Screen name="profile" options={{ title: 'Profile' }} />
```
(Keep the existing `index` and `settings` screens.)

- [ ] **Step 5: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/plan.test.tsx 2>&1 | tail -15
```
Expected: PASS — `Tests: 4 passed`.

- [ ] **Step 6: Commit.**
```bash
git add app/app/plan.tsx app/app/__tests__/plan.test.tsx app/app/_layout.tsx && git commit -m "feat(app): add Plan-my-week screen (parse photo, edit CF week, generate)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 34: App — weekly plan view (per-day cards + fitness summary + rationale + Regenerate)

**Files:**
- Create: `app/app/plan-view.tsx` (PlanViewScreen — renders `usePlan(week)` + `useGeneratePlan` Regenerate)
- Test: `app/app/__tests__/plan-view.test.tsx` (CREATE — mock `expo-router`, `../../src/api/hooks`)
- Modify: `app/app/_layout.tsx` (register `<Stack.Screen name="plan-view" .../>`)

> **Regenerate cold-start note:** Regenerate requires a previously-parsed CrossFit week for the same week. If none exists, `POST /api/plan/generate` returns **404** ("no crossfit week for that week" — see Task 22 handler/contract). The plan view detects that 404 on the mutation error and shows "No CrossFit week for this week — parse a photo first." rather than a generic error.

- [ ] **Step 1: Write the failing test.** Create `app/app/__tests__/plan-view.test.tsx`:
```tsx
import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import type { Plan } from '../../src/api/types';

const plan: Plan = {
  id: 7, week_start: '2026-06-22', generated_at: '2026-06-20T08:05:12Z',
  fitness_summary: '~18 km/week, acute:chronic 1.05.',
  weekly_target_km: 25,
  days: [
    { date: '2026-06-22', dow: 'Mon', run_type: 'rest', distance_km: 0, pace_target: '', time_note: '', optional_if_cns: false, rationale: 'Heavy squat day; no run.' },
    { date: '2026-06-23', dow: 'Tue', run_type: 'easy', distance_km: 5, pace_target: '6:00/km', time_note: '~20:00 after CrossFit', optional_if_cns: true, rationale: 'Low leg load; easy double.' },
  ],
  week_rationale: 'Quality placed Thursday; long run Saturday.',
  one_flag: 'If Thursday skill work runs heavy, downgrade the tempo.',
};

const mockRegenerate = jest.fn();

jest.mock('expo-router', () => ({
  Stack: { Screen: () => null },
  useLocalSearchParams: () => ({ week: '2026-06-22' }),
}));

jest.mock('../../src/api/hooks', () => ({
  usePlan: () => ({ data: plan, isPending: false, isError: false }),
  useGeneratePlan: () => ({ mutate: mockRegenerate, isPending: false, isError: false, error: null }),
}));

import PlanViewScreen from '../plan-view';

afterEach(() => {
  jest.clearAllMocks();
});

describe('PlanViewScreen', () => {
  it('renders the fitness summary and weekly target', async () => {
    const { getByTestId } = await render(<PlanViewScreen />);
    expect(getByTestId('plan-fitness-summary').props.children).toContain('~18 km/week, acute:chronic 1.05.');
    expect(getByTestId('plan-weekly-target').props.children).toContain(25);
  });

  it('renders one card per planned day with type/distance/pace/time/optional', async () => {
    const { getByTestId } = await render(<PlanViewScreen />);
    expect(getByTestId('plan-day-2026-06-22')).toBeTruthy();
    const tue = getByTestId('plan-day-2026-06-23-detail').props.children.join('');
    expect(tue).toContain('5');
    expect(tue).toContain('6:00/km');
    expect(tue).toContain('~20:00 after CrossFit');
    expect(getByTestId('plan-day-2026-06-23-title').props.children.join('')).toContain('optional');
  });

  it('renders the week rationale and one flag', async () => {
    const { getByTestId } = await render(<PlanViewScreen />);
    expect(getByTestId('plan-week-rationale').props.children).toContain('Quality placed Thursday; long run Saturday.');
    expect(getByTestId('plan-one-flag').props.children).toContain('If Thursday skill work runs heavy, downgrade the tempo.');
  });

  it('regenerates the plan for the same week when Regenerate is pressed', async () => {
    const { getByTestId } = await render(<PlanViewScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-regenerate'));
    });
    expect(mockRegenerate).toHaveBeenCalledWith({ week_start: '2026-06-22' });
  });

  it('does NOT show the regenerate error line on the happy path', async () => {
    const { queryByTestId } = await render(<PlanViewScreen />);
    expect(queryByTestId('plan-regenerate-error')).toBeNull();
  });
});

// Cold-start 404: Regenerate with no stored CrossFit week for this week. The
// backend returns 404 ("no crossfit week for that week"); the view must surface a
// friendly, specific message rather than a generic failure. Re-mock the hook for
// this isolated case so useGeneratePlan reports a 404 error.
describe('PlanViewScreen — Regenerate cold-start 404', () => {
  it('surfaces a parse-a-photo hint when there is no CrossFit week for the week', () => {
    jest.resetModules();
    jest.doMock('expo-router', () => ({
      Stack: { Screen: () => null },
      useLocalSearchParams: () => ({ week: '2026-06-22' }),
    }));
    jest.doMock('../../src/api/hooks', () => ({
      usePlan: () => ({ data: undefined, isPending: false, isError: false }),
      useGeneratePlan: () => ({
        mutate: jest.fn(),
        isPending: false,
        isError: true,
        // apiPost rejects with an Error whose message carries the HTTP status + body.
        error: new Error('404: no crossfit week for that week'),
      }),
    }));
    const Isolated = require('../plan-view').default;
    const { getByTestId } = require('@testing-library/react-native').render(<Isolated />);
    expect(getByTestId('plan-regenerate-error').props.children).toContain(
      'No CrossFit week for this week — parse a photo first.',
    );
    jest.dontMock('expo-router');
    jest.dontMock('../../src/api/hooks');
  });
});
```

- [ ] **Step 2: Run the test — expect FAIL.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/plan-view.test.tsx 2>&1 | tail -20
```
Expected: FAIL — `Cannot find module '../plan-view'`.

- [ ] **Step 3: Implement `app/app/plan-view.tsx`.** FULL file:
```tsx
import React from 'react';
import { View, Text, FlatList, Pressable, ScrollView, StyleSheet } from 'react-native';
import { useLocalSearchParams } from 'expo-router';
import { usePlan, useGeneratePlan } from '../src/api/hooks';
import type { PlanDay } from '../src/api/types';

export default function PlanViewScreen() {
  const params = useLocalSearchParams<{ week?: string }>();
  const week = typeof params.week === 'string' ? params.week : '';
  const plan = usePlan(week);
  const generate = useGeneratePlan();

  // Cold start: Regenerate needs a previously-parsed CrossFit week for this week.
  // The backend returns 404 ("no crossfit week for that week") in that case, so
  // surface a specific, friendly hint instead of a generic failure.
  const regenError = generate.isError
    ? /404|no crossfit week/i.test(String(generate.error?.message ?? ''))
      ? 'No CrossFit week for this week — parse a photo first.'
      : 'Could not regenerate the plan. Please try again.'
    : '';

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Fitness</Text>
      <Text testID="plan-fitness-summary" style={styles.statusLine}>
        {plan.data?.fitness_summary ?? '—'}
      </Text>
      <Text testID="plan-weekly-target" style={styles.statusLine}>
        Weekly target: {plan.data?.weekly_target_km ?? 0} km
      </Text>

      <Text style={styles.heading}>Plan</Text>
      <FlatList
        scrollEnabled={false}
        data={plan.data?.days ?? []}
        keyExtractor={(d: PlanDay) => d.date}
        ListEmptyComponent={<Text style={styles.empty}>No plan yet</Text>}
        renderItem={({ item }: { item: PlanDay }) => (
          <View testID={`plan-day-${item.date}`} style={styles.row}>
            <Text testID={`plan-day-${item.date}-title`} style={styles.rowTitle}>
              {item.dow} · {item.run_type}
              {item.optional_if_cns ? ' (optional)' : ''}
            </Text>
            <Text testID={`plan-day-${item.date}-detail`} style={styles.rowSub}>
              {item.distance_km} km · {item.pace_target || '—'} · {item.time_note || '—'}
            </Text>
            <Text style={styles.rowSub}>{item.rationale}</Text>
          </View>
        )}
      />

      <Text style={styles.heading}>Why this week</Text>
      <Text testID="plan-week-rationale" style={styles.statusLine}>
        {plan.data?.week_rationale ?? '—'}
      </Text>
      <Text style={styles.heading}>Flag</Text>
      <Text testID="plan-one-flag" style={styles.statusLine}>
        {plan.data?.one_flag ?? '—'}
      </Text>

      <Pressable
        testID="btn-regenerate"
        style={styles.button}
        disabled={generate.isPending || !week}
        onPress={() => generate.mutate({ week_start: week })}
      >
        <Text style={styles.buttonText}>
          {generate.isPending ? 'Regenerating…' : 'Regenerate'}
        </Text>
      </Pressable>
      {regenError ? (
        <Text testID="plan-regenerate-error" style={styles.errorLine}>
          {regenError}
        </Text>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 6 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  statusLine: { fontSize: 15, color: '#222' },
  row: { paddingVertical: 8, borderBottomWidth: StyleSheet.hairlineWidth, borderBottomColor: '#ddd' },
  rowTitle: { fontSize: 16, fontWeight: '500' },
  rowSub: { fontSize: 13, color: '#666' },
  empty: { fontSize: 14, color: '#999', paddingVertical: 8 },
  button: {
    backgroundColor: '#fc4c02',
    borderRadius: 8,
    paddingVertical: 12,
    alignItems: 'center',
    marginTop: 16,
  },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
  errorLine: { color: '#c0392b', fontSize: 14, marginTop: 8 },
});
```

- [ ] **Step 4: Register the route in `app/app/_layout.tsx`.** Add inside `<Stack>` after the `profile` screen:
```tsx
        <Stack.Screen name="plan-view" options={{ title: 'Weekly plan' }} />
```

- [ ] **Step 5: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/plan-view.test.tsx 2>&1 | tail -15
```
Expected: PASS — `Tests: 6 passed` (4 render/regenerate cases + the happy-path no-error case + the cold-start 404 case).

- [ ] **Step 6: Commit.**
```bash
cd /home/jake/project/help-my-run && git add app/app/plan-view.tsx app/app/__tests__/plan-view.test.tsx app/app/_layout.tsx && git commit -m "feat(app): add weekly plan view (per-day cards, rationale, regenerate)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---
### Task 35: App — Profile/Settings screen

**Files:**
- Create: `app/app/profile.tsx` (ProfileScreen)
- Test: `app/app/__tests__/profile.test.tsx` (CREATE — mock `../../src/api/hooks`)

(The `profile` route was registered in `_layout.tsx` in Task 33.)

- [ ] **Step 1: Write the failing test.** Create `app/app/__tests__/profile.test.tsx`:
```tsx
import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import type { AthleteProfile } from '../../src/api/types';

const profile: AthleteProfile = {
  target_weekly_km: 20,
  progression_mode: 'build',
  zone2_ceiling_bpm: 150,
  threshold_bpm: 168,
  max_hr_bpm: 190,
  run_constraints_json: '{"crossfit_days":["Mon","Tue"]}',
  goal_text: 'Build cardio over time',
  updated_at: '2026-06-20T08:00:00Z',
};

const mockUpdateMutate = jest.fn();

// NOTE: profile.tsx imports nothing from expo-router, so no expo-router mock is needed.
jest.mock('../../src/api/hooks', () => ({
  useProfile: () => ({ data: profile, isPending: false, isError: false }),
  useUpdateProfile: () => ({ mutate: mockUpdateMutate, isPending: false }),
}));

import ProfileScreen from '../profile';

afterEach(() => {
  jest.clearAllMocks();
});

describe('ProfileScreen', () => {
  it('prefills inputs from the loaded profile', async () => {
    const { getByTestId } = await render(<ProfileScreen />);
    expect(getByTestId('input-target-km').props.value).toBe('20');
    expect(getByTestId('input-goal').props.value).toBe('Build cardio over time');
    expect(getByTestId('input-zone2').props.value).toBe('150');
    expect(getByTestId('input-threshold-bpm').props.value).toBe('168');
    expect(getByTestId('input-maxhr').props.value).toBe('190');
    expect(getByTestId('input-constraints').props.value).toBe('{"crossfit_days":["Mon","Tue"]}');
  });

  it('shows the active progression mode', async () => {
    const { getByTestId } = await render(<ProfileScreen />);
    expect(getByTestId('mode-build')).toBeTruthy();
    expect(getByTestId('mode-hold')).toBeTruthy();
  });

  it('saves an edited profile with parsed numeric fields and nulls for blanks', async () => {
    const { getByTestId } = await render(<ProfileScreen />);
    await act(async () => {
      fireEvent.changeText(getByTestId('input-target-km'), '25');
    });
    await act(async () => {
      fireEvent.press(getByTestId('mode-hold'));
    });
    await act(async () => {
      fireEvent.changeText(getByTestId('input-zone2'), '');
    });
    await act(async () => {
      fireEvent.changeText(getByTestId('input-goal'), 'Run a half');
    });
    await act(async () => {
      fireEvent.press(getByTestId('btn-save-profile'));
    });

    expect(mockUpdateMutate).toHaveBeenCalledTimes(1);
    const arg = mockUpdateMutate.mock.calls[0][0] as AthleteProfile;
    expect(arg.target_weekly_km).toBe(25);
    expect(arg.progression_mode).toBe('hold');
    expect(arg.zone2_ceiling_bpm).toBeNull();
    expect(arg.threshold_bpm).toBe(168);
    expect(arg.max_hr_bpm).toBe(190);
    expect(arg.goal_text).toBe('Run a half');
    expect(arg.run_constraints_json).toBe('{"crossfit_days":["Mon","Tue"]}');
  });
});
```

- [ ] **Step 2: Run the test — expect FAIL.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/profile.test.tsx 2>&1 | tail -20
```
Expected: FAIL — `Cannot find module '../profile'`.

- [ ] **Step 3: Implement `app/app/profile.tsx`.** FULL file:
```tsx
import React, { useEffect, useState } from 'react';
import { View, Text, TextInput, Pressable, ScrollView, StyleSheet } from 'react-native';
import { useProfile, useUpdateProfile } from '../src/api/hooks';
import type { AthleteProfile } from '../src/api/types';

type Mode = 'build' | 'hold';

function parseIntOrNull(s: string): number | null {
  const t = s.trim();
  if (t === '') return null;
  const n = parseInt(t, 10);
  return Number.isNaN(n) ? null : n;
}

function parseFloatOr(s: string, fallback: number): number {
  const n = parseFloat(s.trim());
  return Number.isNaN(n) ? fallback : n;
}

export default function ProfileScreen() {
  const profile = useProfile();
  const update = useUpdateProfile();

  const [targetKm, setTargetKm] = useState('');
  const [mode, setMode] = useState<Mode>('build');
  const [zone2, setZone2] = useState('');
  const [thresholdBpm, setThresholdBpm] = useState('');
  const [maxHr, setMaxHr] = useState('');
  const [constraints, setConstraints] = useState('');
  const [goal, setGoal] = useState('');

  const loaded = profile.data;
  useEffect(() => {
    if (loaded) {
      setTargetKm(String(loaded.target_weekly_km));
      setMode(loaded.progression_mode);
      setZone2(loaded.zone2_ceiling_bpm != null ? String(loaded.zone2_ceiling_bpm) : '');
      setThresholdBpm(loaded.threshold_bpm != null ? String(loaded.threshold_bpm) : '');
      setMaxHr(loaded.max_hr_bpm != null ? String(loaded.max_hr_bpm) : '');
      setConstraints(loaded.run_constraints_json);
      setGoal(loaded.goal_text);
    }
  }, [loaded]);

  const onSave = () => {
    const body: AthleteProfile = {
      target_weekly_km: parseFloatOr(targetKm, 20),
      progression_mode: mode,
      zone2_ceiling_bpm: parseIntOrNull(zone2),
      threshold_bpm: parseIntOrNull(thresholdBpm),
      max_hr_bpm: parseIntOrNull(maxHr),
      run_constraints_json: constraints,
      goal_text: goal,
    };
    update.mutate(body);
  };

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Target</Text>
      <Text style={styles.label}>Target weekly km</Text>
      <TextInput
        testID="input-target-km"
        style={styles.input}
        keyboardType="numeric"
        value={targetKm}
        onChangeText={setTargetKm}
      />

      <Text style={styles.label}>Progression mode</Text>
      <View style={styles.chips}>
        <Pressable
          testID="mode-build"
          style={[styles.chip, mode === 'build' && styles.chipOn]}
          onPress={() => setMode('build')}
        >
          <Text style={mode === 'build' ? styles.chipTextOn : styles.chipText}>build</Text>
        </Pressable>
        <Pressable
          testID="mode-hold"
          style={[styles.chip, mode === 'hold' && styles.chipOn]}
          onPress={() => setMode('hold')}
        >
          <Text style={mode === 'hold' ? styles.chipTextOn : styles.chipText}>hold</Text>
        </Pressable>
      </View>

      <Text style={styles.heading}>HR markers (optional)</Text>
      <Text style={styles.label}>Zone 2 ceiling bpm</Text>
      <TextInput
        testID="input-zone2"
        style={styles.input}
        keyboardType="numeric"
        value={zone2}
        onChangeText={setZone2}
      />
      <Text style={styles.label}>Threshold bpm</Text>
      <TextInput
        testID="input-threshold-bpm"
        style={styles.input}
        keyboardType="numeric"
        value={thresholdBpm}
        onChangeText={setThresholdBpm}
      />
      <Text style={styles.label}>Max HR bpm</Text>
      <TextInput
        testID="input-maxhr"
        style={styles.input}
        keyboardType="numeric"
        value={maxHr}
        onChangeText={setMaxHr}
      />

      <Text style={styles.heading}>Constraints</Text>
      <Text style={styles.label}>Run constraints (JSON)</Text>
      <TextInput
        testID="input-constraints"
        style={[styles.input, styles.multiline]}
        autoCapitalize="none"
        autoCorrect={false}
        multiline
        value={constraints}
        onChangeText={setConstraints}
      />

      <Text style={styles.heading}>Goal</Text>
      <TextInput
        testID="input-goal"
        style={[styles.input, styles.multiline]}
        multiline
        value={goal}
        onChangeText={setGoal}
      />

      <Pressable
        testID="btn-save-profile"
        style={styles.button}
        disabled={update.isPending}
        onPress={onSave}
      >
        <Text style={styles.buttonText}>{update.isPending ? 'Saving…' : 'Save profile'}</Text>
      </Pressable>
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
  multiline: { minHeight: 64, textAlignVertical: 'top' },
  chips: { flexDirection: 'row', gap: 8, marginTop: 4 },
  chip: {
    borderWidth: 1,
    borderColor: '#ccc',
    borderRadius: 16,
    paddingHorizontal: 16,
    paddingVertical: 6,
    alignSelf: 'flex-start',
  },
  chipOn: { backgroundColor: '#fc4c02', borderColor: '#fc4c02' },
  chipText: { color: '#444', fontSize: 14 },
  chipTextOn: { color: '#fff', fontSize: 14, fontWeight: '600' },
  button: {
    backgroundColor: '#fc4c02',
    borderRadius: 8,
    paddingVertical: 12,
    alignItems: 'center',
    marginTop: 16,
  },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
});
```

- [ ] **Step 4: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/profile.test.tsx 2>&1 | tail -15
```
Expected: PASS — `Tests: 3 passed`.

- [ ] **Step 5: Commit.**
```bash
git add app/app/profile.tsx app/app/__tests__/profile.test.tsx && git commit -m "feat(app): add profile/settings screen (target km, mode, HR, constraints, goal)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 36: App — home navigation links + full app suite green

**Files:**
- Modify: `app/app/index.tsx` (add `<Link href="/plan">` and `<Link href="/profile">` next to the existing Settings link)
- Modify: `app/app/__tests__/index.test.tsx` (assert the two new nav links render)

- [ ] **Step 1: Write the failing test.** Add a new test at the END of the `describe('HomeScreen', ...)` block in `app/app/__tests__/index.test.tsx`:
```tsx
  it('renders navigation links to Plan and Profile', async () => {
    const { getByText } = await render(<HomeScreen />);
    expect(getByText('Plan my week')).toBeTruthy();
    expect(getByText('Profile')).toBeTruthy();
  });
```

- [ ] **Step 2: Run the test — expect FAIL.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/index.test.tsx 2>&1 | tail -15
```
Expected: FAIL — `Unable to find an element with text: Plan my week`.

- [ ] **Step 3: Add the nav links to `app/app/index.tsx`.** After the existing Settings `<Link>`:
```tsx
      <Link href="/plan" style={styles.link}>
        Plan my week
      </Link>
      <Link href="/profile" style={styles.link}>
        Profile
      </Link>
```
(Match the existing M0 `<Link>`/`styles.link` usage; keep the Settings link.)

- [ ] **Step 4: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/index.test.tsx 2>&1 | tail -12
```
Expected: PASS.

- [ ] **Step 5: Run the FULL app test suite — expect all green.**
```bash
cd /home/jake/project/help-my-run/app && npx jest 2>&1 | tail -20
```
Expected: PASS — every suite green (M0 suites + new `client`, `types`, `hooks`, `imagePicker`, `plan`, `plan-view`, `profile`, `index`). Zero failures.

- [ ] **Step 6: Commit.**
```bash
cd /home/jake/project/help-my-run && git add app/app/index.tsx app/app/__tests__/index.test.tsx && git commit -m "feat(app): add home nav links to Plan and Profile screens

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 37: MANUAL verification — real photo → plan flow on device/simulator

**Files:** (none — manual; the real `expo-image-picker` + camera/photo-library + live `claude -p` backend cannot be unit-tested)

This task is NOT automatable: it exercises the real OS photo picker / camera permission prompts and a live backend whose `POST /api/crossfit/parse` shells out to `claude -p` (vision). Perform once the backend M1 endpoints are running on a host with `claude` installed and logged in (`claude auth login`).

> **Auth note for this manual run.** `claude -p` reads `~/.claude/.credentials.json`, so the backend host must have had `claude auth login` (NOT `--console`) run interactively. If the backend runs on a browser-less/remote host, mint a token elsewhere with `claude setup-token` and expose it to that host's env; `ANTHROPIC_API_KEY` is a paid fallback. (CI never hits real `claude` — stub runner — so this only matters for this manual end-to-end check.)

- [ ] **Step 1: Start the backend with M1 endpoints + a logged-in Claude.** Ensure `claude auth login` was completed (NOT `--console`), then:
```bash
cd /home/jake/project/help-my-run/backend && go run ./cmd/server
```
Expected: server listening on `:8080`; `GET /health` returns `200`. Verify Claude auth: `test -f ~/.claude/.credentials.json && echo "claude logged in"`.

- [ ] **Step 2: Launch the Expo app (dev build, real device or simulator — NOT web).**
```bash
cd /home/jake/project/help-my-run/app && npx expo run:ios
```
(or `npx expo run:android`). Expected: the app boots to the Home screen.

- [ ] **Step 2b: Configure backend connection.** Home → Settings → set Backend URL (e.g. `http://<host-lan-ip>:8080`) and the `API_TOKEN`, press Save. Expected: returning to Home shows non-error status.

- [ ] **Step 3: Open Plan my week and pick a real CrossFit schedule photo.** Home → "Plan my week" → "Pick schedule photo". Expected: the OS photo-library permission prompt appears (text matches the `photosPermission` string from Task 28); after granting and selecting a real box-schedule image, the button shows "Parsing…", then per-day editable cards (`cf-day-*`) render with focus/CNS/leg fields populated by Stage-1 Claude vision.

- [ ] **Step 3b: (Camera variant) Take a photo instead.** Press "Take photo". Expected: the camera permission prompt appears (matching `cameraPermission`); after capturing, the same parsed per-day cards render.

- [ ] **Step 4: Edit a parsed day, then Generate.** Change one day's Focus text and tap a different CNS-load chip, then press "Generate plan". Expected: button shows "Generating…", the backend runs Stage 2 (`claude -p`), and a plan is returned (the `plan-generated` confirmation appears with the week_start). The edited CrossFit week (not the originally parsed one) must be what was sent — confirm via the backend log / `plans.context_pack_json`.

- [ ] **Step 5: View the weekly plan + Regenerate.** Navigate to `/plan-view?week=<the Monday>`. Expected: per-day run cards (type / distance / pace / time note / "(optional)" flag), the fitness summary, weekly target, week rationale, and one flag all render. Press "Regenerate"; a fresh plan for the same week is produced and the view updates after refetch.

- [ ] **Step 6: Verify the Profile screen round-trips against the live backend.** Home → "Profile": confirm fields prefill from `GET /api/profile`. Edit target weekly km, toggle progression mode, clear an HR marker, edit the goal, press "Save profile". Expected: `PUT /api/profile` succeeds; reopening Profile shows the persisted values (blank HR marker persisted as `null`).

- [ ] **Step 7: Verify the no-Claude/limit error path is surfaced.** Temporarily stop/log out Claude (or trigger a limit) and retry "Generate plan". Expected: the backend returns `502` and the app surfaces an error (mutation `isError`) rather than crashing; the fitness read / metrics still render from local data (Claude-independent). Re-login restores the flow.

- [ ] **Step 8: Record the result.** Confirm the M1 §3 end-to-end criterion: a real CrossFit photo produced a dated, progressive plan with runs placed around CrossFit load. No code change/commit for this task; note the outcome (pass/fail + any parsing imperfections, expected to be fixable via the editable Stage-1 cards) in the milestone verification log.

---
## Definition of Done

Each M1 success criterion (spec §3) is mapped to the task(s) that satisfy it. M1 is done when every task above is checked off, `go test ./...` (Task 27) and `npx jest` (Task 36) are fully green, and the manual claude-login check (Task 37) passes.

| # | M1 success criterion (spec §3) | Satisfied by |
|---|---|---|
| 1 | Upload a CrossFit schedule image → structured per-day CrossFit read (focus + CNS/leg load), **editable** before planning | Backend: image save (Task 21), Stage-1 parse via `claude -p` + `crossfit_weeks` upsert (Tasks 16–20, 22 `POST /api/crossfit/parse`). App: image picker (Task 32), `useParseCrossfit` (Task 31), editable per-day cards on the Plan screen (Task 33). |
| 2 | Generate a plan → 7-day schedule (run type, distance, pace target, evening-double time note, "optional if CNS fried" flag), safe weekly target, hard runs off heavy CrossFit days, quality favored on light/Thu + weekends | Backend: Coach Brain Stage-2 prompt (Task 19), context-pack assembly + Stage-2 call (Task 20), `POST /api/plan/generate` + `plans` insert (Task 22). App: `useGeneratePlan` (Task 31), generate button (Task 33), weekly plan view rendering type/distance/pace/time/optional (Task 34). |
| 3 | Plan grounded in **computed fitness** (recent volume, acute:chronic load, easy/threshold paces, recovery trend), shown as a short read | Backend metrics engine (Tasks 8–15: volume, 4-week avg, acute:chronic, paces, recovery trend, safe target); fed into the context pack (Task 20) and served at `GET /api/fitness` (Task 22). App: `useFitness` (Task 31) + fitness summary on the plan view (Task 34). |
| 4 | **Week-over-week progression** (~≤10% ramp, periodic cutback week) using last week's plan | Backend: `safeWeeklyTarget` (+10% ramp, capped) and `isCutbackWeek` (every 4th week) in metrics (Task 14); `last_week_plan` loaded into the context pack (Task 20); Coach Brain ramp/cutback rules (Task 19). |
| 5 | Regenerate works; profile, parsed weeks, and plans persist | Backend: `athlete_profile`/`crossfit_weeks`/`plans` stores (Tasks 2–5), profile/plan handlers (Task 22), `GET /api/plan?week=` latest-plan retrieval (Task 22). App: Regenerate button re-invoking `useGeneratePlan` (Task 34), profile round-trip via `useProfile`/`useUpdateProfile` (Tasks 31, 35), home nav (Task 36). |
| — | M0 review follow-ups folded into M1 (spec §9): validate Strava OAuth `state` (CSRF); run sync once on boot; Strava cursor from latest stored activity `start_time` | OAuth state persist/validate (Tasks 7, 24); sync-on-boot (Task 23); latest-`start_time` cursor (Tasks 6, 25). |
| — | LLM access via `claude -p` under the subscription, no API key; one retry on malformed JSON; failures classified (logged in? limit hit?) | `llm` client: Envelope/ExtractJSON/ClassifyFailure (Task 16), `Client.Call` one-retry loop (Task 17); exact `claude -p` flags incl. no `--bare`/no `ANTHROPIC_API_KEY` (Task 20); config + docs (Tasks 1, 26). |

**Manual claude-login plan-generation check (gates "done"):** On a host with the `claude` CLI installed and logged in (`claude auth login`, NOT `--console`; no `ANTHROPIC_API_KEY`), run the full Task 37 flow end-to-end: pick/take a real CrossFit schedule photo → Stage-1 parse renders editable per-day cards → edit a day → Generate → a dated, progressive plan returns with runs placed around CrossFit load → view it and Regenerate → profile round-trips. Confirm the no-Claude/limit error path returns `502` and is surfaced in the app while the local fitness read still renders. This exercises the real `claude -p` subprocess path that the CI stub-runner tests deliberately do not.
