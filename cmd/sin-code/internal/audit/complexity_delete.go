// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when audit is refactored
// Purpose: delete-tag detectors — dead flags and unread configuration variables.
package audit

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

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
		if ident.Obj == nil || ident.Obj.Kind != ast.Var {
			found = true
			return false
		}
		return true
	})
	return found
}
