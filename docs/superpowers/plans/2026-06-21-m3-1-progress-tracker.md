# Milestone 3.1 (Cardio-Capacity Progress Tracker) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show whether the user's aerobic engine is measurably improving for RX CrossFit (not racing) via deterministic per-signal trend cards (pace-at-fixed-HR, VO2max, resting HR, HRV baseline, weekly load) with unicode sparklines plus an on-demand `claude -p` narrative read.

**Architecture:** M3.1 extends the merged M0 (Garmin/Strava ingestion + SQLite store) / M1 (CrossFit weekly-plan metrics) / M2 (agentic `claude -p` coach with deterministic fallback) codebase. It adds VO2max ingestion (Python worker → Go `WorkerOutput` → `garmin_vo2max` store table via `SyncGarmin`), a new pure deterministic Go progress engine (`internal/progress`) that builds weekly-bucketed trend series, an `Engine.Analyze` that reuses the shared `*llm.Client` for a `claude -p` read with a deterministic templated fallback, two bearer-protected REST routes (`GET /api/progress`, `POST /api/progress/analyze`), and an Expo Progress screen with trend cards + a pure unicode sparkline helper (no charting dependency).

**Tech Stack:** Go (chi router, modernc.org/sqlite, goose migrations); Python worker (`python-garminconnect`); `claude` CLI via the shared `internal/llm` client; Expo / expo-router / @tanstack/react-query (no chart library — unicode block sparklines). All M3.1 wire JSON is **snake_case** (consistent with M0/M1/M2 + `app/src/api/types.ts`).

---

## Setup Prerequisites

- Builds directly on the **M0 + M1 + M2 codebase already merged to `main`** in `/home/jake/project/help-my-run`. No new branch assumed by these tasks (each task commits to the current branch); use a worktree/branch per your execution skill if required.
- `claude` CLI is **already logged in** on the host (M2 established this); `ClaudeBin`/`ClaudeModel` config already exist and are reused via the shared `*llm.Client`. **No new config/env** is required (§7.1: reference HR comes from `profile.Zone2CeilingBpm` with the documented `defaultRefHRBpm = 145.0` constant fallback).
- After deploy, a **Garmin sync backfills VO2max**: `garminBackfillDays` is bumped 30 → 84 so the first sync seeds ~12 weeks of VO2max + recovery history (first-sync-only; later syncs use `sync_log.last_synced_at`). The **manual real-data check** (Definition of Done) is performed after that first sync.
- Build/test commands (verified): backend `cd /home/jake/project/help-my-run/backend && go test ./...`; worker `cd /home/jake/project/help-my-run/garmin-worker && pytest tests -q`; app `cd /home/jake/project/help-my-run/app && npm test -- --watchAll=false`.

---

## File Structure

### New files
- `backend/internal/store/migrations/00005_m3_1_vo2max.sql` — `garmin_vo2max` table.
- `backend/internal/store/vo2max_test.go` — store round-trip/idempotency/ordering test.
- `backend/internal/progress/progress.go` — pure engine: `ComputeProgress`, structs, signal/direction constants, ref-HR band, weekly bucketing, per-signal series builders.
- `backend/internal/progress/progress_test.go` — table-driven engine tests (JSON tags, bucketing, summarize, series, full report, not-enough-data).
- `backend/internal/progress/prompts.go` — `progressReadPrompt`, `ProgressReadInput`, `ProgressRead`, `fallbackProgressText`.
- `backend/internal/progress/engine.go` — `Engine{store,llm,model}`, `New`, `Report`, `Analyze`, `analyzeArgs`.
- `backend/internal/progress/engine_test.go` — `captureRunner`/`failRunner` ai-vs-fallback + Report tests.
- `backend/internal/api/progress_handlers.go` — `progress`, `analyzeProgress` handlers, `analyzeProgressRequest`.
- `backend/internal/api/progress_handlers_test.go` — httptest handler tests with `fakeProgress`.
- `garmin-worker/tests/test_client.py` — `get_max_metrics` delegation test.
- `garmin-worker/tests/fixtures/raw_max_metrics_2026-06-15.json` — VO2max normalize fixture.
- `app/src/lib/sparkline.ts` — unicode sparkline helper.
- `app/src/lib/__tests__/sparkline.test.ts` — sparkline unit test.
- `app/app/progress.tsx` — Progress screen (default export `ProgressScreen`, route `/progress`).
- `app/app/__tests__/progress.test.tsx` — screen test.

### Modified files
- `garmin-worker/garmin_worker/client.py` — add `get_max_metrics` delegation.
- `garmin-worker/garmin_worker/normalize.py` — add `normalize_vo2max_day`; add `vo2max` kwarg+key (last) to `build_output`.
- `garmin-worker/garmin_worker/fetcher.py` — init `vo2max=[]`, append in per-day loop (omit nulls), pass to `build_output`.
- `garmin-worker/garmin_worker/cli.py` — add `_DRY_VO2MAX_RAW`, wire into `_run_dry_fetch`.
- `garmin-worker/tests/fixtures/dry_run_expected.json` — append `vo2max` key (last).
- `garmin-worker/tests/test_normalize.py` — add vo2max normalize tests; update `build_output` key-order asserts.
- `garmin-worker/tests/test_fetcher.py` — extend `_MockClient`; assert vo2max array + omit-null; update key-order asserts.
- `garmin-worker/tests/test_cli.py` — include `"vo2max"` in key-set assert.
- `backend/internal/garmin/types.go` — `Vo2maxDay` struct + `WorkerOutput.VO2Max` (last).
- `backend/internal/garmin/testdata/worker_output.json` — append `vo2max` array.
- `backend/internal/garmin/runner_test.go` — assert parsed `VO2Max`.
- `backend/internal/store/garmin.go` — `Vo2maxRow`+`UpsertVo2max`, `Vo2maxPoint`+`ListVo2max`.
- `backend/internal/store/store_test.go` — add `"garmin_vo2max"` to `wantTables`.
- `backend/internal/sync/sync.go` — VO2max upsert loop; `garminBackfillDays = 84`; doc comments.
- `backend/internal/sync/sync_test.go` — synced 7→9, vo2max count=2, new 84-day backfill test.
- `backend/internal/metrics/metrics.go` — export `IsRun`, `ParseStart`, `Median`, `FormatPace` (lowercase aliases retained).
- `backend/internal/metrics/metrics_test.go` — exported-helpers test.
- `backend/internal/api/router.go` — `Progress` interface; `Deps.Progress`; 2 routes.
- `backend/internal/api/m2_fakes_test.go` — `fakeProgress` + `var _ Progress = (*fakeProgress)(nil)`.
- `backend/internal/api/handlers_test.go` — `Progress: &fakeProgress{}` in `newTestServer`.
- `backend/cmd/server/main.go` — construct `progress.Engine` in `Wire`; inject `api.Deps.Progress`; add `App.Progress`.
- `backend/cmd/server/main_test.go` — `TestWireInjectsProgress`.
- `app/src/api/types.ts` — add `TrendDirection`, `TrendSummary`, `ProgressReport`, `ProgressRead`.
- `app/src/api/hooks.ts` — add `useProgress`, `useAnalyzeProgress` + imports.
- `app/src/api/__tests__/hooks.test.tsx` — add the 2 hooks (+ types) to imports + tests.
- `app/app/_layout.tsx` — register `progress` Stack.Screen.
- `app/app/index.tsx` — add Progress `<Link>`.
- `app/app/__tests__/index.test.tsx` — Progress-link assertion.

---

## Shared Contracts

**CONVENTION (binding):** All M3.1 wire JSON is **snake_case** (consistent with the entire M0/M1/M2 codebase + `app/src/api/types.ts`). Go struct field names are idiomatic PascalCase; their json tags are snake_case. Do not introduce camelCase wire fields.

### Migration — `00005_m3_1_vo2max.sql`

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE garmin_vo2max (
    date     TEXT PRIMARY KEY,
    vo2max   REAL,
    raw_json TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE garmin_vo2max;
-- +goose StatementEnd
```

Notes: no seed rows (matches `garmin_rhr`); VO2max rides the existing `'garmin'` sync_log source. `migrate.go` auto-embeds via `//go:embed migrations/*.sql` — no code change. Add `"garmin_vo2max"` to the `wantTables` migration/table-check assertion.

### VO2max ingestion contract (worker → Go → store)

**python-garminconnect method (VERIFIED against installed 0.3.6):** `get_max_metrics(self, cdate: str) -> dict[str, Any]` returns `self.connectapi(...)` of the maxmet DAILY RANGE endpoint (`/{cdate}/{cdate}`) with NO transform. Despite the `dict` type hint, the real endpoint returns a one-element **LIST** (e.g. `[{"generic": {"vo2MaxValue": 52.0, "calendarDate": "2026-06-20"}, ...}]`). Unwrap `raw[0]` first, then read `["generic"]["vo2MaxValue"]` → float (e.g. `44.0`); an empty list, or `generic` `null`/absent → no-data → null-guard via `_get(...)`. (A plain dict is also tolerated for fixtures.)

**Ingestion risk (VERIFIED, GitHub #74/#253):** the endpoint tends to echo the *latest* known metric with a fixed `generic.calendarDate` regardless of the `cdate` requested. Mitigation: keep the full raw payload in `raw_json` (the list/dict it returns, containing `generic.calendarDate`) so duplicates are detectable; bucket by the worker-supplied loop date. Accept "latest-only" semantics on first sync. **Null-day policy: OMIT** days where the unwrapped `generic.vo2MaxValue` is absent (empty list or `generic` null) (mirrors HRV omission).

Worker `client.py` delegation (1:1; returns whatever the library returns — a list on the real endpoint):
```python
    def get_max_metrics(self, cdate: str):
        return self._g.get_max_metrics(cdate)
```

Worker `normalize.py` normalizer + `build_output` (vo2max LAST key). Unwrap the one-element list (real endpoint shape) before walking `generic.vo2MaxValue`; preserve the ORIGINAL payload in `raw_json`:
```python
def normalize_vo2max_day(date: str, raw) -> dict:
    """Map get_max_metrics(date) -> Vo2maxDay (CONTRACTS §2.2).

    get_max_metrics returns the maxmet daily-range payload, a one-element
    LIST on the real endpoint (raw[0].generic.vo2MaxValue); a plain dict
    is tolerated for fixtures.
    """
    payload = raw
    if isinstance(payload, list):
        payload = payload[0] if payload else None
    val = _get(payload, "generic", "vo2MaxValue")
    return {
        "date": date,
        "vo2max": val,
        "raw_json": raw if raw is not None else {},
    }
```
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

Worker top-level stdout JSON gains a `vo2max` array (LAST key, one entry per emitted day, value float):
```json
"vo2max": [
  {"date": "2026-06-14", "vo2max": 51.0, "raw_json": { "...": "..." }}
]
```

Go `garmin/types.go`:
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

// Vo2maxDay is one per-day VO2max entry (running VO2max from get_max_metrics).
type Vo2maxDay struct {
	Date    string          `json:"date"`
	VO2Max  *float64        `json:"vo2max"`
	RawJSON json.RawMessage `json:"raw_json"`
}
```
Canonical Go identifiers: type `Vo2maxDay`; fields `Date`, `VO2Max`, `RawJSON`; `WorkerOutput.VO2Max`. `runner.go` needs no change.

Go `store/garmin.go`:
```go
// Vo2maxRow maps to garmin_vo2max.
type Vo2maxRow struct {
	Date    string
	Vo2max  *float64
	RawJSON string
}

// UpsertVo2max upserts one garmin_vo2max row by date.
func (s *Store) UpsertVo2max(r Vo2maxRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_vo2max (date, vo2max, raw_json)
		VALUES (?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			vo2max=excluded.vo2max, raw_json=excluded.raw_json`,
		r.Date, r.Vo2max, r.RawJSON)
	return err
}

// Vo2maxPoint is one dated VO2max reading (vo2max may be nil when stored null).
type Vo2maxPoint struct {
	Date   string
	Vo2max *float64
}

// ListVo2max returns up to `limit` garmin_vo2max rows, most-recent-first by date.
func (s *Store) ListVo2max(limit int) ([]Vo2maxPoint, error) {
	rows, err := s.DB.Query(`
		SELECT date, vo2max FROM garmin_vo2max
		ORDER BY date DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Vo2maxPoint
	for rows.Next() {
		var p Vo2maxPoint
		if err := rows.Scan(&p.Date, &p.Vo2max); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
```

Go `sync/sync.go` upsert loop + 84-day backfill:
```go
	for _, d := range out.VO2Max {
		if err := s.UpsertVo2max(store.Vo2maxRow{
			Date: d.Date, Vo2max: d.VO2Max, RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
```
```go
// garminBackfillDays is the default look-back when Garmin has never synced.
// M3.1: 84d (~12 weeks) to seed VO2max + recovery trend history (spec §4).
const garminBackfillDays = 84
```
Bumping 30→84 also deepens sleep/hrv/bb/rhr first-sync history (first-sync-only; acceptable, arguably desirable — those trends feed the progress engine too). No separate VO2max-only window.

### Progress engine types + reference-HR method

Signal keys (string constants — verbatim):
```go
const (
	SignalPaceAtHR    = "pace_at_hr"   // headline: weekly-median pace of in-band runs (sec/km)
	SignalVo2max      = "vo2max"       // Garmin VO2max
	SignalRestingHR   = "resting_hr"   // garmin_rhr
	SignalHRVBaseline = "hrv_baseline" // garmin_hrv last-night avg ms
	SignalWeeklyLoad  = "weekly_load"  // weekly running km (M1 metrics)
)
```

Result structs (snake_case json tags — served directly as the DTO):
```go
// TrendDirection is the value-movement direction of a signal over the window.
type TrendDirection string

const (
	DirectionUp   TrendDirection = "up"
	DirectionDown TrendDirection = "down"
	DirectionFlat TrendDirection = "flat"
)

// TrendSummary is one signal's trend card: weekly series + headline summary.
// Series has exactly weeks entries, oldest-first; nil = a week with no
// qualifying data (rendered as a gap, never interpolated).
type TrendSummary struct {
	Key           string         `json:"key"`
	Label         string         `json:"label"`
	Unit          string         `json:"unit"`
	Current       *float64       `json:"current"`
	Baseline      *float64       `json:"baseline"`
	DeltaAbs      *float64       `json:"delta_abs"`
	Direction     TrendDirection `json:"direction"`
	LowerIsBetter bool           `json:"lower_is_better"`
	Series        []*float64     `json:"series"`
}

// ProgressReport is the full deterministic read served at GET /api/progress.
type ProgressReport struct {
	Weeks       int            `json:"weeks"`
	GeneratedAt string         `json:"generated_at"`
	Signals     []TrendSummary `json:"signals"`
	EnoughData  bool           `json:"enough_data"`
}
```

Field-name + json-tag + TS table (copy verbatim into Go, the DTO, and TS):

| Go field | json tag | TS field | type |
|---|---|---|---|
| `Key` | `key` | `key` | string |
| `Label` | `label` | `label` | string |
| `Unit` | `unit` | `unit` | string |
| `Current` | `current` | `current` | number\|null |
| `Baseline` | `baseline` | `baseline` | number\|null |
| `DeltaAbs` | `delta_abs` | `delta_abs` | number\|null |
| `Direction` | `direction` | `direction` | `'up'\|'down'\|'flat'` |
| `LowerIsBetter` | `lower_is_better` | `lower_is_better` | boolean |
| `Series` | `series` | `series` | `(number\|null)[]` |
| `Weeks` | `weeks` | `weeks` | number |
| `GeneratedAt` | `generated_at` | `generated_at` | string |
| `Signals` | `signals` | `signals` | `TrendSummary[]` |
| `EnoughData` | `enough_data` | `enough_data` | boolean |

`direction` is the raw **value** movement (up = value increased). The app/fallback layer maps direction + `lower_is_better` to improving/worsening (for pace/RHR a `down` direction is an improvement).

Entry point + constants:
```go
const (
	DefaultWeeks         = 12
	MinWeeks             = 4
	MaxWeeks             = 52
	enoughDataMinSignals = 2 // >= this many FITNESS signals (weekly_load excluded) with >= 2 non-nil weekly points
)
```

Reference-HR band (pace-at-fixed-HR) — exact algorithm:
```go
const (
	// refHRBandBpm is the ± window around the reference HR (spec §7: ±5 bpm).
	refHRBandBpm = 5.0
	// defaultRefHRBpm is the fallback reference HR when profile.Zone2CeilingBpm
	// is nil (documented constant per spec §7).
	defaultRefHRBpm = 145.0
)
```
- Reference HR = `float64(*profile.Zone2CeilingBpm)` when non-nil, else `defaultRefHRBpm`. Band = `[ref-refHRBandBpm, ref+refHRBandBpm]`.
- Per weekly bucket: collect `secPerKm = float64(a.MovingTimeS) / (a.DistanceM/1000.0)` for runs (`IsRun(a.Type)`, `a.DistanceM>0`, `a.MovingTimeS>0`) whose `a.AvgHR != nil` and `*a.AvgHR` in-band and whose `ParseStart(a.StartTime)` falls in the bucket. `Median(secPerKm)` → that week's `pace_at_hr` value (sec/km, lower is better). No in-band run → `nil` (gap).
- `Current`/`Baseline`/`DeltaAbs` from last/first non-nil series entries. Direction deadbands: `paceEps = 0.5` (sec/km) for pace; `relDeadband = 0.03` (relative) for others.
- Reuse exported `metrics.IsRun`, `metrics.ParseStart`, `metrics.Median`, `metrics.FormatPace`.
- `weekly_load` series reuses M1 windowed-distance logic per bucket (zero IS data → `0.0`, not a gap). `resting_hr`/`hrv_baseline` = per-week mean of in-bucket non-nil recovery values (`RhrFields.RestingHR`, `HrvFields.LastNightAvgMs`). `vo2max` = latest non-nil reading within each bucket. `EnoughData` = ≥ `enoughDataMinSignals` of the FOUR fitness signals (pace_at_hr, vo2max, resting_hr, hrv_baseline) with ≥2 non-nil weekly points — `weekly_load` is CONTEXT (always non-nil) and is EXCLUDED from the count.

### Analyze prompt + result + fallback

`api.Deps` gains a `Progress` field (do NOT extend `Coach`). Interface in `router.go`:
```go
// Progress is the M3.1 progress-engine seam. Injected from main.go.
type Progress interface {
	Report(ctx context.Context, weeks int) (progress.ProgressReport, error)
	Analyze(ctx context.Context, weeks int) (progress.ProgressRead, error)
}
```

`claude -p` argv (mirror `dailyAdjustArgs`):
```go
func (e *Engine) analyzeArgs() []string {
	return []string{
		"-p", progressReadPrompt,
		"--model", e.model,
		"--output-format", "json",
		"--allowedTools", "",
		"--no-session-persistence",
	}
}
```

Input piped on stdin (snake_case):
```go
// ProgressReadInput is the JSON piped to claude -p stdin for the progress read.
type ProgressReadInput struct {
	Weeks    int            `json:"weeks"`
	Signals  []TrendSummary `json:"signals"`
	GoalText string         `json:"goal_text"`
}
```

Prompt (`progressReadPrompt`):
```
You are a CrossFit-aware running coach giving a short progress read. You receive a JSON
context on stdin: the number of weeks in the window, the athlete's goal text, and an array
of computed trend signals. Each signal has: key, label, unit, current, baseline, delta_abs,
direction (up|down|flat), lower_is_better, and a weekly series (nulls are weeks with no data).

The athlete is training to improve aerobic capacity for RX CrossFit, NOT to race. Interpret
each signal correctly: for pace_at_hr and resting_hr, LOWER is better (a falling value is
improvement); for vo2max and hrv_baseline, HIGHER is better; weekly_load is context, not a
fitness verdict. Faster pace at the same heart rate = a stronger engine.

Write 2-4 sentences, plain text (NO markdown, NO bullet list), that tell the athlete whether
their engine is improving and which signal is the clearest evidence. Be concrete (cite a
number or two). If the data is thin, say so honestly.

Output ONLY a single JSON object (no prose outside it, no markdown fences) of this EXACT shape:
{"text": "..."}
```

Result + fallback (M2 `source = ai|fallback` pattern):
```go
// ProgressRead is the analyze result (mirrors M2 TodayBriefing.source semantics).
type ProgressRead struct {
	Text   string `json:"text"`
	Source string `json:"source"` // "ai" | "fallback"
}
```
`Analyze` mirrors `coach.AdjustToday`: any non-nil `llm.Call` error → log + deterministic fallback with `Source:"fallback"`; success → `Source:"ai"`. Setup errors (Report/profile) returned as `error`.

Deterministic fallback (`fallbackProgressText(rep ProgressReport) string`): pure, no LLM. If `!rep.EnoughData` → `"Not enough history yet to read a trend — keep logging runs and syncing Garmin."`. Else, for each signal with non-nil `Current`+`Baseline`, emit `"<Label> <improved|worsened|held> (<baseline><unit> → <current><unit>)"` (improved/worsened respects `LowerIsBetter` vs `Direction`); join with `; `; prefix `"Over the last N weeks: "`. Pace formatted via `metrics.FormatPace` when unit is `s/km`.

### REST API

Routes inside the bearer-protected `r.Group`, after `r.Post("/api/agent/run", h.agentRun)`:
```go
		// M3.1
		r.Get("/api/progress", h.progress)
		r.Post("/api/progress/analyze", h.analyzeProgress)
```

`GET /api/progress?weeks=12` — bearer auth; `weeks` clamped via `clampQuery(r, "weeks", progress.DefaultWeeks, progress.MinWeeks, progress.MaxWeeks)` (default 12, range [4,52]); instant (no Claude); serializes `progress.ProgressReport` directly:
```go
func (h *handlers) progress(w http.ResponseWriter, r *http.Request) {
	weeks := clampQuery(r, "weeks", progress.DefaultWeeks, progress.MinWeeks, progress.MaxWeeks)
	rep, err := h.d.Progress.Report(r.Context(), weeks)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
```

`POST /api/progress/analyze` — bearer auth; optional body `{"weeks": 12}` (absent/out-of-range → default 12); returns `ProgressRead`:
```go
type analyzeProgressRequest struct {
	Weeks int `json:"weeks"`
}
func (h *handlers) analyzeProgress(w http.ResponseWriter, r *http.Request) {
	var req analyzeProgressRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // empty body OK
	weeks := req.Weeks
	if weeks < progress.MinWeeks || weeks > progress.MaxWeeks {
		weeks = progress.DefaultWeeks
	}
	read, err := h.d.Progress.Analyze(r.Context(), weeks)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, read)
}
```

Example `GET /api/progress` response (`series` oldest-first, `null` = gap):
```json
{
  "weeks": 12,
  "generated_at": "2026-06-21T07:00:00Z",
  "enough_data": true,
  "signals": [
    {"key": "pace_at_hr", "label": "Pace @ Z2 HR", "unit": "s/km", "current": 330.0, "baseline": 350.0, "delta_abs": -20.0, "direction": "down", "lower_is_better": true, "series": [350.0, null, 345.0, 342.0, null, 340.0, 338.0, 336.0, null, 334.0, 332.0, 330.0]},
    {"key": "vo2max", "label": "VO2max", "unit": "ml/kg/min", "current": 52.0, "baseline": 50.0, "delta_abs": 2.0, "direction": "up", "lower_is_better": false, "series": [50.0, 50.0, 51.0, null, 51.0, 51.0, 51.0, 52.0, null, 52.0, 52.0, 52.0]},
    {"key": "resting_hr", "label": "Resting HR", "unit": "bpm", "current": 47.0, "baseline": 50.0, "delta_abs": -3.0, "direction": "down", "lower_is_better": true, "series": [50.0, 49.0, 49.0, 48.0, 48.0, 48.0, 48.0, 47.0, 47.0, 47.0, 47.0, 47.0]},
    {"key": "hrv_baseline", "label": "HRV baseline", "unit": "ms", "current": 52.0, "baseline": 46.0, "delta_abs": 6.0, "direction": "up", "lower_is_better": false, "series": [46.0, 47.0, 47.0, 48.0, 49.0, 49.0, 50.0, 50.0, 51.0, 51.0, 52.0, 52.0]},
    {"key": "weekly_load", "label": "Weekly volume", "unit": "km", "current": 42.0, "baseline": 30.0, "delta_abs": 12.0, "direction": "up", "lower_is_better": false, "series": [30.0, 32.0, 34.0, 28.0, 36.0, 38.0, 40.0, 30.0, 41.0, 42.0, 44.0, 42.0]}
  ]
}
```
Empty-state: `{ "weeks": 12, "generated_at": "...", "enough_data": false, "signals": [] }`.
`POST /api/progress/analyze` response: `{ "text": "...", "source": "ai" }` (fallback: same shape, `"source": "fallback"`).

### App — TS types, hooks, sparkline

`app/src/api/types.ts` (snake_case, mirror Go DTO):
```ts
// --- M3.1 progress types (snake_case wire JSON) ---

export type TrendDirection = 'up' | 'down' | 'flat';

export interface TrendSummary {
  key: string;            // 'pace_at_hr' | 'vo2max' | 'resting_hr' | 'hrv_baseline' | 'weekly_load'
  label: string;
  unit: string;           // 's/km' | 'ml/kg/min' | 'bpm' | 'ms' | 'km'
  current: number | null;
  baseline: number | null;
  delta_abs: number | null;
  direction: TrendDirection;
  lower_is_better: boolean;
  series: (number | null)[]; // len == weeks; oldest-first; null = gap
}

export interface ProgressReport {
  weeks: number;
  generated_at: string;
  enough_data: boolean;
  signals: TrendSummary[];
}

export interface ProgressRead {
  text: string;
  source: 'ai' | 'fallback';
}
```

`app/src/api/hooks.ts`:
```ts
export function useProgress(weeks = 12) {
  return useQuery({
    queryKey: ['progress', weeks],
    queryFn: () => apiGet<ProgressReport>(`/api/progress?weeks=${weeks}`),
  });
}

export function useAnalyzeProgress() {
  return useMutation({
    mutationFn: (body: { weeks?: number }) =>
      apiPost<ProgressRead>('/api/progress/analyze', body),
  });
}
```
Add `ProgressReport`, `ProgressRead` to the `import type {...}` list. No `onSuccess`/invalidation (read shown from `mutation.data`).

`app/src/lib/sparkline.ts` — full 7-level ramp `▁▂▃▄▅▆▇`:
```ts
// sparkline(series): render a numeric series as a unicode-block sparkline.
// null/undefined/NaN -> a blank space (gap, never a fabricated point).
// Output string length === series.length. Flat series -> all mid-level blocks.
export function sparkline(series: (number | null | undefined)[]): string
```
Behavior: `sparkline([1,2,3])` → `"▁▄▇"` (`▁▄▇`); `sparkline([5,null,5])` → `"▄ ▄"`; `sparkline([])` → `""`; `sparkline([null,null])` → `"  "`. Render in monospace.

Canonical testIDs: per-card `progress-card-<key>`, sparkline `progress-spark-<key>`, analyze button `btn-analyze-progress`, coach read `progress-read`, empty state `progress-empty` (plus additive `progress-arrow-<key>`, `progress-read-source`).

### Cross-cutting invariants
- All M3.1 wire JSON is snake_case.
- `api` imports `progress` only for `progress.ProgressReport`/`progress.ProgressRead` types in the `Progress` interface; the concrete `progress.Engine` is injected from `main.go` (mirrors the existing `metrics` import for `Coach`).
- `series` is always exactly `weeks` long, oldest-first, `null` = gap (never interpolated). Go `[]*float64` `nil` → JSON `null`.
- VO2max never breaks other signals: a missing/empty `garmin_vo2max` read → all-`nil` series → "—".
- `Progress.Analyze` returns `error` only for store/compute setup failures; every `llm.Call` failure → `{source:"fallback"}`.
- The progress engine is pure (`ComputeProgress`); only `Engine.Report`/`Engine.Analyze` touch store/llm.

---

## Tasks

> **Execution order (global):** Go ingestion lane `1 → 2 → 9 → 10`; worker lane `3 → 4 → 5 → 6` (4 must precede 5/6); both lanes feed the engine. Engine: `7 (metrics export) → 8 (types) → 11 (bucketing) → 12 (series) → 13 (ComputeProgress) → 14 (prompts) → 15 (engine)`. API/wiring: `16 (seam) → 17 (handlers) → 18 (main wiring)`. App last: `19 → 20 → 21 → 22`. Engine Tasks 13/15 require `store.Vo2maxPoint`/`ListVo2max` (Task 2) merged first; API Task 17 requires the engine types (Task 8) and Task 15. Tasks 1–2 and 3–6 may run in parallel lanes.

> **Locating edit sites:** Find every edit site by the TEXTUAL ANCHOR each task gives (e.g. "after the named function", "after the `rhr` loop", "the `set(obj.keys())` assert"), NOT by the literal line numbers, which may have drifted from the real files.

### Task 1: Migration `00005_m3_1_vo2max.sql` + table-check test

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/store/migrations/00005_m3_1_vo2max.sql`
- Modify: `/home/jake/project/help-my-run/backend/internal/store/store_test.go` (`TestOpenAndMigrate`, `wantTables` slice)

- [ ] **Step 1: Add `garmin_vo2max` to the `wantTables` assertion (failing test).**
In `/home/jake/project/help-my-run/backend/internal/store/store_test.go`, edit the `wantTables` slice inside `TestOpenAndMigrate` (currently lines 27-31):
```go
	wantTables := []string{
		"strava_tokens", "activities", "activity_splits",
		"garmin_sleep", "garmin_hrv", "garmin_body_battery", "garmin_rhr",
		"garmin_vo2max",
		"sync_log",
	}
```

- [ ] **Step 2: Run — expect FAIL.**
Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestOpenAndMigrate`
Expected output (FAIL): `--- FAIL: TestOpenAndMigrate` with `table "garmin_vo2max" not found after migrate: sql: no rows in result set`.

- [ ] **Step 3: Create the migration (minimal impl).**
Create `/home/jake/project/help-my-run/backend/internal/store/migrations/00005_m3_1_vo2max.sql` (mirrors `garmin_rhr` DDL in `00001_init.sql`; value column `REAL` per spec §5; goose annotations identical to existing files; no seed rows; `migrate.go` auto-embeds via `//go:embed migrations/*.sql`, no code change):
```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE garmin_vo2max (
    date     TEXT PRIMARY KEY,
    vo2max   REAL,
    raw_json TEXT NOT NULL
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE garmin_vo2max;
-- +goose StatementEnd
```

- [ ] **Step 4: Run — expect PASS.**
Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestOpenAndMigrate`
Expected output (PASS): `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 5: Commit.**
```
cd /home/jake/project/help-my-run/backend && git add internal/store/migrations/00005_m3_1_vo2max.sql internal/store/store_test.go && git commit -m "feat(store): add garmin_vo2max migration (M3.1 INGEST)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Store `Vo2maxRow`/`UpsertVo2max` + `Vo2maxPoint`/`ListVo2max`

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/store/garmin.go` (add struct + upsert near `RhrRow`/`UpsertRhr`; add getter modeled on `activities.go` `ListActivities`' `ORDER BY ... LIMIT ?`)
- Create: `/home/jake/project/help-my-run/backend/internal/store/vo2max_test.go`

- [ ] **Step 1: Write the round-trip + idempotency + ordering test (failing test).**
Create `/home/jake/project/help-my-run/backend/internal/store/vo2max_test.go` (uses in-package helpers `newTestStore`, `f64p` from `store_test.go`):
```go
package store

import "testing"

func TestUpsertVo2maxAndListVo2max(t *testing.T) {
	s := newTestStore(t)

	// Insert three days out of order; ListVo2max returns most-recent-first.
	if err := s.UpsertVo2max(Vo2maxRow{Date: "2026-06-16", Vo2max: f64p(51.0), RawJSON: `{"generic":{"vo2MaxValue":51.0}}`}); err != nil {
		t.Fatalf("UpsertVo2max 16: %v", err)
	}
	if err := s.UpsertVo2max(Vo2maxRow{Date: "2026-06-18", Vo2max: f64p(52.0), RawJSON: `{"generic":{"vo2MaxValue":52.0}}`}); err != nil {
		t.Fatalf("UpsertVo2max 18: %v", err)
	}
	// A null vo2max value is permitted (column is REAL / nullable).
	if err := s.UpsertVo2max(Vo2maxRow{Date: "2026-06-17", Vo2max: nil, RawJSON: `{"generic":null}`}); err != nil {
		t.Fatalf("UpsertVo2max 17 null: %v", err)
	}

	pts, err := s.ListVo2max(30)
	if err != nil {
		t.Fatalf("ListVo2max error = %v", err)
	}
	if len(pts) != 3 {
		t.Fatalf("ListVo2max len = %d, want 3", len(pts))
	}
	// Most-recent-first by date.
	if pts[0].Date != "2026-06-18" || pts[1].Date != "2026-06-17" || pts[2].Date != "2026-06-16" {
		t.Errorf("dates = [%s,%s,%s], want [18,17,16]", pts[0].Date, pts[1].Date, pts[2].Date)
	}
	if pts[0].Vo2max == nil || *pts[0].Vo2max != 52.0 {
		t.Errorf("06-18 vo2max = %v, want 52.0", pts[0].Vo2max)
	}
	if pts[1].Vo2max != nil {
		t.Errorf("06-17 vo2max = %v, want nil (stored null)", pts[1].Vo2max)
	}

	// LIMIT is honored.
	lim, err := s.ListVo2max(2)
	if err != nil {
		t.Fatalf("ListVo2max(2) error = %v", err)
	}
	if len(lim) != 2 || lim[0].Date != "2026-06-18" {
		t.Errorf("ListVo2max(2) = %+v, want 2 newest", lim)
	}

	// Re-upsert 06-18 with a new value -> update, not duplicate.
	if err := s.UpsertVo2max(Vo2maxRow{Date: "2026-06-18", Vo2max: f64p(53.0), RawJSON: `{"generic":{"vo2MaxValue":53.0}}`}); err != nil {
		t.Fatalf("re-UpsertVo2max: %v", err)
	}
	pts, _ = s.ListVo2max(30)
	if len(pts) != 3 {
		t.Fatalf("after re-upsert len = %d, want 3", len(pts))
	}
	if pts[0].Vo2max == nil || *pts[0].Vo2max != 53.0 {
		t.Errorf("06-18 after re-upsert = %v, want 53.0", pts[0].Vo2max)
	}
}
```

- [ ] **Step 2: Run — expect FAIL (compile error).**
Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestUpsertVo2maxAndListVo2max`
Expected output (FAIL): build error `undefined: Vo2maxRow`, `s.UpsertVo2max undefined`, `s.ListVo2max undefined`, `undefined: Vo2maxPoint`.

- [ ] **Step 3: Add the struct, upsert, point struct, and getter (minimal impl).**
In `/home/jake/project/help-my-run/backend/internal/store/garmin.go`, append after `UpsertRhr` (after line 125, before `CountRecoveryDays`):
```go
// Vo2maxRow maps to garmin_vo2max.
type Vo2maxRow struct {
	Date    string
	Vo2max  *float64
	RawJSON string
}

// UpsertVo2max upserts one garmin_vo2max row by date.
func (s *Store) UpsertVo2max(r Vo2maxRow) error {
	_, err := s.DB.Exec(`
		INSERT INTO garmin_vo2max (date, vo2max, raw_json)
		VALUES (?,?,?)
		ON CONFLICT(date) DO UPDATE SET
			vo2max=excluded.vo2max, raw_json=excluded.raw_json`,
		r.Date, r.Vo2max, r.RawJSON)
	return err
}

// Vo2maxPoint is one dated VO2max reading (vo2max may be nil when stored null).
type Vo2maxPoint struct {
	Date   string
	Vo2max *float64
}

// ListVo2max returns up to `limit` garmin_vo2max rows, most-recent-first by date.
func (s *Store) ListVo2max(limit int) ([]Vo2maxPoint, error) {
	rows, err := s.DB.Query(`
		SELECT date, vo2max FROM garmin_vo2max
		ORDER BY date DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Vo2maxPoint
	for rows.Next() {
		var p Vo2maxPoint
		if err := rows.Scan(&p.Date, &p.Vo2max); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run — expect PASS.**
Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/ -run TestUpsertVo2maxAndListVo2max`
Expected output (PASS): `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 5: Run full store package to confirm no regressions.**
Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/store/`
Expected output (PASS): `ok  	help-my-run/backend/internal/store`.

- [ ] **Step 6: Commit.**
```
cd /home/jake/project/help-my-run/backend && git add internal/store/garmin.go internal/store/vo2max_test.go && git commit -m "feat(store): add Vo2maxRow/UpsertVo2max and Vo2maxPoint/ListVo2max (M3.1 INGEST)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Worker `client.py` — `get_max_metrics` 1:1 delegation

**Files:**
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/client.py` (add to the "verified data methods (1:1 delegation)" block in `GarminClient`)
- Create: `/home/jake/project/help-my-run/garmin-worker/tests/test_client.py`

- [ ] **Step 1: Write the 1:1-delegation test (failing test).**
Create `/home/jake/project/help-my-run/garmin-worker/tests/test_client.py` (injects a fake underlying `Garmin`; asserts `get_max_metrics` delegates verbatim and passes `cdate` through):
```python
from garmin_worker.client import GarminClient


class _FakeGarmin:
    """Fake underlying garminconnect.Garmin recording calls."""

    def __init__(self):
        self.calls = []

    def get_max_metrics(self, cdate):
        self.calls.append(("get_max_metrics", cdate))
        return {"userId": 1, "generic": {"calendarDate": cdate, "vo2MaxValue": 44.0}, "cycling": None}


def test_get_max_metrics_delegates_1to1():
    fake = _FakeGarmin()
    c = GarminClient(fake)
    out = c.get_max_metrics("2026-06-15")
    assert fake.calls == [("get_max_metrics", "2026-06-15")]
    assert out["generic"]["vo2MaxValue"] == 44.0
```

- [ ] **Step 2: Run — expect FAIL.**
Command: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests/test_client.py -q`
Expected output (FAIL): `AttributeError: 'GarminClient' object has no attribute 'get_max_metrics'`.

- [ ] **Step 3: Add the delegation method (minimal impl).**
In `/home/jake/project/help-my-run/garmin-worker/garmin_worker/client.py`, in the "verified data methods (1:1 delegation)" block, append after `get_stats` (after line 96):
```python
    def get_max_metrics(self, cdate: str) -> dict:
        return self._g.get_max_metrics(cdate)
```

- [ ] **Step 4: Run — expect PASS.**
Command: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests/test_client.py -q`
Expected output (PASS): `1 passed`.

- [ ] **Step 5: Commit.**
```
git add garmin-worker/garmin_worker/client.py garmin-worker/tests/test_client.py && git commit -m "feat(worker): add get_max_metrics delegation to GarminClient (M3.1 INGEST)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Worker `normalize.py` — `normalize_vo2max_day` + `build_output` vo2max key

**Files:**
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/normalize.py` (add `normalize_vo2max_day` after `normalize_rhr_day`; add `vo2max` kwarg + last key to `build_output`)
- Create: `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_max_metrics_2026-06-15.json`
- Modify: `/home/jake/project/help-my-run/garmin-worker/tests/test_normalize.py` (add vo2max normalize tests; update both `build_output` key-order asserts)

- [ ] **Step 1: Create the normalize fixture.**
Create `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/raw_max_metrics_2026-06-15.json`:
```json
{"userId": 1, "generic": {"calendarDate": "2026-06-15", "vo2MaxValue": 52.0, "fitnessAge": 30, "fitnessAgeDescription": 0, "maxMetCategory": 0}, "cycling": null}
```

- [ ] **Step 2: Add the normalize tests + update `build_output` key-order asserts (failing test).**
In `/home/jake/project/help-my-run/garmin-worker/tests/test_normalize.py`, append after the `test_normalize_rhr_day_none_raw_yields_null` function:
```python
# --------------------------------------------------------------------------
# normalize_vo2max_day
# get_max_metrics(date) hits the maxmet daily-range endpoint, which returns a
# one-element LIST: raw[0]["generic"]["vo2MaxValue"] (a plain dict is also
# tolerated for fixtures). raw_json preserves the ORIGINAL payload.
# --------------------------------------------------------------------------
def test_normalize_vo2max_day_dict_shape():
    # Dict-shaped fixture (defensive: type hint says dict).
    raw = load("raw_max_metrics_2026-06-15.json")
    out = normalize.normalize_vo2max_day("2026-06-15", raw)
    assert out == {
        "date": "2026-06-15",
        "vo2max": 52.0,
        "raw_json": raw,
    }
    assert list(out.keys()) == ["date", "vo2max", "raw_json"]


def test_normalize_vo2max_day_list_shape_real_endpoint():
    # Real endpoint shape: a one-element list; raw_json keeps the list intact.
    raw = [{"generic": {"calendarDate": "2026-06-15", "vo2MaxValue": 52.0}, "cycling": None}]
    out = normalize.normalize_vo2max_day("2026-06-15", raw)
    assert out["vo2max"] == 52.0  # same value as the dict-shaped case
    assert out["raw_json"] == raw  # original list payload preserved


def test_normalize_vo2max_day_empty_list_yields_null():
    out = normalize.normalize_vo2max_day("2026-06-15", [])
    assert out["vo2max"] is None
    assert out["raw_json"] == []  # original empty list preserved


def test_normalize_vo2max_day_missing_generic_null():
    raw = {"userId": 1, "generic": None, "cycling": None}
    out = normalize.normalize_vo2max_day("2026-06-15", raw)
    assert out["vo2max"] is None
    assert out["raw_json"] == raw


def test_normalize_vo2max_day_none_raw_yields_empty_dict():
    out = normalize.normalize_vo2max_day("2026-06-15", None)
    assert out["vo2max"] is None
    assert out["raw_json"] == {}
```
Then update `test_build_output_top_level_shape` — add `vo2max=[]` to the call and `"vo2max"` (LAST) to the expected key list (lines 173-186):
```python
def test_build_output_top_level_shape():
    out = normalize.build_output(
        since="2026-06-14",
        until="2026-06-15",
        fetched_at="2026-06-15T05:00:12Z",
        sleep=[{"date": "2026-06-14"}],
        hrv=[],
        body_battery=[{"date": "2026-06-14"}, {"date": "2026-06-15"}],
        rhr=[{"date": "2026-06-15"}],
        vo2max=[{"date": "2026-06-15"}],
    )
    assert list(out.keys()) == [
        "since", "until", "fetched_at",
        "sleep", "hrv", "body_battery", "rhr", "vo2max",
    ]
    assert out["since"] == "2026-06-14"
    assert out["until"] == "2026-06-15"
    assert out["fetched_at"] == "2026-06-15T05:00:12Z"
    assert out["hrv"] == []
    assert len(out["body_battery"]) == 2
    assert out["sleep"][0]["date"] == "2026-06-14"
    assert out["rhr"][0]["date"] == "2026-06-15"
    assert out["vo2max"][0]["date"] == "2026-06-15"
```
And update `test_build_output_full_serializes_to_json` to pass the new kwarg (lines 196-205):
```python
def test_build_output_full_serializes_to_json():
    out = normalize.build_output(
        since="2026-06-15", until="2026-06-15",
        fetched_at="2026-06-15T05:00:12Z",
        sleep=[], hrv=[], body_battery=[], rhr=[], vo2max=[],
    )
    # must be JSON-serializable (no datetime / non-primitive leaks)
    text = json.dumps(out)
    again = json.loads(text)
    assert again["since"] == "2026-06-15"
```

- [ ] **Step 3: Run — expect FAIL.**
Command: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests/test_normalize.py -q`
Expected output (FAIL): `AttributeError: module 'garmin_worker.normalize' has no attribute 'normalize_vo2max_day'` and `TypeError: build_output() got an unexpected keyword argument 'vo2max'`.

- [ ] **Step 4: Add `normalize_vo2max_day` + the `vo2max` kwarg/key (minimal impl).**
In `/home/jake/project/help-my-run/garmin-worker/garmin_worker/normalize.py`, insert after the `normalize_rhr_day` function (immediately before `build_output`):
```python
def normalize_vo2max_day(date: str, raw) -> dict:
    """Map get_max_metrics(date) -> Vo2maxDay (CONTRACTS §2.2).

    get_max_metrics hits the maxmet DAILY RANGE endpoint
    (`/{cdate}/{cdate}`) and returns the raw JSON with no transform. On
    the real endpoint this is a one-element LIST whose `[0].generic`
    holds the metric (the library's `dict` type hint is wrong); fixtures
    may also be a plain dict. Unwrap the list first, then walk
    `generic.vo2MaxValue`. The ORIGINAL payload (list or dict) is
    preserved in `raw_json`.
    """
    payload = raw
    if isinstance(payload, list):
        payload = payload[0] if payload else None
    val = _get(payload, "generic", "vo2MaxValue")
    return {
        "date": date,
        "vo2max": val,
        "raw_json": raw if raw is not None else {},
    }
```
Then change the `build_output` signature and returned dict literal (lines 105-127) to add `vo2max` LAST:
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

- [ ] **Step 5: Run — expect PASS.**
Command: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests/test_normalize.py -q`
Expected output (PASS): `... passed` (no failures).

- [ ] **Step 6: Commit.**
```
cd /home/jake/project/help-my-run && git add garmin-worker/garmin_worker/normalize.py garmin-worker/tests/test_normalize.py garmin-worker/tests/fixtures/raw_max_metrics_2026-06-15.json && git commit -m "feat(worker): add normalize_vo2max_day and vo2max key to build_output (M3.1 INGEST)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Worker `fetcher.py` — per-day vo2max loop (omit null days)

**Files:**
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/fetcher.py` (`run_fetch`: init `vo2max=[]`, append in per-day loop after `rhr.append(...)` gating on the normalized result, pass `vo2max=vo2max` to `build_output`)
- Modify: `/home/jake/project/help-my-run/garmin-worker/tests/test_fetcher.py` (extend `_MockClient` with `get_max_metrics`; update key-order asserts; add vo2max-array + omit-null tests)

- [ ] **Step 1: Extend the mock + update asserts + add omit-null test (failing test).**
In `/home/jake/project/help-my-run/garmin-worker/tests/test_fetcher.py`, extend `_MockClient.__init__` to accept a `vo2max_map` and add a `get_max_metrics` method. Edit the `__init__` (lines 9-12):
```python
    def __init__(self, hrv_map=None, vo2max_map=None, raise_on=None):
        self.calls = []
        self._hrv_map = hrv_map or {}
        self._vo2max_map = vo2max_map or {}
        self._raise_on = raise_on  # (method_name, exception) to raise
```
And add the method after `get_stats`. Mirror the REAL endpoint shape: `get_max_metrics` returns the maxmet daily-range payload, a one-element LIST (`[{...}]`), so the default returns a list:
```python
    def get_max_metrics(self, cdate):
        self.calls.append(("vo2max", cdate))
        self._maybe_raise("get_max_metrics")
        # Default: every day has a value unless vo2max_map overrides.
        # Real endpoint returns a one-element list, not a top-level dict.
        if cdate in self._vo2max_map:
            return self._vo2max_map[cdate]
        return [{"generic": {"calendarDate": cdate, "vo2MaxValue": 50.0}, "cycling": None}]
```
Update the `list(out.keys())` assert (lines 52-54 in `test_run_fetch_top_level_shape_and_echo`) to end with `"vo2max"`:
```python
    assert list(out.keys()) == [
        "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr", "vo2max",
    ]
```
Append two new tests at the end of the file (after `test_run_fetch_output_is_json_serializable`, line 128):
```python
def test_run_fetch_appends_vo2max_per_day():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert [v["date"] for v in out["vo2max"]] == ["2026-06-14", "2026-06-15"]
    assert out["vo2max"][0]["vo2max"] == 50.0


def test_run_fetch_omits_vo2max_null_days():
    # 06-14 returns an empty list (no data) -> omitted; 06-15 has a value.
    # Both use the real list-shaped endpoint payload.
    mc = _MockClient(vo2max_map={
        "2026-06-14": [],
        "2026-06-15": [{"generic": {"calendarDate": "2026-06-15", "vo2MaxValue": 52.0}, "cycling": None}],
    })
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert [v["date"] for v in out["vo2max"]] == ["2026-06-15"]  # 06-14 omitted (no value)
    assert out["vo2max"][0]["vo2max"] == 52.0
```

- [ ] **Step 2: Run — expect FAIL.**
Command: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests/test_fetcher.py -q`
Expected output (FAIL): `KeyError: 'vo2max'` / assertion on `list(out.keys())` not ending in `"vo2max"` (and `build_output()` missing the `vo2max` arg until the impl threads it through).

- [ ] **Step 3: Thread vo2max through `run_fetch` (minimal impl).**
In `/home/jake/project/help-my-run/garmin-worker/garmin_worker/fetcher.py`, no new import is needed — the existing `from . import normalize` already exposes `normalize_vo2max_day`, which does the list-unwrap. Init `vo2max = []` next to `sleep/hrv/rhr` (after the `rhr = []` line):
```python
    sleep = []
    hrv = []
    rhr = []
    vo2max = []
```
Append inside the per-day loop after the `rhr.append(...)` line, omitting no-data days. `get_max_metrics` returns the maxmet daily-range payload, a one-element LIST on the real endpoint (dict tolerated for fixtures) — so let `normalize_vo2max_day` do the unwrap and gate on its result rather than calling `_get` on the raw list (a bare `_get(list, ...)` is always `None`, which would silently drop every day):
```python
        rhr.append(normalize.normalize_rhr_day(cdate, client.get_stats(cdate)))

        mm = normalize.normalize_vo2max_day(cdate, client.get_max_metrics(cdate))
        if mm["vo2max"] is not None:  # omit no-data days
            vo2max.append(mm)
```
Pass `vo2max=vo2max` to `build_output` (the final return, lines 69-77):
```python
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

- [ ] **Step 4: Run — expect PASS.**
Command: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests/test_fetcher.py -q`
Expected output (PASS): `... passed` (no failures).

- [ ] **Step 5: Commit.**
```
git add garmin-worker/garmin_worker/fetcher.py garmin-worker/tests/test_fetcher.py && git commit -m "feat(worker): fetch per-day VO2max in run_fetch, omit null days (M3.1 INGEST)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Worker `cli.py` — `--dry-run` vo2max parity + golden fixture

**Files:**
- Modify: `/home/jake/project/help-my-run/garmin-worker/garmin_worker/cli.py` (add `_DRY_VO2MAX_RAW`; build `vo2max` in `_run_dry_fetch`; pass `vo2max=vo2max` to `build_output`)
- Modify: `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/dry_run_expected.json` (append `vo2max` key LAST)
- Modify: `/home/jake/project/help-my-run/garmin-worker/tests/test_cli.py` (update the `set(obj.keys())` assert to include `"vo2max"`)

- [ ] **Step 1: Update the golden fixture + key-set assert (failing test).**
Append a `vo2max` key (LAST) to `/home/jake/project/help-my-run/garmin-worker/tests/fixtures/dry_run_expected.json` — change the closing of the `rhr` array (currently lines 16-20) so the object gains the new key:
```json
  "rhr": [
    {"date": "2026-06-14", "resting_hr": 48, "raw_json": {"restingHeartRate": 48, "totalSteps": 9000}},
    {"date": "2026-06-15", "resting_hr": 47, "raw_json": {"restingHeartRate": 47, "totalSteps": 11000}}
  ],
  "vo2max": [
    {"date": "2026-06-14", "vo2max": 51.0, "raw_json": {"userId": 1, "generic": {"calendarDate": "2026-06-14", "vo2MaxValue": 51.0, "fitnessAge": 30}, "cycling": null}},
    {"date": "2026-06-15", "vo2max": 52.0, "raw_json": {"userId": 1, "generic": {"calendarDate": "2026-06-15", "vo2MaxValue": 52.0, "fitnessAge": 30}, "cycling": null}}
  ]
}
```
In `/home/jake/project/help-my-run/garmin-worker/tests/test_cli.py`, update the key-set assert in `test_main_dry_run_stdout_is_single_json_object` (lines 92-94) to include `"vo2max"`:
```python
    assert set(obj.keys()) == {
        "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr", "vo2max",
    }
```

- [ ] **Step 2: Run — expect FAIL.**
Command: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests/test_cli.py -q`
Expected output (FAIL): `test_main_dry_run_prints_contract_json` — `assert out == expected` mismatch (expected has `vo2max`, actual lacks it); `test_main_dry_run_stdout_is_single_json_object` — `set(obj.keys())` missing `"vo2max"`.

- [ ] **Step 3: Add `_DRY_VO2MAX_RAW` and wire `_run_dry_fetch` (minimal impl).**
In `/home/jake/project/help-my-run/garmin-worker/garmin_worker/cli.py`, add the synthetic raw after `_DRY_STATS_RAW` (after line 83):
```python
_DRY_VO2MAX_RAW = {
    "2026-06-14": {"userId": 1, "generic": {"calendarDate": "2026-06-14", "vo2MaxValue": 51.0, "fitnessAge": 30}, "cycling": None},
    "2026-06-15": {"userId": 1, "generic": {"calendarDate": "2026-06-15", "vo2MaxValue": 52.0, "fitnessAge": 30}, "cycling": None},
}
```
In `_run_dry_fetch` (lines 86-100), build `vo2max` and pass it to `build_output`:
```python
def _run_dry_fetch(since: str, until: str) -> dict:
    """Build the §2.1 object from baked-in synthetic data (no Garmin)."""
    sleep = [normalize.normalize_sleep_day(d, raw) for d, raw in sorted(_DRY_SLEEP_RAW.items())]
    hrv = [normalize.normalize_hrv_day(d, raw) for d, raw in sorted(_DRY_HRV_RAW.items())]
    body_battery = [normalize.normalize_body_battery_day(e["date"], e) for e in _DRY_BB_RANGE]
    rhr = [normalize.normalize_rhr_day(d, raw) for d, raw in sorted(_DRY_STATS_RAW.items())]
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

- [ ] **Step 4: Run — expect PASS.**
Command: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests/test_cli.py -q`
Expected output (PASS): `... passed` (no failures).

- [ ] **Step 5: Run the full worker suite to confirm no regressions.**
Command: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests -q`
Expected output (PASS): all tests pass (`... passed`).

- [ ] **Step 6: Commit.**
```
cd /home/jake/project/help-my-run && git add garmin-worker/garmin_worker/cli.py garmin-worker/tests/test_cli.py garmin-worker/tests/fixtures/dry_run_expected.json && git commit -m "feat(worker): wire VO2max into --dry-run + golden fixture (M3.1 INGEST)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Export metrics helpers for reuse (IsRun, ParseStart, Median, FormatPace)

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/metrics/metrics.go` (rename unexported `isRun`,`parseStart`,`median`,`formatPace` → exported, keep internal call sites working via thin aliases)
- Modify: `/home/jake/project/help-my-run/backend/internal/metrics/metrics_test.go`

Rationale: the `progress` package reuses these pure helpers (CONTRACTS §3.3). Export them; keep lowercase aliases so the rest of `metrics.go` and its tests compile unchanged.

- [ ] **Step 1: Write failing test for exported names.** Append to `metrics_test.go`:
```go
func TestExportedHelpersForProgress(t *testing.T) {
	if !IsRun("Run") || IsRun("Ride") {
		t.Errorf("IsRun broken: Run=%v Ride=%v", IsRun("Run"), IsRun("Ride"))
	}
	tm, ok := ParseStart("2026-06-21T07:00:00Z")
	if !ok || tm.Year() != 2026 {
		t.Errorf("ParseStart = %v ok=%v", tm, ok)
	}
	if got := Median([]float64{1, 2, 3}); got != 2 {
		t.Errorf("Median([1,2,3]) = %v, want 2", got)
	}
	if got := FormatPace(360); got != "6:00/km" {
		t.Errorf("FormatPace(360) = %q, want 6:00/km", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL (undefined).**
`cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`
Expected: `./metrics_test.go:...: undefined: IsRun` (and `ParseStart`, `Median`, `FormatPace`) — build fails.

- [ ] **Step 3: Minimal impl — export the four helpers, keep internal aliases.** In `metrics.go` rename the four definitions to exported and add lowercase aliases so existing internal callers compile:
- `func formatPace(secPerKm float64) string {` → `func FormatPace(secPerKm float64) string {`
- `func isRun(typ string) bool { return runTypes[typ] }` → `func IsRun(typ string) bool { return runTypes[typ] }`
- `func parseStart(startTime string) (time.Time, bool) {` → `func ParseStart(startTime string) (time.Time, bool) {`
- `func median(sorted []float64) float64 {` → `func Median(sorted []float64) float64 {`
- Immediately after the `runTypes` var block add aliases:
```go
// Internal aliases so existing call sites in this package stay unchanged while
// the exported names are reused by internal/progress (CONTRACTS §3.3).
func formatPace(secPerKm float64) string          { return FormatPace(secPerKm) }
func isRun(typ string) bool                        { return IsRun(typ) }
func parseStart(s string) (time.Time, bool)        { return ParseStart(s) }
func median(sorted []float64) float64              { return Median(sorted) }
```
(Leave all existing internal call sites — `distanceKmInWindow`, `runPacesSecPerKm`, `paceEstimates` — untouched; they call the lowercase aliases.)

- [ ] **Step 4: Run — expect PASS.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/metrics/`
Expected: `ok  	help-my-run/backend/internal/metrics`

- [ ] **Step 5: Commit.**
`cd /home/jake/project/help-my-run/backend && git add internal/metrics/metrics.go internal/metrics/metrics_test.go && git commit -m "refactor(metrics): export IsRun, ParseStart, Median, FormatPace for progress reuse" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`

---

### Task 8: Go `garmin/types.go` — `Vo2maxDay` struct + `WorkerOutput.VO2Max` (+ runner fixture)

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/garmin/types.go` (add `VO2Max []Vo2maxDay` LAST to `WorkerOutput`; add `Vo2maxDay` struct mirroring `RhrDay`)
- Modify: `/home/jake/project/help-my-run/backend/internal/garmin/testdata/worker_output.json` (append `vo2max` array — also consumed by the sync test)
- Modify: `/home/jake/project/help-my-run/backend/internal/garmin/runner_test.go` (`TestRunGarminFetchParsesOutput`: assert the parsed `VO2Max`)

- [ ] **Step 1: Append `vo2max` to the runner fixture + add a parse assertion (failing test).**
Append a `vo2max` array to `/home/jake/project/help-my-run/backend/internal/garmin/testdata/worker_output.json` — change the close of the `rhr` array (currently lines 16-19) to add the new top-level key:
```json
  "rhr": [
    { "date": "2026-06-14", "resting_hr": 48, "raw_json": {"restingHeartRate": 48} },
    { "date": "2026-06-15", "resting_hr": 47, "raw_json": {"restingHeartRate": 47} }
  ],
  "vo2max": [
    { "date": "2026-06-14", "vo2max": 51.0, "raw_json": {"generic": {"vo2MaxValue": 51.0}} },
    { "date": "2026-06-15", "vo2max": 52.0, "raw_json": {"generic": {"vo2MaxValue": 52.0}} }
  ]
}
```
In `/home/jake/project/help-my-run/backend/internal/garmin/runner_test.go`, add a VO2max assertion in `TestRunGarminFetchParsesOutput` after the RHR check (after line 48, before the raw_json check):
```go
	if len(out.VO2Max) != 2 || out.VO2Max[1].VO2Max == nil || *out.VO2Max[1].VO2Max != 52.0 {
		t.Errorf("vo2max parse wrong: %+v", out.VO2Max)
	}
```

- [ ] **Step 2: Run — expect FAIL (compile error).**
Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/garmin/ -run TestRunGarminFetchParsesOutput`
Expected output (FAIL): build error `out.VO2Max undefined (type WorkerOutput has no field or method VO2Max)`.

- [ ] **Step 3: Add the struct + field (minimal impl).**
In `/home/jake/project/help-my-run/backend/internal/garmin/types.go`, add `VO2Max` as the LAST field of `WorkerOutput` (after line 15):
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
And append the new struct after `RhrDay` (after line 53):
```go
// Vo2maxDay is one per-day VO2max entry (running VO2max from get_max_metrics).
type Vo2maxDay struct {
	Date    string          `json:"date"`
	VO2Max  *float64        `json:"vo2max"`
	RawJSON json.RawMessage `json:"raw_json"`
}
```

- [ ] **Step 4: Run — expect PASS.**
Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/garmin/ -run TestRunGarminFetchParsesOutput`
Expected output (PASS): `ok  	help-my-run/backend/internal/garmin`.

- [ ] **Step 5: Commit.**
```
cd /home/jake/project/help-my-run/backend && git add internal/garmin/types.go internal/garmin/testdata/worker_output.json internal/garmin/runner_test.go && git commit -m "feat(garmin): add Vo2maxDay struct and WorkerOutput.VO2Max (M3.1 INGEST)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: `sync/sync.go` — VO2max upsert loop + 84-day (~12-week) backfill

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/sync/sync.go` (`SyncGarmin`: add `out.VO2Max` upsert loop after the `out.RHR` loop; bump `garminBackfillDays` 30 → 84; update doc comments)
- Modify: `/home/jake/project/help-my-run/backend/internal/sync/sync_test.go` (`TestSyncGarminUpsertsAllTables`: synced 7 → 9, add `garmin_vo2max` count = 2; add `TestSyncGarminBackfillWindowIs84Days` capturing `since`)

Depends on: Task 1 (migration), Task 2 (`store.UpsertVo2max`/`Vo2maxRow`), Task 8 (`out.VO2Max`).

> **Side-effect (flagged):** `garminBackfillDays` is SHARED, so bumping it 30 → 84 also widens the first-sync backfill window for the existing four recovery tables (`garmin_sleep`/`garmin_hrv`/`garmin_body_battery`/`garmin_rhr`), not just VO2max. This is first-sync-only (later syncs use `sync_log.last_synced_at`) and is acceptable/desirable — the deeper recovery history feeds the progress engine — but is called out here so it is not a surprise.

- [ ] **Step 1: Update sync count + vo2max table assert; add a backfill-window test (failing test).**
In `/home/jake/project/help-my-run/backend/internal/sync/sync_test.go`, update `TestSyncGarminUpsertsAllTables`. Change the synced-count check + comment (lines 135-138):
```go
	// Fixture has 2 sleep + 1 hrv + 2 bb + 2 rhr + 2 vo2max = 9 upserts.
	if res.Synced != 9 {
		t.Errorf("synced = %d, want 9", res.Synced)
	}
```
Add `garmin_vo2max` to the `counts` map (line 140-142):
```go
	counts := map[string]int{
		"garmin_sleep": 0, "garmin_hrv": 0, "garmin_body_battery": 0, "garmin_rhr": 0,
		"garmin_vo2max": 0,
	}
```
Update the count assertion (lines 150-153):
```go
	if counts["garmin_sleep"] != 2 || counts["garmin_hrv"] != 1 ||
		counts["garmin_body_battery"] != 2 || counts["garmin_rhr"] != 2 ||
		counts["garmin_vo2max"] != 2 {
		t.Errorf("counts = %+v, want sleep2 hrv1 bb2 rhr2 vo2max2", counts)
	}
```
Append a new test at the end of the file (after `TestRunTickerCallsAndStops`, line 303) that captures the `--since` arg the runner receives and asserts it equals an 84-day look-back on a fresh DB (runner invoked as `<python> <script> fetch --since <since>`; a /bin/sh stub echoes its args to a temp file). Ensure imports include `os`, `path/filepath`, `runtime`, `strings`, `time`, `context`, `help-my-run/backend/internal/garmin`:
```go
func TestSyncGarminBackfillWindowIs84Days(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	s := newStore(t)

	// Stub worker: write the args it was called with to argfile, emit empty
	// (but valid) contract JSON so the upsert loops run with 0 rows.
	dir := t.TempDir()
	argfile := filepath.Join(dir, "args.txt")
	script := filepath.Join(dir, "capture.sh")
	body := "#!/bin/sh\necho \"$@\" > '" + argfile + "'\n" +
		`echo '{"since":"x","until":"x","fetched_at":"x","sleep":[],"hrv":[],"body_battery":[],"rhr":[],"vo2max":[]}'` + "\n"
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
```

- [ ] **Step 2: Run — expect FAIL.**
Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/sync/ -run 'TestSyncGarminUpsertsAllTables|TestSyncGarminBackfillWindowIs84Days'`
Expected output (FAIL): `TestSyncGarminUpsertsAllTables` — `synced = 7, want 9` and `counts = ... vo2max:0 ... want ... vo2max2`. `TestSyncGarminBackfillWindowIs84Days` — `worker args = "fetch --since <30d>", want --since <84d>`.

- [ ] **Step 3: Add the upsert loop + bump backfill constant (minimal impl).**
In `/home/jake/project/help-my-run/backend/internal/sync/sync.go`, change the constant + doc comment (lines 161-162):
```go
// garminBackfillDays is the default look-back when Garmin has never synced.
// M3.1: 84d (~12 weeks) to seed VO2max + recovery trend history (spec §4).
// NOTE: first-sync-only; subsequent syncs use sync_log.last_synced_at. This
// also deepens sleep/hrv/bb/rhr first-sync history (acceptable, arguably
// desirable — those trends feed the progress engine too).
const garminBackfillDays = 84
```
Update the `SyncGarmin` doc comment (lines 164-165):
```go
// SyncGarmin runs the Python worker since the last successful Garmin sync (or a
// ~84-day / ~12-week backfill), upserts the five garmin_* tables, and records
// sync_log.
```
Add the VO2max upsert loop after the `out.RHR` loop (after line 216, before `return okResult(...)`):
```go
	for _, d := range out.VO2Max {
		if err := s.UpsertVo2max(store.Vo2maxRow{
			Date: d.Date, Vo2max: d.VO2Max, RawJSON: rawString(d.RawJSON),
		}); err != nil {
			return errResult(s, source, err)
		}
		synced++
	}
```

- [ ] **Step 4: Run — expect PASS.**
Command: `cd /home/jake/project/help-my-run/backend && go test ./internal/sync/ -run 'TestSyncGarminUpsertsAllTables|TestSyncGarminBackfillWindowIs84Days'`
Expected output (PASS): `ok  	help-my-run/backend/internal/sync`.

- [ ] **Step 5: Run the full backend suite to confirm no regressions (other tests asserting the 30-day window, if any, are caught here).**
Command: `cd /home/jake/project/help-my-run/backend && go test ./...`
Expected output (PASS): all packages `ok` (no `FAIL`).

- [ ] **Step 6: Commit.**
```
cd /home/jake/project/help-my-run/backend && git add internal/sync/sync.go internal/sync/sync_test.go && git commit -m "feat(sync): upsert VO2max in SyncGarmin; bump backfill to 84d (M3.1 INGEST)

The shared garminBackfillDays bump (30->84) also deepens the first-sync
backfill for the existing sleep/hrv/bb/rhr recovery tables (first-sync-only;
acceptable/desirable — feeds the progress engine).

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Progress engine pure types + signal/direction constants

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/progress/progress.go` (types + constants only in this task)
- Create: `/home/jake/project/help-my-run/backend/internal/progress/progress_test.go`

- [ ] **Step 1: Write failing JSON-tag + constants test.** Create `progress_test.go`:
```go
package progress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTrendSummaryJSONTags(t *testing.T) {
	cur := 330.0
	base := 350.0
	delta := -20.0
	w := 345.0
	ts := TrendSummary{
		Key:           SignalPaceAtHR,
		Label:         "Pace @ Z2 HR",
		Unit:          "s/km",
		Current:       &cur,
		Baseline:      &base,
		DeltaAbs:      &delta,
		Direction:     DirectionDown,
		LowerIsBetter: true,
		Series:        []*float64{&base, nil, &w},
	}
	b, err := json.Marshal(ts)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	got := string(b)
	for _, k := range []string{
		`"key":"pace_at_hr"`, `"label":"Pace @ Z2 HR"`, `"unit":"s/km"`,
		`"current":330`, `"baseline":350`, `"delta_abs":-20`,
		`"direction":"down"`, `"lower_is_better":true`,
		`"series":[350,null,345]`,
	} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}

func TestProgressReportJSONTags(t *testing.T) {
	rep := ProgressReport{Weeks: 12, GeneratedAt: "2026-06-21T07:00:00Z", EnoughData: false, Signals: []TrendSummary{}}
	b, _ := json.Marshal(rep)
	got := string(b)
	for _, k := range []string{
		`"weeks":12`, `"generated_at":"2026-06-21T07:00:00Z"`,
		`"enough_data":false`, `"signals":[]`,
	} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}

func TestSignalConstants(t *testing.T) {
	if SignalPaceAtHR != "pace_at_hr" || SignalVo2max != "vo2max" ||
		SignalRestingHR != "resting_hr" || SignalHRVBaseline != "hrv_baseline" ||
		SignalWeeklyLoad != "weekly_load" {
		t.Errorf("signal key constants drifted from contract")
	}
	if DefaultWeeks != 12 || MinWeeks != 4 || MaxWeeks != 52 {
		t.Errorf("week bounds drifted: %d/%d/%d", DefaultWeeks, MinWeeks, MaxWeeks)
	}
}
```

- [ ] **Step 2: Run — expect FAIL (no package).**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: build error `undefined: TrendSummary` / compile failures on the undefined identifiers.

- [ ] **Step 3: Minimal impl — create `progress.go` with types + constants (no ComputeProgress yet).**
```go
// Package progress computes deterministic cardio-capacity trend signals from
// M0 store rows + M1 metrics. ComputeProgress is pure (no DB, no clock): callers
// pass slices + an explicit `now`, so it is table-test friendly (mirrors metrics).
package progress

import (
	"time"

	"help-my-run/backend/internal/store"
)

// Canonical signal keys (CONTRACTS §3.1) — use verbatim.
const (
	SignalPaceAtHR    = "pace_at_hr"   // headline: weekly-median pace of in-band runs (sec/km)
	SignalVo2max      = "vo2max"       // Garmin VO2max
	SignalRestingHR   = "resting_hr"   // garmin_rhr
	SignalHRVBaseline = "hrv_baseline" // garmin_hrv last-night avg ms
	SignalWeeklyLoad  = "weekly_load"  // weekly running km (M1 metrics)
)

// Window constants (CONTRACTS §3.3).
const (
	DefaultWeeks         = 12
	MinWeeks             = 4
	MaxWeeks             = 52
	enoughDataMinSignals = 2 // >= this many FITNESS signals (weekly_load excluded) with >= 2 non-nil weekly points
)

// Reference-HR band constants (CONTRACTS §3.4).
const (
	// refHRBandBpm is the ± window around the reference HR (spec §7: ±5 bpm).
	refHRBandBpm = 5.0
	// defaultRefHRBpm is the fallback reference HR when profile.Zone2CeilingBpm
	// is nil (documented constant per spec §7).
	defaultRefHRBpm = 145.0
	// paceEps is the sec/km deadband for pace_at_hr direction classification.
	paceEps = 0.5
	// relDeadband is the relative (fraction) deadband for the non-pace signals
	// (mirrors metrics.recoveryDeadband = 0.03).
	relDeadband = 0.03
)

// TrendDirection is the value-movement direction of a signal over the window.
type TrendDirection string

const (
	DirectionUp   TrendDirection = "up"
	DirectionDown TrendDirection = "down"
	DirectionFlat TrendDirection = "flat"
)

// TrendSummary is one signal's trend card: weekly series + headline summary.
// Series has exactly weeks entries, oldest-first; nil = a week with no
// qualifying data (rendered as a gap, never interpolated).
type TrendSummary struct {
	Key           string         `json:"key"`
	Label         string         `json:"label"`
	Unit          string         `json:"unit"`
	Current       *float64       `json:"current"`
	Baseline      *float64       `json:"baseline"`
	DeltaAbs      *float64       `json:"delta_abs"`
	Direction     TrendDirection `json:"direction"`
	LowerIsBetter bool           `json:"lower_is_better"`
	Series        []*float64     `json:"series"`
}

// ProgressReport is the full deterministic read served at GET /api/progress.
type ProgressReport struct {
	Weeks       int            `json:"weeks"`
	GeneratedAt string         `json:"generated_at"`
	Signals     []TrendSummary `json:"signals"`
	EnoughData  bool           `json:"enough_data"`
}

// weekBucket is a half-open 7-day window (start, end] in UTC.
type weekBucket struct {
	start time.Time
	end   time.Time
}

// ensure store is referenced (used by ComputeProgress in a later task).
var _ = store.Activity{}
```

- [ ] **Step 4: Run — expect PASS.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `ok  	help-my-run/backend/internal/progress`

- [ ] **Step 5: Commit.**
`cd /home/jake/project/help-my-run/backend && git add internal/progress/progress.go internal/progress/progress_test.go && git commit -m "feat(progress): add progress engine types, signal keys, direction constants" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`

---

### Task 11: Weekly bucketing + series→summary helpers

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/progress/progress.go` (add `weekBuckets`, `summarize`; add `"math"` import)
- Modify: `/home/jake/project/help-my-run/backend/internal/progress/progress_test.go` (add bucketing + summarize table tests; ensure `"time"` imported)

- [ ] **Step 1: Write failing tests.** Append to `progress_test.go` (add `"time"` to the test imports and the `f64` helper):
```go
func f64(v float64) *float64 { return &v }

func TestWeekBucketsOldestFirstContiguous(t *testing.T) {
	now := time.Date(2026, 6, 21, 7, 0, 0, 0, time.UTC)
	bs := weekBuckets(3, now)
	if len(bs) != 3 {
		t.Fatalf("len = %d, want 3", len(bs))
	}
	// oldest-first: last bucket ends at now.
	if !bs[2].end.Equal(now) {
		t.Errorf("last bucket end = %v, want now %v", bs[2].end, now)
	}
	// contiguous half-open: each start == previous end.
	if !bs[1].start.Equal(bs[0].end) || !bs[2].start.Equal(bs[1].end) {
		t.Errorf("buckets not contiguous: %+v", bs)
	}
	// 7-day width.
	if bs[2].end.Sub(bs[2].start) != 7*24*time.Hour {
		t.Errorf("bucket width = %v, want 168h", bs[2].end.Sub(bs[2].start))
	}
}

func TestSummarizeDeltasAndDirection(t *testing.T) {
	tests := []struct {
		name          string
		series        []*float64
		lowerIsBetter bool
		wantCur       *float64
		wantBase      *float64
		wantDelta     *float64
		wantDir       TrendDirection
	}{
		{
			name:    "pace falling -> down (lower is better)",
			series:  []*float64{f64(350), nil, f64(330)},
			lowerIsBetter: true,
			wantCur: f64(330), wantBase: f64(350), wantDelta: f64(-20), wantDir: DirectionDown,
		},
		{
			name:    "vo2max rising -> up",
			series:  []*float64{f64(50), f64(51), f64(52)},
			lowerIsBetter: false,
			wantCur: f64(52), wantBase: f64(50), wantDelta: f64(2), wantDir: DirectionUp,
		},
		{
			name:    "flat within rel deadband",
			series:  []*float64{f64(50), f64(50.5)},
			lowerIsBetter: false,
			wantCur: f64(50.5), wantBase: f64(50), wantDelta: f64(0.5), wantDir: DirectionFlat,
		},
		{
			name:    "all nil -> nil summary, flat",
			series:  []*float64{nil, nil},
			lowerIsBetter: false,
			wantCur: nil, wantBase: nil, wantDelta: nil, wantDir: DirectionFlat,
		},
		{
			name:    "single point -> cur==base, zero delta, flat",
			series:  []*float64{nil, f64(42), nil},
			lowerIsBetter: false,
			wantCur: f64(42), wantBase: f64(42), wantDelta: f64(0), wantDir: DirectionFlat,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cur, base, delta, dir := summarize(tt.series, tt.lowerIsBetter, false)
			eqp := func(a, b *float64) bool {
				if a == nil || b == nil {
					return a == b
				}
				return *a == *b
			}
			if !eqp(cur, tt.wantCur) || !eqp(base, tt.wantBase) || !eqp(delta, tt.wantDelta) {
				t.Errorf("cur/base/delta = %v/%v/%v, want %v/%v/%v", cur, base, delta, tt.wantCur, tt.wantBase, tt.wantDelta)
			}
			if dir != tt.wantDir {
				t.Errorf("dir = %q, want %q", dir, tt.wantDir)
			}
		})
	}
}

func TestSummarizePaceEpsDeadband(t *testing.T) {
	// pace signal (isPace=true): 350 -> 350.3 is within 0.5 eps -> flat.
	_, _, _, dir := summarize([]*float64{f64(350), f64(350.3)}, true, true)
	if dir != DirectionFlat {
		t.Errorf("dir = %q, want flat (within paceEps)", dir)
	}
	// 350 -> 348 exceeds eps, value fell -> down.
	_, _, _, dir = summarize([]*float64{f64(350), f64(348)}, true, true)
	if dir != DirectionDown {
		t.Errorf("dir = %q, want down", dir)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `undefined: weekBuckets`, `undefined: summarize`.

- [ ] **Step 3: Minimal impl.** Add `"math"` to the import block of `progress.go` and add:
```go
// weekBuckets returns `weeks` contiguous half-open 7-day windows (start, end]
// ending at now, oldest-first (so index 0 is the oldest week).
func weekBuckets(weeks int, now time.Time) []weekBucket {
	out := make([]weekBucket, weeks)
	end := now
	for i := weeks - 1; i >= 0; i-- {
		start := end.AddDate(0, 0, -7)
		out[i] = weekBucket{start: start, end: end}
		end = start
	}
	return out
}

// summarize derives (current, baseline, deltaAbs, direction) from a weekly
// series. current = last non-nil; baseline = first non-nil; deltaAbs =
// current-baseline. Direction is the raw VALUE movement (up = value increased),
// independent of lowerIsBetter (the app maps direction+lowerIsBetter to a
// good/bad color). isPace selects the absolute paceEps deadband; otherwise a
// relative relDeadband (fraction of baseline) is used. All-nil -> (nil,nil,nil,flat).
func summarize(series []*float64, lowerIsBetter, isPace bool) (cur, base, delta *float64, dir TrendDirection) {
	_ = lowerIsBetter // retained for self-documentation; direction is raw value movement
	for _, v := range series {
		if v != nil {
			if base == nil {
				base = v
			}
			cur = v
		}
	}
	if cur == nil || base == nil {
		return nil, nil, nil, DirectionFlat
	}
	d := *cur - *base
	delta = &d

	if isPace {
		switch {
		case d > paceEps:
			return cur, base, delta, DirectionUp
		case d < -paceEps:
			return cur, base, delta, DirectionDown
		default:
			return cur, base, delta, DirectionFlat
		}
	}
	// Relative deadband for non-pace signals.
	var rel float64
	if *base != 0 {
		rel = d / math.Abs(*base)
	}
	switch {
	case rel > relDeadband:
		return cur, base, delta, DirectionUp
	case rel < -relDeadband:
		return cur, base, delta, DirectionDown
	default:
		return cur, base, delta, DirectionFlat
	}
}
```

- [ ] **Step 4: Run — expect PASS.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `ok  	help-my-run/backend/internal/progress`

- [ ] **Step 5: Commit.**
`cd /home/jake/project/help-my-run/backend && git add internal/progress/progress.go internal/progress/progress_test.go && git commit -m "feat(progress): add weekly bucketing and series-to-summary helper" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`

---

### Task 12: Per-signal weekly series builders (pace-at-HR band, vo2max, RHR, HRV, weekly load)

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/progress/progress.go` (add the five series builders; add `"sort"` + `"help-my-run/backend/internal/metrics"` imports; remove the `var _ = store.Activity{}` placeholder)
- Modify: `/home/jake/project/help-my-run/backend/internal/progress/progress_test.go` (table tests over fixtures, incl. ref-HR band + gaps)

Depends on: Task 2 (`store.Vo2maxPoint`), Task 7 (exported metrics helpers).

- [ ] **Step 1: Write failing tests.** Append to `progress_test.go`:
```go
// mkRun builds a Strava run activity for fixtures.
func mkRun(start string, distM float64, movingS int64, avgHR *float64) store.Activity {
	return store.Activity{Type: "Run", StartTime: start, DistanceM: distM, MovingTimeS: movingS, AvgHR: avgHR}
}

func TestPaceAtHRSeriesBandAndMedianAndGap(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	hr := func(v float64) *float64 { return &v }
	z2 := int64(145) // band [140,150]
	prof := store.AthleteProfile{Zone2CeilingBpm: &z2}
	acts := []store.Activity{
		// wk2: in-band, 5km @ 1750s (350s/km) and 5km @ 1650s (330s/km) -> median 340
		mkRun("2026-06-20T07:00:00Z", 5000, 1750, hr(145)),
		mkRun("2026-06-19T07:00:00Z", 5000, 1650, hr(148)),
		mkRun("2026-06-18T07:00:00Z", 5000, 1500, hr(120)), // out of band -> ignored
		// wk0 (oldest): one in-band 5km @ 1800s (360s/km)
		mkRun("2026-06-08T07:00:00Z", 5000, 1800, hr(143)),
		// wk1: NO in-band run -> gap (nil)
		mkRun("2026-06-14T07:00:00Z", 5000, 1500, hr(160)), // out of band
	}
	got := paceAtHRSeries(acts, prof, weekBuckets(3, now))
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0] == nil || *got[0] != 360 {
		t.Errorf("wk0 = %v, want 360", got[0])
	}
	if got[1] != nil {
		t.Errorf("wk1 = %v, want nil (gap, no in-band run)", got[1])
	}
	if got[2] == nil || *got[2] != 340 {
		t.Errorf("wk2 = %v, want median 340", got[2])
	}
}

func TestPaceAtHRSeriesDefaultRefHRWhenProfileNil(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	hr := func(v float64) *float64 { return &v }
	prof := store.AthleteProfile{} // nil Zone2CeilingBpm -> defaultRefHRBpm=145, band [140,150]
	acts := []store.Activity{
		mkRun("2026-06-20T07:00:00Z", 5000, 1750, hr(146)), // in default band
		mkRun("2026-06-19T07:00:00Z", 5000, 1500, hr(120)), // out
	}
	got := paceAtHRSeries(acts, prof, weekBuckets(1, now))
	if got[0] == nil || *got[0] != 350 {
		t.Errorf("wk0 = %v, want 350 (default ref HR band)", got[0])
	}
}

func TestWeeklyLoadSeries(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	acts := []store.Activity{
		mkRun("2026-06-20T07:00:00Z", 10000, 3000, nil), // wk1: 10km
		mkRun("2026-06-19T07:00:00Z", 5000, 1500, nil),  // wk1: +5km = 15km
		mkRun("2026-06-12T07:00:00Z", 8000, 2400, nil),  // wk0: 8km
	}
	got := weeklyLoadSeries(acts, weekBuckets(2, now))
	if got[0] == nil || *got[0] != 8 {
		t.Errorf("wk0 = %v, want 8", got[0])
	}
	if got[1] == nil || *got[1] != 15 {
		t.Errorf("wk1 = %v, want 15", got[1])
	}
}

func TestRecoverySeriesMeanAndGap(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	i := func(v int64) *int64 { return &v }
	// recovery is most-recent-first (as ListRecovery returns).
	rec := []store.RecoveryDay{
		{Date: "2026-06-20", RHR: &store.RhrFields{RestingHR: i(48)}, HRV: &store.HrvFields{LastNightAvgMs: i(52)}},
		{Date: "2026-06-19", RHR: &store.RhrFields{RestingHR: i(50)}, HRV: &store.HrvFields{LastNightAvgMs: i(50)}},
		// wk0: a single RHR day; no HRV -> HRV gap that week
		{Date: "2026-06-12", RHR: &store.RhrFields{RestingHR: i(52)}},
	}
	rhr := rhrSeries(rec, weekBuckets(2, now))
	if rhr[0] == nil || *rhr[0] != 52 {
		t.Errorf("rhr wk0 = %v, want 52", rhr[0])
	}
	if rhr[1] == nil || *rhr[1] != 49 { // mean(48,50)
		t.Errorf("rhr wk1 = %v, want 49", rhr[1])
	}
	hrv := hrvSeries(rec, weekBuckets(2, now))
	if hrv[0] != nil {
		t.Errorf("hrv wk0 = %v, want nil (no HRV that week)", hrv[0])
	}
	if hrv[1] == nil || *hrv[1] != 51 { // mean(52,50)
		t.Errorf("hrv wk1 = %v, want 51", hrv[1])
	}
}

func TestVo2maxSeriesLastInBucket(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	f := func(v float64) *float64 { return &v }
	// ListVo2max returns most-recent-first; date-only strings.
	pts := []store.Vo2maxPoint{
		{Date: "2026-06-20", Vo2max: f(52)},
		{Date: "2026-06-18", Vo2max: f(51)}, // same wk1 but earlier -> last (latest) is 52
		{Date: "2026-06-10", Vo2max: f(50)}, // wk0
	}
	got := vo2maxSeries(pts, weekBuckets(2, now))
	if got[0] == nil || *got[0] != 50 {
		t.Errorf("wk0 = %v, want 50", got[0])
	}
	if got[1] == nil || *got[1] != 52 {
		t.Errorf("wk1 = %v, want 52 (latest in bucket)", got[1])
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `undefined: paceAtHRSeries` (and `weeklyLoadSeries`, `rhrSeries`, `hrvSeries`, `vo2maxSeries`).

- [ ] **Step 3: Minimal impl.** Add `"sort"` and `"help-my-run/backend/internal/metrics"` to the import block; remove the `var _ = store.Activity{}` placeholder from Task 10; add:
```go
// inBucket reports whether t falls in the half-open bucket (start, end].
func inBucket(t time.Time, b weekBucket) bool {
	return t.After(b.start) && !t.After(b.end)
}

// refHRBand returns the reference-HR band (Zone2 ceiling or documented default).
func refHRBand(profile store.AthleteProfile) (lo, hi float64) {
	ref := defaultRefHRBpm
	if profile.Zone2CeilingBpm != nil {
		ref = float64(*profile.Zone2CeilingBpm)
	}
	return ref - refHRBandBpm, ref + refHRBandBpm
}

// paceAtHRSeries builds the headline weekly-median pace (sec/km) of in-band runs
// per bucket. A bucket with no qualifying in-band run -> nil (gap). Lower=better.
func paceAtHRSeries(acts []store.Activity, profile store.AthleteProfile, buckets []weekBucket) []*float64 {
	lo, hi := refHRBand(profile)
	out := make([]*float64, len(buckets))
	for bi, b := range buckets {
		var paces []float64
		for _, a := range acts {
			if !metrics.IsRun(a.Type) || a.DistanceM <= 0 || a.MovingTimeS <= 0 || a.AvgHR == nil {
				continue
			}
			if *a.AvgHR < lo || *a.AvgHR > hi {
				continue
			}
			t, ok := metrics.ParseStart(a.StartTime)
			if !ok || !inBucket(t, b) {
				continue
			}
			paces = append(paces, float64(a.MovingTimeS)/(a.DistanceM/1000.0))
		}
		if len(paces) == 0 {
			continue // gap
		}
		sort.Float64s(paces)
		m := metrics.Median(paces)
		out[bi] = &m
	}
	return out
}

// weeklyLoadSeries builds per-bucket running km. A bucket with zero run km -> 0.0
// (not a gap: zero IS data).
func weeklyLoadSeries(acts []store.Activity, buckets []weekBucket) []*float64 {
	out := make([]*float64, len(buckets))
	for bi, b := range buckets {
		var km float64
		for _, a := range acts {
			if !metrics.IsRun(a.Type) {
				continue
			}
			t, ok := metrics.ParseStart(a.StartTime)
			if !ok || !inBucket(t, b) {
				continue
			}
			km += a.DistanceM / 1000.0
		}
		v := km
		out[bi] = &v
	}
	return out
}

// rhrSeries: per-bucket mean of in-bucket non-nil resting HR. Empty -> nil (gap).
func rhrSeries(rec []store.RecoveryDay, buckets []weekBucket) []*float64 {
	return recoveryMeanSeries(rec, buckets, func(d store.RecoveryDay) *int64 {
		if d.RHR == nil {
			return nil
		}
		return d.RHR.RestingHR
	})
}

// hrvSeries: per-bucket mean of in-bucket non-nil HRV last-night avg. Empty -> nil.
func hrvSeries(rec []store.RecoveryDay, buckets []weekBucket) []*float64 {
	return recoveryMeanSeries(rec, buckets, func(d store.RecoveryDay) *int64 {
		if d.HRV == nil {
			return nil
		}
		return d.HRV.LastNightAvgMs
	})
}

// recoveryMeanSeries averages pick(d) over in-bucket recovery days (date is a
// YYYY-MM-DD string, bucketed at midnight UTC). Empty bucket -> nil (gap).
func recoveryMeanSeries(rec []store.RecoveryDay, buckets []weekBucket, pick func(store.RecoveryDay) *int64) []*float64 {
	out := make([]*float64, len(buckets))
	for bi, b := range buckets {
		var sum float64
		var n int
		for _, d := range rec {
			t, ok := parseDate(d.Date)
			if !ok || !inBucket(t, b) {
				continue
			}
			if v := pick(d); v != nil {
				sum += float64(*v)
				n++
			}
		}
		if n == 0 {
			continue
		}
		m := sum / float64(n)
		out[bi] = &m
	}
	return out
}

// vo2maxSeries: latest (most-recent dated) non-nil VO2max reading within each
// bucket. pts may be most-recent-first; we track the max date seen per bucket.
func vo2maxSeries(pts []store.Vo2maxPoint, buckets []weekBucket) []*float64 {
	out := make([]*float64, len(buckets))
	bestDate := make([]string, len(buckets))
	for _, p := range pts {
		if p.Vo2max == nil {
			continue
		}
		t, ok := parseDate(p.Date)
		if !ok {
			continue
		}
		for bi, b := range buckets {
			if !inBucket(t, b) {
				continue
			}
			if p.Date > bestDate[bi] { // lexical compare works for YYYY-MM-DD
				bestDate[bi] = p.Date
				v := *p.Vo2max
				out[bi] = &v
			}
		}
	}
	return out
}

// parseDate parses a YYYY-MM-DD store date at 00:00:00 UTC.
func parseDate(date string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
```

- [ ] **Step 4: Run — expect PASS.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `ok  	help-my-run/backend/internal/progress`

- [ ] **Step 5: Commit.**
`cd /home/jake/project/help-my-run/backend && git add internal/progress/progress.go internal/progress/progress_test.go && git commit -m "feat(progress): add per-signal weekly series builders (pace@HR band, vo2max, rhr, hrv, load)" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`

---

### Task 13: ComputeProgress assembler + not-enough-data path

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/progress/progress.go` (add `ComputeProgress`, `buildSignal`, `signalMeta`/`signalMetas`, `countNonNil`)
- Modify: `/home/jake/project/help-my-run/backend/internal/progress/progress_test.go` (full-report + empty-state tests)

Depends on: Task 2 (`store.Vo2maxPoint` must be merged before this compiles).

- [ ] **Step 1: Write failing tests.** Append to `progress_test.go`:
```go
func TestComputeProgressFullReport(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	hr := func(v float64) *float64 { return &v }
	i := func(v int64) *int64 { return &v }
	f := func(v float64) *float64 { return &v }
	z2 := int64(145)
	prof := store.AthleteProfile{Zone2CeilingBpm: &z2}

	acts := []store.Activity{
		// newest week in-band 5km@1650s (330)
		mkRun("2026-06-20T07:00:00Z", 5000, 1650, hr(146)),
		// oldest week (12w back) in-band 5km@1750s (350)
		mkRun("2026-03-31T07:00:00Z", 5000, 1750, hr(144)),
	}
	rec := []store.RecoveryDay{
		{Date: "2026-06-20", RHR: &store.RhrFields{RestingHR: i(47)}, HRV: &store.HrvFields{LastNightAvgMs: i(52)}},
		{Date: "2026-03-31", RHR: &store.RhrFields{RestingHR: i(50)}, HRV: &store.HrvFields{LastNightAvgMs: i(46)}},
	}
	vo2 := []store.Vo2maxPoint{
		{Date: "2026-06-19", Vo2max: f(52)},
		{Date: "2026-04-01", Vo2max: f(50)},
	}

	rep := ComputeProgress(acts, rec, vo2, prof, 12, now)
	if rep.Weeks != 12 {
		t.Errorf("Weeks = %d, want 12", rep.Weeks)
	}
	if rep.GeneratedAt == "" {
		t.Error("GeneratedAt empty")
	}
	if len(rep.Signals) != 5 {
		t.Fatalf("len(Signals) = %d, want 5", len(rep.Signals))
	}
	// Signal order is fixed: pace_at_hr, vo2max, resting_hr, hrv_baseline, weekly_load.
	wantOrder := []string{SignalPaceAtHR, SignalVo2max, SignalRestingHR, SignalHRVBaseline, SignalWeeklyLoad}
	for i, s := range rep.Signals {
		if s.Key != wantOrder[i] {
			t.Errorf("signal[%d].Key = %q, want %q", i, s.Key, wantOrder[i])
		}
		if len(s.Series) != 12 {
			t.Errorf("signal[%d] series len = %d, want 12", i, len(s.Series))
		}
	}
	pace := rep.Signals[0]
	if pace.Unit != "s/km" || !pace.LowerIsBetter {
		t.Errorf("pace card = %+v", pace)
	}
	if pace.Current == nil || *pace.Current != 330 || pace.Baseline == nil || *pace.Baseline != 350 {
		t.Errorf("pace cur/base = %v/%v, want 330/350", pace.Current, pace.Baseline)
	}
	if pace.Direction != DirectionDown {
		t.Errorf("pace direction = %q, want down", pace.Direction)
	}
	rhr := rep.Signals[2]
	if !rhr.LowerIsBetter {
		t.Error("resting_hr lower_is_better should be true")
	}
	if !rep.EnoughData {
		t.Error("EnoughData = false, want true (>=2 signals with >=2 points)")
	}
}

func TestComputeProgressNotEnoughData(t *testing.T) {
	now := time.Date(2026, 6, 21, 0, 0, 0, 0, time.UTC)
	hr := func(v float64) *float64 { return &v }
	z2 := int64(145)
	prof := store.AthleteProfile{Zone2CeilingBpm: &z2}
	// Only one in-band run total -> pace has 1 point; nothing else -> < 2 signals w/ >=2 pts.
	acts := []store.Activity{mkRun("2026-06-20T07:00:00Z", 5000, 1650, hr(146))}
	rep := ComputeProgress(acts, nil, nil, prof, 12, now)
	if rep.EnoughData {
		t.Error("EnoughData = true, want false (thin history)")
	}
	// Signals still computed (the handler decides whether to blank them); contract
	// §3.3: EnoughData is the only thin-history gate.
	if len(rep.Signals) != 5 {
		t.Errorf("len(Signals) = %d, want 5", len(rep.Signals))
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `undefined: ComputeProgress`.

- [ ] **Step 3: Minimal impl.** Add to `progress.go`:
```go
// signalMeta is the static label+unit+polarity for each signal key.
type signalMeta struct {
	label         string
	unit          string
	lowerIsBetter bool
	isPace        bool
}

var signalMetas = map[string]signalMeta{
	SignalPaceAtHR:    {label: "Pace @ Z2 HR", unit: "s/km", lowerIsBetter: true, isPace: true},
	SignalVo2max:      {label: "VO2max", unit: "ml/kg/min", lowerIsBetter: false},
	SignalRestingHR:   {label: "Resting HR", unit: "bpm", lowerIsBetter: true},
	SignalHRVBaseline: {label: "HRV baseline", unit: "ms", lowerIsBetter: false},
	SignalWeeklyLoad:  {label: "Weekly volume", unit: "km", lowerIsBetter: false},
}

// buildSignal assembles one TrendSummary from a key + computed series.
func buildSignal(key string, series []*float64) TrendSummary {
	m := signalMetas[key]
	cur, base, delta, dir := summarize(series, m.lowerIsBetter, m.isPace)
	return TrendSummary{
		Key:           key,
		Label:         m.label,
		Unit:          m.unit,
		Current:       cur,
		Baseline:      base,
		DeltaAbs:      delta,
		Direction:     dir,
		LowerIsBetter: m.lowerIsBetter,
		Series:        series,
	}
}

// countNonNil returns the number of non-nil entries in a series.
func countNonNil(series []*float64) int {
	n := 0
	for _, v := range series {
		if v != nil {
			n++
		}
	}
	return n
}

// ComputeProgress builds the deterministic ProgressReport over `weeks` weekly
// buckets ending at `now`. Pure: caller supplies all rows + now. Signal order is
// fixed (pace_at_hr, vo2max, resting_hr, hrv_baseline, weekly_load). Series are
// always exactly `weeks` long, oldest-first, nil = a gap (never interpolated).
func ComputeProgress(
	acts []store.Activity,
	recovery []store.RecoveryDay,
	vo2max []store.Vo2maxPoint,
	profile store.AthleteProfile,
	weeks int,
	now time.Time,
) ProgressReport {
	buckets := weekBuckets(weeks, now)

	signals := []TrendSummary{
		buildSignal(SignalPaceAtHR, paceAtHRSeries(acts, profile, buckets)),
		buildSignal(SignalVo2max, vo2maxSeries(vo2max, buckets)),
		buildSignal(SignalRestingHR, rhrSeries(recovery, buckets)),
		buildSignal(SignalHRVBaseline, hrvSeries(recovery, buckets)),
		buildSignal(SignalWeeklyLoad, weeklyLoadSeries(acts, buckets)),
	}

	// weekly_load is CONTEXT (always filled with 0.0, never nil), not a fitness
	// verdict — exclude it so the gate requires >=2 of the FOUR real fitness
	// signals (pace_at_hr, vo2max, resting_hr, hrv_baseline).
	enough := 0
	for _, s := range signals {
		if s.Key == SignalWeeklyLoad {
			continue
		}
		if countNonNil(s.Series) >= 2 {
			enough++
		}
	}

	return ProgressReport{
		Weeks:       weeks,
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Signals:     signals,
		EnoughData:  enough >= enoughDataMinSignals,
	}
}
```

- [ ] **Step 4: Run — expect PASS.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `ok  	help-my-run/backend/internal/progress`

- [ ] **Step 5: Commit.**
`cd /home/jake/project/help-my-run/backend && git add internal/progress/progress.go internal/progress/progress_test.go && git commit -m "feat(progress): add ComputeProgress assembler and not-enough-data gate" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`

---

### Task 14: Analyze prompt, input struct, ProgressRead, deterministic fallback

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/progress/prompts.go` (`progressReadPrompt`, `ProgressReadInput`, `ProgressRead`, `fallbackProgressText`)
- Create: `/home/jake/project/help-my-run/backend/internal/progress/prompts_test.go`

Depends on: Task 7 (`metrics.FormatPace` exported), Task 10 (types).

- [ ] **Step 1: Write failing tests.** Create `prompts_test.go`:
```go
package progress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProgressReadJSONTags(t *testing.T) {
	pr := ProgressRead{Text: "ok", Source: "ai"}
	b, _ := json.Marshal(pr)
	got := string(b)
	if !strings.Contains(got, `"text":"ok"`) || !strings.Contains(got, `"source":"ai"`) {
		t.Errorf("JSON = %s", got)
	}
}

func TestProgressReadInputJSONTags(t *testing.T) {
	in := ProgressReadInput{Weeks: 12, GoalText: "RX CrossFit engine", Signals: []TrendSummary{{Key: SignalPaceAtHR}}}
	b, _ := json.Marshal(in)
	got := string(b)
	for _, k := range []string{`"weeks":12`, `"goal_text":"RX CrossFit engine"`, `"signals":[`, `"key":"pace_at_hr"`} {
		if !strings.Contains(got, k) {
			t.Errorf("JSON %s missing %q", got, k)
		}
	}
}

func TestProgressReadPromptShape(t *testing.T) {
	for _, sub := range []string{"progress read", "pace_at_hr", "lower_is_better", `{"text":`, "RX CrossFit"} {
		if !strings.Contains(progressReadPrompt, sub) {
			t.Errorf("prompt missing %q", sub)
		}
	}
}

func TestFallbackProgressTextNotEnoughData(t *testing.T) {
	rep := ProgressReport{Weeks: 12, EnoughData: false}
	got := fallbackProgressText(rep)
	if !strings.Contains(got, "Not enough history") {
		t.Errorf("got %q, want not-enough-data sentence", got)
	}
}

func TestFallbackProgressTextImprovedClauses(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	rep := ProgressReport{
		Weeks:      12,
		EnoughData: true,
		Signals: []TrendSummary{
			{ // pace fell 350->330: improved (lowerIsBetter), formatted via metrics.FormatPace
				Key: SignalPaceAtHR, Label: "Pace @ Z2 HR", Unit: "s/km",
				Current: f(330), Baseline: f(350), DeltaAbs: f(-20),
				Direction: DirectionDown, LowerIsBetter: true,
			},
			{ // vo2max rose 50->52: improved (higher better)
				Key: SignalVo2max, Label: "VO2max", Unit: "ml/kg/min",
				Current: f(52), Baseline: f(50), DeltaAbs: f(2),
				Direction: DirectionUp, LowerIsBetter: false,
			},
			{ // resting HR rose 47->50: worsened (lowerIsBetter)
				Key: SignalRestingHR, Label: "Resting HR", Unit: "bpm",
				Current: f(50), Baseline: f(47), DeltaAbs: f(3),
				Direction: DirectionUp, LowerIsBetter: true,
			},
			{ // no data: skipped
				Key: SignalHRVBaseline, Label: "HRV baseline", Unit: "ms",
				Current: nil, Baseline: nil,
			},
		},
	}
	got := fallbackProgressText(rep)
	if !strings.Contains(got, "Over the last 12 weeks") {
		t.Errorf("missing prefix: %q", got)
	}
	if !strings.Contains(got, "Pace @ Z2 HR improved") {
		t.Errorf("pace clause wrong: %q", got)
	}
	// pace formatted M:SS/km via metrics.FormatPace
	if !strings.Contains(got, "5:50/km") || !strings.Contains(got, "5:30/km") {
		t.Errorf("pace not formatted: %q", got)
	}
	if !strings.Contains(got, "VO2max improved") {
		t.Errorf("vo2max clause wrong: %q", got)
	}
	if !strings.Contains(got, "Resting HR worsened") {
		t.Errorf("resting hr clause wrong: %q", got)
	}
	if strings.Contains(got, "HRV baseline") {
		t.Errorf("HRV (no data) should be skipped: %q", got)
	}
}
```

- [ ] **Step 2: Run — expect FAIL.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `undefined: ProgressRead`, `undefined: ProgressReadInput`, `undefined: progressReadPrompt`, `undefined: fallbackProgressText`.

- [ ] **Step 3: Minimal impl.** Create `prompts.go`:
```go
package progress

import (
	"fmt"
	"strings"

	"help-my-run/backend/internal/metrics"
)

// ProgressReadInput is the JSON piped to claude -p stdin for the progress read:
// the computed trends + window + the athlete's goal context. snake_case wire JSON.
type ProgressReadInput struct {
	Weeks    int            `json:"weeks"`
	Signals  []TrendSummary `json:"signals"`
	GoalText string         `json:"goal_text"`
}

// ProgressRead is the analyze result (mirrors M2 source = ai|fallback semantics).
type ProgressRead struct {
	Text   string `json:"text"`
	Source string `json:"source"` // "ai" | "fallback"
}

// progressReadPrompt is the claude -p instruction block for the on-demand read.
// The structured ProgressReadInput is piped on stdin; the model returns ONLY a
// {"text": "..."} JSON object.
const progressReadPrompt = `You are a CrossFit-aware running coach giving a short progress read. You receive a JSON
context on stdin: the number of weeks in the window, the athlete's goal text, and an array
of computed trend signals. Each signal has: key, label, unit, current, baseline, delta_abs,
direction (up|down|flat), lower_is_better, and a weekly series (nulls are weeks with no data).

The athlete is training to improve aerobic capacity for RX CrossFit, NOT to race. Interpret
each signal correctly: for pace_at_hr and resting_hr, LOWER is better (a falling value is
improvement); for vo2max and hrv_baseline, HIGHER is better; weekly_load is context, not a
fitness verdict. Faster pace at the same heart rate = a stronger engine.

Write 2-4 sentences, plain text (NO markdown, NO bullet list), that tell the athlete whether
their engine is improving and which signal is the clearest evidence. Be concrete (cite a
number or two). If the data is thin, say so honestly.

Output ONLY a single JSON object (no prose outside it, no markdown fences) of this EXACT shape:
{"text": "..."}`

// fmtVal renders a signal value respecting its unit (pace formatted as M:SS/km).
func fmtVal(unit string, v float64) string {
	if unit == "s/km" {
		return metrics.FormatPace(v)
	}
	return fmt.Sprintf("%g%s", v, unit)
}

// fallbackProgressText is the deterministic, no-LLM one-paragraph summary built
// from the computed report. Used whenever the claude -p read fails.
func fallbackProgressText(rep ProgressReport) string {
	if !rep.EnoughData {
		return "Not enough history yet to read a trend — keep logging runs and syncing Garmin."
	}
	var clauses []string
	for _, sg := range rep.Signals {
		if sg.Current == nil || sg.Baseline == nil {
			continue
		}
		verb := "held"
		switch sg.Direction {
		case DirectionUp:
			if sg.LowerIsBetter {
				verb = "worsened"
			} else {
				verb = "improved"
			}
		case DirectionDown:
			if sg.LowerIsBetter {
				verb = "improved"
			} else {
				verb = "worsened"
			}
		}
		clauses = append(clauses, fmt.Sprintf("%s %s (%s → %s)",
			sg.Label, verb, fmtVal(sg.Unit, *sg.Baseline), fmtVal(sg.Unit, *sg.Current)))
	}
	if len(clauses) == 0 {
		return "Not enough history yet to read a trend — keep logging runs and syncing Garmin."
	}
	return fmt.Sprintf("Over the last %d weeks: %s.", rep.Weeks, strings.Join(clauses, "; "))
}
```
Note: with baseline 350 / current 330, `fmtVal` → `metrics.FormatPace(350)` = `5:50/km`, `FormatPace(330)` = `5:30/km` — matches the test.

- [ ] **Step 4: Run — expect PASS.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `ok  	help-my-run/backend/internal/progress`

- [ ] **Step 5: Commit.**
`cd /home/jake/project/help-my-run/backend && git add internal/progress/prompts.go internal/progress/prompts_test.go && git commit -m "feat(progress): add analyze prompt, input/result structs, deterministic fallback text" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`

---

### Task 15: Engine (Report + Analyze) reusing the M1/M2 llm Client.Call

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/progress/engine.go` (`Engine{store,llm,model}`, `New`, `Report`, `Analyze`, `analyzeArgs`)
- Create: `/home/jake/project/help-my-run/backend/internal/progress/engine_test.go` (captureRunner ai-path + failRunner fallback-path + Report test)

Depends on: Task 2 (`store.ListVo2max`/`Vo2maxPoint`/`UpsertVo2max`/`Vo2maxRow`), Task 13 (`ComputeProgress`), Task 14 (`prompts.go`), Task 7 (exported metrics). Also uses `store.ListActivities`, `store.ListRecovery`, `store.GetAthleteProfile`, `llm.Client`.

- [ ] **Step 1: Write failing tests.** Create `engine_test.go`:
```go
package progress

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

func newProgressStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "progress.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

// captureRunner records args + stdin and returns a canned envelope.
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

// failRunner returns an is_error envelope (fallback trigger).
type failRunner struct{ calls int }

func (r *failRunner) Run(ctx context.Context, args []string, stdin string) ([]byte, error) {
	r.calls++
	return []byte(`{"type":"result","is_error":true,"result":"please login"}`), nil
}

const readEnv = `{"type":"result","subtype":"success","is_error":false,"result":"{\"text\":\"Engine improving: pace at Z2 dropped.\"}"}`

func fp(v float64) *float64 { return &v }

// seedTrendData inserts enough activities + vo2max so EnoughData is true.
func seedTrendData(t *testing.T, s *store.Store) {
	t.Helper()
	_ = s.UpsertActivity(store.Activity{
		StravaID: 1, Name: "easy", Type: "Run",
		StartTime: "2026-06-15T06:00:00Z", DistanceM: 10000, MovingTimeS: 3300,
		AvgHR: fp(145), RawJSON: "{}",
	})
	_ = s.UpsertActivity(store.Activity{
		StravaID: 2, Name: "easy2", Type: "Run",
		StartTime: "2026-04-06T06:00:00Z", DistanceM: 10000, MovingTimeS: 3500,
		AvgHR: fp(145), RawJSON: "{}",
	})
	_ = s.UpsertVo2max(store.Vo2maxRow{Date: "2026-06-15", Vo2max: fp(52), RawJSON: "{}"})
	_ = s.UpsertVo2max(store.Vo2maxRow{Date: "2026-04-06", Vo2max: fp(50), RawJSON: "{}"})
}

func TestReportBuildsFromStore(t *testing.T) {
	s := newProgressStore(t)
	seedTrendData(t, s)
	e := New(s, &llm.Client{Runner: &captureRunner{}, Model: "m"}, "m")

	rep, err := e.Report(context.Background(), 12)
	if err != nil {
		t.Fatalf("Report error = %v", err)
	}
	if rep.Weeks != 12 {
		t.Errorf("weeks = %d, want 12", rep.Weeks)
	}
	if len(rep.Signals) == 0 {
		t.Error("signals empty, want >=1 signal card")
	}
}

func TestAnalyzeAIPath(t *testing.T) {
	s := newProgressStore(t)
	seedTrendData(t, s)
	r := &captureRunner{out: []byte(readEnv)}
	e := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8")

	read, err := e.Analyze(context.Background(), 12)
	if err != nil {
		t.Fatalf("Analyze error = %v", err)
	}
	if read.Source != "ai" {
		t.Errorf("source = %q, want ai", read.Source)
	}
	if read.Text != "Engine improving: pace at Z2 dropped." {
		t.Errorf("text = %q", read.Text)
	}
	// stdin carries the computed signals + window.
	if !strings.Contains(r.body, `"weeks"`) || !strings.Contains(r.body, `"signals"`) {
		t.Errorf("stdin missing weeks/signals: %s", r.body)
	}
	// argv carries the read prompt + flags.
	joined := strings.Join(r.args, " ")
	if !strings.Contains(joined, "progress read") {
		t.Errorf("args missing progress-read prompt: %v", r.args)
	}
	if !hasPair(r.args, "--output-format", "json") || !hasPair(r.args, "--model", "claude-opus-4-8") {
		t.Errorf("args missing model/output-format: %v", r.args)
	}
}

func TestAnalyzeFallbackOnFailure(t *testing.T) {
	s := newProgressStore(t)
	seedTrendData(t, s)
	r := &failRunner{}
	e := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8")

	read, err := e.Analyze(context.Background(), 12)
	if err != nil {
		t.Fatalf("Analyze fallback returned error = %v, want nil", err)
	}
	if read.Source != "fallback" {
		t.Errorf("source = %q, want fallback", read.Source)
	}
	if read.Text == "" {
		t.Error("fallback text empty, want templated summary")
	}
}

func TestAnalyzeFallbackNotEnoughData(t *testing.T) {
	s := newProgressStore(t) // empty store -> EnoughData false
	r := &failRunner{}
	e := New(s, &llm.Client{Runner: r, Model: "claude-opus-4-8"}, "claude-opus-4-8")

	read, err := e.Analyze(context.Background(), 12)
	if err != nil {
		t.Fatalf("Analyze error = %v", err)
	}
	if read.Source != "fallback" {
		t.Errorf("source = %q, want fallback", read.Source)
	}
	if !strings.Contains(read.Text, "Not enough history") {
		t.Errorf("text = %q, want not-enough-history message", read.Text)
	}
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
NOTE: `fp` is also defined here; do NOT also define a duplicate `f64`/`fp` if another `_test.go` in this package already declares the same name — `f64` (Task 11) and `fp` (this task) are distinct identifiers, so both coexist. If `progress_test.go` already declares `fp`, drop this one.

- [ ] **Step 2: Run — expect FAIL.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `undefined: New` / `undefined: Engine` / `Report` / `Analyze`.

- [ ] **Step 3: Minimal impl.** Create `engine.go`:
```go
package progress

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"help-my-run/backend/internal/llm"
	"help-my-run/backend/internal/store"
)

// Engine reads the store, runs ComputeProgress, and (for Analyze) calls the
// shared llm.Client. It is the concrete impl of the api.Progress seam (M3.1).
// The deterministic computation lives in ComputeProgress (pure); only Report and
// Analyze here read the store / call claude. *Engine satisfies api.Progress.
type Engine struct {
	store *store.Store
	llm   *llm.Client
	model string
}

// New constructs a progress Engine.
func New(s *store.Store, c *llm.Client, model string) *Engine {
	return &Engine{store: s, llm: c, model: model}
}

// Report builds the deterministic ProgressReport over `weeks` weekly buckets
// ending at now (UTC). Reads activities, recovery, vo2max, and the profile, then
// delegates to the pure ComputeProgress.
func (e *Engine) Report(ctx context.Context, weeks int) (ProgressReport, error) {
	acts, err := e.store.ListActivities(500)
	if err != nil {
		return ProgressReport{}, err
	}
	rec, err := e.store.ListRecovery(7 * MaxWeeks) // enough days to cover the deepest window
	if err != nil {
		return ProgressReport{}, err
	}
	vo2, err := e.store.ListVo2max(7 * MaxWeeks)
	if err != nil {
		return ProgressReport{}, err
	}
	prof, err := e.store.GetAthleteProfile()
	if err != nil {
		return ProgressReport{}, err
	}
	return ComputeProgress(acts, rec, vo2, prof, weeks, time.Now().UTC()), nil
}

// analyzeArgs builds the claude -p argv for the progress read (mirrors
// coach.dailyAdjustArgs).
func (e *Engine) analyzeArgs() []string {
	return []string{
		"-p", progressReadPrompt,
		"--model", e.model,
		"--output-format", "json",
		"--allowedTools", "",
		"--no-session-persistence",
	}
}

// Analyze runs the claude -p progress read over the computed trends. It mirrors
// coach.AdjustToday exactly: setup failures (Report/profile) return an error;
// every llm.Call failure (always *llm.CallError or llm.ErrMalformedJSON) -> log +
// deterministic templated fallback with Source:"fallback"; success -> Source:"ai".
func (e *Engine) Analyze(ctx context.Context, weeks int) (ProgressRead, error) {
	rep, err := e.Report(ctx, weeks)
	if err != nil {
		return ProgressRead{}, err
	}
	prof, err := e.store.GetAthleteProfile()
	if err != nil {
		return ProgressRead{}, err
	}
	in := ProgressReadInput{Weeks: rep.Weeks, Signals: rep.Signals, GoalText: prof.GoalText}
	inputJSON, err := json.Marshal(in)
	if err != nil {
		return ProgressRead{}, err
	}

	var parsed struct {
		Text string `json:"text"`
	}
	if cerr := e.llm.Call(ctx, e.analyzeArgs(), string(inputJSON), &parsed); cerr != nil {
		log.Printf("progress.Analyze: claude failed (%v); using deterministic fallback", cerr)
		return ProgressRead{Text: fallbackProgressText(rep), Source: "fallback"}, nil
	}
	return ProgressRead{Text: parsed.Text, Source: "ai"}, nil
}
```

- [ ] **Step 4: Run — expect PASS.**
`cd /home/jake/project/help-my-run/backend && go test ./internal/progress/`
Expected: `ok  	help-my-run/backend/internal/progress`

- [ ] **Step 5: Run full backend suite (no regressions).**
`cd /home/jake/project/help-my-run/backend && go test ./...`
Expected: all packages `ok` (no `FAIL`).

- [ ] **Step 6: Commit.**
`cd /home/jake/project/help-my-run/backend && git add internal/progress/engine.go internal/progress/engine_test.go && git commit -m "feat(progress): add Engine Report/Analyze reusing shared llm.Client with deterministic fallback" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"`

---

### Task 16: `Progress` seam interface + `Deps.Progress` field + test fake

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/internal/api/router.go` (add `Progress` interface, `Deps.Progress` field; import `progress` for `progress.ProgressReport`/`progress.ProgressRead`)
- Modify: `/home/jake/project/help-my-run/backend/internal/api/m2_fakes_test.go` (add `fakeProgress` + compile-time `var _ Progress = (*fakeProgress)(nil)`)
- Modify: `/home/jake/project/help-my-run/backend/internal/api/handlers_test.go` (`newTestServer` `Deps` gains `Progress: &fakeProgress{}`)

Depends on: Task 14 (`progress.ProgressRead`), Task 10 (`progress.ProgressReport`). The api package imports `progress` only for these two types (same allowance as the `metrics` import for `Coach`).

- [ ] **Step 1: RED — add `fakeProgress` + conformance assertion (failing compile).** Append to `/home/jake/project/help-my-run/backend/internal/api/m2_fakes_test.go`:
```go
// fakeProgress is the injected api.Progress for handler tests.
type fakeProgress struct {
	report     progress.ProgressReport
	read       progress.ProgressRead
	reportErr  error
	analyzeErr error
	lastWeeks  int
}

func (f *fakeProgress) Report(ctx context.Context, weeks int) (progress.ProgressReport, error) {
	f.lastWeeks = weeks
	if f.reportErr != nil {
		return progress.ProgressReport{}, f.reportErr
	}
	if f.report.Weeks == 0 {
		f.report.Weeks = weeks
	}
	return f.report, nil
}

func (f *fakeProgress) Analyze(ctx context.Context, weeks int) (progress.ProgressRead, error) {
	f.lastWeeks = weeks
	if f.analyzeErr != nil {
		return progress.ProgressRead{}, f.analyzeErr
	}
	return f.read, nil
}
```
Add `"help-my-run/backend/internal/progress"` to the import block (`"context"` is already imported). Add to the existing conformance var block:
```go
var (
	_ Coach    = (*fakeCoach)(nil)
	_ Agent    = (*fakeAgent)(nil)
	_ Pusher   = (*fakePusher)(nil)
	_ Progress = (*fakeProgress)(nil)
)
```

- [ ] **Step 2: RUN (expect FAIL — `Progress` undefined).**
```
cd /home/jake/project/help-my-run/backend && go vet ./internal/api/ 2>&1 | head
```
Expected: compile error `undefined: Progress` (the interface does not exist yet).

- [ ] **Step 3: GREEN — add the `Progress` interface + `Deps.Progress` field.** In `/home/jake/project/help-my-run/backend/internal/api/router.go`, add `"help-my-run/backend/internal/progress"` to the import block, then add after the `Pusher` interface (line 45):
```go
// Progress is the M3.1 progress-engine seam, injected from main.go (avoids an
// import cycle: api must not import the concrete progress.Engine). *progress.Engine
// satisfies it structurally. Report builds the deterministic trends; Analyze adds
// the claude -p read with deterministic fallback.
type Progress interface {
	Report(ctx context.Context, weeks int) (progress.ProgressReport, error)
	Analyze(ctx context.Context, weeks int) (progress.ProgressRead, error)
}
```
Add the field to `Deps` (after `Pusher Pusher`):
```go
	Pusher   Pusher   // M2: push transport
	Progress Progress // M3.1: progress engine (GET /api/progress, POST /api/progress/analyze)
```

- [ ] **Step 4: GREEN — inject the fake in `newTestServer`.** In `/home/jake/project/help-my-run/backend/internal/api/handlers_test.go`, add to the `deps := Deps{...}` literal (after `Pusher: &fakePusher{},`):
```go
		Pusher:   &fakePusher{},
		Progress: &fakeProgress{},
```

- [ ] **Step 5: RUN (expect PASS — package compiles, existing tests green).**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ 2>&1 | tail -5
```
Expected: `ok  	help-my-run/backend/internal/api`.

- [ ] **Step 6: COMMIT.**
```
git add backend/internal/api/router.go backend/internal/api/m2_fakes_test.go backend/internal/api/handlers_test.go && git commit -m "feat(api): add Progress seam interface and Deps.Progress field (M3.1)"
```

---

### Task 17: `GET /api/progress` + `POST /api/progress/analyze` handlers + routes

**Files:**
- Create: `/home/jake/project/help-my-run/backend/internal/api/progress_handlers.go` (`progress`, `analyzeProgress` handlers, `analyzeProgressRequest`)
- Modify: `/home/jake/project/help-my-run/backend/internal/api/router.go` (register 2 routes inside the bearer `r.Group`, after `r.Post("/api/agent/run", h.agentRun)`)
- Create: `/home/jake/project/help-my-run/backend/internal/api/progress_handlers_test.go` (httptest handler tests w/ `fakeProgress`)

Depends on: Task 16 (`Progress` seam, `fakeProgress`), and `progress.DefaultWeeks`/`MinWeeks`/`MaxWeeks` (Task 10).

- [ ] **Step 1: RED — write handler tests (FULL).** Create `/home/jake/project/help-my-run/backend/internal/api/progress_handlers_test.go`:
```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"help-my-run/backend/internal/progress"
	"help-my-run/backend/internal/store"
	"help-my-run/backend/internal/strava"
)

// newProgressServer wires a server whose Progress seam is the given fake.
func newProgressServer(t *testing.T, fp *fakeProgress) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "prog-api.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	deps := Deps{
		Store:    s,
		Strava:   strava.NewWithBase("1", "x", "http://cb", "http://unused"),
		APIToken: testToken,
		SyncFunc: func(ctx context.Context) (string, int, *string, string, int, *string) {
			return "ok", 0, nil, "ok", 0, nil
		},
		Coach:    &fakeCoach{},
		ImageDir: t.TempDir(),
		Agent:    &fakeAgent{},
		Pusher:   &fakePusher{},
		Progress: fp,
	}
	return NewRouter(deps)
}

func TestProgressRequiresAuth(t *testing.T) {
	h := newProgressServer(t, &fakeProgress{})
	rec := do(t, h, http.MethodGet, "/api/progress", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error":"unauthorized"`) {
		t.Errorf("body = %q, want unauthorized", rec.Body.String())
	}
}

func TestProgressHandlerServesReport(t *testing.T) {
	cur := 330.0
	base := 350.0
	delta := -20.0
	fp := &fakeProgress{report: progress.ProgressReport{
		Weeks:       12,
		GeneratedAt: "2026-06-21T07:00:00Z",
		EnoughData:  true,
		Signals: []progress.TrendSummary{{
			Key: "pace_at_hr", Label: "Pace @ Z2 HR", Unit: "s/km",
			Current: &cur, Baseline: &base, DeltaAbs: &delta,
			Direction: progress.DirectionDown, LowerIsBetter: true,
			Series: []*float64{&base, nil, &cur},
		}},
	}}
	h := newProgressServer(t, fp)

	rec := do(t, h, http.MethodGet, "/api/progress?weeks=12", testToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body progress.ProgressReport
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Weeks != 12 || !body.EnoughData || len(body.Signals) != 1 {
		t.Errorf("report = %+v", body)
	}
	if body.Signals[0].Key != "pace_at_hr" || body.Signals[0].Direction != progress.DirectionDown {
		t.Errorf("signal = %+v", body.Signals[0])
	}
	// snake_case wire tags present.
	if !strings.Contains(rec.Body.String(), `"delta_abs"`) || !strings.Contains(rec.Body.String(), `"lower_is_better"`) {
		t.Errorf("wire JSON not snake_case: %s", rec.Body.String())
	}
	if fp.lastWeeks != 12 {
		t.Errorf("weeks passed = %d, want 12", fp.lastWeeks)
	}
}

func TestProgressHandlerClampsWeeks(t *testing.T) {
	fp := &fakeProgress{}
	h := newProgressServer(t, fp)
	// weeks=999 clamps to MaxWeeks (52).
	do(t, h, http.MethodGet, "/api/progress?weeks=999", testToken)
	if fp.lastWeeks != progress.MaxWeeks {
		t.Errorf("weeks = %d, want %d (clamped)", fp.lastWeeks, progress.MaxWeeks)
	}
	// weeks=1 clamps to MinWeeks (4).
	do(t, h, http.MethodGet, "/api/progress?weeks=1", testToken)
	if fp.lastWeeks != progress.MinWeeks {
		t.Errorf("weeks = %d, want %d (clamped)", fp.lastWeeks, progress.MinWeeks)
	}
	// no param -> default 12.
	do(t, h, http.MethodGet, "/api/progress", testToken)
	if fp.lastWeeks != progress.DefaultWeeks {
		t.Errorf("weeks = %d, want %d (default)", fp.lastWeeks, progress.DefaultWeeks)
	}
}

func TestProgressHandlerReportError(t *testing.T) {
	fp := &fakeProgress{reportErr: context.DeadlineExceeded}
	h := newProgressServer(t, fp)
	rec := do(t, h, http.MethodGet, "/api/progress", testToken)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 on report error", rec.Code)
	}
}

func TestAnalyzeProgressRequiresAuth(t *testing.T) {
	h := newProgressServer(t, &fakeProgress{})
	rec := do(t, h, http.MethodPost, "/api/progress/analyze", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestAnalyzeProgressHandlerAI(t *testing.T) {
	fp := &fakeProgress{read: progress.ProgressRead{Text: "Engine improving.", Source: "ai"}}
	h := newProgressServer(t, fp)
	rec := doBody(t, h, http.MethodPost, "/api/progress/analyze", testToken, `{"weeks":12}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var body progress.ProgressRead
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Text != "Engine improving." || body.Source != "ai" {
		t.Errorf("read = %+v, want ai/Engine improving.", body)
	}
	if fp.lastWeeks != 12 {
		t.Errorf("weeks = %d, want 12", fp.lastWeeks)
	}
}

func TestAnalyzeProgressEmptyBodyDefaults(t *testing.T) {
	fp := &fakeProgress{read: progress.ProgressRead{Text: "x", Source: "fallback"}}
	h := newProgressServer(t, fp)
	// Empty body -> weeks defaults to 12.
	rec := doBody(t, h, http.MethodPost, "/api/progress/analyze", testToken, ``)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if fp.lastWeeks != 12 {
		t.Errorf("weeks = %d, want 12 (default on empty body)", fp.lastWeeks)
	}
}

func TestAnalyzeProgressOutOfRangeDefaults(t *testing.T) {
	fp := &fakeProgress{}
	h := newProgressServer(t, fp)
	// weeks=2 (< MinWeeks) -> defaults to 12.
	doBody(t, h, http.MethodPost, "/api/progress/analyze", testToken, `{"weeks":2}`)
	if fp.lastWeeks != 12 {
		t.Errorf("weeks = %d, want 12 (out-of-range -> default)", fp.lastWeeks)
	}
}

func TestAnalyzeProgressError(t *testing.T) {
	fp := &fakeProgress{analyzeErr: context.DeadlineExceeded}
	h := newProgressServer(t, fp)
	rec := doBody(t, h, http.MethodPost, "/api/progress/analyze", testToken, `{"weeks":12}`)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 on analyze error", rec.Code)
	}
}
```
NOTE: use the existing test helpers `do`/`doBody`/`testToken` from `handlers_test.go`; if the existing `newProgressServer`-style helper or `Deps` literal differs in field names, mirror the project's current `newTestServer` exactly. `strava.NewWithBase(...)` signature must match the one used in `handlers_test.go`.

- [ ] **Step 2: RUN (expect FAIL — handlers undefined).**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ -run Progress 2>&1 | head
```
Expected: compile error `h.progress undefined` / `h.analyzeProgress undefined`.

- [ ] **Step 3: GREEN — write the handlers (FULL).** Create `/home/jake/project/help-my-run/backend/internal/api/progress_handlers.go`:
```go
package api

import (
	"encoding/json"
	"net/http"

	"help-my-run/backend/internal/progress"
)

// progress serves GET /api/progress?weeks=12 — the deterministic trend report
// (no Claude). The progress.ProgressReport is serialized directly (its json tags
// are snake_case, like FitnessMetrics served at /api/fitness).
func (h *handlers) progress(w http.ResponseWriter, r *http.Request) {
	weeks := clampQuery(r, "weeks", progress.DefaultWeeks, progress.MinWeeks, progress.MaxWeeks)
	rep, err := h.d.Progress.Report(r.Context(), weeks)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// analyzeProgressRequest is the optional POST /api/progress/analyze body.
type analyzeProgressRequest struct {
	Weeks int `json:"weeks"`
}

// analyzeProgress serves POST /api/progress/analyze — the claude -p read over the
// computed trends, with a deterministic fallback (handled in the engine). An
// absent/invalid window defaults to DefaultWeeks, clamped to [MinWeeks,MaxWeeks].
func (h *handlers) analyzeProgress(w http.ResponseWriter, r *http.Request) {
	var req analyzeProgressRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // empty body OK
	weeks := req.Weeks
	if weeks < progress.MinWeeks || weeks > progress.MaxWeeks {
		weeks = progress.DefaultWeeks
	}
	read, err := h.d.Progress.Analyze(r.Context(), weeks)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, read)
}
```

- [ ] **Step 4: GREEN — register the routes.** In `/home/jake/project/help-my-run/backend/internal/api/router.go`, inside the bearer `r.Group`, after `r.Post("/api/agent/run", h.agentRun)`:
```go
		r.Post("/api/agent/run", h.agentRun)

		// M3.1
		r.Get("/api/progress", h.progress)
		r.Post("/api/progress/analyze", h.analyzeProgress)
```

- [ ] **Step 5: RUN (expect PASS — full api package).**
```
cd /home/jake/project/help-my-run/backend && go test ./internal/api/ 2>&1 | tail -5
```
Expected: `ok  	help-my-run/backend/internal/api`.

- [ ] **Step 6: COMMIT.**
```
cd /home/jake/project/help-my-run && git add backend/internal/api/progress_handlers.go backend/internal/api/progress_handlers_test.go backend/internal/api/router.go && git commit -m "feat(api): add GET /api/progress and POST /api/progress/analyze handlers (M3.1)"
```

---

### Task 18: `main.go` wiring — construct `progress.Engine`, inject into `api.Deps.Progress`, add to `App`

**Files:**
- Modify: `/home/jake/project/help-my-run/backend/cmd/server/main.go` (`Wire`: construct `progressEngine`, inject `Progress`, add `App.Progress` field; reuses existing `llmClient` + `cfg.ClaudeModel`)
- Modify: `/home/jake/project/help-my-run/backend/cmd/server/main_test.go` (assert the wired progress endpoint + `App.Progress != nil`)

Depends on: Tasks 16–17 (seam + handlers) and `progress.New` (Task 15). No config addition (§7.1).

- [ ] **Step 1: RED — add the wired-graph assertion test (FULL).** Append to `/home/jake/project/help-my-run/backend/cmd/server/main_test.go`:
```go
func TestWireInjectsProgress(t *testing.T) {
	app, err := Wire(testCfg(t))
	if err != nil {
		t.Fatalf("Wire error = %v", err)
	}
	defer func() { _ = app.Store.Close() }()

	if app.Progress == nil {
		t.Error("app.Progress = nil, want a wired *progress.Engine")
	}

	// GET /api/progress is bearer-protected and served by the injected engine ->
	// 200 (computes from an empty store; enough_data:false, no claude needed).
	req := httptest.NewRequest(http.MethodGet, "/api/progress?weeks=12", nil)
	req.Header.Set("Authorization", "Bearer tok")
	rec := httptest.NewRecorder()
	app.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/progress = %d, want 200 (progress engine wired)", rec.Code)
	}
}
```
NOTE: match the existing `main_test.go` helpers — use the project's actual `testCfg(t)` and the `App` field that holds the handler (`app.Handler` here; if the codebase names it differently, e.g. `app.Router`, use that). The bearer token must match what `testCfg` sets (`"tok"` here is illustrative).

- [ ] **Step 2: RUN (expect FAIL — `App.Progress` undefined).**
```
cd /home/jake/project/help-my-run/backend && go test ./cmd/server/ -run Progress 2>&1 | head
```
Expected: compile error `app.Progress undefined (type *App has no field or method Progress)`.

- [ ] **Step 3: GREEN — add the `App.Progress` field.** In `/home/jake/project/help-my-run/backend/cmd/server/main.go`, add `"help-my-run/backend/internal/progress"` to the import block, then add the field to the `App` struct (after `Pusher *push.Client`):
```go
	Pusher   *push.Client     // M2: Expo push transport
	Progress *progress.Engine // M3.1: deterministic trends + claude -p read
```

- [ ] **Step 4: GREEN — construct + inject the engine in `Wire`.** In `Wire`, after `coachEngine := coach.New(...)`, add:
```go
	coachEngine := coach.New(s, llmClient, cfg.ClaudeModel, cfg.ImageDir)
	progressEngine := progress.New(s, llmClient, cfg.ClaudeModel)
```
Add `Progress` to the `api.NewRouter(api.Deps{...})` literal (after `Pusher: pushClient,`):
```go
		Pusher:   pushClient,
		Progress: progressEngine,
	})
```
Add `Progress` to the returned `&App{...}` literal (after `Pusher: pushClient,`):
```go
		Pusher:   pushClient,
		Progress: progressEngine,
	}, nil
```
NOTE: mirror the exact variable names used in the current `Wire` (`s`, `llmClient`, `pushClient`, `cfg.ClaudeModel`); if they differ, use the project's actual identifiers.

- [ ] **Step 5: RUN (expect PASS — full server cmd).**
```
cd /home/jake/project/help-my-run/backend && go test ./cmd/server/ 2>&1 | tail -5
```
Expected: `ok  	help-my-run/backend/cmd/server`.

- [ ] **Step 6: RUN (expect PASS — whole backend).**
```
cd /home/jake/project/help-my-run/backend && go test ./... 2>&1 | tail -15
```
Expected: all packages `ok` (no `FAIL`), including `internal/api`, `internal/progress`, `cmd/server`.

- [ ] **Step 7: COMMIT.**
```
git add backend/cmd/server/main.go backend/cmd/server/main_test.go && git commit -m "feat(server): wire progress.Engine into api.Deps.Progress and App (M3.1)"
```

---

### Task 19: App — Sparkline helper + types + unit test

**Files:**
- Create: `/home/jake/project/help-my-run/app/src/lib/sparkline.ts`
- Create: `/home/jake/project/help-my-run/app/src/lib/__tests__/sparkline.test.ts`
- Modify: `/home/jake/project/help-my-run/app/src/api/types.ts` (append M3.1 types after the M2 block, line 147)

- [ ] **Step 1: Write the failing sparkline unit test.**
Create `/home/jake/project/help-my-run/app/src/lib/__tests__/sparkline.test.ts`:
```ts
import { sparkline } from '../sparkline';

describe('sparkline', () => {
  it('scales a strictly increasing series across the block ramp (min/mid/max)', () => {
    // 7-level ramp ▁▂▃▄▅▆▇; idx = round((v-min)/(max-min) * 6)
    // [1,2,3]: 1->0 (▁ ▁), 2->3 (▄ ▄), 3->6 (▇ ▇)
    expect(sparkline([1, 2, 3])).toBe('▁▄▇');
  });

  it('renders null gaps as a blank space and a flat series as mid-level blocks', () => {
    // [5,null,5]: finite values are flat (span 0) -> mid block ▄ (▄); null -> ' '
    expect(sparkline([5, null, 5])).toBe('▄ ▄');
  });

  it('returns an empty string for an empty series', () => {
    expect(sparkline([])).toBe('');
  });

  it('renders an all-null series as all blanks (length preserved)', () => {
    expect(sparkline([null, null])).toBe('  ');
  });

  it('treats undefined and NaN as gaps', () => {
    expect(sparkline([undefined, NaN, 5])).toBe('  ▄');
  });

  it('always returns a string whose length equals the series length', () => {
    const series = [350, null, 345, 342, null, 340, 338];
    expect(sparkline(series)).toHaveLength(series.length);
  });
});
```

- [ ] **Step 2: Run the test — expect FAIL (module not found).**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/lib/__tests__/sparkline.test.ts --watchAll=false
```
Expected: FAIL — `Cannot find module '../sparkline' from 'src/lib/__tests__/sparkline.test.ts'`.

- [ ] **Step 3: Implement the sparkline helper (minimal, pure, no deps).**
Create `/home/jake/project/help-my-run/app/src/lib/sparkline.ts`:
```ts
// 7 block levels. Index 0 = lowest, 6 = highest. Full 7-level ramp ▁▂▃▄▅▆▇
// (superset of the spec §8 6-glyph example; finer resolution).
const BLOCKS = ['▁', '▂', '▃', '▄', '▅', '▆', '▇'];
const GAP = ' ';

/**
 * Render a numeric series as a unicode-block sparkline.
 * null/undefined/NaN entries become a blank (gap), never a fabricated point.
 * Output string length === series.length. A finite series with no spread
 * renders all mid-level blocks.
 */
export function sparkline(series: (number | null | undefined)[]): string {
  const finite = series.filter(
    (v): v is number => typeof v === 'number' && Number.isFinite(v),
  );
  if (finite.length === 0) return series.map(() => GAP).join('');

  const min = Math.min(...finite);
  const max = Math.max(...finite);
  const span = max - min;
  const last = BLOCKS.length - 1;

  return series
    .map((v) => {
      if (typeof v !== 'number' || !Number.isFinite(v)) return GAP;
      if (span === 0) return BLOCKS[Math.floor(last / 2)]; // flat series -> mid
      const idx = Math.round(((v - min) / span) * last);
      return BLOCKS[idx];
    })
    .join('');
}
```

- [ ] **Step 4: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/lib/__tests__/sparkline.test.ts --watchAll=false
```
Expected: PASS — `Tests: 6 passed, 6 total`.

- [ ] **Step 5: Add M3.1 progress types to `types.ts`.**
Append to `/home/jake/project/help-my-run/app/src/api/types.ts` after line 147 (end of `RunResult`, the M2 block):
```ts

// --- M3.1 progress types (snake_case wire JSON; mirror the Go DTO exactly) ---

export type TrendDirection = 'up' | 'down' | 'flat';

export interface TrendSummary {
  key: string;            // 'pace_at_hr' | 'vo2max' | 'resting_hr' | 'hrv_baseline' | 'weekly_load'
  label: string;
  unit: string;           // 's/km' | 'ml/kg/min' | 'bpm' | 'ms' | 'km'
  current: number | null;
  baseline: number | null;
  delta_abs: number | null;
  direction: TrendDirection;
  lower_is_better: boolean;
  series: (number | null)[]; // len == weeks; oldest-first; null = gap
}

export interface ProgressReport {
  weeks: number;
  generated_at: string;
  enough_data: boolean;
  signals: TrendSummary[];
}

export interface ProgressRead {
  text: string;
  source: 'ai' | 'fallback';
}
```
Note: `types.ts` carries no runtime code; the hook + screen tests (Tasks 20, 21) exercise these via `import type`. Verify the file still compiles in Step 6.

- [ ] **Step 6: Run the full app suite to confirm no regressions (types compile).**
```bash
cd /home/jake/project/help-my-run/app && npm test -- --watchAll=false
```
Expected: PASS — all existing suites + the new `sparkline.test.ts` pass; `0 failed`.

- [ ] **Step 7: Commit.**
```bash
cd /home/jake/project/help-my-run/app && git add src/lib/sparkline.ts src/lib/__tests__/sparkline.test.ts src/api/types.ts && git commit -m "feat(app): add unicode sparkline helper + M3.1 progress types

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 20: App — useProgress + useAnalyzeProgress hooks

**Files:**
- Modify: `/home/jake/project/help-my-run/app/src/api/hooks.ts` (import list lines 8-22; append two hooks after `useRegisterPushToken`, line 167)
- Modify: `/home/jake/project/help-my-run/app/src/api/__tests__/hooks.test.tsx` (import list lines 13-41; append two `describe` blocks)

Depends on: Task 19 (types).

- [ ] **Step 1: Write the failing hook tests.**
Append to `/home/jake/project/help-my-run/app/src/api/__tests__/hooks.test.tsx` (end of file, after the `useRegisterPushToken` describe at line 319):
```ts

describe('useProgress', () => {
  it('fetches /api/progress with default weeks 12', async () => {
    const data: ProgressReport = {
      weeks: 12,
      generated_at: '2026-06-21T07:00:00Z',
      enough_data: true,
      signals: [],
    };
    mockApiGet.mockResolvedValue(data);
    const { result } = await renderHook(() => useProgress(), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/progress?weeks=12');
    expect(result.current.data).toEqual(data);
  });

  it('fetches /api/progress with an explicit weeks value', async () => {
    mockApiGet.mockResolvedValue({
      weeks: 8, generated_at: '2026-06-21T07:00:00Z', enough_data: false, signals: [],
    });
    const { result } = await renderHook(() => useProgress(8), { wrapper: createWrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiGet).toHaveBeenCalledWith('/api/progress?weeks=8');
  });
});

describe('useAnalyzeProgress', () => {
  it('POSTs /api/progress/analyze with the weeks body and returns the read', async () => {
    const read: ProgressRead = { text: 'Your engine is improving.', source: 'ai' };
    mockApiPost.mockResolvedValue(read);
    const { result } = await renderHook(() => useAnalyzeProgress(), { wrapper: createWrapper() });
    await act(async () => { result.current.mutate({ weeks: 12 }); });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(mockApiPost).toHaveBeenCalledWith('/api/progress/analyze', { weeks: 12 });
    expect(result.current.data).toEqual(read);
  });
});
```
Also add the two hooks to the import list at lines 13-28 (after `useRegisterPushToken,`):
```ts
  useRegisterPushToken,
  useProgress,
  useAnalyzeProgress,
```
And add the two types to the type import list at lines 29-41 (after `PushRegisterRequest,`):
```ts
  PushRegisterRequest,
  ProgressReport,
  ProgressRead,
```

- [ ] **Step 2: Run the test — expect FAIL (undefined hooks).**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/hooks.test.tsx --watchAll=false
```
Expected: FAIL — `useProgress is not a function` / `useAnalyzeProgress is not a function`.

- [ ] **Step 3: Add the type imports to `hooks.ts`.**
Edit the `import type {...}` block in `/home/jake/project/help-my-run/app/src/api/hooks.ts` (lines 8-22) — add the two types after `PushRegisterResponse,` (line 21):
```ts
  PushRegisterRequest,
  PushRegisterResponse,
  ProgressReport,
  ProgressRead,
} from './types';
```

- [ ] **Step 4: Append the two hooks to `hooks.ts`.**
Append after `useRegisterPushToken` (end of file, line 167):
```ts

export function useProgress(weeks = 12) {
  return useQuery({
    queryKey: ['progress', weeks],
    queryFn: () => apiGet<ProgressReport>(`/api/progress?weeks=${weeks}`),
  });
}

export function useAnalyzeProgress() {
  return useMutation({
    mutationFn: (body: { weeks?: number }) =>
      apiPost<ProgressRead>('/api/progress/analyze', body),
  });
}
```
Note: no `onSuccess`/invalidation on the mutation — the read is shown from `mutation.data`. `apiGet`/`apiPost`/`useQuery`/`useMutation` are already imported (lines 1-7).

- [ ] **Step 5: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest src/api/__tests__/hooks.test.tsx --watchAll=false
```
Expected: PASS — all `hooks.test.tsx` suites including the 3 new tests; `0 failed`.

- [ ] **Step 6: Commit.**
```bash
cd /home/jake/project/help-my-run/app && git add src/api/hooks.ts src/api/__tests__/hooks.test.tsx && git commit -m "feat(app): add useProgress query + useAnalyzeProgress mutation hooks

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 21: App — Progress screen (trend cards, analyze button, empty state)

**Files:**
- Create: `/home/jake/project/help-my-run/app/app/progress.tsx` (default export `ProgressScreen`, route `/progress`)
- Create: `/home/jake/project/help-my-run/app/app/__tests__/progress.test.tsx`

Depends on: Task 19 (sparkline + types), Task 20 (hooks).

- [ ] **Step 1: Write the failing screen test.**
Create `/home/jake/project/help-my-run/app/app/__tests__/progress.test.tsx`:
```tsx
import React from 'react';
import { render, fireEvent, act } from '@testing-library/react-native';
import type { ProgressReport, ProgressRead } from '../../src/api/types';

const progress: ProgressReport = {
  weeks: 12,
  generated_at: '2026-06-21T07:00:00Z',
  enough_data: true,
  signals: [
    {
      key: 'pace_at_hr', label: 'Pace @ Z2 HR', unit: 's/km',
      current: 330, baseline: 350, delta_abs: -20,
      direction: 'down', lower_is_better: true,
      series: [350, null, 345, 340, null, 335, 330],
    },
    {
      key: 'vo2max', label: 'VO2max', unit: 'ml/kg/min',
      current: 52, baseline: 50, delta_abs: 2,
      direction: 'up', lower_is_better: false,
      series: [50, 50, 51, null, 51, 52, 52],
    },
  ],
};

const read: ProgressRead = { text: 'Your engine is improving.', source: 'ai' };

const mockAnalyze = jest.fn();

// Mutable mock state so the empty-state / analyzed cases swap return values
// without jest.resetModules() (which forks React under jest-expo and breaks the
// reconciler). Default = happy path.
const mockHookState: {
  progress: { data: ProgressReport | undefined; isPending: boolean; isError: boolean };
  analyze: { mutate: jest.Mock; data: ProgressRead | undefined; isPending: boolean };
} = {
  progress: { data: progress, isPending: false, isError: false },
  analyze: { mutate: mockAnalyze, data: undefined, isPending: false },
};

jest.mock('expo-router', () => {
  const { Text: RNText } = require('react-native');
  return {
    Link: ({ children }: { children: React.ReactNode }) => <RNText>{children}</RNText>,
    Stack: { Screen: () => null },
    useLocalSearchParams: () => ({}), // default 12 weeks
  };
});

jest.mock('../../src/api/hooks', () => ({
  useProgress: () => mockHookState.progress,
  useAnalyzeProgress: () => mockHookState.analyze,
}));

import ProgressScreen from '../progress';

afterEach(() => {
  jest.clearAllMocks();
  mockHookState.progress = { data: progress, isPending: false, isError: false };
  mockHookState.analyze = { mutate: mockAnalyze, data: undefined, isPending: false };
});

describe('ProgressScreen', () => {
  it('renders one trend card per signal', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-card-pace_at_hr')).toBeTruthy();
    expect(getByTestId('progress-card-vo2max')).toBeTruthy();
  });

  it('renders the label, current value, and delta vs window start per card', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    const pace = getByTestId('progress-card-pace_at_hr').props.children;
    const flat = JSON.stringify(pace);
    expect(flat).toContain('Pace @ Z2 HR');
    expect(flat).toContain('330');
    expect(flat).toContain('s/km');
  });

  it('shows an improving arrow for pace (lower_is_better + direction down)', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-arrow-pace_at_hr').props.children).toBe('↓');
  });

  it('shows an improving arrow for vo2max (higher is better + direction up)', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-arrow-vo2max').props.children).toBe('↑');
  });

  it('renders a sparkline string whose length equals the series length (gaps blank)', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-spark-pace_at_hr').props.children).toHaveLength(7);
  });

  it('runs analyze with the window when the button is pressed', async () => {
    const { getByTestId } = await render(<ProgressScreen />);
    await act(async () => {
      fireEvent.press(getByTestId('btn-analyze-progress'));
    });
    expect(mockAnalyze).toHaveBeenCalledTimes(1);
    expect(mockAnalyze).toHaveBeenCalledWith({ weeks: 12 });
  });

  it('shows the coach read footer after analyze returns', async () => {
    mockHookState.analyze = { mutate: mockAnalyze, data: read, isPending: false };
    const { getByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-read').props.children).toContain('Your engine is improving.');
  });

  it('does NOT render the empty state on the happy path', async () => {
    const { queryByTestId } = await render(<ProgressScreen />);
    expect(queryByTestId('progress-empty')).toBeNull();
  });
});

describe('ProgressScreen — not enough data', () => {
  it('shows the empty state and no cards when enough_data is false', async () => {
    mockHookState.progress = {
      data: { weeks: 12, generated_at: '2026-06-21T07:00:00Z', enough_data: false, signals: [] },
      isPending: false, isError: false,
    };
    const { getByTestId, queryByTestId } = await render(<ProgressScreen />);
    expect(getByTestId('progress-empty')).toBeTruthy();
    expect(queryByTestId('progress-card-pace_at_hr')).toBeNull();
  });
});
```

- [ ] **Step 2: Run the test — expect FAIL (module not found).**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/progress.test.tsx --watchAll=false
```
Expected: FAIL — `Cannot find module '../progress' from 'app/__tests__/progress.test.tsx'`.

- [ ] **Step 3: Implement the Progress screen.**
Create `/home/jake/project/help-my-run/app/app/progress.tsx`:
```tsx
import React from 'react';
import { View, Text, ScrollView, Pressable, StyleSheet } from 'react-native';
import { useLocalSearchParams } from 'expo-router';
import { useProgress, useAnalyzeProgress } from '../src/api/hooks';
import { sparkline } from '../src/lib/sparkline';
import type { TrendSummary } from '../src/api/types';

// Map raw value-direction + lower_is_better to a glyph the user reads as
// improving/worsening. The arrow reflects the VALUE movement (↑ value up,
// ↓ value down, → flat); color reflects improvement.
function arrowGlyph(s: TrendSummary): string {
  if (s.direction === 'flat') return '→';
  return s.direction === 'up' ? '↑' : '↓';
}

// improving = value moved in the better direction for this signal.
function isImproving(s: TrendSummary): boolean {
  if (s.direction === 'flat') return false;
  const valueWentUp = s.direction === 'up';
  return s.lower_is_better ? !valueWentUp : valueWentUp;
}

function fmtNum(v: number | null): string {
  return v == null ? '—' : String(v);
}

function fmtDelta(v: number | null): string {
  if (v == null) return '—';
  return v > 0 ? `+${v}` : String(v);
}

function TrendCard({ signal }: { signal: TrendSummary }) {
  const improving = isImproving(signal);
  return (
    <View testID={`progress-card-${signal.key}`} style={styles.card}>
      <View style={styles.cardHeaderRow}>
        <Text style={styles.cardLabel}>{signal.label}</Text>
        <Text
          testID={`progress-arrow-${signal.key}`}
          style={[styles.arrow, { color: improving ? '#1b8a3a' : '#c0392b' }]}
        >
          {arrowGlyph(signal)}
        </Text>
      </View>
      <Text style={styles.cardCurrent}>
        {fmtNum(signal.current)} {signal.unit}
      </Text>
      <Text style={styles.cardDelta}>
        {fmtDelta(signal.delta_abs)} {signal.unit} vs {fmtNum(signal.baseline)} (start)
      </Text>
      <Text testID={`progress-spark-${signal.key}`} style={styles.spark}>
        {sparkline(signal.series)}
      </Text>
    </View>
  );
}

export default function ProgressScreen() {
  const params = useLocalSearchParams<{ weeks?: string }>();
  const weeks = typeof params.weeks === 'string' ? Number(params.weeks) : 12;
  const progress = useProgress(weeks);
  const analyze = useAnalyzeProgress();

  const report = progress.data;
  const enoughData = report?.enough_data ?? false;

  return (
    <ScrollView contentContainerStyle={styles.container}>
      <Text style={styles.heading}>Progress</Text>
      {progress.isPending ? <Text style={styles.loading}>Loading…</Text> : null}

      {report && !enoughData ? (
        <Text testID="progress-empty" style={styles.empty}>
          Not enough data yet — keep logging runs and syncing Garmin.
        </Text>
      ) : null}

      {enoughData
        ? report!.signals.map((s) => <TrendCard key={s.key} signal={s} />)
        : null}

      <Pressable
        testID="btn-analyze-progress"
        style={styles.button}
        disabled={analyze.isPending}
        onPress={() => analyze.mutate({ weeks })}
      >
        <Text style={styles.buttonText}>
          {analyze.isPending ? 'Analyzing…' : 'Analyze progress'}
        </Text>
      </Pressable>

      <Text style={styles.heading}>Coach read</Text>
      <Text testID="progress-read" style={styles.read}>
        {analyze.data?.text ?? '—'}
      </Text>
      {analyze.data?.source ? (
        <Text testID="progress-read-source" style={styles.readSource}>
          source: {analyze.data.source}
        </Text>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { padding: 16, gap: 6 },
  heading: { fontSize: 18, fontWeight: '600', marginTop: 16 },
  loading: { fontSize: 14, color: '#666' },
  empty: { fontSize: 14, color: '#999', paddingVertical: 8 },
  card: {
    borderWidth: StyleSheet.hairlineWidth, borderColor: '#ddd', borderRadius: 10,
    padding: 12, gap: 4, backgroundColor: '#fafafa', marginTop: 8,
  },
  cardHeaderRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between' },
  cardLabel: { fontSize: 16, fontWeight: '600', color: '#222' },
  arrow: { fontSize: 18, fontWeight: '700' },
  cardCurrent: { fontSize: 20, fontWeight: '700', color: '#222' },
  cardDelta: { fontSize: 13, color: '#666' },
  spark: { fontFamily: 'monospace', fontSize: 16, letterSpacing: 1, color: '#fc4c02' },
  button: {
    backgroundColor: '#fc4c02', borderRadius: 8, paddingVertical: 12,
    alignItems: 'center', marginTop: 16,
  },
  buttonText: { color: '#fff', fontSize: 16, fontWeight: '600' },
  read: { fontSize: 14, color: '#444', fontStyle: 'italic' },
  readSource: { fontSize: 12, color: '#999' },
});
```

- [ ] **Step 4: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/progress.test.tsx --watchAll=false
```
Expected: PASS — `ProgressScreen` (8 tests) + `ProgressScreen — not enough data` (1 test); `0 failed`.

- [ ] **Step 5: Commit.**
```bash
cd /home/jake/project/help-my-run/app && git add app/progress.tsx app/__tests__/progress.test.tsx && git commit -m "feat(app): add Progress screen with trend cards, sparklines, analyze read + empty state

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 22: App — Register Progress route + home nav link

**Files:**
- Modify: `/home/jake/project/help-my-run/app/app/_layout.tsx` (add `<Stack.Screen name="progress" .../>` after the `plan-view` entry, line 23)
- Modify: `/home/jake/project/help-my-run/app/app/index.tsx` (add `<Link href="/progress">` after the Profile link, lines 106-108)
- Modify: `/home/jake/project/help-my-run/app/app/__tests__/index.test.tsx` (add a Progress-link assertion)

Depends on: Task 21 (the screen route must exist before `_layout` registers it).

- [ ] **Step 1: Add the failing nav-link assertion to the home screen test.**
Add a new test inside the existing `describe('HomeScreen', ...)` block in `/home/jake/project/help-my-run/app/app/__tests__/index.test.tsx` (after the `renders a link to view this week's plan` test, around line 139):
```ts
  it('renders a navigation link to Progress', async () => {
    const { getByText } = await render(<HomeScreen />);
    expect(getByText('Progress')).toBeTruthy();
  });
```

- [ ] **Step 2: Run the test — expect FAIL (link not rendered).**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/index.test.tsx --watchAll=false
```
Expected: FAIL — `Unable to find an element with text: Progress`.

- [ ] **Step 3: Add the Progress nav link to the home screen.**
Edit `/home/jake/project/help-my-run/app/app/index.tsx` — insert after the Profile `<Link>` block (lines 106-108):
```tsx
      <Link href="/profile" style={styles.link}>
        Profile
      </Link>
      <Link href="/progress" style={styles.link}>
        Progress
      </Link>
```
(`styles.link` already exists at line 152: `{ fontSize: 15, color: '#fc4c02', marginTop: 8 }`.)

- [ ] **Step 4: Run the test — expect PASS.**
```bash
cd /home/jake/project/help-my-run/app && npx jest app/__tests__/index.test.tsx --watchAll=false
```
Expected: PASS — all `HomeScreen` tests including the new Progress-link test; `0 failed`.

- [ ] **Step 5: Register the Progress screen title in the Stack navigator.**
Edit `/home/jake/project/help-my-run/app/app/_layout.tsx` — add after the `plan-view` entry (line 23):
```tsx
        <Stack.Screen name="plan-view" options={{ title: 'Weekly plan' }} />
        <Stack.Screen name="progress" options={{ title: 'Progress' }} />
```
Note: `_layout.tsx` has no dedicated test; this registration is verified by the full-suite run in Step 6 + expo-router file routing.

- [ ] **Step 6: Run the full app suite — expect PASS (no regressions).**
```bash
cd /home/jake/project/help-my-run/app && npm test -- --watchAll=false
```
Expected: PASS — every suite (`sparkline`, `hooks`, `progress`, `index`, `plan-view`, plus all M0/M1/M2 suites); `0 failed`.

- [ ] **Step 7: Commit.**
```bash
cd /home/jake/project/help-my-run/app && git add app/_layout.tsx app/index.tsx app/__tests__/index.test.tsx && git commit -m "feat(app): register Progress route + add home nav link

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Definition of Done

Each M3.1 success criterion (spec §2) maps to its task(s); all automated gates green, then the manual real-data check.

| # | Spec §2 success criterion | Implemented by | Verified by |
|---|---|---|---|
| 1 | Progress screen: per-signal trend cards (current, change vs window start, direction arrow, unicode sparkline), configurable window (default 12 weeks, weekly buckets) | Tasks 10–13 (engine series/summary/buckets), 19 (sparkline + types), 21 (screen), 22 (route/nav) | `progress_test.go` (`TestComputeProgressFullReport`), `sparkline.test.ts`, `progress.test.tsx` (card/arrow/spark testIDs), `handlers`/`hooks` weeks-clamp tests |
| 2 | Pace-at-a-fixed-HR headline signal (weekly-median pace of in-band runs) | Task 7 (metrics export), Task 12 (`paceAtHRSeries` + ref-HR band), Task 13 (signal order/meta) | `TestPaceAtHRSeriesBandAndMedianAndGap`, `TestPaceAtHRSeriesDefaultRefHRWhenProfileNil` |
| 3 | VO2max ingested from Garmin and trended | Tasks 1–9 (migration, store, worker fetch/normalize/cli, Go types, sync upsert + 84d backfill), Task 12 (`vo2maxSeries`) | store/worker/garmin/sync tests; `TestVo2maxSeriesLastInBucket`; `TestSyncGarminUpsertsAllTables` (vo2max count=2); `TestSyncGarminBackfillWindowIs84Days` |
| 4 | Resting-HR, HRV-baseline, weekly-volume/load trends from existing M0/M1 data | Task 12 (`rhrSeries`, `hrvSeries`, `weeklyLoadSeries`), Task 13 | `TestRecoverySeriesMeanAndGap`, `TestWeeklyLoadSeries`, `TestComputeProgressFullReport` (5 signals, fixed order) |
| 5 | "Analyze progress" → `claude -p` narrative read; deterministic templated fallback when Claude unavailable | Task 14 (prompt/input/fallback), Task 15 (`Engine.Analyze`), Task 17 (`POST /api/progress/analyze`), Tasks 20–21 (hook + button + read footer) | `TestAnalyzeAIPath` (source=ai), `TestAnalyzeFallbackOnFailure`/`TestAnalyzeFallbackNotEnoughData` (source=fallback + templated text), `TestFallbackProgressTextImprovedClauses`, `TestAnalyzeProgressHandlerAI`, `progress.test.tsx` (analyze flow) |
| 6 | Thin history degrades gracefully ("not enough data"); no-qualifying-run weeks show a gap, not a fabricated point | Task 13 (`EnoughData` gate), Task 12 (nil gaps), Task 14 (not-enough fallback), Task 21 (empty state) | `TestComputeProgressNotEnoughData`, gap assertions in series tests, `sparkline` gap tests, `progress.test.tsx` empty-state test |

**Auth (spec §6, all routes bearer-protected):** Task 17 — `TestProgressRequiresAuth`, `TestAnalyzeProgressRequiresAuth` (401).

**Full-suite gates (all must be green before M3.1 is done):**
- Backend: `cd /home/jake/project/help-my-run/backend && go test ./...` → all packages `ok`.
- Worker: `cd /home/jake/project/help-my-run/garmin-worker && pytest tests -q` → all passed.
- App: `cd /home/jake/project/help-my-run/app && npm test -- --watchAll=false` → `0 failed`.

**Manual real-data check (spec §9 "Manual"):**
1. Deploy the migrated backend + worker; the `claude` CLI is already logged in.
2. Trigger a Garmin sync (first sync backfills ~84 days incl. VO2max). Confirm `garmin_vo2max` is populated.
3. Open the app → **Progress** screen. Verify real trend cards render with current values, deltas vs window start, direction arrows (pace `↓` shown green = improving), and unicode sparklines (gaps blank for weeks with no in-band run).
4. Tap **Analyze progress**; verify a 2–4 sentence coach read appears with `source: ai`. (Force a Claude failure to confirm the templated fallback renders with `source: fallback`.)
5. With thin history, confirm the "not enough data yet" empty state renders instead of fabricated points.
