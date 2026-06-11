// SPDX-License-Identifier: MIT
// Purpose: tests for the arg-parse helpers in serve_rw_handlers.go.

package internal

import "testing"

func TestStringArg_Default(t *testing.T) {
	if got := stringArg(nil, "missing", "def"); got != "def" {
		t.Fatalf("missing key: %q", got)
	}
}

func TestStringArg_StringValue(t *testing.T) {
	if got := stringArg(map[string]any{"k": "v"}, "k", "x"); got != "v" {
		t.Fatalf("got %q", got)
	}
}

func TestStringArg_EmptyString(t *testing.T) {
	if got := stringArg(map[string]any{"k": ""}, "k", "x"); got != "x" {
		t.Fatalf("empty string should use default, got %q", got)
	}
}

func TestStringArg_WrongType(t *testing.T) {
	if got := stringArg(map[string]any{"k": 42}, "k", "x"); got != "x" {
		t.Fatalf("non-string type should use default, got %q", got)
	}
}

func TestIntArg_Default(t *testing.T) {
	if got := intArg(nil, "missing", 7); got != 7 {
		t.Fatalf("missing key: %d", got)
	}
}

func TestIntArg_IntValue(t *testing.T) {
	if got := intArg(map[string]any{"k": 42}, "k", 0); got != 42 {
		t.Fatalf("got %d", got)
	}
}

func TestIntArg_FloatCoerces(t *testing.T) {
	// intArg coerces 42.0 -> 42 (it's how the underlying type-switch
	// works after JSON unmarshal). Documented behavior.
	if got := intArg(map[string]any{"k": 42.0}, "k", 0); got != 42 {
		t.Fatalf("expected float 42.0 to coerce to 42, got %d", got)
	}
}

func TestBoolArg_Default(t *testing.T) {
	if got := boolArg(nil, "missing"); got != false {
		t.Fatalf("missing key: %v", got)
	}
}

func TestBoolArg_True(t *testing.T) {
	if got := boolArg(map[string]any{"k": true}, "k"); !got {
		t.Fatal("got false, expected true")
	}
}

func TestBoolArg_False(t *testing.T) {
	if got := boolArg(map[string]any{"k": false}, "k"); got {
		t.Fatal("got true, expected false")
	}
}
