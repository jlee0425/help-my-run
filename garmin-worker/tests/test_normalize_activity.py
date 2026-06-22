from garmin_worker import normalize


def test_normalize_garmin_activity_maps_all_fields():
    el = {
        "activityId": 14820001234,
        "startTimeGMT": "2026-06-22 05:00:00",
        "duration": 3300.0,
        "distance": 10000.0,
        "activityType": {"typeKey": "running"},
        "extra": "ignored-but-kept-in-raw",
    }
    out = normalize.normalize_garmin_activity(el)
    assert set(out.keys()) == {
        "garmin_activity_id", "start_time", "duration_s",
        "distance_m", "activity_type", "raw_json",
    }
    assert out["garmin_activity_id"] == 14820001234
    assert out["start_time"] == "2026-06-22 05:00:00"
    assert out["duration_s"] == 3300.0
    assert out["distance_m"] == 10000.0
    assert out["activity_type"] == "running"  # nested activityType.typeKey
    assert out["raw_json"] == el  # ORIGINAL element preserved


def test_normalize_garmin_activity_trail_run_typekey():
    el = {
        "activityId": 99,
        "startTimeGMT": "2026-06-21 06:00:00",
        "duration": 2700.0,
        "distance": 8000.0,
        "activityType": {"typeKey": "trail_running"},
    }
    out = normalize.normalize_garmin_activity(el)
    assert out["activity_type"] == "trail_running"


def test_normalize_garmin_activity_missing_fields_are_none():
    out = normalize.normalize_garmin_activity({"activityId": 7})
    assert out["garmin_activity_id"] == 7
    assert out["start_time"] is None
    assert out["duration_s"] is None
    assert out["distance_m"] is None
    assert out["activity_type"] is None  # no activityType -> safe-walk None
    assert out["raw_json"] == {"activityId": 7}


def test_normalize_garmin_activity_none_input():
    out = normalize.normalize_garmin_activity(None)
    assert out["garmin_activity_id"] is None
    assert out["activity_type"] is None
    assert out["raw_json"] == {}


def test_normalize_garmin_activity_json_serializable():
    import json
    out = normalize.normalize_garmin_activity({
        "activityId": 1, "startTimeGMT": "2026-06-22 05:00:00",
        "duration": 100.0, "distance": 200.0,
        "activityType": {"typeKey": "running"},
    })
    json.loads(json.dumps(out))  # must not raise


def test_build_output_emits_activities_last():
    out = normalize.build_output(
        since="2026-06-14", until="2026-06-15", fetched_at="t",
        sleep=[], hrv=[], body_battery=[], rhr=[], vo2max=[],
        activities=[{"garmin_activity_id": 1}],
    )
    assert list(out.keys()) == [
        "since", "until", "fetched_at",
        "sleep", "hrv", "body_battery", "rhr", "vo2max", "activities",
    ]
    assert out["activities"] == [{"garmin_activity_id": 1}]
