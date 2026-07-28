# SPDX-License-Identifier: MIT
"""SIN-Code Go Tools sub-commands — extracted from cli.py."""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import typer

from sin_code_bundle.cli_app import sin_code_app
from sin_code_bundle.cli_forward import _SIN_CODE_TOOLS, _sin_code_tool_path


@sin_code_app.command("run")
def sin_code_run(
    tool: str = typer.Argument(
        ...,
        help="Tool name: discover, execute, map, grasp, scout, harvest, orchestrate, ibd, poc, sckg, adw, oracle, efm",
    ),
    args: list[str] = typer.Argument(default_factory=list, help="Arguments to pass to the tool"),
):
    """Run a SIN-Code Go tool with the given arguments.

    Uses the unified `sin-code` Go binary (v1.0.0+) which consolidates 13 tools
    into a single cobra-based CLI. Falls back to standalone binaries if the
    unified binary is not installed.
    """
    if tool not in _SIN_CODE_TOOLS:
        typer.echo(
            f"[SIN-CODE] Unknown tool: {tool}. Available: {', '.join(_SIN_CODE_TOOLS.keys())}",
            err=True,
        )
        raise typer.Exit(code=1)

    # Prefer the unified sin-code binary (single binary, 13 subcommands)
    unified = shutil.which("sin-code")
    if unified:
        result = subprocess.run([unified, tool, *args], capture_output=True, text=True)
        if result.stdout:
            typer.echo(result.stdout)
        if result.stderr:
            typer.echo(result.stderr, err=True)
        raise typer.Exit(code=result.returncode)

    # Fallback: standalone binary (legacy, kept for backwards compat)
    path = _sin_code_tool_path(tool)
    if not path:
        typer.echo(
            f"[SIN-CODE] Tool '{tool}' not found. Install: go install github.com/OpenSIN-Code/SIN-Code/cmd/sin-code@latest",
            err=True,
        )
        raise typer.Exit(code=1)

    result = subprocess.run([str(path)] + args, capture_output=True, text=True)
    if result.stdout:
        typer.echo(result.stdout)
    if result.stderr:
        typer.echo(result.stderr, err=True)
    raise typer.Exit(code=result.returncode)


@sin_code_app.command("agents-md")
def sin_code_agents_md(
    output: Path = typer.Option(Path("AGENTS.md"), help="Output file path"),
):
    """Generate AGENTS.md with SIN-Code Tool Suite rules for all agents."""
    content = """# 🛠️ SIN-Code Tool Suite — IMMER UNSERE EIGENEN TOOLS NUTZEN (PRIORITY -10.0 - ABSOLUTE OBERSTE REGEL)

## ⚠️ ABSOLUTE REGEL: Niemals opencode-eigene Tools nutzen, wenn unsere SIN-Code Tools verfügbar sind!

**UNSERE 7 SIN-Code Tools sind BESSER als die opencode-eigenen Alternativen.** Jeder Agent MUSS unsere Tools nutzen, niemals die opencode-internen.

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

| Tool | MCP Name | GitHub Repo | Skill | Installiert |
|------|----------|-------------|-------|-------------|
| `sin-discover` | `sin-discover` | `OpenSIN-Code/SIN-Code-Discover-Tool` | `sin-discover` | ✅ `~/.local/bin/discover` |
| `sin-execute` | `sin-execute` | `OpenSIN-Code/SIN-Code-Execute-Tool` | `sin-execute` | ✅ `~/.local/bin/execute` |
| `sin-map` | `sin-map` | `OpenSIN-Code/SIN-Code-Map-Tool` | `sin-map` | ✅ `~/.local/bin/map` |
| `sin-grasp` | `sin-grasp` | `OpenSIN-Code/SIN-Code-Grasp-Tool` | `sin-grasp` | ✅ `~/.local/bin/grasp` |
| `sin-scout` | `sin-scout` | `OpenSIN-Code/SIN-Code-Scout-Tool` | `sin-scout` | ✅ `~/.local/bin/scout` |
| `sin-harvest` | `sin-harvest` | `OpenSIN-Code/SIN-Code-Harvest-Tool` | `sin-harvest` | ✅ `~/.local/bin/harvest` |
| `sin-orchestrate` | `sin-orchestrate` | `OpenSIN-Code/SIN-Code-Orchestrate-Tool` | `sin-orchestrate` | ✅ `~/.local/bin/orchestrate` |

### Anwendungsbeispiele

**1. Neues Projekt erkunden:**
```bash
# NIEMALS opencode-interne Dateisuche nutzen!
/Users/jeremy/.local/bin/discover -path /Users/jeremy/dev/NEUES-PROJEKT -pattern "**/*.py" -sort_by relevance -format json
# Ergebnis: Alle Python-Dateien absteigend nach Relevanz sortiert, mit Dependencies und Related-Files
```

**2. Befehle sicher ausführen:**
```bash
# NIEMALS opencode-interne Shell-Ausführung nutzen!
/Users/jeremy/.local/bin/execute -command "npm test" -timeout 60 -format json
# Ergebnis: Safety-Check, Secret-Redaction, Error-Analyse, Timeout-Handling
```

**3. Architektur verstehen:**
```bash
# NIEMALS opencode-interne Code-Analyse nutzen!
/Users/jeremy/.local/bin/map -path /Users/jeremy/dev/PROJEKT -action map -format json
# Ergebnis: Module, Entry-Points, Hot-Paths, Dependency-Graph, Orphan-Detection, Complexity
```

**4. Code durchsuchen:**
```bash
# NIEMALS opencode-interne Suche nutzen!
/Users/jeremy/.local/bin/scout -query "func.*main" -path /Users/jeremy/dev/PROJEKT -search_type regex -format json
# Ergebnis: Alle Main-Funktionen, mit Usage-Count, Architecture-Info, Context
```

**5. API/URL abrufen:**
```bash
# NIEMALS opencode-interne HTTP-Requests nutzen!
/Users/jeremy/.local/bin/harvest -url "https://api.example.com/data" -format json
# Ergebnis: Status, Body, Headers, Struktur-Extraktion, Caching
```

**6. Tasks planen & tracken:**
```bash
# NIEMALS opencode-interne Task-Planung nutzen!
/Users/jeremy/.local/bin/orchestrate -action add -title "Feature implementieren" -tags "urgent" -format json
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

"""
    output.write_text(content, encoding="utf-8")
    typer.echo(f"[SIN-CODE] Generated {output}")
