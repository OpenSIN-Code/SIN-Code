// SPDX-License-Identifier: MIT
// Purpose: the Spec-Layer (issue #122). A Spec is a single human-edited
// *.spec.md file that captures, between free-text objective and machine-
// checkable acceptance criteria, the contract a change must satisfy. It is
// the bridge between intent (what a human wants) and verification (what the
// agent/CI can check). Parsing mirrors autopilot/program.go for consistency.
//
// Spec-Layer pipeline (Phase 1 implemented here):
//
//	author spec.md  ->  parse  ->  validate  ->  (autopilot/agent consumes)
//
// Docs: docs/spec-layer.md
package spec

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Priority ranks a requirement.
type Priority string

const (
	Must   Priority = "must"
	Should Priority = "should"
	May    Priority = "may"
)

// Requirement is one normative statement parsed from "# Requirements".
type Requirement struct {
	ID       string   // stable id, e.g. "R1" (auto-assigned if absent)
	Text     string   // the requirement statement
	Priority Priority // must | should | may (default: must)
}

// Criterion is one acceptance check parsed from "# Acceptance Criteria".
// When Verify is set it is a shell command whose zero exit means "passed".
type Criterion struct {
	ID     string // stable id, e.g. "A1"
	Text   string // human description
	Verify string // optional shell command to check the criterion
}

// Spec is a parsed *.spec.md document.
type Spec struct {
	ID           string // stable identifier (filename-safe slug of the title)
	Title        string
	Objective    string
	Requirements []Requirement
	Criteria     []Criterion
	Invariants   []string // DO-NOT-MODIFY constraints
	Path         string
	Raw          string
}

// Load reads and parses a spec file at path.
func Load(path string) (*Spec, error) {
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return nil, fmt.Errorf("spec: read %s: %w", path, err)
	}
	s, err := Parse(string(data))
	if err != nil {
		return nil, err
	}
	s.Path = path
	return s, nil
}

// Parse parses raw *.spec.md content into a Spec.
func Parse(raw string) (*Spec, error) {
	s := &Spec{Raw: raw}
	var section string
	var objective strings.Builder
	reqN, accN := 0, 0

	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		if level, h := headingOf(trimmed); h != "" {
			if level == 1 && s.Title == "" && !isKnownSection(h) {
				s.Title = h
				continue
			}
			section = canonicalSection(h)
			continue
		}

		switch section {
		case "objective":
			if trimmed != "" {
				objective.WriteString(trimmed)
				objective.WriteByte('\n')
			}
		case "requirements":
			if item := bulletOf(trimmed); item != "" {
				reqN++
				s.Requirements = append(s.Requirements, parseRequirement(item, reqN))
			}
		case "acceptance":
			if item := bulletOf(trimmed); item != "" {
				accN++
				s.Criteria = append(s.Criteria, parseCriterion(item, accN))
			}
		case "invariants":
			if item := bulletOf(trimmed); item != "" {
				s.Invariants = append(s.Invariants, item)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	s.Objective = strings.TrimSpace(objective.String())
	if s.ID == "" {
		s.ID = slugID(s.Title)
	}
	return s, nil
}

// slugID returns a filename-safe identifier derived from a title.
// Lower-cases, replaces non-alphanumerics with '-', trims leading/
// trailing dashes, and falls back to "spec" if the result is empty.
func slugID(title string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		return "spec"
	}
	if len(id) > 60 {
		id = id[:60]
	}
	return id
}

// Marshal renders a Spec back to its canonical Markdown form. The
// output is what `sin spec author` writes via the --out flag and
// what a hand-edited *.spec.md file should look like.
func Marshal(s *Spec) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", s.Title)
	if s.Objective != "" {
		fmt.Fprintf(&b, "## Objective\n\n%s\n\n", s.Objective)
	}
	if len(s.Requirements) > 0 {
		b.WriteString("## Requirements\n\n")
		for _, r := range s.Requirements {
			prio := string(r.Priority)
			if prio == "" {
				prio = "must"
			}
			fmt.Fprintf(&b, "- [%s] %s: %s\n", prio, r.ID, r.Text)
		}
		b.WriteString("\n")
	}
	if len(s.Criteria) > 0 {
		b.WriteString("## Acceptance Criteria\n\n")
		for _, c := range s.Criteria {
			if c.Verify != "" {
				fmt.Fprintf(&b, "- %s: %s  `verify: %s`\n", c.ID, c.Text, c.Verify)
			} else {
				fmt.Fprintf(&b, "- %s: %s\n", c.ID, c.Text)
			}
		}
		b.WriteString("\n")
	}
	if len(s.Invariants) > 0 {
		b.WriteString("## Invariants\n\n")
		for _, inv := range s.Invariants {
			fmt.Fprintf(&b, "- %s\n", inv)
		}
		b.WriteString("\n")
	}
	return []byte(b.String()), nil
}

// parseRequirement extracts an optional "[must]/[should]/[may]" prefix and a
// leading "Rn:" id, assigning a default id/priority otherwise.
func parseRequirement(item string, n int) Requirement {
	r := Requirement{Priority: Must, ID: fmt.Sprintf("R%d", n)}
	item, r.Priority = stripPriority(item, r.Priority)
	if id, rest, ok := stripID(item); ok {
		r.ID, item = id, rest
	}
	r.Text = strings.TrimSpace(item)
	return r
}

// parseCriterion extracts an optional "An:" id and a trailing
// "`verify: cmd`". The verify annotation must be backtick-wrapped
// (e.g. "`verify: go test ./...`") so the parser can distinguish
// it from a literal "verify:" in the criterion's text. Unwrapped
// occurrences of "verify:" are left in the text.
func parseCriterion(item string, n int) Criterion {
	c := Criterion{ID: fmt.Sprintf("A%d", n)}
	if id, rest, ok := stripID(item); ok {
		c.ID, item = id, rest
	}
	if cmd, ok := extractVerify(item); ok {
		c.Verify = cmd
		item = strings.TrimRight(item[:strings.LastIndex(item, "`verify:")], " `")
	}
	c.Text = strings.TrimSpace(item)
	return c
}

// extractVerify finds the last backtick-wrapped `verify: <cmd>` in
// item. Returns the trimmed command and true on success, "" and
// false otherwise. The backtick requirement prevents the parser
// from misinterpreting prose like "verify: the parser works" as
// a verify-command.
func extractVerify(item string) (string, bool) {
	const open = "`verify:"
	const close = "`"
	i := strings.LastIndex(item, open)
	if i < 0 {
		return "", false
	}
	rest := item[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:j]), true
}

func stripPriority(item string, def Priority) (string, Priority) {
	lower := strings.ToLower(item)
	for _, p := range []Priority{Must, Should, May} {
		// bracket form: "[must] ..."
		if tag := "[" + string(p) + "]"; strings.HasPrefix(lower, tag) {
			return strings.TrimSpace(item[len(tag):]), p
		}
		// bare form: "Must: ..." / "May ..." (word boundary required)
		w := string(p)
		if strings.HasPrefix(lower, w) {
			rest := item[len(w):]
			if rest != "" && (rest[0] == ':' || rest[0] == ' ') {
				return strings.TrimSpace(strings.TrimPrefix(rest, ":")), p
			}
		}
	}
	return item, def
}

// stripID detects a leading "ID:" token (e.g. "R3: ...", "A2 - ...").
func stripID(item string) (string, string, bool) {
	for _, sep := range []string{":", " -", "-"} {
		i := strings.Index(item, sep)
		if i <= 0 || i > 6 {
			continue
		}
		id := strings.TrimSpace(item[:i])
		if len(id) >= 2 && (id[0] == 'R' || id[0] == 'A') && isDigits(id[1:]) {
			return id, strings.TrimSpace(item[i+len(sep):]), true
		}
	}
	return "", item, false
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func canonicalSection(h string) string {
	switch strings.ToLower(strings.TrimSpace(h)) {
	case "objective", "goal", "summary":
		return "objective"
	case "requirements", "requirement":
		return "requirements"
	case "acceptance criteria", "acceptance", "criteria", "done when":
		return "acceptance"
	case "invariants", "invariants (do not modify)", "constraints":
		return "invariants"
	default:
		return ""
	}
}

func isKnownSection(h string) bool { return canonicalSection(h) != "" }

// headingOf returns (level, text) for markdown headings, else (0, "").
func headingOf(line string) (int, string) {
	if !strings.HasPrefix(line, "#") {
		return 0, ""
	}
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	return level, strings.TrimSpace(line[level:])
}

func bulletOf(line string) string {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return strings.TrimSpace(line[2:])
	}
	return ""
}
