#!/usr/bin/env python3
"""Simone MCP — OpenCode Integration Script

Fuegt Simone MCP in OpenCode-Konfig ein und disabled Built-in Tools:
1. MCP Server-Eintrag in ~/.config/opencode/opencode.json
2. grep/glob disablen → sin_simone_mcp_* erlauben
3. AGENTS.md mit Tool-Replacement Regeln patchen
"""

import json
import os
import shutil
import sys
from pathlib import Path

OPCODE_CONFIG = Path.home() / ".config" / "opencode" / "opencode.json"
OPCODE_AGENTS = Path.home() / ".config" / "opencode" / "AGENTS.md"
SIMONE_REPO = Path(__file__).resolve().parent.parent

AGENTS_RULES = """

## Simone MCP — Tool Replacements

Use Simone MCP tools INSTEAD of OpenCode built-ins:

| Instead of | Use |
|------------|-----|
| `grep` / `Grep` for symbol search | `sin_simone_mcp_symbol_search` |
| `edit` / `Edit` for code changes | `sin_simone_mcp_structural_edit` |
| manual grep for references | `sin_simone_mcp_find_references` |
| project exploration | `sin_simone_mcp_project_overview` |

"""


def patch_opencode_config() -> bool:
    if not OPCODE_CONFIG.exists():
        print(f"[SKIP] {OPCODE_CONFIG} not found")
        return False

    with open(OPCODE_CONFIG) as f:
        config = json.load(f)

    backup = OPCODE_CONFIG.with_suffix(".json.bak")
    if not backup.exists():
        shutil.copy2(OPCODE_CONFIG, backup)
        print(f"[BACKUP] {backup}")

    changed = False

    mcp = config.setdefault("mcp", {})
    if "sin-simone-mcp" not in mcp:
        mcp["sin-simone-mcp"] = {
            "type": "local",
            "command": [
                sys.executable or "python3",
                str(SIMONE_REPO / "src" / "cli.py"),
                "serve-mcp",
            ],
            "enabled": True,
        }
        changed = True
        print("[MCP] sin-simone-mcp server added")
    else:
        print("[MCP] sin-simone-mcp already configured")

    perm = config.setdefault("permission", {})

    builtin_replacements = {
        "grep": "deny",
        "glob": "deny",
    }

    for tool, action in builtin_replacements.items():
        current = perm.get(tool)
        if current != action:
            perm[tool] = action
            changed = True
            print(f"[PERM] {tool} → {action}")

    # Allow all Simone MCP tools
    for tool in ("grep", "glob"):
        current = perm.get(tool)
        if current != "deny":
            perm[tool] = "deny"
            changed = True
            print(f"[PERM] {tool} → deny")

    if perm.get("sin_simone_mcp_*") != "allow":
        perm["sin_simone_mcp_*"] = "allow"
        changed = True
        print("[PERM] sin_simone_mcp_* → allow")

    if changed:
        with open(OPCODE_CONFIG, "w") as f:
            json.dump(config, f, indent=2)
        print(f"[OK] {OPCODE_CONFIG} updated")
    else:
        print(f"[OK] {OPCODE_CONFIG} already up-to-date")

    return changed


def patch_agents_md() -> bool:
    if not OPCODE_AGENTS.exists():
        print(f"[SKIP] {OPCODE_AGENTS} not found")
        return False

    with open(OPCODE_AGENTS) as f:
        content = f.read()

    if "## Simone MCP — Tool Replacements" in content:
        print("[AGENTS] Already patched")
        return False

    with open(OPCODE_AGENTS, "a") as f:
        f.write(AGENTS_RULES)

    print(f"[AGENTS] Rules appended to {OPCODE_AGENTS}")
    return True


def main():
    print("=== Simone MCP — OpenCode Integration ===")
    patch_opencode_config()
    patch_agents_md()
    print("=== Done ===")
    print()
    print("Next steps:")
    print("  1. Restart OpenCode (or reload config)")
    print("  2. Agents will now prefer Simone MCP tools")
    print(f"  3. Simone MCP server: python3 src/cli.py serve-mcp")


if __name__ == "__main__":
    main()
