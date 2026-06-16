// SPDX-License-Identifier: MIT
// Purpose: Source adapter for the `internal/assets/` registry. The
// asset loader is a runtime-loaded YAML-frontmatter store, so the
// adapter walks a *assets.Registry and produces catalog.Asset values.
//
// Use NewAssetsSource(reg) to construct. If reg is nil, the source
// returns no assets (the CLI handles the nil case by skipping the
// source).
package catalog

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/assets"
)

// AssetsSource is a Source backed by an *assets.Registry.
type AssetsSource struct {
	Reg *assets.Registry
}

// NewAssetsSource constructs an AssetsSource. The registry may be nil;
// a nil registry is treated as "no assets" (List returns empty,
// Get returns not-found).
func NewAssetsSource(reg *assets.Registry) *AssetsSource {
	return &AssetsSource{Reg: reg}
}

// Name implements Source.
func (s *AssetsSource) Name() string { return "assets" }

// kindMap translates the assets.Kind values (KindAgent, KindCommand,
// KindSkill) to the catalog.Kind values. They happen to be the same
// strings today, but the indirection is here so a future divergence
// in the assets package does not break the catalog.
func kindMap(k assets.Kind) Kind {
	switch k {
	case assets.KindAgent:
		return KindAgent
	case assets.KindCommand:
		return KindCommand
	case assets.KindSkill:
		return KindSkill
	}
	return Kind("")
}

// List implements Source.
func (s *AssetsSource) List(_ context.Context, kind Kind) ([]*Asset, error) {
	if s.Reg == nil {
		return nil, nil
	}
	// Map catalog.Kind back to assets.Kind, or list all.
	var ak assets.Kind
	switch kind {
	case KindAgent:
		ak = assets.KindAgent
	case KindCommand:
		ak = assets.KindCommand
	case KindSkill:
		ak = assets.KindSkill
	}
	if ak != "" {
		all := s.Reg.List(ak)
		return convertAll(all), nil
	}
	var out []*Asset
	for _, k := range []assets.Kind{assets.KindAgent, assets.KindCommand, assets.KindSkill} {
		out = append(out, convertAll(s.Reg.List(k))...)
	}
	return out, nil
}

// Get implements Source.
func (s *AssetsSource) Get(_ context.Context, kind Kind, name string) (*Asset, bool, error) {
	if s.Reg == nil {
		return nil, false, nil
	}
	ak := assets.Kind("")
	switch kind {
	case KindAgent:
		ak = assets.KindAgent
	case KindCommand:
		ak = assets.KindCommand
	case KindSkill:
		ak = assets.KindSkill
	}
	if ak == "" {
		return nil, false, nil
	}
	a, ok := s.Reg.Get(ak, name)
	if !ok {
		return nil, false, nil
	}
	return convertOne(a), true, nil
}

func convertAll(in []*assets.Asset) []*Asset {
	out := make([]*Asset, 0, len(in))
	for _, a := range in {
		out = append(out, convertOne(a))
	}
	return out
}

func convertOne(a *assets.Asset) *Asset {
	if a == nil {
		return nil
	}
	return &Asset{
		Kind:        kindMap(a.Kind),
		Name:        a.Name,
		Description: a.Description,
		Source:      "assets",
		Domain:      a.Domain,
	}
}
