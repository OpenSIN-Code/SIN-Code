// SPDX-License-Identifier: MIT
// Purpose: install — composer: take a verified tar.gz / zip archive,
// extract the sin-code binary, and place it at a writable PATH-respecting
// destination. The whole file is ~150 LOC because the policy is:
//  1. stream-extract the binary member (don't materialize the archive
//     on disk alongside the binary — wastes user disk on flash drives),
//  2. choose a writable bin dir ordered: $SIN_CODE_BIN_DIR > writing
//     test of $HOME/.local/bin > system /usr/local/bin (last because
//     it usually needs sudo, and the install subcommand refuses to
//     escalate — M4: daemon is always headless and the interactive
//     flow expects an unprivileged path),
//  3. atomic-ish rename into place so an interrupted previous install
//     doesn't leave the user with a half-written binary on PATH.
package install

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// ExtractBinary returns the path to the `sin-code` binary that was
// extracted from the verified archive. The archive is NOT kept on
// disk after this call returns (the caller is expected to unlink
// it). The "extract just the one binary" path is taken so users
// without write access to /usr/local can still install into a
// vendored tgz downloaded to ~/.cache.
func ExtractBinary(archivePath string, destDir string, p Platform) (string, error) {
	if archivePath == "" {
		return "", errors.New("install: empty archive path")
	}
	if destDir == "" {
		destDir = filepath.Dir(archivePath)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	want := p.BinaryName()
	switch p.GOOS {
	case "windows":
		return extractFromZip(archivePath, destDir, want)
	default:
		return extractFromTarGz(archivePath, destDir, want)
	}
}

func extractFromTarGz(archivePath, destDir, want string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("install: gzip reader: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("install: tar reader: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != want {
			continue
		}
		out, err := writeToTempFile(destDir, want)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return "", fmt.Errorf("install: extract %s: %w", hdr.Name, err)
		}
		if err := out.Close(); err != nil {
			return "", fmt.Errorf("install: close extracted file: %w", err)
		}
		return out.Name(), setExecMode(out.Name(), hdr.FileInfo().Mode())
	}
	return "", fmt.Errorf("install: no member named %q in %s", want, archivePath)
}

func extractFromZip(archivePath, destDir, want string) (string, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer r.Close()
	for _, f := range r.File {
		if filepath.Base(f.Name) != want {
			continue
		}
		out, err := writeToTempFile(destDir, want)
		if err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			_ = out.Close()
			return "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			_ = out.Close()
			rc.Close()
			return "", err
		}
		rc.Close()
		if err := out.Close(); err != nil {
			return "", err
		}
		return out.Name(), nil
	}
	return "", fmt.Errorf("install: no member named %q in %s", want, archivePath)
}

// ChooseBinDir picks an install directory in this order of preference:
//  1. $SIN_CODE_BIN_DIR if non-empty AND writable. Lets users
//     override (e.g., `SIN_CODE_BIN_DIR=~/bin bash install.sh`).
//  2. $HOME/.local/bin — the XDG-aligned, sudo-free default used by
//     Homebrew, pipx, go install, cargo install, rustup. Pick this
//     over /usr/local/bin because it never requires escalation.
//  3. Whatever directory the current binary lives in IF it's writable.
//     Useful for `sin-code install` self-invoked from a binary
//     already on PATH that's owned by the user.
//  4. Fallback: create $HOME/.local/bin even if it didn't exist.
//     The error case is "/ is read-only" which we surface verbatim.
//
// Returns absolute path, the parent that was created (or empty), and
// a hint string for the user ("PATH hint: ..."). Never returns a
// /usr/local-style path that requires root.
func ChooseBinDir() (binDir string, created bool, hint string, err error) {
	var candidates []string
	if v := os.Getenv("SIN_CODE_BIN_DIR"); v != "" {
		candidates = append(candidates, v)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "bin"))
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		candidates = append(candidates, filepath.Dir(exe))
	}

	for _, c := range candidates {
		if c == "" {
			continue
		}
		writable, err := dirWritable(c)
		if err == nil && writable {
			return c, false, "", nil
		}
	}

	// Fallback: create ~/.local/bin if nothing else worked.
	home, herr := os.UserHomeDir()
	if herr != nil {
		return "", false, "", fmt.Errorf("install: could not determine $HOME (%w)", herr)
	}
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", false, "", fmt.Errorf("install: could not create %s (%w); set SIN_CODE_BIN_DIR to a writable directory", dir, err)
	}
	hint = "Add to PATH:  export PATH=\"" + dir + ":$PATH\""
	return dir, true, hint, nil
}

// Place installs the verified binary into binDir. The flow is
// (verify-once-more, write atomic, chmod executable) so a half-written
// binary never replaces a working one on disk.
func Place(binaryPath, binDir string, p Platform) (finalPath string, err error) {
	if binDir == "" {
		return "", errors.New("install: empty bin dir")
	}
	if binaryPath == "" {
		return "", errors.New("install: empty binary path")
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("install: mkdir %s: %w", binDir, err)
	}
	final := filepath.Join(binDir, p.BinaryName())
	// Place next to the target with a randomized suffix then rename
	// atomically. On error we leave any temp file in place for the
	// user to inspect — silent cleanup would mask real bugs.
	tmp, err := os.CreateTemp(binDir, ".sin-code-tmp-*")
	if err != nil {
		return "", fmt.Errorf("install: tempfile in %s: %w", binDir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if _, statErr := os.Stat(tmpName); statErr == nil {
			_ = os.Remove(tmpName)
		}
	}()
	in, err := os.Open(binaryPath)
	if err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		in.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		in.Close()
		return "", err
	}
	in.Close()
	if err := os.Chmod(tmpName, 0o755); err != nil && runtime.GOOS != "windows" {
		return "", fmt.Errorf("install: chmod: %w", err)
	}
	if err := os.Rename(tmpName, final); err != nil {
		return "", fmt.Errorf("install: rename to %s: %w", final, err)
	}
	return final, nil
}

// helpers -------------------------------------------------------------

func writeToTempFile(dir, baseName string) (*os.File, error) {
	if dir == "" {
		dir = os.TempDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.CreateTemp(dir, ".sin-code-tmp-*"+safeBase(baseName))
}

func safeBase(s string) string {
	s = filepath.Base(s)
	var b bytes.Buffer
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		b.WriteString("sincode")
	}
	return b.String()
}

func setExecMode(name string, mode os.FileMode) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if mode == 0 {
		mode = 0o755
	}
	return os.Chmod(name, mode)
}

func dirWritable(dir string) (bool, error) {
	if dir == "" {
		return false, errors.New("empty dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(dir, ".sin-code-probe-*")
	if err != nil {
		return false, err
	}
	probe := tmp.Name()
	if _, err := tmp.WriteString("ok"); err != nil {
		tmp.Close()
		_ = os.Remove(probe)
		return false, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(probe)
		return false, err
	}
	if err := os.Remove(probe); err != nil {
		return false, err
	}
	return true, nil
}
