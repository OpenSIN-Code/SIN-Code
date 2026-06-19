// SPDX-License-Identifier: MIT
package agentloop

import (
	"sync"
	"testing"
)

func spec(name, desc string) ToolSpec {
	return ToolSpec{Name: name, Description: desc}
}

func TestToolRetrieverRegisterAndSearch(t *testing.T) {
	r := NewToolRetriever()
	r.Register(spec("sin_edit", "edit a file"), []float32{1, 0, 0})
	r.Register(spec("sin_read", "read a file"), []float32{0.9, 0.1, 0})
	r.Register(spec("sin_test", "run tests"), []float32{0, 0, 1})

	// query embedding overlaps with sin_edit (1.0) and sin_read (~0.99) but
	// is orthogonal to sin_test (0 -> filtered out).
	got := r.Search("edit", []float32{1, 0, 0}, 2)
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].Name != "sin_edit" {
		t.Errorf("top result = %q, want sin_edit", got[0].Name)
	}
	if got[1].Name != "sin_read" {
		t.Errorf("second result = %q, want sin_read", got[1].Name)
	}
}

func TestToolRetrieverCosineSimilarity(t *testing.T) {
	if got := CosineSimilarity([]float32{1, 0}, []float32{1, 0}); got != 1 {
		t.Errorf("identical = %v, want 1", got)
	}
	if got := CosineSimilarity([]float32{1, 0}, []float32{0, 1}); got != 0 {
		t.Errorf("orthogonal = %v, want 0", got)
	}
	if got := CosineSimilarity([]float32{1, 0}, []float32{-1, 0}); got != -1 {
		t.Errorf("opposite = %v, want -1", got)
	}
	if got := CosineSimilarity([]float32{}, []float32{1}); got != 0 {
		t.Errorf("empty = %v, want 0", got)
	}
	if got := CosineSimilarity([]float32{0, 0}, []float32{1, 1}); got != 0 {
		t.Errorf("zero vector = %v, want 0", got)
	}
}

func TestToolRetrieverLazyLoad(t *testing.T) {
	r := NewToolRetriever()
	r.Register(spec("a", "small"), []float32{1, 0})
	r.Register(spec("b", "rich embedding"), []float32{3, 4})
	r.Register(spec("c", "medium"), []float32{2, 0})

	got := r.LazyLoad(2)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	// norm(b)=25 > norm(c)=4 > norm(a)=1
	if got[0].Name != "b" {
		t.Errorf("top = %q, want b", got[0].Name)
	}
	if got[1].Name != "c" {
		t.Errorf("second = %q, want c", got[1].Name)
	}
}

func TestToolRetrieverEmpty(t *testing.T) {
	r := NewToolRetriever()
	if got := r.Search("x", []float32{1}, 5); got != nil {
		t.Errorf("empty search = %v, want nil", got)
	}
	if got := r.LazyLoad(5); got != nil {
		t.Errorf("empty lazyload = %v, want nil", got)
	}
	if got := r.Search("x", []float32{}, 5); got != nil {
		t.Errorf("empty embedding = %v, want nil", got)
	}
	if got := r.Search("x", []float32{1}, 0); got != nil {
		t.Errorf("topK=0 = %v, want nil", got)
	}
}

func TestToolRetrieverTopKLimit(t *testing.T) {
	r := NewToolRetriever()
	for i := 0; i < 5; i++ {
		r.Register(spec(toolName(i), "tool"), []float32{float32(i + 1)})
	}
	got := r.Search("tool", []float32{1}, 3)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3", len(got))
	}
	got = r.LazyLoad(2)
	if len(got) != 2 {
		t.Fatalf("lazyload got %d, want 2", len(got))
	}
}

func TestToolRetrieverTags(t *testing.T) {
	r := NewToolRetriever()
	r.RegisterWithTags(spec("sin_edit", "edit a file"), []string{"filesystem", "write"}, []float32{1, 0})
	r.Register(spec("sin_read", "read a file"), []float32{0.9, 0.1})

	if tags := r.Tags("sin_edit"); len(tags) != 2 {
		t.Fatalf("tags = %v, want 2", tags)
	}
	if tags := r.Tags("sin_read"); tags != nil {
		t.Errorf("untagged = %v, want nil", tags)
	}
	// query "filesystem" boosts sin_edit above the near-tie sin_read
	got := r.Search("filesystem", []float32{1, 0}, 2)
	if len(got) == 0 || got[0].Name != "sin_edit" {
		t.Errorf("tag boost failed: %v", names(got))
	}
}

func TestToolRetrieverUpdateExisting(t *testing.T) {
	r := NewToolRetriever()
	r.Register(spec("dup", "first"), []float32{1, 0})
	r.Register(spec("dup", "second"), []float32{0, 1})
	if r.Count() != 1 {
		t.Fatalf("count = %d, want 1", r.Count())
	}
	got := r.Search("x", []float32{0, 1}, 5)
	if len(got) != 1 || got[0].Description != "second" {
		t.Errorf("update failed: %v", got)
	}
}

func TestToolRetrieverConcurrent(t *testing.T) {
	r := NewToolRetriever()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.RegisterWithTags(spec(toolName(i), "concurrent tool"), []string{"tag"}, []float32{float32(i + 1)})
			_ = r.Search("concurrent", []float32{float32(i + 1)}, 5)
			_ = r.LazyLoad(5)
			_ = r.Count()
		}(i)
	}
	wg.Wait()
	if r.Count() != 50 {
		t.Errorf("count = %d, want 50", r.Count())
	}
}

func toolName(i int) string {
	switch {
	case i < 10:
		return "tool_0" + string(rune('0'+i))
	default:
		return "tool_" + string(rune('0'+i/10)) + string(rune('0'+i%10))
	}
}

func names(specs []ToolSpec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.Name
	}
	return out
}
