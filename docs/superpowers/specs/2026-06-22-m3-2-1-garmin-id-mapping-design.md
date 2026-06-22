# help-my-run — Milestone 3.2.1 Design Spec

**Date:** 2026-06-22
**Status:** Approved (M3.2.1 detailed)
**Depends on:** M0 + M1 + M2 + M3.1 + M3.2 (all merged to `main`)
**Author:** Brainstormed with Claude Code

---

## 1. Context

M3.2 shipped per-second stream analysis (time-in-zone + decoupling) with the Garmin `.FIT`
fallback built and unit-tested but **dormant**: `resolveGarminID` returns `(0, false)`
because there was no Strava↔Garmin activity-id mapping, so the fallback never fired and any
run with no Strava HR stream degraded to the "no HR" state. M3.2.1 supplies the missing
mapping and **activates** the fallback. It is a small, focused slice — all the FIT
machinery (worker `stream` subcommand, `garmin-fit-sdk` parse, `RunGarminFetchFIT`, the
`FetchAndAnalyze` call site) already exists.

## 2. Decisions (locked in brainstorming)

| Decision | Choice |
|---|---|
| Strava→Garmin match | **Start-time match (±~120 s), tie-broken by closest duration then distance**, run-type only; no confident match → `(0,false)` (degrade to no-HR) |
| Mapping storage | **New `garmin_activities` table + lazy match at fetch time** (keeps raw Garmin data; re-matchable if logic improves) |
| Ingestion timing | Worker pulls the recent-window Garmin activities list during the existing `SyncGarmin` |

## 3. Goal & success criteria

**Goal:** Activate the Garmin `.FIT` fallback so runs lacking a Strava HR stream still get
HR (and thus time-in-zone + decoupling) from the Garmin `.FIT`.

1. The worker ingests the recent-window **Garmin activities list** (id, start_time,
   duration, distance, type) into a new `garmin_activities` table during sync.
2. `resolveGarminID(stravaActivity)` returns a confident Garmin id via start-time match
   (±~120 s) tie-broken by closest duration/distance; no confident match → `(0,false)`.
3. A no-HR Strava run now fetches HR via the Garmin `.FIT` fallback (the M3.2 path), is
   stored with `source = garmin`, and its time-in-zone + decoupling compute.
4. The run-detail screen shows the analysis **source** ("HR via Garmin .FIT") when the
   fallback supplied it.
5. Graceful degradation throughout: Garmin list fetch failure, no match, or FIT parse
   failure → the existing no-HR state; never a fabricated analysis.

## 4. Components (extends M0/M3.2)

- **Garmin activities ingestion** — a worker fetch of the recent Garmin activities list
  (`garminconnect` `get_activities_by_date` or equivalent; exact method verified at plan
  time), normalized to `{garmin_activity_id, start_time, duration_s, distance_m,
  activity_type}`; a new `garmin_activities` store table + upsert; wired into `SyncGarmin`
  (which already runs the worker each sync).
- **Nearest-match query** (`store`) — given a start_time, return Garmin activities within a
  tolerance window (with duration/distance for tie-break).
- **`resolveGarminID` activation** (`streams` engine) — replace the `(0,false)` stub with
  the start-time + duration/distance match. This lights up the already-built
  `FetchAndAnalyze` → `RunGarminFetchFIT(garminID)` → parse → `source=garmin` path, so
  `garmin-fit-sdk` (already pinned in the worker venv) now runs in production.
- **App (minor)** — surface the analysis `source` on run-detail ("HR via Garmin .FIT").

## 5. Data model (added; SQLite)
- `garmin_activities` — `garmin_activity_id` (PK, INTEGER), `start_time` (TEXT ISO),
  `duration_s` (REAL), `distance_m` (REAL), `activity_type` (TEXT), `raw_json` (TEXT).
  Migration `00007_*.sql`.

## 6. Data flow (fetch a no-HR run)
```
Strava streams (no HR)
  → resolveGarminID(start_time, duration, distance)
       → store: nearest garmin_activities within ±120s, tie-break by duration/distance
       → (garminID, true) | (0, false)
  → if matched: worker `stream` subcommand downloads + parses the Garmin .FIT by garminID
       → normalized {t,hr,v,dist} WITH HR → store activity_streams source=garmin
       → time-in-zone + decoupling computed + cached
  → if no match / parse fail: existing no-HR state (unchanged)
```

## 7. Matching detail
Tolerance ±120 s on start_time (a config knob with that default). Candidates restricted to
run-type Garmin activities. Multiple candidates → pick the one whose `duration_s` is closest
to the Strava activity's moving/elapsed time (then `distance_m`); if still ambiguous or none
within tolerance → `(0,false)`. Matching is **lazy** (performed at fetch time from the
stored `garmin_activities`), so it is re-runnable if the logic improves without re-ingest.

## 8. API
No new endpoints. The existing `POST /api/activities/{id}/stream/fetch` now genuinely
exercises the fallback. The analysis DTO already carries `source` (`strava`|`garmin`); the
run-detail screen surfaces it.

## 9. Testing
- **Worker activities-list fetch**: fixture-based pytest asserting the normalized contract
  shape; no live Garmin in CI.
- **`garmin_activities` store + nearest-match query**: temp-DB tests (within/outside
  tolerance, tie-break by duration/distance, no-match → empty).
- **`resolveGarminID`**: table-driven (match within tolerance, tie-break, no-match → false).
- **End-to-end activation**: a fake no-HR Strava stream + a seeded `garmin_activities` row +
  a stub FIT runner returning a series with HR → `FetchAndAnalyze` stores `source=garmin`
  and computes the analysis. (The FIT parse itself is already covered by M3.2 tests.)
- **App**: run-detail shows the source badge when `source=garmin`.
- **Manual**: a real run with no Strava HR gets HR via the Garmin fallback and its analysis
  populates.

## 10. Risks & mitigations
- **Unofficial Garmin activities-list fragility.** Mitigate: isolated in the Python worker
  behind the existing boundary; a list-fetch failure leaves the fallback dormant for those
  runs (graceful degrade), never breaks Strava ingestion.
- **Mismatched activity (wrong Garmin run matched).** Mitigate: tight ±120 s tolerance +
  duration/distance tie-break + run-type filter; ambiguous → no match (prefer no-HR over a
  wrong match). `source=garmin` is surfaced so a bad match is visible.
- **Garmin rate limits.** Mitigate: the activities list is one call per sync window (cheap);
  reuses the worker's existing auth.
- **Clock/timezone skew between sources.** Mitigate: match on UTC start times; the ±120 s
  window absorbs small skew.

## 11. Out of scope
No new analyses (reuses the M3.2 engine), no backfill of Garmin activities beyond the
recent window, no change to the Strava-primary path, and chat-with-your-data (M3.3).
