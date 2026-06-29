// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when complexity analyzer is refactored
package complexity

import (
	"fmt"
	"go/ast"
)

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
