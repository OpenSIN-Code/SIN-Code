// SPDX-License-Identifier: MIT
package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSplitArgumentsQuotesAndMetacharacters(t *testing.T) {
	got, err := splitArguments(`--path "dir with space" value; touch /tmp/never`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"--path", "dir with space", "value;", "touch", "/tmp/never"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitArguments() = %#v, want %#v", got, want)
	}
}

func TestSplitArgumentsSupportsSingleQuotesAndEscapes(t *testing.T) {
	got, err := splitArguments(`--path 'two words' plain\ value`)
	if err != nil {
		t.Fatalf("splitArguments: %v", err)
	}
	want := []string{"--path", "two words", "plain value"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitArguments() = %#v, want %#v", got, want)
	}
}

func TestBuildArgvSeparatesStaticCommandAndPromptArguments(t *testing.T) {
	m := &Model{}
	got, err := m.buildArgv(Command{Key: "code review"}, `"a b.py"; echo nope`)
	if err != nil {
		t.Fatalf("buildArgv: %v", err)
	}
	want := []string{"sin", "code", "review", "a b.py;", "echo", "nope"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildArgv() = %#v, want %#v", got, want)
	}
}

func TestSplitArgumentsRejectsMalformedInput(t *testing.T) {
	for _, input := range []string{`"unterminated`, `trailing\`} {
		if _, err := splitArguments(input); err == nil {
			t.Fatalf("splitArguments(%q) unexpectedly succeeded", input)
		}
	}
}

func TestRunCommandDoesNotInterpretShellOperators(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "injected")
	msg := runCommand([]string{"/bin/echo", "safe;", "touch", marker})()
	finished, ok := msg.(runFinishedMsg)
	if !ok {
		t.Fatalf("message type = %T, want runFinishedMsg", msg)
	}
	if finished.err != nil {
		t.Fatalf("runCommand failed: %v", finished.err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("shell operator was interpreted; marker exists or stat failed: %v", err)
	}
	if !strings.Contains(finished.output, "safe;") {
		t.Fatalf("output missing literal argument: %q", finished.output)
	}
}

func TestBoundedOutputTruncatesWithoutShortWrite(t *testing.T) {
	out := newBoundedOutput(5)
	if n, err := out.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("first Write = (%d, %v)", n, err)
	}
	if n, err := out.Write([]byte("defg")); err != nil || n != 4 {
		t.Fatalf("second Write = (%d, %v)", n, err)
	}
	if got := out.String(); got != "abcde" {
		t.Fatalf("String() = %q, want abcde", got)
	}
	if !out.Truncated() {
		t.Fatal("expected truncation marker")
	}
}

func TestFormatArgvRedactsSensitiveValues(t *testing.T) {
	got := formatArgv([]string{
		"sin", "serve", "--api-key", "secret-value", "TOKEN=another-secret", "--model=x",
	})
	if strings.Contains(got, "secret-value") || strings.Contains(got, "another-secret") {
		t.Fatalf("formatArgv leaked sensitive value: %q", got)
	}
	for _, want := range []string{"--api-key", "[REDACTED]", "TOKEN=[REDACTED]", "--model=x"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatArgv missing %q: %q", want, got)
		}
	}
}
