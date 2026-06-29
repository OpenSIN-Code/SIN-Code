// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when complexity analyzer is refactored
package complexity

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
)

// analyzeDeadConfigFlags flags unreferenced package-level flag-like variables.
func analyzeDeadConfigFlags(pkg *packageInfo, markers map[string][]Marker) []Finding {
	var findings []Finding
	type varInfo struct {
		name      string
		startLine int
		endLine   int
		fileRel   string
		typeName  string
	}
	var vars []varInfo
	for _, fi := range pkg.files {
		for _, decl := range fi.astFile.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				var typeName string
				if vs.Type != nil {
					typeName = identName(vs.Type)
				}
				start, end := posLines(pkg, vs.Pos(), vs.End())
				for _, n := range vs.Names {
					if ast.IsExported(n.Name) {
						continue
					}
					vars = append(vars, varInfo{
						name:      n.Name,
						startLine: start,
						endLine:   end,
						fileRel:   fi.relPath,
						typeName:  typeName,
					})
				}
			}
		}
	}
	if len(vars) == 0 {
		return nil
	}
	usage := make(map[string]int)
	for _, fi := range pkg.files {
		ast.Inspect(fi.astFile, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			line := pkg.fset.Position(id.Pos()).Line
			for _, v := range vars {
				if id.Name != v.name {
					continue
				}
				if line < v.startLine || line > v.endLine {
					usage[v.name]++
				}
			}
			return true
		})
	}
	for _, v := range vars {
		if usage[v.name] > 0 {
			continue
		}
		if !looksLikeFlag(v.name, v.typeName) {
			continue
		}
		findings = append(findings, Finding{
			Tag:         TagDelete,
			What:        fmt.Sprintf("Unused flag variable %s", v.name),
			Replacement: "Remove the variable and its registration",
			Path:        v.fileRel,
			Line:        v.startLine,
			EndLine:     v.endLine,
			LineCount:   lineCount(v.startLine, v.endLine),
			ApprovedBy:  markerFor(markers, v.fileRel, v.startLine, v.endLine),
		})
	}
	return findings
}

func looksLikeFlag(name, typeName string) bool {
	if typeName != "string" && typeName != "bool" && typeName != "int" && typeName != "int64" {
		return false
	}
	lower := strings.ToLower(name)
	return strings.Contains(lower, "flag") || strings.Contains(lower, "option") ||
		strings.Contains(lower, "param") || strings.Contains(lower, "config") ||
		strings.Contains(lower, "setting")
}
