// SPDX-License-Identifier: MIT
// Purpose: `sin assets ...` subcommand tree — `list`, `validate`, `show`,
// `import`. Pass the directory where source assets are vendored (or use
// `assets import --source <path>`).
// Docs: cli.doc.md
package assets

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewCommand returns `sin assets ...`.
func NewCommand(defaultBase string) *cobra.Command {
	var base string
	root := &cobra.Command{Use: "assets", Short: "Manage harvested agents/commands/skills"}
	root.PersistentFlags().StringVar(&base, "base", defaultBase, "asset source directory")

	load := func() (*Registry, error) {
		list, err := LoadStandardLayout(base)
		if err != nil {
			return nil, err
		}
		reg := NewRegistry()
		reg.AddAll(list)
		return reg, nil
	}

	list := &cobra.Command{
		Use:   "list [agent|command|skill]",
		Short: "List loaded assets",
		RunE: func(c *cobra.Command, args []string) error {
			reg, err := load()
			if err != nil {
				return err
			}
			var k Kind
			if len(args) == 1 {
				k = Kind(args[0])
			}
			for _, a := range reg.List(k) {
				origin := a.Origin
				if origin == "" {
					origin = "-"
				}
				fmt.Printf("  %-9s %-28s origin=%s\n", a.Kind, a.Name, origin)
			}
			return nil
		},
	}

	validate := &cobra.Command{
		Use:   "validate",
		Short: "Validate all assets against schema (ECC CI parity)",
		RunE: func(c *cobra.Command, _ []string) error {
			list, err := LoadStandardLayout(base)
			if err != nil {
				return err
			}
			issues := ValidateAll(list)
			errs := 0
			for _, is := range issues {
				if is.Level == "error" {
					errs++
				}
				fmt.Printf("  [%s] %s: %s\n", is.Level, is.Path, is.Message)
			}
			fmt.Printf("\n%d assets, %d issues (%d errors)\n", len(list), len(issues), errs)
			if errs > 0 {
				return fmt.Errorf("%d validation errors", errs)
			}
			return nil
		},
	}

	show := &cobra.Command{
		Use:   "show [kind] [name]",
		Short: "Print one asset's prompt body",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			reg, err := load()
			if err != nil {
				return err
			}
			a, ok := reg.Get(Kind(args[0]), args[1])
			if !ok {
				return fmt.Errorf("not found: %s/%s", args[0], args[1])
			}
			fmt.Println(a.Body)
			return nil
		},
	}

	var (
		impSource  string
		impDest    string
		impOrigin  string
		impLicense string
		impDomains []string
		impDryRun  bool
		impAll     bool
	)
	importCmd := &cobra.Command{
		Use:   "import",
		Short: "Harvest skills from a vendored source repo (e.g. ECC) with attribution",
		RunE: func(c *cobra.Command, _ []string) error {
			domains := impDomains
			if impAll {
				domains = nil // no domain filter
			} else if len(domains) == 0 {
				// sensible default: language/engineering skills for a coding agent
				domains = []string{
					"golang", "go", "rust", "python", "pytorch", "react", "kotlin",
					"java", "jpa", "springboot", "django", "fastapi", "nestjs", "laravel",
					"quarkus", "cpp", "swift", "postgres", "database", "docker",
					"deployment", "api", "backend", "frontend", "coding-standards", "testing",
				}
			}
			rep, err := ImportSkills(ImportOptions{
				SourceBase:     impSource,
				DestDir:        impDest,
				Origin:         impOrigin,
				License:        impLicense,
				IncludeDomains: domains,
				ExcludeNames:   DefaultExclusions(),
				DryRun:         impDryRun,
			})
			if err != nil {
				return err
			}
			fmt.Printf("considered=%d imported=%d skipped=%d invalid=%d\n",
				rep.Considered, rep.Imported, rep.Skipped, rep.Invalid)
			for _, n := range rep.Names {
				fmt.Printf("  + %s\n", n)
			}
			for _, is := range rep.Issues {
				fmt.Printf("  ! [%s] %s: %s\n", is.Level, is.Path, is.Message)
			}
			if impDryRun {
				fmt.Println("(dry run — nothing written)")
			}
			return nil
		},
	}
	importCmd.Flags().StringVar(&impSource, "source", "./vendor/ecc", "source repo base")
	importCmd.Flags().StringVar(&impDest, "dest", "./skills/imported", "destination dir")
	importCmd.Flags().StringVar(&impOrigin, "origin", "ECC", "attribution origin stamp")
	importCmd.Flags().StringVar(&impLicense, "license", "", "license stamp (set from source LICENSE)")
	importCmd.Flags().StringSliceVar(&impDomains, "domains", nil, "only import skills matching these domains")
	importCmd.Flags().BoolVar(&impAll, "all", false, "import all skills (ignore domain filter)")
	importCmd.Flags().BoolVar(&impDryRun, "dry-run", false, "preview without writing")

	root.AddCommand(list, validate, show, importCmd)
	return root
}
