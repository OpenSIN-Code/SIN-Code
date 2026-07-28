#!/usr/bin/env python3
"""Purpose: Post ceo-audit results to a PR as a comment.

Docs: post_audit_pr.doc.md

Usage:
    python3 post_audit_pr.py \\
        --repo OpenSIN-Code/SIN-Code \\
        --pr 42 \\
        --score-json ceo-audit-output/score.json \\
        --artifact-url "https://github.com/.../actions/runs/123#artifacts" \\
        --run-id 123

Required env:
    SIN_GITHUB_INSTALLATION_TOKEN  (preferred, short-lived)
    OR SIN_GITHUB_APP_CLIENT_SECRET + OAuth code (advanced)
    OR GITHUB_TOKEN                (fallback, CI built-in)
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

# ── Module overview ──────────────────────────────────────────────────
#
# Standalone wrapper around lib.github_app helpers. Usage in CI:
#
#   1. Run scripts/audit.sh to produce score.json
#   2. Run this script with --score-json + --pr to post the comment
#   3. The comment is idempotent — re-running on the same PR updates,
#      it never duplicates.
#
# Idempotency lives in lib.github_app via the COMMENT_MARKER token
# (HTML comment in the first line of every audit body). Searching for
# that token tells us whether to POST a new comment or PATCH an
# existing one.
#
# Dry-run mode (--dry-run) prints the comment to stdout WITHOUT
# touching GitHub — useful for local testing.
# ─────────────────────────────────────────────────────────────────────


# ── Path setup (stand-alone execution) ────────────────────────────────
# Allow running this script standalone (outside the skill context).
# Two levels up from this script = the skill root → expose `lib` as a package.
SKILL_DIR = Path(__file__).parent.parent
sys.path.insert(0, str(SKILL_DIR))

# Re-exports from the shared github_app helper module.
from lib.github_app import (  # noqa: E402
    build_audit_comment,
    find_existing_audit_comment,
    get_token,
    post_pr_comment,
    update_pr_comment,
)

# ── CLI entry point ──────────────────────────────────────────────────


def main() -> int:
    """CLI entry point — post (or update) the ceo-audit comment on a PR.

    Parses CLI flags, loads `--score-json`, builds the comment body via
    `lib.github_app.build_audit_comment`, and either:
      - prints the body when `--dry-run` is set, OR
      - posts a new PR comment, OR
      - updates an existing comment (idempotent via `<!-- ceo-audit -->` marker).

    Required env (exactly ONE of):
        SIN_GITHUB_INSTALLATION_TOKEN  (preferred, short-lived)
        GITHUB_TOKEN                   (fallback, CI built-in)

    Returns:
        0 on success or successful dry-run, 1 on error (missing token,
        missing score-json, or HTTP failure surfaced by github_app).
    """
    # ── argparse: every flag is keyword-only to keep CI invocations explicit
    # required=True for the two non-optional flags (repo + pr + score-json).
    # Defaults match the ceo-audit Action workflow defaults.
    parser = argparse.ArgumentParser(description="Post ceo-audit results to a PR")
    parser.add_argument("--repo", required=True, help="e.g., OpenSIN-Code/SIN-Code")
    parser.add_argument("--pr", type=int, required=True, help="PR number")
    parser.add_argument("--score-json", required=True, help="Path to score.json from audit run")
    parser.add_argument(
        "--artifact-url", default="", help="URL to download artifacts (e.g., workflow run)"
    )
    parser.add_argument("--run-id", default="", help="GitHub Actions run ID")
    parser.add_argument(
        "--profile", default="QUICK", help="Audit profile name (QUICK/RELEASE/SECURITY/FULL)"
    )
    parser.add_argument("--grade", default="B", help="Grade gate used (A/B/C)")
    parser.add_argument("--dry-run", action="store_true", help="Print comment, don't post")
    args = parser.parse_args()

    # ── Load score.json — bail with a clear error if it does not exist
    # score.json is produced by scripts/score.py; this script needs it.
    score_path = Path(args.score_json)
    if not score_path.exists():
        # Most common failure: someone forgot to run scripts/score.py first.
        print(f"ERROR: score.json not found: {score_path}", file=sys.stderr)
        return 1
    score = json.loads(score_path.read_text())

    # ── Build the Markdown comment body via the shared helper
    # `or None` converts empty strings from argparse to true None.
    # build_audit_comment treats None as "omit this section" (cleaner UX).
    body = build_audit_comment(
        grade=score.get("grade", "?"),
        score=score.get("score", 0),
        critical=score.get("critical", 0),
        high=score.get("high", 0),
        medium=score.get("severity_counts", {}).get("MEDIUM", 0),
        profile=args.profile,
        grade_gate=args.grade,
        artifact_url=args.artifact_url or None,
        run_id=args.run_id or None,
    )

    # ── Dry-run mode: print and exit, do NOT hit GitHub
    # Useful for verifying the comment body locally before pushing.
    if args.dry_run:
        print("=== DRY RUN — would post the following comment ===")
        print(body)
        print("=== END ===")
        return 0

    # ── Resolve token (env fallback chain — see github_app.get_token)
    # get_token() can raise EnvironmentError if env is genuinely broken.
    try:
        token = get_token()
    except EnvironmentError as e:
        # github_app raises with a helpful message — surface it directly.
        print(f"ERROR: {e}", file=sys.stderr)
        return 1
    if not token:
        # Defensive: get_token can return None when env is empty.
        # We need a token to make any GitHub API call — fail fast.
        print(
            "ERROR: No GitHub token found (set SIN_GITHUB_INSTALLATION_TOKEN or GITHUB_TOKEN)",
            file=sys.stderr,
        )
        return 1

    # ── Idempotent posting: update existing audit comment if found, else create
    # Matching is via the COMMENT_MARKER constant (HTML comment in body).
    # This is what makes "re-run the audit" safe — it never spams the PR.
    existing = find_existing_audit_comment(args.repo, args.pr, token=token)
    if existing:
        # Re-running the audit on the same PR rewrites the SAME comment.
        # PATCH /issues/comments/<id> is idempotent — same result every time.
        update_pr_comment(args.repo, existing, body, token=token)
        print(f"Updated existing ceo-audit comment #{existing} on {args.repo} PR #{args.pr}")
    else:
        # First audit run on this PR → fresh comment.
        # POST /issues/<pr>/comments creates a new comment, returns ID.
        post_pr_comment(args.repo, args.pr, body, token=token)
        print(f"Posted new ceo-audit comment on {args.repo} PR #{args.pr}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
