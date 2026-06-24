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

type parsedFile struct {
	pkg packageInfo
}

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

// DefaultSinDebtRE matches "sin-debt:" markers.
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
	pkgFiles := make(map[string][]string)
	for _, f := range files {
		dir := filepath.Dir(f)
		pkgFiles[dir] = append(pkgFiles[dir], f)
	}
	for _, files := range pkgFiles {
		pkgFindings := packagePass(files, allowed, opts.SinDebtRE)
		findings = append(findings, pkgFindings...)
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

// packagePass runs detectors across all files in a package directory,
// enabling cross-file interface implementation counting.
func packagePass(files []string, allowed map[string]bool, debtRE *regexp.Regexp) []Finding {
	var allFindings []Finding
	var parsed []parsedFile
	for _, f := range files {
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
		parsed = append(parsed, parsedFile{pkg: packageInfo{f: file, src: src, fset: fset, path: f}})
	}
	if len(parsed) == 0 {
		return nil
	}
	pkgIfaces := make(map[string][]string)
	pkgRecvMethods := make(map[string]map[string]struct{})
	for _, p := range parsed {
		for _, decl := range p.pkg.f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}
				var methods []string
				for _, m := range it.Methods.List {
					for _, n := range m.Names {
						methods = append(methods, n.Name)
					}
				}
				if len(methods) > 0 {
					pkgIfaces[ts.Name.Name] = methods
				}
			}
		}
	}
	for _, p := range parsed {
		for _, decl := range p.pkg.f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recvName := ""
			if id, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
				recvName = id.Name
			} else if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
				if id, ok := star.X.(*ast.Ident); ok {
					recvName = id.Name
				}
			}
			if recvName == "" {
				continue
			}
			if pkgRecvMethods[recvName] == nil {
				pkgRecvMethods[recvName] = make(map[string]struct{})
			}
			pkgRecvMethods[recvName][fn.Name.Name] = struct{}{}
		}
	}
	implCounts := make(map[string]int)
	for ifaceName, required := range pkgIfaces {
		count := 0
		for _, methods := range pkgRecvMethods {
			allPresent := true
			for _, req := range required {
				if _, ok := methods[req]; !ok {
					allPresent = false
					break
				}
			}
			if allPresent {
				count++
			}
		}
		implCounts[ifaceName] = count
	}
	for _, p := range parsed {
		pkgFindings := staticPassWithPkg(p.pkg, allowed, debtRE, pkgIfaces, implCounts, parsed)
		allFindings = append(allFindings, pkgFindings...)
	}
	return allFindings
}

// staticPassWithPkg runs file-level detectors plus cross-file interface detection.
func staticPassWithPkg(pkg packageInfo, allowed map[string]bool, debtRE *regexp.Regexp, pkgIfaces map[string][]string, implCounts map[string]int, allPkgFiles []parsedFile) []Finding {
	var findings []Finding
	findings = append(findings, detectSingleImplInterfacesWithCounts(pkg, allowed, pkgIfaces, implCounts)...)
	findings = append(findings, detectSingleProductFactoriesWithCounts(pkg, allowed, pkgIfaces, implCounts)...)
	findings = append(findings, detectWrappers(pkg, allowed)...)
	findings = append(findings, detectOneExportFiles(pkg, allowed)...)
	findings = append(findings, detectDeadFlagsPkg(pkg, allowed, allPkgFiles)...)
	findings = append(findings, detectHandRolledStdlib(pkg, allowed)...)
	return findings
}

func detectSingleImplInterfacesWithCounts(pkg packageInfo, allowed map[string]bool, pkgIfaces map[string][]string, implCounts map[string]int) []Finding {
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
			if _, has := pkgIfaces[ts.Name.Name]; !has {
				continue
			}
			if implCounts[ts.Name.Name] != 1 {
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

func detectSingleProductFactoriesWithCounts(pkg packageInfo, allowed map[string]bool, pkgIfaces map[string][]string, implCounts map[string]int) []Finding {
	if !allowed[TagYagni] {
		return nil
	}
	var findings []Finding
	for _, decl := range pkg.f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "New") {
			continue
		}
		if fn.Type.Results == nil {
			continue
		}
		returnsIface := false
		for _, field := range fn.Type.Results.List {
			name := ""
			if id, ok := field.Type.(*ast.Ident); ok {
				name = id.Name
			} else if star, ok := field.Type.(*ast.StarExpr); ok {
				if id, ok := star.X.(*ast.Ident); ok {
					name = id.Name
				}
			}
			if _, has := pkgIfaces[name]; has && implCounts[name] == 1 {
				returnsIface = true
				break
			}
		}
		if !returnsIface {
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

// staticPass runs deterministic detectors on a single file (legacy, used by tests).
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
	ifaceMethods := make(map[string][]string)
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
			var methods []string
			for _, m := range it.Methods.List {
				for _, n := range m.Names {
					methods = append(methods, n.Name)
				}
			}
			if len(methods) == 0 {
				continue
			}
			ifaceMethods[ts.Name.Name] = methods
		}
	}
	recvMethodSets := make(map[string]map[string]struct{})
	for _, decl := range pkg.f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
			continue
		}
		recvName := ""
		if id, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
			recvName = id.Name
		} else if star, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
			if id, ok := star.X.(*ast.Ident); ok {
				recvName = id.Name
			}
		}
		if recvName == "" {
			continue
		}
		if recvMethodSets[recvName] == nil {
			recvMethodSets[recvName] = make(map[string]struct{})
		}
		recvMethodSets[recvName][fn.Name.Name] = struct{}{}
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
			required, has := ifaceMethods[ts.Name.Name]
			if !has || len(required) == 0 {
				continue
			}
			count := 0
			for tname, methods := range recvMethodSets {
				allPresent := true
				for _, req := range required {
					if _, ok := methods[req]; !ok {
						allPresent = false
						break
					}
				}
				if allPresent {
					count++
					_ = tname
				}
			}
			if count != 1 {
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
	ifaceNames := make(map[string]bool)
	for _, decl := range pkg.f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if _, ok := ts.Type.(*ast.InterfaceType); ok {
				ifaceNames[ts.Name.Name] = true
			}
		}
	}
	var findings []Finding
	for _, decl := range pkg.f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "New") {
			continue
		}
		if fn.Type.Results == nil {
			continue
		}
		returnsIface := false
		for _, field := range fn.Type.Results.List {
			name := ""
			if id, ok := field.Type.(*ast.Ident); ok {
				name = id.Name
			} else if star, ok := field.Type.(*ast.StarExpr); ok {
				if id, ok := star.X.(*ast.Ident); ok {
					name = id.Name
				}
			}
			if ifaceNames[name] {
				returnsIface = true
				break
			}
		}
		if !returnsIface {
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

func returnsCobraCommand(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, field := range fn.Type.Results.List {
		if star, ok := field.Type.(*ast.StarExpr); ok {
			if sel, ok := star.X.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == "cobra" && sel.Sel.Name == "Command" {
					return true
				}
			}
		}
	}
	return false
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
		call, ok := stmt.Results[0].(*ast.CallExpr)
		if !ok {
			continue
		}
		// Skip wrappers that call functions from external packages — those are
		// adapter/facade patterns, not same-package delegation.
		if isExternalCall(call) {
			continue
		}
		// Skip method wrappers — methods that delegate are usually implementing
		// an interface or providing a facade. Inlining would break the contract.
		if fn.Recv != nil {
			continue
		}
		// Skip test hooks: package-level vars that are overridable for tests.
		if isLikelyTestHook(fn, call, pkg) {
			continue
		}
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
	return findings
}

func isExternalCall(call *ast.CallExpr) bool {
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
		if _, ok := sel.X.(*ast.Ident); ok {
			return true
		}
	}
	return false
}

func isLikelyTestHook(fn *ast.FuncDecl, call *ast.CallExpr, pkg packageInfo) bool {
	// Heuristic: if the function name matches a package-level var name pattern
	// (e.g. fn "foo" and var "fooFn" or "fooHook"), it's likely a test seam.
	if fn.Name == nil {
		return false
	}
	name := fn.Name.Name
	for _, decl := range pkg.f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, n := range vs.Names {
				if n.Name == name+"Fn" || n.Name == name+"Hook" || n.Name == name+"Impl" {
					return true
				}
			}
		}
	}
	return false
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

func detectDeadFlagsPkg(pkg packageInfo, allowed map[string]bool, allPkgFiles []parsedFile) []Finding {
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
				if isTestHookName(n.Name) {
					continue
				}
				readAnywhere := false
				for _, pf := range allPkgFiles {
					if isRead(pf.pkg.f, n.Name) {
						readAnywhere = true
						break
					}
				}
				if !readAnywhere {
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
				// Skip test hooks: variables whose names suggest they are
				// overridden in test files (Hook, test, Test prefix, etc.).
				if isTestHookName(n.Name) {
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

func isTestHookName(name string) bool {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "hook") {
		return true
	}
	if strings.HasPrefix(name, "test") || strings.HasPrefix(name, "Test") {
		return true
	}
	if strings.HasSuffix(lower, "hook") || strings.HasSuffix(lower, "fn") {
		return true
	}
	return false
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
	// sin-debt markers usually sit within 10 lines above the finding,
	// possibly separated by doc comments.
	start := f.Line - 10
	if start < 0 {
		start = 0
	}
	for i := start; i < f.Line && i < len(lines); i++ {
		m := debtRE.FindStringSubmatch(lines[i])
		if m != nil {
			return true, strings.TrimSpace(m[1])
		}
	}
	// For file-level findings (Line == 1), scan the entire file for a marker.
	if f.Line == 1 {
		for i := 0; i < len(lines); i++ {
			m := debtRE.FindStringSubmatch(lines[i])
			if m != nil {
				return true, strings.TrimSpace(m[1])
			}
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
