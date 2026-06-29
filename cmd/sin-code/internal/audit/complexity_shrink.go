// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when audit is refactored
// Purpose: shrink-tag detectors — wrappers and one-export files.
package audit

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

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
		if isExternalCall(call) {
			continue
		}
		if fn.Recv != nil {
			continue
		}
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
