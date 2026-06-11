// SPDX-License-Identifier: MIT
// Purpose: map — architecture mapping with dependency graphs, entry points, hot
// paths, and module-level analysis. Built-in Go implementation.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var (
	mapAction string
	mapFormat string
)

var MapCmd = &cobra.Command{
	Use:   "map [path]",
	Short: "Map code architecture with dependency graphs and hot-path analysis",
	Long: `Map code architecture with dependency graphs, entry points, hot paths,
and module-level analysis. Pure Go implementation.

Example:
  sin-code map . --action map --format json`,
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
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("path not found: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("path is not a directory: %s", absPath)
		}

		result, err := mapArchitecture(absPath, mapAction)
		if err != nil {
			return err
		}

		if mapFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		return outputTextMap(result)
	},
}

type mapResult struct {
	Path         string            `json:"path"`
	Summary      mapSummary        `json:"summary"`
	EntryPoints  []string          `json:"entry_points"`
	HotPaths     []hotPath         `json:"hot_paths"`
	Orphans      []string          `json:"orphans"`
	Dependencies map[string][]string `json:"dependencies"`
	ReverseDeps  map[string][]string `json:"reverse_dependencies"`
	Modules      []moduleInfo      `json:"modules"`
}

type mapSummary struct {
	TotalFiles    int            `json:"total_files"`
	TotalLines    int            `json:"total_lines"`
	Languages     map[string]int `json:"languages"`
	TestFiles     int            `json:"test_files"`
	ConfigFiles   int            `json:"config_files"`
	Documentation int            `json:"documentation"`
}

type hotPath struct {
	Path     string `json:"path"`
	Imports  int    `json:"imports"`
	Importers []string `json:"importers"`
}

type moduleInfo struct {
	Path      string   `json:"path"`
	Files     int      `json:"files"`
	Languages []string `json:"languages"`
	Imports   []string `json:"imports"`
	Exports   []string `json:"exports"`
}

func mapArchitecture(root, action string) (*mapResult, error) {
	var files []fileInfo
	languages := make(map[string]int)
	var entryPoints []string
	var orphans []string
	deps := make(map[string][]string)
	reverseDeps := make(map[string][]string)
	modules := make(map[string]*moduleInfo)
	totalLines := 0
	testFiles := 0
	configFiles := 0
	docs := 0

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				base := filepath.Base(path)
				if base == ".git" || base == "node_modules" || base == "vendor" || base == "__pycache__" || base == "dist" || base == "build" || base == "target" || strings.HasPrefix(base, ".") {
					return filepath.SkipDir
				}
				// Track modules (subdirectories with code)
				rel, _ := filepath.Rel(root, path)
				if rel != "." && rel != "" {
					modules[rel] = &moduleInfo{Path: rel}
				}
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		lang := detectLanguage(path)
		languages[lang]++

		if strings.Contains(strings.ToLower(rel), "_test") || strings.Contains(strings.ToLower(rel), "test_") || strings.Contains(strings.ToLower(rel), ".spec.") || strings.Contains(strings.ToLower(rel), ".test.") {
			testFiles++
		}
		ext := strings.ToLower(filepath.Ext(rel))
		if ext == ".json" || ext == ".yaml" || ext == ".yml" || ext == ".toml" || ext == ".ini" || ext == ".conf" || ext == ".mod" || ext == ".sum" || ext == ".lock" || strings.Contains(rel, "Dockerfile") || strings.Contains(rel, "Makefile") || strings.Contains(rel, ".env") {
			configFiles++
		}
		if ext == ".md" || ext == ".rst" || ext == ".txt" || ext == ".adoc" || strings.Contains(rel, "README") || strings.Contains(rel, "LICENSE") || strings.Contains(rel, "CHANGELOG") || strings.Contains(rel, "CONTRIBUTING") {
			docs++
		}

		data, err := os.ReadFile(path)
		var content string
		if err == nil && len(data) < 1_000_000 {
			content = string(data)
			lines := strings.Count(content, "\n") + 1
			totalLines += lines
			fileDeps := extractDependencies(path)
			if len(fileDeps) > 0 {
				deps[rel] = fileDeps
				for _, d := range fileDeps {
					reverseDeps[d] = append(reverseDeps[d], rel)
				}
			}
		}

		// Check for entry points
		name := strings.ToLower(filepath.Base(path))
		if lang == "go" {
			if name == "main.go" || isGoEntryPoint(path, data) {
				entryPoints = append(entryPoints, rel)
			}
		} else if lang == "python" && (name == "__main__.py" || strings.Contains(content, `if __name__ == "__main__":`)) {
			entryPoints = append(entryPoints, rel)
		} else if (lang == "javascript" || lang == "typescript") && (name == "index.js" || name == "index.ts" || name == "main.js" || name == "main.ts") {
			entryPoints = append(entryPoints, rel)
		} else if lang == "rust" && name == "main.rs" {
			entryPoints = append(entryPoints, rel)
		} else if lang == "java" && strings.Contains(content, "public static void main") {
			entryPoints = append(entryPoints, rel)
		}

		files = append(files, fileInfo{rel, lang, filepath.Dir(rel)})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Find orphans (files with no imports and no reverse dependencies, excluding tests and configs)
	for _, f := range files {
		if f.lang == "unknown" || f.lang == "markdown" || f.lang == "json" || f.lang == "yaml" || f.lang == "text" {
			continue
		}
		if strings.Contains(strings.ToLower(f.path), "_test") || strings.Contains(strings.ToLower(f.path), "test_") {
			continue
		}
		if _, hasDeps := deps[f.path]; !hasDeps {
			if _, imported := reverseDeps[f.path]; !imported {
				orphans = append(orphans, f.path)
			}
		}
	}

	// Hot paths: most imported files
	var hotPaths []hotPath
	for path, importers := range reverseDeps {
		if len(importers) > 2 {
			hotPaths = append(hotPaths, hotPath{Path: path, Imports: len(importers), Importers: importers})
		}
	}
	sort.Slice(hotPaths, func(i, j int) bool {
		return hotPaths[i].Imports > hotPaths[j].Imports
	})
	if len(hotPaths) > 20 {
		hotPaths = hotPaths[:20]
	}

	// Module info
	var moduleList []moduleInfo
	for _, m := range modules {
		if m.Files == 0 && len(m.Languages) == 0 {
			continue
		}
		moduleList = append(moduleList, *m)
	}
	sort.Slice(moduleList, func(i, j int) bool {
		return moduleList[i].Files > moduleList[j].Files
	})

	result := &mapResult{
		Path: root,
		Summary: mapSummary{
			TotalFiles:    len(files),
			TotalLines:    totalLines,
			Languages:     languages,
			TestFiles:     testFiles,
			ConfigFiles:   configFiles,
			Documentation: docs,
		},
		EntryPoints:  entryPoints,
		HotPaths:     hotPaths,
		Orphans:      orphans,
		Dependencies: deps,
		ReverseDeps:  reverseDeps,
		Modules:      moduleList,
	}
	return result, nil
}

type fileInfo struct {
	path string
	lang string
	dir  string
}

func isGoEntryPoint(path string, data []byte) bool {
	outline := parseOutline(path, data)
	if outline == nil || outline.Engine == "none" {
		return false
	}
	var walk func([]SymbolInfo) bool
	walk = func(syms []SymbolInfo) bool {
		for _, sym := range syms {
			if sym.Name == "main" && (sym.Kind == "func" || sym.Kind == "function") {
				return true
			}
			if len(sym.Children) > 0 && walk(sym.Children) {
				return true
			}
		}
		return false
	}
	return walk(outline.Symbols)
}

func outputTextMap(r *mapResult) error {
	fmt.Printf("Architecture Map: %s\n", r.Path)
	fmt.Printf("\nSummary:\n")
	fmt.Printf("  Total files:  %d\n", r.Summary.TotalFiles)
	fmt.Printf("  Total lines:  %d\n", r.Summary.TotalLines)
	fmt.Printf("  Test files:   %d\n", r.Summary.TestFiles)
	fmt.Printf("  Config files: %d\n", r.Summary.ConfigFiles)
	fmt.Printf("  Docs:         %d\n", r.Summary.Documentation)

	fmt.Printf("\nLanguages (%d):\n", len(r.Summary.Languages))
	var langs []struct{ name string; count int }
	for k, v := range r.Summary.Languages {
		langs = append(langs, struct{ name string; count int }{k, v})
	}
	sort.Slice(langs, func(i, j int) bool { return langs[i].count > langs[j].count })
	for _, l := range langs {
		fmt.Printf("  %-12s %d files\n", l.name, l.count)
	}

	if len(r.EntryPoints) > 0 {
		fmt.Printf("\nEntry Points (%d):\n", len(r.EntryPoints))
		for _, ep := range r.EntryPoints {
			fmt.Printf("  %s\n", ep)
		}
	}

	if len(r.HotPaths) > 0 {
		fmt.Printf("\nHot Paths (most imported):\n")
		for _, hp := range r.HotPaths {
			fmt.Printf("  %s  (imported by %d files)\n", hp.Path, hp.Imports)
		}
	}

	if len(r.Orphans) > 0 {
		fmt.Printf("\nOrphans (unimported files):\n")
		for _, o := range r.Orphans[:min(20, len(r.Orphans))] {
			fmt.Printf("  %s\n", o)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func init() {
	MapCmd.Flags().StringVarP(&mapAction, "action", "a", "map", "Action: map|summary|graph|hotpaths")
	MapCmd.Flags().StringVarP(&mapFormat, "format", "f", "text", "Output format: text|json")
}
