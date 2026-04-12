# Contributing

## Setup

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -e .[dev]
```

## Local verification

```bash
pytest tests/ -v
python3 src/cli.py print-card
python3 src/cli.py run-action '{"action":"simone.mcp.health"}'
```

## Development rules

- Keep the Python source in `src/`
- Keep transport compatibility for both stdio and streamable HTTP
- Preserve the public tool names exposed by the MCP interface
- Update `.well-known/` metadata when public capabilities change
- Prefer additive, validated changes over speculative rewrites

## Pull requests

- describe the user-visible change
- include validation output
- keep scope narrow
- update docs when endpoints or metadata change