// SPDX-License-Identifier: MIT
// Purpose: `sin-code status` — single-glance health dashboard for the
// autonomous loop (loop-004). Renders the goal queue as a tree with status
// icons, continuation counts, and per-goal errors, plus the latest ledger
// events. --json emits the same data for CI/monitoring.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

func NewStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the current state of the autonomous loop: queue tree, recent completions, failures",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd.Context(), asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable JSON output")
	return cmd
}

// statusNode is one goal in the rendered tree (used for JSON output).
type statusNode struct {
	ID            int64         `json:"id"`
	Status        string        `json:"status"`
	Prompt        string        `json:"prompt"`
	Depth         int           `json:"depth"`
	Continuations int           `json:"continuations"`
	Attempts      int           `json:"attempts"`
	LastError     string        `json:"last_error,omitempty"`
	Children      []*statusNode `json:"children,omitempty"`
}

func runStatus(ctx context.Context, asJSON bool) error {
	q, err := autonomy.Open(autonomy.DefaultPath())
	if err != nil {
		return err
	}
	defer q.Close()

	all, err := q.List(ctx, "")
	if err != nil {
		return err
	}

	children := map[int64][]autonomy.Goal{}
	var roots []autonomy.Goal
	for _, g := range all {
		if g.ParentID == 0 {
			roots = append(roots, g)
		} else {
			children[g.ParentID] = append(children[g.ParentID], g)
		}
	}

	counts := map[autonomy.GoalStatus]int{}
	for _, g := range all {
		counts[g.Status]++
	}

	if asJSON {
		return printStatusJSON(roots, children, counts)
	}
	return printStatusText(ctx, roots, children, counts)
}

func printStatusText(ctx context.Context, roots []autonomy.Goal,
	children map[int64][]autonomy.Goal, counts map[autonomy.GoalStatus]int) error {

	fmt.Printf("SIN-Code Loop Status  (%s)\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Printf("  pending=%-4d running=%-4d blocked=%-4d verified=%-4d failed=%-4d exhausted=%d\n\n",
		counts[autonomy.StatusPending], counts[autonomy.StatusRunning], counts[autonomy.StatusBlocked],
		counts[autonomy.StatusVerified], counts[autonomy.StatusFailed], counts[autonomy.StatusExhausted])

	if len(roots) == 0 {
		fmt.Println("  (no goals in queue)")
	} else {
		var printTree func(g autonomy.Goal, indent string)
		printTree = func(g autonomy.Goal, indent string) {
			conts := ""
			if g.Continuations > 0 {
				conts = fmt.Sprintf(" [cont:%d]", g.Continuations)
			}
			fmt.Printf("%s%s #%-5d %-10s%s  %s\n",
				indent, statusIcon(g.Status), g.ID, g.Status, conts, truncatePrompt(g.Prompt, 72))
			if g.LastError != "" && g.Status != autonomy.StatusVerified {
				fmt.Printf("%s       err: %s\n", indent, truncatePrompt(g.LastError, 80))
			}
			for _, ch := range children[g.ID] {
				printTree(ch, indent+"  ")
			}
		}
		for _, g := range roots {
			printTree(g, "  ")
		}
	}

	// Recent ledger events (best-effort; missing ledger is not an error).
	if l, err := ledger.Open(ledger.DefaultPath()); err == nil {
		defer l.Close()
		if recent, _ := l.Recent(ctx, 5); len(recent) > 0 {
			fmt.Printf("\nRecent events:\n")
			for _, e := range recent {
				fmt.Printf("  %-19s  %-20s  %s\n",
					e.CreatedAt.Format("01-02 15:04:05"), e.Type, truncatePrompt(e.Summary, 60))
			}
		}
	}
	return nil
}

func printStatusJSON(roots []autonomy.Goal, children map[int64][]autonomy.Goal,
	counts map[autonomy.GoalStatus]int) error {

	var build func(g autonomy.Goal) *statusNode
	build = func(g autonomy.Goal) *statusNode {
		n := &statusNode{
			ID: g.ID, Status: string(g.Status), Prompt: g.Prompt, Depth: g.Depth,
			Continuations: g.Continuations, Attempts: g.Attempts, LastError: g.LastError,
		}
		for _, ch := range children[g.ID] {
			n.Children = append(n.Children, build(ch))
		}
		return n
	}
	tree := make([]*statusNode, 0, len(roots))
	for _, g := range roots {
		tree = append(tree, build(g))
	}
	summary := map[string]int{
		"pending":   counts[autonomy.StatusPending],
		"running":   counts[autonomy.StatusRunning],
		"blocked":   counts[autonomy.StatusBlocked],
		"verified":  counts[autonomy.StatusVerified],
		"failed":    counts[autonomy.StatusFailed],
		"exhausted": counts[autonomy.StatusExhausted],
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"summary": summary, "goals": tree})
}

// statusIcon maps a goal status to a short ASCII marker for the tree view.
func statusIcon(s autonomy.GoalStatus) string {
	switch s {
	case autonomy.StatusVerified:
		return "[x]"
	case autonomy.StatusRunning:
		return "[>]"
	case autonomy.StatusBlocked:
		return "[~]"
	case autonomy.StatusFailed:
		return "[!]"
	case autonomy.StatusExhausted:
		return "[X]"
	default:
		return "[ ]"
	}
}
