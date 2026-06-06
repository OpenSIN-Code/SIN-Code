package main

import (
	"context"
	"testing"
)

{% for tool in tools %}
func Test{{ tool | capitalize }}(t *testing.T) {
	result, err := {{ tool | capitalize }}(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["tool"] != "{{ tool }}" {
		t.Errorf("expected tool={{ tool }}, got %v", result["tool"])
	}
	if result["ok"] != true {
		t.Errorf("expected ok=true, got %v", result["ok"])
	}
}
{% endfor %}
