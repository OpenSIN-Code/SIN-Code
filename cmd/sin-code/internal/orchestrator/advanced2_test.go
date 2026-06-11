// SPDX-License-Identifier: MIT
package orchestrator

import (
	"context"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// ── Miner ──────────────────────────────────────────────────────────

func TestMinerNilDBIsSafe(t *testing.T) {
	m, err := NewMiner(nil)
	if err != nil {
		t.Fatalf("NewMiner(nil) error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil Miner for nil-DB")
	}
	ctx := context.Background()
	if err := m.RecordSequence(ctx, SeqEpisode{EpisodeID: 1, Sequence: []ToolCall{{Tool: "a", ArgShape: "{}"}}, Passed: true}); err != nil {
		t.Fatalf("RecordSequence on nil-db should be a no-op: %v", err)
	}
	if got, err := m.Mine(ctx, "build"); err != nil || got != nil {
		t.Fatalf("Mine on nil-db should be (nil,nil); got (%v, %v)", got, err)
	}
	if s, err := m.SuggestionsFor(ctx, "build"); err != nil || s != "" {
		t.Fatalf("SuggestionsFor on nil-db should be (\"\",nil); got (%q, %v)", s, err)
	}
	if err := m.ReportLive(ctx, 1, true); err != nil {
		t.Fatalf("ReportLive on nil-db should be a no-op: %v", err)
	}
}

func TestSeqKeyDeterministic(t *testing.T) {
	a := []ToolCall{{Tool: "scout", ArgShape: "{}"}, {Tool: "edit", ArgShape: "{path}"}}
	b := []ToolCall{{Tool: "scout", ArgShape: "{}"}, {Tool: "edit", ArgShape: "{path}"}}
	c := []ToolCall{{Tool: "scout", ArgShape: "{}"}, {Tool: "edit", ArgShape: "{file}"}}
	if seqKey(a) != seqKey(b) {
		t.Fatalf("seqKey not deterministic: %q != %q", seqKey(a), seqKey(b))
	}
	if seqKey(a) == seqKey(c) {
		t.Fatalf("seqKey collision on different arg shape: %q", seqKey(a))
	}
	k1, k2 := (&ChainTemplate{Sequence: a}).Key(), (&ChainTemplate{Sequence: b}).Key()
	if k1 != k2 {
		t.Fatalf("ChainTemplate.Key not deterministic: %q != %q", k1, k2)
	}
}

func TestDropContainedRemovesSubpatterns(t *testing.T) {
	longer := &ChainTemplate{Sequence: []ToolCall{{Tool: "a", ArgShape: "{}"}, {Tool: "b", ArgShape: "{}"}}}
	shorter := &ChainTemplate{Sequence: []ToolCall{{Tool: "a", ArgShape: "{}"}}}
	kept := dropContained([]*ChainTemplate{longer, shorter})
	if len(kept) != 1 {
		t.Fatalf("expected 1 surviving template, got %d", len(kept))
	}
	if kept[0] != longer {
		t.Fatalf("expected longer template to survive, got %+v", kept[0].Sequence)
	}
}

// ── Context Compiler ──────────────────────────────────────────────

func TestContextCompilerPinnedItemsFirst(t *testing.T) {
	cc := NewContextCompiler(100000)
	items := []ContextItem{
		{Kind: "file", Name: "loose", Body: "x", Relevance: 0.99},
		{Kind: "contract", Name: "c1", Body: "you may only edit foo.go", Pinned: true},
		{Kind: "file", Name: "loose2", Body: "y", Relevance: 0.98},
		{Kind: "diagnosis", Name: "d1", Body: "build broken", Pinned: true},
	}
	out, err := cc.Compile(items)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	firstNonPinned := -1
	for i, name := range out.Included {
		isPinned := name == "contract:c1" || name == "diagnosis:d1"
		if isPinned {
			if firstNonPinned >= 0 {
				t.Fatalf("pinned %q appears after non-pinned at index %d", name, firstNonPinned)
			}
			continue
		}
		if firstNonPinned == -1 {
			firstNonPinned = i
		}
	}
	if firstNonPinned == -1 {
		t.Fatalf("expected at least one non-pinned item in Included: %v", out.Included)
	}
}

func TestContextCompilerRespectsBudget(t *testing.T) {
	// tokens = len(Body)/4 + 8. A 4000-char body costs 1008 tokens.
	big := strings.Repeat("a", 4000)
	small := strings.Repeat("b", 4) // tokens = 1+8=9
	cc := NewContextCompiler(1020)  // room for one big + one small (1008+9=1017)
	// High relevance on the big item so it sorts first and gets included.
	items := []ContextItem{
		{Kind: "file", Name: "f1", Body: big, Relevance: 12.0},
		{Kind: "file", Name: "f2", Body: small, Relevance: 0.01},
		{Kind: "file", Name: "f3", Body: small, Relevance: 0.01},
	}
	out, err := cc.Compile(items)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if out.Used > cc.Budget {
		t.Fatalf("Used=%d exceeds budget %d", out.Used, cc.Budget)
	}
	found := false
	for _, e := range out.Evicted {
		if strings.Contains(e, "f3") && strings.Contains(e, "budget") {
			found = true
		}
		if strings.Contains(e, "budget") && !strings.Contains(e, "f3") {
			t.Fatalf("unexpected budget eviction: %q", e)
		}
	}
	if !found {
		t.Fatalf("expected f3 evicted with 'budget' reason, got evicted=%v", out.Evicted)
	}
}

func TestContextCompilerKindCapsEvict(t *testing.T) {
	cc := NewContextCompiler(100000)
	items := make([]ContextItem, 7)
	for i := range items {
		items[i] = ContextItem{Kind: "file", Name: "f" + string(rune('a'+i)), Body: "x", Relevance: 0.5}
	}
	out, err := cc.Compile(items)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	includedFiles, evictedKindCap := 0, 0
	for _, n := range out.Included {
		if strings.HasPrefix(n, "file:") {
			includedFiles++
		}
	}
	for _, e := range out.Evicted {
		if strings.Contains(e, "kind cap") {
			evictedKindCap++
		}
	}
	if includedFiles != 6 {
		t.Fatalf("expected 6 file items included, got %d (included=%v)", includedFiles, out.Included)
	}
	if evictedKindCap != 1 {
		t.Fatalf("expected 1 'kind cap' eviction, got %d (evicted=%v)", evictedKindCap, out.Evicted)
	}
}

func TestContextCompilerPinnedExceedsBudget(t *testing.T) {
	cc := NewContextCompiler(100)
	items := []ContextItem{
		{Kind: "contract", Name: "big", Pinned: true, Body: strings.Repeat("z", 4000)},
	}
	_, err := cc.Compile(items)
	if err == nil {
		t.Fatal("expected error for pinned item exceeding budget")
	}
	if !strings.Contains(err.Error(), "pinned") || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("expected error mentioning pinned+budget, got: %v", err)
	}
}

func TestContextCompilerEmptyInput(t *testing.T) {
	cc := NewContextCompiler(1000)
	out, err := cc.Compile(nil)
	if err != nil {
		t.Fatalf("Compile(nil): %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil CompiledContext")
	}
	if out.Prompt != "" || len(out.Included) != 0 || len(out.Evicted) != 0 || out.Used != 0 {
		t.Fatalf("expected empty CompiledContext, got %+v", out)
	}
	if out.Budget != 1000 {
		t.Fatalf("expected budget propagated, got %d", out.Budget)
	}
}

// ── GatherStandard + contractBrief ────────────────────────────────

func TestGatherStandardIncludesContractWhenPresent(t *testing.T) {
	c := &Contract{TaskID: "task-42", AllowedGlobs: []string{"internal/foo/*"}, MaxFilesChanged: 5}
	items := GatherStandard(c, "", nil, nil, "", nil, nil)
	var got *ContextItem
	for i := range items {
		if items[i].Kind == "contract" {
			got = &items[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected a contract item, got %+v", items)
	}
	if !got.Pinned {
		t.Fatal("contract item must be pinned")
	}
	if got.Name != "task-42" {
		t.Fatalf("contract name = %q, want task-42", got.Name)
	}
}

func TestGatherStandardExcludesEmptyOptionals(t *testing.T) {
	items := GatherStandard(nil, "", nil, nil, "", nil, nil)
	for _, it := range items {
		switch it.Kind {
		case "diagnosis", "impact", "suggestion", "episode":
			t.Fatalf("did not expect %s item with empty optionals: %+v", it.Kind, items)
		}
	}
}

func TestContractBriefIncludesAllowedAndFrozen(t *testing.T) {
	c := &Contract{
		TaskID:          "t1",
		AllowedGlobs:    []string{"internal/foo/*"},
		FrozenGlobs:     []string{"go.sum", "*.lock"},
		MaxFilesChanged: 5,
		MaxLinesChanged: 200,
	}
	brief := contractBrief(c)
	for _, want := range []string{"internal/foo/*", "go.sum", "*.lock", "5", "200"} {
		if !strings.Contains(brief, want) {
			t.Fatalf("contractBrief missing %q\n--- brief ---\n%s", want, brief)
		}
	}
}

// ── Kernel ────────────────────────────────────────────────────────

func TestKernelNilDBIsSafe(t *testing.T) {
	k, err := NewKernel(nil, "/tmp")
	if err != nil {
		t.Fatalf("NewKernel(nil, /tmp) error: %v", err)
	}
	if k == nil {
		t.Fatal("expected non-nil Kernel for nil-DB")
	}
	// Capture without workdir avoids the git subprocess path; TreeSHA must be "".
	k.Workdir = ""
	cp, err := k.Capture(context.Background(), "label1", AgentState{TaskID: "t1"}, true)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if cp == nil {
		t.Fatal("expected non-nil Checkpoint")
	}
	if cp.TreeSHA != "" {
		t.Fatalf("expected TreeSHA=\"\" without workdir, got %q", cp.TreeSHA)
	}
	if cp.Label != "label1" {
		t.Fatalf("Label=%q, want label1", cp.Label)
	}
}

func TestKernelRewindWithoutDBErrors(t *testing.T) {
	k, _ := NewKernel(nil, "/tmp")
	_, err := k.Rewind(context.Background(), 1)
	if err == nil {
		t.Fatal("expected Rewind to error on nil-db kernel")
	}
	if !strings.Contains(err.Error(), "no DB") {
		t.Fatalf("expected 'no DB' error, got: %v", err)
	}
}

func TestKernelLastGreenWithoutDBErrors(t *testing.T) {
	k, _ := NewKernel(nil, "/tmp")
	_, _, err := k.LastGreen(context.Background())
	if err == nil {
		t.Fatal("expected LastGreen to error on nil-db kernel")
	}
	if !strings.Contains(err.Error(), "no DB") {
		t.Fatalf("expected 'no DB' error, got: %v", err)
	}
}

func TestKernelTimelineEmpty(t *testing.T) {
	k, _ := NewKernel(nil, "/tmp")
	s, err := k.Timeline(context.Background(), 10)
	if err != nil {
		t.Fatalf("Timeline: %v", err)
	}
	if s != "" {
		t.Fatalf("expected empty timeline for nil-db kernel, got %q", s)
	}
}

func TestHashScratchpadDeterministic(t *testing.T) {
	a := HashScratchpad([]byte("hello world"))
	b := HashScratchpad([]byte("hello world"))
	c := HashScratchpad([]byte("hello WORLD"))
	if a != b {
		t.Fatalf("HashScratchpad not deterministic: %q != %q", a, b)
	}
	if a == c {
		t.Fatalf("HashScratchpad collision on different input: %q", a)
	}
	if len(a) == 0 {
		t.Fatal("HashScratchpad returned empty string")
	}
}

// ── Cartographer ──────────────────────────────────────────────────

func TestCartographerEmptyRootIsSafe(t *testing.T) {
	c := NewCartographer("")
	if c == nil {
		t.Fatal("NewCartographer(\"\") returned nil")
	}
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatalf("IndexAll on empty root should be a no-op: %v", err)
	}
	items := c.SliceFor(nil, 5)
	if len(items) != 0 {
		t.Fatalf("SliceFor on empty cart should return 0 items, got %d", len(items))
	}
}

func TestCartographerSliceForEmptyGraph(t *testing.T) {
	// Use a real empty temp dir so IndexAll's walk completes cleanly with 0 .go files.
	dir := t.TempDir()
	c := NewCartographer(dir)
	if err := c.IndexAll(context.Background()); err != nil {
		t.Fatalf("IndexAll on empty dir: %v", err)
	}
	if c.SymbolCount() != 0 {
		t.Fatalf("expected 0 symbols on empty graph, got %d", c.SymbolCount())
	}
	items := c.SliceFor(nil, 5)
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestNormalizeClampsValues(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{5, 1},
		{-1, 0},
		{0.5, 0.5},
		{1, 1},
		{0, 0},
		{1.0001, 1},
		{-0.0001, 0},
	}
	for _, c := range cases {
		if got := normalize(c.in); got != c.want {
			t.Errorf("normalize(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ── Semantic Merge ────────────────────────────────────────────────

func parseGoString(t *testing.T, src []byte) string {
	t.Helper()
	if _, err := parser.ParseFile(token.NewFileSet(), "", src, 0); err != nil {
		return err.Error()
	}
	return ""
}

func TestSemanticMergeGoNoConflicts(t *testing.T) {
	src := `package x
func Foo() int { return 1 }
`
	res, err := SemanticMergeGo([]byte(src), []byte(src), []byte(src))
	if err != nil {
		t.Fatalf("SemanticMergeGo: %v", err)
	}
	if res.AutoMerged != 0 {
		t.Fatalf("AutoMerged=%d, want 0 (identical inputs)", res.AutoMerged)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("Conflicts=%v, want 0", res.Conflicts)
	}
	if len(res.Merged) == 0 {
		t.Fatal("Merged is empty")
	}
	if perr := parseGoString(t, res.Merged); perr != "" {
		t.Fatalf("Merged not valid Go: %s\n--- merged ---\n%s", perr, string(res.Merged))
	}
}

func TestSemanticMergeGoNonOverlappingChanges(t *testing.T) {
	// base: Foo + Qux. A modifies Foo, B modifies Qux. Non-overlapping → auto-merge.
	baseSrc := `package x
func Foo() int { return 1 }
func Qux() int { return 1 }
`
	aSrc := `package x
func Foo() int { return 2 }
func Qux() int { return 1 }
`
	bSrc := `package x
func Foo() int { return 1 }
func Qux() int { return 2 }
`
	res, err := SemanticMergeGo([]byte(baseSrc), []byte(aSrc), []byte(bSrc))
	if err != nil {
		t.Fatalf("SemanticMergeGo: %v", err)
	}
	if res.AutoMerged != 2 {
		t.Fatalf("AutoMerged=%d, want 2 (A and B each modified a different func)", res.AutoMerged)
	}
	if len(res.Conflicts) != 0 {
		t.Fatalf("Conflicts=%v, want 0 (non-overlapping changes)", res.Conflicts)
	}
	if len(res.Merged) == 0 {
		t.Fatal("Merged is empty")
	}
	if perr := parseGoString(t, res.Merged); perr != "" {
		t.Fatalf("Merged not valid Go: %s\n--- merged ---\n%s", perr, string(res.Merged))
	}
}

func TestSemanticMergeGoSameFuncDifferentBodies(t *testing.T) {
	baseSrc := `package x
func Foo() int { return 1 }
`
	aSrc := `package x
func Foo() int { return 2 }
`
	bSrc := `package x
func Foo() int { return 3 }
`
	res, err := SemanticMergeGo([]byte(baseSrc), []byte(aSrc), []byte(bSrc))
	if err != nil {
		t.Fatalf("SemanticMergeGo: %v", err)
	}
	if res.AutoMerged != 0 {
		t.Fatalf("AutoMerged=%d, want 0 (genuine conflict)", res.AutoMerged)
	}
	if len(res.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict on shared func, got %d (%+v)", len(res.Conflicts), res.Conflicts)
	}
	if res.Conflicts[0].Key != "func:Foo" {
		t.Fatalf("conflict key=%q, want func:Foo", res.Conflicts[0].Key)
	}
}

func TestSemmergeConflictBriefIncludesBoth(t *testing.T) {
	baseSrc := `package x
func Foo() int { return 1 }
`
	aSrc := `package x
func Foo() int { return 2 }
`
	bSrc := `package x
func Foo() int { return 3 }
`
	res, err := SemanticMergeGo([]byte(baseSrc), []byte(aSrc), []byte(bSrc))
	if err != nil {
		t.Fatalf("SemanticMergeGo: %v", err)
	}
	brief := res.ConflictBrief()
	if !strings.Contains(brief, "version A") {
		t.Fatalf("ConflictBrief missing 'version A'\n%s", brief)
	}
	if !strings.Contains(brief, "version B") {
		t.Fatalf("ConflictBrief missing 'version B'\n%s", brief)
	}
}

func TestUnionKeysDedupes(t *testing.T) {
	a := map[string]Decl{"foo": {}, "bar": {}}
	b := map[string]Decl{"bar": {}, "baz": {}}
	c := map[string]Decl{"qux": {}, "foo": {}}
	keys := unionKeys(a, b, c)
	seen := map[string]int{}
	for _, k := range keys {
		seen[k]++
	}
	if len(seen) != 4 {
		t.Fatalf("expected 4 unique keys {foo,bar,baz,qux}, got %v", seen)
	}
	for k, n := range seen {
		if n != 1 {
			t.Fatalf("key %q appeared %d times, want 1", k, n)
		}
	}
}

// ── Adversary ─────────────────────────────────────────────────────

type stubAdversary struct{ attacks []Attack }

func (s *stubAdversary) ProposeAttacks(_ context.Context, _, _ string, _ int) ([]Attack, error) {
	return s.attacks, nil
}

func TestAdversaryEmptyWorkdirRejected(t *testing.T) {
	stub := &stubAdversary{attacks: []Attack{
		{Kind: AttackBoundary, Hypothesis: "empty input crashes", ProbeSource: "package foo\n"},
	}}
	adv := &Adversary{Agent: stub, Workdir: "", MaxAttacks: 1, ProbeTimeout: 1000000000}
	res, err := adv.Review(context.Background(), "diff", "impact")
	if err != nil {
		t.Fatalf("Review: %v", err)
	}
	if len(res.Attacks) != 1 {
		t.Fatalf("expected 1 attack recorded, got %d", len(res.Attacks))
	}
	if !strings.Contains(res.Attacks[0].Output, "probe error") {
		t.Fatalf("expected attack output to contain 'probe error', got %q", res.Attacks[0].Output)
	}
	if res.Attacks[0].Landed {
		t.Fatal("empty workdir must not record a real landed attack")
	}
	if res.Landed != 0 {
		t.Fatalf("res.Landed=%d, want 0 (probe never ran)", res.Landed)
	}
}

func TestAdversaryCounterexampleBriefCleared(t *testing.T) {
	res := &AdversaryResult{Attacks: []Attack{{Kind: AttackBoundary, Hypothesis: "h"}}, Landed: 0, Cleared: true}
	brief := res.CounterexampleBrief()
	if !strings.Contains(brief, "cleared") {
		t.Fatalf("cleared brief should mention 'cleared', got: %q", brief)
	}
	if strings.Contains(brief, "LANDED") {
		t.Fatalf("cleared brief must not say 'LANDED', got: %q", brief)
	}
}

func TestAdversaryCounterexampleBriefLanded(t *testing.T) {
	res := &AdversaryResult{
		Attacks: []Attack{
			{Kind: AttackBoundary, Hypothesis: "h1", Landed: true, Output: "FAIL: x", ProbeSource: "package x\n"},
		},
		Landed:  1,
		Cleared: false,
	}
	brief := res.CounterexampleBrief()
	if !strings.Contains(brief, "LANDED") {
		t.Fatalf("landed brief should contain 'LANDED', got: %q", brief)
	}
	if !strings.Contains(brief, "Reproducible counterexamples") {
		t.Fatalf("landed brief should contain 'Reproducible counterexamples', got: %q", brief)
	}
}

func TestAdversaryResultDecisionStrings(t *testing.T) {
	if string(DecisionAutoMerge) != "auto-merge" {
		t.Errorf("DecisionAutoMerge = %q, want %q", DecisionAutoMerge, "auto-merge")
	}
	if string(DecisionGreenReview) != "green-needs-review" {
		t.Errorf("DecisionGreenReview = %q, want %q", DecisionGreenReview, "green-needs-review")
	}
	if string(DecisionBlock) != "block" {
		t.Errorf("DecisionBlock = %q, want %q", DecisionBlock, "block")
	}
}
