# Purpose: Install a skill from the marketplace
# Docs: marketplace-install.sh.doc.md
#!/usr/bin/env bash
# Purpose: Install a skill from the marketplace catalog
# Docs: marketplace-install.sh.doc.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SLUG="${1:-}"

if [[ -z "$SLUG" ]]; then
    echo "Usage: $0 <slug>"
    exit 1
fi

if command -v sin-marketplace &>/dev/null; then
    sin-marketplace install "$SLUG"
else
    echo "sin-marketplace CLI not found. Installing..."
    pip install -e "$SCRIPT_DIR/.." --quiet
    sin-marketplace install "$SLUG"
fi
