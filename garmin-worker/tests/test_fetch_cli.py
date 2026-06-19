import json

import pytest

from garmin_worker import cli, client


class _AuthError(Exception):
    """Stand-in for GarminConnectAuthenticationError in tests."""


def test_fetch_live_success_prints_json(monkeypatch, capsys):
    # GarminClient.resume() returns a sentinel; run_fetch returns a fixed dict.
    sentinel = object()
    monkeypatch.setattr(client.GarminClient, "resume", classmethod(lambda cls: sentinel))

    def fake_run_fetch(c, *, since, until, fetched_at, **kw):
        assert c is sentinel
        return {
            "since": since, "until": until, "fetched_at": fetched_at,
            "sleep": [], "hrv": [], "body_battery": [], "rhr": [],
        }

    monkeypatch.setattr(cli, "run_fetch", fake_run_fetch)

    rc = cli.main(["fetch", "--since", "2026-06-14", "--until", "2026-06-15"])
    assert rc == 0
    captured = capsys.readouterr()
    assert captured.err == ""
    out = json.loads(captured.out)
    assert out["since"] == "2026-06-14"
    assert out["until"] == "2026-06-15"
    assert set(out.keys()) == {
        "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr",
    }


def test_fetch_live_auth_error_exits_nonzero_with_relogin_hint(monkeypatch, capsys):
    # Point the worker's auth-error type at our stand-in, and make resume() raise it.
    monkeypatch.setattr(cli, "GarminConnectAuthenticationError", _AuthError, raising=False)

    def boom(cls):
        raise _AuthError("token expired")

    monkeypatch.setattr(client.GarminClient, "resume", classmethod(boom))

    rc = cli.main(["fetch", "--since", "2026-06-14"])
    assert rc != 0
    captured = capsys.readouterr()
    assert captured.out == ""  # nothing parseable on stdout
    assert "re-run worker.py login" in captured.err  # CONTRACTS §2.4 literal substring


def test_fetch_live_generic_error_exits_nonzero_with_message(monkeypatch, capsys):
    def boom(cls):
        raise RuntimeError("connection reset")

    monkeypatch.setattr(client.GarminClient, "resume", classmethod(boom))

    rc = cli.main(["fetch", "--since", "2026-06-14"])
    assert rc != 0
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "connection reset" in captured.err


def test_fetch_live_run_fetch_error_exits_nonzero(monkeypatch, capsys):
    sentinel = object()
    monkeypatch.setattr(client.GarminClient, "resume", classmethod(lambda cls: sentinel))

    def boom(c, **kw):
        raise RuntimeError("rate limited")

    monkeypatch.setattr(cli, "run_fetch", boom)

    rc = cli.main(["fetch", "--since", "2026-06-14"])
    assert rc != 0
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "rate limited" in captured.err
