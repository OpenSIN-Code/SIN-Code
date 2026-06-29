// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when complexity analyzer is refactored
package complexity

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func parsePackage(root, dir string, files []string) (*packageInfo, error) {
	pkg := &packageInfo{
		root:        root,
		dir:         dir,
		fset:        token.NewFileSet(),
		interfaces:  make(map[string]*ast.InterfaceType),
		typeSpecs:   make(map[string]*ast.TypeSpec),
		topFuncs:    make(map[string]*ast.FuncDecl),
		recvMethods: make(map[string]map[string]struct{}),
	}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		f, err := parser.ParseFile(pkg.fset, path, src, parser.ParseComments|parser.AllErrors)
		if err != nil && f == nil {
			continue
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "" {
			rel = path
		}
		tf := pkg.fset.File(f.Pos())
		fi := fileInfo{absPath: path, relPath: rel, astFile: f, lines: tf.LineCount()}
		pkg.files = append(pkg.files, fi)

		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ImportSpec:
						imp := importInfo{fileRel: rel, line: pkg.fset.Position(s.Pos()).Line}
						if s.Path != nil {
							imp.path = strings.Trim(s.Path.Value, `"`)
						}
						if s.Name != nil {
							imp.alias = s.Name.Name
						}
						pkg.imports = append(pkg.imports, imp)
					case *ast.TypeSpec:
						pkg.typeSpecs[s.Name.Name] = s
						if iface, ok := s.Type.(*ast.InterfaceType); ok {
							pkg.interfaces[s.Name.Name] = iface
						}
					}
				}
			case *ast.FuncDecl:
				if d.Recv == nil {
					pkg.topFuncs[d.Name.Name] = d
					continue
				}
				recvName := receiverTypeName(d.Recv)
				if recvName == "" {
					continue
				}
				if pkg.recvMethods[recvName] == nil {
					pkg.recvMethods[recvName] = make(map[string]struct{})
				}
				pkg.recvMethods[recvName][d.Name.Name] = struct{}{}
			}
		}
	}
	if len(pkg.files) == 0 {
		return nil, fmt.Errorf("no parseable files in %s", dir)
	}
	return pkg, nil
}

func receiverTypeName(recv *ast.FieldList) string {
	if recv == nil || len(recv.List) == 0 {
		return ""
	}
	expr := recv.List[0].Type
	if ptr, ok := expr.(*ast.StarExpr); ok {
		expr = ptr.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
