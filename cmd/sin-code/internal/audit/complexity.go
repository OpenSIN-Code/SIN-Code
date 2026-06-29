// SPDX-License-Identifier: MIT
// Purpose: complexity audit — repo-wide ponytail-audit analog for SIN-Code.
// Scans Go trees for structural bloat (single-impl interfaces, single-product
// factories, wrappers, one-export files, dead flags, hand-rolled stdlib) and
// emits findings in the ponytail 5-tag format.
// Docs: docs/complexity-audit.md
package audit

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Five ponytail-style tags.
const (
	TagDelete = "delete"
	TagStdlib = "stdlib"
	TagNative = "native"
	TagYagni  = "yagni"
	TagShrink = "shrink"
)

var allTags = []string{TagDelete, TagStdlib, TagNative, TagYagni, TagShrink}

type parsedFile struct {
	pkg packageInfo
}

// LLM is the optional second-pass verifier. Deterministic tests supply a stub.
type LLM interface {
	Judge(ctx context.Context, filePath, content string, candidates []Finding) ([]Finding, error)
}

// Finding is one one-line complexity finding.
type Finding struct {
	Tag         string `json:"tag"`
	Problem     string `json:"problem"`
	Replacement string `json:"replacement"`
	Path        string `json:"path"`
	Line        int    `json:"line"`
	LineCount   int    `json:"line_count"` // estimated lines removable
	Approved    bool   `json:"approved,omitempty"`
	Approver    string `json:"approver,omitempty"`
}

// Result aggregates all findings and the final net delta.
type Result struct {
	Findings      []Finding `json:"findings"`
	NetLines      int       `json:"net_lines"`
	DepsRemovable int       `json:"deps_removable"`
	Status        string    `json:"status"`
}

// Options configure the audit run.
type Options struct {
	Tags      []string // allowed tags, empty = all
	Rank      string   // "lines" or "deps"
	TopN      int      // LLM judge only sees top N static findings
	SinceRef  string   // git ref; not implemented in static pass
	MaxNet    int      // fail threshold for strict mode
	Strict    bool
	NoLLM     bool // skip LLM pass
	SinDebtRE *regexp.Regexp
}

// DefaultSinDebtRE matches "sin-debt:" markers.
func DefaultSinDebtRE() *regexp.Regexp {
	return regexp.MustCompile(`(?i)//\s*sin-debt:\s*(.+)$`)
}

// Auditor performs a repo-wide complexity scan.
type Auditor struct {
	LLM LLM
}

// NewAuditor creates an auditor with an optional LLM second pass.
func NewAuditor(llm LLM) *Auditor {
	return &Auditor{LLM: llm}
}

// Audit scans root and returns a result. The static pass is deterministic and
// LLM-free; the optional LLM pass only reviews the top-N candidates.
func (a *Auditor) Audit(ctx context.Context, root string, opts Options) (*Result, error) {
	if opts.SinDebtRE == nil {
		opts.SinDebtRE = DefaultSinDebtRE()
	}
	if opts.Tags == nil {
		opts.Tags = append([]string(nil), allTags...)
	}
	allowed := make(map[string]bool, len(opts.Tags))
	for _, t := range opts.Tags {
		allowed[strings.ToLower(strings.TrimSpace(t))] = true
	}

	files, err := goFiles(root)
	if err != nil {
		return nil, err
	}

	var findings []Finding
	pkgFiles := make(map[string][]string)
	for _, f := range files {
		dir := filepath.Dir(f)
		pkgFiles[dir] = append(pkgFiles[dir], f)
	}
	for _, files := range pkgFiles {
		pkgFindings := packagePass(files, allowed, opts.SinDebtRE)
		findings = append(findings, pkgFindings...)
	}

	if !opts.NoLLM && a.LLM != nil && len(findings) > 0 {
		for i, f := range findings {
			if i >= opts.TopN && opts.TopN > 0 {
				break
			}
			// #nosec G304 — path is the same user-supplied audit root file.
			data, err := os.ReadFile(f.Path)
			if err != nil {
				continue
			}
			extra, err := a.LLM.Judge(ctx, f.Path, string(data), []Finding{f})
			if err != nil {
				continue
			}
			if len(extra) > 0 {
				findings = append(findings, extra...)
			}
		}
	}

	// Approve findings that sit on or directly after a sin-debt marker.
	for i := range findings {
		findings[i].Approved, findings[i].Approver = approvedBySinDebt(findings[i], opts.SinDebtRE)
	}

	// Filter by allowed tags and remove duplicates by (path, line, tag).
	filtered, seen := findings[:0], make(map[string]bool)
	for _, f := range findings {
		if !allowed[strings.ToLower(f.Tag)] {
			continue
		}
		key := fmt.Sprintf("%s:%d:%s:%s", f.Path, f.Line, f.Tag, f.Problem)
		if seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, f)
	}
	findings = filtered

	if opts.Rank == "deps" {
		// deps rank = stdlib/native tags first, then line count
		sort.Slice(findings, func(i, j int) bool {
			if tagRank(findings[i].Tag) != tagRank(findings[j].Tag) {
				return tagRank(findings[i].Tag) < tagRank(findings[j].Tag)
			}
			return findings[i].LineCount > findings[j].LineCount
		})
	} else {
		// default rank = lines saved, descending
		sort.Slice(findings, func(i, j int) bool {
			return findings[i].LineCount > findings[j].LineCount
		})
	}

	result := aggregate(findings, opts.MaxNet)
	if opts.Strict && result.NetLines > opts.MaxNet {
		return result, fmt.Errorf("complexity net-lines %d exceeds threshold %d", result.NetLines, opts.MaxNet)
	}
	return result, nil
}

func tagRank(tag string) int {
	switch strings.ToLower(tag) {
	case TagStdlib:
		return 0
	case TagNative:
		return 1
	case TagYagni:
		return 2
	case TagDelete:
		return 3
	case TagShrink:
		return 4
	}
	return 9
}

type packageInfo struct {
	f    *ast.File
	src  string
	fset *token.FileSet
	path string
}
