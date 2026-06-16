# instinct/portable.go — JSONL exchange

One JSON object per line. `ExportJSONL` / `ImportJSONL`.

## Why JSONL and not YAML-stream

- Robust against partial corruption (one bad line ≠ the whole file)
- Streaming-friendly (no need to hold the whole export in memory)
- Standard tools: `jq`, `grep`, line-level `diff`
- All fields are scalar/list — no multi-line strings to worry about

## Why a separate `portable` struct

`Instinct` has `yaml:` tags for the frontmatter; we want a *stable
wire format* independent of the storage format. `portable` is the
contract; `Instinct` is the in-memory model.

## Related files

- `cli.go` — `cmdExport` / `cmdImport` use these
