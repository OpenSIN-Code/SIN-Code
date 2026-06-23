// SPDX-License-Identifier: MIT
// Purpose: tests for `sin-code analyse-image` (issue #423). Exercises flag
// parsing, JSON output, and error paths without making real API calls.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vision"
)

type mockVisionTransport struct {
	body    string
	status  int
	lastReq *http.Request
}

func (m *mockVisionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.lastReq = req
	return &http.Response{
		StatusCode: m.status,
		Body:       io.NopCloser(strings.NewReader(m.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func mockVisionClient(status int, body string) *http.Client {
	return &http.Client{Transport: &mockVisionTransport{status: status, body: body}}
}

func TestNewAnalyseImageCmd_DefaultPrompt(t *testing.T) {
	cmd := NewAnalyseImageCmd()
	if cmd.Name() != "analyse-image" {
		t.Errorf("expected command name analyse-image, got %q", cmd.Name())
	}
	flag := cmd.Flags().Lookup("prompt")
	if flag == nil {
		t.Fatal("expected --prompt flag")
	}
	if flag.DefValue != "" {
		t.Errorf("expected default prompt empty, got %q", flag.DefValue)
	}
}

func TestAnalyseImageCmd_RequiresPath(t *testing.T) {
	cmd := NewAnalyseImageCmd()
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestAnalyseImageCmd_JSONOutput(t *testing.T) {
	cmd := NewAnalyseImageCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	// Create a tiny PNG in a temp dir.
	tmp := t.TempDir()
	path := tmp + "/x.png"
	if err := writeTestPNG(path); err != nil {
		t.Fatal(err)
	}

	// Inject a mock transport by overriding VisionConfigFromEnv via the
	// exported analyseImageHook. If the hook is nil, we still exercise the
	// command wiring by expecting a config/validation error.
	cfg := vision.Config{
		BaseURL: "https://fake.example.com/v1",
		APIKey:  "sk",
		Model:   "m",
		Prompt:  "describe",
		HTTP:    mockVisionClient(200, makeVisionResponse("A chart")),
	}

	orig := analyseImageHook
	analyseImageHook = func(context.Context, string, vision.Config) (*vision.AnalyzeResult, error) {
		return &vision.AnalyzeResult{Description: "A chart", Model: "m", Provider: cfg.BaseURL}, nil
	}
	defer func() { analyseImageHook = orig }()

	cmd.SetArgs([]string{path, "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got vision.AnalyzeResult
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, buf.String())
	}
	if got.Description != "A chart" {
		t.Errorf("unexpected description: %q", got.Description)
	}
	if got.Model != "m" {
		t.Errorf("unexpected model: %q", got.Model)
	}
}

func makeVisionResponse(text string) string {
	return `{"choices":[{"message":{"content":"` + text + `"}}]}`
}

func writeTestPNG(path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	_, err = f.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	f.Close()
	return err
}

var createFile = func(path string) (*os.File, error) { return os.Create(path) }
