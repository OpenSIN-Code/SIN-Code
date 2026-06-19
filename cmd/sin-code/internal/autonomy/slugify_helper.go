// SPDX-License-Identifier: MIT
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
		case r == ' ' || r == '_' || r == '/' || r == '.' || r == ':' || r == ',':
			b.WriteByte('-')
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
