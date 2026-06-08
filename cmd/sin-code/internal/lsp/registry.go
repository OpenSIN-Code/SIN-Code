// SPDX-License-Identifier: MIT
// Purpose: language registry — detects available LSP servers (gopls,
// pyright, tsserver) on the system PATH and provides ServerSpec for each
// so the Client knows how to spawn them.
package lsp

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

type ServerSpec struct {
	Language  string
	Binary    string
	Args      []string
	Aliases   []string
	FileExts  []string
}

var DefaultServers = []ServerSpec{
	{Language: "go", Binary: "gopls", FileExts: []string{".go"}},
	{Language: "python", Binary: "pyright-langserver", Args: []string{"--stdio"}, Aliases: []string{"pyright"}, FileExts: []string{".py"}},
	{Language: "python", Binary: "pylsp", FileExts: []string{".py"}},
	{Language: "typescript", Binary: "typescript-language-server", Args: []string{"--stdio"}, Aliases: []string{"tsserver", "ts-ls"}, FileExts: []string{".ts", ".tsx"}},
	{Language: "javascript", Binary: "typescript-language-server", Args: []string{"--stdio"}, Aliases: []string{"tsserver"}, FileExts: []string{".js", ".jsx"}},
	{Language: "rust", Binary: "rust-analyzer", FileExts: []string{".rs"}},
}

func DetectAvailable() []ServerSpec {
	var out []ServerSpec
	for _, spec := range DefaultServers {
		if _, err := exec.LookPath(spec.Binary); err == nil {
			out = append(out, spec)
		}
	}
	return out
}

func LanguageForFile(path string) string {
	ext := ""
	if i := strings.LastIndex(path, "."); i >= 0 {
		ext = strings.ToLower(path[i:])
	}
	for _, spec := range DefaultServers {
		for _, e := range spec.FileExts {
			if e == ext {
				return spec.Language
			}
		}
	}
	return ""
}

type Manager struct {
	mu    sync.Mutex
	langs map[string]*Client
}

func NewManager() *Manager {
	return &Manager{langs: map[string]*Client{}}
}

func (m *Manager) Get(lang, rootURI string) (*Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.langs[lang]; ok {
		return c, nil
	}
	spec, ok := findSpec(lang)
	if !ok {
		return nil, fmt.Errorf("no LSP server configured for language: %s", lang)
	}
	if _, err := exec.LookPath(spec.Binary); err != nil {
		return nil, fmt.Errorf("LSP binary %s not found in PATH (install with: see LSP docs for %s)", spec.Binary, lang)
	}
	c, err := Start(spec.Binary, spec.Args, lang, rootURI)
	if err != nil {
		return nil, err
	}
	m.langs[lang] = c
	return c, nil
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.langs {
		_ = c.Close()
	}
	m.langs = map[string]*Client{}
}

func findSpec(lang string) (ServerSpec, bool) {
	for _, spec := range DefaultServers {
		if spec.Language == lang {
			return spec, true
		}
		for _, a := range spec.Aliases {
			if a == lang {
				return spec, true
			}
		}
	}
	return ServerSpec{}, false
}
