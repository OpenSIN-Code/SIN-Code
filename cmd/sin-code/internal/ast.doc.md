# ast — tiered structure extraction & AST-anchored edits

Phase 4. One interface (parseOutline -> FileOutline with exact start/end
lines per symbol), three engines, best available wins:

1. go/ast (stdlib, always on) — exact Go: methods as "Recv.Name", struct
   fields and interface methods as children, real ranges from token.FileSet.
2. tree-sitter (opt-in: CGO_ENABLED=1 go build -tags treesitter) — exact
   Python/JS/TS/TSX/Rust/Java via smacker/go-tree-sitter, table-driven node
   walking. Overrides the structural engine at init; go/ast keeps Go. The
   DEFAULT build does not link it — the bundle stays zero-dependency.
3. structural (pure Go, always on) — brace-depth tracking (string/comment
   aware) for C-family, indentation tracking for Python. Real end lines
   without CGO; this alone retires the documented single-line regex caveats.

Consumers: read --mode outline (now reports engine + exact symbol ranges),
edit --symbol NAME (AST-anchored: replace/delete/insert around whole
definitions; ambiguity fails listing candidates, qualified "Type.Method"
disambiguates; result is syntax-validated + atomically written), grasp/map/
SCKG migration follows. Every outline degrades gracefully: unknown language
=> engine "none", symbol mode refuses with a pointer to --anchor.
