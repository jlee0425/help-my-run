"""Fetch orchestration: drive an injected GarminClient over a date range and
assemble the §2.1 contract object via the pure normalizers.

run_fetch() takes an ALREADY-CONSTRUCTED client so it is fully unit-testable
with a mock (no Garmin login). The CLI (cli.py) constructs the live client via
GarminClient.resume() and passes it here.

Per-day sources (sleep, HRV, RHR) are looped one date at a time.
Body Battery is a single range-native call (Garmin research §3/§4).
HRV None days are OMITTED from the output array (CONTRACTS §2.2).
"""
from __future__ import annotations

import datetime as _dt
import time
from typing import Callable

from . import normalize

# Small politeness delay between per-day calls (Garmin research §4: keep
# request volume low). Overridable in tests via sleep_fn.
_PER_DAY_DELAY_S = 0.2


def _date_range(since: str, until: str):
    start = _dt.date.fromisoformat(since)
    end = _dt.date.fromisoformat(until)
    cur = start
    while cur <= end:
        yield cur.isoformat()
        cur += _dt.timedelta(days=1)


def run_fetch(
    client,
    *,
    since: str,
    until: str,
    fetched_at: str,
    sleep_fn: Callable[[float], None] = time.sleep,
) -> dict:
    """Fetch + normalize the whole window; return the §2.1 dict."""
    sleep = []
    hrv = []
    rhr = []
    vo2max = []

    # Body Battery: one range call for the whole window.
    bb_entries = client.get_body_battery(since, until) or []
    body_battery = [
        normalize.normalize_body_battery_day(entry.get("date"), entry)
        for entry in bb_entries
        if isinstance(entry, dict)
    ]

    # Per-day sources.
    dates = list(_date_range(since, until))
    for i, cdate in enumerate(dates):
        sleep.append(normalize.normalize_sleep_day(cdate, client.get_sleep_data(cdate)))

        hrv_raw = client.get_hrv_data(cdate)
        if hrv_raw is not None:  # CONTRACTS §2.2: omit None HRV days
            hrv.append(normalize.normalize_hrv_day(cdate, hrv_raw))

        rhr.append(normalize.normalize_rhr_day(cdate, client.get_stats(cdate)))

        mm = normalize.normalize_vo2max_day(cdate, client.get_max_metrics(cdate))
        if mm["vo2max"] is not None:  # omit no-data days
            vo2max.append(mm)

        if i < len(dates) - 1:
            sleep_fn(_PER_DAY_DELAY_S)

    # Garmin activities list: one run-type-filtered call over the whole window.
    # A list-fetch failure must NOT fail the whole recovery sync (spec §10).
    try:
        raw_acts = client.get_activities_by_date(since, until, "running") or []
        activities = [
            normalize.normalize_garmin_activity(el)
            for el in raw_acts
            if isinstance(el, dict) and el.get("activityId") is not None
        ]
    except Exception:
        activities = []

    return normalize.build_output(
        since=since,
        until=until,
        fetched_at=fetched_at,
        sleep=sleep,
        hrv=hrv,
        body_battery=body_battery,
        rhr=rhr,
        vo2max=vo2max,
        activities=activities,
    )


def run_fit_fetch(client, *, activity_id: str, echo_id: int, fetched_at: str) -> dict:
    """Download + parse one activity's FIT -> the §2.6 stream object.

    activity_id is GARMIN's download id; echo_id is the Strava id echoed back so
    the Go store row keys correctly (§7 id mapping)."""
    raw = client.download_activity_original(activity_id)
    series = normalize.normalize_fit_stream(raw)
    return normalize.build_fit_output(activity_id=echo_id, fetched_at=fetched_at, series=series)
