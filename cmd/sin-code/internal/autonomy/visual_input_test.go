// SPDX-License-Identifier: MIT
// Purpose: tests for visual_input.go (issue #386). Uses a mock HTTPDoer
// so no real network calls are made.
package autonomy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

var errFakeTransport = errors.New("fake transport failure")

type mockTransport struct {
	statusCode int
	body       string
	err        error
	lastReq    *http.Request
	lastBody   string
}

func (m *mockTransport) Do(req *http.Request) (*http.Response, error) {
	m.lastReq = req
	raw, _ := io.ReadAll(req.Body)
	m.lastBody = string(raw)
	if m.err != nil {
		return nil, m.err
	}
	return &http.Response{
		StatusCode: m.statusCode,
		Body:       io.NopCloser(strings.NewReader(m.body)),
		Header:     make(http.Header),
	}, nil
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

func TestVisualEncodeImage(t *testing.T) {
	p := NewVisualProcessor()
	img := p.EncodeImage([]byte("test"), "png")
	if img.Format != "png" {
		t.Errorf("expected format png, got %s", img.Format)
	}
	if img.Base64 == "" {
		t.Error("expected non-empty base64")
	}
	if string(img.Data) != "test" {
		t.Errorf("expected data preserved, got %s", img.Data)
	}
}

func TestVisualDescribeImageSuccess(t *testing.T) {
	mt := &mockTransport{
		statusCode: 200,
		body:       makeVisionResponse("A screenshot of a login form"),
	}
	p := &VisualProcessor{transport: mt, endpoint: "https://fake.test/vision"}
	img := ImageInput{Data: []byte("pngdata"), Format: "png", Base64: "cG5nZGF0YQ=="}

	desc, err := p.DescribeImage(context.Background(), img, "What is this?")
	if err != nil {
		t.Fatalf("DescribeImage: %v", err)
	}
	if desc != "A screenshot of a login form" {
		t.Errorf("unexpected description: %q", desc)
	}
	if mt.lastReq == nil {
		t.Fatal("expected request to be sent")
	}
	if mt.lastReq.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", mt.lastReq.Method)
	}
	if !strings.Contains(mt.lastBody, "What is this?") {
		t.Error("expected prompt in request body")
	}
	if !strings.Contains(mt.lastBody, "base64") {
		t.Error("expected base64 image in request body")
	}
}

func TestVisualDescribeImageEmptyData(t *testing.T) {
	p := NewVisualProcessor()
	_, err := p.DescribeImage(context.Background(), ImageInput{}, "describe")
	if err == nil {
		t.Fatal("expected error for empty image data")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected 'empty' in error, got %v", err)
	}
}

func TestVisualDescribeImageTransportError(t *testing.T) {
	mt := &mockTransport{err: errFakeTransport}
	p := &VisualProcessor{transport: mt, endpoint: "https://fake.test/vision"}
	img := ImageInput{Data: []byte("x"), Format: "png", Base64: "eA=="}

	_, err := p.DescribeImage(context.Background(), img, "describe")
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !strings.Contains(err.Error(), "transport") {
		t.Errorf("expected 'transport' in error, got %v", err)
	}
}

func TestVisualDescribeImageNonOKStatus(t *testing.T) {
	mt := &mockTransport{statusCode: 401, body: "unauthorized"}
	p := &VisualProcessor{transport: mt, endpoint: "https://fake.test/vision"}
	img := ImageInput{Data: []byte("x"), Format: "png", Base64: "eA=="}

	_, err := p.DescribeImage(context.Background(), img, "describe")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected status code in error, got %v", err)
	}
}

func TestVisualDescribeImageDefaultPrompt(t *testing.T) {
	mt := &mockTransport{statusCode: 200, body: makeVisionResponse("ok")}
	p := &VisualProcessor{transport: mt, endpoint: "https://fake.test/vision"}
	img := ImageInput{Data: []byte("x"), Format: "png", Base64: "eA=="}

	_, err := p.DescribeImage(context.Background(), img, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(mt.lastBody, "Describe this image") {
		t.Error("expected default prompt when prompt is empty")
	}
}

func TestVisualExtractDiagram(t *testing.T) {
	diagramText := `FORMAT:flowchart
box|Start|100|50|200|80
arrow|next|310|90|150|40
box|Process|100|200|200|80`
	mt := &mockTransport{statusCode: 200, body: makeVisionResponse(diagramText)}
	p := &VisualProcessor{transport: mt, endpoint: "https://fake.test/vision"}
	img := ImageInput{Data: []byte("diagram"), Format: "png", Base64: "ZGlhZ3JhbQ=="}

	result, err := p.ExtractDiagram(context.Background(), img)
	if err != nil {
		t.Fatalf("ExtractDiagram: %v", err)
	}
	if result.Format != "flowchart" {
		t.Errorf("expected format flowchart, got %s", result.Format)
	}
	if len(result.Elements) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result.Elements))
	}
	first := result.Elements[0]
	if first.Type != "box" || first.Label != "Start" {
		t.Errorf("unexpected first element: %+v", first)
	}
	if first.X != 100 || first.Y != 50 || first.Width != 200 || first.Height != 80 {
		t.Errorf("unexpected coordinates: %+v", first)
	}
}

func TestVisualExtractDiagramNoElements(t *testing.T) {
	mt := &mockTransport{statusCode: 200, body: makeVisionResponse("FORMAT:unknown\nnot a diagram")}
	p := &VisualProcessor{transport: mt, endpoint: "https://fake.test/vision"}
	img := ImageInput{Data: []byte("x"), Format: "png", Base64: "eA=="}

	result, err := p.ExtractDiagram(context.Background(), img)
	if err != nil {
		t.Fatalf("ExtractDiagram: %v", err)
	}
	if result.Format != "unknown" {
		t.Errorf("expected format unknown, got %s", result.Format)
	}
	if len(result.Elements) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result.Elements))
	}
}
