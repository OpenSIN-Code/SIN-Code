// SPDX-License-Identifier: MIT
// Purpose: rule-based Fix Suggestions from Findings and Chains.
//
// Suggest maps each Finding to a FixSuggest that names the root cause,
// describes the remediation action, and tags it with a FixClass so the agent
// loop can route it to the right auto-fix handler without LLM interpretation.
//
// Design contract:
//   - Deterministic: same input → same output.
//   - Conservative: never blindly edits code. Points at likely cause + class.
//   - FixClass is the machine-readable routing tag (e.g. "security.cors").
//   - Confidence reflects how reliable the mapping is ("high" | "medium" | "low").
package cdp

import "strings"

// FixSuggest is a deterministic, rule-based hint the agent can act on. It ties
// back to a Finding via Signature and adds a FixClass routing tag and a
// human-readable Cause + Action pair.
type FixSuggest struct {
	// Signature matches the Finding this suggestion was derived from.
	Signature string `json:"signature"`

	// Severity mirrors the parent Finding's severity.
	Severity Severity `json:"severity"`

	// Cause is a one-sentence description of the likely root cause.
	Cause string `json:"cause"`

	// Action is a concrete description of what the agent should do to fix it.
	Action string `json:"action"`

	// FixClass is a dot-namespaced machine tag used by the agent loop to route
	// the fix to the right handler (code edit, config change, endpoint check…).
	FixClass string `json:"fix_class"`

	// Confidence is "high" | "medium" | "low" reflecting how reliable the
	// cause/action mapping is for this finding class.
	Confidence string `json:"confidence"`
}

// Suggest maps findings (and the context from chains) to remediation hints.
// chains is currently used for context but future rules can weight suggestions
// by chain membership.
func Suggest(findings []*Finding, _ []*Chain) []*FixSuggest {
	var out []*FixSuggest
	for _, f := range findings {
		if s := classify(f); s != nil {
			out = append(out, s)
		}
	}
	return out
}

func classify(f *Finding) *FixSuggest {
	sig := f.Signature
	low := strings.ToLower(f.Sample)

	switch {
	case strings.HasPrefix(sig, "vital:"):
		return vitalSuggest(f, strings.TrimPrefix(sig, "vital:"))

	case strings.HasPrefix(sig, "audit:"):
		return auditSuggest(f, strings.TrimPrefix(sig, "audit:"))

	case strings.HasPrefix(sig, "netblock:"):
		return &FixSuggest{sig, f.Severity,
			"A request was blocked by the browser (CSP, mixed content, or interception).",
			"Inspect the blockedReason in the JSONL; adjust CSP/headers or upgrade the resource to https.",
			"network.blocked", "high"}

	case strings.HasPrefix(sig, "http:"):
		return &FixSuggest{sig, f.Severity,
			"Server returned a 4xx/5xx for a fetched resource.",
			"Check the request URL, auth, and payload against the responseReceived entry; fix the endpoint or client call.",
			"network.http_error", "medium"}

	case strings.HasPrefix(sig, "netfail:"):
		return &FixSuggest{sig, f.Severity,
			"A network request failed before completing (DNS, TLS, abort, or timeout).",
			"Read errorText in loadingFailed; verify host/cert/connectivity or add retry semantics.",
			"network.transport", "medium"}

	case strings.HasPrefix(sig, "exc:"):
		fc, conf := "js.exception", "medium"
		if strings.Contains(low, "is not defined") || strings.Contains(low, "is not a function") {
			fc, conf = "js.reference", "high"
		} else if strings.Contains(low, "cannot read properties") || strings.Contains(low, "undefined") {
			fc, conf = "js.null_access", "high"
		}
		return &FixSuggest{sig, f.Severity,
			"Uncaught JavaScript exception in page or component code.",
			"Locate the source from the exception stack trace and guard the access or fix the symbol.",
			fc, conf}

	case strings.HasPrefix(sig, "console:") && f.Severity == SevError:
		return &FixSuggest{sig, f.Severity,
			"Application logged a console error.",
			"Trace the log call site; treat as a symptom and correlate with nearby exceptions or network failures.",
			"console.error", "low"}

	case strings.HasPrefix(sig, "security:"):
		return &FixSuggest{sig, f.Severity,
			"Page security state degraded to insecure or neutral.",
			"Ensure all resources are served over https and that no mixed-content requests are made.",
			"security.state", "medium"}
	}
	return nil
}

func auditSuggest(f *Finding, code string) *FixSuggest {
	switch code {
	case "ContentSecurityPolicyIssue":
		return &FixSuggest{f.Signature, SevWarn,
			"A Content-Security-Policy directive was violated.",
			"Adjust the CSP header to allow the legitimate source, or remove the offending inline/eval usage.",
			"security.csp", "high"}
	case "MixedContentIssue":
		return &FixSuggest{f.Signature, SevWarn,
			"Page loaded an insecure (http) subresource over https.",
			"Upgrade the resource URL to https or use a protocol-relative/secure host.",
			"security.mixed_content", "high"}
	case "CorsIssue":
		return &FixSuggest{f.Signature, SevWarn,
			"A cross-origin request was blocked by CORS.",
			"Set the correct Access-Control-Allow-* headers on the server, or proxy the request.",
			"security.cors", "high"}
	case "DeprecationIssue":
		return &FixSuggest{f.Signature, SevInfo,
			"Page uses a deprecated web platform feature.",
			"Migrate off the deprecated API noted in the issue payload.",
			"compat.deprecation", "medium"}
	case "CookieIssue":
		return &FixSuggest{f.Signature, SevWarn,
			"A cookie was rejected or flagged (SameSite/Secure missing).",
			"Set the SameSite and Secure attributes appropriately on the cookie.",
			"security.cookie", "high"}
	default:
		return &FixSuggest{f.Signature, SevWarn,
			"DevTools reported an issue: " + code,
			"Read the full issue payload in the JSONL and remediate per its details.",
			"audit.generic", "low"}
	}
}

func vitalSuggest(f *Finding, name string) *FixSuggest {
	switch name {
	case "LCP":
		return &FixSuggest{f.Signature, f.Severity,
			"The largest content element renders too late.",
			"Preload the LCP image/font, reduce render-blocking JS/CSS, and serve the hero from a fast/cached source.",
			"perf.lcp", "medium"}
	case "CLS":
		return &FixSuggest{f.Signature, f.Severity,
			"Visible elements shift after load, hurting layout stability.",
			"Set explicit width/height on images/embeds and reserve space for late-loading content.",
			"perf.cls", "medium"}
	case "INP":
		return &FixSuggest{f.Signature, f.Severity,
			"Interactions respond slowly due to main-thread work.",
			"Break up long tasks, defer non-critical JS, and debounce heavy event handlers.",
			"perf.inp", "medium"}
	case "LongTask":
		return &FixSuggest{f.Signature, f.Severity,
			"A long task blocked the main thread.",
			"Split the work with scheduling APIs (yield/requestIdleCallback) or move it to a worker.",
			"perf.longtask", "low"}
	}
	return nil
}
