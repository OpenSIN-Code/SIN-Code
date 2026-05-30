"""Generator fuer eine AGENTS.md (WS4, Issue #4).

OpenCode und Codex lesen automatisch eine ``AGENTS.md`` im Repo-Root. Dieser
Generator schreibt einen SIN-Code-Block, der dem Agenten erklaert, *wann*
welches SIN-Tool aufzurufen ist. Der Block ist zwischen Markern eingefasst und
wird idempotent ersetzt -- der restliche Inhalt der Datei bleibt unangetastet.
"""
from __future__ import annotations

from pathlib import Path

START_MARKER = "<!-- sin:start -->"
END_MARKER = "<!-- sin:end -->"

# Mapping: wann welches Tool. Bewusst knapp und handlungsorientiert.
_PLAYBOOK = [
    ("Before refactoring or deleting a symbol", "impact",
     "Get the blast radius (downstream dependents) before you change a shared symbol."),
    ("After producing a diff / before committing", "semantic_review",
     "Summarize the intent and risk of the change instead of eyeballing line diffs."),
    ("Before merging or marking a task done", "verify_tests",
     "Run independent, execution-based verification. Never trust a self-reported 'done'."),
    ("When you need correctness guarantees", "prove",
     "Generate and check properties/proofs for pure functions."),
    ("When code needs external services in tests", "mock_env",
     "Spin up an ephemeral full-stack mock environment, then tear it down."),
    ("To understand overall code health", "architectural_debt",
     "Check the current architectural debt score before large changes."),
]


def _build_block() -> str:
    """Baut den Inhalt zwischen den Markern (ohne die Marker selbst)."""
    lines = [
        "## SIN-Code Agent Tooling",
        "",
        "This repository is wired to the SIN-Code verification stack via MCP",
        "(`sin serve`). Use these tools proactively -- they are cheap signals that",
        "prevent shipping broken code.",
        "",
        "### When to call which tool",
        "",
        "| Situation | Tool | Why |",
        "| --- | --- | --- |",
    ]
    for situation, tool, why in _PLAYBOOK:
        lines.append(f"| {situation} | `{tool}` | {why} |")
    lines += [
        "",
        "### Rules",
        "",
        "1. **Do not claim success without `verify_tests`.** A green compile is not proof.",
        "2. **Run `impact` before touching shared code** to avoid silent breakage.",
        "3. **Prefer `semantic_review` over raw diffs** when assessing your own changes.",
        "4. If a tool is unavailable, continue gracefully and say so explicitly.",
    ]
    return "\n".join(lines)


def render_block() -> str:
    """Vollstaendiger, markierter SIN-Block (inkl. Marker)."""
    return f"{START_MARKER}\n{_build_block()}\n{END_MARKER}"


def render_full_document() -> str:
    """Eine komplette AGENTS.md fuer den Fall, dass noch keine existiert."""
    header = "# AGENTS.md\n\nGuidance for AI coding agents working in this repository.\n"
    return f"{header}\n{render_block()}\n"


def upsert(path: Path) -> str:
    """Schreibt/aktualisiert die AGENTS.md idempotent.

    - Datei fehlt        -> komplette Vorlage mit SIN-Block anlegen.
    - Datei ohne Block   -> SIN-Block am Ende anhaengen.
    - Datei mit Block    -> nur den Bereich zwischen den Markern ersetzen.

    Gibt eine kurze Statusmeldung zurueck.
    """
    block = render_block()
    if not path.exists():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(render_full_document())
        return f"Created {path} with SIN-Code block"

    content = path.read_text()
    if START_MARKER in content and END_MARKER in content:
        start = content.index(START_MARKER)
        end = content.index(END_MARKER) + len(END_MARKER)
        new_content = content[:start] + block + content[end:]
        if new_content == content:
            path.write_text(new_content)
            return f"{path} already up to date (no change)"
        path.write_text(new_content)
        return f"Updated SIN-Code block in {path}"

    # Block fehlt: anhaengen, mit sauberem Abstand.
    sep = "" if content.endswith("\n\n") else ("\n" if content.endswith("\n") else "\n\n")
    path.write_text(content + sep + block + "\n")
    return f"Appended SIN-Code block to {path}"
