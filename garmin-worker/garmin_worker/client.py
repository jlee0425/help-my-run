"""Live Garmin client wrapper (the ONLY module that imports garminconnect).

Isolates all network/SSO behind a thin class so the rest of the worker
(normalize.py, cli.py) stays pure and unit-testable. Wraps the VERIFIED
python-garminconnect method names:
    get_sleep_data(cdate)
    get_hrv_data(cdate)            -> dict | None
    get_body_battery(start, end)  -> list[dict]   (range-native)
    get_stats(cdate)              -> dict (flat; restingHeartRate top-level)

Login strategy (Garmin research §2): the widget+cffi multi-strategy login is
BUILT INTO garminconnect's resilient login(); there is NO login_strategy=
argument. We simply call login(tokenstore). curl_cffi (in requirements.txt)
provides the TLS impersonation that defeats the March-2026 Cloudflare block.

The live SSO/HTTP call is deliberately NOT unit-tested (it requires a real
Garmin account). Tests cover env/tokenstore plumbing and 1:1 delegation only,
by injecting a fake underlying object or monkeypatching `_new_garmin`.
"""
from __future__ import annotations

import os
from typing import Any, Callable, Optional


def tokenstore_path() -> str:
    """Resolve the OAuth token directory (CONTRACTS §4: GARMIN_TOKENSTORE).

    Default ~/.garminconnect; ~ is expanded.
    """
    raw = os.getenv("GARMIN_TOKENSTORE", "~/.garminconnect")
    return os.path.expanduser(raw)


def _new_garmin(**kwargs: Any):
    """Factory for the underlying garminconnect.Garmin.

    Imported lazily and isolated here so tests can monkeypatch this function
    without importing garminconnect or touching the network.
    """
    from garminconnect import Garmin  # noqa: WPS433 (intentional local import)

    return Garmin(**kwargs)


class GarminClient:
    """Thin wrapper delegating 1:1 to a logged-in garminconnect.Garmin."""

    def __init__(self, garmin: Any):
        self._g = garmin

    # ---- construction --------------------------------------------------
    @classmethod
    def resume(cls) -> "GarminClient":
        """Non-interactive: build a credential-less Garmin and resume tokens.

        Per Garmin research §2: no creds needed when resuming; login(tokenstore)
        loads tokens from the dir and auto-refreshes per request. If tokens are
        stale/revoked this raises GarminConnectAuthenticationError (handled by
        the fetch command in Task 16).
        """
        g = _new_garmin()
        g.login(tokenstore_path())
        return cls(g)

    @classmethod
    def login_interactive(cls, prompt_mfa: Callable[[], str]) -> "GarminClient":
        """One-time interactive SSO; persists OAuth tokens to the token dir.

        Reads GARMIN_EMAIL / GARMIN_PASSWORD from env (CONTRACTS §4). The
        widget+cffi strategy is automatic inside login(); MFA is handled via
        the prompt_mfa callback.
        """
        email = os.getenv("GARMIN_EMAIL")
        password = os.getenv("GARMIN_PASSWORD")
        if not email or not password:
            raise ValueError(
                "GARMIN_EMAIL and GARMIN_PASSWORD must be set for interactive login"
            )
        g = _new_garmin(email=email, password=password, prompt_mfa=prompt_mfa)
        g.login(tokenstore_path())
        return cls(g)

    # ---- verified data methods (1:1 delegation) ------------------------
    def get_sleep_data(self, cdate: str) -> dict:
        return self._g.get_sleep_data(cdate)

    def get_hrv_data(self, cdate: str) -> Optional[dict]:
        return self._g.get_hrv_data(cdate)

    def get_body_battery(self, startdate: str, enddate: Optional[str] = None) -> list:
        return self._g.get_body_battery(startdate, enddate)

    def get_stats(self, cdate: str) -> dict:
        return self._g.get_stats(cdate)
