// SPDX-License-Identifier: MIT
// Purpose: catalog — unified tool catalog that merges the legacy
// `internal/hub/` (static subcommand list) and `internal/assets/`
// (loaded Markdown frontmatter assets) under one Source interface.
//
// Issue #163: the v3.18.0 codebase has two CLIs (sin-code hub,
// sin-code assets) for the same concept (a catalog of tools).
// Operators think "do I have a tool for this?" — not "do I want
// the hub or the assets?". The answer should be one command.
//
// The Source interface is the abstraction that lets the catalog
// walk both backends without coupling to either. Adding a new
// source (e.g. a remote registry) is one file: a Source
// implementation.
//
// Docs: catalog.doc.md
package catalog

import (
	"context"
	"sort"
	"strings"
)

// Kind is the asset family. The same values as internal/assets.Kind
// so the catalog is a superset, not a replacement. The hub uses
// "command" exclusively; the asset loader uses all three.
type Kind string

const (
	KindAgent    Kind = "agent"
	KindCommand  Kind = "command"
	KindSkill    Kind = "skill"
	KindHub      Kind = "hub"      // legacy hub.Tool entries (subcommand metadata)
	KindMCP      Kind = "mcp"      // in-process MCP tools (sin_*)
	KindChat     Kind = "chat"     // built-in chat tools (sin_*)
	KindExternal Kind = "external" // external MCP server prefixes (browser__*, ...)
)

// Asset is the catalog-level representation of one tool. It is a
// superset of both internal/hub.Tool (Name, Short, Description,
// Example) and internal/assets.Asset (Kind, Name, Description,
// Body). Fields unique to one source (Body, Example) are optional.
type Asset struct {
	Kind        Kind     `json:"kind"`
	Name        string   `json:"name"`
	Namespace   string   `json:"namespace,omitempty"` // fully-qualified namespaced name
	Short       string   `json:"short,omitempty"`
	Description string   `json:"description"`
	Example     string   `json:"example,omitempty"`
	Source      string   `json:"source"` // "hub", "assets", etc.
	Domain      string   `json:"domain,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	ReadOnly    bool     `json:"read_only,omitempty"`
	Destructive bool     `json:"destructive,omitempty"`
}

// Source produces assets for the catalog. The interface is minimal
// because the two backends differ significantly: the hub is a static
// function call (no I/O), the asset loader is a Registry walk. Both
// can be expressed as a Source with the three methods below.
// sin-debt: yagni, upgrade: when a remote registry Source implementation lands, remove this marker
type Source interface {
	// Name returns the source's identifier ("hub", "assets", ...).
	// Used in the Source field of Asset and in de-duplication keys.
	Name() string

	// List returns all assets of the given kind. The implementation
	// is expected to filter by kind; the catalog does not pre-filter.
	// If kind is empty, all kinds are returned.
	List(ctx context.Context, kind Kind) ([]*Asset, error)

	// Get returns the asset with the given (kind, name), or nil +
	// (false, nil) if not found. (false, error) is reserved for
	// I/O failures; "not found" is not an error.
	Get(ctx context.Context, kind Kind, name string) (*Asset, bool, error)
}

// dedupKey is the (kind, name, source) triple used for de-duplication.
// The source is part of the key so two sources can have an asset with
// the same name (e.g. a hub.Tool "chat" and an assets.Asset "chat"
// both legitimately exist) without being merged. The merger only
// collapses exact duplicates.
type dedupKey struct {
	Kind, Name, Source string
}

// Merge walks the given sources and produces a sorted, de-duplicated
// list of assets. The order is: by kind (alphabetical), then by name.
//
// De-duplication rule: an asset is "the same" as a previous asset
// iff (kind, name) match. The first source to provide a given
// (kind, name) pair wins; subsequent duplicates are dropped. This
// keeps the merger deterministic even if multiple sources cover the
// same tool.
func Merge(ctx context.Context, sources []Source) ([]*Asset, error) {
	seen := map[dedupKey]bool{}
	var out []*Asset
	for _, src := range sources {
		for _, kind := range []Kind{KindAgent, KindCommand, KindSkill, KindHub, KindMCP, KindChat, KindExternal} {
			assets, err := src.List(ctx, kind)
			if err != nil {
				return nil, err
			}
			for _, a := range assets {
				if a == nil {
					continue
				}
				// Normalize: assign the source name so the catalog
				// always knows where each asset came from.
				if a.Source == "" {
					a.Source = src.Name()
				}
				// De-dup by (kind, name). Source name is intentionally
				// not in the dedup key so a hub.Tool and an assets.Asset
				// with the same name are merged.
				key := dedupKey{Kind: string(a.Kind), Name: a.Name, Source: ""}
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, a)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Search applies a case-insensitive substring match over Name,
// Short, Description, and Tags. The score is the number of fields
// that contained the query; assets with score > 0 are returned,
// sorted by score descending, then by name ascending.
func Search(assets []*Asset, query string) []*Asset {
	if query == "" {
		return assets
	}
	q := strings.ToLower(query)
	type scored struct {
		a     *Asset
		score int
	}
	var hits []scored
	for _, a := range assets {
		s := 0
		if strings.Contains(strings.ToLower(a.Name), q) {
			s += 4
		}
		if strings.Contains(strings.ToLower(a.Short), q) {
			s += 2
		}
		if strings.Contains(strings.ToLower(a.Description), q) {
			s += 1
		}
		for _, tag := range a.Tags {
			if strings.Contains(strings.ToLower(tag), q) {
				s += 1
				break
			}
		}
		if s > 0 {
			hits = append(hits, scored{a: a, score: s})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].a.Name < hits[j].a.Name
	})
	out := make([]*Asset, len(hits))
	for i, h := range hits {
		out[i] = h.a
	}
	return out
}

// FilterByKind returns the subset of assets with the given kind.
// If kind is empty, all assets are returned.
func FilterByKind(assets []*Asset, kind Kind) []*Asset {
	if kind == "" {
		return assets
	}
	out := make([]*Asset, 0, len(assets))
	for _, a := range assets {
		if a.Kind == kind {
			out = append(out, a)
		}
	}
	return out
}

// FilterUnused returns assets whose Name or Namespace has never been recorded
// in the used map. If an asset has a Namespace, the namespace is checked
// first; the simple Name is always checked as a fallback.
func FilterUnused(assets []*Asset, used map[string]int64) []*Asset {
	out := make([]*Asset, 0, len(assets))
	for _, a := range assets {
		name := a.Name
		if a.Namespace != "" {
			name = a.Namespace
		}
		if used[name] == 0 && used[a.Name] == 0 {
			out = append(out, a)
		}
	}
	return out
}
