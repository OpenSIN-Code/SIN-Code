// SPDX-License-Identifier: MIT
// Purpose: LSP auto-configure — detects LSP servers on PATH and generates
// config for the LLM to use (issue #492).
package lspconfig

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type LSPServer struct {
	Name      string
	Language  string
	Path      string
	Version   string
	Available bool
}

var knownServers = []LSPServer{
	{Name: "gopls", Language: "go", Path: "gopls"},
	{Name: "pyright", Language: "python", Path: "pyright-langserver"},
	{Name: "typescript-language-server", Language: "typescript", Path: "typescript-language-server"},
	{Name: "rust-analyzer", Language: "rust", Path: "rust-analyzer"},
	{Name: "clangd", Language: "c/cpp", Path: "clangd"},
	{Name: "lua-language-server", Language: "lua", Path: "lua-language-server"},
	{Name: "jdtls", Language: "java", Path: "jdtls"},
}

type Detector struct {
	mu    sync.RWMutex
	found []LSPServer
}

func NewDetector() *Detector {
	return &Detector{}
}

func (d *Detector) Detect() []LSPServer {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.found = d.found[:0]

	for _, s := range knownServers {
		path, err := exec.LookPath(s.Path)
		if err == nil {
			s.Path = path
			s.Available = true
			s.Version = getVersion(s.Name, path)
			d.found = append(d.found, s)
		}
	}
	return d.found
}

func getVersion(name, path string) string {
	var cmd *exec.Cmd
	switch name {
	case "gopls":
		cmd = exec.Command(path, "version")
	case "pyright":
		cmd = exec.Command(path, "--version")
	case "rust-analyzer":
		cmd = exec.Command(path, "--version")
	case "clangd":
		cmd = exec.Command(path, "--version")
	default:
		return ""
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (d *Detector) Found() []LSPServer {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]LSPServer, len(d.found))
	copy(out, d.found)
	return out
}

func (d *Detector) ForLanguage(lang string) *LSPServer {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for i := range d.found {
		if strings.EqualFold(d.found[i].Language, lang) {
			return &d.found[i]
		}
	}
	return nil
}

func (d *Detector) ForFile(filename string) *LSPServer {
	ext := strings.ToLower(filepath.Ext(filename))
	lang := ExtToLang(ext)
	if lang == "" {
		return nil
	}
	return d.ForLanguage(lang)
}

func ExtToLang(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "typescript"
	case ".rs":
		return "rust"
	case ".c", ".cpp", ".h", ".hpp":
		return "c/cpp"
	case ".lua":
		return "lua"
	case ".java":
		return "java"
	default:
		return ""
	}
}

type Config struct {
	Servers []LSPServerConfig `json:"servers"`
}

type LSPServerConfig struct {
	Name     string   `json:"name"`
	Language string   `json:"language"`
	Command  string   `json:"command"`
	Args     []string `json:"args"`
}

func GenerateConfig(servers []LSPServer) Config {
	var config Config
	for _, s := range servers {
		config.Servers = append(config.Servers, LSPServerConfig{
			Name:     s.Name,
			Language: s.Language,
			Command:  s.Path,
			Args:     GetArgs(s.Name),
		})
	}
	return config
}

func GetArgs(name string) []string {
	switch name {
	case "gopls":
		return []string{"serve"}
	case "pyright":
		return []string{"--stdio"}
	case "typescript-language-server":
		return []string{"--stdio"}
	case "rust-analyzer":
		return []string{}
	case "clangd":
		return []string{"--background-index"}
	default:
		return []string{"--stdio"}
	}
}

func WriteConfig(config Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
