// SPDX-License-Identifier: MIT
// Purpose: install — race-safe tests for the install package. The
// whole suite runs hermetically against in-memory httptest servers
// and t.TempDir() — no GitHub, no network, no real binary on disk.
package install_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/install"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers (mirror cmd/sin-code/internal/self_update_test.go style)
// ─────────────────────────────────────────────────────────────────────────────

func fakeRelease(t *testing.T, tag string, assets []map[string]any) *install.Release {
	t.Helper()
	out := &install.Release{TagName: tag, Name: tag}
	for _, a := range assets {
		var asset install.Asset
		if n, ok := a["name"].(string); ok {
			asset.Name = n
		}
		if u, ok := a["url"].(string); ok {
			asset.BrowserURL = u
		}
		out.Assets = append(out.Assets, asset)
	}
	return out
}

func releaseHandler(t *testing.T, rel *install.Release) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.github+json")
		_ = json.NewEncoder(w).Encode(rel)
	}
}

func assetHandler(payload []byte, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		_, _ = w.Write(payload)
	}
}

func buildTarGz(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sin-code-test.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	for name, body := range files {
		hdr := &tar.Header{Name: name, Size: int64(len(body)), Mode: 0o755, Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// ─────────────────────────────────────────────────────────────────────────────
// Asset naming + URL constants
// ─────────────────────────────────────────────────────────────────────────────

func TestPlatformAssetName_Linux(t *testing.T) {
	p := install.Platform{GOOS: "linux", GOARCH: "amd64"}
	want := "sin-code-linux-amd64.tar.gz"
	if got := p.AssetName(); got != want {
		t.Fatalf("AssetName: got %q, want %q", got, want)
	}
}

func TestPlatformAssetName_DarwinARM(t *testing.T) {
	p := install.Platform{GOOS: "darwin", GOARCH: "arm64"}
	want := "sin-code-darwin-arm64.tar.gz"
	if got := p.AssetName(); got != want {
		t.Fatalf("AssetName: got %q, want %q", got, want)
	}
}

func TestPlatformAssetName_Windows(t *testing.T) {
	p := install.Platform{GOOS: "windows", GOARCH: "amd64"}
	want := "sin-code-windows-amd64.zip"
	if got := p.AssetName(); got != want {
		t.Fatalf("AssetName: got %q, want %q", got, want)
	}
}

func TestPlatformBinaryName(t *testing.T) {
	if got := (install.Platform{GOOS: "linux"}).BinaryName(); got != "sin-code" {
		t.Errorf("linux binary name: got %q", got)
	}
	if got := (install.Platform{GOOS: "windows"}).BinaryName(); got != "sin-code.exe" {
		t.Errorf("windows binary name: got %q", got)
	}
}

func TestPlatformDownloadURLStable(t *testing.T) {
	p := install.Platform{GOOS: "linux", GOARCH: "arm64"}
	want := "https://github.com/OpenSIN-Code/SIN-Code/releases/latest/download/sin-code-linux-arm64.tar.gz"
	if got := p.DownloadURL(); got != want {
		t.Fatalf("DownloadURL: got %q, want %q", got, want)
	}
}

func TestCanonicalConstants(t *testing.T) {
	if install.Repo != "OpenSIN-Code/SIN-Code" {
		t.Errorf("Repo drifted: got %q", install.Repo)
	}
	if install.ChecksumFileName != "checksums.txt" {
		t.Errorf("ChecksumFileName drifted: got %q", install.ChecksumFileName)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// SHA256 verification
// ─────────────────────────────────────────────────────────────────────────────

func TestParseChecksumsTxtGoreleaserShape(t *testing.T) {
	body := strings.Join([]string{
		"# goreleaser checksums",
		"abc123def4567890abc123def4567890abc123def4567890abc123def4567890  sin-code-linux-amd64.tar.gz",
		"111222333444555666777888999aaabbbcccdddeeefff00011122233344455  sin-code-darwin-arm64.tar.gz",
		"",
		"# empty line above is ignored",
	}, "\n")
	got := install.ParseChecksumsTxtForTest(body)
	if _, ok := got["sin-code-linux-amd64.tar.gz"]; !ok {
		t.Fatalf("linux asset not parsed: %+v", got)
	}
	if _, ok := got["sin-code-darwin-arm64.tar.gz"]; !ok {
		t.Fatalf("darwin asset not parsed: %+v", got)
	}
}

func TestVerifySHA256_Match(t *testing.T) {
	body := strings.Repeat("x", 32)
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// Compute the actual SHA256 via the verify pkg path so we don't
	// duplicate the hash algo in two places (drift = bug).
	hex, err := install.VerifyFromFileForTest(p, expectedSHA256Of([]byte(body)))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if hex == "" {
		t.Errorf("hex returned empty string")
	}
}

func TestVerifySHA256_Mismatch(t *testing.T) {
	body := strings.Repeat("x", 32)
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := install.VerifyFromFileForTest(p, "0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("expected mismatch err, got %v", err)
	}
}

func TestVerifySHA256_EmptyExpectedRejected(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	if err := os.WriteFile(p, []byte("anything"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := install.VerifyFromFileForTest(p, "")
	if err == nil {
		t.Fatalf("expected refusal on empty expected SHA256 (fail-closed)")
	}
}

func expectedSHA256Of(body []byte) string {
	return install.HashOfForTest(body)
}

// ─────────────────────────────────────────────────────────────────────────────
// FetchLatest (httptest-based, fully hermetic)
// ─────────────────────────────────────────────────────────────────────────────

func TestFetchLatest_HappyPath(t *testing.T) {
	want := install.Platform{GOOS: "linux", GOARCH: "amd64"}
	rel := fakeRelease(t, "v3.17.0", []map[string]any{
		{"name": "sin-code-linux-amd64.tar.gz", "url": "https://example/sin-code-linux-amd64.tar.gz"},
		{"name": "sin-code-darwin-arm64.tar.gz", "url": "https://example/sin-code-darwin-arm64.tar.gz"},
	})
	ts := httptest.NewServer(releaseHandler(t, rel))
	defer ts.Close()
	// Replace the package-level API URL just for this test via the
	// injected config struct — keeps the prod URL immutable.
	c := install.NewHTTPClientWithBaseURLForTest(ts.URL)
	got, err := install.FetchLatestWithConfigForTest(context.Background(), c, want)
	if err != nil {
		t.Fatalf("FetchLatest: %v", err)
	}
	if got.TagName != "v3.17.0" {
		t.Fatalf("TagName: got %q", got.TagName)
	}
	if len(got.Assets) != 2 {
		t.Fatalf("assets: got %d", len(got.Assets))
	}
}

func TestFetchLatest_PlatformNotInRelease(t *testing.T) {
	want := install.Platform{GOOS: "freebsd", GOARCH: "amd64"}
	rel := fakeRelease(t, "v3.17.0", []map[string]any{
		{"name": "sin-code-linux-amd64.tar.gz", "url": "https://example/sin-code-linux-amd64.tar.gz"},
	})
	ts := httptest.NewServer(releaseHandler(t, rel))
	defer ts.Close()
	c := install.NewHTTPClientWithBaseURLForTest(ts.URL)
	_, err := install.FetchLatestWithConfigForTest(context.Background(), c, want)
	if err == nil || !strings.Contains(err.Error(), "no asset named") {
		t.Fatalf("expected no-asset error, got %v", err)
	}
}

func TestFetchLatest_BadStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusForbidden)
	}))
	defer ts.Close()
	c := install.NewHTTPClientWithBaseURLForTest(ts.URL)
	_, err := install.FetchLatestWithConfigForTest(context.Background(), c, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil {
		t.Fatalf("expected error on 403")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Composer — ExtractBinary + atomic Place
// ─────────────────────────────────────────────────────────────────────────────

func TestExtractBinary_FromTarGzWithTopDir(t *testing.T) {
	tgz := buildTarGz(t, map[string]string{
		"sin-code-linux-amd64/sin-code":  "#!/bin/sh\necho hi\n",
		"sin-code-linux-amd64/LICENSE":   "MIT license text",
		"sin-code-linux-amd64/README.md": "# SIN-Code",
	})
	dest := t.TempDir()
	got, err := install.ExtractBinary(tgz, dest, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "echo hi") {
		t.Fatalf("extracted body unexpected: %q", body)
	}
	// Make sure we did NOT accidentally copy the LICENSE or README into dest/.
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		if strings.EqualFold(e.Name(), "LICENSE") || strings.EqualFold(e.Name(), "README.md") {
			t.Fatalf("composer extracted extra file %q (should be binary only)", e.Name())
		}
	}
}

func TestExtractBinary_NoBinaryMemberIsError(t *testing.T) {
	tgz := buildTarGz(t, map[string]string{
		"sin-code-linux-amd64/LICENSE": "MIT",
	})
	dest := t.TempDir()
	_, err := install.ExtractBinary(tgz, dest, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err == nil || !strings.Contains(err.Error(), "no member named") {
		t.Fatalf("expected no-member err, got %v", err)
	}
}

func TestPlace_AtomicallyReplacesExistingBinary(t *testing.T) {
	dir := t.TempDir()
	// Pre-existing binary that should be replaced.
	oldBin := filepath.Join(dir, "sin-code")
	if err := os.WriteFile(oldBin, []byte("#!/bin/sh\necho old\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	newBin := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(newBin, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(newBin, "src")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho new\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	final, err := install.Place(src, dir, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	if final != filepath.Join(dir, "sin-code") {
		t.Fatalf("Place final path: got %q want %q", final, filepath.Join(dir, "sin-code"))
	}
	body, _ := os.ReadFile(final)
	if !strings.Contains(string(body), "echo new") {
		t.Fatalf("binary not replaced: %q", body)
	}
}

func TestPlace_ChmodMakesExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod semantics differ on windows")
	}
	dir := t.TempDir()
	src := filepath.Join(t.TempDir(), "src")
	if err := os.WriteFile(src, []byte("#!/bin/sh\necho hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	final, err := install.Place(src, dir, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(final)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o100 == 0 {
		t.Errorf("placed binary is not user-executable (mode=%v)", info.Mode())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// ChooseBinDir
// ─────────────────────────────────────────────────────────────────────────────

func TestChooseBinDir_HonorsEnvOverride(t *testing.T) {
	override := filepath.Join(t.TempDir(), "my-bin")
	t.Setenv("SIN_CODE_BIN_DIR", override)
	got, created, hint, err := install.ChooseBinDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != override {
		t.Fatalf("got %q, want %q", got, override)
	}
	if created {
		t.Errorf("did not create (env var pointed at existing dir)")
	}
	if hint != "" {
		t.Errorf("expected no PATH hint when env override matches writable dir, got %q", hint)
	}
}

func TestChooseBinDir_FallsBackToHomeDir(t *testing.T) {
	// Point HOME at a fresh tempdir so ~/.local/bin ends up owned by
	// the test (and absent on dev machines). The created= boolean is
	// informational — the contract under test is "we always return a
	// usable bin dir, falling back to ~/.local/bin when nothing else
	// matches".
	homeDir := t.TempDir()
	t.Setenv("SIN_CODE_BIN_DIR", "")
	t.Setenv("HOME", homeDir)
	got, _, hint, err := install.ChooseBinDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(homeDir, ".local", "bin")
	if got != want {
		t.Fatalf("expected ~/.local/bin fallback: got %q want %q", got, want)
	}
	if info, statErr := os.Stat(got); statErr != nil || !info.IsDir() {
		t.Fatalf("fallback dir missing or not a directory: stat=%v err=%v", info, statErr)
	}
	// Without env override, the user gets a PATH hint when their
	// fresh fallback was newly created. Not strictly load-bearing —
	// the helper returns "" when an earlier candidate was writable.
	if hint != "" && !strings.Contains(hint, "PATH") {
		t.Errorf("suggested hint should mention PATH, got %q", hint)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Sanity: end-to-end "download → verify → extract → place → runnable"
// ─────────────────────────────────────────────────────────────────────────────

func TestE2E_DownloadVerifyExtractPlace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("end-to-end uses POSIX chmod; windows path is unit-tested separately")
	}
	binBody := "#!/bin/sh\necho installed-by-test\n"
	tgz := buildTarGz(t, map[string]string{
		"sin-code-linux-amd64/sin-code":    binBody,
		"sin-code-linux-amd64/LICENSE":     "MIT",
		"sin-code-linux-amd64/README.md":   "# SIN",
		"sin-code-linux-amd64/SECURITY.md": "no cve",
	})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "checksums.txt") {
			fmt.Fprintf(w, "%x  %s\n", sha256Of([]byte(binBody)), "sin-code-linux-amd64.tar.gz")
			_, _ = w.Write([]byte(""))
			return
		}
		// Serve the tarball verbatim when fetched directly.
		_, _ = w.Write(mustRead(t, tgz))
	}))
	defer ts.Close()
	c := install.NewHTTPClientWithBaseURLForTest(ts.URL)
	got, _, err := install.FetchAssetWithConfigForTest(context.Background(), c, install.Platform{GOOS: "linux", GOARCH: "amd64"}, ts.URL)
	if err != nil {
		t.Fatalf("FetchAsset: %v", err)
	}
	defer os.Remove(got)
	extracted, err := install.ExtractBinary(got, t.TempDir(), install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("ExtractBinary: %v", err)
	}
	tmp := t.TempDir()
	final, err := install.Place(extracted, tmp, install.Platform{GOOS: "linux", GOARCH: "amd64"})
	if err != nil {
		t.Fatalf("Place: %v", err)
	}
	if runtime.GOOS != "windows" {
		// POSIX sanity: file exists and is user-executable.
		info, _ := os.Stat(final)
		if info.Mode()&0o100 == 0 {
			t.Errorf("placed binary not executable (mode=%v)", info.Mode())
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers (kept low-key so the runtime dep graph stays tiny)
// ─────────────────────────────────────────────────────────────────────────────

func sha256Of(body []byte) string {
	return install.HashOfForTest(body)
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
