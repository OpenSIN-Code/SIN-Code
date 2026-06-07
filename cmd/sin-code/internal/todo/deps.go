package todo

import (
	"encoding/json"
	"fmt"
	"strings"

	bolt "go.etcd.io/bbolt"
)

func depKey(from, to string, dtype DepType) []byte {
	return []byte(from + "\x00" + to + "\x00" + string(dtype))
}

func depPrefix(from string) []byte {
	return []byte(from + "\x00")
}

func (s *Store) AddDep(dep Dependency) error {
	if dep.From == "" || dep.To == "" {
		return fmt.Errorf("from and to required")
	}
	if dep.From == dep.To {
		return fmt.Errorf("self-dependency not allowed")
	}
	if !dep.Type.Valid() {
		return fmt.Errorf("invalid dep type: %q", dep.Type)
	}
	if dep.Type == DepBlocks {
		if cycle, err := s.wouldCreateCycle(dep.From, dep.To); err != nil {
			return err
		} else if cycle {
			return fmt.Errorf("dependency would create a cycle: %s -> %s", dep.From, dep.To)
		}
	}
	return s.update(func(tx *bolt.Tx) error {
		bT := tx.Bucket([]byte(bucketTodos))
		if bT.Get(todoKey(dep.From)) == nil {
			return fmt.Errorf("from todo not found: %s", dep.From)
		}
		if bT.Get(todoKey(dep.To)) == nil {
			return fmt.Errorf("to todo not found: %s", dep.To)
		}
		bD := tx.Bucket([]byte(bucketDeps))
		return bD.Put(depKey(dep.From, dep.To, dep.Type), []byte("1"))
	})
}

func (s *Store) RemoveDep(from, to string) error {
	return s.update(func(tx *bolt.Tx) error {
		bD := tx.Bucket([]byte(bucketDeps))
		found := false
		for _, dt := range []DepType{DepBlocks, DepParentChild, DepRelated, DepDiscoveredFrom, DepDuplicates, DepSupersedes} {
			if err := bD.Delete(depKey(from, to, dt)); err != nil {
				return err
			}
			found = true
		}
		if !found {
			return fmt.Errorf("no dependency from %s to %s", from, to)
		}
		return nil
	})
}

func (s *Store) GetDeps(id string) ([]Dependency, error) {
	var out []Dependency
	err := s.view(func(tx *bolt.Tx) error {
		bD := tx.Bucket([]byte(bucketDeps))
		prefix := depPrefix(id)
		c := bD.Cursor()
		for k, _ := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, _ = c.Next() {
			parts := strings.SplitN(string(k), "\x00", 3)
			if len(parts) != 3 {
				continue
			}
			out = append(out, Dependency{From: parts[0], To: parts[1], Type: DepType(parts[2])})
		}
		return nil
	})
	return out, err
}

func (s *Store) GetReverseDeps(id string) ([]Dependency, error) {
	var out []Dependency
	err := s.view(func(tx *bolt.Tx) error {
		bD := tx.Bucket([]byte(bucketDeps))
		bT := tx.Bucket([]byte(bucketTodos))
		_ = bT
		c := bD.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			parts := strings.SplitN(string(k), "\x00", 3)
			if len(parts) != 3 {
				continue
			}
			if parts[1] == id {
				out = append(out, Dependency{From: parts[0], To: parts[1], Type: DepType(parts[2])})
			}
		}
		return nil
	})
	return out, err
}

func (s *Store) BlockingDepsOf(id string) ([]Dependency, error) {
	all, err := s.GetDeps(id)
	if err != nil {
		return nil, err
	}
	var out []Dependency
	for _, d := range all {
		if d.Type == DepBlocks {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *Store) wouldCreateCycle(from, to string) (bool, error) {
	visited := map[string]bool{}
	var dfs func(string) (bool, error)
	dfs = func(node string) (bool, error) {
		if node == from {
			return true, nil
		}
		if visited[node] {
			return false, nil
		}
		visited[node] = true
		deps, err := s.GetDeps(node)
		if err != nil {
			return false, err
		}
		for _, d := range deps {
			if d.Type != DepBlocks {
				continue
			}
			if cycle, err := dfs(d.To); err != nil {
				return false, err
			} else if cycle {
				return true, nil
			}
		}
		return false, nil
	}
	res, err := dfs(to)
	return res, err
}

func (s *Store) DependencyTree(root string, maxDepth int) (map[string][]Dependency, error) {
	out := map[string][]Dependency{}
	visited := map[string]bool{}
	var walk func(string, int) error
	walk = func(id string, depth int) error {
		if visited[id] || depth > maxDepth {
			return nil
		}
		visited[id] = true
		deps, err := s.GetDeps(id)
		if err != nil {
			return err
		}
		out[id] = deps
		for _, d := range deps {
			if err := walk(d.To, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, err
	}
	return out, nil
}

func depJSON(d Dependency) ([]byte, error) {
	return json.Marshal(d)
}

var _ = bolt.Tx{}
