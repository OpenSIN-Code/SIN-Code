# GitHub Issues — SIN-Code Tool Suite v2

**Erstellt:** 2026-06-02
**Repo:** OpenSIN-Code/SIN-Code-Discover-Tool (und alle anderen Tool-Repos)

---

## Issue 1: [MCP Server] Add MCP server to Discover-Tool

**Repo:** OpenSIN-Code/SIN-Code-Discover-Tool
**Labels:** `enhancement`, `mcp`, `priority: critical`
**Milestone:** v0.3.0

### Description
The Discover-Tool currently only works as a CLI binary. It needs an MCP server implementation so it can be used as a tool in opencode.

### Acceptance Criteria
- [ ] `cmd/discover/mcp_server.go` created
- [ ] `--mcp` flag added to CLI
- [ ] JSON-RPC 2.0 protocol implemented
- [ ] `tools/list` returns tool definition
- [ ] `tools/call` executes discover with arguments
- [ ] Tested with `echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | discover --mcp`
- [ ] Documentation updated in README.md

### Related
- Plan: `PLAN_MCP_SERVERS.md`
- Blocks: opencode.json registration

---

## Issue 2: [MCP Server] Add MCP server to Execute-Tool

**Repo:** OpenSIN-Code/SIN-Code-Execute-Tool
**Labels:** `enhancement`, `mcp`, `priority: critical`
**Milestone:** v0.3.0

### Description
The Execute-Tool currently only works as a CLI binary. It needs an MCP server implementation.

### Acceptance Criteria
- [ ] `cmd/execute/mcp_server.go` created
- [ ] `--mcp` flag added
- [ ] JSON-RPC 2.0 protocol implemented
- [ ] `execute_command` method works
- [ ] Secret redaction preserved
- [ ] Timeout handling works via MCP
- [ ] Tested and documented

### Related
- Plan: `PLAN_MCP_SERVERS.md`

---

## Issue 3: [MCP Server] Add MCP server to Map-Tool

**Repo:** OpenSIN-Code/SIN-Code-Map-Tool
**Labels:** `enhancement`, `mcp`, `priority: critical`
**Milestone:** v0.3.0

### Acceptance Criteria
- [ ] `cmd/map/mcp_server.go` created
- [ ] `--mcp` flag added
- [ ] JSON-RPC 2.0 protocol implemented
- [ ] `map_architecture` method works
- [ ] Dependency graph preserved
- [ ] Module-level edges preserved
- [ ] Tested and documented

---

## Issue 4: [MCP Server] Add MCP server to Grasp-Tool

**Repo:** OpenSIN-Code/SIN-Code-Grasp-Tool
**Labels:** `enhancement`, `mcp`, `priority: critical`
**Milestone:** v0.3.0

### Acceptance Criteria
- [ ] `cmd/grasp/mcp_server.go` created
- [ ] `--mcp` flag added
- [ ] JSON-RPC 2.0 protocol implemented
- [ ] `grasp_file` method works
- [ ] `file` and `file_path` aliases work
- [ ] Tested and documented

---

## Issue 5: [MCP Server] Add MCP server to Scout-Tool

**Repo:** OpenSIN-Code/SIN-Code-Scout-Tool
**Labels:** `enhancement`, `mcp`, `priority: critical`
**Milestone:** v0.2.0

### Acceptance Criteria
- [ ] `cmd/scout/mcp_server.go` created
- [ ] `--mcp` flag added
- [ ] JSON-RPC 2.0 protocol implemented
- [ ] `scout_code` method works
- [ ] `.venv` exclusion preserved
- [ ] Tested and documented

---

## Issue 6: [MCP Server] Add MCP server to Harvest-Tool

**Repo:** OpenSIN-Code/SIN-Code-Harvest-Tool
**Labels:** `enhancement`, `mcp`, `priority: critical`
**Milestone:** v0.2.0

### Acceptance Criteria
- [ ] `cmd/harvest/mcp_server.go` created
- [ ] `--mcp` flag added
- [ ] JSON-RPC 2.0 protocol implemented
- [ ] `harvest_url` method works
- [ ] 404 handling preserved
- [ ] `status` and `body` fields preserved
- [ ] Tested and documented

---

## Issue 7: [MCP Server] Add MCP server to Orchestrate-Tool

**Repo:** OpenSIN-Code/SIN-Code-Orchestrate-Tool
**Labels:** `enhancement`, `mcp`, `priority: critical`
**Milestone:** v0.2.0

### Acceptance Criteria
- [ ] `cmd/orchestrate/mcp_server.go` created
- [ ] `--mcp` flag added
- [ ] JSON-RPC 2.0 protocol implemented
- [ ] `orchestrate_task` method works
- [ ] `add`, `list`, `update`, `delete` actions work
- [ ] `plan` and `rollback` fields preserved
- [ ] `-id` shorthand preserved
- [ ] Tested and documented

---

## Issue 8: [Tests] Add unit tests for Discover-Tool

**Repo:** OpenSIN-Code/SIN-Code-Discover-Tool
**Labels:** `tests`, `priority: critical`
**Milestone:** v0.3.0

### Description
0 test files currently. Add comprehensive unit tests.

### Acceptance Criteria
- [ ] `pkg/tools/discover_test.go` created
- [ ] TestDiscoverJSON — Valide Pfad → JSON Array
- [ ] TestDiscoverSortByRelevance
- [ ] TestDiscoverSortByName
- [ ] TestDiscoverMaxResults
- [ ] TestDiscoverNonExistentPath — JSON Error
- [ ] TestDiscoverTotalMatches
- [ ] Coverage ≥ 70%

### Related
- Plan: `PLAN_UNIT_TESTS.md`

---

## Issue 9: [Tests] Add unit tests for Execute-Tool

**Repo:** OpenSIN-Code/SIN-Code-Execute-Tool
**Labels:** `tests`, `priority: critical`
**Milestone:** v0.3.0

### Acceptance Criteria
- [ ] `pkg/tools/execute_test.go` created
- [ ] TestExecuteSimple
- [ ] TestExecuteTimeout
- [ ] TestExecuteSecretRedaction
- [ ] TestExecuteEnvVarRedaction
- [ ] TestExecuteErrorField
- [ ] TestExecuteDurationMs
- [ ] Coverage ≥ 70%

---

## Issue 10: [Tests] Add unit tests for Map-Tool

**Repo:** OpenSIN-Code/SIN-Code-Map-Tool
**Labels:** `tests`, `priority: critical`

### Acceptance Criteria
- [ ] `pkg/tools/map_test.go` created
- [ ] TestMapBasic
- [ ] TestMapNonExistentPath
- [ ] TestMapModuleEdges
- [ ] TestMapDependencyGraph
- [ ] Coverage ≥ 70%

---

## Issue 11: [Tests] Add unit tests for Grasp-Tool

**Repo:** OpenSIN-Code/SIN-Code-Grasp-Tool
**Labels:** `tests`, `priority: critical`

### Acceptance Criteria
- [ ] `pkg/tools/grasp_test.go` created
- [ ] TestGraspFile
- [ ] TestGraspFileAlias
- [ ] TestGraspNonExistentFile
- [ ] TestGraspRelatedFiles
- [ ] Coverage ≥ 70%

---

## Issue 12: [Tests] Add unit tests for Scout-Tool

**Repo:** OpenSIN-Code/SIN-Code-Scout-Tool
**Labels:** `tests`, `priority: critical`

### Acceptance Criteria
- [ ] `pkg/tools/scout_test.go` created
- [ ] TestScoutRegex
- [ ] TestScoutVenvExclusion
- [ ] TestScoutSummary
- [ ] TestScoutDurationMs
- [ ] Coverage ≥ 70%

---

## Issue 13: [Tests] Add unit tests for Harvest-Tool

**Repo:** OpenSIN-Code/SIN-Code-Harvest-Tool
**Labels:** `tests`, `priority: critical`

### Acceptance Criteria
- [ ] `pkg/tools/harvest_test.go` created
- [ ] TestHarvestSuccess
- [ ] TestHarvest404
- [ ] TestHarvestInvalidURL
- [ ] TestHarvestStatusField
- [ ] Coverage ≥ 70%

---

## Issue 14: [Tests] Add unit tests for Orchestrate-Tool

**Repo:** OpenSIN-Code/SIN-Code-Orchestrate-Tool
**Labels:** `tests`, `priority: critical`

### Acceptance Criteria
- [ ] `pkg/tools/orchestrate_test.go` created
- [ ] TestOrchestrateAdd
- [ ] TestOrchestrateList
- [ ] TestOrchestrateIdShorthand
- [ ] TestOrchestratePlanField
- [ ] TestOrchestrateRollbackField
- [ ] TestOrchestrateDependencies
- [ ] TestOrchestrateTags
- [ ] TestOrchestrateInvalidAction
- [ ] Coverage ≥ 70%

---

## Issue 15: [Forge] Implement Code Generation in Forge-Tool

**Repo:** OpenSIN-Code/SIN-Code-Forge-Tool
**Labels:** `enhancement`, `priority: critical`
**Milestone:** v0.2.0

### Description
Forge is currently a 51-line stub. It needs to be a real code generation tool.

### Acceptance Criteria
- [ ] `pkg/forge/forge.go` — Core Generation Logic
- [ ] `pkg/forge/template.go` — Template Engine
- [ ] `pkg/forge/templates/tool.go.tmpl` — Tool Template
- [ ] `pkg/forge/templates/mcp_server.go.tmpl` — MCP Server Template
- [ ] `pkg/forge/forge_test.go` — Tests
- [ ] `forge --name my-tool` generates complete tool
- [ ] `forge --name my-tool --with-mcp` includes MCP server
- [ ] Coverage ≥ 70%

### Related
- Plan: `PLAN_FORGE.md`

---

## Issue 16: [opencode.json] Restore and register all MCP servers

**Repo:** N/A (global config)
**Labels:** `config`, `priority: critical`

### Description
`~/.config/opencode/opencode.json` is missing. Restore from backup and register all 7 MCP servers.

### Acceptance Criteria
- [ ] `~/.config/opencode/opencode.json` restored
- [ ] All 7 MCP servers registered
- [ ] JSON is valid
- [ ] opencode recognizes all tools
- [ ] Tested with `discover()` in opencode

### Related
- Plan: `PLAN_OPENCODE_JSON.md`

---

## Issue 17: [README] Improve README for Discover-Tool

**Repo:** OpenSIN-Code/SIN-Code-Discover-Tool
**Labels:** `documentation`

### Acceptance Criteria
- [ ] Features section
- [ ] Installation (via bundle + manual)
- [ ] Usage examples (basic, pattern, sort)
- [ ] Parameter table
- [ ] MCP server section
- [ ] Troubleshooting
- [ ] Examples section

### Related
- Plan: `PLAN_README.md`

---

## Issue 18: [README] Improve README for Execute-Tool

**Repo:** OpenSIN-Code/SIN-Code-Execute-Tool
**Labels:** `documentation`

### Acceptance Criteria
- [ ] Same structure as Issue 17
- [ ] Examples: Simple, Long-running, With env vars
- [ ] Secret redaction documented

---

## Issue 19-23: [README] Improve README for Map, Grasp, Scout, Harvest, Orchestrate

Same structure as Issue 17-18.

---

## Issue 24: [CI/CD] Add GitHub Actions for Discover-Tool

**Repo:** OpenSIN-Code/SIN-Code-Discover-Tool
**Labels:** `ci`, `priority: high`

### Acceptance Criteria
- [ ] `.github/workflows/ci.yml` created
- [ ] Test job runs on push/PR
- [ ] Release job runs on tags
- [ ] Multi-platform builds (darwin amd64/arm64, linux amd64)
- [ ] Coverage report uploaded

### Related
- Plan: `PLAN_CICD.md`

---

## Issue 25-30: [CI/CD] Add GitHub Actions for other 6 tools

Same structure as Issue 24.

---

## Summary

**Total Issues:** 30
- 7 MCP Server issues
- 7 Unit Test issues
- 1 Forge implementation
- 1 opencode.json
- 7 README improvements
- 7 CI/CD pipelines

**Estimated Total Effort:** ~15-20 hours

**Suggested Order:**
1. opencode.json (blocks everything)
2. MCP Servers (7 issues, parallel)
3. Unit Tests (7 issues, parallel)
4. Forge (1 issue)
5. READMEs (7 issues, parallel)
6. CI/CD (7 issues, parallel)

## Performance Issues — 2026-06-02

### Discover — discovery_500_py_files
- **Result**: 5.222s
- **Target**: 3.000s
- **Gap**: 2.22s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — discovery_1000_py_files
- **Result**: 17.801s
- **Target**: 5.000s
- **Gap**: 12.80s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — max_results_early_stop_10
- **Result**: 18.685s
- **Target**: 0.500s
- **Gap**: 18.19s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — max_results_early_stop_100
- **Result**: 18.863s
- **Target**: 1.000s
- **Gap**: 17.86s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — max_results_early_stop_1000
- **Result**: 18.708s
- **Target**: 3.000s
- **Gap**: 15.71s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — extension_filter_py
- **Result**: 5.523s
- **Target**: 2.000s
- **Gap**: 3.52s over target
- **Recommendation**: Optimize critical path or adjust target.

### SCKG — simple_query_100_files
- **Result**: 0.070s
- **Target**: 0.050s
- **Gap**: 0.02s over target
- **Recommendation**: Optimize critical path or adjust target.

### SCKG — complex_query_100_files
- **Result**: 0.076s
- **Target**: 0.050s
- **Gap**: 0.03s over target
- **Recommendation**: Optimize critical path or adjust target.

### SCKG — simple_query_10000_files
- **Result**: 1.218s
- **Target**: 1.000s
- **Gap**: 0.22s over target
- **Recommendation**: Optimize critical path or adjust target.

### SCKG — complex_query_10000_files
- **Result**: 1.174s
- **Target**: 1.000s
- **Gap**: 0.17s over target
- **Recommendation**: Optimize critical path or adjust target.


## Performance Issues — 2026-06-02

### Discover — discovery_500_py_files
- **Result**: 5.888s
- **Target**: 3.000s
- **Gap**: 2.89s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — discovery_1000_py_files
- **Result**: 25.318s
- **Target**: 5.000s
- **Gap**: 20.32s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — max_results_early_stop_10
- **Result**: 26.405s
- **Target**: 0.500s
- **Gap**: 25.91s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — max_results_early_stop_100
- **Result**: 24.303s
- **Target**: 1.000s
- **Gap**: 23.30s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — max_results_early_stop_1000
- **Result**: 23.270s
- **Target**: 3.000s
- **Gap**: 20.27s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — relevance_scoring_500
- **Result**: 6.307s
- **Target**: 5.000s
- **Gap**: 1.31s over target
- **Recommendation**: Optimize critical path or adjust target.

### Discover — extension_filter_py
- **Result**: 6.654s
- **Target**: 2.000s
- **Gap**: 4.65s over target
- **Recommendation**: Optimize critical path or adjust target.

### SCKG — simple_query_100_files
- **Result**: 0.089s
- **Target**: 0.050s
- **Gap**: 0.04s over target
- **Recommendation**: Optimize critical path or adjust target.

### SCKG — complex_query_100_files
- **Result**: 0.080s
- **Target**: 0.050s
- **Gap**: 0.03s over target
- **Recommendation**: Optimize critical path or adjust target.

### SCKG — simple_query_1000_files
- **Result**: 0.249s
- **Target**: 0.200s
- **Gap**: 0.05s over target
- **Recommendation**: Optimize critical path or adjust target.

### SCKG — complex_query_1000_files
- **Result**: 0.217s
- **Target**: 0.200s
- **Gap**: 0.02s over target
- **Recommendation**: Optimize critical path or adjust target.

