#!/usr/bin/env python3
"""Purpose: Add a finding record to an axis JSON file.

Docs: add_finding.doc.md
"""

import json
import sys
from pathlib import Path

# ── Module overview ──────────────────────────────────────────────────
#
# Single-purpose CLI: append one finding to an axis JSON file.
# Called by every axis_*.sh script after a scout/grep hit. The same
# script also bootstraps a fresh JSON file if it doesn't yet exist.
#
# Wire format (positional args, NOT argparse — keeps shell-call cheap):
#   1. file        — path to the axis JSON (e.g., findings/security.json)
#   2. gate_id     — gate identifier (e.g., "1.1", "3.3")
#   3. severity    — CRITICAL / HIGH / MEDIUM / LOW / INFO
#   4. cwe         — CWE-* identifier or empty
#   5. title       — short one-line description
#   6. description — full sentence
#   7. fix         — recommended remediation
#
# Idempotency: gate records are deduplicated by gate_id. Findings
# are NOT deduplicated — multiple scout matches each get their own
# finding entry, and score.py aggregates by gate.
# ─────────────────────────────────────────────────────────────────────


# ── CLI arg parsing (positional only — fast, no argparse overhead) ────
# 7 positional args required; otherwise we print usage and exit 1.
if len(sys.argv) < 8:
    print(
        "Usage: add_finding.py <file> <gate_id> <severity> <cwe> <title> <description> <fix>",
        file=sys.stderr,
    )
    sys.exit(1)

# Each positional arg is mandatory — see usage line above.
f = Path(sys.argv[1])
gate_id = sys.argv[2]
severity = sys.argv[3]
cwe = sys.argv[4]
title = sys.argv[5]
description = sys.argv[6]
fix = sys.argv[7]

# ── Load existing JSON or initialise a fresh skeleton ─────────────────
# axis JSON shape: {"axis": <name>, "gates": [...], "findings": [...]}.
if f.exists():
    data = json.loads(f.read_text())
else:
    # Bootstrap: caller will overwrite "axis" later if needed.
    data = {"axis": "unknown", "gates": [], "findings": []}

# ── Append a gate record if not already present (idempotent) ──────────
# Multiple findings can share a gate_id; we only add the gate metadata once.
gate_exists = any(g.get("id") == gate_id for g in data.get("gates", []))
if not gate_exists:
    data.setdefault("gates", []).append(
        {
            "id": gate_id,
            "severity": severity,
            # status: "finding" for actionable items, "info" for purely informational.
            "status": "finding" if severity in ("CRITICAL", "HIGH", "MEDIUM", "LOW") else "info",
        }
    )

# ── Append finding — duplicates are intentionally NOT de-duplicated ───
# (Each scout hit becomes its own finding; the scorer aggregates by gate.)
data.setdefault("findings", []).append(
    {
        "gate": gate_id,
        "severity": severity,
        "cwe": cwe,
        "title": title,
        "description": description,
        "fix": fix,
    }
)

# ── Write back atomically (write_text replaces the file in one syscall) ─
f.write_text(json.dumps(data, indent=2))
