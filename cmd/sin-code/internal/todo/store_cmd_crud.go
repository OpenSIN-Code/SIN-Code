// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second crud-related function is needed, merge into a shared file
package todo

import (
	"fmt"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"

	"github.com/spf13/cobra"
)

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
			project = currentProjectFn()
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
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Add(t); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: t.ID, Actor: currentActorFn(), Action: "create",
			To: t.Title,
		})
		fireHooksFn(store, EventPostAdd, t, "", t.Title, "")
		notifyFn(notifications.TypeTodoCreated, t.ID, t.Title,
			fmt.Sprintf("New %s %s: %s", t.Priority, t.Type, t.Title), currentActorFn())
		if todoFormat == "json" {
			return printJSONFn(t)
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
		store, err := openStoreFn()
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
				return printJSONFn(ts)
			}
			printTodoTable(ts)
			return nil
		}
		ts, err := store.ListFiltered(f)
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
		store, err := openStoreFn()
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
			return printJSONFn(map[string]interface{}{
				"todo":    t,
				"deps":    deps,
				"deps_of": rev,
				"audit":   audit,
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
		store, err := openStoreFn()
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
			TodoID: t.ID, Actor: currentActorFn(), Action: "update",
			From: string(old), To: string(t.Status), Note: strings.Join(changes, ","),
		})
		if todoFormat == "json" {
			return printJSONFn(t)
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
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		t, err := store.Get(args[0])
		if err != nil {
			return err
		}
		actor := currentActorFn()
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
		fireHooksFn(store, EventPostClaim, t, old, actor, "")
		fmt.Printf("Claimed %s by %s\n", t.ID, actor)
		return nil
	},
}

var unclaimCmd = &cobra.Command{
	Use:   "unclaim <id>",
	Short: "Release a claim on a todo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStoreFn()
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
			TodoID: t.ID, Actor: currentActorFn(), Action: "unclaim",
			From: old, To: "",
		})
		fmt.Printf("Unclaimed %s (was %s)\n", t.ID, old)
		return nil
	},
}
