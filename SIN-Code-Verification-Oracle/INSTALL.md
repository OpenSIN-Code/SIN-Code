# Installation — `sin-code-oracle`

## Requirements

- Python **3.11+**
- `pip` (or `uv`/`pipx`)
- Git (for repository-aware features)

## Install from source (recommended during preview)

```bash
git clone https://github.com/OpenSIN-Code/SIN-Code-Verification-Oracle.git
cd SIN-Code-Verification-Oracle
pip install -e .
```

This installs the `oracle` command and the importable package `sin_code_oracle`.

## Install into an isolated environment

```bash
python -m venv .venv
source .venv/bin/activate      # Windows: .venv\Scripts\activate
pip install -e .
```

## Optional: MCP server support

The MCP server requires the optional `mcp` dependency:

```bash
pip install -e ".[mcp]"
```

## Verify the installation

```bash
oracle --help
pytest -q
```

## Uninstall

```bash
pip uninstall sin-code-oracle
```
