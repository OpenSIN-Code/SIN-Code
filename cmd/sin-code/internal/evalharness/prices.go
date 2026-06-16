// SPDX-License-Identifier: MIT
// Purpose: per-arm self-pricing for the three-arm eval comparator
// (issue #171). The comparator emits a markdown matrix whose USD
// column is computed from prompt + completion token counts; without
// a price book we couldn't compare the per-arm cost of running the
// harness. Prices are kept inline because:
//
//	(a) we are byte-stable (the comparator must NOT change USD
//	    numbers between runs with identical input),
//	(b) we are dependency-free (mandate M2 — single static binary,
//	    no network calls during eval),
//	(c) the price book is tiny and rarely changes.
//
// Reserved key:
//
//	"stub"      — 0 USD / token; the default for offline + CI runs.
//	"gpt-4o"    — 2026 Q1 OpenAI list price (issue #168 contract).
//	"gpt-4o-mini"
//	"claude-3.5-sonnet"
//	"fireworks-qwen2.5-7b"
//	"fireworks-llama-3.1-70b"
//
// To add a model, append to the table. The comparator keys into the
// table via Arm.PricingName (set by the CLI from --model-pricing).
//
// Docs: prices.doc.md
package evalharness

// Price is the prompt + completion price per 1k tokens in USD.
type Price struct {
	PromptPer1k     float64
	CompletionPer1k float64
}

// Prices is the canonical price book. ARM.PricingName picks an entry.
// Zero-value entries are valid — they produce USD=0 and warn at
// load-time via CompareReport.Warnings so the user can spot when
// they accidentally passed an unrecognised model.
var Prices = map[string]Price{
	"stub":                    {PromptPer1k: 0, CompletionPer1k: 0},
	"gpt-4o":                  {PromptPer1k: 0.0025, CompletionPer1k: 0.01},
	"gpt-4o-mini":             {PromptPer1k: 0.00015, CompletionPer1k: 0.0006},
	"claude-3.5-sonnet":       {PromptPer1k: 0.003, CompletionPer1k: 0.015},
	"fireworks-qwen2.5-7b":    {PromptPer1k: 0.0002, CompletionPer1k: 0.0002},
	"fireworks-llama-3.1-70b": {PromptPer1k: 0.0009, CompletionPer1k: 0.0009},
}

// PriceOf returns the Price for name, or the "stub" price when the
// name is missing. The boolean reports whether the name was known —
// the comparator appends a warning into CompareReport.Warnings when
// false so an unrecognised model doesn't silently produce USD=0.
func PriceOf(name string) (Price, bool) {
	if name == "" {
		return Prices["stub"], true
	}
	p, ok := Prices[name]
	if !ok {
		return Price{}, false
	}
	return p, true
}

// Cost returns the USD cost of (promptTokens, completionTokens) at
// the given Price. Tokens < 0 are clamped to 0. The result is
// rounded to 6 decimals so the snapshot diffs cleanly across runs.
func Cost(p Price, promptTokens, completionTokens int) float64 {
	if promptTokens < 0 {
		promptTokens = 0
	}
	if completionTokens < 0 {
		completionTokens = 0
	}
	usd := float64(promptTokens)/1000.0*p.PromptPer1k + float64(completionTokens)/1000.0*p.CompletionPer1k
	// Round to 6 decimals so 0.0000123 stays stable across runs.
	usd = float64(int64(usd*1_000_000+0.5)) / 1_000_000
	return usd
}
