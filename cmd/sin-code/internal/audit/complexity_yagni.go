// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when audit is refactored
// Purpose: YAGNI-tag detectors — single-impl interfaces and single-product factories.
package audit

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

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
