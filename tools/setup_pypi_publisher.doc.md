# setup_pypi_publisher.sh — One-time PyPI Trusted Publisher registration

## What it does

Automates the PyPI **Trusted Publisher** setup for `OpenSIN-Code/SIN-Code-Bundle`.
After a single interactive run, every `git tag v*.*.* && git push origin v*.*.*`
auto-publishes to PyPI via `release.yml` without any API tokens.

The script:

1. Prompts for PyPI username + password (TOTP-aware: append the 6-digit code
   to the password if you have 2FA enabled).
2. POSTs a "pending publisher" registration to PyPI's
   [`_/v1/publisher`](https://docs.pypi.org/trusted-publishers/adding-a-publisher/#api)
   endpoint with the project metadata (owner, repo, workflow file, environment).
3. PyPI emails the maintainer a magic link. Click it to confirm.
4. Once confirmed, `release.yml` (which already has
   `permissions: id-token: write` and `environment: pypi`) gains the ability to
   mint short-lived OIDC tokens and publish to PyPI directly.

## Why this exists

Without a Trusted Publisher, `release.yml` would need a long-lived `PYPI_API_TOKEN`
stored as a GitHub Secret. Trusted Publishing is:

- **More secure** — no long-lived token to leak.
- **PyPI's recommended path** — the API token workflow is officially
  "legacy".
- **One-time cost** — register once, never touch a token again.

Reference: <https://docs.pypi.org/trusted-publishers/>

## Usage

```bash
bash tools/setup_pypi_publisher.sh
# or with explicit args:
bash tools/setup_pypi_publisher.sh OpenSIN-Code SIN-Code-Bundle release.yml pypi
```

Defaults: owner=`OpenSIN-Code`, repo=`SIN-Code-Bundle`, workflow=`release.yml`,
environment=`pypi`.

The script is **interactive** — it must run on a terminal (not a CI runner
without TTY) because it prompts for credentials. The credentials are passed
only to `curl` via `-u user:pass` and never written to disk.

## What release.yml already needs

Verified at v0.6.5: `.github/workflows/release.yml` already has:

```yaml
permissions:
  contents: write     # create GitHub Release
  id-token: write     # mint OIDC token for PyPI Trusted Publishing
```

```yaml
pypi-publish:
  environment: pypi   # MUST match the environment name registered here
```

If you change the environment name in the script (e.g. `pypi-test` for the
test instance), you must also update the `environment:` field in `release.yml`.

## Pre-conditions

- The PyPI project `sin-code-bundle` must already exist. The **first-ever**
  release must be a manual upload (e.g. `twine upload dist/*` with a
  short-lived API token). After that, the Trusted Publisher takes over for
  all subsequent releases.
- You must own (or be a maintainer of) the PyPI project.
- The maintainer's PyPI account email must be reachable to receive the
  confirmation magic link.

## Failure modes and fallbacks

The script distinguishes:

| HTTP | Meaning | Action |
|---|---|---|
| 201 / 200 | Pending publisher created | Check email, click magic link |
| 400 | Project doesn't exist on PyPI | Upload first release manually via `twine` |
| 409 | Publisher already pending | Check `https://pypi.org/manage/account/publishing/` |
| 422 | Validation error (bad env name, etc.) | Inspect response body, re-run with correct values |
| 401 / 403 | Auth failed | Wrong creds, or 2FA TOTP not appended to password |
| 000 | Network error | Check DNS / connectivity to pypi.org |

For any failure, the script prints a **manual fallback path**: open
<https://pypi.org/manage/account/publishing/>, click "Add a new pending
publisher", and enter the same values.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Pending publisher created — check email |
| 1 | Authentication failure, validation error, or network error |
| 2 | Missing prerequisite (`curl` or `python3` not on PATH) |

## Security notes

- Password is read via `read -s` (silent, no terminal echo) and passed
  directly to `curl -u`. It is **never** written to disk or echoed.
- The response body from PyPI is kept in `/tmp/pypi_publisher_resp.json`
  only for the duration of the script and deleted on exit (`rm -f`).
- `--max-time 30` caps any hung connection.
- Project names are normalised per [PEP 503](https://peps.python.org/pep-0503/)
  (lowercase, underscores → hyphens) — PyPI does this on its side too, so
  `SIN-Code-Bundle` and `sin-code-bundle` are the same project.

## Related files

- `.github/workflows/release.yml` — the workflow that consumes the Trusted
  Publisher on every `v*` tag push.
- `CHANGELOG.md` — entry for v0.6.6 documenting this script's addition.
- `README.md` — "Publishing to PyPI" section with a quick-start.
