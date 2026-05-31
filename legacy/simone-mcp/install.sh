#!/usr/bin/env bash
# Simone MCP Installer with auto-migration
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
echo "Installing Simone MCP..."
cd "$DIR"

python_version=$(python3 --version 2>/dev/null | cut -d' ' -f2 | cut -d'.' -f1,2)
if [[ "$python_version" < "3.12" ]]; then
    echo "ERROR: Python 3.12+ required. Found: $python_version"
    exit 1
fi

pip install -e ".[dev]"

echo "Migrating OpenCode configs to use Simone MCP tools..."
python3 "$DIR/scripts/migrate-opencode.py"

echo ""
echo "Simone MCP installed."
echo "  Start MCP stdio:  python3 src/cli.py serve-mcp"
echo "  Start HTTP:       python3 src/cli.py serve"
echo "  Integrate CLI:    python3 src/cli.py integrate"
