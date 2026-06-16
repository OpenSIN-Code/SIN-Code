// SPDX-License-Identifier: MIT
// Purpose: `sin prp ...` subcommand tree. Full-pipeline + per-phase
// commands for stepwise control.
// Docs: cli.doc.md
package prp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Deps supplies the engine collaborators to the CLI.
type Deps struct {
	Planner     Planner
	Implementer Implementer
	Verifier    Verifier
	PR          PRController
}

// NewCommand returns `sin prp ...`.
func NewCommand(deps Deps) *cobra.Command {
	root := &cobra.Command{Use: "prp", Short: "Product Requirement Prompt workflow (plan→implement→verify→pr)"}

	newEngine := func() *Engine {
		wd, _ := os.Getwd()
		return &Engine{
			Store: NewStore(wd), Workdir: wd,
			Planner: deps.Planner, Implementer: deps.Implementer,
			Verifier: deps.Verifier, PR: deps.PR,
			Log: func(f string, a ...any) { fmt.Printf("  "+f+"\n", a...) },
		}
	}

	var goal, ctxText string
	newCmd := &cobra.Command{
		Use:   "new [title]",
		Short: "Create a new PRP",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			title := strings.Join(args, " ")
			id := slugID(title)
			p, err := newEngine().New(id, title, goal, ctxText)
			if err != nil {
				return err
			}
			fmt.Printf("created PRP %s (%s)\n", p.ID, p.Phase)
			return nil
		},
	}
	newCmd.Flags().StringVar(&goal, "goal", "", "the goal of this change")
	newCmd.Flags().StringVar(&ctxText, "context", "", "relevant context")

	runCmd := &cobra.Command{
		Use:   "run [id]",
		Short: "Run the full pipeline for a PRP",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			eng := newEngine()
			p, err := eng.Store.Load(args[0])
			if err != nil {
				return err
			}
			return eng.RunAll(context.Background(), p)
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status [id]",
		Short: "Show PRP status (or list all)",
		RunE: func(c *cobra.Command, args []string) error {
			eng := newEngine()
			if len(args) == 1 {
				p, err := eng.Store.Load(args[0])
				if err != nil {
					return err
				}
				done, total := p.Progress()
				fmt.Printf("%s [%s] %d/%d tasks\n", p.ID, p.Phase, done, total)
				for _, t := range p.Tasks {
					fmt.Printf("  [%s] %s %s\n", t.State, t.ID, t.Title)
				}
				return nil
			}
			list, err := eng.Store.List()
			if err != nil {
				return err
			}
			for _, p := range list {
				done, total := p.Progress()
				fmt.Printf("  %-22s %-12s %d/%d  %s\n", p.ID, p.Phase, done, total, p.Title)
			}
			return nil
		},
	}

	// individual phase commands for stepwise control
	phaseCmd := func(use, short string, fn func(*Engine, *PRP) error) *cobra.Command {
		return &cobra.Command{
			Use: use, Short: short, Args: cobra.ExactArgs(1),
			RunE: func(c *cobra.Command, args []string) error {
				eng := newEngine()
				p, err := eng.Store.Load(args[0])
				if err != nil {
					return err
				}
				return fn(eng, p)
			},
		}
	}
	planCmd := phaseCmd("plan [id]", "Decompose the goal into tasks", func(e *Engine, p *PRP) error {
		return e.RunPlan(context.Background(), p)
	})
	implCmd := phaseCmd("implement [id]", "Implement tasks", func(e *Engine, p *PRP) error {
		return e.RunImplement(context.Background(), p)
	})
	verifyCmd := phaseCmd("verify [id]", "Run the quality gate", func(e *Engine, p *PRP) error {
		ok, report, err := e.RunVerify(context.Background(), p)
		fmt.Printf("verify: passed=%v\n%s\n", ok, report)
		return err
	})
	prCmd := phaseCmd("pr [id]", "Open a pull request", func(e *Engine, p *PRP) error {
		url, err := e.RunPR(context.Background(), p)
		if err != nil {
			return err
		}
		fmt.Printf("PR: %s\n", url)
		return nil
	})

	root.AddCommand(newCmd, runCmd, statusCmd, planCmd, implCmd, verifyCmd, prCmd)
	return root
}

func slugID(title string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = "prp"
	}
	return fmt.Sprintf("%s-%d", id, time.Now().Unix()%100000)
}
