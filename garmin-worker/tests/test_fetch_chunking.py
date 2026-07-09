"""Range-native Garmin calls must be chunked: the body-battery endpoint rejects
windows over ~31 days ("API Error 400 - requested date range is too big"),
which broke the first sync's 84-day backfill (M5 review finding)."""
from __future__ import annotations

import datetime as _dt

import pytest

from garmin_worker.fetcher import _RANGE_CHUNK_DAYS, run_fetch


class RangeCappedClient:
    """Fake Garmin client that enforces the real endpoint's range cap."""

    def __init__(self, max_days=31):
        self.max_days = max_days
        self.bb_calls = []

    def get_body_battery(self, since, until=None):
        start = _dt.date.fromisoformat(since)
        end = _dt.date.fromisoformat(until)
        days = (end - start).days + 1
        if days > self.max_days:
            raise RuntimeError(
                "API call client error (400): API Error 400 - requested date range is too big."
            )
        self.bb_calls.append((since, until))
        return [
            {"date": d.isoformat(), "charged": 60, "drained": 70, "bodyBatteryValuesArray": []}
            for d in (start + _dt.timedelta(days=i) for i in range(days))
        ]

    # Per-day endpoints: minimal valid payloads.
    def get_sleep_data(self, cdate):
        return {"dailySleepDTO": {"sleepTimeSeconds": 25200}}

    def get_hrv_data(self, cdate):
        return None

    def get_stats(self, cdate):
        return {"restingHeartRate": 50}

    def get_max_metrics(self, cdate):
        return {}

    def get_activities_by_date(self, since, until, activitytype=None, sortorder=None):
        return []


def test_84_day_backfill_chunks_body_battery():
    client = RangeCappedClient()
    since = "2026-04-17"
    until = "2026-07-09"  # 84 days inclusive
    out = run_fetch(client, since=since, until=until, fetched_at="2026-07-09T05:00:00Z", sleep_fn=lambda s: None)

    # The capped endpoint never saw an oversized window (would have raised).
    assert len(client.bb_calls) >= 3
    for s, u in client.bb_calls:
        days = (_dt.date.fromisoformat(u) - _dt.date.fromisoformat(s)).days + 1
        assert days <= _RANGE_CHUNK_DAYS

    # Chunks are contiguous: no gaps, no overlaps, covering [since, until].
    assert client.bb_calls[0][0] == since
    assert client.bb_calls[-1][1] == until
    for (_, prev_end), (next_start, _) in zip(client.bb_calls, client.bb_calls[1:]):
        expected = (_dt.date.fromisoformat(prev_end) + _dt.timedelta(days=1)).isoformat()
        assert next_start == expected

    # Results concatenate to one entry per day, in order.
    bb = out["body_battery"]
    assert len(bb) == 84
    assert bb[0]["date"] == since
    assert bb[-1]["date"] == until
    dates = [d["date"] for d in bb]
    assert dates == sorted(dates)


def test_short_window_is_a_single_call():
    client = RangeCappedClient()
    out = run_fetch(client, since="2026-07-06", until="2026-07-09", fetched_at="x", sleep_fn=lambda s: None)
    assert client.bb_calls == [("2026-07-06", "2026-07-09")]
    assert len(out["body_battery"]) == 4


def test_single_day_window():
    client = RangeCappedClient()
    out = run_fetch(client, since="2026-07-09", until="2026-07-09", fetched_at="x", sleep_fn=lambda s: None)
    assert client.bb_calls == [("2026-07-09", "2026-07-09")]
    assert len(out["body_battery"]) == 1


def test_chunk_boundary_exact_multiple():
    """A window that is an exact multiple of the chunk size has no runt chunk."""
    client = RangeCappedClient()
    since = "2026-05-14"
    until = (_dt.date.fromisoformat(since) + _dt.timedelta(days=2 * _RANGE_CHUNK_DAYS - 1)).isoformat()
    run_fetch(client, since=since, until=until, fetched_at="x", sleep_fn=lambda s: None)
    assert len(client.bb_calls) == 2


@pytest.mark.parametrize("max_days", [28, 31])
def test_never_exceeds_garmin_cap(max_days):
    client = RangeCappedClient(max_days=max_days)
    # 180-day window: even a deep backfill must stay under the cap.
    run_fetch(client, since="2026-01-11", until="2026-07-09", fetched_at="x", sleep_fn=lambda s: None)
    assert all(
        (_dt.date.fromisoformat(u) - _dt.date.fromisoformat(s)).days + 1 <= max_days
        for s, u in client.bb_calls
    )
