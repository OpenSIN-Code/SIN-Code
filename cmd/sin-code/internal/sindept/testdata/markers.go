// SPDX-License-Identifier: MIT
// Purpose: real golden-file fixtures for the sindept package scanner.
// The marker comments below are the source of truth - any reflow must
// preserve the exact byte offsets that TestParseGolden asserts.
package fixtures

// sin-debt: global mutex, upgrade: per-account locks when throughput > 1k req/s
// sin-debt: O(n²) scan, upgrade: switch to map lookup when n > 100
// sin-debt: hand-rolled retry, upgrade: use cenkalti/backoff when context cancellation matters
// sin-debt: this exists
func FixturesAreAtTopOfFile() {}

const (
	FixtureLineA = 9
	FixtureLineB = 10
	FixtureLineC = 11
	FixtureLineD = 12
)
