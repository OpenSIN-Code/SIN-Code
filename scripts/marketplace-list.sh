# Purpose: List installed skills
# Docs: marketplace-list.sh.doc.md
#!/usr/bin/env bash
# Purpose: List installed skills in the local registry
# Docs: marketplace-list.sh.doc.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if command -v sin-marketplace &>/dev/null; then
    sin-marketplace list
else
    echo "sin-marketplace CLI not found. Installing..."
    pip install -e "$SCRIPT_DIR/.." --quiet
    sin-marketplace list
fi
