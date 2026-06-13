// SPDX-License-Identifier: MIT
// Purpose: unit tests for the Go-native SCA package.
package sca

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsGoProject(t *testing.T) {
	dir := t.TempDir()
	if IsGoProject(dir) {
		t.Fatal("empty directory should not be a Go project")
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !IsGoProject(dir) {
		t.Fatal("directory with go.mod should be a Go project")
	}
}

func TestDetectGoProject(t *testing.T) {
	dir := t.TempDir()
	if DetectGoProject(dir) {
		t.Fatal("expected false for empty dir")
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if !DetectGoProject(dir) {
		t.Fatal("expected true for go.mod dir")
	}
}

func TestParseGoModBytes(t *testing.T) {
	data := []byte(`module example.com/demo

go 1.23

require (
	github.com/spf13/cobra v1.10.2
	github.com/BurntSushi/toml v1.6.0 // indirect
)

require github.com/sirupsen/logrus v1.9.3
`)
	pkgs, err := parseGoModBytes(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(pkgs) != 3 {
		t.Fatalf("expected 3 packages, got %d: %+v", len(pkgs), pkgs)
	}
	want := map[string]string{
		"github.com/spf13/cobra":     "v1.10.2",
		"github.com/BurntSushi/toml": "v1.6.0",
		"github.com/sirupsen/logrus": "v1.9.3",
	}
	for _, p := range pkgs {
		if p.Ecosystem != "Go" {
			t.Errorf("expected ecosystem Go, got %q", p.Ecosystem)
		}
		if want[p.Name] != p.Version {
			t.Errorf("unexpected version for %s: got %s, want %s", p.Name, p.Version, want[p.Name])
		}
	}
}

func TestParseGoModBytes_Duplicates(t *testing.T) {
	// Duplicate require lines must be deduplicated by module path.
	data := []byte(`module x

go 1.23
require github.com/a/b v1.0.0
require github.com/a/b v1.0.0
`)
	pkgs, err := parseGoModBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 unique package, got %d", len(pkgs))
	}
}

func TestParseGoModBytes_Invalid(t *testing.T) {
	_, err := parseGoModBytes([]byte("not a go.mod file"))
	if err == nil {
		t.Fatal("expected error for invalid go.mod")
	}
}

func TestParseGoMod(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module example.com/test\n\nrequire github.com/foo/bar v1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	pkgs, err := ParseGoMod(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "github.com/foo/bar" || pkgs[0].Version != "v1.2.3" {
		t.Fatalf("unexpected packages: %+v", pkgs)
	}
}

func TestParseGoMod_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := ParseGoMod(dir)
	if err == nil {
		t.Fatal("expected error for missing go.mod")
	}
}

func TestNormalizeSeverity(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Critical", "critical"},
		{"HIGH", "high"},
		{"Medium", "medium"},
		{"low", "low"},
		{"Negligible", "negligible"},
		{"", "unknown"},
		{"foo", "unknown"},
	}
	for _, c := range cases {
		got := normalizeSeverity(c.in)
		if got != c.want {
			t.Errorf("normalizeSeverity(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSummarize(t *testing.T) {
	vulns := []Vulnerability{
		{Severity: "High"},
		{Severity: "medium"},
		{Severity: "Low"},
		{Severity: ""},
	}
	s := summarize(vulns, 7)
	if s["total"] != 4 || s["packages"] != 7 || s["high"] != 1 || s["medium"] != 1 || s["low"] != 1 || s["unknown"] != 1 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestParseGrypeReport(t *testing.T) {
	data := []byte(`{
  "matches": [
    {
      "vulnerability": {
        "id": "CVE-2024-1234",
        "severity": "High",
        "description": "A bad thing",
        "fix": {
          "versions": ["v1.2.4"],
          "state": "fixed"
        }
      },
      "artifact": {
        "name": "github.com/foo/bar",
        "version": "v1.2.3",
        "type": "go-module"
      }
    }
  ]
}`)
	vulns, err := parseGrypeReport(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(vulns) != 1 {
		t.Fatalf("expected 1 vuln, got %d", len(vulns))
	}
	v := vulns[0]
	if v.ID != "CVE-2024-1234" || v.Severity != "High" || v.Package != "github.com/foo/bar" || v.Version != "v1.2.3" {
		t.Fatalf("unexpected vuln: %+v", v)
	}
	if len(v.FixedIn) != 1 || v.FixedIn[0] != "v1.2.4" {
		t.Fatalf("unexpected fixedIn: %+v", v.FixedIn)
	}
	if v.Description != "A bad thing" {
		t.Fatalf("unexpected description: %q", v.Description)
	}
}

func TestParseGrypeReport_InvalidJSON(t *testing.T) {
	_, err := parseGrypeReport([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGrypeClientScanDirectory_Success(t *testing.T) {
	client := &GrypeClient{
		Path: "grype-mock",
		CommandRunner: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "cat")
			cmd.Stdin = grypeFixture(t)
			return cmd
		},
	}
	vulns, err := client.ScanDirectory(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(vulns) != 1 {
		t.Fatalf("expected 1 vuln, got %d", len(vulns))
	}
}

func TestGrypeClientScanDirectory_Failure(t *testing.T) {
	client := &GrypeClient{
		Path: "grype-mock",
		CommandRunner: func(ctx context.Context, name string, arg ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "false")
		},
	}
	_, err := client.ScanDirectory(context.Background(), t.TempDir())
	if err == nil {
		t.Fatal("expected error when grype fails")
	}
}

func TestScannerScan_NoGrype(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module example.com/test\n\nrequire github.com/foo/bar v1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	scanner := NewWithGrype(&GrypeClient{
		Path: "this-binary-does-not-exist-12345",
	})
	res, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.PackagesScanned != 1 {
		t.Fatalf("expected 1 package scanned, got %d", res.PackagesScanned)
	}
	if len(res.Vulnerabilities) != 0 {
		t.Fatalf("expected 0 vulns without grype, got %d", len(res.Vulnerabilities))
	}
	if res.Summary["packages"] != 1 || res.Summary["total"] != 0 {
		t.Fatalf("unexpected summary: %+v", res.Summary)
	}
}

func TestScannerScan_WithGrype(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module example.com/test\n\nrequire github.com/foo/bar v1.2.3\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mock := mockGrype(t)
	scanner := NewWithGrype(mock)
	res, err := scanner.Scan(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.PackagesScanned != 1 {
		t.Fatalf("expected 1 package scanned, got %d", res.PackagesScanned)
	}
	if len(res.Vulnerabilities) != 1 {
		t.Fatalf("expected 1 vuln, got %d", len(res.Vulnerabilities))
	}
	if res.Summary["high"] != 1 {
		t.Fatalf("expected high=1, got summary: %+v", res.Summary)
	}
}

func TestScannerScanPackages(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module example.com/test\n\nrequire github.com/a/b v1.0.0\nrequire github.com/c/d v1.1.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	pkgs, err := New().ScanPackages(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
}

func TestScannerScan_NonGoProject(t *testing.T) {
	dir := t.TempDir()
	_, err := New().Scan(context.Background(), dir)
	if err == nil {
		t.Fatal("expected error for non-Go project")
	}
}

func TestLower(t *testing.T) {
	if lower("AbC") != "abc" {
		t.Fatalf("unexpected lower: %s", lower("AbC"))
	}
}

func TestFileExists(t *testing.T) {
	f := filepath.Join(t.TempDir(), "exists.txt")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if !fileExists(f) {
		t.Fatal("expected file to exist")
	}
	if fileExists(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("expected file to not exist")
	}
}

func TestTruncate(t *testing.T) {
	short := []byte("hello")
	if truncate(short) != "hello" {
		t.Fatal("truncate should not change short input")
	}
	long := make([]byte, 500)
	for i := range long {
		long[i] = 'a'
	}
	out := truncate(long)
	if len(out) != 403 {
		t.Fatalf("expected truncated length 403, got %d", len(out))
	}
}

func grypeFixture(t *testing.T) *os.File {
	t.Helper()
	data := []byte(`{
  "matches": [
    {
      "vulnerability": {
        "id": "CVE-2024-1234",
        "severity": "High",
        "description": "Bad thing",
        "fix": {
          "versions": ["v1.2.4"],
          "state": "fixed"
        }
      },
      "artifact": {
        "name": "github.com/foo/bar",
        "version": "v1.2.3",
        "type": "go-module"
      }
    }
  ]
}`)
	f, err := os.CreateTemp(t.TempDir(), "grype-*.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	return f
}

// mockGrype creates a temporary executable named "grype" that outputs a
// fixed JSON report. The returned GrypeClient is configured to run it.
func mockGrype(t *testing.T) *GrypeClient {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "grype")
	// Use /bin/sh because PATH on some macOS environments may not include bash.
	body := `#!/bin/sh
printf '%s' '{
  "matches": [
    {
      "vulnerability": {
        "id": "CVE-2024-1234",
        "severity": "High",
        "description": "Bad thing",
        "fix": {
          "versions": ["v1.2.4"],
          "state": "fixed"
        }
      },
      "artifact": {
        "name": "github.com/foo/bar",
        "version": "v1.2.3",
        "type": "go-module"
      }
    }
  ]
}'
`
	if err := os.WriteFile(script, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", dir+":"+oldPath)
	t.Cleanup(func() { os.Setenv("PATH", oldPath) })
	return &GrypeClient{Path: script, CommandRunner: exec.CommandContext}
}

// BenchmarkParseGoModBytes gives a rough sense of parser performance.
func BenchmarkParseGoModBytes(b *testing.B) {
	data := []byte(fmt.Sprintf(`module example.com/bench

go 1.23

require (
%s
)
`, generateRequires(200)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parseGoModBytes(data); err != nil {
			b.Fatal(err)
		}
	}
}

func generateRequires(n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += fmt.Sprintf("\tgithub.com/pkg%d/lib v1.0.%d\n", i, i%10)
	}
	return out
}
