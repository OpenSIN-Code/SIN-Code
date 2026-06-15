// SPDX-License-Identifier: MIT
// Purpose: cluster eligible instincts into Skill/Command/Agent proposals.
// This is the "evolution" stage — turning a few related learned
// behaviors into a real reusable artifact. Mirrors the
// continuous-learning-v2 evolution step in a clean-room port.
// Docs: evolve.doc.md
package instinct

import (
	"sort"
	"strings"
	"time"
)

// EvolutionKind is the artifact an instinct cluster graduates into.
type EvolutionKind string

const (
	EvolveSkill   EvolutionKind = "skill"
	EvolveCommand EvolutionKind = "command"
	EvolveAgent   EvolutionKind = "agent"
)

// Proposal is a suggested graduation of clustered instincts.
type Proposal struct {
	Kind          EvolutionKind
	Domain        string
	Name          string
	Members       []*Instinct
	AvgConfidence float64
	Rationale     string
}

// Evolve clusters eligible instincts by domain and proposes artifacts.
//
//	cluster size 1            -> command (a single sharp behavior)
//	cluster size 2-3          -> skill   (a small coherent capability)
//	cluster size 4+           -> agent   (a domain specialist)
func Evolve(instincts []*Instinct) []Proposal {
	byDomain := map[string][]*Instinct{}
	for _, i := range instincts {
		if !i.EligibleForEvolution() {
			continue
		}
		byDomain[i.Domain] = append(byDomain[i.Domain], i)
	}

	var proposals []Proposal
	for domain, members := range byDomain {
		SortByConfidence(members)
		sum := 0.0
		for _, m := range members {
			sum += m.Confidence
		}
		avg := round2(sum / float64(len(members)))

		kind := EvolveCommand
		switch {
		case len(members) >= 4:
			kind = EvolveAgent
		case len(members) >= 2:
			kind = EvolveSkill
		}
		proposals = append(proposals, Proposal{
			Kind:          kind,
			Domain:        domain,
			Name:          domain + "-" + string(kind),
			Members:       members,
			AvgConfidence: avg,
			Rationale:     "Cluster of " + itoa(len(members)) + " high-confidence instincts in domain '" + domain + "'.",
		})
	}
	sort.SliceStable(proposals, func(a, b int) bool {
		return proposals[a].AvgConfidence > proposals[b].AvgConfidence
	})
	return proposals
}

// RenderArtifact produces the Markdown body for a graduated artifact.
func (p Proposal) RenderArtifact() string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + p.Name + "\n")
	b.WriteString("kind: " + string(p.Kind) + "\n")
	b.WriteString("domain: " + p.Domain + "\n")
	b.WriteString("origin: SIN-Code/instinct-evolution\n")
	b.WriteString("confidence: " + ftoa(p.AvgConfidence) + "\n")
	b.WriteString("---\n\n")
	b.WriteString("# " + strings.Title(p.Domain) + " " + strings.Title(string(p.Kind)) + "\n\n")
	b.WriteString(p.Rationale + "\n\n")
	b.WriteString("## Learned behaviors\n\n")
	for _, m := range p.Members {
		b.WriteString("### " + titleFromTrigger(m.Trigger) + "\n")
		b.WriteString("- Trigger: " + m.Trigger + "\n")
		b.WriteString("- Action: " + m.Action + "\n")
		b.WriteString("- Confidence: " + ftoa(m.Confidence) + "\n\n")
	}
	return b.String()
}

// MarkEvolved sets every member of a proposal to StatusEvolved and
// persists it, so future Evolve() calls won't re-propose the same
// cluster.
func MarkEvolved(store *Store, p Proposal) error {
	for _, m := range p.Members {
		m.Status = StatusEvolved
		m.UpdatedAt = time.Now().UTC()
		if err := store.Save(m); err != nil {
			return err
		}
		store.Append(AuditEvent{InstinctID: m.ID, Kind: "evolved", Confidence: m.Confidence, Detail: p.Name})
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func ftoa(f float64) string {
	whole := int(f)
	frac := int(round2(f-float64(whole)) * 100)
	if frac < 0 {
		frac = -frac
	}
	fs := itoa(frac)
	if len(fs) < 2 {
		fs = "0" + fs
	}
	return itoa(whole) + "." + fs
}
