// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second meta-related function is needed, merge into a shared file
package todo

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"

	"github.com/spf13/cobra"
)

// ── mine ────────────────────────────────────────────────────────────────────

var mineCmd = &cobra.Command{
	Use:   "mine",
	Short: "List todos assigned to current user",
	RunE: func(cmd *cobra.Command, args []string) error {
		actor := currentActorFn()
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.Mine(actor)
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSONFn(ts)
		}
		fmt.Printf("Assigned to %s:\n", actor)
		printTodoTable(ts)
		return nil
	},
}

// ── project ─────────────────────────────────────────────────────────────────

var projectCmd = &cobra.Command{
	Use:   "project [name]",
	Short: "Switch project namespace (no arg = show current)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		if len(args) == 0 {
			p, _ := store.GetMeta("current_project")
			if p == "" {
				p = currentProjectFn()
			}
			fmt.Printf("Current project: %s\n", p)
			return nil
		}
		if err := store.SetMeta("current_project", args[0]); err != nil {
			return err
		}
		fmt.Printf("Switched to project: %s\n", args[0])
		return nil
	},
}

// ── remember / prime ────────────────────────────────────────────────────────

var rememberCmd = &cobra.Command{
	Use:   "remember <insight>",
	Short: "Store a persistent memory/insight",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		m := &Memory{
			Insight: args[0],
			Actor:   currentActorFn(),
		}
		if err := store.AddMemory(m); err != nil {
			return err
		}
		fmt.Printf("Remembered %s\n", m.ID)
		return nil
	},
}

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Print context to prepend to an agent prompt",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		ready, _ := store.Ready()
		blocked, _ := store.Blocked()
		mine, _ := store.Mine(currentActorFn())
		fmt.Println("# sin-code todo context")
		fmt.Printf("Project: %s\n", currentProjectFn())
		fmt.Printf("Ready: %d  Blocked: %d  Mine: %d\n", len(ready), len(blocked), len(mine))
		if len(ready) > 0 {
			fmt.Println("\n## Ready work")
			for _, t := range ready {
				fmt.Printf("- %s [%s] %s\n", t.ID, t.Priority, t.Title)
			}
		}
		if len(blocked) > 0 {
			fmt.Println("\n## Blocked")
			for _, t := range blocked {
				fmt.Printf("- %s [%s] %s\n", t.ID, t.Priority, t.Title)
			}
		}
		if len(mine) > 0 {
			fmt.Println("\n## Mine")
			for _, t := range mine {
				fmt.Printf("- %s [%s] %s\n", t.ID, t.Priority, t.Title)
			}
		}
		return nil
	},
}

// ── compact ─────────────────────────────────────────────────────────────────

var compactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Summarize old closed todos to free memory",
	RunE: func(cmd *cobra.Command, args []string) error {
		older, _ := cmd.Flags().GetString("older-than")
		dry, _ := cmd.Flags().GetBool("dry-run")
		dur, err := time.ParseDuration(older)
		if err != nil {
			return fmt.Errorf("invalid --older-than: %w", err)
		}
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		res, err := store.Compact(CompactOptions{OlderThan: dur, DryRun: dry})
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSONFn(res)
		}
		verb := "Compacted"
		if dry {
			verb = "Would compact"
		}
		fmt.Printf("%s %d todos\n", verb, res.Compacted)
		if len(res.IDs) > 0 && len(res.IDs) <= 20 {
			for _, id := range res.IDs {
				fmt.Printf("  %s\n", id)
			}
		}
		return nil
	},
}

func init() {
	compactCmd.Flags().String("older-than", "720h", "Only compact todos older than this (Go duration, e.g. 720h, 30d invalid - use 720h)")
	compactCmd.Flags().Bool("dry-run", false, "Show what would be compacted without modifying")
}

// ── init / doctor ───────────────────────────────────────────────────────────

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the bbolt database",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		fmt.Printf("Initialized: %s\n", store.Path())
		return nil
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Health check of the todo database",
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
		stats, err := store.ComputeStats()
		if err != nil {
			return err
		}
		auditCount, _ := store.CountAudit()
		report := map[string]interface{}{
			"db_path":     store.Path(),
			"total":       len(ts),
			"by_status":   stats.ByStatus,
			"by_priority": stats.ByPriority,
			"audit_count": auditCount,
			"ready":       stats.Ready,
			"blocked":     stats.Blocked,
			"healthy":     true,
		}
		if todoFormat == "json" {
			return printJSONFn(report)
		}
		fmt.Printf("DB: %s\n", store.Path())
		fmt.Printf("Total todos: %d\n", len(ts))
		fmt.Printf("Ready: %d  Blocked: %d\n", stats.Ready, stats.Blocked)
		fmt.Printf("Audit entries: %d\n", auditCount)
		fmt.Println("Status: healthy")
		return nil
	},
}

// ── export / import ─────────────────────────────────────────────────────────

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export todos as JSON, Markdown, or JSONL",
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		output, _ := cmd.Flags().GetString("output")
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.List()
		if err != nil {
			return err
		}
		var data []byte
		switch format {
		case "json":
			data, _ = jsonMarshalIndentTodo(ts, "", "  ")
		case "jsonl":
			var b strings.Builder
			for _, t := range ts {
				line, _ := jsonMarshalTodo(t)
				b.Write(line)
				b.WriteByte('\n')
			}
			data = []byte(b.String())
		case "markdown", "md":
			data = []byte(exportMarkdown(ts))
		default:
			return fmt.Errorf("unknown format: %q (use json|jsonl|markdown)", format)
		}
		if output != "" && output != "-" {
			return osWriteFileTodo(output, data, filemode.Default())
		}
		fmt.Print(string(data))
		return nil
	},
}

func init() {
	exportCmd.Flags().String("format", "json", "Export format: json|jsonl|markdown")
	exportCmd.Flags().StringP("output", "o", "-", "Output file or - for stdout")
}

func exportMarkdown(ts []*Todo) string {
	var b strings.Builder
	b.WriteString("# Todo Export\n\n")
	for _, t := range ts {
		fmt.Fprintf(&b, "## %s — %s\n\n", t.ID, t.Title)
		fmt.Fprintf(&b, "- **Status:** %s\n", t.Status)
		fmt.Fprintf(&b, "- **Priority:** %s\n", t.Priority)
		fmt.Fprintf(&b, "- **Type:** %s\n", t.Type)
		if t.Assignee != "" {
			fmt.Fprintf(&b, "- **Assignee:** %s\n", t.Assignee)
		}
		if len(t.Tags) > 0 {
			fmt.Fprintf(&b, "- **Tags:** %s\n", strings.Join(t.Tags, ", "))
		}
		if t.Description != "" {
			fmt.Fprintf(&b, "\n%s\n", t.Description)
		}
		b.WriteString("\n")
	}
	return b.String()
}

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import todos from JSON or JSONL file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		data, err := osReadFileTodo(args[0])
		if err != nil {
			return err
		}
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		var items []*Todo
		switch format {
		case "json":
			if err := json.Unmarshal(data, &items); err != nil {
				return err
			}
		case "jsonl":
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var t Todo
				if err := json.Unmarshal([]byte(line), &t); err != nil {
					return err
				}
				items = append(items, &t)
			}
		default:
			return fmt.Errorf("unknown format: %q (use json|jsonl)", format)
		}
		imported := 0
		for _, t := range items {
			t.ID = ""
			if err := store.Add(t); err != nil {
				return err
			}
			imported++
		}
		if todoFormat == "json" {
			return printJSONFn(map[string]int{"imported": imported})
		}
		fmt.Printf("Imported %d todos\n", imported)
		return nil
	},
}

func init() {
	importCmd.Flags().String("format", "json", "Import format: json|jsonl")
}

