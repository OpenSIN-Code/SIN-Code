// SPDX-License-Identifier: MIT
// Purpose: sin-code lsp CLI — wrapper around Language Server Protocol
// (gopls, pyright, tsserver) for IDE-grade code intelligence.
package internal

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lsp"
)

var (
	lspLang     string
	lspRoot     string
	lspFile     string
	lspLine     int
	lspCol      int
	lspNewName  string
)

var LSPCmd = &cobra.Command{
	Use:   "lsp",
	Short: "LSP (Language Server Protocol) — IDE-grade code intelligence",
	Long: `Language Server Protocol wrapper for sin-code. Provides go-to-definition,
find-references, hover, rename, document symbols, formatting, and diagnostics
without launching an IDE. Spawns gopls/pyright/typescript-language-server
on demand and caches them per-language.

Examples:
  sin-code lsp servers                    # list detected LSPs
  sin-code lsp definition main.go 5 9     # go-to-def at line 5, col 9
  sin-code lsp references main.go 5 9    # find all references
  sin-code lsp hover main.go 5 9         # type/doc on hover
  sin-code lsp rename main.go 5 9 MyFunc # rename symbol
  sin-code lsp symbols main.go            # outline
  sin-code lsp format main.go             # format file
  sin-code lsp diagnostics main.go        # all errors/warnings`,
	SilenceUsage: true,
}

func init() {
	LSPCmd.PersistentFlags().StringVar(&lspRoot, "root", "", "Project root (default: current dir)")

	LSPCmd.AddCommand(lspServersCmd)
	LSPCmd.AddCommand(lspDefinitionCmd)
	LSPCmd.AddCommand(lspReferencesCmd)
	LSPCmd.AddCommand(lspHoverCmd)
	LSPCmd.AddCommand(lspRenameCmd)
	LSPCmd.AddCommand(lspSymbolsCmd)
	LSPCmd.AddCommand(lspFormatCmd)
	LSPCmd.AddCommand(lspDiagnosticsCmd)

	for _, c := range []*cobra.Command{lspDefinitionCmd, lspReferencesCmd, lspHoverCmd, lspRenameCmd, lspSymbolsCmd, lspFormatCmd, lspDiagnosticsCmd} {
		c.Flags().StringVar(&lspFile, "file", "", "File path (relative to --root, or absolute)")
	}
	lspDefinitionCmd.Flags().IntVar(&lspLine, "line", 0, "Line (0-indexed)")
	lspDefinitionCmd.Flags().IntVar(&lspCol, "col", 0, "Column (0-indexed)")
	lspReferencesCmd.Flags().IntVar(&lspLine, "line", 0, "Line (0-indexed)")
	lspReferencesCmd.Flags().IntVar(&lspCol, "col", 0, "Column (0-indexed)")
	lspReferencesCmd.Flags().Bool("include-decl", true, "Include declaration in results")
	lspHoverCmd.Flags().IntVar(&lspLine, "line", 0, "Line (0-indexed)")
	lspHoverCmd.Flags().IntVar(&lspCol, "col", 0, "Column (0-indexed)")
	lspRenameCmd.Flags().IntVar(&lspLine, "line", 0, "Line (0-indexed)")
	lspRenameCmd.Flags().IntVar(&lspCol, "col", 0, "Column (0-indexed)")
	lspRenameCmd.Flags().StringVar(&lspNewName, "new-name", "", "New symbol name (required)")
}

var lspServersCmd = &cobra.Command{
	Use:   "servers",
	Short: "List detected LSP servers on PATH",
	RunE: func(cmd *cobra.Command, args []string) error {
		specs := lsp.DetectAvailable()
		if len(specs) == 0 {
			fmt.Println("(no LSP servers detected on PATH)")
			fmt.Println("Install one of: gopls (go), pyright-langserver (python), typescript-language-server (ts/js)")
			return nil
		}
		if orch2Format == "json" {
			return json.NewEncoder(os.Stdout).Encode(specs)
		}
		fmt.Printf("Detected %d LSP server(s):\n", len(specs))
		for _, s := range specs {
			fmt.Printf("  %-12s  binary=%s  exts=%s\n", s.Language, s.Binary, strings.Join(s.FileExts, ","))
		}
		return nil
	},
}


var lspDefinitionCmd = &cobra.Command{
	Use:   "definition <file> <line> <col>",
	Short: "Go to definition at file:line:col",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRun(cmd, args, func(c *lsp.Client, uri string, line, col int) (any, error) {
			positions, err := c.Definition(uri, lsp.Position{Line: line, Character: col})
			if err != nil {
				return nil, err
			}
			return positions, nil
		})
	},
}

var lspReferencesCmd = &cobra.Command{
	Use:   "references <file> <line> <col>",
	Short: "Find all references to the symbol at file:line:col",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		includeDecl, _ := cmd.Flags().GetBool("include-decl")
		return lspRun(cmd, args, func(c *lsp.Client, uri string, line, col int) (any, error) {
			return c.References(uri, lsp.Position{Line: line, Character: col}, includeDecl)
		})
	},
}

var lspHoverCmd = &cobra.Command{
	Use:   "hover <file> <line> <col>",
	Short: "Show type/doc on hover at file:line:col",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRun(cmd, args, func(c *lsp.Client, uri string, line, col int) (any, error) {
			return c.Hover(uri, lsp.Position{Line: line, Character: col})
		})
	},
}

var lspRenameCmd = &cobra.Command{
	Use:   "rename <file> <line> <col> <new-name>",
	Short: "Rename symbol at file:line:col to <new-name>",
	Args:  cobra.RangeArgs(1, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRun(cmd, args, func(c *lsp.Client, uri string, line, col int) (any, error) {
			if lspNewName == "" {
				return nil, fmt.Errorf("--new-name required")
			}
			return c.Rename(uri, lsp.Position{Line: line, Character: col}, lspNewName)
		})
	},
}

var lspSymbolsCmd = &cobra.Command{
	Use:   "symbols <file>",
	Short: "Show document outline (symbols) for a file",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRunSimple(cmd, args, func(c *lsp.Client, uri string) (any, error) {
			return c.Symbols(uri)
		})
	},
}

var lspFormatCmd = &cobra.Command{
	Use:   "format <file>",
	Short: "Format a file using the LSP textDocument/formatting",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRunSimple(cmd, args, func(c *lsp.Client, uri string) (any, error) {
			return c.Format(uri)
		})
	},
}

var lspDiagnosticsCmd = &cobra.Command{
	Use:   "diagnostics <file>",
	Short: "Read file contents and return what diagnostics the LSP reports",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return lspRunSimple(cmd, args, func(c *lsp.Client, uri string) (any, error) {
			text, err := os.ReadFile(stripURI(uri))
			if err != nil {
				return nil, err
			}
			_ = c.DidOpen(lsp.TextDocumentItem{URI: uri, LanguageID: langForPath(uri), Version: 1, Text: string(text)})
			return map[string]any{"file": uri, "hint": "diagnostics arrive via publishDiagnostics notification, not request; use LSP for full stream"}, nil
		})
	},
}

func lspRun(cmd *cobra.Command, args []string, fn func(c *lsp.Client, uri string, line, col int) (any, error)) error {
	if err := lspParseArgs(args, true); err != nil {
		return err
	}
	if lspNewName != "" && cmd.Name() != "rename" {
		return nil
	}
	mgr, rootURI, fileURI, err := lspSetup(cmd, lspFile, true)
	if err != nil {
		return err
	}
	defer mgr.Close()
	lang := lsp.LanguageForFile(lspFile)
	if lang == "" {
		lang = lspLang
	}
	if lang == "" {
		return fmt.Errorf("could not determine language for %s (use --lang)", lspFile)
	}
	c, err := mgr.Get(lang, rootURI)
	if err != nil {
		return err
	}
	if text, err := os.ReadFile(stripURI(fileURI)); err == nil {
		_ = c.DidOpen(lsp.TextDocumentItem{URI: fileURI, LanguageID: lang, Version: 1, Text: string(text)})
	}
	out, err := fn(c, fileURI, lspLine, lspCol)
	if err != nil {
		return err
	}
	if orch2Format == "json" {
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	printLSPResult(cmd.Name(), out)
	return nil
}

func lspRunSimple(cmd *cobra.Command, args []string, fn func(c *lsp.Client, uri string) (any, error)) error {
	if err := lspParseArgs(args, false); err != nil {
		return err
	}
	mgr, rootURI, fileURI, err := lspSetup(cmd, lspFile, false)
	if err != nil {
		return err
	}
	defer mgr.Close()
	lang := lsp.LanguageForFile(lspFile)
	if lang == "" {
		lang = lspLang
	}
	if lang == "" {
		return fmt.Errorf("could not determine language for %s", lspFile)
	}
	c, err := mgr.Get(lang, rootURI)
	if err != nil {
		return err
	}
	if text, err := os.ReadFile(stripURI(fileURI)); err == nil {
		_ = c.DidOpen(lsp.TextDocumentItem{URI: fileURI, LanguageID: lang, Version: 1, Text: string(text)})
	}
	out, err := fn(c, fileURI)
	if err != nil {
		return err
	}
	if orch2Format == "json" {
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	printLSPResult(cmd.Name(), out)
	return nil
}

func lspParseArgs(args []string, withPos bool) error {
	if len(args) > 0 {
		lspFile = args[0]
	}
	if withPos {
		if len(args) > 1 {
			n, err := strconv.Atoi(args[1])
			if err != nil {
				return fmt.Errorf("invalid line: %s", args[1])
			}
			lspLine = n
		}
		if len(args) > 2 {
			n, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("invalid col: %s", args[2])
			}
			lspCol = n
		}
	}
	return nil
}

func lspSetup(cmd *cobra.Command, fileFlag string, withPos bool) (*lsp.Manager, string, string, error) {
	root := lspRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return nil, "", "", err
		}
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, "", "", err
	}
	if lspFile == "" && fileFlag != "" {
		lspFile = fileFlag
	}
	if !strings.HasPrefix(lspFile, "/") {
		lspFile = filepath.Join(rootAbs, lspFile)
	}
	rootURI := (&url.URL{Scheme: "file", Path: rootAbs}).String()
	fileURI := (&url.URL{Scheme: "file", Path: lspFile}).String()
	if lspLine == 0 && withPos {
		return nil, "", "", fmt.Errorf("--line required (0-indexed)")
	}
	if lspCol == 0 && withPos {
		return nil, "", "", fmt.Errorf("--col required (0-indexed)")
	}
	return lsp.NewManager(), rootURI, fileURI, nil
}

func stripURI(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		u, err := url.PathUnescape(uri[len("file://"):])
		if err == nil {
			return u
		}
	}
	return uri
}

func langForPath(p string) string {
	return lsp.LanguageForFile(p)
}

func printLSPResult(cmd string, out any) {
	switch v := out.(type) {
	case []lsp.Location:
		if len(v) == 0 {
			fmt.Println("(no results)")
			return
		}
		for _, loc := range v {
			fmt.Printf("%s:%d:%d\n", stripURI(loc.URI), loc.Range.Start.Line+1, loc.Range.Start.Character+1)
		}
	case *lsp.Hover:
		if v == nil {
			fmt.Println("(no hover info)")
			return
		}
		fmt.Printf("%v\n", v.Contents)
	case []lsp.DocumentSymbol:
		if len(v) == 0 {
			fmt.Println("(no symbols)")
			return
		}
		for _, s := range v {
			fmt.Printf("  %s\n", s.Name)
		}
	case *lsp.WorkspaceEdit:
		if v == nil {
			fmt.Println("(no edit)")
			return
		}
		for uri, edits := range v.Changes {
			for _, e := range edits {
				fmt.Printf("%s:%d:%d  +%q\n", stripURI(uri), e.Range.Start.Line+1, e.Range.Start.Character+1, e.NewText)
			}
		}
	case []lsp.TextEdit:
		for _, e := range v {
			fmt.Printf("  +%q\n", e.NewText)
		}
	case map[string]any:
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
	default:
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
	}
}
