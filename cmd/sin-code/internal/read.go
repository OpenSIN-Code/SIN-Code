// SPDX-License-Identifier: MIT
// Purpose: read — token-efficient, anchor-aware file reading. Replaces naive
// native read/cat for agents: hashline mode emits stable edit anchors, outline
// mode emits structure instead of raw content (80–95% token savings on large
// files), and hard byte/line guards prevent context blowouts.
// Docs: cmd/sin-code/internal/read.doc.md
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/spf13/cobra"
)

var (
	readMode     string
	readOffset   int
	readLimit    int
	readMaxBytes int64
	readFormat   string
)

const readDefaultLimit = 2000
const readDefaultMaxBytes int64 = 1 << 20

var ReadCmd = &cobra.Command{
	Use:   "read [path]",
	Short: "Read files with hashline anchors, outline, and size guards",
	Long: `Token-efficient file reading for agents and humans.

Modes:
  hashline  (default) lines prefixed with "LINE:HASH|" — anchors feed 'sin-code edit'
  raw       plain content (still offset/limit guarded)
  outline   structure only: imports, functions, classes, exports (huge files)

Examples:
  sin-code read main.go
  sin-code read main.go --mode outline
  sin-code read big.log --offset 5000 --limit 200 --mode raw
  sin-code read pkg/x.go --format json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		result, err := readFile(absPath, readMode, readOffset, readLimit, readMaxBytes)
		if err != nil {
			return err
		}
		if readFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		fmt.Print(result.Content)
		if result.Truncated {
			fmt.Fprintf(os.Stderr, "\n[truncated: showing lines %d-%d of %d — use --offset/--limit, or --mode outline]\n",
				result.Offset, result.Offset+result.ReturnedLines-1, result.TotalLines)
		}
		return nil
	},
}

func init() {
	ReadCmd.Flags().StringVarP(&readMode, "mode", "m", "hashline", "Mode: hashline, raw, outline")
	ReadCmd.Flags().IntVar(&readOffset, "offset", 1, "1-based line to start from")
	ReadCmd.Flags().IntVar(&readLimit, "limit", 0, fmt.Sprintf("Max lines to return (default %d)", readDefaultLimit))
	ReadCmd.Flags().Int64Var(&readMaxBytes, "max-bytes", readDefaultMaxBytes, "Refuse raw/hashline reads of files larger than this")
	ReadCmd.Flags().StringVarP(&readFormat, "format", "f", "text", "Output: text, json")
}

type readResult struct {
	Path          string `json:"path"`
	Mode          string `json:"mode"`
	Language      string `json:"language"`
	Size          int64  `json:"size"`
	TotalLines    int    `json:"total_lines"`
	Offset        int    `json:"offset"`
	ReturnedLines int    `json:"returned_lines"`
	Truncated     bool   `json:"truncated"`
	Content       string `json:"content"`
}

func readFile(path, mode string, offset, limit int, maxBytes int64) (*readResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s (use 'sin-code discover')", path)
	}
	if mode != "outline" && maxBytes > 0 && info.Size() > maxBytes {
		return nil, fmt.Errorf("file is %d bytes (limit %d) — use --mode outline, narrow with --offset/--limit, or raise --max-bytes",
			info.Size(), maxBytes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("binary or non-UTF-8 file: %s (use 'sin-code execute' with a hex tool if needed)", path)
	}

	content := string(data)
	lang := detectLanguage(path)
	lines, _ := SplitLines(content)
	total := len(lines)

	res := &readResult{
		Path: path, Mode: mode, Language: lang,
		Size: info.Size(), TotalLines: total,
	}

	if mode == "outline" {
		return buildOutlineResult(res, content, lang), nil
	}

	if offset < 1 {
		offset = 1
	}
	if offset > total && total > 0 {
		return nil, fmt.Errorf("offset %d beyond end of file (%d lines)", offset, total)
	}
	if limit <= 0 {
		limit = readDefaultLimit
	}
	end := offset - 1 + limit
	if end > total {
		end = total
	}
	window := lines[offset-1 : end]

	res.Offset = offset
	res.ReturnedLines = len(window)
	res.Truncated = offset > 1 || end < total

	switch mode {
	case "raw":
		res.Content = JoinLines(window, true)
	case "hashline":
		res.Content = FormatHashlines(window, offset)
	default:
		return nil, fmt.Errorf("unknown mode %q: want hashline, raw, or outline", mode)
	}
	return res, nil
}

func buildOutlineResult(res *readResult, content, lang string) *readResult {
	outline := parseOutline(res.Path, []byte(content))
	exports := extractExports(content, lang)
	deps := extractDependencies(res.Path)

	outlineMap := map[string]any{
		"language":     outline.Language,
		"engine":       outline.Engine,
		"total_lines":  res.TotalLines,
		"symbols":      outline.Symbols,
		"imports":      outline.Imports,
		"exports":      exports,
		"dependencies": deps,
	}
	b, err := json.MarshalIndent(outlineMap, "", "  ")
	if err != nil {
		b = []byte(fmt.Sprintf(`{"error":"%v"}`, err))
	}
	res.Offset = 1
	res.Content = string(b) + "\n"
	return res
}
