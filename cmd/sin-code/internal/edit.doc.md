# edit — hashline-anchored surgical editing

Replaces native string/line editors. Anchor mode: anchors (`LINE:HASH` from
`read --mode hashline`) carry a content hash, so stale edits fail loudly with
a refresh hint instead of corrupting files; drift up to ±25 lines auto-
resolves. Operations: replace (single line or `--end-anchor` range), insert
before/after, delete. String mode: exact `--old-string` with occurrence
counting (ambiguity is an error unless `--replace-all`). Every result is
syntax-validated (see write.doc.md) and applied via the atomic write path.
`--dry-run` returns a unified diff. MCP: `sin_edit` (in-process).
