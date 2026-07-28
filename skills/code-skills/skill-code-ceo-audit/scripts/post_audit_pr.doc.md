# post_audit_pr.py

**Purpose:** Posts ceo-audit results as a sticky comment on a GitHub Pull Request using the SIN-GitHub-Issues-Prod-2026 GitHub App (OAuth-based, no Private Key required).

**Docs:** post_audit_pr.doc.md

## What it does

1. Reads `score.json` (produced by `audit.sh`)
2. Builds a Markdown comment body with grade, score, and top risks
3. Resolves a GitHub token from env (priority: `SIN_GITHUB_INSTALLATION_TOKEN` > `GITHUB_TOKEN`)
4. Looks for an existing `<!-- ceo-audit -->` comment on the PR (idempotent)
5. If found: updates it; if not: posts a new comment
6. Returns exit 0 on success, 1 on failure

## Usage

```bash
# From a ceo-audit skill run
python3 skills/code-skills/skill-code-ceo-audit/scripts/post_audit_pr.py \
    --repo OpenSIN-Code/SIN-Code \
    --pr 42 \
    --score-json /tmp/ceo-audit-output/score.json \
    --artifact-url "https://github.com/.../actions/runs/123#artifacts" \
    --run-id 123
```

## In a GitHub Actions workflow (after ceo-audit.yml)

```yaml
- name: Post ceo-audit comment on PR
  if: github.event_name == 'pull_request'
  env:
    SIN_GITHUB_INSTALLATION_TOKEN: ${{ secrets.SIN_GITHUB_INSTALLATION_TOKEN }}
  run: |
    python3 skills/code-skills/skill-code-ceo-audit/scripts/post_audit_pr.py \
      --repo "${{ github.repository }}" \
      --pr "${{ github.event.pull_request.number }}" \
      --score-json ceo-audit-output/score.json \
      --artifact-url "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}#artifacts" \
      --run-id "${{ github.run_id }}" \
      --profile "${{ env.AUDIT_PROFILE }}" \
      --grade "${{ env.AUDIT_GRADE }}"
```

## Environment variables

| Name | Required? | Purpose |
|------|-----------|---------|
| `SIN_GITHUB_INSTALLATION_TOKEN` | For real posting | Pre-generated GitHub App installation token (expires in 1h) |
| `GITHUB_TOKEN` | Fallback | Built-in CI token (works for most cases) |
| `SIN_GITHUB_APP_CLIENT_ID` | Optional | For OAuth flow (default: `Iv23livllaHIBTdQdyhY`) |
| `SIN_GITHUB_APP_CLIENT_SECRET` | For OAuth | Required only if using OAuth code exchange |

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Success (comment posted or updated) |
| 1 | Error (no score.json, no token, network failure, etc.) |

## Touched by

Anyone who wants to enable PR-comment-based ceo-audit reports. Backwards-compatible — falls back to `GITHUB_TOKEN` if no installation token is provided.
