// SPDX-License-Identifier: MIT
// Purpose: tests for the scout regex/glob helpers.

package internal

import (
	"os"
	"regexp"
	"testing"
)

func TestCompileQuery_Regex(t *testing.T) {
	re, err := compileQuery("foo.*bar", "regex")
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("foo-bar") {
		t.Fatal("expected match")
	}
}

func TestCompileQuery_InvalidRegex(t *testing.T) {
	if _, err := compileQuery("[unclosed", "regex"); err == nil {
		t.Fatal("expected error for unclosed bracket")
	}
}

func TestCompileQuery_Semantic(t *testing.T) {
	re, err := compileQuery("hello world", "semantic")
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("hello there, world!") {
		t.Fatal("expected match for both words")
	}
}

func TestCompileQuery_Usage(t *testing.T) {
	re, err := compileQuery("hello", "usage")
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("say hello to me") {
		t.Fatal("expected usage match for whole word")
	}
	if re.MatchString("hellos") {
		t.Fatal("usage must be whole-word, hellos is not hello")
	}
}

func TestGitignoreGlobToRegex_Wildcard(t *testing.T) {
	re := gitignoreGlobToRegex("*.log")
	if re == nil {
		t.Fatal("nil regex")
	}
	if !re.MatchString("foo.log") {
		t.Fatal("expected *.log to match foo.log")
	}
	if re.MatchString("foo.txt") {
		t.Fatal("*.log must not match foo.txt")
	}
}

func TestGitignoreGlobToRegex_Exact(t *testing.T) {
	re := gitignoreGlobToRegex("build/")
	if !re.MatchString("build/") {
		t.Fatal("expected exact match for build/")
	}
}

func TestGitignoreMatcher_LoadAndMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/.gitignore",
		[]byte("*.log\nbuild\n# a comment\n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := loadGitignore(dir)
	if m == nil {
		t.Fatal("nil matcher")
	}
	if !m.matchFile("foo.log") {
		t.Fatal(".log must be ignored")
	}
	if !m.matchFile("build") {
		t.Fatal("build must be ignored")
	}
	if m.matchFile("foo.txt") {
		t.Fatal("foo.txt must NOT be ignored")
	}
}

var _ = regexp.MustCompile
