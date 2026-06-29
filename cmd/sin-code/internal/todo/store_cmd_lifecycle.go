// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second lifecycle-related function is needed, merge into a shared file
package todo

import (
	"fmt"

	"github.com/spf13/cobra"
)

// ── complete / cancel / delete ──────────────────────────────────────────────

var completeCmd = &cobra.Command{
	Use:   "complete <id>",
	Short: "Mark a todo as done",
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
		t.Status = StatusDone
		if err := store.Update(t); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: t.ID, Actor: currentActorFn(), Action: "complete",
			From: string(old), To: string(t.Status),
		})
		fireHooksFn(store, EventPostComplete, t, string(old), string(t.Status), "")
		fmt.Printf("Completed %s: %s\n", t.ID, t.Title)
		return nil
	},
}

var cancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Mark a todo as cancelled",
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
		t.Status = StatusCancelled
		if err := store.Update(t); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: t.ID, Actor: currentActorFn(), Action: "cancel",
			From: string(old), To: string(t.Status),
		})
		fireHooksFn(store, EventPostCancel, t, string(old), string(t.Status), "")
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
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Delete(args[0], !soft); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: args[0], Actor: currentActorFn(),
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
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		dep := Dependency{From: args[0], To: args[1], Type: DepType(dtype)}
		if err := store.AddDep(dep); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: args[0], Actor: currentActorFn(), Action: "dep:add",
			Note: fmt.Sprintf("%s -> %s (%s)", args[0], args[1], dtype),
		})
		if child, err := store.Get(args[0]); err == nil && child != nil {
			fireHooksFn(store, EventPostDepAdd, child, args[1], dtype, "")
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
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.RemoveDep(args[0], args[1]); err != nil {
			return err
		}
		_ = store.AppendAudit(AuditEntry{
			TodoID: args[0], Actor: currentActorFn(), Action: "dep:remove",
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
		store, err := openStoreFn()
		if err != nil {
			return err
		}
		defer store.Close()
		tree, err := store.DependencyTree(args[0], maxDepth)
		if err != nil {
			return err
		}
		if todoFormat == "json" {
			return printJSONFn(tree)
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
