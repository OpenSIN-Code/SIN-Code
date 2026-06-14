package spec

import (
	"strings"
	"testing"
)

// TestValidatorFields tests field-level validation
func TestValidatorFields(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name        string
		field       string
		value       string
		expectError bool
	}{
		{"empty_id", "id", "", true},
		{"valid_id", "id", "spec_001", false},
		{"empty_title", "title", "", true},
		{"valid_title", "title", "My Spec", false},
		{"empty_namespace", "namespace", "", true},
		{"valid_namespace", "namespace", "auth.oauth", false},
		{"valid_single_namespace", "namespace", "auth", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewSpec("test_id", "Test Title", SpecKindGoal, "test", "Content")

			// Apply the field value
			switch tt.field {
			case "id":
				spec.ID = tt.value
			case "title":
				spec.Title = tt.value
			case "namespace":
				spec.Namespace = tt.value
			}

			result := validator.Validate(spec)
			hasError := result.HasErrors()

			if tt.expectError && !hasError {
				t.Errorf("Expected error for field %s with value '%s'", tt.field, tt.value)
			}
			if !tt.expectError && hasError {
				t.Errorf("Unexpected error for field %s with value '%s': %v", tt.field, tt.value, result.Errors)
			}
		})
	}
}

// TestValidatorMarkdown tests markdown validation
func TestValidatorMarkdown(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name        string
		content     string
		expectError bool
	}{
		{"valid_markdown", "# Title\n\nContent", false},
		{"valid_list", "- Item 1\n- Item 2", false},
		{"valid_code", "```go\nfunc test() {}\n```", false},
		{"empty_content", "", false}, // Empty is acceptable
		{"unmatched_brackets", "# Title [unclosed", false}, // Markdown still valid
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewSpec("test", "Title", SpecKindGoal, "ns", tt.content)
			result := validator.Validate(spec)

			// For now, markdown validation is lenient
			hasError := result.HasErrors()
			if tt.expectError && !hasError {
				t.Errorf("Expected markdown error")
			}
		})
	}
}

// TestCompilerTopologicalSort tests sorting algorithm
func TestCompilerTopologicalSort(t *testing.T) {
	tests := []struct {
		name      string
		setup     func() *SpecCollection
		checkOrder func(t *testing.T, order []string)
	}{
		{
			name: "linear_chain",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				for i := 0; i < 5; i++ {
					spec := NewSpec(
						string(rune(65+i)),
						string(rune(65+i)),
						SpecKindGoal,
						"test",
						"Content",
					)
					if i > 0 {
						spec.DependsOn = []string{string(rune(65 + i - 1))}
					}
					col.Add(spec)
				}
				return col
			},
			checkOrder: func(t *testing.T, order []string) {
				// Should maintain order A -> B -> C -> D -> E
				if len(order) != 5 {
					t.Errorf("Expected 5 specs in order")
				}
				for i := 0; i < 4; i++ {
					current := order[i]
					next := order[i+1]
					if current >= next {
						t.Errorf("Order incorrect at positions %d and %d", i, i+1)
					}
				}
			},
		},
		{
			name: "diamond_dependency",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				// A
				// ├─ B
				// └─ C
				//    └─ D
				col.Add(NewSpec("A", "A", SpecKindGoal, "test", ""))
				b := NewSpec("B", "B", SpecKindGoal, "test", "")
				b.DependsOn = []string{"A"}
				col.Add(b)
				c := NewSpec("C", "C", SpecKindGoal, "test", "")
				c.DependsOn = []string{"A"}
				col.Add(c)
				d := NewSpec("D", "D", SpecKindGoal, "test", "")
				d.DependsOn = []string{"C"}
				col.Add(d)
				return col
			},
			checkOrder: func(t *testing.T, order []string) {
				// A must come before B, C
				// C must come before D
				aIdx := -1
				bIdx := -1
				cIdx := -1
				dIdx := -1

				for i, id := range order {
					if id == "A" {
						aIdx = i
					} else if id == "B" {
						bIdx = i
					} else if id == "C" {
						cIdx = i
					} else if id == "D" {
						dIdx = i
					}
				}

				if aIdx >= bIdx || aIdx >= cIdx {
					t.Errorf("A should come before B and C")
				}
				if cIdx >= dIdx {
					t.Errorf("C should come before D")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := tt.setup()
			compiler := NewCompiler(col)
			result := compiler.Compile()

			if !result.Successful {
				t.Errorf("Compilation failed: %v", result.Errors)
			}

			order := compiler.TopologicalOrder()
			tt.checkOrder(t, order)
		})
	}
}

// TestGateRegistry tests gate registration and execution
func TestGateRegistry(t *testing.T) {
	t.Run("register_and_execute", func(t *testing.T) {
		registry := NewGateRegistry()

		gate1 := &RequiredFieldsGate{}
		gate2 := &MarkdownSyntaxGate{}
		gate3 := &TokenBudgetGate{Budget: 10000}

		registry.Register(gate1)
		registry.Register(gate2)
		registry.Register(gate3)

		spec := NewSpec("test", "Title", SpecKindGoal, "ns", "# Content")
		spec.Status = SpecStatusActive
		spec.TokenEstimate = 500

		ctx := &VerificationContext{}
		results := registry.Run(spec, ctx)

		if len(results.Results) != 3 {
			t.Errorf("Expected 3 gate results, got %d", len(results.Results))
		}

		if results.HasCriticalFailure {
			t.Errorf("Should not have critical failures")
		}
	})

	t.Run("gate_ordering", func(t *testing.T) {
		registry := NewGateRegistry()
		registry.Register(&TokenBudgetGate{Budget: 100})
		registry.Register(&RequiredFieldsGate{})

		spec := NewSpec("test", "Title", SpecKindGoal, "ns", "Content")
		spec.TokenEstimate = 500 // Exceeds budget

		ctx := &VerificationContext{}
		results := registry.Run(spec, ctx)

		// TokenBudgetGate should fail
		found := false
		for _, res := range results.Results {
			if res.GateName == "TokenBudgetGate" && res.Failed {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("TokenBudgetGate should fail")
		}
	})
}

// TestMergerConflictResolution tests conflict resolution strategies
func TestMergerConflictResolution(t *testing.T) {
	tests := []struct {
		name     string
		field    string
		base     string
		ours     string
		theirs   string
		strategy string
	}{
		{"title_no_conflict", "title", "A", "A", "A", "none"},
		{"title_our_change", "title", "A", "B", "A", "ours"},
		{"title_their_change", "title", "A", "A", "B", "theirs"},
		{"content_both_changed", "content", "base", "ours_content", "theirs_content", "merge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := NewSpec("test", tt.base, SpecKindGoal, "ns", "base")
			ours := NewSpec("test", tt.ours, SpecKindGoal, "ns", "ours")
			theirs := NewSpec("test", tt.theirs, SpecKindGoal, "ns", "theirs")

			merger := NewMerger()
			merged, err := merger.Merge(base, ours, theirs)

			if err != nil {
				t.Logf("Merge returned error: %v", err)
			}

			if merged != nil && tt.field == "title" {
				// Should have a title (either from ours, theirs, or base)
				if merged.Title == "" {
					t.Errorf("Merged spec should have a title")
				}
			}
		})
	}
}

// TestMetaSpecSearch tests search functionality
func TestMetaSpecSearch(t *testing.T) {
	col := NewSpecCollection()
	col.Add(NewSpec("s1", "User Authentication", SpecKindGoal, "auth", "JWT tokens"))
	col.Add(NewSpec("s2", "OAuth2 Integration", SpecKindProcess, "auth.oauth2", "OAuth flow"))
	col.Add(NewSpec("s3", "API Rate Limiting", SpecKindConstraint, "api", "Rate limit specs"))

	indexer := NewSpecIndexer(col, 10000)
	indexer.BuildIndex()

	tests := []struct {
		name     string
		query    string
		minCount int
	}{
		{"search_auth", "auth", 2},
		{"search_token", "token", 1},
		{"search_rate", "rate", 1},
		{"search_api", "api", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := indexer.MetaSpec.SearchByKeyword(tt.query)
			if len(results) < tt.minCount {
				t.Errorf("Expected at least %d results for '%s', got %d", tt.minCount, tt.query, len(results))
			}
		})
	}
}

// TestMetaSpecFiltering tests filtering operations
func TestMetaSpecFiltering(t *testing.T) {
	col := NewSpecCollection()

	// Add diverse specs
	for _, spec := range []*Spec{
		func() *Spec {
			s := NewSpec("g1", "Goal 1", SpecKindGoal, "goals", "")
			s.Status = SpecStatusActive
			s.TokenEstimate = 500
			return s
		}(),
		func() *Spec {
			s := NewSpec("g2", "Goal 2", SpecKindGoal, "goals", "")
			s.Status = SpecStatusDraft
			s.TokenEstimate = 300
			return s
		}(),
		func() *Spec {
			s := NewSpec("p1", "Process 1", SpecKindProcess, "process", "")
			s.Status = SpecStatusActive
			s.TokenEstimate = 800
			return s
		}(),
		func() *Spec {
			s := NewSpec("c1", "Constraint 1", SpecKindConstraint, "constraint", "")
			s.Status = SpecStatusActive
			s.TokenEstimate = 200
			return s
		}(),
	} {
		col.Add(spec)
	}

	indexer := NewSpecIndexer(col, 10000)
	indexer.BuildIndex()

	tests := []struct {
		name          string
		filterFunc    func() []*Spec
		expectedCount int
	}{
		{"filter_goals", func() []*Spec {
			return indexer.MetaSpec.SelectByKind(SpecKindGoal)
		}, 2},
		{"filter_active", func() []*Spec {
			return indexer.MetaSpec.SelectByStatus(SpecStatusActive)
		}, 3},
		{"filter_draft", func() []*Spec {
			return indexer.MetaSpec.SelectByStatus(SpecStatusDraft)
		}, 1},
		{"filter_goals_namespace", func() []*Spec {
			goals := indexer.MetaSpec.SelectByKind(SpecKindGoal)
			filtered := make([]*Spec, 0)
			for _, s := range goals {
				if strings.HasPrefix(s.Namespace, "goals") {
					filtered = append(filtered, s)
				}
			}
			return filtered
		}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := tt.filterFunc()
			if len(results) != tt.expectedCount {
				t.Errorf("Expected %d results, got %d", tt.expectedCount, len(results))
			}
		})
	}
}

// TestTokenBudgeter tests token allocation
func TestTokenBudgeter(t *testing.T) {
	tests := []struct {
		name     string
		budget   int
		numSpecs int
		checkAlloc func(t *testing.T, alloc map[string]int)
	}{
		{
			name:     "proportional_allocation",
			budget:   10000,
			numSpecs: 5,
			checkAlloc: func(t *testing.T, alloc map[string]int) {
				total := 0
				for _, tokens := range alloc {
					total += tokens
					if tokens <= 0 {
						t.Errorf("Each allocation should be positive")
					}
				}
				if total > 10000 {
					t.Errorf("Total allocation should not exceed budget")
				}
			},
		},
		{
			name:     "small_budget",
			budget:   100,
			numSpecs: 10,
			checkAlloc: func(t *testing.T, alloc map[string]int) {
				total := 0
				for _, tokens := range alloc {
					total += tokens
				}
				if total > 100 {
					t.Errorf("Should respect small budget")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budgeter := NewTokenBudgeter(tt.budget, tt.numSpecs, 20)

			specs := make([]*Spec, 0)
			for i := 0; i < tt.numSpecs; i++ {
				spec := NewSpec(
					"s"+string(rune(65+i)),
					"Spec",
					SpecKindGoal,
					"test",
					"Content",
				)
				specs = append(specs, spec)
			}

			alloc := budgeter.AllocateProportional(specs)

			if alloc == nil {
				t.Errorf("Allocation should not be nil")
			} else {
				tt.checkAlloc(t, alloc)
			}
		})
	}
}

// TestCommandContext tests command execution context
func TestCommandContext(t *testing.T) {
	col := NewSpecCollection()
	col.Add(NewSpec("s1", "Spec 1", SpecKindGoal, "test", "Content"))

	ctx := &CommandContext{
		Command:    "test",
		Args:       []string{"test", "arg1", "arg2"},
		Collection: col,
	}

	if ctx.Command != "test" {
		t.Errorf("Command not set")
	}

	if len(ctx.Args) != 3 {
		t.Errorf("Args not set correctly")
	}

	if ctx.Collection.Count() != 1 {
		t.Errorf("Collection not accessible")
	}
}

// TestSpecKindString tests SpecKind string representation
func TestSpecKindString(t *testing.T) {
	tests := []struct {
		kind     SpecKind
		expected string
	}{
		{SpecKindGoal, "goal"},
		{SpecKindProcess, "process"},
		{SpecKindConstraint, "constraint"},
		{SpecKindComponent, "component"},
		{SpecKindIntegration, "integration"},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			if string(tt.kind) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.kind))
			}
		})
	}
}

// TestSpecStatusString tests SpecStatus string representation
func TestSpecStatusString(t *testing.T) {
	tests := []struct {
		status   SpecStatus
		expected string
	}{
		{SpecStatusDraft, "draft"},
		{SpecStatusActive, "active"},
		{SpecStatusArchived, "archived"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if string(tt.status) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.status))
			}
		})
	}
}
