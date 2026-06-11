// SPDX-License-Identifier: MIT
// Purpose: Repo Cartographer — PageRank-weighted symbol map with
// incremental updates. Ranks symbols by graph centrality; emits scored
// ContextItems for the compiler (relevance scores, not full-text dumps).
// Incremental invalidation: only files in the change set are re-parsed.
package orchestrator

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Symbol struct {
	Key       string
	File      string
	Line      int
	Kind      string
	Signature string
	Rank      float64
}

type Cartographer struct {
	mu          sync.RWMutex
	repoRoot    string
	symbols     map[string]*Symbol
	edges       map[string][]string
	fileSymbols map[string][]string
	ranked      bool
}

func NewCartographer(repoRoot string) *Cartographer {
	return &Cartographer{
		repoRoot:    repoRoot,
		symbols:     map[string]*Symbol{},
		edges:       map[string][]string{},
		fileSymbols: map[string][]string{},
	}
}

func (c *Cartographer) IndexAll(ctx context.Context) error {
	if c.repoRoot == "" {
		return nil
	}
	var goFiles []string
	err := filepath.WalkDir(c.repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() && (name == "node_modules" || name == ".git" || name == "vendor") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("cartographer walk: %w", err)
	}
	for _, f := range goFiles {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := c.indexFile(f); err != nil {
			continue
		}
	}
	c.computeRank()
	return nil
}

func (c *Cartographer) Invalidate(files []string) {
	if c.repoRoot == "" {
		return
	}
	c.mu.Lock()
	for _, rel := range files {
		for _, key := range c.fileSymbols[rel] {
			delete(c.symbols, key)
			delete(c.edges, key)
		}
		delete(c.fileSymbols, rel)
	}
	c.mu.Unlock()

	for _, rel := range files {
		if strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, "_test.go") {
			_ = c.indexFile(filepath.Join(c.repoRoot, rel))
		}
	}
	c.computeRank()
}

func (c *Cartographer) SliceFor(imp *Impact, k int) []ContextItem {
	c.mu.RLock()
	defer c.mu.RUnlock()

	affected := map[string]bool{}
	if imp != nil {
		for _, p := range imp.AffectedPkgs {
			affected[p] = true
		}
	}

	type scored struct {
		sym   *Symbol
		score float64
	}
	var all []scored
	for _, s := range c.symbols {
		score := s.Rank
		pkg := filepath.Dir(s.File)
		if affected[pkg] || affectedByPath(affected, s.File) {
			score *= 3.0
		}
		all = append(all, scored{s, score})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score > all[j].score })

	if k > len(all) {
		k = len(all)
	}
	items := make([]ContextItem, 0, k)
	for _, sc := range all[:k] {
		items = append(items, ContextItem{
			Kind:      "file",
			Name:      fmt.Sprintf("%s:%d %s", sc.sym.File, sc.sym.Line, sc.sym.Key),
			Body:      sc.sym.Signature,
			Relevance: normalize(sc.score),
		})
	}
	return items
}

func (c *Cartographer) SymbolCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.symbols)
}

func (c *Cartographer) indexFile(absPath string) error {
	if c.repoRoot == "" {
		return fmt.Errorf("no repo root")
	}
	rel, err := filepath.Rel(c.repoRoot, absPath)
	if err != nil {
		rel = absPath
	}
	src, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, src, 0)
	if err != nil {
		return err
	}
	pkg := f.Name.Name

	c.mu.Lock()
	defer c.mu.Unlock()

	var keys []string
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		key := pkg + "." + fd.Name.Name
		kind := "func"
		if fd.Recv != nil && len(fd.Recv.List) > 0 {
			key = pkg + "." + recvTypeName(fd.Recv.List[0].Type) + "." + fd.Name.Name
			kind = "method"
		}
		pos := fset.Position(fd.Pos())
		sig := signatureLine(string(src), pos.Line)

		c.symbols[key] = &Symbol{
			Key: key, File: rel, Line: pos.Line, Kind: kind, Signature: sig,
		}
		keys = append(keys, key)

		ast.Inspect(fd, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if target := callName(call, pkg); target != "" {
					c.edges[key] = append(c.edges[key], target)
				}
			}
			return true
		})
	}
	c.fileSymbols[rel] = keys
	c.ranked = false
	return nil
}

func (c *Cartographer) computeRank() {
	c.mu.Lock()
	defer c.mu.Unlock()

	n := len(c.symbols)
	if n == 0 {
		return
	}
	const d = 0.85
	base := (1 - d) / float64(n)

	rank := map[string]float64{}
	for k := range c.symbols {
		rank[k] = 1.0 / float64(n)
	}

	incoming := map[string][]string{}
	outDegree := map[string]int{}
	for from, tos := range c.edges {
		for _, to := range tos {
			if _, exists := c.symbols[to]; exists {
				incoming[to] = append(incoming[to], from)
				outDegree[from]++
			}
		}
	}

	for iter := 0; iter < 20; iter++ {
		next := map[string]float64{}
		for k := range c.symbols {
			sum := 0.0
			for _, in := range incoming[k] {
				if od := outDegree[in]; od > 0 {
					sum += rank[in] / float64(od)
				}
			}
			next[k] = base + d*sum
		}
		rank = next
	}

	for k, s := range c.symbols {
		s.Rank = rank[k]
	}
	c.ranked = true
}

func callName(call *ast.CallExpr, pkg string) string {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return pkg + "." + fn.Name
	case *ast.SelectorExpr:
		if x, ok := fn.X.(*ast.Ident); ok {
			return x.Name + "." + fn.Sel.Name
		}
	}
	return ""
}

func signatureLine(src string, line int) string {
	lines := strings.Split(src, "\n")
	if line-1 < len(lines) {
		return strings.TrimSpace(lines[line-1])
	}
	return ""
}

func affectedByPath(affected map[string]bool, file string) bool {
	for p := range affected {
		if strings.Contains(file, lastSegment(p)) {
			return true
		}
	}
	return false
}

func lastSegment(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func normalize(x float64) float64 {
	if x > 1 {
		return 1
	}
	if x < 0 {
		return 0
	}
	return x
}
