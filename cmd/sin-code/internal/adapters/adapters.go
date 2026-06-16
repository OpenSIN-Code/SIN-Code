// SPDX-License-Identifier: MIT
// Purpose: concrete adapters that implement the abstract hooklife /
// instinct interfaces against SIN-Code's real packages. Each file
// in this package owns ONE adapter.
// Docs: adapters.doc.md
package adapters

import (
	"context"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// VerifyGate adapts the real verify.Gate to the hooklife.Verifier
// interface. Uses the gate's Run(ctx, workdir) -> Result contract
// (passed bool, report string, err error).
type VerifyGate struct {
	Gate *verify.Gate
}

// QualityGate implements hooklife.Verifier.
func (a VerifyGate) QualityGate(ctx context.Context, workdir string) (bool, string, error) {
	if a.Gate == nil {
		return true, "", nil
	}
	res := a.Gate.Run(ctx, workdir)
	return res.Passed, res.Report, nil
}

// MemoryBridge adapts the real memory.Store to the
// instinct.MemorySink interface. Each instinct observation is
// stored as a Memory{Insight, Tags=[domain]}.
type MemoryBridge struct {
	Store *memory.Store
}

// RecordInstinct implements instinct.MemorySink.
func (b MemoryBridge) RecordInstinct(_ context.Context, trigger, action, domain string, confidence float64) error {
	if b.Store == nil {
		return nil
	}
	return b.Store.Add(&memory.Memory{
		Insight: "instinct: " + trigger + " -> " + action,
		Tags:    []string{"instinct", domain, "confidence:" + ftoa2(confidence)},
	})
}

// BackgroundCompleter adapts the LLM client to instinct.Completer.
// Uses the supplied cheap model alias (e.g. an Anthropic Haiku
// alias) for mining.
type BackgroundCompleter struct {
	Client *llm.Client
	Model  string
}

// Complete implements instinct.Completer. Never returns an error —
// the LLMExtractor falls back to the heuristic pass on any failure.
func (c BackgroundCompleter) Complete(ctx context.Context, system, user string) (string, error) {
	if c.Client == nil {
		return "", nil
	}
	model := c.Model
	if model == "" {
		model = "anthropic/claude-haiku-4-5"
	}
	resp, err := c.Client.Chat(ctx, llm.ChatRequest{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.ExtractText(), nil
}

func ftoa2(f float64) string {
	// 2-decimal float → "0.45" / "0.90"
	whole := int(f)
	frac := int((f-float64(whole))*100+0.5) * 1
	if frac < 0 {
		frac = -frac
	}
	s := intToStr(frac)
	if len(s) < 2 {
		s = "0" + s
	}
	return intToStr(whole) + "." + s
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// containsAny is a small helper used by tests and adapters alike.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
