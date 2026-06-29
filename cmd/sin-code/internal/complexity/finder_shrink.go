// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when complexity analyzer is refactored
package complexity

import (
	"fmt"
	"go/ast"
	"go/token"
)

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

func exprString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprString(e.X) + "." + e.Sel.Name
	}
	return ""
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
