#!/usr/bin/env bash
# install-sin-skills.sh — one-shot installer for the SIN-Code ecosystem MCP skills.
#
# This script is the documented entry point referenced by SKILLS_INTEGRATION_README.md
# and INTEGRATION_INDEX.md. It simply delegates to the built-in skill manager:
#
#   sin-code skill install all
#
# Requirements on PATH: git, python3, go (for the skills that need them).
# The individual skill repos are cloned into ~/.local/share/sin-code/skills/.
set -euo pipefail

if ! command -v sin-code >/dev/null 2>&1; then
  echo "ERROR: sin-code not found on PATH. Build or install it first." >&2
  exit 1
fi

mkdir -p "${SIN_SKILLS_DIR:-$HOME/.local/share/sin-code/skills}"
echo "Installing SIN-Code ecosystem skills via 'sin-code skill install all'..."
sin-code skill install all
