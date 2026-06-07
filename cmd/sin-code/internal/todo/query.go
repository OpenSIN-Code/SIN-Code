package todo

import (
	"errors"
	"sort"
	"strings"
)

func (s *Store) ListFiltered(f ListFilter) ([]*Todo, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var out []*Todo
	for _, t := range all {
		if f.Status != "" && t.Status != f.Status {
			continue
		}
		if f.Priority != "" && t.Priority != f.Priority {
			continue
		}
		if f.Type != "" && t.Type != f.Type {
			continue
		}
		if f.Assignee != "" && t.Assignee != f.Assignee {
			continue
		}
		if f.Project != "" && t.Project != f.Project {
			continue
		}
		if f.Tag != "" {
			found := false
			for _, tg := range t.Tags {
				if tg == f.Tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if f.Search != "" {
			needle := strings.ToLower(f.Search)
			if !strings.Contains(strings.ToLower(t.Title), needle) &&
				!strings.Contains(strings.ToLower(t.Description), needle) {
				continue
			}
		}
		out = append(out, t)
	}
	sortTodos(out, f)
	return out, nil
}

func sortTodos(ts []*Todo, f ListFilter) {
	sort.SliceStable(ts, func(i, j int) bool {
		pi, pj := ts[i].Priority.Rank(), ts[j].Priority.Rank()
		if pi != pj {
			return pi < pj
		}
		return ts[i].CreatedAt.Before(ts[j].CreatedAt)
	})
}

func (s *Store) Ready() ([]*Todo, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	open := make([]*Todo, 0, len(all))
	for _, t := range all {
		if !t.IsOpen() {
			continue
		}
		blocking, err := s.BlockingDepsOf(t.ID)
		if err != nil {
			return nil, err
		}
		blocked := false
		for _, bd := range blocking {
			other, gerr := s.Get(bd.To)
			if gerr != nil {
				continue
			}
			if other.Status != StatusDone {
				blocked = true
				break
			}
		}
		if !blocked {
			open = append(open, t)
		}
	}
	sortTodos(open, ListFilter{})
	return open, nil
}

func (s *Store) Blocked() ([]*Todo, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var out []*Todo
	for _, t := range all {
		if !t.IsOpen() {
			continue
		}
		blocking, err := s.BlockingDepsOf(t.ID)
		if err != nil {
			return nil, err
		}
		hasOpenBlocker := false
		var blockers []string
		for _, bd := range blocking {
			other, gerr := s.Get(bd.To)
			if gerr != nil {
				continue
			}
			if other.Status != StatusDone {
				hasOpenBlocker = true
				blockers = append(blockers, bd.To)
			}
		}
		if hasOpenBlocker {
			tt := *t
			tt.Notes = t.Notes + "\nblockers: " + strings.Join(blockers, ",")
			out = append(out, &tt)
		}
	}
	sortTodos(out, ListFilter{})
	return out, nil
}

func (s *Store) Search(query string) ([]*Todo, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("query required")
	}
	f := ListFilter{Search: query}
	return s.ListFiltered(f)
}

func (s *Store) Mine(assignee string) ([]*Todo, error) {
	return s.ListFiltered(ListFilter{Assignee: assignee})
}

func (s *Store) ByProject(project string) ([]*Todo, error) {
	return s.ListFiltered(ListFilter{Project: project})
}

func (s *Store) ComputeStats() (*Stats, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	st := &Stats{
		ByStatus:   map[string]int{},
		ByPriority: map[string]int{},
		ByType:     map[string]int{},
		ByAssignee: map[string]int{},
	}
	for _, t := range all {
		st.Total++
		st.ByStatus[string(t.Status)]++
		st.ByPriority[string(t.Priority)]++
		st.ByType[string(t.Type)]++
		if t.Assignee != "" {
			st.ByAssignee[t.Assignee]++
		}
	}
	ready, err := s.Ready()
	if err != nil {
		return nil, err
	}
	st.Ready = len(ready)
	blocked, err := s.Blocked()
	if err != nil {
		return nil, err
	}
	st.Blocked = len(blocked)
	return st, nil
}
