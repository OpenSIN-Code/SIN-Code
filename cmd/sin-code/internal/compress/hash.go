// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when compaction is refactored
//
// Hashing and JSON helpers — planHash computes a content-addressed
// SHA-256 over the canonical (Keeps + Drops + Merges) ordering;
// the JSON wrappers keep encoding/json in one place.
package compress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"sort"
	"strings"
)

// planHash returns SHA-256 of the canonical (Keeps + Drops + Merges)
// ordering. Two plans with the same content produce the same hash.
func planHash(p Plan) string {
	// We serialize a deterministic projection: Subject+Hash+Bytes
	// for keeps, Subject+Hash+Bytes for drops, Sources+Body+Bytes
	// for merges. No timestamps; no CreatedAt.
	var b strings.Builder
	w := func(s string) { b.WriteString(s); b.WriteByte('\n') }
	sort.SliceStable(p.Keeps, func(i, j int) bool { return p.Keeps[i].Hash < p.Keeps[j].Hash })
	for _, k := range p.Keeps {
		w(string(k.Target) + "\x00" + k.Hash + "\x00" + itoa(k.Bytes))
	}
	sort.SliceStable(p.Drops, func(i, j int) bool { return p.Drops[i].Hash < p.Drops[j].Hash })
	for _, k := range p.Drops {
		w(string(k.Target) + "\x00" + k.Hash + "\x00" + itoa(k.Bytes))
	}
	sort.SliceStable(p.Merges, func(i, j int) bool { return p.Merges[i].ID < p.Merges[j].ID })
	for _, m := range p.Merges {
		sort.Strings(m.SourceHashes)
		w(m.ID + "\x00" + string(m.Strategy) + "\x00" + strings.Join(m.SourceHashes, ",") + "\x00" + itoa(m.Bytes))
	}
	h := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(h[:])
}

// itoa is a stdlib-free small-int printer used by planHash. Negative
// values are not expected.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

// jsonMarshalIndent is a tiny wrapper to avoid pulling encoding/json
// twice. Kept as a separate function to make it easy to replace with
// a streaming encoder if a future caller needs it.
func jsonMarshalIndent(v any) ([]byte, error) {
	return jsonIndent(v, "", "  ")
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

func jsonIndent(v any, prefix, indent string) ([]byte, error) {
	return indentingMarshal(v, prefix, indent)
}

// sin-debt: shrink, upgrade: inline when wiring is consolidated
// indentingMarshal pulls in encoding/json once; indentingMarshal
// is a single-line wrapper because we want to keep the wiring
// paralell to lessons.Marshalled-style injection sites.
func indentingMarshal(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// ensure imports are used.
var _ = io.Discard
