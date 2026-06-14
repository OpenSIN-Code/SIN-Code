// SPDX-License-Identifier: MIT
// Purpose: extract a numeric metric from verify-command output and decide
// whether a new measurement is an improvement (the autoresearch core idea:
// keep-if-better, revert-otherwise).
package autopilot

import (
	"math"
	"regexp"
	"strconv"
)

// Measurement is a single metric reading from one experiment.
type Measurement struct {
	Value float64 // parsed metric value
	Found bool    // whether the regex matched
	Raw   string  // the raw captured substring
}

// ExtractMetric runs the program's extract regex over verify output.
// If no regex is configured, Found is false (pass/fail-only mode).
func ExtractMetric(re *regexp.Regexp, output string) Measurement {
	if re == nil {
		return Measurement{Found: false}
	}
	m := re.FindStringSubmatch(output)
	if len(m) < 2 {
		return Measurement{Found: false}
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return Measurement{Found: false, Raw: m[1]}
	}
	return Measurement{Value: v, Found: true, Raw: m[1]}
}

// Improved reports whether candidate beats best given the direction.
// When best is not yet set (NaN), any found candidate is an improvement.
func Improved(dir Direction, best, candidate float64) bool {
	if math.IsNaN(best) {
		return true
	}
	if dir == Maximize {
		return candidate > best
	}
	return candidate < best
}

// BetterOf returns the better of two values for the direction.
func BetterOf(dir Direction, a, b float64) float64 {
	if math.IsNaN(a) {
		return b
	}
	if math.IsNaN(b) {
		return a
	}
	if dir == Maximize {
		return math.Max(a, b)
	}
	return math.Min(a, b)
}

// NoMetric is the sentinel "unset" best value.
func NoMetric() float64 { return math.NaN() }
