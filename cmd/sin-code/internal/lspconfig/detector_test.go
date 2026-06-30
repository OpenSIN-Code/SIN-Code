// SPDX-License-Identifier: MIT
package lspconfig

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDetect(t *testing.T) {
	d := NewDetector()
	_ = d.Detect()
	_ = d.Found()
}

func TestForLanguage(t *testing.T) {
	d := NewDetector()
	d.Detect()
	if s := d.ForLanguage("go"); s != nil && !s.Available {
		t.Error("gopls should be available")
	}
}

func TestForFile(t *testing.T) {
	d := NewDetector()
	d.Detect()
	_ = d.ForFile("main.go")
	_ = d.ForFile("foo.py")
	_ = d.ForFile("unknown.xyz")
}

func TestExtToLang(t *testing.T) {
	tests := []struct{ ext, want string }{
		{".go", "go"},
		{".py", "python"},
		{".ts", "typescript"},
		{".tsx", "typescript"},
		{".js", "typescript"},
		{".rs", "rust"},
		{".c", "c/cpp"},
		{".cpp", "c/cpp"},
		{".lua", "lua"},
		{".java", "java"},
		{".xyz", ""},
	}
	for _, tt := range tests {
		got := ExtToLang(tt.ext)
		if got != tt.want {
			t.Errorf("ExtToLang(%q) = %q, want %q", tt.ext, got, tt.want)
		}
	}
}

func TestGenerateConfig(t *testing.T) {
	servers := []LSPServer{
		{Name: "gopls", Language: "go", Path: "/usr/local/bin/gopls", Available: true},
		{Name: "pyright", Language: "python", Path: "/usr/local/bin/pyright-langserver", Available: true},
	}
	config := GenerateConfig(servers)
	if len(config.Servers) != 2 {
		t.Fatalf("servers = %d", len(config.Servers))
	}
	if config.Servers[0].Name != "gopls" {
		t.Errorf("server[0] = %q", config.Servers[0].Name)
	}
}

func TestGetArgs(t *testing.T) {
	tests := []struct{ name string; wantLen int }{
		{"gopls", 1},
		{"pyright", 1},
		{"typescript-language-server", 1},
		{"rust-analyzer", 0},
		{"clangd", 1},
		{"unknown", 1},
	}
	for _, tt := range tests {
		args := GetArgs(tt.name)
		if len(args) != tt.wantLen {
			t.Errorf("GetArgs(%q) = %v, want len %d", tt.name, args, tt.wantLen)
		}
	}
}

func TestWriteConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lsp.json")
	config := Config{Servers: []LSPServerConfig{{Name: "gopls", Language: "go", Command: "/usr/bin/gopls", Args: []string{"serve"}}}}
	if err := WriteConfig(config, path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentDetect(t *testing.T) {
	d := NewDetector()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); d.Detect() }()
		go func() { defer wg.Done(); d.Found() }()
	}
	wg.Wait()
}
