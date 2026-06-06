# Purpose: Search skills in the marketplace catalog
# Docs: marketplace-search.sh.doc.md
#!/usr/bin/env bash
# Purpose: Search skills by keyword in the marketplace catalog
# Docs: marketplace-search.sh.doc.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
QUERY="${1:-}"
REMOTE="${2:-}"

if [[ -z "$QUERY" ]]; then
    echo "Usage: $0 <query> [--remote]"
    exit 1
fi

if command -v sin-marketplace &>/dev/null; then
    if [[ "$REMOTE" == "--remote" ]]; then
        sin-marketplace search "$QUERY" --remote
    else
        sin-marketplace search "$QUERY"
    fi
else
    echo "sin-marketplace CLI not found. Installing..."
    pip install -e "$SCRIPT_DIR/.." --quiet
    if [[ "$REMOTE" == "--remote" ]]; then
        sin-marketplace search "$QUERY" --remote
    else
        sin-marketplace search "$QUERY"
    fi
fi
