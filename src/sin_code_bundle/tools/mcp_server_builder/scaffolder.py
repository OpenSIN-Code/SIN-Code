"""
Purpose: Generate a new MCP server directory structure from a ScaffoldSpec.
Docs: scaffolder.doc.md
"""

from __future__ import annotations

import json
import re
import shutil
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from .templates import TEMPLATE_REGISTRY, TemplateEngine

# ── Constants ──────────────────────────────────────
SLUG_RE = re.compile(r"[^a-z0-9]+")


def slugify(value: str) -> str:
    """Lowercase, replace non-alphanumerics with `-`, trim, lowercase."""
    s = SLUG_RE.sub("-", value.strip().lower()).strip("-")
    return s or "unnamed"


def package_name(slug: str) -> str:
    """Convert a slug to a Python-style package name (no dashes)."""
    return slug.replace("-", "_")


@dataclass
class ScaffoldSpec:
    """
    Specification for a new MCP server.

    Attributes:
        name: Human-readable name (e.g. "My Cool Tool").
        description: One-line description (used in README, pyproject, etc.).
        tools: List of tool names to scaffold (e.g. ["do_x", "do_y"]).
        template: Template key — one of `python-fastmcp`, `node-mcp`, `go-mcp`.
        author: Author name and email for pyproject/package.json.
        version: Initial semver version.
    """

    name: str
    description: str = ""
    tools: list[str] = field(default_factory=list)
    template: str = "python-fastmcp"
    author: str = "OpenSIN-Code <contact@opensincode.org>"
    version: str = "0.1.0"

    def __post_init__(self) -> None:
        if self.template not in TEMPLATE_REGISTRY:
            raise ValueError(
                f"Unknown template: {self.template!r}. "
                f"Choose from: {sorted(TEMPLATE_REGISTRY)}"
            )
        if not self.tools:
            self.tools = ["ping"]  # Always scaffold at least one tool.
        # Validate tool names — must be valid Python identifiers.
        bad = [t for t in self.tools if not t.isidentifier()]
        if bad:
            raise ValueError(f"Tool names must be valid identifiers: {bad}")

    @property
    def slug(self) -> str:
        """Filesystem-friendly slug (e.g. "my-cool-tool")."""
        return slugify(self.name)

    @property
    def pkg(self) -> str:
        """Python package name (e.g. "my_cool_tool")."""
        return package_name(self.slug)

    def to_context(self) -> dict[str, Any]:
        """Render context dict for the template engine."""
        return {
            "name": self.name,
            "slug": self.slug,
            "pkg": self.pkg,
            "description": self.description,
            "tools": self.tools,
            "tools_json": json.dumps(self.tools),
            "author": self.author,
            "version": self.version,
        }


@dataclass
class Scaffolder:
    """
    Generate a new MCP server from a `ScaffoldSpec`.

    The scaffolder is intentionally filesystem-only — no network, no git ops.
    Caller is responsible for the initial `git init` and `git add` if needed.
    """

    engine: TemplateEngine = field(default_factory=TemplateEngine)

    def scaffold(self, target_dir: str | Path, spec: ScaffoldSpec) -> dict[str, Any]:
        """
        Create the directory tree at `target_dir` from the spec's template.

        Returns:
            Summary dict with `target`, `template`, `files`, `tools`.

        Raises:
            FileExistsError: if `target_dir` already exists and is non-empty.
            FileNotFoundError: if the template is missing.
        """
        target = Path(target_dir).expanduser().resolve()
        if target.exists() and any(target.iterdir()):
            raise FileExistsError(f"Target directory is not empty: {target}")
        target.mkdir(parents=True, exist_ok=True)

        template_dir = self.engine.get_template_dir(spec.template)
        context = spec.to_context()
        files_written: list[str] = []

        # Walk the template tree and render / copy each file.
        for src in sorted(template_dir.rglob("*")):
            if not src.is_file():
                continue
            rel = src.relative_to(template_dir)
            # Render the path so `{{ pkg }}` becomes the actual package name.
            rel_str = str(rel)
            for key, value in context.items():
                rel_str = rel_str.replace("{{ " + key + " }}", str(value))
            dest = target / rel_str
            dest.parent.mkdir(parents=True, exist_ok=True)
            try:
                # Try Jinja2 rendering (only succeeds for .j2 files).
                text = self.engine.render_file(spec.template, str(rel), context)
                dest.write_text(text, encoding="utf-8")
            except Exception:
                # Non-template or missing jinja2 — copy raw.
                shutil.copy2(src, dest)
            files_written.append(rel_str)

        return {
            "target": str(target),
            "template": spec.template,
            "files": files_written,
            "tools": spec.tools,
        }

    def dry_run(self, spec: ScaffoldSpec) -> dict[str, Any]:
        """Return a summary of what would be created without writing anything."""
        template_dir = self.engine.get_template_dir(spec.template)
        files = [
            str(p.relative_to(template_dir))
            for p in template_dir.rglob("*")
            if p.is_file()
        ]
        return {
            "target": f"<unsaved>/{spec.slug}",
            "template": spec.template,
            "files": files,
            "tools": spec.tools,
        }
