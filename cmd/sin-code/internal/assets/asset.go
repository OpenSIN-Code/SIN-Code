// SPDX-License-Identifier: MIT
// Purpose: asset data model — agent/command/skill with YAML frontmatter +
// Markdown body. Port of ECC's asset shape in a clean-room Go reimplementation.
// Also contains: `sin assets ...` subcommand tree — `list`, `validate`,
// `show`, `import` (merged from cli.go).
// Docs: asset.doc.md, cli.doc.md
package assets

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"gopkg.in/yaml.v3"
)

// yamlMarshalHook is swapable for testing the Render error branch.
var yamlMarshalHook = yaml.Marshal

// Kind distinguishes the asset families harvested from ECC.
type Kind string

const (
	KindAgent   Kind = "agent"
	KindCommand Kind = "command"
	KindSkill   Kind = "skill"
)

// Asset is a loaded Markdown asset (agent/command/skill).
type Asset struct {
	Kind         Kind     `yaml:"-"`
	Path         string   `yaml:"-"`
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Model        string   `yaml:"model,omitempty"`         // agents
	Tools        []string `yaml:"tools,omitempty"`         // agents
	Color        string   `yaml:"color,omitempty"`         // agents (cosmetic)
	Argument     string   `yaml:"argument-hint,omitempty"` // commands
	AllowedTools []string `yaml:"allowed-tools,omitempty"` // commands
	Domain       string   `yaml:"domain,omitempty"`
	Origin       string   `yaml:"origin,omitempty"` // attribution, e.g. "ECC"
	License      string   `yaml:"license,omitempty"`

	Body string `yaml:"-"` // markdown content after frontmatter (the prompt itself)
}

// ParseAsset reads frontmatter + body from a Markdown file's bytes.
func ParseAsset(kind Kind, path string, data []byte) (*Asset, error) {
	text := string(data)
	if !strings.HasPrefix(text, "---") {
		return nil, fmt.Errorf("%s: missing frontmatter", path)
	}
	rest := strings.TrimPrefix(text, "---")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, fmt.Errorf("%s: unterminated frontmatter", path)
	}
	fm := rest[:idx]
	body := strings.TrimSpace(rest[idx+len("\n---"):])

	a := &Asset{Kind: kind, Path: path}
	if err := yaml.Unmarshal([]byte(fm), a); err != nil {
		return nil, fmt.Errorf("%s: parse frontmatter: %w", path, err)
	}
	a.Body = body
	return a, nil
}

// Render reassembles the asset back into canonical Markdown (for re-export).
func (a *Asset) Render() ([]byte, error) {
	fm, err := yamlMarshalHook(a)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(fm)
	b.WriteString("---\n\n")
	b.WriteString(a.Body)
	b.WriteString("\n")
	return b.Bytes(), nil
}

// ── CLI subcommand tree (merged from cli.go) ───────────────────────────
// `sin assets ...` subcommand tree — `list`, `validate`, `show`, `import`.
// Pass the directory where source assets are vendored (or use
// `assets import --source <path>`).

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
