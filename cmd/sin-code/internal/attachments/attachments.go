// SPDX-License-Identifier: MIT
// Purpose: attachment store with SHA-256 dedup and magic-byte MIME detection.
// Supports images (PNG, JPEG, GIF), PDFs, and text. Backed by filesystem
// (no bbolt needed; files are content-addressed by hash).
package attachments

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	MaxSize       = 50 * 1024 * 1024
	DefaultExpiry = 30 * 24 * time.Hour
)

var (
	ErrTooLarge = errors.New("attachment exceeds 50MB")
	ErrNotFound = errors.New("attachment not found")
)

type Attachment struct {
	ID        string    `json:"id"`
	Hash      string    `json:"hash"`
	Name      string    `json:"name"`
	MIME      string    `json:"mime"`
	Size      int64     `json:"size"`
	Path      string    `json:"path"`
	Created   time.Time `json:"created"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Store struct {
	baseDir string
}

func NewStore() (*Store, error) {
	dir, err := defaultDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{baseDir: dir}, nil
}

func NewStoreAt(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{baseDir: dir}, nil
}

func (s *Store) Attach(srcPath string) (*Attachment, error) {
	info, err := os.Stat(srcPath)
	if err != nil {
		return nil, err
	}
	if info.Size() > MaxSize {
		return nil, ErrTooLarge
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return s.AttachReader(f, filepath.Base(srcPath), info.Size())
}

func (s *Store) AttachReader(r io.Reader, name string, size int64) (*Attachment, error) {
	if size > MaxSize {
		return nil, ErrTooLarge
	}
	h := sha256.New()
	buf, err := io.ReadAll(io.TeeReader(r, h))
	if err != nil {
		return nil, err
	}
	hashHex := hex.EncodeToString(h.Sum(nil))
	mime := detectMIME(buf, name)
	ext := extFor(mime, name)
	relPath := hashHex[:2] + "/" + hashHex + ext
	fullPath := filepath.Join(s.baseDir, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, err
	}
	if _, err := os.Stat(fullPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(fullPath, buf, 0o644); err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	return &Attachment{
		ID:        hashHex[:8],
		Hash:      hashHex,
		Name:      name,
		MIME:      mime,
		Size:      int64(len(buf)),
		Path:      relPath,
		Created:   now,
		ExpiresAt: now.Add(DefaultExpiry),
	}, nil
}

func (s *Store) Get(hash string) (*Attachment, error) {
	dir := filepath.Join(s.baseDir, hash[:2])
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, ErrNotFound
	}
	prefix := hash
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			info, err := e.Info()
			if err != nil {
				continue
			}
			now := time.Now().UTC()
			return &Attachment{
				Hash:      hash,
				Name:      strings.TrimPrefix(e.Name(), prefix),
				Path:      hash[:2] + "/" + e.Name(),
				Size:      info.Size(),
				Created:   info.ModTime().UTC(),
				ExpiresAt: now.Add(DefaultExpiry),
			}, nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) Prune() (int, error) {
	count := 0
	now := time.Now().UTC()
	err := filepath.Walk(s.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if now.Sub(info.ModTime()) > DefaultExpiry {
			_ = os.Remove(path)
			count++
		}
		return nil
	})
	return count, err
}

func (s *Store) BaseDir() string { return s.baseDir }

func defaultDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "sin-code", "attachments"), nil
}

func detectMIME(data []byte, name string) string {
	if len(data) >= 8 {
		if data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' {
			return "image/png"
		}
		if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
			return "image/jpeg"
		}
		if string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a" {
			return "image/gif"
		}
		if string(data[:4]) == "%PDF" {
			return "application/pdf"
		}
		if string(data[:2]) == "PK" {
			return "application/zip"
		}
	}
	if len(data) >= 4 {
		if string(data[:4]) == "RIFF" && len(data) > 12 && string(data[8:12]) == "WEBP" {
			return "image/webp"
		}
	}
	if isLikelyText(data) {
		return "text/plain"
	}
	if ext := strings.ToLower(filepath.Ext(name)); ext != "" {
		return "application/octet-stream"
	}
	return "application/octet-stream"
}

func extFor(mime, name string) string {
	switch mime {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "text/plain":
		return extFromName(name, ".txt")
	}
	return extFromName(name, "")
}

func extFromName(name, def string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return def
	}
	return ext
}

func isLikelyText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	for i, b := range data {
		if i > 8192 {
			break
		}
		if b == 0 {
			return false
		}
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' && b != 0x1B {
			return false
		}
	}
	return true
}

func (a *Attachment) Marker() string {
	switch a.MIME {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return fmt.Sprintf("[image: %s (%s, %d bytes)](sin://attachments/%s)", a.Name, a.MIME, a.Size, a.Hash)
	case "application/pdf":
		return fmt.Sprintf("[pdf: %s (%d bytes)](sin://attachments/%s)", a.Name, a.Size, a.Hash)
	default:
		return fmt.Sprintf("[file: %s (%s, %d bytes)](sin://attachments/%s)", a.Name, a.MIME, a.Size, a.Hash)
	}
}

func (a *Attachment) IsImage() bool {
	return strings.HasPrefix(a.MIME, "image/")
}
