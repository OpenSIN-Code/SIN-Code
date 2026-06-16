// SPDX-License-Identifier: MIT
// Purpose: `sin-code tokens` — query the LLM token-usage ledger
// (`internal/usage`, issue #168). Subcommands: `show` for current / named
// session or lifetime aggregations, `tail` for recent events, `aggregate`
// for grouped roll-ups. Never renders a fake number (caveman lesson:
// absent until the first call).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/usage"
)

// NewTokensCmd builds the `tokens` cobra subcommand.
func NewTokensCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tokens",
		Short: "Inspect LLM token usage (per session, day, lifetime, model)",
		Long: `sin-code tokens reads the local token-usage ledger at
$XDG_DATA_HOME/sin-code/tokens.db (see AGENTS.md §7). Each row is one
LLM call captured via internal/llm (issue #168).

  tokens show [--session ID] [--today] [--month] [--cost] [--share]
  tokens tail [--session ID] [-n 20]
  tokens aggregate [--by day|month|model|source|session] [--json]

Cost is USD per 1k tokens, pulled from internal/usage.DefaultPricing and
overlaid by ` + "`llm.pricing_per_1k`" + ` from the user config.`,
	}
	cmd.AddCommand(newTokensShowCmd())
	cmd.AddCommand(newTokensTailCmd())
	cmd.AddCommand(newTokensAggregateCmd())
	return cmd
}

func openUsageStoreOrFail(cmd *cobra.Command) (*usage.Store, error) {
	path := usage.DefaultPath()
	store, err := usage.OpenWithPricing(path, loadPricingOverrides())
	if err != nil {
		return nil, fmt.Errorf("open tokens db at %s: %w (has sin-code recorded any LLM calls yet?)", path, err)
	}
	return store, nil
}

// loadPricingOverrides reads `llm.pricing_per_1k.KEY = USD` from
// ~/.config/sin/sin-code.toml (or the project override). Keys use the
// syntax `llm.pricing_per_1k."org/model"`. Empty / missing map is fine;
// best-effort: parser errors are swallowed, falls back to defaults.
func loadPricingOverrides() map[string]float64 {
	usersPath, projectsPath := tokensConfigPaths()
	merged := map[string]float64{}
	for _, p := range []string{usersPath, projectsPath} {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.HasPrefix(line, "llm.pricing_per_1k.") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimPrefix(strings.TrimSpace(parts[0]), "llm.pricing_per_1k.")
			key = strings.Trim(key, `"`)
			val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
			if v, err := strconv.ParseFloat(val, 64); err == nil && key != "" {
				merged[key] = v
			}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

// tokensConfigPaths mirrors internal/config.configDir / projectConfigPath.
// Duplicated here because the `internal` package's config helpers are
// unexported and tokens_cmd.go lives in `package main`. Keep in sync with
// cmd/sin-code/internal/config.go.
func tokensConfigPaths() (userPath, projectPath string) {
	if home, err := os.UserHomeDir(); err == nil {
		userPath = filepath.Join(home, ".config", "sin", "sin-code.toml")
	}
	projectPath = filepath.Join(".", ".sin-code", "config.toml")
	return
}

func newTokensShowCmd() *cobra.Command {
	var sessionID string
	var today, month, lifetime, costFlag, share, jsonOut bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show token usage for a session, today, the current month, or lifetime",
		Long: `Prints prompt + completion + total tokens, USD cost, and per-model
breakdown. Default scope is lifetime (all sessions to date). Pass
--session <id>, --today, or --month to narrow. Combine --cost to include
USD and --share for a single-line tweetable summary.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			store, err := openUsageStoreOrFail(nil)
			if err != nil {
				return err
			}
			defer store.Close()

			f := buildShowFilter(sessionID, today, month, lifetime)
			top, _, err := store.Aggregate(context.Background(), f, "")
			if err != nil {
				return err
			}
			if share {
				fmt.Fprintln(out, renderShareLine(top, costFlag))
				return nil
			}
			if jsonOut {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(top)
			}
			renderTable(out, top, f, costFlag)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Specific session ID (default: aggregate everything)")
	cmd.Flags().BoolVar(&today, "today", false, "Today's usage only")
	cmd.Flags().BoolVar(&month, "month", false, "Current calendar month")
	cmd.Flags().BoolVar(&lifetime, "lifetime", true, "All recorded sessions (default)")
	cmd.Flags().BoolVar(&costFlag, "cost", true, "Include USD cost estimate (default true; pass --cost=false to suppress)")
	cmd.Flags().BoolVar(&share, "share", false, "Single-line tweetable summary")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func buildShowFilter(sessionID string, today, month, lifetime bool) usage.Filter {
	f := usage.Filter{}
	switch {
	case sessionID != "":
		f.SessionID = sessionID
	case today:
		f.Since = startOfDay(time.Now())
		f.Until = f.Since.Add(24 * time.Hour)
	case month:
		f.Since = startOfMonth(time.Now())
		f.Until = time.Date(f.Since.Year(), f.Since.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	}
	_ = lifetime // lifetime is the default (no filter); kept for symmetry
	return f
}

func newTokensTailCmd() *cobra.Command {
	var sessionID string
	var n int
	cmd := &cobra.Command{
		Use:          "tail",
		Short:        "Show the most recent N token-usage events (default 20)",
		Long:         "Newest-first list of recorded LLM calls. Useful for debugging recent spend.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			store, err := openUsageStoreOrFail(nil)
			if err != nil {
				return err
			}
			defer store.Close()

			events, err := store.Tail(context.Background(),
				usage.Filter{SessionID: sessionID}, n)
			if err != nil {
				return err
			}
			if len(events) == 0 {
				fmt.Fprintln(out, "no recorded token events (yet)")
				return nil
			}
			renderEventTable(out, events)
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionID, "session", "", "Optional session ID filter")
	cmd.Flags().IntVarP(&n, "count", "n", 20, "Number of events to show")
	return cmd
}

func newTokensAggregateCmd() *cobra.Command {
	var by string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:          "aggregate",
		Short:        "Aggregate token usage grouped by day|month|model|source|session",
		Long:         "Returns the top-level totals plus per-group rows.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			store, err := openUsageStoreOrFail(nil)
			if err != nil {
				return err
			}
			defer store.Close()

			top, subs, err := store.Aggregate(context.Background(), usage.Filter{}, by)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(struct {
					Total     usage.Aggregation   `json:"total"`
					Subgroups []usage.Aggregation `json:"subgroups"`
				}{Total: *top, Subgroups: subs})
			}
			fmt.Fprintln(out, "== totals ==")
			renderTable(out, top, usage.Filter{}, true)
			if len(subs) > 0 {
				fmt.Fprintf(out, "\n== grouped by %s (%d rows) ==\n", by, len(subs))
				renderGroupedTable(out, subs)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&by, "by", "day", "Group by: day|month|model|source|session")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

// ─── renderers ────────────────────────────────────────────────────────────

func renderTable(w io.Writer, a *usage.Aggregation, f usage.Filter, withCost bool) {
	scope := "lifetime"
	if f.SessionID != "" {
		scope = "session=" + f.SessionID
	} else if !f.Since.IsZero() {
		scope = "since=" + f.Since.Format("2006-01-02")
	}
	fmt.Fprintf(w, "Scope: %s\n", scope)
	fmt.Fprintf(w, "Sessions recorded: %d\n", a.SessionsCount)
	fmt.Fprintf(w, "Events:            %d\n", a.EventCount)
	fmt.Fprintf(w, "Prompt tokens:     %s\n", humanTokens(a.InputTokens))
	fmt.Fprintf(w, "Completion tokens: %s\n", humanTokens(a.OutputTokens))
	fmt.Fprintf(w, "Total tokens:      %s\n", humanTokens(a.TotalTokens))
	if withCost {
		fmt.Fprintf(w, "Estimated cost:    $%.4f\n", a.CostUSD)
	}
	if !a.FirstEvent.IsZero() {
		fmt.Fprintf(w, "First event:       %s\n", a.FirstEvent.Format(time.RFC3339))
	}
	if !a.LastEvent.IsZero() {
		fmt.Fprintf(w, "Last event:        %s\n", a.LastEvent.Format(time.RFC3339))
	}
	if len(a.ByModel) > 0 {
		fmt.Fprintln(w, "\nBy model (sorted desc):")
		keys := sortedKeys(a.ByModel)
		for _, k := range keys {
			fmt.Fprintf(w, "  %-48s  %10s\n", shorten(k, 48), humanTokens(a.ByModel[k]))
		}
	}
	if len(a.BySource) > 0 {
		fmt.Fprintln(w, "\nBy source:")
		keys := sortedKeys(a.BySource)
		for _, k := range keys {
			fmt.Fprintf(w, "  %-16s  %10s\n", k, humanTokens(a.BySource[k]))
		}
	}
}

func renderGroupedTable(w io.Writer, rows []usage.Aggregation) {
	maxKey := 12
	for _, r := range rows {
		if w := len(r.Group); w > maxKey {
			maxKey = w
		}
	}
	if maxKey > 40 {
		maxKey = 40
	}
	fmt.Fprintf(w, "  %-*s  %10s  %10s  %12s  %8s\n", maxKey, "group", "input", "output", "total", "events")
	for _, r := range rows {
		key := r.Group
		if len(key) > maxKey {
			key = key[:maxKey-1] + "…"
		}
		fmt.Fprintf(w, "  %-*s  %10s  %10s  %12s  %8d\n",
			maxKey, key,
			humanTokens(r.InputTokens), humanTokens(r.OutputTokens),
			humanTokens(r.TotalTokens), r.EventCount)
	}
}

func renderEventTable(w io.Writer, events []usage.Event) {
	fmt.Fprintf(w, "  %-22s  %-20s  %-48s  %8s  %8s  %8s  %s\n",
		"created_at", "source", "model", "input", "output", "total", "cost")
	for _, e := range events {
		fmt.Fprintf(w, "  %-22s  %-20s  %-48s  %8d  %8d  %8d  $%.4f\n",
			e.CreatedAt.Format("2006-01-02 15:04:05"),
			string(e.Source),
			shorten(e.Model, 48),
			e.InputTokens, e.OutputTokens, e.TotalTokens,
			e.CostUSD)
	}
}

// renderShareLine produces the tweetable one-liner used by caveman's
// --share. Format: "sin-code ⛏ 12.4k · $1.23 (12 events, 3 sessions)"
func renderShareLine(a *usage.Aggregation, _ bool) string {
	if a == nil || (a.TotalTokens == 0 && a.EventCount == 0) {
		return "sin-code ⛏ 0 (no usage recorded yet)"
	}
	return fmt.Sprintf("sin-code ⛏ %s · $%.2f (%d events, %d sessions)",
		humanTokens(a.TotalTokens), a.CostUSD, a.EventCount, a.SessionsCount)
}

func humanTokens(n int) string {
	abs := n
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= 1_000_000:
		return fmt.Sprintf("%d (%.2fM)", n, float64(abs)/1_000_000.0)
	case abs >= 1_000:
		return fmt.Sprintf("%d (%.2fk)", n, float64(abs)/1_000.0)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k, v := range m {
		if v > 0 {
			keys = append(keys, k)
		}
	}
	// sort by value desc, then key asc.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && (m[keys[j]] > m[keys[j-1]] ||
			(m[keys[j]] == m[keys[j-1]] && keys[j] < keys[j-1])); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

func shorten(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func startOfDay(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func startOfMonth(t time.Time) time.Time {
	t = t.Local()
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// _ keeps strings import used (errors is also kept for future use).
var _ = errors.New
var _ = strings.TrimSpace
