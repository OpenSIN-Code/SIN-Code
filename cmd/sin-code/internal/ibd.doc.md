# `ibd.doc.md` — Intent-Based Diffing Subcommand

Compares two versions of code and scores whether the changes match the stated intent using a custom diff engine and keyword-based intent scoring.

## What it does

- **Compares two file versions** (before/after) and produces a line-by-line diff with added, removed, and context lines.
- **Extracts symbols** from both versions using language-specific parsers (Go AST, regex for Python/JS/Rust/Java) to identify added, removed, and modified functions/classes/types.
- **Scores intent match** (0-100) with keyword-based evaluation:
  - `add`/`create`/`implement` → expects new symbols (+30 if found, -20 if not)
  - `remove`/`delete` → expects removed symbols (+30 if found, -20 if not)
  - `refactor`/`modify`/`change`/`update` → expects any changes (+20)
  - `fix`/`optimize`/`improve` → expects modifications (+25)
  - `rename` → looks for add+remove pairs with similar names (+30)
  - `error`/`retry`/`test` → checks diff lines for matching keywords (+15-20)
- **Match levels:** `strong` (≥80), `partial` (≥60), `weak` (≥40), `none` (<40).

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `IbdCmd` into the root cobra command
- `cmd/sin-code/internal/ibd.go` — self-contained diff engine and intent scorer
- `cmd/sin-code/internal/grasp.go` — reuses `detectLanguage` and `extractStructure` patterns via `extractSymbolsFromContent`
- `cmd/sin-code/internal/poc.go` — shares `symbolInfo` struct definition (currently duplicated; should be unified)

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--before` | `""` | Before version (file path, commit, or raw string) |
| `--after` | `""` | After version (file path or raw string) |
| `--intent` | `""` | Stated intent of the change |
| `--from` | `""` | Git commit (old) for path target |
| `--to` | `""` | Git commit (new) for path target |
| `--output` | `""` | Output JSON file (optional) |
| `--format` | `text` | Output: `text` or `json` |

- **Git diff not implemented:** `--from`/`--to` flags print a note and fall back to reading the file as-is. Use `git diff` externally and pipe to files.
- **Diff algorithm:** Simple line-by-line positional comparison. Does NOT use Myers diff or LCS — moved lines appear as remove+add pairs.

## Usage examples

```bash
# Compare two files with intent
sin-code ibd --before old.py --after new.py --intent "add retry logic"

# Evaluate a refactoring
sin-code ibd --before v1.go --after v2.go --intent "refactor authentication"

# JSON output for CI
sin-code ibd --before src.go --after src.go --intent "add error handling" --format json
```

## Known caveats / footguns

- **Diff is naive positional:** Lines are compared by index. If a block of code was moved, every line in the block appears as removed+added, inflating the change count.
- **Intent scoring is keyword-based, not semantic:** It does not understand the actual logic of changes. A comment saying "retry" could score higher than a real retry implementation.
- **Git integration is a stub:** `--from`/`--to` do not extract historical versions. Always use `--before`/`--after` with actual file paths.
- **Symbol extraction is regex-based for non-Go:** Same limitations as `grasp.go` — may miss multi-line or complex definitions.
- **Score can be misleading:** A score of 80+ ("strong") does not guarantee the change is correct, only that the keywords and structural changes align with the stated intent.