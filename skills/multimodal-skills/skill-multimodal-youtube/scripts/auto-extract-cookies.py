#!/usr/bin/env python3
# SPDX-License-Identifier: MIT
"""Auto-extract YouTube cookies from the local browser and store them.

Uses browser_cookie3 to read cookies from Chrome/Safari/Firefox,
saves them to ~/.config/sin-youtube/cookies.json (chmod 600),
and optionally stores them in Infisical.

No user interaction needed — agents can run this autonomously.

Usage:
    python3 auto-extract-cookies.py [--store-infisical]

Exit codes:
    0  cookies extracted and saved
    1  no browser cookies found
    2  browser_cookie3 not installed
    3  Infisical store failed
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

COOKIE_PATH = Path.home() / ".config" / "sin-youtube" / "cookies.json"


def extract_from_chrome() -> list[dict] | None:
    try:
        import browser_cookie3

        cj = browser_cookie3.chrome(domain_name="youtube.com")
        return _to_json(cj)
    except Exception:
        return None


def extract_from_safari() -> list[dict] | None:
    try:
        import browser_cookie3

        cj = browser_cookie3.safari(domain_name="youtube.com")
        return _to_json(cj)
    except Exception:
        return None


def extract_from_firefox() -> list[dict] | None:
    try:
        import browser_cookie3

        cj = browser_cookie3.firefox(domain_name="youtube.com")
        return _to_json(cj)
    except Exception:
        return None


def _to_json(cookiejar) -> list[dict]:
    cookies = []
    for c in cookiejar:
        cookies.append(
            {
                "name": c.name,
                "value": c.value,
                "domain": c.domain,
                "path": c.path,
                "secure": c.secure,
                "expires": c.expires if c.expires else None,
            }
        )
    return cookies


def save_local(cookies: list[dict]) -> Path:
    COOKIE_PATH.parent.mkdir(parents=True, exist_ok=True)
    with open(COOKIE_PATH, "w") as f:
        json.dump(cookies, f, indent=2)
    os.chmod(COOKIE_PATH, 0o600)
    return COOKIE_PATH


def store_infisical(cookie_path: Path) -> bool:
    import subprocess

    script = Path.home() / ".config/opencode/skills/sin-youtube/scripts/cookie-store.sh"
    if not script.exists():
        print("[skip] cookie-store.sh not found", file=sys.stderr)
        return False
    result = subprocess.run(
        ["bash", str(script), str(cookie_path)],
        capture_output=True,
        text=True,
    )
    if result.returncode == 0:
        print("[ok] Stored in Infisical as YOUTUBE_COOKIES_JSON")
        return True
    print(f"[fail] Infisical store: {result.stderr}", file=sys.stderr)
    return False


def main() -> int:
    store_inf = "--store-infisical" in sys.argv

    # Try browsers in order
    for name, fn in [
        ("Chrome", extract_from_chrome),
        ("Safari", extract_from_safari),
        ("Firefox", extract_from_firefox),
    ]:
        cookies = fn()
        if cookies and len(cookies) > 0:
            path = save_local(cookies)
            print(f"[ok] Extracted {len(cookies)} YouTube cookies from {name}")
            print(f"     Saved to {path} (chmod 600)")
            if store_inf:
                store_infisical(path)
            # Print cookie names (not values)
            for c in cookies:
                print(f"     {c['name']} (domain={c['domain']})")
            return 0

    print("[fail] No YouTube cookies found in any browser", file=sys.stderr)
    print("       Open youtube.com in your browser and log in, then re-run.", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
