// SPDX-License-Identifier: MIT
// Purpose: Unit tests for structural AST helpers (brace/parenthesis block ends). (st-cov1)
package internal

import (
	"strings"
	"testing"
)

func TestBraceBlockEnd_Balanced(t *testing.T) {
	lines := strings.Split("func main() {\n  if true {\n    x()\n  }\n}\n", "\n")
	end := braceBlockEnd(lines, 0)
	if end != 5 {
		t.Errorf("expected end=5, got %d", end)
	}
}

func TestBraceBlockEnd_SingleLine(t *testing.T) {
	lines := strings.Split("func main() { x() }\n", "\n")
	end := braceBlockEnd(lines, 0)
	if end != 1 {
		t.Errorf("expected end=1, got %d", end)
	}
}

func TestBraceBlockEnd_SemicolonBeforeOpen(t *testing.T) {
	lines := strings.Split("func main();\nfunc other() {}", "\n")
	end := braceBlockEnd(lines, 0)
	if end != 1 {
		t.Errorf("expected end=1, got %d", end)
	}
}

func TestBraceBlockEnd_NoOpen(t *testing.T) {
	lines := strings.Split("func main()\nfunc other() {}", "\n")
	end := braceBlockEnd(lines, 0)
	if end != 2 {
		t.Errorf("expected end=2, got %d", end)
	}
}

func TestBraceBlockEnd_Unclosed(t *testing.T) {
	lines := strings.Split("func main() {\n  x()\n", "\n")
	end := braceBlockEnd(lines, 0)
	if end != len(lines) {
		t.Errorf("expected end=%d, got %d", len(lines), end)
	}
}

func TestBraceBlockEnd_IgnoresBlockComment(t *testing.T) {
	lines := strings.Split("func main() {\n  /* } */\n}\n", "\n")
	end := braceBlockEnd(lines, 0)
	if end != 3 {
		t.Errorf("expected end=3, got %d", end)
	}
}

func TestBraceBlockEnd_IgnoresLineComment(t *testing.T) {
	lines := strings.Split("func main() {\n  // }\n}\n", "\n")
	end := braceBlockEnd(lines, 0)
	if end != 3 {
		t.Errorf("expected end=3, got %d", end)
	}
}

func TestBraceBlockEnd_IgnoresString(t *testing.T) {
	lines := strings.Split("func main() {\n  x := \"}\"\n}\n", "\n")
	end := braceBlockEnd(lines, 0)
	if end != 3 {
		t.Errorf("expected end=3, got %d", end)
	}
}

func TestBraceBlockEnd_EscapedString(t *testing.T) {
	lines := strings.Split("func main() {\n  x := \"\\\"}\n}\n", "\n")
	end := braceBlockEnd(lines, 0)
	if end != 3 {
		t.Errorf("expected end=3, got %d", end)
	}
}

func TestPythonBlockEnd(t *testing.T) {
	lines := strings.Split("def f():\n  if True:\n    pass\nreturn\n", "\n")
	end := pythonBlockEnd(lines, 0, 0)
	if end != 3 {
		t.Errorf("expected end=3, got %d", end)
	}
}

func TestPythonBlockEnd_SkipsComments(t *testing.T) {
	lines := strings.Split("def f():\n  # comment\n  pass\nreturn\n", "\n")
	end := pythonBlockEnd(lines, 0, 0)
	if end != 3 {
		t.Errorf("expected end=3, got %d", end)
	}
}

func TestPythonBlockEnd_Empty(t *testing.T) {
	lines := []string{"def f():"}
	end := pythonBlockEnd(lines, 0, 0)
	if end != 1 {
		t.Errorf("expected end=1, got %d", end)
	}
}

func TestExpandTabs(t *testing.T) {
	if expandTabs("\t") != "    " {
		t.Errorf("expandTabs: %q", expandTabs("\t"))
	}
}
