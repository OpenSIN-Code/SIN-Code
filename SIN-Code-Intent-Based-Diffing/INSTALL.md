# Installation — `sin-code-ibd`

## Requirements

- Python **3.11+**
- `pip` (or `uv`/`pipx`)
- Git (for repository-aware features)

## Install from source (recommended during preview)

```bash
git clone https://github.com/OpenSIN-Code/SIN-Code-Intent-Based-Diffing.git
cd SIN-Code-Intent-Based-Diffing
pip install -e .
```

This installs the `ibd` command and the importable package `sin_code_ibd`.

## Install into an isolated environment

```bash
python -m venv .venv
source .venv/bin/activate      # Windows: .venv\Scripts\activate
pip install -e .
```

## Verify the installation

```bash
ibd --help
pytest -q
```

## Uninstall

```bash
pip uninstall sin-code-ibd
```
