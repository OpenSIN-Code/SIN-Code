// SPDX-License-Identifier: MIT
// Purpose: `sin-code swarm` — race N agent profiles on the same prompt;
// the first verified result wins (issue #51, AGENTS.md §8 v3.6.0). All
// workers run headless: --yolo is intentionally NOT exposed; ask tools
// are denied by default. Cancellation propagates the moment the first
// worker returns Result.Verified == true.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/loopbuilder"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/spf13/cobra"
)

const (
	swarmDefaultTimeout = 10 * time.Minute
	swarmDefaultTurns   = 40
	swarmMinAgents      = 2
)

// agentRunner is the function shape used to build a fresh isolated loop
// for a given agent. It is overridable in tests so we can run hermetic
// swarm scenarios without a real LLM backend.
type agentRunner func(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error)

// swarmResult is the per-agent outcome surfaced to the user. Status is
// one of VERIFIED / UNVERIFIED / FAILED / CANCELLED / TIMEOUT.
type swarmResult struct {
	Agent   string `json:"agent"`
	Status  string `json:"status"`
	Turns   int    `json:"turns"`
	Summary string `json:"summary"`
}

// swarmReport aggregates every per-agent result plus the winner (or a
// non-empty Error if no agent verified within the timeout).
type swarmReport struct {
	Prompt  string        `json:"prompt"`
	Winner  string        `json:"winner,omitempty"`
	Error   string        `json:"error,omitempty"`
	Results []swarmResult `json:"results"`
}

type swarmOptions struct {
	prompt    string
	agentCSV  string
	timeout   time.Duration
	maxTurns  int
	jsonOut   bool
	workspace string

	// runner is the factory for fresh per-agent loops; defaults to
	// defaultAgentRunner. Tests override this to keep swarm hermetic.
	runner agentRunner
}

func NewSwarmCmd() *cobra.Command {
	opts := &swarmOptions{
		timeout:  swarmDefaultTimeout,
		maxTurns: swarmDefaultTurns,
	}
	cmd := &cobra.Command{
		Use:   "swarm",
		Short: "Race N agent profiles on the same prompt (first verified wins)",
		Long: `sin-code swarm runs the same prompt through N agent profiles in parallel.
All workers run HEADLESS (--yolo is not exposed; the loop never asks the user).
The first worker to return a verified Result wins; remaining workers are
cancelled via the parent context.

  sin-code swarm -p "fix the failing test" --agents coder,reviewer`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSwarm(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	f.StringVarP(&opts.prompt, "prompt", "p", "", "shared prompt (required)")
	f.StringVar(&opts.agentCSV, "agents", "", "comma-separated agent profile names (>=2 required)")
	f.DurationVar(&opts.timeout, "timeout", swarmDefaultTimeout, "global swarm timeout (per-agent budget)")
	f.IntVar(&opts.maxTurns, "max-turns", swarmDefaultTurns, "max turns per agent")
	f.BoolVar(&opts.jsonOut, "json", false, "emit structured JSON report")
	return cmd
}

func runSwarm(ctx context.Context, opts *swarmOptions) error {
	report, err := executeSwarm(ctx, opts)
	if err != nil {
		return err
	}
	return emitSwarm(opts, report)
}

// executeSwarm is the testable core. It is split out so tests can
// inspect the swarmReport without going through stdout emission.
func executeSwarm(ctx context.Context, opts *swarmOptions) (*swarmReport, error) {
	if opts.prompt == "" {
		return nil, errors.New("--prompt is required")
	}
	agents := splitNonEmpty(opts.agentCSV, ",")
	if len(agents) < swarmMinAgents {
		return nil, fmt.Errorf("--agents requires at least %d profiles (got %d)", swarmMinAgents, len(agents))
	}
	if opts.timeout <= 0 {
		opts.timeout = swarmDefaultTimeout
	}
	if opts.maxTurns <= 0 {
		opts.maxTurns = swarmDefaultTurns
	}
	if opts.workspace == "" {
		ws, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		opts.workspace = ws
	}
	runner := opts.runner
	if runner == nil {
		runner = defaultAgentRunner
	}

	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	type runOut struct {
		agent string
		res   *agentloop.Result
		err   error
	}
	results := make(chan runOut, len(agents))
	var wg sync.WaitGroup
	wg.Add(len(agents))

	for _, name := range agents {
		agentName := name
		go func() {
			defer wg.Done()
			loop, sess, cleanup, err := runner(ctx, agentName, opts.workspace)
			if err != nil {
				results <- runOut{agent: agentName, err: fmt.Errorf("setup: %w", err)}
				return
			}
			defer func() { _ = cleanup() }()

			// Mandate M4 + swarm hard mandate: headless, no Ask, no Yolo.
			// These overrides defend against an agent profile that
			// somehow ships with permissive defaults.
			loop.Ask = nil
			if loop.Perm != nil {
				loop.Perm.Headless = true
				loop.Perm.Yolo = false
			}
			loop.MaxTurns = opts.maxTurns

			res, err := loop.Run(ctx, sess, opts.prompt)
			if err != nil {
				results <- runOut{agent: agentName, err: err}
				return
			}
			results <- runOut{agent: agentName, res: res}
		}()
	}

	report := &swarmReport{Prompt: opts.prompt, Results: make([]swarmResult, 0, len(agents))}
	finished := 0
	winner := ""
	// First verified result cancels all other workers and is reported
	// as the winner. Non-verified completions are still collected so
	// the user sees every agent's outcome.
	for finished < len(agents) {
		select {
		case <-ctx.Done():
			cancel()
			wg.Wait()
			if winner == "" {
				report.Error = classifyCtxErr(ctx.Err())
			}
			report.Results = append(report.Results, cancelledMarkers(agents, report.Results)...)
			return report, nil
		case out := <-results:
			finished++
			if out.res != nil {
				status := "UNVERIFIED"
				if out.res.Verified {
					status = "VERIFIED"
				}
				report.Results = append(report.Results, swarmResult{
					Agent:   out.agent,
					Status:  status,
					Turns:   out.res.Turns,
					Summary: out.res.Summary,
				})
				if out.res.Verified && winner == "" {
					winner = out.agent
					report.Winner = winner
					cancel()
				}
			} else {
				report.Results = append(report.Results, swarmResult{
					Agent:  out.agent,
					Status: classifyErr(out.err),
					Turns:  0,
				})
			}
		}
	}
	cancel()
	wg.Wait()

	if winner == "" {
		report.Error = "no agent verified within timeout"
	}
	return report, nil
}

func emitSwarm(opts *swarmOptions, report *swarmReport) error {
	if opts.jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	fmt.Printf("swarm: %d agents on prompt %q (timeout=%s)\n",
		len(report.Results), truncate(opts.prompt, 60), opts.timeout)
	for _, r := range report.Results {
		fmt.Printf("  %-12s %-11s turns=%d  %s\n", r.Agent, r.Status, r.Turns, truncate(r.Summary, 80))
	}
	if report.Winner != "" {
		fmt.Printf("winner: %s\n", report.Winner)
	}
	if report.Error != "" {
		fmt.Fprintf(os.Stderr, "swarm error: %s\n", report.Error)
	}
	return nil
}

// defaultAgentRunner is the production wiring: a fully isolated loop
// per agent (no shared session, no shared completion function, no
// shared DB). We create a per-agent sessions DB under
// <workspace>/.sin-code/swarm/ so concurrent agents never share a
// *session.Session (mandate M7).
func defaultAgentRunner(ctx context.Context, agentName, workspace string) (*agentloop.Loop, *session.Session, func() error, error) {
	loop, cleanup, err := loopbuilder.Build(ctx, loopbuilder.Config{
		Workspace: workspace,
		AgentName: agentName,
		MaxTurns:  swarmDefaultTurns,
		// Hard mandates: headless, no yolo. The swarm sets these again
		// post-Build as defense in depth.
		Headless: true,
		Yolo:     false,
	}, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	dbPath, err := perAgentDBPath(workspace, agentName)
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		_ = cleanup()
		return nil, nil, nil, err
	}
	sess, err := store.StartOrResume("")
	if err != nil {
		_ = store.Close()
		_ = cleanup()
		return nil, nil, nil, err
	}
	return loop, sess, func() error {
		_ = store.Close()
		return cleanup()
	}, nil
}

func perAgentDBPath(workspace, agentName string) (string, error) {
	dir := filepath.Join(workspace, ".sin-code", "swarm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	suffix := randHex(4)
	return filepath.Join(dir, fmt.Sprintf("%s-%s.db", sanitizeFile(agentName), suffix)), nil
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(b)
}

func sanitizeFile(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "agent"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func splitNonEmpty(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func classifyErr(err error) string {
	if err == nil {
		return "FAILED"
	}
	if errors.Is(err, context.Canceled) {
		return "CANCELLED"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT"
	}
	return "FAILED"
}

func classifyCtxErr(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "swarm timeout exceeded"
	}
	return "swarm cancelled"
}

// cancelledMarkers fills in CANCELLED rows for agents that never
// returned a result before ctx was cancelled.
func cancelledMarkers(all []string, seen []swarmResult) []swarmResult {
	have := make(map[string]struct{}, len(seen))
	for _, r := range seen {
		have[r.Agent] = struct{}{}
	}
	var extra []swarmResult
	for _, a := range all {
		if _, ok := have[a]; !ok {
			extra = append(extra, swarmResult{Agent: a, Status: "CANCELLED"})
		}
	}
	return extra
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
