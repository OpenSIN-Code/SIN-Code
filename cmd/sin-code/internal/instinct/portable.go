// SPDX-License-Identifier: MIT
// Purpose: lossless, parseable exchange format. One JSON object per line.
// Replaces the fragile Markdown string-splitting used by the old
// export/import.
// Docs: portable.doc.md
package instinct

import (
	"bufio"
	"encoding/json"
	"io"
)

// portable is the wire form of an instinct.
type portable struct {
	ID             string   `json:"id"`
	Trigger        string   `json:"trigger"`
	Confidence     float64  `json:"confidence"`
	Domain         string   `json:"domain"`
	Source         string   `json:"source"`
	Scope          Scope    `json:"scope"`
	ProjectID      string   `json:"project_id,omitempty"`
	ProjectName    string   `json:"project_name,omitempty"`
	SourceRepo     string   `json:"source_repo,omitempty"`
	Status         Status   `json:"status,omitempty"`
	Observations   int      `json:"observations,omitempty"`
	Contradictions int      `json:"contradictions,omitempty"`
	SeenInProjects []string `json:"seen_in_projects,omitempty"`
	Action         string   `json:"action"`
	Evidence       []string `json:"evidence,omitempty"`
}

func toPortable(i *Instinct) portable {
	return portable{
		ID: i.ID, Trigger: i.Trigger, Confidence: i.Confidence, Domain: i.Domain,
		Source: i.Source, Scope: i.Scope, ProjectID: i.ProjectID, ProjectName: i.ProjectName,
		SourceRepo: i.SourceRepo, Status: i.Status, Observations: i.Observations,
		Contradictions: i.Contradictions, SeenInProjects: i.SeenInProjects,
		Action: i.Action, Evidence: i.Evidence,
	}
}

func (p portable) toInstinct() *Instinct {
	i := &Instinct{
		ID: p.ID, Trigger: p.Trigger, Confidence: p.Confidence, Domain: p.Domain,
		Source: p.Source, Scope: p.Scope, ProjectID: p.ProjectID, ProjectName: p.ProjectName,
		SourceRepo: p.SourceRepo, Status: p.Status, Observations: p.Observations,
		Contradictions: p.Contradictions, SeenInProjects: p.SeenInProjects,
		Action: p.Action, Evidence: p.Evidence,
	}
	if i.ID == "" {
		i.ID = i.computeID()
	}
	if i.Scope == "" {
		i.Scope = ScopeGlobal
	}
	if i.Status == "" {
		i.recomputeStatus()
	}
	return i
}

// ExportJSONL writes instincts as newline-delimited JSON.
func ExportJSONL(w io.Writer, list []*Instinct) error {
	enc := json.NewEncoder(w)
	for _, i := range list {
		if err := enc.Encode(toPortable(i)); err != nil {
			return err
		}
	}
	return nil
}

// ImportJSONL reads newline-delimited JSON instincts. Blank lines are
// skipped; malformed lines are returned in errs but do not abort the
// import.
func ImportJSONL(r io.Reader) (out []*Instinct, errs []error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var p portable
		if err := json.Unmarshal(line, &p); err != nil {
			errs = append(errs, err)
			continue
		}
		out = append(out, p.toInstinct())
	}
	if err := sc.Err(); err != nil {
		errs = append(errs, err)
	}
	return out, errs
}
