# help-my-run — Milestone 2 Design Spec

**Date:** 2026-06-20
**Status:** Approved (M2 detailed)
**Depends on:** M0 Foundation + M1 Weekly Plan (both merged to `main`)
**Author:** Brainstormed with Claude Code

---

## 1. Context

M0 delivered the data foundation (Strava + Garmin → one store, in the app). M1 delivered the
CrossFit-aware **weekly** run-plan generator (deterministic metrics + `claude -p` vision &
planning under the user's subscription). M2 is the iceberg's **L5 north star**: the
**agentic daily coach** — the loop that "runs before you wake," reshaping today's run from
last night's recovery so the user just runs it.

This makes the coach *prospective and autonomous* rather than on-demand: M1 produces the
week; M2 adapts the day.

## 2. Locked decisions (from brainstorming)

| Decision | Choice |
|---|---|
| Autonomy | **Auto-adjust + notify** — the agent rewrites today's session in place and pushes a briefing; the user just runs it. Includes an Undo escape hatch. |
| Trigger | **In-process Go scheduler** — a daily job in the already-long-running backend (no new infra). |
| Notification | **Expo push** — real morning phone push (needs a dev build + token registration). |
| Decision engine | **Hybrid** — deterministic readiness gate (GREEN/AMBER/RED) + `claude -p` "Coach Brain" rewrite, with a **deterministic fallback** so today is always decided if Claude is unavailable. |
| Week scope | **Today-only** — adjust only today's session; the rest of the week stays as M1 generated it. Week re-planning remains manual/weekly. |

## 3. Goal & success criteria

**Goal:** Every morning before the user wakes, the backend reads last night's recovery,
decides today's readiness, automatically rewrites today's run, and pushes a briefing.

1. A daily job fires unattended at the configured wake time, **idempotent** (once per day).
2. It assesses readiness (**GREEN/AMBER/RED**) from last night's sleep / overnight HRV /
   resting HR / Body Battery + recent trend, recording the driver numbers.
3. It rewrites today's planned session (**STAND / SOFTEN / MOVE-to-easy-or-rest**) and
   persists it as today's **effective** session, leaving the rest of the week untouched.
4. A morning **Expo push** lands with readiness + what changed + why.
5. The app shows a **Today briefing** with **Undo** (revert today to the original session).
6. If Claude is unavailable at run time, a **deterministic fallback** still decides today
   (RED → easy/rest, AMBER → trim volume/intensity, GREEN → stand). Today is never undecided.

## 4. Components (new; reuses M1)

- **Readiness engine** (`backend/internal/readiness`) — *deterministic Go*. Inputs: last
  night's `garmin_sleep` / `garmin_hrv` / `garmin_rhr` / `garmin_body_battery` + recent
  trend (reuses M1 `metrics`). Output: a readiness color + the driver numbers that decided
  it. Pure functions, table-driven tests.
- **Daily agent** (`backend/internal/agent`) — orchestrates the morning run (§5). The heart
  of M2.
- **Coach extension** (reuse `coach` + `llm`) — a "daily adjust" prompt (the PDF's Coach
  Brain) returning a structured today-decision JSON; **deterministic fallback** on any
  `claude -p` failure (not-logged-in / rate-limited / timeout / malformed).
- **Scheduler** — extends `cmd/server/main.go` + the M0 ticker: a daily job at a configured
  local time with a once-per-day guard. Clock is injectable for tests.
- **Push** (`backend/internal/push`) — Expo Push API client + device-token storage.

## 5. Data flow (each morning at configured time T)

```
scheduler fires (T, once/day)
  → SyncAll (fresh Garmin + Strava)                     [reuse M0 sync]
  → readiness.Assess(today)  → color + drivers          [deterministic]
  → load today's session from the latest M1 plan
  → if a run is scheduled today:
        coach.AdjustToday(readiness, session, context)   [claude -p Coach Brain]
          └─ on failure → deterministic fallback rule
     else:
        readiness-only briefing (no rewrite)
  → persist daily_decisions (+ effective adjusted session)
  → push.Send(deviceToken, summary)
```

Failure handling: Claude fails → fallback rule. Push fails → decision still stored; app
shows it on open. Sync fails → use last-known data, flag staleness in the briefing. The
per-day idempotency guard prevents double-runs.

## 6. Data model (added; SQLite)

- `device_tokens` — `expo_push_token` (PK), `platform` (`ios`|`android`), `updated_at`.
- `daily_decisions` — `date` (PK, ISO), `readiness_color` (`green`|`amber`|`red`),
  `drivers_json` (the numbers: sleep, HRV vs baseline, RHR vs baseline, Body Battery),
  `original_session_json`, `adjusted_session_json`, `action` (`STAND`|`SOFTEN`|`MOVE`),
  `rationale`, `source` (`ai`|`fallback`), `raw_response`, `created_at`. Keeping original +
  adjusted is what makes Undo possible.
- `agent_runs` — `last_run_date` (idempotency), `status`, `error`, `ran_at`.

## 7. API (added; all under bearer auth)
- `POST /api/push/register` — store/refresh the device's Expo push token.
- `GET /api/today` — today's briefing (readiness color, drivers, original + effective
  session, action, rationale, source, staleness flag).
- `POST /api/today/undo` — set today's effective session back to the original.
- `POST /api/agent/run` — manually trigger the daily loop now (for testing/first-run).
- Daily-run time + enable flag added to `GET`/`PUT /api/profile`.

## 8. App (added)
- `expo-notifications` setup; register the push token with the backend on launch (push
  requires a dev build, not Expo Go).
- A **Today** card on Home: readiness color + driver numbers + what changed + why + an
  **Undo** button.
- Settings additions: daily-run time, agent enable/disable.

## 9. Unattended auth & operational notes
- `claude -p` must authenticate non-interactively at run time → the host runs
  **`claude setup-token`** once (long-lived subscription token) so the backend's daily call
  works headless; `ANTHROPIC_API_KEY` remains a paid fallback. (Path established in the M1
  spec.) One daily call is far within subscription rate limits.
- Timezone: the run time is interpreted in the user's configured local timezone.

## 10. Testing
- **Readiness**: table-driven Go tests over fixture recovery rows (deterministic colors +
  drivers).
- **Daily agent**: injected fakes (fake clock, fake sync, fake coach, fake pusher) — the
  whole loop incl. the **fallback path** and the once-per-day guard runs offline in CI.
- **Push**: `httptest` server emulating the Expo Push API; assert payload + token handling.
- **Scheduler**: injected clock; assert it fires at T and not twice/day.
- **Coach daily-adjust**: stub `claude -p` runner returning canned today-decision JSON;
  assert prompt carries readiness + session + context, parse into the struct, and that a
  failure triggers the deterministic fallback. **No real `claude`/network in CI.**
- **App**: jest with mocked `expo-notifications` + api client.
- **Manual**: one real morning run on a device with `claude setup-token` configured and the
  app dev build installed.

## 11. Risks & mitigations
- **Unattended Claude failure.** Mitigate: deterministic fallback always decides today;
  `source` field records ai vs fallback; briefing still sent.
- **Surprising auto-changes / trust.** Mitigate: Undo to original; the briefing always
  states what changed and why; `STAND` is the default when readiness is GREEN.
- **Push delivery / dev-build friction.** Mitigate: decision is persisted regardless; the
  app shows the briefing on open even if the push didn't land.
- **Double-runs / missed days.** Mitigate: `agent_runs.last_run_date` idempotency guard;
  `POST /api/agent/run` for manual catch-up.
- **Stale data at run time.** Mitigate: sync first; flag staleness if the sync failed.

## 12. Out of scope for M2 (deferred to M3)
Week re-periodization / multi-day rebalance, per-second streams & time-in-zone, race
predictor and taper planning, chat-with-your-data, multi-user.
