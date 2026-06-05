# Purpose: Doc companion for pypi_setup.py — what it does and when to use it.
# Docs: pypi_setup.py

# `pypi_setup.py` — One-click PyPI Trusted Publisher setup

## What it does

Registers a **PyPI Trusted Publisher** for the
`OpenSIN-Code/SIN-Code-Bundle` repo (or any other repo, via flags) in a
single CLI call. After registration, every `git tag v*.*.* && git push
origin v*.*.*` triggers `.github/workflows/release.yml`, which mints a
short-lived OIDC token via `id-token: write` and publishes to PyPI
**without any API token stored in GitHub secrets**.

This is the **API-token variant** of the older
`tools/setup_pypi_publisher.sh` script. That script required interactive
PyPI username + password (and a TOTP code if 2FA was enabled). This
module replaces that flow with a non-interactive call that uses a
**PyPI API token** (created at <https://pypi.org/manage/account/token/>)
as the credential.

## Usage

```bash
# 1. Generate a PyPI API token at https://pypi.org/manage/account/token/
#    (scope: "Entire account" works, or scope to a single project)
# 2. Run:
export PYPI_API_TOKEN="pypi-..."
python -m sin_code_bundle.tools.pypi_setup --api-token "$PYPI_API_TOKEN"

# Or with custom values for a different repo:
python -m sin_code_bundle.tools.pypi_setup \
  --project my-package \
  --owner MyOrg \
  --repo MyRepo \
  --workflow release.yml \
  --environment pypi \
  --api-token "$PYPI_API_TOKEN"

# Dry run (prints payload, no HTTP call):
python -m sin_code_bundle.tools.pypi_setup --dry-run --api-token dummy

# Machine-readable output (one JSON line):
python -m sin_code_bundle.tools.pypi_setup --api-token "$PYPI_API_TOKEN" --json
```

The script then:

1. Builds the JSON payload (`name`, `owner`, `repository`,
   `workflow_filename`, `environment`) with PEP 503 normalisation on
   `name` (lowercase, runs of `[-_.]+` collapsed to `-`).
2. POSTs to `https://pypi.org/_/v1/publisher` with
   `Authorization: Token <api-token>`.
3. On HTTP 201: prints a success message + next-step instructions
   (check email, click magic link).
4. On any other status: prints a failure message + the manual-fallback
   URL with the same fields filled in.

## Why an API token (not username/password)?

| Aspect | Username + password | API token |
|---|---|---|
| Interactive prompt | Yes (bad for CI / agents) | No (1 CLI call) |
| 2FA / TOTP | Must append 6-digit code to password | Not needed |
| Revocation granularity | Whole account | Per-token, scoped |
| Scriptable in CI | No | Yes |
| Token leak blast radius | Account compromise | Single project, expirable |

PyPI's own docs recommend API tokens for programmatic use. Username +
password is officially "legacy" and being phased out of the PyPI web
UI's publisher registration flow.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Pending publisher registered (HTTP 201), or `--dry-run` |
| 1 | Any PyPI error (400, 401, 403, 409, 422), network error, or timeout |

## Failure modes

The HTTP responses from `https://pypi.org/_/v1/publisher` are
interpreted as follows (the `add_pending_publisher` function surfaces
the status and body in the message):

| HTTP | Likely cause | Action |
|---|---|---|
| 201 | Success | Check email, click magic link |
| 400 | Project name doesn't exist on PyPI | Manually upload the first release with `twine`, then re-run |
| 401 | Invalid / expired API token | Re-generate at <https://pypi.org/manage/account/token/> |
| 403 | Token doesn't have scope to register publishers | Generate a token with "Entire account" scope |
| 409 | Publisher already pending for this (owner, repo, workflow) | Check the existing registration at <https://pypi.org/manage/account/publishing/> |
| 422 | Validation error (bad workflow filename, env name, etc.) | Inspect the response body, re-run with correct values |

## Pre-conditions

- The PyPI project `sin-code-bundle` must already exist (PyPI refuses
  to register a Trusted Publisher for a project that has never been
  uploaded to). For a brand-new project, do a one-off `twine upload`
  first; from then on, Trusted Publishing takes over.
- The API token must belong to a user with **Maintainer** rights on
  the PyPI project.
- The maintainer account's email must be reachable to receive the
  magic-link confirmation.
- `.github/workflows/release.yml` must have `permissions: id-token:
  write` and `environment: pypi` (already in place for this repo since
  v0.6.5).

## Security notes

- The API token is read from the `--api-token` flag. Prefer the env-var
  pattern (`--api-token "$PYPI_API_TOKEN"`) so the token doesn't end up
  in shell history.
- The token is sent over HTTPS only (`Authorization: Token ...`).
- The module does **not** write the token to disk or to logs.
- The HTTP timeout is 15s by default; tune with `--timeout`.

## Related files

- `tools/setup_pypi_publisher.sh` — the older, interactive
  username/password variant. Kept for users who do not have an API
  token (e.g. legacy PyPI accounts without 2FA disabled).
- `tools/setup_pypi_publisher.doc.md` — its CoDocs companion.
- `.github/workflows/release.yml` — the workflow that consumes the
  Trusted Publisher (id-token: write + environment: pypi).
- `CHANGELOG.md` — v0.8.1 entry documenting the addition of this
  module.
- `README.md` — "Publishing to PyPI" section with the quick-start.
