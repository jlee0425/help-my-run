# help-my-run — Milestone 1 Design Spec

**Date:** 2026-06-20
**Status:** Approved (M1 detailed)
**Depends on:** M0 Foundation (merged to `main`) — see `2026-06-19-help-my-run-m0-foundation-design.md`
**Author:** Brainstormed with Claude Code

---

## 1. Context

M0 delivered the data foundation: Strava activities + Garmin recovery (sleep/HRV/Body
Battery/RHR) syncing into one SQLite store, exposed via a Go REST API and visible in an
Expo app. M1 builds the first AI layer on top.

M1's shape was set by what the user actually wants (in their words): *"analyze the
garmin/strava reports and help me improve my cardio over time — what workouts should I do
this week? I target ~20 km/week; how should I space them (5k easy, 3k tempo, 5k recovery,
8k long) on which dates, based on my CrossFit calendar?"*

So M1 is **prospective planning**, not retrospective reads: a **CrossFit-aware weekly
run-plan generator**. Supporting analysis (a short fitness read) justifies the plan rather
than being the headline.

### User constraints (captured during brainstorming)
- CrossFit **Mon–Fri**, 18:15–19:15, sometimes +15–30 min accessory work (OHP, pull-up
  skills). **Thursday is a barbell-skill day** (lighter legs/CNS), not hard pushing.
- The box publishes the week's programming as an **image**; the user shares that photo.
- **Evening doubles**: run ~15–30 min after CrossFit (≈20:00). On high-CNS days the user
  skips the run or waits ~2 h. Already fuels CrossFit with a ~16:30–17:00 carb snack.
- Baseline **~20 km/week**, wants to **build cardio over time** (progressive, open-ended —
  no goal race).

## 2. Approach (decided)

**Hybrid: deterministic metrics in Go + AI for vision and planning.** Go computes the
trustworthy numbers (load, paces, recovery trend, safe weekly target); Claude's vision
parses the CrossFit image; Claude composes the dated plan + rationale from a clean context
pack. Rejected alternatives: AI-heavy (unreliable arithmetic, costlier, hard to test) and
rule-based-only (rigid, loses coaching intelligence).

### LLM access: Claude Code headless under the user's subscription (decided)
To avoid metered API billing, the backend calls Claude through **Claude Code in headless
print mode (`claude -p …`)**, which runs under the user's existing Pro/Max subscription —
**no per-token cost**. The Go backend invokes the `claude` CLI **as a subprocess** (the same
pattern as M0's Garmin Python worker). The metered Anthropic API + `anthropic-sdk-go` +
`ANTHROPIC_API_KEY` path is explicitly NOT used (the Claude Agent SDK and raw API both
require a paid API key).

Implications vs the API path (accepted trade-offs):
- **Model**: `claude-opus-4-8`, selected via Claude Code settings (`~/.claude/settings.json`
  or project `.claude/settings.json`).
- **Structured output**: no schema-guaranteed `output_config.format`. Instead, prompt for
  JSON, run with `--output-format json` (clean envelope; `.result` holds the model's text),
  extract the JSON from `.result`, and **parse defensively with one retry** on malformed
  JSON. The parsed CrossFit week is editable anyway, which absorbs occasional imperfection.
- **Vision**: no base64 API image block. The backend saves the uploaded image to disk and
  the prompt instructs `claude -p` to **read that image file** (Claude Code's read tool
  handles images). Requires the image dir to be readable and the read tool permitted
  (working dir / `--add-dir` / allowed-tools config).
- **Setup prerequisite**: the self-hosted host must have the `claude` CLI installed and be
  logged in once (`claude login`). Documented in the README alongside `make garmin-login`.

### Caveats (documented, accepted for single-user personal use)
- **Shared rate limits**: `claude -p` draws on the same 5-hour / weekly subscription limits
  as interactive Claude Code use. ~1 plan/week + occasional regenerates is negligible.
- **ToS**: single-user, self-hosted, personal use is appropriate for the subscription;
  high-volume/multi-user would belong on the metered API.
- **Billing-model risk**: Anthropic announced (then paused, as of mid-June 2026) splitting
  Agent-SDK / `claude -p` usage into separate plan credits. Today it draws from normal
  subscription limits; this could change.

## 3. Goal & success criteria

**Goal:** Upload the week's box programming photo → get a dated, progressive running plan
that places runs intelligently around CrossFit load, with pace targets from real fitness.

1. Upload a CrossFit schedule image → app shows a structured per-day CrossFit read (focus +
   CNS/leg load), **editable** before planning.
2. Generate a plan → a 7-day schedule (each day: run type, distance, pace target,
   evening-double time note, "optional if CNS fried" flag) hitting a safe weekly target,
   with hard runs kept off heavy CrossFit days and quality favored on light days (Thu) +
   weekends.
3. The plan is grounded in **computed fitness** (recent volume, acute:chronic load,
   easy/threshold paces, recovery trend), shown as a short read.
4. **Week-over-week progression** (~≤10% ramp, periodic cutback week) using last week's plan.
5. Regenerate works; profile, parsed weeks, and plans persist.

## 4. Components (new)

- **Metrics engine** (`backend/internal/metrics`) — *deterministic Go*. From M0's
  `activities` + `garmin_*` tables: recent weekly volume (last 4 wks), acute:chronic load
  ratio (7-day vs 28-day distance/time), easy-pace and threshold-pace estimates, recovery
  trend (HRV / sleep score / Body Battery direction over ~14 days), and the **safe
  next-week volume target** (baseline × progression, ≤~10% ramp, cutback every 4th week).
  Pure functions, table-driven tests.
- **Coach engine** (`backend/internal/coach`) — assembles the **context pack** (metrics +
  athlete profile + constraints + last week's plan + parsed CrossFit week) and orchestrates
  the two Claude calls.
- **Claude client** (`backend/internal/llm`) — invokes the `claude` CLI in headless print
  mode (`claude -p … --output-format json`) via `os/exec`; extracts and parses the JSON the
  model returns, with one retry on malformed JSON. The command/runner is **injectable** so
  tests run against a stub that returns canned envelopes (no real `claude`/network in CI).
- **CrossFit ingestion** — image upload → Claude vision → structured per-day CF model.
- **Plan storage** + **athlete profile** (§6).
- **Expo screens** — "Plan my week" (upload → review/edit CF week → generate) and a
  **weekly plan view**; plus a profile/settings screen.

## 5. Two-stage Claude flow

Both stages run via `claude -p --output-format json`; the prompt instructs Claude to emit
**only** a JSON object matching the documented shape, which the Go client extracts from the
envelope's `.result` and parses (one retry on malformed JSON).

**Stage 1 — image → CrossFit week.** The backend writes the uploaded image to disk and the
prompt tells `claude -p` to read that image file. Input also includes the user's known
pattern as hints (CF Mon–Fri, Thu skill/lighter, 18:15–19:15). Output JSON:
`{ week_start, days: [{ date, dow, has_crossfit, focus, cns_load: "low"|"med"|"high",
leg_load: "low"|"med"|"high", notes }] }`. Stored in `crossfit_weeks`; **editable** in the
app before planning.

**Stage 2 — context pack → plan.** A "Coach Brain" instruction block (a CrossFit-aware
running coach: periodization toward general aerobic improvement; ≤~10% weekly ramp with a
cutback week; place hard runs on low-CNS/low-leg CrossFit days + weekends; easy stays easy;
evening-double timing; mark runs optional after high-CNS days) is prepended to the prompt,
followed by the full context pack. Output JSON:
`{ fitness_summary, weekly_target_km, days: [{ date, dow, run_type, distance_km,
pace_target, time_note, optional_if_cns, rationale }], week_rationale, one_flag }`.

Two stages (not one) because image parsing is a distinct, reviewable/editable concern and
keeps each prompt simple and testable.

## 6. Data model (added; SQLite)

- `athlete_profile` — single row: `target_weekly_km`, `progression_mode`
  (`build` | `hold`), `zone2_ceiling_bpm` (nullable), `threshold_bpm` (nullable),
  `max_hr_bpm` (nullable), `run_constraints_json` (days, doubles pref, CNS rule text, CF
  times), `goal_text`, `updated_at`. Nullable HR markers → Claude estimates from data.
- `crossfit_weeks` — `week_start` (PK, ISO date of Monday), `image_path`, `parsed_json`,
  `raw_response`, `created_at`.
- `plans` — `id` (PK), `week_start`, `generated_at`, `status`, `plan_json`,
  `fitness_summary`, `context_pack_json` (reproducibility/debug), `model`.

## 7. API (added; all under bearer auth)
- `GET /api/profile` · `PUT /api/profile`
- `POST /api/crossfit/parse` (multipart image) → parsed CrossFit week (Stage 1); upserts
  `crossfit_weeks`.
- `POST /api/plan/generate` (body: `week_start`, optional overrides) → builds context pack,
  runs Stage 2, upserts `plans`, returns the plan.
- `GET /api/plan?week=YYYY-MM-DD` → latest plan for that week.
- `GET /api/fitness` → current computed metrics (for the fitness read / debugging).

## 8. App screens (added)
- **Plan my week**: pick/take a photo of the box schedule → `POST /api/crossfit/parse` →
  show the parsed week (editable per-day fields) → "Generate plan" → `POST /api/plan/generate`.
- **Weekly plan view**: per-day cards (CrossFit summary + planned run: type / distance /
  pace / time note / optional flag) + the fitness summary + week rationale + one flag +
  "Regenerate".
- **Profile/Settings**: target weekly km, progression mode, optional HR markers, run
  constraints, goal text.

## 9. M0 review follow-ups folded into M1
- Validate the Strava OAuth `state` param (CSRF) in `/api/strava/callback`.
- Run the sync ticker once immediately on server boot (fresh-instance UX).
- Switch the Strava incremental cursor to the latest stored activity `start_time`.

## 10. Testing
- **Metrics**: table-driven Go tests over fixture activities/recovery (deterministic).
- **Claude client/coach**: inject a stub runner returning canned `claude -p` JSON
  envelopes; assert the constructed prompt references the image file (Stage 1) and the
  Coach Brain (Stage 2), that `.result` JSON is extracted and parses into the typed structs,
  and that malformed JSON triggers exactly one retry. **No real `claude`/network in CI.**
- **Handlers**: `httptest` for profile / crossfit / plan / fitness, incl. auth rejection.
- **App**: jest with mocked api client + react-query, for the plan flow and plan view.
- **Manual integration**: generate a real plan from a real CrossFit photo on a host with
  the `claude` CLI installed and logged in (`claude login`) — no API key needed.

## 11. Risks & mitigations
- **CrossFit image parsing wrong/ambiguous.** Mitigate: parsed week is editable before
  planning; persist `raw_response`.
- **Malformed JSON from `claude -p`.** No schema guarantee in headless mode. Mitigate:
  explicit "JSON only" prompt, extract from the `--output-format json` envelope, one retry
  on parse failure, and an editable Stage-1 result.
- **Shared subscription rate limits / availability.** `claude -p` shares the 5-hour/weekly
  budget with interactive Claude Code. Mitigate: on-demand (not per-day) generation; surface
  limit/CLI errors clearly with a "logged in? limit hit?" message; the fitness read +
  metrics still render from local data without Claude. Subprocess timeout (~2 min).
- **`claude` CLI not installed / not logged in.** Mitigate: documented setup
  (`claude login`); the client distinguishes "not found / not logged in" from other errors.
- **Pace targets without streams.** Easy/threshold estimates come from activity summaries;
  good enough for planning. True time-in-zone needs streams — deferred.

## 12. Explicitly out of scope for M1 (deferred)
Per-second activity streams, time-in-zone / decoupling / easy-day audit, the autonomous
nightly agent that adjusts the day-of plan from overnight readiness (M2), push
notifications, and race/taper planning (M3).
