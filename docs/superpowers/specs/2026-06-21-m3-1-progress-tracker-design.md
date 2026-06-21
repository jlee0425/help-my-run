# help-my-run — Milestone 3.1 Design Spec

**Date:** 2026-06-21
**Status:** Approved (M3.1 detailed; user approved "write spec, then execute")
**Depends on:** M0 + M1 + M2 (all merged to `main`)
**Author:** Brainstormed with Claude Code

---

## 1. Context & reframe

M0 delivered the data foundation, M1 the CrossFit-aware weekly run-plan generator, M2 the
agentic daily coach. "M3" was originally a basket of four independent features (race
predictor, taper planner, streams/time-in-zone, chat-with-data). During brainstorming the
user reframed the project's purpose:

> "I'm not going to race in anything. The app is to improve my own cardio capacity for RX
> CrossFit."

That **removes the race predictor and taper/race-week planner entirely** (no race) and
re-centers M3 on one question: **is my aerobic engine measurably improving?** The remaining
features are decomposed into independent slices, each with its own spec → plan → build:

- **M3.1 (this spec): Cardio-Capacity Progress Tracker** — trends that answer "is my engine
  improving?"
- M3.2 (future): per-second streams + time-in-zone / decoupling.
- M3.3 (future): chat-with-your-data.

## 2. Goal & success criteria

**Goal:** Show whether the user's aerobic capacity is improving over time (for RX CrossFit,
not racing), via signal trends + an on-demand AI read.

1. A **Progress** screen shows per-signal trend cards: current value, change vs the start of
   the window, a direction arrow, and a **unicode sparkline** (no charting dependency), over
   a configurable window (default **12 weeks, weekly buckets**).
2. **Pace-at-a-fixed-HR** (headline engine signal): weekly-median pace of runs whose average
   HR falls in a reference band → faster at the same HR = fitter.
3. **VO2max** is ingested from Garmin and trended.
4. **Resting-HR**, **HRV-baseline**, and **weekly-volume/load** trends come from existing M0
   data (+ M1 metrics).
5. An **"Analyze progress"** action returns a short `claude -p` narrative read; a
   deterministic templated summary is used if Claude is unavailable.
6. Thin history degrades gracefully ("not enough data yet"); weeks with no qualifying run
   show a gap, not a fabricated point.

## 3. Decisions (locked in brainstorming)

| Decision | Choice |
|---|---|
| Focus | Cardio-capacity progress tracker (engine improvement), not racing |
| Signals | pace-at-fixed-HR, resting HR, HRV baseline, weekly volume/load, **+ VO2max (new ingestion)** |
| Presentation | **Trend cards + unicode sparklines** — no charting library |
| AI read | On-demand `claude -p` with deterministic fallback (reuse M1/M2 llm path) |
| Window | Default 12 weeks, weekly buckets, configurable via query param |

## 4. Components

- **VO2max ingestion** (extends M0) — add a VO2max fetch to the Python Garmin worker
  (`garmin-worker`), a `garmin_vo2max` store table + upsert/get, and wire it into
  `SyncGarmin` (the Go runner that execs the worker) with a ~12-week backfill on first sync.
  This is the only piece that touches ingestion. The exact `python-garminconnect` method
  (e.g. `get_max_metrics`) is verified at plan time against the installed library.
- **Progress engine** (`backend/internal/progress`) — *deterministic Go*. For each signal,
  build a weekly series over the window and a summary `{current, baseline, deltaAbs,
  direction}`. Pace-at-fixed-HR uses a **reference-HR band** (default = profile Zone-2
  ceiling ± a small window; sensible constant if unset) and the weekly-median pace of
  in-band runs. Reuses M1 `metrics` for volume/load. Pure functions, table-driven tests.
- **Progress read** (reuse `coach` + `llm`) — a "progress read" prompt fed the computed
  trends → `claude -p` → a short narrative; deterministic templated fallback on any Claude
  failure (the M2 pattern: `source = ai|fallback`).
- **App** — a **Progress** screen: trend cards + unicode sparklines + an "Analyze progress"
  button; `useProgress` (query) and `useAnalyzeProgress` (mutation) hooks; a home nav link.

## 5. Data model (added; SQLite)
- `garmin_vo2max` — `date` (PK, ISO), `vo2max` (REAL), `raw_json`. Migration `00005_*.sql`
  following M0–M2 conventions.
- No other tables: progress is computed on demand from `activities` + `garmin_*` (YAGNI; no
  caching of computed trends or AI reads).

## 6. API (added; all under bearer auth)
- `GET /api/progress?weeks=12` → deterministic trends + per-signal sparkline series. Instant
  (no Claude). The shape the Progress screen consumes.
- `POST /api/progress/analyze` → the AI read over the computed trends (`claude -p`, with
  deterministic fallback). Body may carry the window; returns `{ text, source }`.

## 7. Reference-HR method (pace-at-fixed-HR)
"Fixed HR" = a configurable reference band. Default reference HR = the profile's Zone-2
ceiling (`zone2_ceiling_bpm`); band = reference ± a small window (e.g. ±5 bpm). Runs whose
`avg_hr` is in-band are weekly-bucketed and the **median pace** taken. If HR markers are
unset, fall back to a sensible default reference (documented constant). Weeks with no
in-band run produce a gap in the series (not interpolated). "Faster pace at the same HR over
time" is the improvement signal; the card shows pace decreasing = fitter.

## 8. App screen
**Progress**: one card per signal — label, current value, change vs window start, a ↑/↓/→
direction arrow interpreted correctly per signal (for pace, *lower is better*), and a
unicode sparkline rendered from the weekly series (block glyphs ▁▂▃▅▆▇, scaled to min/max,
gaps rendered blank). A footer "Coach read" area populated by the "Analyze progress" button.
A "not enough data yet" empty state when the window has too few points. Home gets a nav link
to Progress.

## 9. Testing
- **Progress engine**: table-driven Go tests over fixture `activities`/`garmin_*`/`vo2max`
  rows — series construction, deltas/direction (incl. pace lower-is-better), reference-HR
  banding, gap handling, and the not-enough-data path.
- **VO2max worker fetch**: fixture-based `normalize` test asserting the contract JSON shape;
  no live Garmin in CI.
- **Analyze**: stub `claude -p` runner returning canned narrative + a failing runner to
  prove the deterministic fallback.
- **API**: `httptest` for `/api/progress` and `/api/progress/analyze` incl. auth rejection.
- **App**: jest with mocked api client — trend-card rendering, sparkline output, the
  analyze flow, and the empty state.
- **Manual**: view real trends after a sync that includes VO2max history.

## 10. Risks & mitigations
- **Sparse in-band runs → noisy pace-at-HR.** Mitigate: weekly median, configurable band,
  explicit gaps, "not enough data" state.
- **VO2max ingestion (unofficial Garmin).** Mitigate: isolated in the Python worker behind
  the existing boundary; missing VO2max degrades to "—" without breaking other signals.
- **Claude unavailable for the read.** Mitigate: deterministic templated summary; the
  numeric trends always render without Claude.
- **Thin history.** Mitigate: graceful empty state; window is configurable.

## 11. Out of scope for M3.1 (separate future slices)
Per-second streams + time-in-zone / decoupling (M3.2), chat-with-your-data (M3.3), and any
race/taper features (removed from the project per the reframe).
