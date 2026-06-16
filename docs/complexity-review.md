# Complexity Review

`sin-code review --complexity` implements ponytail's 5-tag complexity review
format for Go code. It is static, deterministic, and never applies fixes.

## Usage

```bash
sin-code review --complexity
sin-code review --complexity --path ./pkg --since HEAD~1 --tags yagni,shrink
sin-code review --complexity --format json
```

## Output format

One line per finding:

```text
<tag>: <what to cut>. <replacement>. [path:line]
```

The report ends with:

```text
net: -<N> lines, -<M> deps possible.
```

If nothing is found, it prints:

```text
Lean already. Ship.
```

## Tags

| Tag | Meaning | Example replacement |
|---|---|---|
| `delete` | Dead code, unused flexibility | Nothing replaces it |
| `stdlib` | Hand-rolled thing the standard library ships | Use `min`/`max`, `slices.Repeat` |
| `native` | Dependency or code doing what the platform already does | `errors.New` / `fmt.Errorf` with `%w` |
| `yagni` | Abstraction with one implementation | Inline until a second one exists |
| `shrink` | Same logic, fewer lines | Replace wrapper with the callee |

## Respecting `// sin-debt:` markers

Findings that overlap a `// sin-debt:` or `# sin-debt:` annotation are shown
as `(approved: sin-debt)`. They are still reported so the team can track them,
but the marker signals that the debt is deliberate and should not be blindly
removed.

## Boundaries

- Complexity only. Correctness bugs, security holes, and performance issues go
  through the normal review pass.
- Does not apply fixes; it only lists cuts.
- A single smoke test or `assert`-based self-check is the ponytail minimum, not
  bloat, and is never flagged for deletion.

## JSON output

```bash
sin-code review --complexity --format json
```

The JSON envelope contains `findings`, `net_lines`, `net_deps`, and `status`
(`lean` or `cuts-available`).

## Markdown output

```bash
sin-code review --complexity --format markdown
```

Produces a Markdown table with the same findings.
