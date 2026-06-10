// SPDX-License-Identifier: MIT
// Purpose: tests for hashline anchoring, atomic write, and anchored edit.
package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLineHashStableAcrossTrailingWhitespace(t *testing.T) {
	if LineHash("foo()") != LineHash("foo()  \t") {
		t.Fatal("trailing whitespace must not change the hash")
	}
	if LineHash("  foo()") == LineHash("foo()") {
		t.Fatal("leading whitespace (indentation) must change the hash")
	}
}

func TestParseAnchor(t *testing.T) {
	a, err := ParseAnchor("12:" + LineHash("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if a.Line != 12 {
		t.Fatalf("want line 12, got %d", a.Line)
	}
	for _, bad := range []string{"", "12", ":abcd1234", "0:abcd1234", "x:abcd1234", "5:tooshor"} {
		if _, err := ParseAnchor(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestResolveAnchorWithDrift(t *testing.T) {
	lines := []string{"a", "b", "target", "d"}
	a := Anchor{Line: 1, Hash: LineHash("target")}
	idx, drift, err := ResolveAnchor(lines, a, DefaultDriftWindow)
	if err != nil {
		t.Fatal(err)
	}
	if idx != 2 || drift != 2 {
		t.Fatalf("want idx=2 drift=2, got idx=%d drift=%d", idx, drift)
	}
	if _, _, err := ResolveAnchor(lines, Anchor{Line: 2, Hash: "deadbeef"}, 5); err == nil {
		t.Fatal("expected error for unresolvable anchor")
	}
}

func TestWriteFileAtomicValidatesGo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	if _, err := writeFileAtomic(path, "package x\nfunc broken( {", writeOpts{validate: true}); err == nil {
		t.Fatal("expected validation error for broken Go")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed validation must not leave a file behind")
	}
	if _, err := writeFileAtomic(path, "package x\n\nfunc OK() {}\n", writeOpts{validate: true}); err != nil {
		t.Fatal(err)
	}
}

func TestApplyEditAnchorReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0644)

	res, err := applyEdit(path, editRequest{
		Anchor:  "2:" + LineHash("two"),
		NewText: "TWO",
		Drift:   DefaultDriftWindow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.LineDelta != 0 {
		t.Fatalf("want delta 0, got %d", res.LineDelta)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "one\nTWO\nthree\n" {
		t.Fatalf("unexpected content: %q", data)
	}
}

func TestApplyEditStringModeAmbiguity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	os.WriteFile(path, []byte("x\nx\n"), 0644)

	if _, err := applyEdit(path, editRequest{OldString: "x", NewString: "y", Drift: 1}); err == nil {
		t.Fatal("ambiguous match must fail without --replace-all")
	}
	if _, err := applyEdit(path, editRequest{OldString: "x", NewString: "y", ReplaceAll: true, Drift: 1}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "x") {
		t.Fatalf("replace-all incomplete: %q", data)
	}
}

func TestApplyEditRejectsSyntaxBreak(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.go")
	orig := "package x\n\nfunc OK() {}\n"
	os.WriteFile(path, []byte(orig), 0644)

	_, err := applyEdit(path, editRequest{
		Anchor:   "3:" + LineHash("func OK() {}"),
		NewText:  "func Broken( {",
		Validate: true,
		Drift:    DefaultDriftWindow,
	})
	if err == nil {
		t.Fatal("expected syntax rejection")
	}
	data, _ := os.ReadFile(path)
	if string(data) != orig {
		t.Fatal("rejected edit must not modify the file")
	}
}
