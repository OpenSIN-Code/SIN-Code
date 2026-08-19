# Simone-MCP migration provenance

- Source: `OpenSIN-Code/Simone-MCP`
- Source tip before rewrite: `5ff9aaca03cd2d54911648e400c5f78fb7e19a59`
- License: MIT (`LICENSE` preserved).
- Migration: source history filtered only to remove tracked `node_modules`, generated `dist`, and `.coverage`, then rewritten under `legacy/simone-mcp/` and merged with `--no-ff`.
- Secret scan: no high-confidence private-key/API-token patterns found in migrated tree.
- Source tests after migration: 58 passed with `PYTHONPATH=legacy/simone-mcp/src`.
- SIN-Code regression check: `go test ./cmd/sin-code/internal/...` passed.
