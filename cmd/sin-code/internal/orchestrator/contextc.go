// SPDX-License-Identifier: MIT
// Purpose: Context Compiler — budget-aware knapsack over context items.
// Pinned items (contracts, active diagnoses) are constitutional and
// always included; the rest is greedy by relevance-per-token. Eviction
// is audited so a bad agent decision can be traced to missing/noisy
// context.
package orchestrator

import (
	"fmt"
	"sort"
	"strings"
)

type ContextItem struct {
	Kind      string
	Name      string
	Body      string
	Relevance float64
	Pinned    bool
}

func (c *ContextItem) tokens() int { return len(c.Body)/4 + 8 }

type CompiledContext struct {
	Prompt   string
	Included []string
	Evicted  []string
	Used     int
	Budget   int
}

type ContextCompiler struct {
	Budget   int
	KindCaps map[string]int
}

func NewContextCompiler(budget int) *ContextCompiler {
	if budget <= 0 {
		budget = 12000
	}
	return &ContextCompiler{
		Budget: budget,
		KindCaps: map[string]int{
			"file": 6, "episode": 3, "suggestion": 2, "impact": 1,
		},
	}
}

func (cc *ContextCompiler) Compile(items []ContextItem) (*CompiledContext, error) {
	out := &CompiledContext{Budget: cc.Budget}
	var b strings.Builder
	kindCount := map[string]int{}

	for _, it := range items {
		if !it.Pinned {
			continue
		}
		cost := it.tokens()
		if out.Used+cost > cc.Budget {
			return nil, fmt.Errorf("context compiler: pinned items exceed budget (%d > %d) — shrink %q",
				out.Used+cost, cc.Budget, it.Name)
		}
		writeItem(&b, it)
		out.Used += cost
		out.Included = append(out.Included, it.Kind+":"+it.Name)
	}

	rest := make([]ContextItem, 0, len(items))
	for _, it := range items {
		if !it.Pinned {
			rest = append(rest, it)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool {
		ri := rest[i].Relevance / float64(rest[i].tokens())
		rj := rest[j].Relevance / float64(rest[j].tokens())
		return ri > rj
	})

	for _, it := range rest {
		cost := it.tokens()
		cap, hasCap := cc.KindCaps[it.Kind]
		switch {
		case hasCap && kindCount[it.Kind] >= cap:
			out.Evicted = append(out.Evicted, it.Kind+":"+it.Name+" (kind cap)")
		case out.Used+cost > cc.Budget:
			out.Evicted = append(out.Evicted, it.Kind+":"+it.Name+" (budget)")
		default:
			writeItem(&b, it)
			out.Used += cost
			kindCount[it.Kind]++
			out.Included = append(out.Included, it.Kind+":"+it.Name)
		}
	}

	out.Prompt = b.String()
	return out, nil
}

func writeItem(b *strings.Builder, it ContextItem) {
	fmt.Fprintf(b, "<context kind=%q name=%q>\n%s\n</context>\n\n", it.Kind, it.Name, strings.TrimSpace(it.Body))
}

func GatherStandard(
	contract *Contract,
	diagnosis string,
	impact *Impact,
	episodes []*Episode,
	suggestions string,
	fileSlices map[string]string,
	fileScores map[string]float64,
) []ContextItem {
	var items []ContextItem

	if contract != nil {
		items = append(items, ContextItem{
			Kind: "contract", Name: contract.TaskID, Pinned: true,
			Body: contractBrief(contract),
		})
	}
	if diagnosis != "" {
		items = append(items, ContextItem{
			Kind: "diagnosis", Name: "active-failure", Pinned: true, Body: diagnosis,
		})
	}
	if impact != nil {
		items = append(items, ContextItem{
			Kind: "impact", Name: "blast-radius", Relevance: 0.9, Body: impact.RiskBrief(),
		})
	}
	if suggestions != "" {
		items = append(items, ContextItem{
			Kind: "suggestion", Name: "mined-chains", Relevance: 0.8, Body: suggestions,
		})
	}
	if prior := PlanningPrior(episodes); prior != "" {
		items = append(items, ContextItem{
			Kind: "episode", Name: "planning-prior", Relevance: 0.7, Body: prior,
		})
	}
	for path, slice := range fileSlices {
		items = append(items, ContextItem{
			Kind: "file", Name: path, Relevance: fileScores[path], Body: slice,
		})
	}
	return items
}

func contractBrief(c *Contract) string {
	var b strings.Builder
	b.WriteString("## Binding contract for this task\n")
	if len(c.AllowedGlobs) > 0 {
		fmt.Fprintf(&b, "- You may ONLY modify: %v\n", c.AllowedGlobs)
	}
	if len(c.FrozenGlobs) > 0 {
		fmt.Fprintf(&b, "- FROZEN (never touch): %v\n", c.FrozenGlobs)
	}
	if c.MaxFilesChanged > 0 {
		fmt.Fprintf(&b, "- Max files changed: %d, max lines: %d\n", c.MaxFilesChanged, c.MaxLinesChanged)
	}
	b.WriteString("- Violations are mechanically rejected before any write.\n")
	return b.String()
}
