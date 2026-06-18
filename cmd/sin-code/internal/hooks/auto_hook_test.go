// SPDX-License-Identifier: MIT
// Purpose: tests for the auto-lint / auto-test PostListeners (issue #376).
package hooks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoLintFiresAfterSinEdit(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not on PATH; cannot exercise auto-lint listener")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; cannot exercise auto-lint listener")
	}

	tmp := t.TempDir()
	pkgDir := filepath.Join(tmp, "alpkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module almod\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	src := "package alpkg\n\nfunc  Hello() string {\n   return \"hi\"\n}\n"
	goFile := filepath.Join(pkgDir, "hello.go")
	if err := os.WriteFile(goFile, []byte(src), 0o644); err != nil {
		t.Fatalf("write hello.go: %v", err)
	}

	eng := New(nil)
	eng.RegisterPostListener(AutoLintListener(AutoHookConfig{}))

	res := eng.Fire(context.Background(), Payload{
		Event:     ToolPost,
		Name:      "sin_edit",
		Data:      map[string]any{"path": "alpkg/hello.go"},
		Workspace: tmp,
	})
	if res.Blocked {
		t.Fatalf("auto-lint must NEVER block tool.post; got %+v", res)
	}
	foundGofmt := false
	for _, m := range res.PromptInjects {
		if strings.Contains(m, "gofmt") && strings.Contains(m, "hello.go") {
			foundGofmt = true
			break
		}
	}
	if !foundGofmt {
		t.Fatalf("expected gofmt prompt-inject mentioning hello.go; got %+v", res.PromptInjects)
	}
}

func TestAutoTestFiresAfterTestFileEdit(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH; cannot exercise auto-test listener")
	}

	tmp := t.TempDir()
	pkgDir := filepath.Join(tmp, "atpkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module autotest_exercise\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "hello.go"), []byte("package atpkg\n\nfunc Hello() string { return \"hi\" }\n"), 0o644); err != nil {
		t.Fatalf("write hello.go: %v", err)
	}
	testPath := filepath.Join(pkgDir, "hello_test.go")
	testSrc := "package atpkg\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {\n\tif Hello() != \"hi\" {\n\t\tt.Fatal(\"wrong\")\n\t}\n}\n"
	if err := os.WriteFile(testPath, []byte(testSrc), 0o644); err != nil {
		t.Fatalf("write hello_test.go: %v", err)
	}

	eng := New(nil)
	eng.RegisterPostListener(AutoTestListener(AutoHookConfig{Timeout: AutoTestDefaultTimeout}))

	res := eng.Fire(context.Background(), Payload{
		Event:     ToolPost,
		Name:      "sin_edit",
		Data:      map[string]any{"path": "atpkg/hello_test.go"},
		Workspace: tmp,
	})
	if res.Blocked {
		t.Fatalf("auto-test must NEVER block tool.post; got %+v", res)
	}
	if len(res.PromptInjects) != 0 {
		t.Fatalf("expected silent inject on PASS, got %+v", res.PromptInjects)
	}

	failSrc := "package atpkg\n\nimport \"testing\"\n\nfunc TestHello(t *testing.T) {\n\tt.Fatal(\"intentional fail\")\n}\n"
	if err := os.WriteFile(testPath, []byte(failSrc), 0o644); err != nil {
		t.Fatalf("write failing test: %v", err)
	}
	res = eng.Fire(context.Background(), Payload{
		Event:     ToolPost,
		Name:      "sin_edit",
		Data:      map[string]any{"path": "atpkg/hello_test.go"},
		Workspace: tmp,
	})
	if len(res.PromptInjects) == 0 {
		t.Fatalf("expected failure injection when test fails; got %+v", res)
	}
	foundFail := false
	for _, m := range res.PromptInjects {
		if strings.Contains(m, "[auto-test") && strings.Contains(m, "FAIL") {
			foundFail = true
			break
		}
	}
	if !foundFail {
		t.Fatalf("expected [auto-test ...] FAIL inject on failing test; got %+v", res.PromptInjects)
	}
}

func TestAutoLintDisabledByDefault(t *testing.T) {
	eng := New(nil)
	res := eng.Fire(context.Background(), Payload{
		Event: ToolPost,
		Name:  "sin_edit",
		Data:  map[string]any{"path": "hello.go"},
	})
	if res.Blocked {
		t.Fatal("without registered listeners, tool.post must NEVER block")
	}
	if len(res.PromptInjects) != 0 {
		t.Fatalf("without listeners, PromptInjects must be empty; got %+v", res.PromptInjects)
	}
	res = eng.Fire(context.Background(), Payload{
		Event: ToolPost,
		Name:  "sin_write",
		Data:  map[string]any{"path": "foo.go"},
	})
	if res.Blocked || len(res.PromptInjects) != 0 {
		t.Fatalf("sin_write without listeners expected empty, got %+v", res)
	}
	res = eng.Fire(context.Background(), Payload{
		Event: ToolPost,
		Name:  "sin_bash",
		Data:  map[string]any{"command": "ls"},
	})
	if res.Blocked || len(res.PromptInjects) != 0 {
		t.Fatalf("sin_bash without listeners expected empty, got %+v", res)
	}
}

func TestPostListenerRegistration(t *testing.T) {
	var nilEng *Engine
	nilEng.RegisterPostListener(nil)
	nilEng.RegisterPostListener(func(_ context.Context, _ Payload) []string { return nil })

	realEng := New(nil)
	realEng.RegisterPostListener(nil)
	called := false
	realEng.RegisterPostListener(func(_ context.Context, _ Payload) []string {
		called = true
		return nil
	})
	res := realEng.Fire(context.Background(), Payload{Event: ToolPost, Name: "sin_edit"})
	if !called {
		t.Fatalf("registered listener must run on tool.post; got %+v", res)
	}
}

func TestSafeInvokePostListenerPanicRecovers(t *testing.T) {
	eng := New(nil)
	eng.RegisterPostListener(func(_ context.Context, _ Payload) []string {
		panic("intentional panic for recovery test")
	})
	res := eng.Fire(context.Background(), Payload{Event: ToolPost, Name: "sin_edit"})
	if res.Blocked {
		t.Fatal("panicked listener must not block")
	}
	if len(res.PromptInjects) != 0 {
		t.Fatalf("panicked listener should not inject; got %+v", res.PromptInjects)
	}
}
