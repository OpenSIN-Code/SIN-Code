# `eval-n8n` workflow

## Purpose

Run the first-party eval golden datasets on the n8n-managed OCI VM.
This workflow delegates to n8n and only runs a local `go vet` pre-check
(mandate M1).

Default datasets:

- `evals/critical.json`
- `evals/test-generation.json`
- `evals/mutation.json`
- `evals/fuzzing.json`
- `evals/property.json`
- `evals/quality-gate.json`

## Trigger

- Push to `main` affecting `evals/` or eval-related Go code.
- Pull requests touching the same paths.
- Manual dispatch via `workflow_dispatch` with optional `datasets` and
  `min_pass_rate` inputs.

## Required secrets

- `N8N_CI_WEBHOOK_URL` — the n8n webhook that receives the delegation payload.

## Webhook payload

```json
{
  "workflow": "eval-n8n",
  "ref": "refs/heads/main",
  "sha": "<commit-sha>",
  "repo": "OpenSIN-Code/SIN-Code",
  "actor": "<github-user>",
  "datasets": "evals/critical.json,evals/test-generation.json,evals/mutation.json,evals/fuzzing.json,evals/property.json,evals/quality-gate.json",
  "min_pass_rate": "0.8"
}
```

## n8n side

The n8n workflow should:
1. Clone the repo at the given SHA.
2. Build `sin-code`.
3. For each dataset, run `sin-code eval run --dataset <dataset> --min-pass-rate <min_pass_rate> --json`.
4. Report the aggregated result back to GitHub via checks API or PR comment.
