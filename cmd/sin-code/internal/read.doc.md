# read — token-efficient anchored file reading

Replaces native `read`/`cat` for agents. Three modes: `hashline` (default,
emits `LINE:HASH|content` anchors consumed by `edit`), `raw` (offset/limit
guarded), `outline` (structure only — reuses grasp's extractStructure/
extractDependencies/extractExports, 80–95% token savings on large files).
Guards: 1 MiB default `--max-bytes`, 2000-line default limit, binary/non-UTF-8
rejection. MCP: `sin_read` (in-process handler, no subprocess).
