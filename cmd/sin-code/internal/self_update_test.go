// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the self-update subcommand.
package internal

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func createTarGz(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()
	for name, content := range files {
		hdr := &tar.Header{Name: name, Size: int64(len(content)), Mode: 0755, Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func createZip(t *testing.T, files map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func githubReleaseHandler(tag, published string, assets []map[string]interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		release := map[string]interface{}{
			"tag_name":     tag,
			"name":         tag,
			"published_at": published,
			"body":         "release notes",
			"assets":       assets,
		}
		json.NewEncoder(w).Encode(release)
	}
}

func resetSelfUpdateFlags() {
	SelfUpdateCmd.Flags().Set("dry-run", "false")
	SelfUpdateCmd.Flags().Set("version", "false")
}

func TestSetCurrentVersion(t *testing.T) {
	SetCurrentVersion("v1.0.0")
	if currentVersion != "v1.0.0" {
		t.Errorf("expected v1.0.0, got %s", currentVersion)
	}
	SetCurrentVersion("v2.3.4")
	if currentVersion != "v2.3.4" {
		t.Errorf("expected v2.3.4, got %s", currentVersion)
	}
	SetCurrentVersion("dev")
}

func TestFormatDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2024-01-15T10:30:00Z", "2024-01-15"},
		{"2023-12-31T23:59:59Z", "2023-12-31"},
		{"2024-06-07T00:00:00+02:00", "2024-06-07"},
		{"not-a-date", "not-a-date"},
		{"", ""},
		{"2024-02-29T12:00:00Z", "2024-02-29"},
	}
	for _, tt := range tests {
		got := formatDate(tt.input)
		if got != tt.want {
			t.Errorf("formatDate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractTarGz(t *testing.T) {
	files := map[string]string{
		"sin-code": "binary-content-here",
	}
	archivePath := createTarGz(t, files)
	destDir := t.TempDir()

	extractedPath, err := extractTarGz(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractTarGz failed: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(extractedPath), "sin-code") {
		t.Errorf("extracted path base should start with sin-code, got %s", extractedPath)
	}
	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("read extracted file failed: %v", err)
	}
	if string(data) != "binary-content-here" {
		t.Errorf("extracted content = %q, want %q", string(data), "binary-content-here")
	}
}

func TestExtractTarGz_NoBinary(t *testing.T) {
	files := map[string]string{
		"readme.md": "readme content",
		"other.txt": "other content",
	}
	archivePath := createTarGz(t, files)
	destDir := t.TempDir()

	_, err := extractTarGz(archivePath, destDir)
	if err == nil {
		t.Error("expected error when no sin-code binary in archive")
	}
	if !strings.Contains(err.Error(), "no sin-code binary found") {
		t.Errorf("error = %q, want 'no sin-code binary found'", err.Error())
	}
}

func TestExtractTarGz_InvalidPath(t *testing.T) {
	_, err := extractTarGz("/nonexistent/path.tar.gz", t.TempDir())
	if err == nil {
		t.Error("expected error for nonexistent archive")
	}
}

func TestExtractZip(t *testing.T) {
	files := map[string]string{
		"sin-code.exe": "binary-content-windows",
	}
	archivePath := createZip(t, files)
	destDir := t.TempDir()

	extractedPath, err := extractZip(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractZip failed: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(extractedPath), "sin-code") {
		t.Errorf("extracted path base should start with sin-code, got %s", extractedPath)
	}
	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("read extracted file failed: %v", err)
	}
	if string(data) != "binary-content-windows" {
		t.Errorf("extracted content = %q, want %q", string(data), "binary-content-windows")
	}
}

func TestExtractZip_NoBinary(t *testing.T) {
	files := map[string]string{
		"readme.md": "readme content",
	}
	archivePath := createZip(t, files)
	destDir := t.TempDir()

	_, err := extractZip(archivePath, destDir)
	if err == nil {
		t.Error("expected error when no sin-code binary in archive")
	}
	if !strings.Contains(err.Error(), "no sin-code binary found") {
		t.Errorf("error = %q, want 'no sin-code binary found'", err.Error())
	}
}

func TestExtractZip_InvalidPath(t *testing.T) {
	_, err := extractZip("/nonexistent/path.zip", t.TempDir())
	if err == nil {
		t.Error("expected error for nonexistent archive")
	}
}

func TestExtractBinary_TarGz(t *testing.T) {
	files := map[string]string{
		"sin-code": "tar-gz-binary",
	}
	archivePath := createTarGz(t, files)
	destDir := t.TempDir()

	extractedPath, err := extractBinary(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractBinary failed for tar.gz: %v", err)
	}
	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(data) != "tar-gz-binary" {
		t.Errorf("content = %q, want %q", string(data), "tar-gz-binary")
	}
}

func TestExtractBinary_Zip(t *testing.T) {
	files := map[string]string{
		"sin-code.exe": "zip-binary",
	}
	archivePath := createZip(t, files)
	destDir := t.TempDir()

	extractedPath, err := extractBinary(archivePath, destDir)
	if err != nil {
		t.Fatalf("extractBinary failed for zip: %v", err)
	}
	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(data) != "zip-binary" {
		t.Errorf("content = %q, want %q", string(data), "zip-binary")
	}
}

func TestDownloadFile(t *testing.T) {
	content := "downloaded-file-content"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(content))
	}))
	defer server.Close()

	destPath := filepath.Join(t.TempDir(), "downloaded.bin")
	if err := downloadFile(server.URL, destPath); err != nil {
		t.Fatalf("downloadFile failed: %v", err)
	}
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read downloaded file failed: %v", err)
	}
	if string(data) != content {
		t.Errorf("downloaded content = %q, want %q", string(data), content)
	}
}

func TestDownloadFile_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	destPath := filepath.Join(t.TempDir(), "downloaded.bin")
	err := downloadFile(server.URL, destPath)
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error = %q, want '404'", err.Error())
	}
}

func TestDownloadFile_InvalidURL(t *testing.T) {
	destPath := filepath.Join(t.TempDir(), "downloaded.bin")
	err := downloadFile("http://127.0.0.1:1/nonexistent", destPath)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestDownloadFile_InvalidPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer server.Close()

	// Parent "directory" is a regular file: fails with ENOTDIR on every
	// platform, even in sandboxes/CI where "/" happens to be writable.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if werr := os.WriteFile(blocker, []byte("x"), 0644); werr != nil {
		t.Fatal(werr)
	}
	err := downloadFile(server.URL, filepath.Join(blocker, "dir", "file.bin"))
	if err == nil {
		t.Error("expected error for invalid destination path")
	}
}

func TestFetchLatestRelease(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()

	server := httptest.NewServer(githubReleaseHandler("v1.0.8", "2024-06-01T12:00:00Z", nil))
	defer server.Close()
	githubAPIURL = server.URL

	release, err := fetchLatestRelease()
	if err != nil {
		t.Fatalf("fetchLatestRelease failed: %v", err)
	}
	if release.TagName != "v1.0.8" {
		t.Errorf("tag_name = %q, want %q", release.TagName, "v1.0.8")
	}
	if release.Published != "2024-06-01T12:00:00Z" {
		t.Errorf("published_at = %q, want %q", release.Published, "2024-06-01T12:00:00Z")
	}
}

func TestFetchLatestRelease_HTTPError(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer server.Close()
	githubAPIURL = server.URL

	_, err := fetchLatestRelease()
	if err == nil {
		t.Error("expected error for HTTP 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want '403'", err.Error())
	}
}

func TestFetchLatestRelease_InvalidJSON(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not-json"))
	}))
	defer server.Close()
	githubAPIURL = server.URL

	_, err := fetchLatestRelease()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestFetchLatestRelease_ConnectionError(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	githubAPIURL = "http://127.0.0.1:1/fake"

	_, err := fetchLatestRelease()
	if err == nil {
		t.Error("expected error for connection failure")
	}
}

func TestFetchLatestRelease_WithAssets(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()

	assets := []map[string]interface{}{
		{"name": "sin-code-darwin-arm64.tar.gz", "size": 12345, "browser_download_url": "https://example.com/darwin.tar.gz"},
		{"name": "sin-code-linux-amd64.tar.gz", "size": 12340, "browser_download_url": "https://example.com/linux.tar.gz"},
	}
	server := httptest.NewServer(githubReleaseHandler("v2.0.0", "2024-07-01T00:00:00Z", assets))
	defer server.Close()
	githubAPIURL = server.URL

	release, err := fetchLatestRelease()
	if err != nil {
		t.Fatalf("fetchLatestRelease failed: %v", err)
	}
	if len(release.Assets) != 2 {
		t.Fatalf("assets count = %d, want 2", len(release.Assets))
	}
	if release.Assets[0].Name != "sin-code-darwin-arm64.tar.gz" {
		t.Errorf("asset[0].Name = %q, want %q", release.Assets[0].Name, "sin-code-darwin-arm64.tar.gz")
	}
	if release.Assets[0].Size != 12345 {
		t.Errorf("asset[0].Size = %d, want 12345", release.Assets[0].Size)
	}
	if release.Assets[0].URL != "https://example.com/darwin.tar.gz" {
		t.Errorf("asset[0].URL = %q, want %q", release.Assets[0].URL, "https://example.com/darwin.tar.gz")
	}
}

func TestCheckUpdateAvailable_UpdateAvailable(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.0")
	defer SetCurrentVersion("dev")

	server := httptest.NewServer(githubReleaseHandler("v2.0.0", "2024-07-01T00:00:00Z", nil))
	defer server.Close()
	githubAPIURL = server.URL

	tag, available, err := CheckUpdateAvailable()
	if err != nil {
		t.Fatalf("CheckUpdateAvailable failed: %v", err)
	}
	if !available {
		t.Error("expected update available = true")
	}
	if tag != "v2.0.0" {
		t.Errorf("tag = %q, want %q", tag, "v2.0.0")
	}
}

func TestCheckUpdateAvailable_UpToDate(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.8")
	defer SetCurrentVersion("dev")

	server := httptest.NewServer(githubReleaseHandler("v1.0.8", "2024-06-01T12:00:00Z", nil))
	defer server.Close()
	githubAPIURL = server.URL

	tag, available, err := CheckUpdateAvailable()
	if err != nil {
		t.Fatalf("CheckUpdateAvailable failed: %v", err)
	}
	if available {
		t.Error("expected update available = false")
	}
	if tag != "v1.0.8" {
		t.Errorf("tag = %q, want %q", tag, "v1.0.8")
	}
}

func TestCheckUpdateAvailable_APIError(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	githubAPIURL = "http://127.0.0.1:1/fake"

	_, _, err := CheckUpdateAvailable()
	if err == nil {
		t.Error("expected error for API failure")
	}
}

func TestRunSelfUpdate_AlreadyLatest(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.8")
	defer SetCurrentVersion("dev")

	server := httptest.NewServer(githubReleaseHandler("v1.0.8", "2024-06-01T12:00:00Z", nil))
	defer server.Close()
	githubAPIURL = server.URL

	err := runSelfUpdate(false)
	if err != nil {
		t.Errorf("runSelfUpdate(false) failed: %v", err)
	}
}

func TestRunSelfUpdate_DryRun(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.0")
	defer SetCurrentVersion("dev")

	server := httptest.NewServer(githubReleaseHandler("v2.0.0", "2024-07-01T00:00:00Z", nil))
	defer server.Close()
	githubAPIURL = server.URL

	err := runSelfUpdate(true)
	if err != nil {
		t.Errorf("runSelfUpdate(dryRun=true) failed: %v", err)
	}
}

func TestRunSelfUpdate_NoAssetForPlatform(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.0")
	defer SetCurrentVersion("dev")

	assets := []map[string]interface{}{
		{"name": "sin-code-other-platform.tar.gz", "size": 100, "browser_download_url": "https://example.com/other.tar.gz"},
	}
	server := httptest.NewServer(githubReleaseHandler("v2.0.0", "2024-07-01T00:00:00Z", assets))
	defer server.Close()
	githubAPIURL = server.URL

	err := runSelfUpdate(false)
	if err == nil {
		t.Error("expected error when no matching asset for platform")
	}
	if !strings.Contains(err.Error(), "no binary found") {
		t.Errorf("error = %q, want 'no binary found'", err.Error())
	}
}

func TestRunSelfUpdate_APIError(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.0")
	defer SetCurrentVersion("dev")
	githubAPIURL = "http://127.0.0.1:1/fake"

	err := runSelfUpdate(false)
	if err == nil {
		t.Error("expected error for API failure")
	}
}

func TestPrintVersionInfo(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.8")
	defer SetCurrentVersion("dev")

	server := httptest.NewServer(githubReleaseHandler("v1.0.8", "2024-06-01T12:00:00Z", nil))
	defer server.Close()
	githubAPIURL = server.URL

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printVersionInfo()

	w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)

	if err != nil {
		t.Errorf("printVersionInfo failed: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "sin-code version: v1.0.8") {
		t.Errorf("output missing version line, got:\n%s", s)
	}
	if !strings.Contains(s, fmt.Sprintf("Platform: %s/%s", runtime.GOOS, runtime.GOARCH)) {
		t.Errorf("output missing platform line, got:\n%s", s)
	}
	if !strings.Contains(s, "Latest release: v1.0.8") {
		t.Errorf("output missing latest release line, got:\n%s", s)
	}
}

func TestPrintVersionInfo_APIError(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.0")
	defer SetCurrentVersion("dev")
	githubAPIURL = "http://127.0.0.1:1/fake"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printVersionInfo()

	w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)

	if err != nil {
		t.Errorf("printVersionInfo should not return error on API failure: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "sin-code version: v1.0.0") {
		t.Errorf("output missing version line, got:\n%s", s)
	}
	if !strings.Contains(s, "Could not check for updates") {
		t.Errorf("output missing update-check warning, got:\n%s", s)
	}
}

func TestPrintVersionInfo_UpdateAvailable(t *testing.T) {
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.0")
	defer SetCurrentVersion("dev")

	server := httptest.NewServer(githubReleaseHandler("v2.0.0", "2024-07-01T00:00:00Z", nil))
	defer server.Close()
	githubAPIURL = server.URL

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printVersionInfo()

	w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)

	if err != nil {
		t.Errorf("printVersionInfo failed: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "Update available") {
		t.Errorf("output missing 'Update available', got:\n%s", s)
	}
}

func TestSelfUpdateCmd_Structure(t *testing.T) {
	if SelfUpdateCmd.Use != "self-update" {
		t.Errorf("Use = %q, want %q", SelfUpdateCmd.Use, "self-update")
	}
	if SelfUpdateCmd.RunE == nil {
		t.Error("RunE should not be nil")
	}

	dryRunFlag := SelfUpdateCmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Error("missing --dry-run flag")
	} else if dryRunFlag.DefValue != "false" {
		t.Errorf("dry-run default = %q, want %q", dryRunFlag.DefValue, "false")
	}

	versionFlag := SelfUpdateCmd.Flags().Lookup("version")
	if versionFlag == nil {
		t.Error("missing --version flag")
	} else if versionFlag.DefValue != "false" {
		t.Errorf("version default = %q, want %q", versionFlag.DefValue, "false")
	}
}

func TestSelfUpdateCmd_VersionFlag(t *testing.T) {
	resetSelfUpdateFlags()
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.8")
	defer SetCurrentVersion("dev")

	server := httptest.NewServer(githubReleaseHandler("v1.0.8", "2024-06-01T12:00:00Z", nil))
	defer server.Close()
	githubAPIURL = server.URL

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	SelfUpdateCmd.SetArgs([]string{"--version"})
	err := SelfUpdateCmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)

	if err != nil {
		t.Errorf("SelfUpdateCmd --version failed: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "sin-code version: v1.0.8") {
		t.Errorf("output missing version, got:\n%s", s)
	}
}

func TestSelfUpdateCmd_DryRunFlag(t *testing.T) {
	resetSelfUpdateFlags()
	saved := githubAPIURL
	defer func() { githubAPIURL = saved }()
	SetCurrentVersion("v1.0.0")
	defer SetCurrentVersion("dev")

	server := httptest.NewServer(githubReleaseHandler("v2.0.0", "2024-07-01T00:00:00Z", nil))
	defer server.Close()
	githubAPIURL = server.URL

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	SelfUpdateCmd.SetArgs([]string{"--dry-run"})
	err := SelfUpdateCmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)

	if err != nil {
		t.Errorf("SelfUpdateCmd --dry-run failed: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "Dry run mode") {
		t.Errorf("output missing 'Dry run mode', got:\n%s", s)
	}
}
