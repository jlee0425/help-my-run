# help-my-run

Self-hosted, **single-owner** AI running coach: pulls runs + recovery (sleep,
HRV, Body Battery, resting HR, VO2max) from Garmin Connect into local SQLite,
then coaches via Claude — daily readiness verdict at 06:00, CrossFit-aware
weekly plans, 12-week trends, chat-with-your-data. Ships as ONE binary: the Go
server embeds the built web app.

## Hard constraints (do not violate)

- **Claude is subscription-only.** All AI runs through `claude -p` (Claude Code
  CLI) under the owner's subscription at $0/token. NEVER introduce
  `ANTHROPIC_API_KEY` usage — any value in that env var makes `claude -p`
  switch to metered billing and 401. The headless path is a `claude
  setup-token` value stored in the DB and injected as `CLAUDE_CODE_OAUTH_TOKEN`
  (see `internal/llm.ExecRunner.EnvFunc`).
- **Garmin is the unofficial `python-garminconnect` library — personal use only.**
  Login is per-IP rate-limited (HTTP 429, lockouts extend on retry): never
  auto-retry logins. Range-native endpoints (body battery, etc.) cap at ~31
  days: chunk every range call ≤28 days (`_RANGE_CHUNK_DAYS`, fetcher.py).
- **Single origin, single user.** The SPA is served by the Go binary; all
  fetches are relative, no CORS anywhere. Auth = one owner password (argon2id)
  + session cookie (`hmr_session`, SameSite=Lax) + one bearer API token
  (`hmr_…`, hashed) for scripts. No secrets in `.env` — credentials live in
  SQLite (`app_settings`) or the Garmin tokenstore.
- **Failed syncs must be loud.** `/api/sync` returns 502 + top-level `error`
  on failure, never a silent 200.

## Architecture

- `backend/` — Go (chi, goose migrations, modernc sqlite; one writer, WAL).
  `cmd/server` wires everything; engines in `internal/`: `sync` (Garmin
  ingest), `agent` (daily loop), `coach` (plans), `readiness`, `progress`,
  `streams` (FIT time-in-zone/decoupling), `chat`, `llm` (claude -p runner),
  `auth`, `webpush` (VAPID), `garmin` (worker subprocess + web-login manager),
  `webui` (go:embed of web/dist).
- `garmin-worker/` — stateless Python subprocess; the ONLY Garmin talker.
  Subcommands: `fetch --since` (prints one JSON contract object), `stream
  --activity-id` (FIT series), `login-web` (stdin creds → MFA handshake →
  tokenstore; creds never in argv/env/logs), `login` (CLI fallback).
- `web/` — React 19 + Vite + TS + TanStack Query SPA (dark
  design; tokens in `src/styles/tokens.css` are canonical). PWA via
  vite-plugin-pwa injectManifest (`src/sw.ts`: precache, offline, Web Push).

## Commands

- `make build` → single binary `bin/helpmyrun` (builds web, embeds, compiles).
- `make run-backend` (Go on :8080) + `make run-web` (Vite, /api proxied) = dev.
- `make test` → all three suites: `go test ./...`, worker pytest, web vitest.
- `bin/helpmyrun --reset-password` → owner lockout escape hatch.
- Web typecheck: `cd web && npm run lint` (tsc).

## Conventions

- Milestones: spec in `docs/superpowers/specs/`, plan in
  `docs/superpowers/plans/`, then TDD execution; roadmap in `docs/ROADMAP.md`.
- Wire JSON is snake_case; Go DTOs in `internal/api/dto.go`; web mirrors in
  `web/src/api/types.ts` — keep them in lockstep.
- Migrations: goose, `backend/internal/store/migrations/NNNNN_name.sql`,
  next free number; store fns take/return typed rows, `ErrNotFound` sentinel.
- Store timestamps are server-side RFC3339 UTC strings.
- Worker discipline: stdout carries ONLY contract JSON; diagnostics to stderr.
- Tests: Go table/handler tests with fakes injected via `api.Deps` seams;
  worker pytest with monkeypatched client; web vitest+RTL with `vi.mock`ed
  hooks. Suite must be green before every commit.
