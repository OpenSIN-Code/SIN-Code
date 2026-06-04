# SPDX-License-Identifier: MIT
"""Generator fuer eine AGENTS.md (WS4, Issue #4).

OpenCode und Codex lesen automatisch eine ``AGENTS.md`` im Repo-Root. Dieser
Generator schreibt einen SIN-Code-Block, der dem Agenten erklaert, *wann*
welches SIN-Tool aufzurufen ist. Der Block ist zwischen Markern eingefasst und
wird idempotent ersetzt -- der restliche Inhalt der Datei bleibt unangetastet.

Docs: agents_md.doc.md
"""

from __future__ import annotations

from pathlib import Path

START_MARKER = "<!-- sin:start -->"
END_MARKER = "<!-- sin:end -->"

# ── Playbooks (situation → tool) ───────────────────────────────────────────
# Mapping: wann welches Tool. Bewusst knapp und handlungsorientiert.
_PLAYBOOK = [
    (
        "Before refactoring or deleting a symbol",
        "impact",
        "Get the blast radius (downstream dependents) before you change a shared symbol.",
    ),
    (
        "After producing a diff / before committing",
        "semantic_review",
        "Summarize the intent and risk of the change instead of eyeballing line diffs.",
    ),
    (
        "Before merging or marking a task done",
        "verify_tests",
        "Run independent, execution-based verification. Never trust a self-reported 'done'.",
    ),
    (
        "When you need correctness guarantees",
        "prove",
        "Generate and check properties/proofs for pure functions.",
    ),
    (
        "When code needs external services in tests",
        "mock_env",
        "Spin up an ephemeral full-stack mock environment, then tear it down.",
    ),
    (
        "To understand overall code health",
        "architectural_debt",
        "Check the current architectural debt score before large changes.",
    ),
]

# Memory playbook — only surfaced when SIN-Brain is installed (BR-2, Issue #15).
_MEMORY_PLAYBOOK = [
    (
        "Before starting a task",
        "recall",
        "Pull prior decisions, conventions and known pitfalls for this area first.",
    ),
    (
        "After making a non-obvious decision",
        "remember",
        "Persist the decision/convention/fix so future turns do not relitigate it.",
    ),
    (
        "When a subsystem returns a verdict",
        "link_evidence",
        "Attach the oracle/poc/ibd/sckg/adw verdict to the affected code entity.",
    ),
    (
        "When a memory is stale or wrong",
        "forget",
        "Remove outdated memories; `pin` the ones that must never be evicted.",
    ),
]


# Red-zones: hard "do not" constraints. Negative constraints reliably steer
# agents away from anti-patterns far better than positive guidance alone.
_NEGATIVE_CONSTRAINTS = [
    "Do **not** mark a task done or merge without a passing `verify_tests` run.",
    "Do **not** refactor or delete a shared symbol before checking `impact`.",
    "Do **not** invent file paths, APIs or symbols — confirm via `recall` / graph context.",
    "Do **not** discard a memory verdict to make a change look clean.",
    "Do **not** weaken or delete tests to get them to pass.",
    "Do **not** commit secrets, tokens or credentials.",
]


# ── Block Builder + Public Renderers ───────────────────────────────────────
def _build_block(memory_available: bool = False, inject_text: str = "") -> str:
    """Baut den Inhalt zwischen den Markern (ohne die Marker selbst).

    ``memory_available`` schaltet die Memory-Playbook-Zeilen frei; ``inject_text``
    ist der von SIN-Brain gelieferte Kontext-Block (SB-4), der unveraendert
    eingebettet wird.
    """
    lines = [
        "## SIN-Code Agent Tooling",
        "",
        "This repository is wired to the SIN-Code verification stack via MCP",
        "(`sin serve`). Use these tools proactively -- they are cheap signals that",
        "prevent shipping broken code.",
        "",
        "### SIN-Brain Memory Protocol",
        "",
        "The project uses a 4-tier memory system:",
        "- **Core**: Critical conventions (always recalled)",
        "- **Recall**: Recent context (last 7 days)",
        "- **Episodic**: Task history (last 30 days)",
        "- **Consolidated**: Long-term knowledge (summaries)",
        "",
        "Call `recall` before starting work. Call `remember` after completing.",
        "",
        "### Memory Rules",
        "",
        "1. **Always recall first.** Check for existing context.",
        "2. **Remember conventions.** Store patterns that caused bugs.",
        "3. **Pin critical rules.** Use `pin` for must-remember items.",
        "4. **Link evidence.** Connect related memories with `link_evidence`.",
        "",
        "### When to call which tool",
        "",
        "| Situation | Tool | Why |",
        "| --- | --- | --- |",
    ]
    playbook = list(_PLAYBOOK)
    if memory_available:
        playbook += _MEMORY_PLAYBOOK
    for situation, tool, why in playbook:
        lines.append(f"| {situation} | `{tool}` | {why} |")

    lines += [
        "",
        "### Rules",
        "",
        "1. **Do not claim success without `verify_tests`.** A green compile is not proof.",
        "2. **Run `impact` before touching shared code** to avoid silent breakage.",
        "3. **Prefer `semantic_review` over raw diffs** when assessing your own changes.",
        "4. If a tool is unavailable, continue gracefully and say so explicitly.",
        "5. **Recall before you code.** Always check SIN-Brain for relevant context.",
        "6. **Remember after you learn.** Store conventions and pitfalls in memory.",
    ]
    if memory_available:
        lines += [
            "5. **Start with `recall` and persist decisions with `remember`** so the",
            "   project's memory compounds across sessions.",
        ]

    lines += [
        "",
        "### Negative constraints (red-zones)",
        "",
    ]
    lines += [f"- {c}" for c in _NEGATIVE_CONSTRAINTS]

    if inject_text.strip():
        lines += [
            "",
            "### Project memory (SIN-Brain)",
            "",
            "<!-- Injected by `sin agents-md` from SIN-Brain; regenerated each run. -->",
            inject_text.strip(),
        ]

    return "\n".join(lines)


def _memory_context() -> tuple[bool, str]:
    """Return (sin-brain available?, inject text) from the memory adapter.

    Defensive: any failure degrades to (False, "") so `sin agents-md` always
    produces a valid file even without SIN-Brain installed.
    """
    try:
        from sin_code_bundle import memory

        env = memory.detect_env()
        if not env.available:
            return False, ""
        inject = ""
        getter = getattr(memory, "inject", None)
        if callable(getter):
            try:
                inject = getter() or ""
            except Exception:  # noqa: BLE001 - inject must never break generation
                inject = ""
        return True, inject
    except Exception:  # noqa: BLE001
        return False, ""


def render_block(memory_available: bool | None = None, inject_text: str | None = None) -> str:
    """Vollstaendiger, markierter SIN-Block (inkl. Marker)."""
    if memory_available is None or inject_text is None:
        detected_available, detected_inject = _memory_context()
        memory_available = detected_available if memory_available is None else memory_available
        inject_text = detected_inject if inject_text is None else inject_text
    body = _build_block(memory_available=memory_available, inject_text=inject_text)
    return f"{START_MARKER}\n{body}\n{END_MARKER}"


def render_full_document(
    memory_available: bool | None = None, inject_text: str | None = None
) -> str:
    """Eine komplette AGENTS.md fuer den Fall, dass noch keine existiert."""
    header = "# AGENTS.md\n\nGuidance for AI coding agents working in this repository.\n"
    block = render_block(memory_available=memory_available, inject_text=inject_text)
    return f"{header}\n{block}\n"


def upsert(path: Path) -> str:
    """Schreibt/aktualisiert die AGENTS.md idempotent.

    - Datei fehlt        -> komplette Vorlage mit SIN-Block anlegen.
    - Datei ohne Block   -> SIN-Block am Ende anhaengen.
    - Datei mit Block    -> nur den Bereich zwischen den Markern ersetzen.

    Gibt eine kurze Statusmeldung zurueck.
    """
    mem_available, inject_text = _memory_context()
    block = render_block(memory_available=mem_available, inject_text=inject_text)
    if not path.exists():
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            render_full_document(memory_available=mem_available, inject_text=inject_text)
        )
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
