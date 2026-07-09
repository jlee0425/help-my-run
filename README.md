# help-my-run · Running on AI

A self-hostable, single-user AI running coach — now a **website + installable PWA**. It pulls your runs **and** your recovery data (sleep, HRV, Body Battery, resting HR) from **Garmin Connect** into a local database, then uses Claude to coach you: a readiness verdict and a (possibly reshaped) session every morning, 12-week trends, a weekly plan periodised around your CrossFit schedule, and a chat that answers from your own data.

Everything ships as **one binary**: the Go server embeds the built web app. Open it in a browser, install it to your phone's home screen, done.

## Architecture

- **Go core** (`backend/`) owns the SQLite database, the REST API, the daily agent + sync scheduler, auth, Web Push — and serves the embedded web UI. Single source of truth, single process.
- **Python Garmin worker** (`garmin-worker/`) is a thin, stateless subprocess the Go core invokes to talk to Garmin (fetch, FIT streams, and the web-driven login with MFA over stdin/stdout).
- **Web app** (`web/`) is a React + Vite SPA (dark "Running on AI" design) built into the binary. Installable as a PWA; morning briefings arrive via Web Push.

## Prerequisites

- **A Garmin Connect account** — you'll sign in inside the app on first run (MFA supported). Credentials are used once to mint OAuth tokens and never stored.
- **A Claude subscription + Claude Code CLI.** All AI features run through `claude -p` at **$0 per token** under your subscription. ⚠️ **Leave `ANTHROPIC_API_KEY` UNSET** — any value there (even a placeholder) switches `claude -p` to metered API billing and fails with a 401.
- Go 1.22+, Python 3.11+, and Node.js 18+ (Node is needed only to build the UI).

## Setup

```bash
git clone <your-fork-url> help-my-run
cd help-my-run

# 1. Paths config (no secrets — see the absolute-path warning inside)
cp .env.example .env

# 2. Worker deps
cd garmin-worker && python -m venv .venv && . .venv/bin/activate && pip install -r requirements.txt && deactivate && cd ..

# 3. Claude Code (subscription path)
claude auth login        # once, on this host — NOT --console
# Headless/VPS with no browser? Run `claude setup-token` on any machine with one,
# then paste the token in the app: Settings → Claude → "Headless server?"

# 4. Build the single binary (builds the web UI + embeds it)
make build
```

## First run

```bash
./bin/helpmyrun          # or: make run-backend (dev mode, runs from source)
```

Open `http://localhost:8080` (or `http://<your-LAN-ip>:8080` from your phone). The first-run wizard walks you through everything:

1. **Secure** — set the owner password (this also mints an API token for scripts, shown once).
2. **Connect Garmin** — sign in with your Garmin email/password; enter the MFA code if asked. Tokens are stored locally; the nightly pull runs unattended from then on.
3. **Goals, numbers, week rhythm, guardrails** — shape how the coach makes calls (e.g. "keep running ≤55% of load so legs stay fresh for lifting").

Then it's live: **Today** (readiness verdict + session), **Trends** (aerobic engine over 12 weeks), **Coach** (ask your data), **Plan** (CrossFit-aware week), Settings.

## Install as an app + notifications

- **Android/desktop Chrome:** you'll get an install prompt (or ⋮ → *Install app*).
- **iPhone:** Share → *Add to Home Screen*. (iOS only allows Web Push for installed PWAs, 16.4+.)
- Enable **morning notifications** in the wizard's last step or Settings → Notifications — the daily agent pushes its verdict when it runs (default 06:00, configurable in Settings).

### HTTPS note

Service workers, installation, and push need HTTPS (localhost is exempt). The simplest options:

```bash
# Tailscale (easiest — private HTTPS on your tailnet):
tailscale serve https / http://localhost:8080

# Caddy (public domain):
# Caddyfile:  yourdomain.example { reverse_proxy localhost:8080 }
```

Plain `http://<lan-ip>:8080` still works as a normal website (no install/push).

## Development

```bash
make run-backend   # Go API on :8080
make run-web       # Vite dev server with /api proxied to :8080 (hot reload)
make test          # Go + Python + web test suites
```

## Scripts / API access

The API accepts `Authorization: Bearer <token>` with the API token from setup (regenerate in Settings → Security — it's shown once). E.g. `make sync` POSTs `/api/sync` with `API_TOKEN` from your environment.

Locked out? `./bin/helpmyrun --reset-password` clears the owner password so setup runs again.

## Security notes

- No secrets live in `.env` — the owner password (argon2id), API token (hashed), optional Claude setup-token, and VAPID keys live in the local SQLite DB; Garmin OAuth tokens live in `GARMIN_TOKENSTORE`.
- Garmin credentials are passed to the login subprocess over stdin, used once, never logged or persisted.
- Sessions are HttpOnly SameSite cookies with a 30-day sliding expiry.

## Disclaimer

Garmin access uses the unofficial [`python-garminconnect`](https://github.com/cyberjunky/python-garminconnect) library — Garmin provides no public API for this data. Use it only for **personal access to your own account**. It may break at any time if Garmin changes their site, and you are responsible for complying with Garmin's terms of service.
