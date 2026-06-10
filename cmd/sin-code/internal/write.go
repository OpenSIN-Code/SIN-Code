// SPDX-License-Identifier: MIT
// Purpose: write — atomic, validated file writing. Replaces naive native write:
// temp-file + fsync + rename (never a half-written file), syntax pre-validation
// before anything touches disk (Go via go/parser, JSON via encoding/json,
// bracket-balance heuristic elsewhere), and optional backup.
// Docs: cmd/sin-code/internal/write.doc.md
package internal

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	writeContent    string
	writeStdin      bool
	writeNoValidate bool
	writeBackup     bool
	writeMkdir      bool
	writeFormat     string
)

var WriteCmd = &cobra.Command{
	Use:   "write [path]",
	Short: "Write files atomically with syntax pre-validation",
	Long: `Atomic file writing: content is validated, written to a temp file in the
target directory, fsynced, then renamed over the destination. A crash or
validation failure never leaves a corrupt file behind.

Validation (skip with --no-validate):
  .go    full parse via go/parser
  .json  encoding/json
  other  bracket/brace/paren balance heuristic (string/comment aware)

Examples:
  sin-code write pkg/new.go --content "$(cat /tmp/draft.go)"
  cat draft.json | sin-code write config.json --stdin --backup
  sin-code write docs/new/file.md --stdin --mkdir < notes.md`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		content := writeContent
		if writeStdin {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			content = string(data)
		}
		result, err := writeFileAtomic(absPath, content, writeOpts{
			validate: !writeNoValidate,
			backup:   writeBackup,
			mkdir:    writeMkdir,
		})
		if err != nil {
			return err
		}
		if writeFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Printf("wrote %s (%d bytes, %d lines)%s\n", result.Path, result.Bytes, result.Lines,
			map[bool]string{true: " [backup: " + result.BackupPath + "]", false: ""}[result.BackupPath != ""])
		return nil
	},
}

func init() {
	WriteCmd.Flags().StringVarP(&writeContent, "content", "c", "", "Content to write")
	WriteCmd.Flags().BoolVar(&writeStdin, "stdin", false, "Read content from stdin")
	WriteCmd.Flags().BoolVar(&writeNoValidate, "no-validate", false, "Skip syntax pre-validation")
	WriteCmd.Flags().BoolVar(&writeBackup, "backup", false, "Keep a .bak copy of the previous content")
	WriteCmd.Flags().BoolVar(&writeMkdir, "mkdir", false, "Create parent directories if missing")
	WriteCmd.Flags().StringVarP(&writeFormat, "format", "f", "text", "Output: text, json")
}

type writeOpts struct {
	validate bool
	backup   bool
	mkdir    bool
}

type writeResult struct {
	Path       string `json:"path"`
	Bytes      int    `json:"bytes"`
	Lines      int    `json:"lines"`
	Created    bool   `json:"created"`
	Validated  bool   `json:"validated"`
	BackupPath string `json:"backup_path,omitempty"`
}

func writeFileAtomic(path, content string, opts writeOpts) (*writeResult, error) {
	if opts.validate {
		if err := validateSyntax(path, content); err != nil {
			return nil, fmt.Errorf("validation failed, nothing written: %w", err)
		}
	}

	dir := filepath.Dir(path)
	if opts.mkdir {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating parent directories: %w", err)
		}
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("parent directory missing: %s (use --mkdir)", dir)
	}

	res := &writeResult{Path: path, Bytes: len(content), Validated: opts.validate}

	prevInfo, statErr := os.Stat(path)
	res.Created = statErr != nil
	mode := os.FileMode(0644)
	if statErr == nil {
		mode = prevInfo.Mode().Perm()
		if opts.backup {
			bak := path + ".bak"
			prev, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("reading previous content for backup: %w", err)
			}
			if err := os.WriteFile(bak, prev, mode); err != nil {
				return nil, fmt.Errorf("writing backup: %w", err)
			}
			res.BackupPath = bak
		}
	}

	tmp, err := os.CreateTemp(dir, ".sin-write-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { tmp.Close(); os.Remove(tmpName) }

	if _, err := tmp.WriteString(content); err != nil {
		cleanup()
		return nil, fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return nil, fmt.Errorf("fsync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return nil, fmt.Errorf("closing temp file: %w", err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		os.Remove(tmpName)
		return nil, fmt.Errorf("chmod: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return nil, fmt.Errorf("atomic rename: %w", err)
	}

	lines, _ := SplitLines(content)
	res.Lines = len(lines)
	return res, nil
}

func validateSyntax(path, content string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		fset := token.NewFileSet()
		if _, err := parser.ParseFile(fset, filepath.Base(path), content, parser.AllErrors); err != nil {
			return fmt.Errorf("go syntax: %v", err)
		}
		return nil
	case ".json":
		var v any
		if err := json.Unmarshal([]byte(content), &v); err != nil {
			return fmt.Errorf("json syntax: %v", err)
		}
		return nil
	case ".md", ".txt", ".log", "":
		return nil
	default:
		return checkBracketBalance(path, content)
	}
}

func checkBracketBalance(path, content string) error {
	pairs := map[rune]rune{')': '(', ']': '[', '}': '{'}
	var stack []rune
	inString := rune(0)
	escaped := false
	lineComment := false
	line := 1

	for _, r := range content {
		if r == '\n' {
			line++
			lineComment = false
			if inString == '\'' || inString == '"' {
				inString = 0
			}
			continue
		}
		if lineComment {
			continue
		}
		if inString != 0 {
			if escaped {
				escaped = false
			} else if r == '\\' {
				escaped = true
			} else if r == inString {
				inString = 0
			}
			continue
		}
		switch r {
		case '"', '\'', '`':
			inString = r
		case '#':
			if isHashCommentLang(path) {
				lineComment = true
			}
		case '(', '[', '{':
			stack = append(stack, r)
		case ')', ']', '}':
			if len(stack) == 0 || stack[len(stack)-1] != pairs[r] {
				return fmt.Errorf("bracket balance: unexpected %q at line %d (use --no-validate to override)", r, line)
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) > 0 {
		return fmt.Errorf("bracket balance: %d unclosed brackets — content looks truncated (use --no-validate to override)", len(stack))
	}
	return nil
}

func isHashCommentLang(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py", ".rb", ".sh", ".bash", ".yaml", ".yml", ".toml", ".pl", ".r":
		return true
	}
	return false
}
