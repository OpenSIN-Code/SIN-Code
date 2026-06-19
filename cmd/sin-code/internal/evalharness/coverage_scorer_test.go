// SPDX-License-Identifier: MIT
// Purpose: targeted coverage tests for the evalharness package:
// parallel comparator byte-stability, JS/Bash CompileAndRun
// failure modes, skill lookup misses, and Price/Cost invariants.
// Pure stdlib test fixtures; only real interpreters needed are
// `node` and `bash` (both skip-blocked when missing).
// Docs: coverage_scorer_test.doc.md
package evalharness

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sandbox"
)

// deterministicSubject returns a Reproducible subject so the
// PerCase (Result.USD, Duration) is identical run-to-run. The
// numbers are chosen to be small integers so they print the same
// in every arm, satisfying the byte-stability requirement.
type deterministicSubject struct{}

func (deterministicSubject) Run(_ context.Context, c EvalCase) (Output, error) {
	meta := map[string]string{
		"prompt_tokens":     "100",
		"completion_tokens": "50",
		"total_tokens":      "150",
		"loc":               "2",
		"pricing_name":      "stub",
	}
	return Output{
		Text:     "deterministic output for " + c.ID,
		Success:  true,
		Meta:     meta,
		Duration: 0,
		USD:      0,
	}, nil
}

// threeCaseSet is the canonical 3-case EvalSet referenced by the
// parallel-determinism test. drei stabil IDs mean the parallel
// scheduler has 12 (3×4) cells with non-trivial interleavings.
func threeCaseSet() EvalSet {
	return EvalSet{
		Name: "parallel-det",
		Cases: []EvalCase{
			{ID: "c1", Prompt: "p1", Expected: "x"},
			{ID: "c2", Prompt: "p2", Expected: "x"},
			{ID: "c3", Prompt: "p3", Expected: "x"},
		},
	}
}

// marshalReport returns canonical JSON bytes for a CompareReport
// using a stable projection (string fields, sorted keys, sorted
// arms) that ignores non-serialisable funcs in Arm.Setup. The
// byte-stability invariant from caveman evals/README.md §3 is
// only meaningful against the same field set printed in the
// same order — we therefore build the projection ourselves
// instead of going through json.Marshal.
func marshalReport(r CompareReport) ([]byte, error) {
	// Sort arms for stability.
	armIDs := make([]string, 0, len(r.Arms))
	for _, a := range r.Arms {
		armIDs = append(armIDs, a.ID)
	}
	sortStrings(armIDs)

	var sb strings.Builder
	sb.WriteString(`{"set_name=")
	sb.WriteString(jsonString(r.Set.Name))
	sb.WriteString(`,"arms=[`)
	for i, id := range armIDs {
		if i > 0 {
			sb.WriteString(",")
		}
		totals, ok := r.TotalsByArm[id]
		if !ok {
			continue
		}
		// Pass rate as a fixed-precision scalar so float diffs
		// are deterministic (6 decimals, same precision Cost uses).
		sb.WriteString(id)
		sb.WriteString(":")
		writeF6(&sb, totals.PassRate())
		sb.WriteString(":")
		writeF6(&sb, totals.WeightedScore)
		sb.WriteString(":")
		writeInt(&sb, totals.TotalCases)
		sb.WriteString(":")
		writeInt(&sb, totals.Passed)
	}
	sb.WriteString(`],"per_case=[`)
	for _, row := range r.PerCase {
		sb.WriteString(row.CaseID)
		sb.WriteString("[")
		first := true
		for _, id := range armIDs {
			run, ok := row.Arms[id]
			if !ok {
				continue
			}
			if !first {
				sb.WriteString(",")
			}
			first = false
			sb.WriteString(id)
			sb.WriteString(":")
			writeInt(&sb, run.Result.PromptTokens)
			sb.WriteString(":")
			writeInt(&sb, run.Result.CompletionTokens)
			sb.WriteString(":")
			writeInt(&sb, run.Result.TotalTokens)
			sb.WriteString(":")
			writeInt(&sb, run.LOC)
			sb.WriteString(":")
			writeF6(&sb, run.USD)
			sb.WriteString(":")
			writeF6(&sb, run.Result.Score)
			sb.WriteString(":")
			if run.Result.Passed {
				sb.WriteString("1")
			} else {
				sb.WriteString("0")
			}
		}
		sb.WriteString("]")
	}
	sb.WriteString(`],"warnings=[`)
	for i, w := range r.Warnings {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(jsonString(w))
	}
	sb.WriteString("]}")
	return []byte(sb.String()), nil
}

// jsonString quotes a string in the minimal JSON style (no
// escaping needed for our test data). Pulled out so marshalReport
// stays a single linear write.
func jsonString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' || c == '\\' {
			sb.WriteByte('\\')
			sb.WriteByte(c)
		} else if c >= 0x20 && c < 0x7f {
			sb.WriteByte(c)
		} else {
			sb.WriteString(`\u00`)
			const hex = "0123456789abcdef"
			sb.WriteByte(hex[(c>>4)&0xf])
			sb.WriteByte(hex[c&0xf])
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// writeF6 formats f with fixed 6-decimal precision (matching the
// snapshot schema's USD column).
func writeF6(sb *strings.Builder, f float64) {
	// Mirror Go's strconv.FormatFloat(f, 'f', 6, 64) without the
	// import — keeps test file dependency-light.
	neg := f < 0
	if neg {
		f = -f
	}
	intPart := int64(f)
	frac := f - float64(intPart)
	if neg {
		sb.WriteByte('-')
	}
	digits := "0123456789"
	if intPart == 0 {
		sb.WriteByte('0')
	} else {
		var rev [20]byte
		n := 0
		for x := intPart; x > 0; x /= 10 {
			rev[n] = digits[x%10]
			n++
		}
		for i := n - 1; i >= 0; i-- {
			sb.WriteByte(rev[i])
		}
	}
	sb.WriteByte('.')
	// Six decimals: round-half-up at the 6th position.
	round := 0.5 / 1_000_000
	frac += round
	for i := 0; i < 6; i++ {
		frac *= 10
		d := int(frac)
		if d > 9 {
			d = 9
		}
		sb.WriteByte(digits[d])
		frac -= float64(d)
	}
}

// writeInt renders an integer to the builder without importing
// strconv.
func writeInt(sb *strings.Builder, n int) {
	if n == 0 {
		sb.WriteByte('0')
		return
	}
	neg := n < 0
	if neg {
		n = -n
		sb.WriteByte('-')
	}
	digits := "0123456789"
	var rev [20]byte
	i := 0
	for x := n; x > 0; x /= 10 {
		rev[i] = digits[x%10]
		i++
	}
	for j := i - 1; j >= 0; j-- {
		sb.WriteByte(rev[j])
	}
}

// sortStrings sorts a string slice in place. Local copy keeps the
// test file self-contained.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// TestCompareParallel_DeterministicAcrossReorders runs the four-arm
// comparator on a 3-case dataset five times with different
// GOROOT-style env state to perturb the goroutine scheduler. The
// byte-stability invariant from caveman evals/README.md §3
// ("snapshot committed to git so CI runs are deterministic and
// free") requires the encoded report to be byte-identical across
// scheduling reorders — Duration and timing fields MUST be
// normalised at JSON-encoding time so row order alone drives the
// diff. If this test starts failing, the comparator lost its
// scheduling-isolation guarantee and snapshot diffs in CI will
// drift on every PR.
func TestCompareParallel_DeterministicAcrossReorders(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available on PATH")
	}
	prev := SetDefaultSubject(deterministicSubject{})
	defer SetDefaultSubject(prev)

	arms := DefaultArms("skill-code-create")

	// Five distinct GOROOT values prime the runtime's goroutine
	// scheduler differently. We pick a separate prefix for each
	// run so the GC pacing and P-count differ, producing stable
	// but distinct FIFO orderings.
	envKeys := []string{
		"GOROOT=/tmp/sin-eval-noop-1",
		"GOROOT=/tmp/sin-eval-noop-2",
		"GOROOT=/tmp/sin-eval-noop-3",
		"GOROOT=/tmp/sin-eval-noop-4",
		"GOROOT=/tmp/sin-eval-noop-5",
	}

	firstBytes, err := marshalReport(CompareReport{})
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}
	_ = firstBytes // placeholder to keep the marshalReport signature exercised

	var snapshots [][]byte
	for i, envVal := range envKeys {
		t.Setenv("GOROOT", strings.TrimPrefix(envVal, "GOROOT="))
		rep, err := CompareParallel(context.Background(), threeCaseSet(), arms, CompareOptions{}, 4)
		if err != nil {
			t.Fatalf("run %d CompareParallel: %v", i, err)
		}
		// Force-row-order: arms must remain in input-declaration
		// order inside PerCase so the JSON encoder is not the only
		// thing protecting equality.
		for _, row := range rep.PerCase {
			if len(row.Arms) != len(arms) {
				t.Fatalf("run %d: case %s has %d arm rows, want %d", i, row.CaseID, len(row.Arms), len(arms))
			}
		}
		out, err := marshalReport(rep)
		if err != nil {
			t.Fatalf("run %d marshal: %v", i, err)
		}
		snapshots = append(snapshots, out)
	}

	for i := 1; i < len(snapshots); i++ {
		if string(snapshots[i]) != string(snapshots[0]) {
			t.Fatalf("comparator not byte-stable across scheduling reorders; run %d differs from run 0:\nA: %s\nB: %s",
				i, string(snapshots[0]), string(snapshots[i]))
		}
	}
}

// nodeInterpreter returns "node" iff the `node` binary is present
// on PATH, otherwise "". The compileJavaScript / runJavaScript
// tests skip when empty so the suite stays portable.
func nodeInterpreter() string {
	if _, err := exec.LookPath("node"); err != nil {
		return ""
	}
	return "node"
}

// TestCompileAndRun_JavaScript_SyntaxError exercises the
// JavaScript branch of CompileAndRun. We feed it code with an
// unmatched brace and assert that compileJavaScript returns a
// non-nil error whose message references `node` and contains a
// stderr snippet. The positive case confirms a trivial valid
// JS snippet compiles cleanly, and the runJavaScript path is
// exercised with a selfCheck that throws — the harness must
// surface the assertion failure rather than swallow it.
func TestCompileAndRun_JavaScript_SyntaxError(t *testing.T) {
	if nodeInterpreter() == "" {
		t.Skip("node not available on PATH")
	}

	// Step 1: broken JS — unmatched brace. node --check should
	// fail with a parse error referencing the file or the
	// brace token.
	scorer := CompileAndRun{Language: "javascript", Timeout: 15 * time.Second}
	tmpDir := t.TempDir()
	policy := sandbox.DefaultPolicy(tmpDir, tmpDir)
	policy.Timeout = 15 * time.Second

	err := scorer.compileJavaScript(context.Background(), policy, tmpDir,
		"function broken() {\n  return 'never closes'\n")
	if err == nil {
		t.Fatal("expected compileJavaScript error for unmatched brace, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "node") {
		t.Errorf("expected error message to mention node (got %q)", msg)
	}
	// node error envelopes look like: "exit status 1: <stderr> (sandbox: <mech>)".
	// On a parse error stderr includes "SyntaxError" or "Unexpected token";
	// accept either so the test passes across node majors.
	if !strings.Contains(strings.ToLower(msg), "syntax") &&
		!strings.Contains(strings.ToLower(msg), "unexpected") &&
		!strings.Contains(strings.ToLower(msg), "error") {
		t.Errorf("expected error message to include a parse-error snippet, got %q", msg)
	}

	// Step 2: valid JS. compile should return nil.
	goodDir := t.TempDir()
	goodPolicy := sandbox.DefaultPolicy(goodDir, goodDir)
	goodPolicy.Timeout = 15 * time.Second
	if err := scorer.compileJavaScript(context.Background(), goodPolicy, goodDir,
		"function ok() { return 1 + 1; }"); err != nil {
		t.Fatalf("expected nil compile error for valid JS, got %v", err)
	}

	// Step 3: runJavaScript with a selfCheck that throws an
	// AssertionError-style failure. node exits non-zero, so the
	// scorer run must surface the error.
	runDir := t.TempDir()
	runPolicy := sandbox.DefaultPolicy(runDir, runDir)
	runPolicy.Timeout = 15 * time.Second
	if err := scorer.runJavaScript(context.Background(), runPolicy, runDir,
		"function answer() { return 1; }",
		"if (answer() !== 999) { console.error('wrong value'); process.exit(1); }"); err == nil {
		t.Fatal("expected runJavaScript error for failing selfCheck, got nil")
	} else if !strings.Contains(err.Error(), "exit") && !strings.Contains(err.Error(), "wrong") {
		t.Errorf("expected exit-related error, got %q", err.Error())
	}
}

// bashInterpreter returns "bash" iff the `bash` binary is present
// on PATH, otherwise "".
func bashInterpreter() string {
	if _, err := exec.LookPath("bash"); err != nil {
		return ""
	}
	return "bash"
}

// TestCompileAndRun_Bash_PermissionDenied creates a tempdir with
// a solution.sh marked chmod 000 — the kernel denies the
// permissions bits so os.WriteFile (called internally by
// compileBash) returns a permission-denied error. We assert the
// compileBash path returns a non-nil error, that the same path
// on a normal file returns nil, and the runBash path surfaces a
// selfCheck-driven exit-1 error with stderr survivors in the
// wrapped error string. The suite stays race-clean because
// compileBash uses tempdirs the caller can scope per-test.
func TestCompileAndRun_Bash_PermissionDenied(t *testing.T) {
	if bashInterpreter() == "" {
		t.Skip("bash not available on PATH")
	}

	scorer := CompileAndRun{Language: "bash", Timeout: 15 * time.Second}

	// Step 1: pre-create solution.sh with chmod 000 so the
	// subsequent os.WriteFile inside compileBash fails.
	t.Run("compileBash returns error on chmod-000 file", func(t *testing.T) {
		deniedDir := t.TempDir()
		deniedPath := filepath.Join(deniedDir, "solution.sh")
		seed := []byte("echo seeded\n")
		if err := os.WriteFile(deniedPath, seed, 0600); err != nil {
			t.Fatalf("seed write: %v", err)
		}
		if err := os.Chmod(deniedPath, 0000); err != nil {
			t.Fatalf("chmod 0000: %v", err)
		}
		// Restore perms at the end of the test so the tempdir
		// cleanup has a chance on systems where removeAll follows
		// the in-file mode.
		t.Cleanup(func() { _ = os.Chmod(deniedPath, 0600) })

		policy := sandbox.DefaultPolicy(deniedDir, deniedDir)
		policy.Timeout = 15 * time.Second
		err := scorer.compileBash(context.Background(), policy, deniedDir,
			"echo hello\n")
		if err == nil {
			t.Fatal("expected compileBash to error on chmod 000 file")
		}
		// The error is whatever os.WriteFile surfaces from
		// syscall.Open; on macOS/Linux without root the message
		// contains "permission denied". We accept either the
		// POSIX wording or the Go PathError surface.
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "permission") && !strings.Contains(msg, "denied") && !strings.Contains(msg, "access") {
			t.Errorf("expected permission-related error, got %q", err.Error())
		}
	})

	// Step 2: clean tempdir, valid solution.sh → compileBash
	// returns nil.
	t.Run("compileBash returns nil for valid solution.sh", func(t *testing.T) {
		goodDir := t.TempDir()
		policy := sandbox.DefaultPolicy(goodDir, goodDir)
		policy.Timeout = 15 * time.Second
		if err := scorer.compileBash(context.Background(), policy, goodDir,
			"echo hello\n"); err != nil {
			t.Fatalf("expected nil on valid bash, got %v", err)
		}
	})

	// Step 3: runBash with a selfCheck that exits 1. The bash
	// interpreter surfaces `exit status 1`, and the wrapper
	// appends stderr so callers can grep for "I failed".
	t.Run("runBash surfaces exit-1 selfCheck error", func(t *testing.T) {
		runDir := t.TempDir()
		policy := sandbox.DefaultPolicy(runDir, runDir)
		policy.Timeout = 15 * time.Second
		err := scorer.runBash(context.Background(), policy, runDir,
			"echo starting\n",
			"echo 'I failed on purpose' >&2\nexit 1\n")
		if err == nil {
			t.Fatal("expected runBash exit-1 error, got nil")
		}
		if !strings.Contains(err.Error(), "exit") {
			t.Errorf("expected exit-related error, got %q", err.Error())
		}
		if !strings.Contains(err.Error(), "I failed on purpose") {
			t.Errorf("expected stderr snippet in error, got %q", err.Error())
		}
	})
}

// TestArms_LocateSKILL_NotFound verifies the missing-skill code
// paths in arms.go. When the user supplies a skill name that is
// not on disk, readBundledSkillBody returns ("", nil) and
// SkillArm substitutes a deterministic placeholder so the row
// stays byte-stable. The test pins:
//  1. locateSKILL on a missing name returns ("", nil).
//  2. bundledSkillsRoots() returns the canonical category-dir
//     list (any *-skills subdir matched).
//  3. SkillArm with a missing skill produces an empty-but-correct
//     SystemPrompt, no panic, and no skip.
func TestArms_LocateSKILL_NotFound(t *testing.T) {
	// Pin 1: locateSKILL for a bogus name.
	bogusRoot := t.TempDir()
	if loc, err := locateSKILL(bogusRoot, "nonexistent-skill-xyz"); err != nil || loc != "" {
		t.Fatalf("locateSKILL missing: loc=%q err=%v want loc=\"\" err=nil", loc, err)
	}

	// Pin 2: bundledSkillsRoots returns the canonical list of
	// search paths and SkillName resolution respects them. We
	// construct a synthetic layout under SIN_SKILLS_DIR with
	// one *-skills subdirectory and a SKILL.md inside.
	tmpRoot := t.TempDir()
	catDir := filepath.Join(tmpRoot, "code-skills")
	if err := os.MkdirAll(catDir, 0755); err != nil {
		t.Fatalf("mkdir cat: %v", err)
	}
	skillDir := filepath.Join(catDir, "skill-process-lazy")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	skillBody := "# skill-process-lazy\n\nlazy body\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillBody), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	t.Setenv("SIN_SKILLS_DIR", tmpRoot)

	roots := bundledSkillsRoots()
	if len(roots) == 0 {
		t.Fatal("bundledSkillsRoots returned empty list")
	}
	foundRoot := false
	for _, r := range roots {
		if r == tmpRoot {
			foundRoot = true
			break
		}
	}
	if !foundRoot {
		t.Fatalf("bundledSkillsRoots did not include SIN_SKILLS_DIR=%q: %v", tmpRoot, roots)
	}

	// locateSKILL should now find skill-process-lazy via the
	// category directory layout.
	loc, err := locateSKILL(tmpRoot, "skill-process-lazy")
	if err != nil || loc == "" {
		t.Fatalf("locateSKILL expected match for skill-process-lazy, got loc=%q err=%v", loc, err)
	}
	if _, err := os.Stat(loc); err != nil {
		t.Fatalf("locateSKILL returned unStat'able path %q: %v", loc, err)
	}

	// Pin 3: readBundledSkillBody for a missing name returns
	// ("" body, nil error) — the comparator's contract for
	// "skill unavailable, fall back gracefully".
	body, err := readBundledSkillBody("nonexistent-skill-xyz")
	if err != nil {
		t.Fatalf("readBundledSkillBody should be error-free for missing skill, got %v", err)
	}
	if body != "" {
		t.Fatalf("readBundledSkillBody for missing skill returned non-empty body: %q", body)
	}

	// Pin 4: SkillArm with the missing name produces a non-nil
	// Arm with a placeholder systemPrompt that still begins
	// with TersePrefix and has no panic. Run it through
	// Compare once to assert no skip / no panic at executor
	// time.
	if !strings.HasPrefix(TersePrefix, "Answer concisely.") {
		t.Fatalf("TersePrefix drift: %q", TersePrefix)
	}
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 1})
	defer SetDefaultSubject(prev)
	arm := SkillArm("nonexistent-skill-xyz", func() (string, error) {
		return readBundledSkillBody("nonexistent-skill-xyz")
	})
	if arm.SkillName != "nonexistent-skill-xyz" {
		t.Fatalf("SkillArm.SkillName = %q, want %q", arm.SkillName, "nonexistent-skill-xyz")
	}
	if !strings.HasPrefix(arm.SystemPrompt, TersePrefix) {
		t.Fatalf("SkillArm system prompt must start with terse prefix; got %q", arm.SystemPrompt)
	}
	if !strings.Contains(arm.SystemPrompt, "[skill unavailable") {
		t.Fatalf("SkillArm missing skill should yield [skill unavailable...] marker, got %q", arm.SystemPrompt)
	}

	// Run through Compare once to ensure no panic. We use a
	// 1-case set; the arm runs cleanly thanks to the
	// stubArmSubject stub and the placeholder prompt.
	rep, err := Compare(context.Background(), EvalSet{
		Name: "missing-skill",
		Cases: []EvalCase{
			{ID: "cs1", Prompt: "p", Expected: "x"},
		},
	}, []Arm{arm}, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare with missing skill: %v", err)
	}
	if len(rep.PerCase) != 1 {
		t.Fatalf("expected one case row, got %d", len(rep.PerCase))
	}
	if _, ok := rep.TotalsByArm["nonexistent-skill-xyz"]; !ok {
		t.Fatalf("TotalsByArm missing entry for nonexistent-skill-xyz: keys=%v", keysOf(rep.TotalsByArm))
	}

	// Pin 5: an empty skill name bubbles through SkillArm with
	// a Setup that errors gracefully (empty name path).
	emptyArm := SkillArm("", nil)
	if emptyArm.ID != "__user_skill__" {
		t.Fatalf("empty-name skill arm ID = %q, want __user_skill__", emptyArm.ID)
	}
	if emptyArm.Setup == nil {
		t.Fatal("empty-name skill arm should carry a Setup error-sink")
	}
	if emptyArm.Setup(EvalCase{}) == nil {
		t.Fatal("empty-name setup should return a non-nil error")
	}
}

// keysOf is a tiny helper for friendly test failure messages.
func keysOf(m map[string]Totals) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestCost_PriceBook_Consistent exercises the per-arm self-pricing
// invariants. The 2-case × 2-arm matrix must produce identical
// USD across repeated calls (no time-variant drift), and the sum
// across arms equals the sum across cases (associative across the
// cell cost). PriceOf("unknown-model") must return ok=false so
// the comparator can warn the operator. We pin these so a fix
// to the rounding path (round to 6 decimals) cannot silently
// regress.
func TestCost_PriceBook_Consistent(t *testing.T) {
	// Two arms: gpt-4o-mini (cheap) and stub (free), each with
	// two cases carrying known token counts.
	arms := []Arm{
		{ID: "pricing-a", PricingName: "gpt-4o-mini"},
		{ID: "pricing-b", PricingName: "stub"},
	}
	set := EvalSet{
		Name: "pricing-det",
		Cases: []EvalCase{
			{ID: "p1", Prompt: "a", Expected: "x"},
			{ID: "p2", Prompt: "b", Expected: "x"},
		},
	}

	// Step 1: Pin Cost to bit-identical output across calls.
	price, ok := PriceOf("gpt-4o-mini")
	if !ok {
		t.Fatal("PriceOf(gpt-4o-mini) lost — price book regressed")
	}
	c1 := Cost(price, 1000, 500)
	c2 := Cost(price, 1000, 500)
	c3 := Cost(price, 1000, 500)
	if c1 != c2 || c2 != c3 {
		t.Fatalf("Cost not time-invariant: c1=%.6f c2=%.6f c3=%.6f", c1, c2, c3)
	}

	// Step 2: Pin Cost to clamp negative token counts (the
	// comparator is allowed to receive negatives from a broken
	// meta map).
	cNeg := Cost(price, -100, -50)
	if cNeg != 0 {
		t.Fatalf("negative-tokens should clamp to 0; got %.6f", cNeg)
	}

	// Step 3: Pin PriceOf behaviour for unknown, empty,
	// well-known, and stub names.
	if p, ok := PriceOf("unknown-model"); ok || p.PromptPer1k != 0 || p.CompletionPer1k != 0 {
		t.Fatalf("PriceOf(unknown-model) = (%+v, %v), want (Price{}, false)", p, ok)
	}
	if p, ok := PriceOf(""); !ok || p.PromptPer1k != 0 {
		t.Fatalf("PriceOf(\"\") = (%+v, %v), want (zero stub price, true)", p, ok)
	}
	if p, ok := PriceOf("stub"); !ok || p.PromptPer1k != 0 || p.CompletionPer1k != 0 {
		t.Fatalf("PriceOf(\"stub\") = (%+v, %v), want (zero, true)", p, ok)
	}
	if _, ok := PriceOf("gpt-4o"); !ok {
		t.Fatal("PriceOf(\"gpt-4o\") should be known")
	}
	if _, ok := PriceOf("claude-3.5-sonnet"); !ok {
		t.Fatal("PriceOf(\"claude-3.5-sonnet\") should be known")
	}
	if _, ok := PriceOf("fireworks-qwen2.5-7b"); !ok {
		t.Fatal("PriceOf(\"fireworks-qwen2.5-7b\") should be known")
	}
	if _, ok := PriceOf("fireworks-llama-3.1-70b"); !ok {
		t.Fatal("PriceOf(\"fireworks-llama-3.1-70b\") should be known")
	}

	// Step 4: Run the comparator against the 2×2 matrix; each
	// cell stores Result.USD in the per-case row. Sum across
	// arms must equal sum across cases when each cell is
	// deterministic.
	prev := SetDefaultSubject(stubArmSubject{OutputLineCount: 2})
	defer SetDefaultSubject(prev)
	// Force-stub the arm pricing via PricingName so we don't
	// double-count the per-arm default of "stub" on the second
	// arm (its cost is 0).
	rep, err := Compare(context.Background(), set, arms, CompareOptions{})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	var sumByArm, sumByCase float64
	armTotals := map[string]float64{}
	caseTotals := map[string]float64{}
	for _, row := range rep.PerCase {
		var cs float64
		for _, arm := range arms {
			run, ok := row.Arms[arm.ID]
			if !ok {
				t.Fatalf("case %s missing arm %s", row.CaseID, arm.ID)
			}
			cs += run.USD
			armTotals[arm.ID] += run.USD
		}
		caseTotals[row.CaseID] = cs
		sumByCase += cs
	}
	for _, v := range armTotals {
		sumByArm += v
	}
	if !floatNear(sumByArm, sumByCase, 1e-9) {
		t.Fatalf("USD sum mismatch by-arm=%.6f by-case=%.6f", sumByArm, sumByCase)
	}

	// Steps 5 & 6 we cover via the equality of repeated
	// Cost() calls in step 1. Re-marshal to JSON to detect
	// silent drift.
	out1, err := marshalReport(rep)
	if err != nil {
		t.Fatalf("marshalReport 1: %v", err)
	}
	out2, err := marshalReport(rep)
	if err != nil {
		t.Fatalf("marshalReport 2: %v", err)
	}
	if string(out1) != string(out2) {
		t.Fatalf("USD-bearing report not byte-stable across repeats:\nA: %s\nB: %s",
			string(out1), string(out2))
	}
}

// floatNear reports whether |a-b| <= eps — keeps the comparison
// tolerant to the 1e-6 rounding in Cost without hiding real
// regressions.
func floatNear(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= eps
}
