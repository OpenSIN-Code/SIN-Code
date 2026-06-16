// SPDX-License-Identifier: MIT
// Purpose: Python signature extraction via subprocess to the system
// `python3` interpreter. Uses Python's stdlib `ast` module to parse
// each .py file and yield function signatures. The Go side never
// imports Python — it just shells out, reads JSON from stdout.
//
// Why subprocess: the Go side stays CGO-free and stdlib-only (M2).
// The cost is one `python3` invocation per file; in practice that's
// <100ms per file on a warm interpreter.
//
// Docs: docs/SPEC-LAYER.md §"Drift detection (the hardening)" (Python)
package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// pythonExtractor runs the embedded `extractor.py` via `python3` and
// returns the function definitions as a map. The script is
// embedded as a const string to keep the deployment artifact
// count at 1 (no separate .py file to ship alongside sin-code).
type pythonExtractor struct {
	pythonBin string // default: "python3"; override for tests
}

// pyFunc mirrors the JSON shape produced by extractor.py.
type pyFunc struct {
	Name    string   `json:"name"`
	Params  []string `json:"params"`  // canonicalized, e.g. ["x: int", "y: str"]
	Returns []string `json:"returns"` // canonicalized, e.g. ["str"]
	Code    string   `json:"code"`    // the original def line
}

// parsePythonFuncs runs the extractor under root and returns a map
// from function name to its overloads. Skips __init__.py-style
// "magic" names by default.
func parsePythonFuncs(ctx context.Context, root, pythonBin string) (map[string][]pyFunc, error) {
	if pythonBin == "" {
		pythonBin = "python3"
	}
	cmd := exec.CommandContext(ctx, pythonBin, "-c", extractorPy)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("spec: python extractor: %w (is %s on PATH?)", err, pythonBin)
	}
	var raw map[string][]pyFunc
	if err := json.Unmarshal(out, &raw); err != nil {
		// The first line of stdout is the JSON payload; anything
		// before it is stderr noise. Strip the prefix defensively.
		i := strings.IndexByte(string(out), '{')
		if i < 0 {
			return nil, fmt.Errorf("spec: python extractor: %w (raw: %s)", err, truncateForLog(string(out), 200))
		}
		if err := json.Unmarshal(out[i:], &raw); err != nil {
			return nil, fmt.Errorf("spec: python extractor: %w (raw: %s)", err, truncateForLog(string(out), 200))
		}
	}
	return raw, nil
}

// truncateForLog caps a string for inclusion in an error message.
// Renamed from `truncate` to avoid clashing with the one in check.go
// (which is in the same package but uses different truncation rules).
func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "... (truncated)"
}

// extractorPy is the Python source run by parsePythonFuncs. It walks
// the current directory recursively, parses every .py file with
// ast, and emits JSON: {"func_name": [{"name": ..., "params": [...],
// "returns": [...], "code": ...}, ...], ...}.
//
// Lines starting with `# skip-spec-drift:` let a Python source file
// opt out of the drift check entirely (e.g. for files with
// decorators that ast can't represent). The skip directive must
// appear in the first 10 lines.
const extractorPy = `
import ast, json, os, sys

SKIP_MARK = "# skip-spec-drift:"
SKIP_LINE_LIMIT = 10

def canonical(node):
    return ast.unparse(node).strip()

def signature(fn):
    args = []
    pos = list(fn.args.posonlyargs) + list(fn.args.args)
    for a in pos:
        ann = canonical(a.annotation) if a.annotation else ""
        args.append(f"{a.arg}: {ann}" if ann else a.arg)
    if fn.args.vararg:
        ann = canonical(fn.args.vararg.annotation) if fn.args.vararg.annotation else ""
        args.append(("*" + fn.args.vararg.arg) + (f": {ann}" if ann else ""))
    for a in fn.args.kwonlyargs:
        ann = canonical(a.annotation) if a.annotation else ""
        args.append(f"{a.arg}: {ann}" if ann else a.arg)
    if fn.args.kwarg:
        ann = canonical(fn.args.kwarg.annotation) if fn.args.kwarg.annotation else ""
        args.append(("**" + fn.args.kwarg.arg) + (f": {ann}" if ann else ""))
    rets = [canonical(r) for r in fn.returns.elts] if (fn.returns and isinstance(fn.returns, ast.Tuple)) else (
        [canonical(fn.returns)] if fn.returns else [])
    line = fn.lineno
    return {
        "name": fn.name,
        "params": args,
        "returns": rets,
        "code": f"def {fn.name}({', '.join(args)}){(' -> ' + ', '.join(rets)) if rets else ''}  (line {line})",
    }

def walk(path):
    out = []
    try:
        with open(path, "r", encoding="utf-8") as f:
            head = "".join([next(f, "") for _ in range(SKIP_LINE_LIMIT)])
        if SKIP_MARK in head:
            return out
    except (OSError, StopIteration):
        return out
    try:
        with open(path, "r", encoding="utf-8") as f:
            tree = ast.parse(f.read(), filename=path)
    except (OSError, SyntaxError):
        return out
    for node in ast.walk(tree):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and not isinstance(node, ast.Lambda):
            # Only top-level functions get a chance to be referenced
            # by name; inner functions are nested and we can't
            # meaningfully drift-check them in v0.
            if any(isinstance(p, ast.FunctionDef) for p in ast.iter_child_nodes(tree)) and node not in list(tree.body):
                continue
            out.append(signature(node))
    return out

result = {}
for dirpath, dirnames, filenames in os.walk("."):
    # Skip vendor, dotfile, and common cache dirs.
    dirnames[:] = [d for d in dirnames if d not in ("vendor", ".git", "__pycache__", "node_modules", ".venv", "venv") and not d.startswith(".")]
    for fn in filenames:
        if not fn.endswith(".py"):
            continue
        if fn.startswith("test_") or fn.endswith("_test.py"):
            continue  # skip test files for v0
        path = os.path.join(dirpath, fn)
        for sig in walk(path):
            result.setdefault(sig["name"], []).append(sig)

sys.stdout.write(json.dumps(result))
`
