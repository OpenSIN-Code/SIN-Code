# Installation — `sin-code-efsm`

## Requirements

- Python **3.11+**
- `pip` (or `uv`/`pipx`)
- Git (for repository-aware features)

## Install from source (recommended during preview)

```bash
git clone https://github.com/OpenSIN-Code/SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration.git
cd SIN-Code-Ephemeral-Full-Stack-Mocking-Orchestration
pip install -e .
```

This installs the `efsm` command and the importable package `sin_code_efsm`.

## Install into an isolated environment

```bash
python -m venv .venv
source .venv/bin/activate      # Windows: .venv\Scripts\activate
pip install -e .
```

## Verify the installation

```bash
efsm --help
pytest -q
```

## Uninstall

```bash
pip uninstall sin-code-efsm
```
