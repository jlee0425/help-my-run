# M6.5 — Demo mode (design spec)

Date: 2026-07-11 · Status: approved (design discussion 2026-07-11)
Goal: a stranger runs ONE command and sees the full product in 60 seconds —
no Garmin account, no Claude subscription, no setup wizard. First step of the
OSS-self-host release; doubles as the portfolio demo.

## 1. Invocation & isolation

- `--demo` CLI flag on the binary (precedent: `--reset-password`).
- Overrides `DB_PATH` to in-memory SQLite (`:memory:`; modernc supports it,
  one-writer model unchanged), migrates, seeds, serves. No disk writes, no
  interference with a real instance, zero residue on exit.
- Garmin/Claude config is ignored in demo; log prints a loud `DEMO MODE`
  banner on boot.
- `make demo` = build + run `--demo`.

## 2. Fixture: 84 days authored as a story

- `backend/internal/demo/fixture.json`, embedded via `go:embed` (single-binary
  promise holds). All dates are day offsets `-83..0`; the seeder materializes
  them onto real dates at boot so the demo never rots.
- Narrative beats (authored to show the product off):
  - Base-building block, ~3 runs/week, plausible HR/pace physiology.
  - A cutback week; VO2max trending gently up.
  - Overreach dip ~day −10: HRV tanks, readiness goes red, coach prescribes
    rest.
  - **Today**: amber readiness; the coach visibly downgrades the planned
    session with CrossFit-aware rationale. This is the centerpiece.
- Seeded content: activities (~36 runs), recovery days (84), VO2max history,
  pre-computed stream analyses (time-in-zone + decoupling) for the last ~6
  runs, last 14 days of daily decisions incl. today, current-week plan,
  2–3 scripted chat exchanges, generic athlete profile.
- Seeder inserts via existing store functions (never raw SQL) so schema drift
  breaks the demo test at compile time, not at runtime.

## 3. Canned coach (DemoRunner)

- `DemoRunner` implements the existing `llm.Runner` seam; the real coach /
  chat / progress engines run unchanged — only the LLM call is substituted.
- Routes on the distinct prompt shapes each engine produces (routing pinned by
  tests); returns curated envelopes: verdict prose, a realistic weekly plan,
  rotating sample chat replies.
- Every canned output carries a visible label: *"sample response — the live
  coach runs on your Claude subscription"*. Honesty is the point.

## 4. Auth & guardrails

- Demo auth: middleware treats every request as the authenticated owner; no
  setup wizard, no login page.
- `/api/auth/state` gains `demo: bool` (mirrored in `web/src/api/types.ts`,
  lockstep). Web shows a persistent DEMO badge.
- Endpoints that make no sense in demo — password change, token regen, Garmin
  login/disconnect, sync — return `409 {"error": "demo mode: …"}`; the UI
  surfaces it via the existing error rendering (loud-failure convention).
- Scheduler and boot-sync disabled; today's decision comes from the fixture.

## 5. Testing

- Go: seeder test (row counts + today's decision present, dates relative to
  now); handler test with demo Deps (auth bypass, sync blocked, state
  advertises demo); DemoRunner routing test; `--demo` wire test.
- Web: DEMO badge renders from auth state; guarded action surfaces the 409.
- Manual acceptance: `./bin/helpmyrun --demo` → Today/Trends/Plan/Run
  detail/Coach/Settings all populated, no dead pages, labels on canned output.

## Non-goals (deferred)

Hosted-demo hardening (multi-visitor, auto-reset), onboarding tour, demo-data
persistence, per-visitor state. Revisit only with a concrete reason.

## Plan (execution order)

1. Fixture + seeder (`internal/demo`) with tests.
2. DemoRunner + routing tests.
3. Demo wiring: `--demo` flag, auth bypass, guarded endpoints, `demo` in auth
   state (tests).
4. Web: types lockstep, DEMO badge, guard surfacing (tests).
5. `make demo`, README quickstart snippet.
6. Full suites → adversarial review → fix → commit.
