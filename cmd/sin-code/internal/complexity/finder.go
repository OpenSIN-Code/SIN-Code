// SPDX-License-Identifier: MIT
// Purpose: static complexity finder — ponytail 5-tag review (delete/stdlib/native/yagni/shrink).
package complexity

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"sync"
)

// Options controls the scope of the static review.
type Options struct {
	Root      string
	SinceRef  string
	Tags      []string
	MarkerMap map[string][]Marker
}

// Find runs the static complexity analysis over the selected files.
func Find(opts Options) ([]Finding, error) {
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, fmt.Errorf("root path: %w", err)
	}
	if opts.MarkerMap == nil {
		opts.MarkerMap = make(map[string][]Marker)
	}
	if len(opts.Tags) == 0 {
		opts.Tags = AllTags
	}
	tagSet := make(map[string]bool, len(opts.Tags))
	for _, t := range opts.Tags {
		tagSet[t] = true
	}

	files, err := selectFiles(root, opts.SinceRef)
	if err != nil {
		return nil, err
	}

	byDir := groupByDir(files)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		findings []Finding
		errCh    = make(chan error, len(byDir))
	)
	for dir, fs := range byDir {
		wg.Add(1)
		go func(dir string, fs []string) {
			defer wg.Done()
			pkg, err := parsePackage(root, dir, fs)
			if err != nil {
				errCh <- err
				return
			}
			local := analyzePackage(pkg, tagSet, opts.MarkerMap)
			mu.Lock()
			findings = append(findings, local...)
			mu.Unlock()
		}(dir, fs)
	}
	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			return nil, e
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

// packageInfo holds parsed state for one directory of Go source.
type packageInfo struct {
	root        string
	dir         string
	files       []fileInfo
	fset        *token.FileSet
	interfaces  map[string]*ast.InterfaceType
	typeSpecs   map[string]*ast.TypeSpec
	topFuncs    map[string]*ast.FuncDecl
	recvMethods map[string]map[string]struct{}
	imports     []importInfo
}

type fileInfo struct {
	absPath string
	relPath string
	astFile *ast.File
	lines   int
}

type importInfo struct {
	path    string
	alias   string
	line    int
	fileRel string
}

func analyzePackage(pkg *packageInfo, tags map[string]bool, markers map[string][]Marker) []Finding {
	var findings []Finding
	if tags[TagYagni] {
		findings = append(findings, analyzeSingleImplInterfaces(pkg, markers)...)
		findings = append(findings, analyzeOneProductFactories(pkg, markers)...)
	}
	if tags[TagShrink] {
		findings = append(findings, analyzeWrapperFunctions(pkg, markers)...)
		findings = append(findings, analyzeRepeatAppendLoops(pkg, markers)...)
	}
	if tags[TagDelete] {
		findings = append(findings, analyzeDeadConfigFlags(pkg, markers)...)
	}
	if tags[TagStdlib] {
		findings = append(findings, analyzeManualMinMax(pkg, markers)...)
	}
	if tags[TagNative] {
		findings = append(findings, analyzeNativeImports(pkg, markers)...)
	}
	if tags[TagYagni] {
		findings = append(findings, analyzeOneExportFiles(pkg, markers, findings)...)
	}
	return findings
}
