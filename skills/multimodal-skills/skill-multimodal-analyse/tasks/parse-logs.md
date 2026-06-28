# Task: Parse Logs

Use this task when the user opens a log file (`.log`, `.jsonl`,
`.ndjson`) and asks the agent to find errors, signatures, anomalies,
or frequency patterns. Log files are text but their volume and
structure make them a multimodal artefact: the agent should never
read them line-by-line into context — it routes through
`analyse__log_analyze`.

## Inputs

- `path` — absolute path to the log file inside the workspace root
- Optional filters: `level`, `since`, `until`, `grep`

## Workflow

```
1. Verify extension in {.log, .jsonl, .ndjson}
2. Call analyse__log_analyze(path=..., level=error, since=...)
3. Surface errors + signatures as a ranked list
4. Pair with skill-code-ceo-audit when security/leaked-token patterns match
5. Optional: sin_edit a config that needs the offending pattern stripped
```

## Step-by-step

### Step 1 — Verify extension

If the file is `.log`, `.jsonl`, or `.ndjson`, skip detection and
call directly. For labelled `.txt` files that look like logs (line
prefixes with timestamps, JSON per line), call
`analyse__data_detect` first to confirm the structure.

### Step 2 — Call the tool

```bash
sin-code mcp call analyse__log_analyze \
  --path logs/production-2026-06-18.log \
  --level error \
  --since 2026-06-18T00:00:00Z \
  --json
```

Expected response:

```json
{
  "ok": true,
  "path": "logs/production-2026-06-18.log",
  "format": "jsonl",
  "lines": 18432,
  "errors": [
    {"ts": "2026-06-18T03:14:02Z", "level": "error", "msg": "ECONNREFUSED api.openai.com:443"},
    {"ts": "2026-06-18T03:14:11Z", "level": "error", "msg": "rate limit exceeded"}
  ],
  "signatures": [
    {"pattern": "ECONNREFUSED", "count": 14, "first": "...", "last": "..."},
    {"pattern": "rate limit", "count": 3, "first": "...", "last": "..."}
  ]
}
```

### Step 3 — Surface ranked

`signatures` is the **ranked** view: most frequent patterns first.
Lead the user-visible summary with the top-3 signatures and a count.
Do not paste raw error lines into the conversation; reference
`errors[].ts` and let the user pick a window to deep-dive.

### Step 4 — Pair with skill-code-ceo-audit

Certain log signatures are security-relevant:

| Pattern in `signatures` | Pair with |
|------------------------|-----------|
| `Bearer eyJ...` / `sk-...` / `ghp_...` | `skill-code-ceo-audit` secret scan |
| `MISSING_PERMISSION` / `403` spikes | `skill-code-ceo-audit` permission flow |
| `5xx` ≥ 1% of total | `skill-code-ceo-audit` reliability gate |

Always surface the pairing suggestion to the user; never auto-spawn
the audit (M4 — `ask` for destructive sweeps).

### Step 5 — Surgical edits

If the user wants to **fix** a recurring error pattern (e.g. add
retry logic), do not edit log files. Edit the code that emits the
errors. `sin_edit` against the source, not the log.

## Verification

- [ ] `analyse__log_analyze` returned `{ok: true, ...}`.
- [ ] Top-3 signatures surfaced with counts and time windows.
- [ ] Security-relevant signatures flagged with pairing hints.
- [ ] No raw error lines were pasted into the conversation; only
  referenced by `(ts, msg-prefix)`.
- [ ] If user requested a fix, edit target is the upstream code, not
  the log file.
