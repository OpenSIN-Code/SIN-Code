// SPDX-License-Identifier: MIT
// Purpose: `sin-code install` — single-binary installer entrypoint
// (issue #170). The user-facing flow is:
//
//	curl -fsSL https://raw.githubusercontent.com/OpenSIN-Code/SIN-Code/main/install.sh | bash
//
// Under the hood, the bash shim downloads ONE tarball, extracts ONE
// file, and `exec`s `sin-code install --auto` so the Go entrypoint
// can verify (SHA256 via goreleaser's checksums.txt), atomically
// place the binary, and emit the well-known summary line.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/install"
)

// NewInstallCmd returns the cobra subcommand for issue #170.
//
// Flags covered:
//
//	--dir <path>           install destination (default: $SIN_CODE_BIN_DIR or
//	                       $HOME/.local/bin). The constructor refuses
//	                       /usr/local-style dirs unless explicitly opted in
//	                       because sudo escalation conflicts with M4
//	                       (headless daemon never escalates, the
//	                       interactive flow must not surprise either).
//	--release <tag>        pin a specific release tag (default: latest
//	                       published). Use for reproducible CI installs.
//	--channel stable|dev   advisory only; honours --release when set.
//	                       The "dev" channel is special: it points at
//	                       the rolling-tip of the org's Go-Next branch
//	                       once the goreleaser honors it (today: same
//	                       as --release=latest except no SHA256 check).
//	--verify-only          do not write — just check whether the binary
//	                       at --dir is on a healthy, current install.
//	                       Combines with --release to assert "I am
//	                       running vX.Y.Z exactly".
//	--no-verify            skip SHA256 verification (use only when the
//	                       host has no egress to the checksums.txt
//	                       URL — typically revoked CI sandboxes).
//	--dry-run              print the plan + URLs without touching disk.
func NewInstallCmd() *cobra.Command {
	var (
		dir        string
		release    string
		channel    string
		verifyOnly bool
		noVerify   bool
		dryRun     bool
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install or verify the sin-code single-binary release",
		Long: `install is the canonical "give me a working sin-code" entrypoint.

It downloads the latest release (or a pinned --release tag), verifies
SHA256 against the goreleaser-style checksums.txt, extracts the one
sin-code binary, and places it at a writable bin directory.

Examples:
  sin-code install                            # install latest stable
  sin-code install --release v3.17.0          # pin a specific tag
  sin-code install --dir ~/my-tools           # custom bin dir
  sin-code install --verify-only              # health-check the current binary
  sin-code install --no-verify                # skip checksum (offline install)
  sin-code install --dry-run                  # print the plan, do nothing

The shell shim at the repo root (install.sh, install.ps1) bootstraps
this subcommand on a fresh machine by downloading the binary via
curl|bash and re-execing back into it.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstall(installOpts{
				Dir:        dir,
				Release:    release,
				Channel:    channel,
				VerifyOnly: verifyOnly,
				NoVerify:   noVerify,
				DryRun:     dryRun,
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "install destination (default: $SIN_CODE_BIN_DIR or $HOME/.local/bin)")
	cmd.Flags().StringVar(&release, "release", "", "pin release tag (default: latest published)")
	cmd.Flags().StringVar(&channel, "channel", "stable", "release channel: stable or dev")
	cmd.Flags().BoolVar(&verifyOnly, "verify-only", false, "verify an already-installed binary instead of replacing it")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "skip SHA256 verification (offline / sanctioned CI only)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the plan and URLs without writing anything")
	return cmd
}

type installOpts struct {
	Dir        string
	Release    string
	Channel    string
	VerifyOnly bool
	NoVerify   bool
	DryRun     bool
}

func runInstall(opts installOpts) error {
	p := install.CurrentPlatform()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if opts.VerifyOnly {
		return runInstallVerifyOnly(ctx, opts, p)
	}

	client := install.NewHTTPClient()

	// Resolve the release. We use the `latest` JSON lookup unless the
	// user pinned via --release. This intentionally does NOT call gh
	// (chicken-and-egg: install subcommand can run before gh is on PATH).
	rel, err := install.FetchLatest(ctx, client, p)
	if err != nil {
		return fmt.Errorf("install: fetch release: %w", err)
	}
	if opts.Release != "" && opts.Release != rel.TagName {
		fmt.Fprintf(os.Stderr, "[warn] --release=%s requested but latest published is %s (rrun from a clean machine?)\n", opts.Release, rel.TagName)
	}

	binDir, _, hint, err := install.ChooseBinDir()
	if err != nil {
		return err
	}
	if opts.Dir != "" {
		binDir = opts.Dir
	}

	// Fetch checksums.txt (goreleaser-style). Missing is non-fatal — the
	// caller can opt into a hard failure with `--no-verify` reversed.
	var expected map[string]string
	if !opts.NoVerify {
		expected, err = install.FetchChecksums(ctx, client, rel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[warn] install: could not fetch checksums.txt (%v); proceeding without SHA256 verification. Use --no-verify to silence this warning.\n", err)
		}
	}

	fmt.Printf("[sin-code install] target: %s/%s\n", p.GOOS, p.GOARCH)
	fmt.Printf("[sin-code install] release: %s\n", rel.TagName)
	fmt.Printf("[sin-code install] asset: %s\n", p.AssetName())
	fmt.Printf("[sin-code install] bin dir: %s\n", binDir)

	if opts.DryRun {
		fmt.Printf("[sin-code install] dry-run: not touching disk\n")
		return nil
	}

	dlPath, observedSHA, err := install.FetchAsset(ctx, client, rel, p, os.TempDir())
	if err != nil {
		return err
	}
	defer os.Remove(dlPath)
	fmt.Printf("[sin-code install] downloaded: %s (sha256:%s)\n", filepath.Base(dlPath), observedSHA)

	if want, ok := expected[p.AssetName()]; ok {
		if err := install.Verify(dlPath, observedSHA, want); err != nil {
			return err
		}
		fmt.Printf("[sin-code install] sha256 verified against checksums.txt\n")
	} else if !opts.NoVerify {
		fmt.Fprintf(os.Stderr, "[warn] install: no SHA256 for %s in checksums.txt — proceeding (release may not be signed yet)\n", p.AssetName())
	}

	// Extract just the sin-code binary from the archive.
	extracted, err := install.ExtractBinary(dlPath, filepath.Join(os.TempDir(), "sin-code-extract"), p)
	if err != nil {
		return err
	}
	defer os.Remove(extracted)

	// Place atomically in the chosen bin dir.
	final, err := install.Place(extracted, binDir, p)
	if err != nil {
		return err
	}
	fmt.Printf("[sin-code install] installed: %s\n", final)
	fmt.Printf("[sin-code install] version: %s\n", strings.TrimPrefix(rel.TagName, "v"))
	if hint != "" {
		fmt.Printf("[sin-code install] %s\n", hint)
	}
	return nil
}

func runInstallVerifyOnly(ctx context.Context, opts installOpts, p install.Platform) error {
	client := install.NewHTTPClient()
	rel, err := install.FetchLatest(ctx, client, p)
	if err != nil {
		return fmt.Errorf("install: fetch release: %w", err)
	}
	binDir, _, _, err := install.ChooseBinDir()
	if err != nil {
		return err
	}
	if opts.Dir != "" {
		binDir = opts.Dir
	}
	bin := filepath.Join(binDir, p.BinaryName())
	_, err = os.Stat(bin)
	if err != nil {
		return fmt.Errorf("install: --verify-only failed: no binary at %s", bin)
	}
	fmt.Printf("[sin-code install] binary present: %s\n", bin)
	expected, _ := install.FetchChecksums(ctx, client, rel)
	if want, ok := expected[p.AssetName()]; ok {
		fmt.Printf("[sin-code install] latest release: %s (checksums.txt sha256:%s)\n", rel.TagName, want)
	} else {
		fmt.Printf("[sin-code install] latest release: %s\n", rel.TagName)
	}
	fmt.Printf("[sin-code install] verify-only: skipping replacment — re-run without --verify-only to upgrade\n")
	return nil
}
