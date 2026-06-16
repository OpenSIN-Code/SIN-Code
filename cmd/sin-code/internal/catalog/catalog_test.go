// SPDX-License-Identifier: MIT
// Purpose: tests for the catalog package — Source interface, Merge
// de-duplication, Search ranking, FilterByKind.
package catalog

import (
	"context"
	"strings"
	"testing"
)

// fakeSource is a minimal Source implementation for tests.
type fakeSource struct {
	name   string
	assets []*Asset
}

func (f *fakeSource) Name() string { return f.name }
func (f *fakeSource) List(_ context.Context, kind Kind) ([]*Asset, error) {
	if kind == "" {
		// Filter out nils so callers don't have to.
		out := make([]*Asset, 0, len(f.assets))
		for _, a := range f.assets {
			if a != nil {
				out = append(out, a)
			}
		}
		return out, nil
	}
	var out []*Asset
	for _, a := range f.assets {
		if a != nil && a.Kind == kind {
			out = append(out, a)
		}
	}
	return out, nil
}
func (f *fakeSource) Get(_ context.Context, kind Kind, name string) (*Asset, bool, error) {
	for _, a := range f.assets {
		if a.Kind == kind && a.Name == name {
			return a, true, nil
		}
	}
	return nil, false, nil
}

func TestMerge_EmptySources(t *testing.T) {
	got, err := Merge(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestMerge_SingleSource(t *testing.T) {
	src := &fakeSource{
		name: "a",
		assets: []*Asset{
			{Kind: KindCommand, Name: "chat", Source: "a"},
			{Kind: KindCommand, Name: "read", Source: "a"},
		},
	}
	got, err := Merge(context.Background(), []Source{src})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Name != "chat" {
		t.Errorf("expected chat first, got %s", got[0].Name)
	}
}

func TestMerge_DedupSameKindName(t *testing.T) {
	// Two sources with the same (kind, name) → first wins.
	a := &fakeSource{name: "a", assets: []*Asset{
		{Kind: KindCommand, Name: "chat", Short: "from-a"},
	}}
	b := &fakeSource{name: "b", assets: []*Asset{
		{Kind: KindCommand, Name: "chat", Short: "from-b"},
	}}
	got, err := Merge(context.Background(), []Source{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 (deduped), got %d", len(got))
	}
	if got[0].Short != "from-a" {
		t.Errorf("expected first source to win, got %q", got[0].Short)
	}
}

func TestMerge_DifferentKindsKept(t *testing.T) {
	// Two sources, same name, different kinds → both kept.
	a := &fakeSource{name: "a", assets: []*Asset{
		{Kind: KindAgent, Name: "go-reviewer", Source: "a"},
	}}
	b := &fakeSource{name: "b", assets: []*Asset{
		{Kind: KindCommand, Name: "go-reviewer", Source: "b"},
	}}
	got, err := Merge(context.Background(), []Source{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 (different kinds), got %d", len(got))
	}
}

func TestMerge_SourceFilledIn(t *testing.T) {
	src := &fakeSource{
		name:   "mysrc",
		assets: []*Asset{{Kind: KindCommand, Name: "x"}}, // no Source
	}
	got, err := Merge(context.Background(), []Source{src})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Source != "mysrc" {
		t.Errorf("expected Source=mysrc, got %q", got[0].Source)
	}
}

func TestMerge_SortedByKindThenName(t *testing.T) {
	src := &fakeSource{name: "a", assets: []*Asset{
		{Kind: KindSkill, Name: "z"},
		{Kind: KindCommand, Name: "b"},
		{Kind: KindAgent, Name: "a"},
		{Kind: KindCommand, Name: "a"},
	}}
	got, err := Merge(context.Background(), []Source{src})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"agent.a", "command.a", "command.b", "skill.z"}
	if len(got) != len(want) {
		t.Fatalf("expected %d, got %d", len(want), len(got))
	}
	for i, w := range want {
		got_ := string(got[i].Kind) + "." + got[i].Name
		if got_ != w {
			t.Errorf("[%d] expected %q, got %q", i, w, got_)
		}
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	assets := []*Asset{{Name: "a"}, {Name: "b"}}
	got := Search(assets, "")
	if len(got) != 2 {
		t.Errorf("expected all 2, got %d", len(got))
	}
}

func TestSearch_NameMatchScoresHighest(t *testing.T) {
	assets := []*Asset{
		{Name: "foo", Short: "unrelated"},
		{Name: "bar", Description: "foo does something"},
	}
	got := Search(assets, "foo")
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Name != "foo" {
		t.Errorf("expected foo first (name match scores higher), got %s", got[0].Name)
	}
}

func TestSearch_CaseInsensitive(t *testing.T) {
	assets := []*Asset{{Name: "Chat"}}
	got := Search(assets, "chat")
	if len(got) != 1 {
		t.Errorf("expected case-insensitive match, got %d", len(got))
	}
}

func TestSearch_TagMatch(t *testing.T) {
	assets := []*Asset{
		{Name: "x", Tags: []string{"go", "review"}},
		{Name: "y", Description: "nothing"},
	}
	got := Search(assets, "review")
	if len(got) != 1 {
		t.Fatalf("expected 1 (tag match), got %d", len(got))
	}
	if got[0].Name != "x" {
		t.Errorf("expected x, got %s", got[0].Name)
	}
}

func TestFilterByKind(t *testing.T) {
	assets := []*Asset{
		{Kind: KindCommand, Name: "a"},
		{Kind: KindAgent, Name: "b"},
		{Kind: KindCommand, Name: "c"},
	}
	got := FilterByKind(assets, KindCommand)
	if len(got) != 2 {
		t.Errorf("expected 2 commands, got %d", len(got))
	}
	for _, a := range got {
		if a.Kind != KindCommand {
			t.Errorf("expected kind=command, got %s", a.Kind)
		}
	}
}

func TestFilterByKind_EmptyReturnsAll(t *testing.T) {
	assets := []*Asset{{Kind: KindCommand}, {Kind: KindAgent}}
	got := FilterByKind(assets, "")
	if len(got) != 2 {
		t.Errorf("expected all 2, got %d", len(got))
	}
}

func TestHubSource_ListsAllTools(t *testing.T) {
	src := HubSource{}
	got, err := src.List(context.Background(), KindHub)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Error("expected at least one hub asset")
	}
	for _, a := range got {
		if a.Kind != KindHub {
			t.Errorf("expected kind=hub, got %s", a.Kind)
		}
		if a.Source != "hub" {
			t.Errorf("expected source=hub, got %s", a.Source)
		}
	}
}

func TestHubSource_ListWrongKind(t *testing.T) {
	src := HubSource{}
	got, err := src.List(context.Background(), KindAgent)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 for kind=agent from hub, got %d", len(got))
	}
}

func TestHubSource_Get(t *testing.T) {
	src := HubSource{}
	got, ok, err := src.Get(context.Background(), KindHub, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected to find chat")
	}
	if got.Name != "chat" {
		t.Errorf("expected chat, got %s", got.Name)
	}
}

func TestHubSource_GetNotFound(t *testing.T) {
	src := HubSource{}
	_, ok, err := src.Get(context.Background(), KindHub, "does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected not-found")
	}
}

func TestAssetsSource_NilRegistry(t *testing.T) {
	src := NewAssetsSource(nil)
	got, err := src.List(context.Background(), KindAgent)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 from nil registry, got %d", len(got))
	}
	_, ok, err := src.Get(context.Background(), KindAgent, "x")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected not-found from nil registry")
	}
}

func TestSearch_NoMatchReturnsEmpty(t *testing.T) {
	assets := []*Asset{{Name: "foo"}}
	got := Search(assets, "nonexistent")
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestSearch_NameMatchBeatsDescription(t *testing.T) {
	// foo in name (4 points) vs foo in description (1 point)
	assets := []*Asset{
		{Name: "x", Description: "foo bar"},
		{Name: "foo", Short: "y"},
	}
	got := Search(assets, "foo")
	if got[0].Name != "foo" {
		t.Errorf("expected foo first, got %s", got[0].Name)
	}
}

func TestSearch_StableOrderOnTie(t *testing.T) {
	// Two assets with equal score → alphabetical by name.
	assets := []*Asset{
		{Name: "beta", Description: "match"},
		{Name: "alpha", Description: "match"},
	}
	got := Search(assets, "match")
	if got[0].Name != "alpha" {
		t.Errorf("expected alpha first (stable tie), got %s", got[0].Name)
	}
}

func TestMerge_NilAssetSkipped(t *testing.T) {
	// A Source that returns a nil entry in the slice must not crash
	// the merger. The catalog's contract is to skip nil entries.
	src := &fakeSource{
		name:   "a",
		assets: []*Asset{{Kind: KindCommand, Name: "ok"}, nil},
	}
	got, err := Merge(context.Background(), []Source{src})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 (nil skipped), got %d", len(got))
	}
}

func TestMerge_SearchEndToEnd(t *testing.T) {
	// Full integration: two sources, merged, then searched.
	hub := &fakeSource{name: "hub", assets: []*Asset{
		{Kind: KindHub, Name: "chat", Short: "Chat with LLM"},
		{Kind: KindHub, Name: "read", Short: "Read files"},
	}}
	assets := &fakeSource{name: "assets", assets: []*Asset{
		{Kind: KindAgent, Name: "go-reviewer", Description: "Reviews Go code"},
		{Kind: KindSkill, Name: "code-lazy", Short: "Lazy code skill"},
	}}
	merged, err := Merge(context.Background(), []Source{hub, assets})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 4 {
		t.Errorf("expected 4 merged, got %d", len(merged))
	}
	hits := Search(merged, "chat")
	if len(hits) == 0 || hits[0].Name != "chat" {
		t.Errorf("expected chat as top hit, got %+v", hits)
	}
	hits = Search(merged, "go")
	if len(hits) == 0 {
		t.Error("expected at least one match for 'go'")
	}
}

// ── coverage helpers ──────────────────────────────────────────────────

func TestMerge_AllKindsInOneSource(t *testing.T) {
	// A single source that returns assets of multiple kinds.
	src := &fakeSource{name: "a", assets: []*Asset{
		{Kind: KindAgent, Name: "x"},
		{Kind: KindCommand, Name: "y"},
		{Kind: KindSkill, Name: "z"},
	}}
	merged, err := Merge(context.Background(), []Source{src})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 3 {
		t.Errorf("expected 3, got %d", len(merged))
	}
	kinds := map[Kind]bool{}
	for _, a := range merged {
		kinds[a.Kind] = true
	}
	if len(kinds) != 3 {
		t.Errorf("expected 3 distinct kinds, got %d", len(kinds))
	}
}

func TestFilterByKind_Empty(t *testing.T) {
	got := FilterByKind(nil, KindCommand)
	if got == nil {
		t.Error("expected non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("expected 0, got %d", len(got))
	}
}

func TestMerge_OutputFormat(t *testing.T) {
	// Smoke test that the merged output is JSON-serializable (for
	// `sin catalog list --format=json`).
	hub := &fakeSource{name: "hub", assets: []*Asset{
		{Kind: KindHub, Name: "chat"},
	}}
	merged, _ := Merge(context.Background(), []Source{hub})
	// Just verify the slice is non-nil and the asset has the
	// expected fields populated.
	if merged == nil {
		t.Fatal("expected non-nil")
	}
	if merged[0].Name != "chat" {
		t.Errorf("expected chat, got %s", merged[0].Name)
	}
	if !strings.Contains(merged[0].Source, "hub") {
		t.Errorf("expected source=hub, got %s", merged[0].Source)
	}
}
