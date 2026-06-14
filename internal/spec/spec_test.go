package spec

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestSpecCreation tests basic spec creation and properties
func TestSpecCreation(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *Spec
		check   func(t *testing.T, spec *Spec)
		wantErr bool
	}{
		{
			name: "create goal spec",
			setup: func() *Spec {
				return NewSpec(
					"spec_goal_001",
					"Authentication System",
					SpecKindGoal,
					"auth",
					"Implement secure user authentication",
				)
			},
			check: func(t *testing.T, spec *Spec) {
				if spec.ID != "spec_goal_001" {
					t.Errorf("ID mismatch: got %s, want spec_goal_001", spec.ID)
				}
				if spec.Title != "Authentication System" {
					t.Errorf("Title mismatch")
				}
				if spec.Kind != SpecKindGoal {
					t.Errorf("Kind mismatch: got %v, want %v", spec.Kind, SpecKindGoal)
				}
				if spec.Status != SpecStatusDraft {
					t.Errorf("Status should be Draft initially")
				}
				if spec.CreatedAt.IsZero() {
					t.Errorf("CreatedAt not set")
				}
			},
		},
		{
			name: "create process spec",
			setup: func() *Spec {
				return NewSpec(
					"spec_proc_001",
					"OAuth2 Flow",
					SpecKindProcess,
					"auth.oauth",
					"Document OAuth2 authentication flow",
				)
			},
			check: func(t *testing.T, spec *Spec) {
				if spec.Kind != SpecKindProcess {
					t.Errorf("Kind mismatch")
				}
				if !strings.Contains(spec.Namespace, ".") {
					t.Errorf("Nested namespace not preserved")
				}
			},
		},
		{
			name: "create constraint spec",
			setup: func() *Spec {
				return NewSpec(
					"spec_const_001",
					"Performance SLA",
					SpecKindConstraint,
					"perf",
					"API responses must be < 200ms",
				)
			},
			check: func(t *testing.T, spec *Spec) {
				if spec.Kind != SpecKindConstraint {
					t.Errorf("Kind mismatch")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.setup()
			tt.check(t, spec)
		})
	}
}

// TestSpecValidation tests validation rules
func TestSpecValidation(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *Spec
		validator   func(*Spec) *ValidationResult
		expectValid bool
	}{
		{
			name: "valid spec passes validation",
			setup: func() *Spec {
				spec := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Content")
				spec.Status = SpecStatusActive
				return spec
			},
			validator: func(spec *Spec) *ValidationResult {
				validator := NewValidator()
				return validator.Validate(spec)
			},
			expectValid: true,
		},
		{
			name: "spec with empty ID fails",
			setup: func() *Spec {
				spec := NewSpec("", "Title", SpecKindGoal, "ns", "Content")
				return spec
			},
			validator: func(spec *Spec) *ValidationResult {
				validator := NewValidator()
				return validator.Validate(spec)
			},
			expectValid: false,
		},
		{
			name: "spec with empty title fails",
			setup: func() *Spec {
				spec := NewSpec("spec_001", "", SpecKindGoal, "ns", "Content")
				return spec
			},
			validator: func(spec *Spec) *ValidationResult {
				validator := NewValidator()
				return validator.Validate(spec)
			},
			expectValid: false,
		},
		{
			name: "spec with empty namespace fails",
			setup: func() *Spec {
				spec := NewSpec("spec_001", "Title", SpecKindGoal, "", "Content")
				return spec
			},
			validator: func(spec *Spec) *ValidationResult {
				validator := NewValidator()
				return validator.Validate(spec)
			},
			expectValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := tt.setup()
			result := tt.validator(spec)
			if tt.expectValid && result.HasErrors() {
				t.Errorf("validation failed unexpectedly: %v", result.Errors)
			}
			if !tt.expectValid && !result.HasErrors() {
				t.Errorf("validation succeeded unexpectedly")
			}
		})
	}
}

// TestSpecCollection tests collection operations
func TestSpecCollection(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *SpecCollection
		ops   func(t *testing.T, col *SpecCollection)
	}{
		{
			name: "add and retrieve specs",
			setup: func() *SpecCollection {
				return NewSpecCollection()
			},
			ops: func(t *testing.T, col *SpecCollection) {
				spec1 := NewSpec("spec_001", "Goal 1", SpecKindGoal, "goals", "First goal")
				spec2 := NewSpec("spec_002", "Goal 2", SpecKindGoal, "goals", "Second goal")

				col.Add(spec1)
				col.Add(spec2)

				if len(col.Specs) != 2 {
					t.Errorf("Expected 2 specs, got %d", len(col.Specs))
				}

				retrieved := col.Get("spec_001")
				if retrieved == nil || retrieved.ID != "spec_001" {
					t.Errorf("Failed to retrieve spec")
				}
			},
		},
		{
			name: "list specs by namespace",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				col.Add(NewSpec("spec_001", "Auth Goal", SpecKindGoal, "auth", "Auth goal"))
				col.Add(NewSpec("spec_002", "OAuth Flow", SpecKindProcess, "auth.oauth", "OAuth process"))
				col.Add(NewSpec("spec_003", "API Goal", SpecKindGoal, "api", "API goal"))
				return col
			},
			ops: func(t *testing.T, col *SpecCollection) {
				authSpecs := col.ListByNamespace("auth")
				if len(authSpecs) != 2 {
					t.Errorf("Expected 2 auth specs, got %d", len(authSpecs))
				}

				goalSpecs := col.ListByKind(SpecKindGoal)
				if len(goalSpecs) != 2 {
					t.Errorf("Expected 2 goal specs, got %d", len(goalSpecs))
				}
			},
		},
		{
			name: "update spec status",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				col.Add(NewSpec("spec_001", "Goal", SpecKindGoal, "goals", "Goal content"))
				return col
			},
			ops: func(t *testing.T, col *SpecCollection) {
				spec := col.Get("spec_001")
				spec.Status = SpecStatusActive
				spec.UpdatedAt = time.Now()

				if spec.Status != SpecStatusActive {
					t.Errorf("Status not updated")
				}
				if spec.UpdatedAt.IsZero() {
					t.Errorf("UpdatedAt not set")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := tt.setup()
			tt.ops(t, col)
		})
	}
}

// TestDependencyGraph tests graph operations
func TestDependencyGraph(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *SpecCollection
		check func(t *testing.T, col *SpecCollection, graph *DependencyGraph)
	}{
		{
			name: "build dependency graph",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				spec1 := NewSpec("spec_001", "Auth", SpecKindGoal, "auth", "Auth goal")
				spec2 := NewSpec("spec_002", "OAuth", SpecKindProcess, "auth", "OAuth depends on Auth")
				spec2.DependsOn = []string{"spec_001"}
				spec3 := NewSpec("spec_003", "API", SpecKindGoal, "api", "API depends on Auth")
				spec3.DependsOn = []string{"spec_001"}

				col.Add(spec1)
				col.Add(spec2)
				col.Add(spec3)
				return col
			},
			check: func(t *testing.T, col *SpecCollection, graph *DependencyGraph) {
				if len(graph.Nodes) != 3 {
					t.Errorf("Expected 3 nodes, got %d", len(graph.Nodes))
				}

				edges := graph.GetOutgoing("spec_001")
				if len(edges) != 2 {
					t.Errorf("Expected 2 outgoing edges from spec_001, got %d", len(edges))
				}

				incomingAuth := graph.GetIncoming("spec_002")
				if len(incomingAuth) != 1 {
					t.Errorf("Expected 1 incoming edge to spec_002, got %d", len(incomingAuth))
				}
			},
		},
		{
			name: "detect cycles",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				spec1 := NewSpec("spec_001", "A", SpecKindGoal, "test", "A")
				spec2 := NewSpec("spec_002", "B", SpecKindGoal, "test", "B")
				spec3 := NewSpec("spec_003", "C", SpecKindGoal, "test", "C")

				spec1.DependsOn = []string{"spec_002"}
				spec2.DependsOn = []string{"spec_003"}
				spec3.DependsOn = []string{"spec_001"} // cycle

				col.Add(spec1)
				col.Add(spec2)
				col.Add(spec3)
				return col
			},
			check: func(t *testing.T, col *SpecCollection, graph *DependencyGraph) {
				cycles := graph.DetectCycles()
				if len(cycles) == 0 {
					t.Errorf("Should detect cycle")
				}
				if len(cycles[0]) != 3 {
					t.Errorf("Cycle should have 3 nodes")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := tt.setup()
			graph := NewDependencyGraph(col)
			tt.check(t, col, graph)
		})
	}
}

// TestSpecCompiler tests compilation
func TestSpecCompiler(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *SpecCollection
		check func(t *testing.T, result *CompilationResult)
	}{
		{
			name: "successful compilation",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				col.Add(NewSpec("spec_001", "Auth", SpecKindGoal, "auth", "Auth"))
				col.Add(NewSpec("spec_002", "OAuth", SpecKindProcess, "auth", "OAuth"))
				return col
			},
			check: func(t *testing.T, result *CompilationResult) {
				if !result.Successful {
					t.Errorf("Compilation should succeed")
				}
				if result.SpecCount() != 2 {
					t.Errorf("Expected 2 specs")
				}
			},
		},
		{
			name: "compilation fails with cycles",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				s1 := NewSpec("spec_001", "A", SpecKindGoal, "test", "A")
				s2 := NewSpec("spec_002", "B", SpecKindGoal, "test", "B")
				s1.DependsOn = []string{"spec_002"}
				s2.DependsOn = []string{"spec_001"}
				col.Add(s1)
				col.Add(s2)
				return col
			},
			check: func(t *testing.T, result *CompilationResult) {
				if result.Successful {
					t.Errorf("Compilation should fail with cycles")
				}
				if len(result.Errors) == 0 {
					t.Errorf("Should have cycle error")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := tt.setup()
			compiler := NewCompiler(col)
			result := compiler.Compile()
			tt.check(t, result)
		})
	}
}

// TestGates tests quality gates
func TestGates(t *testing.T) {
	tests := []struct {
		name        string
		spec        *Spec
		gate        Gate
		expectPass  bool
		description string
	}{
		{
			name: "required fields gate pass",
			spec: func() *Spec {
				spec := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Content")
				spec.Status = SpecStatusActive
				return spec
			}(),
			gate: &RequiredFieldsGate{},
			expectPass: true,
			description: "All required fields present",
		},
		{
			name: "required fields gate fail",
			spec: func() *Spec {
				spec := NewSpec("spec_001", "", SpecKindGoal, "ns", "Content")
				return spec
			}(),
			gate: &RequiredFieldsGate{},
			expectPass: false,
			description: "Missing title",
		},
		{
			name: "markdown syntax gate pass",
			spec: func() *Spec {
				spec := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "# Heading\n\nContent")
				return spec
			}(),
			gate: &MarkdownSyntaxGate{},
			expectPass: true,
			description: "Valid markdown",
		},
		{
			name: "token budget gate pass",
			spec: func() *Spec {
				spec := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Small content")
				spec.TokenEstimate = 100
				return spec
			}(),
			gate: &TokenBudgetGate{Budget: 1000},
			expectPass: true,
			description: "Under budget",
		},
		{
			name: "token budget gate fail",
			spec: func() *Spec {
				spec := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Content")
				spec.TokenEstimate = 5000
				return spec
			}(),
			gate: &TokenBudgetGate{Budget: 1000},
			expectPass: false,
			description: "Over budget",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := &VerificationContext{}
			result := tt.gate.Run(tt.spec, ctx)

			if tt.expectPass && result.Failed {
				t.Errorf("Gate should pass: %s", tt.description)
			}
			if !tt.expectPass && !result.Failed {
				t.Errorf("Gate should fail: %s", tt.description)
			}
		})
	}
}

// TestMerge tests three-way merge
func TestMerge(t *testing.T) {
	tests := []struct {
		name         string
		base         *Spec
		ours         *Spec
		theirs       *Spec
		expectMerge  bool
		checkMerged  func(t *testing.T, merged *Spec)
	}{
		{
			name: "non-conflicting merge",
			base: func() *Spec {
				s := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Base content")
				return s
			}(),
			ours: func() *Spec {
				s := NewSpec("spec_001", "Title Updated", SpecKindGoal, "ns", "Base content")
				return s
			}(),
			theirs: func() *Spec {
				s := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Updated content")
				return s
			}(),
			expectMerge: true,
			checkMerged: func(t *testing.T, merged *Spec) {
				if merged.Title != "Title Updated" {
					t.Errorf("Title should be from ours")
				}
				if !strings.Contains(merged.Content, "Updated content") {
					t.Errorf("Content should be from theirs")
				}
			},
		},
		{
			name: "conflicting titles merge with strategy",
			base: func() *Spec {
				s := NewSpec("spec_001", "Original", SpecKindGoal, "ns", "Content")
				return s
			}(),
			ours: func() *Spec {
				s := NewSpec("spec_001", "Our Title", SpecKindGoal, "ns", "Content")
				return s
			}(),
			theirs: func() *Spec {
				s := NewSpec("spec_001", "Their Title", SpecKindGoal, "ns", "Content")
				return s
			}(),
			expectMerge: true,
			checkMerged: func(t *testing.T, merged *Spec) {
				// Merge uses "ours" strategy by default for titles
				if merged.Title == "" {
					t.Errorf("Title should be set after merge")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewMerger()
			merged, err := merger.Merge(tt.base, tt.ours, tt.theirs)

			if tt.expectMerge && err != nil {
				t.Errorf("Merge should succeed: %v", err)
			}
			if merged != nil && tt.checkMerged != nil {
				tt.checkMerged(t, merged)
			}
		})
	}
}

// TestMetaSpecIndexing tests MetaSpec indexing
func TestMetaSpecIndexing(t *testing.T) {
	tests := []struct {
		name  string
		setup func() *SpecCollection
		check func(t *testing.T, indexer *SpecIndexer)
	}{
		{
			name: "build and search index",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				col.Add(NewSpec("spec_001", "Authentication", SpecKindGoal, "auth", "User authentication system"))
				col.Add(NewSpec("spec_002", "Authorization", SpecKindGoal, "auth", "Permission and role management"))
				col.Add(NewSpec("spec_003", "API Rate Limiting", SpecKindConstraint, "api", "Rate limit constraints"))
				return col
			},
			check: func(t *testing.T, indexer *SpecIndexer) {
				indexer.BuildIndex()

				// Search for "authentication"
				results := indexer.MetaSpec.SearchByKeyword("authentication")
				if len(results) == 0 {
					t.Errorf("Should find authentication spec")
				}

				// Check namespace filtering
				authSpecs := indexer.MetaSpec.SelectByNamespace("auth")
				if len(authSpecs) != 2 {
					t.Errorf("Expected 2 auth specs, got %d", len(authSpecs))
				}

				// Check kind filtering
				goals := indexer.MetaSpec.SelectByKind(SpecKindGoal)
				if len(goals) != 2 {
					t.Errorf("Expected 2 goals, got %d", len(goals))
				}
			},
		},
		{
			name: "token budget allocation",
			setup: func() *SpecCollection {
				col := NewSpecCollection()
				for i := 1; i <= 5; i++ {
					spec := NewSpec(
						fmt.Sprintf("spec_%03d", i),
						fmt.Sprintf("Spec %d", i),
						SpecKindGoal,
						"test",
						fmt.Sprintf("Content %d", i),
					)
					spec.TokenEstimate = 500 * i
					col.Add(spec)
				}
				return col
			},
			check: func(t *testing.T, indexer *SpecIndexer) {
				// Total tokens: 500 + 1000 + 1500 + 2000 + 2500 = 7500
				budgeter := NewTokenBudgeter(5000, 5, 20)
				selected := indexer.MetaSpec.SelectByBudget(5000, 5)

				totalTokens := 0
				for _, spec := range selected {
					totalTokens += spec.TokenEstimate
				}

				if totalTokens > 5000 {
					t.Errorf("Selected specs exceed budget: %d > 5000", totalTokens)
				}

				if len(selected) == 0 {
					t.Errorf("Should select at least one spec")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := tt.setup()
			indexer := NewSpecIndexer(col, 10000)
			tt.check(t, indexer)
		})
	}
}

// TestSpecKitCommands tests SpecKit commands
func TestSpecKitCommands(t *testing.T) {
	tests := []struct {
		name        string
		setupCollection func() *SpecCollection
		command     string
		args        []string
		expectError bool
		checkResult func(t *testing.T, result *CommandResult)
	}{
		{
			name: "list command",
			setupCollection: func() *SpecCollection {
				col := NewSpecCollection()
				col.Add(NewSpec("spec_001", "Goal 1", SpecKindGoal, "goals", "First"))
				col.Add(NewSpec("spec_002", "Goal 2", SpecKindGoal, "goals", "Second"))
				return col
			},
			command:     "list",
			args:        []string{"spec", "list"},
			expectError: false,
			checkResult: func(t *testing.T, result *CommandResult) {
				if !strings.Contains(result.Output, "spec_001") {
					t.Errorf("Output should contain spec_001")
				}
				if !strings.Contains(result.Output, "spec_002") {
					t.Errorf("Output should contain spec_002")
				}
			},
		},
		{
			name: "show command",
			setupCollection: func() *SpecCollection {
				col := NewSpecCollection()
				col.Add(NewSpec("spec_001", "Authentication", SpecKindGoal, "auth", "Auth system"))
				return col
			},
			command:     "show",
			args:        []string{"spec", "show", "spec_001"},
			expectError: false,
			checkResult: func(t *testing.T, result *CommandResult) {
				if !strings.Contains(result.Output, "Authentication") {
					t.Errorf("Output should contain spec title")
				}
			},
		},
		{
			name: "search command",
			setupCollection: func() *SpecCollection {
				col := NewSpecCollection()
				col.Add(NewSpec("spec_001", "User Auth", SpecKindGoal, "auth", "User authentication system"))
				col.Add(NewSpec("spec_002", "API Design", SpecKindGoal, "api", "RESTful API design"))
				return col
			},
			command:     "search",
			args:        []string{"spec", "search", "auth"},
			expectError: false,
			checkResult: func(t *testing.T, result *CommandResult) {
				if !strings.Contains(result.Output, "spec_001") {
					t.Errorf("Should find auth-related spec")
				}
			},
		},
		{
			name: "verify command",
			setupCollection: func() *SpecCollection {
				col := NewSpecCollection()
				spec := NewSpec("spec_001", "Valid Goal", SpecKindGoal, "goals", "Valid content")
				spec.Status = SpecStatusActive
				col.Add(spec)
				return col
			},
			command:     "verify",
			args:        []string{"spec", "verify", "spec_001"},
			expectError: false,
			checkResult: func(t *testing.T, result *CommandResult) {
				if result.Failed {
					t.Errorf("Verification should pass for valid spec")
				}
			},
		},
		{
			name: "compile command",
			setupCollection: func() *SpecCollection {
				col := NewSpecCollection()
				col.Add(NewSpec("spec_001", "Goal", SpecKindGoal, "goals", "Goal"))
				col.Add(NewSpec("spec_002", "Process", SpecKindProcess, "process", "Process"))
				return col
			},
			command:     "compile",
			args:        []string{"spec", "compile"},
			expectError: false,
			checkResult: func(t *testing.T, result *CommandResult) {
				if !strings.Contains(result.Output, "compile") || !strings.Contains(result.Output, "success") {
					t.Errorf("Should show compilation result")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := tt.setupCollection()
			kit := NewSpecKit(col)

			ctx := &CommandContext{
				Command:    tt.command,
				Args:       tt.args,
				Collection: col,
			}

			result, err := kit.Execute(ctx)

			if tt.expectError && err == nil {
				t.Errorf("Command should error")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Command should not error: %v", err)
			}

			if result != nil && tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

// TestEndToEndWorkflow tests complete workflow
func TestEndToEndWorkflow(t *testing.T) {
	t.Run("complete spec lifecycle", func(t *testing.T) {
		// 1. Create collection
		col := NewSpecCollection()

		// 2. Add specs
		authGoal := NewSpec("spec_auth_goal", "User Authentication", SpecKindGoal, "auth", "# User Authentication\n\nSecure user authentication system")
		authGoal.Status = SpecStatusActive
		authGoal.TokenEstimate = 800

		oauthProcess := NewSpec("spec_oauth_proc", "OAuth2 Flow", SpecKindProcess, "auth.oauth", "# OAuth2 Implementation\n\nImplement OAuth2 flow")
		oauthProcess.DependsOn = []string{"spec_auth_goal"}
		oauthProcess.TokenEstimate = 600

		apiGoal := NewSpec("spec_api_goal", "REST API", SpecKindGoal, "api", "# REST API Design\n\nDesign REST endpoints")
		apiGoal.DependsOn = []string{"spec_auth_goal"}
		apiGoal.TokenEstimate = 1200

		col.Add(authGoal)
		col.Add(oauthProcess)
		col.Add(apiGoal)

		// 3. Validate
		validator := NewValidator()
		for _, spec := range col.Specs {
			result := validator.Validate(spec)
			if result.HasErrors() {
				t.Errorf("Spec validation failed: %v", result.Errors)
			}
		}

		// 4. Compile
		compiler := NewCompiler(col)
		compResult := compiler.Compile()
		if !compResult.Successful {
			t.Errorf("Compilation failed: %v", compResult.Errors)
		}

		// 5. Run gates
		registry := NewGateRegistry()
		registry.Register(&RequiredFieldsGate{})
		registry.Register(&MarkdownSyntaxGate{})
		registry.Register(&TokenBudgetGate{Budget: 5000})

		gateResults := registry.Run(authGoal, &VerificationContext{})
		if gateResults.HasCriticalFailure {
			t.Errorf("Critical gate failure: %v", gateResults.Results)
		}

		// 6. Index and search
		indexer := NewSpecIndexer(col, 10000)
		indexer.BuildIndex()

		authSpecs := indexer.MetaSpec.SelectByNamespace("auth")
		if len(authSpecs) != 2 {
			t.Errorf("Expected 2 auth specs, got %d", len(authSpecs))
		}

		// 7. Allocate tokens
		budgeter := NewTokenBudgeter(3000, 3, 20)
		selected := indexer.MetaSpec.SelectByBudget(3000, 3)
		if len(selected) == 0 {
			t.Errorf("Should select specs within budget")
		}

		// 8. Chat commands
		kit := NewSpecKit(col)
		ctx := &CommandContext{
			Command:    "list",
			Args:       []string{"spec", "list"},
			Collection: col,
		}

		cmdResult, err := kit.Execute(ctx)
		if err != nil {
			t.Errorf("Command execution failed: %v", err)
		}
		if cmdResult == nil {
			t.Errorf("Command result should not be nil")
		}

		// 9. Verify collection stats
		if col.Count() != 3 {
			t.Errorf("Collection should have 3 specs, got %d", col.Count())
		}

		if len(compResult.Order) != 3 {
			t.Errorf("Topological order should have 3 specs")
		}
	})
}

// TestConcurrency tests concurrent operations
func TestConcurrency(t *testing.T) {
	t.Run("concurrent spec creation", func(t *testing.T) {
		col := NewSpecCollection()
		done := make(chan bool, 100)

		for i := 0; i < 100; i++ {
			go func(idx int) {
				spec := NewSpec(
					fmt.Sprintf("spec_%03d", idx),
					fmt.Sprintf("Spec %d", idx),
					SpecKindGoal,
					"test",
					"Content",
				)
				col.Add(spec)
				done <- true
			}(i)
		}

		for i := 0; i < 100; i++ {
			<-done
		}

		if col.Count() != 100 {
			t.Errorf("Expected 100 specs, got %d", col.Count())
		}
	})
}

// TestErrorHandling tests error conditions
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		op          func() error
		expectError bool
	}{
		{
			name: "retrieve non-existent spec",
			op: func() error {
				col := NewSpecCollection()
				spec := col.Get("non_existent")
				if spec != nil {
					return fmt.Errorf("should return nil")
				}
				return nil
			},
			expectError: false,
		},
		{
			name: "validate spec with invalid kind",
			op: func() error {
				spec := &Spec{
					ID:        "test",
					Title:     "Test",
					Kind:      SpecKind("INVALID"),
					Namespace: "test",
					Content:   "Content",
				}
				validator := NewValidator()
				result := validator.Validate(spec)
				if !result.HasErrors() {
					return fmt.Errorf("should have validation errors")
				}
				return nil
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.op()
			if tt.expectError && err == nil {
				t.Errorf("Should return error")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Should not return error: %v", err)
			}
		})
	}
}

// BenchmarkSpecCreation benchmarks spec creation
func BenchmarkSpecCreation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewSpec(
			fmt.Sprintf("spec_%d", i),
			"Test Spec",
			SpecKindGoal,
			"test",
			"Content",
		)
	}
}

// BenchmarkCompilation benchmarks graph compilation
func BenchmarkCompilation(b *testing.B) {
	col := NewSpecCollection()
	for i := 0; i < 100; i++ {
		spec := NewSpec(
			fmt.Sprintf("spec_%03d", i),
			fmt.Sprintf("Spec %d", i),
			SpecKindGoal,
			"test",
			"Content",
		)
		if i > 0 {
			spec.DependsOn = []string{fmt.Sprintf("spec_%03d", i-1)}
		}
		col.Add(spec)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiler := NewCompiler(col)
		_ = compiler.Compile()
	}
}

// BenchmarkValidation benchmarks validation
func BenchmarkValidation(b *testing.B) {
	spec := NewSpec("spec_001", "Test", SpecKindGoal, "test", "Content")
	validator := NewValidator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validator.Validate(spec)
	}
}

// BenchmarkMerge benchmarks three-way merge
func BenchmarkMerge(b *testing.B) {
	base := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Content")
	ours := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Content Updated")
	theirs := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Content Variant")
	merger := NewMerger()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = merger.Merge(base, ours, theirs)
	}
}
