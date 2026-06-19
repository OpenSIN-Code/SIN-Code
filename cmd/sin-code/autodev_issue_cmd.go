// SPDX-License-Identifier: MIT
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/goalcontract"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/spf13/cobra"
)

type issuePayload struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	State  string `json:"state"`
	URL    string `json:"url"`
}

func fetchIssue(ctx context.Context, repo string, number int) (*issuePayload, error) {
	b := ghbridge.New()
	out, _, err := b.Execute(ctx, []string{"issue", "view", fmt.Sprintf("%d", number), "--repo", repo, "--json", "number,title,body,state,url"})
	if err != nil {
		return nil, fmt.Errorf("gh issue view: %w", err)
	}
	var issue issuePayload
	if err := json.Unmarshal([]byte(out), &issue); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return &issue, nil
}

func issueToPrompt(issue *issuePayload) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Resolve GitHub issue #%d: %s\n\nURL: %s\n\n%s\n\nInstructions: implement a fix, write tests, update docs, ensure build passes.", issue.Number, issue.Title, issue.URL, issue.Body)
	return b.String()
}

func defaultIssueContract(issue *issuePayload) *goalcontract.GoalContract {
	return &goalcontract.GoalContract{
		DeterministicChecks: []orchestrator.Check{
			{Kind: orchestrator.CheckBuild, Name: "build", Cmd: []string{"go", "build", "./..."}},
			{Kind: orchestrator.CheckTest, Name: "test", Cmd: []string{"go", "test", "./...", "-race", "-count=1"}},
		},
		SemanticCriteria: []string{fmt.Sprintf("Addresses issue #%d", issue.Number), "Tests exercise new behavior", "No debug scaffolding"},
	}
}

func newGoalAddFromIssueCmd() *cobra.Command {
	var priority, retries int
	var repo string
	var extraCriteria []string
	cmd := &cobra.Command{
		Use: "add-from-issue <number>", Short: "Read a GitHub issue and enqueue it as an autonomous goal (Autodev, issue #391)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			num, err := fmtAtoi(args[0])
			if err != nil {
				return fmt.Errorf("issue number must be positive: %w", err)
			}
			if repo == "" {
				repo = detectRepo()
				if repo == "" {
					return fmt.Errorf("--repo not specified")
				}
			}
			ctx := cmd.Context()
			issue, err := fetchIssue(ctx, repo, num)
			if err != nil {
				return err
			}
			if issue.State != "open" {
				return fmt.Errorf("issue #%d is %s", num, issue.State)
			}
			contract := defaultIssueContract(issue)
			contract.SemanticCriteria = append(contract.SemanticCriteria, extraCriteria...)
			contractJSON, _ := contract.Marshal()
			q, err := autonomy.Open(autonomy.DefaultPath())
			if err != nil {
				return err
			}
			defer q.Close()
			ws, _ := os.Getwd()
			id, err := q.AddWithContract(ctx, issueToPrompt(issue), ws, priority, retries, contractJSON)
			if err != nil {
				return err
			}
			fmt.Printf("goal %d enqueued from issue #%d\n", id, num)
			return nil
		},
	}
	cmd.Flags().IntVar(&priority, "priority", 5, "higher runs sooner")
	cmd.Flags().IntVar(&retries, "retries", 3, "retry budget")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (auto-detected if empty)")
	cmd.Flags().StringArrayVar(&extraCriteria, "criteria", nil, "extra acceptance criterion")
	return cmd
}

func detectRepo() string {
	b := ghbridge.New()
	out, _, err := b.Execute(context.Background(), []string{"repo", "view", "--json", "nameWithOwner"})
	if err != nil {
		return ""
	}
	var v struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if json.Unmarshal([]byte(out), &v) == nil {
		return v.NameWithOwner
	}
	return ""
}

func fmtAtoi(s string) (int, error) { var n int; _, err := fmt.Sscanf(s, "%d", &n); return n, err }
