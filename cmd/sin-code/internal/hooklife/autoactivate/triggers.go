// SPDX-License-Identifier: MIT
// Purpose: tiny TOML-ish parser for `.sin-code/autoactivate.toml` and
// the `trigger:` field inside a skill frontmatter. SIN-Code's existing
// config parser is dependency-free and rejects complex shapes; the
// autoactivate file is small enough that a hand-rolled scanner is
// cheaper than dragging in a TOML library (mandate M2: single static
// binary, CGO=0).
//
// Supported grammar:
//
//	# comment
//	[rule]                # exactly one rule per file in v1; multi-rule
//	name      = "compact" # keys live in this section
//	body      = "be terse"
//	trigger   = "/compact"
//	no_trigger = false
//	[default]             # optional one-time defaults
//	auto_on    = false
//	no_trigger = false
//
// Keys may be unquoted (bare identifier) or quoted with "...".
// Booleans are `true`/`false`. Missing keys default to the zero value.
// Comments use `#` outside of quoted strings.
// Docs: triggers.doc.md
package autoactivate

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// Default carries the section-table defaults a project wants applied
// to every session opened in that directory.
type Default struct {
	AutoOn    bool // when true, every session opens with auto-activation enabled
	NoTrigger bool // when true, prompt-based triggers are ignored
}

// LoadFile parses a single `.sin-code/autoactivate.toml` file. Returns
// an empty RuleSet and zero Default when the file does not exist;
// never errors for a missing file (privacy-first: silent, opt-in by
// presence).
func LoadFile(path string) (RuleSet, Default, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RuleSet{}, Default{}, nil
		}
		return nil, Default{}, err
	}
	defer f.Close()
	return parse(f)
}

// parse scans r with an internal state struct so no package-level
// globals are touched (cleaner testing + concurrency hygiene).
func parse(r io.Reader) (RuleSet, Default, error) {
	rs := RuleSet{}
	state := &parserState{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(stripComment(sc.Text()))
		if line == "" {
			continue
		}
		if line[0] == '[' {
			name, ok := stripSection(line)
			if !ok {
				continue
			}
			// entering a new section flushes any half-parsed [rule]
			state.flushRule(rs)
			state.section = name
			continue
		}
		k, v, ok := splitKV(line)
		if !ok {
			continue
		}
		state.handle(rs, k, v)
	}
	if err := sc.Err(); err != nil {
		return nil, state.def, err
	}
	state.flushRule(rs)
	return rs, state.def, nil
}

type parserState struct {
	section   string
	def       Default
	name      string
	body      string
	trigger   string
	noTrigger bool
}

func (p *parserState) handle(rs RuleSet, k, v string) {
	switch p.section {
	case "rule":
		switch k {
		case "name":
			// Entering a new (different) name flushes any prior rule
			// so the parser supports multi-rule files like:
			//
			//			[rule]
			//			name = "alpha"
			//			body = "first"
			//
			//			[rule]
			//			name = "beta"
			//			body = "second"
			//
			// Note: a second `name = "x"` inside the SAME [rule] block
			// without an intervening section is treated as a flush+new
			// (last-write-wins on body).
			if v != p.name && p.name != "" {
				p.flushRule(rs)
			}
			p.name = v
		case "body":
			p.body = v
		case "trigger":
			p.trigger = v
		case "no_trigger":
			p.noTrigger = parseBool(v)
		}
	case "default":
		switch k {
		case "auto_on":
			p.def.AutoOn = parseBool(v)
		case "no_trigger":
			p.def.NoTrigger = parseBool(v)
		}
	}
}

func (p *parserState) flushRule(rs RuleSet) {
	if p.name == "" {
		return
	}
	r := Rule{
		Name:      p.name,
		Body:      p.body,
		Trigger:   p.trigger,
		NoTrigger: p.noTrigger,
	}
	rs.Add(r)
	p.name = ""
	p.body = ""
	p.trigger = ""
	p.noTrigger = false
}

// stripComment removes everything from `#` onward when `#` is outside
// a quoted string. Our grammar does not allow `#` inside quoted
// values; this is a deliberate simplification.
func stripComment(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			for j := i + 1; j < len(s); j++ {
				if s[j] == '"' {
					i = j
					break
				}
			}
			continue
		}
		if s[i] == '#' {
			return s[:i]
		}
	}
	return s
}

// stripSection extracts the section name from `[name]`. Empty names
// return ok=false and the caller silently skips them.
func stripSection(line string) (string, bool) {
	if len(line) < 3 || line[len(line)-1] != ']' {
		return "", false
	}
	name := strings.TrimSpace(line[1 : len(line)-1])
	if name == "" {
		return "", false
	}
	return name, true
}

// splitKV parses `key = value` (whitespace flexible). Returns ok=false
// for malformed lines; caller silently skips them.
func splitKV(line string) (string, string, bool) {
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(line[:eq])
	v := strings.TrimSpace(line[eq+1:])
	v = trimQuotes(v)
	if k == "" {
		return "", "", false
	}
	return k, v, true
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func parseBool(s string) bool {
	switch strings.ToLower(s) {
	case "true", "yes", "1":
		return true
	}
	return false
}
