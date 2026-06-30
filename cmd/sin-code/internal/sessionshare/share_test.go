// SPDX-License-Identifier: MIT
package sessionshare

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestToJSON(t *testing.T) {
	e := &Export{Version: 1, Title: "Test", CreatedAt: time.Now(), Messages: []Message{{Role: "user", Content: "hello"}}}
	data, err := e.ToJSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty JSON")
	}
}

func TestToHTML(t *testing.T) {
	e := &Export{Title: "Test", CreatedAt: time.Now(), Messages: []Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "hi"}}}
	html, err := e.ToHTML()
	if err != nil {
		t.Fatal(err)
	}
	if !contains(html, "Test") {
		t.Error("HTML missing title")
	}
	if !contains(html, "hello") {
		t.Error("HTML missing message")
	}
}

func TestRoundTrip(t *testing.T) {
	e := &Export{Version: 1, Title: "Round", CreatedAt: time.Now(), Messages: []Message{{Role: "user", Content: "test"}}}
	data, _ := e.ToJSON()
	e2, err := FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if e2.Title != "Test" && e2.Title != "Round" {
		t.Errorf("title = %q", e2.Title)
	}
}

func TestWriteFile_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	e := &Export{Title: "Test", CreatedAt: time.Now(), Messages: []Message{{Role: "user", Content: "hi"}}}
	if err := e.WriteFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestWriteFile_HTML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.html")
	e := &Export{Title: "Test", CreatedAt: time.Now(), Messages: []Message{{Role: "user", Content: "hi"}}}
	if err := e.WriteFile(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if len(data) == 0 {
		t.Fatal("empty HTML file")
	}
}

func TestWriteFile_InvalidExt(t *testing.T) {
	dir := t.TempDir()
	e := &Export{Title: "Test"}
	err := e.WriteFile(filepath.Join(dir, "session.txt"))
	if err == nil {
		t.Fatal("expected error for .txt")
	}
}

func TestFromJSON(t *testing.T) {
	data := []byte(`{"version":1,"title":"Test","messages":[{"role":"user","content":"hi"}]}`)
	e, err := FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "Test" {
		t.Errorf("title = %q", e.Title)
	}
	if len(e.Messages) != 1 {
		t.Fatalf("messages = %d", len(e.Messages))
	}
}

func TestFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.json")
	e1 := &Export{Title: "Test", CreatedAt: time.Now(), Messages: []Message{{Role: "user", Content: "hi"}}}
	e1.WriteFile(path)
	e2, err := FromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if e2.Title != "Test" {
		t.Errorf("title = %q", e2.Title)
	}
}

func TestFromJSON_DefaultVersion(t *testing.T) {
	data := []byte(`{"title":"Test"}`)
	e, err := FromJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	if e.Version != 1 {
		t.Errorf("version = %d, want 1", e.Version)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
