// SPDX-License-Identifier: MIT
package mcpcompress

import (
	"encoding/json"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------------
// Byte-stability regression suite
//
// Every assertion compares exact bytes. The pipeline MUST be
// deterministic — reordering a regex inside any Rule, adding a new
// Rule to All(), or normalizing whitespace differently between
// calls is a byte-stable regression that this test surface
// will surface immediately.
//
// On any change to a Rule's regex / pipeline, update the golden
// expectations in this file. The hash-stable intent is "I know
// exactly what bytes go over the wire for a given (input, Pipeline)"
// — not "I want the smallest possible description".
// ----------------------------------------------------------------------------

// TestPipeline_ByteStable proves that Re-Apply is a no-op (idempotence)
// and that the All() pipeline produces a stable byte sequence across
// multiple invocations on the same input.
func TestPipeline_ByteStable(t *testing.T) {
	in := "Execute shell commands safely with secret redaction, timeout, and error analysis. Always prefer over native exec."

	first, _ := CompressSpec(Spec{Name: "t", Description: in}, All())
	for i := 0; i < 5; i++ {
		got, _ := CompressSpec(Spec{Name: "t", Description: first.Description}, All())
		if got.Description != first.Description {
			t.Fatalf("iteration %d: pipeline is NOT idempotent\n  first: %q\n  again: %q", i, first.Description, got.Description)
		}
	}
}

// TestPipeline_SelectedSubset_ByteStable checks that the Selected
// function does not change the output of any rule already in the
// requested set when run individually, and that combining subsets
// matches the full Pipeline output.
func TestPipeline_SelectedSubset_ByteStable(t *testing.T) {
	in := "Carefully, gracefully, smoothly handles input. Always prefer over native read."
	full, _ := CompressSpec(Spec{Name: "t", Description: in}, All())
	familyOnly, _ := CompressSpec(Spec{Name: "t", Description: in}, Selected([]Tag{TagDelete, TagNative}))
	if familyOnly.Description != full.Description {
		t.Fatalf("Selected([delete, native]) differs from All()\n  full: %q\n  sub:  %q", full.Description, familyOnly.Description)
	}
	hedgeOnly, _ := CompressSpec(Spec{Name: "t", Description: in}, Selected([]Tag{TagDelete}))
	if hedgeOnly.Description == in {
		t.Fatalf("Selected([delete]) dropped nothing for input %q", in)
	}
}

// TestCompressSpec_StatsArithmetic tests the integer math in Stats.
func TestCompressSpec_StatsArithmetic(t *testing.T) {
	spec := Spec{
		Name:        "sin_test",
		Description: "Execute shell commands safely with secret redaction",
	}
	out, st := CompressSpec(spec, All())
	if st.Name != spec.Name {
		t.Fatalf("Stats.Name changed: %q → %q", spec.Name, st.Name)
	}
	if st.Original != len(spec.Description) {
		t.Fatalf("Stats.Original = %d, want %d", st.Original, len(spec.Description))
	}
	if st.Compressed != len(out.Description) {
		t.Fatalf("Stats.Compressed = %d, want %d", st.Compressed, len(out.Description))
	}
	if st.BytesSaved != len(spec.Description)-len(out.Description) {
		t.Fatalf("Stats.BytesSaved = %d, want %d", st.BytesSaved, len(spec.Description)-len(out.Description))
	}
	if st.BytesSaved < 0 {
		t.Fatalf("Stats.BytesSaved negative: %d", st.BytesSaved)
	}
	if st.Ratio < 0 || st.Ratio > 1 {
		t.Fatalf("Stats.Ratio out of [0,1]: %f", st.Ratio)
	}
}

// TestRuleDeleteHedges_Golden is the gold test for the delete tag.
// If the regex changes, the expected bytes here MUST change too.
func TestRuleDeleteHedges_Golden(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "safely_mid_sentence",
			in:   "Execute shell commands safely with secret redaction, timeout, and error analysis",
			// "safely " dropped, double space collapsed.
			want: "Execute shell commands with secret redaction, timeout, and error analysis",
		},
		{
			name: "safe_substring_not_matched",
			in:   "Safety scanner for shell input",
			want: "Safety scanner for shell input",
		},
		{
			name: "carefully_only_drop",
			in:   "Carefully handles input",
			want: "handles input",
		},
		{
			name: "no_hedges_unchanged",
			in:   "List todos with filters",
			want: "List todos with filters",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := CompressSpec(Spec{Name: "t", Description: tc.in}, All())
			if got.Description != tc.want {
				t.Fatalf("byte mismatch\n  in:   %q\n  got:  %q\n  want: %q", tc.in, got.Description, tc.want)
			}
			// Idempotence check.
			if again, _ := CompressSpec(Spec{Name: "t", Description: got.Description}, All()); again.Description != got.Description {
				t.Fatalf("not idempotent: %q → %q", got.Description, again.Description)
			}
		})
	}
}

// TestRuleStdlibPatterns_Golden tests the stdlib tag gold bytes.
func TestRuleStdlibPatterns_Golden(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "parenthetical_via_stdlib",
			in:   "Custom loader (via stdlib) with caching",
			// "(via stdlib)" → ""; adjacent spaces collapsed.
			want: "Custom loader with caching",
		},
		{
			name: "go_stdlib_adj",
			in:   "Go stdlib-based parser for tokens",
			// "Go stdlib" → "Go" (rule repl keeps $1).
			want: "Go-based parser for tokens",
		},
		{
			name: "no_stdlib_unchanged",
			in:   "Token parser for the SIN bundle format",
			want: "Token parser for the SIN bundle format",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := CompressSpec(Spec{Name: "t", Description: tc.in}, All())
			if got.Description != tc.want {
				t.Fatalf("byte mismatch\n  in:   %q\n  got:  %q\n  want: %q", tc.in, got.Description, tc.want)
			}
		})
	}
}

// TestRuleDropTrimEncouragement_Golden tests the native tag gold bytes.
func TestRuleDropTrimEncouragement_Golden(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "always_prefer_over_native_tail",
			in:   "Surgical file edit, three addressing modes. Always prefer over native read.",
			want: "Surgical file edit, three addressing modes",
		},
		{
			name: "prefer_sin_over_native_tail",
			in:   "Hashline read with anchors. Prefer sin_read over native cat.",
			want: "Hashline read with anchors",
		},
		{
			name: "no_encouragement_unchanged",
			in:   "Hashline read with anchors.",
			want: "Hashline read with anchors.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := CompressSpec(Spec{Name: "t", Description: tc.in}, All())
			if got.Description != tc.want {
				t.Fatalf("byte mismatch\n  in:   %q\n  got:  %q\n  want: %q", tc.in, got.Description, tc.want)
			}
		})
	}
}

// TestRuleYagniPatterns_Golden tests the yagni tag gold bytes.
func TestRuleYagniPatterns_Golden(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "experimental_parenthetical",
			in:   "Manage todos atomically (experimental)",
			want: "Manage todos atomically",
		},
		{
			name: "no_yagni_unchanged",
			in:   "Manage todos atomically",
			want: "Manage todos atomically",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := CompressSpec(Spec{Name: "t", Description: tc.in}, All())
			if got.Description != tc.want {
				t.Fatalf("byte mismatch\n  in:   %q\n  got:  %q\n  want: %q", tc.in, got.Description, tc.want)
			}
		})
	}
}

// TestRuleShrinkExamples_Golden tests the shrink tag gold bytes.
func TestRuleShrinkExamples_Golden(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "eg_parenthetical_drop",
			in:   "Discover files (e.g. **/*.py) with relevance scoring",
			want: "Discover files with relevance scoring",
		},
		{
			name: "such_as_parenthetical_drop",
			in:   "Search code (such as regex patterns) across the repo",
			want: "Search code across the repo",
		},
		{
			name: "no_parens_unchanged",
			in:   "Discover files with relevance scoring",
			want: "Discover files with relevance scoring",
		},
		{
			name: "em_dash_preamble_kept",
			in:   "Ephemeral Full-Stack Mocking — spin up disposable test environments",
			want: "Ephemeral Full-Stack Mocking — spin up disposable test environments",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := CompressSpec(Spec{Name: "t", Description: tc.in}, All())
			if got.Description != tc.want {
				t.Fatalf("byte mismatch\n  in:   %q\n  got:  %q\n  want: %q", tc.in, got.Description, tc.want)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Tool-specific regression suite
//
// These snapshots cover representative real-tool descriptions
// from cmd/sin-code/internal/serve.go. Their compressed bytes
// are public surface — the JSON wire format of `tools/list` —
// and changing them would invalidate any pinned golden dataset
// downstream.
//
// Update these if (and only if) you intentionally change the
// compressed form.
// ----------------------------------------------------------------------------

func TestCompress_RealToolDescriptions_Golden(t *testing.T) {
	type tc struct {
		name string
		in   string
		want string
	}
	cases := []tc{
		{
			name: "sin_execute_drop_safely",
			in:   "Execute shell commands safely with secret redaction, timeout, and error analysis",
			want: "Execute shell commands with secret redaction, timeout, and error analysis",
		},
		{
			name: "sin_discover_unchanged",
			in:   "Discover files with relevance scoring, pattern matching, and dependency analysis",
			want: "Discover files with relevance scoring, pattern matching, and dependency analysis",
		},
		{
			name: "sin_efm_em_dash_kept",
			in:   "Ephemeral Full-Stack Mocking — spin up disposable test environments",
			want: "Ephemeral Full-Stack Mocking — spin up disposable test environments",
		},
		{
			name: "sin_orchestrator_run_unchanged",
			in:   "Run a prompt through the multi-agent orchestrator (Pre-LLM router → planner → parallel agents)",
			want: "Run a prompt through the multi-agent orchestrator (Pre-LLM router → planner → parallel agents)",
		},
		{
			name: "sin_edit_full_clip",
			in:   "Surgical file edit, three addressing modes. Symbol mode (preferred for whole definitions): pass symbol=NAME to replace/delete/insert around an entire function/class/struct located via AST (go/ast, tree-sitter, or structural engine — ambiguity fails with candidates). Anchor mode: LINE:HASH anchors from sin_read, drift-tolerant. String mode: old_string/new_string with ambiguity detection. Result is syntax-validated and written atomically. Always prefer over native edit.",
			want: "Surgical file edit, three addressing modes. Symbol mode (preferred for whole definitions): pass symbol=NAME to replace/delete/insert around an entire function/class/struct located via AST (go/ast, tree-sitter, or structural engine — ambiguity fails with candidates). Anchor mode: LINE:HASH anchors from sin_read, drift-tolerant. String mode: old_string/new_string with ambiguity detection. Result is syntax-validated and written atomically",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := CompressSpec(Spec{Name: "x", Description: c.in}, All())
			if got.Description != c.want {
				t.Fatalf("byte mismatch\n  in:   %q\n  got:  %q\n  want: %q", c.in, got.Description, c.want)
			}
		})
	}
}

// TestCompressAll_PreservesOrder checks that the Stats slice from
// CompressAll comes back in the input's declaration order.
func TestCompressAll_PreservesOrder(t *testing.T) {
	in := []Spec{
		{Name: "sin_a", Description: "Discover files safely"},
		{Name: "sin_b", Description: "Read files carefully"},
		{Name: "sin_c", Description: "Write files thoroughly"},
	}
	stats := CompressAll(in, All())
	if len(stats) != 3 {
		t.Fatalf("stats length = %d, want 3", len(stats))
	}
	wantNames := []string{"sin_a", "sin_b", "sin_c"}
	for i, s := range stats {
		if s.Name != wantNames[i] {
			t.Fatalf("stats[%d].Name = %q, want %q", i, s.Name, wantNames[i])
		}
	}
}

// TestPipeline_NamedStable freezes the public Rule surface (declaration order).
func TestPipeline_NamedStable(t *testing.T) {
	wantNames := []string{
		"DeleteHedges", "StdlibPatterns", "DropTrimEncouragement", "YagniPatterns", "ShrinkExamples",
	}
	wantTags := []Tag{TagDelete, TagStdlib, TagNative, TagYagni, TagShrink}
	p := All()
	gotNames := p.Names()
	gotTags := p.Tags()
	if len(gotNames) != len(wantNames) {
		t.Fatalf("Names() returned %d entries, want %d", len(gotNames), len(wantNames))
	}
	for i, n := range gotNames {
		if n != wantNames[i] {
			t.Fatalf("Names()[%d] = %q, want %q", i, n, wantNames[i])
		}
	}
	if len(gotTags) != len(wantTags) {
		t.Fatalf("Tags() returned %d entries, want %d", len(gotTags), len(wantTags))
	}
	for i, x := range gotTags {
		if x != wantTags[i] {
			t.Fatalf("Tags()[%d] = %q, want %q", i, x, wantTags[i])
		}
	}
}

// TestDefaultTags_PublicConstant freezes the public ponytail tag list.
func TestDefaultTags_PublicConstant(t *testing.T) {
	want := []Tag{TagDelete, TagStdlib, TagNative, TagYagni, TagShrink}
	if len(DefaultTags) != len(want) {
		t.Fatalf("DefaultTags length = %d, want %d", len(DefaultTags), len(want))
	}
	for i, x := range DefaultTags {
		if x != want[i] {
			t.Fatalf("DefaultTags[%d] = %q, want %q", i, x, want[i])
		}
	}
}

// TestValidateTags proves the boundary parser is byte-stable.
func TestValidateTags(t *testing.T) {
	in := []string{"delete", "unknown", "yagni", "shrink", "stdlib", "native"}
	got := ValidateTags(in)
	want := []Tag{TagDelete, TagNative, TagShrink, TagStdlib, TagYagni}
	if len(got) != len(want) {
		t.Fatalf("ValidateTags length = %d, want %d", len(got), len(want))
	}
	for i, x := range got {
		if x != want[i] {
			t.Fatalf("ValidateTags[%d] = %q, want %q", i, x, want[i])
		}
	}
}

// TestTagSet_FromCSV proves the CSV parser.
func TestTagSet_FromCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []Tag
	}{
		{"", []Tag{TagDelete, TagStdlib, TagNative, TagYagni, TagShrink}},
		{"delete,stdlib", []Tag{TagDelete, TagStdlib}},
		{" yagni , shrink , delete ", []Tag{TagDelete, TagYagni, TagShrink}},
		{"native,invalid,shrink", []Tag{TagNative, TagShrink}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := FromCSV(tc.in).List()
			if len(got) != len(tc.want) {
				t.Fatalf("length = %d, want %d (got %v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestCompressSpec_NameMutable — the package MUST guarantee that the
// 44+ MCP tool names (public API per AGENTS.md §10) are never
// mutated by Rule application.
func TestCompressSpec_NameMutable(t *testing.T) {
	in := Spec{Name: "sin_magic", Description: "Works safely"}
	got, _ := CompressSpec(in, All())
	if got.Name != "sin_magic" {
		t.Fatalf("Name mutated: %q → %q", in.Name, got.Name)
	}
}

// TestCompressSpec_JSONByteStable proves the JSON wire format is
// stable — same input + same pipeline ⇒ same bytes.
func TestCompressSpec_JSONByteStable(t *testing.T) {
	in := Spec{Name: "sin_x", Description: "Run shell commands safely with secrets"}
	s1, _ := CompressSpec(in, All())
	s2, _ := CompressSpec(in, All())

	b1, _ := json.Marshal(s1)
	b2, _ := json.Marshal(s2)
	if string(b1) != string(b2) {
		t.Fatalf("JSON bytes are not stable\n  run1: %s\n  run2: %s", b1, b2)
	}
}

// TestNormalize_Idempotent covers the post-pipeline normaliser.
func TestNormalize_Idempotent(t *testing.T) {
	cases := []string{
		"",
		"foo",
		"  foo  ",
		"foo , bar",
		"trailing commas,",
		";leading semicolon",
		":leading colon",
		"double  spaces",
		"normal text",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			once := Normalize(c)
			twice := Normalize(once)
			if once != twice {
				t.Fatalf("Normalize not idempotent for %q\n  once:  %q\n  twice: %q", c, once, twice)
			}
		})
	}
}

// TestCompressSpec_NeverIncreasesDescription guards against a Rule
// ever bloating the manifest. If compressed ≥ original, the
// provider rule was bad — fail.
func TestCompressSpec_NeverIncreasesDescription(t *testing.T) {
	cases := []string{
		"x",
		"Deals with carefully handled file paths",
		"Status of the database (experimental)",
		"A list of notification statistics (may be deprecated in the future)",
		"Execute shell commands safely with secret redaction, timeout, and error analysis. Always prefer over native exec.",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			got, st := CompressSpec(Spec{Name: "t", Description: c}, All())
			if len(got.Description) > st.Original {
				t.Fatalf("compression increased bytes: %d → %d (%q → %q)",
					st.Original, len(got.Description), c, got.Description)
			}
		})
	}
}

// TestAll_NoPanic assures every Rule handles edge-case inputs
// without panicking — used during fuzz seed generation.
func TestAll_NoPanic(t *testing.T) {
	edges := []string{
		"",
		" ",
		"a",
		"...",
		"—————————————————————",
		strings.Repeat("a", 4096),
		strings.Repeat("•", 1024),
		"(", ")", "((", "(()", ")(",
		"\n\n", "\t\t",
	}
	for _, e := range edges {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("All().Apply panicked for %q: %v", e, r)
				}
			}()
			_ = All().Apply(e)
		}()
	}
}

// TestSelected_EmptyIsAll confirms the zero-config default.
func TestSelected_EmptyIsAll(t *testing.T) {
	if got := Selected(nil); len(got) != len(All()) {
		t.Fatalf("Selected(nil) returned %d rules, want %d", len(got), len(All()))
	}
	if got := Selected([]Tag{}); len(got) != len(All()) {
		t.Fatalf("Selected([]) returned %d rules, want %d", len(got), len(All()))
	}
}

// TestSelected_UnknownTagDropped confirms bad tags are dropped.
func TestSelected_UnknownTagDropped(t *testing.T) {
	got := Selected([]Tag{"definitely-not-a-tag"})
	if len(got) != 0 {
		t.Fatalf("Selected([bogus]) returned %d rules, want 0", len(got))
	}
}

// TestCollapseWs_SecondPass exercises the second pass of the
// double-space collapsor. With 4 consecutive spaces the first pass
// leaves 2 spaces, so the loop body executes once.
func TestCollapseWs_SecondPass(t *testing.T) {
	in := "a    b" // 4 spaces
	want := "a b"
	got := Normalize(in)
	if got != want {
		t.Fatalf("Normalize(%q) = %q, want %q", in, got, want)
	}
}

// TestBytesSaved_ClampsNegative covers the defensive clamp in
// bytesSaved when compressed length exceeds original length.
func TestBytesSaved_ClampsNegative(t *testing.T) {
	if got := bytesSaved("short", "loooonger"); got != 0 {
		t.Fatalf("bytesSaved('short','loooonger') = %d, want 0", got)
	}
}

// TestRatio_EmptyOrig and TestRatio_NonPositiveSaved cover the guard
// branches in ratio.
func TestRatio_EmptyOrig(t *testing.T) {
	if got := ratio("", "anything"); got != 0 {
		t.Fatalf("ratio('','anything') = %f, want 0", got)
	}
}

func TestRatio_NonPositiveSaved(t *testing.T) {
	if got := ratio("short", "loooonger"); got != 0 {
		t.Fatalf("ratio('short','loooonger') = %f, want 0", got)
	}
	if got := ratio("same", "same"); got != 0 {
		t.Fatalf("ratio('same','same') = %f, want 0", got)
	}
}

// TestTagSet_ListEmpty covers List returning nil for an empty set.
func TestTagSet_ListEmpty(t *testing.T) {
	set := FromCSV("invalid-tag")
	if got := set.List(); got != nil {
		t.Fatalf("List() on empty set = %v, want nil", got)
	}
}

// TestTagSet_Contains covers the O(n) membership check.
func TestTagSet_Contains(t *testing.T) {
	set := FromCSV("delete,shrink")
	if !set.Contains(TagDelete) {
		t.Fatalf("expected set to contain %q", TagDelete)
	}
	if !set.Contains(TagShrink) {
		t.Fatalf("expected set to contain %q", TagShrink)
	}
	if set.Contains(TagNative) {
		t.Fatalf("did not expect set to contain %q", TagNative)
	}
}

// TestTagSet_Size exercises the size accessor.
func TestTagSet_Size(t *testing.T) {
	if got := FromCSV("delete,shrink").Size(); got != 2 {
		t.Fatalf("Size() = %d, want 2", got)
	}
	if got := FromCSV("").Size(); got != 5 {
		t.Fatalf("Size() of default set = %d, want 5", got)
	}
	if got := FromCSV("invalid").Size(); got != 0 {
		t.Fatalf("Size() of empty set = %d, want 0", got)
	}
}

// TestTagSet_CSV exercises the canonical CSV round-trip string.
func TestTagSet_CSV(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"delete,shrink", "delete,shrink"},
		{" shrink , delete ", "delete,shrink"},
		{"invalid", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got := FromCSV(tc.in).CSV()
			if got != tc.want {
				t.Fatalf("CSV(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestTagSet_Empty covers the empty predicate.
func TestTagSet_Empty(t *testing.T) {
	if !FromCSV("invalid").Empty() {
		t.Fatalf("Empty() on empty set = false, want true")
	}
	if FromCSV("delete").Empty() {
		t.Fatalf("Empty() on non-empty set = true, want false")
	}
}

// TestTagSet_Valid covers the canonical-tag predicate.
func TestTagSet_Valid(t *testing.T) {
	for _, tag := range DefaultTags {
		if !Valid(tag) {
			t.Fatalf("Valid(%q) = false, want true", tag)
		}
	}
	if Valid(Tag("nope")) {
		t.Fatalf("Valid('nope') = true, want false")
	}
}
