# Purpose: One-click PyPI Trusted Publisher registration via API token.
# Docs: pypi_setup.doc.md
"""One-click PyPI Trusted Publisher setup via API token.

Replaces the manual flow at https://pypi.org/manage/account/publishing/
with a single CLI invocation. After setup, every `git tag v*` + `git push`
auto-publishes to PyPI via GitHub Actions OIDC (no API token needed in CI).

Docs: pypi_setup.doc.md
"""

from __future__ import annotations

import argparse
import json
import re
import sys
import urllib.error
import urllib.request
from typing import Any, Dict, Optional, Tuple

PYPI_API = "https://pypi.org"
PUBLISHER_ENDPOINT = f"{PYPI_API}/_/v1/publisher"
FALLBACK_URL = "https://pypi.org/manage/account/publishing/"


def normalise_project_name(name: str) -> str:
    """Normalise a project name per PEP 503.

    PyPI normalises project names to lowercase, replacing runs of
    ``.``, ``_``, ``-`` with a single ``-``. The Trusted Publisher
    registration endpoint requires the normalised form.
    """
    return re.sub(r"[-_.]+", "-", name).lower()


def get_pending_publisher_payload(
    project: str,
    owner: str,
    repo: str,
    workflow: str,
    environment: str,
) -> Dict[str, Any]:
    """Build the JSON payload for a new pending Trusted Publisher.

    Args:
        project: PyPI project name (normalised to PEP 503 form).
        owner: GitHub owner (org or user). May be ``org`` or ``user/repo``.
        repo: GitHub repository name.
        workflow: Workflow filename (e.g. ``release.yml``).
        environment: GitHub Actions environment name (e.g. ``pypi``).

    Returns:
        Dict matching the PyPI ``_/v1/publisher`` schema.
    """
    # The schema uses `repository_owner` for the GitHub login (org or user).
    # If `owner` happens to contain a `/`, take the first segment — that's
    # the GitHub login the rest of the payload already uses.
    repo_owner = owner.split("/", 1)[0] if "/" in owner else owner
    return {
        "name": normalise_project_name(project),
        "owner": repo_owner,
        "repository": repo,
        "workflow_filename": workflow,
        "environment": environment,
    }


def add_pending_publisher(
    api_token: str,
    payload: Dict[str, Any],
    *,
    timeout: float = 15.0,
    base_url: str = PYPI_API,
) -> Tuple[bool, str]:
    """POST a pending-publisher registration to PyPI.

    Args:
        api_token: PyPI API token (format: ``pypi-...``). NOT a password.
        payload: The dict produced by :func:`get_pending_publisher_payload`.
        timeout: HTTP timeout in seconds.
        base_url: Override the PyPI base URL (for test fixtures).

    Returns:
        Tuple ``(success, message)``. ``success`` is ``True`` only when
        PyPI returns HTTP 201. The ``message`` is a human-readable
        description suitable for printing to a terminal.
    """
    url = f"{base_url}/_/v1/publisher"
    body = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=body,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Token {api_token}",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            status = resp.status
            text = resp.read().decode("utf-8", errors="replace")
            if status == 201:
                return True, (
                    "Pending publisher created. PyPI emailed the maintainer "
                    "at the account email. Click the magic link to confirm."
                )
            return False, f"PyPI returned HTTP {status}: {text}"
    except urllib.error.HTTPError as e:
        # The error body often carries machine-readable details. We surface
        # both the status and the body so the maintainer can diagnose
        # without re-running the request.
        err_body = e.read().decode("utf-8", errors="replace")
        return False, f"HTTP {e.code}: {err_body}"
    except urllib.error.URLError as e:
        return False, f"Network error: {e.reason}"
    except TimeoutError:
        return False, f"Request timed out after {timeout}s."
    except Exception as e:  # pragma: no cover — defensive
        return False, f"Unexpected error: {e}"


def build_argparser() -> argparse.ArgumentParser:
    """Construct the CLI argument parser.

    Kept as a separate function so tests can introspect / invoke it
    without spawning the full ``main()`` flow.
    """
    parser = argparse.ArgumentParser(
        prog="python -m sin_code_bundle.tools.pypi_setup",
        description=(
            "One-click PyPI Trusted Publisher setup for "
            "OpenSIN-Code/SIN-Code-Bundle (tokenless OIDC publishing)."
        ),
    )
    parser.add_argument(
        "--project",
        default="sin-code-bundle",
        help="PyPI project name (default: %(default)s)",
    )
    parser.add_argument(
        "--owner",
        default="OpenSIN-Code",
        help="GitHub owner/org (default: %(default)s)",
    )
    parser.add_argument(
        "--repo",
        default="SIN-Code-Bundle",
        help="GitHub repository name (default: %(default)s)",
    )
    parser.add_argument(
        "--workflow",
        default="release.yml",
        help="GitHub Actions workflow filename (default: %(default)s)",
    )
    parser.add_argument(
        "--environment",
        default="pypi",
        help="GitHub Actions environment name (default: %(default)s)",
    )
    parser.add_argument(
        "--api-token",
        required=True,
        help="PyPI API token (format: pypi-...). NOT a password.",
    )
    parser.add_argument(
        "--timeout",
        type=float,
        default=15.0,
        help="HTTP timeout in seconds (default: %(default)s)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print the payload that would be sent, then exit 0.",
    )
    parser.add_argument(
        "--json",
        dest="as_json",
        action="store_true",
        help="Emit the result as a single JSON line (for piping).",
    )
    return parser


def _print_human(
    project: str,
    owner: str,
    repo: str,
    workflow: str,
    environment: str,
    payload: Dict[str, Any],
    result: Tuple[bool, str],
) -> None:
    """Pretty-print the result to stdout/stderr for human operators."""
    out = sys.stdout
    print("=== PyPI Trusted Publisher setup ===", file=out)
    print(f"Project:    {project}", file=out)
    print(f"Owner:      {owner}", file=out)
    print(f"Repository: {repo}", file=out)
    print(f"Workflow:   {workflow}", file=out)
    print(f"Environment: {environment}", file=out)
    print("", file=out)
    print("Payload:", file=out)
    print(json.dumps(payload, indent=2), file=out)
    print("", file=out)

    success, message = result
    if success:
        print(f"OK  {message}", file=out)
        print("", file=out)
        print("Next steps:", file=out)
        print("  1. Check the email registered on the PyPI account.", file=out)
        print("  2. Click the magic link PyPI sent.", file=out)
        print(
            "  3. From now on, every `git tag v*.*.* && git push origin v*.*.*` in",
            file=out,
        )
        print(
            f"     {owner}/{repo} auto-publishes to PyPI in ~30s.",
            file=out,
        )
    else:
        print(f"FAIL  {message}", file=sys.stderr)
        print("", file=sys.stderr)
        print("Manual fallback:", file=sys.stderr)
        print(f"  1. Open {FALLBACK_URL}", file=sys.stderr)
        print("  2. Click 'Add a new pending publisher'.", file=sys.stderr)
        print(f"  3. Project name:        {project}", file=sys.stderr)
        print(f"  4. Owner:               {owner}", file=sys.stderr)
        print(f"  5. Repository name:     {repo}", file=sys.stderr)
        print(f"  6. Workflow filename:   {workflow}", file=sys.stderr)
        print(f"  7. Environment name:    {environment}", file=sys.stderr)


def main(argv: Optional[list] = None) -> int:
    """CLI entry point.

    Args:
        argv: Optional argument list (defaults to ``sys.argv[1:]``).

    Returns:
        Process exit code — ``0`` on success, ``1`` on any failure.
    """
    args = build_argparser().parse_args(argv)
    payload = get_pending_publisher_payload(
        args.project,
        args.owner,
        args.repo,
        args.workflow,
        args.environment,
    )

    if args.dry_run:
        if args.as_json:
            print(json.dumps({"dry_run": True, "payload": payload}))
        else:
            print(json.dumps(payload, indent=2))
        return 0

    result = add_pending_publisher(args.api_token, payload, timeout=args.timeout)

    if args.as_json:
        print(
            json.dumps(
                {
                    "success": result[0],
                    "message": result[1],
                    "payload": payload,
                }
            )
        )
    else:
        _print_human(
            args.project,
            args.owner,
            args.repo,
            args.workflow,
            args.environment,
            payload,
            result,
        )

    return 0 if result[0] else 1


if __name__ == "__main__":  # pragma: no cover
    sys.exit(main())
