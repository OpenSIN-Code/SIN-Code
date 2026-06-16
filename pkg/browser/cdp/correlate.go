// SPDX-License-Identifier: MIT
// Purpose: root-cause correlation — links failing network requests to the
// downstream exceptions and console errors they caused.
//
// Correlate is a companion to Analyze: where Analyze produces a flat,
// deduplicated list of problems, Correlate produces causal chains that answer
// "why did this happen?" for the agent loop without requiring an LLM to read
// raw JSONL.
package cdp

import (
	"encoding/json"
	"fmt"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
)

// Chain links a likely root-cause event (a failed or errored network request)
// to downstream symptoms (exceptions and console errors) that occurred within
// window sequence steps after it. This turns "50 red lines" into one
// actionable story: failing request → exception → console errors.
type Chain struct {
	RootCategory string    `json:"root_category"` // always "network"
	RootTitle    string    `json:"root_title"`
	RootSeq      uint64    `json:"root_seq"`
	RootURL      string    `json:"root_url,omitempty"`
	Symptoms     []Symptom `json:"symptoms"`
}

// Symptom is a downstream event attributed to a Chain's root cause.
type Symptom struct {
	Category string `json:"category"` // "exception" | "console"
	Seq      uint64 `json:"seq"`
	Message  string `json:"message"`
}

// Correlate scans the event stream and builds root-cause chains.
// window is how many sequence steps after a failure we still attribute
// symptoms to it — lower values are stricter, higher values catch more
// indirect effects on dense pages. A value of 25 works well for most pages.
func Correlate(events []*Event, window uint64) []*Chain {
	type sym struct {
		seq      uint64
		category string
		message  string
	}
	var symptoms []sym
	for _, e := range events {
		switch {
		case e.Domain == "Runtime" && e.Method == "exceptionThrown":
			var p runtime.EventExceptionThrown
			if json.Unmarshal(e.Params, &p) == nil && p.ExceptionDetails != nil {
				symptoms = append(symptoms, sym{e.Seq, "exception", firstLine(p.ExceptionDetails.Text)})
			}
		case e.Domain == "Runtime" && e.Method == "consoleAPICalled":
			var p runtime.EventConsoleAPICalled
			if json.Unmarshal(e.Params, &p) == nil &&
				(p.Type == runtime.APITypeError || p.Type == runtime.APITypeAssert) {
				symptoms = append(symptoms, sym{e.Seq, "console", consoleText(&p)})
			}
		}
	}

	attach := func(rootSeq uint64) []Symptom {
		var out []Symptom
		for _, s := range symptoms {
			if s.seq > rootSeq && s.seq <= rootSeq+window {
				out = append(out, Symptom{Category: s.category, Seq: s.seq, Message: s.message})
			}
		}
		return out
	}

	var chains []*Chain
	for _, e := range events {
		var rootTitle, rootURL string
		isRoot := false

		switch {
		case e.Domain == "Network" && e.Method == "loadingFailed":
			var p network.EventLoadingFailed
			if json.Unmarshal(e.Params, &p) == nil {
				isRoot = true
				rootTitle = "Request failed: " + p.ErrorText
				if p.BlockedReason != "" {
					rootTitle = "Request blocked: " + string(p.BlockedReason)
				}
			}
		case e.Domain == "Network" && e.Method == "responseReceived":
			var p network.EventResponseReceived
			if json.Unmarshal(e.Params, &p) == nil && p.Response != nil && p.Response.Status >= 400 {
				isRoot = true
				rootTitle = fmt.Sprintf("HTTP %d response", p.Response.Status)
				rootURL = p.Response.URL
			}
		}

		if !isRoot {
			continue
		}
		if syms := attach(e.Seq); len(syms) > 0 {
			chains = append(chains, &Chain{
				RootCategory: "network",
				RootTitle:    rootTitle,
				RootSeq:      e.Seq,
				RootURL:      rootURL,
				Symptoms:     syms,
			})
		}
	}
	return chains
}
