#!/usr/bin/env bash
set -euo pipefail

echo "Simone-MCP Setup"
echo "================="

python_version=$(python3 --version 2>/dev/null | cut -d' ' -f2 | cut -d'.' -f1,2)
if [[ "$python_version" < "3.12" ]]; then
    echo "ERROR: Python 3.12+ required. Found: $python_version"
    exit 1
fi
echo "OK: Python $python_version"

if ! command -v uv &>/dev/null; then
    echo "Installing uv..."
    pip install --break-system-packages uv 2>/dev/null || pip install uv
fi
echo "OK: uv installed"

echo "Creating virtual environment..."
uv venv
source .venv/bin/activate

echo "Installing dependencies..."
uv pip install -e ".[dev]"

if [ ! -f .env ]; then
    cp .env.example .env 2>/dev/null || echo "No .env.example found — create .env manually"
    echo "NOTE: Edit .env with your configuration"
fi

if command -v docker &>/dev/null && [ -f docker-compose.yml ]; then
    echo "Starting databases (Qdrant + Neo4j)..."
    docker-compose up -d qdrant neo4j 2>/dev/null || echo "Docker compose skipped"
else
    echo "NOTE: Docker not found — databases not started"
fi

echo ""
echo "Setup complete!"
echo "  Start HTTP server:  python src/cli.py serve"
echo "  Start MCP stdio:    python src/cli.py serve-mcp"
echo "  Validate config:    python src/cli.py validate"
