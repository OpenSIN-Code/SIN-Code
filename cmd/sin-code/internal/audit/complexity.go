// SPDX-License-Identifier: MIT
// Purpose: complexity audit — repo-wide ponytail-audit analog for SIN-Code.
// Scans Go trees for structural bloat (single-impl interfaces, single-product
// factories, wrappers, one-export files, dead flags, hand-rolled stdlib) and
// emits findings in the ponytail 5-tag format.
// Docs: docs/complexity-audit.md
package audit

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Five ponytail-style tags.
const (
	TagDelete = "delete"
	TagStdlib = "stdlib"
	TagNative = "native"
	TagYagni  = "yagni"
	TagShrink = "shrink"
)

var allTags = []string{TagDelete, TagStdlib, TagNative, TagYagni, TagShrink}

// LLM is the optional second-pass verifier. Deterministic tests supply a stub.
type LLM interface {
	Judge(ctx context.Context, filePath, content string, candidates []Finding) ([]Finding, error)
}

// Finding is one one-line complexity finding.
type Finding struct {
	Tag         string `json:"tag"`
	Problem     string `json:"problem"`
	Replacement string `json:"replacement"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	LineCount   int    `json:"line_count"` // estimated lines removable
	Approved    bool   `json:"approved,omitempty"`
	Approver    string `json:"approver,omitempty"`
}

// Result aggregates all findings and the final net delta.
type Result struct {
	Findings      []Finding `json:"findings"`
	NetLines      int       `json:"net_lines"`
	DepsRemovable int       `json:"deps_removable"`
	Status        string    `json:"status"`
}

// Options configure the audit run.
type Options struct {
	Tags      []string // allowed tags, empty = all
	Rank      string   // "lines" or "deps"
	TopN      int      // LLM judge only sees top N static findings
	SinceRef  string   // git ref; not implemented in static pass
	MaxNet    int      // fail threshold for strict mode
	Strict    bool
	NoLLM     bool // skip LLM pass
	SinDebtRE *regexp.Regexp
}

// DefaultSinDebtRE matches // sin-debt: ... markers.
func DefaultSinDebtRE() *regexp.Regexp {
	return regexp.MustCompile(`(?i)//\s*sin-debt:\s*(.+)$`)
}

// Auditor performs a repo-wide complexity scan.
type Auditor struct {
	LLM LLM
}

// NewAuditor creates an auditor with an optional LLM second pass.
func NewAuditor(llm LLM) *Auditor {
	return &Auditor{LLM: llm}
}

// Audit scans root and returns a result. The static pass is deterministic and
// LLM-free; the optional LLM pass only reviews the top-N candidates.
func (a *Auditor) Audit(ctx context.Context, root string, opts Options) (*Result, error) {
	if opts.SinDebtRE == nil {
		opts.SinDebtRE = DefaultSinDebtRE()
	}
	if opts.Tags == nil {
		opts.Tags = append([]string(nil), allTags...)
	}
	allowed := make(map[string]bool, len(opts.Tags))
	for _, t := range opts.Tags {
		allowed[strings.ToLower(strings.TrimSpace(t))] = true
	}

	files, err := goFiles(root)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	for _, f := range files {
		// #nosec G304 — f is a file discovered under the user-supplied audit root.
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		src := string(data)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, data, parser.ParseComments)
		if err != nil {
			continue
		}
		pkg := packageInfo{f: file, src: src, fset: fset, path: f}
		findings = append(findings, staticPass(pkg, allowed, opts.SinDebtRE)...)
	}

	if !opts.NoLLM && a.LLM != nil && len(findings) > 0 {
		for i, f := range findings {
			if i >= opts.TopN && opts.TopN > 0 {
				break
			}
			// #nosec G304 — path is the same user-supplied audit root file.
			data, err := os.ReadFile(f.Path)
			if err != nil {
				continue
			}
			extra, err := a.LLM.Judge(ctx, f.Path, string(data), []Finding{f})
			if err != nil {
				continue
			}
			if len(extra) > 0 {
				findings = append(findings, extra...)
			}
		}
	}

	// Approve findings that sit on or directly after a sin-debt marker.
	for i := range findings {
		findings[i].Approved, findings[i].Approver = approvedBySinDebt(findings[i], opts.SinDebtRE)
	}

	// Filter by allowed tags and remove duplicates by (path, line, tag).
	filtered, seen := findings[:0], make(map[string]bool)
	for _, f := range findings {
		if !allowed[strings.ToLower(f.Tag)] {
			continue
		}
		key := fmt.Sprintf("%s:%d:%s:%s", f.Path, f.Line, f.Tag, f.Problem)
		if seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, f)
	}
	findings = filtered

	if opts.Rank == "deps" {
		// deps rank = stdlib/native tags first, then line count
		sort.Slice(findings, func(i, j int) bool {
			if tagRank(findings[i].Tag) != tagRank(findings[j].Tag) {
				return tagRank(findings[i].Tag) < tagRank(findings[j].Tag)
			}
			return findings[i].LineCount > findings[j].LineCount
		})
	} else {
		// default rank = lines saved, descending
		sort.Slice(findings, func(i, j int) bool {
			return findings[i].LineCount > findings[j].LineCount
		})
	}

	result := aggregate(findings, opts.MaxNet)
	if opts.Strict && result.NetLines > opts.MaxNet {
		return result, fmt.Errorf("complexity net-lines %d exceeds threshold %d", result.NetLines, opts.MaxNet)
	}
	return result, nil
}

func tagRank(tag string) int {
	switch strings.ToLower(tag) {
	case TagStdlib:
		return 0
	case TagNative:
		return 1
	case TagYagni:
		return 2
	case TagDelete:
		return 3
	case TagShrink:
		return 4
	}
	return 9
}

type packageInfo struct {
	f    *ast.File
	src  string
	fset *token.FileSet
	path string
}

// goFiles returns all .go files under root, excluding _test.go and vendor.
func goFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == "node_modules" || name == ".git" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// staticPass runs deterministic detectors on a single file.
func staticPass(pkg packageInfo, allowed map[string]bool, debtRE *regexp.Regexp) []Finding {
	var findings []Finding
	findings = append(findings, detectSingleImplInterfaces(pkg, allowed)...)
	findings = append(findings, detectSingleProductFactories(pkg, allowed)...)
	findings = append(findings, detectWrappers(pkg, allowed)...)
	findings = append(findings, detectOneExportFiles(pkg, allowed)...)
	findings = append(findings, detectDeadFlags(pkg, allowed)...)
	findings = append(findings, detectHandRolledStdlib(pkg, allowed)...)
	return findings
}

func detectSingleImplInterfaces(pkg packageInfo, allowed map[string]bool) []Finding {
	if !allowed[TagYagni] {
		return nil
	}
	var findings []Finding
	for _, decl := range pkg.f.Decls {
		d, ok := decl.(*ast.GenDecl)
		if !ok || d.Tok != token.TYPE {
			continue
		}
		for _, spec := range d.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			it, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			start, end := lineRange(pkg.fset, ts)
			findings = append(findings, Finding{
				Tag:         TagYagni,
				Problem:     fmt.Sprintf("interface %s has only one likely implementation", ts.Name.Name),
				Replacement: fmt.Sprintf("inline %s as concrete type", ts.Name.Name),
				Path:        pkg.path,
				Line:        start,
				LineCount:   end - start + 1,
			})
			_ = it
		}
	}
	return findings
}

func detectSingleProductFactories(pkg packageInfo, allowed map[string]bool) []Finding {
	if !allowed[TagYagni] {
		return nil
	}
	var findings []Finding
	for _, decl := range pkg.f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "New") {
			continue
		}
		start, end := lineRange(pkg.fset, fn)
		findings = append(findings, Finding{
			Tag:         TagYagni,
			Problem:     fmt.Sprintf("factory %s may have only one product/caller", fn.Name.Name),
			Replacement: "collapse to direct constructor",
			Path:        pkg.path,
			Line:        start,
			LineCount:   end - start + 1,
		})
	}
	return findings
}

func detectWrappers(pkg packageInfo, allowed map[string]bool) []Finding {
	if !allowed[TagShrink] {
		return nil
	}
	var findings []Finding
	for _, decl := range pkg.f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || len(fn.Body.List) != 1 {
			continue
		}
		stmt, ok := fn.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(stmt.Results) != 1 {
			continue
		}
		if _, call := stmt.Results[0].(*ast.CallExpr); call {
			start, _ := lineRange(pkg.fset, fn)
			findings = append(findings, Finding{
				Tag:         TagShrink,
				Problem:     fmt.Sprintf("wrapper %s only delegates", fn.Name.Name),
				Replacement: "call inner function directly",
				Path:        pkg.path,
				Line:        start,
				LineCount:   3,
			})
		}
	}
	return findings
}

func detectOneExportFiles(pkg packageInfo, allowed map[string]bool) []Finding {
	if !allowed[TagShrink] {
		return nil
	}
	exported := 0
	for _, decl := range pkg.f.Decls {
		exported += countExportedDecls(decl)
	}
	if exported == 1 {
		return []Finding{{
			Tag:         TagShrink,
			Problem:     "file exports only one top-level symbol",
			Replacement: "merge into package or caller module",
			Path:        pkg.path,
			Line:        1,
			LineCount:   strings.Count(pkg.src, "\n") + 1,
		}}
	}
	return nil
}

func countExportedDecls(decl ast.Decl) int {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if ast.IsExported(d.Name.Name) {
			return 1
		}
	case *ast.GenDecl:
		count := 0
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				if ast.IsExported(s.Name.Name) {
					count++
				}
			case *ast.ValueSpec:
				for _, n := range s.Names {
					if ast.IsExported(n.Name) {
						count++
					}
				}
			}
		}
		return count
	}
	return 0
}

func detectDeadFlags(pkg packageInfo, allowed map[string]bool) []Finding {
	if !allowed[TagDelete] {
		return nil
	}
	var findings []Finding
	for _, decl := range pkg.f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				if !strings.Contains(n.Name, "Flag") && !strings.Contains(n.Name, "Config") {
					continue
				}
				if !isRead(pkg.f, n.Name) {
					start, _ := lineRange(pkg.fset, vs)
					findings = append(findings, Finding{
						Tag:         TagDelete,
						Problem:     fmt.Sprintf("flag/config %s appears never read", n.Name),
						Replacement: "remove unused configuration",
						Path:        pkg.path,
						Line:        start,
						LineCount:   1,
					})
				}
			}
		}
	}
	return findings
}

func isRead(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if found {
			return false
		}
		ident, ok := n.(*ast.Ident)
		if !ok || ident.Name != name {
			return true
		}
		// simple heuristic: name used in a non-declaration position
		if ident.Obj == nil || ident.Obj.Kind != ast.Var {
			found = true
			return false
		}
		return true
	})
	return found
}

func detectHandRolledStdlib(pkg packageInfo, allowed map[string]bool) []Finding {
	if !allowed[TagStdlib] {
		return nil
	}
	var findings []Finding
	ast.Inspect(pkg.f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		fn, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		name := fn.Name
		line := pkg.fset.Position(call.Pos()).Line
		replacement := ""
		switch name {
		case "contains":
			replacement = "use strings.Contains"
		case "sortInts":
			replacement = "use sort.Ints"
		case "reverse":
			replacement = "use slices.Reverse"
		default:
			return true
		}
		findings = append(findings, Finding{
			Tag:         TagStdlib,
			Problem:     fmt.Sprintf("hand-rolled %s duplicates stdlib", name),
			Replacement: replacement,
			Path:        pkg.path,
			Line:        line,
			LineCount:   3,
		})
		return true
	})
	return findings
}

func lineRange(fset *token.FileSet, n ast.Node) (int, int) {
	return fset.Position(n.Pos()).Line, fset.Position(n.End()).Line
}

func approvedBySinDebt(f Finding, debtRE *regexp.Regexp) (bool, string) {
	// #nosec G304 — f.Path is a file from the user-supplied audit root.
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return false, ""
	}
	lines := strings.Split(string(data), "\n")
	if f.Line < 1 || f.Line > len(lines) {
		return false, ""
	}
	// sin-debt markers usually sit on the line directly above the finding.
	start := f.Line - 3
	if start < 0 {
		start = 0
	}
	for i := start; i < f.Line && i < len(lines); i++ {
		m := debtRE.FindStringSubmatch(lines[i])
		if m != nil {
			return true, strings.TrimSpace(m[1])
		}
	}
	return false, ""
}

func aggregate(findings []Finding, maxNet int) *Result {
	r := &Result{Findings: findings}
	for _, f := range findings {
		if f.Approved {
			continue
		}
		r.NetLines += f.LineCount
		if f.Tag == TagStdlib || f.Tag == TagNative {
			r.DepsRemovable++
		}
	}
	if len(findings) == 0 {
		r.Status = "Lean already. Ship."
		r.NetLines = 0
		r.DepsRemovable = 0
		return r
	}
	if maxNet > 0 && r.NetLines > maxNet {
		r.Status = fmt.Sprintf("net: -%d lines, -%d deps possible. (exceeds threshold %d)", r.NetLines, r.DepsRemovable, maxNet)
		return r
	}
	r.Status = fmt.Sprintf("net: -%d lines, -%d deps possible.", r.NetLines, r.DepsRemovable)
	return r
}

// FormatFinding returns the one-line ponytail format.
func FormatFinding(f Finding) string {
	approved := ""
	if f.Approved {
		approved = fmt.Sprintf(" (approved: sin-debt marker %s)", f.Approver)
	}
	return fmt.Sprintf("%s: %s. %s. [%s:%d]%s", f.Tag, f.Problem, f.Replacement, f.Path, f.Line, approved)
}

// FormatResult returns the full text report.
func FormatResult(r *Result, format string) string {
	if format == "json" {
		return ""
	}
	var sb strings.Builder
	for _, f := range r.Findings {
		sb.WriteString(FormatFinding(f))
		sb.WriteString("\n")
	}
	sb.WriteString(r.Status)
	sb.WriteString("\n")
	return sb.String()
}

// ValidateTags returns an error if any tag is unknown.
func ValidateTags(tags []string) error {
	known := map[string]bool{TagDelete: true, TagStdlib: true, TagNative: true, TagYagni: true, TagShrink: true}
	for _, t := range tags {
		if !known[strings.ToLower(strings.TrimSpace(t))] {
			return fmt.Errorf("unknown tag: %s", t)
		}
	}
	return nil
}
