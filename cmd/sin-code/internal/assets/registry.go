// SPDX-License-Identifier: MIT
// Purpose: in-memory index of loaded assets by (kind, name) and domain.
// Docs: registry.doc.md
package assets

import "sort"

// Registry indexes loaded assets for lookup by kind/name/domain.
type Registry struct {
	byKindName map[string]*Asset // key: kind + "/" + name
	all        []*Asset
}

func NewRegistry() *Registry {
	return &Registry{byKindName: map[string]*Asset{}}
}

func key(kind Kind, name string) string { return string(kind) + "/" + name }

// Add inserts/overwrites an asset.
func (r *Registry) Add(a *Asset) {
	k := key(a.Kind, a.Name)
	if _, exists := r.byKindName[k]; !exists {
		r.all = append(r.all, a)
	}
	r.byKindName[k] = a
}

// AddAll inserts a batch.
func (r *Registry) AddAll(list []*Asset) {
	for _, a := range list {
		r.Add(a)
	}
}

// Get fetches one asset.
func (r *Registry) Get(kind Kind, name string) (*Asset, bool) {
	a, ok := r.byKindName[key(kind, name)]
	return a, ok
}

// List returns all assets of a kind (empty kind = everything),
// sorted by name.
func (r *Registry) List(kind Kind) []*Asset {
	var out []*Asset
	for _, a := range r.all {
		if kind == "" || a.Kind == kind {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ForDomain returns assets matching a domain (e.g. "go", "security").
func (r *Registry) ForDomain(domain string) []*Asset {
	var out []*Asset
	for _, a := range r.all {
		if a.Domain == domain {
			out = append(out, a)
		}
	}
	return out
}
