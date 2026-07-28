# github_app.py

**Purpose:** GitHub App OAuth integration for ceo-audit skill — enables the SIN-GitHub-Issues-Prod-2026 app to post ceo-audit results on PRs and enforce branch protection.

**Docs:** github_app.doc.md

## What it does

This module provides:

1. **OAuth flow** (`exchange_code_for_token`) — exchanges a one-time OAuth code for an access token. Used in the room13.delqhi.com callback when a user installs the app.

2. **Token resolution** (`get_token`) — returns a GitHub token from:
   - `SIN_GITHUB_INSTALLATION_TOKEN` env var (pre-generated, expires in 1h)
   - Falls back to `GITHUB_TOKEN` env var (the built-in CI token)

3. **PR comment API** (`post_pr_comment`, `update_pr_comment`, `find_existing_audit_comment`) — idempotent PR commenting with the `<!-- ceo-audit -->` marker for upserts.

4. **Branch protection** (`set_branch_protection`) — enables "require ceo-audit status check" on `main` so PRs without a passing audit cannot be merged.

5. **Webhook signature verification** (`verify_webhook_signature`) — for the room13.delqhi.com webhook receiver to verify that incoming events are from GitHub.

## Why OAuth (not JWT)

The SIN-GitHub-Issues-Prod-2026 app's **private key never leaves** https://github.com/settings/apps/sin-github-issues-prod-2026 (encrypted at rest). The user shared:
- App ID: 3223886 (public)
- Client ID: Iv23livllaHIBTdQdyhY (public, used for OAuth)
- Client Secret: rotated (NEVER share in chat — see SECURITY section)
- Webhook URL: https://room13.delqhi.com/api/webhooks/github-apps (public)

OAuth uses the **Client ID + Client Secret** to mint short-lived access tokens. No private key required.

## Usage

### In a PR workflow (GitHub Actions)

```yaml
- name: Post ceo-audit comment
  env:
    SIN_GITHUB_INSTALLATION_TOKEN: ${{ secrets.SIN_GITHUB_INSTALLATION_TOKEN }}
  run: |
    python3 -c "
    from sin_code_bundle.skills.ceo_audit.lib.github_app import (
        get_token, post_pr_comment, find_existing_audit_comment, update_pr_comment, build_audit_comment
    )
    from pathlib import Path
    import json
    score = json.loads(Path('score.json').read_text())
    token = get_token()
    repo = 'OpenSIN-Code/SIN-Code'
    pr_number = ${{ github.event.pull_request.number }}
    body = build_audit_comment(
        grade=score['grade'],
        score=score['score'],
        critical=score['critical'],
        high=score['high'],
        medium=score['severity_counts'].get('MEDIUM', 0),
        artifact_url=f'${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}#artifacts',
        run_id='${{ github.run_id }}',
    )
    existing = find_existing_audit_comment(repo, pr_number, token=token)
    if existing:
        update_pr_comment(repo, existing, body, token=token)
    else:
        post_pr_comment(repo, pr_number, body, token=token)
    "
```

### Branch protection (one-time setup per repo)

```python
from sin_code_bundle.skills.ceo_audit.lib.github_app import set_branch_protection
set_branch_protection("OpenSIN-Code/SIN-Code", branch="main", require_status_checks=["ceo-audit"])
```

## Environment variables

| Name | Required? | Where to get | Security |
|------|-----------|--------------|----------|
| `SIN_GITHUB_APP_CLIENT_ID` | No (has default) | https://github.com/settings/apps/sin-github-issues-prod-2026 | Public-ish |
| `SIN_GITHUB_APP_CLIENT_SECRET` | **YES** for OAuth | Same page → Generate | ⚠️ SECRET — use GitHub Secrets |
| `SIN_GITHUB_INSTALLATION_TOKEN` | For comment posting | https://github.com/settings/apps/.../installations | ⚠️ Expires in 1h — refresh |
| `SIN_GITHUB_APP_WEBHOOK_SECRET` | For incoming webhooks | Same page → Webhook secret | ⚠️ SECRET — use env on receiver |

## SECURITY — Important

The **Client Secret** is a credential. **Never**:
- Commit it to git
- Put it in chat/logs
- Store it in plaintext files
- Share it publicly

The **Client Secret shared in this conversation MUST be rotated immediately**:
1. Go to https://github.com/settings/apps/sin-github-issues-prod-2026
2. Click "Generate a new client secret"
3. Old secret becomes invalid
4. Store new secret in GitHub Secrets (encrypted at rest) or 1Password

## Limitations

- `get_installation_token()` raises `NotImplementedError` — proper installation tokens require either:
  - Pre-generated token (env var) — what we use
  - User-to-server OAuth flow with a one-time code — requires a web flow
- This is a **deliberate trade-off**: simpler implementation, less secure than JWT, but **sufficient for our use case** (CI bots posting PR comments)

## Files

- `lib/github_app.py` — this module
- `lib/github_app.doc.md` — this file
- `SKILL.md` — references this module for OAuth-based app integration

## Touched by

SIN-Code maintainers. Changes here affect how the ceo-audit skill integrates with the SIN-GitHub-Issues-Prod-2026 GitHub App.
