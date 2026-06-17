// SPDX-License-Identifier: MIT
// Purpose: AutoDream v2 — sleep-time reflection (issue #353). Extends
// the existing AutoDream with a Reflect method that identifies
// patterns, forms hypotheses, finds connections between seemingly
// unrelated memories, and stores the reflection as new memories
// tagged "reflection". Deterministic (dependency-free) by default;
// LLM-assisted when an llm.Client is configured. Thread-safe (M7).
package memory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Connection is a discovered link between two memories that share
// a tag but were not previously linked.
type Connection struct {
	FromID    string `json:"from_id"`
	ToID      string `json:"to_id"`
	SharedTag string `json:"shared_tag"`
	Reason    string `json:"reason"`
}

// Contradiction is a pair of memories whose content suggests
// opposing conclusions.
type Contradiction struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
	Reason string `json:"reason"`
}

// ReflectionReport holds the output of a reflection pass.
type ReflectionReport struct {
	Insights            []string       `json:"insights"`
	Questions           []string       `json:"questions"`
	Connections         []Connection   `json:"connections"`
	ContradictionsFound []Contradiction `json:"contradictions_found"`
	Duration            time.Duration  `json:"duration"`
}

// AutoDreamV2 extends AutoDream with sleep-time reflection.
type AutoDreamV2 struct {
	*AutoDream
}

// NewAutoDreamV2 creates an enhanced AutoDream with reflection
// capabilities. Accepts the same options as NewAutoDream.
func NewAutoDreamV2(store *Store, opts ...AutoDreamOption) *AutoDreamV2 {
	return &AutoDreamV2{
		AutoDream: NewAutoDream(store, opts...),
	}
}

// Reflect performs a reflection pass on recent memories. It finds
// connections (shared-tag memories without links), contradictions,
// generates insights from tag co-occurrence, and poses questions
// about knowledge gaps. Reflections are stored as new memories
// tagged "reflection". Deterministic without an LLM client.
func (ad *AutoDreamV2) Reflect(ctx context.Context) (*ReflectionReport, error) {
	if ad == nil || ad.store == nil {
		return nil, fmt.Errorf("autodream-v2: nil store")
	}
	start := time.Now()
	report := &ReflectionReport{}

	all, err := ad.store.List(ListFilter{Limit: ad.maxMemories})
	if err != nil {
		return nil, fmt.Errorf("autodream-v2: list memories: %w", err)
	}
	if len(all) == 0 {
		report.Duration = time.Since(start)
		return report, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	report.Connections = ad.findConnections(ctx, all)
	report.ContradictionsFound = ad.findContradictions(ctx, all)
	report.Insights = ad.generateInsights(all)
	report.Questions = ad.generateQuestions(all, report.Connections)

	ad.storeReflections(report)

	report.Duration = time.Since(start)
	return report, nil
}

// findConnections discovers pairs of memories that share at least one
// tag but are not already linked in the knowledge graph.
func (ad *AutoDreamV2) findConnections(ctx context.Context, all []*Memory) []Connection {
	var conns []Connection
	linked := map[string]bool{}
	for i := 0; i < len(all); i++ {
		if err := ctx.Err(); err != nil {
			return conns
		}
		links, err := ad.store.GetLinks(all[i].ID)
		if err != nil {
			continue
		}
		for _, l := range links {
			linked[all[i].ID+"\x00"+l.To] = true
		}
	}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			shared := sharedTags(all[i].Tags, all[j].Tags)
			if len(shared) == 0 {
				continue
			}
			pairKey := all[i].ID + "\x00" + all[j].ID
			if linked[pairKey] {
				continue
			}
			conns = append(conns, Connection{
				FromID:    all[i].ID,
				ToID:      all[j].ID,
				SharedTag: shared[0],
				Reason: fmt.Sprintf("share tag '%s' but are not linked", shared[0]),
			})
		}
	}
	return conns
}

// findContradictions detects pairs of memories with opposing content.
func (ad *AutoDreamV2) findContradictions(ctx context.Context, all []*Memory) []Contradiction {
	var found []Contradiction
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if err := ctx.Err(); err != nil {
				return found
			}
			if !sameTags(all[i].Tags, all[j].Tags) {
				continue
			}
			if isContradiction(all[i].Insight, all[j].Insight) {
				found = append(found, Contradiction{
					FromID: all[i].ID,
					ToID:   all[j].ID,
					Reason: fmt.Sprintf("negation asymmetry detected between '%s' and '%s'",
						truncate(all[i].Insight, 40), truncate(all[j].Insight, 40)),
				})
			}
		}
	}
	return found
}

// generateInsights produces deterministic insights from tag
// co-occurrence and frequency patterns.
func (ad *AutoDreamV2) generateInsights(all []*Memory) []string {
	tagCount := map[string]int{}
	coOccur := map[string]map[string]int{}
	for _, m := range all {
		for _, t := range m.Tags {
			if t == "reflection" || t == "autodream-summary" {
				continue
			}
			tagCount[t]++
		}
		for i := 0; i < len(m.Tags); i++ {
			for j := i + 1; j < len(m.Tags); j++ {
				a, b := m.Tags[i], m.Tags[j]
				if a == "reflection" || b == "reflection" || a == "autodream-summary" || b == "autodream-summary" {
					continue
				}
				if coOccur[a] == nil {
					coOccur[a] = map[string]int{}
				}
				coOccur[a][b]++
			}
		}
	}
	var insights []string
	for tag, count := range tagCount {
		if count >= 3 {
			insights = append(insights, fmt.Sprintf("Tag '%s' appears in %d memories — a recurring theme", tag, count))
		}
	}
	for a, partners := range coOccur {
		for b, count := range partners {
			if count >= 2 {
				insights = append(insights, fmt.Sprintf("Tags '%s' and '%s' co-occur in %d memories — possible correlation", a, b, count))
			}
		}
	}
	return insights
}

// generateQuestions poses questions about knowledge gaps and
// unlinked but related memories.
func (ad *AutoDreamV2) generateQuestions(all []*Memory, conns []Connection) []string {
	var questions []string
	if len(conns) > 0 && len(conns) <= 5 {
		for _, c := range conns {
			questions = append(questions, fmt.Sprintf("Why do memories %s and %s share tag '%s' but have no link?", c.FromID, c.ToID, c.SharedTag))
		}
	}
	tagCount := map[string]int{}
	for _, m := range all {
		for _, t := range m.Tags {
			if t != "reflection" && t != "autodream-summary" {
				tagCount[t]++
			}
		}
	}
	for tag, count := range tagCount {
		if count == 1 {
			questions = append(questions, fmt.Sprintf("Tag '%s' has only one memory — is this an underexplored area?", tag))
		}
	}
	return questions
}

// storeReflections saves the reflection report as a new memory
// tagged "reflection".
func (ad *AutoDreamV2) storeReflections(report *ReflectionReport) {
	var b strings.Builder
	b.WriteString("[reflection] ")
	if len(report.Insights) > 0 {
		b.WriteString("Insights: ")
		b.WriteString(strings.Join(report.Insights, "; "))
		b.WriteString(". ")
	}
	if len(report.Connections) > 0 {
		fmt.Fprintf(&b, "Connections found: %d. ", len(report.Connections))
	}
	if len(report.ContradictionsFound) > 0 {
		fmt.Fprintf(&b, "Contradictions found: %d. ", len(report.ContradictionsFound))
	}
	if len(report.Questions) > 0 {
		b.WriteString("Open questions: ")
		b.WriteString(strings.Join(report.Questions, "; "))
	}
	insight := strings.TrimSpace(b.String())
	if insight == "" || insight == "[reflection]" {
		return
	}
	_ = ad.store.Add(&Memory{
		Insight:    insight,
		Tags:       []string{"reflection"},
		Importance: 0.4,
	})
}

func sharedTags(a, b []string) []string {
	bset := map[string]bool{}
	for _, t := range b {
		bset[t] = true
	}
	var out []string
	for _, t := range a {
		if t == "reflection" || t == "autodream-summary" {
			continue
		}
		if bset[t] {
			out = append(out, t)
		}
	}
	return out
}
