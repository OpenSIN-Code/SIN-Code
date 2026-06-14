// SPDX-License-Identifier: MIT
// Purpose: parse program.md — the single human-edited file that defines the
// autonomous objective, success metric, budget, and hard invariants.
// Mirrors autoresearch's program.md and autodev-cli's config parser.
package autopilot

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// Direction is the optimization direction for the metric.
type Direction string

const (
	Minimize Direction = "minimize"
	Maximize Direction = "maximize"
)

// Program is the parsed program.md.
type Program struct {
	Objective      string         // free-text high-level goal
	MetricName     string         // e.g. "bench_ns_per_op"
	Direction      Direction      // minimize | maximize
	ExtractRegex   *regexp.Regexp // captures the metric value from verify output
	BudgetMinutes  int            // wall-clock cap (M4)
	MaxExperiments int            // experiment cap (M4)
	Invariants     []string       // DO-NOT-MODIFY constraints, injected read-only
	Raw            string         // original file content
}

// DefaultProgram returns conservative defaults used when a field is omitted.
func DefaultProgram() Program {
	return Program{
		Direction:      Minimize,
		BudgetMinutes:  60,
		MaxExperiments: 12,
	}
}

// LoadProgram reads and parses program.md at path.
func LoadProgram(path string) (*Program, error) {
	data, err := os.ReadFile(path) // #nosec G304
	if err != nil {
		return nil, fmt.Errorf("autopilot: read program.md: %w", err)
	}
	p := DefaultProgram()
	p.Raw = string(data)

	var section string
	var objective strings.Builder
	sc := bufio.NewScanner(strings.NewReader(p.Raw))
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)

		if h := headingOf(trimmed); h != "" {
			section = strings.ToLower(h)
			continue
		}
		switch section {
		case "objective":
			if trimmed != "" {
				objective.WriteString(trimmed)
				objective.WriteByte('\n')
			}
		case "metric":
			parseMetricLine(&p, trimmed)
		case "budget":
			parseBudgetLine(&p, trimmed)
		case "invariants", "invariants (do not modify)":
			if item := bulletOf(trimmed); item != "" {
				p.Invariants = append(p.Invariants, item)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	p.Objective = strings.TrimSpace(objective.String())
	if p.Objective == "" {
		return nil, fmt.Errorf("autopilot: program.md has no # Objective section")
	}
	return &p, nil
}

func parseMetricLine(p *Program, line string) {
	key, val, ok := keyVal(line)
	if !ok {
		return
	}
	switch key {
	case "name":
		p.MetricName = val
	case "direction":
		if val == string(Maximize) {
			p.Direction = Maximize
		} else {
			p.Direction = Minimize
		}
	case "extract":
		expr := strings.Trim(val, "/")
		if re, err := regexp.Compile(expr); err == nil {
			p.ExtractRegex = re
		}
	}
}

func parseBudgetLine(p *Program, line string) {
	key, val, ok := keyVal(line)
	if !ok {
		return
	}
	n, err := strconv.Atoi(strings.Fields(val)[0])
	if err != nil {
		return
	}
	switch key {
	case "minutes":
		p.BudgetMinutes = n
	case "max_experiments":
		p.MaxExperiments = n
	}
}

// headingOf returns the heading text for "# H" / "## H" lines, else "".
func headingOf(line string) string {
	if !strings.HasPrefix(line, "#") {
		return ""
	}
	return strings.TrimSpace(strings.TrimLeft(line, "#"))
}

// bulletOf returns the item text for "- x" / "* x" lines, else "".
func bulletOf(line string) string {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return strings.TrimSpace(line[2:])
	}
	return ""
}

// keyVal parses "key: value" (case-insensitive key).
func keyVal(line string) (string, string, bool) {
	i := strings.Index(line, ":")
	if i < 0 {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(line[:i])), strings.TrimSpace(line[i+1:]), true
}

// InvariantBriefing renders invariants as a read-only prompt block.
func (p *Program) InvariantBriefing() string {
	if len(p.Invariants) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("HARD INVARIANTS (DO NOT MODIFY, violating these fails the experiment):\n")
	for _, inv := range p.Invariants {
		b.WriteString("- ")
		b.WriteString(inv)
		b.WriteByte('\n')
	}
	return b.String()
}
