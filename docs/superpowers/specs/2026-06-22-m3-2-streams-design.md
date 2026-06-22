# help-my-run — Milestone 3.2 Design Spec

**Date:** 2026-06-22
**Status:** Approved (M3.2 detailed)
**Depends on:** M0 + M1 + M2 + M3.1 (all merged to `main`)
**Author:** Brainstormed with Claude Code

---

## 1. Context

M3.1 shipped the cardio-capacity progress tracker but explicitly **deferred decoupling and
time-in-zone** because they need per-second streams. M3.2 adds that physiological layer: it
ingests each run's per-second HR/pace stream and computes the L2 analyses —
**time-in-zone** (is an easy run actually aerobic?) and **decoupling** (is aerobic
durability improving?) — surfaced per run and as a trend. Goal context is unchanged:
building the aerobic engine for RX CrossFit, not racing.

## 2. Decisions (locked in brainstorming)

| Decision | Choice |
|---|---|
| Stream source | **Strava streams API primary, Garmin `.FIT` fallback** when a run has no Strava HR stream (most complete coverage) |
| Storage | **Raw (gzipped) + computed** — store each stream once, never re-fetch; cache the per-run analysis; recompute the cache from raw when zones/logic change |
| Fetch strategy | **Recent window (~12 weeks) auto-fetch during sync, trickled within rate limits, + on-demand for older runs** |
| Analyses | **Time-in-zone + decoupling** |
| Surfacing | **Per-run detail screen + a decoupling trend card on the M3.1 Progress screen** |

## 3. Goal & success criteria

**Goal:** Pull each run's per-second stream and surface time-in-zone + decoupling, per run
and as a trend.

1. Each run's stream is fetched (Strava primary; Garmin `.FIT` fallback when Strava has no
   HR), stored **gzipped once**, and never re-fetched.
   > **Note (M3.2 scope):** the Strava path is fully working; the Garmin `.FIT` fallback is
   > **infrastructure landed; activation deferred to an id-mapping slice** — there is no
   > Strava↔Garmin activity-id mapping in the M3.2 data model yet, so the fallback ships
   > DORMANT (built + unit-tested but never fires at runtime). Until the mapping exists,
   > runs with no Strava HR show the no-HR state. DoD for §3.1 = "Strava path fully working;
   > Garmin fallback dormant".
2. **Recent-window auto-fetch** (~12 weeks) is trickled during sync within Strava rate
   limits; any older run can be fetched **on-demand**.
3. Per run: **time-in-zone** (minutes + % per HR zone using the athlete's profile zones) and
   **decoupling** (Pa:HR first-half vs second-half drift %), computed and cached.
4. Cached analyses **recompute from the stored raw stream when the profile's HR zones
   change** (no re-fetch).
5. A **run-detail screen** shows time-in-zone bars + the decoupling number, with a "fetch
   stream" action for un-fetched older runs.
6. A **decoupling trend** card appears on the M3.1 Progress screen (the signal M3.1 deferred
   to streams).
7. Missing/short/HR-less streams degrade gracefully (clear "no stream / no HR" states; never
   fabricate analysis).

## 4. Components (extends M0/M1/M3.1)

- **Stream ingestion** —
  - Strava: a client method `GetActivityStreams(id)` hitting
    `/activities/{id}/streams` (keys: `time`, `heartrate`, `velocity_smooth`, `distance`;
    `key_by_type=true`), reusing M0's Strava OAuth (`activity:read_all`) + backoff. Exact
    params/response verified at plan time.
  - Garmin `.FIT` fallback: the Python worker downloads + parses the per-second record
    stream (timestamp, heart_rate, speed/enhanced_speed, distance) when Strava lacks HR. The
    FIT-parsing approach (library + method) is verified at plan time against the installed
    `garminconnect`/FIT tooling (`garmin-fit-sdk==21.208.0`, pinned).
    > **Infrastructure landed; activation deferred to an id-mapping slice.** This fallback
    > ships DORMANT in M3.2: it requires a Strava↔Garmin activity-id mapping (absent from the
    > M3.2 data model), so the worker `stream` subcommand + `RunGarminFetchFIT` + the
    > `FetchAndAnalyze` call site are built and unit-tested but never fire at runtime (the
    > id-resolver returns "no Garmin id"). A future slice (ingest Garmin activity ids + match
    > by `start_time`) activates it; the runtime path in M3.2 is Strava-only.
  - Both normalize to a `{t, hr, v, dist}` series; rate-limit-aware **trickle** + an
    on-demand path.
- **Streams engine** (`backend/internal/streams`) — *deterministic Go*. From a decompressed
  series: **time-in-zone** (zone boundaries from the profile; documented defaults if unset)
  and **decoupling** (Pa:HR = speed-per-heartbeat, first vs second half split at the
  moving-time midpoint, drift %). Pure functions, table-driven tests.
- **Analysis cache + recompute** — compute and cache `stream_analyses` from stored raw; the
  cached row snapshots the zones used, so a profile-zone change triggers recompute.
- **Decoupling signal → M3.1 progress engine** — add a `decoupling` signal series (per-run,
  from `stream_analyses`) to `ProgressReport` so the deferred card has data.
- **App** — a run-detail screen (tap a run in the recent-runs list) and the decoupling card
  on the Progress screen.

## 5. Data model (added; SQLite)
- `activity_streams` — `activity_id` (PK, = Strava activity id), `source`
  (`strava`|`garmin`), `series_gz` (BLOB, gzipped JSON of the normalized series),
  `fetched_at`.
- `stream_analyses` — `activity_id` (PK), `time_in_zone_json`, `decoupling_pct` (REAL,
  nullable), `pa_hr_first` (REAL), `pa_hr_second` (REAL), `zones_json` (snapshot of the zone
  boundaries used), `computed_at`.
- `stream_fetch_log` — `source` row tracking recent-window fetch progress + last error
  (mirrors M0 `sync_log` conventions), so the trickle is resumable.
- Migration `00006_*.sql`.

## 6. API (added; all under bearer auth)
- `POST /api/activities/{id}/stream/fetch` → fetch the stream if missing, compute + cache
  the analysis, return it. (On-demand path; also used to retry.)
- `GET /api/activities/{id}/analysis` → the cached time-in-zone + decoupling for a run
  (404/empty state if not yet fetched).
- `GET /api/progress` (existing) now includes the `decoupling` signal in its report.

## 7. Fetch orchestration
During sync: enqueue recent-window (~12 weeks) runs that lack a stream; fetch up to a small
per-sync budget, trickled, with 429 backoff (reusing M0's Strava backoff); progress tracked
in `stream_fetch_log` so it resumes across syncs without re-fetching. The on-demand endpoint
fetches a single run immediately (used by the run-detail screen for older runs). Garmin
`.FIT` fallback is attempted only when the Strava stream has no HR.

## 8. App
- **Run detail** (new screen, reached from the recent-runs list): time-in-zone bars
  (per-zone minutes/%), the decoupling number with a plain-language read ("<5% on an easy
  long run = good durability"), and a "Fetch stream" button when the run has no stored
  stream yet (older than the auto-window). Clear "no HR in this stream" state.
- **Progress screen**: the decoupling trend card (per-run decoupling over the window, lower
  = better) joins the existing M3.1 cards.

## 9. Testing
- **Streams engine**: table-driven Go tests over fixture series — time-in-zone minutes/%,
  decoupling split + drift %, and gap/missing-HR/short-stream handling.
- **Strava streams client**: `httptest` + recorded streams JSON; assert keys/params and
  normalization; 429 backoff path.
- **Garmin `.FIT` fallback**: a fixture `.FIT` (or mocked parser) → normalized series; no
  live Garmin in CI.
- **Analysis cache**: recompute-on-zone-change (changed profile zones → recompute from raw).
- **Fetch orchestration**: trickle budget + resumable `stream_fetch_log` with injected fakes
  (no live Strava).
- **Progress integration**: the decoupling signal series from fixture `stream_analyses`.
- **API**: `httptest` for fetch/analysis endpoints incl. auth + not-fetched states.
- **App**: jest with mocked api — run-detail (bars + decoupling + fetch button + no-HR
  state) and the Progress decoupling card.
- **Manual**: fetch a real run's stream and view the analysis; confirm the decoupling card
  populates after the recent-window backfill.

## 10. Risks & mitigations
- **Strava streams rate limits.** Mitigate: trickle within a per-sync budget, 429 backoff,
  resumable `stream_fetch_log`, on-demand for the rest.
- **Garmin `.FIT` parsing fragility.** Mitigate: it's a *fallback only* (Strava primary),
  isolated in the Python worker behind the existing boundary; a parse failure degrades to
  "no stream" without breaking other runs.
- **Streams without HR (e.g. watch lost contact).** Mitigate: time-in-zone/decoupling need
  HR; show a clear "no HR" state, store the raw anyway, mark the analysis null.
- **Zone changes invalidating cached analysis.** Mitigate: zones snapshot in
  `stream_analyses`; recompute from stored raw on mismatch (no re-fetch).
- **DB growth.** Mitigate: gzipped blobs (~20–50 KB/run) are negligible at single-user
  scale.

## 11. Out of scope for M3.2 (deferred)
Easy-day-honesty aggregate (% of easy runs actually moderate — a later slice), other
stream-derived metrics (cadence/power/altitude/grade analysis), and chat-with-your-data
(M3.3). No race/taper features (removed from the project).
