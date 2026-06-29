// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when complexity analyzer is refactored
package complexity

import (
	"go/ast"
	"go/token"
	"path/filepath"
)

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

func flattenFieldNames(fl *ast.FieldList) []string {
	var names []string
	for _, f := range fl.List {
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
	}
	return names
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
