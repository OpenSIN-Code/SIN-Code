// SPDX-License-Identifier: MIT
// Purpose: edit — hashline-anchored surgical file editing. Replaces fragile
// native string/line editors: anchors carry a content hash so stale edits
// fail loudly instead of corrupting files, drift up to ±25 lines is
// auto-resolved, occurrence counting prevents ambiguous string replaces, and
// every edit re-runs the atomic write path with syntax validation.
// Also contains: read — token-efficient, anchor-aware file reading (merged from read.go).
// Also contains: write — atomic, validated file writing (merged from write.go).
// Docs: cmd/sin-code/internal/read.doc.md, cmd/sin-code/internal/edit.doc.md, cmd/sin-code/internal/write.doc.md
package internal

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

// ── read ───────────────────────────────────────────────────────────────
// read — token-efficient, anchor-aware file reading. Replaces naive
// native read/cat for agents: hashline mode emits stable edit anchors,
// outline mode emits structure instead of raw content (80–95% token
// savings on large files), and hard byte/line guards prevent context
// blowouts.

var buildOutlineMarshal = func(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }

var (
	readMode     string
	readOffset   int
	readLimit    int
	readMaxBytes int64
	readFormat   string
)

var readAbsPath = filepath.Abs

const readDefaultLimit = 2000
const readDefaultMaxBytes int64 = 1 << 20

var ReadCmd = &cobra.Command{
	Use:   "read [path]",
	Short: "Read files with hashline anchors, outline, and size guards",
	Long: `Token-efficient file reading for agents and humans.

Modes:
  hashline  (default) lines prefixed with "LINE:HASH|" — anchors feed 'sin-code edit'
  raw       plain content (still offset/limit guarded)
  outline   structure only: imports, functions, classes, exports (huge files)

Examples:
  sin-code read main.go
  sin-code read main.go --mode outline
  sin-code read big.log --offset 5000 --limit 200 --mode raw
  sin-code read pkg/x.go --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := readAbsPath(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		result, err := readFile(absPath, readMode, readOffset, readLimit, readMaxBytes)
		if err != nil {
			return err
		}
		if readFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Print(result.Content)
		if result.Truncated {
			fmt.Fprintf(os.Stderr, "\n[truncated: showing lines %d-%d of %d — use --offset/--limit, or --mode outline]\n",
				result.Offset, result.Offset+result.ReturnedLines-1, result.TotalLines)
		}
		return nil
	},
}

func init() {
	ReadCmd.Flags().StringVarP(&readMode, "mode", "m", "hashline", "Mode: hashline, raw, outline")
	ReadCmd.Flags().IntVar(&readOffset, "offset", 1, "1-based line to start from")
	ReadCmd.Flags().IntVar(&readLimit, "limit", 0, fmt.Sprintf("Max lines to return (default %d)", readDefaultLimit))
	ReadCmd.Flags().Int64Var(&readMaxBytes, "max-bytes", readDefaultMaxBytes, "Refuse raw/hashline reads of files larger than this")
	ReadCmd.Flags().StringVarP(&readFormat, "format", "f", "text", "Output: text, json")
}

type readResult struct {
	Path          string `json:"path"`
	Mode          string `json:"mode"`
	Language      string `json:"language"`
	Size          int64  `json:"size"`
	TotalLines    int    `json:"total_lines"`
	Offset        int    `json:"offset"`
	ReturnedLines int    `json:"returned_lines"`
	Truncated     bool   `json:"truncated"`
	Content       string `json:"content"`
}

func readFile(path, mode string, offset, limit int, maxBytes int64) (*readResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s (use 'sin-code discover')", path)
	}
	if mode != "outline" && maxBytes > 0 && info.Size() > maxBytes {
		return nil, fmt.Errorf("file is %d bytes (limit %d) — use --mode outline, narrow with --offset/--limit, or raise --max-bytes",
			info.Size(), maxBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("binary or non-UTF-8 file: %s (use 'sin-code execute' with a hex tool if needed)", path)
	}

	content := string(data)
	lang := detectLanguage(path)
	lines, _ := SplitLines(content)
	total := len(lines)

	res := &readResult{
		Path: path, Mode: mode, Language: lang,
		Size: info.Size(), TotalLines: total,
	}

	if mode == "outline" {
		return buildOutlineResult(res, content, lang), nil
	}

	if offset < 1 {
		offset = 1
	}
	if offset > total && total > 0 {
		return nil, fmt.Errorf("offset %d beyond end of file (%d lines)", offset, total)
	}
	if limit <= 0 {
		limit = readDefaultLimit
	}
	end := offset - 1 + limit
	if end > total {
		end = total
	}
	window := lines[offset-1 : end]

	res.Offset = offset
	res.ReturnedLines = len(window)
	res.Truncated = offset > 1 || end < total

	switch mode {
	case "raw":
		res.Content = JoinLines(window, true)
	case "hashline":
		res.Content = FormatHashlines(window, offset)
	default:
		return nil, fmt.Errorf("unknown mode %q: want hashline, raw, or outline", mode)
	}
	return res, nil
}

func buildOutlineResult(res *readResult, content, lang string) *readResult {
	outline := parseOutline(res.Path, []byte(content))
	exports := extractExports(content, lang)
	deps := extractDependencies(res.Path)

	outlineMap := map[string]any{
		"language":     outline.Language,
		"engine":       outline.Engine,
		"total_lines":  res.TotalLines,
		"symbols":      outline.Symbols,
		"imports":      outline.Imports,
		"exports":      exports,
		"dependencies": deps,
	}
	b, err := buildOutlineMarshal(outlineMap)
	if err != nil {
		b = []byte(fmt.Sprintf(`{"error":"%v"}`, err))
	}
	res.Offset = 1
	res.Content = string(b) + "\n"
	return res
}

// ── edit ───────────────────────────────────────────────────────────────

var (
	editAnchor     string
	editEndAnchor  string
	editNewText    string
	editOldString  string
	editNewString  string
	editReplaceAll bool
	editInsert     string
	editDelete     bool
	editDryRun     bool
	editNoValidate bool
	editDrift      int
	editFormat     string
	editSymbol     string
	editGetwd      = os.Getwd
	editWriteFile  = writeFileAtomic
)

var EditCmd = &cobra.Command{
	Use:   "edit [path]",
	Short: "Hashline-anchored surgical edits with validation",
	Long: `Surgical file editing with two addressing modes:

Anchor mode (preferred — anchors come from 'sin-code read'):
  --anchor 12:ab34cd56 --new-text "replacement"        replace one line
  --anchor 12:ab34cd56 --end-anchor 20:ef99aa01 ...    replace a line range
  --anchor 12:ab34cd56 --insert after --new-text "..." insert after a line
  --anchor 12:ab34cd56 --delete                        delete line (or range)

String mode (exact match, fails on ambiguity):
  --old-string "foo(a, b)" --new-string "foo(a, b, c)"
  --old-string "x" --new-string "y" --replace-all

Every edit validates syntax (like 'sin-code write') and applies atomically.
--dry-run prints a unified diff without touching the file.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		wd, err := editGetwd()
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		absPath := filepath.Join(wd, args[0])
		req := editRequest{
			Anchor: editAnchor, EndAnchor: editEndAnchor, NewText: editNewText,
			OldString: editOldString, NewString: editNewString,
			ReplaceAll: editReplaceAll, Insert: editInsert, Delete: editDelete,
			DryRun: editDryRun, Validate: !editNoValidate, Drift: editDrift,
			Symbol: editSymbol,
		}
		result, err := applyEdit(absPath, req)
		if err != nil {
			return err
		}
		if editFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		if result.DryRun {
			fmt.Print(result.Diff)
			return nil
		}
		fmt.Printf("edited %s: %s (%+d lines)\n", result.Path, result.Operation, result.LineDelta)
		if result.Diff != "" {
			fmt.Print(result.Diff)
		}
		return nil
	},
}

func init() {
	EditCmd.Flags().StringVar(&editAnchor, "anchor", "", "Hashline anchor LINE:HASH (from 'sin-code read')")
	EditCmd.Flags().StringVar(&editEndAnchor, "end-anchor", "", "End anchor for range operations (inclusive)")
	EditCmd.Flags().StringVar(&editNewText, "new-text", "", "Replacement/insertion text (may be multi-line)")
	EditCmd.Flags().StringVar(&editOldString, "old-string", "", "Exact string to replace (string mode)")
	EditCmd.Flags().StringVar(&editNewString, "new-string", "", "Replacement string (string mode)")
	EditCmd.Flags().BoolVar(&editReplaceAll, "replace-all", false, "Replace every occurrence (string mode)")
	EditCmd.Flags().StringVar(&editInsert, "insert", "", "Insert relative to anchor: before, after")
	EditCmd.Flags().BoolVar(&editDelete, "delete", false, "Delete the anchored line or range")
	EditCmd.Flags().BoolVar(&editDryRun, "dry-run", false, "Print diff without writing")
	EditCmd.Flags().BoolVar(&editNoValidate, "no-validate", false, "Skip syntax validation of the result")
	EditCmd.Flags().IntVar(&editDrift, "drift", DefaultDriftWindow, "Anchor drift tolerance in lines")
	EditCmd.Flags().StringVarP(&editFormat, "format", "f", "text", "Output: text, json")
	EditCmd.Flags().StringVar(&editSymbol, "symbol", "", "Edit a whole symbol by name (AST-anchored, e.g. \"handleScout\" or \"Server.Start\")")
}

type editRequest struct {
	Anchor     string
	EndAnchor  string
	NewText    string
	OldString  string
	NewString  string
	ReplaceAll bool
	Insert     string
	Delete     bool
	DryRun     bool
	Validate   bool
	Drift      int
	Symbol     string
}

type editResult struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	LineDelta int    `json:"line_delta"`
	Drift     int    `json:"anchor_drift,omitempty"`
	DryRun    bool   `json:"dry_run"`
	Diff      string `json:"diff"`
}

// EditByString performs a single string-mode surgical edit, replacing the first
// exact occurrence of old with new in path. It is exported so the chat tool can
// share the same engine as the MCP sin_edit tool (issue #373).
func EditByString(path, old, new string) error {
	if path == "" || old == "" {
		return fmt.Errorf("edit: path and old string required")
	}
	_, err := applyEdit(path, editRequest{
		OldString:  old,
		NewString:  new,
		ReplaceAll: false,
		Validate:   true,
		Drift:      DefaultDriftWindow,
	})
	return err
}

func applyEdit(path string, req editRequest) (*editResult, error) {
	anchorMode := req.Anchor != ""
	stringMode := req.OldString != ""
	symbolMode := req.Symbol != ""
	modes := 0
	for _, m := range []bool{anchorMode, stringMode, symbolMode} {
		if m {
			modes++
		}
	}
	if modes != 1 {
		return nil, fmt.Errorf("exactly one addressing mode required: --anchor LINE:HASH, --old-string, or --symbol NAME")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	original := string(data)
	lines, trailingNL := SplitLines(original)

	res := &editResult{Path: path, DryRun: req.DryRun}
	var updated []string

	switch {
	case symbolMode:
		updated, err = applySymbolEdit(lines, path, original, req, res)
	case anchorMode:
		updated, err = applyAnchorEdit(lines, req, res)
	default:
		updated, err = applyStringEdit(lines, original, req, res, &trailingNL)
	}
	if err != nil {
		return nil, err
	}

	newContent := JoinLines(updated, trailingNL)
	res.LineDelta = len(updated) - len(lines)
	res.Diff = unifiedDiff(path, lines, updated)

	if req.Validate {
		if err := validateSyntax(path, newContent); err != nil {
			return nil, fmt.Errorf("edit would break syntax, nothing written: %w\ndiff that was rejected:\n%s", err, res.Diff)
		}
	}
	if req.DryRun {
		return res, nil
	}
	if _, err := editWriteFile(path, newContent, writeOpts{validate: false}); err != nil {
		return nil, err
	}
	return res, nil
}

func applyAnchorEdit(lines []string, req editRequest, res *editResult) ([]string, error) {
	start, err := ParseAnchor(req.Anchor)
	if err != nil {
		return nil, err
	}
	startIdx, drift, err := ResolveAnchor(lines, start, req.Drift)
	if err != nil {
		return nil, err
	}
	res.Drift = drift

	endIdx := startIdx
	if req.EndAnchor != "" {
		end, err := ParseAnchor(req.EndAnchor)
		if err != nil {
			return nil, fmt.Errorf("end anchor: %w", err)
		}
		endIdx, _, err = ResolveAnchor(lines, end, req.Drift)
		if err != nil {
			return nil, fmt.Errorf("end anchor: %w", err)
		}
		if endIdx < startIdx {
			return nil, fmt.Errorf("end anchor (line %d) precedes start anchor (line %d)", endIdx+1, startIdx+1)
		}
	}

	newLines, _ := SplitLines(req.NewText)
	if req.NewText == "" {
		newLines = []string{}
	} else if !strings.HasSuffix(req.NewText, "\n") {
		newLines, _ = SplitLines(req.NewText + "\n")
	}

	out := make([]string, 0, len(lines)+len(newLines))
	switch {
	case req.Delete:
		res.Operation = fmt.Sprintf("delete lines %d-%d", startIdx+1, endIdx+1)
		out = append(out, lines[:startIdx]...)
		out = append(out, lines[endIdx+1:]...)
	case req.Insert == "before":
		if len(newLines) == 0 {
			return nil, fmt.Errorf("--insert requires --new-text")
		}
		res.Operation = fmt.Sprintf("insert %d line(s) before line %d", len(newLines), startIdx+1)
		out = append(out, lines[:startIdx]...)
		out = append(out, newLines...)
		out = append(out, lines[startIdx:]...)
	case req.Insert == "after":
		if len(newLines) == 0 {
			return nil, fmt.Errorf("--insert requires --new-text")
		}
		res.Operation = fmt.Sprintf("insert %d line(s) after line %d", len(newLines), endIdx+1)
		out = append(out, lines[:endIdx+1]...)
		out = append(out, newLines...)
		out = append(out, lines[endIdx+1:]...)
	case req.Insert != "":
		return nil, fmt.Errorf("invalid --insert %q: want before or after", req.Insert)
	default:
		if req.NewText == "" {
			return nil, fmt.Errorf("replace requires --new-text (use --delete to remove lines)")
		}
		res.Operation = fmt.Sprintf("replace lines %d-%d with %d line(s)", startIdx+1, endIdx+1, len(newLines))
		out = append(out, lines[:startIdx]...)
		out = append(out, newLines...)
		out = append(out, lines[endIdx+1:]...)
	}
	return out, nil
}

func applyStringEdit(lines []string, original string, req editRequest, res *editResult, trailingNL *bool) ([]string, error) {
	count := strings.Count(original, req.OldString)
	if count == 0 {
		return nil, fmt.Errorf("old string not found — re-read the file, content may have changed")
	}
	if count > 1 && !req.ReplaceAll {
		return nil, fmt.Errorf("old string matches %d times — add surrounding context to disambiguate, or pass --replace-all", count)
	}

	var newContent string
	if req.ReplaceAll {
		newContent = strings.ReplaceAll(original, req.OldString, req.NewString)
		res.Operation = fmt.Sprintf("replace %d occurrence(s)", count)
	} else {
		newContent = strings.Replace(original, req.OldString, req.NewString, 1)
		res.Operation = "replace 1 occurrence"
	}
	updated, tnl := SplitLines(newContent)
	*trailingNL = tnl
	return updated, nil
}

func applySymbolEdit(lines []string, path, original string, req editRequest, res *editResult) ([]string, error) {
	outline := parseOutline(path, []byte(original))
	if outline.Engine == "none" {
		return nil, fmt.Errorf("no AST engine for %s files — use --anchor or --old-string", outline.Language)
	}
	hits := findSymbol(outline, req.Symbol)
	if len(hits) == 0 {
		available := make([]string, 0, len(outline.Symbols))
		for _, s := range outline.Symbols {
			available = append(available, s.Kind+" "+s.Name)
		}
		return nil, fmt.Errorf("symbol %q not found (engine: %s). Top-level symbols: %s",
			req.Symbol, outline.Engine, strings.Join(available, ", "))
	}
	if len(hits) > 1 {
		var locs []string
		for _, h := range hits {
			locs = append(locs, fmt.Sprintf("%s %s (lines %d-%d)", h.Kind, h.Name, h.StartLine, h.EndLine))
		}
		return nil, fmt.Errorf("symbol %q is ambiguous: %s — use the qualified name or --anchor",
			req.Symbol, strings.Join(locs, "; "))
	}
	sym := hits[0]
	startIdx, endIdx := sym.StartLine-1, sym.EndLine-1
	if startIdx < 0 || endIdx >= len(lines) || startIdx > endIdx {
		return nil, fmt.Errorf("engine %s returned invalid range %d-%d for %q", outline.Engine, sym.StartLine, sym.EndLine, req.Symbol)
	}

	newLines, _ := SplitLines(strings.TrimRight(req.NewText, "\n") + "\n")
	if req.NewText == "" {
		newLines = []string{}
	}

	out := make([]string, 0, len(lines)+len(newLines))
	switch {
	case req.Delete:
		res.Operation = fmt.Sprintf("delete %s %s (lines %d-%d, engine %s)", sym.Kind, sym.Name, sym.StartLine, sym.EndLine, outline.Engine)
		out = append(out, lines[:startIdx]...)
		out = append(out, lines[endIdx+1:]...)
	case req.Insert == "before":
		if len(newLines) == 0 {
			return nil, fmt.Errorf("--insert requires --new-text")
		}
		res.Operation = fmt.Sprintf("insert %d line(s) before %s %s (engine %s)", len(newLines), sym.Kind, sym.Name, outline.Engine)
		out = append(out, lines[:startIdx]...)
		out = append(out, newLines...)
		out = append(out, lines[startIdx:]...)
	case req.Insert == "after":
		if len(newLines) == 0 {
			return nil, fmt.Errorf("--insert requires --new-text")
		}
		res.Operation = fmt.Sprintf("insert %d line(s) after %s %s (engine %s)", len(newLines), sym.Kind, sym.Name, outline.Engine)
		out = append(out, lines[:endIdx+1]...)
		out = append(out, newLines...)
		out = append(out, lines[endIdx+1:]...)
	case req.Insert != "":
		return nil, fmt.Errorf("invalid --insert %q: want before or after", req.Insert)
	default:
		if len(newLines) == 0 {
			return nil, fmt.Errorf("replace requires --new-text (use --delete to remove the symbol)")
		}
		res.Operation = fmt.Sprintf("replace %s %s (lines %d-%d -> %d line(s), engine %s)",
			sym.Kind, sym.Name, sym.StartLine, sym.EndLine, len(newLines), outline.Engine)
		out = append(out, lines[:startIdx]...)
		out = append(out, newLines...)
		out = append(out, lines[endIdx+1:]...)
	}
	return out, nil
}

func unifiedDiff(path string, before, after []string) string {
	p := 0
	for p < len(before) && p < len(after) && before[p] == after[p] {
		p++
	}
	s := 0
	for s < len(before)-p && s < len(after)-p && before[len(before)-1-s] == after[len(after)-1-s] {
		s++
	}
	bStart, bEnd := p, len(before)-s
	aStart, aEnd := p, len(after)-s
	if bStart == bEnd && aStart == aEnd {
		return ""
	}

	ctx := 2
	cStart := bStart - ctx
	if cStart < 0 {
		cStart = 0
	}
	cEndB := bEnd + ctx
	if cEndB > len(before) {
		cEndB = len(before)
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "--- %s\n+++ %s\n", path, path)
	fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n", cStart+1, cEndB-cStart, cStart+1, (aEnd-aStart)+(cEndB-cStart)-(bEnd-bStart))
	for i := cStart; i < bStart; i++ {
		fmt.Fprintf(&buf, " %s\n", before[i])
	}
	for i := bStart; i < bEnd; i++ {
		fmt.Fprintf(&buf, "-%s\n", before[i])
	}
	for i := aStart; i < aEnd; i++ {
		fmt.Fprintf(&buf, "+%s\n", after[i])
	}
	for i := bEnd; i < cEndB; i++ {
		fmt.Fprintf(&buf, " %s\n", before[i])
	}
	return buf.String()
}

// write — atomic, validated file writing. Replaces naive native write:
// temp-file + fsync + rename (never a half-written file), syntax pre-validation
// before anything touches disk (Go via go/parser, JSON via encoding/json,
// bracket-balance heuristic elsewhere), and optional backup.

var (
	writeContent     string
	writeStdin       bool
	writeNoValidate  bool
	writeBackup      bool
	writeMkdir       bool
	writeFormat      string
	writeStdinReader = io.Reader(os.Stdin)
)

var writeAbsPath = filepath.Abs

var WriteCmd = &cobra.Command{
	Use:   "write [path]",
	Short: "Write files atomically with syntax pre-validation",
	Long: `Atomic file writing: content is validated, written to a temp file in the
target directory, fsynced, then renamed over the destination. A crash or
validation failure never leaves a corrupt file behind.

Validation (skip with --no-validate):
  .go    full parse via go/parser
  .json  encoding/json
  other  bracket/brace/paren balance heuristic (string/comment aware)

Examples:
  sin-code write pkg/new.go --content "$(cat /tmp/draft.go)"
  cat draft.json | sin-code write config.json --stdin --backup
  sin-code write docs/new/file.md --stdin --mkdir < notes.md`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := writeAbsPath(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		content := writeContent
		if writeStdin {
			data, err := io.ReadAll(writeStdinReader)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			content = string(data)
		}
		result, err := writeFileAtomic(absPath, content, writeOpts{
			validate: !writeNoValidate,
			backup:   writeBackup,
			mkdir:    writeMkdir,
		})
		if err != nil {
			return err
		}
		if writeFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("wrote %s (%d bytes, %d lines)%s\n", result.Path, result.Bytes, result.Lines,
			map[bool]string{true: " [backup: " + result.BackupPath + "]", false: ""}[result.BackupPath != ""])
		return nil
	},
}

func init() {
	WriteCmd.Flags().StringVarP(&writeContent, "content", "c", "", "Content to write")
	WriteCmd.Flags().BoolVar(&writeStdin, "stdin", false, "Read content from stdin")
	WriteCmd.Flags().BoolVar(&writeNoValidate, "no-validate", false, "Skip syntax pre-validation")
	WriteCmd.Flags().BoolVar(&writeBackup, "backup", false, "Keep a .bak copy of the previous content")
	WriteCmd.Flags().BoolVar(&writeMkdir, "mkdir", false, "Create parent directories if missing")
	WriteCmd.Flags().StringVarP(&writeFormat, "format", "f", "text", "Output: text, json")
}

type writeOpts struct {
	validate bool
	backup   bool
	mkdir    bool
}

type writeResult struct {
	Path       string `json:"path"`
	Bytes      int    `json:"bytes"`
	Lines      int    `json:"lines"`
	Created    bool   `json:"created"`
	Validated  bool   `json:"validated"`
	BackupPath string `json:"backup_path,omitempty"`
}

// writeHooks abstracts the file-system operations in writeFileAtomic so
// every error branch can be exercised without depending on real disk faults.
// Production uses the default hooks; tests override individual functions.
type writeHooks struct {
	createTemp func(dir, pattern string) (*os.File, error)
	writeAll   func(w io.Writer, data []byte) (int, error)
	syncFile   func(f *os.File) error
	closeFile  func(f *os.File) error
	chmod      func(name string, mode os.FileMode) error
	rename     func(oldpath, newpath string) error
	remove     func(name string) error
	readFile   func(name string) ([]byte, error)
	writeFile  func(name string, data []byte, perm os.FileMode) error
	mkdirAll   func(path string, perm os.FileMode) error
	stat       func(name string) (os.FileInfo, error)
}

var defaultWriteHooks = writeHooks{
	createTemp: os.CreateTemp,
	writeAll:   func(w io.Writer, data []byte) (int, error) { return w.Write(data) },
	syncFile:   func(f *os.File) error { return f.Sync() },
	closeFile:  func(f *os.File) error { return f.Close() },
	chmod:      os.Chmod,
	rename:     os.Rename,
	remove:     os.Remove,
	readFile:   os.ReadFile,
	writeFile:  os.WriteFile,
	mkdirAll:   os.MkdirAll,
	stat:       os.Stat,
}

// writeHooksCurrent is the active hook set. It is reset after each test by
// TestMain, but tests can override individual fields directly.
var writeHooksCurrent = defaultWriteHooks

func writeFileAtomic(path, content string, opts writeOpts) (*writeResult, error) {
	hooks := writeHooksCurrent
	return writeFileAtomicWithHooks(path, content, opts, hooks)
}

func writeFileAtomicWithHooks(path, content string, opts writeOpts, hooks writeHooks) (*writeResult, error) {
	if opts.validate {
		if err := validateSyntax(path, content); err != nil {
			return nil, fmt.Errorf("validation failed, nothing written: %w", err)
		}
	}

	dir := filepath.Dir(path)
	if opts.mkdir {
		if err := hooks.mkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating parent directories: %w", err)
		}
	}
	if _, err := hooks.stat(dir); err != nil {
		return nil, fmt.Errorf("parent directory missing: %s (use --mkdir)", dir)
	}

	res := &writeResult{Path: path, Bytes: len(content), Validated: opts.validate}

	prevInfo, statErr := hooks.stat(path)
	res.Created = statErr != nil
	mode := os.FileMode(0644)
	if statErr == nil {
		mode = prevInfo.Mode().Perm()
		if opts.backup {
			bak := path + ".bak"
			prev, err := hooks.readFile(path)
			if err != nil {
				return nil, fmt.Errorf("reading previous content for backup: %w", err)
			}
			if err := hooks.writeFile(bak, prev, mode); err != nil {
				return nil, fmt.Errorf("writing backup: %w", err)
			}
			res.BackupPath = bak
		}
	}

	tmp, err := hooks.createTemp(dir, ".sin-write-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = hooks.closeFile(tmp); _ = hooks.remove(tmpName) }

	if _, err := hooks.writeAll(tmp, []byte(content)); err != nil {
		cleanup()
		return nil, fmt.Errorf("writing temp file: %w", err)
	}
	if err := hooks.syncFile(tmp); err != nil {
		cleanup()
		return nil, fmt.Errorf("fsync: %w", err)
	}
	if err := hooks.closeFile(tmp); err != nil {
		_ = hooks.remove(tmpName)
		return nil, fmt.Errorf("closing temp file: %w", err)
	}
	if err := hooks.chmod(tmpName, mode); err != nil {
		_ = hooks.remove(tmpName)
		return nil, fmt.Errorf("chmod: %w", err)
	}
	if err := hooks.rename(tmpName, path); err != nil {
		_ = hooks.remove(tmpName)
		return nil, fmt.Errorf("atomic rename: %w", err)
	}

	lines, _ := SplitLines(content)
	res.Lines = len(lines)
	return res, nil
}

func validateSyntax(path, content string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, filepath.Base(path), content, parser.AllErrors); err != nil {
			return fmt.Errorf("go syntax: %v", err)
		}
		return nil
	case ".json":
		var v any
		if err := json.Unmarshal([]byte(content), &v); err != nil {
			return fmt.Errorf("json syntax: %v", err)
		}
		return nil
	case ".md", ".txt", ".log", "":
		return nil
	default:
		return checkBracketBalance(path, content)
	}
}

func checkBracketBalance(path, content string) error {
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	var stack []rune
	inString := rune(0)
	escaped := false
	lineComment := false
	line := 1

	for _, r := range content {
		if r == '\n' {
			line++
			lineComment = false
			if inString == '\'' || inString == '"' {
				inString = 0
			}
			continue
		}
		if lineComment {
			continue
		}
		if inString != 0 {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == inString {
				inString = 0
			}
			continue
		}
		switch r {
		case '"', '\'', '`':
			inString = r
		case '#':
			if isHashCommentLang(path) {
				lineComment = true
			}
		case '(', '[', '{':
			stack = append(stack, r)
		case ')', ']', '}':
			if len(stack) == 0 || stack[len(stack)-1] != pairs[r] {
				return fmt.Errorf("bracket balance: unexpected %q at line %d (use --no-validate to override)", r, line)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		return fmt.Errorf("bracket balance: %d unclosed brackets — content looks truncated (use --no-validate to override)", len(stack))
	}
	return nil
}

func isHashCommentLang(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py", ".rb", ".sh", ".bash", ".yaml", ".yml", ".toml", ".pl", ".r":
		return true
	}
	return false
}
