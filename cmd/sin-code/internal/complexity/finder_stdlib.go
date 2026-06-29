// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when complexity analyzer is refactored
package complexity

import (
	"fmt"
	"go/ast"
	"go/token"
)

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
