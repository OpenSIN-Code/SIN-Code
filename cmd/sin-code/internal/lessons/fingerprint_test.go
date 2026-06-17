// SPDX-License-Identifier: MIT
// Purpose: Tests for the generic Fingerprint(content string) function
// (issue #340) — 40-hex SHA-1 digest with collision-resistant properties.
package lessons

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
)

// TestFingerprint_Returns40HexSHA1 verifies the fingerprint is exactly
// 40 hex characters (160-bit SHA-1).
func TestFingerprint_Returns40HexSHA1(t *testing.T) {
	fp := Fingerprint("test content")
	if len(fp) != 40 {
		t.Fatalf("expected 40-hex fingerprint, got len=%d: %s", len(fp), fp)
	}
	for _, c := range fp {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("expected lowercase hex, got %c in %s", c, fp)
		}
	}
}

// TestFingerprint_SameContentSameFingerprint verifies determinism: the
// same input always produces the same fingerprint.
func TestFingerprint_SameContentSameFingerprint(t *testing.T) {
	content := "package main\n\nfunc main() {}"
	fp1 := Fingerprint(content)
	fp2 := Fingerprint(content)
	if fp1 != fp2 {
		t.Fatalf("same content produced different fingerprints: %s != %s", fp1, fp2)
	}
}

// TestFingerprint_DifferentContentNoCollision verifies that different
// content strings produce different fingerprints.
func TestFingerprint_DifferentContentNoCollision(t *testing.T) {
	contents := []string{
		"hello world",
		"hello world!",
		"Hello World",
		"hello world\n",
		"",
		" ",
		"\x00",
		"package main",
	}
	seen := make(map[string]string)
	for _, c := range contents {
		fp := Fingerprint(c)
		if prev, ok := seen[fp]; ok {
			t.Fatalf("collision: %q and %q both produced %s", prev, c, fp)
		}
		seen[fp] = c
	}
}

// TestFingerprint_MatchesSHA1 verifies the fingerprint matches a manual
// SHA-1 computation, confirming the algorithm.
func TestFingerprint_MatchesSHA1(t *testing.T) {
	content := "verify me"
	h := sha1.Sum([]byte(content))
	expected := hex.EncodeToString(h[:])
	got := Fingerprint(content)
	if got != expected {
		t.Fatalf("Fingerprint(%q) = %s, expected %s", content, got, expected)
	}
}

// TestFingerprint_EmptyString verifies the function handles empty input.
func TestFingerprint_EmptyString(t *testing.T) {
	fp := Fingerprint("")
	if fp == "" {
		t.Fatal("expected non-empty fingerprint for empty string")
	}
	if len(fp) != 40 {
		t.Fatalf("expected 40-hex for empty string, got len=%d", len(fp))
	}
}

// TestFingerprint_LargeContent verifies the function handles large inputs
// without error.
func TestFingerprint_LargeContent(t *testing.T) {
	content := make([]byte, 1<<20) // 1MB
	for i := range content {
		content[i] = byte(i % 256)
	}
	fp := Fingerprint(string(content))
	if len(fp) != 40 {
		t.Fatalf("expected 40-hex for large content, got len=%d", len(fp))
	}
}

// TestFingerprint_BackwardCompat verifies that LessonFingerprint (the
// existing 64-hex SHA-256 function) still works alongside the new
// 40-hex Fingerprint function. They serve different purposes and
// must coexist without interference.
func TestFingerprint_BackwardCompat(t *testing.T) {
	// LessonFingerprint should still return 64-hex SHA-256.
	lfp := LessonFingerprint(TypeConstraint, "/tmp", map[string]any{"k": "v"})
	if len(lfp) != 64 {
		t.Fatalf("LessonFingerprint should return 64-hex, got len=%d", len(lfp))
	}
	// Fingerprint should return 40-hex SHA-1.
	fp := Fingerprint("canonical content")
	if len(fp) != 40 {
		t.Fatalf("Fingerprint should return 40-hex, got len=%d", len(fp))
	}
	// They should produce different-length outputs.
	if len(lfp) == len(fp) {
		t.Fatal("LessonFingerprint and Fingerprint should have different output lengths")
	}
}

// TestFingerprint_ConcurrentSafety verifies that concurrent calls to
// Fingerprint are race-free (mandate M7).
func TestFingerprint_ConcurrentSafety(t *testing.T) {
	contents := make([]string, 100)
	for i := range contents {
		contents[i] = fmt.Sprintf("content-%d", i)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, c := range contents {
				fp := Fingerprint(c)
				if len(fp) != 40 {
					t.Errorf("expected 40-hex, got len=%d", len(fp))
				}
			}
		}()
	}
	wg.Wait()
}
