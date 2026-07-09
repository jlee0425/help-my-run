"""login-web subcommand: stdin JSON creds, MFA handshake over stdout/stdin.

Protocol (M5 spec §7.2):
  stdin  line 1: {"email": "...", "password": "..."}
  stdout       : {"status": "mfa_required"}            (only when Garmin asks)
  stdin  line 2: {"code": "123456"}
  stdout final : {"status": "ok", "tokenstore": "..."} | {"status": "error", "error": "<kind>"}

Credentials must never appear on stdout or stderr.
"""
from __future__ import annotations

import io
import json

import pytest

from garmin_worker import cli, client


class _FakeStdin:
    """Line-oriented stdin stub."""

    def __init__(self, lines):
        self._lines = list(lines)

    def readline(self):
        if not self._lines:
            return ""
        return self._lines.pop(0)


def run_login_web(monkeypatch, capsys, stdin_lines, login_web_impl):
    monkeypatch.setattr(cli.sys, "stdin", _FakeStdin(stdin_lines))
    monkeypatch.setattr(client.GarminClient, "login_web", staticmethod(login_web_impl))
    monkeypatch.setattr(client, "tokenstore_path", lambda: "/tmp/tokens")
    rc = cli.main(["login-web"])
    out = capsys.readouterr()
    lines = [json.loads(l) for l in out.out.strip().splitlines() if l.strip()]
    return rc, lines, out.err


EMAIL = "runner@example.com"
PASSWORD = "s3cret-garmin-pw"
CREDS_LINE = json.dumps({"email": EMAIL, "password": PASSWORD}) + "\n"


def test_happy_path_no_mfa(monkeypatch, capsys):
    seen = {}

    def fake_login(email, password, prompt_mfa):
        seen["email"] = email
        seen["password"] = password

    rc, lines, err = run_login_web(monkeypatch, capsys, [CREDS_LINE], fake_login)
    assert rc == 0
    assert lines == [{"status": "ok", "tokenstore": "/tmp/tokens"}]
    assert seen == {"email": EMAIL, "password": PASSWORD}
    assert PASSWORD not in err and EMAIL not in err


def test_mfa_handshake_emits_marker_before_reading_code(monkeypatch, capsys):
    got = {}

    def fake_login(email, password, prompt_mfa):
        # garminconnect calls prompt_mfa mid-login; the callback must emit the
        # mfa_required line and then block on the next stdin line.
        got["code"] = prompt_mfa()

    rc, lines, err = run_login_web(
        monkeypatch,
        capsys,
        [CREDS_LINE, json.dumps({"code": "424242"}) + "\n"],
        fake_login,
    )
    assert rc == 0
    assert lines[0] == {"status": "mfa_required"}
    assert lines[1] == {"status": "ok", "tokenstore": "/tmp/tokens"}
    assert got["code"] == "424242"


def test_bad_first_line_is_bad_input(monkeypatch, capsys):
    rc, lines, _ = run_login_web(monkeypatch, capsys, ["not json at all\n"], lambda **kw: None)
    assert rc == 2
    assert lines == [{"status": "error", "error": "bad_input"}]


def test_missing_fields_is_bad_input(monkeypatch, capsys):
    rc, lines, _ = run_login_web(
        monkeypatch, capsys, [json.dumps({"email": EMAIL}) + "\n"], lambda **kw: None
    )
    assert rc == 2
    assert lines == [{"status": "error", "error": "bad_input"}]


def test_rate_limited_maps_to_kind(monkeypatch, capsys):
    def fake_login(email, password, prompt_mfa):
        raise RuntimeError("mobile+cffi returned 429 too many requests")

    rc, lines, err = run_login_web(monkeypatch, capsys, [CREDS_LINE], fake_login)
    assert rc == 1
    assert lines == [{"status": "error", "error": "rate_limited"}]
    assert PASSWORD not in err


def test_auth_failure_maps_to_kind_without_leaking(monkeypatch, capsys):
    def fake_login(email, password, prompt_mfa):
        raise RuntimeError(f"bad credentials for {email} / {password}")

    rc, lines, err = run_login_web(monkeypatch, capsys, [CREDS_LINE], fake_login)
    assert rc == 1
    assert lines == [{"status": "error", "error": "auth_failed"}]
    # The exception text contains the credentials; stderr must not echo them.
    assert PASSWORD not in err and EMAIL not in err


def test_stdout_carries_only_protocol_json(monkeypatch, capsys):
    def fake_login(email, password, prompt_mfa):
        print("library noise", file=cli.sys.stderr)

    rc, lines, _ = run_login_web(monkeypatch, capsys, [CREDS_LINE], fake_login)
    assert rc == 0
    for obj in lines:
        assert isinstance(obj, dict) and "status" in obj


def test_client_login_web_delegates(monkeypatch):
    """client.login_web logs in with explicit creds + persists to tokenstore."""
    calls = {}

    class FakeGarmin:
        def login(self, tokenstore):
            calls["tokenstore"] = tokenstore

    def fake_new_garmin(email=None, password=None, prompt_mfa=None):
        calls["email"] = email
        calls["password"] = password
        calls["prompt_mfa"] = prompt_mfa
        return FakeGarmin()

    monkeypatch.setattr(client, "_new_garmin", fake_new_garmin)
    monkeypatch.setattr(client, "tokenstore_path", lambda: "/tmp/ts")

    sentinel = lambda: "000000"  # noqa: E731
    client.GarminClient.login_web(email="e@x", password="pw", prompt_mfa=sentinel)
    assert calls["email"] == "e@x"
    assert calls["password"] == "pw"
    assert calls["prompt_mfa"] is sentinel
    assert calls["tokenstore"] == "/tmp/ts"


def test_stdin_binary_safety(monkeypatch, capsys):
    """A creds line with unicode content round-trips."""
    creds = json.dumps({"email": "ü@例.com", "password": "påss"}) + "\n"
    seen = {}

    def fake_login(email, password, prompt_mfa):
        seen["email"] = email

    rc, lines, _ = run_login_web(monkeypatch, capsys, [creds], fake_login)
    assert rc == 0 and seen["email"] == "ü@例.com"


@pytest.mark.parametrize("reply", ["", "not-json\n", json.dumps({"nope": 1}) + "\n"])
def test_bad_mfa_reply_yields_empty_code(monkeypatch, capsys, reply):
    got = {}

    def fake_login(email, password, prompt_mfa):
        got["code"] = prompt_mfa()

    lines_in = [CREDS_LINE] + ([reply] if reply else [])
    rc, lines, _ = run_login_web(monkeypatch, capsys, lines_in, fake_login)
    assert rc == 0
    assert got["code"] == ""
    assert lines[0] == {"status": "mfa_required"}


def test_stdout_flushed_before_blocking_on_mfa(monkeypatch, capsys):
    """The mfa_required line must be flushed so Go sees it before writing the code."""
    flushed = {"was": False}

    real_stdout = cli.sys.stdout

    class TrackingStdout(io.TextIOBase):
        def write(self, s):
            return real_stdout.write(s)

        def flush(self):
            flushed["was"] = True
            return real_stdout.flush()

    monkeypatch.setattr(cli.sys, "stdout", TrackingStdout())

    def fake_login(email, password, prompt_mfa):
        prompt_mfa()

    monkeypatch.setattr(cli.sys, "stdin", _FakeStdin([CREDS_LINE, '{"code":"1"}\n']))
    monkeypatch.setattr(client.GarminClient, "login_web", staticmethod(fake_login))
    monkeypatch.setattr(client, "tokenstore_path", lambda: "/tmp/tokens")
    rc = cli.main(["login-web"])
    assert rc == 0
    assert flushed["was"] is True
