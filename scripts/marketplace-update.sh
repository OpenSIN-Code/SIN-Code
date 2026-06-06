# Purpose: Update all installed skills
# Docs: marketplace-update.sh.doc.md
#!/usr/bin/env bash
# Purpose: Update all installed skills to their latest versions
# Docs: marketplace-update.sh.doc.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SLUG="${1:-}"

if command -v sin-marketplace &>/dev/null; then
    if [[ -n "$SLUG" ]]; then
        sin-marketplace update "$SLUG"
    else
        sin-marketplace update
    fi
else
    echo "sin-marketplace CLI not found. Installing..."
    pip install -e "$SCRIPT_DIR/.." --quiet
    if [[ -n "$SLUG" ]]; then
        sin-marketplace update "$SLUG"
    else
        sin-marketplace update
    fi
fi
