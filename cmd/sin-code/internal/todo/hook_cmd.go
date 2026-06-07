// SPDX-License-Identifier: MIT
// Purpose: sin-code todo hook — manage pre/post event shell commands.
package todo

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	hookConfigPath string
	hookCmd        string
	hookEvent      string
	hookTimeout    time.Duration
	hookOnError    string
	hookIndex      int
	hookTestTitle  string
)

var hookCmd_Cobra = &cobra.Command{
	Use:   "hook",
	Short: "Manage pre/post event hooks",
	Long: `Manage shell command hooks for todo events.

Hooks are stored in ~/.config/sin-code/hooks.toml and run synchronously after
each event. Available events: ` + strings.Join(toStringSlice(AllEvents()), ", ") + `

Each hook can set:
  - command: shell command to run (passed to sh -c)
  - timeout: max execution time (default 30s)
  - on_error: ignore | warn | fail (default warn)

Env vars passed to hook:
  SIN_EVENT, SIN_ACTOR, SIN_TODO_ID, SIN_TODO_TITLE, SIN_TODO_STATUS,
  SIN_TODO_PRIORITY, SIN_TODO_TYPE, SIN_TODO_ASSIGNEE, SIN_TODO_TAGS,
  SIN_TODO_PROJECT, SIN_FROM, SIN_TO, SIN_NOTE

Examples:
  sin-code todo hook list
  sin-code todo hook add post_complete --command "open https://github.com/foo/bar/issues/$SIN_TODO_ID"
  sin-code todo hook add pre_add --command "echo creating $SIN_TODO_TITLE" --timeout 5s
  sin-code todo hook test post_complete --title "Demo"`,
	SilenceUsage: true,
}

var hookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured hooks",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := LoadHooksConfig(hookConfigPath)
		if err != nil {
			return err
		}
		if len(cfg.Hooks) == 0 {
			fmt.Println("(no hooks configured)")
			return nil
		}
		fmt.Printf("Config: %s\n\n", cfg.Path())
		for _, event := range AllEvents() {
			hooks := cfg.Get(event)
			if len(hooks) == 0 {
				continue
			}
			fmt.Printf("[%s]\n", event)
			for i, h := range hooks {
				timeout := h.Timeout
				if timeout == 0 {
					timeout = 30 * time.Second
				}
				onError := h.OnError
				if onError == "" {
					onError = "warn"
				}
				fmt.Printf("  %d. %s\n", i, h.Command)
				fmt.Printf("     timeout=%s on_error=%s\n", timeout, onError)
			}
			fmt.Println()
		}
		return nil
	},
}

var hookAddCmd = &cobra.Command{
	Use:   "add <event>",
	Short: "Add a hook for an event",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		event := HookEvent(args[0])
		if !event.Valid() {
			return fmt.Errorf("invalid event: %s (use one of: %s)",
				args[0], strings.Join(toStringSlice(AllEvents()), ", "))
		}
		cfg, err := LoadHooksConfig(hookConfigPath)
		if err != nil {
			return err
		}
		h := Hook{Command: hookCmd, Timeout: hookTimeout, OnError: hookOnError}
		if err := cfg.Add(event, h); err != nil {
			return err
		}
		fmt.Printf("Added hook for %s: %s\n", event, h.Command)
		return nil
	},
}

var hookRemoveCmd = &cobra.Command{
	Use:   "remove <event> --index <n>",
	Short: "Remove a hook by index",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		event := HookEvent(args[0])
		if !event.Valid() {
			return fmt.Errorf("invalid event: %s", args[0])
		}
		cfg, err := LoadHooksConfig(hookConfigPath)
		if err != nil {
			return err
		}
		if err := cfg.Remove(event, hookIndex); err != nil {
			return err
		}
		fmt.Printf("Removed hook %d from %s\n", hookIndex, event)
		return nil
	},
}

var hookTestCmd = &cobra.Command{
	Use:   "test <event>",
	Short: "Test hooks for an event with a synthetic todo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		event := HookEvent(args[0])
		if !event.Valid() {
			return fmt.Errorf("invalid event: %s", args[0])
		}
		cfg, err := LoadHooksConfig(hookConfigPath)
		if err != nil {
			return err
		}
		title := hookTestTitle
		if title == "" {
			title = "Test todo from hook test"
		}
		synth := &Todo{
			ID:       "st-test",
			Title:    title,
			Status:   StatusOpen,
			Priority: PriorityP0,
			Type:     TypeTask,
		}
		ctx := HookContext{Event: event, Todo: synth, Actor: "hook-test", From: "open", To: "done"}
		results := cfg.Fire(ctx)
		if len(results) == 0 {
			fmt.Printf("No hooks configured for %s\n", event)
			return nil
		}
		for i, r := range results {
			fmt.Printf("Hook %d: %s\n", i, r.Hook.Command)
			fmt.Printf("  Elapsed:  %s\n", r.Elapsed)
			if r.Err != nil {
				fmt.Printf("  Error:    %v\n", r.Err)
			} else {
				fmt.Printf("  Status:   OK\n")
			}
			if r.Stdout != "" {
				fmt.Printf("  Stdout:   %s\n", strings.TrimSpace(r.Stdout))
			}
			if r.Stderr != "" {
				fmt.Printf("  Stderr:   %s\n", strings.TrimSpace(r.Stderr))
			}
			fmt.Println()
		}
		return nil
	},
}

var hookPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show the hooks config file path",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := hookConfigPath
		if path == "" {
			path = DefaultHooksPath()
		}
		fmt.Println(path)
		return nil
	},
}

func init() {
	hookCmd_Cobra.PersistentFlags().StringVar(&hookConfigPath, "config", "", "Path to hooks.toml (default ~/.config/sin-code/hooks.toml)")

	hookAddCmd.Flags().StringVar(&hookCmd, "command", "", "Shell command (required)")
	hookAddCmd.Flags().DurationVar(&hookTimeout, "timeout", 30*time.Second, "Max execution time")
	hookAddCmd.Flags().StringVar(&hookOnError, "on-error", "warn", "ignore|warn|fail")

	hookRemoveCmd.Flags().IntVar(&hookIndex, "index", -1, "Hook index to remove (0-based)")

	hookTestCmd.Flags().StringVar(&hookTestTitle, "title", "", "Synthetic todo title")

	hookCmd_Cobra.AddCommand(hookListCmd)
	hookCmd_Cobra.AddCommand(hookAddCmd)
	hookCmd_Cobra.AddCommand(hookRemoveCmd)
	hookCmd_Cobra.AddCommand(hookTestCmd)
	hookCmd_Cobra.AddCommand(hookPathCmd)

	TodoCmd.AddCommand(hookCmd_Cobra)
}

func toStringSlice(es []HookEvent) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = string(e)
	}
	return out
}

var _ = os.Getenv
