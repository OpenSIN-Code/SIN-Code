// SPDX-License-Identifier: MIT
// Purpose: tests for the attachments package: magic-byte detection, store
// CRUD, dedup, size limit, expire.
package attachments

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		"image/png":     ".png",
		"image/jpeg":    ".jpg",
		"image/gif":     ".gif",
		"image/webp":    ".webp",
		"application/pdf": ".pdf",
		"application/zip": ".zip",
		"text/plain":    ".txt",
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
		"image/png":      true,
		"image/jpeg":     true,
		"image/gif":      true,
		"image/webp":     true,
		"application/pdf": false,
		"text/plain":     false,
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
