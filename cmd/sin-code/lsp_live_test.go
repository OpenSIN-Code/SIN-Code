// SPDX-License-Identifier: MIT

//go:build lsp_live

package main

import (
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestLspLive runs the lsp_live.txt testscript only when the `lsp_live`
// build tag is set AND gopls is on PATH. It is intentionally opt-in
// (see st-lsp3) because the test requires gopls and a real Go file
// to LSP-query, which makes it unsuitable for the default CI target.
//
// Run locally with:    go test -tags lsp_live ./cmd/sin-code/
// CI integration:      see .github/workflows/go-ci.yml (conditional)
func TestLspLive(t *testing.T) {
	if _, err := os.Stat("/opt/homebrew/bin/gopls"); err != nil {
		// Try common locations
		for _, p := range []string{
			"/usr/local/bin/gopls",
			"/opt/homebrew/bin/gopls",
			"/usr/bin/gopls",
		} {
			if _, err := os.Stat(p); err == nil {
				_ = os.Setenv("PATH", p+":"+os.Getenv("PATH"))
				break
			}
		}
	}
	testscript.Run(t, testscript.Params{
		Dir: "testdata/scripts",
	})
}
