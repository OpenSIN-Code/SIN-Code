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
	return s, nil
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

// parseCriterion extracts an optional "An:" id and a trailing "`verify: cmd`".
func parseCriterion(item string, n int) Criterion {
	c := Criterion{ID: fmt.Sprintf("A%d", n)}
	if id, rest, ok := stripID(item); ok {
		c.ID, item = id, rest
	}
	if i := strings.Index(item, "verify:"); i >= 0 {
		c.Verify = strings.TrimSpace(strings.Trim(item[i+len("verify:"):], " `"))
		item = strings.TrimRight(item[:i], " `")
	}
	c.Text = strings.TrimSpace(item)
	return c
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
