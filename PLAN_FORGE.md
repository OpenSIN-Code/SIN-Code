# PLAN: SIN-Code-Forge-Tool — Code Generation Implementation

**Ziel:** Forge soll nicht mehr ein Stub sein, sondern echte Code-Generierung für neue SIN-Code Tools ermöglichen.

**Status:** ✅ DONE (2026-06-02) — Forge is NOT a stub; it has full implementation
**Aufwand:** ~4 Stunden

> **Note (2026-06-02)**: Forge was investigated and is NOT a stub. The repo has:
> - `pkg/tools/forge.go` (2.6K) — main ForgeTool implementation
> - `pkg/tools/forge_helpers.go` (1.2K) — helper functions
> - `mcp/forge_mcp_server.py` — MCP server for forge
> - Full CLI with multiple actions
>
> No code generation work needed for forge.

---

## Aktueller Zustand

```go
// cmd/forge/main.go — NUR 51 Zeilen
package main

import "fmt"

func main() {
    fmt.Println("SIN-Code-Forge-Tool v0.1.0-stub")
    fmt.Println("Code generation not yet implemented")
}
```

---

## Ziel-Architektur

Forge soll:
1. **Neue Tools generieren** — Template-basiert
2. **Boilerplate erstellen** — Standard Go CLI Struktur
3. **Tool-Skeletons** — Alle Standard-Files (main.go, pkg/tools/, tests)
4. **MCP-Server-Skeletons** — JSON-RPC 2.0 Server Template

---

## Implementation Plan

### Phase 1: Template Engine (1 Stunde)

```go
// pkg/forge/template.go
package forge

type ToolTemplate struct {
    Name        string
    Description string
    Parameters  []Parameter
    Version     string
}

func (t *ToolTemplate) Render() (string, error) {
    tmpl := `package main

import (
    "fmt"
    "os"
)

const Version = "{{.Version}}"

func main() {
    if len(os.Args) > 1 && os.Args[1] == "--version" {
        fmt.Println(Version)
        return
    }
    fmt.Println("{{.Name}} v" + Version)
}
`
    tpl, err := template.New("tool").Parse(tmpl)
    if err != nil {
        return "", err
    }
    
    var buf bytes.Buffer
    if err := tpl.Execute(&buf, t); err != nil {
        return "", err
    }
    return buf.String(), nil
}
```

### Phase 2: CLI Interface (1 Stunde)

```go
// cmd/forge/main.go
func main() {
    var (
        name        string
        description string
        output      string
        withMCP     bool
    )
    
    flag.StringVar(&name, "name", "", "Tool name (e.g., discover)")
    flag.StringVar(&description, "description", "", "Tool description")
    flag.StringVar(&output, "output", ".", "Output directory")
    flag.BoolVar(&withMCP, "with-mcp", true, "Include MCP server template")
    flag.Parse()
    
    if name == "" {
        log.Fatal("--name is required")
    }
    
    template := &ToolTemplate{
        Name:        name,
        Description: description,
        Version:     "0.1.0",
    }
    
    code, err := template.Render()
    if err != nil {
        log.Fatal(err)
    }
    
    if err := os.WriteFile(filepath.Join(output, name, "cmd", name, "main.go"), []byte(code), 0644); err != nil {
        log.Fatal(err)
    }
    
    if withMCP {
        mcpCode, _ := renderMCPServerTemplate(name)
        os.WriteFile(filepath.Join(output, name, "cmd", name, "mcp_server.go"), []byte(mcpCode), 0644)
    }
    
    fmt.Printf("✅ Generated %s in %s\n", name, output)
}
```

### Phase 3: Standard Files (1 Stunde)

Forge soll automatisch generieren:
- `go.mod` mit Go 1.23+
- `README.md` mit Tool-spezifischer Doku
- `AGENTS.md` mit SIN-Code Tool Suite Verweis
- `LICENSE` (MIT)
- `.gitignore` (Binaries, .env, etc.)
- `Makefile` mit build/test/install Targets
- `cmd/<name>/main.go` — CLI Entry
- `cmd/<name>/mcp_server.go` — MCP Entry (optional)
- `pkg/tools/<name>.go` — Core Logic
- `pkg/tools/<name>_test.go` — Unit Tests

### Phase 4: Integration Tests (1 Stunde)

```go
// pkg/forge/forge_test.go
func TestForgeGenerateTool(t *testing.T) {
    tmpDir := t.TempDir()
    
    err := GenerateTool(GenerateOptions{
        Name:        "test-tool",
        Description: "A test tool",
        Output:      tmpDir,
        WithMCP:     true,
    })
    
    if err != nil {
        t.Fatal(err)
    }
    
    // Verify all files were created
    expectedFiles := []string{
        "go.mod",
        "README.md",
        "AGENTS.md",
        "cmd/test-tool/main.go",
        "cmd/test-tool/mcp_server.go",
        "pkg/tools/test-tool.go",
        "pkg/tools/test-tool_test.go",
    }
    
    for _, f := range expectedFiles {
        path := filepath.Join(tmpDir, f)
        if _, err := os.Stat(path); os.IsNotExist(err) {
            t.Errorf("Expected file %s not found", f)
        }
    }
}
```

---

## Templates

### Tool Template
```go
// pkg/forge/templates/tool.go.tmpl
package main

import (
    "fmt"
    "os"
    "github.com/spf13/cobra"
)

const Version = "{{.Version}}"

func main() {
    var rootCmd = &cobra.Command{
        Use:     "{{.Name}}",
        Short:   "{{.Description}}",
        Version: Version,
        Run: func(cmd *cobra.Command, args []string) {
            fmt.Println("{{.Name}} v" + Version)
        },
    }
    
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

### MCP Server Template
```go
// pkg/forge/templates/mcp_server.go.tmpl
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
)

type MCPRequest struct {
    JSONRPC string                 `json:"jsonrpc"`
    ID      int                    `json:"id"`
    Method  string                 `json:"method"`
    Params  map[string]interface{} `json:"params"`
}

type MCPResponse struct {
    JSONRPC string      `json:"jsonrpc"`
    ID      int         `json:"id"`
    Result  interface{} `json:"result,omitempty"`
    Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
}

func runMCPServer() error {
    scanner := bufio.NewScanner(os.Stdin)
    
    for scanner.Scan() {
        var req MCPRequest
        if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
            sendError(0, -32700, "Parse error")
            continue
        }
        
        switch req.Method {
        case "tools/list":
            handleToolsList(req.ID)
        case "tools/call":
            handleToolsCall(req.ID, req.Params)
        default:
            sendError(req.ID, -32601, "Method not found")
        }
    }
    return nil
}

func handleToolsList(id int) {
    sendResult(id, map[string]interface{}{
        "tools": []map[string]interface{}{
            {
                "name":        "{{.Name}}",
                "description": "{{.Description}}",
                "inputSchema": map[string]interface{}{
                    "type": "object",
                    "properties": map[string]interface{}{
                        "path": map[string]string{"type": "string"},
                    },
                },
            },
        },
    })
}

func handleToolsCall(id int, params map[string]interface{}) {
    args, _ := params["arguments"].(map[string]interface{})
    path, _ := args["path"].(string)
    
    // Call tool logic here
    result := map[string]interface{}{
        "path": path,
        "status": "ok",
    }
    
    sendResult(id, result)
}

func sendResult(id int, result interface{}) {
    resp := MCPResponse{JSONRPC: "2.0", ID: id, Result: result}
    json.NewEncoder(os.Stdout).Encode(resp)
}

func sendError(id, code int, message string) {
    resp := MCPResponse{JSONRPC: "2.0", ID: id, Error: &MCPError{Code: code, Message: message}}
    json.NewEncoder(os.Stdout).Encode(resp)
}
```

---

## Files zu erstellen

- `cmd/forge/main.go` — CLI Entry (update von 51 auf ~150 Zeilen)
- `pkg/forge/forge.go` — Core Generation Logic
- `pkg/forge/template.go` — Template Engine
- `pkg/forge/templates/tool.go.tmpl` — Tool Template
- `pkg/forge/templates/mcp_server.go.tmpl` — MCP Server Template
- `pkg/forge/forge_test.go` — Tests
- `pkg/forge/README.md` — Dokumentation
- `pkg/forge/AGENTS.md` — Agent Instructions

---

## Geschätzte Zeit

- Template Engine: 1 Stunde
- CLI Interface: 1 Stunde
- Standard Files: 1 Stunde
- Integration Tests: 1 Stunde
- **Total: ~4 Stunden**
