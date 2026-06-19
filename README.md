# help-my-run

A self-hostable, single-user AI running coach. It pulls your runs from **Strava** and your recovery data (sleep, HRV, Body Battery, resting HR) from **Garmin Connect** into a local database, then (in a later milestone) uses Claude to coach you. M0 delivers the data foundation: connect Strava, log in to Garmin once, sync, and view your runs + recovery in a small Expo app.

## Architecture

- **Go core** (`backend/`) owns the SQLite database, the REST API, and the periodic sync scheduler. It is the single source of truth.
- **Python Garmin worker** (`garmin-worker/`) is a thin, stateless subprocess that the Go core invokes to fetch Garmin data and print one JSON object to stdout. It is the only component that talks to Garmin.
- **Expo app** (`app/`) is the client. It stores your backend URL + API token in `expo-secure-store` and reads/writes the Go API over HTTP.

## Prerequisites

- **A Strava API application.** Create one at <https://www.strava.com/settings/api>. Copy the **Client ID** and **Client Secret** into `.env` (`STRAVA_CLIENT_ID`, `STRAVA_CLIENT_SECRET`). Set the application's **Authorization Callback Domain** to match `STRAVA_REDIRECT_URL` (e.g. `localhost`); the redirect URL must point at `/api/strava/callback`.
- **A Garmin Connect account** (email + password) for the one-time `worker.py login`.
- **An Anthropic API key** is **not needed for M0** — it is loaded but unused until M1. You can leave `ANTHROPIC_API_KEY` blank for now.
- Go 1.22+, Python 3.11+, and Node.js 18+ installed.

## Setup

```bash
git clone <your-fork-url> help-my-run
cd help-my-run

# 1. Configure secrets
cp .env.example .env
# edit .env and fill in STRAVA_CLIENT_ID, STRAVA_CLIENT_SECRET, STRAVA_REDIRECT_URL,
# API_TOKEN, GARMIN_EMAIL, GARMIN_PASSWORD (and any optional overrides)

# 2. Backend deps
cd backend && go mod download && cd ..

# 3. Garmin worker deps
cd garmin-worker && python -m venv .venv && . .venv/bin/activate && pip install -r requirements.txt && deactivate && cd ..

# 4. App deps
cd app && npm install && cd ..
```

## One-time Garmin login

Run the interactive login once. It will prompt for an **MFA code** if your Garmin account has multi-factor auth enabled. On success it persists OAuth tokens to `GARMIN_TOKENSTORE` (default `~/.garminconnect`) so subsequent syncs run non-interactively.

```bash
make garmin-login
```

## Running

```bash
make run-backend   # starts the Go API + periodic sync ticker on $PORT (default 8080)
make run-app       # starts the Expo dev server (open in Expo Go or a dev build)
```

In the app's Settings screen, enter the backend URL (e.g. `http://<your-LAN-ip>:8080`) and your `API_TOKEN`, then connect Strava.

## Syncing

```bash
make sync          # POSTs /api/sync (the backend must be running)
```

## Testing

```bash
make test          # runs the Go, Python worker, and Expo app test suites
```

## Security note

All secrets live in `.env`, which is **gitignored**. Never commit credentials (Strava secret, API token, Garmin password) or your Garmin token directory. Review `.gitignore` before pushing.

## Disclaimer

Garmin access uses the unofficial [`python-garminconnect`](https://github.com/cyberjunky/python-garminconnect) library — Garmin provides no public API for this data. Use it only for **personal access to your own account**. It may break at any time if Garmin changes their site, and you are responsible for complying with Garmin's terms of service.
