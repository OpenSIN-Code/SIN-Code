// SPDX-License-Identifier: MIT
// Purpose: core analysis tools — discover, map, and lsp. Consolidated module
// that merges the previously standalone lsp_cmd.go, discover.go, and map.go
// files into a single source of truth for the core sin-code subcommands.
package internal

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lsp"
)

// ── LSP command state ───────────────────────────────────────────────────────

var (
	lspLang            string
	lspRoot            string
	lspFile            string
	lspLine            int
	lspCol             int
	lspDetectAvailable = lsp.DetectAvailable
	lspNewName         string

	// filepathAbs is a test hook for lspSetup error paths.
	// osGetwd is defined in serve.go as a shared package hook.
	filepathAbs = filepath.Abs
)

var LSPCmd = &cobra.Command{
	Use:   "lsp",
	Short: "LSP (Language Server Protocol) — IDE-grade code intelligence",
	Long: `Language Server Protocol wrapper for sin-code. Provides go-to-definition,
find-references, hover, rename, document symbols, formatting, and diagnostics
without launching an IDE. Spawns gopls/pyright/typescript-language-server
on demand and caches them per-language.

Examples:
  sin-code lsp servers                    # list detected LSPs
  sin-code lsp definition main.go 5 9     # go-to-def at line 5, col 9
  sin-code lsp references main.go 5 9    # find all references
  sin-code lsp hover main.go 5 9         # type/doc on hover
  sin-code lsp rename main.go 5 9 MyFunc # rename symbol
  sin-code lsp symbols main.go            # outline
  sin-code lsp format main.go             # format file
  sin-code lsp diagnostics main.go        # all errors/warnings`,
	SilenceUsage: true,
}

var lspServersCmd = &cobra.Command{
	Use:   "servers",
	Short: "List detected LSP servers on PATH",
	RunE: func(cmd *cobra.Command, args []string) error {
		specs := lspDetectAvailable()
		if len(specs) == 0 {
			fmt.Println("(no LSP servers detected on PATH)")
			fmt.Println("Install one of: gopls (go), pyright-langserver (python), typescript-language-server (ts/js)")
			return nil
		}
		if orch2Format == "json" {
			return json.NewEncoder(os.Stdout).Encode(specs)
		}
		fmt.Printf("Detected %d LSP server(s):\n", len(specs))
		for _, s := range specs {
			fmt.Printf("  %-12s  binary=%s  exts=%s\n", s.Language, s.Binary, strings.Join(s.FileExts, ","))
		}
		return nil
	},
}

var lspDefinitionCmd = &cobra.Command{
	Use:   "definition <file> <line> <col>",
	Short: "Go to definition at file:line:col",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRun(cmd, args, func(c *lsp.Client, uri string, line, col int) (any, error) {
			positions, err := c.Definition(uri, lsp.Position{Line: line, Character: col})
			if err != nil {
				return nil, err
			}
			return positions, nil
		})
	},
}

var lspReferencesCmd = &cobra.Command{
	Use:   "references <file> <line> <col>",
	Short: "Find all references to the symbol at file:line:col",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		includeDecl, _ := cmd.Flags().GetBool("include-decl")
		return lspRun(cmd, args, func(c *lsp.Client, uri string, line, col int) (any, error) {
			return c.References(uri, lsp.Position{Line: line, Character: col}, includeDecl)
		})
	},
}

var lspHoverCmd = &cobra.Command{
	Use:   "hover <file> <line> <col>",
	Short: "Show type/doc on hover at file:line:col",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRun(cmd, args, func(c *lsp.Client, uri string, line, col int) (any, error) {
			return c.Hover(uri, lsp.Position{Line: line, Character: col})
		})
	},
}

var lspRenameCmd = &cobra.Command{
	Use:   "rename <file> <line> <col> <new-name>",
	Short: "Rename symbol at file:line:col to <new-name>",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRun(cmd, args, func(c *lsp.Client, uri string, line, col int) (any, error) {
			if lspNewName == "" {
				return nil, fmt.Errorf("--new-name required")
			}
			return c.Rename(uri, lsp.Position{Line: line, Character: col}, lspNewName)
		})
	},
}

var lspSymbolsCmd = &cobra.Command{
	Use:   "symbols <file>",
	Short: "Show document outline (symbols) for a file",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRunSimple(cmd, args, func(c *lsp.Client, uri string) (any, error) {
			return c.Symbols(uri)
		})
	},
}

var lspFormatCmd = &cobra.Command{
	Use:   "format <file>",
	Short: "Format a file using the LSP textDocument/formatting",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRunSimple(cmd, args, func(c *lsp.Client, uri string) (any, error) {
			return c.Format(uri)
		})
	},
}

var lspDiagnosticsCmd = &cobra.Command{
	Use:   "diagnostics <file>",
	Short: "Read file contents and return what diagnostics the LSP reports",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRunSimple(cmd, args, func(c *lsp.Client, uri string) (any, error) {
			text, err := os.ReadFile(stripURI(uri))
			if err != nil {
				return nil, err
			}
			_ = c.DidOpen(lsp.TextDocumentItem{URI: uri, LanguageID: langForPath(uri), Version: 1, Text: string(text)})
			return map[string]any{"file": uri, "hint": "diagnostics arrive via publishDiagnostics notification, not request; use LSP for full stream"}, nil
		})
	},
}

func lspRun(cmd *cobra.Command, args []string, fn func(c *lsp.Client, uri string, line, col int) (any, error)) error {
	if err := lspParseArgsFn(args, true); err != nil {
		return err
	}
	if lspNewName != "" && cmd.Name() != "rename" {
		return nil
	}
	mgr, rootURI, fileURI, err := lspSetup(cmd, lspFile, true)
	if err != nil {
		return err
	}
	defer mgr.Close()
	lang := lsp.LanguageForFile(lspFile)
	if lang == "" {
		lang = lspLang
	}
	if lang == "" {
		return fmt.Errorf("could not determine language for %s (use --lang)", lspFile)
	}
	c, err := mgr.Get(lang, rootURI)
	if err != nil {
		return err
	}
	if text, err := os.ReadFile(stripURI(fileURI)); err == nil {
		_ = c.DidOpen(lsp.TextDocumentItem{URI: fileURI, LanguageID: lang, Version: 1, Text: string(text)})
	}
	out, err := fn(c, fileURI, lspLine, lspCol)
	if err != nil {
		return err
	}
	if orch2Format == "json" {
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	printLSPResult(cmd.Name(), out)
	return nil
}

func lspRunSimple(cmd *cobra.Command, args []string, fn func(c *lsp.Client, uri string) (any, error)) error {
	if err := lspParseArgsFn(args, false); err != nil {
		return err
	}
	mgr, rootURI, fileURI, err := lspSetup(cmd, lspFile, false)
	if err != nil {
		return err
	}
	defer mgr.Close()
	lang := lsp.LanguageForFile(lspFile)
	if lang == "" {
		lang = lspLang
	}
	if lang == "" {
		return fmt.Errorf("could not determine language for %s", lspFile)
	}
	c, err := mgr.Get(lang, rootURI)
	if err != nil {
		return err
	}
	if text, err := os.ReadFile(stripURI(fileURI)); err == nil {
		_ = c.DidOpen(lsp.TextDocumentItem{URI: fileURI, LanguageID: lang, Version: 1, Text: string(text)})
	}
	out, err := fn(c, fileURI)
	if err != nil {
		return err
	}
	if orch2Format == "json" {
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	printLSPResult(cmd.Name(), out)
	return nil
}

func lspParseArgs(args []string, withPos bool) error {
	if len(args) > 0 {
		lspFile = args[0]
	}
	if withPos {
		if len(args) > 1 {
			n, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid line: %s", args[1])
			}
			lspLine = n
		}
		if len(args) > 2 {
			n, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("invalid col: %s", args[2])
			}
			lspCol = n
		}
	}
	return nil
}

// lspParseArgsFn is the callable used by lspRun/lspRunSimple. It defaults to
// lspParseArgs and is overridable in tests to exercise argument-error paths.
var lspParseArgsFn = lspParseArgs

func lspSetup(cmd *cobra.Command, fileFlag string, withPos bool) (*lsp.Manager, string, string, error) {
	root := lspRoot
	if root == "" {
		var err error
		root, err = osGetwd()
		if err != nil {
			return nil, "", "", err
		}
	}
	rootAbs, err := filepathAbs(root)
	if err != nil {
		return nil, "", "", err
	}
	if lspFile == "" && fileFlag != "" {
		lspFile = fileFlag
	}
	if !strings.HasPrefix(lspFile, "/") {
		lspFile = filepath.Join(rootAbs, lspFile)
	}
	rootURI := (&url.URL{Scheme: "file", Path: rootAbs}).String()
	fileURI := (&url.URL{Scheme: "file", Path: lspFile}).String()
	if lspLine == 0 && withPos {
		return nil, "", "", fmt.Errorf("--line required (0-indexed)")
	}
	if lspCol == 0 && withPos {
		return nil, "", "", fmt.Errorf("--col required (0-indexed)")
	}
	return lsp.NewManager(), rootURI, fileURI, nil
}

func stripURI(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		u, err := url.PathUnescape(uri[len("file://"):])
		if err == nil {
			return u
		}
	}
	return uri
}

// sin-debt: shrink, upgrade: inline when callers are consolidated
func langForPath(p string) string {
	return lsp.LanguageForFile(p)
}

func printLSPResult(cmd string, out any) {
	switch v := out.(type) {
	case []lsp.Location:
		if len(v) == 0 {
			fmt.Println("(no results)")
			return
		}
		for _, loc := range v {
			fmt.Printf("%s:%d:%d\n", stripURI(loc.URI), loc.Range.Start.Line+1, loc.Range.Start.Character+1)
		}
	case *lsp.Hover:
		if v == nil {
			fmt.Println("(no hover info)")
			return
		}
		fmt.Printf("%v\n", v.Contents)
	case []lsp.DocumentSymbol:
		if len(v) == 0 {
			fmt.Println("(no symbols)")
			return
		}
		for _, s := range v {
			fmt.Printf("  %s\n", s.Name)
		}
	case *lsp.WorkspaceEdit:
		if v == nil {
			fmt.Println("(no edit)")
			return
		}
		for uri, edits := range v.Changes {
			for _, e := range edits {
				fmt.Printf("%s:%d:%d  +%q\n", stripURI(uri), e.Range.Start.Line+1, e.Range.Start.Character+1, e.NewText)
			}
		}
	case []lsp.TextEdit:
		for _, e := range v {
			fmt.Printf("  +%q\n", e.NewText)
		}
	case map[string]any:
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
	default:
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
	}
}

// ── Discover command state ──────────────────────────────────────────────────

var (
	discoverPattern string
	discoverSort    string
	discoverFormat  string
	discoverLimit   int
)

var (
	discoverAbsPath = filepath.Abs
	discoverWalk    = filepath.Walk
)

var DiscoverCmd = &cobra.Command{
	Use:   "discover [path]",
	Short: "Discover files with relevance scoring and pattern matching",
	Long: `Discover files in a directory with relevance scoring, pattern matching,
and dependency analysis. Pure Go implementation — no external binary needed.

Example:
  sin-code discover . --pattern "**/*.go" --sort_by relevance --format json`,
	Args:    cobra.ArbitraryArgs,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		absPath, err := discoverAbsPath(path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			if err != nil {
				return fmt.Errorf("path not found: %w", err)
			}
			return fmt.Errorf("path is not a directory: %s", absPath)
		}

		results, err := discoverFiles(absPath, discoverPattern, discoverLimit)
		if err != nil {
			return err
		}

		sortResults(results, discoverSort)
		if len(results) > discoverLimit {
			results = results[:discoverLimit]
		}

		if discoverFormat == "json" {
			return outputJSON(results)
		}
		return outputText(results)
	},
}

type fileResult struct {
	Path         string   `json:"path"`
	RelPath      string   `json:"rel_path"`
	Size         int64    `json:"size"`
	ModTime      string   `json:"mod_time"`
	Relevance    float64  `json:"relevance"`
	Dependencies []string `json:"dependencies,omitempty"`
}

func discoverFiles(root, pattern string, limit int) ([]fileResult, error) {
	matcher, err := buildGlobMatcher(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}

	var results []fileResult
	walked := 0
	err = discoverWalk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "vendor" || base == ".venv" || base == "__pycache__" || base == ".pytest_cache" || base == "dist" || base == "build" || base == "target" || base == ".idea" || base == ".vscode" || strings.HasPrefix(base, ".") && (base == ".DS_Store" || base == ".gitignore") {
				return filepath.SkipDir
			}
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		if !matcher(rel) {
			return nil
		}

		fr := fileResult{
			Path:    path,
			RelPath: rel,
			Size:    info.Size(),
			ModTime: info.ModTime().Format(time.RFC3339),
		}
		fr.Relevance = scoreRelevance(rel, info.Size())
		fr.Dependencies = extractDependencies(path)
		results = append(results, fr)

		walked++
		if walked > limit*10 && len(results) > limit {
			// Early stop if we have enough results
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}

func buildGlobMatcher(pattern string) (func(string) bool, error) {
	// Convert glob pattern to regex
	// ** matches any number of directories
	// * matches any characters except /
	// ? matches single character
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "**/*" {
		return func(s string) bool { return true }, nil
	}

	// Replace ** with placeholder
	pattern = strings.ReplaceAll(pattern, "**", "{{DOUBLESTAR}}")
	var reParts []string
	parts := strings.Split(pattern, "/")
	for _, part := range parts {
		if part == "{{DOUBLESTAR}}" {
			reParts = append(reParts, ".*")
		} else {
			reParts = append(reParts, globToRegex(part))
		}
	}
	rePattern := "^" + strings.Join(reParts, "/") + "$"
	re, err := regexp.Compile(rePattern)
	if err != nil {
		return nil, err
	}
	return func(s string) bool {
		return re.MatchString(s)
	}, nil
}

func globToRegex(glob string) string {
	var b strings.Builder
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.':
			b.WriteString("\\.")
		case '+', '(', ')', '[', ']', '{', '}', '^', '$', '|':
			b.WriteString("\\")
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func scoreRelevance(relPath string, size int64) float64 {
	score := 50.0

	// Proximity to root (shorter paths = more relevant)
	depth := strings.Count(relPath, string(filepath.Separator))
	score -= float64(depth) * 5.0

	// File extension bonus
	ext := strings.ToLower(filepath.Ext(relPath))
	bonus := map[string]float64{
		".go": 15, ".py": 15, ".js": 12, ".ts": 14, ".tsx": 12,
		".rs": 14, ".java": 10, ".c": 8, ".cpp": 10, ".h": 8,
		".md": 10, ".json": 5, ".yaml": 8, ".yml": 8, ".toml": 8,
		".sh": 8, ".dockerfile": 5, ".mod": 10, ".sum": 3,
	}
	if b, ok := bonus[ext]; ok {
		score += b
	}

	// Important filename keywords
	name := strings.ToLower(filepath.Base(relPath))
	keywords := map[string]float64{
		"main": 20, "index": 15, "app": 15, "config": 15, "server": 12,
		"router": 12, "handler": 10, "middleware": 10, "model": 10,
		"service": 10, "controller": 10, "test": 8, "spec": 8,
		"readme": 15, "license": 5, "makefile": 10, "dockerfile": 8,
		"compose": 8, "go.mod": 15, "package.json": 12, "requirements": 10,
	}
	for kw, b := range keywords {
		if strings.Contains(name, kw) {
			score += b
		}
	}

	// Penalty for very large files (likely generated or data)
	if size > 1_000_000 {
		score -= 20
	} else if size > 100_000 {
		score -= 10
	}

	// Penalty for certain paths
	lowerPath := strings.ToLower(relPath)
	penalties := []string{"vendor/", "node_modules/", "dist/", "build/", "__pycache__/", ".git/", "target/", ".next/", "coverage/"}
	for _, p := range penalties {
		if strings.Contains(lowerPath, p) {
			score -= 30
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

func extractDependencies(path string) []string {
	ext := strings.ToLower(filepath.Ext(path))
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 500_000 {
		return nil
	}
	content := string(data)

	var deps []string
	switch ext {
	case ".go":
		deps = extractGoImports(content, path)
	case ".py":
		deps = extractPythonImports(content)
	case ".js", ".ts", ".tsx", ".jsx":
		deps = extractJSImports(content)
	}

	if len(deps) > 20 {
		deps = deps[:20]
	}
	return deps
}

func extractGoImports(content, path string) []string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	var deps []string
	for _, imp := range f.Imports {
		deps = append(deps, strings.Trim(imp.Path.Value, `"`))
	}
	return deps
}

func extractPythonImports(content string) []string {
	var deps []string
	seen := make(map[string]bool)
	re := regexp.MustCompile(`^(?:import|from)\s+([a-zA-Z_][a-zA-Z0-9_.]*)`)
	for _, line := range strings.Split(content, "\n") {
		matches := re.FindStringSubmatch(line)
		if len(matches) > 1 {
			pkg := matches[1]
			if strings.Contains(pkg, ".") {
				pkg = strings.Split(pkg, ".")[0]
			}
			if !seen[pkg] && pkg != "" {
				seen[pkg] = true
				deps = append(deps, pkg)
			}
		}
	}
	return deps
}

func extractJSImports(content string) []string {
	var deps []string
	seen := make(map[string]bool)
	re := regexp.MustCompile(`(?:import\s+.*?\s+from\s+['"]([^'"]+)['"]|require\s*\(\s*['"]([^'"]+)['"]\s*\))`)
	for _, match := range re.FindAllStringSubmatch(content, -1) {
		for i := 1; i < len(match); i++ {
			if match[i] != "" && !seen[match[i]] {
				seen[match[i]] = true
				deps = append(deps, match[i])
			}
		}
	}
	return deps
}

func sortResults(results []fileResult, sortBy string) {
	switch sortBy {
	case "name":
		sort.Slice(results, func(i, j int) bool {
			return results[i].RelPath < results[j].RelPath
		})
	case "size":
		sort.Slice(results, func(i, j int) bool {
			return results[i].Size > results[j].Size
		})
	case "mtime":
		sort.Slice(results, func(i, j int) bool {
			return results[i].ModTime > results[j].ModTime
		})
	default: // relevance
		sort.Slice(results, func(i, j int) bool {
			return results[i].Relevance > results[j].Relevance
		})
	}
}

func outputJSON(results []fileResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

func outputText(results []fileResult) error {
	for _, r := range results {
		fmt.Printf("%s  (score: %.1f, size: %d, deps: %d)\n",
			r.RelPath, r.Relevance, r.Size, len(r.Dependencies))
	}
	return nil
}

// ── Map command state ───────────────────────────────────────────────────────

var (
	mapAction string
	mapFormat string
)

var (
	mapAbsPath = filepath.Abs
	mapWalk    = filepath.Walk
)

var MapCmd = &cobra.Command{
	Use:   "map [path]",
	Short: "Map code architecture with dependency graphs and hot-path analysis",
	Long: `Map code architecture with dependency graphs, entry points, hot paths,
and module-level analysis. Pure Go implementation.

Example:
  sin-code map . --action map --format json`,
	Args:    cobra.ArbitraryArgs,
	Version: Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "."
		if len(args) > 0 {
			path = args[0]
		}
		absPath, err := mapAbsPath(path)
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
	Path         string              `json:"path"`
	Summary      mapSummary          `json:"summary"`
	EntryPoints  []string            `json:"entry_points"`
	HotPaths     []hotPath           `json:"hot_paths"`
	Orphans      []string            `json:"orphans"`
	Dependencies map[string][]string `json:"dependencies"`
	ReverseDeps  map[string][]string `json:"reverse_dependencies"`
	Modules      []moduleInfo        `json:"modules"`
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
	Path      string   `json:"path"`
	Imports   int      `json:"imports"`
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

	err := mapWalk(root, func(path string, info os.FileInfo, err error) error {
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

		// Track per-module file counts and languages.
		dir := filepath.Dir(rel)
		if m, ok := modules[dir]; ok && dir != "." && dir != "" {
			m.Files++
			if !sliceContains(m.Languages, lang) {
				m.Languages = append(m.Languages, lang)
			}
		}

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

		// Check for entry points (skip test files — they often have func main
		// for fuzz tests, testdata generators, or E2E test harnesses).
		lowerRel := strings.ToLower(rel)
		if strings.Contains(lowerRel, "_test") || strings.Contains(lowerRel, "test_") {
			files = append(files, fileInfo{rel, lang, filepath.Dir(rel)})
			return nil
		}
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

var isGoEntryPointParseOutline = parseOutline

func isGoEntryPoint(path string, data []byte) bool {
	outline := isGoEntryPointParseOutline(path, data)
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
	var langs []struct {
		name  string
		count int
	}
	for k, v := range r.Summary.Languages {
		langs = append(langs, struct {
			name  string
			count int
		}{k, v})
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

func sliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// ── Registration ──────────────────────────────────────────────────────────

func init() {
	RegisterVersionCmd(DiscoverCmd)
	DiscoverCmd.Flags().StringVarP(&discoverPattern, "pattern", "p", "**/*", "File pattern (glob)")
	DiscoverCmd.Flags().StringVarP(&discoverSort, "sort_by", "s", "relevance", "Sort by: relevance|name|size|mtime")
	DiscoverCmd.Flags().StringVarP(&discoverFormat, "format", "f", "text", "Output format: text|json")
	DiscoverCmd.Flags().IntVarP(&discoverLimit, "limit", "l", 100, "Max results")
}

func init() {
	RegisterVersionCmd(MapCmd)
	MapCmd.Flags().StringVarP(&mapAction, "action", "a", "map", "Action: map|summary|graph|hotpaths")
	MapCmd.Flags().StringVarP(&mapFormat, "format", "f", "text", "Output format: text|json")
}

func init() {
	LSPCmd.PersistentFlags().StringVar(&lspRoot, "root", "", "Project root (default: current dir)")

	LSPCmd.AddCommand(lspServersCmd)
	LSPCmd.AddCommand(lspDefinitionCmd)
	LSPCmd.AddCommand(lspReferencesCmd)
	LSPCmd.AddCommand(lspHoverCmd)
	LSPCmd.AddCommand(lspRenameCmd)
	LSPCmd.AddCommand(lspSymbolsCmd)
	LSPCmd.AddCommand(lspFormatCmd)
	LSPCmd.AddCommand(lspDiagnosticsCmd)

	for _, c := range []*cobra.Command{lspDefinitionCmd, lspReferencesCmd, lspHoverCmd, lspRenameCmd, lspSymbolsCmd, lspFormatCmd, lspDiagnosticsCmd} {
		c.Flags().StringVar(&lspFile, "file", "", "File path (relative to --root, or absolute)")
	}
	lspDefinitionCmd.Flags().IntVar(&lspLine, "line", 0, "Line (0-indexed)")
	lspDefinitionCmd.Flags().IntVar(&lspCol, "col", 0, "Column (0-indexed)")
	lspReferencesCmd.Flags().IntVar(&lspLine, "line", 0, "Line (0-indexed)")
	lspReferencesCmd.Flags().IntVar(&lspCol, "col", 0, "Column (0-indexed)")
	lspReferencesCmd.Flags().Bool("include-decl", true, "Include declaration in results")
	lspHoverCmd.Flags().IntVar(&lspLine, "line", 0, "Line (0-indexed)")
	lspHoverCmd.Flags().IntVar(&lspCol, "col", 0, "Column (0-indexed)")
	lspRenameCmd.Flags().IntVar(&lspLine, "line", 0, "Line (0-indexed)")
	lspRenameCmd.Flags().IntVar(&lspCol, "col", 0, "Column (0-indexed)")
	lspRenameCmd.Flags().StringVar(&lspNewName, "new-name", "", "New symbol name (required)")
}
