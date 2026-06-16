// SPDX-License-Identifier: MIT
// Purpose: project → global promotion. An instinct signature seen in
// >= PromotionThreshold (default 2) distinct projects becomes a
// candidate for global scope. Mirrors continuous-learning-v2 promotion
// step in a clean-room port.
// Docs: promote.doc.md
package instinct

import "time"

// PromotionThreshold: an instinct signature seen in this many distinct
// projects becomes a candidate for global scope.
const PromotionThreshold = 2

// PromotionCandidate bundles the per-project copies of one signature.
type PromotionCandidate struct {
	Signature string
	Projects  []string
	Best      *Instinct
}

// FindPromotable scans all project instincts and returns signatures
// that appear across >= PromotionThreshold projects and are not
// already global.
func FindPromotable(store *Store) ([]PromotionCandidate, error) {
	projects, err := store.ListProjects()
	if err != nil {
		return nil, err
	}
	globalSig := map[string]bool{}
	if globals, err := store.LoadGlobal(); err == nil {
		for _, g := range globals {
			globalSig[g.SignatureKey()] = true
		}
	}

	bySig := map[string]*PromotionCandidate{}
	for _, p := range projects {
		instincts, err := store.LoadProject(p.ID)
		if err != nil {
			return nil, err
		}
		for _, i := range instincts {
			sig := i.SignatureKey()
			if globalSig[sig] {
				continue
			}
			c := bySig[sig]
			if c == nil {
				c = &PromotionCandidate{Signature: sig, Best: i}
				bySig[sig] = c
			}
			c.Projects = appendUnique(c.Projects, p.ID)
			if i.Confidence > c.Best.Confidence {
				c.Best = i
			}
		}
	}

	var out []PromotionCandidate
	for _, c := range bySig {
		if len(c.Projects) >= PromotionThreshold {
			out = append(out, *c)
		}
	}
	return out, nil
}

// Promote writes a global copy of the candidate's strongest instinct.
func Promote(store *Store, c PromotionCandidate) (*Instinct, error) {
	g := *c.Best // copy
	g.Scope = ScopeGlobal
	g.ProjectID = ""
	g.ProjectName = ""
	g.Source = "promotion"
	g.SeenInProjects = c.Projects
	g.UpdatedAt = time.Now().UTC()
	g.ID = g.computeID()
	if err := store.Save(&g); err != nil {
		return nil, err
	}
	store.Append(AuditEvent{InstinctID: g.ID, Kind: "promoted", Confidence: g.Confidence, Detail: c.Signature})
	return &g, nil
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
