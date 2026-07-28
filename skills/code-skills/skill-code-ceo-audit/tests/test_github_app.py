"""Purpose: Tests for github_app.py (OAuth-based GitHub App integration).

Docs: test_github_app.doc.md

Run: python3 -m pytest tests/test_github_app.py -v
"""

import os
import sys
from pathlib import Path
from unittest.mock import patch

import pytest

# Make `lib/` importable without installing the skill as a package.
# Mirrors the layout: skills/code-skills/skill-code-ceo-audit/{lib,tests}/.
SKILL_DIR = Path(__file__).parent.parent
sys.path.insert(0, str(SKILL_DIR / "lib"))

import github_app  # noqa: E402

# ── Credentials & client identity ─────────────────────────────────────


def test_default_client_id():
    """Default client ID is the production SIN-GitHub-Issues-Prod-2026 app."""
    # Pinned: changing this constant means rotating the OAuth app entirely.
    assert github_app.DEFAULT_CLIENT_ID == "Iv23livllaHIBTdQdyhY"


def test_get_credentials_raises_without_secret():
    """_get_credentials must raise EnvironmentError if no secret is set."""
    # clear=True wipes the host env so SIN_GITHUB_APP_CLIENT_SECRET cannot leak in.
    with patch.dict(os.environ, {}, clear=True):
        # Belt + suspenders: explicit pop even after clear=True.
        os.environ.pop("SIN_GITHUB_APP_CLIENT_SECRET", None)
        with pytest.raises(EnvironmentError, match="SIN_GITHUB_APP_CLIENT_SECRET"):
            github_app._get_credentials()


def test_get_credentials_uses_env():
    """_get_credentials reads from env vars."""
    # Both env vars set — function returns them verbatim.
    with patch.dict(
        os.environ,
        {
            "SIN_GITHUB_APP_CLIENT_ID": "test_client",
            "SIN_GITHUB_APP_CLIENT_SECRET": "test_secret",
        },
    ):
        cid, secret = github_app._get_credentials()
        assert cid == "test_client"
        assert secret == "test_secret"


def test_get_credentials_uses_default_client_id():
    """When client_id not in env, DEFAULT_CLIENT_ID is used."""
    # Only secret in env → client_id should fall back to the default.
    with patch.dict(os.environ, {"SIN_GITHUB_APP_CLIENT_SECRET": "s"}, clear=False):
        os.environ.pop("SIN_GITHUB_APP_CLIENT_ID", None)
        cid, secret = github_app._get_credentials()
        assert cid == github_app.DEFAULT_CLIENT_ID


# ── Token resolution (env → fallback chain) ───────────────────────────


def test_get_token_from_env():
    """get_token_from_env returns SIN_GITHUB_INSTALLATION_TOKEN if set."""
    # Pop GITHUB_TOKEN so we know the installation token wins on its own.
    with patch.dict(os.environ, {"SIN_GITHUB_INSTALLATION_TOKEN": "tok123"}, clear=False):
        os.environ.pop("GITHUB_TOKEN", None)
        assert github_app.get_token_from_env() == "tok123"


def test_get_token_from_env_returns_none():
    """Returns None when no token is set."""
    # No tokens anywhere — function must return None, NOT raise.
    with patch.dict(os.environ, {}, clear=True):
        os.environ.pop("SIN_GITHUB_INSTALLATION_TOKEN", None)
        os.environ.pop("GITHUB_TOKEN", None)
        assert github_app.get_token_from_env() is None


def test_get_token_priority():
    """get_token prefers installation token over GITHUB_TOKEN."""
    # Both tokens present — installation token (short-lived, App identity) wins.
    with patch.dict(
        os.environ,
        {
            "SIN_GITHUB_INSTALLATION_TOKEN": "inst_tok",
            "GITHUB_TOKEN": "gh_tok",
        },
    ):
        assert github_app.get_token() == "inst_tok"


def test_get_token_fallback_to_github_token():
    """get_token falls back to GITHUB_TOKEN."""
    # No installation token → fall back to the built-in CI GITHUB_TOKEN.
    with patch.dict(os.environ, {"GITHUB_TOKEN": "gh_tok"}, clear=False):
        os.environ.pop("SIN_GITHUB_INSTALLATION_TOKEN", None)
        assert github_app.get_token() == "gh_tok"


# ── Audit comment builder (Markdown PR body) ──────────────────────────


def test_build_audit_comment_includes_marker():
    """Audit comment starts with the idempotency marker."""
    body = github_app.build_audit_comment(grade="A+", score=99.5, critical=0, high=0, medium=2)
    # Marker must be FIRST line — find_existing_audit_comment matches on it.
    assert body.startswith(github_app.COMMENT_MARKER)
    assert "A+" in body
    assert "99.5" in body
    assert "**Critical findings**" in body
    assert "**High findings**" in body


def test_build_audit_comment_includes_artifact_url():
    """Audit comment includes artifact download link when provided."""
    body = github_app.build_audit_comment(
        grade="A",
        score=90,
        critical=0,
        high=0,
        artifact_url="https://github.com/test/artifacts/123",
    )
    # Download section must appear when artifact_url is non-empty.
    assert "https://github.com/test/artifacts/123" in body
    assert "Download" in body


def test_build_audit_comment_no_artifact_url():
    """Audit comment omits artifact section when no URL."""
    body = github_app.build_audit_comment(grade="B", score=80, critical=0, high=0)
    # No artifact_url kwarg → "Download" section should NOT be rendered.
    assert "Download" not in body


# ── Webhook signature verification (HMAC-SHA256) ──────────────────────


def test_verify_webhook_signature_no_secret():
    """Returns False when no secret is configured."""
    # Defensive default: missing secret means we cannot verify, so reject.
    with patch.dict(os.environ, {}, clear=True):
        os.environ.pop("SIN_GITHUB_APP_WEBHOOK_SECRET", None)
        result = github_app.verify_webhook_signature(
            payload_body=b"{}",
            signature_header="sha256=abc",
        )
        assert result is False


def test_verify_webhook_signature_valid():
    """Returns True when signature matches."""
    # Compute the HMAC inline so the test self-verifies the algorithm.
    import hashlib
    import hmac

    secret = "test-webhook-secret"
    payload = b'{"action":"opened"}'
    # Format must be 'sha256=<hex>' — GitHub's canonical header shape.
    expected = "sha256=" + hmac.new(secret.encode(), payload, hashlib.sha256).hexdigest()
    with patch.dict(os.environ, {"SIN_GITHUB_APP_WEBHOOK_SECRET": secret}):
        result = github_app.verify_webhook_signature(
            payload_body=payload, signature_header=expected
        )
        assert result is True


def test_verify_webhook_signature_invalid():
    """Returns False when signature does NOT match."""
    # Body 'abc' (empty payload signed with wrong digest) must be rejected.
    with patch.dict(os.environ, {"SIN_GITHUB_APP_WEBHOOK_SECRET": "secret"}):
        result = github_app.verify_webhook_signature(
            payload_body=b"{}",
            signature_header="sha256=deadbeef",
        )
        assert result is False


def test_verify_webhook_signature_wrong_prefix():
    """Returns False when signature header doesn't start with 'sha256='."""
    # We only accept sha256 — md5/sha1 are downgrade attacks, reject early.
    with patch.dict(os.environ, {"SIN_GITHUB_APP_WEBHOOK_SECRET": "secret"}):
        result = github_app.verify_webhook_signature(
            payload_body=b"{}",
            signature_header="md5=abc",
        )
        assert result is False


# ── Deliberate-NotImplemented + token requirement ─────────────────────


def test_get_installation_token_raises():
    """get_installation_token is intentionally not implemented (OAuth choice)."""
    # OAuth-only design: a JWT installation flow would require the private key.
    # We use pre-generated tokens instead — see github_app.doc.md.
    with patch.dict(os.environ, {"SIN_GITHUB_APP_CLIENT_SECRET": "s"}):
        with pytest.raises(NotImplementedError, match="user-to-server"):
            github_app.get_installation_token("12345")


def test_gh_api_requires_token():
    """gh_api raises if no token is available."""
    # Fail fast with a clear message — better than a 401 HTTP roundtrip.
    with patch.dict(os.environ, {}, clear=True):
        os.environ.pop("SIN_GITHUB_INSTALLATION_TOKEN", None)
        os.environ.pop("GITHUB_TOKEN", None)
        with pytest.raises(EnvironmentError, match="No GitHub token"):
            github_app.gh_api("GET", "/repos/test/test")


def test_build_audit_comment_with_run_id():
    """Run ID appears in the comment footer."""
    body = github_app.build_audit_comment(grade="A", score=90, critical=0, high=0, run_id="12345")
    # Run ID enables traceability from the PR back to the workflow run.
    assert "Run ID" in body
    assert "12345" in body
