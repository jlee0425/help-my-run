import json
import os

import pytest

from garmin_worker import normalize

FIXTURES = os.path.join(os.path.dirname(__file__), "fixtures")


def load(name):
    with open(os.path.join(FIXTURES, name), encoding="utf-8") as fh:
        return json.load(fh)


# --------------------------------------------------------------------------
# normalize_sleep_day
# --------------------------------------------------------------------------
def test_normalize_sleep_day_full_shape():
    raw = load("raw_sleep_2026-06-15.json")
    out = normalize.normalize_sleep_day("2026-06-15", raw)
    assert out == {
        "date": "2026-06-15",
        "duration_s": 27000,
        "deep_s": 6300,
        "light_s": 14400,
        "rem_s": 5400,
        "awake_s": 900,
        "score": 82,
        "raw_json": raw,
    }
    assert list(out.keys()) == [
        "date", "duration_s", "deep_s", "light_s",
        "rem_s", "awake_s", "score", "raw_json",
    ]


def test_normalize_sleep_day_missing_fields_become_null():
    raw = {"dailySleepDTO": {"sleepTimeSeconds": 26400}}
    out = normalize.normalize_sleep_day("2026-06-15", raw)
    assert out["date"] == "2026-06-15"
    assert out["duration_s"] == 26400
    assert out["deep_s"] is None
    assert out["light_s"] is None
    assert out["rem_s"] is None
    assert out["awake_s"] is None
    assert out["score"] is None
    assert out["raw_json"] == raw


def test_normalize_sleep_day_no_dto_all_null():
    raw = {}
    out = normalize.normalize_sleep_day("2026-06-15", raw)
    assert out["duration_s"] is None
    assert out["score"] is None
    assert out["raw_json"] == {}


# --------------------------------------------------------------------------
# normalize_hrv_day  (get_hrv_data may return None -> caller omits;
# normalizer is only called for non-None payloads)
# --------------------------------------------------------------------------
def test_normalize_hrv_day_full_shape():
    raw = load("raw_hrv_2026-06-15.json")
    out = normalize.normalize_hrv_day("2026-06-15", raw)
    assert out == {
        "date": "2026-06-15",
        "last_night_avg_ms": 48,
        "status": "BALANCED",
        "raw_json": raw,
    }
    assert list(out.keys()) == ["date", "last_night_avg_ms", "status", "raw_json"]


def test_normalize_hrv_day_missing_summary_null():
    raw = {"hrvReadings": []}
    out = normalize.normalize_hrv_day("2026-06-15", raw)
    assert out["last_night_avg_ms"] is None
    assert out["status"] is None
    assert out["raw_json"] == raw


# --------------------------------------------------------------------------
# normalize_body_battery_day  (one entry of the range list)
# --------------------------------------------------------------------------
def test_normalize_body_battery_day_full_shape():
    entry = load("raw_body_battery_range.json")[1]  # 2026-06-15
    out = normalize.normalize_body_battery_day("2026-06-15", entry)
    assert out == {
        "date": "2026-06-15",
        "charged": 62,
        "drained": 78,
        "high": 91,
        "low": 14,
        "raw_json": entry,
    }
    assert list(out.keys()) == [
        "date", "charged", "drained", "high", "low", "raw_json",
    ]


def test_normalize_body_battery_high_low_from_values_array():
    entry = {
        "date": "2026-06-15",
        "charged": 30,
        "drained": 40,
        "bodyBatteryValuesArray": [
            [1, "ACTIVE", 55], [2, "ACTIVE", 12], [3, "ACTIVE", 80],
        ],
    }
    out = normalize.normalize_body_battery_day("2026-06-15", entry)
    assert out["high"] == 80
    assert out["low"] == 12


def test_normalize_body_battery_empty_array_high_low_null():
    entry = {"date": "2026-06-15", "charged": None, "drained": None,
             "bodyBatteryValuesArray": []}
    out = normalize.normalize_body_battery_day("2026-06-15", entry)
    assert out["high"] is None
    assert out["low"] is None
    assert out["charged"] is None
    assert out["drained"] is None


def test_normalize_body_battery_missing_charged_drained_fallback_from_deltas():
    # No "charged"/"drained" keys -> derive from value deltas.
    entry = {
        "date": "2026-06-15",
        "bodyBatteryValuesArray": [
            [1, "ACTIVE", 50], [2, "ACTIVE", 60], [3, "ACTIVE", 45],
            [4, "ACTIVE", 70],
        ],
    }
    out = normalize.normalize_body_battery_day("2026-06-15", entry)
    # positive deltas: +10, +25 = 35 ; negative deltas: -15 = -15 -> drained 15
    assert out["charged"] == 35
    assert out["drained"] == 15
    assert out["high"] == 70
    assert out["low"] == 45


# --------------------------------------------------------------------------
# normalize_rhr_day  (source: get_stats(date)["restingHeartRate"])
# --------------------------------------------------------------------------
def test_normalize_rhr_day_full_shape():
    raw = load("raw_stats_2026-06-15.json")
    out = normalize.normalize_rhr_day("2026-06-15", raw)
    assert out == {
        "date": "2026-06-15",
        "resting_hr": 47,
        "raw_json": raw,
    }
    assert list(out.keys()) == ["date", "resting_hr", "raw_json"]


def test_normalize_rhr_day_missing_rhr_null():
    raw = {"totalSteps": 9000}
    out = normalize.normalize_rhr_day("2026-06-15", raw)
    assert out["resting_hr"] is None
    assert out["raw_json"] == raw


def test_normalize_rhr_day_none_raw_yields_null():
    out = normalize.normalize_rhr_day("2026-06-15", None)
    assert out["resting_hr"] is None
    assert out["raw_json"] is None


# --------------------------------------------------------------------------
# normalize_vo2max_day
# get_max_metrics(date) hits the maxmet daily-range endpoint, which returns a
# one-element LIST: raw[0]["generic"]["vo2MaxValue"] (a plain dict is also
# tolerated for fixtures). raw_json preserves the ORIGINAL payload.
# --------------------------------------------------------------------------
def test_normalize_vo2max_day_dict_shape():
    # Dict-shaped fixture (defensive: type hint says dict).
    raw = load("raw_max_metrics_2026-06-15.json")
    out = normalize.normalize_vo2max_day("2026-06-15", raw)
    assert out == {
        "date": "2026-06-15",
        "vo2max": 52.0,
        "raw_json": raw,
    }
    assert list(out.keys()) == ["date", "vo2max", "raw_json"]


def test_normalize_vo2max_day_list_shape_real_endpoint():
    # Real endpoint shape: a one-element list; raw_json keeps the list intact.
    raw = [{"generic": {"calendarDate": "2026-06-15", "vo2MaxValue": 52.0}, "cycling": None}]
    out = normalize.normalize_vo2max_day("2026-06-15", raw)
    assert out["vo2max"] == 52.0  # same value as the dict-shaped case
    assert out["raw_json"] == raw  # original list payload preserved


def test_normalize_vo2max_day_empty_list_yields_null():
    out = normalize.normalize_vo2max_day("2026-06-15", [])
    assert out["vo2max"] is None
    assert out["raw_json"] == []  # original empty list preserved


def test_normalize_vo2max_day_missing_generic_null():
    raw = {"userId": 1, "generic": None, "cycling": None}
    out = normalize.normalize_vo2max_day("2026-06-15", raw)
    assert out["vo2max"] is None
    assert out["raw_json"] == raw


def test_normalize_vo2max_day_none_raw_yields_empty_dict():
    out = normalize.normalize_vo2max_day("2026-06-15", None)
    assert out["vo2max"] is None
    assert out["raw_json"] == {}


# --------------------------------------------------------------------------
# build_output  (assembles the full §2.1 top-level object)
# --------------------------------------------------------------------------
def test_build_output_top_level_shape():
    out = normalize.build_output(
        since="2026-06-14",
        until="2026-06-15",
        fetched_at="2026-06-15T05:00:12Z",
        sleep=[{"date": "2026-06-14"}],
        hrv=[],
        body_battery=[{"date": "2026-06-14"}, {"date": "2026-06-15"}],
        rhr=[{"date": "2026-06-15"}],
        vo2max=[{"date": "2026-06-15"}],
    )
    assert list(out.keys()) == [
        "since", "until", "fetched_at",
        "sleep", "hrv", "body_battery", "rhr", "vo2max",
    ]
    assert out["since"] == "2026-06-14"
    assert out["until"] == "2026-06-15"
    assert out["fetched_at"] == "2026-06-15T05:00:12Z"
    assert out["hrv"] == []
    assert len(out["body_battery"]) == 2
    assert out["sleep"][0]["date"] == "2026-06-14"
    assert out["rhr"][0]["date"] == "2026-06-15"
    assert out["vo2max"][0]["date"] == "2026-06-15"


def test_build_output_full_serializes_to_json():
    out = normalize.build_output(
        since="2026-06-15", until="2026-06-15",
        fetched_at="2026-06-15T05:00:12Z",
        sleep=[], hrv=[], body_battery=[], rhr=[], vo2max=[],
    )
    # must be JSON-serializable (no datetime / non-primitive leaks)
    text = json.dumps(out)
    again = json.loads(text)
    assert again["since"] == "2026-06-15"
