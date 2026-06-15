// SPDX-License-Identifier: MIT
// Purpose: Scorer implementations — exact, contains, success, LLM-judge,
// composite. Each Scorer turns a (case, output) into (score, passed, detail).
// Docs: scorer.doc.md
package evalharness

import "strings"

// Scorer turns a case + output into a 0..1 score and a pass/fail.
type Scorer interface {
	Score(c EvalCase, out Output) (score float64, passed bool, detail string)
}

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
