// SPDX-License-Identifier: MIT
// Purpose: `sin instinct` subcommand tree. Slim CLI on top of Manager.
// Docs: cli.doc.md
package instinct

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// NewCommand returns the `sin instinct ...` command tree. Register it
// from `cmd/sin-code/main.go` via `internal.InstinctCmd = instinct.NewCommand()`.
func NewCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "instinct",
		Short: "Manage learned instincts (continuous learning)",
	}
	root.AddCommand(
		cmdStatus(),
		cmdProjects(),
		cmdEvolve(),
		cmdPromote(),
		cmdPrune(),
		cmdExport(),
		cmdImport(),
		cmdShow(),
		cmdForget(),
		cmdHistory(),
	)
	return root
}

func mgr() (*Manager, error) {
	wd, _ := os.Getwd()
	return NewManager(wd, nil)
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show active instincts for this project + global",
		RunE: func(c *cobra.Command, _ []string) error {
			m, err := mgr()
			if err != nil {
				return err
			}
			all, err := m.store.LoadEffective(m.project.ID)
			if err != nil {
				return err
			}
			fmt.Printf("Project: %s (%s)\n\n", m.project.Name, m.project.ID)
			if len(all) == 0 {
				fmt.Println("No instincts yet.")
				return nil
			}
			for _, i := range all {
				fmt.Printf("  [%.2f] %-8s %-12s %s\n", i.Confidence, i.Status, i.Domain, i.Trigger)
			}
			return nil
		},
	}
}

func cmdProjects() *cobra.Command {
	return &cobra.Command{
		Use:   "projects",
		Short: "List known projects and instinct counts",
		RunE: func(c *cobra.Command, _ []string) error {
			store := NewStore("")
			projects, err := store.ListProjects()
			if err != nil {
				return err
			}
			for _, p := range projects {
				pi, _ := store.LoadProject(p.ID)
				fmt.Printf("  %-14s %-30s %d instincts\n", p.ID, p.Name, len(pi))
			}
			return nil
		},
	}
}

func cmdEvolve() *cobra.Command {
	var apply bool
	c := &cobra.Command{
		Use:   "evolve",
		Short: "Cluster instincts into skill/command/agent proposals",
		RunE: func(c *cobra.Command, _ []string) error {
			m, err := mgr()
			if err != nil {
				return err
			}
			props, err := m.EvolveAll()
			if err != nil {
				return err
			}
			if len(props) == 0 {
				fmt.Println("No clusters ready to evolve.")
				return nil
			}
			for _, p := range props {
				fmt.Printf("- %s (%s, avg %.2f, %d members)\n", p.Name, p.Kind, p.AvgConfidence, len(p.Members))
				if apply {
					out := "./.sin/evolved/" + p.Name + ".md"
					_ = os.MkdirAll("./.sin/evolved", 0o755)
					if err := os.WriteFile(out, []byte(p.RenderArtifact()), 0o644); err != nil {
						return err
					}
					if err := MarkEvolved(m.store, p); err != nil {
						return err
					}
					fmt.Printf("    wrote %s\n", out)
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&apply, "apply", false, "write evolved artifacts to ./.sin/evolved/")
	return c
}

func cmdPromote() *cobra.Command {
	var apply bool
	c := &cobra.Command{
		Use:   "promote",
		Short: "Promote cross-project instincts to global scope",
		RunE: func(c *cobra.Command, _ []string) error {
			store := NewStore("")
			cands, err := FindPromotable(store)
			if err != nil {
				return err
			}
			if len(cands) == 0 {
				fmt.Println("Nothing to promote.")
				return nil
			}
			for _, cand := range cands {
				fmt.Printf("- %s (seen in %d projects)\n", cand.Signature, len(cand.Projects))
				if apply {
					g, err := Promote(store, cand)
					if err != nil {
						return err
					}
					fmt.Printf("    promoted -> global/%s.md\n", g.ID)
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&apply, "apply", false, "actually write the global instincts")
	return c
}

func cmdPrune() *cobra.Command {
	var ttl int
	c := &cobra.Command{
		Use:   "prune",
		Short: "Delete stale pending instincts and decay the rest",
		RunE: func(c *cobra.Command, _ []string) error {
			m, err := mgr()
			if err != nil {
				return err
			}
			n, err := m.Prune(ttl)
			if err != nil {
				return err
			}
			fmt.Printf("Pruned %d pending instincts.\n", n)
			return nil
		},
	}
	c.Flags().IntVar(&ttl, "ttl-days", 30, "delete pending instincts older than N days")
	return c
}

func cmdExport() *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "export",
		Short: "Export all instincts as JSONL",
		RunE: func(c *cobra.Command, _ []string) error {
			store := NewStore("")
			all, err := store.LoadAll()
			if err != nil {
				return err
			}
			w := os.Stdout
			if out != "" {
				f, err := os.Create(out)
				if err != nil {
					return err
				}
				defer f.Close()
				w = f
			}
			return ExportJSONL(w, all)
		},
	}
	c.Flags().StringVarP(&out, "out", "o", "", "output file (default stdout)")
	return c
}

func cmdImport() *cobra.Command {
	return &cobra.Command{
		Use:   "import [file]",
		Short: "Import instincts from a JSONL export",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			f, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer f.Close()
			list, parseErrs := ImportJSONL(f)
			store := NewStore("")
			n := 0
			for _, i := range list {
				if err := store.Save(i); err != nil {
					return err
				}
				n++
			}
			fmt.Printf("Imported %d instincts (%d malformed lines skipped).\n", n, len(parseErrs))
			return nil
		},
	}
}

func cmdShow() *cobra.Command {
	return &cobra.Command{
		Use:   "show [id]",
		Short: "Print one instinct (project + global search)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			m, err := mgr()
			if err != nil {
				return err
			}
			all, err := m.store.LoadEffective(m.project.ID)
			if err != nil {
				return err
			}
			for _, i := range all {
				if i.ID == args[0] || strings.HasPrefix(i.ID, args[0]) {
					b, _ := Marshal(i)
					fmt.Print(string(b))
					return nil
				}
			}
			return fmt.Errorf("instinct %q not found", args[0])
		},
	}
}

func cmdForget() *cobra.Command {
	var global bool
	c := &cobra.Command{
		Use:   "forget [id]",
		Short: "Delete an instinct by id",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			m, err := mgr()
			if err != nil {
				return err
			}
			scope := ScopeProject
			if global {
				scope = ScopeGlobal
			}
			target := &Instinct{ID: args[0], Scope: scope, ProjectID: m.project.ID}
			if err := m.store.Delete(target); err != nil {
				return err
			}
			m.store.Append(AuditEvent{InstinctID: args[0], Kind: "pruned", Detail: "manual forget"})
			fmt.Printf("Forgot %s\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVar(&global, "global", false, "delete from global scope instead of project")
	return c
}

func cmdHistory() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "history",
		Short: "Show recent learning events",
		RunE: func(c *cobra.Command, _ []string) error {
			store := NewStore("")
			events, err := store.ReadAudit(limit)
			if err != nil {
				return err
			}
			if len(events) == 0 {
				fmt.Println("No audit history.")
				return nil
			}
			for _, e := range events {
				fmt.Printf("%s  %-12s [%.2f] %s %s\n",
					e.Time.Format("2006-01-02 15:04"), e.Kind, e.Confidence, e.InstinctID, e.Detail)
			}
			return nil
		},
	}
	c.Flags().IntVar(&limit, "limit", 50, "max events to show")
	return c
}
