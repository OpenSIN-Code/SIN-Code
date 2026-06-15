// SPDX-License-Identifier: MIT
// Purpose: rank loaded assets against a task context so the orchestrator
// can pick the right subagent(s) / command(s) without hardcoding names.
// Docs: selector.doc.md
package assets

import (
	"sort"
	"strings"
)

// Selector ranks loaded assets against a task context.
type Selector struct {
	reg *Registry
}

func NewSelector(reg *Registry) *Selector { return &Selector{reg: reg} }

// Match is a scored asset.
type Match struct {
	Asset *Asset
	Score int
}

// Context describes what the orchestrator is about to do.
type Context struct {
	Domain   string   // e.g. "go", "security", "testing"
	Keywords []string // e.g. ["review", "race", "concurrency"]
}

// SelectAgents returns the best-matching agents, highest score first.
func (s *Selector) SelectAgents(ctx Context, limit int) []Match {
	return s.selectKind(KindAgent, ctx, limit)
}

// SelectCommands returns the best-matching commands.
func (s *Selector) SelectCommands(ctx Context, limit int) []Match {
	return s.selectKind(KindCommand, ctx, limit)
}

func (s *Selector) selectKind(kind Kind, ctx Context, limit int) []Match {
	var matches []Match
	for _, a := range s.reg.List(kind) {
		if sc := score(a, ctx); sc > 0 {
			matches = append(matches, Match{Asset: a, Score: sc})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Asset.Name < matches[j].Asset.Name
	})
	if limit > 0 && len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

// score is a transparent heuristic: domain match dominates, then
// name/keyword overlap, then description overlap.
func score(a *Asset, ctx Context) int {
	s := 0
	dom := strings.ToLower(ctx.Domain)
	if dom != "" {
		if strings.ToLower(a.Domain) == dom {
			s += 10
		}
		if strings.Contains(strings.ToLower(a.Name), dom) {
			s += 6
		}
	}
	hayName := strings.ToLower(a.Name)
	hayDesc := strings.ToLower(a.Description)
	for _, kw := range ctx.Keywords {
		k := strings.ToLower(strings.TrimSpace(kw))
		if k == "" {
			continue
		}
		switch {
		case strings.Contains(hayName, k):
			s += 4
		case strings.Contains(hayDesc, k):
			s += 2
		}
	}
	return s
}
