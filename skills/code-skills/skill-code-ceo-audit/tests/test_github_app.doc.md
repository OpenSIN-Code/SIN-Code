# test_github_app.py

**Purpose:** Tests for `lib/github_app.py` — the OAuth-based GitHub App
integration used to post `ceo-audit` results on Pull Requests.

**Docs:** test_github_app.doc.md

## What it tests

The 18 tests in this file cover every public function of
`lib/github_app.py`. Each test is hermetic — no network calls, no real
GitHub App credentials required. All HTTP and `os.environ` reads are
patched with `unittest.mock`.

| Test | Covers |
|------|--------|
| `test_default_client_id` | `DEFAULT_CLIENT_ID` constant is the production app |
| `test_get_credentials_raises_without_secret` | `_get_credentials` validates secret presence |
| `test_get_credentials_uses_env` | `_get_credentials` reads `SIN_GITHUB_APP_CLIENT_*` |
| `test_get_credentials_uses_default_client_id` | Default client ID fallback |
| `test_get_token_from_env` | `get_token_from_env` reads installation token |
| `test_get_token_from_env_returns_none` | Returns `None` cleanly when no token |
| `test_get_token_priority` | Installation token preferred over `GITHUB_TOKEN` |
| `test_get_token_fallback_to_github_token` | Falls back to `GITHUB_TOKEN` |
| `test_build_audit_comment_includes_marker` | Comment starts with `<!-- ceo-audit -->` |
| `test_build_audit_comment_includes_artifact_url` | Artifact link rendered |
| `test_build_audit_comment_no_artifact_url` | Omitted when not provided |
| `test_verify_webhook_signature_no_secret` | Defensive False on missing secret |
| `test_verify_webhook_signature_valid` | HMAC-SHA256 match returns True |
| `test_verify_webhook_signature_invalid` | Wrong digest returns False |
| `test_verify_webhook_signature_wrong_prefix` | Only `sha256=` prefix accepted |
| `test_get_installation_token_raises` | `NotImplementedError` (OAuth choice) |
| `test_gh_api_requires_token` | `gh_api` raises when no token available |
| `test_build_audit_comment_with_run_id` | `run_id` appears in footer |

## How tests stay hermetic

- All `os.environ` access goes through `patch.dict(os.environ, ...)` so
  the host environment cannot bleed into the tests.
- The `clear=True` flag in critical tests guarantees no leftover env
  vars from prior test runs poison the assertions.
- `os.environ.pop(..., None)` is used between `patch.dict` calls to
  cover the case where the host process exports these vars.
- The webhook signature tests compute the HMAC inline (`hmac.new(...)`)
  so they verify both the happy path and the rejection path without
  needing recorded fixtures.

## Fixtures

None — all setup is inline via `patch.dict` / `MagicMock`. No
`conftest.py` is required.

## Running

```bash
cd ~/.config/opencode/skills/ceo-audit

# Run only this test file
python3 -m pytest tests/test_github_app.py -v

# Run a single test
python3 -m pytest tests/test_github_app.py::test_get_token_priority -v

# Run the full suite (this file + test_sin_tools + test_audit_end_to_end)
python3 -m pytest tests/ -q
```

## Required environment

None. The tests deliberately **do not** require real GitHub credentials.
If `SIN_GITHUB_APP_CLIENT_SECRET` is set in your shell, the tests still
pass — `patch.dict` overrides for the duration of each test.

## Known caveats

- `test_verify_webhook_signature_*` uses `hmac.compare_digest` indirectly
  through `github_app.verify_webhook_signature`. Do **not** replace the
  inline `hmac.new` computation with a cached fixture — that would mask
  bugs in the comparison logic.
- `test_get_installation_token_raises` documents a **deliberate
  limitation**: we use OAuth + short-lived installation tokens, not JWT.
  This test must continue to assert `NotImplementedError` so anyone who
  later "implements" the function knows to update the spec first.
- The tests import `github_app` via `sys.path.insert(0, str(SKILL_DIR / "lib"))`
  so this file must remain inside `tests/` (one level deep from skill root).

## See also

- `tests/test_sin_tools.py` — sibling test for the SIN-Code wrapper
- `tests/test_audit_end_to_end.py` — full-pipeline integration test
- `lib/github_app.py` — the module under test
- `lib/github_app.doc.md` — its companion docs
- `scripts/post_audit_pr.py` — the script that uses the tested API in CI
