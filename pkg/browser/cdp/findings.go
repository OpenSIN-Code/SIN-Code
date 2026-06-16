// SPDX-License-Identifier: MIT
// Purpose: deterministic Findings engine for the CDP ground-truth log.
//
// The gap identified in the audit (gap-report 2024-06-16) was that the existing
// Python diagnostics layer could only filter/query the event stream — the
// classification, grouping, and correlation work was left entirely to the LLM.
// This package closes that gap with a single-pass analyser that produces
// structured, deduplicated Findings from the in-memory event slice, making
// "problems" machine-readable and directly actionable by the agent loop.
//
// Design principles:
//   - Deterministic: given the same event slice, Analyze always returns the
//     same ordered Finding slice. No randomness, no LLM calls.
//   - Grouped by signature: identical errors (e.g. the same exception thrown
//     50 times) collapse into a single Finding with Count > 1.
//   - Sorted by severity then frequency: errors first, then warnings, then
//     info; within each tier, higher-count findings come first.
//   - Causally minimal: each Finding references its first and last seq numbers
//     so downstream consumers can retrieve the full event context from the JSONL.
package cdp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
)

// Severity classifies how urgent a Finding is.
type Severity string

const (
	SevError Severity = "error"
	SevWarn  Severity = "warn"
	SevInfo  Severity = "info"
)

// Finding is a grouped, classified problem derived from the CDP event stream.
// Multiple identical events are collapsed into a single Finding (Count tracks
// how many times the issue was observed).
type Finding struct {
	// Category is the CDP domain family: "console", "exception", "network",
	// "audit", "security", "vital".
	Category string `json:"category"`

	// Severity is the worst-case severity of this class of event.
	Severity Severity `json:"severity"`

	// Title is a short human-readable description of the problem.
	Title string `json:"title"`

	// Signature is the stable key used for grouping and deduplication.
	// It is not intended for display; use Title for that.
	Signature string `json:"signature"`

	// Count is the number of events that matched this Finding's signature.
	Count int `json:"count"`

	// FirstSeq and LastSeq are the global sequence numbers of the first and
	// last matching events. Use them to retrieve the raw event from the JSONL.
	FirstSeq uint64 `json:"first_seq"`
	LastSeq  uint64 `json:"last_seq"`

	// Sample is a short representative excerpt from the first matching event
	// to aid human reading without requiring a JSONL lookup.
	Sample string `json:"sample,omitempty"`
}

// Analyze runs a deterministic single pass over events and returns grouped
// findings sorted by severity (error → warn → info) then count (descending).
//
// Pass rec.Events() directly after a recording session:
//
//	findings := cdp.Analyze(rec.Events())
func Analyze(events []*Event) []*Finding {
	groups := map[string]*Finding{}

	add := func(cat string, sev Severity, title, sig, sample string, seq uint64) {
		f, ok := groups[sig]
		if !ok {
			f = &Finding{
				Category:  cat,
				Severity:  sev,
				Title:     title,
				Signature: sig,
				FirstSeq:  seq,
				Sample:    sample,
			}
			groups[sig] = f
		}
		f.Count++
		f.LastSeq = seq
	}

	for _, e := range events {
		switch {

		// ---- Web Vitals injected by vitals.go (must precede console case) --
		// vitals.go installs a PerformanceObserver script that forwards each
		// metric via console.debug("__SINCDP_VITAL__", ...). We intercept them
		// here before the general consoleAPICalled handler so they are not
		// double-counted as console errors.
		case e.Domain == "Runtime" && e.Method == "consoleAPICalled" && isVital(e.Params):
			name, value, ok := parseVital(e.Params)
			if !ok {
				continue
			}
			if sev, title := vitalSeverity(name, value); sev != "" {
				sig := "vital:" + name
				sample := fmt.Sprintf("%s = %.0fms", name, value)
				if name == "CLS" {
					sample = fmt.Sprintf("%s = %.3f", name, value)
				}
				add("vital", sev, title, sig, sample, e.Seq)
			}

		// ---- Console errors / warnings ------------------------------------
		case e.Domain == "Runtime" && e.Method == "consoleAPICalled":
			var p runtime.EventConsoleAPICalled
			if json.Unmarshal(e.Params, &p) != nil {
				continue
			}
			switch p.Type {
			case runtime.APITypeError, runtime.APITypeAssert:
				msg := consoleText(&p)
				add("console", SevError, "Console error", "console:"+msg, msg, e.Seq)
			case runtime.APITypeWarning:
				msg := consoleText(&p)
				add("console", SevWarn, "Console warning", "console:warning:"+msg, msg, e.Seq)
			}

		// ---- Uncaught JS exceptions ----------------------------------------
		case e.Domain == "Runtime" && e.Method == "exceptionThrown":
			var p runtime.EventExceptionThrown
			if json.Unmarshal(e.Params, &p) != nil {
				continue
			}
			msg := "uncaught exception"
			if p.ExceptionDetails != nil {
				msg = p.ExceptionDetails.Text
				if p.ExceptionDetails.Exception != nil &&
					p.ExceptionDetails.Exception.Description != "" {
					msg = p.ExceptionDetails.Exception.Description
				}
			}
			add("exception", SevError, "Uncaught exception", "exc:"+firstLine(msg), msg, e.Seq)

		// ---- Network failures (request blocked or connection error) --------
		case e.Domain == "Network" && e.Method == "loadingFailed":
			var p network.EventLoadingFailed
			if json.Unmarshal(e.Params, &p) != nil {
				continue
			}
			sev := SevError
			var sig, title string
			if p.BlockedReason != "" {
				sig = "netblock:" + string(p.BlockedReason)
				title = fmt.Sprintf("Request blocked: %s", p.BlockedReason)
			} else {
				sig = fmt.Sprintf("netfail:%s:%s", p.Type, p.ErrorText)
				title = fmt.Sprintf("Request failed (%s)", p.ErrorText)
			}
			add("network", sev, title, sig, p.ErrorText, e.Seq)

		// ---- HTTP error responses (4xx warn, 5xx error) --------------------
		case e.Domain == "Network" && e.Method == "responseReceived":
			var p network.EventResponseReceived
			if json.Unmarshal(e.Params, &p) != nil || p.Response == nil {
				continue
			}
			if p.Response.Status >= 400 {
				sev := SevWarn
				if p.Response.Status >= 500 {
					sev = SevError
				}
				sig := fmt.Sprintf("http:%d", p.Response.Status)
				add("network", sev,
					fmt.Sprintf("HTTP %d response", p.Response.Status),
					sig, p.Response.URL, e.Seq)
			}

		// ---- DevTools Audits Issues (CORS, CSP, mixed content, ...) --------
		// Chrome pre-classifies these; we surface the issue code as a Finding
		// so the agent can act on it without parsing the full audit payload.
		case e.Domain == "Audits" && e.Method == "issueAdded":
			var raw struct {
				Issue struct {
					Code string `json:"code"`
				} `json:"issue"`
			}
			if json.Unmarshal(e.Params, &raw) != nil {
				continue
			}
			code := raw.Issue.Code
			if code == "" {
				code = "UnknownIssue"
			}
			add("audit", SevWarn, "DevTools issue: "+code, "audit:"+code, code, e.Seq)

		// ---- Security state degradation ------------------------------------
		case e.Domain == "Security" && e.Method == "securityStateChanged":
			var raw struct {
				SecurityState         string `json:"securityState"`
				SchemeIsCryptographic bool   `json:"schemeIsCryptographic"`
			}
			if json.Unmarshal(e.Params, &raw) != nil {
				continue
			}
			if raw.SecurityState == "insecure" || raw.SecurityState == "neutral" {
				add("security", SevWarn,
					"Insecure security state: "+raw.SecurityState,
					"security:"+raw.SecurityState,
					raw.SecurityState, e.Seq)
			}
		}
	}

	out := make([]*Finding, 0, len(groups))
	for _, f := range groups {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		ri, rj := severityRank(out[i].Severity), severityRank(out[j].Severity)
		if ri != rj {
			return ri < rj
		}
		return out[i].Count > out[j].Count
	})
	return out
}

// severityRank maps Severity to a sort key (lower = more urgent).
func severityRank(s Severity) int {
	switch s {
	case SevError:
		return 0
	case SevWarn:
		return 1
	default:
		return 2
	}
}

// consoleText extracts the first meaningful string from a console event's args.
func consoleText(p *runtime.EventConsoleAPICalled) string {
	for _, a := range p.Args {
		if a == nil {
			continue
		}
		if a.Value != nil {
			return firstLine(string(a.Value))
		}
		if a.Description != "" {
			return firstLine(a.Description)
		}
	}
	return string(p.Type)
}

// firstLine returns up to the first newline of s, capped at 200 characters.
func firstLine(s string) string {
	for i, c := range s {
		if c == '\n' {
			if i > 200 {
				return s[:200]
			}
			return s[:i]
		}
	}
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

// ---------------------------------------------------------------------------
// Web Vitals helpers (used by Analyze above and by vitals.go)
// ---------------------------------------------------------------------------

// isVital is a cheap pre-check before full unmarshal: returns true when the
// consoleAPICalled payload carries one of our injected __SINCDP_VITAL__ tags.
func isVital(params json.RawMessage) bool {
	return bytes.Contains(params, []byte("__SINCDP_VITAL__"))
}

// parseVital extracts the metric name and float value from the tagged console
// call emitted by vitals.go. The page logs:
//
//	console.debug("__SINCDP_VITAL__", JSON.stringify({name, value, ...}))
func parseVital(params json.RawMessage) (string, float64, bool) {
	var p runtime.EventConsoleAPICalled
	if json.Unmarshal(params, &p) != nil || len(p.Args) < 2 {
		return "", 0, false
	}
	// Args[1] is the stringified JSON payload.
	var rawStr string
	if json.Unmarshal(p.Args[1].Value, &rawStr) != nil {
		return "", 0, false
	}
	var v struct {
		Name  string  `json:"name"`
		Value float64 `json:"value"`
	}
	if json.Unmarshal([]byte(rawStr), &v) != nil || v.Name == "" {
		return "", 0, false
	}
	return v.Name, v.Value, true
}

// vitalSeverity applies Core Web Vitals "poor" / "needs-improvement" thresholds.
// Returns an empty severity string when the metric is in the "good" range.
func vitalSeverity(name string, value float64) (Severity, string) {
	switch name {
	case "LCP": // ms — good <2500, poor >4000
		if value > 4000 {
			return SevError, "Poor LCP (largest contentful paint)"
		}
		if value > 2500 {
			return SevWarn, "LCP needs improvement"
		}
	case "CLS": // unitless — good <0.1, poor >0.25
		if value > 0.25 {
			return SevError, "Poor CLS (cumulative layout shift)"
		}
		if value > 0.1 {
			return SevWarn, "CLS needs improvement"
		}
	case "INP": // ms — good <200, poor >500
		if value > 500 {
			return SevError, "Poor INP (interaction to next paint)"
		}
		if value > 200 {
			return SevWarn, "INP needs improvement"
		}
	case "LongTask": // ms — anything >50ms blocks the main thread
		if value > 200 {
			return SevWarn, "Long task blocking the main thread"
		}
		if value > 50 {
			return SevInfo, "Long task on the main thread"
		}
	}
	return "", ""
}
