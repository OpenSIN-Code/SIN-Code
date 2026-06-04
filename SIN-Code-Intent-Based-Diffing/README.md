# SIN-Code Intent-Based Diffing (IBD)

> Semantic diffs that summarize *architectural intent and risk* — not just lines
> added and removed.

[![Python](https://img.shields.io/badge/python-3.11%2B-blue)](https://www.python.org/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

Part of the [SIN-Code](https://github.com/OpenSIN-Code) agent-engineering stack.

## Why

A line-based diff tells you *what text* changed. It does not tell you that a
function signature changed (and may break callers), that a decorator controlling
auth was removed, or that a rename happened. IBD diffs the **AST**, classifies
the change into an **intent**, and assigns a **risk score** an agent or reviewer
can act on.

## Features

- **AST-level diff** (Python) comparing symbols, signatures, decorators,
  docstrings and bodies — not raw text.
- **Intent classification**: `api_change`, `refactor`, `feature`, `cleanup`, …
- **Risk scoring** with a weighted breakdown by change type.
- **Git review mode** — analyze uncommitted working-tree changes in one command.
- **Rich terminal output** and `--json` for machine consumption.

## Quickstart

```bash
pip install -e .
ibd diff old.py new.py          # semantic diff of two files
ibd diff old.py new.py --json   # machine-readable
ibd review-git                  # review uncommitted changes in the repo
```

## Example

```text
╭───────────────── SIN-Code IBD ─────────────────╮
│ Risk: high (6.0)                                │
╰─────────────────────────────────────────────────╯
[HIGH] API signatures modified: 1 symbol(s)
    Changed function/method signatures may break callers. Review required.
```

## Documentation

- [INSTALL.md](./INSTALL.md)
- [docs/USAGE.md](./docs/USAGE.md)
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [CHANGELOG.md](./CHANGELOG.md)

## License

MIT — see [LICENSE](./LICENSE).
