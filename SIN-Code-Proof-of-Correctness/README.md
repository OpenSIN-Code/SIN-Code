# SIN-Code Proof of Correctness (POC)

> Lightweight, practical formal verification for AI-generated code —
> property-based testing driven by function signatures and natural-language
> intent.

[![Python](https://img.shields.io/badge/python-3.11%2B-blue)](https://www.python.org/)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](./LICENSE)

Part of the [SIN-Code](https://github.com/OpenSIN-Code) agent-engineering stack.

## Why

Full theorem proving (TLA+, Lean) is too heavy for everyday agent loops. POC
brings the *spirit* of formal methods to the practical layer: it infers
**properties** (invariants) a function should satisfy, renders runnable
[Hypothesis](https://hypothesis.readthedocs.io/) tests, and verifies them at
runtime — turning "looks right" into "checked against hundreds of inputs".

## Features

- **Property generator** — infers candidate invariants (idempotence,
  monotonicity, length-preservation, sorted output, reversibility, purity, …)
  from a function's signature and name.
- **Runtime verifier** — executes properties with Hypothesis and reports
  `PASS` / `FAIL` / `SKIPPED` per property (no unsafe `eval`).
- **Spec compiler** — turns declarative pre/post-conditions, or a
  natural-language intent, into a property test.
- **CLI** (`poc`) for suggesting, verifying, and compiling specs.

## Quickstart

```bash
pip install -e .
poc suggest mymodule.py my_function       # print suggested Hypothesis tests
poc verify mymodule.py my_function        # run runtime verification
poc from-intent "returns a non-negative, sorted list" my_function
```

## Documentation

- [INSTALL.md](./INSTALL.md)
- [docs/USAGE.md](./docs/USAGE.md)
- [docs/CONFIGURATION.md](./docs/CONFIGURATION.md)
- [CONTRIBUTING.md](./CONTRIBUTING.md)
- [CHANGELOG.md](./CHANGELOG.md)

## License

MIT — see [LICENSE](./LICENSE).
