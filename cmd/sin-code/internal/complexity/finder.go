// SPDX-License-Identifier: MIT
// Purpose: static complexity finder — ponytail 5-tag review (delete/stdlib/native/yagni/shrink).
package complexity

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Options controls the scope of the static review.
type Options struct {
	Root      string
	SinceRef  string
	Tags      []string
	MarkerMap map[string][]Marker
}

// Find runs the static complexity analysis over the selected files.
func Find(opts Options) ([]Finding, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("root path: %w", err)
	}
	if opts.MarkerMap == nil {
		opts.MarkerMap = make(map[string][]Marker)
	}
	if len(opts.Tags) == 0 {
		opts.Tags = AllTags
	}
	tagSet := make(map[string]bool, len(opts.Tags))
	for _, t := range opts.Tags {
		tagSet[t] = true
	}

	files, err := selectFiles(root, opts.SinceRef)
	if err != nil {
		return nil, err
	}

	byDir := groupByDir(files)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		findings []Finding
		errCh    = make(chan error, len(byDir))
	)
	for dir, fs := range byDir {
		wg.Add(1)
		go func(dir string, fs []string) {
			defer wg.Done()
			pkg, err := parsePackage(root, dir, fs)
			if err != nil {
				errCh <- err
				return
			}
			local := analyzePackage(pkg, tagSet, opts.MarkerMap)
			mu.Lock()
			findings = append(findings, local...)
			mu.Unlock()
		}(dir, fs)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			return nil, e
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

// selectFiles returns the .go files to analyze. If sinceRef is set, it analyzes
// every .go file in directories touched by the diff.
func selectFiles(root, sinceRef string) ([]string, error) {
	if sinceRef != "" {
		cmd := exec.Command("git", "diff", "--name-only", sinceRef)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git diff --name-only %s: %w", sinceRef, err)
		}
		dirs := make(map[string]struct{})
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" || !strings.HasSuffix(line, ".go") {
				continue
			}
			dirs[filepath.Dir(line)] = struct{}{}
		}
		var files []string
		for dir := range dirs {
			entries, err := os.ReadDir(filepath.Join(root, dir))
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				n := e.Name()
				if strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
					files = append(files, filepath.Join(root, dir, n))
				}
			}
		}
		return files, nil
	}

	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") || strings.Contains(path, "/vendor/") || strings.Contains(path, "\\vendor\\") {
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return files, nil
}

func groupByDir(files []string) map[string][]string {
	out := make(map[string][]string)
	for _, f := range files {
		dir := filepath.Dir(f)
		out[dir] = append(out[dir], f)
	}
	return out
}

// packageInfo holds parsed state for one directory of Go source.
type packageInfo struct {
	root        string
	dir         string
	files       []fileInfo
	fset        *token.FileSet
	interfaces  map[string]*ast.InterfaceType
	typeSpecs   map[string]*ast.TypeSpec
	topFuncs    map[string]*ast.FuncDecl
	recvMethods map[string]map[string]struct{}
	imports     []importInfo
}

type fileInfo struct {
	absPath string
	relPath string
	astFile *ast.File
	lines   int
}

type importInfo struct {
	path    string
	alias   string
	line    int
	fileRel string
}

func parsePackage(root, dir string, files []string) (*packageInfo, error) {
	pkg := &packageInfo{
		root:        root,
		dir:         dir,
		fset:        token.NewFileSet(),
		interfaces:  make(map[string]*ast.InterfaceType),
		typeSpecs:   make(map[string]*ast.TypeSpec),
		topFuncs:    make(map[string]*ast.FuncDecl),
		recvMethods: make(map[string]map[string]struct{}),
	}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(pkg.fset, path, src, parser.ParseComments|parser.AllErrors)
		if err != nil && f == nil {
			continue
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "" {
			rel = path
		}
		tf := pkg.fset.File(f.Pos())
		fi := fileInfo{absPath: path, relPath: rel, astFile: f, lines: tf.LineCount()}
		pkg.files = append(pkg.files, fi)

		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ImportSpec:
						imp := importInfo{fileRel: rel, line: pkg.fset.Position(s.Pos()).Line}
						if s.Path != nil {
							imp.path = strings.Trim(s.Path.Value, `"`)
						}
						if s.Name != nil {
							imp.alias = s.Name.Name
						}
						pkg.imports = append(pkg.imports, imp)
					case *ast.TypeSpec:
						pkg.typeSpecs[s.Name.Name] = s
						if iface, ok := s.Type.(*ast.InterfaceType); ok {
							pkg.interfaces[s.Name.Name] = iface
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					pkg.topFuncs[d.Name.Name] = d
					continue
				}
				recvName := receiverTypeName(d.Recv)
				if recvName == "" {
					continue
				}
				if pkg.recvMethods[recvName] == nil {
					pkg.recvMethods[recvName] = make(map[string]struct{})
				}
				pkg.recvMethods[recvName][d.Name.Name] = struct{}{}
			}
		}
	}
	if len(pkg.files) == 0 {
		return nil, fmt.Errorf("no parseable files in %s", dir)
	}
	return pkg, nil
}

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	if ptr, ok := expr.(*ast.StarExpr); ok {
		expr = ptr.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func analyzePackage(pkg *packageInfo, tags map[string]bool, markers map[string][]Marker) []Finding {
	var findings []Finding
	if tags[TagYagni] {
		findings = append(findings, analyzeSingleImplInterfaces(pkg, markers)...)
		findings = append(findings, analyzeOneProductFactories(pkg, markers)...)
	}
	if tags[TagShrink] {
		findings = append(findings, analyzeWrapperFunctions(pkg, markers)...)
		findings = append(findings, analyzeRepeatAppendLoops(pkg, markers)...)
	}
	if tags[TagDelete] {
		findings = append(findings, analyzeDeadConfigFlags(pkg, markers)...)
	}
	if tags[TagStdlib] {
		findings = append(findings, analyzeManualMinMax(pkg, markers)...)
	}
	if tags[TagNative] {
		findings = append(findings, analyzeNativeImports(pkg, markers)...)
	}
	if tags[TagYagni] {
		findings = append(findings, analyzeOneExportFiles(pkg, markers, findings)...)
	}
	return findings
}

// analyzeSingleImplInterfaces flags interfaces with exactly one implementing type.
func analyzeSingleImplInterfaces(pkg *packageInfo, markers map[string][]Marker) []Finding {
	var findings []Finding
	for name, iface := range pkg.interfaces {
		required := interfaceMethodNames(iface)
		if len(required) == 0 {
			continue
		}
		var impl string
		count := 0
		for tname, methods := range pkg.recvMethods {
			if implements(methods, required) {
				impl = tname
				count++
			}
		}
		if count != 1 {
			continue
		}
		start, end := posLines(pkg, iface.Pos(), iface.End())
		rel := fileRelForPos(pkg, iface.Pos())
		findings = append(findings, Finding{
			Tag:         TagYagni,
			What:        fmt.Sprintf("Interface %s has one implementation (%s)", name, impl),
			Replacement: "Inline it until a second implementation exists",
			Path:        rel,
			Line:        start,
			EndLine:     end,
			LineCount:   lineCount(start, end),
			ApprovedBy:  markerFor(markers, rel, start, end),
		})
	}
	return findings
}

func interfaceMethodNames(iface *ast.InterfaceType) []string {
	var names []string
	for _, m := range iface.Methods.List {
		for _, n := range m.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

func implements(methods map[string]struct{}, required []string) bool {
	for _, r := range required {
		if _, ok := methods[r]; !ok {
			return false
		}
	}
	return true
}

// analyzeOneProductFactories flags functions returning an interface that has only one implementation.
func analyzeOneProductFactories(pkg *packageInfo, markers map[string][]Marker) []Finding {
	var findings []Finding
	implCounts := make(map[string]int)
	implName := make(map[string]string)
	for name, iface := range pkg.interfaces {
		required := interfaceMethodNames(iface)
		if len(required) == 0 {
			continue
		}
		count := 0
		var single string
		for tname, methods := range pkg.recvMethods {
			if implements(methods, required) {
				single = tname
				count++
			}
		}
		implCounts[name] = count
		implName[name] = single
	}
	for _, fn := range pkg.topFuncs {
		if fn.Type.Results == nil {
			continue
		}
		for _, field := range fn.Type.Results.List {
			name := identName(field.Type)
			if name == "" {
				continue
			}
			if implCounts[name] != 1 {
				continue
			}
			start, end := posLines(pkg, fn.Pos(), fn.End())
			rel := fileRelForPos(pkg, fn.Pos())
			findings = append(findings, Finding{
				Tag:         TagYagni,
				What:        fmt.Sprintf("Factory %s returns interface %s with one product", fn.Name.Name, name),
				Replacement: fmt.Sprintf("Return concrete type %s directly", implName[name]),
				Path:        rel,
				Line:        start,
				EndLine:     end,
				LineCount:   lineCount(start, end),
				ApprovedBy:  markerFor(markers, rel, start, end),
			})
		}
	}
	return findings
}

func identName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return identName(e.X)
	case *ast.ArrayType:
		return ""
	}
	return ""
}

// analyzeWrapperFunctions flags functions that do nothing but forward arguments.
func analyzeWrapperFunctions(pkg *packageInfo, markers map[string][]Marker) []Finding {
	var findings []Finding
	for _, fn := range pkg.topFuncs {
		if fn.Body == nil || fn.Type.Params == nil {
			continue
		}
		params := flattenFieldNames(fn.Type.Params)
		if len(params) == 0 {
			continue
		}
		if len(fn.Body.List) != 1 {
			continue
		}
		ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}
		call, ok := ret.Results[0].(*ast.CallExpr)
		if !ok || len(call.Args) != len(params) {
			continue
		}
		match := true
		for i, arg := range call.Args {
			id, ok := arg.(*ast.Ident)
			if !ok || id.Name != params[i] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		callee := exprString(call.Fun)
		start, end := posLines(pkg, fn.Pos(), fn.End())
		rel := fileRelForPos(pkg, fn.Pos())
		findings = append(findings, Finding{
			Tag:         TagShrink,
			What:        fmt.Sprintf("Function %s only delegates to %s", fn.Name.Name, callee),
			Replacement: fmt.Sprintf("Replace calls with %s", callee),
			Path:        rel,
			Line:        start,
			EndLine:     end,
			LineCount:   lineCount(start, end),
			ApprovedBy:  markerFor(markers, rel, start, end),
		})
	}
	return findings
}

func flattenFieldNames(fl *ast.FieldList) []string {
	var names []string
	for _, f := range fl.List {
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
	}
	return names
}

func exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	}
	return ""
}

// analyzeOneExportFiles flags Go files that export exactly one symbol and have
// no other finding in the same file.
func analyzeOneExportFiles(pkg *packageInfo, markers map[string][]Marker, existing []Finding) []Finding {
	var findings []Finding
	fileWithFinding := make(map[string]struct{})
	for _, f := range existing {
		fileWithFinding[f.Path] = struct{}{}
	}
	for _, fi := range pkg.files {
		if fi.astFile.Name.Name == "main" {
			continue
		}
		exported := exportedNames(fi.astFile)
		if len(exported) != 1 {
			continue
		}
		if _, ok := fileWithFinding[fi.relPath]; ok {
			continue
		}
		name := exported[0]
		findings = append(findings, Finding{
			Tag:         TagYagni,
			What:        fmt.Sprintf("File exports only %s", name),
			Replacement: "Merge into callers or remove the thin file",
			Path:        fi.relPath,
			Line:        1,
			EndLine:     fi.lines,
			LineCount:   fi.lines,
			ApprovedBy:  markerForPath(markers, fi.relPath),
		})
	}
	return findings
}

func exportedNames(f *ast.File) []string {
	var names []string
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if ast.IsExported(d.Name.Name) {
				names = append(names, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if ast.IsExported(s.Name.Name) {
						names = append(names, s.Name.Name)
					}
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if ast.IsExported(n.Name) {
							names = append(names, n.Name)
						}
					}
				}
			}
		}
	}
	return names
}

// analyzeDeadConfigFlags flags unreferenced package-level flag-like variables.
func analyzeDeadConfigFlags(pkg *packageInfo, markers map[string][]Marker) []Finding {
	var findings []Finding
	type varInfo struct {
		name      string
		startLine int
		endLine   int
		fileRel   string
		typeName  string
	}
	var vars []varInfo
	for _, fi := range pkg.files {
		for _, decl := range fi.astFile.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				var typeName string
				if vs.Type != nil {
					typeName = identName(vs.Type)
				}
				start, end := posLines(pkg, vs.Pos(), vs.End())
				for _, n := range vs.Names {
					if ast.IsExported(n.Name) {
						continue
					}
					vars = append(vars, varInfo{
						name:      n.Name,
						startLine: start,
						endLine:   end,
						fileRel:   fi.relPath,
						typeName:  typeName,
					})
				}
			}
		}
	}
	if len(vars) == 0 {
		return nil
	}
	usage := make(map[string]int)
	for _, fi := range pkg.files {
		ast.Inspect(fi.astFile, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			line := pkg.fset.Position(id.Pos()).Line
			for _, v := range vars {
				if id.Name != v.name {
					continue
				}
				if line < v.startLine || line > v.endLine {
					usage[v.name]++
				}
			}
			return true
		})
	}
	for _, v := range vars {
		if usage[v.name] > 0 {
			continue
		}
		if !looksLikeFlag(v.name, v.typeName) {
			continue
		}
		findings = append(findings, Finding{
			Tag:         TagDelete,
			What:        fmt.Sprintf("Unused flag variable %s", v.name),
			Replacement: "Remove the variable and its registration",
			Path:        v.fileRel,
			Line:        v.startLine,
			EndLine:     v.endLine,
			LineCount:   lineCount(v.startLine, v.endLine),
			ApprovedBy:  markerFor(markers, v.fileRel, v.startLine, v.endLine),
		})
	}
	return findings
}

func looksLikeFlag(name, typeName string) bool {
	if typeName != "string" && typeName != "bool" && typeName != "int" && typeName != "int64" {
		return false
	}
	lower := strings.ToLower(name)
	return strings.Contains(lower, "flag") || strings.Contains(lower, "option") ||
		strings.Contains(lower, "param") || strings.Contains(lower, "config") ||
		strings.Contains(lower, "setting")
}

// analyzeManualMinMax flags hand-rolled min/max functions.
func analyzeManualMinMax(pkg *packageInfo, markers map[string][]Marker) []Finding {
	var findings []Finding
	for _, fn := range pkg.topFuncs {
		if fn.Name.Name != "min" && fn.Name.Name != "max" {
			continue
		}
		if fn.Type.Params == nil || fn.Body == nil {
			continue
		}
		paramNames := flattenFieldNames(fn.Type.Params)
		if len(paramNames) != 2 || !bodyIsMinMax(fn.Body, paramNames) {
			continue
		}
		start, end := posLines(pkg, fn.Pos(), fn.End())
		rel := fileRelForPos(pkg, fn.Pos())
		findings = append(findings, Finding{
			Tag:         TagStdlib,
			What:        fmt.Sprintf("Hand-rolled %s function", fn.Name.Name),
			Replacement: fmt.Sprintf("Use the built-in %s function", fn.Name.Name),
			Path:        rel,
			Line:        start,
			EndLine:     end,
			LineCount:   lineCount(start, end),
			ApprovedBy:  markerFor(markers, rel, start, end),
		})
	}
	return findings
}

// bodyIsMinMax accepts either a single if/else or an if-then followed by a return.
func bodyIsMinMax(body *ast.BlockStmt, params []string) bool {
	if len(body.List) == 1 {
		ifStmt, ok := body.List[0].(*ast.IfStmt)
		if !ok || ifStmt.Else == nil {
			return false
		}
		cond, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok || (cond.Op != token.LSS && cond.Op != token.GTR) {
			return false
		}
		elseBlock, ok := ifStmt.Else.(*ast.BlockStmt)
		if !ok {
			return false
		}
		return bodyReturnsParam(ifStmt.Body, params) && bodyReturnsParam(elseBlock, params)
	}
	if len(body.List) == 2 {
		ifStmt, ok := body.List[0].(*ast.IfStmt)
		if !ok || ifStmt.Else != nil {
			return false
		}
		cond, ok := ifStmt.Cond.(*ast.BinaryExpr)
		if !ok || (cond.Op != token.LSS && cond.Op != token.GTR) {
			return false
		}
		ret, ok := body.List[1].(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return false
		}
		return bodyReturnsParam(ifStmt.Body, params) && isParam(ret.Results[0], params)
	}
	return false
}

func bodyReturnsParam(body *ast.BlockStmt, names []string) bool {
	if len(body.List) != 1 {
		return false
	}
	ret, ok := body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	return isParam(ret.Results[0], names)
}

func isParam(expr ast.Expr, names []string) bool {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	for _, n := range names {
		if n == id.Name {
			return true
		}
	}
	return false
}

// nativeReplacements maps dependency imports to platform/stdlib equivalents.
var nativeReplacements = map[string]string{
	"github.com/pkg/errors": "errors.New / fmt.Errorf with %w",
}

// analyzeNativeImports flags imports that duplicate stdlib/platform features.
func analyzeNativeImports(pkg *packageInfo, markers map[string][]Marker) []Finding {
	var findings []Finding
	for _, imp := range pkg.imports {
		replacement, ok := nativeReplacements[imp.path]
		if !ok {
			continue
		}
		findings = append(findings, Finding{
			Tag:         TagNative,
			What:        fmt.Sprintf("Import %s duplicates stdlib/platform behavior", imp.path),
			Replacement: replacement,
			Path:        imp.fileRel,
			Line:        imp.line,
			EndLine:     imp.line,
			LineCount:   1,
			DepsRemoved: []string{imp.path},
			ApprovedBy:  markerForLine(markers, imp.fileRel, imp.line),
		})
	}
	return findings
}

// analyzeRepeatAppendLoops flags C-style loops that repeatedly append the same element.
func analyzeRepeatAppendLoops(pkg *packageInfo, markers map[string][]Marker) []Finding {
	var findings []Finding
	for _, fi := range pkg.files {
		ast.Inspect(fi.astFile, func(n ast.Node) bool {
			stmt, ok := n.(*ast.ForStmt)
			if !ok {
				return true
			}
			if !isRepeatAppendLoop(stmt) {
				return true
			}
			start, end := posLines(pkg, stmt.Pos(), stmt.End())
			findings = append(findings, Finding{
				Tag:         TagShrink,
				What:        "Loop repeats append to build a slice",
				Replacement: "Use slices.Repeat",
				Path:        fi.relPath,
				Line:        start,
				EndLine:     end,
				LineCount:   lineCount(start, end),
				ApprovedBy:  markerFor(markers, fi.relPath, start, end),
			})
			return true
		})
	}
	return findings
}

func isRepeatAppendLoop(stmt *ast.ForStmt) bool {
	if stmt.Init == nil || stmt.Cond == nil || stmt.Post == nil || stmt.Body == nil {
		return false
	}
	assign, ok := stmt.Init.(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	idx, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || idx.Name != "i" {
		return false
	}
	initLit, ok := assign.Rhs[0].(*ast.BasicLit)
	if !ok || initLit.Value != "0" {
		return false
	}
	cond, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS {
		return false
	}
	if _, ok := cond.X.(*ast.Ident); !ok {
		return false
	}
	if _, ok := cond.Y.(*ast.Ident); !ok {
		return false
	}
	inc, ok := stmt.Post.(*ast.IncDecStmt)
	if !ok || inc.Tok != token.INC {
		return false
	}
	incID, ok := inc.X.(*ast.Ident)
	if !ok || incID.Name != idx.Name {
		return false
	}
	if len(stmt.Body.List) != 1 {
		return false
	}
	body, ok := stmt.Body.List[0].(*ast.AssignStmt)
	if !ok || body.Tok != token.ASSIGN || len(body.Rhs) != 1 {
		return false
	}
	call, ok := body.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "append" {
		return false
	}
	return true
}

func posLines(pkg *packageInfo, pos, end token.Pos) (int, int) {
	return pkg.fset.Position(pos).Line, pkg.fset.Position(end).Line
}

func fileRelForPos(pkg *packageInfo, pos token.Pos) string {
	file := pkg.fset.File(pos)
	if file == nil {
		return ""
	}
	rel, _ := filepath.Rel(pkg.root, file.Name())
	if rel == "" {
		return file.Name()
	}
	return rel
}

func lineCount(start, end int) int {
	if end < start {
		end = start
	}
	return end - start + 1
}
