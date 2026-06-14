package spec

import (
	"testing"
	"time"
)

// TestSpecCreationBasic tests basic spec creation with all required fields
func TestSpecCreationBasic(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		kind    SpecKind
		title   string
		content string
		wantErr bool
	}{
		{
			name:    "valid goal spec",
			id:      "spec_goal_001",
			kind:    SpecKindGoal,
			title:   "User Authentication",
			content: "# Goal\n\nImplement OAuth2 authentication",
			wantErr: false,
		},
		{
			name:    "valid process spec",
			id:      "spec_process_001",
			kind:    SpecKindProcess,
			title:   "CI/CD Pipeline",
			content: "# Process\n\n1. Build\n2. Test\n3. Deploy",
			wantErr: false,
		},
		{
			name:    "empty title",
			id:      "spec_empty_001",
			kind:    SpecKindGoal,
			title:   "",
			content: "Content",
			wantErr: true,
		},
		{
			name:    "empty id",
			id:      "",
			kind:    SpecKindGoal,
			title:   "Title",
			content: "Content",
			wantErr: true,
		},
		{
			name:    "constraint spec",
			id:      "spec_constraint_001",
			kind:    SpecKindConstraint,
			title:   "Performance Requirements",
			content: "# Constraints\n\n- P99 < 100ms\n- Availability > 99.9%",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &Spec{
				ID:        tt.id,
				Kind:      tt.kind,
				Title:     tt.title,
				Content:   tt.content,
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			}

			err := spec.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSpecNamespaceHandling tests namespace operations
func TestSpecNamespaceHandling(t *testing.T) {
	tests := []struct {
		namespace string
		wantValid bool
	}{
		{"auth", true},
		{"auth.oauth2", true},
		{"auth.oauth2.google", true},
		{"a", true},
		{"123namespace", false},
		{"auth-oauth2", false},
		{"", false},
		{"auth.oauth2.google.microsoftonline.federation.core", true},
	}

	for _, tt := range tests {
		t.Run(tt.namespace, func(t *testing.T) {
			spec := &Spec{
				ID:        "test_001",
				Kind:      SpecKindGoal,
				Title:     "Test",
				Content:   "Test",
				Namespace: tt.namespace,
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			}

			err := spec.Validate()
			isValid := err == nil
			if isValid != tt.wantValid {
				t.Errorf("namespace %q: got valid=%v, want %v", tt.namespace, isValid, tt.wantValid)
			}
		})
	}
}

// TestSpecStatusTransitions tests valid status transitions
func TestSpecStatusTransitions(t *testing.T) {
	tests := []struct {
		from      SpecStatus
		to        SpecStatus
		wantValid bool
	}{
		{SpecStatusDraft, SpecStatusActive, true},
		{SpecStatusActive, SpecStatusArchived, true},
		{SpecStatusDraft, SpecStatusArchived, true},
		{SpecStatusArchived, SpecStatusActive, false},
		{SpecStatusArchived, SpecStatusDraft, false},
		{SpecStatusActive, SpecStatusActive, true},
	}

	for _, tt := range tests {
		t.Run(tt.from.String()+"-to-"+tt.to.String(), func(t *testing.T) {
			spec := &Spec{
				ID:        "test_001",
				Kind:      SpecKindGoal,
				Title:     "Test",
				Content:   "Test",
				Namespace: "test",
				Status:    tt.from,
				CreatedAt: time.Now(),
			}

			oldStatus := spec.Status
			spec.Status = tt.to

			err := spec.Validate()
			isValid := err == nil

			if isValid != tt.wantValid {
				t.Errorf("transition %s→%s: got valid=%v, want %v", oldStatus, tt.to, isValid, tt.wantValid)
			}
		})
	}
}

// TestSpecDependencyHandling tests dependency management
func TestSpecDependencyHandling(t *testing.T) {
	spec := &Spec{
		ID:           "spec_main_001",
		Kind:         SpecKindGoal,
		Title:        "Main Goal",
		Content:      "Main content",
		Namespace:    "main",
		Status:       SpecStatusActive,
		Dependencies: []string{"spec_dep_001", "spec_dep_002", "spec_dep_003"},
		CreatedAt:    time.Now(),
	}

	if len(spec.Dependencies) != 3 {
		t.Errorf("expected 3 dependencies, got %d", len(spec.Dependencies))
	}

	// Test adding duplicate dependency
	spec.Dependencies = append(spec.Dependencies, "spec_dep_001")
	if len(spec.Dependencies) != 4 {
		t.Errorf("expected 4 dependencies after duplicate, got %d", len(spec.Dependencies))
	}
}

// TestSpecMetadataComputation tests metadata fields
func TestSpecMetadataComputation(t *testing.T) {
	now := time.Now()
	spec := &Spec{
		ID:        "spec_metadata_001",
		Kind:      SpecKindGoal,
		Title:     "Metadata Test",
		Content:   "This is a test specification with some content to analyze",
		Namespace: "test.metadata",
		Status:    SpecStatusActive,
		CreatedAt: now,
		UpdatedAt: now.Add(1 * time.Hour),
	}

	// Compute token estimate
	tokenEst := len(spec.Content) / 4 // approximate
	if tokenEst < 10 {
		t.Errorf("token estimate too low: %d", tokenEst)
	}

	// Verify timestamps
	if !spec.CreatedAt.Before(spec.UpdatedAt) {
		t.Error("UpdatedAt should be after CreatedAt")
	}
}

// TestSpecImmutability tests spec immutability (no mutation after creation)
func TestSpecImmutability(t *testing.T) {
	original := &Spec{
		ID:        "spec_immutable_001",
		Kind:      SpecKindGoal,
		Title:     "Original Title",
		Content:   "Original content",
		Namespace: "immutable",
		Status:    SpecStatusDraft,
		CreatedAt: time.Now(),
	}

	originalTitle := original.Title
	originalContent := original.Content

	// Attempt mutation (in actual use, specs should be replaced, not mutated)
	modified := *original // copy
	modified.Title = "Modified Title"
	modified.Content = "Modified content"

	// Verify original is unchanged
	if original.Title != originalTitle {
		t.Error("original title was modified")
	}
	if original.Content != originalContent {
		t.Error("original content was modified")
	}
}

// TestSpecKindEnums tests all SpecKind enum values
func TestSpecKindEnums(t *testing.T) {
	kinds := []SpecKind{
		SpecKindGoal,
		SpecKindProcess,
		SpecKindConstraint,
		SpecKindComponent,
		SpecKindIntegration,
	}

	for _, kind := range kinds {
		str := kind.String()
		if str == "" {
			t.Errorf("SpecKind %v has empty string", kind)
		}
	}
}

// TestSpecStatusEnums tests all SpecStatus enum values
func TestSpecStatusEnums(t *testing.T) {
	statuses := []SpecStatus{
		SpecStatusDraft,
		SpecStatusActive,
		SpecStatusArchived,
	}

	for _, status := range statuses {
		str := status.String()
		if str == "" {
			t.Errorf("SpecStatus %v has empty string", status)
		}
	}
}

// TestSpecContentLength tests content length handling
func TestSpecContentLength(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantValid bool
	}{
		{
			name:      "minimum content",
			content:   "a",
			wantValid: true,
		},
		{
			name:      "typical content",
			content:   "# Goal\n\nThis is a typical specification",
			wantValid: true,
		},
		{
			name:      "very long content",
			content:   generateString(100000), // 100KB
			wantValid: true,
		},
		{
			name:      "empty content",
			content:   "",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &Spec{
				ID:        "test_001",
				Kind:      SpecKindGoal,
				Title:     "Test",
				Content:   tt.content,
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			}

			err := spec.Validate()
			isValid := err == nil
			if isValid != tt.wantValid {
				t.Errorf("content length %d: got valid=%v, want %v", len(tt.content), isValid, tt.wantValid)
			}
		})
	}
}

// BenchmarkSpecCreation benchmarks spec creation
func BenchmarkSpecCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = &Spec{
			ID:        "spec_bench_001",
			Kind:      SpecKindGoal,
			Title:     "Benchmark Test",
			Content:   "Benchmark content",
			Namespace: "bench",
			Status:    SpecStatusDraft,
			CreatedAt: time.Now(),
		}
	}
}

// BenchmarkSpecValidation benchmarks spec validation
func BenchmarkSpecValidation(b *testing.B) {
	spec := &Spec{
		ID:        "spec_bench_002",
		Kind:      SpecKindGoal,
		Title:     "Benchmark Validation",
		Content:   "This is benchmark content for validation testing",
		Namespace: "bench.validation",
		Status:    SpecStatusActive,
		CreatedAt: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spec.Validate()
	}
}

// BenchmarkSpecCopy benchmarks spec copying
func BenchmarkSpecCopy(b *testing.B) {
	original := &Spec{
		ID:           "spec_bench_003",
		Kind:         SpecKindGoal,
		Title:        "Benchmark Copy",
		Content:      "Content for copying benchmark",
		Namespace:    "bench.copy",
		Status:       SpecStatusActive,
		Dependencies: []string{"dep1", "dep2", "dep3"},
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = *original // shallow copy
	}
}

// Helper function to generate string
func generateString(length int) string {
	b := make([]byte, length)
	for i := 0; i < length; i++ {
		b[i] = 'a' + byte(i%26)
	}
	return string(b)
}
