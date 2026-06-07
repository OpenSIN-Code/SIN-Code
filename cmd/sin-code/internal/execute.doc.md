# `execute.doc.md` — Safe Shell Execution Subcommand

Executes shell commands with safety checks, automatic secret redaction, timeout handling, and error analysis.

## What it does

- **Runs shell commands** via `/bin/sh -c` (macOS/Linux) or `cmd /c` (Windows) with configurable timeout.
- **Blocks dangerous commands** with a safety blacklist: `rm -rf /`, `mkfs`, `dd if=/dev/zero`, `curl | sh`, `chmod 000 /`, recursive `rm` on root/home, and fork bombs.
- **Redacts secrets** from stdout/stderr using regex patterns for API keys, tokens, passwords, AWS credentials, bearer tokens, and private keys.
- **Analyzes exit codes** with human-readable descriptions (e.g., 127 = "command not found", 137 = "killed (SIGKILL, likely OOM)").
- **Supports streaming mode** (`--stream`) for real-time output without buffering.
- **Persists nothing:** No log files or history are written; all output goes to stdout/stderr.

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `ExecuteCmd` into the root cobra command
- `cmd/sin-code/internal/execute_test.go` — unit tests for safety checks, redaction, and timeout
- `cmd/sin-code/internal/efm.go` — uses `exec.CommandContext` pattern for docker compose calls

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--command` | *(required)* | Shell command to execute |
| `--timeout` | `60` | Timeout in seconds (0 = no timeout) |
| `--format` | `text` | Output: `text` or `json` |
| `--stream` | `false` | Stream output in real-time instead of buffering |

- **Safety patterns:** 15+ hardcoded dangerous patterns. Recursive `rm` on root/home requires explicit confirmation (blocked by regex).
- **Secret redaction:** 9 regex patterns covering API keys, tokens, passwords, secrets, auth headers, AWS keys, and private keys. Minimum match length 8-20 chars depending on type.
- **Timeout exit code:** `124` (matches `timeout` utility convention).

## Usage examples

```bash
# Basic command with JSON output
sin-code execute --command "ls -la" --format json

# Long-running command with 5-minute timeout
sin-code execute --command "go test ./..." --timeout 300

# Stream output in real-time
sin-code execute --command "tail -f /var/log/app.log" --stream

# Command with potential secrets (auto-redacted)
sin-code execute --command "echo API_KEY=sk-1234567890abcdef"
```

## Known caveats / footguns

- **Safety is blacklist-based, not sandboxed:** It blocks known dangerous patterns but does NOT prevent all malicious commands. Never use on untrusted input.
- **Secret redaction is regex-based:** May miss novel secret formats or obfuscated values. It is a safety net, not a guarantee.
- **Streaming disables output capture:** In `--stream` mode, the result struct has empty stdout/stderr fields. Use non-streaming for programmatic parsing.
- **Timeout uses `context.WithTimeout`:** The process receives SIGKILL after timeout. Cleanup (temp files, locks) may be left behind.
- **Windows shell differences:** Uses `cmd /c` instead of `/bin/sh -c`. Some bash-isms will fail on Windows.