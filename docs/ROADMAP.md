# help-my-run — Roadmap (post-M5)

Decided 2026-07-09 with Jake. Standing decisions: **self-hosted, single owner**
(multi-user explicitly rejected — Claude runs on the owner's subscription and
the unofficial Garmin library must stay personal-use); **private access via
Tailscale** (no public exposure; a later VPS promotion must stay a config
change, not a project).

Each milestone gets its own spec under `docs/superpowers/specs/` before
implementation. Order rationale: M6 turns M5 into a daily habit (push on the
phone, survives reboots); M7 before M8 because richer Garmin data improves the
chat/agent for free; chat polish last so it has more to say.

## R1 — Post-migration cleanup (timeboxed, before M6)
Debt inventory from the RN→PWA migration — confirmed items only:
- Local `.env` scrubbed of dead credentials (done 2026-07-09: API_TOKEN,
  GARMIN_EMAIL/PASSWORD, STRAVA_*, EXPO_PUSH_BASE_URL); `.env` is path/port
  config only from here on.
- Remove dead config fields: `AnthropicAPIKey` stub, `ExpoPushBaseURL`,
  `AgentTickInterval`.
- Untrack `web/tsconfig.tsbuildinfo` (build artifact) + gitignore.
- Remove dead `fmtDelta` chart helper.
- Split the 789-line `OnboardingPage.tsx` into per-step components.
- `make run-backend` supplies absolute-path defaults (kills the README's
  absolute-paths footgun at the source).
- Add repo `CLAUDE.md` (architecture map, commands, hard constraints).
Skipped deliberately: coach/chat pack-helper unification (mirrored-package
convention), ESLint (tsc gates types), DATA_DIR rework (M6 systemd
WorkingDirectory solves it properly).

## M6 — "In your pocket"
Unattended operation + private remote access + owner-journey polish.
- systemd unit + `make install`; auto-restart; journald logging.
- Nightly SQLite backup (`VACUUM INTO`, keep 14) + Garmin tokenstore copy;
  restore documented and tested.
- Tailscale HTTPS (`tailscale serve`) documented; acceptance: morning briefing
  push received on the phone away from home.
- Sessions/devices management: list active sessions (created / last seen /
  device), revoke one, sign out everywhere. (The "registration done properly"
  item, single-owner scope.)

## M7 — "The whole iceberg"
Deeper Garmin utilization; every signal feeds four consumers (Today tiles,
Trends, daily agent readiness, chat pack).
- New worker fetches (endpoints verified against the installed garminconnect
  before spec): Training Readiness, Training Status + Load Focus, all-day
  stress, race predictor, per-activity laps/splits.
- All range-native calls chunked ≤28 days (Garmin caps ~31d — learned in M5).
- UI: Today gets Garmin-TR + stress tiles (our verdict vs Garmin's); Trends
  gets load-focus split + race-predictor trend; Run detail regains the
  intervals section from the original design.
- Readiness engine considers stress/TR; chat pack enriched.

## M8 — "A coach worth talking to"
- Streaming chat replies (claude -p stream-json → SSE → live typing bubble).
- Markdown rendering in coach bubbles (design-system styled, XSS-safe).
- Context pack additions: today's decision + current plan week (the coach
  currently cannot see its own morning call).
- Persona/format prompt pass; context-aware suggestion chips.

## Explicitly out of scope
- Multi-user registration / SaaS (Claude-subscription + Garmin-ToS collision).
- Splitting frontend/backend deployments (same-origin single binary is a
  feature; CORS/CSRF split buys nothing at this scale).
