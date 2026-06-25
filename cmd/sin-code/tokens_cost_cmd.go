// SPDX-License-Identifier: MIT
// Purpose: `sin-code tokens cost` — cost projection and budget alerts.
// Reads the token-usage ledger (issue #168) and the user/project config
// for `tokens.budget_monthly_usd`, then renders a spend summary with
// end-of-month projection and traffic-light budget status.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/usage"
)

// budgetAlertLevel is the traffic-light status for budget consumption.
type budgetAlertLevel string

const (
	budgetGreen    budgetAlertLevel = "green"    // under 50% of budget
	budgetYellow   budgetAlertLevel = "yellow"   // 50–80% of budget
	budgetRed      budgetAlertLevel = "red"      // over 80% of budget
	budgetCritical budgetAlertLevel = "critical" // over budget (100%+)
	budgetNone     budgetAlertLevel = ""         // no budget configured
)

// costReport is the structured result assembled by computeCostReport.
// It is the single source for both text and JSON renderers so the
// --json output is a faithful superset of the human-readable view.
type costReport struct {
	TotalSpendUSD float64          `json:"total_spend_usd"`
	TodaySpendUSD float64          `json:"today_spend_usd"`
	MonthSpendUSD float64          `json:"month_spend_usd"`
	ProjectionUSD float64          `json:"projection_usd"`
	BudgetUSD     float64          `json:"budget_usd,omitempty"`
	BudgetUsedPct float64          `json:"budget_used_pct,omitempty"`
	BudgetAlert   budgetAlertLevel `json:"budget_alert,omitempty"`
	ByModel       []costModelRow   `json:"by_model"`
	TopSessions   []costSessionRow `json:"top_sessions"`
	GeneratedAt   time.Time        `json:"generated_at"`
	WindowDays    int              `json:"window_days"`
	DaysRemaining int              `json:"days_remaining"`
	HasProjection bool             `json:"has_projection"`
}

type costModelRow struct {
	Model       string  `json:"model"`
	CostUSD     float64 `json:"cost_usd"`
	TotalTokens int     `json:"total_tokens"`
	EventCount  int     `json:"event_count"`
}

type costSessionRow struct {
	SessionID   string    `json:"session_id"`
	CostUSD     float64   `json:"cost_usd"`
	TotalTokens int       `json:"total_tokens"`
	EventCount  int       `json:"event_count"`
	FirstEvent  time.Time `json:"first_event"`
	LastEvent   time.Time `json:"last_event"`
}

// newTokensCostCmd builds the `tokens cost` cobra subcommand.
func newTokensCostCmd() *cobra.Command {
	var jsonOut bool
	var modelFilter string
	var budgetOverride float64
	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Cost projection and budget alerts from the token-usage ledger",
		Long: `Shows current USD spend (total, today, this month), an
end-of-month projection based on the trailing 7-day average, optional
budget alerts (traffic-light), a per-model cost breakdown, and the top
5 most expensive sessions.

Budget is read from ` + "`tokens.budget_monthly_usd`" + ` in the user or project
config. Override for this run with --budget <usd>.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			store, err := openUsageStoreOrFail(nil)
			if err != nil {
				return err
			}
			defer store.Close()

			budget := budgetOverride
			if budget <= 0 {
				budget = loadBudgetConfig()
			}

			report, err := computeCostReport(context.Background(), store, budget, modelFilter)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			renderCostReport(out, report)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	cmd.Flags().StringVar(&modelFilter, "model", "", "Filter by model name")
	cmd.Flags().Float64Var(&budgetOverride, "budget", 0, "Override monthly budget (USD) for this run")
	return cmd
}

// loadBudgetConfig reads `tokens.budget_monthly_usd` from the user and
// project config files (same line-based parser as loadPricingOverrides).
// Returns 0 when unset or unparseable.
func loadBudgetConfig() float64 {
	userPath, projectPath := tokensConfigPaths()
	for _, p := range []string{projectPath, userPath} {
		if p == "" {
			continue
		}
		data, err := readConfigFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if !strings.HasPrefix(line, "tokens.budget_monthly_usd") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			val := strings.Trim(strings.TrimSpace(parts[1]), `"`)
			if v, err := strconv.ParseFloat(val, 64); err == nil && v > 0 {
				return v
			}
		}
	}
	return 0
}

// computeCostReport assembles the full cost report from the store. It
// performs four aggregate queries (total, today, month, trailing-7-day)
// plus two grouped queries (by model, by session). All queries honour
// the optional model filter.
func computeCostReport(ctx context.Context, store *usage.Store, budget float64, modelFilter string) (*costReport, error) {
	now := time.Now().UTC()
	monthStart := startOfMonth(now)
	nextMonth := time.Date(monthStart.Year(), monthStart.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	todayStart := startOfDay(now)
	tomorrow := todayStart.Add(24 * time.Hour)
	sevenDaysAgo := todayStart.AddDate(0, 0, -7)

	modelF := usage.Filter{Model: modelFilter}

	// 1. Total spend (lifetime, optionally filtered by model).
	totalTop, _, err := store.Aggregate(ctx, modelF, "")
	if err != nil {
		return nil, fmt.Errorf("aggregate total: %w", err)
	}

	// 2. Today's spend.
	todayF := modelF
	todayF.Since = todayStart
	todayF.Until = tomorrow
	todayTop, _, err := store.Aggregate(ctx, todayF, "")
	if err != nil {
		return nil, fmt.Errorf("aggregate today: %w", err)
	}

	// 3. This month's spend.
	monthF := modelF
	monthF.Since = monthStart
	monthF.Until = nextMonth
	monthTop, _, err := store.Aggregate(ctx, monthF, "")
	if err != nil {
		return nil, fmt.Errorf("aggregate month: %w", err)
	}

	// 4. Trailing 7-day daily average for projection.
	trailF := modelF
	trailF.Since = sevenDaysAgo
	trailF.Until = tomorrow
	_, trailSubs, err := store.Aggregate(ctx, trailF, "day")
	if err != nil {
		return nil, fmt.Errorf("aggregate trailing 7d: %w", err)
	}

	// 5. By-model breakdown (all time, filtered).
	_, modelSubs, err := store.Aggregate(ctx, modelF, "model")
	if err != nil {
		return nil, fmt.Errorf("aggregate by model: %w", err)
	}

	// 6. By-session (all time, filtered) — we sort by cost desc and take top 5.
	_, sessionSubs, err := store.Aggregate(ctx, modelF, "session")
	if err != nil {
		return nil, fmt.Errorf("aggregate by session: %w", err)
	}

	report := &costReport{
		TotalSpendUSD: totalTop.CostUSD,
		TodaySpendUSD: todayTop.CostUSD,
		MonthSpendUSD: monthTop.CostUSD,
		BudgetUSD:     budget,
		GeneratedAt:   now,
		WindowDays:    7,
	}

	// Projection: average daily cost over trailing 7 days × days remaining
	// (including today) + current month spend.
	daysRemaining := daysInMonth(now) - now.Day() + 1
	report.DaysRemaining = daysRemaining
	if len(trailSubs) > 0 {
		var trailCost float64
		for _, s := range trailSubs {
			trailCost += s.CostUSD
		}
		avgDaily := trailCost / float64(len(trailSubs))
		report.ProjectionUSD = monthTop.CostUSD + avgDaily*float64(daysRemaining)
		report.HasProjection = true
	}

	// Budget alert.
	if budget > 0 {
		report.BudgetUsedPct = (monthTop.CostUSD / budget) * 100
		report.BudgetAlert = classifyBudget(monthTop.CostUSD, budget)
	}

	// By-model rows.
	report.ByModel = make([]costModelRow, 0, len(modelSubs))
	for _, s := range modelSubs {
		report.ByModel = append(report.ByModel, costModelRow{
			Model:       s.Group,
			CostUSD:     s.CostUSD,
			TotalTokens: s.TotalTokens,
			EventCount:  s.EventCount,
		})
	}
	sort.Slice(report.ByModel, func(i, j int) bool {
		if report.ByModel[i].CostUSD != report.ByModel[j].CostUSD {
			return report.ByModel[i].CostUSD > report.ByModel[j].CostUSD
		}
		return report.ByModel[i].Model < report.ByModel[j].Model
	})

	// Top-5 sessions by cost.
	sessions := make([]costSessionRow, 0, len(sessionSubs))
	for _, s := range sessionSubs {
		sid := s.Group
		if sid == "(no-session)" {
			sid = ""
		}
		sessions = append(sessions, costSessionRow{
			SessionID:   sid,
			CostUSD:     s.CostUSD,
			TotalTokens: s.TotalTokens,
			EventCount:  s.EventCount,
			FirstEvent:  s.FirstEvent,
			LastEvent:   s.LastEvent,
		})
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].CostUSD != sessions[j].CostUSD {
			return sessions[i].CostUSD > sessions[j].CostUSD
		}
		return sessions[i].SessionID < sessions[j].SessionID
	})
	if len(sessions) > 5 {
		sessions = sessions[:5]
	}
	report.TopSessions = sessions

	return report, nil
}

// classifyBudget maps (spend, budget) to a traffic-light level.
func classifyBudget(spend, budget float64) budgetAlertLevel {
	if budget <= 0 {
		return budgetNone
	}
	pct := spend / budget
	switch {
	case pct > 1.0:
		return budgetCritical
	case pct > 0.80:
		return budgetRed
	case pct >= 0.50:
		return budgetYellow
	default:
		return budgetGreen
	}
}

// daysInMonth returns the number of calendar days in the month containing t.
func daysInMonth(t time.Time) int {
	start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	next := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return int(next.Sub(start).Hours() / 24)
}

// renderCostReport writes the human-readable cost report to w.
func renderCostReport(w io.Writer, r *costReport) {
	fmt.Fprintf(w, "╔══════════════════════════════════════════════╗\n")
	fmt.Fprintf(w, "║          COST PROJECTION & BUDGET             ║\n")
	fmt.Fprintf(w, "╚══════════════════════════════════════════════╝\n\n")

	fmt.Fprintf(w, "Spend Summary\n")
	fmt.Fprintf(w, "─────────────────────────────────────────────\n")
	fmt.Fprintf(w, "  Total (lifetime):   $%.4f\n", r.TotalSpendUSD)
	fmt.Fprintf(w, "  Today:              $%.4f\n", r.TodaySpendUSD)
	fmt.Fprintf(w, "  This month:         $%.4f\n", r.MonthSpendUSD)

	if r.HasProjection {
		fmt.Fprintf(w, "  ─────────────────────────────────────────\n")
		fmt.Fprintf(w, "  End-of-month proj:  $%.4f", r.ProjectionUSD)
		fmt.Fprintf(w, "  (based on %d-day avg, %d days remaining)\n", r.WindowDays, r.DaysRemaining)
	}
	fmt.Fprintln(w)

	// Budget section.
	if r.BudgetUSD > 0 {
		fmt.Fprintf(w, "Budget Alert\n")
		fmt.Fprintf(w, "─────────────────────────────────────────────\n")
		fmt.Fprintf(w, "  Monthly budget:     $%.2f\n", r.BudgetUSD)
		fmt.Fprintf(w, "  Used:               %.1f%% ($%.4f of $%.2f)\n",
			r.BudgetUsedPct, r.MonthSpendUSD, r.BudgetUSD)
		fmt.Fprintf(w, "  Status:             %s\n", budgetAlertLabel(r.BudgetAlert))
		fmt.Fprintln(w)
	} else {
		fmt.Fprintf(w, "  (no budget configured — set tokens.budget_monthly_usd in config)\n\n")
	}

	// By-model breakdown.
	if len(r.ByModel) > 0 {
		fmt.Fprintf(w, "Cost by Model\n")
		fmt.Fprintf(w, "─────────────────────────────────────────────\n")
		fmt.Fprintf(w, "  %-40s  %12s  %10s  %8s\n", "model", "cost", "tokens", "events")
		for _, m := range r.ByModel {
			fmt.Fprintf(w, "  %-40s  $%11.4f  %10s  %8d\n",
				shorten(m.Model, 40), m.CostUSD, humanTokens(m.TotalTokens), m.EventCount)
		}
		fmt.Fprintln(w)
	}

	// Top sessions.
	if len(r.TopSessions) > 0 {
		fmt.Fprintf(w, "Top 5 Most Expensive Sessions\n")
		fmt.Fprintf(w, "─────────────────────────────────────────────\n")
		fmt.Fprintf(w, "  %-20s  %12s  %10s  %8s\n", "session", "cost", "tokens", "events")
		for _, s := range r.TopSessions {
			sid := s.SessionID
			if sid == "" {
				sid = "(no-session)"
			}
			fmt.Fprintf(w, "  %-20s  $%11.4f  %10s  %8d\n",
				shorten(sid, 20), s.CostUSD, humanTokens(s.TotalTokens), s.EventCount)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Generated: %s\n", r.GeneratedAt.Format(time.RFC3339))
}

// budgetAlertLabel returns a human-readable label with a visual indicator
// for the traffic-light status.
func budgetAlertLabel(level budgetAlertLevel) string {
	switch level {
	case budgetGreen:
		return "🟢 GREEN — under 50% of budget"
	case budgetYellow:
		return "🟡 YELLOW — 50–80% of budget"
	case budgetRed:
		return "🔴 RED — over 80% of budget"
	case budgetCritical:
		return "🔴 CRITICAL — over budget!"
	default:
		return "— (no budget set)"
	}
}

// readConfigFile is a small wrapper around os.ReadFile so tests can
// override it. Mirrors the pattern in loadPricingOverrides.
var readConfigFile = func(path string) ([]byte, error) {
	return os.ReadFile(path)
}
