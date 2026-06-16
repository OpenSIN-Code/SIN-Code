// SPDX-License-Identifier: MIT
// Purpose: argument parsing + placeholder substitution for slash commands.
// Supports ECC-style $ARGUMENTS, $1..$9, $@, and ${flag}.
// Docs: args.doc.md
package dispatch

import "strings"

// Args holds parsed command arguments: positional + the raw remainder.
type Args struct {
	Positional []string          // $1, $2, ...
	Flags      map[string]string // --key value / --key=value
	Raw        string            // everything after the command name ($ARGUMENTS)
}

// ParseArgs splits a raw argument string into positional args and
// flags. Quoting with double quotes is respected for positional
// values.
func ParseArgs(raw string) Args {
	a := Args{Flags: map[string]string{}, Raw: strings.TrimSpace(raw)}
	tokens := tokenize(a.Raw)
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if strings.HasPrefix(tok, "--") {
			key := strings.TrimPrefix(tok, "--")
			if eq := strings.IndexByte(key, '='); eq >= 0 {
				a.Flags[key[:eq]] = key[eq+1:]
				continue
			}
			// look ahead for a value that isn't another flag
			if i+1 < len(tokens) && !strings.HasPrefix(tokens[i+1], "--") {
				a.Flags[key] = tokens[i+1]
				i++
			} else {
				a.Flags[key] = "true"
			}
			continue
		}
		a.Positional = append(a.Positional, tok)
	}
	return a
}

// Substitute replaces ECC-style placeholders in a command body:
//
//	$ARGUMENTS -> the full raw argument string
//	$1, $2 ... -> positional arguments (empty string if absent)
//	$@         -> all positional args joined by space
//	${flag}    -> flag value (empty if unset)
func (a Args) Substitute(body string) string {
	out := body
	out = strings.ReplaceAll(out, "$ARGUMENTS", a.Raw)
	out = strings.ReplaceAll(out, "$@", strings.Join(a.Positional, " "))

	// $1..$9 (descending so $10 isn't clobbered by $1)
	for i := 9; i >= 1; i-- {
		ph := "$" + itoa(i)
		val := ""
		if i-1 < len(a.Positional) {
			val = a.Positional[i-1]
		}
		out = strings.ReplaceAll(out, ph, val)
	}

	// ${flag}
	for k, v := range a.Flags {
		out = strings.ReplaceAll(out, "${"+k+"}", v)
	}
	return out
}

func tokenize(s string) []string {
	var tokens []string
	var b strings.Builder
	inQuote := false
	flush := func() {
		if b.Len() > 0 {
			tokens = append(tokens, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
