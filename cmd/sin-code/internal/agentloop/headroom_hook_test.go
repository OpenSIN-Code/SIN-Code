// SPDX-License-Identifier: MIT
// Purpose: cover headroom_hook.go statements (issue #118 wiring).
package agentloop

import (
	"context"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/internal/headroom"
)

func TestNewHeadroomHook_Disabled(t *testing.T) {
	h := NewHeadroomHook(headroom.Config{Enabled: false})
	if h.Enabled() {
		t.Fatal("expected disabled hook")
	}
	if h.Compressor() != nil {
		t.Fatal("expected nil compressor for disabled hook")
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewHeadroomHook_StartFailure(t *testing.T) {
	// Unknown mode makes Start fail deterministically.
	h := NewHeadroomHook(headroom.Config{Enabled: true, Mode: headroom.Mode("unknown")})
	if h.Enabled() {
		t.Fatal("expected disabled hook after Start failure")
	}
}

func TestNewHeadroomHook_StartSuccess(t *testing.T) {
	h := NewHeadroomHook(headroom.Config{Enabled: true, Mode: headroom.ModeProxy})
	if !h.Enabled() {
		t.Fatal("expected enabled proxy hook")
	}
	if h.Compressor() == nil {
		t.Fatal("expected compressor for enabled proxy hook")
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHeadroomHook_NilMethods(t *testing.T) {
	var h *HeadroomHook
	if h.Enabled() {
		t.Fatal("nil hook must report disabled")
	}
	if h.Compressor() != nil {
		t.Fatal("nil hook compressor must be nil")
	}
	if err := h.Close(); err != nil {
		t.Fatal("nil hook Close must return nil")
	}
}

func TestHeadroomHook_CompressMessages_Disabled(t *testing.T) {
	h := NewHeadroomHook(headroom.Config{Enabled: false})
	msgs := []session.Message{{Role: "user", Content: "hi"}}
	out, err := h.CompressMessages(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != len(msgs) || out[0].Content != "hi" {
		t.Fatal("disabled hook must pass messages through unchanged")
	}
}

func TestHeadroomHook_CompressMessages_Empty(t *testing.T) {
	h := NewHeadroomHook(headroom.Config{Enabled: true, Mode: headroom.ModeProxy})
	out, err := h.CompressMessages(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatal("expected empty output for nil input")
	}
}

func TestHeadroomHook_CompressMessages_PreservesEmptyContent(t *testing.T) {
	h := NewHeadroomHook(headroom.Config{Enabled: true, Mode: headroom.ModeProxy})
	msgs := []session.Message{{Role: "assistant", Content: ""}}
	out, err := h.CompressMessages(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Content != "" {
		t.Fatal("expected empty content preserved")
	}
}

func TestHeadroomHook_CompressMessages_Passthrough(t *testing.T) {
	h := NewHeadroomHook(headroom.Config{Enabled: true, Mode: headroom.ModeProxy})
	msgs := []session.Message{{Role: "user", Content: "hello"}}
	out, err := h.CompressMessages(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Content != "hello" {
		t.Fatal("proxy mode must pass content through unchanged")
	}
}

func TestHeadroomHook_CompressMessages_KeepsOriginalOnError(t *testing.T) {
	// A compressor with mode MCP but no started client returns an error from CompressContent.
	comp := headroom.NewCompressor(headroom.Config{Enabled: true, Mode: headroom.ModeMCP})
	h := &HeadroomHook{compressor: comp, enabled: true}
	msgs := []session.Message{{Role: "user", Content: "keep me"}}
	out, err := h.CompressMessages(context.Background(), msgs)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].Content != "keep me" {
		t.Fatal("expected original content preserved on compression error")
	}
}
