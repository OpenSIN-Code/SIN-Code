# Installation — `sin-code-adw`

## Requirements

- Python **3.11+**
- `pip` (or `uv`/`pipx`)
- Git (for repository-aware features)

## Install from source (recommended during preview)

```bash
git clone https://github.com/OpenSIN-Code/SIN-Code-Architectural-Debt-Watchdogs.git
cd SIN-Code-Architectural-Debt-Watchdogs
pip install -e .
```

This installs the `adw` command and the importable package `sin_code_adw`.

## Install into an isolated environment

```bash
python -m venv .venv
source .venv/bin/activate      # Windows: .venv\Scripts\activate
pip install -e .
```

## Verify the installation

```bash
adw --help
pytest -q
```

## Uninstall

```bash
pip uninstall sin-code-adw
```
