package spec

import (
	"testing"
	"time"
)

// FuzzSpecValidation fuzzes spec validation
func FuzzSpecValidation(f *testing.F) {
	testCases := []string{
		"valid spec",
		"",
		"a",
		"very long content that exceeds typical lengths",
		"content with special chars: !@#$%^&*()",
		"content with unicode: 你好世界 🌍",
	}

	for _, tc := range testCases {
		f.Add(tc)
	}

	f.Fuzz(func(t *testing.T, content string) {
		spec := &Spec{
			ID:        "fuzz_spec_001",
			Kind:      SpecKindGoal,
			Title:     "Fuzz Test",
			Content:   content,
			Namespace: "fuzz",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}

		// Should not panic
		_ = spec.Validate()
	})
}

// FuzzCompilerGraph fuzzes compiler with various graph structures
func FuzzCompilerGraph(f *testing.F) {
	f.Add(0)   // empty graph
	f.Add(1)   // single spec
	f.Add(5)   // small graph
	f.Add(10)  // medium graph
	f.Add(50)  // large graph
	f.Add(100) // very large graph

	f.Fuzz(func(t *testing.T, specCount int) {
		if specCount < 0 || specCount > 1000 {
			return
		}

		collection := &SpecCollection{
			Specs: make(map[string]*Spec),
		}

		// Build random graph
		for i := 0; i < specCount; i++ {
			id := string(rune('a' + i%26)) + string(rune(i / 26))
			deps := []string{}

			// Add some random dependencies
			for j := 0; j < i%3; j++ {
				depIdx := (i - j - 1) % specCount
				if depIdx >= 0 && depIdx < i {
					deps = append(deps, string(rune('a'+depIdx%26))+string(rune(depIdx/26)))
				}
			}

			collection.Specs[id] = createTestSpec(id, "Spec "+id, deps)
		}

		// Should not panic
		compiler := NewCompiler(collection)
		_ = compiler.Compile()
	})
}

// FuzzMergeOperation fuzzes merge operations
func FuzzMergeOperation(f *testing.F) {
	f.Add("base", "ours", "theirs")
	f.Add("", "", "")
	f.Add("a", "b", "c")
	f.Add("same", "same", "same")

	f.Fuzz(func(t *testing.T, base, ours, theirs string) {
		baseSpec := &Spec{
			ID:        "base",
			Kind:      SpecKindGoal,
			Title:     base,
			Content:   "Base content",
			Namespace: "fuzz",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}

		oursSpec := &Spec{
			ID:        "ours",
			Kind:      SpecKindGoal,
			Title:     ours,
			Content:   "Ours content",
			Namespace: "fuzz",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}

		theirsSpec := &Spec{
			ID:        "theirs",
			Kind:      SpecKindGoal,
			Title:     theirs,
			Content:   "Theirs content",
			Namespace: "fuzz",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}

		// Should not panic
		_ = ThreeWayMerge(baseSpec, oursSpec, theirsSpec)
	})
}

// TestPropertyBasedValidation tests properties that should always hold
func TestPropertyBasedValidation(t *testing.T) {
	t.Run("validation is idempotent", func(t *testing.T) {
		spec := &Spec{
			ID:        "prop_001",
			Kind:      SpecKindGoal,
			Title:     "Property Test",
			Content:   "Content",
			Namespace: "prop",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}

		err1 := spec.Validate()
		err2 := spec.Validate()

		if (err1 == nil) != (err2 == nil) {
			t.Error("validation should be idempotent")
		}
	})

	t.Run("validation result consistent across calls", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			spec := &Spec{
				ID:        "prop_002",
				Kind:      SpecKindGoal,
				Title:     "Property Test",
				Content:   "Content",
				Namespace: "prop",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			}

			err := spec.Validate()
			if err != nil {
				t.Error("validation should not fail for valid spec")
			}
		}
	})
}

// TestPropertyBasedCompilation tests compiler properties
func TestPropertyBasedCompilation(t *testing.T) {
	t.Run("compilation result is deterministic", func(t *testing.T) {
		collection := &SpecCollection{
			Specs: map[string]*Spec{
				"a": createTestSpec("a", "A", []string{}),
				"b": createTestSpec("b", "B", []string{"a"}),
				"c": createTestSpec("c", "C", []string{"b"}),
			},
		}

		compiler1 := NewCompiler(collection)
		result1 := compiler1.Compile()

		compiler2 := NewCompiler(collection)
		result2 := compiler2.Compile()

		if result1.Successful != result2.Successful {
			t.Error("compilation results should be deterministic")
		}

		if len(result1.Order) != len(result2.Order) {
			t.Error("compilation order length should be deterministic")
		}

		for i, id := range result1.Order {
			if i >= len(result2.Order) || result2.Order[i] != id {
				t.Error("compilation order should be deterministic")
			}
		}
	})

	t.Run("topological sort preserves dependencies", func(t *testing.T) {
		collection := &SpecCollection{
			Specs: map[string]*Spec{
				"a": createTestSpec("a", "A", []string{}),
				"b": createTestSpec("b", "B", []string{"a"}),
				"c": createTestSpec("c", "C", []string{"a", "b"}),
			},
		}

		compiler := NewCompiler(collection)
		result := compiler.Compile()

		if !result.Successful {
			t.Fatalf("compilation failed: %v", result.Errors)
		}

		// Build position map
		pos := make(map[string]int)
		for i, id := range result.Order {
			pos[id] = i
		}

		// Verify all dependencies come before dependents
		for _, spec := range collection.Specs {
			for _, dep := range spec.Dependencies {
				if pos[dep] > pos[spec.ID] {
					t.Errorf("dependency %s should come before %s", dep, spec.ID)
				}
			}
		}
	})
}

// TestPropertyBasedMerge tests merge properties
func TestPropertyBasedMerge(t *testing.T) {
	t.Run("merge with unchanged inputs returns result", func(t *testing.T) {
		base := &Spec{
			ID:        "base",
			Kind:      SpecKindGoal,
			Title:     "Base Title",
			Content:   "Base content",
			Namespace: "base",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}

		result := ThreeWayMerge(base, base, base)

		if result == nil {
			t.Error("merge should return a result")
		}

		if result.Title != "Base Title" {
			t.Error("merged title should be from base")
		}
	})

	t.Run("merge result is valid spec", func(t *testing.T) {
		base := &Spec{
			ID:        "base",
			Kind:      SpecKindGoal,
			Title:     "Base",
			Content:   "Base",
			Namespace: "test",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}

		ours := &Spec{
			ID:        "ours",
			Kind:      SpecKindGoal,
			Title:     "Ours",
			Content:   "Ours",
			Namespace: "test",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}

		theirs := &Spec{
			ID:        "theirs",
			Kind:      SpecKindGoal,
			Title:     "Theirs",
			Content:   "Theirs",
			Namespace: "test",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}

		result := ThreeWayMerge(base, ours, theirs)

		if result != nil {
			err := result.Validate()
			if err != nil {
				t.Errorf("merged result should be valid spec: %v", err)
			}
		}
	})
}
