import os
import sys
import types

import pytest

from garmin_worker import client


# --------------------------------------------------------------------------
# tokenstore_path: expands ~ and honors GARMIN_TOKENSTORE (CONTRACTS §4 env)
# --------------------------------------------------------------------------
def test_tokenstore_path_default(monkeypatch):
    monkeypatch.delenv("GARMIN_TOKENSTORE", raising=False)
    monkeypatch.setenv("HOME", "/home/tester")
    assert client.tokenstore_path() == "/home/tester/.garminconnect"


def test_tokenstore_path_from_env(monkeypatch):
    monkeypatch.setenv("GARMIN_TOKENSTORE", "~/customtokens")
    monkeypatch.setenv("HOME", "/home/tester")
    assert client.tokenstore_path() == "/home/tester/customtokens"


# --------------------------------------------------------------------------
# GarminClient wraps the verified method names and delegates 1:1.
# We inject a fake underlying garmin object (no garminconnect / no network).
# --------------------------------------------------------------------------
class _FakeGarmin:
    def __init__(self):
        self.calls = []

    def get_sleep_data(self, cdate):
        self.calls.append(("get_sleep_data", cdate))
        return {"dailySleepDTO": {"sleepTimeSeconds": 100}}

    def get_hrv_data(self, cdate):
        self.calls.append(("get_hrv_data", cdate))
        return None  # exercises the None path

    def get_body_battery(self, startdate, enddate=None):
        self.calls.append(("get_body_battery", startdate, enddate))
        return [{"date": startdate, "charged": 1, "drained": 2, "bodyBatteryValuesArray": []}]

    def get_stats(self, cdate):
        self.calls.append(("get_stats", cdate))
        return {"restingHeartRate": 47}

    def get_max_metrics(self, cdate):
        self.calls.append(("get_max_metrics", cdate))
        return {"userId": 1, "generic": {"calendarDate": cdate, "vo2MaxValue": 44.0}, "cycling": None}


def test_client_delegates_sleep():
    fake = _FakeGarmin()
    c = client.GarminClient(garmin=fake)
    assert c.get_sleep_data("2026-06-15") == {"dailySleepDTO": {"sleepTimeSeconds": 100}}
    assert ("get_sleep_data", "2026-06-15") in fake.calls


def test_client_delegates_hrv_none():
    fake = _FakeGarmin()
    c = client.GarminClient(garmin=fake)
    assert c.get_hrv_data("2026-06-15") is None
    assert ("get_hrv_data", "2026-06-15") in fake.calls


def test_client_delegates_body_battery_range():
    fake = _FakeGarmin()
    c = client.GarminClient(garmin=fake)
    out = c.get_body_battery("2026-06-14", "2026-06-15")
    assert isinstance(out, list)
    assert ("get_body_battery", "2026-06-14", "2026-06-15") in fake.calls


def test_client_delegates_stats():
    fake = _FakeGarmin()
    c = client.GarminClient(garmin=fake)
    assert c.get_stats("2026-06-15") == {"restingHeartRate": 47}
    assert ("get_stats", "2026-06-15") in fake.calls


def test_get_max_metrics_delegates_1to1():
    fake = _FakeGarmin()
    c = client.GarminClient(fake)
    out = c.get_max_metrics("2026-06-15")
    assert fake.calls == [("get_max_metrics", "2026-06-15")]
    assert out["generic"]["vo2MaxValue"] == 44.0


# --------------------------------------------------------------------------
# resume() builds a credential-less Garmin and logs in from the token dir.
# We monkeypatch the factory to avoid importing garminconnect / hitting net.
# --------------------------------------------------------------------------
def test_resume_uses_tokenstore(monkeypatch, tmp_path):
    created = {}

    class _ResumeGarmin:
        def __init__(self, *a, **k):
            created["init_args"] = (a, k)

        def login(self, tokenstore=None):
            created["login_tokenstore"] = tokenstore
            return ("oauth1", "oauth2")

    monkeypatch.setattr(client, "_new_garmin", lambda **kw: _ResumeGarmin(**kw))
    monkeypatch.setenv("GARMIN_TOKENSTORE", str(tmp_path))

    c = client.GarminClient.resume()
    assert isinstance(c, client.GarminClient)
    # resume() passes NO credentials (token-only) per Garmin research §2.
    assert created["init_args"] == ((), {})
    assert created["login_tokenstore"] == str(tmp_path)


# --------------------------------------------------------------------------
# login_interactive() passes creds + prompt_mfa and writes tokenstore.
# --------------------------------------------------------------------------
def test_login_interactive_passes_creds_and_mfa(monkeypatch, tmp_path):
    captured = {}

    class _LoginGarmin:
        def __init__(self, email=None, password=None, prompt_mfa=None, **k):
            captured["email"] = email
            captured["password"] = password
            captured["prompt_mfa"] = prompt_mfa

        def login(self, tokenstore=None):
            captured["login_tokenstore"] = tokenstore
            return ("oauth1", "oauth2")

    monkeypatch.setattr(client, "_new_garmin", lambda **kw: _LoginGarmin(**kw))
    monkeypatch.setenv("GARMIN_EMAIL", "you@example.com")
    monkeypatch.setenv("GARMIN_PASSWORD", "s3cret")
    monkeypatch.setenv("GARMIN_TOKENSTORE", str(tmp_path))

    prompt = lambda: "000000"
    c = client.GarminClient.login_interactive(prompt_mfa=prompt)
    assert isinstance(c, client.GarminClient)
    assert captured["email"] == "you@example.com"
    assert captured["password"] == "s3cret"
    assert captured["prompt_mfa"] is prompt
    assert captured["login_tokenstore"] == str(tmp_path)


def test_login_interactive_missing_creds_raises(monkeypatch, tmp_path):
    monkeypatch.delenv("GARMIN_EMAIL", raising=False)
    monkeypatch.delenv("GARMIN_PASSWORD", raising=False)
    monkeypatch.setenv("GARMIN_TOKENSTORE", str(tmp_path))
    with pytest.raises(ValueError):
        client.GarminClient.login_interactive(prompt_mfa=lambda: "0")
