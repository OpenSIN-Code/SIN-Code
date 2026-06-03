"""Compile portable SIN skills into each agent's native command/skill format.

One source of truth: `skills/*.md` with YAML frontmatter (name, description,
arguments) + a prompt body. `compile_skills()` renders them into:

- opencode -> .opencode/command/<name>.md        (frontmatter: description, agent)
- codex    -> ~/.codex/prompts/<name>.md          (plain prompt, $N positional args)
- claude   -> .claude/skills/<name>/SKILL.md       (frontmatter: name, description)

This mirrors how cross-agent tools (Ulis/Nexel) keep a single prompt library in
sync across CLIs.

Docs: skills.doc.md
"""
from __future__ import annotations

import re
from dataclasses import dataclass, field
from pathlib import Path
from typing import Literal

try:
    import yaml
except ImportError:  # pragma: no cover
    # pyyaml is optional at import time so that ``import sin_code_bundle.skills``
    # never fails. Parsing functions raise a clear error instead. This lets the
    # module be loaded in slim CI environments that don't need YAML support.
    yaml = None  # type: ignore

# ── Targets & Schemas ────────────────────────────────────────────────

# Supported target agents. Adding a new target requires:
#   1. extend the ``Target`` literal
#   2. add a branch in :func:`render_skill`
#   3. decide where it writes (per-user, per-repo, etc.)
Target = Literal["opencode", "codex", "claude"]
SUPPORTED_TARGETS: tuple[Target, ...] = ("opencode", "codex", "claude")

# Matches the standard YAML frontmatter used by our skills/*.md sources.
# Captures (yaml_meta, body). The trailing ``.*$`` with DOTALL lets the body
# span newlines so multi-paragraph prompts round-trip cleanly.
_FRONTMATTER_RE = re.compile(r"^---\s*\n(.*?)\n---\s*\n(.*)$", re.DOTALL)


# ── Skill Model ──────────────────────────────────────────────────────


@dataclass
class Skill:
    """A parsed source skill (one ``skills/*.md`` file).

    Attributes:
        name: Stable identifier — used as the filename in every target's
            output. Falls back to the source filename stem if the frontmatter
            omits it.
        description: One-line summary shown in command pickers.
        body: Prompt body, with YAML frontmatter stripped.
        arguments: List of argument descriptors (each a dict with at least a
            ``"name"`` key). Used to template ``{{name}}`` placeholders when
            rendering to codex (which expects positional ``$N``).
    """

    name: str
    description: str
    body: str
    arguments: list[dict] = field(default_factory=list)

    @classmethod
    def parse(cls, path: Path) -> "Skill":
        """Parse a single ``skills/*.md`` file into a :class:`Skill`.

        Args:
            path: Filesystem path to a markdown file with YAML frontmatter
                delimited by ``---`` fences.

        Returns:
            The parsed :class:`Skill`.

        Raises:
            ValueError: If the file is missing the YAML frontmatter block.
            RuntimeError: If PyYAML is not installed in the environment.
        """
        text = path.read_text(encoding="utf-8")
        m = _FRONTMATTER_RE.match(text)
        if not m:
            raise ValueError(f"{path} is missing YAML frontmatter")
        if yaml is None:
            raise RuntimeError("pyyaml is required to parse skills")
        meta = yaml.safe_load(m.group(1)) or {}
        return cls(
            name=meta.get("name", path.stem),
            description=meta.get("description", ""),
            body=m.group(2).strip(),
            arguments=meta.get("arguments", []) or [],
        )


# ── Rendering ────────────────────────────────────────────────────────


def _body_for_codex(skill: Skill) -> str:
    """Codex prompts use positional $1, $2 ... — map {{arg}} -> $N."""
    body = skill.body
    for i, arg in enumerate(skill.arguments, start=1):
        body = body.replace("{{" + arg["name"] + "}}", f"${i}")
    return body


def render_skill(skill: Skill, target: Target) -> tuple[str, str]:
    """Return (relative_output_path, file_content) for a target agent.

    The relative output path is interpreted relative to the base directory
    chosen by :func:`compile_skills` (per-user for codex, per-repo for the
    others). The path includes a trailing filename and parent directory so
    callers only have to ``open`` it for writing.
    """
    if target == "opencode":
        # ``agent: build`` is the default opencode sub-agent that runs custom
        # commands; we hard-code it here because all our skills target it.
        fm = f"---\ndescription: {skill.description}\nagent: build\n---\n\n"
        return f".opencode/command/{skill.name}.md", fm + skill.body + "\n"

    if target == "codex":
        # codex uses ``prompts/`` (not dot-prefixed) and a flat layout under
        # ~/.codex — no frontmatter, just the templated body.
        return f"prompts/{skill.name}.md", _body_for_codex(skill) + "\n"

    if target == "claude":
        # claude skills live in a per-skill subdirectory with a fixed
        # ``SKILL.md`` name — that's the convention their loader expects.
        fm = f"---\nname: {skill.name}\ndescription: {skill.description}\n---\n\n"
        return f".claude/skills/{skill.name}/SKILL.md", fm + skill.body + "\n"

    raise ValueError(f"unknown target: {target}")


# ── Discovery & Compilation ──────────────────────────────────────────


def load_skills(source_dir: Path = Path("skills")) -> list[Skill]:
    """Load every ``*.md`` file under ``source_dir`` as a :class:`Skill`.

    Returns an empty list if ``source_dir`` does not exist (e.g. running
    in a clone that hasn't checked out the skills library). The result is
    sorted by filename for deterministic, diff-friendly output ordering.
    """
    if not source_dir.exists():
        return []
    return [Skill.parse(p) for p in sorted(source_dir.glob("*.md"))]


def compile_skills(
    target: Target,
    source_dir: Path = Path("skills"),
    out_root: Path = Path("."),
    dry_run: bool = False,
) -> list[Path]:
    """Compile every source skill into `target`'s native format.

    For codex, paths are written under the user's ~/.codex/; for opencode and
    claude they are written relative to the repo (out_root).

    Args:
        target: Which agent to render for.
        source_dir: Directory containing ``*.md`` source skills.
        out_root: Base directory for opencode/claude outputs (ignored for
            codex, which always lands under ``~/.codex``).
        dry_run: If True, return the would-be destination paths without
            writing files or creating parent directories.

    Returns:
        A list of destination paths in the same order as the source skills.
    """
    written: list[Path] = []
    # codex installs prompts per-user (other agents are per-repo), so its
    # base dir is fixed to ~/.codex regardless of out_root.
    base = Path.home() / ".codex" if target == "codex" else out_root

    for skill in load_skills(source_dir):
        rel, content = render_skill(skill, target)
        dest = base / rel
        written.append(dest)
        if not dry_run:
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text(content, encoding="utf-8")
    return written
