# Purpose: Sync marketplace catalog with Infra-SIN-OpenCode-Stack
# Docs: marketplace-sync.sh.doc.md
#!/usr/bin/env bash
# Purpose: Sync marketplace catalog with Infra-SIN-OpenCode-Stack
# Docs: marketplace-sync.sh.doc.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if command -v sin-marketplace &>/dev/null; then
    sin-marketplace sync
else
    echo "sin-marketplace CLI not found. Installing..."
    pip install -e "$SCRIPT_DIR/.." --quiet
    sin-marketplace sync
fi
