// SPDX-License-Identifier: MIT
// Purpose: cobra CLI for sin-code notifications: list/read/dismiss/listen/clear/stats/prune.
package notifications

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	notifDBPath   string
	notifFormat   string
	notifWebhook string
	notifTTL      time.Duration
	notifNoMac    bool
	notifNoStderr bool
)

var NotificationsCmd = &cobra.Command{
	Use:   "notifications",
	Short: "Manage sin-code todo notifications",
	Long: `System notifications for sin-code todo events.

  list [--unread]            List recent notifications
  read <id>                   Mark a notification as read
  unread <id>                 Mark a notification as unread
  dismiss <id>                Dismiss a notification
  listen                      Stream notifications as JSONL (Ctrl-C to stop)
  clear                       Delete all notifications
  prune [--older-than 168h]   Remove old/dismissed notifications
  stats                       Counts by type and read state`,
	SilenceUsage: true,
}

func init() {
	NotificationsCmd.PersistentFlags().StringVar(&notifDBPath, "db", "", "Path to bbolt DB (default ~/.config/sin-code/notifications.db)")
	NotificationsCmd.PersistentFlags().StringVar(&notifFormat, "format", "text", "Output format: text|json")
	NotificationsCmd.PersistentFlags().StringVar(&notifWebhook, "webhook", "", "POST URL for webhook delivery")
	NotificationsCmd.PersistentFlags().BoolVar(&notifNoMac, "no-macos", false, "Disable macOS notification")
	NotificationsCmd.PersistentFlags().BoolVar(&notifNoStderr, "no-stderr", false, "Disable stderr output")

	NotificationsCmd.AddCommand(listCmd)
	NotificationsCmd.AddCommand(readCmd)
	NotificationsCmd.AddCommand(unreadCmd)
	NotificationsCmd.AddCommand(dismissCmd)
	NotificationsCmd.AddCommand(listenCmd)
	NotificationsCmd.AddCommand(clearCmd)
	NotificationsCmd.AddCommand(pruneCmd)
	NotificationsCmd.AddCommand(statsCmd)
}

func openStore() (*Store, error) {
	return Open(notifDBPath)
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printNotifList(ns []*Notification) {
	if len(ns) == 0 {
		fmt.Println("(no notifications)")
		return
	}
	fmt.Printf("%-12s %-22s %-6s %s\n", "ID", "TYPE", "READ", "TITLE")
	fmt.Println(strings.Repeat("─", 80))
	for _, n := range ns {
		rd := "•"
		if n.Read {
			rd = "✓"
		}
		fmt.Printf("%-12s %-22s %-6s %s\n", n.ID, string(n.Type), rd, n.Title)
	}
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List notifications",
	RunE: func(cmd *cobra.Command, args []string) error {
		unread, _ := cmd.Flags().GetBool("unread")
		limit, _ := cmd.Flags().GetInt("limit")
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		ns, err := store.List(ListFilter{Unread: unread, NotDismissed: true}, limit)
		if err != nil {
			return err
		}
		if notifFormat == "json" {
			return printJSON(ns)
		}
		printNotifList(ns)
		return nil
	},
}

func init() {
	listCmd.Flags().Bool("unread", false, "Only show unread")
	listCmd.Flags().Int("limit", 0, "Max items (0 = all)")
}

var readCmd = &cobra.Command{
	Use:   "read <id>",
	Short: "Mark a notification as read",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.MarkRead(args[0]); err != nil {
			return err
		}
		fmt.Printf("Read %s\n", args[0])
		return nil
	},
}

var unreadCmd = &cobra.Command{
	Use:   "unread <id>",
	Short: "Mark a notification as unread",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.MarkUnread(args[0]); err != nil {
			return err
		}
		fmt.Printf("Unread %s\n", args[0])
		return nil
	},
}

var dismissCmd = &cobra.Command{
	Use:   "dismiss <id>",
	Short: "Dismiss a notification",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Dismiss(args[0]); err != nil {
			return err
		}
		fmt.Printf("Dismissed %s\n", args[0])
		return nil
	},
}

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Stream notifications as JSONL (Ctrl-C to stop)",
	RunE: func(cmd *cobra.Command, args []string) error {
		ch := TUIBroadcaster()
		enc := json.NewEncoder(os.Stdout)
		for n := range ch {
			if err := enc.Encode(n); err != nil {
				return err
			}
		}
		return nil
	},
}

var clearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Delete all notifications",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		if err := store.Clear(); err != nil {
			return err
		}
		fmt.Println("Cleared all notifications")
		return nil
	},
}

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Remove old/dismissed notifications",
	RunE: func(cmd *cobra.Command, args []string) error {
		ttl, _ := cmd.Flags().GetDuration("older-than")
		store, err := openStore()
		if err != nil {
			return err
		}
		defer store.Close()
		n, err := store.Prune(ttl)
		if err != nil {
			return err
		}
		fmt.Printf("Pruned %d notifications\n", n)
		return nil
	},
}

func init() {
	pruneCmd.Flags().Duration("older-than", 7*24*time.Hour, "Remove notifications older than this")
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Counts by type and read state",
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
		if notifFormat == "json" {
			return printJSON(st)
		}
		fmt.Printf("Total:  %d\n", st.Total)
		fmt.Printf("Unread: %d\n", st.Unread)
		if len(st.ByType) > 0 {
			fmt.Println("\nBy type:")
			for t, c := range st.ByType {
				fmt.Printf("  %-22s %d\n", t, c)
			}
		}
		return nil
	},
}

// Dispatch is a convenience wrapper used by other packages (e.g. todo).
func Dispatch(n *Notification) error {
	store, err := Open(notifDBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	d := NewDispatcher(store)
	if notifWebhook != "" {
		d.WebhookURL = notifWebhook
	}
	if notifNoMac {
		d.MacOS = false
	}
	if notifNoStderr {
		d.Stderr = false
	}
	return d.Send(n)
}
