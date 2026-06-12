# Installation — `sin-code-bundle`

## Requirements

- Python **3.11+**
- `pip` (or `uv`/`pipx`)
- Git (for repository-aware features)

## Install from source (recommended during preview)

```bash
git clone https://github.com/OpenSIN-Code/SIN-Code.git
cd SIN-Code-Bundle
pip install -e .
```

This installs the `sin` command and the importable package `sin_code_bundle`.

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
sin --help
pytest -q
```

## Uninstall

```bash
pip uninstall sin-code-bundle
```
