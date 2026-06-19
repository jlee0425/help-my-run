import io
import json
import os

import pytest

from garmin_worker import cli

FIXTURES = os.path.join(os.path.dirname(__file__), "fixtures")


def load(name):
    with open(os.path.join(FIXTURES, name), encoding="utf-8") as fh:
        return json.load(fh)


# --------------------------------------------------------------------------
# build_parser
# --------------------------------------------------------------------------
def test_parser_fetch_with_since():
    p = cli.build_parser()
    args = p.parse_args(["fetch", "--since", "2026-06-14"])
    assert args.command == "fetch"
    assert args.since == "2026-06-14"
    assert args.until is None
    assert args.dry_run is False


def test_parser_fetch_with_since_until_and_dry_run():
    p = cli.build_parser()
    args = p.parse_args(
        ["fetch", "--since", "2026-06-14", "--until", "2026-06-15", "--dry-run"]
    )
    assert args.command == "fetch"
    assert args.since == "2026-06-14"
    assert args.until == "2026-06-15"
    assert args.dry_run is True


def test_parser_login_command():
    p = cli.build_parser()
    args = p.parse_args(["login"])
    assert args.command == "login"


def test_parser_fetch_requires_since():
    p = cli.build_parser()
    with pytest.raises(SystemExit):
        p.parse_args(["fetch"])


def test_parser_no_command_errors():
    p = cli.build_parser()
    with pytest.raises(SystemExit):
        p.parse_args([])


# --------------------------------------------------------------------------
# validate_date
# --------------------------------------------------------------------------
def test_validate_date_ok():
    assert cli.validate_date("2026-06-14") == "2026-06-14"


@pytest.mark.parametrize("bad", ["2026-6-14", "06-14-2026", "2026/06/14", "nope", ""])
def test_validate_date_rejects_bad(bad):
    with pytest.raises(ValueError):
        cli.validate_date(bad)


# --------------------------------------------------------------------------
# --dry-run path: deterministic JSON to stdout, exit 0, nothing on stderr
# --------------------------------------------------------------------------
def test_main_dry_run_prints_contract_json(capsys):
    rc = cli.main(["fetch", "--since", "2026-06-14", "--until", "2026-06-15", "--dry-run"])
    assert rc == 0
    captured = capsys.readouterr()
    assert captured.err == ""
    out = json.loads(captured.out)  # must be exactly one parseable JSON object
    expected = load("dry_run_expected.json")
    assert out == expected


def test_main_dry_run_stdout_is_single_json_object(capsys):
    cli.main(["fetch", "--since", "2026-06-14", "--until", "2026-06-15", "--dry-run"])
    out = capsys.readouterr().out
    # exactly one top-level JSON value: json.loads on the whole buffer works,
    # and there is no trailing non-whitespace second document.
    decoder = json.JSONDecoder()
    obj, end = decoder.raw_decode(out.lstrip())
    assert out.lstrip()[end:].strip() == ""
    assert set(obj.keys()) == {
        "since", "until", "fetched_at", "sleep", "hrv", "body_battery", "rhr",
    }


def test_main_dry_run_bad_date_exits_nonzero_with_stderr(capsys):
    rc = cli.main(["fetch", "--since", "2026/06/14", "--dry-run"])
    assert rc != 0
    captured = capsys.readouterr()
    assert captured.out == ""
    assert "2026/06/14" in captured.err or "date" in captured.err.lower()
