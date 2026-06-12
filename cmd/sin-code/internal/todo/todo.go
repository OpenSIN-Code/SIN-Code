package todo

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"
	"github.com/spf13/cobra"
)

var (
	todoDBPath  string
	todoProject string
	todoFormat  string
	todoAs      string
)

var TodoCmd = &cobra.Command{
	Use:   "todo",
	Short: "Issue tracker with dependencies, audit log, and project namespaces",
	Long: "todo is the SIN-Code issue tracker, matching the UX of `bd` and opencode's todo system. Backed by bbolt for durability, append-only audit log for history, and project namespaces for multi-repo work.\n\nCommon workflows:\n  sin-code todo add --title \"...\" --priority P0 --type feature\n  sin-code todo ready\n  sin-code todo dep add st-1234 st-5678 --type blocks\n  sin-code todo compact --older-than 30d",
	SilenceUsage: true,
}

func init() {
	TodoCmd.PersistentFlags().StringVar(&todoDBPath, "db", "", "Path to bbolt DB (default ~/.config/sin-code/todo.db)")
	TodoCmd.PersistentFlags().StringVar(&todoProject, "project", "", "Project namespace (default: current directory name)")
	TodoCmd.PersistentFlags().StringVar(&todoFormat, "format", "text", "Output format: text|json")
	TodoCmd.PersistentFlags().StringVar(&todoAs, "as", "", "Actor identity (default: git user.name)")

	TodoCmd.AddCommand(addCmd)
	TodoCmd.AddCommand(listCmd)
	TodoCmd.AddCommand(showCmd)
	TodoCmd.AddCommand(updateCmd)
	TodoCmd.AddCommand(claimCmd)
	TodoCmd.AddCommand(unclaimCmd)
	TodoCmd.AddCommand(completeCmd)
	TodoCmd.AddCommand(cancelCmd)
	TodoCmd.AddCommand(deleteCmd)
	TodoCmd.AddCommand(depCmd)
	TodoCmd.AddCommand(depsCmd)
	TodoCmd.AddCommand(readyCmd)
	TodoCmd.AddCommand(blockedCmd)
	TodoCmd.AddCommand(searchCmd)
	TodoCmd.AddCommand(graphCmd)
	TodoCmd.AddCommand(statsCmd)
	TodoCmd.AddCommand(timelineCmd)
	TodoCmd.AddCommand(mineCmd)
	TodoCmd.AddCommand(projectCmd)
	TodoCmd.AddCommand(rememberCmd)
	TodoCmd.AddCommand(primeCmd)
	TodoCmd.AddCommand(compactCmd)
	TodoCmd.AddCommand(initCmd)
	TodoCmd.AddCommand(doctorCmd)
	TodoCmd.AddCommand(exportCmd)
	TodoCmd.AddCommand(importCmd)
}

func openStore() (*Store, error) {
	return Open(todoDBPath)
}

func currentActor() string {
	if todoAs != "" {
		return todoAs
	}
	out, err := exec.Command("git", "config", "user.name").Output()
	if err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return name
		}
	}
	if u, err := os.UserConfigDir(); err == nil {
		return filepath.Base(u)
	}
	return "unknown"
}

func currentProject() string {
	if todoProject != "" {
		return todoProject
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Base(cwd)
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func statusIcon(s Status) string {
	switch s {
	case StatusDone:
		return "✓"
	case StatusCancelled:
		return "✗"
	case StatusBlocked:
		return "✗"
	case StatusInProgress:
		return "●"
	default:
		return "○"
	}
}

func printTodoTable(ts []*Todo) {
	if len(ts) == 0 {
		fmt.Println("(no todos)")
		return
	}
	fmt.Printf("%-8s %-4s %-12s %-8s %-12s %s\n", "ID", "PRI", "STATUS", "TYPE", "ASSIGNEE", "TITLE")
	fmt.Println(strings.Repeat("─", 80))
	for _, t := range ts {
		assignee := t.Assignee
		if assignee == "" {
			assignee = "-"
		}
		title := t.Title
		if t.Compacted {
			title = "[compacted] " + title
		}
		fmt.Printf("%-8s %-4s %s %-10s %-8s %-12s %s\n",
			t.ID, string(t.Priority), statusIcon(t.Status),
			string(t.Status), string(t.Type), assignee, title)
	}
}

// ── add ─────────────────────────────────────────────────────────────────────

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Create a new todo",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		desc, _ := cmd.Flags().GetString("desc")
		priority, _ := cmd.Flags().GetString("priority")
		ttype, _ := cmd.Flags().GetString("type")
		tags, _ := cmd.Flags().GetString("tags")
		assignee, _ := cmd.Flags().GetString("assignee")
		parent, _ := cmd.Flags().GetString("parent")
		externalRef, _ := cmd.Flags().GetString("external-ref")
		project, _ := cmd.Flags().GetString("project")

		if title == "" {
			return fmt.Errorf("--title is required")
		}
		if priority != "" && !Priority(priority).Valid() {
			return fmt.Errorf("invalid priority: %q (use P0..P3)", priority)
		}
		if ttype != "" && !TodoType(ttype).Valid() {
			return fmt.Errorf("invalid type: %q", ttype)
		}
		if project == "" {
			project = currentProject()
		}
		t := &Todo{
			Title:       title,
			Description: desc,
			Priority:    Priority(priority),
			Type:        TodoType(ttype),
			Tags:        splitList(tags),
			Assignee:    assignee,
			Parent:      parent,
			ExternalRef: externalRef,
			Project:     project,
		}
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Add(t); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: t.ID, Actor: currentActor(), Action: "create",
			To: t.Title,
		})
		fireHooks(store, EventPostAdd, t, "", t.Title, "")
		notify(notifications.TypeTodoCreated, t.ID, t.Title,
			fmt.Sprintf("New %s %s: %s", t.Priority, t.Type, t.Title), currentActor())
		if todoFormat == "json" {
			return printJSON(t)
		}
		fmt.Printf("Created %s: %s\n", t.ID, t.Title)
		return nil
	},
}

func init() {
	addCmd.Flags().StringP("title", "t", "", "Title (required)")
	addCmd.Flags().StringP("desc", "d", "", "Description")
	addCmd.Flags().StringP("priority", "p", "P2", "Priority: P0|P1|P2|P3")
	addCmd.Flags().String("type", "task", "Type: task|bug|feature|chore|epic|question")
	addCmd.Flags().String("tags", "", "Comma-separated tags")
	addCmd.Flags().String("assignee", "", "Assignee")
	addCmd.Flags().String("parent", "", "Parent todo ID")
	addCmd.Flags().String("external-ref", "", "External reference (e.g. GitHub issue)")
	addCmd.Flags().String("project", "", "Project namespace")
}

// ── list ────────────────────────────────────────────────────────────────────

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List todos with optional filters",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, _ := cmd.Flags().GetString("status")
		priority, _ := cmd.Flags().GetString("priority")
		ttype, _ := cmd.Flags().GetString("type")
		tag, _ := cmd.Flags().GetString("tag")
		assignee, _ := cmd.Flags().GetString("assignee")
		project, _ := cmd.Flags().GetString("project")
		search, _ := cmd.Flags().GetString("search")
		all, _ := cmd.Flags().GetBool("all")

		f := ListFilter{
			Status:   Status(status),
			Priority: Priority(priority),
			Type:     TodoType(ttype),
			Tag:      tag,
			Assignee: assignee,
			Project:  project,
			Search:   search,
		}
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()

		if all {
			ts, err := store.List()
			if err != nil {
				return err
			}
			if todoFormat == "json" {
				return printJSON(ts)
			}
			printTodoTable(ts)
			return nil
		}
		ts, err := store.ListFiltered(f)
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSON(ts)
		}
		printTodoTable(ts)
		return nil
	},
}

func init() {
	listCmd.Flags().String("status", "", "Filter by status")
	listCmd.Flags().String("priority", "", "Filter by priority")
	listCmd.Flags().String("type", "", "Filter by type")
	listCmd.Flags().String("tag", "", "Filter by tag")
	listCmd.Flags().String("assignee", "", "Filter by assignee")
	listCmd.Flags().String("project", "", "Filter by project")
	listCmd.Flags().String("search", "", "Substring search in title/description")
	listCmd.Flags().Bool("all", false, "Show all todos (ignore filters)")
}

// ── show ────────────────────────────────────────────────────────────────────

var showCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show full details of a todo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		t, err := store.Get(args[0])
		if err != nil {
			return err
		}
		audit, _ := store.ListAudit(t.ID)
		deps, _ := store.GetDeps(t.ID)
		rev, _ := store.GetReverseDeps(t.ID)
		if todoFormat == "json" {
			return printJSON(map[string]interface{}{
				"todo":     t,
				"deps":     deps,
				"deps_of":  rev,
				"audit":    audit,
			})
		}
		fmt.Printf("ID:        %s\n", t.ID)
		fmt.Printf("Title:     %s\n", t.Title)
		if t.Description != "" {
			fmt.Printf("Description:\n%s\n", t.Description)
		}
		fmt.Printf("Status:    %s %s\n", statusIcon(t.Status), t.Status)
		fmt.Printf("Priority:  %s\n", t.Priority)
		fmt.Printf("Type:      %s\n", t.Type)
		if t.Assignee != "" {
			fmt.Printf("Assignee:  %s\n", t.Assignee)
		}
		if t.Parent != "" {
			fmt.Printf("Parent:    %s\n", t.Parent)
		}
		if t.ExternalRef != "" {
			fmt.Printf("External:  %s\n", t.ExternalRef)
		}
		if t.Project != "" {
			fmt.Printf("Project:   %s\n", t.Project)
		}
		if len(t.Tags) > 0 {
			fmt.Printf("Tags:      %s\n", strings.Join(t.Tags, ", "))
		}
		fmt.Printf("Created:   %s\n", t.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Updated:   %s\n", t.UpdatedAt.Format(time.RFC3339))
		if t.ClosedAt != nil {
			fmt.Printf("Closed:    %s\n", t.ClosedAt.Format(time.RFC3339))
		}
		if t.Compacted {
			fmt.Printf("Summary:   %s\n", t.Summary)
		}
		if len(deps) > 0 {
			fmt.Println("\nDependencies (this depends on):")
			for _, d := range deps {
				fmt.Printf("  -> %s (%s)\n", d.To, d.Type)
			}
		}
		if len(rev) > 0 {
			fmt.Println("\nDepended on by:")
			for _, d := range rev {
				fmt.Printf("  <- %s (%s)\n", d.From, d.Type)
			}
		}
		if len(audit) > 0 {
			fmt.Println("\nAudit log:")
			for _, e := range audit {
				fmt.Printf("  [%s] %s %s: %s\n",
					e.Timestamp.Format(time.RFC3339), e.Actor, e.Action, e.Note)
			}
		}
		return nil
	},
}

// ── update ──────────────────────────────────────────────────────────────────

var updateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update fields of a todo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		t, err := store.Get(args[0])
		if err != nil {
			return err
		}
		old := t.Status
		changes := []string{}
		if v, _ := cmd.Flags().GetString("title"); v != "" {
			changes = append(changes, "title")
			t.Title = v
		}
		if v, _ := cmd.Flags().GetString("desc"); cmd.Flags().Changed("desc") {
			changes = append(changes, "desc")
			t.Description = v
		}
		if v, _ := cmd.Flags().GetString("priority"); v != "" {
			if !Priority(v).Valid() {
				return fmt.Errorf("invalid priority: %q", v)
			}
			changes = append(changes, "priority")
			t.Priority = Priority(v)
		}
		if v, _ := cmd.Flags().GetString("type"); v != "" {
			if !TodoType(v).Valid() {
				return fmt.Errorf("invalid type: %q", v)
			}
			changes = append(changes, "type")
			t.Type = TodoType(v)
		}
		if v, _ := cmd.Flags().GetString("status"); v != "" {
			if !Status(v).Valid() {
				return fmt.Errorf("invalid status: %q", v)
			}
			changes = append(changes, "status")
			t.Status = Status(v)
		}
		if v, _ := cmd.Flags().GetString("tags"); cmd.Flags().Changed("tags") {
			changes = append(changes, "tags")
			t.Tags = splitList(v)
		}
		if v, _ := cmd.Flags().GetString("assignee"); cmd.Flags().Changed("assignee") {
			changes = append(changes, "assignee")
			t.Assignee = v
		}
		if v, _ := cmd.Flags().GetString("external-ref"); cmd.Flags().Changed("external-ref") {
			changes = append(changes, "external-ref")
			t.ExternalRef = v
		}
		if v, _ := cmd.Flags().GetString("parent"); cmd.Flags().Changed("parent") {
			changes = append(changes, "parent")
			t.Parent = v
		}
		if v, _ := cmd.Flags().GetString("notes"); cmd.Flags().Changed("notes") {
			changes = append(changes, "notes")
			t.Notes = v
		}
		if len(changes) == 0 {
			return fmt.Errorf("no fields to update")
		}
		if err := store.Update(t); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: t.ID, Actor: currentActor(), Action: "update",
			From: string(old), To: string(t.Status), Note: strings.Join(changes, ","),
		})
		if todoFormat == "json" {
			return printJSON(t)
		}
		fmt.Printf("Updated %s (%s)\n", t.ID, strings.Join(changes, ","))
		return nil
	},
}

func init() {
	updateCmd.Flags().String("title", "", "New title")
	updateCmd.Flags().String("desc", "", "New description")
	updateCmd.Flags().String("priority", "", "New priority")
	updateCmd.Flags().String("type", "", "New type")
	updateCmd.Flags().String("status", "", "New status")
	updateCmd.Flags().String("tags", "", "New tags (comma-separated)")
	updateCmd.Flags().String("assignee", "", "New assignee")
	updateCmd.Flags().String("parent", "", "New parent")
	updateCmd.Flags().String("external-ref", "", "New external ref")
	updateCmd.Flags().String("notes", "", "New notes")
}

// ── claim / unclaim ─────────────────────────────────────────────────────────

var claimCmd = &cobra.Command{
	Use:   "claim <id>",
	Short: "Atomically claim a todo (assign to current user)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		t, err := store.Get(args[0])
		if err != nil {
			return err
		}
		actor := currentActor()
		if t.Assignee != "" && t.Assignee != actor {
			return fmt.Errorf("already claimed by %s", t.Assignee)
		}
		old := t.Assignee
		t.Assignee = actor
		if t.Status == StatusOpen {
			t.Status = StatusInProgress
		}
		if err := store.Update(t); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: t.ID, Actor: actor, Action: "claim",
			From: old, To: actor,
		})
		fireHooks(store, EventPostClaim, t, old, actor, "")
		fmt.Printf("Claimed %s by %s\n", t.ID, actor)
		return nil
	},
}

var unclaimCmd = &cobra.Command{
	Use:   "unclaim <id>",
	Short: "Release a claim on a todo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		t, err := store.Get(args[0])
		if err != nil {
			return err
		}
		old := t.Assignee
		t.Assignee = ""
		if t.Status == StatusInProgress {
			t.Status = StatusOpen
		}
		if err := store.Update(t); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: t.ID, Actor: currentActor(), Action: "unclaim",
			From: old, To: "",
		})
		fmt.Printf("Unclaimed %s (was %s)\n", t.ID, old)
		return nil
	},
}

// ── complete / cancel / delete ──────────────────────────────────────────────

var completeCmd = &cobra.Command{
	Use:   "complete <id>",
	Short: "Mark a todo as done",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		t, err := store.Get(args[0])
		if err != nil {
			return err
		}
		old := t.Status
		t.Status = StatusDone
		if err := store.Update(t); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: t.ID, Actor: currentActor(), Action: "complete",
			From: string(old), To: string(t.Status),
		})
		fireHooks(store, EventPostComplete, t, string(old), string(t.Status), "")
		fmt.Printf("Completed %s: %s\n", t.ID, t.Title)
		return nil
	},
}

var cancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Mark a todo as cancelled",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		t, err := store.Get(args[0])
		if err != nil {
			return err
		}
		old := t.Status
		t.Status = StatusCancelled
		if err := store.Update(t); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: t.ID, Actor: currentActor(), Action: "cancel",
			From: string(old), To: string(t.Status),
		})
		fireHooks(store, EventPostCancel, t, string(old), string(t.Status), "")
		fmt.Printf("Cancelled %s: %s\n", t.ID, t.Title)
		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a todo (soft by default, --hard for permanent)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		soft, _ := cmd.Flags().GetBool("soft")
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Delete(args[0], !soft); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: args[0], Actor: currentActor(),
			Action: "delete", Note: boolStr(soft, "soft", "hard"),
		})
		fmt.Printf("Deleted %s (%s)\n", args[0], boolStr(soft, "soft", "hard"))
		return nil
	},
}

func init() {
	deleteCmd.Flags().Bool("soft", true, "Soft delete (mark as cancelled)")
}

// ── dep ─────────────────────────────────────────────────────────────────────

var depCmd = &cobra.Command{
	Use:   "dep",
	Short: "Manage dependencies between todos",
}

var depAddCmd = &cobra.Command{
	Use:   "add <child> <parent>",
	Short: "Add a dependency (child depends on parent)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dtype, _ := cmd.Flags().GetString("type")
		if !DepType(dtype).Valid() {
			return fmt.Errorf("invalid type: %q (use blocks|parent-child|related|discovered-from|duplicates|supersedes)", dtype)
		}
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		dep := Dependency{From: args[0], To: args[1], Type: DepType(dtype)}
		if err := store.AddDep(dep); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: args[0], Actor: currentActor(), Action: "dep:add",
			Note: fmt.Sprintf("%s -> %s (%s)", args[0], args[1], dtype),
		})
		if child, err := store.Get(args[0]); err == nil && child != nil {
			fireHooks(store, EventPostDepAdd, child, args[1], dtype, "")
		}
		fmt.Printf("Added %s -> %s (%s)\n", args[0], args[1], dtype)
		return nil
	},
}

var depRemoveCmd = &cobra.Command{
	Use:   "remove <child> <parent>",
	Short: "Remove a dependency",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.RemoveDep(args[0], args[1]); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: args[0], Actor: currentActor(), Action: "dep:remove",
			Note: fmt.Sprintf("%s -> %s", args[0], args[1]),
		})
		fmt.Printf("Removed dep %s -> %s\n", args[0], args[1])
		return nil
	},
}

func init() {
	depAddCmd.Flags().String("type", "blocks", "Dep type: blocks|parent-child|related|discovered-from|duplicates|supersedes")
	depCmd.AddCommand(depAddCmd)
	depCmd.AddCommand(depRemoveCmd)
}

// ── deps ────────────────────────────────────────────────────────────────────

var depsCmd = &cobra.Command{
	Use:   "deps <id>",
	Short: "Show dependency tree of a todo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		maxDepth, _ := cmd.Flags().GetInt("depth")
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		tree, err := store.DependencyTree(args[0], maxDepth)
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSON(tree)
		}
		fmt.Printf("Dependency tree for %s (depth %d):\n", args[0], maxDepth)
		seen := map[string]bool{}
		var print func(string, string, int)
		print = func(id, prefix string, depth int) {
			if seen[id] || depth > maxDepth {
				return
			}
			seen[id] = true
			t, _ := store.Get(id)
			title := id
			if t != nil {
				title = id + ": " + t.Title
			}
			fmt.Println(prefix + title)
			for _, d := range tree[id] {
				print(d.To, prefix+"  └─ ", depth+1)
			}
		}
		print(args[0], "", 0)
		return nil
	},
}

func init() {
	depsCmd.Flags().Int("depth", 5, "Max traversal depth")
}

// ── ready / blocked / search ────────────────────────────────────────────────

var readyCmd = &cobra.Command{
	Use:   "ready",
	Short: "List unblocked open work (P0 first)",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.Ready()
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSON(ts)
		}
		printTodoTable(ts)
		return nil
	},
}

var blockedCmd = &cobra.Command{
	Use:   "blocked",
	Short: "List blocked work",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.Blocked()
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSON(ts)
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
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.Search(args[0])
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSON(ts)
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
		store, err := openStore()
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
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		st, err := store.ComputeStats()
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSON(st)
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
		store, err := openStore()
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
			return printJSON(entries)
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

// ── mine ────────────────────────────────────────────────────────────────────

var mineCmd = &cobra.Command{
	Use:   "mine",
	Short: "List todos assigned to current user",
	RunE: func(cmd *cobra.Command, args []string) error {
		actor := currentActor()
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		ts, err := store.Mine(actor)
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSON(ts)
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
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if len(args) == 0 {
			p, _ := store.GetMeta("current_project")
			if p == "" {
				p = currentProject()
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
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		m := &Memory{
			Insight: args[0],
			Actor:   currentActor(),
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
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		ready, _ := store.Ready()
		blocked, _ := store.Blocked()
		mine, _ := store.Mine(currentActor())
		fmt.Println("# sin-code todo context")
		fmt.Printf("Project: %s\n", currentProject())
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
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		res, err := store.Compact(CompactOptions{OlderThan: dur, DryRun: dry})
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSON(res)
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
		store, err := openStore()
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
		store, err := openStore()
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
			return printJSON(report)
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
		store, err := openStore()
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
			data, _ = json.MarshalIndent(ts, "", "  ")
		case "jsonl":
			var b strings.Builder
			for _, t := range ts {
				line, _ := json.Marshal(t)
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
			return os.WriteFile(output, data, 0644)
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
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		store, err := openStore()
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
			return printJSON(map[string]int{"imported": imported})
		}
		fmt.Printf("Imported %d todos\n", imported)
		return nil
	},
}

func init() {
	importCmd.Flags().String("format", "json", "Import format: json|jsonl")
}

// ── helpers ─────────────────────────────────────────────────────────────────

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func boolStr(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}

func notify(nt notifications.Type, todoID, title, message, actor string) {
	_ = notifications.Dispatch(&notifications.Notification{
		Type:    nt,
		TodoID:  todoID,
		Title:   title,
		Message: message,
		Actor:   actor,
	})
}

var hookConfigOnce sync.Once
var hookConfig     *HookConfig

func getHookConfig() *HookConfig {
	hookConfigOnce.Do(func() {
		hc, err := LoadHooksConfig("")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load hooks: %v\n", err)
			hc = &HookConfig{Hooks: map[HookEvent][]Hook{}}
		}
		hookConfig = hc
	})
	return hookConfig
}

func fireHooks(store *Store, event HookEvent, t *Todo, from, to, note string) {
	hc := getHookConfig()
	if hc == nil {
		return
	}
	ctx := HookContext{Event: event, Todo: t, From: from, To: to, Note: note, Actor: currentActor()}
	results := hc.Fire(ctx)
	for _, r := range results {
		if r.Err == nil {
			continue
		}
		switch r.Hook.OnError {
		case "fail":
			fmt.Fprintf(os.Stderr, "hook failed: event=%s cmd=%q err=%v\n", event, r.Hook.Command, r.Err)
		case "warn", "":
			fmt.Fprintf(os.Stderr, "hook warning: event=%s cmd=%q err=%v\n", event, r.Hook.Command, r.Err)
		case "ignore":
		}
	}
	firePluginHooks(store, event, t, from, to, note)
}
