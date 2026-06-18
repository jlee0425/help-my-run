# help-my-run — Design Spec

**Date:** 2026-06-19
**Status:** Approved (Milestone 0 detailed; M1–M3 outlined)
**Author:** Brainstormed with Claude Code

---

## 1. Context & vision

This project productizes the "Running on AI — The AI Training Iceberg" field guide as a
**self-hostable, single-user AI running coach** (React Native app + backend).

The guide describes five levels of using AI to train, each deeper level trading setup
effort for better coaching signal:

1. **L1** — Screenshots into ChatGPT (gut-check reads of a run)
2. **L2** — Export raw `.FIT`/`.GPX`/`.CSV` (trends: decoupling, time-in-zone, 80/20)
3. **L3** — MCP connectors: live Strava + Garmin (sleep, HRV, Body Battery, load, readiness)
4. **L4** — Own the pipe: nightly pull → your own DB → query your whole history
5. **L5** — Agentic coach: a "Coach Brain" + daily trigger that reshapes the plan before you wake

The guide's 21 prompts are effectively a product spec for an AI running coach: readiness
(GREEN/AMBER/RED), session decisions (STAND/SOFTEN/MOVE), easy-day honesty audits,
load/injury risk, race prediction, taper planning.

**North star:** the full **agentic coach (L5)** — it senses (sleep/HRV/load), decides
(readiness + today's session), reshapes the week before you wake, and notifies you. Built
bottom-up, because the agent cannot sense readiness until the data pipeline and coaching
engine exist beneath it.

## 2. Decisions & constraints

| Decision | Choice | Rationale |
|---|---|---|
| Audience | Single-user, **self-hosted**, **public open-source repo** | Matches the guide's "build in public" ethos; others fork and run their own instance. No multi-tenant auth; secrets via env, never committed. |
| Data sources | **Garmin + Strava** | Strava = clean run data (pace/HR/splits); Garmin = recovery (sleep, HRV, Body Battery, resting HR) required for a true readiness coach. |
| Backend | **Go core + thin Python Garmin worker** | Go for ~90% (API, Strava, DB, scheduler, Claude). Python isolated to Garmin only. See §3 for why Garmin must be Python. |
| LLM | **Claude** (Anthropic API) | Most capable; the guide itself references Claude. |
| Mobile | **React Native + Expo** | User is frontend-strong; Expo dev build + Expo Push fit the data/notification needs. |
| Store | **SQLite** first (Postgres later) | Trivial to self-host; the guide's own recommendation. |
| Hosting | Cheap always-on **VPS or Fly.io** | The nightly agentic loop (M2) needs to run before you wake. |

### Garmin data access — findings (June 2026)

This materially shaped the architecture and was verified during brainstorming:

- **Official Garmin API is unavailable to us.** The Garmin Connect Developer Program
  (Health/Activity API) does **not** permit personal use (must apply as a legal entity;
  personal apps rejected) **and the program is currently suspended** (cannot register).
  Refs: <https://developer.garmin.com/gc-developer-program/health-api/>,
  <https://developer.garmin.com/gc-developer-program/program-faq/>
- **Unofficial scraping is the only path, and it is Python.** In March 2026 Garmin
  deployed Cloudflare + TLS-fingerprinting that broke every community library.
  `python-garminconnect` recovered (April 2026) by replacing `garth` with a native auth
  engine using **`curl_cffi` TLS impersonation** + a **widget+cffi** SSO strategy (tested
  with MFA and non-MFA accounts). It works today but is an arms race.
  Refs: <https://github.com/cyberjunky/python-garminconnect>,
  <https://github.com/cyberjunky/python-garminconnect/issues/344>,
  <https://github.com/matin/garth/discussions/222>
- **Pure-Go Garmin is effectively dead.** The only Go library
  (`abrander/garmin-connect`) is an unmaintained reverse-engineered POC with no TLS
  impersonation, so almost certainly broken post-March-2026. Replicating the Cloudflare
  bypass in Go is a research project. Ref: <https://github.com/abrander/garmin-connect>

**Consequence:** Garmin ingestion must run on `python-garminconnect`. Everything else is Go.

## 3. System architecture (end state)

```
 React Native app (Expo)  ──HTTPS──▶  Go backend (the "core")
  • onboarding / connect              • REST API for the app
  • daily readiness card              • Strava OAuth + activity sync
  • run detail + AI read              • SQLite store (one place for everything)
  • weekly review                     • metrics engine (HR zones, decoupling,
  • push notifications  ◀─Expo Push──   time-in-zone, acute:chronic load)
                                      • Claude orchestration (the coach prompts)
                                      • scheduler (nightly agentic loop)  ◀── L5
                                              │ shares the SQLite DB
                                              ▼
                                      Python Garmin worker (thin)
                                      • python-garminconnect + curl_cffi
                                      • pulls sleep / HRV / Body Battery / RHR
                                      • emits normalized JSON → Go writes to SQLite
   External:  Strava API · Garmin (unofficial) · Anthropic API (Claude)
```

**Design principle (all milestones):** the coach **degrades gracefully** if Garmin breaks —
falls back to Strava-only reads or manual `.FIT` upload. Garmin scraping is the single
biggest ongoing risk; running self-hosted with one personal account keeps request volume
low and reduces flagging risk.

## 4. Phased roadmap (bottom-up toward L5)

| Milestone | Delivers | Iceberg | Why this order |
|---|---|---|---|
| **M0 · Foundation** | All data flowing end-to-end: Strava runs + Garmin recovery → one store, visible in the app | L2–L4 data layer | **De-risks Garmin (the fragile piece) first.** Nothing works without data. |
| **M1 · Coaching reads** | On-demand AI coaching: session read, weekly review, daily readiness, easy-day audit, load check | L1–L3 value | First "wow"; proves the prompts + the context pack fed to Claude. |
| **M2 · Agentic loop** | Nightly Coach Brain assesses readiness + reshapes the week before you wake, with a morning push | **L5** | The north star — needs M0 + M1 beneath it. |
| **M3 · Depth** *(optional)* | Race predictor, taper/race-week planner, "find the link" stats, trends, chat-with-your-data | L4–L5 extras | Polish after the loop works. |

Each milestone gets its own spec → plan → implementation cycle. **This spec details M0.**

---

## 5. Milestone 0 — Foundation (detailed)

**Goal:** Every signal the coach will ever need flows end-to-end into one local store and is
visible in the app. No AI yet — M0 exists to *prove the data pipeline works*, especially the
fragile Garmin path, before anything is built on top of it.

### 5.1 Success criteria
- Connect Strava once → recent runs land in the DB with HR, pace, distance, splits.
- Garmin worker logs in once (MFA-aware) → last ~30 days of sleep / HRV / Body Battery /
  resting HR land in the DB; daily incremental works non-interactively afterward.
- The Expo app shows connection status, last-sync times, a recent-runs list, and recent
  recovery days.
- A `Sync now` button and an automatic periodic sync both work.
- A fresh clone + `.env` + documented steps gets someone else running their own instance.

### 5.2 Components

**Go core (`backend/`)** — owns the database (single writer) and all app-facing API.
- Router **chi**; SQLite via **`modernc.org/sqlite`** (pure Go, no CGO); migrations as
  embedded SQL via **goose**; config from env (`godotenv` + `envconfig`).
- Responsibilities: Strava OAuth + activity sync; invoke the Python Garmin worker and
  persist its output; serve the REST API; a simple periodic sync ticker (the *agentic*
  schedule is M2).
- App ↔ backend auth: a single **bearer token** (env `API_TOKEN`); single-user, self-hosted.

**Python Garmin worker (`garmin-worker/`)** — a thin, stateless fetcher (not a server).
- `python-garminconnect` with the **widget+cffi** login strategy.
- Commands: `worker.py login` (interactive, one-time, handles MFA, persists OAuth tokens to
  a configured token dir) and `worker.py fetch --since YYYY-MM-DD` (non-interactive, reuses/
  refreshes tokens, **prints normalized JSON to stdout**).
- Go invokes it via `os/exec`, parses JSON, writes to SQLite → **one DB writer (Go)**, no
  shared-file locking, no extra network service.
- *Alternative considered:* long-running localhost FastAPI service — rejected for M0 (more
  moving parts) but the boundary is clean enough to swap later if needed.

**Expo app (`app/`)** — expo-router; **React Query** for data; `expo-secure-store` for the
backend URL + token.
- M0 screens: **Connect/Settings** (backend URL + token, Strava "Connect" → opens OAuth,
  Garmin status, "Sync now") and **Home/Status** (connection + last-sync + counts, a
  recent-runs list, a recent-recovery list). No AI screens yet.

### 5.3 Data model (SQLite)

Principle: **store the raw payload + a few normalized columns.** M1's metrics engine derives
everything else, so we never lose fidelity.

- `strava_tokens` — access_token, refresh_token, expires_at
- `activities` — strava_id (PK), start_time, type, name, distance_m, moving_time_s,
  elapsed_time_s, avg_hr, max_hr, avg_speed, avg_cadence, elevation_gain_m, `raw_json`
- `activity_splits` — activity_id, idx, distance_m, elapsed_time_s, avg_hr, avg_speed
- `garmin_sleep` — date (PK), duration_s, deep_s, light_s, rem_s, awake_s, score, `raw_json`
- `garmin_hrv` — date (PK), last_night_avg_ms, status, `raw_json`
- `garmin_body_battery` — date (PK), charged, drained, high, low, `raw_json`
- `garmin_rhr` — date (PK), resting_hr, `raw_json`
- `sync_log` — source, last_synced_at, status, error

### 5.4 Data flow
- **Strava connect:** app → `GET /api/strava/connect` (returns authorize URL) → user
  authorizes in browser → `GET /api/strava/callback` → Go exchanges code, stores tokens.
- **Strava sync:** Go refreshes access token if expired → pulls activities since
  `last_synced_at` → upserts `activities` (+ splits). *(Per-second HR/pace streams deferred
  to M1, where the metrics engine needs them for decoupling/time-in-zone.)*
- **Garmin sync:** Go runs `worker.py fetch --since <last>` → parses JSON → upserts the four
  `garmin_*` tables.
- **App reads:** `GET /api/status`, `GET /api/activities?limit=`, `GET /api/recovery?days=`.

### 5.5 API surface (Go)
`GET /health` · `GET /api/status` · `GET /api/strava/connect` · `GET /api/strava/callback` ·
`POST /api/sync` · `GET /api/activities` · `GET /api/recovery` — all under bearer auth except
`/health` and the OAuth callback.

### 5.6 Repo layout (public-repo ready)
```
backend/   cmd/server, internal/{api,store,strava,garmin,sync,config}, migrations/
garmin-worker/   worker.py, requirements.txt
app/   expo-router screens, src/api (client + react-query hooks)
.env.example   README.md   .gitignore (secrets, token dir, *.db)
```

### 5.7 Testing
- Go: table-driven tests for token refresh + upsert logic; `httptest` for handlers; recorded
  Strava/Garmin JSON fixtures (no live calls in CI).
- Python: a `--dry-run`/fixture smoke test for the JSON shape.
- Manual integration: `make sync` against real accounts.

### 5.8 Secrets & public-repo hygiene
`.env.example` documents: `STRAVA_CLIENT_ID`, `STRAVA_CLIENT_SECRET`, `STRAVA_REDIRECT_URL`,
`GARMIN_EMAIL`, `GARMIN_PASSWORD` (or token-dir path), `API_TOKEN`, `DB_PATH`,
`ANTHROPIC_API_KEY` (stub until M1). `.gitignore` excludes `.env`, the Garmin token dir, and
`*.db`. README documents the full self-host setup including the one-time `worker.py login`.

### 5.9 Out of scope for M0 (kept bounded; deferred to M1–M3)
Activity streams, the metrics engine (zones/decoupling/load), any Claude calls, the agentic
nightly decision loop, push notifications, race/taper features.

## 6. Risks & mitigations
- **Garmin scraping breaks (highest risk).** Mitigate: isolate behind the Python worker;
  pin a known-good `python-garminconnect`; graceful degradation to Strava-only; clear error
  surfaced in `sync_log` and the app.
- **Garmin MFA / token expiry.** Mitigate: one-time interactive `login`; persisted, auto-
  refreshed tokens; documented re-login step.
- **Strava rate limits.** Mitigate: incremental sync since `last_synced_at`; backoff.
- **Secrets in a public repo.** Mitigate: env-only secrets, `.gitignore`, `.env.example`.

## 7. Open questions (revisit at M1+)
- Streams storage shape (raw stream blobs vs downsampled) when the metrics engine arrives.
- Whether to move SQLite → Postgres/Supabase before or after M2.
- Notification provider details (Expo Push) confirmed at M2.
