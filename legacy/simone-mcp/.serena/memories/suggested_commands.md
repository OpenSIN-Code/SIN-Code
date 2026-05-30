Key commands for Simone-MCP:
- python3 -m venv .venv && source .venv/bin/activate
- pip install -e .[dev]
- python3 src/cli.py print-card
- python3 src/cli.py run-action '{"action":"simone.mcp.health"}'
- python3 src/cli.py serve
- python3 src/cli.py serve-mcp
- python3 -m pytest tests/test_simone_mcp.py -q
- python3 -m compileall src
- docker-compose up --build
Useful git/GitHub commands:
- git status --short
- gh repo view --json name,owner,url,defaultBranchRef
- gh api repos/<owner>/<repo>/readme --jq .content | base64 --decode
System context is Darwin/macOS with zsh; python3 is available and used throughout the repo.