#!/usr/bin/env python3
"""help-my-run Garmin worker (scaffold stub).

Real subcommands (`login`, `fetch --since YYYY-MM-DD`) are implemented by the
WORKER tasks. This stub only establishes the argparse surface so scaffolding
verification (`worker.py --help`) succeeds.
"""
import argparse
import sys


def main() -> int:
    parser = argparse.ArgumentParser(prog="worker.py", description="help-my-run Garmin worker")
    sub = parser.add_subparsers(dest="command", required=True)
    sub.add_parser("login", help="interactive one-time Garmin login (MFA-aware)")
    fetch = sub.add_parser("fetch", help="fetch recovery data and print JSON to stdout")
    fetch.add_argument("--since", required=True, metavar="YYYY-MM-DD", help="inclusive start date")
    args = parser.parse_args()
    print(f"scaffold stub: command={args.command}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
