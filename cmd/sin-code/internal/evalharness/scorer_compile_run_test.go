// SPDX-License-Identifier: MIT
// Purpose: unit tests for CompileAndRun scorer. Requires Python 3 on
// PATH; otherwise the Python cases are skipped. Sandbox is used for
// every execution, so these tests exercise the same path as the CLI.
// Docs: scorer_compile_run_test.doc.md
package evalharness

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func pythonInterpreter() string {
	for _, name := range []string{"python3", "python"} {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return ""
}

func TestCompileAndRun_PythonFizzBuzz(t *testing.T) {
	if pythonInterpreter() == "" {
		t.Skip("python not available on PATH")
	}
	code := "```python\ndef fizzbuzz(n):\n    if n % 15 == 0:\n        return 'FizzBuzz'\n    if n % 3 == 0:\n        return 'Fizz'\n    if n % 5 == 0:\n        return 'Buzz'\n    return str(n)\n```"
	scorer := CompileAndRun{
		Language:  "python",
		SelfCheck: "assert fizzbuzz(15) == 'FizzBuzz'\nassert fizzbuzz(3) == 'Fizz'\nassert fizzbuzz(5) == 'Buzz'\nassert fizzbuzz(7) == '7'",
		Timeout:   30 * time.Second,
	}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: code})
	if score != 1.0 || !passed {
		t.Fatalf("expected 1.0 pass, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
	if !strings.Contains(detail, "compile + self-check passed") {
		t.Fatalf("unexpected detail: %s", detail)
	}
}

func TestCompileAndRun_PythonSyntaxError(t *testing.T) {
	if pythonInterpreter() == "" {
		t.Skip("python not available on PATH")
	}
	code := "```python\ndef fizzbuzz(n:\n    return n\n```"
	scorer := CompileAndRun{
		Language:  "python",
		SelfCheck: "assert fizzbuzz(1) == 1",
		Timeout:   30 * time.Second,
	}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: code})
	if score != 0.0 || passed {
		t.Fatalf("expected 0.0 fail, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
	if !strings.Contains(detail, "compile failed") {
		t.Fatalf("expected compile failure, got detail=%s", detail)
	}
}

func TestCompileAndRun_SelfCheckFailure(t *testing.T) {
	if pythonInterpreter() == "" {
		t.Skip("python not available on PATH")
	}
	code := "```python\ndef fizzbuzz(n):\n    return 'nope'\n```"
	scorer := CompileAndRun{
		Language:  "python",
		SelfCheck: "assert fizzbuzz(15) == 'FizzBuzz'",
		Timeout:   30 * time.Second,
	}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: code})
	if score != 0.0 || passed {
		t.Fatalf("expected 0.0 fail, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
	if !strings.Contains(detail, "self-check failed") {
		t.Fatalf("expected self-check failure, got detail=%s", detail)
	}
}

func TestCompileAndRun_SkipTestTrivialOneLiner(t *testing.T) {
	if pythonInterpreter() == "" {
		t.Skip("python not available on PATH")
	}
	code := "```python\ndict(zip(['a'], [1]))\n```"
	scorer := CompileAndRun{
		Language: "python",
		SkipTest: true,
		Timeout:  30 * time.Second,
	}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: code})
	if score != 1.0 || !passed {
		t.Fatalf("expected 1.0 pass, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
	if !strings.Contains(detail, "trivial one-liner") {
		t.Fatalf("expected trivial one-liner detail, got %s", detail)
	}
}

func TestCompileAndRun_NoSelfCheckScoresHalf(t *testing.T) {
	if pythonInterpreter() == "" {
		t.Skip("python not available on PATH")
	}
	code := "```python\ndef fizzbuzz(n):\n    return str(n)\n```"
	scorer := CompileAndRun{
		Language: "python",
		Timeout:  30 * time.Second,
	}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: code})
	if score != 0.5 || passed {
		t.Fatalf("expected 0.5 fail, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
	if !strings.Contains(detail, "no self-check") {
		t.Fatalf("expected no self-check detail, got %s", detail)
	}
}

func TestCompileAndRun_NoCodeBlock(t *testing.T) {
	scorer := CompileAndRun{Language: "python"}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: "just some prose"})
	if score != 0.0 || passed {
		t.Fatalf("expected 0.0 fail, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
	if !strings.Contains(detail, "no code block") {
		t.Fatalf("expected no code block detail, got %s", detail)
	}
}

func TestExtractCodeBlock(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"```python\nprint('hi')\n```", "print('hi')"},
		{"pre\n```go\npackage main\n```\npost", "package main"},
		{"no block here", ""},
		{"```\n\n```", ""},
		{"```bash\necho hi\n```", "echo hi"},
	}
	for _, tc := range cases {
		got := extractCodeBlock(tc.in)
		if got != tc.want {
			t.Errorf("extractCodeBlock(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

func TestCompileAndRun_LanguageUnsupported(t *testing.T) {
	scorer := CompileAndRun{Language: "rust"}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: "```rust\nfn main(){}\n```"})
	if score != 0.0 || passed {
		t.Fatalf("expected 0.0 fail, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
	if !strings.Contains(detail, "unsupported language") {
		t.Fatalf("expected unsupported language detail, got %s", detail)
	}
}

func TestIsCompileAndRunLanguage(t *testing.T) {
	for _, lang := range []string{"go", "python", "javascript", "bash"} {
		if !IsCompileAndRunLanguage(lang) {
			t.Errorf("expected %q to be supported", lang)
		}
	}
	if IsCompileAndRunLanguage("rust") {
		t.Error("expected rust to be unsupported")
	}
}

func TestCompileAndRun_GoSkipTest(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available on PATH")
	}
	code := "```go\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"hello\")\n}\n```"
	scorer := CompileAndRun{
		Language: "go",
		SkipTest: true,
		Timeout:  60 * time.Second,
	}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: code})
	if score != 1.0 || !passed {
		t.Fatalf("expected 1.0 pass, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
	if !strings.Contains(detail, "trivial one-liner") && !strings.Contains(detail, "compile passed") {
		t.Fatalf("unexpected detail: %s", detail)
	}
}

func TestCompileAndRun_GoSelfCheck(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available on PATH")
	}
	code := "```go\npackage main\n\nfunc answer() int { return 42 }\n\nfunc main() {}\n```"
	scorer := CompileAndRun{
		Language:  "go",
		SelfCheck: "func init() { if answer() != 42 { panic(\"wrong\") } }",
		Timeout:   60 * time.Second,
	}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: code})
	if score != 1.0 || !passed {
		t.Fatalf("expected 1.0 pass, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
}

func TestCompileAndRun_GoSelfCheckFail(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available on PATH")
	}
	code := "```go\npackage main\n\nfunc answer() int { return 42 }\n\nfunc main() {}\n```"
	scorer := CompileAndRun{
		Language:  "go",
		SelfCheck: "func init() { if answer() != 7 { panic(\"wrong\") } }",
		Timeout:   60 * time.Second,
	}
	score, passed, detail := scorer.Score(EvalCase{}, Output{Text: code})
	if score != 0.0 || passed {
		t.Fatalf("expected 0.0 fail, got score=%.2f pass=%v detail=%s", score, passed, detail)
	}
	if !strings.Contains(detail, "self-check failed") {
		t.Fatalf("expected self-check failure, got detail=%s", detail)
	}
}

func TestScorerFromConfig(t *testing.T) {
	scorer, err := ScorerFromConfig(map[string]any{
		"type":       "compile_and_run",
		"language":   "python",
		"self_check": "assert True",
		"skip_test":  true,
	})
	if err != nil {
		t.Fatalf("ScorerFromConfig: %v", err)
	}
	car, ok := scorer.(CompileAndRun)
	if !ok {
		t.Fatalf("expected CompileAndRun, got %T", scorer)
	}
	if car.Language != "python" || car.SelfCheck != "assert True" || !car.SkipTest {
		t.Fatalf("unexpected scorer: %+v", car)
	}

	if _, err := ScorerFromConfig(map[string]any{"type": "compile_and_run", "language": "rust"}); err == nil {
		t.Fatal("expected error for unsupported language")
	}
	if _, err := ScorerFromConfig(map[string]any{"type": "unknown"}); err == nil {
		t.Fatal("expected error for unknown scorer type")
	}
}
