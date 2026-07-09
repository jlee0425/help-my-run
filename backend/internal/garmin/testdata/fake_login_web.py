#!/usr/bin/env python3
"""Fake worker for LoginManager tests. Speaks the M5 login-web protocol.

FAKE_MODE:
  ok       — creds line -> {"status":"ok",...}
  mfa      — creds line -> mfa_required; code "424242" -> ok, else auth_failed error
  autherr  — creds line -> {"status":"error","error":"auth_failed"}, exit 1
  ratelimit— creds line -> {"status":"error","error":"rate_limited"}, exit 1
  hang     — creds line -> block forever (timeout path)
  garbage  — creds line -> non-JSON stdout line
"""
import json
import os
import sys
import time

mode = os.environ.get("FAKE_MODE", "ok")

# argv: fake_login_web.py login-web   (the manager passes the subcommand)
if len(sys.argv) < 2 or sys.argv[1] != "login-web":
    print("unexpected argv", file=sys.stderr)
    sys.exit(9)

line = sys.stdin.readline()
try:
    creds = json.loads(line)
    assert creds["email"] and creds["password"]
except Exception:
    print(json.dumps({"status": "error", "error": "bad_input"}), flush=True)
    sys.exit(2)

def emit(obj):
    print(json.dumps(obj), flush=True)

if mode == "ok":
    emit({"status": "ok", "tokenstore": "/tmp/fake-tokens"})
    sys.exit(0)
elif mode == "mfa":
    emit({"status": "mfa_required"})
    reply = sys.stdin.readline()
    try:
        code = json.loads(reply).get("code", "")
    except Exception:
        code = ""
    if code == "424242":
        emit({"status": "ok", "tokenstore": "/tmp/fake-tokens"})
        sys.exit(0)
    emit({"status": "error", "error": "auth_failed"})
    sys.exit(1)
elif mode == "autherr":
    emit({"status": "error", "error": "auth_failed"})
    sys.exit(1)
elif mode == "ratelimit":
    emit({"status": "error", "error": "rate_limited"})
    sys.exit(1)
elif mode == "hang":
    time.sleep(60)
    sys.exit(1)
elif mode == "garbage":
    print("THIS IS NOT JSON", flush=True)
    sys.exit(0)
else:
    sys.exit(9)
