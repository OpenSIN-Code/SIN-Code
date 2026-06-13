// SPDX-License-Identifier: MIT
// Purpose: grasp — deep code understanding for a single file. Built-in Go
// implementation providing structure, dependencies, and usage context.
package internal

import (
	"bufio"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var (
	graspPath   string
	graspFormat string
)

var GraspCmd = &cobra.Command{
	Use:   "grasp [path]",
	Short: "Deep code understanding for a single file",
	Long: `Deep code understanding for individual files — structure, dependencies,
usage, and related context. Pure Go implementation.

Example:
  sin-code grasp cmd/sin-code/main.go --format json`,
	Args: cobra.ExactArgs(1),
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("file not found: %w", err)
		}
		if info.IsDir() {
			return fmt.Errorf("path is a directory, not a file: %s", absPath)
		}

		result, err := analyzeFile(absPath, info)
		if err != nil {
			return err
		}

		if graspFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		return outputTextGrasp(result)
	},
}

type graspResult struct {
	Path         string            `json:"path"`
	Language     string            `json:"language"`
	Size         int64             `json:"size"`
	Lines        int               `json:"lines"`
	BlankLines   int               `json:"blank_lines"`
	CommentLines int               `json:"comment_lines"`
	CodeLines    int               `json:"code_lines"`
	ModTime      string            `json:"mod_time"`
	Structure    []structItem      `json:"structure"`
	Dependencies []string          `json:"dependencies"`
	Summary      string            `json:"summary"`
	Exports      []string          `json:"exports"`
}

type structItem struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Line int    `json:"line"`
}

func analyzeFile(path string, info os.FileInfo) (*graspResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	content := string(data)

	lang := detectLanguage(path)
	lines, blank, comments, code := countLines(content, lang)
	structure := extractStructure(path, content)
	deps := extractDependencies(path) // reuses discover.go function
	exports := extractExports(content, lang)

	summary := fmt.Sprintf("%d lines (%d code, %d comments, %d blank) in %s",
		lines, code, comments, blank, lang)

	return &graspResult{
		Path:         path,
		Language:     lang,
		Size:         info.Size(),
		Lines:        lines,
		BlankLines:   blank,
		CommentLines: comments,
		CodeLines:    code,
		ModTime:      info.ModTime().Format("2006-01-02 15:04:05"),
		Structure:    structure,
		Dependencies: deps,
		Summary:      summary,
		Exports:      exports,
	}, nil
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	langMap := map[string]string{
		".go": "go", ".py": "python", ".js": "javascript", ".ts": "typescript",
		".tsx": "tsx", ".jsx": "jsx", ".rs": "rust", ".java": "java",
		".c": "c", ".cpp": "cpp", ".h": "c-header", ".hpp": "cpp-header",
		".sh": "bash", ".md": "markdown", ".json": "json", ".yaml": "yaml",
		".yml": "yaml", ".toml": "toml", ".html": "html", ".css": "css",
		".sql": "sql", ".rb": "ruby", ".php": "php", ".swift": "swift",
		".kt": "kotlin", ".scala": "scala", ".r": "r", ".lua": "lua",
		".dockerfile": "dockerfile", ".makefile": "makefile", ".mod": "go",
	}
	if lang, ok := langMap[ext]; ok {
		return lang
	}
	name := strings.ToLower(filepath.Base(path))
	if name == "dockerfile" || strings.HasPrefix(name, "dockerfile") {
		return "dockerfile"
	}
	if name == "makefile" || name == "gnumakefile" {
		return "makefile"
	}
	return "unknown"
}

func countLines(content, lang string) (total, blank, comments, code int) {
	scanner := bufio.NewScanner(strings.NewReader(content))
	inBlockComment := false
	blockStart := ""
	blockEnd := ""

	switch lang {
	case "go", "c", "cpp", "c-header", "cpp-header", "java", "rust", "kotlin", "swift", "scala":
		blockStart = "/*"
		blockEnd = "*/"
	case "python":
		blockStart = `"""`
		blockEnd = `"""`
		if !strings.Contains(content, `"""`) {
			blockStart = "'''"
			blockEnd = "'''"
		}
	}

	lineComment := ""
	switch lang {
	case "go", "c", "cpp", "c-header", "cpp-header", "java", "rust", "kotlin", "swift", "scala", "bash", "makefile":
		lineComment = "//"
	case "python", "ruby", "php", "r", "perl", "yaml", "dockerfile":
		lineComment = "#"
	case "javascript", "typescript", "tsx", "jsx", "css", "sql":
		lineComment = "//"
	}

	for scanner.Scan() {
		line := scanner.Text()
		total++
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			blank++
			continue
		}

		if inBlockComment {
			comments++
			if blockEnd != "" && strings.Contains(line, blockEnd) {
				inBlockComment = false
			}
			continue
		}

		if blockStart != "" && strings.Contains(line, blockStart) {
			comments++
			if !strings.Contains(line, blockEnd) || strings.Index(line, blockStart) > strings.Index(line, blockEnd) {
				inBlockComment = true
			}
			continue
		}

		if lineComment != "" && strings.HasPrefix(trimmed, lineComment) {
			comments++
			continue
		}

		code++
	}
	return
}

func extractStructure(path, content string) []structItem {
	// Phase 4b: unified AST-based extraction via parseOutline.
	outline := parseOutline(path, []byte(content))
	if outline == nil || outline.Engine == "none" {
		// Fallback to generic regex for unknown languages.
		return extractGenericStructure(strings.Split(content, "\n"), detectLanguage(path))
	}
	var items []structItem
	var walk func([]SymbolInfo)
	walk = func(syms []SymbolInfo) {
		for _, sym := range syms {
			items = append(items, structItem{Type: normalizeGraspKind(sym.Kind), Name: sym.Name, Line: sym.StartLine})
			if len(sym.Children) > 0 {
				walk(sym.Children)
			}
		}
	}
	walk(outline.Symbols)
	return items
}

func normalizeGraspKind(kind string) string {
	switch kind {
	case "func":
		return "function"
	case "method":
		return "function"
	case "var":
		return "variable"
	case "const":
		return "variable"
	default:
		return kind
	}
}

func extractGenericStructure(lines []string, lang string) []structItem {
	var items []structItem
	re := regexp.MustCompile(`(?:function|def|fn|func|method|class|struct|interface|trait|enum|record|sub|procedure)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	for i, line := range lines {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			items = append(items, structItem{Type: "symbol", Name: matches[1], Line: i + 1})
		}
	}
	return items
}

func extractExports(content, lang string) []string {
	var exports []string
	switch lang {
	case "go":
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "", content, parser.AllErrors)
		if err != nil {
			return nil
		}
		for name := range f.Scope.Objects {
			if ast.IsExported(name) {
				exports = append(exports, name)
			}
		}
	case "python":
		re := regexp.MustCompile(`^\s*__(all__)\s*=\s*\[(.*?)\]`)
		if m := re.FindStringSubmatch(content); len(m) > 2 {
			all := strings.Split(m[2], ",")
			for _, e := range all {
				e = strings.Trim(strings.TrimSpace(e), `"' `)
				if e != "" {
					exports = append(exports, e)
				}
			}
		}
	case "javascript", "typescript", "tsx", "jsx":
		re := regexp.MustCompile(`(?:export\s+(?:default\s+)?(?:class|function|const|let|var|interface|type)\s+)([a-zA-Z_$][a-zA-Z0-9_$]*)`)
		seen := make(map[string]bool)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) > 1 && !seen[m[1]] {
				seen[m[1]] = true
				exports = append(exports, m[1])
			}
		}
	case "rust":
		re := regexp.MustCompile(`^pub\s+(?:fn|struct|enum|trait|type|use|const|static|mod)\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
		for _, m := range re.FindAllStringSubmatch(content, -1) {
			if len(m) > 1 {
				exports = append(exports, m[1])
			}
		}
	}
	return exports
}

func outputTextGrasp(r *graspResult) error {
	fmt.Printf("File:     %s\n", r.Path)
	fmt.Printf("Language: %s\n", r.Language)
	fmt.Printf("Size:     %d bytes\n", r.Size)
	fmt.Printf("Lines:    %d total (%d code, %d comments, %d blank)\n",
		r.Lines, r.CodeLines, r.CommentLines, r.BlankLines)
	fmt.Printf("Modified: %s\n", r.ModTime)
	fmt.Printf("Summary:  %s\n", r.Summary)

	if len(r.Structure) > 0 {
		fmt.Printf("\nStructure (%d symbols):\n", len(r.Structure))
		for _, s := range r.Structure {
			fmt.Printf("  %-10s %-20s (line %d)\n", s.Type, s.Name, s.Line)
		}
	}

	if len(r.Dependencies) > 0 {
		fmt.Printf("\nDependencies (%d):\n", len(r.Dependencies))
		for _, d := range r.Dependencies {
			fmt.Printf("  %s\n", d)
		}
	}

	if len(r.Exports) > 0 {
		fmt.Printf("\nExports (%d):\n", len(r.Exports))
		for _, e := range r.Exports {
			fmt.Printf("  %s\n", e)
		}
	}
	return nil
}

func init() {
	RegisterVersionCmd(GraspCmd)
	GraspCmd.Flags().StringVarP(&graspFormat, "format", "f", "text", "Output format: text|json")
}
