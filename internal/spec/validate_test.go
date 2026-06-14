package spec

import (
	"strings"
	"testing"
	"time"
)

// TestValidatorRequiredFields tests required field validation
func TestValidatorRequiredFields(t *testing.T) {
	tests := []struct {
		name      string
		spec      *Spec
		wantError bool
	}{
		{
			name: "all fields present",
			spec: &Spec{
				ID:        "spec_001",
				Kind:      SpecKindGoal,
				Title:     "Test Goal",
				Content:   "Test content",
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			},
			wantError: false,
		},
		{
			name: "missing ID",
			spec: &Spec{
				ID:        "",
				Kind:      SpecKindGoal,
				Title:     "Test Goal",
				Content:   "Test content",
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			},
			wantError: true,
		},
		{
			name: "missing Title",
			spec: &Spec{
				ID:        "spec_001",
				Kind:      SpecKindGoal,
				Title:     "",
				Content:   "Test content",
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			},
			wantError: true,
		},
		{
			name: "missing Content",
			spec: &Spec{
				ID:        "spec_001",
				Kind:      SpecKindGoal,
				Title:     "Test Goal",
				Content:   "",
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			},
			wantError: true,
		},
		{
			name: "missing Namespace",
			spec: &Spec{
				ID:        "spec_001",
				Kind:      SpecKindGoal,
				Title:     "Test Goal",
				Content:   "Test content",
				Namespace: "",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spec.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestValidatorMarkdownFormat tests markdown validation
func TestValidatorMarkdownFormat(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError bool
	}{
		{
			name:      "valid markdown with heading",
			content:   "# Goal\n\nThis is a goal",
			wantError: false,
		},
		{
			name:      "valid markdown with multiple headings",
			content:   "# Section 1\n\nContent\n\n## Subsection\n\nMore content",
			wantError: false,
		},
		{
			name:      "valid markdown with list",
			content:   "# Goal\n\n- Item 1\n- Item 2\n- Item 3",
			wantError: false,
		},
		{
			name:      "valid markdown with code block",
			content:   "# Goal\n\n```go\nfunc main() {}\n```",
			wantError: false,
		},
		{
			name:      "plain text",
			content:   "Just plain text without markdown",
			wantError: false,
		},
		{
			name:      "markdown with special characters",
			content:   "# Goal\n\nContent with **bold** and *italic* and `code`",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &Spec{
				ID:        "spec_001",
				Kind:      SpecKindGoal,
				Title:     "Test",
				Content:   tt.content,
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			}

			err := spec.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestValidatorIDFormat tests ID format validation
func TestValidatorIDFormat(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantError bool
	}{
		{
			name:      "valid ID",
			id:        "spec_auth_001",
			wantError: false,
		},
		{
			name:      "ID with uppercase",
			id:        "Spec_Auth_001",
			wantError: false,
		},
		{
			name:      "ID with numbers",
			id:        "spec_123_456",
			wantError: false,
		},
		{
			name:      "ID with hyphens",
			id:        "spec-auth-001",
			wantError: false,
		},
		{
			name:      "very long ID",
			id:        strings.Repeat("a", 1000),
			wantError: false,
		},
		{
			name:      "ID with spaces",
			id:        "spec auth 001",
			wantError: true,
		},
		{
			name:      "ID with special chars",
			id:        "spec@auth#001",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &Spec{
				ID:        tt.id,
				Kind:      SpecKindGoal,
				Title:     "Test",
				Content:   "Test content",
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			}

			err := spec.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestValidatorDependencies tests dependency validation
func TestValidatorDependencies(t *testing.T) {
	tests := []struct {
		name         string
		dependencies []string
		wantError    bool
	}{
		{
			name:         "no dependencies",
			dependencies: []string{},
			wantError:    false,
		},
		{
			name:         "single dependency",
			dependencies: []string{"spec_dep_001"},
			wantError:    false,
		},
		{
			name:         "multiple dependencies",
			dependencies: []string{"spec_dep_001", "spec_dep_002", "spec_dep_003"},
			wantError:    false,
		},
		{
			name:         "many dependencies",
			dependencies: genDependencies(100),
			wantError:    false,
		},
		{
			name:         "empty string dependency",
			dependencies: []string{"spec_dep_001", "", "spec_dep_003"},
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &Spec{
				ID:           "spec_001",
				Kind:         SpecKindGoal,
				Title:        "Test",
				Content:      "Test content",
				Namespace:    "test",
				Status:       SpecStatusDraft,
				Dependencies: tt.dependencies,
				CreatedAt:    time.Now(),
			}

			err := spec.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestValidatorNamespaceFormat tests namespace format validation
func TestValidatorNamespaceFormat(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantError bool
	}{
		{
			name:      "single level",
			namespace: "auth",
			wantError: false,
		},
		{
			name:      "two levels",
			namespace: "auth.oauth2",
			wantError: false,
		},
		{
			name:      "three levels",
			namespace: "auth.oauth2.google",
			wantError: false,
		},
		{
			name:      "many levels",
			namespace: "a.b.c.d.e.f.g.h.i.j",
			wantError: false,
		},
		{
			name:      "single character",
			namespace: "a",
			wantError: false,
		},
		{
			name:      "with numbers",
			namespace: "auth.oauth2",
			wantError: false,
		},
		{
			name:      "namespace with hyphen",
			namespace: "auth-oauth",
			wantError: true,
		},
		{
			name:      "namespace with space",
			namespace: "auth oauth",
			wantError: true,
		},
		{
			name:      "empty namespace",
			namespace: "",
			wantError: true,
		},
		{
			name:      "trailing dot",
			namespace: "auth.oauth.",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &Spec{
				ID:        "spec_001",
				Kind:      SpecKindGoal,
				Title:     "Test",
				Content:   "Test content",
				Namespace: tt.namespace,
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			}

			err := spec.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestValidatorSpecKind tests SpecKind validation
func TestValidatorSpecKind(t *testing.T) {
	validKinds := []SpecKind{
		SpecKindGoal,
		SpecKindProcess,
		SpecKindConstraint,
		SpecKindComponent,
		SpecKindIntegration,
	}

	for _, kind := range validKinds {
		t.Run(kind.String(), func(t *testing.T) {
			spec := &Spec{
				ID:        "spec_001",
				Kind:      kind,
				Title:     "Test",
				Content:   "Test content",
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: time.Now(),
			}

			err := spec.Validate()
			if err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

// TestValidatorSpecStatus tests SpecStatus validation
func TestValidatorSpecStatus(t *testing.T) {
	validStatuses := []SpecStatus{
		SpecStatusDraft,
		SpecStatusActive,
		SpecStatusArchived,
	}

	for _, status := range validStatuses {
		t.Run(status.String(), func(t *testing.T) {
			spec := &Spec{
				ID:        "spec_001",
				Kind:      SpecKindGoal,
				Title:     "Test",
				Content:   "Test content",
				Namespace: "test",
				Status:    status,
				CreatedAt: time.Now(),
			}

			err := spec.Validate()
			if err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

// TestValidatorTimestamps tests timestamp validation
func TestValidatorTimestamps(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		createdAt time.Time
		updatedAt time.Time
		wantError bool
	}{
		{
			name:      "same creation and update",
			createdAt: now,
			updatedAt: now,
			wantError: false,
		},
		{
			name:      "update after creation",
			createdAt: now,
			updatedAt: now.Add(1 * time.Hour),
			wantError: false,
		},
		{
			name:      "update before creation",
			createdAt: now,
			updatedAt: now.Add(-1 * time.Hour),
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &Spec{
				ID:        "spec_001",
				Kind:      SpecKindGoal,
				Title:     "Test",
				Content:   "Test content",
				Namespace: "test",
				Status:    SpecStatusDraft,
				CreatedAt: tt.createdAt,
				UpdatedAt: tt.updatedAt,
			}

			err := spec.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// BenchmarkValidation benchmarks validation performance
func BenchmarkValidation(b *testing.B) {
	spec := &Spec{
		ID:           "spec_bench_001",
		Kind:         SpecKindGoal,
		Title:        "Benchmark Validation",
		Content:      "This is a specification with various markdown content\n\n# Section 1\n\nContent here\n\n## Subsection\n\nMore content",
		Namespace:    "bench.validation",
		Status:       SpecStatusActive,
		Dependencies: []string{"dep1", "dep2", "dep3"},
		CreatedAt:    time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spec.Validate()
	}
}

// BenchmarkValidationLargeContent benchmarks validation with large content
func BenchmarkValidationLargeContent(b *testing.B) {
	spec := &Spec{
		ID:           "spec_bench_002",
		Kind:         SpecKindGoal,
		Title:        "Benchmark Large Content",
		Content:      generateString(50000), // 50KB
		Namespace:    "bench.large",
		Status:       SpecStatusActive,
		Dependencies: []string{"dep1", "dep2", "dep3", "dep4", "dep5"},
		CreatedAt:    time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spec.Validate()
	}
}

// BenchmarkValidationManyDependencies benchmarks validation with many dependencies
func BenchmarkValidationManyDependencies(b *testing.B) {
	spec := &Spec{
		ID:           "spec_bench_003",
		Kind:         SpecKindGoal,
		Title:        "Benchmark Many Dependencies",
		Content:      "Content",
		Namespace:    "bench.deps",
		Status:       SpecStatusActive,
		Dependencies: genDependencies(1000),
		CreatedAt:    time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = spec.Validate()
	}
}

// Helper function to generate dependencies
func genDependencies(count int) []string {
	deps := make([]string, count)
	for i := 0; i < count; i++ {
		deps[i] = "spec_dep_" + string(rune(i))
	}
	return deps
}
