// SPDX-License-Identifier: MIT
// Purpose: tests for the vision package (issue #423). All network calls are
// mocked via a custom http.RoundTripper; no real API key or vision model is
// needed.
package vision

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

var errFakeTransport = errors.New("fake transport failure")

type fakeTransport struct {
	statusCode int
	body       string
	lastReq    *http.Request
	lastBody   string
	err        error
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.lastReq = req
	raw, _ := io.ReadAll(req.Body)
	f.lastBody = string(raw)
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: f.statusCode,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func newFakeClient(statusCode int, body string, err error) *http.Client {
	return &http.Client{Transport: &fakeTransport{statusCode: statusCode, body: body, err: err}}
}

func makeVisionResponse(text string) string {
	resp := visionResponse{}
	resp.Choices = append(resp.Choices, struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{Message: struct {
		Content string `json:"content"`
	}{Content: text}})
	b, _ := json.Marshal(resp)
	return string(b)
}

func TestConfig_Validate_MissingBaseURL(t *testing.T) {
	cfg := Config{Model: "m", APIKey: "k"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "base URL") {
		t.Fatalf("expected base URL error, got %v", err)
	}
}

func TestConfig_Validate_MissingModel(t *testing.T) {
	cfg := Config{BaseURL: "https://api.example.com/v1", APIKey: "k"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("expected model error, got %v", err)
	}
}

func TestConfig_Validate_MissingAPIKey(t *testing.T) {
	cfg := Config{BaseURL: "https://api.example.com/v1", Model: "m"}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "API key") {
		t.Fatalf("expected API key error, got %v", err)
	}
}

func TestConfig_Validate_LocalProviderNoKey(t *testing.T) {
	cfg := Config{BaseURL: "http://127.0.0.1:11434/v1", Model: "llava"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("local provider should not require API key: %v", err)
	}
}

func TestAnalyzeImageWithConfig_Success(t *testing.T) {
	fake := &fakeTransport{statusCode: 200, body: makeVisionResponse("A screenshot of a login form")}
	cfg := Config{
		BaseURL: "https://api.example.com/v1",
		APIKey:  "sk-test",
		Model:   "vision-model",
		Prompt:  "Describe this image.",
		HTTP:    &http.Client{Transport: fake},
	}
	// Create a tiny 1x1 PNG: minimal valid PNG bytes.
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	path := t.TempDir() + "/test.png"
	if err := writeFile(path, png); err != nil {
		t.Fatal(err)
	}

	res, err := AnalyzeImageWithConfig(context.Background(), path, cfg)
	if err != nil {
		t.Fatalf("AnalyzeImageWithConfig: %v", err)
	}
	if res.Description != "A screenshot of a login form" {
		t.Errorf("unexpected description: %q", res.Description)
	}
	if res.Model != "vision-model" {
		t.Errorf("unexpected model: %q", res.Model)
	}
	if fake.lastReq == nil {
		t.Fatal("expected request to be sent")
	}
	if fake.lastReq.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", fake.lastReq.Method)
	}
	if !strings.Contains(fake.lastBody, "Describe this image.") {
		t.Errorf("expected prompt in body, got %s", fake.lastBody)
	}
	if !strings.Contains(fake.lastBody, "data:image/png;base64,") {
		t.Errorf("expected base64 PNG in body, got %s", fake.lastBody)
	}
	if !strings.Contains(fake.lastBody, "vision-model") {
		t.Errorf("expected model in body, got %s", fake.lastBody)
	}
}

func TestAnalyzeImageWithConfig_EmptyImage(t *testing.T) {
	cfg := Config{BaseURL: "https://api.example.com/v1", APIKey: "k", Model: "m", HTTP: newFakeClient(0, "", nil)}
	path := t.TempDir() + "/empty.png"
	if err := writeFile(path, []byte{}); err != nil {
		t.Fatal(err)
	}
	_, err := AnalyzeImageWithConfig(context.Background(), path, cfg)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-file error, got %v", err)
	}
}

func TestAnalyzeImageWithConfig_TransportError(t *testing.T) {
	cfg := Config{
		BaseURL: "https://api.example.com/v1",
		APIKey:  "k",
		Model:   "m",
		HTTP:    newFakeClient(0, "", errFakeTransport),
	}
	path := t.TempDir() + "/test.png"
	if err := writeFile(path, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}); err != nil {
		t.Fatal(err)
	}
	_, err := AnalyzeImageWithConfig(context.Background(), path, cfg)
	if err == nil || !strings.Contains(err.Error(), "transport") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestAnalyzeImageWithConfig_NonOKStatus(t *testing.T) {
	cfg := Config{
		BaseURL: "https://api.example.com/v1",
		APIKey:  "k",
		Model:   "m",
		HTTP:    newFakeClient(401, "unauthorized", nil),
	}
	path := t.TempDir() + "/test.png"
	if err := writeFile(path, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}); err != nil {
		t.Fatal(err)
	}
	_, err := AnalyzeImageWithConfig(context.Background(), path, cfg)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got %v", err)
	}
}

func TestAnalyzeImageWithConfig_NoChoices(t *testing.T) {
	cfg := Config{
		BaseURL: "https://api.example.com/v1",
		APIKey:  "k",
		Model:   "m",
		HTTP:    newFakeClient(200, `{"choices": []}`, nil),
	}
	path := t.TempDir() + "/test.png"
	if err := writeFile(path, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}); err != nil {
		t.Fatal(err)
	}
	_, err := AnalyzeImageWithConfig(context.Background(), path, cfg)
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("expected no choices error, got %v", err)
	}
}

func TestAnalyzeImageWithConfig_APIError(t *testing.T) {
	body := `{"error": {"message": "model not found"}}`
	cfg := Config{
		BaseURL: "https://api.example.com/v1",
		APIKey:  "k",
		Model:   "m",
		HTTP:    newFakeClient(200, body, nil),
	}
	path := t.TempDir() + "/test.png"
	if err := writeFile(path, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}); err != nil {
		t.Fatal(err)
	}
	_, err := AnalyzeImageWithConfig(context.Background(), path, cfg)
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Fatalf("expected API error, got %v", err)
	}
}

func TestImageMimeType(t *testing.T) {
	cases := []struct {
		ext  string
		want string
	}{
		{"png", "image/png"},
		{"jpg", "image/jpeg"},
		{"jpeg", "image/jpeg"},
		{"gif", "image/gif"},
		{"webp", "image/webp"},
		{"unknown", "image/png"},
	}
	for _, c := range cases {
		got := imageMimeType(c.ext, "x."+c.ext)
		if got != c.want {
			t.Errorf("imageMimeType(%q) = %q, want %q", c.ext, got, c.want)
		}
	}
}

func writeFile(path string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	f.Close()
	return err
}
