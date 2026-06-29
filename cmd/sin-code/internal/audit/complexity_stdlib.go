// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when audit is refactored
// Purpose: stdlib-tag detector — hand-rolled reimplementations of stdlib functions.
package audit

import (
	"fmt"
	"go/ast"
)

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
