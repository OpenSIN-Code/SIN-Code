// SPDX-License-Identifier: MIT
// Purpose: Spec↔Code signature drift. Reads the Go source tree with
// go/parser, finds the functions named in the spec's backtick-wrapped
// signature requirements, and reports any drift. PR 3 adds Python
// (via python3 + ast subprocess) and JSON (structural shape match).
// Docs: docs/SPEC-LAYER.md §"Drift detection (the hardening)"
package spec

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
)

// signaturePattern matches a Go function signature in backticks
// inside a spec requirement. The name can be a plain identifier
// (`Foo`), a method receiver (`Receiver.Method`), a pointer
// receiver (`*Receiver.Method`), or a generic type or method
// (`Box[T]`, `Box[T].Get`). Captures the parameter list (in
// parentheses) and an optional return list (after the closing
// paren).
var signaturePattern = regexp.MustCompile("`(\\*?[A-Za-z_][A-Za-z0-9_]*(?:\\[[^]]+\\])?(?:\\.[A-Za-z_][A-Za-z0-9_]*(?:\\[[^]]+\\])?)?)\\s*\\(([^)]*)\\)\\s*([^`]*)`")

// pySignaturePattern matches a Python function signature in backticks
// (e.g. `def foo(x: int, y: str) -> str`). Captures the function
// name, parameter list, and optional return-list. Mirrors the Go
// pattern's shape.
var pySignaturePattern = regexp.MustCompile("`def\\s+([A-Za-z_][A-Za-z0-9_]*)\\s*\\(([^)]*)\\)(?:\\s*->\\s*([^`]+))?`")

// SignatureHit is one requirement that names a function signature.
// Kind discriminates Go vs Python (JSON shapes use JSONShapeHit
// instead because their structure is different).
type SignatureHit struct {
	Kind          string // "go" or "python"
	RequirementID string
	FuncName      string
	RawParamText  string
	RawResultText string
	Code          string
	Match         bool
	Note          string
}

// JSONShapeHit is one requirement that names a JSON object shape
// (e.g. `{"name": str, "id": int}`).
type JSONShapeHit struct {
	RequirementID string
	ShapeText     string // the spec shape, as written
	MatchedFile   string // path of a JSON file that matched, "" if none
	Match         bool   // true if any JSON file satisfies the shape
	Note          string
}

// DriftReport aggregates signature hits across the spec.
type DriftReport struct {
	SpecPath string
	Hits     []SignatureHit
	JSON     []JSONShapeHit
}

// HasFailures reports whether any signature requirement was not
// matched in the code.
func (d *DriftReport) HasFailures() bool {
	for _, h := range d.Hits {
		if !h.Match {
			return true
		}
	}
	for _, j := range d.JSON {
		if !j.Match {
			return true
		}
	}
	return false
}

// DetectSignatureDrift walks the spec's requirements, extracts
// backtick-wrapped Go/Python signatures and JSON shapes, and
// checks each one against the source tree under root. Returns a
// report with one hit per matched requirement; unmatched
// requirements have Match=false.
func (s *Spec) DetectSignatureDrift(root string) (*DriftReport, error) {
	return s.DetectSignatureDriftWithPython(root, "")
}

// DetectSignatureDriftWithPython is the same as DetectSignatureDrift
// but lets the caller override the python3 binary path (used by
// tests on minimal images that have only `python`, not `python3`).
func (s *Spec) DetectSignatureDriftWithPython(root, pythonBin string) (*DriftReport, error) {
	rep := &DriftReport{SpecPath: s.Path}

	// 1. Build the function lookups.
	gofuncs, err := parseGoFuncs(root)
	if err != nil {
		return nil, fmt.Errorf("spec: drift: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var pyfuncs map[string][]pyFunc
	if hasPySigs(s) || hasJSONShapes(s) {
		pyfuncs, err = parsePythonFuncs(ctx, root, pythonBin)
		if err != nil {
			// Don't hard-fail: a missing python3 just means we
			// can't check Python signatures. Surface as a Note on
			// the relevant hits.
			pyfuncs = nil
		}
	}
	var jsonfiles []jsonFile
	if hasJSONShapes(s) {
		jsonfiles, _ = parseJSONFiles(root)
	}

	// 2. For each requirement, extract every signature/shape.
	for _, r := range s.Requirements {
		// 2a. Go signatures.
		for _, m := range signaturePattern.FindAllStringSubmatch(r.Text, -1) {
			hit := SignatureHit{
				Kind:          "go",
				RequirementID: r.ID,
				FuncName:      m[1],
				RawParamText:  strings.TrimSpace(m[2]),
				RawResultText: strings.TrimSpace(m[3]),
			}
			candidates, ok := gofuncs[hit.FuncName]
			if !ok {
				hit.Note = "function not found in source tree"
				rep.Hits = append(rep.Hits, hit)
				continue
			}
			matched := false
			for _, c := range candidates {
				if signatureEqual(hit.RawParamText, hit.RawResultText, c.params, c.results) {
					hit.Code = c.code
					hit.Match = true
					matched = true
					break
				}
			}
			if !matched {
				hit.Code = candidates[0].code
				hit.Note = fmt.Sprintf("Go signature drift: spec is `%s(%s) %s`, code is `%s`",
					hit.FuncName, hit.RawParamText, hit.RawResultText, candidates[0].code)
			}
			rep.Hits = append(rep.Hits, hit)
		}
		// 2b. Python signatures.
		for _, m := range pySignaturePattern.FindAllStringSubmatch(r.Text, -1) {
			hit := SignatureHit{
				Kind:          "python",
				RequirementID: r.ID,
				FuncName:      m[1],
				RawParamText:  strings.TrimSpace(m[2]),
				RawResultText: strings.TrimSpace(m[3]),
			}
			if pyfuncs == nil {
				hit.Note = "python3 not available; skipping Python signature check"
				rep.Hits = append(rep.Hits, hit)
				continue
			}
			candidates, ok := pyfuncs[hit.FuncName]
			if !ok {
				hit.Note = "Python function not found in source tree"
				rep.Hits = append(rep.Hits, hit)
				continue
			}
			hit.Code = candidates[0].Code
			hit.Match = true // v0: presence is enough; full param match is PR 4
			rep.Hits = append(rep.Hits, hit)
		}
		// 2c. JSON shapes.
		for _, j := range extractJSONShapes(r.Text) {
			jhit := JSONShapeHit{
				RequirementID: r.ID,
				ShapeText:     j.Raw,
			}
			matched := false
			for _, f := range jsonfiles {
				if ok, note := jsonMatch(j.Shape, f.Value, j.Strict); ok {
					jhit.MatchedFile = f.Path
					jhit.Match = true
					matched = true
					break
				} else {
					jhit.Note = note
				}
			}
			if !matched {
				if jhit.Note == "" {
					jhit.Note = "no JSON file matches this shape"
				}
			}
			rep.JSON = append(rep.JSON, jhit)
		}
	}

	sort.SliceStable(rep.Hits, func(i, j int) bool {
		if rep.Hits[i].Kind != rep.Hits[j].Kind {
			return rep.Hits[i].Kind < rep.Hits[j].Kind
		}
		if rep.Hits[i].RequirementID != rep.Hits[j].RequirementID {
			return rep.Hits[i].RequirementID < rep.Hits[j].RequirementID
		}
		return rep.Hits[i].FuncName < rep.Hits[j].FuncName
	})
	sort.SliceStable(rep.JSON, func(i, j int) bool { return rep.JSON[i].RequirementID < rep.JSON[j].RequirementID })
	return rep, nil
}

// hasPySigs reports whether any requirement contains a Python
// `def f(...)` signature. Cheaper than re-running the regex on
// every drift check.
func hasPySigs(s *Spec) bool {
	for _, r := range s.Requirements {
		if pySignaturePattern.MatchString(r.Text) {
			return true
		}
	}
	return false
}

// hasJSONShapes reports whether any requirement contains a
// backtick-wrapped JSON object literal.
func hasJSONShapes(s *Spec) bool {
	for _, r := range s.Requirements {
		if jsonPattern.MatchString(r.Text) {
			return true
		}
	}
	return false
}

// --- Go AST helpers -------------------------------------------------------

type goFunc struct {
	params  string // canonicalized param list, e.g. "x int, y string"
	results string // canonicalized result list, e.g. "error"
	code    string // the source line, e.g. "func Foo(x int, y string) error {"
}

// parseGoFuncs walks the Go source tree under root and returns a
// map from function name to the list of overloads (parameter +
// result canonicalized). The walk is recursive and skips
// auto-generated files and test files (best-effort; we don't
// fail the whole drift check on a single unparseable file).
// parseGoFuncs walks the Go source tree and returns a map keyed by
// function name (plain) or "Receiver.Method" (methods). Pointer
// receivers get a "*Receiver" qualifier; value receivers use the bare
// type name.
func parseGoFuncs(root string) (map[string][]goFunc, error) {
	out := map[string][]goFunc{}
	fset := token.NewFileSet()
	err := walkGoFiles(root, func(path string) {
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return // skip unparseable files
		}
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Name == nil {
				return true
			}
			name := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				// Method: qualify with the receiver type. Pointer
				// receivers are written "*T.Method", value receivers
				// are "T.Method". This matches the canonical Go
				// calling convention used in spec requirements.
				recvType := receiverTypeName(fn.Recv.List[0].Type)
				if recvType != "" {
					name = recvType + "." + name
				}
			}
			out[name] = append(out[name], goFunc{
				params:  canonicalFieldList(fn.Type.Params),
				results: canonicalFieldList(fn.Type.Results),
				code:    renderFuncLine(fset, fn),
			})
			return true
		})
	})
	return out, err
}

// receiverTypeName extracts a readable name from a method receiver's
// type expression. Returns "" for anonymous or unparseable receivers.
func receiverTypeName(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + receiverTypeName(v.X)
	case *ast.IndexExpr:
		// Generic receiver: T[int] -> "T[int]"
		return exprText(v.X) + "[" + exprText(v.Index) + "]"
	}
	return ""
}

// canonicalFieldList renders an AST FieldList as the canonical
// "name type, name type" form for comparison. Names are joined
// by "," and types are extracted verbatim (incl. pointers, slices,
// generics). For our v0 drift check this is good enough; full
// type-equivalence checking is PR 3.
func canonicalFieldList(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	var parts []string
	for _, f := range fl.List {
		typ := exprText(f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, typ)
			continue
		}
		var names []string
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		parts = append(parts, strings.Join(names, ", ")+" "+typ)
	}
	return strings.Join(parts, ", ")
}

func exprText(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return "*" + exprText(v.X)
	case *ast.SelectorExpr:
		return exprText(v.X) + "." + v.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprText(v.Elt)
	case *ast.MapType:
		return "map[" + exprText(v.Key) + "]" + exprText(v.Value)
	case *ast.Ellipsis:
		return "..." + exprText(v.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	}
	return "?"
}

// renderFuncLine extracts the source-text line of the function
// declaration. Best-effort: falls back to a synthesized form.
func renderFuncLine(fset *token.FileSet, fn *ast.FuncDecl) string {
	if fn.Pos().IsValid() {
		pos := fset.Position(fn.Pos())
		if pos.Line > 0 {
			return fmt.Sprintf("func %s(%s) %s   (at %s:%d)",
				fn.Name.Name,
				canonicalFieldList(fn.Type.Params),
				canonicalFieldList(fn.Type.Results),
				pos.Filename, pos.Line)
		}
	}
	return fmt.Sprintf("func %s(%s) %s",
		fn.Name.Name,
		canonicalFieldList(fn.Type.Params),
		canonicalFieldList(fn.Type.Results))
}

// signatureEqual returns true if the spec's param/result text is
// canonical-equal to the code's. The comparison is whitespace-
// tolerant but otherwise literal; no type-equivalence.
func signatureEqual(specParams, specResults, codeParams, codeResults string) bool {
	return canonicalize(specParams) == canonicalize(codeParams) &&
		canonicalize(specResults) == canonicalize(codeResults)
}

func canonicalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// walkGoFiles is a small filesystem walker. The full SIN-Code repo
// has a custom walker (cmd/sin-code/internal/walk); for spec drift
// we use a minimal version to keep this package dependency-free.
func walkGoFiles(root string, fn func(path string)) error {
	return walkGo(root, fn)
}
