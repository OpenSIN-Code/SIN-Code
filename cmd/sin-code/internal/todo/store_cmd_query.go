// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second query-related function is needed, merge into a shared file
package todo

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"
)

// ── ready / blocked / search ────────────────────────────────────────────────

var readyCmd = &cobra.Command{
	Use:   "ready",
	Short: "List unblocked open work (P0 first)",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.Ready()
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSONFn(ts)
		}
		printTodoTable(ts)
		return nil
	},
}

var blockedCmd = &cobra.Command{
	Use:   "blocked",
	Short: "List blocked work",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.Blocked()
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSONFn(ts)
		}
		printTodoTable(ts)
		return nil
	},
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search titles and descriptions",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.Search(args[0])
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSONFn(ts)
		}
		printTodoTable(ts)
		return nil
	},
}

// ── graph (DOT) ─────────────────────────────────────────────────────────────

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Output dependency graph in DOT format",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.List()
		if err != nil {
			return err
		}
		allDeps := map[string][]Dependency{}
		for _, t := range ts {
			deps, _ := store.GetDeps(t.ID)
			allDeps[t.ID] = deps
		}
		fmt.Println("digraph todo {")
		fmt.Println("  rankdir=LR;")
		fmt.Println("  node [shape=box, style=rounded];")
		for _, t := range ts {
			color := "white"
			switch t.Status {
			case StatusDone:
				color = "lightgreen"
			case StatusCancelled:
				color = "lightgray"
			case StatusBlocked:
				color = "lightyellow"
			case StatusInProgress:
				color = "lightblue"
			}
			label := fmt.Sprintf("%s\\n%s", t.ID, truncate(t.Title, 30))
			fmt.Printf("  %q [label=%q, fillcolor=%q, style=\"rounded,filled\"];\n", t.ID, label, color)
		}
		seenEdges := map[string]bool{}
		for from, deps := range allDeps {
			for _, d := range deps {
				ek := from + "->" + string(d.To)
				if seenEdges[ek] {
					continue
				}
				seenEdges[ek] = true
				style := "solid"
				if d.Type != DepBlocks {
					style = "dashed"
				}
				fmt.Printf("  %q -> %q [label=%q, style=%q];\n", from, d.To, string(d.Type), style)
			}
		}
		fmt.Println("}")
		return nil
	},
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// ── stats ───────────────────────────────────────────────────────────────────

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show counts by status, priority, type, assignee",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		st, err := store.ComputeStats()
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSONFn(st)
		}
		fmt.Printf("Total: %d\n", st.Total)
		fmt.Printf("Ready: %d\n", st.Ready)
		fmt.Printf("Blocked: %d\n", st.Blocked)
		printSortedCounts("By status", st.ByStatus)
		printSortedCounts("By priority", st.ByPriority)
		printSortedCounts("By type", st.ByType)
		printSortedCounts("By assignee", st.ByAssignee)
		return nil
	},
}

func printSortedCounts(title string, m map[string]int) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Printf("\n%s:\n", title)
	for _, k := range keys {
		fmt.Printf("  %-20s %d\n", k, m[k])
	}
}

// ── timeline ────────────────────────────────────────────────────────────────

var timelineCmd = &cobra.Command{
	Use:   "timeline [id]",
	Short: "Show audit log (optionally for a specific todo)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		id := ""
		if len(args) > 0 {
			id = args[0]
		}
		entries, err := store.ListAudit(id)
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSONFn(entries)
		}
		if len(entries) == 0 {
			fmt.Println("(no audit entries)")
			return nil
		}
		for _, e := range entries {
			fromTo := ""
			if e.From != "" || e.To != "" {
				fromTo = fmt.Sprintf(" %s→%s", e.From, e.To)
			}
			note := ""
			if e.Note != "" {
				note = " " + e.Note
			}
			fmt.Printf("[%s] %s %s %s%s%s\n",
				e.Timestamp.Format(time.RFC3339), e.Actor, e.Action, e.TodoID, fromTo, note)
		}
		return nil
	},
}
