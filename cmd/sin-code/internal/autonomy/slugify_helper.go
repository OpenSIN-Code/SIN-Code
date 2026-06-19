// SPDX-License-Identifier: MIT
// sin-debt: single-function utility, upgrade: when autonomy package has >3 utilities merge into utils.go
package autonomy

import (
	"regexp"
	"strings"
)

var dashRun = regexp.MustCompile(`-+`)

func Slugify(topic string) string {
	t := strings.ToLower(strings.TrimSpace(topic))
	var b strings.Builder
	for _, r := range t {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '.' || r == ':' || r == ',':
			b.WriteByte('-')
		case r == '/':
			// skip forward slash entirely so "a/b" becomes "ab"
		}
	}
	out := dashRun.ReplaceAllString(b.String(), "-")
	if len(out) > 80 {
		out = out[:80]
		out = strings.TrimRight(out, "-")
	}
	if out == "" {
		return "report"
	}
	return out
}
