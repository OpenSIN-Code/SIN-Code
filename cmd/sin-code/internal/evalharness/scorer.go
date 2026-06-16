// SPDX-License-Identifier: MIT
// Purpose: Scorer implementations — exact, contains, success, LLM-judge,
// composite, compile-and-run. Each Scorer turns a (case, output) into
// (score, passed, detail).
// Docs: scorer.doc.md
package evalharness

import (
	"fmt"
	"strings"
	"time"
)

// Scorer turns a case + output into a 0..1 score and a pass/fail.
type Scorer interface {
	Score(c EvalCase, out Output) (score float64, passed bool, detail string)
}

// compileRunTimeout is the default wall-clock budget for compile + run.
const compileRunTimeout = 30 * time.Second

// ExactMatch scores 1.0 when output equals the expected string (trimmed).
type ExactMatch struct{}

func (ExactMatch) Score(c EvalCase, out Output) (float64, bool, string) {
	got := strings.TrimSpace(out.Text)
	want := strings.TrimSpace(c.Expected)
	if got == want {
		return 1, true, "exact match"
	}
	return 0, false, "no exact match"
}

// ContainsAll scores by the fraction of expected substrings present.
// Expected is treated as a newline-separated list of required snippets.
type ContainsAll struct{ PassThreshold float64 }

func (s ContainsAll) Score(c EvalCase, out Output) (float64, bool, string) {
	want := splitNonEmpty(c.Expected)
	if len(want) == 0 {
		return boolScore(out.Success), out.Success, "no expectations; used success flag"
	}
	hay := strings.ToLower(out.Text)
	hits := 0
	var missing []string
	for _, w := range want {
		if strings.Contains(hay, strings.ToLower(strings.TrimSpace(w))) {
			hits++
		} else {
			missing = append(missing, w)
		}
	}
	score := float64(hits) / float64(len(want))
	th := s.PassThreshold
	if th == 0 {
		th = 1.0
	}
	detail := "matched all"
	if len(missing) > 0 {
		detail = "missing: " + strings.Join(missing, ", ")
	}
	return score, score >= th, detail
}

// SuccessFlag scores purely on the subject's success boolean. Useful
// when the subject is a verify gate that already decides pass/fail.
type SuccessFlag struct{}

func (SuccessFlag) Score(_ EvalCase, out Output) (float64, bool, string) {
	return boolScore(out.Success), out.Success, "subject success flag"
}

// LLMJudge delegates scoring to a model. Wire instinct.Completer-style client.
type LLMJudge struct {
	Judge func(prompt, expected, got string) (score float64, ok bool, detail string)
}

func (j LLMJudge) Score(c EvalCase, out Output) (float64, bool, string) {
	if j.Judge == nil {
		return 0, false, "no judge configured"
	}
	return j.Judge(c.Prompt, c.Expected, out.Text)
}

// Composite averages several scorers with optional weights.
type Composite struct {
	Scorers       []Scorer
	Weights       []float64
	PassThreshold float64
}

func (comp Composite) Score(c EvalCase, out Output) (float64, bool, string) {
	if len(comp.Scorers) == 0 {
		return 0, false, "no scorers"
	}
	var sumW, sumWS float64
	var details []string
	for i, s := range comp.Scorers {
		w := 1.0
		if i < len(comp.Weights) {
			w = comp.Weights[i]
		}
		sc, _, d := s.Score(c, out)
		sumW += w
		sumWS += w * sc
		details = append(details, d)
	}
	score := 0.0
	if sumW > 0 {
		score = sumWS / sumW
	}
	th := comp.PassThreshold
	if th == 0 {
		th = 0.7
	}
	return score, score >= th, strings.Join(details, " | ")
}

func splitNonEmpty(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func boolScore(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// CompileAndRun extracts a fenced code block from the model output,
// compiles it, and runs a self-check. It returns 1.0 only when both
// compile and self-check pass. This is the SIN-Code equivalent of
// ponytail's correctness.js gate (ponytail/benchmarks/README.md:69-79).
//
// If SkipTest is true, a trivial one-liner is accepted after compile
// without a self-check (YAGNI for tests). If SelfCheck is empty and
// SkipTest is false, the score is 0.5 because compile alone is not
// enough to prove correctness.
type CompileAndRun struct {
	Language  string        // "go" | "python" | "javascript" | "bash"
	SelfCheck string        // code appended to the extracted block before execution
	Timeout   time.Duration // default 30s
	Binary    string        // optional explicit compiler/interpreter path
	SkipTest  bool          // true for trivial one-liners that need no test
}

// Score implements Scorer.
func (c CompileAndRun) Score(ca EvalCase, out Output) (float64, bool, string) {
	code := extractCodeBlock(out.Text)
	if code == "" {
		return 0, false, "no code block in output"
	}

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = compileRunTimeout
	}

	// Step 1: compile.
	if err := c.compile(code, timeout); err != nil {
		return 0, false, "compile failed: " + err.Error()
	}

	// Step 2: run self-check (unless YAGNI applies).
	if c.SkipTest {
		return 1, true, "compile passed, trivial one-liner"
	}
	if c.SelfCheck == "" {
		return 0.5, false, "compile passed but no self-check (set SkipTest for trivial one-liners)"
	}
	if err := c.run(code, c.SelfCheck, timeout); err != nil {
		return 0, false, "self-check failed: " + err.Error()
	}
	return 1, true, "compile + self-check passed"
}

// compileAndRunLanguages lists the languages the scorer currently supports.
var compileAndRunLanguages = map[string]struct{}{
	"go":         {},
	"python":     {},
	"javascript": {},
	"bash":       {},
}

// IsCompileAndRunLanguage reports whether lang is supported.
func IsCompileAndRunLanguage(lang string) bool {
	_, ok := compileAndRunLanguages[lang]
	return ok
}

// ScorerFromConfig builds a Scorer from a map[string]any configuration.
// It returns (nil, nil) when cfg is empty or no recognized type is given,
// so the caller can fall back to SuccessFlag.
func ScorerFromConfig(cfg map[string]any) (Scorer, error) {
	if len(cfg) == 0 {
		return nil, nil
	}
	typ, _ := cfg["type"].(string)
	switch typ {
	case "exact":
		return ExactMatch{}, nil
	case "contains":
		th := float64(0)
		if v, ok := cfg["pass_threshold"].(float64); ok {
			th = v
		}
		return ContainsAll{PassThreshold: th}, nil
	case "compile_and_run":
		lang, _ := cfg["language"].(string)
		if lang == "" {
			lang, _ = cfg["lang"].(string)
		}
		if !IsCompileAndRunLanguage(lang) {
			return nil, fmt.Errorf("compile_and_run: unsupported language %q", lang)
		}
		selfCheck, _ := cfg["self_check"].(string)
		skip := false
		if v, ok := cfg["skip_test"].(bool); ok {
			skip = v
		}
		timeout := time.Duration(0)
		if v, ok := cfg["timeout"].(string); ok && v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				timeout = d
			}
		}
		binary, _ := cfg["binary"].(string)
		return CompileAndRun{
			Language:  lang,
			SelfCheck: selfCheck,
			Timeout:   timeout,
			Binary:    binary,
			SkipTest:  skip,
		}, nil
	default:
		return nil, fmt.Errorf("unknown scorer type %q", typ)
	}
}
