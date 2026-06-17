// SPDX-License-Identifier: MIT
// Purpose: Source adapter for the legacy `internal/hub/` catalog.
// The hub is a static function call (DefaultCatalog returns a
// hard-coded list of tools) so the adapter is one method that
// converts each hub.Tool to a catalog.Asset of kind=hub.
package catalog

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hub"
)

// HubSource is a Source backed by the static hub catalog. It is
// cheap (no I/O) and safe to use in CLI startup.
type HubSource struct{}

// Name implements Source.
func (HubSource) Name() string { return "hub" }

// List implements Source.
func (HubSource) List(_ context.Context, kind Kind) ([]*Asset, error) {
	if kind != "" && kind != KindHub {
		return nil, nil
	}
	tools := hub.AllTools()
	out := make([]*Asset, 0, len(tools))
	for _, t := range tools {
		out = append(out, &Asset{
			Kind:        KindHub,
			Name:        t.Name,
			Namespace:   t.Namespace,
			Short:       t.Short,
			Description: t.Description,
			Example:     t.Example,
			Source:      "hub",
			Tags:        append([]string{t.Category}, t.Tags...),
			ReadOnly:    t.ReadOnly,
			Destructive: t.Destructive,
		})
	}
	return out, nil
}

// Get implements Source.
func (HubSource) Get(_ context.Context, kind Kind, name string) (*Asset, bool, error) {
	if kind != "" && kind != KindHub {
		return nil, false, nil
	}
	for _, t := range hub.AllTools() {
		if t.Name == name {
			return &Asset{
				Kind:        KindHub,
				Name:        t.Name,
				Namespace:   t.Namespace,
				Short:       t.Short,
				Description: t.Description,
				Example:     t.Example,
				Source:      "hub",
				Tags:        append([]string{t.Category}, t.Tags...),
				ReadOnly:    t.ReadOnly,
				Destructive: t.Destructive,
			}, true, nil
		}
	}
	return nil, false, nil
}
