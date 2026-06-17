// SPDX-License-Identifier: MIT
// Purpose: #264 — full splice round-trip with json.Number / time.Time /
// []byte. Confirms the generator renders these types end-to-end in the
// generated test source.
package testgen

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSplice_EdgeCasesRoundTrip(t *testing.T) {
	src := `package x

import "time"

func At(t time.Time, n int64, data []byte) (string, error) { _ = t; _ = n; _ = data; return "", nil }
`
	dir := t.TempDir()
	srcFile := filepath.Join(dir, "x.go")
	if err := os.WriteFile(srcFile, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	tm, _ := time.Parse(time.RFC3339, "2026-06-17T12:34:56Z")
	cases := map[string][]TestCase{
		"At": {
			{
				Name: "givenTime",
				Args: map[string]any{
					"t":    tm,
					"n":    json.Number("42"),
					"data": []byte("hi"),
				},
				Wants: map[string]any{"got": "ok"},
			},
		},
	}
	out, err := generateFallback(context.Background(), srcFile, nil, cases)
	if err != nil {
		t.Fatalf("generateFallback: %v", err)
	}
	for _, want := range []string{
		`time.Date(2026, time.June, 17, 12, 34, 56, 0, time.UTC)`,
		`[]byte("hi")`,
		`n: 42,`,
		`wantGot: "ok",`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("generated test missing %q.\n--- output ---\n%s", want, out)
		}
	}
}
