# CodeGraph Integration Proposal

**Status:** Strategic Analysis  
**Reference:** https://github.com/colbymchenry/codegraph  
**Last Updated:** 2026-06-14

---

## Executive Summary

SIN-Code already possesses the core code-graph capabilities (symbol indexing, call-graph analysis, impact prediction, AST extraction, MCP serving). CodeGraph (colbymchenry/codegraph) adds **multi-language strength** (20+ languages via tree-sitter), **SQLite/FTS5 indexing**, **cross-language bridging** (Swift↔ObjC, React-Native), and a **native file-watcher with debouncing**.

**Recommendation:** Integrate CodeGraph as an **external MCP tool** (similar to RTK pattern), not by rebuilding SIN-Code's graph engine.

---

## What SIN-Code Already Has

| Component | Location | Capability |
|---|---|---|
| Symbol/Call-Graph | `cartographer.go` | PageRank-weighted symbol map, graph centrality ranking |
| Impact Analysis | `impact.go` | Blast-radius prediction (reverse dependency, affected tests) |
| Index Storage | `index_store.go`, `.sin-code/index.bin` | Persistent incremental index, trigram-based search |
| AST Extraction | `ast_provider.go`, `ast_treesitter_stub.go` | tree-sitter (optional CGO), structural fallback |
| MCP Server | `serve.go`, `--mcp` flag | Exposes tools: search, symbols, callers, callees, impact, lsp |
| Full-Text Search | Trigram index + `searchSymbols()` | Fast symbol lookup |
| LSP | `lsp_cmd.go` | Language Server Protocol implementation |

**Assessment:** The architecture is **complete for SIN-Code's primary use case** (Go-centric, single-binary CLI agent). Multi-language symbol graphs are not the bottleneck.

---

## What CodeGraph Adds (Real Differentiators)

1. **Multi-Language Symbol Resolution** (20+ languages)
   - SIN-Code: Go-zentric (uses `go list -json`), tree-sitter is opt-in/fallback
   - CodeGraph: tree-sitter-first for all 20+ supported languages
   - **Impact:** Better context for polyglot codebases (Go + Rust + Python + TS)

2. **SQLite + FTS5 Backend**
   - SIN-Code: gob-serialized in-memory index (trigram-based)
   - CodeGraph: persistent SQL full-text search (more powerful queries, survives restarts)
   - **Impact:** Query expressiveness, no memory footprint for large repos

3. **Cross-Language Call Bridges**
   - Swift ↔ ObjC, React-Native, Expo linking
   - **Impact:** Useful for mobile teams; not needed for backend-only SIN-Code workflows

4. **File-Watcher with Debounce & Staleness UI**
   - SIN-Code: Refresh is manual (`sin-code index` command) or implicit on agent spawn
   - CodeGraph: inotify/FSEvents with configurable debounce, staleness banner
   - **Impact:** Better DX for IDE integration + Claude Code/Cursor

5. **MCP-First Design**
   - CodeGraph is built to serve **other agents** (Claude Code, Cursor, etc.)
   - SIN-Code **is** the agent; it consumes its own graph
   - **Impact:** Different distribution model; CodeGraph shines as a shared service

---

## Integration Options

### Option A: CodeGraph as External MCP Tool (Recommended)
- **What:** Register CodeGraph as an MCP preset (like RTK was registered as a binary)
- **How:**
  1. `sin-code codegraph install` fetches the binary from https://github.com/colbymchenry/codegraph/releases
  2. SIN-Code's own MCP server adds a proxy tool `codegraph:explore` → calls the external service
  3. Callers can request symbol/impact data from CodeGraph for all 20+ languages; SIN-Code's Go-specific tools remain
- **Upsides:** Zero duplication, automatic multi-language support, clean separation
- **Downsides:** Requires CodeGraph binary to be running; network latency if served over HTTP
- **Effort:** ~200 lines: install command + MCP proxy tool + config

### Option B: Enhanced SIN-Code Index (Moderate)
- **What:** Improve the existing `index` + `cartographer` to use SQLite/FTS5 + tree-sitter-first
- **How:**
  1. Add optional SQLite backend to `index_store.go` (keep gob as fallback)
  2. Promote tree-sitter from CGO-optional to default (requires Go 1.22+ and tree-sitter C headers)
  3. Add native fsnotify-based auto-watcher
  4. Extend cartographer to rank multi-language symbols uniformly
- **Upsides:** Single unified graph, no external dependency, better FTS5 queries
- **Downsides:** Complex migration, tree-sitter requires C build deps, longer compilation
- **Effort:** ~800 lines; touches index_store, cartographer, ast_provider, watch subsystem

### Option C: Hybrid (Best Long-Term, Complex)
- **What:** SIN-Code has a Go-fast path (current index); CodeGraph for multi-language exploration
- **How:**
  1. Keep SIN-Code's Go-optimized index for speed
  2. Add CodeGraph as optional MCP tool for multi-language exploration
  3. Cache CodeGraph results locally; use for impact analysis when needed
- **Upsides:** Fast Go path, extensible to other languages
- **Downsides:** Two graph systems, consistency issues, cache invalidation
- **Effort:** ~600 lines; complex integration

### Option D: Do Nothing
- **What:** Status quo
- **Why:** Current graph is sufficient for SIN-Code's primary workflows (Go agents, local code context)
- **When to revisit:** If you're targeting polyglot teams or integrating with Claude Code/Cursor as a shared service

---

## Recommended Path: Option A

**Rationale:**
- Zero risk of duplication (external binary, MCP interface)
- RTK pattern is already proven in SIN-Code
- CodeGraph's multi-language + FTS5 benefits flow naturally to agents without major refactoring
- Low effort (~200 lines), high value

**Implementation Sketch:**

```go
// cmd/sin-code/codegraph_cmd.go (new)
func newCodeGraphInstallCmd() *cobra.Command {
  // Downloads codegraph binary from https://github.com/colbymchenry/codegraph/releases
  // Stores in ~/.local/bin or custom config path
  // Verifies with `codegraph --version`
}

// internal/serve.go (addition)
// Add MCP tool proxy:
// {
//   name: "codegraph:explore",
//   description: "Multi-language code graph (tree-sitter, 20+ langs, SQLite/FTS5)",
//   inputSchema: { queries, languages, limits },
//   impl: forwards to external codegraph MCP server
// }

// internal/orchestrator/cartographer.go (minimal change)
// When impact/callers/callees is requested for non-Go:
// - if CodeGraph binary available: delegate via MCP proxy
// - else: fall back to Go-specific analysis
```

**Next Steps:**
1. Create GitHub issue #126 with this proposal + recommend Option A
2. If approved: implement `codegraph install` + MCP proxy tool (~200 LOC)
3. Verify CodeGraph binary integrates cleanly with SIN-Code's MCP server architecture
4. Add docs: "Multi-language code exploration with CodeGraph"

---

## Decision Matrix

| Criterion | Option A | Option B | Option C | Option D |
|---|---|---|---|---|
| **Duplication Risk** | None | Moderate | High | N/A |
| **Effort** | 200 LOC | 800 LOC | 600 LOC | 0 |
| **Multi-Language** | Yes (via external) | Yes (deep integration) | Yes (hybrid) | No (Go-focused) |
| **Performance** | Good (network latency) | Excellent (local) | Good | Excellent (current) |
| **Maintenance** | Minimal (external tool) | High (big refactor) | High (two systems) | None |
| **Risk to SIN-Code** | Low | Medium (migrations) | High (consistency) | None |
| **Recommended** | ✅ | — | — | — |

---

## Questions for Discussion

1. **Do you want SIN-Code to serve multi-language teams?** → leans Option A/B
2. **Is CodeGraph's SQLite/FTS5 significantly better than current trigram index?** → validate with real queries
3. **Will CodeGraph be a shared service (Claude Code, Cursor) or SIN-Code-only?** → affects deployment model
4. **Can you rely on an external binary (like RTK)?** → enables Option A

---

## References

- CodeGraph Repo: https://github.com/colbymchenry/codegraph
- SIN-Code Index: `cmd/sin-code/internal/index_store.go`, `cartographer.go`, `impact.go`
- RTK Integration (pattern): `cmd/sin-code/rtk_cmd.go`, `internal/rtk/`
- Current MCP Tools: `internal/serve.go` (search, callers, callees, impact, lsp)
