// SPDX-License-Identifier: MIT
// Purpose: poc requirements — extracts requirements from spec documents and
// renders Proof-of-Correctness text output.
// sin-debt: shrink, upgrade: when a second requirements-related function is needed, merge into a shared file
package internal

import (
	"fmt"
	"regexp"
	"strings"
)

type requirement struct {
	Name        string
	Type        string // function, class, method, import
	Description string
}

type symbolLocation struct {
	Name string
	Type string
	File string
	Line int
}

// pocStopwords are common English / spec-prose words that must never be
// treated as required symbol names. This prevents natural-language specs
// ("The Hello() function must return ...") from producing bogus requirements
// like "must" or "Spec" (dogfooding bug st-bug1 #3).
var pocStopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "not": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"being": true, "have": true, "has": true, "had": true, "do": true,
	"does": true, "did": true, "will": true, "would": true, "should": true,
	"shall": true, "must": true, "may": true, "might": true, "can": true,
	"could": true, "if": true, "then": true, "else": true, "when": true,
	"where": true, "that": true, "this": true, "these": true, "those": true,
	"it": true, "its": true, "with": true, "for": true, "from": true,
	"to": true, "in": true, "on": true, "at": true, "by": true, "of": true,
	"as": true, "return": true, "returns": true, "returning": true,
	"function": true, "functions": true, "method": true, "methods": true,
	"class": true, "classes": true, "struct": true, "structs": true,
	"type": true, "types": true, "interface": true, "interfaces": true,
	"spec": true, "specs": true, "specification": true, "requirement": true,
	"requirements": true, "string": true, "int": true, "bool": true,
	"float": true, "error": true, "true": true, "false": true, "nil": true,
	"null": true, "none": true, "void": true, "all": true, "any": true,
	"each": true, "no": true, "side": true, "effects": true, "value": true,
	"values": true,
}

func extractRequirements(content string) []requirement {
	var reqs []requirement
	if content == "" {
		return reqs
	}

	seen := make(map[string]bool)
	add := func(name, desc string) {
		if name == "" || seen[name] || pocStopwords[strings.ToLower(name)] {
			return
		}
		seen[name] = true
		reqs = append(reqs, requirement{Name: name, Type: "symbol", Description: desc})
	}

	// 1. Function-call references: `Hello()`, processOrder(args), REQ-1: hello().
	//    An identifier immediately followed by "(" is the strongest signal a
	//    spec is naming a concrete callable.
	callRe := regexp.MustCompile("[`\"']?([a-zA-Z_][a-zA-Z0-9_]*)\\s*\\(")
	for _, m := range callRe.FindAllStringSubmatch(content, -1) {
		add(m[1], m[0])
	}

	// 2. Keyword-introduced symbols: "must implement X", "requires X",
	//    "function X", "class X", etc. Articles ("a", "the") and a chained
	//    kind keyword ("define type Config") are skipped so the regex lands
	//    on the actual identifier instead of a filler word.
	re := regexp.MustCompile(`(?i)(?:must\s+(?:implement|have|define|call)|requires?|should\s+(?:have|define|implement)|function|method|class|struct|type|interface)\s+(?:(?:a|an|the)\s+)?(?:(?:function|method|class|struct|type|interface)\s+)?[` + "`" + `"']?([a-zA-Z_][a-zA-Z0-9_]*)[` + "`" + `"']?`)
	for _, match := range re.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			add(match[1], match[0])
		}
	}

	// 2b. Identifier-before-keyword: "The `Hello` function" (natural prose).
	//     Only quoted/backticked identifiers are considered, to avoid
	//     false positives on bare prose like "the main function".
	preRe := regexp.MustCompile("(?i)[`\"']([a-zA-Z_][a-zA-Z0-9_]*)[`\"']\\s+(?:function|method|class|struct|type|interface|module)")
	for _, match := range preRe.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			name := match[1]
			// Reject single lowercase words like "hello" from prose.
			if !isLikelyCodeName(name) {
				continue
			}
			add(name, match[0])
		}
	}

	// 3. Code blocks in markdown are treated as embedded specs.
	codeRe := regexp.MustCompile("```[a-z]*\n([^`]+)\n```")
	for _, block := range codeRe.FindAllStringSubmatch(content, -1) {
		if len(block) > 1 {
			for _, req := range pocExtractRequirementsCodeBlock(block[1]) {
				if !seen[req.Name] {
					seen[req.Name] = true
					reqs = append(reqs, req)
				}
			}
		}
	}

	return reqs
}

// isLikelyCodeName returns true if name looks like a real code identifier
// (has uppercase, underscore, hyphen, or dot). Rejects single lowercase
// prose words like "hello" / "world".
func isLikelyCodeName(name string) bool {
	if len(name) == 0 {
		return false
	}
	hasUpper := false
	hasSep := false
	for _, c := range name {
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
		if c == '_' || c == '-' || c == '.' {
			hasSep = true
		}
	}
	return hasUpper || hasSep
}

func outputTextPOC(result *pocResult) error {
	fmt.Printf("Proof-of-Correctness\n")
	fmt.Printf("Spec:     %s\n", result.Spec)
	fmt.Printf("Code:     %s\n", result.Code)
	fmt.Printf("Coverage: %.1f%% (%d/%d passed)\n\n", result.Coverage, result.Passed, result.Passed+result.Failed)

	if len(result.Checks) > 0 {
		fmt.Printf("Checks (%d):\n", len(result.Checks))
		for _, check := range result.Checks {
			icon := "?"
			switch check.Status {
			case "pass":
				icon = "✓"
			case "fail":
				icon = "✗"
			case "warn":
				icon = "▲"
			}
			loc := ""
			if check.File != "" {
				loc = fmt.Sprintf(" (%s:%d)", check.File, check.Line)
			}
			fmt.Printf("  %s [%s] %s: %s%s\n", icon, check.Type, check.Name, check.Message, loc)
		}
	}
	fmt.Printf("\n%s\n", result.Summary)
	return nil
}
