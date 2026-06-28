// SPDX-License-Identifier: MIT
package tui

import (
	"strings"
	"testing"
)

func TestLinkifyText_WithURL(t *testing.T) {
	styles := NewStyles(Themes[0])
	text := "Check out https://example.com for more info"
	result := LinkifyText(text, styles)

	if !strings.Contains(result, "https://example.com") {
		t.Errorf("result should contain the URL text")
	}
	if !strings.Contains(result, osc8Begin) {
		t.Errorf("result should contain OSC 8 begin sequence")
	}
	if !strings.Contains(result, osc8End) {
		t.Errorf("result should contain OSC 8 end sequence")
	}
}

func TestLinkifyText_NoURL(t *testing.T) {
	styles := NewStyles(Themes[0])
	text := "This is just regular text without any links"
	result := LinkifyText(text, styles)

	if result != text {
		t.Errorf("text without URLs should be unchanged, got %q", result)
	}
}

func TestLinkifyText_CodeBlockNotLinkified(t *testing.T) {
	styles := NewStyles(Themes[0])
	text := "```\nhttps://example.com\n```\nNormal https://example.org"
	result := LinkifyText(text, styles)

	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
	if strings.Contains(lines[1], osc8Begin) {
		t.Errorf("URL inside code block should not be linkified, line: %q", lines[1])
	}
}

func TestHasURLs(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"Check https://example.com", true},
		{"http://foo.bar/baz", true},
		{"No links here", false},
		{"", false},
		{"Just text with dots...", false},
		{"Visit https://docs.opensin.ai/best-practices/ci-cd-n8n", true},
	}
	for _, tc := range tests {
		got := HasURLs(tc.text)
		if got != tc.want {
			t.Errorf("HasURLs(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestExtractURLs(t *testing.T) {
	text := "Visit https://example.com and http://foo.bar/baz for details"
	urls := ExtractURLs(text)

	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d: %v", len(urls), urls)
	}
	if urls[0] != "https://example.com" {
		t.Errorf("first URL = %q, want %q", urls[0], "https://example.com")
	}
	if urls[1] != "http://foo.bar/baz" {
		t.Errorf("second URL = %q, want %q", urls[1], "http://foo.bar/baz")
	}
}
