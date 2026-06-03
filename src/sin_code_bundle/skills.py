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
    yaml = None  # type: ignore

Target = Literal["opencode", "codex", "claude"]
SUPPORTED_TARGETS: tuple[Target, ...] = ("opencode", "codex", "claude")

_FRONTMATTER_RE = re.compile(r"^---\s*\n(.*?)\n---\s*\n(.*)$", re.DOTALL)


@dataclass
class Skill:
    name: str
    description: str
    body: str
    arguments: list[dict] = field(default_factory=list)

    @classmethod
    def parse(cls, path: Path) -> "Skill":
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


def _body_for_codex(skill: Skill) -> str:
    """Codex prompts use positional $1, $2 ... — map {{arg}} -> $N."""
    body = skill.body
    for i, arg in enumerate(skill.arguments, start=1):
        body = body.replace("{{" + arg["name"] + "}}", f"${i}")
    return body


def render_skill(skill: Skill, target: Target) -> tuple[str, str]:
    """Return (relative_output_path, file_content) for a target agent."""
    if target == "opencode":
        fm = f"---\ndescription: {skill.description}\nagent: build\n---\n\n"
        return f".opencode/command/{skill.name}.md", fm + skill.body + "\n"

    if target == "codex":
        return f"prompts/{skill.name}.md", _body_for_codex(skill) + "\n"

    if target == "claude":
        fm = f"---\nname: {skill.name}\ndescription: {skill.description}\n---\n\n"
        return f".claude/skills/{skill.name}/SKILL.md", fm + skill.body + "\n"

    raise ValueError(f"unknown target: {target}")


def load_skills(source_dir: Path = Path("skills")) -> list[Skill]:
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
    """
    written: list[Path] = []
    base = Path.home() / ".codex" if target == "codex" else out_root

    for skill in load_skills(source_dir):
        rel, content = render_skill(skill, target)
        dest = base / rel
        written.append(dest)
        if not dry_run:
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text(content, encoding="utf-8")
    return written
