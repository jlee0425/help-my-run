import datetime as _dt
import io
import os
import zipfile

import pytest

from garmin_worker import normalize

_SAMPLE_FIT = os.path.join(os.path.dirname(__file__), "fixtures", "sample.fit")


def _zip_of(fit_bytes: bytes) -> bytes:
    buf = io.BytesIO()
    with zipfile.ZipFile(buf, "w") as z:
        z.writestr("activity.fit", fit_bytes)
    return buf.getvalue()


def _records_with_hr():
    base = _dt.datetime(2026, 6, 22, 5, 0, 0)
    return [
        {"timestamp": base, "heart_rate": 104, "enhanced_speed": 0.0, "distance": 0.0},
        {"timestamp": base + _dt.timedelta(seconds=1), "heart_rate": 105, "enhanced_speed": 1.59, "distance": 2.9},
        {"timestamp": base + _dt.timedelta(seconds=2), "heart_rate": 106, "enhanced_speed": 1.66, "distance": 5.6},
    ]


def _records_no_hr():
    base = _dt.datetime(2026, 6, 22, 5, 0, 0)
    return [
        {"timestamp": base, "enhanced_speed": 0.0, "distance": 0.0},
        {"timestamp": base + _dt.timedelta(seconds=1), "speed": 1.59, "distance": 2.9},
    ]


def test_normalize_fit_stream_with_hr(monkeypatch):
    monkeypatch.setattr(normalize, "_fit_decode", lambda b: ({"record_mesgs": _records_with_hr()}, []))
    series = normalize.normalize_fit_stream(_zip_of(b"FITBYTES"))
    assert series == {
        "t": [0.0, 1.0, 2.0],
        "hr": [104, 105, 106],
        "v": [0.0, 1.59, 1.66],
        "dist": [0.0, 2.9, 5.6],
    }


def test_normalize_fit_stream_no_hr_degrades(monkeypatch):
    monkeypatch.setattr(normalize, "_fit_decode", lambda b: ({"record_mesgs": _records_no_hr()}, []))
    series = normalize.normalize_fit_stream(_zip_of(b"FITBYTES"))
    # enhanced_speed preferred, fallback to speed; HR omitted entirely -> [].
    assert series["hr"] == []
    assert series["t"] == [0.0, 1.0]
    assert series["v"] == [0.0, 1.59]
    assert series["dist"] == [0.0, 2.9]


def test_normalize_fit_stream_unzips_fit(monkeypatch):
    captured = {}
    monkeypatch.setattr(normalize, "_fit_decode", lambda b: (captured.update(fit=b) or {"record_mesgs": _records_with_hr()}, []))
    normalize.normalize_fit_stream(_zip_of(b"INNERFIT"))
    assert captured["fit"] == b"INNERFIT"


def test_normalize_fit_stream_real_fixture():
    pytest.importorskip("garmin_fit_sdk")  # skip if the FIT SDK isn't installed
    if not os.path.exists(_SAMPLE_FIT):
        pytest.skip("no real sample.fit fixture recorded")
    with open(_SAMPLE_FIT, "rb") as fh:
        series = normalize.normalize_fit_stream(fh.read())
    assert len(series["t"]) > 0
