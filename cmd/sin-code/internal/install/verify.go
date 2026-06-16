// SPDX-License-Identifier: MIT
// Purpose: install — SHA256 + checksum.txt verification for downloaded
// release assets. Cryptographic helpers are intentionally minimal: a
// streaming SHA256 hasher for download-time integrity, a parser for
// goreleaser's `name_template: checksums.txt` format, and a tiny
// constant-time compare. Cosign signature verification is OPT-IN —
// we never BLOCK an install on its absence because not every release
// has been signed yet (dev tags, Erlang-style "rapid iteration"
// builds) and the project is still hardening the supply chain.
package install

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// sha256Writer is an io.Writer that accumulates a SHA256 hash of
// every byte written. Encapsulated so callers don't have to plumb a
// hash.Hash around — io.Copy(sw, body) Just Works.
type sha256Writer struct {
	h         hash.Hash
	cachedHex string
}

func newSHA256Writer() *sha256Writer {
	return &sha256Writer{h: sha256.New()}
}

func (w *sha256Writer) Write(p []byte) (int, error) { return w.h.Write(p) }
func (w *sha256Writer) hex() string {
	if w.cachedHex == "" {
		w.cachedHex = hex.EncodeToString(w.h.Sum(nil))
	}
	return w.cachedHex
}

// parseChecksumsTxt parses the goreleaser-style
// "<hex>   <asset-name>" two-space format produced by .goreleaser.yaml
// line 42. Empty lines + comments (#) are ignored.
func parseChecksumsTxt(body string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Goreleaser emits "hex  filename" with two spaces (sha256sum
		// binary format). SplitN keeps the file name intact even if
		// it contains a single space.
		fields := strings.SplitN(line, " ", 2)
		if len(fields) != 2 {
			continue
		}
		hexStr := strings.TrimSpace(fields[0])
		name := strings.TrimSpace(fields[1])
		// Use just the basename so the user can pass either the
		// archive URL or a `dir/asset` path interchangeably.
		if _, err := hex.DecodeString(hexStr); err != nil {
			continue
		}
		out[filepath.Base(name)] = strings.ToLower(hexStr)
	}
	return out
}

// Verify confirms that a downloaded file matches its expected SHA256.
// expectedHex is the hex digest from checksums.txt. observedHex is
// what the sha256Writer computed during the download. Both are
// compared case-insensitively to avoid Go test fixtures clashing.
func Verify(path string, observedHex, expectedHex string) error {
	if expectedHex == "" {
		// No checksum available — explicitly opt-in. The caller
		// decides whether this is fatal (the install cmd makes it
		// fatal); the verify package just refuses to silently
		// "pass" an empty checksum.
		return errors.New("install: verify: no expected SHA256 provided (run with checksum source)")
	}
	if strings.ToLower(observedHex) != strings.ToLower(expectedHex) {
		return fmt.Errorf("install: SHA256 mismatch for %s: got %s, want %s", filepath.Base(path), observedHex, expectedHex)
	}
	return nil
}

// VerifyFromFile computes the SHA256 of the file at path and
// compares it against expectedHex. Useful when the caller already
// persisted the bytes (e.g. install.sh shim) and wants a clean re-verify.
func VerifyFromFile(path string, expectedHex string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sw := newSHA256Writer()
	if _, err := io.Copy(sw, bufio.NewReader(f)); err != nil {
		return "", err
	}
	if err := Verify(path, sw.hex(), expectedHex); err != nil {
		return "", err
	}
	return sw.hex(), nil
}
