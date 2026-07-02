// SPDX-License-Identifier: MIT
package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/gsd"
)

func NewGSDCmd() *cobra.Command {
	var rootFlag string
	cmd := &cobra.Command{
		Use:   "gsd",
		Short: "GSD — Get Shit Done project lifecycle management",
		Long: `sin-code gsd manages a phased project lifecycle with plans,
execution waves, and deterministic state tracking.

State persists in a .gsd/ directory containing PROJECT.md,
ROADMAP.md, and STATE.md.

Subcommands:
  init --name <name> [--description <desc>]   initialize a new project
  status [--json]                              show project status
  phase add|insert|remove|edit|list            phase CRUD
  plan <phase-id>                              show plan for a phase
  execute <phase-id> [--task <id> --status s]  show or update execution
  help                                         show this help`,
	}
	cmd.PersistentFlags().StringVar(&rootFlag, "root", ".", "project root directory")

	cmd.AddCommand(newGSDInitCmd(&rootFlag))
	cmd.AddCommand(newGSDStatusCmd(&rootFlag))
	cmd.AddCommand(newGSDPhaseCmd(&rootFlag))
	cmd.AddCommand(newGSDPlanCmd(&rootFlag))
	cmd.AddCommand(newGSDExecuteCmd(&rootFlag))
	cmd.AddCommand(newGSDHelpCmd())
	return cmd
}

func newGSDInitCmd(rootFlag *string) *cobra.Command {
	var name, description string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a new GSD project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name is required")
			}
			if err := gsd.InitProject(*rootFlag, name, description); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Initialized GSD project %q in %s/.gsd/\n", name, *rootFlag)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "project name (required)")
	cmd.Flags().StringVar(&description, "description", "", "project description")
	cmd.MarkFlagRequired("name")
	return cmd
}

func newGSDStatusCmd(rootFlag *string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show project status",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := gsd.ProjectStatus(*rootFlag)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Project: %s\n", report.Project.Name)
			if report.Project.Description != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", report.Project.Description)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Phases: %d\n", report.PhaseCount)
			fmt.Fprintf(cmd.OutOrStdout(), "Completed: %d\n", report.CompletedCount)
			fmt.Fprintf(cmd.OutOrStdout(), "Completion: %.1f%%\n", report.CompletionPct)
			if report.CurrentPhase != "" {
				ph, err := gsd.GetPhase(*rootFlag, report.CurrentPhase)
				if err == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Current phase: %s [%s] (%s)\n", ph.ID, ph.Title, ph.Status)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "Current phase: %s\n", report.CurrentPhase)
				}
			} else if report.PhaseCount > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Current phase: (all done)")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

func newGSDPhaseCmd(rootFlag *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "phase",
		Short: "Phase CRUD operations",
	}
	cmd.AddCommand(newGSDPhaseAddCmd(rootFlag))
	cmd.AddCommand(newGSDPhaseInsertCmd(rootFlag))
	cmd.AddCommand(newGSDPhaseRemoveCmd(rootFlag))
	cmd.AddCommand(newGSDPhaseEditCmd(rootFlag))
	cmd.AddCommand(newGSDPhaseListCmd(rootFlag))
	return cmd
}

func newGSDPhaseAddCmd(rootFlag *string) *cobra.Command {
	var priority string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a new phase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ph, err := gsd.AddPhase(*rootFlag, args[0], priority)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Added phase %s [%s] (priority %s)\n", ph.ID, ph.Title, ph.Priority)
			return nil
		},
	}
	cmd.Flags().StringVar(&priority, "priority", gsd.PriorityP1, "priority: P0|P1|P2|P3")
	return cmd
}

func newGSDPhaseInsertCmd(rootFlag *string) *cobra.Command {
	var priority string
	cmd := &cobra.Command{
		Use:   "insert <after-id> <title>",
		Short: "Insert a phase after the specified phase",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ph, err := gsd.InsertPhase(*rootFlag, args[0], args[1], priority)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Inserted phase %s [%s] after %s\n", ph.ID, ph.Title, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&priority, "priority", gsd.PriorityP1, "priority: P0|P1|P2|P3")
	return cmd
}

func newGSDPhaseRemoveCmd(rootFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a phase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := gsd.RemovePhase(*rootFlag, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed phase %s\n", args[0])
			return nil
		},
	}
}

func newGSDPhaseEditCmd(rootFlag *string) *cobra.Command {
	var title, priority, status string
	cmd := &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a phase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := gsd.EditOpts{
				Title:    title,
				Priority: priority,
				Status:   status,
			}
			if err := gsd.EditPhase(*rootFlag, args[0], opts); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated phase %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&priority, "priority", "", "new priority: P0|P1|P2|P3")
	cmd.Flags().StringVar(&status, "status", "", "new status: planning|in-progress|completed|blocked")
	return cmd
}

func newGSDPhaseListCmd(rootFlag *string) *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all phases",
		RunE: func(cmd *cobra.Command, args []string) error {
			phases, err := gsd.ListPhases(*rootFlag)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(phases)
			}
			if len(phases) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no phases")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-6s %-8s %-12s %s\n", "ID", "PRIORITY", "STATUS", "TITLE")
			for _, ph := range phases {
				fmt.Fprintf(cmd.OutOrStdout(), "%-6s %-8s %-12s %s\n", ph.ID, ph.Priority, ph.Status, ph.Title)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}

func newGSDPlanCmd(rootFlag *string) *cobra.Command {
	return &cobra.Command{
		Use:   "plan <phase-id>",
		Short: "Show plan for a phase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !gsd.PlanExists(*rootFlag, args[0]) {
				fmt.Fprintf(cmd.OutOrStdout(), "No plan for phase %s\n", args[0])
				return nil
			}
			plan, err := gsd.LoadPlan(*rootFlag, args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Plan for phase %s (%d tasks):\n\n", args[0], len(plan.Tasks))
			for _, t := range plan.Tasks {
				marker := "[ ]"
				switch t.Status {
				case gsd.TaskStatusInProgress:
					marker = "[~]"
				case gsd.TaskStatusDone:
					marker = "[x]"
				case gsd.TaskStatusBlocked:
					marker = "[!]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  %s %s: %s\n", marker, t.ID, t.Description)
			}
			return nil
		},
	}
}

func newGSDExecuteCmd(rootFlag *string) *cobra.Command {
	var jsonOut bool
	var taskID, taskStatus string
	cmd := &cobra.Command{
		Use:   "execute <phase-id>",
		Short: "Show or update execution state for a phase",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if taskID != "" {
				if taskStatus == "" {
					return fmt.Errorf("--status is required with --task")
				}
				if err := gsd.UpdateTaskStatus(*rootFlag, args[0], taskID, taskStatus); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Updated task %s → %s\n", taskID, taskStatus)
				return nil
			}
			report, err := gsd.ExecuteState(*rootFlag, args[0])
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			if len(report.Waves) == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No execution state for phase %s\n", args[0])
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Execution for phase %s:\n", args[0])
			var pct float64
			if report.TotalCount > 0 {
				pct = float64(report.CompletedCount) / float64(report.TotalCount) * 100
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Progress: %.1f%% (%d/%d)\n", pct, report.CompletedCount, report.TotalCount)
			if report.NextWave >= 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Next wave: %d\n\n", report.NextWave+1)
			} else {
				fmt.Fprint(cmd.OutOrStdout(), "Next wave: (all complete)\n\n")
			}
			for i, wave := range report.Waves {
				fmt.Fprintf(cmd.OutOrStdout(), "Wave %d (%d tasks):\n", i+1, len(wave))
				for _, t := range wave {
					marker := "[ ]"
					switch t.Status {
					case gsd.TaskStatusInProgress:
						marker = "[~]"
					case gsd.TaskStatusDone:
						marker = "[x]"
					case gsd.TaskStatusBlocked:
						marker = "[!]"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  %s %s: %s\n", marker, t.ID, t.Description)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	cmd.Flags().StringVar(&taskID, "task", "", "task id to update (requires --status)")
	cmd.Flags().StringVar(&taskStatus, "status", "", "new task status: todo|in-progress|done|blocked")
	return cmd
}

func newGSDHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "help",
		Short: "Show GSD help",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), `GSD — Get Shit Done project lifecycle management

Commands:
  init --name <name> [--description <desc>]
      Initialize a new GSD project in .gsd/

  status [--json]
      Show project name, phase count, completion %, current phase

  phase add <title> [--priority P0|P1|P2|P3]
      Add a new phase (default priority P1)

  phase insert <after-id> <title> [--priority P0|P1|P2|P3]
      Insert a phase after the specified phase ID

  phase remove <id>
      Remove a phase and its associated plan

  phase edit <id> [--title <t>] [--priority <p>] [--status <s>]
      Edit a phase (all flags optional, only specified fields change)

  phase list [--json]
      List all phases in order

  plan <phase-id>
      Show the plan for a phase

  execute <phase-id> [--json] [--task <id> --status <s>]
      Without --task: show waves, progress, next wave
      With --task --status: update a task's status

  help
      Show this help message

Project root defaults to the current directory. Override with --root <path>.
`)
		},
	}
}
