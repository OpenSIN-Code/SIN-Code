// SPDX-License-Identifier: MIT
// Purpose: tests for the attachments package: magic-byte detection, store
// CRUD, dedup, size limit, expire.
package attachments

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetectMIMEPNG(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 13}
	if got := detectMIME(png, "test.png"); got != "image/png" {
		t.Errorf("got %q", got)
	}
}

func TestDetectMIMEJPEG(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'}
	if got := detectMIME(jpeg, "test.jpg"); got != "image/jpeg" {
		t.Errorf("got %q", got)
	}
}

func TestDetectMIMEGIF(t *testing.T) {
	gif := []byte("GIF89a\x00\x00\x00\x00")
	if got := detectMIME(gif, "test.gif"); got != "image/gif" {
		t.Errorf("got %q", got)
	}
}

func TestDetectMIMEPDF(t *testing.T) {
	pdf := []byte("%PDF-1.4\n%âãÏÓ\n")
	if got := detectMIME(pdf, "test.pdf"); got != "application/pdf" {
		t.Errorf("got %q", got)
	}
}

func TestDetectMIMEZip(t *testing.T) {
	zip := []byte{'P', 'K', 0x03, 0x04, 0, 0, 0, 0, 0, 0}
	if got := detectMIME(zip, "test.zip"); got != "application/zip" {
		t.Errorf("got %q", got)
	}
}

func TestDetectMIMEWebP(t *testing.T) {
	webp := []byte{0x52, 0x49, 0x46, 0x46, 0, 0, 0, 0, 0x57, 0x45, 0x42, 0x50, 0x56, 0x50, 0x38, 0x4C, 0, 0, 0, 0}
	if got := detectMIME(webp, "test.webp"); got != "image/webp" {
		t.Errorf("got %q", got)
	}
}

func TestDetectMIMEText(t *testing.T) {
	txt := []byte("Hello, world!\nThis is plain text.\n")
	if got := detectMIME(txt, "test.txt"); got != "text/plain" {
		t.Errorf("got %q", got)
	}
}

func TestDetectMIMEBinary(t *testing.T) {
	bin := []byte{0, 0, 0, 0, 0xFF, 0xFE, 0, 0}
	if got := detectMIME(bin, "test.bin"); got != "application/octet-stream" {
		t.Errorf("got %q", got)
	}
}

func TestIsLikelyText(t *testing.T) {
	if !isLikelyText([]byte("hello")) {
		t.Error("plain text should be text")
	}
	if !isLikelyText([]byte{}) {
		t.Error("empty should be text")
	}
	if isLikelyText([]byte{0, 1, 2}) {
		t.Error("NUL bytes should be binary")
	}
}

func TestExtFor(t *testing.T) {
	cases := map[string]string{
		"image/png":       ".png",
		"image/jpeg":      ".jpg",
		"image/gif":       ".gif",
		"image/webp":      ".webp",
		"application/pdf": ".pdf",
		"application/zip": ".zip",
		"text/plain":      ".txt",
	}
	for mime, want := range cases {
		got := extFor(mime, "x")
		if got != want {
			t.Errorf("mime %s: got %q, want %q", mime, got, want)
		}
	}
}

func TestExtFromName(t *testing.T) {
	if ext := extFromName("test.png", ".txt"); ext != ".png" {
		t.Errorf("got %q", ext)
	}
	if ext := extFromName("test", ".txt"); ext != ".txt" {
		t.Errorf("got %q", ext)
	}
	if ext := extFromName("test", ""); ext != "" {
		t.Errorf("got %q", ext)
	}
}

func TestNewStore(t *testing.T) {
	dir := t.TempDir()
	s, err := NewStoreAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.BaseDir() != dir {
		t.Errorf("BaseDir: got %s", s.BaseDir())
	}
}

func TestStoreAttachBytes(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 13, 'I', 'D', 'A', 'T'}
	a, err := s.AttachReader(bytes.NewReader(png), "test.png", int64(len(png)))
	if err != nil {
		t.Fatal(err)
	}
	if a.MIME != "image/png" {
		t.Errorf("got %q", a.MIME)
	}
	if a.Size != int64(len(png)) {
		t.Errorf("size: %d", a.Size)
	}
}

func TestStoreAttachFile(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	srcPath := filepath.Join(dir, "src.txt")
	_ = os.WriteFile(srcPath, []byte("hello world"), 0o644)
	a, err := s.Attach(srcPath)
	if err != nil {
		t.Fatal(err)
	}
	if a.MIME != "text/plain" {
		t.Errorf("got %q", a.MIME)
	}
}

func TestStoreAttachTooLarge(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	big := bytes.Repeat([]byte("a"), MaxSize+1)
	_, err := s.AttachReader(bytes.NewReader(big), "huge.txt", int64(len(big)))
	if err == nil {
		t.Error("expected error for too large")
	}
}

func TestStoreDedup(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	data := []byte("duplicate me")
	a, _ := s.AttachReader(bytes.NewReader(data), "a.txt", int64(len(data)))
	b, _ := s.AttachReader(bytes.NewReader(data), "b.txt", int64(len(data)))
	if a.Hash != b.Hash {
		t.Error("expected same hash for same content")
	}
}

func TestStoreGetByHash(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	a, _ := s.AttachReader(bytes.NewReader([]byte("hello")), "test.txt", 5)
	got, err := s.Get(a.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash != a.Hash {
		t.Errorf("hash mismatch")
	}
}

func TestStoreGetMissing(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	if _, err := s.Get("nonexistent"); err == nil {
		t.Error("expected error for missing")
	}
}

func TestStoreAttachFileMissing(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	if _, err := s.Attach("/nonexistent/file/zzz"); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestAttachmentMarker(t *testing.T) {
	a := &Attachment{Name: "x.png", MIME: "image/png", Hash: "abcdef123456", Size: 100}
	marker := a.Marker()
	if !strings.Contains(marker, "image:") {
		t.Errorf("missing image tag: %s", marker)
	}
	if !strings.Contains(marker, "x.png") {
		t.Errorf("missing filename: %s", marker)
	}
}

func TestAttachmentMarkerPDF(t *testing.T) {
	a := &Attachment{Name: "doc.pdf", MIME: "application/pdf", Hash: "xyz", Size: 200}
	marker := a.Marker()
	if !strings.Contains(marker, "pdf:") {
		t.Errorf("missing pdf tag: %s", marker)
	}
}

func TestAttachmentIsImage(t *testing.T) {
	cases := map[string]bool{
		"image/png":       true,
		"image/jpeg":      true,
		"image/gif":       true,
		"image/webp":      true,
		"application/pdf": false,
		"text/plain":      false,
	}
	for mime, want := range cases {
		a := &Attachment{MIME: mime}
		if got := a.IsImage(); got != want {
			t.Errorf("%s: got %v, want %v", mime, got, want)
		}
	}
}

func TestStorePrune(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	_, err := s.Prune()
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewStoreDefaultDirError(t *testing.T) {
	orig := osUserConfigDir
	osUserConfigDir = func() (string, error) { return "", fmt.Errorf("no config dir") }
	defer func() { osUserConfigDir = orig }()
	if _, err := NewStore(); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewStoreMkdirAllError(t *testing.T) {
	orig := osMkdirAll
	osMkdirAll = func(string, os.FileMode) error { return fmt.Errorf("mkdir failed") }
	defer func() { osMkdirAll = orig }()
	if _, err := NewStoreAt(t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewStoreMkdirAllError2(t *testing.T) {
	origDir := osUserConfigDir
	origMkdir := osMkdirAll
	osUserConfigDir = func() (string, error) { return t.TempDir(), nil }
	osMkdirAll = func(string, os.FileMode) error { return fmt.Errorf("mkdir failed") }
	defer func() {
		osUserConfigDir = origDir
		osMkdirAll = origMkdir
	}()
	if _, err := NewStore(); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewStoreSuccess(t *testing.T) {
	origDir := osUserConfigDir
	origMkdir := osMkdirAll
	osUserConfigDir = func() (string, error) { return t.TempDir(), nil }
	osMkdirAll = origMkdir
	defer func() {
		osUserConfigDir = origDir
		osMkdirAll = origMkdir
	}()
	s, err := NewStore()
	if err != nil {
		t.Fatal(err)
	}
	if s.BaseDir() == "" {
		t.Fatal("expected base dir")
	}
}

func TestAttachTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	_ = os.WriteFile(path, []byte("x"), 0o644)
	s, _ := NewStoreAt(dir)
	_, err := s.AttachReader(bytes.NewReader([]byte("x")), "big.txt", MaxSize+1)
	if err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestAttachFileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	_ = os.WriteFile(path, bytes.Repeat([]byte("a"), MaxSize+1), 0o644)
	s, _ := NewStoreAt(dir)
	_, err := s.Attach(path)
	if err != ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

func TestAttachOpenError(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	orig := osOpen
	osOpen = func(string) (*os.File, error) { return nil, fmt.Errorf("open error") }
	defer func() { osOpen = orig }()
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte("x"), 0o644)
	if _, err := s.Attach(path); err == nil {
		t.Fatal("expected error")
	}
}

func TestAttachReaderReadAllError(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	orig := osReadAllHook
	osReadAllHook = func(io.Reader) ([]byte, error) { return nil, fmt.Errorf("read error") }
	defer func() { osReadAllHook = orig }()
	if _, err := s.AttachReader(bytes.NewReader([]byte("x")), "x.txt", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestAttachReaderMkdirAllError(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	orig := osMkdirAll
	osMkdirAll = func(string, os.FileMode) error { return fmt.Errorf("mkdir error") }
	defer func() { osMkdirAll = orig }()
	if _, err := s.AttachReader(bytes.NewReader([]byte("x")), "x.txt", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestAttachReaderWriteFileError(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	orig := osWriteFileHook
	osWriteFileHook = func(string, []byte, os.FileMode) error { return fmt.Errorf("write error") }
	defer func() { osWriteFileHook = orig }()
	if _, err := s.AttachReader(bytes.NewReader([]byte("x")), "x.txt", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestGetNotFound(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	if _, err := s.Get("aabbccdd"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetInfoError(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	hash := "aa" + strings.Repeat("0", 62)
	orig := osReadDir
	osReadDir = func(string) ([]os.DirEntry, error) {
		return []os.DirEntry{infoErrEntry(hash + "_link")}, nil
	}
	defer func() { osReadDir = orig }()
	_, err := s.Get(hash)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

type infoErrEntry string

func (e infoErrEntry) Name() string                { return string(e) }
func (e infoErrEntry) IsDir() bool                 { return false }
func (e infoErrEntry) Type() os.FileMode           { return 0 }
func (e infoErrEntry) Info() (os.FileInfo, error)  { return nil, fmt.Errorf("info error") }
func (e infoErrEntry) String() string              { return string(e) }

func TestGetPrefixMismatch(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	targetDir := filepath.Join(dir, "aa")
	_ = os.MkdirAll(targetDir, 0o755)
	_ = os.WriteFile(filepath.Join(targetDir, "bb_"), []byte("x"), 0o644)
	if _, err := s.Get("aa"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestPruneExpired(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewStoreAt(dir)
	copyPath := filepath.Join(s.BaseDir(), "aa", "old.txt")
	_ = os.MkdirAll(filepath.Dir(copyPath), 0o755)
	_ = os.WriteFile(copyPath, []byte("old"), 0o644)
	oldTime := time.Now().UTC().Add(-DefaultExpiry - time.Hour)
	_ = os.Chtimes(copyPath, oldTime, oldTime)
	n, err := s.Prune()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 pruned, got %d", n)
	}
}

func TestDetectMIMEUnknownWithExt(t *testing.T) {
	if got := detectMIME([]byte{0xFF, 0xFE, 0, 0}, "x.bin"); got != "application/octet-stream" {
		t.Errorf("got %q", got)
	}
}

func TestDetectMIMEUnknownNoExt(t *testing.T) {
	if got := detectMIME([]byte{0xFF, 0xFE, 0, 0}, ""); got != "application/octet-stream" {
		t.Errorf("got %q", got)
	}
}

func TestExtForTextFromName(t *testing.T) {
	if got := extFor("text/plain", "data.md"); got != ".md" {
		t.Errorf("got %q", got)
	}
}

func TestExtForUnknown(t *testing.T) {
	if got := extFor("application/x-custom", "data.bin"); got != ".bin" {
		t.Errorf("got %q", got)
	}
}

func TestIsLikelyTextLong(t *testing.T) {
	if !isLikelyText(bytes.Repeat([]byte("a"), 9000)) {
		t.Error("long ascii should be text")
	}
}

func TestIsLikelyTextControlChar(t *testing.T) {
	if isLikelyText([]byte{0x01}) {
		t.Error("control char should be binary")
	}
}

func TestAttachmentMarkerDefault(t *testing.T) {
	a := &Attachment{Name: "x", MIME: "application/x", Hash: "h", Size: 1}
	if got := a.Marker(); !strings.Contains(got, "file:") {
		t.Errorf("got %q", got)
	}
}
