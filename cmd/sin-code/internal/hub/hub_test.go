// SPDX-License-Identifier: MIT
// Purpose: Tests for the hub tool catalog.
// Docs: hub.doc.md
package hub

import (
	"strings"
	"testing"
)

func TestDefaultCatalogNonEmpty(t *testing.T) {
	cats := DefaultCatalog()
	if len(cats) == 0 {
		t.Fatal("DefaultCatalog() returned empty")
	}
	total := 0
	for _, c := range cats {
		if c.Name == "" {
			t.Fatal("category has no name")
		}
		if c.Description == "" {
			t.Fatalf("category %q has no description", c.Name)
		}
		if len(c.Tools) == 0 {
			t.Fatalf("category %q has no tools", c.Name)
		}
		for _, tool := range c.Tools {
			if tool.Name == "" || tool.Short == "" || tool.Description == "" {
				t.Fatalf("invalid tool in category %q: %+v", c.Name, tool)
			}
		}
		total += len(c.Tools)
	}
	if total < 30 {
		t.Fatalf("catalog too small: got %d tools, expected at least 30", total)
	}
}

func TestAllToolsFlat(t *testing.T) {
	if len(AllTools()) < 30 {
		t.Fatalf("AllTools() returned %d tools", len(AllTools()))
	}
}

func TestSearchFound(t *testing.T) {
	for _, q := range []string{"discover", "map", "security", "vane", "gh", "orchestrate"} {
		res := Search(q)
		found := false
		for _, r := range res {
			if strings.EqualFold(r.Name, q) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Search(%q) did not find exact tool; got %+v", q, res)
		}
	}
}

func TestSearchByDescription(t *testing.T) {
	res := Search("citation")
	found := false
	for _, r := range res {
		if r.Name == "vane" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Search('citation') should find vane, got %+v", res)
	}
}

func TestSearchEmptyReturnsAll(t *testing.T) {
	all := Search("")
	if len(all) != len(AllTools()) {
		t.Fatalf("empty query should return all tools, got %d vs %d", len(all), len(AllTools()))
	}
}

func TestSearchNoMatch(t *testing.T) {
	res := Search("zzzzzzzzzzzzzzzz")
	if len(res) != 0 {
		t.Fatalf("expected no results for nonsense query, got %d", len(res))
	}
}

func TestFormatList(t *testing.T) {
	out := FormatList(AllTools())
	if !strings.Contains(out, "discover") {
		t.Fatal("FormatList missing discover tool")
	}
	if !strings.Contains(out, "Find files") {
		t.Fatal("FormatList missing short description")
	}
}

func TestFormatListEmpty(t *testing.T) {
	out := FormatList(nil)
	if out != "No tools found." {
		t.Fatalf("unexpected empty list output: %q", out)
	}
}

func TestFormatDetail(t *testing.T) {
	out := FormatDetail(Tool{Name: "test", Short: "short", Description: "desc", Example: "ex"})
	for _, want := range []string{"Name:", "Short:", "Description:", "Example:"} {
		if !strings.Contains(out, want) {
			t.Fatalf("FormatDetail missing %q; got %q", want, out)
		}
	}
}

func TestFormatCategories(t *testing.T) {
	out := FormatCategories(DefaultCatalog())
	if !strings.Contains(out, "Core Analysis") {
		t.Fatal("FormatCategories missing Core Analysis category")
	}
	if !strings.Contains(out, "discover") {
		t.Fatal("FormatCategories missing discover tool")
	}
}

func TestFormatCategoriesEmpty(t *testing.T) {
	out := FormatCategories(nil)
	if out != "Catalog is empty." {
		t.Fatalf("unexpected empty categories output: %q", out)
	}
}

func TestCatalogNoDuplicateNames(t *testing.T) {
	seen := make(map[string]bool)
	for _, tool := range AllTools() {
		if seen[tool.Name] {
			t.Fatalf("duplicate tool name: %q", tool.Name)
		}
		seen[tool.Name] = true
	}
}

func TestCatalogIncludesRecentTools(t *testing.T) {
	recent := []string{"gh", "update", "stack", "vane", "superpowers", "dox"}
	all := AllTools()
	names := make(map[string]bool)
	for _, tool := range all {
		names[tool.Name] = true
	}
	for _, want := range recent {
		if !names[want] {
			t.Fatalf("recent tool %q missing from catalog", want)
		}
	}
}
