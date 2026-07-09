# M6 — "In your pocket" (design spec)

Date: 2026-07-09 · Status: approved (roadmap sign-off 2026-07-09)
Goal: the coach runs unattended on a home machine and reaches the owner's
phone anywhere — privately (Tailscale), reliably (systemd + backups), with the
owner in control of their sessions.

## 1. Run as a service

- `deploy/helpmyrun.service` — a **user-level** systemd unit (no root):
  `Restart=always`, `WorkingDirectory=` the install dir, `EnvironmentFile=-`
  pointing at the repo `.env`, journald logging.
- `make install`: `make build`, copy `bin/helpmyrun` → `~/.local/bin/`,
  install the unit → `~/.config/systemd/user/`, print the enable one-liners
  (`systemctl --user enable --now helpmyrun`, `loginctl enable-linger $USER`
  so it starts at boot without a login).
- README gains a "Run it as a service" section; `tailscale serve` snippet
  moves next to it.

## 2. Nightly backups

- New `backend/internal/backup` package:
  `Run(db *store.Store, tokenstore, dir string, keep int) (string, error)` —
  `VACUUM INTO '<dir>/helpmyrun-YYYY-MM-DD.db'` (atomic, consistent snapshot,
  works on modernc sqlite), copy the Garmin tokenstore files to
  `<dir>/tokenstore-YYYY-MM-DD/`, then prune both sets to the newest `keep`.
- Wired into the daily scheduler callback in `main.go` right after
  `agent.RunDaily` (same cadence, zero new timers). Config: `BACKUP_DIR`
  (default `<dir of DB_PATH>/backups`), `BACKUP_KEEP` (default 14).
- Failure posture: log loudly, never crash the agent loop.
- README documents restore: stop service, copy snapshot over `DB_PATH`, copy
  tokenstore dir back, start.

## 3. Sessions / devices (the single-owner "registration done properly")

- Migration **00011**: `ALTER TABLE sessions ADD COLUMN user_agent TEXT NOT
  NULL DEFAULT ''` + `ADD COLUMN created_ip TEXT NOT NULL DEFAULT ''`.
  (M7's migration shifts to 00012.)
- `auth.Service.Setup/Login` accept a `SessionMeta{UserAgent, IP}`; handlers
  pass `r.UserAgent()` + `RemoteAddr` (chi RealIP middleware already set).
- Store: `ListSessions()` (last-seen desc), existing delete fns reused.
- API (protected): `GET /api/auth/sessions` →
  `{sessions:[{id_hash, created_at, last_seen_at, user_agent, ip, current}]}`
  (`id_hash` is the SHA-256 of the cookie value — possession grants nothing);
  `DELETE /api/auth/sessions/{idHash}` (revoking the current session = logout);
  `POST /api/auth/sessions/revoke-others` → 204.
- Settings → SECURITY gains a DEVICES block: one row per session (browser/OS
  summary parsed client-side from the UA, created + last-seen, CURRENT badge),
  per-row Revoke, and "Sign out everywhere else".

## 4. Acceptance (manual, owner)

- `make install` → reboot → service is up without logging in.
- Phone on cellular via Tailscale: app loads, morning push arrives.
- A backup file appears after the nightly run; restore drill executed once.
- Settings shows ≥2 devices; revoking one signs it out.

## Plan (execution order)

1. Migration 00011 + store `ListSessions` + auth `SessionMeta` (tests).
2. Sessions endpoints + Settings DEVICES UI (tests).
3. `internal/backup` + config + scheduler wiring (tests: snapshot readable,
   rotation prunes, tokenstore copied, agent loop survives backup failure).
4. `deploy/` unit + `make install` + README sections.
5. Full suites → adversarial review workflow → fix → commit.
