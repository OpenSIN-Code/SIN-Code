# SIN Interceptor — Pre-Flight Architectural Rule Enforcement

## Purpose

ADW (Architectural Debt Watchdog) interceptor — runs regex rules over the
content of a tool call **before** the call is executed, and returns a
preflight verdict (`allowed` / `violations`). Prevents the agent from
writing code that violates the project's architectural rules.

The interceptor is **not** an LSP. It is a stateless, regex-based gate
that the orchestrator (or the agent itself) calls before
`sin_write`/`sin_edit`/`sin_ast_edit`/`sin_bash`.

## Default rules (hardcoded)

Three rules ship with every `SINInterceptor` instance:

| Rule name             | Pattern (regex, case-insensitive)                                                          | Severity |
|-----------------------|---------------------------------------------------------------------------------------------|----------|
| `no_frontend_db_direct` | `(import\|require).*(database\|db\|sql).*from.*(frontend\|ui\|component)`                 | error    |
| `no_hardcoded_secrets`  | `(password\|secret\|api_key\|token)\s*=\s*['"][^'"]+['"]`                                | error    |
| `no_eval_exec`          | `\b(eval\|exec\|subprocess\.shell=True)\b`                                                | warning  |

The first two block writes (severity=`error`); `no_eval_exec` only
warns (severity=`warning`) so the agent is informed but not blocked.

## Public API

### `InterceptorRule`

Single rule container.

- `__init__(name, pattern, message, severity="error")` — compiles
  `pattern` with `re.IGNORECASE`.
- `matches(content) -> bool` — `bool(self.pattern.search(content))`.

### `SINInterceptor`

- `__init__(repo_root=None)` — defaults to `Path.cwd()`. On init:
  1. Loads the 3 default rules.
  2. Tries to import `sin_code_adw.ADW` and merge its `get_active_rules()`
     output (graceful — exception = no ADW rules).
- `add_rule(name, pattern, message, severity="error")` — append a custom
  rule at runtime.
- `preflight(tool_name, tool_input) -> dict` — the main entry point.
  - Extracts content from `tool_input` based on `tool_name`:
    - `sin_write`/`sin_edit`/`sin_ast_edit` → `content` or
      `new_content` field
    - `sin_bash` → `command` field
    - everything else → returns `{"allowed": True, "violations": []}`
      immediately (no rules apply)
  - Runs every rule. If any matches, builds a `violations` list.
  - If at least one violation has `severity == "error"`, returns
    `{"allowed": False, "violations": [...], "system_reminder": "..."}`.
  - If only warnings, returns `{"allowed": True, "violations": [...],
    "system_reminder": null}`.
  - The `system_reminder` is a formatted Markdown block the agent
    pipeline can inject back into the LLM context (the `⚠️
    ARCHITECTURAL VIOLATION DETECTED` block).

## ADW integration (graceful)

The interceptor tries to pull additional rules from the `sin-code-adw`
subsystem at `__init__` time:

```python
try:
    from sin_code_adw import ADW
    adw = ADW(repo_root=self.repo_root)
    for rule in adw.get_active_rules():
        self.rules.append(InterceptorRule(...))
except Exception:
    pass
```

If `sin_code_adw` is missing or `get_active_rules()` raises, the
interceptor silently continues with just the 3 defaults. This is
intentional — the interceptor must work even on minimal installs.

## Usage example

```python
from sin_code_bundle.interceptor import SINInterceptor

ic = SINInterceptor()
verdict = ic.preflight(
    "sin_write",
    {"content": "import db from 'frontend/components'", "path": "x.ts"},
)
# {
#   "allowed": False,
#   "violations": [{
#     "rule": "no_frontend_db_direct",
#     "message": "Frontend components must not import database/SQL...",
#     "severity": "error",
#     "tool": "sin_write"
#   }],
#   "system_reminder": "⚠️ **ARCHITECTURAL VIOLATION DETECTED** ⚠️ ..."
# }
```

Also exposed via MCP: `sin_check_architecture(tool_name, tool_input)`
(see `mcp_server.py`).

## Known caveats

- Regex-based, not AST-based — patterns can be tricked by string
  concatenation, comments, or escapes. The interceptor is a *first
  line of defense*, not a security boundary.
- The `no_frontend_db_direct` rule uses `.` (any char) between segments,
  so multi-line imports spanning regex-relevant whitespace still match
  in practice.
- `system_reminder` is built only when at least one `error`-severity
  violation exists — warnings don't generate a reminder string.
