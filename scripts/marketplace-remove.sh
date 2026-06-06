# Purpose: Remove a skill from the marketplace
# Docs: marketplace-remove.sh.doc.md
#!/usr/bin/env bash
# Purpose: Remove a skill from the local registry and disk
# Docs: marketplace-remove.sh.doc.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SLUG="${1:-}"

if [[ -z "$SLUG" ]]; then
    echo "Usage: $0 <slug> [--force]"
    exit 1
fi

FORCE="${2:-}"
if command -v sin-marketplace &>/dev/null; then
    if [[ "$FORCE" == "--force" ]]; then
        sin-marketplace remove "$SLUG" --force
    else
        sin-marketplace remove "$SLUG"
    fi
else
    echo "sin-marketplace CLI not found. Installing..."
    pip install -e "$SCRIPT_DIR/.." --quiet
    if [[ "$FORCE" == "--force" ]]; then
        sin-marketplace remove "$SLUG" --force
    else
        sin-marketplace remove "$SLUG"
    fi
fi
