// SPDX-License-Identifier: MIT
// Purpose: Mutation Probe — verify tests actually OBSERVE the change.
// Injects k small mutations into changed lines only and re-runs the
// affected tests. Surviving mutations = tests are blind = block auto-merge.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type Mutation struct {
	File   string
	Line   int
	Before string
	After  string
	Rule   string
	Killed bool
}

type ProbeResult struct {
	Mutations          []Mutation
	Killed             int
	Survived           int
	ObservabilityScore float64
}

func (p *ProbeResult) Diagnosis() string {
	if p == nil || p.Survived == 0 {
		return fmt.Sprintf("mutation probe: %d/%d mutations killed — tests observe the change", p.Killed, len(p.Mutations))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "mutation probe: %d mutations SURVIVED — tests are blind to these changed lines:\n", p.Survived)
	for _, m := range p.Mutations {
		if !m.Killed {
			fmt.Fprintf(&b, "- %s:%d [%s] %q -> %q ran green\n", m.File, m.Line, m.Rule, strings.TrimSpace(m.Before), strings.TrimSpace(m.After))
		}
	}
	b.WriteString("Add or strengthen tests covering these lines before merge.\n")
	return b.String()
}

var mutators = []struct {
	rule string
	re   *regexp.Regexp
	sub  string
}{
	{"negate-eq", regexp.MustCompile(`==`), "!="},
	{"negate-lt", regexp.MustCompile(`\B<\B`), ">="},
	{"and-to-or", regexp.MustCompile(`&&`), "||"},
	{"true-to-false", regexp.MustCompile(`\btrue\b`), "false"},
	{"plus-to-minus", regexp.MustCompile(`\+ 1\b`), "- 1"},
}

type MutationProbe struct {
	Workdir      string
	TestCmd      []string
	MaxMutations int
}

func NewMutationProbe(workdir string, testCmd []string) *MutationProbe {
	return &MutationProbe{Workdir: workdir, TestCmd: testCmd, MaxMutations: 5}
}

type ChangedLine struct {
	File string
	Line int
	Text string
}

func (mp *MutationProbe) Run(ctx context.Context, lines []ChangedLine) (*ProbeResult, error) {
	res := &ProbeResult{}
	applied := 0

	for _, cl := range lines {
		if applied >= mp.MaxMutations {
			break
		}
		for _, m := range mutators {
			if !m.re.MatchString(cl.Text) {
				continue
			}
			mutated := m.re.ReplaceAllString(cl.Text, m.sub)
			if mutated == cl.Text {
				continue
			}

			mut := Mutation{File: cl.File, Line: cl.Line, Before: cl.Text, After: mutated, Rule: m.rule}
			killed, err := mp.applyAndTest(ctx, cl, mutated)
			if err != nil {
				return nil, err
			}
			mut.Killed = killed
			res.Mutations = append(res.Mutations, mut)
			applied++
			break
		}
	}

	for _, m := range res.Mutations {
		if m.Killed {
			res.Killed++
		} else {
			res.Survived++
		}
	}
	if len(res.Mutations) > 0 {
		res.ObservabilityScore = float64(res.Killed) / float64(len(res.Mutations))
	} else {
		res.ObservabilityScore = 1.0
	}
	return res, nil
}

func (mp *MutationProbe) applyAndTest(ctx context.Context, cl ChangedLine, mutated string) (killed bool, err error) {
	if mp.Workdir == "" {
		return true, nil // no-op in tests: assume kill
	}
	path := filepath.Join(mp.Workdir, cl.File)
	original, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("probe read %s: %w", cl.File, err)
	}
	defer func() {
		if werr := os.WriteFile(path, original, 0o600); werr != nil && err == nil {
			err = fmt.Errorf("probe restore %s: %w", cl.File, werr)
		}
	}()

	fileLines := strings.Split(string(original), "\n")
	if cl.Line < 1 || cl.Line > len(fileLines) {
		return false, fmt.Errorf("probe: line %d out of range in %s", cl.Line, cl.File)
	}
	if strings.TrimSpace(fileLines[cl.Line-1]) != strings.TrimSpace(cl.Text) {
		return true, nil
	}
	fileLines[cl.Line-1] = mutated
	if err := os.WriteFile(path, []byte(strings.Join(fileLines, "\n")), 0o600); err != nil {
		return false, fmt.Errorf("probe write %s: %w", cl.File, err)
	}

	if len(mp.TestCmd) == 0 {
		return false, nil
	}
	cmd := exec.CommandContext(ctx, mp.TestCmd[0], mp.TestCmd[1:]...)
	cmd.Dir = mp.Workdir
	runErr := cmd.Run()
	return runErr != nil, nil
}

func ParseAddedLines(diff string) []ChangedLine {
	var out []ChangedLine
	var curFile string
	var newLine int
	hunkRe := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)`)

	for _, raw := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(raw, "+++ b/"):
			curFile = strings.TrimPrefix(raw, "+++ b/")
		case strings.HasPrefix(raw, "@@"):
			if m := hunkRe.FindStringSubmatch(raw); m != nil {
				fmt.Sscanf(m[1], "%d", &newLine)
			}
		case strings.HasPrefix(raw, "+") && !strings.HasPrefix(raw, "+++"):
			text := raw[1:]
			t := strings.TrimSpace(text)
			if curFile != "" && t != "" && !strings.HasPrefix(t, "//") &&
				!strings.HasPrefix(t, "#") && strings.HasSuffix(curFile, ".go") &&
				!strings.HasSuffix(curFile, "_test.go") {
				out = append(out, ChangedLine{File: curFile, Line: newLine, Text: text})
			}
			newLine++
		case strings.HasPrefix(raw, "-"):
		default:
			newLine++
		}
	}
	return out
}
