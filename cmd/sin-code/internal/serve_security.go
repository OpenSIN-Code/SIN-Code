// SPDX-License-Identifier: MIT
// Purpose: serve — security scan and SBOM generation MCP tool handlers.
// sin-debt: shrink, upgrade: when a second security-related function is needed, merge into a shared file
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	securityMarshalIndent = json.MarshalIndent
	sbomMarshalIndent     = json.MarshalIndent
	sbomEncode            = func(enc *json.Encoder, v any) error { return enc.Encode(v) }
)

// handleSecurity dispatches sin_security_scan MCP calls to the existing
// CLI subcommand. Mirrors cmd/sin-code/internal/security.go SecurityCmd
// flags: --type, --tools, --format, --timeout, --strict. The path is a
// required positional argument. Timeout is clamped to 3600s (1h) max.
func handleSecurity(ctx context.Context, args map[string]any) (string, error) {
	path := stringArg(args, "path", ".")
	projType := stringArg(args, "type", "auto")
	toolFilter := stringArg(args, "tools", "")
	format := stringArg(args, "format", "json")
	timeout := intArg(args, "timeout", 300)
	strict := boolArg(args, "strict")

	abs, err := pathAbs(path)
	if err != nil {
		return "", fmt.Errorf("security: resolve path: %w", err)
	}
	if projType == "" || projType == "auto" {
		projType = detectProjectType(abs)
	}

	// Hard ceiling: 1 hour regardless of what the caller asks for.
	// Without this cap a misbehaving client could pin a goroutine for
	// 24h via --timeout, which mirrors the per-tool timeout already
	// enforced by runWithTimeout in security.go.
	if timeout <= 0 || timeout > 3600 {
		timeout = 3600
	}

	result := runSecurityScan(abs, projType, toolFilter, timeout)
	result.Strict = strict

	if format == "json" {
		out, mErr := securityMarshalIndent(result, "", "  ")
		if mErr != nil {
			return "", mErr
		}
		return string(out), nil
	}
	return formatSecurityResultText(result), nil
}

// handleSbom dispatches sin_sbom_generate MCP calls to the existing
// CLI subcommand. Mirrors cmd/sin-code/internal/sbom.go SbomCmd flags:
// --format, --output. The path is a required positional argument.
// Output paths that escape the scan root are rejected to prevent the
// MCP layer from being a write-anywhere primitive.
func handleSbom(ctx context.Context, args map[string]any) (string, error) {
	path := stringArg(args, "path", ".")
	format := stringArg(args, "format", "spdx-json")
	output := stringArg(args, "output", "")

	abs, err := pathAbs(path)
	if err != nil {
		return "", fmt.Errorf("sbom: resolve path: %w", err)
	}

	projType := detectProjectType(abs)
	doc, err := generateSBOM(abs, projType, format)
	if err != nil {
		return "", fmt.Errorf("sbom generation failed: %w", err)
	}

	if output == "" || output == "-" {
		out, mErr := sbomMarshalIndent(doc, "", "  ")
		if mErr != nil {
			return "", mErr
		}
		return string(out), nil
	}

	// File-output path: refuse if outside the worktree to prevent
	// the MCP layer from being a write-anywhere primitive.
	absOut, _ := filepath.Abs(output)
	if !strings.HasPrefix(absOut, abs+string(filepath.Separator)) &&
		absOut != abs {
		return "", fmt.Errorf("sbom: output path %q escapes scan root %q", absOut, abs)
	}
	f, ferr := os.Create(absOut)
	if ferr != nil {
		return "", fmt.Errorf("sbom: create output: %w", ferr)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := sbomEncode(enc, doc); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote SBOM to %s", absOut), nil
}

// formatSecurityResultText returns a human-readable string for a
// SecurityResult, mirroring printSecurityResult in security.go but
// writing to a string instead of stdout.
func formatSecurityResultText(r SecurityResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\U0001f512 Security Scan Summary \u2014 %s project at %s\n", r.ProjectType, r.Path)
	fmt.Fprintf(&b, "   Duration: %s\n\n", r.Duration)

	for _, t := range r.Tools {
		switch t.Status {
		case "ok":
			fmt.Fprintf(&b, "   \u2705 %-20s  %s  (no issues)\n", t.Name, t.Duration)
		case "issues":
			fmt.Fprintf(&b, "   \u26a0\ufe0f  %-20s  %s  (%d issues)\n", t.Name, t.Duration, t.Issues)
		case "error":
			fmt.Fprintf(&b, "   \u274c %-20s  %s  ERROR: %s\n", t.Name, t.Duration, t.Error)
		case "not_found":
			fmt.Fprintf(&b, "   \u23ed\ufe0f  %-20s  not installed\n", t.Name)
		case "skipped":
			fmt.Fprintf(&b, "   \u23ed\ufe0f  %-20s  skipped\n", t.Name)
		}
	}

	fmt.Fprintf(&b, "\n   Tools run: %d  |  Issues: %d  |  Errors: %d  |  Not found: %d  |  Skipped: %d\n",
		r.Summary.ToolsRun, r.Summary.Issues, r.Summary.Errors, r.Summary.NotFound, r.Summary.Skipped)

	if r.Strict && r.Summary.Issues > 0 {
		fmt.Fprintf(&b, "\n   \u26a0\ufe0f  Strict mode: %d issues found \u2014 exiting with error\n", r.Summary.Issues)
	} else if r.Summary.Issues > 0 {
		fmt.Fprintf(&b, "\n   \u26a0\ufe0f  %d issues found \u2014 review recommended\n", r.Summary.Issues)
	} else {
		fmt.Fprintf(&b, "\n   \u2705 No security issues detected\n")
	}
	return b.String()
}
