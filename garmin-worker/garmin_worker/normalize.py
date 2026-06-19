"""Pure normalization functions: raw python-garminconnect responses -> contract JSON.

These functions perform NO I/O and import NO garminconnect symbols. They are the
unit-testable core of the worker (CONTRACTS §2.2). The live client (client.py) and
the CLI (worker.py) call these, never the reverse.
"""
from __future__ import annotations

from typing import Any, Optional


def _get(d: Optional[dict], *path: str) -> Any:
    """Safely walk a nested dict by keys; return None on any miss / non-dict."""
    cur: Any = d
    for key in path:
        if not isinstance(cur, dict):
            return None
        cur = cur.get(key)
    return cur


def normalize_sleep_day(date: str, raw: Optional[dict]) -> dict:
    """Map get_sleep_data(date) -> SleepDay (CONTRACTS §2.2)."""
    dto = _get(raw, "dailySleepDTO") or {}
    return {
        "date": date,
        "duration_s": dto.get("sleepTimeSeconds"),
        "deep_s": dto.get("deepSleepSeconds"),
        "light_s": dto.get("lightSleepSeconds"),
        "rem_s": dto.get("remSleepSeconds"),
        "awake_s": dto.get("awakeSleepSeconds"),
        "score": _get(dto, "sleepScores", "overall", "value"),
        "raw_json": raw if raw is not None else {},
    }


def normalize_hrv_day(date: str, raw: Optional[dict]) -> dict:
    """Map get_hrv_data(date) -> HrvDay (CONTRACTS §2.2).

    Caller is responsible for OMITTING dates where get_hrv_data returned None;
    this function is only invoked for non-None payloads.
    """
    summary = _get(raw, "hrvSummary") or {}
    return {
        "date": date,
        "last_night_avg_ms": summary.get("lastNightAvg"),
        "status": summary.get("status"),
        "raw_json": raw if raw is not None else {},
    }


def normalize_body_battery_day(date: str, entry: Optional[dict]) -> dict:
    """Map one entry of get_body_battery(since, until) -> BodyBatteryDay (CONTRACTS §2.2).

    high/low = max/min of bodyBatteryValuesArray values.
    charged/drained: direct keys if present, else derived from value deltas
    (sum of positive deltas = charged; abs(sum of negative deltas) = drained).
    """
    entry = entry or {}
    values = [
        row[2]
        for row in entry.get("bodyBatteryValuesArray") or []
        if isinstance(row, (list, tuple)) and len(row) >= 3 and row[2] is not None
    ]
    high = max(values) if values else None
    low = min(values) if values else None

    charged = entry.get("charged")
    drained = entry.get("drained")
    if charged is None and drained is None and len(values) >= 2:
        pos = 0
        neg = 0
        for prev, nxt in zip(values, values[1:]):
            delta = nxt - prev
            if delta > 0:
                pos += delta
            elif delta < 0:
                neg += delta
        charged = pos
        drained = -neg

    return {
        "date": date,
        "charged": charged,
        "drained": drained,
        "high": high,
        "low": low,
        "raw_json": entry,
    }


def normalize_rhr_day(date: str, raw: Optional[dict]) -> dict:
    """Map get_stats(date) -> RhrDay (CONTRACTS §2.2).

    Source path: get_stats(date)["restingHeartRate"] (confirmed; not get_rhr_day).
    """
    rhr = raw.get("restingHeartRate") if isinstance(raw, dict) else None
    return {
        "date": date,
        "resting_hr": rhr,
        "raw_json": raw,
    }


def build_output(
    *,
    since: str,
    until: str,
    fetched_at: str,
    sleep: list,
    hrv: list,
    body_battery: list,
    rhr: list,
) -> dict:
    """Assemble the full worker stdout object (CONTRACTS §2.1).

    Key order is fixed to match the contract exactly.
    """
    return {
        "since": since,
        "until": until,
        "fetched_at": fetched_at,
        "sleep": sleep,
        "hrv": hrv,
        "body_battery": body_battery,
        "rhr": rhr,
    }
