// SPDX-License-Identifier: MIT
// Purpose: `sin-code catalog` — unified tool catalog (issue #163).
// Merges the legacy `sin-code hub` and `sin-code assets` into a
// single source-aware CLI.
//
// Subcommands:
//
//	sin-code catalog list                 # all assets, all sources
//	sin-code catalog list --kind=agent    # filter by kind
//	sin-code catalog search "query"       # substring search across name/desc/tags
//	sin-code catalog info <name>          # one asset by name
//
// Sources are the registered Source implementations (HubSource,
// AssetsSource). New sources are added by registering a Source
// in the DefaultSources() slice in internal/catalog/catalog.go.
//
// Docs: cmd/sin-code/internal/catalog/catalog.doc.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/catalog"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/telemetry"
)

// NewCatalogCmd builds the `catalog` cobra subcommand.
func NewCatalogCmd() *cobra.Command {
	var (
		kind   string
		format string
	)
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Unified tool catalog (hub + assets, one CLI)",
		Long: `sin-code catalog is the unified tool catalog that merges
the legacy 'sin-code hub' (static subcommand list) and
'sin-code assets' (loaded Markdown frontmatter assets) into one
source-aware CLI. Issue #163 closes the long-standing UX confusion
between the two commands.`,
		RunE: func(c *cobra.Command, _ []string) error {
			return runCatalog(c, kind, format)
		},
	}
	cmd.AddCommand(newCatalogListCmd())
	cmd.AddCommand(newCatalogSearchCmd())
	cmd.AddCommand(newCatalogInfoCmd())
	cmd.AddCommand(newCatalogUnusedCmd())
	cmd.PersistentFlags().StringVar(&kind, "kind", "", "filter by kind: agent|command|skill|hub|mcp|chat|external")
	cmd.PersistentFlags().StringVar(&format, "format", "text", "output format: text|json")
	return cmd
}

func newCatalogListCmd() *cobra.Command {
	var (
		kind   string
		format string
	)
	return &cobra.Command{
		Use:   "list",
		Short: "Flat list of all assets in the catalog",
		RunE: func(c *cobra.Command, _ []string) error {
			return runCatalog(c, kind, format)
		},
	}
}

func newCatalogSearchCmd() *cobra.Command {
	var (
		kind   string
		format string
	)
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Substring search across all assets",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			assets, err := loadCatalog(c)
			if err != nil {
				return err
			}
			assets = catalog.FilterByKind(assets, catalog.Kind(kind))
			hits := catalog.Search(assets, args[0])
			return renderCatalog(c.OutOrStdout(), hits, format)
		},
	}
}

func newCatalogInfoCmd() *cobra.Command {
	var kind string
	return &cobra.Command{
		Use:   "info <name>",
		Short: "Show one asset by name",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			sources := defaultCatalogSources()
			ctx := c.Context()
			for _, src := range sources {
				for _, k := range []catalog.Kind{catalog.KindAgent, catalog.KindCommand, catalog.KindSkill, catalog.KindHub, catalog.KindMCP, catalog.KindChat, catalog.KindExternal} {
					if kind != "" && k != catalog.Kind(kind) {
						continue
					}
					a, ok, err := src.Get(ctx, k, args[0])
					if err != nil {
						return err
					}
					if ok {
						return renderCatalog(c.OutOrStdout(), []*catalog.Asset{a}, "text")
					}
				}
			}
			fmt.Fprintf(c.ErrOrStderr(), "catalog: not found: %s\n", args[0])
			os.Exit(1)
			return nil
		},
	}
}

func newCatalogUnusedCmd() *cobra.Command {
	var (
		format string
		stub   bool
	)
	cmd := &cobra.Command{
		Use:   "unused",
		Short: "List catalog tools never used according to telemetry",
		RunE: func(c *cobra.Command, _ []string) error {
			assets, err := loadCatalog(c)
			if err != nil {
				return err
			}
			var provider telemetry.Provider
			if stub {
				provider = telemetry.Stub()
			} else {
				provider, err = telemetry.DefaultProvider()
				if err != nil {
					return err
				}
			}
			used, err := provider.UsedTools(c.Context())
			if err != nil {
				return err
			}
			unused := catalog.FilterUnused(assets, used)
			return renderCatalog(c.OutOrStdout(), unused, format)
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "output format: text|json")
	cmd.Flags().BoolVar(&stub, "stub", false, "use stub telemetry (assume nothing used)")
	return cmd
}

// runCatalog is the shared implementation for the root and list
// subcommands. Both call into Merge + FilterByKind + render.
func runCatalog(c *cobra.Command, kind, format string) error {
	assets, err := loadCatalog(c)
	if err != nil {
		return err
	}
	assets = catalog.FilterByKind(assets, catalog.Kind(kind))
	return renderCatalog(c.OutOrStdout(), assets, format)
}

// loadCatalog runs the merger over the default sources. The source
// list is intentionally hard-coded here (not a flag) so the
// deprecation story is clear: new sources are added in code, not
// by the operator.
func loadCatalog(c *cobra.Command) ([]*catalog.Asset, error) {
	return catalog.Merge(c.Context(), defaultCatalogSources())
}

// defaultCatalogSources returns the registered sources. Hub is
// always present; assets is added when a registry is available.
// The order matters for de-duplication (first source wins).
func defaultCatalogSources() []catalog.Source {
	return []catalog.Source{
		catalog.HubSource{},
		catalog.MCPSource{},
		catalog.ChatSource{},
		catalog.ExternalSource{},
		// AssetsSource is wired conditionally in the future when
		// the asset loader exposes a registry at startup. For
		// now, the hub covers the operator-facing catalog.
		// catalog.NewAssetsSource(reg),
	}
}

// renderCatalog writes the assets in the chosen format. JSON is
// stable; text is a human-readable table.
func renderCatalog(w io.Writer, assets []*catalog.Asset, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(assets)
	default:
		// text: one line per asset
		for _, a := range assets {
			line := fmt.Sprintf("%-7s %-20s %s",
				strings.ToUpper(string(a.Kind)),
				a.Name,
				catalogFirstLine(a.Description))
			if a.Short != "" && a.Short != firstWord(a.Description) {
				line = fmt.Sprintf("%-7s %-20s %s",
					strings.ToUpper(string(a.Kind)),
					a.Name,
					a.Short)
			}
			fmt.Fprintln(w, line)
		}
		return nil
	}
}

// firstLine returns the first non-empty line of s, trimmed.
func catalogFirstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// firstWord returns the first whitespace-separated word of s.
func firstWord(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// _ is a compile-time guard that the imports are used.
var _ = context.Background
