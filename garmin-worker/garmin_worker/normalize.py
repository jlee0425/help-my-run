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


def normalize_vo2max_day(date: str, raw) -> dict:
    """Map get_max_metrics(date) -> Vo2maxDay (CONTRACTS §2.2).

    get_max_metrics hits the maxmet DAILY RANGE endpoint
    (`/{cdate}/{cdate}`) and returns the raw JSON with no transform. On
    the real endpoint this is a one-element LIST whose `[0].generic`
    holds the metric (the library's `dict` type hint is wrong); fixtures
    may also be a plain dict. Unwrap the list first, then walk
    `generic.vo2MaxValue`. The ORIGINAL payload (list or dict) is
    preserved in `raw_json`.
    """
    payload = raw
    if isinstance(payload, list):
        payload = payload[0] if payload else None
    val = _get(payload, "generic", "vo2MaxValue")
    return {
        "date": date,
        "vo2max": val,
        "raw_json": raw if raw is not None else {},
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
    vo2max: list,
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
        "vo2max": vo2max,
    }


import io
import zipfile


def _fit_decode(fit_bytes: bytes):
    """Decode a raw .fit byte string -> (messages, errors). Isolated so tests
    can monkeypatch it without the garmin-fit-sdk dependency."""
    from garmin_fit_sdk import Decoder, Stream  # local import; dep added in requirements

    return Decoder(Stream.from_byte_array(fit_bytes)).read()


def normalize_fit_stream(raw: bytes) -> dict:
    """Parse the ORIGINAL download (a ZIP of a .fit) -> the §2.6 series object.

    Units already match Strava: enhanced_speed/speed in m/s, distance in meters.
    t = (timestamp - first_timestamp).total_seconds(); per-record HR that is
    None/absent is DROPPED so a HR-less FIT yields hr=[] (degraded state)."""
    with zipfile.ZipFile(io.BytesIO(raw)) as z:
        fit_name = next(n for n in z.namelist() if n.lower().endswith(".fit"))
        fit_bytes = z.read(fit_name)

    messages, _errors = _fit_decode(fit_bytes)
    records = messages.get("record_mesgs", []) or []

    t, hr, v, dist = [], [], [], []
    first_ts = None
    any_hr = False
    for r in records:
        ts = r.get("timestamp")
        if ts is None:
            continue
        if first_ts is None:
            first_ts = ts
        t.append((ts - first_ts).total_seconds())
        speed = r.get("enhanced_speed", r.get("speed"))
        v.append(speed if speed is not None else 0.0)
        dist.append(r.get("distance") if r.get("distance") is not None else 0.0)
        h = r.get("heart_rate")
        if h is not None:
            any_hr = True
            hr.append(h)
    if not any_hr:
        hr = []
    return {"t": t, "hr": hr, "v": v, "dist": dist}


def build_fit_output(*, activity_id: int, fetched_at: str, series: dict) -> dict:
    """Assemble the §2.6 worker `stream` stdout object. Key order fixed."""
    return {
        "activity_id": activity_id,
        "source": "garmin",
        "fetched_at": fetched_at,
        "series": series,
    }
