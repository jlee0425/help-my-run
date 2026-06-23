import json

from garmin_worker import normalize

FULL_EL = {
    "activityId": 14820001234,
    "activityName": "Morning Run",
    "startTimeGMT": "2026-06-22 05:00:00",
    "startTimeLocal": "2026-06-22 07:00:00",
    "activityType": {"typeKey": "running"},
    "distance": 10000.0,
    "movingDuration": 3200.0,
    "elapsedDuration": 3300.0,
    "duration": 3300.0,
    "averageHR": 148.0,
    "maxHR": 168.0,
    "averageSpeed": 3.05,
    "maxSpeed": 4.2,
    "averageRunningCadenceInStepsPerMinute": 172.0,
    "elevationGain": 85.0,
    "extra": "ignored-but-kept-in-raw",
}

EXPECTED_KEYS = {
    "garmin_activity_id", "name", "start_time", "start_time_local",
    "activity_type", "distance_m", "moving_time_s", "elapsed_time_s",
    "avg_hr", "max_hr", "avg_speed", "max_speed", "avg_cadence",
    "elevation_gain_m", "raw_json",
}


def test_normalize_garmin_activity_maps_all_15_fields():
    out = normalize.normalize_garmin_activity(FULL_EL)
    assert set(out.keys()) == EXPECTED_KEYS
    assert out["garmin_activity_id"] == 14820001234
    assert out["name"] == "Morning Run"
    # RFC3339: space -> T, append Z.
    assert out["start_time"] == "2026-06-22T05:00:00Z"
    # local stays Garmin space format (no offset available).
    assert out["start_time_local"] == "2026-06-22 07:00:00"
    assert out["activity_type"] == "running"
    assert out["distance_m"] == 10000.0
    assert out["moving_time_s"] == 3200.0
    assert out["elapsed_time_s"] == 3300.0
    assert out["avg_hr"] == 148.0
    assert out["max_hr"] == 168.0
    assert out["avg_speed"] == 3.05
    assert out["max_speed"] == 4.2
    assert out["avg_cadence"] == 172.0
    assert out["elevation_gain_m"] == 85.0
    assert out["raw_json"] == FULL_EL  # ORIGINAL element preserved


def test_elapsed_falls_back_to_duration_when_elapsedDuration_missing():
    el = dict(FULL_EL)
    del el["elapsedDuration"]
    out = normalize.normalize_garmin_activity(el)
    assert out["elapsed_time_s"] == 3300.0  # from "duration"


def test_normalize_garmin_activity_trail_run_typekey():
    out = normalize.normalize_garmin_activity({
        "activityId": 99, "activityType": {"typeKey": "trail_running"},
    })
    assert out["activity_type"] == "trail_running"


def test_normalize_garmin_activity_all_enriched_fields_none():
    out = normalize.normalize_garmin_activity({"activityId": 7})
    assert set(out.keys()) == EXPECTED_KEYS
    assert out["garmin_activity_id"] == 7
    assert out["name"] is None
    assert out["start_time"] is None        # missing startTimeGMT -> None (no RFC3339 coercion)
    assert out["start_time_local"] is None
    assert out["activity_type"] is None
    assert out["distance_m"] is None
    assert out["moving_time_s"] is None
    assert out["elapsed_time_s"] is None
    assert out["avg_hr"] is None
    assert out["max_hr"] is None
    assert out["avg_speed"] is None
    assert out["max_speed"] is None
    assert out["avg_cadence"] is None
    assert out["elevation_gain_m"] is None
    assert out["raw_json"] == {"activityId": 7}


def test_normalize_garmin_activity_none_input():
    out = normalize.normalize_garmin_activity(None)
    assert out["garmin_activity_id"] is None
    assert out["start_time"] is None
    assert out["raw_json"] == {}


def test_normalize_garmin_activity_json_serializable():
    json.loads(json.dumps(normalize.normalize_garmin_activity(FULL_EL)))


def test_normalize_garmin_activity_from_fixture():
    import pathlib
    p = pathlib.Path(__file__).parent / "fixtures" / "activity_list_element.json"
    el = json.loads(p.read_text())
    out = normalize.normalize_garmin_activity(el)
    assert set(out.keys()) == EXPECTED_KEYS
    assert out["avg_hr"] is not None
    assert out["avg_cadence"] is not None
    assert out["elevation_gain_m"] is not None
    assert out["start_time"].endswith("Z")


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
