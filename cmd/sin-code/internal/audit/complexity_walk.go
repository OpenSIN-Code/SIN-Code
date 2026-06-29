// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when audit is refactored
// Purpose: file walking and package-level analysis pass for complexity audit.
package audit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// goFiles returns all .go files under root, excluding _test.go and vendor.
func goFiles(root string) ([]string, error) {
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == "node_modules" || name == ".git" || name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// packagePass runs detectors across all files in a package directory,
// enabling cross-file interface implementation counting.
func packagePass(files []string, allowed map[string]bool, debtRE *regexp.Regexp) []Finding {
	var allFindings []Finding
	var parsed []parsedFile
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		src := string(data)
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, data, parser.ParseComments)
		if err != nil {
			continue
		}
		parsed = append(parsed, parsedFile{pkg: packageInfo{f: file, src: src, fset: fset, path: f}})
	}
	if len(parsed) == 0 {
		return nil
	}
	pkgIfaces := make(map[string][]string)
	pkgRecvMethods := make(map[string]map[string]struct{})
	for _, p := range parsed {
		for _, decl := range p.pkg.f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
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
				if len(methods) > 0 {
					pkgIfaces[ts.Name.Name] = methods
				}
			}
		}
	}
	for _, p := range parsed {
		for _, decl := range p.pkg.f.Decls {
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
			if pkgRecvMethods[recvName] == nil {
				pkgRecvMethods[recvName] = make(map[string]struct{})
			}
			pkgRecvMethods[recvName][fn.Name.Name] = struct{}{}
		}
	}
	implCounts := make(map[string]int)
	for ifaceName, required := range pkgIfaces {
		count := 0
		for _, methods := range pkgRecvMethods {
			allPresent := true
			for _, req := range required {
				if _, ok := methods[req]; !ok {
					allPresent = false
					break
				}
			}
			if allPresent {
				count++
			}
		}
		implCounts[ifaceName] = count
	}
	for _, p := range parsed {
		pkgFindings := staticPassWithPkg(p.pkg, allowed, debtRE, pkgIfaces, implCounts, parsed)
		allFindings = append(allFindings, pkgFindings...)
	}
	return allFindings
}

// staticPassWithPkg runs file-level detectors plus cross-file interface detection.
func staticPassWithPkg(pkg packageInfo, allowed map[string]bool, debtRE *regexp.Regexp, pkgIfaces map[string][]string, implCounts map[string]int, allPkgFiles []parsedFile) []Finding {
	var findings []Finding
	findings = append(findings, detectSingleImplInterfacesWithCounts(pkg, allowed, pkgIfaces, implCounts)...)
	findings = append(findings, detectSingleProductFactoriesWithCounts(pkg, allowed, pkgIfaces, implCounts)...)
	findings = append(findings, detectWrappers(pkg, allowed)...)
	findings = append(findings, detectOneExportFiles(pkg, allowed)...)
	findings = append(findings, detectDeadFlagsPkg(pkg, allowed, allPkgFiles)...)
	findings = append(findings, detectHandRolledStdlib(pkg, allowed)...)
	return findings
}

// staticPass runs deterministic detectors on a single file (legacy, used by tests).
func staticPass(pkg packageInfo, allowed map[string]bool, debtRE *regexp.Regexp) []Finding {
	var findings []Finding
	findings = append(findings, detectSingleImplInterfaces(pkg, allowed)...)
	findings = append(findings, detectSingleProductFactories(pkg, allowed)...)
	findings = append(findings, detectWrappers(pkg, allowed)...)
	findings = append(findings, detectOneExportFiles(pkg, allowed)...)
	findings = append(findings, detectDeadFlags(pkg, allowed)...)
	findings = append(findings, detectHandRolledStdlib(pkg, allowed)...)
	return findings
}

func lineRange(fset *token.FileSet, n ast.Node) (int, int) {
	return fset.Position(n.Pos()).Line, fset.Position(n.End()).Line
}
