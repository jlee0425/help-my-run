#!/usr/bin/env python3
"""help-my-run Garmin worker entrypoint (CONTRACTS §2).

Usage:
  python worker.py login
  python worker.py fetch --since YYYY-MM-DD [--until YYYY-MM-DD] [--dry-run]
"""
import sys

from garmin_worker.cli import main

if __name__ == "__main__":
    sys.exit(main())
