// SPDX-License-Identifier: MIT
// Purpose: Spec↔Code signature drift. Reads the Go source tree with
// go/parser, finds the functions named in the spec's backtick-wrapped
// signature requirements, and reports any drift. PR 2 covers Go only;
// Python (via subprocess) and JSON Schema are deferred to PR 3.
// Docs: docs/SPEC-LAYER.md §"Drift detection (the hardening)"
package spec

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"sort"
	"strings"
)

// signaturePattern matches a Go function signature in backticks inside
// a spec requirement. Captures the name, parameter list (in
// parentheses), and an optional return list (after the closing
// paren). The return list is allowed to be empty for void functions
// like `Bar()`. Whitespace and line breaks inside the signature
// are tolerated by the canonicalize() step, not the regex.
var signaturePattern = regexp.MustCompile("`([A-Za-z_][A-Za-z0-9_]*)\\s*\\(([^)]*)\\)\\s*([^`]*)`")

// SignatureHit is one requirement that names a Go function signature.
type SignatureHit struct {
	RequirementID string // e.g. "R1"
	FuncName      string // e.g. "Foo"
	RawParamText  string // exact text of the param list, as captured
	RawResultText string // exact text of the result list
	Code         string // the matching code line, "" if not found
	Match        bool   // true if the code matches the spec
	Note         string // human-readable drift message if any
}

// DriftReport aggregates signature hits across the spec.
type DriftReport struct {
	SpecPath string
	Hits     []SignatureHit
}

// HasFailures reports whether any signature requirement was not
// matched in the code.
func (d *DriftReport) HasFailures() bool {
	for _, h := range d.Hits {
		if !h.Match {
			return true
		}
	}
	return false
}

// DetectSignatureDrift walks the spec's requirements, extracts
// backtick-wrapped Go signatures, and checks each one against the
// Go source tree under root. Returns a report with one hit per
// matched requirement; unmatched requirements have Match=false.
func (s *Spec) DetectSignatureDrift(root string) (*DriftReport, error) {
	rep := &DriftReport{SpecPath: s.Path}

	// 1. Build a function lookup: name -> [{params, results, code}]
	funcs, err := parseGoFuncs(root)
	if err != nil {
		return nil, fmt.Errorf("spec: drift: %w", err)
	}

	// 2. For each requirement, try to extract a signature.
	for _, r := range s.Requirements {
		for _, m := range signaturePattern.FindAllStringSubmatch(r.Text, -1) {
			hit := SignatureHit{
				RequirementID: r.ID,
				FuncName:      m[1],
				RawParamText:  strings.TrimSpace(m[2]),
				RawResultText: strings.TrimSpace(m[3]),
			}
			// 3. Look up the function and compare.
			candidates, ok := funcs[hit.FuncName]
			if !ok {
				hit.Note = "function not found in source tree"
				rep.Hits = append(rep.Hits, hit)
				continue
			}
			// Pick the first candidate whose signature matches the spec.
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
				// Report the first candidate as the closest match.
				hit.Code = candidates[0].code
				hit.Note = fmt.Sprintf("signature drift: spec is `%s(%s) %s`, code is `%s`",
					hit.FuncName, hit.RawParamText, hit.RawResultText, candidates[0].code)
			}
			rep.Hits = append(rep.Hits, hit)
		}
	}

	// Stable order for tests and human reading.
	sort.SliceStable(rep.Hits, func(i, j int) bool {
		if rep.Hits[i].RequirementID != rep.Hits[j].RequirementID {
			return rep.Hits[i].RequirementID < rep.Hits[j].RequirementID
		}
		return rep.Hits[i].FuncName < rep.Hits[j].FuncName
	})
	return rep, nil
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
			if !ok || fn.Recv != nil { // skip methods (this is the v0; method support is PR 3)
				return true
			}
			if fn.Name == nil {
				return true
			}
			out[fn.Name.Name] = append(out[fn.Name.Name], goFunc{
				params:  canonicalFieldList(fn.Type.Params),
				results: canonicalFieldList(fn.Type.Results),
				code:    renderFuncLine(fset, fn),
			})
			return true
		})
	})
	return out, err
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
