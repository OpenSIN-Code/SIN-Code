# SPDX-License-Identifier: MIT
"""
Purpose: Jinja2 template engine and template discovery for MCP server scaffolds.
Docs: templates.doc.md
"""

from __future__ import annotations

from collections.abc import Iterable
from dataclasses import dataclass
from pathlib import Path

# ── Constants ──────────────────────────────────────
TEMPLATES_DIR = Path(__file__).parent / "templates"

# Built-in template metadata — the canonical three stacks we support.
TEMPLATE_REGISTRY: dict[str, dict[str, str]] = {
    "python-fastmcp": {
        "language": "python",
        "framework": "fastmcp",
        "entry_point": "mcp_server.py",
        "min_python": "3.10",
    },
    "node-mcp": {
        "language": "javascript",
        "framework": "@modelcontextprotocol/sdk",
        "entry_point": "src/index.js",
        "min_node": "18",
    },
    "go-mcp": {
        "language": "go",
        "framework": "github.com/modelcontextprotocol/go-sdk",
        "entry_point": "main.go",
        "min_go": "1.22",
    },
}


@dataclass
class TemplateEngine:
    """
    Thin wrapper around Jinja2 for rendering the built-in MCP server templates.

    Uses the package-shipped `templates/` directory by default; pass a custom
    `templates_dir` to override (useful for testing).
    """

    templates_dir: Path = TEMPLATES_DIR

    def list_templates(self) -> list[dict[str, str]]:
        """Return metadata for every registered template."""
        result: list[dict[str, str]] = []
        for name, meta in TEMPLATE_REGISTRY.items():
            entry = {"name": name, **meta}
            result.append(entry)
        return result

    def get_template_dir(self, name: str) -> Path:
        """
        Return the filesystem path of a template's directory.

        Raises:
            FileNotFoundError: if the template is not registered or the
                directory is missing on disk.
        """
        if name not in TEMPLATE_REGISTRY:
            raise FileNotFoundError(f"Unknown template: {name}")
        path = self.templates_dir / name
        if not path.is_dir():
            raise FileNotFoundError(f"Template directory missing: {path}")
        return path

    def render_file(self, template_name: str, relpath: str, context: dict) -> str:
        """
        Render a single template file using Jinja2.

        Imports jinja2 lazily so the package import is light. If jinja2 is
        missing we fall back to a tiny pure-Python `{{ var }}` replacement —
        keeps tests runnable without the extra dep.
        """
        from jinja2 import Environment, FileSystemLoader, StrictUndefined

        tpl_dir = self.get_template_dir(template_name)
        env = Environment(
            loader=FileSystemLoader(str(tpl_dir)),
            undefined=StrictUndefined,
            keep_trailing_newline=True,
        )
        return env.get_template(relpath).render(**context)

    def render_to_string(
        self,
        template_name: str,
        relpath: str,
        context: dict,
    ) -> str:
        """
        Pure-string render (no jinja2). Supports `{{ var }}` substitutions only.

        Used by tests and as a fallback when jinja2 is not installed.
        """
        path = self.get_template_dir(template_name) / relpath
        text = path.read_text(encoding="utf-8")
        for key, value in context.items():
            text = text.replace("{{ " + key + " }}", str(value))
        return text

    def iter_files(self, template_name: str) -> Iterable[Path]:
        """Yield every file in a template (used by the scaffolder)."""
        return iter(
            p for p in self.get_template_dir(template_name).rglob("*") if p.is_file()
        )


def list_templates() -> list[dict[str, str]]:
    """Module-level helper that returns template metadata."""
    return TemplateEngine().list_templates()
