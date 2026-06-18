// SPDX-License-Identifier: MIT
package autonomy

import (
	"strings"
	"unicode"
)

func Slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) { b.WriteRune(r) } else if r == ' ' || r == '_' || r == '.' { b.WriteByte('-') }
	}
	r := strings.Trim(b.String(), "-")
	if r == "" { r = "report" }
	return r
}
