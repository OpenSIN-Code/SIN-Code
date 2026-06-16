// SPDX-License-Identifier: MIT
// Purpose: LLM-backed extractor for richer pattern mining. Wraps a
// cheap background model behind a tiny `Completer` interface and
// always falls back to HeuristicExtractor on any error. The session
// never blocks on instinct extraction.
// Docs: extract_llm.doc.md
package instinct

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Completer is the minimal interface SIN-Code's model client must
// satisfy. Implement it where the instinct package is wired into the
// model layer; see internal/adapters/completer.go for the reference.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// LLMExtractor uses a cheap model to mine richer patterns than the
// heuristic pass. It falls back to the heuristic extractor on any
// error so the learning loop never breaks a session.
type LLMExtractor struct {
	Model    Completer
	Fallback Extractor // defaults to HeuristicExtractor if nil
	MaxObs   int       // cap observations sent to the model (default 40)
}

const extractSystemPrompt = `You extract reusable coding "instincts" from a session log.
An instinct is a small, generalizable behavior with a trigger and an action.
Return STRICT JSON: {"instincts":[{"trigger":"when ...","domain":"git|testing|...","action":"...","evidence":["..."]}]}
Rules: max 5 instincts, only patterns seen 2+ times, no project-specific names, no secrets.`

// Extract runs the LLM-backed pattern miner.
func (e LLMExtractor) Extract(ctx context.Context, obs []Observation) ([]Candidate, error) {
	if e.Model == nil {
		return e.fallback().Extract(ctx, obs)
	}
	batch := obs
	if e.MaxObs > 0 && len(batch) > e.MaxObs {
		batch = batch[len(batch)-e.MaxObs:]
	}
	user := renderObservations(batch)

	raw, err := e.Model.Complete(ctx, extractSystemPrompt, user)
	if err != nil {
		return e.fallback().Extract(ctx, obs)
	}
	cands, err := parseCandidatesJSON(raw)
	if err != nil || len(cands) == 0 {
		return e.fallback().Extract(ctx, obs)
	}
	return cands, nil
}

func (e LLMExtractor) fallback() Extractor {
	if e.Fallback != nil {
		return e.Fallback
	}
	return HeuristicExtractor{MinRepeats: 2}
}

func renderObservations(obs []Observation) string {
	var b strings.Builder
	b.WriteString("Session events:\n")
	for i, o := range obs {
		status := "ok"
		if !o.Success {
			status = "failed"
		}
		fmt.Fprintf(&b, "%d. [%s/%s] %s — %s\n", i+1, o.Tool, o.Domain, status, o.Action)
	}
	return b.String()
}

func parseCandidatesJSON(raw string) ([]Candidate, error) {
	raw = strings.TrimSpace(raw)
	// Tolerate code fences the model may add.
	if i := strings.Index(raw, "{"); i > 0 {
		raw = raw[i:]
	}
	if j := strings.LastIndex(raw, "}"); j >= 0 && j < len(raw)-1 {
		raw = raw[:j+1]
	}
	var payload struct {
		Instincts []struct {
			Trigger  string   `json:"trigger"`
			Domain   string   `json:"domain"`
			Action   string   `json:"action"`
			Evidence []string `json:"evidence"`
		} `json:"instincts"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}
	var out []Candidate
	for _, p := range payload.Instincts {
		if strings.TrimSpace(p.Action) == "" {
			continue
		}
		dom := p.Domain
		if dom == "" {
			dom = "general"
		}
		out = append(out, Candidate{
			Trigger:  p.Trigger,
			Domain:   dom,
			Action:   p.Action,
			Evidence: p.Evidence,
		})
	}
	return out, nil
}
