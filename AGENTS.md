# 🛠️ SIN-Code Tool Suite — IMMER UNSERE EIGENEN TOOLS NUTZEN (PRIORITY -10.0 - ABSOLUTE OBERSTE REGEL)

## ⚠️ ABSOLUTE REGEL: Niemals opencode-eigene Tools nutzen, wenn unsere SIN-Code Tools verfügbar sind!

**UNSERE 19 SIN-Code Subcommands (13 MCP + 6 CLI-only) sind BESSER als die opencode-eigenen Alternativen.** Jeder Agent MUSS unsere Tools nutzen, niemals die opencode-internen.

### Wann welches Tool?

| Aufgabe | SIN-Code Tool (NUTZEN!) | Opencode Alternative (NICHT nutzen!) | Warum unsere besser |
|---------|------------------------|-----------------------------------|-------------------|
| **Dateien suchen, Projekt-Struktur erkunden** | `sin-discover` | `opencode` interne Dateisuche | Pattern-Matching, Relevanz-Scoring, Dependency-Map, Gruppierung |
| **Befehle ausführen, Kommandos laufen lassen** | `sin-execute` | `opencode` interne Shell-Ausführung | Secret-Redaction, Safety-Checks, Timeout-Handling, Error-Analyse |
| **Architektur analysieren, Module mappen** | `sin-map` | `opencode` interne Code-Analyse | Module-Level, Entry-Points, Hot-Paths, Dependency-Graph, Orphan-Detection |
| **Einzelne Datei verstehen, Code analysieren** | `sin-grasp` | `opencode` interne Code-Analyse | Struktur, Dependencies, Usage, Context, Related-Files |
| **Code durchsuchen, Patterns finden** | `sin-scout` | `opencode` interne Suche | Regex, Semantic, Symbol, Usage-Search, Dead-Code-Detection |
| **URLs abrufen, APIs konsumieren** | `sin-harvest` | `opencode` interne HTTP-Requests | Caching, Struktur-Extraktion, Change-Detection, Auth-Management |
| **Tasks managen, Planung, Rollback** | `sin-orchestrate` | `opencode` interne Task-Planung | Dependencies, Parallel-Execution, Blocker-Detection, Rollback-Plan |

## ⚠️ DEPRECATION WARNING — `sin-code-bundle` (Python, legacy `sin` CLI)

> ⚠️ **DEPRECATED:** The `sin-code-bundle` MCP server (Python, old `sin` CLI) is DEPRECATED as of v1.1.0.
> Use `sin-code` (Go binary at `~/.local/bin/sin-code serve`) instead. The Go binary's tools are named
> `sin_discover`, `sin_execute`, `sin_map`, `sin_grasp`, `sin_scout`, `sin_harvest`, `sin_orchestrate`,
> `sin_ibd`, `sin_poc`, `sin_sckg`, `sin_adw`, `sin_oracle`, `sin_efm` — NOT `sin-code-bundle_*`.
>
> **Reason:** The legacy Python MCP server's tools (`sin-code-bundle_sin_edit`, `sin-code-bundle_sin_search`,
> etc.) have a longer `sin-code-bundle_` prefix and were winning tool-selection over the newer Go tools.
> The legacy server is now `enabled: false` in `opencode.json`. Re-enable only for rollback.

### Tool-Verweisung & Skills/MCP

**⚡ UNIFIED BINARY (v1.0.5+):** All 19 sin-code subcommands (13 MCP + 6 CLI-only) live in a single Go binary: `~/.local/bin/sin-code`.
The opencode.json registers ONE MCP server `sin-code` that exposes all 13 tools via the `serve` subcommand.
Note: 6 utility subcommands (config, sbom, security, self-update, tui, serve) are CLI-only, not exposed via MCP.

| Tool (MCP, **preferred — Go**) | Backend | Status | Purpose |
|------------------------------|---------|--------|---------|
| `sin_discover` ✅ | `sin-code` (Go) | ✅ Native | Dateien suchen, Relevanz-Scoring |
| `sin_execute` ✅ | `sin-code` (Go) | ✅ Native | Befehle sicher ausführen |
| `sin_map` ✅ | `sin-code` (Go) | ✅ Native | Architektur analysieren |
| `sin_grasp` ✅ | `sin-code` (Go) | ✅ Native | Einzelne Datei verstehen |
| `sin_scout` ✅ | `sin-code` (Go) | ✅ Native | Code durchsuchen |
| `sin_harvest` ✅ | `sin-code` (Go) | ✅ Native | URLs abrufen |
| `sin_orchestrate` ✅ | `sin-code` (Go) | ✅ Native | Tasks managen |
| `sin_ibd` ✅ | `sin-code` (Go) | ✅ Native | Intent-Based Diffing |
| `sin_poc` ✅ | `sin-code` (Go) | ✅ Native | Proof-of-Correctness |
| `sin_sckg` ✅ | `sin-code` (Go) | ✅ Native | Semantic Codebase Knowledge Graphs |
| `sin_adw` ✅ | `sin-code` (Go) | ✅ Native | Architectural Debt Watchdogs |
| `sin_oracle` ✅ | `sin-code` (Go) | ✅ Native | Verification Oracle |
| `sin_efm` ✅ | `sin-code` (Go) | ✅ Native | Ephemeral Full-Stack Mocking (auto: OrbStack on macOS, Docker on Linux; `--runtime orb|docker|auto` to override) |
| `sin-code-bundle_sin_edit` ⚠️ | `sin-code-bundle` (Python, **DEPRECATED**) | ⚠️ Legacy | Hashline-anchored edit only — disabled by default |
| `sin-code-bundle_sin_read` 🚫 | `sin-code-bundle` (Python, **DEPRECATED**) | 🚫 Do not use | Use `read` URI schemes instead (sckg://, oracle://, etc.) |
| `sin-code-bundle_sin_write` 🚫 | `sin-code-bundle` (Python, **DEPRECATED**) | 🚫 Do not use | Use `sin_discover` + native editors |
| `sin-code-bundle_sin_search` 🚫 | `sin-code-bundle` (Python, **DEPRECATED**) | 🚫 Do not use | Use `sin_scout` instead |
| `sin-code-bundle_sin_bash` 🚫 | `sin-code-bundle` (Python, **DEPRECATED**) | 🚫 Do not use | Use `sin_execute` instead |
| `sin_security` ✅ | `sin-code` (Go) | ✅ Native | Security-Scan (Go/Python/Node/Generic) — CLI-only |
| `sin_config` ✅ | `sin-code` (Go) | ✅ Native | Konfiguration verwalten — CLI-only |
| `sin_tui` ✅ | `sin-code` (Go) | ✅ Native | Interaktives TUI Menu — CLI-only |
| `sin_sbom` ✅ | `sin-code` (Go) | ✅ Native | SBOM Generation (SPDX/CycloneDX) — CLI-only |
| — | `sin-code` (Go) | ✅ Native | `self-update` — Update to latest release — CLI-only |
| — | `sin-code` (Go) | ✅ Native | `serve` — Start MCP server (13 tools) — CLI-only |


**Installiert:** `~/.local/bin/sin-code` (1 binary, 19 subcommands: 13 MCP + 6 CLI-only)
**Repo:** `OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code`
| `sin-websearch` | `sin-websearch` | `OpenSIN-Code/SIN-Code-Websearch-Skill` | `sin-websearch` | ✅ `~/.config/opencode/skills/sin-websearch` |
| `sin-scheduler` | `sin-scheduler` | `OpenSIN-Code/SIN-Code-Scheduler-Skill` | `sin-scheduler` | ✅ `~/.config/opencode/skills/sin-scheduler` |
| `sin-marketplace` | `sin-marketplace` | `OpenSIN-Code/SIN-Code-Marketplace-Skill` | `sin-marketplace` | ✅ `~/.config/opencode/skills/sin-marketplace` |
| `sin-slash` | `sin-slash` | `OpenSIN-Code/SIN-Code-Slash-Skill` | `sin-slash` | ✅ `~/.config/opencode/skills/sin-slash` |
| `sin-goal-mode` | `sin-goal-mode` | `OpenSIN-Code/SIN-Code-Goal-Mode-Skill` | `sin-goal-mode` | ✅ `~/.config/opencode/skills/sin-goal-mode` |

### Anwendungsbeispiele

**1. Neues Projekt erkunden:**
```bash
# NIEMALS opencode-interne Dateisuche nutzen!
sin-code discover --path /Users/jeremy/dev/NEUES-PROJEKT --pattern "**/*.py" --sort_by relevance --format json
# Ergebnis: Alle Python-Dateien absteigend nach Relevanz sortiert, mit Dependencies und Related-Files
```

**2. Befehle sicher ausführen:**
```bash
# NIEMALS opencode-interne Shell-Ausführung nutzen!
sin-code execute --command "npm test" --timeout 60 --format json
# Ergebnis: Safety-Check, Secret-Redaction, Error-Analyse, Timeout-Handling
```

**3. Architektur verstehen:**
```bash
# NIEMALS opencode-interne Code-Analyse nutzen!
sin-code map --path /Users/jeremy/dev/PROJEKT --action map --format json
# Ergebnis: Module, Entry-Points, Hot-Paths, Dependency-Graph, Orphan-Detection, Complexity
```

**4. Code durchsuchen:**
```bash
# NIEMALS opencode-interne Suche nutzen!
sin-code scout --query "func.*main" --path /Users/jeremy/dev/PROJEKT --search_type regex --format json
# Ergebnis: Alle Main-Funktionen, mit Usage-Count, Architecture-Info, Context
```

**5. API/URL abrufen:**
```bash
# NIEMALS opencode-interne HTTP-Requests nutzen!
sin-code harvest --url "https://api.example.com/data" --format json
# Ergebnis: Status, Body, Headers, Struktur-Extraktion, Caching
```

**6. Tasks planen & tracken:**
```bash
# NIEMALS opencode-interne Task-Planung nutzen!
sin-code orchestrate --action add --title "Feature implementieren" --tags "urgent" --format json
# Ergebnis: Task-ID, Dependencies, Parallel-Execution-Plan, Rollback-Plan
```

**7. SBOM generieren:**
```bash
# NIEMALS opencode-interne SBOM-Tools nutzen!
sin-code sbom --path /Users/jeremy/dev/PROJEKT --format spdx-json --output sbom.json
# Ergebnis: SPDX- oder CycloneDX-konforme SBOM mit allen Dependencies
```

### Regeln

- **SOFORTIGER PERMANENTER BAN** bei Nutzung von opencode-internen Tools statt unserer SIN-Code Tools
- **VOR jedem Coding-Task:** Erst mit `sin-discover` das Projekt erkunden, dann mit `sin-grasp` die relevante Datei verstehen
- **VOR jedem Refactoring:** Erst mit `sin-map` die Architektur analysieren, dann mit `sin-scout` alle Referenzen finden
- **VOR jedem Befehl:** Immer `sin-execute` mit Safety-Check und Secret-Redaction nutzen
- **VOR jedem API-Call:** Immer `sin-harvest` mit Caching und Struktur-Extraktion nutzen
- **Für jede Planung:** Immer `sin-orchestrate` mit Dependencies und Rollback-Plan nutzen

### Versionen (aktuell)

| Tool | Version | Status |
|------|---------|--------|
| sin-discover | v0.2.5-fixes | ✅ Stable |
| sin-execute | v0.2.4-fixes | ✅ Stable |
| sin-map | v0.2.5-fixes | ✅ Stable |
| sin-grasp | v0.2.4-fixes | ✅ Stable |
| sin-scout | v0.1.5-fixes | ✅ Stable |
| sin-harvest | v0.1.4-fixes | ✅ Stable |
| sin-orchestrate | v0.1.6-fixes | ✅ Stable |

---

# 🐳 CONTAINER MANDATE — OrbStack on macOS (PRIORITY -5.0)

**STRICT RULE:** On macOS, ALWAYS use **OrbStack** (`orb`) for container operations. NEVER use `docker` directly.

- **OrbStack binary:** `orb` (Docker CLI-compatible, located at `/opt/homebrew/bin/orb`)
- **Docker binary:** `docker` (installed but FLAKY — crashes on Mac, must NOT be used)
- **Rule:** When writing scripts, MCP tools, CI configs, or any container operations targeting macOS, use `orb` instead of `docker`
- **Why:** Docker Desktop on Mac is unstable and crashes. OrbStack uses native macOS virtualization and is fully Docker CLI-compatible
- **Cross-platform:** On Linux CI (GitHub Actions), regular `docker` is fine. The rule only applies to macOS dev environments
- **How to detect:** Use `uname` to check OS. If Darwin/macOS → use `orb`. If Linux → use `docker`
- **EFM (Ephemeral Full-Stack Mocking) in sin-code:** Already supports both. On Mac, OrbStack is the default runtime

---

# 📚 CoDocs Standard — Code Documentation (PRIORITY -9.0)

Every meaningful code file needs **two documentation layers**:

1. **`.doc.md` companion** — the "what and why" overview
2. **Inline `#` comments** — the "how and why here" detail in the code itself

Both layers must exist. Neither replaces the other.

---

## Layer 1: CoDocs (.doc.md companion)

Every code file gets a `.doc.md` companion file in the same directory.

### Naming

```
router.py         → router.doc.md
config.yaml       → config.doc.md
api/types.ts      → api/types.doc.md
Makefile          → Makefile.doc.md
```

### Code reference

First line of the code file:

```python
# Docs: router.doc.md
```

```ts
// Docs: types.doc.md
```

```makefile
# Docs: Makefile.doc.md
```

### What belongs in a `.doc.md`

- What does this file do? (1 sentence)
- Which other files import / touch it? (dependency map)
- Important config values & limits
- Why certain decisions were made (e.g. "no async here because X")
- Usage examples (1-2 lines)
- Known caveats or footguns

### What does NOT belong in a `.doc.md`

- Implementation details (inline comments handle that)
- Git history (that's what `git log` is for)

---

## Layer 2: SOTA Inline Documentation

Every code file must also have professional inline `#`/`//`/`#:` comments. This is **not** about "comment every line" — it is about providing **semantic context** that an agent can't infer from the code alone.

### SOTA Inline Doc Rules

#### 1. File header (mandatory)

Every code file starts with:

```
# Purpose: <what this file does in 1 line>
# Docs: <companion .doc.md path>
```

For Python use `"""` module docstrings instead of `#`:

```python
"""Handle user authentication.

Docs: auth.doc.md
"""
```

For TypeScript/Rust/etc use doc-comment style:

```ts
/**
 * Handle user authentication.
 * Docs: auth.doc.md
 */
```

#### 2. Public API: docstrings (mandatory)

Every public function, method, class, type, and constant needs a docstring:

```python
def calculate_route(
    origin: Coordinate,
    dest: Coordinate,
    traffic: bool = False,
) -> Route:
    """Shortest path between two coordinates.

    Uses A* with Manhattan heuristic. Raises if both coords
    are identical (avoids zero-length route).
    """
    ...
```

```ts
/** Shortest path between two coordinates.
 *
 * Uses A* with Manhattan heuristic. Throws if both coords
 * are identical (avoids zero-length route).
 */
function calculateRoute(
  origin: Coordinate,
  dest: Coordinate,
  traffic: boolean = false,
): Route { ... }
```

#### 3. Non-obvious logic: inline context comments

Add a comment when the code does something surprising:

- **Why NOT the obvious approach**: `# not using dict comprehension because ...`
- **Why this value**: `# 50ms timeout — must be < retry-after of upstream (60ms)`
- **Why this ordering**: `# flush before close — close may skip unflushed data`
- **Edge case**: `# handles None because protocol allows null fields`
- **Performance note**: `# O(n²) but n ≤ 10 in practice`
- **Security note**: `# sanitize_input() prevents SQL injection here`

#### 4. Section separators (recommended for 100+ line files)

```
# ── Auth ──────────────────────────────────────
```

Visually group related blocks. The long line makes sections scannable.

#### 5. Magic values & config keys

Always explain:

```python
MAX_RETRIES = 3    # upstream SLA guarantees < 2 failures per 1000
WAIT_SECONDS = 60  # must match upstream rate-limit window
```

```ts
const MAX_RETRIES = 3   // upstream SLA guarantees < 2 per 1000
const WAIT_SECONDS = 60 // must match upstream rate-limit window
```

#### 6. Tests: describe scenario + expected behavior

```python
def test_retry_exhaustion():
    """After 3 retries, route should raise UpstreamError."""
```

Test names plus docstrings = executable documentation.

#### 7. Deprecation & migration markers

```python
def old_login():  # DEPRECATED(v2): use authenticate() instead
```

### When to update inline docs

- **Every change to a function's signature**: update its docstring
- **Every change to non-obvious logic**: add/update the context comment
- **Every new module**: file header + section separators
- **Every new public API**: docstring on add

### When NOT to comment

- `i += 1` — obvious code needs no comment
- `x = 1` — unless 1 is a meaningful constant
- Getter/setter boilerplate
- Standard library calls with obvious semantics

---

## Validation

After changes, verify with the bundle CLI:

```bash
sin codocs check            # exit 1 if any .doc.md reference is broken
sin codocs check --json     # machine-readable output
sin codocs list             # list every reference and whether it resolves
```

For inline docs, use manual review with:

```bash
# Check files that have NO module-level docstring/Purpose line
python3 -c "
import ast, sys
for f in sys.argv[1:]:
    try:
        tree = ast.parse(open(f).read())
        if not (isinstance(tree.body[0], ast.Expr) and hasattr(tree.body[0].value, 'value') and 'Purpose' in tree.body[0].value.s if hasattr(tree.body[0].value, 's') else isinstance(tree.body[0], ast.Expr) and isinstance(tree.body[0].value, ast.Constant)):
            print(f'MISSING PURPOSE: {f}')
    except: print(f'PARSE ERROR: {f}')
"
```

## Exceptions

- `docs/` folder — architecture docs, ADRs, setup guides
- `README.md` — project overview
- No `.doc.md` for pure config files without logic (`.gitignore`, `.prettierrc`, etc.)
- No inline docs required for throwaway scripts in `debug/`, `tmp/`, experimental branches

---

## MarkItDown Integration (Microsoft)

**Converts everything to Markdown** for LLM consumption: PDF, DOCX, PPTX, XLSX,
Images (OCR), Audio, HTML, CSV/JSON/XML, ZIP, YouTube, EPUB, Outlook MSG.

### Installation

```bash
pipx install markitdown                          # recommended (CLI + library)
pip install 'markitdown[pdf, docx, pptx, xlsx]'  # minimal
pip install 'markitdown[all]'                    # everything
```

### CLI

```bash
markitdown file.pdf > file.md                     # stdout
markitdown file.pdf -o file.md                    # output file
cat file.pdf | markitdown                         # pipe
markitdown --use-plugins file.pdf                 # with plugins (OCR)
markitdown file.pdf --use-cu --cu-endpoint "<e>"  # Azure Content Understanding
```

### Python API

```python
from markitdown import MarkItDown
md = MarkItDown()
result = md.convert("document.pdf")
print(result.text_content)

# With LLM vision (image descriptions in PPTX/Images)
from openai import OpenAI
md = MarkItDown(llm_client=OpenAI(), llm_model="gpt-4o")

# Security: local files only
result = md.convert_local("document.pdf")
```

### CoDocs pipeline

```bash
for f in docs/*.pdf docs/*.docx docs/*.pptx; do
    markitdown "$f" -o "${f%.*}.doc.md"
done
# then add `# Docs: filename.doc.md` to the matching code file
```

### Security

- `convert()` runs with the calling process's full file-IO rights. Never pass
  untrusted input directly.
- Prefer `convert_local()` / `convert_stream()` for controlled access.

### Reference

https://github.com/microsoft/markitdown | `pipx install markitdown`

---

# SIN-Code-Execute-Tool

Safe command execution with timeout, output capture, safety checks, and error analysis.

## Quick Start

```bash
# Build and install
go build -o ~/.local/bin/execute ./cmd/execute

# Run a command
execute -command "echo hello" -format json

# With timeout
execute -command "sleep 5" -timeout 2 -format json

# Redact secrets
execute -command "echo API_KEY=secret123" -format json
```

## Features

- **Safety checks**: Blocks dangerous commands (rm -rf /, etc.)
- **Secret redaction**: Automatically redacts API keys, tokens, passwords
- **Environment variable redaction**: Redacts USER, HOME, etc.
- **Timeout handling**: Kills processes after timeout, shows [TIMEOUT]
- **Error analysis**: Analyzes exit codes and provides suggestions
- **Persistent history**: Saves execution history to ~/.local/state/sinator/
- **Signal handling**: Handles SIGKILL, SIGTERM, SIGINT, SIGSEGV

## Links

- [GitHub](https://github.com/OpenSIN-Code/SIN-Code-Execute-Tool)
- [SIN-Code-Bundle](https://github.com/OpenSIN-Code/SIN-Code-Bundle)
- [SIN-Brain](https://github.com/OpenSIN-Code/SIN-Brain)

## Version

v0.2.4-fixes

---

# SIN-Code-Bundle (LEGACY — superseded by `sin-code` Go binary)

> ⚠️ **DEPRECATED as of v1.1.0.** The Python `sin` CLI / `sin-code-bundle` MCP server is **superseded**
> by the unified Go binary `sin-code` (`~/.local/bin/sin-code serve`). The Python repo lives on for
> historical reference and as a fallback for users who cannot install the Go toolchain. Do not start a
> new project on the Python stack.

The Python implementation that originally unified the SIN-Code agent-engineering stack. The active stack
is now the **Go binary** documented in the **SIN-Code Tool Suite** section above.

## Migration

| Old (Python, deprecated) | New (Go, current) |
|--------------------------|-------------------|
| `pip install -e .` (in `SIN-Code-Bundle/`) | `go install` or download release of `sin-code` |
| `sin status` | `sin-code tui` (or just `sin-code --help`) |
| `sin bootstrap .` | `sin-code adw` (Architectural Debt Watchdogs) |
| `sin sin-code run discover --path . ...` | `sin-code discover --path . ...` |
| `sin serve` (launches the Python MCP) | `sin-code serve` (launches the Go MCP, 13 tools) |
| `sin sin-code agents-md --output AGENTS.md` | Not needed — the canonical AGENTS.md ships in this repo |

## Why the Go binary won

- **Single binary** — no Python venv to maintain, no pip resolution, no platform issues
- **13 MCP tools in one server** — replaces the Python MCP's tool sprawl with a clean, discoverable set
- **No tool-shadowing** — Go tools have short, consistent names (`sin_discover`, `sin_execute`, …) that
  do not collide with the deprecated `sin-code-bundle_*` prefix
- **Atomic, AST-verified writes** — `sin-code-bundle_sin_edit` survives line shifts; for everything else
  the Go binary uses standard `read`/`write` URI schemes (`sckg://`, `oracle://`, etc.)

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **SIN-Code-Bundle** (9997 symbols, 17832 relationships, 273 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/SIN-Code-Bundle/context` | Codebase overview, check index freshness |
| `gitnexus://repo/SIN-Code-Bundle/clusters` | All functional areas |
| `gitnexus://repo/SIN-Code-Bundle/processes` | All execution flows |
| `gitnexus://repo/SIN-Code-Bundle/process/{name}` | Step-by-step execution trace |

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->

---

## Documentation

- [CHANGELOG.md](CHANGELOG.md) — version history and release notes
- [docs/](/docs) — architectural documentation and design decisions
- CoDocs companion files (`.doc.md`) exist alongside every significant code file
