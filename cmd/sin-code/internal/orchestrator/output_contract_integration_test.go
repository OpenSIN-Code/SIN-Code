// SPDX-License-Identifier: MIT
// Purpose: end-to-end integration of the caveman output contract across
// the four orchestrator sub-agents (Critic, Adversary, Governor,
// Cartographer). One fixture per agent — every rendered byte is checked
// verbatim, no drift between expected and actual.
package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── Critic integration ─────────────────────────────────────────────

// cavemanCriticReply is the literal prose a Critic's agent produces
// when it follows the output contract. Three Findings, one stale comment
// stripped by the trailing-newline rule.
const cavemanCriticReply = "" +
	"cmd/sin-code/internal/foo/foo.go:42 — truncate — delete — drop unused 5-line wrapper # c=0.85\n" +
	"cmd/sin-code/internal/foo/foo.go:55 — readBuf — simplify — inline small helper # c=0.65\n" +
	"cmd/sin-code/internal/foo/foo.go:0 — - — verify — manual review needed # c=0.40\n"

type cavemanCriticAgent struct{ name, reply string }

func (s *cavemanCriticAgent) Name() string        { return s.name }
func (s *cavemanCriticAgent) Config() AgentConfig { return AgentConfig{Name: s.name} }
func (s *cavemanCriticAgent) Run(_ context.Context, _ *Task, _ *Scratchpad) (string, error) {
	return s.reply, nil
}

// TestCriticContractExtractsFindings verifies the Critic's last attempt
// is parsed through the contract and the structured slice is exposed.
func TestCriticContractExtractsFindings(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	critic := NewCritic(vf, []Check{{Kind: CheckBuild, Name: "ok", Cmd: []string{"true"}}})
	ag := &cavemanCriticAgent{name: "reviewer", reply: cavemanCriticReply}
	res, err := critic.Drive(context.Background(), ag, &Task{ID: "t1", Title: "x", Description: "d"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatal("expected pass on first attempt")
	}
	if len(res.Findings) != 3 {
		t.Fatalf("expected 3 Findings, got %d", len(res.Findings))
	}
	if len(res.ParseErrors) != 0 {
		t.Fatalf("clean reply must produce no ParseErrors, got %v", res.ParseErrors)
	}

	// Byte-stable: render Criter's first finding and pin it.
	const expectedFirst = "cmd/sin-code/internal/foo/foo.go:42 — truncate — delete — drop unused 5-line wrapper # c=0.85"
	if got := res.Findings[0].Render(); got != expectedFirst {
		t.Fatalf("byte drift:\n  got:  %q\n  want: %q", got, expectedFirst)
	}
	// File-level finding (Line=0) drops the ":0" suffix.
	if strings.Contains(res.Findings[2].Render(), ":0") {
		t.Fatalf("file-level Finding must drop :0, got %q", res.Findings[2].Render())
	}
}

// TestCriticContractCapturesParseErrors verifies a malformed reply
// produces a non-empty ParseErrors trace WITHOUT failing the Critic
// itself (Critic still reports the agent error / verdict).
func TestCriticContractCapturesParseErrors(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	critic := NewCritic(vf, []Check{{Kind: CheckBuild, Name: "ok", Cmd: []string{"true"}}})

	malformedReply := strings.Join([]string{
		goldenFinding.Render(),
		"",
		"this prose line has no em-dashes and no confidence suffix",
	}, "\n")
	ag := &cavemanCriticAgent{name: "reviewer", reply: malformedReply}
	res, err := critic.Drive(context.Background(), ag, &Task{ID: "t1", Title: "x", Description: "d"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatal("verifier still passes — the contract failure is at a different layer")
	}
	if len(res.Findings) != 1 {
		t.Fatalf("expected 1 valid finding, got %d", len(res.Findings))
	}
	if len(res.ParseErrors) != 1 {
		t.Fatalf("expected 1 per-line ParseError, got %d", len(res.ParseErrors))
	}
	if !strings.Contains(res.ParseErrors[0], "not a caveman line") {
		t.Fatalf("ParseError should mention shape mismatch, got %q", res.ParseErrors[0])
	}
}

// ─── Adversary integration ──────────────────────────────────────────

// TestAdversaryContractDerivesFromAttacks verifies the Adversary's
// Findings come from the structured Attack list (not free-form prose).
func TestAdversaryContractDerivesFromAttacks(t *testing.T) {
	stub := &stubAdversary{attacks: []Attack{
		{Kind: AttackBoundary, Hypothesis: "empty input crashes", ProbeSource: "package foo\n"},
		{Kind: AttackInjection, Hypothesis: "sql injection", ProbeSource: "package bar\n"},
	}}
	adv := &Adversary{Agent: stub, Workdir: "", MaxAttacks: 2, ProbeTimeout: 1000000000}
	res, err := adv.Review(context.Background(), "diff", "impact")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(res.Findings) != 2 {
		t.Fatalf("expected 2 derived Findings, got %d", len(res.Findings))
	}
	// Empty workdir means ALL probes error out — every Finding is `verify`
	// (cleared). Verify Tag + structure with byte-stable expectation.
	const expectedFirst = "adversary://probe — boundary — verify — empty input crashes # c=0.50"
	if got := res.Findings[0].Render(); got != expectedFirst {
		t.Fatalf("byte drift:\n  got:  %q\n  want: %q", got, expectedFirst)
	}

	// Verifier must accept the Adversary's cleared state — no hedging
	// in "empty input crashes" or "sql injection".
	if errs := VerifyFindings(res.Findings); len(errs) > 0 {
		t.Fatalf("Adversary's derived Findings must pass contract, got errors: %v", errs)
	}
}

// ─── Governor integration ───────────────────────────────────────────

// TestGovernorContractDerivesFromEscalations exercises the Governor's
// Findings derivation across a 2-rung ladder that climbs.
func TestGovernorContractDerivesFromEscalations(t *testing.T) {
	vf := NewVerifier(t.TempDir())
	gov := &Governor{
		Ladder: []Rung{
			{Name: "cheap", Agents: 1, RepairRounds: 0, Timeout: 5 * _5s},
			{Name: "escalated", Agents: 1, RepairRounds: 0, Timeout: 5 * _5s},
		},
		Verifier: vf,
		Checks:   []Check{{Kind: CheckBuild, Name: "fail", Cmd: []string{"false"}}},
		Factory: func(r Rung) []Agent {
			return []Agent{&scriptAgent{name: "a", reply: "ok"}}
		},
	}
	res, err := gov.Execute(context.Background(), &Task{ID: "t42", Title: "x", Description: "d"}, NewScratchpad())
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed {
		t.Fatal("cannot pass on failing check")
	}
	if len(res.Escalations) != 1 {
		t.Fatalf("expected 1 escalation, got %d", len(res.Escalations))
	}
	if len(res.Findings) != 1 {
		t.Fatalf("expected 1 Finding derived from escalation, got %d", len(res.Findings))
	}
	// Byte-stable render — pin the exact format.
	wantPath := "task://t42"
	if !strings.HasPrefix(res.Findings[0].Path, wantPath) {
		t.Fatalf("Governor Finding must anchor at %q, got %q", wantPath, res.Findings[0].Path)
	}
	if res.Findings[0].Tag != TagRisk {
		t.Fatalf("Governor Finding must be Tag=risk, got %q", res.Findings[0].Tag)
	}
	if !strings.HasPrefix(res.Findings[0].Symbol, "cheap->escalated") {
		t.Fatalf("Governor Finding Symbol must encode the rung climb, got %q", res.Findings[0].Symbol)
	}
}

// ─── Cartographer integration ───────────────────────────────────────

// TestCartographerContractEmitsFindings walks a tiny repo, asks for the
// top-2 by PageRank, and verifies the Findings are byte-stable.
func TestCartographerContractEmitsFindings(t *testing.T) {
	dir := t.TempDir()
	src := `package x
func Hello() {}
func World() { Hello() }
func Greet() { Hello() }
func Goodbye() { World() }
`
	if err := writeFile(dir, "x.go", src); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(dir, "go.mod", "module x\n\ngo 1.22\n"); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	fs := c.Findings(2)
	if len(fs) == 0 {
		t.Fatal("cartographer must emit at least one Finding")
	}
	if len(fs) > 2 {
		t.Fatalf("k cap broken: %d", len(fs))
	}
	for _, f := range fs {
		if f.Tag != TagVerify {
			t.Errorf("Cartographer Findings must all be Tag=verify, got %q", f.Tag)
		}
		if f.Path == "" || f.Line <= 0 {
			t.Errorf("Cartographer Finding missing locator: %+v", f)
		}
	}
	// Verifier must accept (no hedging in rank:N hint).
	if errs := VerifyFindings(fs); len(errs) > 0 {
		t.Fatalf("Cartographer Findings must pass contract, got errors: %v", errs)
	}
}

// TestCartographerContractEmptyKReturnsEmpty ensures k=0 (and negative k)
// yield the empty slice — the cartographer is opt-in, never floods.
func TestCartographerContractEmptyKReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := writeFile(dir, "x.go", "package x\nfunc A() {}\n"); err != nil {
		t.Fatal(err)
	}
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, k := range []int{0, -1, -100} {
		if fs := c.Findings(k); len(fs) != 0 {
			t.Errorf("Findings(%d): expected empty, got %d", k, len(fs))
		}
	}
}

// ─── Shared helpers ─────────────────────────────────────────────────

const _5s = 1_000_000_000 // 1 second, ns

func writeFile(dir, name, body string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
}
