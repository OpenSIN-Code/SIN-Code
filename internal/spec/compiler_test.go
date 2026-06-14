package spec

import (
	"fmt"
	"testing"
	"time"
)

// TestCompilerSimpleGraph tests compilation of simple graphs
func TestCompilerSimpleGraph(t *testing.T) {
	collection := &SpecCollection{
		Specs: map[string]*Spec{
			"spec_1": {
				ID:        "spec_1",
				Kind:      SpecKindGoal,
				Title:     "Goal 1",
				Content:   "Goal 1 content",
				Namespace: "test",
				Status:    SpecStatusActive,
				CreatedAt: time.Now(),
			},
			"spec_2": {
				ID:           "spec_2",
				Kind:         SpecKindGoal,
				Title:        "Goal 2",
				Content:      "Goal 2 content",
				Namespace:    "test",
				Status:       SpecStatusActive,
				Dependencies: []string{"spec_1"},
				CreatedAt:    time.Now(),
			},
		},
	}

	compiler := NewCompiler(collection)
	result := compiler.Compile()

	if !result.Successful {
		t.Errorf("Compile() unsuccessful: %v", result.Errors)
	}

	if len(result.Order) != 2 {
		t.Errorf("expected 2 specs in order, got %d", len(result.Order))
	}

	if result.Order[0] != "spec_1" || result.Order[1] != "spec_2" {
		t.Errorf("unexpected topological order: %v", result.Order)
	}
}

// TestCompilerDiamondDependency tests diamond dependency graph
func TestCompilerDiamondDependency(t *testing.T) {
	collection := &SpecCollection{
		Specs: map[string]*Spec{
			"base": createTestSpec("base", "Base", []string{}),
			"left": createTestSpec("left", "Left", []string{"base"}),
			"right": createTestSpec("right", "Right", []string{"base"}),
			"top": createTestSpec("top", "Top", []string{"left", "right"}),
		},
	}

	compiler := NewCompiler(collection)
	result := compiler.Compile()

	if !result.Successful {
		t.Errorf("Compile() unsuccessful: %v", result.Errors)
	}

	if len(result.Order) != 4 {
		t.Errorf("expected 4 specs in order, got %d", len(result.Order))
	}

	// base must come first
	if result.Order[0] != "base" {
		t.Errorf("base should be first, got %v", result.Order[0])
	}

	// top must come last
	if result.Order[3] != "top" {
		t.Errorf("top should be last, got %v", result.Order[3])
	}
}

// TestCompilerCycleDetection tests cycle detection
func TestCompilerCycleDetection(t *testing.T) {
	tests := []struct {
		name      string
		specs     map[string][]string
		wantCycle bool
	}{
		{
			name: "no cycle",
			specs: map[string][]string{
				"a": {},
				"b": {"a"},
				"c": {"a", "b"},
			},
			wantCycle: false,
		},
		{
			name: "self cycle",
			specs: map[string][]string{
				"a": {"a"},
			},
			wantCycle: true,
		},
		{
			name: "two cycle",
			specs: map[string][]string{
				"a": {"b"},
				"b": {"a"},
			},
			wantCycle: true,
		},
		{
			name: "three cycle",
			specs: map[string][]string{
				"a": {"b"},
				"b": {"c"},
				"c": {"a"},
			},
			wantCycle: true,
		},
		{
			name: "complex graph no cycle",
			specs: map[string][]string{
				"a": {},
				"b": {"a"},
				"c": {"a"},
				"d": {"b", "c"},
				"e": {"d"},
			},
			wantCycle: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collection := buildSpecCollection(tt.specs)
			compiler := NewCompiler(collection)
			result := compiler.Compile()

			if result.Successful == tt.wantCycle {
				t.Errorf("expected cycle=%v, got success=%v", tt.wantCycle, result.Successful)
			}
		})
	}
}

// TestCompilerMetadata tests metadata computation
func TestCompilerMetadata(t *testing.T) {
	collection := &SpecCollection{
		Specs: map[string]*Spec{
			"spec_1": createTestSpec("spec_1", "Spec 1", []string{}),
			"spec_2": createTestSpec("spec_2", "Spec 2", []string{"spec_1"}),
			"spec_3": createTestSpec("spec_3", "Spec 3", []string{"spec_2"}),
		},
	}

	compiler := NewCompiler(collection)
	result := compiler.Compile()

	if !result.Successful {
		t.Fatalf("Compile failed: %v", result.Errors)
	}

	// Verify depths
	if result.Depths["spec_1"] != 0 {
		t.Errorf("spec_1 depth: got %d, want 0", result.Depths["spec_1"])
	}
	if result.Depths["spec_2"] != 1 {
		t.Errorf("spec_2 depth: got %d, want 1", result.Depths["spec_2"])
	}
	if result.Depths["spec_3"] != 2 {
		t.Errorf("spec_3 depth: got %d, want 2", result.Depths["spec_3"])
	}
}

// TestCompilerEmptyCollection tests compilation of empty collection
func TestCompilerEmptyCollection(t *testing.T) {
	collection := &SpecCollection{
		Specs: make(map[string]*Spec),
	}

	compiler := NewCompiler(collection)
	result := compiler.Compile()

	if !result.Successful {
		t.Errorf("empty collection compilation should succeed")
	}

	if len(result.Order) != 0 {
		t.Errorf("empty collection should have empty order, got %d", len(result.Order))
	}
}

// TestCompilerMissingDependency tests missing dependency handling
func TestCompilerMissingDependency(t *testing.T) {
	collection := &SpecCollection{
		Specs: map[string]*Spec{
			"spec_1": {
				ID:           "spec_1",
				Kind:         SpecKindGoal,
				Title:        "Spec 1",
				Content:      "Content",
				Namespace:    "test",
				Status:       SpecStatusActive,
				Dependencies: []string{"nonexistent"},
				CreatedAt:    time.Now(),
			},
		},
	}

	compiler := NewCompiler(collection)
	result := compiler.Compile()

	// Should fail due to missing dependency
	if result.Successful {
		t.Error("compilation should fail for missing dependency")
	}

	if len(result.Errors) == 0 {
		t.Error("should have errors for missing dependency")
	}
}

// TestCompilerLargeGraph tests compilation of large graph
func TestCompilerLargeGraph(t *testing.T) {
	// Create a large chain: 0 -> 1 -> 2 -> ... -> 99
	specs := make(map[string]*Spec)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("spec_%d", i)
		deps := []string{}
		if i > 0 {
			deps = []string{fmt.Sprintf("spec_%d", i-1)}
		}
		specs[id] = createTestSpec(id, fmt.Sprintf("Spec %d", i), deps)
	}

	collection := &SpecCollection{Specs: specs}
	compiler := NewCompiler(collection)
	result := compiler.Compile()

	if !result.Successful {
		t.Errorf("large graph compilation failed: %v", result.Errors)
	}

	if len(result.Order) != 100 {
		t.Errorf("expected 100 specs in order, got %d", len(result.Order))
	}

	// Verify order is correct
	for i, id := range result.Order {
		expected := fmt.Sprintf("spec_%d", i)
		if id != expected {
			t.Errorf("position %d: got %s, want %s", i, id, expected)
		}
	}
}

// TestCompilerDeepDependency tests deeply nested dependencies
func TestCompilerDeepDependency(t *testing.T) {
	// Create chain: spec_0 -> spec_1 -> spec_2 -> ... -> spec_10
	depth := 10
	collection := &SpecCollection{
		Specs: make(map[string]*Spec),
	}

	for i := 0; i <= depth; i++ {
		id := fmt.Sprintf("spec_%d", i)
		deps := []string{}
		if i > 0 {
			deps = []string{fmt.Sprintf("spec_%d", i-1)}
		}
		collection.Specs[id] = createTestSpec(id, fmt.Sprintf("Spec %d", i), deps)
	}

	compiler := NewCompiler(collection)
	result := compiler.Compile()

	if !result.Successful {
		t.Errorf("deep chain compilation failed: %v", result.Errors)
	}

	// Verify depths
	for i := 0; i <= depth; i++ {
		id := fmt.Sprintf("spec_%d", i)
		if result.Depths[id] != i {
			t.Errorf("spec_%d depth: got %d, want %d", i, result.Depths[id], i)
		}
	}
}

// BenchmarkCompilation benchmarks graph compilation
func BenchmarkCompilation(b *testing.B) {
	specs := make(map[string]*Spec)
	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("spec_%d", i)
		deps := []string{}
		for j := 0; j < i%5; j++ {
			deps = append(deps, fmt.Sprintf("spec_%d", j))
		}
		specs[id] = createTestSpec(id, fmt.Sprintf("Spec %d", i), deps)
	}

	collection := &SpecCollection{Specs: specs}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiler := NewCompiler(collection)
		_ = compiler.Compile()
	}
}

// BenchmarkCompilationLarge benchmarks large graph compilation
func BenchmarkCompilationLarge(b *testing.B) {
	specs := make(map[string]*Spec)
	for i := 0; i < 500; i++ {
		id := fmt.Sprintf("spec_%d", i)
		deps := []string{}
		for j := 0; j < i%10; j++ {
			deps = append(deps, fmt.Sprintf("spec_%d", j))
		}
		specs[id] = createTestSpec(id, fmt.Sprintf("Spec %d", i), deps)
	}

	collection := &SpecCollection{Specs: specs}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiler := NewCompiler(collection)
		_ = compiler.Compile()
	}
}

// BenchmarkCycleDetection benchmarks cycle detection
func BenchmarkCycleDetection(b *testing.B) {
	// Create graph without cycles
	specs := make(map[string]*Spec)
	for i := 0; i < 100; i++ {
		id := fmt.Sprintf("spec_%d", i)
		deps := []string{}
		for j := 0; j < i%10; j++ {
			deps = append(deps, fmt.Sprintf("spec_%d", j))
		}
		specs[id] = createTestSpec(id, fmt.Sprintf("Spec %d", i), deps)
	}

	collection := &SpecCollection{Specs: specs}
	compiler := NewCompiler(collection)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = compiler.Compile()
	}
}

// Helper function to create test spec
func createTestSpec(id, title string, deps []string) *Spec {
	return &Spec{
		ID:           id,
		Kind:         SpecKindGoal,
		Title:        title,
		Content:      "Test content",
		Namespace:    "test",
		Status:       SpecStatusActive,
		Dependencies: deps,
		CreatedAt:    time.Now(),
	}
}

// Helper function to build collection from spec map
func buildSpecCollection(specMap map[string][]string) *SpecCollection {
	collection := &SpecCollection{
		Specs: make(map[string]*Spec),
	}

	for id, deps := range specMap {
		collection.Specs[id] = createTestSpec(id, "Test "+id, deps)
	}

	return collection
}
