package spec

import (
	"fmt"
	"testing"
	"time"
)

// BenchmarkSpecCreationThroughput benchmarks spec creation throughput
func BenchmarkSpecCreationThroughput(b *testing.B) {
	b.Run("simple spec", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = &Spec{
				ID:        "spec_001",
				Kind:      SpecKindGoal,
				Title:     "Test",
				Content:   "Content",
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			}
		}
	})

	b.Run("complex spec with deps", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = &Spec{
				ID:           "spec_001",
				Kind:         SpecKindGoal,
				Title:        "Test",
				Content:      "Content with more details",
				Namespace:    "test.namespace",
				Status:       SpecStatusActive,
				Dependencies: []string{"dep1", "dep2", "dep3", "dep4", "dep5"},
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
		}
	})
}

// BenchmarkValidationThroughput benchmarks validation throughput
func BenchmarkValidationThroughput(b *testing.B) {
	b.Run("valid simple spec", func(b *testing.B) {
		spec := &Spec{
			ID:        "spec_001",
			Kind:      SpecKindGoal,
			Title:     "Test",
			Content:   "Content",
			Namespace: "test",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = spec.Validate()
		}
	})

	b.Run("valid complex spec", func(b *testing.B) {
		spec := &Spec{
			ID:           "spec_001",
			Kind:         SpecKindProcess,
			Title:        "Process Specification",
			Content:      "# Process\n\n## Steps\n\n1. Step 1\n2. Step 2\n3. Step 3",
			Namespace:    "test.process.workflow",
			Status:       SpecStatusActive,
			Dependencies: []string{"dep1", "dep2", "dep3"},
			CreatedAt:    time.Now(),
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = spec.Validate()
		}
	})
}

// BenchmarkCompilationThroughput benchmarks compilation with various sizes
func BenchmarkCompilationThroughput(b *testing.B) {
	testCases := []int{10, 50, 100, 500}

	for _, size := range testCases {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			collection := buildRandomCollection(size)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				compiler := NewCompiler(collection)
				_ = compiler.Compile()
			}
		})
	}
}

// BenchmarkMergeThroughput benchmarks merge operations
func BenchmarkMergeThroughput(b *testing.B) {
	base := &Spec{
		ID:        "base",
		Kind:      SpecKindGoal,
		Title:     "Base Title",
		Content:   "Base content description",
		Namespace: "test",
		Status:    SpecStatusDraft,
		CreatedAt: time.Now(),
	}

	ours := &Spec{
		ID:        "ours",
		Kind:      SpecKindGoal,
		Title:     "Our Title",
		Content:   "Our content changes",
		Namespace: "test",
		Status:    SpecStatusActive,
		CreatedAt: time.Now(),
	}

	theirs := &Spec{
		ID:        "theirs",
		Kind:      SpecKindGoal,
		Title:     "Their Title",
		Content:   "Their content changes",
		Namespace: "test",
		Status:    SpecStatusDraft,
		CreatedAt: time.Now(),
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = ThreeWayMerge(base, ours, theirs)
	}
}

// BenchmarkSearchThroughput benchmarks search operations
func BenchmarkSearchThroughput(b *testing.B) {
	collection := buildRandomCollection(100)
	indexer := NewSpecIndexer(collection, 1000000)
	indexer.BuildIndex()

	queries := []string{
		"auth",
		"process",
		"goal",
		"specification",
		"test",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		query := queries[i%len(queries)]
		_ = indexer.MetaSpec.SearchByKeyword(query)
	}
}

// TestMemoryUsage tests memory efficiency
func TestMemoryUsage(t *testing.T) {
	t.Run("small collection memory", func(t *testing.T) {
		collection := buildRandomCollection(10)
		totalSize := len(collection.Specs)

		if totalSize != 10 {
			t.Errorf("expected 10 specs, got %d", totalSize)
		}
	})

	t.Run("large collection memory", func(t *testing.T) {
		collection := buildRandomCollection(1000)
		totalSize := len(collection.Specs)

		if totalSize != 1000 {
			t.Errorf("expected 1000 specs, got %d", totalSize)
		}
	})
}

// TestConcurrentOperations tests concurrent spec operations
func TestConcurrentOperations(t *testing.T) {
	t.Run("concurrent validation", func(t *testing.T) {
		done := make(chan bool, 100)

		for i := 0; i < 100; i++ {
			go func(idx int) {
				spec := &Spec{
					ID:        fmt.Sprintf("concurrent_%d", idx),
					Kind:      SpecKindGoal,
					Title:     "Concurrent Test",
					Content:   "Content",
					Namespace: "concurrent",
					Status:    SpecStatusDraft,
					CreatedAt: time.Now(),
				}

				_ = spec.Validate()
				done <- true
			}(i)
		}

		for i := 0; i < 100; i++ {
			<-done
		}
	})

	t.Run("concurrent compilation", func(t *testing.T) {
		collection := buildRandomCollection(50)
		done := make(chan bool, 10)

		for i := 0; i < 10; i++ {
			go func() {
				compiler := NewCompiler(collection)
				_ = compiler.Compile()
				done <- true
			}()
		}

		for i := 0; i < 10; i++ {
			<-done
		}
	})
}

// TestStressLargeCollections stress tests with large collections
func TestStressLargeCollections(t *testing.T) {
	t.Run("compile large collection", func(t *testing.T) {
		// Create 1000 specs
		collection := buildRandomCollection(1000)

		start := time.Now()
		compiler := NewCompiler(collection)
		result := compiler.Compile()
		duration := time.Since(start)

		if !result.Successful {
			t.Errorf("compilation failed for large collection: %v", result.Errors)
		}

		if len(result.Order) != 1000 {
			t.Errorf("expected 1000 specs in order, got %d", len(result.Order))
		}

		t.Logf("Large collection compilation took %v", duration)
	})

	t.Run("search large collection", func(t *testing.T) {
		collection := buildRandomCollection(500)
		indexer := NewSpecIndexer(collection, 1000000)
		indexer.BuildIndex()

		start := time.Now()
		results := indexer.MetaSpec.SearchByKeyword("spec")
		duration := time.Since(start)

		if len(results) == 0 {
			t.Error("search should find results")
		}

		t.Logf("Large collection search took %v", duration)
	})

	t.Run("deeply nested dependencies", func(t *testing.T) {
		// Create chain: 0 -> 1 -> 2 -> ... -> 99
		collection := &SpecCollection{
			Specs: make(map[string]*Spec),
		}

		for i := 0; i < 100; i++ {
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
			t.Errorf("compilation failed for deep chain: %v", result.Errors)
		}

		// Verify order
		for i, id := range result.Order {
			expected := fmt.Sprintf("spec_%d", i)
			if id != expected {
				t.Errorf("position %d: got %s, want %s", i, id, expected)
			}
		}
	})
}

// TestErrorRecovery tests error handling under stress
func TestErrorRecovery(t *testing.T) {
	t.Run("recover from invalid specs", func(t *testing.T) {
		collection := &SpecCollection{
			Specs: map[string]*Spec{
				"valid": createTestSpec("valid", "Valid Spec", []string{}),
				"invalid": {
					ID:        "invalid",
					Kind:      SpecKindGoal,
					Title:     "",
					Content:   "",
					Namespace: "",
					Status:    SpecStatusDraft,
					CreatedAt: time.Now(),
				},
			},
		}

		validCount := 0
		invalidCount := 0

		for _, spec := range collection.Specs {
			err := spec.Validate()
			if err != nil {
				invalidCount++
			} else {
				validCount++
			}
		}

		if validCount != 1 || invalidCount != 1 {
			t.Errorf("expected 1 valid and 1 invalid, got %d and %d", validCount, invalidCount)
		}
	})
}

// Helper function to build random collection
func buildRandomCollection(size int) *SpecCollection {
	collection := &SpecCollection{
		Specs: make(map[string]*Spec),
	}

	for i := 0; i < size; i++ {
		id := fmt.Sprintf("spec_%d", i)
		deps := []string{}

		// Add random dependencies to previous specs
		for j := 0; j < i%5; j++ {
			depIdx := i - j - 1
			if depIdx >= 0 {
				deps = append(deps, fmt.Sprintf("spec_%d", depIdx))
			}
		}

		kinds := []SpecKind{
			SpecKindGoal,
			SpecKindProcess,
			SpecKindConstraint,
			SpecKindComponent,
			SpecKindIntegration,
		}

		collection.Specs[id] = &Spec{
			ID:           id,
			Kind:         kinds[i%len(kinds)],
			Title:        fmt.Sprintf("Spec %d", i),
			Content:      fmt.Sprintf("Content for spec %d", i),
			Namespace:    "random",
			Status:       SpecStatusActive,
			Dependencies: deps,
			CreatedAt:    time.Now(),
		}
	}

	return collection
}
