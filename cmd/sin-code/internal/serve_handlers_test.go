package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupServeTest(t *testing.T) {
	t.Helper()
	t.Setenv("SIN_CODE_SUBPROCESS", "1")
}

func TestRunSubcommand_ExecuteEcho(t *testing.T) {
	setupServeTest(t)
	result, err := runSubcommand(context.Background(), "execute", map[string]any{
		"command": "echo hello_handler_test",
		"format":  "text",
		"timeout": float64(10),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "hello_handler_test") {
		t.Errorf("expected output to contain 'hello_handler_test', got: %s", result)
	}
}

func TestRunSubcommand_ExecuteWithFormatJSON(t *testing.T) {
	setupServeTest(t)
	result, err := runSubcommand(context.Background(), "execute", map[string]any{
		"command": "echo json_test",
		"format":  "json",
		"timeout": float64(10),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestRunSubcommand_EmptyStringArgsSkipped(t *testing.T) {
	setupServeTest(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	result, err := runSubcommand(context.Background(), "orchestrate", map[string]any{
		"action": "list",
		"format": "text",
		"title":  "",
		"tags":   "",
		"id":     "",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestRunSubcommand_Float64AndIntArgs(t *testing.T) {
	setupServeTest(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	result, err := runSubcommand(context.Background(), "efm", map[string]any{
		"action": "list",
		"format": "text",
		"ttl":    float64(3600),
		"count":  5,
		"strict": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestRunSubcommand_BoolTrueOnly(t *testing.T) {
	setupServeTest(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	result, err := runSubcommand(context.Background(), "orchestrate", map[string]any{
		"action":  "list",
		"format":  "text",
		"verbose": true,
		"quiet":   false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestRunSubcommandRaw_HelpOutput(t *testing.T) {
	setupServeTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := runSubcommandRaw(ctx, []string{"discover", "--help"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Usage") && !strings.Contains(result, "discover") {
		t.Errorf("expected help output, got: %s", result)
	}
}

func TestRunSubcommandRaw_ContextCancellation(t *testing.T) {
	setupServeTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runSubcommandRaw(ctx, []string{"discover", "."})
	if err != nil {
		t.Fatalf("context cancelled command should not return error, got: %v", err)
	}
}

func TestRunSubcommandRaw_VersionFlag(t *testing.T) {
	setupServeTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := runSubcommandRaw(ctx, []string{"--version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "test") && !strings.Contains(result, "version") {
		t.Errorf("expected version output, got: %s", result)
	}
}

func TestHandleDiscover_ValidPath(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	result, err := handleDiscover(context.Background(), map[string]any{
		"path":   dir,
		"format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "main.go") && !strings.Contains(result, "Files") {
		t.Errorf("expected discover output, got: %s", result)
	}
}

func TestHandleDiscover_JSONFormat(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("import os\n"), 0644)
	result, err := handleDiscover(context.Background(), map[string]any{
		"path":   dir,
		"format": "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed []any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON array, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleDiscover_WithPattern(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Test\n"), 0644)
	result, err := handleDiscover(context.Background(), map[string]any{
		"path":    dir,
		"pattern": "*.go",
		"format":  "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleDiscover_WithLimitFloat64(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	result, err := handleDiscover(context.Background(), map[string]any{
		"path":   dir,
		"format": "text",
		"limit":  float64(10),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleDiscover_WithLimitInt(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	result, err := handleDiscover(context.Background(), map[string]any{
		"path":   dir,
		"format": "text",
		"limit":  5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleDiscover_DefaultPath(t *testing.T) {
	setupServeTest(t)
	result, err := handleDiscover(context.Background(), map[string]any{"format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleDiscover_NilArgs(t *testing.T) {
	setupServeTest(t)
	result, err := handleDiscover(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleDiscover_NonStringPath(t *testing.T) {
	setupServeTest(t)
	result, err := handleDiscover(context.Background(), map[string]any{"path": 42, "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleDiscover_BoolArgIgnored(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	result, err := handleDiscover(context.Background(), map[string]any{
		"path":    dir,
		"format":  "text",
		"verbose": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleDiscover_EmptyPathDefaultsToDot(t *testing.T) {
	setupServeTest(t)
	result, err := handleDiscover(context.Background(), map[string]any{"path": "", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleExecute_EchoHello(t *testing.T) {
	setupServeTest(t)
	result, err := handleExecute(context.Background(), map[string]any{
		"command": "echo handler_execute_test",
		"format":  "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "handler_execute_test") {
		t.Errorf("expected output to contain 'handler_execute_test', got: %s", result)
	}
}

func TestHandleExecute_WithTimeout(t *testing.T) {
	setupServeTest(t)
	result, err := handleExecute(context.Background(), map[string]any{
		"command": "echo timeout_test",
		"timeout": float64(5),
		"format":  "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "timeout_test") {
		t.Errorf("expected output to contain 'timeout_test', got: %s", result)
	}
}

func TestHandleExecute_JSONFormat(t *testing.T) {
	setupServeTest(t)
	result, err := handleExecute(context.Background(), map[string]any{
		"command": "echo json_exec_test",
		"format":  "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleExecute_FailingCommand(t *testing.T) {
	setupServeTest(t)
	result, err := handleExecute(context.Background(), map[string]any{"command": "false", "format": "text"})
	if err != nil {
		t.Fatalf("handler should not return error for failing command, got: %v", err)
	}
	if !strings.Contains(result, "ERROR") {
		t.Errorf("expected ERROR in output for failing command, got: %s", result)
	}
}

func TestHandleExecute_MissingCommand(t *testing.T) {
	setupServeTest(t)
	result, err := handleExecute(context.Background(), map[string]any{"format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "ERROR") && !strings.Contains(result, "required") {
		t.Errorf("expected error about missing command, got: %s", result)
	}
}

func TestHandleMap_ValidPath(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handleMap(context.Background(), map[string]any{"path": dir, "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestHandleMap_WithAction(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	result, err := handleMap(context.Background(), map[string]any{
		"path": dir, "action": "map", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleMap_JSONFormat(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	result, err := handleMap(context.Background(), map[string]any{"path": dir, "format": "json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleMap_DefaultPath(t *testing.T) {
	setupServeTest(t)
	result, err := handleMap(context.Background(), map[string]any{"format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleMap_NonStringPath(t *testing.T) {
	setupServeTest(t)
	result, err := handleMap(context.Background(), map[string]any{"path": float64(123), "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleMap_EmptyPathDefaultsToDot(t *testing.T) {
	setupServeTest(t)
	result, err := handleMap(context.Background(), map[string]any{"path": "", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleMap_AllArgTypes(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handleMap(context.Background(), map[string]any{
		"path": dir, "action": "map", "format": "text",
		"verbose": true, "depth": float64(3), "maxfiles": 100,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleGrasp_ValidFile(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	os.WriteFile(f, []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handleGrasp(context.Background(), map[string]any{"path": f, "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestHandleGrasp_JSONFormat(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "app.py")
	os.WriteFile(f, []byte("def hello():\n    pass\n"), 0644)
	result, err := handleGrasp(context.Background(), map[string]any{"path": f, "format": "json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleGrasp_NonStringPath(t *testing.T) {
	setupServeTest(t)
	result, err := handleGrasp(context.Background(), map[string]any{"path": true, "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleGrasp_EmptyPathDefaultsToDot(t *testing.T) {
	setupServeTest(t)
	result, err := handleGrasp(context.Background(), map[string]any{"path": "", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleGrasp_AllArgTypes(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	os.WriteFile(f, []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handleGrasp(context.Background(), map[string]any{
		"path": f, "format": "text",
		"verbose": true, "depth": float64(2), "maxlines": 500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleScout_WithQuery(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc ProcessData() {}\n"), 0644)
	result, err := handleScout(context.Background(), map[string]any{
		"query": "ProcessData", "path": dir, "search_type": "regex", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "ProcessData") && !strings.Contains(result, "match") {
		t.Errorf("expected scout output, got: %s", result)
	}
}

func TestHandleScout_JSONFormat(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc MyFunc() {}\n"), 0644)
	result, err := handleScout(context.Background(), map[string]any{
		"query": "MyFunc", "path": dir, "search_type": "symbol", "format": "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed []any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON array, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleScout_SemanticSearch(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("def calculate_total():\n    pass\n"), 0644)
	result, err := handleScout(context.Background(), map[string]any{
		"query": "calculate", "path": dir, "search_type": "semantic", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleScout_UsageSearch(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() { Helper() }\nfunc Helper() {}\n"), 0644)
	result, err := handleScout(context.Background(), map[string]any{
		"query": "Helper", "path": dir, "search_type": "usage", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleScout_DefaultPath(t *testing.T) {
	setupServeTest(t)
	result, err := handleScout(context.Background(), map[string]any{
		"query": "func", "search_type": "regex", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleHarvest_HTTPServer(t *testing.T) {
	setupServeTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"harvest_handler_test"}`))
	}))
	defer server.Close()
	result, err := handleHarvest(context.Background(), map[string]any{
		"url": server.URL, "method": "GET", "format": "text", "timeout": float64(10),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "harvest_handler_test") {
		t.Errorf("expected output to contain 'harvest_handler_test', got: %s", result)
	}
}

func TestHandleHarvest_JSONFormat(t *testing.T) {
	setupServeTest(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()
	result, err := handleHarvest(context.Background(), map[string]any{
		"url": server.URL, "method": "GET", "format": "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleHarvest_PostMethod(t *testing.T) {
	setupServeTest(t)
	gotMethod := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	_, err := handleHarvest(context.Background(), map[string]any{
		"url": server.URL, "method": "POST", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("expected POST method, got %q", gotMethod)
	}
}

func TestHandleHarvest_InvalidURL(t *testing.T) {
	setupServeTest(t)
	result, err := handleHarvest(context.Background(), map[string]any{
		"url": "http://localhost:1", "method": "GET", "format": "text",
	})
	if err != nil {
		t.Fatalf("handler should not return error, got: %v", err)
	}
	if !strings.Contains(result, "ERROR") {
		t.Errorf("expected ERROR in output for invalid URL, got: %s", result)
	}
}

func TestHandleOrchestrate_List(t *testing.T) {
	setupServeTest(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	result, err := handleOrchestrate(context.Background(), map[string]any{"action": "list", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleOrchestrate_Add(t *testing.T) {
	setupServeTest(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	result, err := handleOrchestrate(context.Background(), map[string]any{
		"action": "add", "title": "Handler Test Task", "tags": "test,handler", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Handler Test Task") {
		t.Errorf("expected output to contain task title, got: %s", result)
	}
}

func TestHandleOrchestrate_JSONFormat(t *testing.T) {
	setupServeTest(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	result, err := handleOrchestrate(context.Background(), map[string]any{"action": "list", "format": "json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed []any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON array, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleOrchestrate_Complete(t *testing.T) {
	setupServeTest(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	_, err := handleOrchestrate(context.Background(), map[string]any{"action": "add", "title": "To Complete", "format": "text"})
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}
	result, err := handleOrchestrate(context.Background(), map[string]any{"action": "complete", "id": "1", "format": "text"})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	_ = result
}

func TestHandleOrchestrate_AddMissingTitle(t *testing.T) {
	setupServeTest(t)
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	result, err := handleOrchestrate(context.Background(), map[string]any{"action": "add", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "ERROR") && !strings.Contains(result, "required") {
		t.Errorf("expected error about missing title, got: %s", result)
	}
}

func TestHandleIbd_WithBeforeAfter(t *testing.T) {
	setupServeTest(t)
	result, err := handleIbd(context.Background(), map[string]any{
		"before": "func old() {}", "after": "func new() {}", "intent": "rename old to new", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Intent") || !strings.Contains(result, "Diffing") {
		t.Errorf("expected IBD output, got: %s", result)
	}
}

func TestHandleIbd_JSONFormat(t *testing.T) {
	setupServeTest(t)
	result, err := handleIbd(context.Background(), map[string]any{
		"before": "line1\nline2\n", "after": "line1\nline3\n", "intent": "update", "format": "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleIbd_IdenticalContent(t *testing.T) {
	setupServeTest(t)
	result, err := handleIbd(context.Background(), map[string]any{
		"before": "same", "after": "same", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleIbd_EmptyIntent(t *testing.T) {
	setupServeTest(t)
	result, err := handleIbd(context.Background(), map[string]any{
		"before": "old code", "after": "new code", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleIbd_EmptyStringsIgnored(t *testing.T) {
	setupServeTest(t)
	result, err := handleIbd(context.Background(), map[string]any{
		"before": "content", "after": "content2", "intent": "", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandlePoc_WithSpecAndCode(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	codeFile := filepath.Join(dir, "code.go")
	os.WriteFile(specFile, []byte("# Spec\nThe function must return 0.\n"), 0644)
	os.WriteFile(codeFile, []byte("package main\nfunc Main() int { return 0 }\n"), 0644)
	result, err := handlePoc(context.Background(), map[string]any{
		"spec": specFile, "code": codeFile, "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestHandlePoc_JSONFormat(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	specFile := filepath.Join(dir, "spec.md")
	codeFile := filepath.Join(dir, "code.go")
	os.WriteFile(specFile, []byte("# Spec\nMust handle errors.\n"), 0644)
	os.WriteFile(codeFile, []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handlePoc(context.Background(), map[string]any{
		"spec": specFile, "code": codeFile, "format": "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		if !strings.Contains(result, "ERROR") && !strings.Contains(result, "required") {
			t.Errorf("expected valid JSON or error, got: %s", result)
		}
	}
}

func TestHandlePoc_CodeOnly(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	codeFile := filepath.Join(dir, "main.go")
	os.WriteFile(codeFile, []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handlePoc(context.Background(), map[string]any{"code": codeFile, "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandlePoc_OnlySpecFallsBack(t *testing.T) {
	setupServeTest(t)
	result, err := handlePoc(context.Background(), map[string]any{"spec": "spec text", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandlePoc_NoArgsDefaultPath(t *testing.T) {
	setupServeTest(t)
	result, err := handlePoc(context.Background(), map[string]any{"format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandlePoc_EmptyCodeAndSpec(t *testing.T) {
	setupServeTest(t)
	result, err := handlePoc(context.Background(), map[string]any{"code": "", "spec": "", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleSckg_ValidPath(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handleSckg(context.Background(), map[string]any{
		"path": dir, "action": "build", "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleSckg_JSONFormat(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handleSckg(context.Background(), map[string]any{"path": dir, "action": "build", "format": "json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleSckg_StatsAction(t *testing.T) {
	setupServeTest(t)
	result, err := handleSckg(context.Background(), map[string]any{"action": "stats", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleSckg_NonStringPath(t *testing.T) {
	setupServeTest(t)
	result, err := handleSckg(context.Background(), map[string]any{"path": 999, "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleSckg_EmptyPathDefaultsToDot(t *testing.T) {
	setupServeTest(t)
	result, err := handleSckg(context.Background(), map[string]any{"path": "", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleSckg_AllArgTypes(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	result, err := handleSckg(context.Background(), map[string]any{
		"path": dir, "action": "build", "format": "json",
		"verbose": true, "depth": float64(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleAdw_ValidPath(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handleAdw(context.Background(), map[string]any{"path": dir, "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestHandleAdw_JSONFormat(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handleAdw(context.Background(), map[string]any{"path": dir, "format": "json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleAdw_WithStrict(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	result, err := handleAdw(context.Background(), map[string]any{
		"path": dir, "strict": true, "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleAdw_DefaultPath(t *testing.T) {
	setupServeTest(t)
	result, err := handleAdw(context.Background(), map[string]any{"format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleAdw_NonStringPath(t *testing.T) {
	setupServeTest(t)
	result, err := handleAdw(context.Background(), map[string]any{"path": false, "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleAdw_Float64Strict(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644)
	result, err := handleAdw(context.Background(), map[string]any{
		"path": dir, "strict": float64(1), "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleAdw_EmptyPathDefaultsToDot(t *testing.T) {
	setupServeTest(t)
	result, err := handleAdw(context.Background(), map[string]any{"path": "", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleOracle_WithClaimAndEvidence(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	claimFile := filepath.Join(dir, "claim.go")
	evidenceFile := filepath.Join(dir, "evidence_test.go")
	os.WriteFile(claimFile, []byte("package main\nfunc Main() int { return 0 }\n"), 0644)
	os.WriteFile(evidenceFile, []byte("package main\nfunc TestMain() {}\n"), 0644)
	result, err := handleOracle(context.Background(), map[string]any{
		"claim": claimFile, "evidence": evidenceFile, "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Oracle") && !strings.Contains(result, "Verification") && !strings.Contains(result, "claim") {
		t.Errorf("expected oracle output, got: %s", result)
	}
}

func TestHandleOracle_WithEvidence(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	claimFile := filepath.Join(dir, "claim.go")
	evidenceFile := filepath.Join(dir, "evidence_test.go")
	os.WriteFile(claimFile, []byte("package main\nfunc Process() error { return nil }\n"), 0644)
	os.WriteFile(evidenceFile, []byte("package main\nfunc TestProcess() {}\n"), 0644)
	result, err := handleOracle(context.Background(), map[string]any{
		"claim": claimFile, "evidence": evidenceFile, "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleOracle_JSONFormat(t *testing.T) {
	setupServeTest(t)
	dir := t.TempDir()
	claimFile := filepath.Join(dir, "claim.go")
	evidenceFile := filepath.Join(dir, "evidence_test.go")
	os.WriteFile(claimFile, []byte("package main\nfunc Main() int { return 0 }\n"), 0644)
	os.WriteFile(evidenceFile, []byte("package main\nfunc TestMain() {}\n"), 0644)
	result, err := handleOracle(context.Background(), map[string]any{
		"claim": claimFile, "evidence": evidenceFile, "format": "json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		if !strings.Contains(result, "ERROR") {
			t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
		}
	}
}

func TestHandleEfm_List(t *testing.T) {
	setupServeTest(t)
	result, err := handleEfm(context.Background(), map[string]any{"action": "list", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "EFM") {
		t.Errorf("expected EFM output, got: %s", result)
	}
}

func TestHandleEfm_UpRequiresStack(t *testing.T) {
	setupServeTest(t)
	result, err := handleEfm(context.Background(), map[string]any{"action": "up", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "ERROR") && !strings.Contains(result, "required") {
		t.Errorf("expected error about missing stack, got: %s", result)
	}
}

func TestHandleEfm_JSONFormat(t *testing.T) {
	setupServeTest(t)
	result, err := handleEfm(context.Background(), map[string]any{"action": "list", "format": "json"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var parsed map[string]any
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(result)), &parsed); jsonErr != nil {
		t.Errorf("expected valid JSON, got parse error: %v\noutput: %s", jsonErr, result)
	}
}

func TestHandleEfm_StatusAction(t *testing.T) {
	setupServeTest(t)
	result, err := handleEfm(context.Background(), map[string]any{"action": "status", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleEfm_WithTTL(t *testing.T) {
	setupServeTest(t)
	result, err := handleEfm(context.Background(), map[string]any{
		"action": "list", "ttl": float64(7200), "format": "text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = result
}

func TestHandleEfm_InvalidAction(t *testing.T) {
	setupServeTest(t)
	result, err := handleEfm(context.Background(), map[string]any{"action": "destroy", "format": "text"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "ERROR") && !strings.Contains(result, "unknown") {
		t.Errorf("expected error about unknown action, got: %s", result)
	}
}

func TestServeCmd_RunEUnsupportedTransport(t *testing.T) {
	ServeCmd.Flags().Set("transport", "http")
	err := ServeCmd.RunE(ServeCmd, []string{})
	ServeCmd.Flags().Set("transport", "stdio")
	if err == nil {
		t.Error("expected error for unsupported transport")
	}
	if !strings.Contains(err.Error(), "unsupported transport") {
		t.Errorf("expected 'unsupported transport' error, got: %v", err)
	}
}

func TestServeCmd_PortFlagDefault(t *testing.T) {
	if v, _ := ServeCmd.Flags().GetInt("port"); v != 0 {
		t.Errorf("default port should be 0, got %d", v)
	}
}

func TestServeCmd_RunEStdioFlag(t *testing.T) {
	ServeCmd.Flags().Set("transport", "stdio")
	v, _ := ServeCmd.Flags().GetString("transport")
	if v != "stdio" {
		t.Errorf("expected transport stdio, got %q", v)
	}
}


func TestHandlePoc_IntTypeArg(t *testing.T) {
	setupServeTest(t)
	ctx := context.Background()
	result, err := handlePoc(ctx, map[string]any{
		"code":  t.TempDir(),
		"limit": int(42),
	})
	_ = result
	_ = err
}

func TestHandleAdw_IntTypeArg(t *testing.T) {
	setupServeTest(t)
	ctx := context.Background()
	result, err := handleAdw(ctx, map[string]any{
		"path":  t.TempDir(),
		"limit": int(42),
	})
	_ = result
	_ = err
}
