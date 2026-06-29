// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second helper-related function is needed, merge into a shared file
package todo

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/notifications"

	"github.com/spf13/cobra"
)

var (
	openStoreFn       = openStore
	currentActorFn    = currentActor
	currentProjectFn  = currentProject
	printJSONFn       = printJSON
	notifyFn          = notify
	getHookConfigFn   = getHookConfig
	fireHooksFn       = fireHooks
	firePluginHooksFn = firePluginHooks
	gitUserNameFn     = func() ([]byte, error) { return exec.Command("git", "config", "user.name").Output() }
	// sin-debt: delete, upgrade: remove when test no longer needs this override
	osUserConfigDirTodo             = os.UserConfigDir
	osGetwdTodo                     = os.Getwd
	osReadFileTodo                  = os.ReadFile
	osWriteFileTodo                 = os.WriteFile
	jsonMarshalIndentTodo           = json.MarshalIndent
	jsonMarshalTodo                 = json.Marshal
	osStdoutTodo          io.Writer = os.Stdout
)

var (
	todoDBPath  string
	todoProject string
	todoFormat  string
	todoAs      string
)

func openStore() (*Store, error) {
	return Open(todoDBPath)
}

func currentActor() string {
	if todoAs != "" {
		return todoAs
	}
	out, err := gitUserNameFn()
	if err == nil {
		name := strings.TrimSpace(string(out))
		if name != "" {
			return name
		}
	}
	if u, err := osUserConfigDirTodo(); err == nil {
		return filepath.Base(u)
	}
	return "unknown"
}

func currentProject() string {
	if todoProject != "" {
		return todoProject
	}
	cwd, err := osGetwdTodo()
	if err != nil {
		return ""
	}
	return filepath.Base(cwd)
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(osStdoutTodo)
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
var hookConfig *HookConfig

var loadHookConfigFn = LoadHooksConfig

func getHookConfig() *HookConfig {
	hookConfigOnce.Do(func() {
		hc, err := loadHookConfigFn("")
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
	ctx := HookContext{Event: event, Todo: t, From: from, To: to, Note: note, Actor: currentActorFn()}
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
	firePluginHooksFn(store, event, t, from, to, note)
}

var TodoCmd = &cobra.Command{
	Use:          "todo",
	Short:        "Issue tracker with dependencies, audit log, and project namespaces",
	Long:         "todo is the SIN-Code issue tracker, matching the UX of `bd` and opencode's todo system. Backed by bbolt for durability, append-only audit log for history, and project namespaces for multi-repo work.\n\nCommon workflows:\n  sin-code todo add --title \"...\" --priority P0 --type feature\n  sin-code todo ready\n  sin-code todo dep add st-1234 st-5678 --type blocks\n  sin-code todo compact --older-than 30d",
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

var _ = os.Getenv
