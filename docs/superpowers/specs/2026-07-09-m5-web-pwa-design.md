# M5 — Web + PWA rewrite (design spec)

Date: 2026-07-09
Status: proposed
Visual source of truth: claude.ai/design project `c631d32a-ebde-478f-8956-c590b4f0640f`
("Garmin Running Coach PWA") — `CoachApp.dc.html` composing `Onboarding`,
`RunCoachPhone` (direction A), `CoachWeb`.

## 1. Goal

Replace the native Expo client with a single responsive **website + installable
PWA**, served by the Go backend itself, and move all credential entry into the
web UI:

- **Owner login** (password + session cookie) replaces the pasted `API_TOKEN`.
- **Garmin login** (email/password/MFA form) replaces `GARMIN_EMAIL`/`GARMIN_PASSWORD`
  in `.env` and the CLI-only `make garmin-login`.
- **Claude stays on Claude Code** (`claude -p`, subscription, $0/token). Default
  auth is host-level `claude auth login`; Settings optionally accepts a
  `claude setup-token` value for headless hosts.
- **Web Push** replaces Expo push for the 06:00 daily-coach notification.

Deployment model is unchanged: **self-hosted, single user per instance.**

## 2. Non-goals

- Multi-user accounts, tenancy, or any hosted service.
- Pay-per-use Anthropic API keys (subscription path only).
- OAuth "Sign in with Claude" (does not exist for third-party apps).
- Recording/starting runs from the web app (watch-side concern).
- Offline browsing of historical data (SW caches the app shell only; data
  offlining can come later).
- Keeping the Expo app alive in any form.

## 3. Architecture

```
help-my-run/
├── backend/                 Go core (unchanged role) + new front door
│   └── internal/
│       ├── webui/           go:embed of built SPA (dist/) + SPA-fallback handler
│       ├── auth/            owner password (argon2id), sessions, middleware
│       └── webpush/         VAPID keys, subscriptions, sender (replaces push/)
├── web/                     NEW — Vite + React + TS + TanStack Query + vite-plugin-pwa
├── garmin-worker/           + `login-web` subcommand (non-interactive login w/ MFA)
└── app/                     DELETED at end of migration
```

- **Single origin, single binary.** `make build`: `npm run build` in `web/` →
  output copied to `backend/internal/webui/dist/` → `go build` embeds it
  (`//go:embed all:dist`). Router: `/api/*` + `/health` as today; everything
  else serves static assets with `index.html` fallback (client-side routing).
- **Dev loop:** `vite dev` proxies `/api` + `/health` to `localhost:8080`
  (same-origin semantics in dev too; no CORS anywhere).
- Engines (sync, coach, agent, progress, streams, chat, readiness, metrics)
  are untouched except where noted (§9 push seam, §11 profile fields).

## 4. Owner auth

- **State machine:** `GET /api/auth/state` (public) → `{setup_required}` |
  `{authed}` | `{unauthed}`. SPA routes to Onboarding / Login / app accordingly.
- **Setup:** `POST /api/setup {password}` — only while no password exists
  (409 afterwards). Stores argon2id hash (settings row), creates the session,
  and generates the initial **API token** (§ below).
- **Login/logout:** `POST /api/login {password}` → session; `POST /api/logout`.
  Sessions: 256-bit random id, SHA-256 at rest in `sessions` table, 30-day
  sliding expiry. Cookie: `HttpOnly; SameSite=Lax; Path=/`, plus `Secure` when
  served over TLS or behind `X-Forwarded-Proto: https`. In-memory exponential backoff on
  failed logins. `POST /api/auth/password {current,new}` to change.
- **Middleware:** replaces `BearerAuth` — accepts session cookie **or**
  `Authorization: Bearer <api-token>`. Applied to the same protected group as
  today.
- **API token (machine credential):** kept for `make sync`/scripts. Generated
  at setup and on regenerate — displayed once at that moment, stored only as a
  hash; Settings offers regenerate. `.env` `API_TOKEN` is removed.
- **CSRF:** SameSite=Lax + reject non-GET requests whose `Origin` /
  `Sec-Fetch-Site` indicates a cross-site caller (bearer-token requests exempt).
- **Lockout escape hatch:** `server --reset-password` clears the hash so setup
  runs again (sessions and API token revoked at the same time).

## 5. UI — design system & screens

### 5.1 Tokens (extracted from the design files)

| Token | Value |
|---|---|
| bg base / gradient | `#07090C`; app shell `linear-gradient(180deg,#0E1319,#0A0D12 55%,#07090C)` |
| surface / border | card `#12161C` / `#212833`; subtle `#0F1319` / `#1c232d`; inset `#0B0E13` / `#232a34`; hairline `#171d25` |
| text | `#E7ECF0` primary, `#C4CDD6` secondary, `#8A96A3` muted, `#566270` label, `#4b5560` faint |
| accent green | `#5FD08B` (on-accent text `#06210f`, hover `#86E5A6`, tint `rgba(95,208,139,.07-.12)` + border `.3`) |
| amber / red | `#E8B24C` / `#E8685C` (readiness states, deltas) |
| HR zones | Z1 `#2f6f52` · Z2 `#5FD08B` · Z3 `#E8B24C` · Z4 `#E8935C` · Z5 `#E8685C`; CrossFit bar `#3a4657` |
| fonts | Space Grotesk (UI), JetBrains Mono (labels `// LIKE THIS`, numerals, letter-spaced small caps). Self-hosted via `@fontsource` (no Google CDN — PWA must work offline/LAN) |
| radii | cards 18–20px, tiles 12–14px, buttons 11–14px, chips/pills 20–22px |
| brand | "RUNNING ON AI" wordmark; ↓ glyph logo tile |

Charts are inline SVG (sparklines, area chart, stacked bars, zone band) exactly
as in the design — no chart library.

### 5.2 Screens & data mapping

| Screen | Content (from design) | Data |
|---|---|---|
| **Onboarding** (§6) | 8-step wizard | `/api/setup`, `/api/garmin/*`, `/api/fitness`, `PUT /api/profile`, first `POST /api/sync` |
| **Today** | readiness verdict card (word + score + HRV/RHR driver boxes, colored edge bar), 6 signal tiles (HRV, sleep, Body Battery, resting HR, load a:c, Garmin readiness; value+delta; sparklines on desktop), "Coach reshaped your day → why ›" banner (→ Coach), today's-session card (title, target, amber changed-chip, **Details** primary + "Ask coach"), last-session row (→ run detail) | `GET /api/today` (readiness, session, decision/changed reason, undo), `GET /api/recovery?days=7` (tiles + sparklines), `GET /api/activities?limit=1` |
| **Trends** | aerobic-engine hero (easy pace @ ref HR, 12-wk area chart, Δ badge), 3 delta chips, HRV-baseline + resting-HR mini charts, sleep→pace insight card, weekly run×CrossFit stacked bars + a:c | `GET /api/progress?weeks=12` (report extended where fields missing: pace@refHR series, weekly load split, insights[]), `POST /api/progress/analyze` for the LLM read |
| **Coach** | chat bubbles (coach left, user right green-tint), suggestion chips ("Why easy today?", "Am I improving?", "Running × CrossFit?" — client-side canned prompts), input bar | `GET/POST/DELETE /api/chat` |
| **Run detail** (`/runs/:id`) | back header + "✓ on target" pill, 4 stat tiles, intervals list (conditional — see §17), HR-zone band + legend, "ASK COACH ABOUT THIS RUN ›" | `GET /api/activities/{id}/analysis`, `POST .../stream/fetch` |
| **Plan** (`/plan`) | not in mock — existing M1 features restyled to system: week plan view, CrossFit schedule photo upload + parse + generate | `GET /api/plan`, `POST /api/crossfit/parse`, `POST /api/plan/generate` |
| **Settings** (`/settings`) | not in mock — cards in the same language: Garmin (status/connect/disconnect), Claude (status + optional setup-token), Notifications (push subscribe toggle + test), Sync (status, Sync now), Security (change password, API token regenerate — displayed once, logout) | `/api/garmin/status`, `/api/claude/*`, `/api/push/*`, `/api/status`, `/api/sync`, auth endpoints |
| **Login** | minimal card, brand + password field | `/api/login` |

### 5.3 Layout

- **< 1024px:** bottom tab bar `TODAY · TRENDS · COACH` (mono, letter-spaced,
  green active bar) per `RunCoachPhone` direction A. Run detail/Plan/Settings
  are pushed routes. Settings reachable from a header glyph.
- **≥ 1024px:** `CoachWeb` shell — 236px left rail (brand, nav with green
  active dot/tint, "AGENT · ARMED / Next run 06:00" card, profile chip),
  content header (mono breadcrumb, title, readiness pill), two-column content
  grids as mocked. Agent card ← `GET /api/status` extended with the
  scheduler's next daily-run time.
- Same React routes/components throughout; the shell (tabs vs rail) switches
  on a breakpoint.

### 5.4 Deviations from the mock (accepted)

1. "Start run" → **Details** (primary): a PWA cannot start a watch activity.
2. Onboarding gains the SECURE step (8 steps, not 7).
3. Garmin connect is a real credential + MFA form, not a one-tap button.
4. Phone frame/notch/status bar in mocks = device chrome, not implemented.
5. Fonts self-hosted rather than Google Fonts CDN.

## 6. Onboarding flow

`WELCOME → SECURE → CONNECT GARMIN → GOAL → MARKERS → RHYTHM → GUARDRAILS → READY`
(progress `n/8`, back button per design; SECURE and CONNECT cannot be skipped,
later steps prefill sensible defaults and can be revisited via Settings/Profile.)

- **SECURE:** set owner password → `POST /api/setup` (also mints session; the
  remaining steps run authenticated).
- **CONNECT GARMIN:** §7 flow; on success show the design's "NOW SYNCING"
  checklist driven by the first `POST /api/sync` (item states from sync result
  counts; purely presentational).
- **GOAL:** multi-select chips (`Fuel my CrossFit`, `General fitness & health`,
  `Train for a race`) → `profile.goals`.
- **MARKERS:** grid of Zone-2 ceiling, LTHR, easy pace, 5K — prefilled from
  `GET /api/fitness` + existing profile where derivable, all editable →
  profile HR/pace fields.
- **RHYTHM:** steppers runs/week + CrossFit days/week, rest-day picker →
  `profile.week`.
- **GUARDRAILS:** five toggles as designed (no back-to-back hard days, protect
  long run, easy-stays-easy, HRV back-off, running ≤55% load) →
  `profile.guardrails`.
- **READY:** summary card ("NIGHTLY PULL · ARMED / AGENT · RUNS 06:00 DAILY /
  RUNNING CAP …") + push-permission prompt (§9) → "Open today".

## 7. Garmin connect (web login)

### 7.1 Endpoints

| Endpoint | Behavior |
|---|---|
| `GET /api/garmin/status` | `{connected, tokenstore_present, last_sync}` (moves the existing Settings-screen status concept server-side) |
| `POST /api/garmin/login {email,password}` | spawns worker `login-web`; → `200 {connected:true}` on immediate success, `202 {mfa_required:true, login_id}` when MFA is demanded, `4xx/502 {error}` otherwise |
| `POST /api/garmin/login/mfa {login_id, code}` | writes code to the pending worker's stdin → `200` / `401 {error:"bad_code"}` / `410` (expired) |
| `POST /api/garmin/disconnect` | deletes tokenstore contents; status → disconnected |

Server holds at most **one** pending login (mutex), with a 5-minute timeout
that kills the subprocess. Credentials are passed via stdin (never argv/env),
used once, never persisted; log lines must never echo them.

### 7.2 Worker `login-web` protocol (new subcommand)

```
stdin  line 1: {"email":"…","password":"…"}
stdout        : {"status":"mfa_required"}          (only when Garmin asks)
stdin  line 2: {"code":"123456"}                   (Go writes when user submits)
stdout final  : {"status":"ok","tokenstore":"…"}   | {"status":"error","error":"…"}
exit          : 0 on ok, 1 on error
```

Implementation: `GarminClient.login_interactive(prompt_mfa=…)` already takes a
callback; `login-web`'s `prompt_mfa` prints the `mfa_required` line and blocks
on `stdin.readline()`. CLI `login` stays for those who prefer the terminal.

## 8. Claude connection

- Engine unchanged: `internal/llm` shells out to `claude -p` under the
  subscription. **Default:** host ran `claude auth login` once → nothing to
  configure in the UI.
- `GET /api/claude/status` → `{binary_found, authenticated, model}`.
  Implementation: `claude -p` one-word probe with short timeout, result cached
  ~10 min (avoid burning quota); classify via existing `llm.ClassifyFailure`.
- `PUT /api/claude/token {token}` stores a `claude setup-token` value in the
  settings table; `DELETE` removes it. When present, `ExecRunner` gains
  `cmd.Env = append(os.Environ(), "CLAUDE_CODE_OAUTH_TOKEN="+tok)`.
  (Env-var name verified against current Claude Code docs at implementation
  time; it is the documented long-lived-token variable.)
- Settings card mirrors the README guidance: subscription only, never an API
  key; token field marked "headless hosts only".

## 9. Web Push (replaces Expo push)

- `internal/webpush` using `github.com/SherClockHolmes/webpush-go`. VAPID
  keypair generated on first boot, persisted in DB.
- `GET /api/push/vapid-public-key`; `POST /api/push/subscribe {subscription}`
  (upsert; multiple subscriptions allowed — phone + desktop);
  `DELETE /api/push/subscribe {endpoint}`; expired/410 subscriptions pruned on
  send. `POST /api/push/test` fires a test notification.
- The M2 `Pusher` seam is re-implemented by the webpush sender (payload:
  title/body/url). `agent.RunDaily` behavior unchanged: 06:00 daily decision →
  "Coach reshaped your day — Easy · 8 km" → click opens `/` (Today).
- Expo path fully removed: `internal/push`, `POST /api/push/register`,
  `device_tokens` table (drop via migration).
- SW `push` + `notificationclick` handlers live in the custom service worker
  (§10). iOS: push requires the PWA installed to home screen (iOS 16.4+);
  documented in README.

## 10. PWA specifics

- `vite-plugin-pwa` (Workbox `injectManifest` — we own the SW file to add the
  push handlers): precache app shell + fonts + icons; **network-only for
  `/api/*`** (coach data must never be stale); offline fallback page for
  navigation when unreachable.
- Manifest: name "Running on AI", `display: standalone`, `theme_color #07090C`,
  `background_color #07090C`, icons reused from `app/assets/images/`
  (icon/adaptive/favicon set, copied into `web/` before `app/` is deleted).
- **HTTPS requirement:** SW/install/push need a secure context. README gains a
  deployment section: Caddy reverse-proxy or `tailscale serve` one-liners;
  plain `http://<lan-ip>` keeps working as a normal website (no install/push).

## 11. Data & schema changes (SQLite migrations)

1. `auth_settings` single-row table: password hash, API-token hash,
   claude setup-token, VAPID keypair.
2. `sessions` (id-hash, created, last_seen, expiry).
3. `push_subscriptions` (endpoint PK, p256dh, auth, created).
4. `drop table device_tokens`.
5. `profile` gains: `goals` (json array), `week` (json: runs, crossfit_days,
   rest_day), `guardrails` (json object of the five booleans). Coach/chat
   prompt packs include the new fields where they already include profile.
6. `GET /api/recovery` gains `?days=N` (default keeps current behavior) —
   tiles need 7-day series.
7. `GET /api/progress` report extended (only if absent after code inspection):
   easy-pace@ref-HR weekly series, weekly run/CrossFit load split, insight
   sentences (sleep→pace computed deterministically from stored dailies+runs).
8. `GET /api/status` gains `agent_next_run` (scheduler) for the rail card.

## 12. Error handling conventions

- API: existing `{error}` JSON shape everywhere; 401 triggers SPA redirect to
  login (or onboarding when `setup_required`). 502-class Claude failures keep
  `llm.ClassifyFailure` texts, surfaced as inline banners (chat, plan, trends
  analyze) exactly like the current app's error states.
- Garmin login: distinct errors for bad credentials, MFA timeout, rate-limit
  (Garmin 429 — reuse M4 backoff learnings: surface "wait and retry", never
  auto-retry login).
- SW/network: offline banner + retry on failed queries (TanStack Query retry:
  1 for GETs, none for mutations).

## 13. Testing

- **Go:** handler tests for auth state machine (setup/login/backoff/middleware
  cookie+bearer), Garmin login flow with a fake worker binary (happy, MFA,
  timeout, bad code), webpush subscribe/prune with httptest, claude status
  classification, SPA fallback handler.
- **Worker:** pytest for `login-web` protocol (mocked Garmin: ok / mfa / error),
  asserting no credential leakage to logs.
- **Web:** Vitest + React Testing Library per screen (mirrors deleted Jest
  suites: today, trends, coach, run detail, plan, settings, onboarding,
  login); MSW for API mocks; one router-level auth-redirect test.
- `make test` runs all three suites (replaces Expo suite).

## 14. Migration & cleanup

1. Land backend + web behind the same binary (old Expo app keeps working
   against bearer auth until deleted — middleware keeps bearer support).
2. Copy icon assets `app/assets` → `web/public`.
3. Delete `app/` (incl. its CLAUDE.md/AGENTS.md), drop Expo bits from Makefile.
4. README rewrite: setup (no creds in .env), first-run wizard, HTTPS/deploy
   section, headless-Claude section, updated `make` targets.
5. `.env.example` slims to: `DB_PATH, PORT, PYTHON_BIN, WORKER_SCRIPT,
   GARMIN_TOKENSTORE, CLAUDE_BIN, CLAUDE_MODEL, IMAGE_DIR` (removed:
   `API_TOKEN, GARMIN_EMAIL, GARMIN_PASSWORD`).
6. Memory/docs: this spec supersedes the client sections of earlier specs.

## 15. Phasing (implementation-plan skeleton)

1. **M5.1 Foundation:** auth package + migrations + middleware swap; webui
   embed + SPA fallback; Vite scaffold with tokens, shell (tabs/rail), login +
   guarded routing; `make` targets.
2. **M5.2 Screens:** Today, Trends, Coach, Run detail, Plan, Settings against
   existing endpoints (+ `?days`, status extensions).
3. **M5.3 Garmin web login:** worker `login-web`, endpoints, onboarding wizard
   (full 8 steps), profile extensions.
4. **M5.4 Push + PWA polish:** webpush package + seam swap + SW handlers,
   manifest/icons, offline fallback, Expo-push removal.
5. **M5.5 Cleanup:** delete `app/`, README/.env/Makefile, migration notes.

Each phase ends green (`make test`) and usable.

## 16. Open items / accepted gaps

- **Intervals on run detail:** rendered only when lap/split data exists in
  stored Garmin data; if the .FIT/store doesn't yield laps in v1 the section
  hides (design degrades gracefully). Decide during M5.2 from actual data.
- **Marker auto-detect quality:** prefill is best-effort from `/api/fitness`;
  fields are editable, so imperfect detection is acceptable.
- **`CLAUDE_CODE_OAUTH_TOKEN` env-var name:** verify against Claude Code docs
  during M5.1 (one-line change if renamed).
- **Trends payload gaps:** exact `/api/progress` field inspection happens at
  plan time; anything missing is added server-side (all inputs are already in
  the DB).
