// SPDX-License-Identifier: MIT
// Purpose: `sin-code compile-spec` CLI (issue #164). Reads
// .sin-code.yml, validates it, and writes the four derived
// JSON outputs to disk. Has three modes:
//
//	sin-code compile-spec                       # compile .sin-code.yml in cwd
//	sin-code compile-spec --init                # write a starter .sin-code.yml
//	sin-code compile-spec --check               # check that derived files are in sync
//	sin-code compile-spec --out <dir>           # override the output directory
//
// Docs: docs/SPEC-COMPILER.md
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/spec/compiler"
)

// NewCompileSpecCmd builds the `compile-spec` cobra subcommand.
func NewCompileSpecCmd() *cobra.Command {
	var (
		outDir   string
		initMode bool
		check    bool
		dryRun   bool
	)
	cmd := &cobra.Command{
		Use:   "compile-spec",
		Short: "Compile .sin-code.yml into the four derived JSON artifacts",
		Long: `sin-code compile-spec reads .sin-code.yml in the current
directory (or --file), validates it against the schema, and
writes the four derived JSON files the SIN-Code engines need:

  .sin/hooks.json                  (for internal/hooks/)
  internal/verify/config.json      (for internal/verify/)
  internal/permission/policies.json (for internal/permission/)
  .sin/loop.json                   (v1.1: for the loop builder)

Use --init to write a starter .sin-code.yml; use --check to
verify the derived files are in sync with the source.`,
		RunE: func(c *cobra.Command, _ []string) error {
			if initMode {
				return runCompileSpecInit(c.OutOrStdout(), outDir)
			}
			if check {
				return runCompileSpecCheck(c.OutOrStdout(), c.ErrOrStderr(), outDir)
			}
			return runCompileSpecCompile(c.OutOrStdout(), c.ErrOrStderr(), outDir, dryRun)
		},
	}
	cmd.Flags().StringVar(&outDir, "out", ".", "output directory (defaults to cwd)")
	cmd.Flags().BoolVar(&initMode, "init", false, "write a starter .sin-code.yml and exit")
	cmd.Flags().BoolVar(&check, "check", false, "verify derived files are in sync with .sin-code.yml")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be written without writing")
	return cmd
}

// runCompileSpecInit writes a starter .sin-code.yml.
func runCompileSpecInit(out io.Writer, outDir string) error {
	path := filepath.Join(outDir, compiler.DefaultFile)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("compile-spec: %s already exists", path)
	}
	// Default to a project matching the cwd name, type "go".
	name := filepath.Base(outDir)
	data := compiler.InitTemplate(name, "go")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, filemode.Default()); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s\n", path)
	return nil
}

// runCompileSpecCompile is the default path: parse, validate, emit.
func runCompileSpecCompile(out, errOut io.Writer, outDir string, dryRun bool) error {
	src := filepath.Join(outDir, compiler.DefaultFile)
	c, err := compiler.ParseFile(src)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	if err := compiler.Validate(c); err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	files, err := compilerEmitAll(c)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	for _, f := range files {
		dest := filepath.Join(outDir, f.Path)
		if dryRun {
			fmt.Fprintf(out, "would write %s (%d bytes)\n", dest, len(f.Data))
			continue
		}
		// Atomic write: temp file + rename, so a crash mid-write
		// never leaves a half-written file behind.
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		tmp := dest + ".tmp"
		if err := os.WriteFile(tmp, f.Data, filemode.Default()); err != nil {
			return err
		}
		if err := os.Rename(tmp, dest); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s (%d bytes)\n", dest, len(f.Data))
	}
	return nil
}

// runCompileSpecCheck verifies the derived files are in sync.
// Returns exit 0 if in sync, exit 1 if any file is stale.
func runCompileSpecCheck(out, errOut io.Writer, outDir string) error {
	src := filepath.Join(outDir, compiler.DefaultFile)
	c, err := compiler.ParseFile(src)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	if err := compiler.Validate(c); err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	files, err := compilerEmitAll(c)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		os.Exit(1)
	}
	drift := false
	for _, f := range files {
		dest := filepath.Join(outDir, f.Path)
		existing, err := os.ReadFile(dest)
		if err != nil {
			fmt.Fprintf(errOut, "drift: %s missing or unreadable: %v\n", dest, err)
			drift = true
			continue
		}
		if !bytesEqual(existing, f.Data) {
			fmt.Fprintf(errOut, "drift: %s is out of date (re-run `sin-code compile-spec`)\n", dest)
			drift = true
		}
	}
	if drift {
		os.Exit(1)
	}
	fmt.Fprintln(out, "all derived files are in sync")
	return nil
}

// compilerEmitAll wraps the package-private emitAll. We need a
// public entry point or this duplicate, so we duplicate the
// four-emitter orchestration here. (The internal emitAll is
// package-private to keep the API surface small.)
func compilerEmitAll(c *compiler.Config) ([]compilerOutputFile, error) {
	hooks, err := compiler.EmitHooks(c)
	if err != nil {
		return nil, err
	}
	verify, err := compiler.EmitVerify(c)
	if err != nil {
		return nil, err
	}
	perms, err := compiler.EmitPermissions(c)
	if err != nil {
		return nil, err
	}
	loop, err := compiler.EmitLoop(c)
	if err != nil {
		return nil, err
	}
	return []compilerOutputFile{
		{Path: ".sin/hooks.json", Data: hooks},
		{Path: "internal/verify/config.json", Data: verify},
		{Path: "internal/permission/policies.json", Data: perms},
		{Path: ".sin/loop.json", Data: loop},
	}, nil
}

// compilerOutputFile is a public mirror of compiler's private
// type. Keeping it small + duplicative avoids widening the
// package's public API for one CLI.
type compilerOutputFile struct {
	Path string
	Data []byte
}

// bytesEqual is a small helper to avoid importing bytes in this
// file (keeps the import list short). It is correct because the
// emitted JSON is canonical (json.MarshalIndent + a stable
// Config means the same input always produces the same bytes).
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
