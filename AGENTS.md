# 🛠️ SIN-Code Tool Suite — IMMER UNSERE EIGENEN TOOLS NUTZEN (PRIORITY -10.0 - ABSOLUTE OBERSTE REGEL)

## ⚠️ ABSOLUTE REGEL: Niemals opencode-eigene Tools nutzen, wenn unsere SIN-Code Tools verfügbar sind!

**UNSERE 16 SIN-Code Tools sind BESSER als die opencode-eigenen Alternativen.** Jeder Agent MUSS unsere Tools nutzen, niemals die opencode-internen.

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

### Tool-Verweisung & Skills/MCP

**⚡ UNIFIED BINARY (v1.0.0+):** All 16 sin-code tools live in a single Go binary: `~/.local/bin/sin-code`.
The opencode.json registers ONE MCP server `sin-code` that exposes all 16 tools via the `serve` subcommand.

| Tool (MCP) | CLI | Subcommand | Purpose |
|------------|-----|------------|---------|
| `sin_discover` | `sin-code` | `discover` | Dateien suchen, Relevanz-Scoring |
| `sin_execute` | `sin-code` | `execute` | Befehle sicher ausführen |
| `sin_map` | `sin-code` | `map` | Architektur analysieren |
| `sin_grasp` | `sin-code` | `grasp` | Einzelne Datei verstehen |
| `sin_scout` | `sin-code` | `scout` | Code durchsuchen |
| `sin_harvest` | `sin-code` | `harvest` | URLs abrufen |
| `sin_orchestrate` | `sin-code` | `orchestrate` | Tasks managen |
| `sin_ibd` | `sin-code` | `ibd` | Intent-Based Diffing |
| `sin_poc` | `sin-code` | `poc` | Proof-of-Correctness |
| `sin_sckg` | `sin-code` | `sckg` | Semantic Codebase Knowledge Graphs |
| `sin_adw` | `sin-code` | `adw` | Architectural Debt Watchdogs |
| `sin_oracle` | `sin-code` | `oracle` | Verification Oracle |
| `sin_efm` | `sin-code` | `efm` | Ephemeral Full-Stack Mocking |
| `sin_security` | `sin-code` | `security` | Security-Scan (Go/Python/Node/Generic) |
| `sin_config` | `sin-code` | `config` | Konfiguration verwalten |
| `sin_tui` | `sin-code` | `tui` | Interaktives TUI Menu |

**Installiert:** `~/.local/bin/sin-code` (1 binary, 16 tools)
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

# SIN-Code-Bundle

Unified CLI and MCP server for the entire SIN-Code agent-engineering stack.

## Quick Start

```bash
# Install
pip install -e .

# Show status
sin status

# Bootstrap repo
sin bootstrap .

# Run SIN-Code Go tool
sin sin-code run discover --path . -pattern "**/*.py" -format json

# Generate AGENTS.md
sin sin-code agents-md --output AGENTS.md
```

## Features

- **Unified CLI**: Single `sin` command for all tools
- **MCP server**: Unified MCP server for all agents
- **Status detection**: Auto-detects installed subsystems
- **Graceful degradation**: Works with partial installations
- **Go tool integration**: Runs SIN-Code Go tools via `sin sin-code run`
- **AGENTS.md generator**: Generates tool suite documentation

## Links

- [GitHub](https://github.com/OpenSIN-Code/SIN-Code-Bundle)
- [SIN-Code-Discover-Tool](https://github.com/OpenSIN-Code/SIN-Code-Discover-Tool)
- [SIN-Code-Execute-Tool](https://github.com/OpenSIN-Code/SIN-Code-Execute-Tool)
- [SIN-Code-Map-Tool](https://github.com/OpenSIN-Code/SIN-Code-Map-Tool)
- [SIN-Code-Grasp-Tool](https://github.com/OpenSIN-Code/SIN-Code-Grasp-Tool)
- [SIN-Code-Scout-Tool](https://github.com/OpenSIN-Code/SIN-Code-Scout-Tool)
- [SIN-Code-Harvest-Tool](https://github.com/OpenSIN-Code/SIN-Code-Harvest-Tool)
- [SIN-Code-Orchestrate-Tool](https://github.com/OpenSIN-Code/SIN-Code-Orchestrate-Tool)
- [SIN-Brain](https://github.com/OpenSIN-Code/SIN-Brain)

## Version

v0.2.0

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
