# PLAN: MCP Server für alle 7 SIN-Code Tools

**Ziel:** Jedes Tool braucht einen MCP Server (JSON-RPC 2.0 über stdio), damit es in opencode als Tool aufgerufen werden kann.

**Status:** ✅ DONE (2026-06-02)
**Aufwand:** ~30 Minuten pro Tool = ~3.5 Stunden total

---

## Architektur

Jedes Tool hat:
- **CLI-Modus:** `discover -path ... -format json` (bereits implementiert)
- **MCP-Server-Modus:** `discover --mcp` (JSON-RPC 2.0 über stdio)

### MCP Protocol

```json
// Request
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "discover",
    "arguments": {
      "path": "/Users/jeremy/dev/PROJECT",
      "pattern": "**/*.py",
      "sort_by": "relevance",
      "max_results": 10
    }
  }
}

// Response
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": [{
      "type": "text",
      "text": "{...JSON output...}"
    }]
  }
}
```

### Implementation Pattern

```go
// cmd/discover/mcp_server.go
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
)

type MCPRequest struct {
    JSONRPC string         `json:"jsonrpc"`
    ID      int            `json:"id"`
    Method  string         `json:"method"`
    Params  MCPRequestParams `json:"params"`
}

type MCPRequestParams struct {
    Name      string                 `json:"name"`
    Arguments map[string]interface{} `json:"arguments"`
}

type MCPResponse struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      int         `json:"id"`
    Result  MCPResult   `json:"result,omitempty"`
    Error   MCPError    `json:"error,omitempty"`
}

type MCPResult struct {
    Content []MCPContent `json:"content"`
}

type MCPContent struct {
    Type string `json:"type"`
    Text string `json:"text"`
}

type MCPError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

func runMCPServer() error {
    scanner := bufio.NewScanner(os.Stdin)
    scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
    
    for scanner.Scan() {
        var req MCPRequest
        if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
            sendError(0, -32700, "Parse error")
            continue
        }
        
        switch req.Method {
        case "tools/list":
            handleToolsList(&req)
        case "tools/call":
            handleToolsCall(&req)
        default:
            sendError(req.ID, -32601, "Method not found")
        }
    }
    return nil
}

func handleToolsCall(req *MCPRequest) {
    args := req.Params.Arguments
    
    // Convert MCP args to CLI flags
    path, _ := args["path"].(string)
    pattern, _ := args["pattern"].(string)
    
    // Call the tool's Run function
    result, err := runDiscover(path, pattern, ...)
    if err != nil {
        sendError(req.ID, -32000, err.Error())
        return
    }
    
    // Send JSON result
    jsonBytes, _ := json.MarshalIndent(result, "", "  ")
    sendResult(req.ID, string(jsonBytes))
}
```

---

## Tool-spezifische Implementation

### 1. Discover-Tool
- **Method:** `discover_files`
- **Args:** `path`, `pattern`, `sort_by`, `max_results`, `format`
- **Returns:** Array of file metadata

### 2. Execute-Tool
- **Method:** `execute_command`
- **Args:** `command`, `timeout`, `cwd`
- **Returns:** Exit code, stdout, stderr, duration

### 3. Map-Tool
- **Method:** `map_architecture`
- **Args:** `path`, `action`, `max_depth`
- **Returns:** Module map, dependency graph

### 4. Grasp-Tool
- **Method:** `grasp_file`
- **Args:** `file`, `file_path`
- **Returns:** File structure, dependencies, related files

### 5. Scout-Tool
- **Method:** `scout_code`
- **Args:** `query`, `path`, `search_type`
- **Returns:** Code matches with context

### 6. Harvest-Tool
- **Method:** `harvest_url`
- **Args:** `url`, `method`, `headers`, `body`
- **Returns:** Status, headers, body

### 7. Orchestrate-Tool
- **Method:** `orchestrate_task`
- **Args:** `action`, `title`, `dependencies`, `tags`
- **Returns:** Task ID, plan, rollback

---

## Test-Plan

Für jeden MCP Server:

1. **Test 1:** `tools/list` — Gibt korrekte Tool-Definition zurück
2. **Test 2:** `tools/call` mit validen Args — Gibt JSON-Ergebnis zurück
3. **Test 3:** `tools/call` mit ungültigen Args — Gibt Fehler zurück
4. **Test 4:** Integration mit opencode — Tool ist verfügbar

---

## Umsetzung

1. ✅ Erstelle `cmd/discover/mcp_server.go`
2. ✅ Erstelle `cmd/execute/mcp_server.go`
3. ✅ Erstelle `cmd/map/mcp_server.go`
4. ✅ Erstelle `cmd/grasp/mcp_server.go`
5. ✅ Erstelle `cmd/scout/mcp_server.go`
6. ✅ Erstelle `cmd/harvest/mcp_server.go`
7. ✅ Erstelle `cmd/orchestrate/mcp_server.go`
8. ✅ Füge `--mcp` Flag zu jedem Tool hinzu
9. ✅ Teste jeden Server mit `echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | discover --mcp`
10. ✅ Update AGENTS.md mit MCP-Server-Anweisungen

---

## Commit-Strategie

Ein Commit pro Tool:
- `feat(mcp): add MCP server to discover tool`
- `feat(mcp): add MCP server to execute tool`
- ...

Oder ein Mega-Commit:
- `feat(mcp): add MCP servers to all 7 SIN-Code tools`

---

## Geschätzte Zeit

- Pro Tool: 30 Minuten (Server + Tests)
- Total: ~3.5 Stunden
- Plus Debugging: +1 Stunde
- **Total: ~5 Stunden**

---

## Nächste Schritte

1. Starte mit Discover-Tool (einfachstes Beispiel)
2. Kopiere Pattern für andere Tools
3. Teste mit opencode Integration
