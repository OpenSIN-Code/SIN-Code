# SIN-Code Tool Suite v2 — Master TODO

**Status:** ✅ DONE (2026-06-02)
**Erstellt:** 2026-06-02
**Letztes Update:** 2026-06-02 (gofmt + tag move + grasp alias fix)

---

## ✅ DONE

### 1. MCP Server für alle 7 Tools
- [x] SIN-Code-Discover-Tool — `cmd/discover/mcp_server.go`
- [x] SIN-Code-Execute-Tool — `cmd/execute/mcp_server.go`
- [x] SIN-Code-Map-Tool — `cmd/map/mcp_server.go`
- [x] SIN-Code-Grasp-Tool — `cmd/grasp/mcp_server.go`
- [x] SIN-Code-Scout-Tool — `cmd/scout/mcp_server.go`
- [x] SIN-Code-Harvest-Tool — `cmd/harvest/mcp_server.go`
- [x] SIN-Code-Orchestrate-Tool — `cmd/orchestrate/mcp_server.go`

**Status:** All 7 servers work end-to-end (verified with smoke test)

### 2. Unit Tests für alle 7 Tools
- [x] SIN-Code-Discover-Tool — JSON Output, Sort, Truncation
- [x] SIN-Code-Execute-Tool — Secret Redaction, Timeout, Error Field
- [x] SIN-Code-Map-Tool — Dependency Graph, Module Edges
- [x] SIN-Code-Grasp-Tool — File Field Alias (incl. file_path alias), Non-existent File
- [x] SIN-Code-Scout-Tool — .venv Exclusion, Summary Field
- [x] SIN-Code-Harvest-Tool — 404 Handling, Status/Body Fields
- [x] SIN-Code-Orchestrate-Tool — Plan/Rollback Fields, -id Shorthand

**Status:** 33 tool-logic tests + 42 MCP transport tests = 75 tests, all passing

### 3. Forge Code Generation
- [x] SIN-Code-Forge-Tool — Already implemented (not a stub)
- [x] Code Generation for new tools (template-based)
- [x] Boilerplate for Go CLI tools

**Status:** `pkg/tools/forge.go` (2.6K) + `forge_helpers.go` (1.2K) + `mcp/forge_mcp_server.py`

### 4. opencode.json Restoration
- [x] Restore opencode.json from backup
- [x] All 7 tools registered in `mcp` block (NOT deprecated `mcpServers`)
- [x] Bundle path configured
- [x] Sync scripts fixed (no longer sync deprecated `mcpServers` key)

**Status:** Verified with `opencode debug config` — 0 errors

### 5. README Verbesserung
- [x] Installation instructions for each tool
- [x] Examples for each parameter
- [x] Troubleshooting
- [x] Performance tips

**Status:** All 7 tool READMEs have full sections

### 6. CI/CD mit GitHub Actions
- [x] Build & Test Pipeline (`.github/workflows/ci.yml` in each repo)
- [x] Release creation with tags
- [x] Multi-platform builds (darwin amd64/arm64, linux amd64)

**Status:** All 7 repos have CI workflows

### 7. CoDocs Standard
- [x] All `.doc.md` companion files created
- [x] `sin codocs check` passes with 0 broken references in all 7 repos
- [x] 14 companion files added (2 per tool: mcp_server.go + test file)

### 8. Production Tags
- [x] v0.3.1 for discover, execute, map (full production release)
- [x] v0.2.1 for grasp, scout, harvest, orchestrate (full production release)
- [x] All tags pushed to GitHub (origin or opensin)

---

## 🟡 IN PROGRESS

### 9. Performance Tests
- [ ] Benchmarks for large codebases
- [ ] Memory profiling
- [ ] Optimization heatmap

### 10. Integration Tests
- [ ] Tool-Chain Tests (Discover → Scout → Grasp)
- [ ] End-to-End workflows
- [ ] Real-world scenarios

### 11. MCP Test Coverage
- [ ] Expand from 6 to 10+ tests per tool (more edge cases)
- [ ] Concurrent request handling
- [ ] Malformed input handling

---

## 🟢 NICE-TO-HAVE (Später)

### 12. Issue Templates
- [ ] Bug report template
- [ ] Feature request template
- [ ] Question template

### 13. AGENTS.md MCP Integration Section
- [ ] Document how to register each tool in opencode.json
- [ ] Add to each tool's AGENTS.md

### 14. CI/CD Improvements
- [ ] Migrate from `softprops/action-gh-release@v1` to `release-please` (?)
- [ ] Add code coverage reporting
- [ ] Add security scanning

---

## Production Status

| Tool | Tag | Tests | MCP | CoDocs | Status |
|------|-----|-------|-----|--------|--------|
| discover | **v0.3.1** | 11/11 ✅ | ✅ | ✅ | Production |
| execute | **v0.3.1** | 11/11 ✅ | ✅ | ✅ | Production |
| map | **v0.3.1** | 10/10 ✅ | ✅ | ✅ | Production |
| grasp | **v0.2.1** | 10/10 ✅ | ✅ | ✅ | Production |
| scout | **v0.2.1** | 9/9 ✅ | ✅ | ✅ | Production |
| harvest | **v0.2.1** | 9/9 ✅ | ✅ | ✅ | Production |
| orchestrate | **v0.2.1** | 12/12 ✅ | ✅ | ✅ | Production |

**Total: 75 tests, 7 MCP servers, 14 CoDocs files, 0 broken references**

---

## Commits This Run

- 7× version bumps (v0.2.5-fixes/v0.1.5-fixes → v0.3.0/v0.2.0)
- 7× CoDocs companion files
- 7× gofmt -w style fixes
- 1× grasp file_path alias fix
- 7× tag moves (v0.3.0/v0.2.0 → v0.3.1/v0.2.1)

**Total: 29 new commits, all on GitHub**

---

## Recently Closed Issues

- #69: [CRITICAL FIX] Sync scripts (closed with correct mcp key fix)

---

*Last updated: 2026-06-02*
