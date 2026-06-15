// SPDX-License-Identifier: MIT
// Purpose: coverage tests for hub_cmd.go — exercises root/list/search/info
// using package-level hooks so tests can control the catalog and formatting.
// Docs: hub_cmd.go
package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hub"
)

func saveHubHooks(t *testing.T) {
	t.Helper()
	origDefault := hubDefaultCatalogHook
	origAll := hubAllToolsHook
	origSearch := hubSearchHook
	origFormatCats := hubFormatCategoriesHook
	origFormatList := hubFormatListHook
	origFormatDetail := hubFormatDetailHook
	t.Cleanup(func() {
		hubDefaultCatalogHook = origDefault
		hubAllToolsHook = origAll
		hubSearchHook = origSearch
		hubFormatCategoriesHook = origFormatCats
		hubFormatListHook = origFormatList
		hubFormatDetailHook = origFormatDetail
	})
}

func runHubCmd(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	cmd := NewHubCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs(args)
	return &out, cmd.Execute()
}

func TestHubCmd_NewHubCmd(t *testing.T) {
	cmd := NewHubCmd()
	if cmd.Use != "hub" {
		t.Errorf("Use = %q, want hub", cmd.Use)
	}
	names := []string{}
	for _, c := range cmd.Commands() {
		names = append(names, c.Name())
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"list", "search", "info"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing subcommand %q in %q", want, joined)
		}
	}
}

func TestHubCmd_Root(t *testing.T) {
	saveHubHooks(t)
	hubDefaultCatalogHook = func() []hub.Category { return []hub.Category{} }
	hubFormatCategoriesHook = func([]hub.Category) string { return "formatted categories\n" }
	out, err := runHubCmd(t)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "SIN-Code Tool Catalog") {
		t.Errorf("expected catalog title, got %q", out.String())
	}
	if !strings.Contains(out.String(), "formatted categories") {
		t.Errorf("expected formatted categories, got %q", out.String())
	}
}

func TestHubCmd_List(t *testing.T) {
	saveHubHooks(t)
	hubAllToolsHook = func() []hub.Tool {
		return []hub.Tool{{Name: "discover", Short: "Find files"}}
	}
	hubFormatListHook = func([]hub.Tool) string { return "formatted list\n" }
	out, err := runHubCmd(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "formatted list") {
		t.Errorf("expected formatted list, got %q", out.String())
	}
}

func TestHubCmd_Search_NoMatch(t *testing.T) {
	saveHubHooks(t)
	hubSearchHook = func(string) []hub.Tool { return nil }
	out, err := runHubCmd(t, "search", "xyz")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No tools matched") {
		t.Errorf("expected no match message, got %q", out.String())
	}
}

func TestHubCmd_Search_Match(t *testing.T) {
	saveHubHooks(t)
	hubSearchHook = func(string) []hub.Tool {
		return []hub.Tool{{Name: "discover", Short: "Find files"}}
	}
	hubFormatListHook = func([]hub.Tool) string { return "formatted results\n" }
	out, err := runHubCmd(t, "search", "disc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Matched 1 tool(s)") {
		t.Errorf("expected match header, got %q", out.String())
	}
	if !strings.Contains(out.String(), "formatted results") {
		t.Errorf("expected formatted results, got %q", out.String())
	}
}

func TestHubCmd_Info_Found(t *testing.T) {
	saveHubHooks(t)
	hubAllToolsHook = func() []hub.Tool {
		return []hub.Tool{{Name: "discover", Short: "Find files", Description: "smart", Example: "ex"}}
	}
	hubFormatDetailHook = func(hub.Tool) string { return "formatted detail\n" }
	out, err := runHubCmd(t, "info", "discover")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "formatted detail") {
		t.Errorf("expected formatted detail, got %q", out.String())
	}
}

func TestHubCmd_Info_NotFound(t *testing.T) {
	saveHubHooks(t)
	hubAllToolsHook = func() []hub.Tool { return nil }
	_, err := runHubCmd(t, "info", "xyz")
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("expected unknown tool error, got %v", err)
	}
}

func TestHubCmd_Info_CaseInsensitive(t *testing.T) {
	saveHubHooks(t)
	hubAllToolsHook = func() []hub.Tool {
		return []hub.Tool{{Name: "Discover", Short: "Find files"}}
	}
	hubFormatDetailHook = func(hub.Tool) string { return "formatted detail\n" }
	_, err := runHubCmd(t, "info", "DISCOVER")
	if err != nil {
		t.Fatal(err)
	}
}

func TestHubCmd_CobraHooks(t *testing.T) {
	// Ensure command construction is exercised even when the format hook errors.
	saveHubHooks(t)
	hubFormatListHook = func([]hub.Tool) string { return "list" }
	hubAllToolsHook = func() []hub.Tool { return []hub.Tool{{Name: "a", Short: "b"}} }
	cmd := NewHubCmd()
	var out bytes.Buffer
	setOutAll(cmd, &out)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	_ = bytes.NewBuffer(nil)
	_ = errors.New("placeholder")
}
