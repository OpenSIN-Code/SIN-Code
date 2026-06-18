// SPDX-License-Identifier: MIT
// Purpose: coverage tests for the v3.22.0 sin-analyse-suite permission
// defaults at permission_defaults.go:64-66. The suite is documented as
// read-only multimodal preprocessing (image, video, PDF, logs, data, audio);
// every analyse__* tool MUST resolve to "allow" so the agent loop never
// interrupts a pure read. This file is the byte-stable regression guard.
package internal

import (
	"path"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/permission"
)

// analyseAllowTools is the canon of concrete analyse__* tool names whose
// policy resolution MUST be Allow under DefaultPermissionRules(). The set
// covers every modality called out in the comment at
// permission_defaults.go:64-66.
var analyseAllowTools = []string{
	"analyse__image_extract", // image
	"analyse__video_extract", // video
	"analyse__pdf_parse",     // PDF
	"analyse__log_analyze",   // logs
	"analyse__data_detect",   // data
	"analyse__audio_probe",   // audio
}

// TestAnalyseToolsAllowed verifies each concrete analyse__* tool resolves
// to Allow through the engine (not just by direct rule inspection). This
// catches regressions where someone reorders/inserts a rule above
// analyse__* that would tie-break first.
func TestAnalyseToolsAllowed(t *testing.T) {
	rules := DefaultPermissionRules()
	eng := permission.New(rules)
	for _, tool := range analyseAllowTools {
		got := eng.Check(tool)
		if got != permission.Allow {
			t.Errorf("%s: expected Allow, got %s", tool, got)
		}
	}
}

// TestAnalyseWildcardMatch verifies the analyse__* glob covers every tool
// name with the analyse__ prefix, including future additions the engine
// knows nothing about yet. Uses path.Match directly to keep the test
// independent of any specific suite member.
func TestAnalyseWildcardMatch(t *testing.T) {
	positive := []string{
		"analyse__x",
		"analyse__image_extract",
		"analyse__video_thumbnail",
		"analyse__pdf_parse",
		"analyse__log_analyze",
		"analyse__data_detect",
		"analyse__audio_probe",
		"analyse__deeply_nested_module_substep",
		// Go path.Match semantics: `*` matches the empty string too,
		// so a bare `analyse__` (no suffix) is also covered.
		// The engine does not lowercase the separator before matching,
		// so the double underscore is preserved on both sides.
		"analyse__",
		// Case-insensitivity: the engine lowercases both sides before
		// path.Match (permission.go:99); the rule must still cover
		// mixed-case writes if any caller is sloppy.
		"Analyse__Mixed",
		"ANALYSE__UPPER",
	}
	for _, tool := range positive {
		ok, err := path.Match(strings.ToLower("analyse__*"), strings.ToLower(tool))
		if err != nil {
			t.Fatalf("path.Match(%q): %v", tool, err)
		}
		if !ok {
			t.Errorf("analyse__* expected to match %q", tool)
		}
	}
}

// TestAnalyseNonMatchingTool verifies the analyse__* glob does NOT bleed
// into unrelated tool names. A tool that merely contains "analyse" but
// not the analyse__ server__tool separator must fall through to the
// backstop `*` -> ask rule (which becomes Deny in headless mode per
// permission.go:167-169). This is the M4 invariant: prefix-with-glob
// must not grant unintended access.
func TestAnalyseNonMatchingTool(t *testing.T) {
	nonMatching := []string{
		"notanalyse__x",
		"analyse_other",      // missing double underscore separator
		"foo_analyse__bar",   // analyse__ not at the start
		"analyse",            // bare prefix, no separator
		"pre_analyse__post",  // analyse__ embedded mid-string
		"ANALYZE__X",         // wrong separator characters
		"analyse__/x",        // slash is path.Match separator — excluded
	}
	for _, tool := range nonMatching {
		ok, err := path.Match(strings.ToLower("analyse__*"), strings.ToLower(tool))
		if err != nil {
			t.Fatalf("path.Match(%q): %v", tool, err)
		}
		if ok {
			t.Errorf("analyse__* should NOT match %q", tool)
		}
	}

	// Round-trip through the full engine: a non-matching tool must NOT
	// resolve to Allow. The engine's resolveRules returns Ask as the
	// final value (the backstop `*` rule at permission_defaults.go:144
	// matches Ask), and Check() then applies the Headless fallback to
	// produce Deny in headless mode. We assert both ends of that path:
	// never Allow (M4 invariant) and Deny under Headless (default
	// runtime posture for the daemon and CI).
	rules := DefaultPermissionRules()
	eng := permission.New(rules)
	eng.Headless = true
	sample := "notanalyse__x"
	if got := eng.Check(sample); got == permission.Allow {
		t.Errorf("%s unexpectedly resolved to Allow — analyse__* leaked", sample)
	}
	if got := eng.Check(sample); got != permission.Deny {
		t.Errorf("%s under Headless: expected Deny, got %s", sample, got)
	}
}

// TestAnalyseReadOnlyInvariant is the cardinal read-only guarantee:
// no analyse__* tagged rule may ever have policy ask or deny, because
// the suite is documented as never modifying input files
// (permission_defaults.go:64-66). Any future rule insertion that
// narrows the wildcard (e.g. analyse__image_extract -> ask) trips this
// test and forces an explicit review.
func TestAnalyseReadOnlyInvariant(t *testing.T) {
	rules := DefaultPermissionRules()
	for _, r := range rules {
		// Catch every variant by glob: the literal `"analyse__*"` rule
		// and any future, narrower rule like `"analyse__x"` or
		// `"analyse__image_*"`. Keep the loop simple so reviewers
		// don't have to touch it when new prefixes ship.
		if !strings.HasPrefix(strings.ToLower(r.Tool), "analyse__") {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(r.Policy)) {
		case "allow":
			// ok — read-only invariant holds
		default:
			t.Errorf(
				"analyse rule %q has policy %q; sin-analyse-suite is documented as read-only (permission_defaults.go:64-66); expected allow",
				r.Tool, r.Policy,
			)
		}
	}
}
