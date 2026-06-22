import pytest

from garmin_worker import fetcher


class _MockClient:
    """Mock GarminClient: deterministic per-date data, records calls."""

    def __init__(self, hrv_map=None, vo2max_map=None, raise_on=None, activities=None):
        self.calls = []
        self._hrv_map = hrv_map or {}
        self._vo2max_map = vo2max_map or {}
        self._raise_on = raise_on  # (method_name, exception) to raise
        self._activities = activities  # None -> default one running element

    def _maybe_raise(self, method):
        if self._raise_on and self._raise_on[0] == method:
            raise self._raise_on[1]

    def get_sleep_data(self, cdate):
        self.calls.append(("sleep", cdate))
        self._maybe_raise("get_sleep_data")
        return {"dailySleepDTO": {"sleepTimeSeconds": 100, "sleepScores": {"overall": {"value": 70}}}}

    def get_hrv_data(self, cdate):
        self.calls.append(("hrv", cdate))
        self._maybe_raise("get_hrv_data")
        return self._hrv_map.get(cdate)  # None unless provided

    def get_body_battery(self, startdate, enddate=None):
        self.calls.append(("bb", startdate, enddate))
        self._maybe_raise("get_body_battery")
        return [
            {"date": startdate, "charged": 10, "drained": 20, "bodyBatteryValuesArray": [[1, "ACTIVE", 80], [2, "ACTIVE", 5]]},
            {"date": enddate, "charged": 11, "drained": 22, "bodyBatteryValuesArray": [[3, "ACTIVE", 90], [4, "ACTIVE", 7]]},
        ]

    def get_stats(self, cdate):
        self.calls.append(("stats", cdate))
        self._maybe_raise("get_stats")
        return {"restingHeartRate": 50}

    def get_max_metrics(self, cdate):
        self.calls.append(("vo2max", cdate))
        self._maybe_raise("get_max_metrics")
        # Default: every day has a value unless vo2max_map overrides.
        # Real endpoint returns a one-element list, not a top-level dict.
        if cdate in self._vo2max_map:
            return self._vo2max_map[cdate]
        return [{"generic": {"calendarDate": cdate, "vo2MaxValue": 50.0}, "cycling": None}]

    def get_activities_by_date(self, startdate, enddate=None, activitytype=None):
        self.calls.append(("activities", startdate, enddate, activitytype))
        self._maybe_raise("get_activities_by_date")
        if self._activities is not None:
            return self._activities
        return [
            {
                "activityId": 14820001234,
                "startTimeGMT": "2026-06-22 05:00:00",
                "duration": 3300.0,
                "distance": 10000.0,
                "activityType": {"typeKey": "running"},
            }
        ]


def _noop_sleep(_):
    return None


def test_run_fetch_top_level_shape_and_echo():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="2026-06-15T05:00:12Z", sleep_fn=_noop_sleep,
    )
    assert list(out.keys()) == [
        "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr", "vo2max", "activities",
    ]
    assert out["since"] == "2026-06-14"
    assert out["until"] == "2026-06-15"
    assert out["fetched_at"] == "2026-06-15T05:00:12Z"


def test_run_fetch_iterates_each_day_for_per_day_sources():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    # 2 days -> 2 sleep, 2 rhr entries
    assert [s["date"] for s in out["sleep"]] == ["2026-06-14", "2026-06-15"]
    assert [r["date"] for r in out["rhr"]] == ["2026-06-14", "2026-06-15"]
    assert out["sleep"][0]["duration_s"] == 100
    assert out["sleep"][0]["score"] == 70
    assert out["rhr"][0]["resting_hr"] == 50


def test_run_fetch_body_battery_single_range_call():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    bb_calls = [c for c in mc.calls if c[0] == "bb"]
    assert bb_calls == [("bb", "2026-06-14", "2026-06-15")]  # exactly one range call
    assert [b["date"] for b in out["body_battery"]] == ["2026-06-14", "2026-06-15"]
    assert out["body_battery"][0]["high"] == 80
    assert out["body_battery"][0]["low"] == 5


def test_run_fetch_omits_hrv_none_days():
    # HRV present only for 2026-06-15
    mc = _MockClient(hrv_map={
        "2026-06-15": {"hrvSummary": {"lastNightAvg": 48, "status": "BALANCED"}, "hrvReadings": []},
    })
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert [h["date"] for h in out["hrv"]] == ["2026-06-15"]  # 06-14 omitted (None)
    assert out["hrv"][0]["last_night_avg_ms"] == 48
    assert out["hrv"][0]["status"] == "BALANCED"


def test_run_fetch_single_day_range():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-15", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert len(out["sleep"]) == 1
    assert out["sleep"][0]["date"] == "2026-06-15"


def test_run_fetch_body_battery_failure_propagates():
    err = RuntimeError("bb boom")
    mc = _MockClient(raise_on=("get_body_battery", err))
    with pytest.raises(RuntimeError, match="bb boom"):
        fetcher.run_fetch(
            mc, since="2026-06-14", until="2026-06-15",
            fetched_at="t", sleep_fn=_noop_sleep,
        )


def test_run_fetch_output_is_json_serializable():
    import json
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    json.loads(json.dumps(out))  # must not raise


def test_run_fetch_appends_vo2max_per_day():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert [v["date"] for v in out["vo2max"]] == ["2026-06-14", "2026-06-15"]
    assert out["vo2max"][0]["vo2max"] == 50.0


def test_run_fetch_omits_vo2max_null_days():
    # 06-14 returns an empty list (no data) -> omitted; 06-15 has a value.
    # Both use the real list-shaped endpoint payload.
    mc = _MockClient(vo2max_map={
        "2026-06-14": [],
        "2026-06-15": [{"generic": {"calendarDate": "2026-06-15", "vo2MaxValue": 52.0}, "cycling": None}],
    })
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert [v["date"] for v in out["vo2max"]] == ["2026-06-15"]  # 06-14 omitted (no value)
    assert out["vo2max"][0]["vo2max"] == 52.0


def test_run_fetch_top_level_shape_includes_activities():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="2026-06-15T05:00:12Z", sleep_fn=_noop_sleep,
    )
    assert list(out.keys()) == [
        "since", "until", "fetched_at", "sleep", "hrv", "body_battery",
        "rhr", "vo2max", "activities",
    ]


def test_run_fetch_normalizes_activities():
    mc = _MockClient()
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert len(out["activities"]) == 1
    a = out["activities"][0]
    assert a["garmin_activity_id"] == 14820001234
    assert a["start_time"] == "2026-06-22 05:00:00"
    assert a["duration_s"] == 3300.0
    assert a["distance_m"] == 10000.0
    assert a["activity_type"] == "running"
    assert "raw_json" in a


def test_run_fetch_activities_uses_running_filter_over_window():
    mc = _MockClient()
    fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    act_calls = [c for c in mc.calls if c[0] == "activities"]
    # one call, whole window, run-type filtered server-side.
    assert act_calls == [("activities", "2026-06-14", "2026-06-15", "running")]


def test_run_fetch_skips_activities_without_id():
    mc = _MockClient(activities=[
        {"activityId": 1, "startTimeGMT": "2026-06-22 05:00:00",
         "duration": 100.0, "distance": 200.0, "activityType": {"typeKey": "running"}},
        {"startTimeGMT": "2026-06-22 06:00:00"},  # no activityId -> skipped
        "not-a-dict",                              # non-dict -> skipped
    ])
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert [a["garmin_activity_id"] for a in out["activities"]] == [1]


def test_run_fetch_activities_failure_degrades_to_empty():
    # A list-fetch failure must NOT fail the whole recovery sync (spec §10).
    mc = _MockClient(raise_on=("get_activities_by_date", RuntimeError("acts boom")))
    out = fetcher.run_fetch(
        mc, since="2026-06-14", until="2026-06-15",
        fetched_at="t", sleep_fn=_noop_sleep,
    )
    assert out["activities"] == []
    # Other sources still populated (degrade is isolated to activities).
    assert len(out["sleep"]) == 2
