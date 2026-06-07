// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the sckg (Semantic Codebase Knowledge Graphs) subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"fmt"
	"testing"
)

func TestBuildGraph_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	if len(graph.Nodes) != 0 {
		t.Errorf("expected 0 nodes for empty dir, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected 0 edges for empty dir, got %d", len(graph.Edges))
	}
}

func TestBuildGraph_GoFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nimport \"fmt\"\nfunc Hello() { fmt.Println(\"hi\") }\nfunc World() {}\n"), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	if len(graph.Nodes) == 0 {
		t.Fatal("expected at least 1 node for Go file")
	}
	if len(graph.Edges) == 0 {
		t.Fatal("expected at least 1 edge for Go file with imports")
	}

	foundFile := false
	foundFunc := false
	for _, node := range graph.Nodes {
		if node.Type == "file" && strings.HasSuffix(node.Path, "main.go") {
			foundFile = true
		}
		if node.Type == "function" && (node.Name == "Hello" || node.Name == "World") {
			foundFunc = true
		}
	}
	if !foundFile {
		t.Error("expected file node for main.go")
	}
	if !foundFunc {
		t.Errorf("expected function nodes for Hello/World, got %v", graph.Nodes)
	}
}

func TestBuildGraph_PythonFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("def hello():\n    pass\nclass MyClass:\n    def method(self):\n        pass\n"), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	foundFunc := false
	foundClass := false
	for _, node := range graph.Nodes {
		if node.Type == "function" && node.Name == "hello" {
			foundFunc = true
		}
		if node.Type == "class" && node.Name == "MyClass" {
			foundClass = true
		}
	}
	if !foundFunc {
		t.Errorf("expected function node 'hello', got %v", graph.Nodes)
	}
	if !foundClass {
		t.Errorf("expected class node 'MyClass', got %v", graph.Nodes)
	}
}

func TestBuildGraph_JSFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("function hello() { return 1; }\nclass MyClass {}\n"), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	foundFunc := false
	foundClass := false
	for _, node := range graph.Nodes {
		if node.Type == "function" && node.Name == "hello" {
			foundFunc = true
		}
		if node.Type == "class" && node.Name == "MyClass" {
			foundClass = true
		}
	}
	if !foundFunc {
		t.Errorf("expected function node 'hello', got %v", graph.Nodes)
	}
	if !foundClass {
		t.Errorf("expected class node 'MyClass', got %v", graph.Nodes)
	}
}

func TestBuildGraph_SkipsIgnoredDirs(t *testing.T) {
	dir := t.TempDir()
	nodeModules := filepath.Join(dir, "node_modules")
	os.MkdirAll(nodeModules, 0755)
	os.WriteFile(filepath.Join(nodeModules, "lib.js"), []byte("function hidden() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("function visible() {}\n"), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	for _, node := range graph.Nodes {
		if strings.Contains(node.Path, "node_modules") {
			t.Errorf("expected node_modules to be skipped, found node: %v", node)
		}
	}
}

func TestBuildGraph_SkipsNonCodeFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Hello"), 0644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	for _, node := range graph.Nodes {
		if node.Type == "file" {
			t.Errorf("expected no file nodes for md/txt, found: %v", node)
		}
	}
}

func TestBuildGraph_MixedLanguages(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc GoFunc() {}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "app.py"), []byte("def py_func():\n    pass\n"), 0644)
	os.WriteFile(filepath.Join(dir, "index.js"), []byte("function jsFunc() {}\n"), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	langs := make(map[string]bool)
	for _, node := range graph.Nodes {
		if node.Language != "" {
			langs[node.Language] = true
		}
	}
	if !langs["go"] || !langs["python"] || !langs["javascript"] {
		t.Errorf("expected go+python+javascript languages, got %v", langs)
	}
}

func TestBuildGraph_InvalidGoFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "bad.go"), []byte("not valid go syntax!!!"), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	foundFile := false
	for _, node := range graph.Nodes {
		if node.Type == "file" && strings.HasSuffix(node.Path, "bad.go") {
			foundFile = true
		}
	}
	if !foundFile {
		t.Error("expected file node even for invalid Go syntax")
	}
}

func TestQueryGraph(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
			{ID: "func:main.go:Hello", Type: "function", Name: "Hello", Path: "main.go", Line: 5},
			{ID: "func:main.go:World", Type: "function", Name: "World", Path: "main.go", Line: 10},
			{ID: "dep:fmt", Type: "module", Name: "fmt"},
		},
		Edges: []sckgEdge{
			{Source: "file:main.go", Target: "func:main.go:Hello", Type: "contains"},
			{Source: "file:main.go", Target: "func:main.go:World", Type: "contains"},
			{Source: "file:main.go", Target: "dep:fmt", Type: "imports"},
		},
	}

	result := queryGraph(graph, "hello")
	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 match for 'hello', got %d", len(result.Matches))
	}
	if result.Matches[0].Name != "Hello" {
		t.Errorf("expected match 'Hello', got %q", result.Matches[0].Name)
	}
	if result.Total < 2 {
		t.Errorf("expected at least 2 total (match + related), got %d", result.Total)
	}
}

func TestQueryGraph_NoMatch(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
		},
		Edges: []sckgEdge{},
	}

	result := queryGraph(graph, "nonexistent")
	if len(result.Matches) != 0 {
		t.Errorf("expected 0 matches for 'nonexistent', got %d", len(result.Matches))
	}
	if result.Total != 0 {
		t.Errorf("expected 0 total, got %d", result.Total)
	}
}

func TestQueryGraph_ByType(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
			{ID: "func:main.go:Hello", Type: "function", Name: "Hello", Path: "main.go"},
			{ID: "type:main.go:Config", Type: "type", Name: "Config", Path: "main.go"},
		},
		Edges: []sckgEdge{},
	}

	result := queryGraph(graph, "function")
	if len(result.Matches) < 1 {
		t.Errorf("expected at least 1 match for type 'function', got %d", len(result.Matches))
	}
}

func TestGraphStats(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:a.go", Type: "file", Name: "a.go"},
			{ID: "file:b.go", Type: "file", Name: "b.go"},
			{ID: "func:a.go:Hello", Type: "function", Name: "Hello"},
			{ID: "dep:fmt", Type: "module", Name: "fmt"},
		},
		Edges: []sckgEdge{
			{Source: "file:a.go", Target: "func:a.go:Hello", Type: "contains"},
			{Source: "file:a.go", Target: "dep:fmt", Type: "imports"},
			{Source: "file:b.go", Target: "dep:fmt", Type: "imports"},
		},
	}

	stats := graphStats(graph)
	if stats.TotalNodes != 4 {
		t.Errorf("expected 4 total nodes, got %d", stats.TotalNodes)
	}
	if stats.TotalEdges != 3 {
		t.Errorf("expected 3 total edges, got %d", stats.TotalEdges)
	}
	if stats.NodeTypes["file"] != 2 {
		t.Errorf("expected 2 file nodes, got %d", stats.NodeTypes["file"])
	}
	if stats.EdgeTypes["imports"] != 2 {
		t.Errorf("expected 2 import edges, got %d", stats.EdgeTypes["imports"])
	}
	if len(stats.TopImports) == 0 || stats.TopImports[0].Name != "dep:fmt" {
		t.Errorf("expected fmt as top import, got %v", stats.TopImports)
	}
}

func TestGraphStats_OrphanNodes(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:a.go", Type: "file", Name: "a.go"},
			{ID: "func:a.go:Hello", Type: "function", Name: "Hello"},
			{ID: "func:a.go:Orphan", Type: "function", Name: "Orphan"},
		},
		Edges: []sckgEdge{
			{Source: "file:a.go", Target: "func:a.go:Hello", Type: "contains"},
		},
	}

	stats := graphStats(graph)
	found := false
	for _, o := range stats.OrphanNodes {
		if o == "Orphan" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Orphan' in orphan nodes, got %v", stats.OrphanNodes)
	}
}

func TestOutputTextSCKGBuild(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
			{ID: "func:main.go:Hello", Type: "function", Name: "Hello", Path: "main.go", Line: 5},
		},
		Edges: []sckgEdge{
			{Source: "file:main.go", Target: "func:main.go:Hello", Type: "contains"},
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextSCKGBuild(graph); err != nil {
		t.Fatalf("outputTextSCKGBuild failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "SCKG Knowledge Graph Built") {
		t.Errorf("expected header in output, got %q", out)
	}
	if !strings.Contains(out, "Nodes: 2") {
		t.Errorf("expected 'Nodes: 2' in output, got %q", out)
	}
}

func TestOutputTextSCKGQuery(t *testing.T) {
	result := &queryResult{
		Query: "hello",
		Matches: []sckgNode{
			{ID: "func:main.go:Hello", Type: "function", Name: "Hello", Path: "main.go", Line: 5},
		},
		Related: []sckgNode{
			{ID: "file:main.go", Type: "file", Name: "main.go", Path: "main.go"},
		},
		Total: 2,
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextSCKGQuery(result); err != nil {
		t.Fatalf("outputTextSCKGQuery failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Query: hello") {
		t.Errorf("expected 'Query: hello' in output, got %q", out)
	}
	if !strings.Contains(out, "Hello") {
		t.Errorf("expected 'Hello' in matches, got %q", out)
	}
}

func TestOutputTextSCKGStats(t *testing.T) {
	stats := &sckgStats{
		TotalNodes: 10,
		TotalEdges: 5,
		NodeTypes:  map[string]int{"file": 3, "function": 7},
		EdgeTypes:  map[string]int{"contains": 5},
		TopImports: []importCount{{Name: "fmt", Count: 3}},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextSCKGStats(stats); err != nil {
		t.Fatalf("outputTextSCKGStats failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "SCKG Graph Statistics") {
		t.Errorf("expected header in output, got %q", out)
	}
	if !strings.Contains(out, "Total nodes: 10") {
		t.Errorf("expected 'Total nodes: 10' in output, got %q", out)
	}
}

func TestSckgCmd_BuildWithGoFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0644)

	sckgAction = "build"
	sckgQuery = ""
	sckgFormat = "text"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := SckgCmd.RunE(SckgCmd, []string{dir})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SckgCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "SCKG Knowledge Graph Built") {
		t.Errorf("expected build header in output, got %q", out)
	}
}

func TestSckgCmd_ActionBuildJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0644)

	sckgAction = "build"
	sckgQuery = ""
	sckgFormat = "json"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := SckgCmd.RunE(SckgCmd, []string{dir})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SckgCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var graph sckgGraph
	if err := json.Unmarshal([]byte(out), &graph); err != nil {
		t.Fatalf("expected valid JSON output, got parse error: %v", err)
	}
}

func TestSckgCmd_ActionQueryRequiresQuery(t *testing.T) {
	dir := t.TempDir()
	sckgAction = "query"
	sckgQuery = ""
	sckgFormat = "text"

	err := SckgCmd.RunE(SckgCmd, []string{dir})
	if err == nil {
		t.Error("expected error when --query is missing for action=query")
	}
}

func TestSckgCmd_ActionQuery(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello() {}\nfunc World() {}\n"), 0644)

	sckgAction = "query"
	sckgQuery = "hello"
	sckgFormat = "text"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := SckgCmd.RunE(SckgCmd, []string{dir})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SckgCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "Query: hello") {
		t.Errorf("expected 'Query: hello' in output, got %q", out)
	}
}

func TestSckgCmd_ActionStats(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0644)

	sckgAction = "stats"
	sckgQuery = ""
	sckgFormat = "text"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := SckgCmd.RunE(SckgCmd, []string{dir})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SckgCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "SCKG Graph Statistics") {
		t.Errorf("expected stats header in output, got %q", out)
	}
}

func TestSckgCmd_ActionExport(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0644)

	sckgAction = "export"
	sckgQuery = ""
	sckgFormat = "text"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := SckgCmd.RunE(SckgCmd, []string{dir})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SckgCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var graph sckgGraph
	if err := json.Unmarshal([]byte(out), &graph); err != nil {
		t.Fatalf("expected valid JSON export, got parse error: %v", err)
	}
}

func TestSckgCmd_InvalidAction(t *testing.T) {
	dir := t.TempDir()
	sckgAction = "invalid"
	sckgQuery = ""
	sckgFormat = "text"

	err := SckgCmd.RunE(SckgCmd, []string{dir})
	if err == nil {
		t.Error("expected error for invalid action")
	}
}

func TestSckgCmd_InvalidPath(t *testing.T) {
	sckgAction = "build"
	sckgQuery = ""
	sckgFormat = "text"

	err := SckgCmd.RunE(SckgCmd, []string{"/nonexistent/path"})
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestSckgCmd_FileAsPath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	os.WriteFile(f, []byte("package main\n"), 0644)

	sckgAction = "build"
	sckgQuery = ""
	sckgFormat = "text"

	err := SckgCmd.RunE(SckgCmd, []string{f})
	if err == nil {
		t.Error("expected error when path is a file not a directory")
	}
}

func TestBuildGraph_TypeScriptFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app.ts"), []byte("interface MyInterface {}\ntype MyType = string;\nexport function hello(): string { return 'hi'; }\nconst x = 5;\n"), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	foundInterface := false
	foundType := false
	for _, node := range graph.Nodes {
		if node.Type == "interface" && node.Name == "MyInterface" {
			foundInterface = true
		}
		if node.Type == "type" && node.Name == "MyType" {
			foundType = true
		}
	}
	if !foundInterface {
		t.Error("expected interface node 'MyInterface'")
	}
	if !foundType {
		t.Error("expected type node 'MyType'")
	}
}

func TestBuildGraph_LargeFileSkipped(t *testing.T) {
	dir := t.TempDir()
	bigContent := strings.Repeat("package main\nfunc F() {}\n", 200000)
	os.WriteFile(filepath.Join(dir, "big.go"), []byte(bigContent), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	for _, node := range graph.Nodes {
		if strings.Contains(node.Path, "big.go") && node.Type == "function" {
			t.Error("expected large file to skip symbol extraction")
		}
	}
}

func TestBuildGraph_ReadErrorSkipped(t *testing.T) {
	dir := t.TempDir()
	secretFile := filepath.Join(dir, "secret.go")
	os.WriteFile(secretFile, []byte("package main\n"), 0644)
	os.Chmod(secretFile, 0000)
	defer os.Chmod(secretFile, 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph should not fail on unreadable file: %v", err)
	}
	for _, node := range graph.Nodes {
		if strings.Contains(node.Path, "secret.go") && node.Type == "function" {
			t.Error("expected unreadable file to be skipped")
		}
	}
}

func TestBuildGraph_VendorDir(t *testing.T) {
	dir := t.TempDir()
	vendorDir := filepath.Join(dir, "vendor")
	os.MkdirAll(vendorDir, 0755)
	os.WriteFile(filepath.Join(vendorDir, "lib.go"), []byte("package lib\nfunc Hidden() {}\n"), 0644)

	graph, err := buildGraph(dir)
	if err != nil {
		t.Fatalf("buildGraph failed: %v", err)
	}
	for _, node := range graph.Nodes {
		if strings.Contains(node.Path, "vendor") {
			t.Errorf("expected vendor dir to be skipped, found: %v", node)
		}
	}
}

func TestQueryGraph_RelatedByTarget(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:a.go", Type: "file", Name: "a.go", Path: "a.go"},
			{ID: "file:b.go", Type: "file", Name: "b.go", Path: "b.go"},
			{ID: "func:a.go:Hello", Type: "function", Name: "Hello", Path: "a.go", Line: 5},
		},
		Edges: []sckgEdge{
			{Source: "file:b.go", Target: "func:a.go:Hello", Type: "contains"},
		},
	}

	result := queryGraph(graph, "hello")
	if len(result.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result.Matches))
	}
	if len(result.Related) == 0 {
		t.Error("expected related nodes when match is edge target")
	}
}

func TestQueryGraph_ByPath(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:auth/login.go", Type: "file", Name: "login.go", Path: "auth/login.go"},
		},
		Edges: []sckgEdge{},
	}

	result := queryGraph(graph, "auth")
	if len(result.Matches) != 1 {
		t.Errorf("expected 1 match for path 'auth', got %d", len(result.Matches))
	}
}

func TestGraphStats_TopImportsTruncated(t *testing.T) {
	var nodes []sckgNode
	var edges []sckgEdge
	nodes = append(nodes, sckgNode{ID: "file:main.go", Type: "file", Name: "main.go"})
	for i := 0; i < 15; i++ {
		depID := fmt.Sprintf("dep:pkg%d", i)
		nodes = append(nodes, sckgNode{ID: depID, Type: "module", Name: fmt.Sprintf("pkg%d", i)})
		edges = append(edges, sckgEdge{Source: "file:main.go", Target: depID, Type: "imports"})
	}

	graph := &sckgGraph{Nodes: nodes, Edges: edges}
	stats := graphStats(graph)
	if len(stats.TopImports) > 10 {
		t.Errorf("expected at most 10 top imports, got %d", len(stats.TopImports))
	}
}

func TestGraphStats_NoOrphansForFiles(t *testing.T) {
	graph := &sckgGraph{
		Nodes: []sckgNode{
			{ID: "file:a.go", Type: "file", Name: "a.go"},
		},
		Edges: []sckgEdge{},
	}

	stats := graphStats(graph)
	for _, o := range stats.OrphanNodes {
		if o == "a.go" {
			t.Error("file nodes should not be reported as orphans")
		}
	}
}

func TestOutputTextSCKGStats_NoImports(t *testing.T) {
	stats := &sckgStats{
		TotalNodes:  1,
		TotalEdges:  0,
		NodeTypes:   map[string]int{"file": 1},
		EdgeTypes:   map[string]int{},
		TopImports:  nil,
		OrphanNodes: nil,
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextSCKGStats(stats); err != nil {
		t.Fatalf("outputTextSCKGStats failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "SCKG Graph Statistics") {
		t.Errorf("expected header in output, got %q", out)
	}
}

func TestOutputTextSCKGStats_WithOrphans(t *testing.T) {
	stats := &sckgStats{
		TotalNodes:  3,
		TotalEdges:  1,
		NodeTypes:   map[string]int{"file": 1, "function": 2},
		EdgeTypes:   map[string]int{"contains": 1},
		OrphanNodes: []string{"OrphanFunc"},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextSCKGStats(stats); err != nil {
		t.Fatalf("outputTextSCKGStats failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	if !strings.Contains(out, "Orphan nodes") {
		t.Errorf("expected 'Orphan nodes' in output, got %q", out)
	}
}

func TestSckgCmd_ActionQueryJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0644)

	sckgAction = "query"
	sckgQuery = "hello"
	sckgFormat = "json"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := SckgCmd.RunE(SckgCmd, []string{dir})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SckgCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	var result queryResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
}

func TestSckgCmd_ActionStatsJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Hello() {}\n"), 0644)

	sckgAction = "stats"
	sckgQuery = ""
	sckgFormat = "json"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := SckgCmd.RunE(SckgCmd, []string{dir})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("SckgCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()
	var stats sckgStats
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatalf("expected valid JSON, got parse error: %v", err)
	}
}
