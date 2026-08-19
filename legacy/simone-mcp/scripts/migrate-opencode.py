#!/usr/bin/env python3
"""Auto-migrate OpenCode configs to use Simone MCP tools.

Scans AGENTS.md, opencode.json, and .opencode/*.md files for patterns
that reference old built-in tools (grep, find, edit) and replaces them
with Simone MCP tool equivalents.

Also structurally patches opencode.json: adds MCP server entry, denies
built-in grep/glob, allows sin_simone_mcp_* tools.
"""

import json
import re
import shutil
import sys
from pathlib import Path


MAPPINGS = [
    (r'Use grep for.*symbol', 'Use sin_simone_mcp_symbol_search for symbol search'),
    (r'grep -r.*function', 'sin_simone_mcp_symbol_search with type=function'),
    (r'find \. -name', 'sin_simone_mcp_project_overview for file discovery'),
    (r'IMMER grep verwenden', 'IMMER sin_simone_mcp_symbol_search verwenden'),
    (r'statt grep', 'statt grep (DEPRECATED: use sin_simone_mcp_symbol_search)'),
    (r'Use grep for.*references', 'Use sin_simone_mcp_find_references for reference lookup'),
    (r'`grep` / `Grep`', '`sin_simone_mcp_symbol_search`'),
    (r'`edit` / `Edit`', '`sin_simone_mcp_structural_edit`'),
    (r'manual grep for references', 'sin_simone_mcp_find_references'),
    (r'project exploration', 'sin_simone_mcp_project_overview'),
]

AGENTS_RULES = """

## Simone MCP — Tool Replacements (auto-migrated)

Use Simone MCP tools INSTEAD of OpenCode built-ins:

| Instead of | Use |
|------------|-----|
| `grep` / `Grep` for symbol search | `sin_simone_mcp_symbol_search` |
| `edit` / `Edit` for code changes | `sin_simone_mcp_structural_edit` |
| manual grep for references | `sin_simone_mcp_find_references` |
| project exploration | `sin_simone_mcp_project_overview` |

"""


def _patch_opencode_json(path: Path) -> list[str]:
    results = []
    try:
        config = json.loads(path.read_text(encoding="utf-8"))
    except Exception as e:
        return [f"SKIP: cannot parse {path}: {e}"]

    backup = path.with_suffix(".json.bak")
    if not backup.exists():
        shutil.copy2(path, backup)
        results.append(f"BACKUP: {backup}")

    changed = False

    mcp = config.setdefault("mcp", {})
    if "sin-simone-mcp" not in mcp:
        repo_root = Path(__file__).resolve().parent.parent
        mcp["sin-simone-mcp"] = {
            "type": "local",
            "command": [sys.executable or "python3", str(repo_root / "src" / "cli.py"), "serve-mcp"],
            "enabled": True,
        }
        changed = True
        results.append("ADD: sin-simone-mcp MCP server")
    else:
        results.append("OK: sin-simone-mcp already configured")

    perm = config.setdefault("permission", {})
    for tool in ("grep", "glob"):
        if perm.get(tool) != "deny":
            perm[tool] = "deny"
            changed = True
            results.append(f"DENY: built-in {tool}")

    if perm.get("sin_simone_mcp_*") != "allow":
        perm["sin_simone_mcp_*"] = "allow"
        changed = True
        results.append("ALLOW: sin_simone_mcp_*")

    if changed:
        path.write_text(json.dumps(config, indent=2), encoding="utf-8")
        results.append(f"SAVED: {path}")

    return results


def _patch_agents_md(path: Path) -> list[str]:
    results = []
    try:
        content = path.read_text(encoding="utf-8")
    except Exception:
        return [f"SKIP: cannot read {path}"]

    original = content

    for pattern, replacement in MAPPINGS:
        content = re.sub(pattern, replacement, content, flags=re.IGNORECASE)

    if "## Simone MCP — Tool Replacements" not in content:
        content += AGENTS_RULES
        results.append("APPEND: tool replacement rules")

    if content == original:
        return []

    bak = path.with_suffix(path.name + ".bak")
    parent_bak = path.parent / (path.name + ".bak")
    for b in (bak, parent_bak):
        if not b.exists():
            shutil.copy2(path, b)
            results.append(f"BACKUP: {b}")
            break

    path.write_text(content, encoding="utf-8")
    results.append(f"MIGRATED: {path}")
    return results


def find_configs() -> list[Path]:
    paths = set()
    home = Path.home()

    for p in [
        home / ".config" / "opencode" / "AGENTS.md",
        home / ".config" / "opencode" / "opencode.json",
        home / ".agents" / "AGENTS.md",
    ]:
        if p.exists():
            paths.add(p)

    for p in Path.cwd().rglob("AGENTS.md"):
        paths.add(p)
    for p in Path.cwd().rglob("opencode.json"):
        paths.add(p)
    for p in Path.cwd().rglob(".opencode/*.md"):
        paths.add(p)

    return sorted(paths, key=lambda p: str(p))


def main():
    print("Simone MCP: Migrating OpenCode configs...")
    configs = find_configs()

    agents_files = [p for p in configs if p.suffix == ".md"]
    json_files = [p for p in configs if p.suffix == ".json"]

    results = []
    for p in agents_files:
        results.extend(_patch_agents_md(p))
    for p in json_files:
        results.extend(_patch_opencode_json(p))

    for r in results:
        print(f"  {r}")

    if results:
        print("\nDone. Restart OpenCode to apply changes.")
    else:
        print("Nothing to migrate — all configs already up to date.")


if __name__ == "__main__":
    main()
