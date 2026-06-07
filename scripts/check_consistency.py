# SPDX-License-Identifier: MIT
#!/usr/bin/env python3
"""Cross-repo consistency check for the SIN-Code Bundle (WS4 of operational-hardening).

The Bundle orchestrates 8 sibling subsystems that are installed via local
``pip install -e`` of adjacent repos. This script asserts that the Bundle's
own expectations stay internally consistent and reports drift against any
subsystems that happen to be installed.

Design goals:
- Exit 0 on a clean *bundle-only* checkout (subsystems absent -> warnings, not
  failures), so it is safe to wire into CI as a non-blocking job first.
- Promote ``--strict`` to make any missing subsystem or mismatch fail (exit 1),
  for use once the full multi-repo environment is provisioned.

Checks performed:
1. Bundle metadata: ``pyproject`` version == ``__init__.__version__``.
2. Subsystem import specs: each subsystem the ``status`` command probes either
   imports cleanly or is reported as not-installed.
3. MCP advertising: every client config emitted by ``sin mcp-config`` points at
   the same ``sin serve`` entry point that the package actually registers.
"""

from __future__ import annotations

import argparse
import importlib.metadata as md
import importlib.util
import sys
import tomllib
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

# Canonical subsystem map -- kept in sync with cli.status().
SUBSYSTEMS = {
    "sin_code_sckg": "SCKG (knowledge graph)",
    "sin_code_ibd": "IBD (intent diff)",
    "sin_code_poc": "POC (proof of correctness)",
    "sin_code_efsm": "EFSM (mock orchestration)",
    "sin_code_adw": "ADW (debt watchdog)",
    "sin_code_oracle": "Oracle (verification)",
    "sin_code_orchestration": "Orchestration (multi-agent workflow)",
    "sin_code_review_interface": "Review-Interface (semantic review UI)",
}

GREEN, YELLOW, RED, RESET = "\033[32m", "\033[33m", "\033[31m", "\033[0m"


def _ok(msg: str) -> None:
    print(f"{GREEN}OK{RESET}    {msg}")


def _warn(msg: str) -> None:
    print(f"{YELLOW}WARN{RESET}  {msg}")


def _fail(msg: str) -> None:
    print(f"{RED}FAIL{RESET}  {msg}")


def check_version() -> list[str]:
    errors: list[str] = []
    pyproject = tomllib.loads((REPO_ROOT / "pyproject.toml").read_text())
    declared = pyproject["project"]["version"]
    init_text = (REPO_ROOT / "src" / "sin_code_bundle" / "__init__.py").read_text()
    runtime = next(
        (
            line.split("=", 1)[1].strip().strip('"').strip("'")
            for line in init_text.splitlines()
            if line.startswith("__version__")
        ),
        None,
    )
    if runtime == declared:
        _ok(f"version aligned: pyproject == __init__ == {declared}")
    else:
        _fail(f"version drift: pyproject={declared!r} but __init__={runtime!r}")
        errors.append("version drift")
    return errors


def check_subsystems(strict: bool) -> list[str]:
    errors: list[str] = []
    for module, desc in SUBSYSTEMS.items():
        installed = importlib.util.find_spec(module) is not None
        if installed:
            try:
                version = md.version(module.replace("_", "-"))
            except md.PackageNotFoundError:
                version = "unknown"
            _ok(f"{desc}: importable (v{version})")
        elif strict:
            _fail(f"{desc}: module '{module}' not installed (strict)")
            errors.append(f"{module} missing")
        else:
            _warn(f"{desc}: module '{module}' not installed (expected in bundle-only checkout)")
    return errors


def check_mcp_advertising() -> list[str]:
    errors: list[str] = []
    from sin_code_bundle import mcp_config

    expected_cmd, expected_args = mcp_config.COMMAND, mcp_config.ARGS
    if (expected_cmd, expected_args) != ("sin", ["serve"]):
        _fail(f"mcp entry point unexpected: {expected_cmd} {expected_args}")
        errors.append("mcp entry point")
        return errors

    # The package must actually expose the `sin` console script the configs point at.
    scripts = {ep.name: ep.value for ep in md.entry_points(group="console_scripts")}
    if scripts.get("sin", "").startswith("sin_code_bundle.cli"):
        _ok("'sin' console script resolves to sin_code_bundle.cli")
    else:
        _fail(f"'sin' console script missing or wrong: {scripts.get('sin')!r}")
        errors.append("console script")

    for client in mcp_config.SUPPORTED_CLIENTS:
        rendered = mcp_config.generate(client)
        if expected_cmd in rendered and "serve" in rendered:
            _ok(f"mcp-config[{client}] advertises '{expected_cmd} serve'")
        else:
            _fail(f"mcp-config[{client}] does not advertise the serve entry point")
            errors.append(f"mcp-config {client}")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--strict",
        action="store_true",
        help="Treat missing subsystems as failures (full multi-repo env).",
    )
    args = parser.parse_args()

    print("== SIN-Code Bundle consistency check ==")
    errors: list[str] = []
    errors += check_version()
    errors += check_subsystems(args.strict)
    errors += check_mcp_advertising()

    print()
    if errors:
        _fail(f"{len(errors)} consistency problem(s): {', '.join(errors)}")
        return 1
    _ok("all consistency checks passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
