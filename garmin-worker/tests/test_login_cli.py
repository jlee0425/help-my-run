"""worker.py login error-path tests.

Covers exit codes and stderr messaging for the login subcommand without ever
contacting Garmin (login_interactive is monkeypatched). Mirrors the cli test
pattern used by test_fetch_cli.py (monkeypatch client.GarminClient).
"""
import pytest

from garmin_worker import cli, client


# --------------------------------------------------------------------------
# 429 / rate-limit detection -> distinct exit code 3 + actionable cooldown text
# --------------------------------------------------------------------------
@pytest.mark.parametrize(
    "msg",
    [
        "Too many requests: mobile+cffi/mobile+requests returned 429",
        "HTTP Error 429: rate limited",
        "You have been Rate Limited by the server",
        "garmin says: RATE LIMIT exceeded",
    ],
)
def test_login_rate_limited_returns_3_with_cooldown(monkeypatch, capsys, msg):
    def boom(prompt_mfa=None):
        raise RuntimeError(msg)

    monkeypatch.setattr(client.GarminClient, "login_interactive", staticmethod(boom))

    rc = cli.main(["login"])
    assert rc == 3
    captured = capsys.readouterr()
    assert captured.out == ""  # diagnostics on stderr only
    err = captured.err
    assert "429" in err
    assert "Wait" in err  # cooldown guidance
    assert "Do NOT retry in a loop" in err  # explicit anti-hammer instruction


def test_login_generic_error_returns_1(monkeypatch, capsys):
    def boom(prompt_mfa=None):
        raise RuntimeError("connection reset by peer")

    monkeypatch.setattr(client.GarminClient, "login_interactive", staticmethod(boom))

    rc = cli.main(["login"])
    assert rc == 1
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "login failed: connection reset by peer" in captured.err
    # A non-429 error must NOT trigger the cooldown message.
    assert "Do NOT retry in a loop" not in captured.err


def test_login_value_error_returns_2(monkeypatch, capsys):
    def boom(prompt_mfa=None):
        raise ValueError("GARMIN_EMAIL and GARMIN_PASSWORD must be set")

    monkeypatch.setattr(client.GarminClient, "login_interactive", staticmethod(boom))

    rc = cli.main(["login"])
    assert rc == 2
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "GARMIN_EMAIL" in captured.err


def test_login_success_returns_0(monkeypatch, capsys):
    monkeypatch.setattr(
        client.GarminClient, "login_interactive", staticmethod(lambda prompt_mfa=None: None)
    )
    monkeypatch.setattr(client, "tokenstore_path", lambda: "/home/tester/.garminconnect")

    rc = cli.main(["login"])
    assert rc == 0
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "login ok" in captured.err
