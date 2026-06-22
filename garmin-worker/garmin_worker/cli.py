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

from . import client, fetcher, normalize
from .fetcher import run_fetch

try:  # re-exported from package root (Garmin research §5)
    from garminconnect import GarminConnectAuthenticationError
except Exception:  # pragma: no cover - import guard for environments w/o lib

    class GarminConnectAuthenticationError(Exception):
        """Fallback when garminconnect is unavailable (e.g. unit tests)."""

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

    strm = sub.add_parser("stream", help="download+parse one activity FIT; print §2.6 JSON to stdout")
    strm.add_argument("--activity-id", required=True, help="GARMIN activity id to download")
    strm.add_argument("--echo-id", default=None, help="Strava id to echo as activity_id (default: --activity-id)")
    strm.add_argument(
        "--dry-run",
        action="store_true",
        help="emit synthetic §2.6 JSON without contacting Garmin",
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
_DRY_VO2MAX_RAW = {
    "2026-06-14": {"userId": 1, "generic": {"calendarDate": "2026-06-14", "vo2MaxValue": 51.0, "fitnessAge": 30}, "cycling": None},
    "2026-06-15": {"userId": 1, "generic": {"calendarDate": "2026-06-15", "vo2MaxValue": 52.0, "fitnessAge": 30}, "cycling": None},
}


def _run_dry_fetch(since: str, until: str) -> dict:
    """Build the §2.1 object from baked-in synthetic data (no Garmin)."""
    sleep = [normalize.normalize_sleep_day(d, raw) for d, raw in sorted(_DRY_SLEEP_RAW.items())]
    hrv = [normalize.normalize_hrv_day(d, raw) for d, raw in sorted(_DRY_HRV_RAW.items())]
    body_battery = [normalize.normalize_body_battery_day(e["date"], e) for e in _DRY_BB_RANGE]
    rhr = [normalize.normalize_rhr_day(d, raw) for d, raw in sorted(_DRY_STATS_RAW.items())]
    vo2max = [normalize.normalize_vo2max_day(d, raw) for d, raw in sorted(_DRY_VO2MAX_RAW.items())]
    return normalize.build_output(
        since=since,
        until=until,
        fetched_at="2026-06-15T05:00:12Z",
        sleep=sleep,
        hrv=hrv,
        body_battery=body_battery,
        rhr=rhr,
        vo2max=vo2max,
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

    if args.command == "stream":
        echo_id = int(args.echo_id) if args.echo_id else int(args.activity_id)
        if args.dry_run:
            output = normalize.build_fit_output(
                activity_id=echo_id,
                fetched_at="2026-06-22T05:00:12Z",
                series={"t": [0, 1, 2], "hr": [104, 105, 106], "v": [0.0, 1.59, 1.66], "dist": [0.0, 2.9, 5.6]},
            )
            json.dump(output, sys.stdout)
            sys.stdout.write("\n")
            return 0
        fetched_at = (
            _dt.datetime.now(_dt.timezone.utc).replace(microsecond=0).strftime("%Y-%m-%dT%H:%M:%SZ")
        )
        try:
            live = client.GarminClient.resume()
            output = fetcher.run_fit_fetch(live, activity_id=args.activity_id, echo_id=echo_id, fetched_at=fetched_at)
        except GarminConnectAuthenticationError as exc:
            print(f"garmin authentication failed ({exc}); re-run worker.py login", file=sys.stderr)
            return 1
        except Exception as exc:
            print(f"stream fetch failed: {exc}", file=sys.stderr)
            return 1
        json.dump(output, sys.stdout)
        sys.stdout.write("\n")
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

    fetched_at = (
        _dt.datetime.now(_dt.timezone.utc)
        .replace(microsecond=0)
        .strftime("%Y-%m-%dT%H:%M:%SZ")
    )
    try:
        live = client.GarminClient.resume()
        output = run_fetch(live, since=since, until=until, fetched_at=fetched_at)
    except GarminConnectAuthenticationError as exc:
        print(
            f"garmin authentication failed ({exc}); re-run worker.py login",
            file=sys.stderr,
        )
        return 1
    except Exception as exc:  # connection / rate-limit / unexpected
        print(f"fetch failed: {exc}", file=sys.stderr)
        return 1

    json.dump(output, sys.stdout)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
