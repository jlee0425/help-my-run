# help-my-run — Milestone 4 Design Spec

**Date:** 2026-06-23
**Status:** Approved (M4 detailed; user approved "write spec, then execute")
**Depends on:** M0 + M1 + M2 + M3.1 + M3.2 + M3.2.1 + M3.3 (all merged to `main`)
**Author:** Brainstormed with Claude Code

---

## 1. Context & motivation

The project's purpose is a **self-hostable, single-user AI running coach with no
subscriptions**. As of **June 1, 2026**, Strava's programmatic API (Standard Tier) requires
an **$11.99/mo Strava subscription** — directly contradicting the goal. (Free users can
still export their own data and device integrations are unaffected; it's specifically the
*API* that is now paywalled.) Sources:
<https://communityhub.strava.com/insider-journal-9/an-update-to-our-developer-program-13428>,
<https://www.heise.de/en/news/Strava-API-access-only-with-paid-subscription-in-the-future-11315017.html>.

M4 removes the Strava dependency and runs the whole app on **free Garmin data** via the
unofficial `python-garminconnect` worker — which already supplies recovery (M0), the
activities list (M3.2.1), and `.FIT` per-second streams (M3.2). So M4 is mostly **deletion +
rewiring**, not new features; the downstream engines (metrics, progress, coach, chat) are
untouched because they read the canonical `activities` table.

**No data migration risk:** sync never succeeded under the placeholder Strava creds, so the
DB has **no real activity data** — the `activities` re-key below is clean.

## 2. Decisions (locked in brainstorming)

| Decision | Choice |
|---|---|
| Activity source | **Repopulate the canonical `activities` table from Garmin** (re-keyed by Garmin activity id); downstream code unchanged. |
| Strava | **Full removal** — `strava` package, OAuth endpoints, Connect-Strava screen, `SyncStrava`, `STRAVA_*` env, and the now-moot Strava M0-follow-ups. |
| Streams | **Garmin `.FIT` only** — `resolveGarminID` becomes identity; the M3.2 Strava streams client is removed. |
| Connection UX | The app shows **Garmin connection status** (worker logged in + last sync), not "Connect Strava". |

## 3. Goal & success criteria

**Goal:** Run the entire app on free Garmin data; remove the Strava API dependency.

1. Runs come from **Garmin**, landing in `activities`; metrics/progress/streams/coach/chat
   work unchanged.
2. Per-second **streams** come from the Garmin `.FIT` worker only; `resolveGarminID` returns
   the activity id directly.
3. **No Strava** remains: package, OAuth endpoints, Connect screen, `SyncStrava`, `STRAVA_*`
   env vars all removed; the Go build has no `strava` import.
4. `/api/status` reports **Garmin only**; removed Strava routes 404.
5. The app's Connect/Settings shows Garmin status + "Sync now"; all other screens unchanged.
6. Full Go/worker/jest suites green; a real `make garmin-login` + sync lands real runs and
   the downstream features work on real data.

## 4. Components / changes (extends M0–M3.3)

- **Worker — full activity records.** Enrich the Garmin activity normalizer (from M3.2.1's
  list ingest) to emit every field `activities` needs: `garmin_activity_id`, `start_time`,
  `type`, `distance_m`, `moving_time_s`, `elapsed_time_s`, `avg_hr`, `max_hr`, `avg_speed`,
  `avg_cadence`, `elevation_gain_m`, `raw_json`.
  - ⚠️ **Verify at plan time:** whether `get_activities_by_date` includes avg/max HR, avg
    speed, cadence, elevation. If the list is summary-only, the worker makes a per-activity
    `get_activity(id)` detail call to fill the gaps (extra calls — fits the trickle/budget).
- **Store — re-key `activities` + drop Strava tables.** Migration `00009`:
  - `ALTER TABLE activities RENAME COLUMN strava_id TO activity_id` (= Garmin activityId);
  - `DROP TABLE strava_tokens; DROP TABLE oauth_states; DROP TABLE garmin_activities;`
    (the last is superseded — its data now lands directly in `activities`).
  - Update the `Activity` struct (`StravaID` → `ActivityID`) + all queries/upserts.
    `activity_streams`/`stream_analyses` already key on `activity_id`.
- **Sync — Garmin-only.** `SyncGarmin` pulls recovery (as today) **+ activities** into
  `activities`. Delete `SyncStrava`, the `strava` package, the OAuth callback/connect
  handlers, and the 3 Strava-specific M0 follow-ups (OAuth-state CSRF, latest-activity
  cursor) — all moot. `SyncAll` becomes Garmin-only.
- **Streams — Garmin `.FIT` only.** Remove `strava.GetActivityStreams`; the `.FIT` worker
  path is the sole stream source. `resolveGarminID(activityID)` returns `(activityID, true)`
  (identity) — the dormant→active fallback becomes the primary path. Recent-window trickle
  uses Garmin.
- **API.** Remove `GET /api/strava/connect` and `GET /api/strava/callback`. `/api/status`
  reports Garmin only (`connected` = worker has valid login; `last_synced_at`). `/api/sync`
  unchanged (now Garmin-only).
- **App.** Remove the Connect-Strava screen/flow. Settings shows **Garmin** connection
  status + "Sync now". Plan/Progress/Streams/Today/Chat untouched (read `activities`).
- **Docs/.env.** Remove `STRAVA_*` from `.env.example`/README; README → Garmin-only
  (retains today's ANTHROPIC-key-unset + absolute-path fixes).

## 5. Data model (migration `00009`)
- `ALTER TABLE activities RENAME COLUMN strava_id TO activity_id;` — `activity_id` (INTEGER
  PK) is the Garmin activityId.
- `DROP TABLE IF EXISTS strava_tokens;`
- `DROP TABLE IF EXISTS oauth_states;`
- `DROP TABLE IF EXISTS garmin_activities;`
- (Down migration recreates them per the original DDL, for reversibility.)

## 6. Identity
`activities.activity_id` = Garmin `activityId` (int64). `activity_streams.activity_id` and
`stream_analyses.activity_id` reference it directly. There is no Strava↔Garmin mapping step
anymore — the activity *is* the Garmin activity.

## 7. Connection / onboarding
"Connected" no longer means a Strava OAuth token; it means the Garmin worker has a valid
login (the one-time `make garmin-login`, MFA-aware, tokens in `GARMIN_TOKENSTORE`).
`/api/status.garmin.connected` reflects whether the last worker invocation authenticated
successfully; the app surfaces that + last-sync + a "Sync now" button. A failed/expired
Garmin login surfaces a clear "re-run garmin login" message (already the worker's behavior).

## 8. Testing
- **Worker**: fixture-based normalize test for the enriched activity record (all fields);
  if a detail call is added, a fixture for that path. No live Garmin in CI.
- **Store**: migration `00009` (re-key + drops) via temp DB; `activities` upsert-from-Garmin
  round-trip; confirm `activity_streams`/`stream_analyses` still join on `activity_id`.
- **Sync**: Garmin-only `SyncAll` (recovery + activities) with the stub worker/runner; assert
  no Strava path remains.
- **Streams**: Garmin-only fetch + identity `resolveGarminID`; the recent-window trickle.
- **API**: `/api/status` Garmin-only shape; removed Strava routes return 404; `/api/sync`.
- **App**: Connect-Strava screen removed; Garmin status + Sync render; other screens
  unaffected (smoke).
- **Build hygiene**: `grep` proves no `strava` import remains; `go build ./...`,
  `go test ./...`, worker `pytest`, app `jest` + `tsc`, `gofmt` all clean.
- **Manual**: `make garmin-login` → sync → real runs land in `activities` → Progress,
  streams (time-in-zone/decoupling), a weekly plan, and chat all work on real Garmin data.

## 9. Risks & mitigations
- **Unofficial Garmin API is now the ONLY data path.** Mitigate: the worker's existing
  resilience (curl_cffi/TLS-impersonation from M0), graceful degradation to last-known data,
  clear "re-run garmin login" errors; this is the accepted cost of zero subscriptions.
- **Garmin rate limits** (activities list + per-`.FIT` + optional per-activity detail).
  Mitigate: reuse the M3.2 stream trickle/budget; the activities list is one call per window.
- **Re-key migration** — trivial (empty DB); Down migration restores the old tables for
  reversibility.
- **Scope creep into the engines.** Mitigate: M4 explicitly does NOT touch metrics/progress/
  coach/chat logic — only the activity/stream *source* and Strava removal.

## 10. Out of scope
No new features, no additional data sources, no change to the AI engines or lock-screen
push. (Strava remains permanently removed unless a future milestone re-adds it behind the
paid API.)
