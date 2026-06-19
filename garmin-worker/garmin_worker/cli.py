"""CLI for the help-my-run Garmin worker (CONTRACTS §2).

Subcommands:
  login                       interactive one-time SSO; persists OAuth tokens
  fetch --since YYYY-MM-DD [--until YYYY-MM-DD] [--dry-run]
                              non-interactive; prints §2.1 JSON to stdout

Discipline (CONTRACTS §2 / §2.4):
  - stdout carries ONLY the single JSON object (or nothing on failure)
  - all diagnostics go to stderr
  - exit 0 on success; non-zero on auth/connection/validation failure
"""
from __future__ import annotations

import argparse
import datetime as _dt
import json
import sys
from typing import Optional, Sequence

from . import client, normalize

PROG = "worker.py"


def validate_date(value: str) -> str:
    """Accept exactly YYYY-MM-DD; raise ValueError otherwise.

    strptime("%Y-%m-%d") is lenient on some Python versions (e.g. it accepts
    "2026-6-14"), so we additionally require the input to be the canonical,
    zero-padded form by round-tripping against date.isoformat().
    """
    try:
        parsed = _dt.datetime.strptime(value, "%Y-%m-%d").date()
    except (ValueError, TypeError) as exc:
        raise ValueError(f"invalid date {value!r}; expected YYYY-MM-DD") from exc
    canonical = parsed.isoformat()
    if value != canonical:
        raise ValueError(f"invalid date {value!r}; expected YYYY-MM-DD")
    return canonical


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog=PROG, description="help-my-run Garmin worker")
    sub = parser.add_subparsers(dest="command", required=True)

    sub.add_parser("login", help="interactive one-time Garmin SSO login")

    fetch = sub.add_parser("fetch", help="fetch recovery metrics; print JSON to stdout")
    fetch.add_argument("--since", required=True, help="inclusive start date YYYY-MM-DD")
    fetch.add_argument("--until", default=None, help="inclusive end date YYYY-MM-DD (default: today)")
    fetch.add_argument(
        "--dry-run",
        action="store_true",
        help="emit synthetic contract JSON without contacting Garmin",
    )
    return parser


# ---- synthetic data for --dry-run (mirrors CONTRACTS §2.3) -----------------
_DRY_SLEEP_RAW = {
    "2026-06-14": {"dailySleepDTO": {"sleepTimeSeconds": 26400, "deepSleepSeconds": 6000, "lightSleepSeconds": 14100, "remSleepSeconds": 5400, "awakeSleepSeconds": 900, "sleepScores": {"overall": {"value": 79}}}},
    "2026-06-15": {"dailySleepDTO": {"sleepTimeSeconds": 27000, "deepSleepSeconds": 6300, "lightSleepSeconds": 14400, "remSleepSeconds": 5400, "awakeSleepSeconds": 900, "sleepScores": {"overall": {"value": 82}}}},
}
_DRY_HRV_RAW = {
    "2026-06-15": {"hrvSummary": {"lastNightAvg": 48, "lastNight5MinHigh": 70, "weeklyAvg": 46, "status": "BALANCED"}, "hrvReadings": []},
}
_DRY_BB_RANGE = [
    {"date": "2026-06-14", "charged": 60, "drained": 75, "bodyBatteryValuesArray": [[1718323200000, "ACTIVE", 88], [1718366400000, "ACTIVE", 16]]},
    {"date": "2026-06-15", "charged": 62, "drained": 78, "bodyBatteryValuesArray": [[1718409600000, "ACTIVE", 91], [1718452800000, "ACTIVE", 14]]},
]
_DRY_STATS_RAW = {
    "2026-06-14": {"restingHeartRate": 48, "totalSteps": 9000},
    "2026-06-15": {"restingHeartRate": 47, "totalSteps": 11000},
}


def _run_dry_fetch(since: str, until: str) -> dict:
    """Build the §2.1 object from baked-in synthetic data (no Garmin)."""
    sleep = [normalize.normalize_sleep_day(d, raw) for d, raw in sorted(_DRY_SLEEP_RAW.items())]
    hrv = [normalize.normalize_hrv_day(d, raw) for d, raw in sorted(_DRY_HRV_RAW.items())]
    body_battery = [normalize.normalize_body_battery_day(e["date"], e) for e in _DRY_BB_RANGE]
    rhr = [normalize.normalize_rhr_day(d, raw) for d, raw in sorted(_DRY_STATS_RAW.items())]
    return normalize.build_output(
        since=since,
        until=until,
        fetched_at="2026-06-15T05:00:12Z",
        sleep=sleep,
        hrv=hrv,
        body_battery=body_battery,
        rhr=rhr,
    )


def main(argv: Optional[Sequence[str]] = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    if args.command == "login":
        try:
            client.GarminClient.login_interactive(prompt_mfa=lambda: input("MFA code: "))
        except ValueError as exc:
            print(str(exc), file=sys.stderr)
            return 2
        except Exception as exc:  # garminconnect auth/connection errors
            print(f"login failed: {exc}", file=sys.stderr)
            return 1
        print(f"login ok; tokens saved to {client.tokenstore_path()}", file=sys.stderr)
        return 0

    # command == "fetch"
    try:
        since = validate_date(args.since)
        until = validate_date(args.until) if args.until else _dt.date.today().isoformat()
    except ValueError as exc:
        print(str(exc), file=sys.stderr)
        return 2

    if args.dry_run:
        output = _run_dry_fetch(since, until)
        json.dump(output, sys.stdout)
        sys.stdout.write("\n")
        return 0

    # Live fetch wiring is added in Task 16.
    print("live fetch is not wired yet", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
