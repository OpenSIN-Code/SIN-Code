// SPDX-License-Identifier: MIT

package internal

import (
	"errors"
	"testing"
)

// mockProvider is a test double for astProvider.
type mockProvider struct {
	langs  []string
	engine string
	err    error
}

func (m mockProvider) languages() []string { return m.langs }

func (m mockProvider) parse(path string, src []byte) (*FileOutline, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &FileOutline{Engine: m.engine}, nil
}

// nilProvider always returns nil so parseOutline must fall back to the
// no-engine result.
type nilProvider struct{}

func (nilProvider) languages() []string { return []string{"ruby"} }

func (nilProvider) parse(path string, src []byte) (*FileOutline, error) { return nil, nil }

// saveRegistry returns a deep copy of the global providerRegistry.
func saveRegistry() map[string]astProvider {
	cpy := make(map[string]astProvider, len(providerRegistry))
	for k, v := range providerRegistry {
		cpy[k] = v
	}
	return cpy
}

// restoreRegistry replaces the global providerRegistry with saved.
func restoreRegistry(saved map[string]astProvider) {
	providerRegistry = saved
}

func TestRegisterProvider_NoOverride(t *testing.T) {
	saved := saveRegistry()
	defer restoreRegistry(saved)

	// "go" is already registered by goASTProvider. With override=false the
	// existing provider must stay in place.
	registerProvider(mockProvider{langs: []string{"go"}, engine: "mock"}, false)

	if got := outlineEngineFor("test.go"); got != "go/ast" {
		t.Errorf("outlineEngineFor(test.go) = %q, want %q (original provider should be preserved)", got, "go/ast")
	}
}

func TestRegisterProvider_Override(t *testing.T) {
	saved := saveRegistry()
	defer restoreRegistry(saved)

	// With override=true the mock provider should replace the real one.
	registerProvider(mockProvider{langs: []string{"go"}, engine: "mock"}, true)

	if got := outlineEngineFor("test.go"); got != "mock" {
		t.Errorf("outlineEngineFor(test.go) = %q, want %q (mock provider should override)", got, "mock")
	}
}

func TestOutlineEngineFor_ParseError(t *testing.T) {
	saved := saveRegistry()
	defer restoreRegistry(saved)

	// Register a provider that always errors. outlineEngineFor should fall
	// through to the no-engine branch (the `_ = filepath.Ext(path)` line is
	// exercised on this path).
	registerProvider(mockProvider{
		langs: []string{"python"},
		err:   errors.New("mock parse error"),
	}, true)

	if got := outlineEngineFor("test.py"); got != "none" {
		t.Errorf("outlineEngineFor(test.py) = %q, want %q when parser errors", got, "none")
	}
}

func TestOutlineEngineFor_NoProvider(t *testing.T) {
	// Unknown extension returns "none" without touching the registry.
	if got := outlineEngineFor("test.unknown"); got != "none" {
		t.Errorf("outlineEngineFor(test.unknown) = %q, want %q", got, "none")
	}
}

func TestAstProvider_ParseOutline(t *testing.T) {
	t.Run("go", func(t *testing.T) {
		src := []byte("package main\n\nfunc main() {}\n")
		out := parseOutline("test.go", src)
		if out.Engine != "go/ast" {
			t.Errorf("parseOutline engine = %q, want %q", out.Engine, "go/ast")
		}
		if out.Language != "go" {
			t.Errorf("parseOutline language = %q, want %q", out.Language, "go")
		}
		if len(out.Symbols) != 1 || out.Symbols[0].Name != "main" {
			t.Errorf("parseOutline symbols = %+v, want [main]", out.Symbols)
		}
	})

	t.Run("python", func(t *testing.T) {
		src := []byte("def foo():\n    pass\n")
		out := parseOutline("test.py", src)
		if out.Engine != "structural" {
			t.Errorf("parseOutline engine = %q, want %q", out.Engine, "structural")
		}
		if out.Language != "python" {
			t.Errorf("parseOutline language = %q, want %q", out.Language, "python")
		}
		if len(out.Symbols) != 1 || out.Symbols[0].Name != "foo" {
			t.Errorf("parseOutline symbols = %+v, want [foo]", out.Symbols)
		}
	})

	t.Run("unknown", func(t *testing.T) {
		out := parseOutline("test.unknown", []byte("anything"))
		if out.Engine != "none" {
			t.Errorf("parseOutline engine = %q, want %q", out.Engine, "none")
		}
		if out.Language != "unknown" {
			t.Errorf("parseOutline language = %q, want %q", out.Language, "unknown")
		}
	})

	t.Run("error", func(t *testing.T) {
		saved := saveRegistry()
		defer restoreRegistry(saved)
		registerProvider(mockProvider{langs: []string{"ruby"}, err: errors.New("mock error")}, true)

		out := parseOutline("test.rb", []byte("def foo():\n    pass\n"))
		if out.Engine != "none" {
			t.Errorf("parseOutline engine = %q, want %q", out.Engine, "none")
		}
		if out.Language != "ruby" {
			t.Errorf("parseOutline language = %q, want %q", out.Language, "ruby")
		}
	})

	t.Run("nil", func(t *testing.T) {
		saved := saveRegistry()
		defer restoreRegistry(saved)
		registerProvider(nilProvider{}, true)

		out := parseOutline("test.rb", []byte("def foo():\n    pass\n"))
		if out.Engine != "none" {
			t.Errorf("parseOutline engine = %q, want %q", out.Engine, "none")
		}
		if out.Language != "ruby" {
			t.Errorf("parseOutline language = %q, want %q", out.Language, "ruby")
		}
	})
}

func TestAstProvider_FindSymbol(t *testing.T) {
	outline := &FileOutline{
		Symbols: []SymbolInfo{
			{Name: "main", Kind: "func", StartLine: 1, EndLine: 5},
			{Name: "Server", Kind: "struct", StartLine: 7, EndLine: 15,
				Children: []SymbolInfo{
					{Name: "Server.handle", Kind: "method", StartLine: 10, EndLine: 14},
					{Name: "Server.nested", Kind: "struct", StartLine: 16, EndLine: 20,
						Children: []SymbolInfo{
							{Name: "Server.nested.deep", Kind: "method", StartLine: 18, EndLine: 19},
						}},
				}},
		},
	}

	t.Run("direct", func(t *testing.T) {
		hits := findSymbol(outline, "main")
		if len(hits) != 1 || hits[0].Name != "main" {
			t.Errorf("findSymbol(main) = %+v, want [main]", hits)
		}
	})

	t.Run("qualified", func(t *testing.T) {
		hits := findSymbol(outline, "handle")
		if len(hits) != 1 || hits[0].Name != "Server.handle" {
			t.Errorf("findSymbol(handle) = %+v, want [Server.handle]", hits)
		}
	})

	t.Run("deeply nested", func(t *testing.T) {
		hits := findSymbol(outline, "deep")
		if len(hits) != 1 || hits[0].Name != "Server.nested.deep" {
			t.Errorf("findSymbol(deep) = %+v, want [Server.nested.deep]", hits)
		}
	})

	t.Run("not found", func(t *testing.T) {
		hits := findSymbol(outline, "missing")
		if len(hits) != 0 {
			t.Errorf("findSymbol(missing) = %+v, want []", hits)
		}
	})

	t.Run("empty", func(t *testing.T) {
		hits := findSymbol(&FileOutline{}, "main")
		if len(hits) != 0 {
			t.Errorf("findSymbol(empty) = %+v, want []", hits)
		}
	})
}
