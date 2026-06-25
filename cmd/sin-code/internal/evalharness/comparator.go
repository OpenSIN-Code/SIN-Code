// SPDX-License-Identifier: MIT
// Purpose: four-arm eval comparator (issue #171). Runs the same
// EvalSet against N arms in turn, aggregates per-arm metrics, and
// emits a row-per-arm matrix that mirrors ponytail's
// benchmarks/README.md:34-58 layout (LOC, USD, latency,
// correctness). The honest delta between an arm and the terse
// control is exposed in CompareReport; this lets reviewers grade
// any new skill on the same scale rather than on absolute numbers
// (which conflate the skill with the "be terse" instruction).
//
// Three pinned arms (`__baseline__`, `__terse__`, `__lazy_skill__`)
// plus one user-supplied skill make up the default four. The
// comparator NEVER compares a skill's absolute numbers to the
// baseline — that delta is what caveman's evals/README.md calls
// "inflated by the generic 'be terse' effect".
//
// Docs: comparator.doc.md
package evalharness

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// CompareOptions controls one Compare run. All fields are
// optional; zero values yield a serial single-subject run with
// consecutive arm execution.
type CompareOptions struct {
	// PerCaseTimeout bounds one (case, arm) pair. Zero = no bound.
	PerCaseTimeout time.Duration
	// OnProgress (optional) is called after each (case, arm)
	// completion. armID is "" — callers receive every arm on the
	// same channel and discriminate via Result.ArmID.
	OnProgress func(done, total int, last ArmRun)
	// Warmup repeats the first case once and discards the result
	// to remove LLM warm-up cost from the median calculation. Zero
	// = no warm-up (the default for CI deterministic snapshots).
	Warmup bool
}

// ArmRun is one (case, arm) result plus the unsorted per-arm
// metrics it contributes to. We stuff the per-arm totals into
// TotalsByArm; the per-arm detail stays in ArmRun for diffing
// against a historical snapshot.
type ArmRun struct {
	Result Result `json:"result"`
	ArmID  string `json:"arm_id"`
	// LOC is the line count of the subject's output for this
	// case/under this arm. Zero when the subject is stubby.
	LOC int `json:"loc"`
	// USD is the computed cost of this single (case, arm) call.
	USD float64 `json:"usd"`
	// Tokens is the prompt + completion total at the time of run.
	Tokens int `json:"tokens"`
	// UsedPricingName echoes the PricingName used by the
	// comparator — handy for snapshot diffs.
	UsedPricingName string `json:"used_pricing_name"`
}

// CaseComparison is one row of the matrix: across all arms for
// one case. Rows are kept aligned by ArmID across cases so the
// output is `.by_arm{arm_id} = []int{...}` if you scrub it via
// jq (issue #171 §3).
type CaseComparison struct {
	CaseID string            `json:"case_id"`
	Arms   map[string]ArmRun `json:"arms"` // keyed by ArmID; absent entries were skipped
}

// Totals is per-arm rollup across all cases. Fields are slices so
// that the comparator can compute medians, means, etc. without
// touching a one-off helper.
type Totals struct {
	ArmID           string    `json:"arm_id"`
	TotalCases      int       `json:"total_cases"`
	Passed          int       `json:"passed"`
	WeightedScore   float64   `json:"weighted_score"`
	Scores          []float64 `json:"-"` // for median / mean / stddev
	USD             []float64 `json:"-"`
	Tokens          []int     `json:"-"`
	LatencyMS       []int     `json:"-"`
	LOC             []int     `json:"-"`
	SkillName       string    `json:"skill_name,omitempty"`
	VerbosityLevel  string    `json:"verbosity,omitempty"`
	SystemPrompt    string    `json:"-"` // used to compute shortHash for snapshots
	PricingName     string    `json:"pricing_name,omitempty"`
	FirstToPassRate float64   `json:"first_to_pass_rate,omitempty"`
}

// PassRate returns the integer-pass / total ratio as a 0..1 float.
func (t Totals) PassRate() float64 {
	if t.TotalCases == 0 {
		return 0
	}
	return float64(t.Passed) / float64(t.TotalCases)
}

// CompareReport is the full output of one compare run. The
// matrix is recoverable through ByArm — by walking cases × arms.
type CompareReport struct {
	Set         EvalSet           `json:"set"`
	Arms        []Arm             `json:"arms"` // ordered
	PerCase     []CaseComparison  `json:"per_case"`
	TotalsByArm map[string]Totals `json:"totals_by_arm"`
	Warnings    []string          `json:"warnings,omitempty"`
}

// ByArm pivots the report: slice of arm-id → ordered per-case runs.
// Comparable to ponytail's benchmarks table; preserved for stable
// downstream tooling.
func (r CompareReport) ByArm() map[string][]ArmRun {
	out := make(map[string][]ArmRun, len(r.Arms))
	for _, arm := range r.Arms {
		out[arm.ID] = nil
	}
	for _, c := range r.PerCase {
		for _, arm := range r.Arms {
			if run, ok := c.Arms[arm.ID]; ok {
				out[arm.ID] = append(out[arm.ID], run)
			}
		}
	}
	return out
}

// Compare runs every arm in serial against every case and returns
// the aggregated report. Subject MUST be non-nil; Scorer is
// optional (falls back to SuccessFlag).
//
// The contract is: if Subject.Run fails for a (case, arm), the run
// is recorded as score=0 with an error message; we never panic.
// Concurrency is intentionally zero — caveman evals is serial too,
// and parallel runs introduce float drift in median calc and
// break snapshot byte-stability (issue #171 §6).
func Compare(ctx context.Context, set EvalSet, arms []Arm, opts CompareOptions) (CompareReport, error) {
	if len(arms) == 0 {
		return CompareReport{}, errors.New("comparator: empty arms list")
	}
	report := CompareReport{
		Set:         set,
		Arms:        append([]Arm(nil), arms...),
		PerCase:     make([]CaseComparison, 0, len(set.Cases)),
		TotalsByArm: make(map[string]Totals, len(arms)),
	}
	for _, a := range arms {
		report.TotalsByArm[a.ID] = Totals{
			ArmID:          a.ID,
			SkillName:      a.SkillName,
			VerbosityLevel: a.Verbosity,
			SystemPrompt:   a.SystemPrompt,
			PricingName:    a.PricingName,
		}
	}
	total := len(set.Cases) * len(arms)
	done := 0
	warmupDone := !opts.Warmup
	for _, c := range set.Cases {
		row := CaseComparison{CaseID: c.ID, Arms: make(map[string]ArmRun, len(arms))}
		for _, arm := range arms {
			if !warmupDone && done == 0 {
				// intentional one-shot warmup that is NOT counted
				if _, err := runArmOnce(ctx, c, arm); err != nil {
					report.Warnings = append(report.Warnings, fmt.Sprintf("warmup %s/%s: %v", c.ID, arm.ID, err))
				}
				warmupDone = true
				continue
			}
			cc, err := runArmOnce(ctx, c, arm)
			if err != nil {
				report.Warnings = append(report.Warnings, fmt.Sprintf("case %s/arm %s: %v", c.ID, arm.ID, err))
				cc.Result = Result{CaseID: c.ID, ArmID: arm.ID, Err: err.Error(), Passed: false, Weight: 1}
			} else {
				cc.Result.ArmID = arm.ID
			}
			cc.ArmID = arm.ID
			cc.UsedPricingName = priceLookup(arm)
			row.Arms[arm.ID] = cc
			t := report.TotalsByArm[arm.ID]
			t.TotalCases++
			if cc.Result.Passed {
				t.Passed++
			}
			t.WeightedScore += cc.Result.Score
			t.Scores = append(t.Scores, cc.Result.Score)
			t.USD = append(t.USD, cc.USD)
			t.Tokens = append(t.Tokens, cc.Tokens)
			t.LatencyMS = append(t.LatencyMS, int(cc.Result.Duration/time.Millisecond))
			t.LOC = append(t.LOC, cc.LOC)
			if arm.FusionEnabled {
				t.FirstToPassRate = 0.0
			}
			report.TotalsByArm[arm.ID] = t
			done++
			if opts.OnProgress != nil {
				opts.OnProgress(done, total, cc)
			}
		}
		report.PerCase = append(report.PerCase, row)
	}
	// Average the weighted score over total cases (it's a sum right now).
	for id, t := range report.TotalsByArm {
		if t.TotalCases > 0 {
			t.WeightedScore = t.WeightedScore / float64(t.TotalCases)
		}
		sort.SliceStable(t.LatencyMS, func(i, j int) bool { return t.LatencyMS[i] < t.LatencyMS[j] })
		report.TotalsByArm[id] = t
	}
	return report, nil
}

// CompareParallel is the same as Compare but with per-arm
// concurrency. We keep it behind a separate function so the default
// path stays deterministic and the comparator doesn't carry
// goroutine-orchestration code that we'd need to test under
// -race every time someone tweaks the loop.
//
// Currently unused by callers; left in place for the eventual
// "harness speedup" follow-up — see BACKLOG.md.
func CompareParallel(ctx context.Context, set EvalSet, arms []Arm, opts CompareOptions, workers int) (CompareReport, error) {
	if workers <= 0 {
		workers = len(arms)
	}
	if workers > len(arms) {
		workers = len(arms)
	}
	var (
		mu     sync.Mutex
		report = CompareReport{
			Set:         set,
			Arms:        append([]Arm(nil), arms...),
			PerCase:     make([]CaseComparison, 0, len(set.Cases)),
			TotalsByArm: make(map[string]Totals, len(arms)),
		}
	)
	for _, a := range arms {
		report.TotalsByArm[a.ID] = Totals{
			ArmID:          a.ID,
			SkillName:      a.SkillName,
			VerbosityLevel: a.Verbosity,
			SystemPrompt:   a.SystemPrompt,
			PricingName:    a.PricingName,
		}
	}
	var wg sync.WaitGroup
	armCh := make(chan Arm, len(arms))
	for _, a := range arms {
		armCh <- a
	}
	close(armCh)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for arm := range armCh {
				for _, c := range set.Cases {
					cc, err := runArmOnce(ctx, c, arm)
					mu.Lock()
					if err != nil {
						report.Warnings = append(report.Warnings, fmt.Sprintf("case %s/arm %s: %v", c.ID, arm.ID, err))
						cc.Result = Result{CaseID: c.ID, ArmID: arm.ID, Err: err.Error(), Passed: false, Weight: 1}
					} else {
						cc.Result.ArmID = arm.ID
					}
					cc.ArmID = arm.ID
					cc.UsedPricingName = priceLookup(arm)
					t := report.TotalsByArm[arm.ID]
					t.TotalCases++
					if cc.Result.Passed {
						t.Passed++
					}
					t.WeightedScore += cc.Result.Score
					t.Scores = append(t.Scores, cc.Result.Score)
					t.USD = append(t.USD, cc.USD)
					t.Tokens = append(t.Tokens, cc.Tokens)
					t.LatencyMS = append(t.LatencyMS, int(cc.Result.Duration/time.Millisecond))
					t.LOC = append(t.LOC, cc.LOC)
					report.TotalsByArm[arm.ID] = t
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	for id, t := range report.TotalsByArm {
		if t.TotalCases > 0 {
			t.WeightedScore = t.WeightedScore / float64(t.TotalCases)
		}
		report.TotalsByArm[id] = t
	}
	return report, nil
}

// runArmOnce runs one subject against one case under one arm and
// returns the populated ArmRun. The PairwiseContract:
//
//   - arm.Setup (if non-nil) is invoked once before Run.
//   - subject.Run is called with the arm's prompt + system prompt
//     embedded into the case.Prompt via the conventional
//     "System: <p>\n\nUser: <case.Prompt>" framing.
//   - the LLMJudge scorer is honoured if no explicit Scorer is
//     supplied (subject.Run returns Output.Success=true → use it).
//
// Token counts, LOC, and USD are all derived from Output.Meta so
// callers don't need to plumb extra instrumentation through their
// Subject implementation. Convention keys:
//
//	"prompt_tokens", "completion_tokens", "total_tokens"  (int)
//	"loc"                                              (int)
//	"pricing_name"                                     (string, optional)
func runArmOnce(ctx context.Context, c EvalCase, arm Arm) (ArmRun, error) {
	if arm.Setup != nil {
		_ = arm.Setup(c) // ignored: setup is best-effort, errors are surfaced via Subject
	}
	cctx := ctx
	if dl, ok := ctx.Deadline(); ok && time.Until(dl) <= 0 {
		return ArmRun{}, fmt.Errorf("arm %s: ctx already past deadline", arm.ID)
	}
	start := time.Now()
	// We rely on the *registered* Subject interface (evalharness
	// owns it). Some wiring layers hand us a stub subject; in
	// that case the subject signature is fine — it just doesn't
	// know about arms. We pack the arm's prompt into the case's
	// Meta so a Subject that reads Meta gets the equivalent of
	// a real arm prompt.
	caseCopy := c
	if caseCopy.Meta == nil {
		caseCopy.Meta = make(map[string]string, 3)
	}
	caseCopy.Meta["arm_id"] = arm.ID
	caseCopy.Meta["system_prompt"] = arm.SystemPrompt
	// Standard Subject interface — caller wires a subject that
	// knows how to read arm meta OR not. The default Subject in
	// the comparator package is the "no-op" stub, which just
	// echoes the prompt.
	subj := chooseSubject(arm)
	out, err := subj.Run(cctx, caseCopy)
	dur := time.Since(start)
	if err != nil {
		return ArmRun{Result: Result{CaseID: c.ID, Duration: dur, Err: err.Error()}}, err
	}
	// Score via DefaultScorer arm-aware scorer.
	score, passed, detail := DefaultScorer{}.Score(caseCopy, out)
	ptok := readIntMeta(out.Meta, "prompt_tokens", 0)
	ctok := readIntMeta(out.Meta, "completion_tokens", 0)
	ttok := readIntMeta(out.Meta, "total_tokens", ptok+ctok)
	loc := readIntMeta(out.Meta, "loc", 0)
	if loc == 0 {
		// Default metric — count lines of the output text. The LLM
		// judges then have a "honest" LOC for the cell of the matrix.
		loc = countLOC(out.Text)
	}
	priceName := readStringMeta(out.Meta, "pricing_name", arm.PricingName)
	if priceName == "" {
		priceName = arm.PricingName
	}
	price, known := PriceOf(priceName)
	if !known {
		price = Price{}
	}
	usd := out.USD
	if usd == 0 {
		usd = Cost(price, ptok, ctok)
	}
	res := Result{
		CaseID:           c.ID,
		Score:            score,
		Passed:           passed,
		Detail:           detail,
		Weight:           ensureWeight(c.Weight),
		Output:           out.Text,
		Duration:         dur,
		ArmID:            arm.ID,
		PromptTokens:     ptok,
		CompletionTokens: ctok,
		TotalTokens:      ttok,
		LOC:              loc,
		USD:              usd,
	}
	return ArmRun{Result: res, ArmID: arm.ID, LOC: loc, USD: usd, Tokens: ttok, UsedPricingName: priceName}, nil
}

// ensureWeight returns the weight with a safe default so the
// median never sees a NaN.
func ensureWeight(w float64) float64 {
	if w == 0 {
		return 1
	}
	return w
}

// priceLookup is a tiny helper that pins the pricing name on the
// per-(case,arm) run. Kept as a function rather than an inline so
// tests can swap in a stub. When arm.PricingName is empty we
// default to "stub"; callers can override via Output.Meta["pricing_name"]
// (already extracted in runArmOnce before we get here).
func priceLookup(arm Arm) string {
	if arm.PricingName != "" {
		return arm.PricingName
	}
	return "stub"
}

// DefaultScorer is the Subject-agnostic scorer used when the
// caller didn't pass one. Mirrors SuccessFlag for the result row
// and falls back to a 0.5 / false-rule when Output.Success is
// unset. The brachiation keeps the comparator honest for stub
// subjects (issue #171 §2.4).
type DefaultScorer struct{}

func (DefaultScorer) Score(c EvalCase, out Output) (float64, bool, string) {
	if out.Success {
		return 1, true, "subject success flag"
	}
	return 0, false, "no success flag"
}

// readIntMeta reads an int key from a meta map with a fallback.
func readIntMeta(m map[string]string, key string, fallback int) int {
	if m == nil {
		return fallback
	}
	v, ok := m[key]
	if !ok {
		return fallback
	}
	n := 0
	for i := 0; i < len(v); i++ {
		if v[i] < '0' || v[i] > '9' {
			if v[i] == '-' && i == 0 {
				continue
			}
			return fallback
		}
		n = n*10 + int(v[i]-'0')
	}
	return n
}

// readStringMeta reads a string from the meta map or returns the
// fallback. Saves Subject authors from having to know the key is
// at the meta level.
func readStringMeta(m map[string]string, key, fallback string) string {
	if m == nil {
		return fallback
	}
	if v, ok := m[key]; ok {
		return v
	}
	return fallback
}

// countLOC returns the number of non-empty lines in the output.
// The comparator counts lines of output text as the LOC metric
// for the matrix — matching ponytail's benchmarks/README.md:34-58
// column heading. Empty outputs are 0 lines.
func countLOC(text string) int {
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

// chooseSubject returns the Subject to drive this arm. Default
// is the stub NoOpSubject. Tests replace this via SetDefaultSubject
// once per process — the function is pkg-level mutable to keep the
// comparator side dependency-free.
var defaultSubject Subject = NoOpSubject{}

// sin-debt: yagni, upgrade: when a second Subject implementation lands, remove this factory
// SetDefaultSubject swaps the Subject used by the comparator
// when the arm's Setup function does not provide its own. Returns
// the previous subject so callers can swap back safely.
func SetDefaultSubject(s Subject) Subject {
	prev := defaultSubject
	defaultSubject = s
	return prev
}

// sin-debt: yagni, upgrade: when a second Subject implementation lands, remove this factory
func chooseSubject(_ Arm) Subject { return defaultSubject }

// NoOpSubject returns the original prompt as output. Used as the
// default Subject — keeps the comparator honest for offline runs
// where no LLM bridge is wired.
type NoOpSubject struct{ Prefix string }

func (s NoOpSubject) Run(_ context.Context, c EvalCase) (Output, error) {
	text := s.Prefix + c.Prompt
	if c.Meta != nil {
		if sp, ok := c.Meta["system_prompt"]; ok && sp != "" {
			text = "[system:" + truncateMeta(sp, 80) + "] " + text
		}
		if id, ok := c.Meta["arm_id"]; ok {
			text = "[arm:" + id + "] " + text
		}
	}
	return Output{
		Text:    text,
		Success: false, // NoOp = uncategorised; LLM/Subject decides pass
		Meta:    map[string]string{},
	}, nil
}

// CloneSessionBySkill is the test/production symmetry helper. It
// returns a *Session scoped to a particular skill name so the
// comparator can build a controlled environment for each arm.
//
// In this package we don't actually have an *exec session; we
// return the testable wrapper (name + metadata) so a higher layer
// that DOES own sessions (cmd/sin-code/internal/session) can wrap
// the same contract without re-writing the comparator.
//
// This stub is in evalharness for the issue #171 acceptance test
// (CloneSessionBySkill API present even when the user can't yet
// open a real session). Pinning the signature today prevents a
// premature-maturity footgun when wiring layers line up tomorrow.
type CompareSession struct {
	CaseID    string `json:"case_id"`
	SkillName string `json:"skill_name,omitempty"`
	ArmID     string `json:"arm_id,omitempty"`
}

// CloneSessionBySkill returns a fresh CompareSession tagged with
// the given case ID and skill name. Real session wiring is a
// follow-up PR; this returns the canonical in-memory envelope.
func CloneSessionBySkill(caseID, skillName string) *CompareSession {
	return &CompareSession{CaseID: caseID, SkillName: skillName}
}

// truncateMeta keeps the NoOpSubject output bounded so the
// "system:..." prefix doesn't blow up the result text. The full
// prompt survives in Result.Meta.
func truncateMeta(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
