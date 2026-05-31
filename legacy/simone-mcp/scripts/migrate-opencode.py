#!/usr/bin/env python3
"""Auto-migrate OpenCode configs to use Simone MCP tools.

Scans AGENTS.md, opencode.json, and .opencode/*.md files for patterns
that reference old built-in tools (grep, find, edit) and replaces them
with Simone MCP tool equivalents.
"""

import json
import re
import shutil
from pathlib import Path


def load_mappings(path: Path) -> list[tuple[str, str]]:
    raw = path.read_text()
    lines = []
    for line in raw.splitlines():
        line = line.strip()
        if line.startswith("replacements:") or line.startswith("tools:") or line.startswith("targets:"):
            continue
        if line.startswith("#") or not line:
            continue
        if "replace_with:" in line:
            continue
        if "when:" in line:
            continue
        if '"**"' in line:
            continue
        if line.startswith("- pattern:"):
            pattern = line.split('"')[1] if '"' in line else ""
            continue
        if line.startswith("  replacement:"):
            replacement = line.split('"')[1] if '"' in line else ""
            if pattern and replacement:
                lines.append((pattern, replacement))
    return lines


MAPPINGS = [
    (r'Use grep for.*symbol', 'Use sin_simone_mcp_symbol_search for symbol search'),
    (r'grep -r.*function', 'sin_simone_mcp_symbol_search with type=function'),
    (r'find \. -name', 'sin_simone_mcp_project_overview for file discovery'),
    (r'IMMER grep verwenden', 'IMMER sin_simone_mcp_symbol_search verwenden'),
    (r'statt grep', 'statt grep (DEPRECATED: use sin_simone_mcp_symbol_search)'),
    (r'Use grep for.*references', 'Use sin_simone_mcp_find_references for reference lookup'),
]


def migrate_file(path: Path) -> bool:
    try:
        content = path.read_text(encoding="utf-8")
    except Exception:
        return False

    original = content

    for pattern, replacement in MAPPINGS:
        content = re.sub(pattern, replacement, content, flags=re.IGNORECASE)

    if content == original:
        return False

    bak = path.with_suffix(path.suffix + ".bak")
    if not bak.exists():
        shutil.copy2(path, bak)

    path.write_text(content, encoding="utf-8")
    print(f"  MIGRATED: {path}")
    return True


def find_configs() -> list[Path]:
    paths = []
    home = Path.home()

    for p in [
        home / ".config" / "opencode" / "AGENTS.md",
        home / ".agents" / "AGENTS.md",
    ]:
        if p.exists():
            paths.append(p)

    for p in Path.cwd().rglob("AGENTS.md"):
        paths.append(p)

    for p in Path.cwd().rglob("opencode.json"):
        paths.append(p)

    for p in Path.cwd().rglob(".opencode/*.md"):
        paths.append(p)

    seen = set()
    return [p for p in paths if p not in seen and not seen.add(p)]


def main():
    print("Simone MCP: Migrating OpenCode configs...")
    configs = find_configs()
    migrated = sum(migrate_file(p) for p in configs)
    print(f"Done. {migrated}/{len(configs)} files migrated.")
    if migrated:
        print("Restart OpenCode to apply changes.")


if __name__ == "__main__":
    main()
