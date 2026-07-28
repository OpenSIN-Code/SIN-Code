"""Purpose: GitHub App OAuth integration for ceo-audit.

Docs: github_app.doc.md

Uses the SIN-GitHub-Issues-Prod-2026 GitHub App via OAuth flow.
NO Private Key required — uses Client ID + Client Secret (OAuth Web Flow)
or installation tokens via the GitHub App's user-to-server flow.

Environment variables (NEVER hardcode):
  SIN_GITHUB_APP_CLIENT_ID       — e.g., "Iv23livllaHIBTdQdyhY"
  SIN_GITHUB_APP_CLIENT_SECRET   — rotated via github.com/settings/apps/...
  SIN_GITHUB_APP_WEBHOOK_SECRET — for verifying incoming webhooks (optional)

Reference: https://docs.github.com/en/apps/creating-github-apps
"""

from __future__ import annotations

import hashlib
import hmac
import json
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, Optional

# ── Module overview ──────────────────────────────────────────────────
#
# Five API surfaces in this module:
#
#   1. CREDENTIALS    — _get_credentials reads OAuth client_id/secret
#                       from env vars (with DEFAULT_CLIENT_ID fallback).
#
#   2. TOKEN          — get_token / get_token_from_env resolve a usable
#                       Bearer token via env fallback chain:
#                         SIN_GITHUB_INSTALLATION_TOKEN → GITHUB_TOKEN
#                       (get_installation_token raises NotImplementedError
#                        on purpose — see github_app.doc.md for why.)
#
#   3. API HELPER     — gh_api handles every REST call: token resolution,
#                       URL prefixing, headers, JSON body, 30s timeout.
#
#   4. PR COMMENTS    — post_pr_comment / update_pr_comment +
#                       find_existing_audit_comment make audit comments
#                       idempotent via the COMMENT_MARKER token.
#
#   5. WEBHOOK / BP   — verify_webhook_signature (HMAC-SHA256, constant-time
#                       compare) + set_branch_protection (require ceo-audit
#                       as a status check).
#
# Why OAuth (not JWT)? The App's private key never leaves GitHub's
# encrypted store. Client ID + Client Secret + short-lived installation
# tokens are sufficient for our use case (CI bot posting PR comments).
# See github_app.doc.md for the full security rationale.
# ─────────────────────────────────────────────────────────────────────


# ── App credentials (public, safe to share) ──────────────────────────
# DEFAULT_CLIENT_ID is checked into source because GitHub OAuth treats
# the client ID as a public identifier, not a secret. The Client SECRET
# (stored in env) is the actual sensitive credential.
DEFAULT_CLIENT_ID = "Iv23livllaHIBTdQdyhY"
GITHUB_API_BASE = "https://api.github.com"
# OAuth Web Flow token-exchange endpoint (POST with client_id + secret + code).
OAUTH_TOKEN_URL = f"{GITHUB_API_BASE}/login/oauth/access_token"
# GitHub App installation listing endpoint (parent path; ID is appended later).
INSTALLATION_TOKEN_URL = f"{GITHUB_API_BASE}/app/installations"


# ── OAuth credentials ─────────────────────────────────────────────────


def _get_credentials() -> tuple[str, str]:
    """Read OAuth credentials from env. Raises if missing."""
    # Falls back to DEFAULT_CLIENT_ID when env var is unset.
    client_id = os.environ.get("SIN_GITHUB_APP_CLIENT_ID") or DEFAULT_CLIENT_ID
    client_secret = os.environ.get("SIN_GITHUB_APP_CLIENT_SECRET")
    if not client_secret:
        # Secret is mandatory — provide a self-help message with rotation URL.
        raise EnvironmentError(
            "SIN_GITHUB_APP_CLIENT_SECRET not set. "
            "Generate one at https://github.com/settings/apps/sin-github-issues-prod-2026 "
            "and export it: export SIN_GITHUB_APP_CLIENT_SECRET=<new-secret>"
        )
    return client_id, client_secret


# ── OAuth code exchange ───────────────────────────────────────────────


def exchange_code_for_token(code: str, redirect_uri: Optional[str] = None) -> Dict[str, Any]:
    """Exchange a one-time OAuth code for an access token.

    Used in the GitHub App's OAuth callback (e.g., on room13.delqhi.com).
    Returns {"access_token": "...", "expires_in": ..., "refresh_token": "...", "scope": "..."}
    """
    # Read credentials from env — raises if SIN_GITHUB_APP_CLIENT_SECRET missing.
    client_id, client_secret = _get_credentials()
    # OAuth 2.0 standard request body — see GitHub OAuth docs.
    # `code` comes from the GitHub redirect after user authorisation.
    data = {
        "client_id": client_id,
        "client_secret": client_secret,
        "code": code,
    }
    if redirect_uri:
        # Optional: only required if the App's callback URL is dynamic.
        # Most setups configure a single static URL in the App settings.
        data["redirect_uri"] = redirect_uri
    # Construct a POST request with form-encoded body.
    # Accept: application/json so GitHub returns JSON (vs default text).
    req = urllib.request.Request(
        OAUTH_TOKEN_URL,
        # urlencode → application/x-www-form-urlencoded body.
        data=urllib.parse.urlencode(data).encode("utf-8"),
        headers={"Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded"},
        method="POST",
    )
    # 30s timeout — GitHub's OAuth endpoint responds in < 1s normally.
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode("utf-8"))


# ── Installation token (deliberately not implemented) ────────────────


def get_installation_token(installation_id: str) -> str:
    """Fetch a fresh installation access token via the App's API.

    This is the proper way to act on behalf of the App.
    Requires the App to be installed on the target repo.
    """
    # Pulled to enforce the precondition; we still raise below.
    client_id, client_secret = _get_credentials()
    # First, get a JWT-style app token using client credentials (App-Owned)
    # Note: this requires the App to be configured for user-to-server or
    # we need a different flow. For most cases, use exchange_code_for_token()
    # OR have the user provide a pre-generated installation token.
    # See: https://docs.github.com/en/apps/creating-github-apps/authenticating-with-a-github-app/about-authentication-with-a-github-app
    raise NotImplementedError(
        "Installation tokens via OAuth alone require user-to-server flow. "
        "Either: (a) provide a pre-generated installation token via env SIN_GITHUB_INSTALLATION_TOKEN, "
        "or (b) use exchange_code_for_token() with a user OAuth flow."
    )


# ── Token resolution (env fallback chain) ────────────────────────────


def get_token_from_env() -> Optional[str]:
    """Convenience: read pre-generated installation token from env.

    The user can pre-generate an installation token at
    https://github.com/settings/apps/sin-github-issues-prod-2026/installations
    and set SIN_GITHUB_INSTALLATION_TOKEN=<token> in CI.
    """
    # Single-source-of-truth read from env. Returns None if unset.
    return os.environ.get("SIN_GITHUB_INSTALLATION_TOKEN")


def get_token() -> Optional[str]:
    """Resolve a GitHub token via OAuth or env.

    Priority:
      1. SIN_GITHUB_INSTALLATION_TOKEN (pre-generated, expires in 1h)
      2. GITHUB_TOKEN (built-in CI token)
      3. None (caller should fail with clear error)
    """
    # Check the App-specific installation token FIRST — preferred for App identity.
    tok = get_token_from_env()
    if tok:
        return tok
    # Fall back to the generic GitHub Actions token (works in any CI).
    return os.environ.get("GITHUB_TOKEN")


# ─────────────────────────────────────────────────────────────────────
# GitHub API helpers (work with any valid token)
# ─────────────────────────────────────────────────────────────────────


def gh_api(
    method: str, path: str, token: Optional[str] = None, body: Optional[Dict] = None
) -> Dict:
    """Call GitHub API with the given method/path/token."""
    # Token resolution chain: arg → env helpers → raw env GITHUB_TOKEN.
    # First non-empty value wins — explicit arg takes precedence.
    token = token or get_token() or os.environ.get("GITHUB_TOKEN")
    if not token:
        raise EnvironmentError(
            "No GitHub token (set SIN_GITHUB_INSTALLATION_TOKEN or GITHUB_TOKEN)"
        )
    # Allow absolute URLs (artifact downloads, etc.) by skipping the prefix.
    # If `path` already starts with http, treat it as a full URL.
    url = path if path.startswith("http") else f"{GITHUB_API_BASE}{path}"
    # Standard GitHub headers — required for all API calls.
    headers = {
        "Authorization": f"Bearer {token}",
        # `application/vnd.github+json` activates GitHub's stable API media type.
        "Accept": "application/vnd.github+json",
        # API version pin — locks behaviour even if GitHub releases a new default.
        "X-GitHub-Api-Version": "2022-11-28",
    }
    # Encode body as JSON if present; otherwise leave None for GET/DELETE.
    data = json.dumps(body).encode("utf-8") if body else None
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    # 30s timeout — generous; GitHub usually responds in 200-500 ms.
    # urlopen raises HTTPError for 4xx/5xx — caller handles via try/except.
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode("utf-8"))


# ─────────────────────────────────────────────────────────────────────
# PR Commenter (the actual ceo-audit integration)
# ─────────────────────────────────────────────────────────────────────


def post_pr_comment(repo: str, pr_number: int, body: str, token: Optional[str] = None) -> Dict:
    """Post a comment on a PR.

    repo: e.g., 'OpenSIN-Code/SIN-Code'
    pr_number: e.g., 42
    body: Markdown comment text
    """
    # Note: PR comments use the issues endpoint — PRs are issues + diff.
    # GitHub treats every PR as an issue with extra metadata (diff, mergeability).
    # The comments endpoint is shared between issues and PRs.
    return gh_api(
        "POST",
        f"/repos/{repo}/issues/{pr_number}/comments",
        token=token,
        body={"body": body},
    )


def update_pr_comment(repo: str, comment_id: int, body: str, token: Optional[str] = None) -> Dict:
    """Update an existing PR comment (idempotent PR comments)."""
    # PATCH endpoint preserves the comment ID — same URL for everyone.
    # The body field is the ONLY mutable property (author + timestamps are fixed).
    return gh_api(
        "PATCH",
        f"/repos/{repo}/issues/comments/{comment_id}",
        token=token,
        body={"body": body},
    )


def find_existing_audit_comment(
    repo: str, pr_number: int, marker: str = "<!-- ceo-audit -->", token: Optional[str] = None
) -> Optional[int]:
    """Find an existing ceo-audit comment on a PR (for idempotent updates)."""
    # per_page=100 = max GitHub allows in one call; enough for any normal PR.
    # If a PR has >100 comments, the audit comment is likely in the recent
    # batch anyway (we always add it last) so this single page is sufficient.
    comments = gh_api(
        "GET",
        f"/repos/{repo}/issues/{pr_number}/comments?per_page=100",
        token=token,
    )
    # Linear scan — fine for the small comment counts we expect.
    # Returns the FIRST matching comment (we never post multiple).
    for c in comments:
        if marker in c.get("body", ""):
            return c["id"]
    # No existing audit comment → caller should POST a new one.
    return None


# ─────────────────────────────────────────────────────────────────────
# Branch protection (enforce ceo-audit as required check)
# ─────────────────────────────────────────────────────────────────────


def set_branch_protection(
    repo: str,
    branch: str = "main",
    require_status_checks: Optional[list] = None,
    token: Optional[str] = None,
) -> Dict:
    """Enable branch protection requiring status checks (e.g., ceo-audit)."""
    # PUT /branches/<branch>/protection — fully replaces existing protection.
    # `strict: True` requires the branch to be up-to-date before merge.
    # This blocks merging stale PRs even if all checks pass.
    body = {
        "required_status_checks": {
            "strict": True,
            # Default: only ceo-audit. Callers can pass their own list.
            # Multiple contexts means ALL must pass for merge to be allowed.
            "contexts": require_status_checks or ["ceo-audit"],
        },
        # Even admins must follow the rules (no escape hatch).
        # Disable this if you need an emergency merge override.
        "enforce_admins": True,
        # We don't enforce review requirements here — that's a separate concern.
        # Review requirements would be configured via a separate API call.
        "required_pull_request_reviews": None,
        # No user/team restrictions on who can push to the branch.
        "restrictions": None,
    }
    return gh_api(
        "PUT",
        f"/repos/{repo}/branches/{branch}/protection",
        token=token,
        body=body,
    )


# ─────────────────────────────────────────────────────────────────────
# Webhook verification (for room13.delqhi.com to verify incoming events)
# ─────────────────────────────────────────────────────────────────────


def verify_webhook_signature(
    payload_body: bytes,
    signature_header: str,
    secret: Optional[str] = None,
) -> bool:
    """Verify GitHub webhook signature against the shared secret.

    See: https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
    """
    # Secret resolution: arg → env var. Env var is the production path.
    secret = secret or os.environ.get("SIN_GITHUB_APP_WEBHOOK_SECRET")
    if not secret:
        # No secret configured → cannot verify → reject (defensive default).
        # Better to reject than to accept all webhooks blindly.
        return False
    if not signature_header.startswith("sha256="):
        # Reject downgrade attacks: only sha256 is acceptable.
        # GitHub also sends sha1 for backward compat — we ignore it.
        return False
    # Compute expected HMAC-SHA256 over the raw payload bytes.
    # IMPORTANT: hash the RAW request body, NOT a re-serialised version.
    # Even whitespace differences would break the signature.
    expected = (
        "sha256="
        + hmac.new(secret.encode("utf-8"), msg=payload_body, digestmod=hashlib.sha256).hexdigest()
    )
    # Constant-time compare to defeat timing attacks.
    # hmac.compare_digest takes the same time regardless of mismatch position.
    return hmac.compare_digest(expected, signature_header)


# ─────────────────────────────────────────────────────────────────────
# ceo-audit comment template
# ─────────────────────────────────────────────────────────────────────

# COMMENT_MARKER is the HTML-comment idempotency token embedded in the
# first line of every audit comment. find_existing_audit_comment greps
# for this marker to decide between POST (new) and PATCH (update).
# Changing this string would orphan all existing audit comments — they
# would no longer be found and a duplicate would be posted instead.
COMMENT_MARKER = "<!-- ceo-audit -->"


def build_audit_comment(
    grade: str,
    score: float,
    critical: int,
    high: int,
    medium: int = 0,
    profile: str = "QUICK",
    grade_gate: str = "B",
    report_url: Optional[str] = None,
    artifact_url: Optional[str] = None,
    run_id: Optional[str] = None,
) -> str:
    """Build the Markdown body for a ceo-audit PR comment."""
    # COMMENT_MARKER must be the FIRST line — find_existing_audit_comment greps for it.
    # The "🏆" emoji + grade in the heading make the comment easy to scan.
    body = f"""{COMMENT_MARKER}
## 🏆 CEO Audit — {grade} ({score}/100)

| Metric | Value |
|--------|-------|
| **Grade** | **{grade}** |
| **Score** | **{score}/100** |
| **Critical findings** | {critical} |
| **High findings** | {high} |
| **Medium findings** | {medium} |
| **Profile** | `{profile}` |
| **Min grade gate** | {grade_gate} |
"""
    # Optional sections — only rendered when the corresponding kwarg is set.
    # Empty-string args are coerced to None by the caller via `or None`.
    if artifact_url:
        # 📥 = download icon — instantly recognisable in PR threads.
        body += f"\n📥 [Download full report (Markdown)]({artifact_url})\n"
    if report_url:
        # 🔗 = link icon — for the ceo-audit skill landing page.
        body += f"\n🔗 [View in ceo-audit skill]({report_url})\n"
    if run_id:
        # Run ID enables traceability from the PR comment to the workflow run.
        # ${{github.sha}} is rendered as the commit SHA by the GitHub UI.
        body += f"\n_Run ID: `{run_id}` · Commit: `${{github.sha}}`_\n"
    # Footer with the reproducer command — empowers reviewers to re-run locally.
    # The path is canonical inside a SIN-Code source checkout.
    body += f"\n> Run `skills/code-skills/skill-code-ceo-audit/scripts/audit.sh . --profile={profile}` locally to reproduce.\n"
    return body


# ── Public API ────────────────────────────────────────────────────────
# Explicit __all__ — only these names are intended for re-export.
# `_get_credentials` is intentionally included so tests can patch it.
__all__ = [
    "DEFAULT_CLIENT_ID",
    "GITHUB_API_BASE",
    "OAUTH_TOKEN_URL",
    "INSTALLATION_TOKEN_URL",
    "COMMENT_MARKER",
    "_get_credentials",
    "exchange_code_for_token",
    "get_installation_token",
    "get_token_from_env",
    "get_token",
    "gh_api",
    "post_pr_comment",
    "update_pr_comment",
    "find_existing_audit_comment",
    "set_branch_protection",
    "verify_webhook_signature",
    "build_audit_comment",
]
