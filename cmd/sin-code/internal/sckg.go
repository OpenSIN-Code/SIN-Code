// SPDX-License-Identifier: MIT
// Purpose: sckg — Semantic Codebase Knowledge Graphs. Builds a knowledge
// graph of a codebase: files, functions, imports, and their relationships.
// Pure Go implementation.
package internal

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	sckgPath   string
	sckgAction string
	sckgQuery  string
	sckgFormat string
)

var SckgCmd = &cobra.Command{
	Use:   "sckg",
	Short: "Semantic Codebase Knowledge Graphs — build & query code graph",
	Long: `Build and query a semantic graph of a codebase. Pure Go implementation.

Actions:
  build  — Build the knowledge graph from source code
  query  — Query the graph for relationships (requires --query)
  stats  — Show graph statistics
  export — Export graph as JSON

Examples:
  sin-code sckg . --action build
  sin-code sckg . --action query --query "auth module dependencies"
  sin-code sckg . --action stats
  sin-code sckg . --action export --format json`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			if err != nil {
				return fmt.Errorf("path not found: %w", err)
			}
			return fmt.Errorf("path is not a directory: %s", absPath)
		}

		switch sckgAction {
		case "build":
			graph, err := buildGraph(absPath)
			if err != nil {
				return err
			}
			if sckgFormat == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(graph)
			}
			return outputTextSCKGBuild(graph)
		case "query":
			if sckgQuery == "" {
				return fmt.Errorf("--query is required for action=query")
			}
			graph, err := buildGraph(absPath)
			if err != nil {
				return err
			}
			results := queryGraph(graph, sckgQuery)
			if sckgFormat == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}
			return outputTextSCKGQuery(results)
		case "stats":
			graph, err := buildGraph(absPath)
			if err != nil {
				return err
			}
			stats := graphStats(graph)
			if sckgFormat == "json" {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(stats)
			}
			return outputTextSCKGStats(stats)
		case "export":
			graph, err := buildGraph(absPath)
			if err != nil {
				return err
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(graph)
		default:
			return fmt.Errorf("unknown action: %s (use build|query|stats|export)", sckgAction)
		}
	},
}

type sckgGraph struct {
	Nodes []sckgNode `json:"nodes"`
	Edges []sckgEdge `json:"edges"`
}

type sckgNode struct {
	ID       string `json:"id"`
	Type     string `json:"type"`     // file, function, class, module
	Name     string `json:"name"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Language string `json:"language,omitempty"`
}

type sckgEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`   // imports, calls, defines, contains
}

type sckgStats struct {
	TotalNodes  int            `json:"total_nodes"`
	TotalEdges  int            `json:"total_edges"`
	NodeTypes   map[string]int `json:"node_types"`
	EdgeTypes   map[string]int `json:"edge_types"`
	TopImports  []importCount  `json:"top_imports"`
	OrphanNodes []string       `json:"orphan_nodes,omitempty"`
}

type importCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type queryResult struct {
	Query      string     `json:"query"`
	Matches    []sckgNode `json:"matches"`
	Related    []sckgNode `json:"related"`
	Total      int        `json:"total"`
}

func buildGraph(root string) (*sckgGraph, error) {
	var nodes []sckgNode
	var edges []sckgEdge
	nodeIndex := make(map[string]int) // id -> index in nodes

	addNode := func(id, typ, name, path string, line int, lang string) int {
		if idx, ok := nodeIndex[id]; ok {
			return idx
		}
		idx := len(nodes)
		nodes = append(nodes, sckgNode{
			ID:       id,
			Type:     typ,
			Name:     name,
			Path:     path,
			Line:     line,
			Language: lang,
		})
		nodeIndex[id] = idx
		return idx
	}

	addEdge := func(source, target, typ string) {
		edges = append(edges, sckgEdge{Source: source, Target: target, Type: typ})
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				base := filepath.Base(path)
				if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "dist" || base == "build" || base == "target" || base == ".venv" {
					return filepath.SkipDir
				}
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		lang := detectLanguage(path)
		if lang == "unknown" || lang == "markdown" || lang == "text" {
			return nil
		}

		fileID := "file:" + rel
		addNode(fileID, "file", filepath.Base(rel), rel, 0, lang)

		data, err := os.ReadFile(path)
		if err != nil || len(data) > 2_000_000 {
			return nil
		}
		content := string(data)

		// Extract imports and create edges
		deps := extractDependencies(path)
		for _, dep := range deps {
			depID := "dep:" + dep
			addNode(depID, "module", dep, "", 0, "")
			addEdge(fileID, depID, "imports")
		}

		// Extract symbols and create edges
		switch lang {
		case "go":
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, content, parser.AllErrors)
			if err != nil {
				return nil
			}
			for _, decl := range f.Decls {
				pos := fset.Position(decl.Pos())
				switch d := decl.(type) {
				case *ast.FuncDecl:
					funcID := fmt.Sprintf("func:%s:%s", rel, d.Name.Name)
					addNode(funcID, "function", d.Name.Name, rel, pos.Line, lang)
					addEdge(fileID, funcID, "contains")
				case *ast.GenDecl:
					for _, spec := range d.Specs {
						if ts, ok := spec.(*ast.TypeSpec); ok {
							typeID := fmt.Sprintf("type:%s:%s", rel, ts.Name.Name)
							addNode(typeID, "type", ts.Name.Name, rel, pos.Line, lang)
							addEdge(fileID, typeID, "contains")
						}
					}
				}
			}
		case "python":
			re := regexp.MustCompile(`^(\s*)(def|class)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
			lines := strings.Split(content, "\n")
			for i, line := range lines {
				matches := re.FindStringSubmatch(line)
				if len(matches) > 3 {
					typ := "function"
					if matches[2] == "class" {
						typ = "class"
					}
					id := fmt.Sprintf("%s:%s:%s", typ, rel, matches[3])
					addNode(id, typ, matches[3], rel, i+1, lang)
					addEdge(fileID, id, "contains")
				}
			}
		case "javascript", "typescript", "tsx", "jsx":
			re := regexp.MustCompile(`(?:export\s+)?(?:async\s+)?(?:function|class|const|let|var|interface|type)\s+([a-zA-Z_$][a-zA-Z0-9_$]*)`)
			lines := strings.Split(content, "\n")
			for i, line := range lines {
				matches := re.FindAllStringSubmatch(line, -1)
				for _, m := range matches {
					if len(m) > 1 {
						typ := "function"
						if strings.Contains(line, "class") {
							typ = "class"
						} else if strings.Contains(line, "interface") {
							typ = "interface"
						} else if strings.Contains(line, "type") {
							typ = "type"
						} else if strings.Contains(line, "const") || strings.Contains(line, "let") || strings.Contains(line, "var") {
							typ = "variable"
						}
						id := fmt.Sprintf("%s:%s:%s", typ, rel, m[1])
						addNode(id, typ, m[1], rel, i+1, lang)
						addEdge(fileID, id, "contains")
					}
				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &sckgGraph{Nodes: nodes, Edges: edges}, nil
}

func queryGraph(graph *sckgGraph, query string) *queryResult {
	query = strings.ToLower(query)
	var matches []sckgNode
	var related []sckgNode
	matchSet := make(map[string]bool)
	relatedSet := make(map[string]bool)

	// Find matching nodes
	for _, node := range graph.Nodes {
		if strings.Contains(strings.ToLower(node.Name), query) ||
			strings.Contains(strings.ToLower(node.Path), query) ||
			strings.Contains(strings.ToLower(node.Type), query) {
			matches = append(matches, node)
			matchSet[node.ID] = true
		}
	}

	// Find related nodes (connected by edges)
	for _, edge := range graph.Edges {
		if matchSet[edge.Source] && !matchSet[edge.Target] && !relatedSet[edge.Target] {
			for _, node := range graph.Nodes {
				if node.ID == edge.Target {
					related = append(related, node)
					relatedSet[node.ID] = true
					break
				}
			}
		}
		if matchSet[edge.Target] && !matchSet[edge.Source] && !relatedSet[edge.Source] {
			for _, node := range graph.Nodes {
				if node.ID == edge.Source {
					related = append(related, node)
					relatedSet[node.ID] = true
					break
				}
			}
		}
	}

	return &queryResult{
		Query:   query,
		Matches: matches,
		Related: related,
		Total:   len(matches) + len(related),
	}
}

func graphStats(graph *sckgGraph) *sckgStats {
	nodeTypes := make(map[string]int)
	edgeTypes := make(map[string]int)
	importCounts := make(map[string]int)
	nodeConnections := make(map[string]int)

	for _, node := range graph.Nodes {
		nodeTypes[node.Type]++
	}
	for _, edge := range graph.Edges {
		edgeTypes[edge.Type]++
		nodeConnections[edge.Source]++
		nodeConnections[edge.Target]++
		if edge.Type == "imports" {
			importCounts[edge.Target]++
		}
	}

	var topImports []importCount
	for name, count := range importCounts {
		topImports = append(topImports, importCount{Name: name, Count: count})
	}
	sort.Slice(topImports, func(i, j int) bool {
		return topImports[i].Count > topImports[j].Count
	})
	if len(topImports) > 10 {
		topImports = topImports[:10]
	}

	var orphans []string
	for _, node := range graph.Nodes {
		if nodeConnections[node.ID] == 0 && node.Type != "file" {
			orphans = append(orphans, node.Name)
		}
	}

	return &sckgStats{
		TotalNodes:  len(graph.Nodes),
		TotalEdges:  len(graph.Edges),
		NodeTypes:   nodeTypes,
		EdgeTypes:   edgeTypes,
		TopImports:  topImports,
		OrphanNodes: orphans,
	}
}

func outputTextSCKGBuild(graph *sckgGraph) error {
	fmt.Printf("SCKG Knowledge Graph Built\n")
	fmt.Printf("Nodes: %d\n", len(graph.Nodes))
	fmt.Printf("Edges: %d\n", len(graph.Edges))
	fmt.Printf("\nNode Types:\n")
	types := make(map[string]int)
	for _, node := range graph.Nodes {
		types[node.Type]++
	}
	var typeList []struct{ name string; count int }
	for k, v := range types {
		typeList = append(typeList, struct{ name string; count int }{k, v})
	}
	sort.Slice(typeList, func(i, j int) bool { return typeList[i].count > typeList[j].count })
	for _, t := range typeList {
		fmt.Printf("  %-12s %d\n", t.name, t.count)
	}
	return nil
}

func outputTextSCKGQuery(result *queryResult) error {
	fmt.Printf("Query: %s\n", result.Query)
	fmt.Printf("Total matches: %d\n\n", result.Total)

	if len(result.Matches) > 0 {
		fmt.Printf("Direct matches (%d):\n", len(result.Matches))
		for _, node := range result.Matches {
			fmt.Printf("  %-12s %-20s %s:%d\n", node.Type, node.Name, node.Path, node.Line)
		}
	}

	if len(result.Related) > 0 {
		fmt.Printf("\nRelated (%d):\n", len(result.Related))
		for _, node := range result.Related {
			fmt.Printf("  %-12s %-20s %s:%d\n", node.Type, node.Name, node.Path, node.Line)
		}
	}
	return nil
}

func outputTextSCKGStats(stats *sckgStats) error {
	fmt.Printf("SCKG Graph Statistics\n")
	fmt.Printf("Total nodes: %d\n", stats.TotalNodes)
	fmt.Printf("Total edges: %d\n", stats.TotalEdges)

	fmt.Printf("\nNode Types:\n")
	for typ, count := range stats.NodeTypes {
		fmt.Printf("  %-12s %d\n", typ, count)
	}

	fmt.Printf("\nEdge Types:\n")
	for typ, count := range stats.EdgeTypes {
		fmt.Printf("  %-12s %d\n", typ, count)
	}

	if len(stats.TopImports) > 0 {
		fmt.Printf("\nTop Imports:\n")
		for _, imp := range stats.TopImports {
			fmt.Printf("  %-40s %d\n", imp.Name, imp.Count)
		}
	}

	if len(stats.OrphanNodes) > 0 {
		fmt.Printf("\nOrphan nodes (%d):\n", len(stats.OrphanNodes))
		for _, orphan := range stats.OrphanNodes {
			fmt.Printf("  %s\n", orphan)
		}
	}
	return nil
}

func init() {
	SckgCmd.Flags().StringVarP(&sckgAction, "action", "a", "build", "Action: build|query|stats|export")
	SckgCmd.Flags().StringVarP(&sckgQuery, "query", "q", "", "Query (for action=query)")
	SckgCmd.Flags().StringVarP(&sckgFormat, "format", "f", "text", "Output format: text|json")
}
