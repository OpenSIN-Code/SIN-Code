package todo

import (
	"strings"
	"time"
)

type CompactOptions struct {
	OlderThan    time.Duration
	OnlyStatuses []Status
	DryRun       bool
}

type CompactResult struct {
	Compacted int      `json:"compacted"`
	Skipped   int      `json:"skipped"`
	IDs       []string `json:"ids,omitempty"`
}

func (s *Store) Compact(opts CompactOptions) (*CompactResult, error) {
	if opts.OnlyStatuses == nil {
		opts.OnlyStatuses = []Status{StatusDone, StatusCancelled}
	}
	cutoff := time.Now().Add(-opts.OlderThan)
	if opts.OlderThan == 0 {
		cutoff = time.Time{}
	}
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	res := &CompactResult{}
	for _, t := range all {
		eligible := false
		for _, st := range opts.OnlyStatuses {
			if t.Status == st {
				eligible = true
				break
			}
		}
		if !eligible {
			continue
		}
		refTime := t.UpdatedAt
		if t.ClosedAt != nil {
			refTime = *t.ClosedAt
		}
		if !cutoff.IsZero() && refTime.After(cutoff) {
			continue
		}
		if !opts.DryRun {
			t.Compacted = true
			t.Summary = summarize(t)
			t.Description = ""
			if err := s.Update(t); err != nil {
				return nil, err
			}
		}
		res.Compacted++
		res.IDs = append(res.IDs, t.ID)
	}
	return res, nil
}

func summarize(t *Todo) string {
	var b strings.Builder
	b.WriteString(t.Title)
	if t.Priority != "" {
		b.WriteString(" [")
		b.WriteString(string(t.Priority))
		b.WriteString("]")
	}
	if t.Assignee != "" {
		b.WriteString(" @")
		b.WriteString(t.Assignee)
	}
	if len(t.Tags) > 0 {
		b.WriteString(" (")
		b.WriteString(strings.Join(t.Tags, ","))
		b.WriteString(")")
	}
	if t.Summary != "" {
		b.WriteString(": ")
		b.WriteString(t.Summary)
	}
	return b.String()
}
