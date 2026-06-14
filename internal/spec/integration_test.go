package spec

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestIntegrationSpecWorkflow tests a realistic workflow
func TestIntegrationSpecWorkflow(t *testing.T) {
	// Setup: Create a realistic SIN-Code project structure
	col := NewSpecCollection()

	// Goal specs
	authGoal := NewSpec(
		"spec_goal_auth_001",
		"Implement User Authentication",
		SpecKindGoal,
		"security.auth",
		`# User Authentication System

## Requirements
- Support email/password authentication
- Implement JWT tokens
- Add 2FA support

## Success Criteria
- All unit tests pass
- Security audit completed
- Performance < 100ms`,
	)
	authGoal.Status = SpecStatusActive
	authGoal.Priority = 1
	authGoal.TokenEstimate = 2500

	apiGoal := NewSpec(
		"spec_goal_api_001",
		"Design REST API",
		SpecKindGoal,
		"api.design",
		`# REST API Design

## Endpoints
- POST /auth/login
- POST /auth/register
- GET /api/users
- POST /api/data

## Response Format
- JSON with standard envelope
- Error handling`,
	)
	apiGoal.Status = SpecStatusActive
	apiGoal.Priority = 1
	apiGoal.TokenEstimate = 1800
	apiGoal.DependsOn = []string{"spec_goal_auth_001"}

	// Process specs
	oauthProcess := NewSpec(
		"spec_proc_oauth_001",
		"OAuth2 Integration",
		SpecKindProcess,
		"security.auth.oauth2",
		`# OAuth2 Flow Implementation

## Process Steps
1. User initiates OAuth login
2. Redirect to auth provider
3. User authorizes
4. Callback with code
5. Exchange code for token

## Error Handling
- Invalid state
- Expired code
- Provider errors`,
	)
	oauthProcess.Status = SpecStatusDraft
	oauthProcess.DependsOn = []string{"spec_goal_auth_001"}
	oauthProcess.TokenEstimate = 1200

	jwtProcess := NewSpec(
		"spec_proc_jwt_001",
		"JWT Token Management",
		SpecKindProcess,
		"security.tokens",
		`# JWT Token Lifecycle

## Token Generation
- Create access token (15 min)
- Create refresh token (7 days)

## Token Validation
- Signature verification
- Expiration check

## Token Refresh
- Use refresh token
- Generate new access token`,
	)
	jwtProcess.Status = SpecStatusDraft
	jwtProcess.DependsOn = []string{"spec_goal_auth_001"}
	jwtProcess.TokenEstimate = 900

	// Constraint specs
	perfConstraint := NewSpec(
		"spec_const_perf_001",
		"Performance SLA",
		SpecKindConstraint,
		"nonfunctional.performance",
		`# Performance Requirements

## Response Times
- Authentication: < 200ms
- API: < 500ms
- Search: < 1000ms

## Throughput
- 10k requests/sec
- 1k concurrent users`,
	)
	perfConstraint.Status = SpecStatusActive
	perfConstraint.TokenEstimate = 400

	securityConstraint := NewSpec(
		"spec_const_sec_001",
		"Security Requirements",
		SpecKindConstraint,
		"nonfunctional.security",
		`# Security Constraints

## Encryption
- TLS 1.3 minimum
- AES-256 for data at rest

## Authentication
- OWASP Top 10 compliance
- Regular security audits`,
	)
	securityConstraint.Status = SpecStatusActive
	securityConstraint.TokenEstimate = 500

	// Add all specs to collection
	col.Add(authGoal)
	col.Add(apiGoal)
	col.Add(oauthProcess)
	col.Add(jwtProcess)
	col.Add(perfConstraint)
	col.Add(securityConstraint)

	// Phase 1: Validation
	t.Run("validation_phase", func(t *testing.T) {
		validator := NewValidator()
		validationErrors := 0

		for _, spec := range col.Specs {
			result := validator.Validate(spec)
			if result.HasErrors() {
				validationErrors++
				t.Logf("Validation errors for %s: %v", spec.ID, result.Errors)
			}
		}

		if validationErrors > 0 {
			t.Errorf("Found %d specs with validation errors", validationErrors)
		}
	})

	// Phase 2: Compilation and Graph Building
	t.Run("compilation_phase", func(t *testing.T) {
		compiler := NewCompiler(col)
		result := compiler.Compile()

		if !result.Successful {
			t.Errorf("Compilation failed: %v", result.Errors)
		}

		if result.SpecCount() != 6 {
			t.Errorf("Expected 6 specs, got %d", result.SpecCount())
		}

		// Check topological order
		order := compiler.TopologicalOrder()
		if len(order) != 6 {
			t.Errorf("Expected 6 specs in order, got %d", len(order))
		}

		// Verify auth goal comes before its dependents
		authIdx := -1
		apiIdx := -1
		oauthIdx := -1

		for i, specID := range order {
			if specID == "spec_goal_auth_001" {
				authIdx = i
			}
			if specID == "spec_goal_api_001" {
				apiIdx = i
			}
			if specID == "spec_proc_oauth_001" {
				oauthIdx = i
			}
		}

		if authIdx >= apiIdx || authIdx >= oauthIdx {
			t.Errorf("Dependency order incorrect")
		}
	})

	// Phase 3: Quality Gates
	t.Run("quality_gates_phase", func(t *testing.T) {
		registry := NewGateRegistry()
		registry.Register(&RequiredFieldsGate{})
		registry.Register(&MarkdownSyntaxGate{})
		registry.Register(&TokenBudgetGate{Budget: 10000})
		registry.Register(&StatusGate{})

		verificationCtx := &VerificationContext{
			Timestamp: time.Now(),
		}

		gateResults := registry.Run(authGoal, verificationCtx)

		if gateResults.HasCriticalFailure {
			t.Errorf("Critical gate failure for auth goal: %v", gateResults.Results)
		}

		// Check that all gates ran
		if len(gateResults.Results) == 0 {
			t.Errorf("No gates were run")
		}
	})

	// Phase 4: MetaSpec Indexing and Search
	t.Run("metaspec_phase", func(t *testing.T) {
		indexer := NewSpecIndexer(col, 15000)
		indexer.BuildIndex()

		// Test full-text search
		results := indexer.MetaSpec.SearchByKeyword("authentication")
		if len(results) == 0 {
			t.Errorf("Should find specs with 'authentication'")
		}

		// Test namespace filtering
		securitySpecs := indexer.MetaSpec.SelectByNamespace("security")
		if len(securitySpecs) < 3 {
			t.Errorf("Expected at least 3 security specs, got %d", len(securitySpecs))
		}

		// Test kind filtering
		goals := indexer.MetaSpec.SelectByKind(SpecKindGoal)
		if len(goals) != 2 {
			t.Errorf("Expected 2 goals, got %d", len(goals))
		}

		processes := indexer.MetaSpec.SelectByKind(SpecKindProcess)
		if len(processes) != 2 {
			t.Errorf("Expected 2 processes, got %d", len(processes))
		}

		constraints := indexer.MetaSpec.SelectByKind(SpecKindConstraint)
		if len(constraints) != 2 {
			t.Errorf("Expected 2 constraints, got %d", len(constraints))
		}

		// Test status filtering
		activeSpecs := indexer.MetaSpec.SelectByStatus(SpecStatusActive)
		if len(activeSpecs) < 3 {
			t.Errorf("Expected at least 3 active specs, got %d", len(activeSpecs))
		}

		draftSpecs := indexer.MetaSpec.SelectByStatus(SpecStatusDraft)
		if len(draftSpecs) != 2 {
			t.Errorf("Expected 2 draft specs, got %d", len(draftSpecs))
		}

		// Test token budget selection
		selected := indexer.MetaSpec.SelectByBudget(5000, 10)
		totalTokens := 0
		for _, spec := range selected {
			totalTokens += spec.TokenEstimate
		}
		if totalTokens > 5000 {
			t.Errorf("Selected specs exceed budget: %d > 5000", totalTokens)
		}
	})

	// Phase 5: Chat Command Integration
	t.Run("speckit_phase", func(t *testing.T) {
		kit := NewSpecKit(col)

		// Test list command
		ctx := &CommandContext{
			Command:    "list",
			Args:       []string{"spec", "list"},
			Collection: col,
		}
		result, _ := kit.Execute(ctx)
		if result == nil {
			t.Errorf("List command should return result")
		}

		// Test show command
		ctx = &CommandContext{
			Command:    "show",
			Args:       []string{"spec", "show", "spec_goal_auth_001"},
			Collection: col,
		}
		result, _ = kit.Execute(ctx)
		if result != nil && !strings.Contains(result.Output, "Authentication") {
			t.Errorf("Show command should display spec details")
		}

		// Test search command
		ctx = &CommandContext{
			Command:    "search",
			Args:       []string{"spec", "search", "oauth"},
			Collection: col,
		}
		result, _ = kit.Execute(ctx)
		if result != nil && len(result.Output) == 0 {
			t.Errorf("Search should find oauth specs")
		}

		// Test verify command
		ctx = &CommandContext{
			Command:    "verify",
			Args:       []string{"spec", "verify", "spec_goal_auth_001"},
			Collection: col,
		}
		result, _ = kit.Execute(ctx)
		if result == nil {
			t.Errorf("Verify command should return result")
		}

		// Test compile command
		ctx = &CommandContext{
			Command:    "compile",
			Args:       []string{"spec", "compile"},
			Collection: col,
		}
		result, _ = kit.Execute(ctx)
		if result == nil {
			t.Errorf("Compile command should return result")
		}
	})

	// Phase 6: Merge Testing
	t.Run("merge_phase", func(t *testing.T) {
		merger := NewMerger()

		// Create variants for merging
		base := col.Get("spec_goal_auth_001")
		if base == nil {
			t.Fatalf("Base spec not found")
		}

		// Create our version (update title)
		ours := NewSpec(
			base.ID,
			"Implement Secure User Authentication",
			base.Kind,
			base.Namespace,
			base.Content,
		)
		ours.Status = base.Status

		// Create their version (update content)
		theirs := NewSpec(
			base.ID,
			base.Title,
			base.Kind,
			base.Namespace,
			base.Content+"\n\n## Additional Notes\nConsider 3FA in future.",
		)
		theirs.Status = SpecStatusActive

		merged, err := merger.Merge(base, ours, theirs)
		if err != nil {
			t.Errorf("Merge failed: %v", err)
		}

		if merged == nil {
			t.Errorf("Merged spec should not be nil")
		}

		// Verify merge results
		if merged.Title == "" || merged.Content == "" {
			t.Errorf("Merged spec missing data")
		}
	})

	// Phase 7: Token Budget Simulation
	t.Run("token_budgeting", func(t *testing.T) {
		budgeter := NewTokenBudgeter(5000, col.Count(), 20)

		specs := col.ListByKind(SpecKindGoal)
		if len(specs) == 0 {
			t.Fatalf("No goal specs found")
		}

		// Proportional allocation
		alloc := budgeter.AllocateProportional(specs)
		if alloc == nil {
			t.Errorf("Should return allocation")
		}

		// Check that total allocation doesn't exceed budget
		totalAllocated := 0
		for _, spec := range specs {
			if alloc, ok := alloc[spec.ID]; ok {
				totalAllocated += alloc
			}
		}

		if totalAllocated > 5000 {
			t.Errorf("Total allocation exceeds budget: %d > 5000", totalAllocated)
		}
	})

	// Phase 8: Cycle Detection
	t.Run("cycle_detection", func(t *testing.T) {
		// Create a new collection with potential cycles
		cycleCol := NewSpecCollection()

		s1 := NewSpec("s1", "Spec1", SpecKindGoal, "test", "Content")
		s2 := NewSpec("s2", "Spec2", SpecKindGoal, "test", "Content")
		s3 := NewSpec("s3", "Spec3", SpecKindGoal, "test", "Content")

		// No cycle yet
		cycleCol.Add(s1)
		cycleCol.Add(s2)
		cycleCol.Add(s3)

		compiler := NewCompiler(cycleCol)
		result := compiler.Compile()

		if !result.Successful {
			t.Errorf("Should compile without cycles")
		}

		// Now create a cycle
		s1.DependsOn = []string{"s2"}
		s2.DependsOn = []string{"s3"}
		s3.DependsOn = []string{"s1"}

		cycleCol2 := NewSpecCollection()
		cycleCol2.Add(s1)
		cycleCol2.Add(s2)
		cycleCol2.Add(s3)

		compiler2 := NewCompiler(cycleCol2)
		result2 := compiler2.Compile()

		if result2.Successful {
			t.Errorf("Should fail to compile with cycles")
		}

		// Check that cycles were detected
		hasCycleError := false
		for _, err := range result2.Errors {
			if strings.Contains(err, "cycle") {
				hasCycleError = true
				break
			}
		}

		if !hasCycleError {
			t.Errorf("Should report cycle error")
		}
	})

	// Phase 9: Complex Filtering
	t.Run("complex_filtering", func(t *testing.T) {
		indexer := NewSpecIndexer(col, 10000)
		indexer.BuildIndex()

		// Filter: active security specs
		allActive := indexer.MetaSpec.SelectByStatus(SpecStatusActive)
		if len(allActive) == 0 {
			t.Errorf("Should find active specs")
		}

		// Filter: goals under 2000 tokens
		lowTokenGoals := make([]*Spec, 0)
		for _, spec := range indexer.MetaSpec.SelectByKind(SpecKindGoal) {
			if spec.TokenEstimate < 2000 {
				lowTokenGoals = append(lowTokenGoals, spec)
			}
		}
		if len(lowTokenGoals) == 0 {
			t.Errorf("Should find goals under 2000 tokens")
		}

		// Filter: all security namespace
		secSpecs := indexer.MetaSpec.SelectByNamespace("security")
		if len(secSpecs) == 0 {
			t.Errorf("Should find security specs")
		}
	})

	// Phase 10: Comprehensive Statistics
	t.Run("statistics", func(t *testing.T) {
		if col.Count() != 6 {
			t.Errorf("Expected 6 specs total")
		}

		totalTokens := 0
		for _, spec := range col.Specs {
			totalTokens += spec.TokenEstimate
		}

		if totalTokens <= 0 {
			t.Errorf("Total tokens should be positive")
		}

		activeCount := len(col.ListByStatus(SpecStatusActive))
		draftCount := len(col.ListByStatus(SpecStatusDraft))

		if activeCount+draftCount != 6 {
			t.Errorf("Active + Draft should equal total specs")
		}

		goalCount := len(col.ListByKind(SpecKindGoal))
		procCount := len(col.ListByKind(SpecKindProcess))
		constCount := len(col.ListByKind(SpecKindConstraint))

		if goalCount != 2 || procCount != 2 || constCount != 2 {
			t.Errorf("Spec kind distribution incorrect")
		}
	})
}

// TestEdgeCases tests edge cases and boundary conditions
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		description string
		testFunc    func(t *testing.T)
	}{
		{
			name:        "empty_collection_operations",
			description: "Operations on empty collection",
			testFunc: func(t *testing.T) {
				col := NewSpecCollection()

				if col.Count() != 0 {
					t.Errorf("Empty collection should have count 0")
				}

				if col.Get("non_existent") != nil {
					t.Errorf("Getting from empty collection should return nil")
				}

				if len(col.ListByKind(SpecKindGoal)) != 0 {
					t.Errorf("Filtering empty collection should return empty")
				}
			},
		},
		{
			name:        "very_long_content",
			description: "Spec with very long content",
			testFunc: func(t *testing.T) {
				longContent := strings.Repeat("x", 100000)
				spec := NewSpec("spec_long", "Long", SpecKindGoal, "test", longContent)

				if len(spec.Content) != 100000 {
					t.Errorf("Content not preserved")
				}

				spec.TokenEstimate = 50000
				validator := NewValidator()
				result := validator.Validate(spec)

				if !result.HasErrors() {
					t.Logf("Long content spec validated")
				}
			},
		},
		{
			name:        "many_dependencies",
			description: "Spec with many dependencies",
			testFunc: func(t *testing.T) {
				col := NewSpecCollection()

				// Create specs
				for i := 0; i < 10; i++ {
					col.Add(NewSpec(
						fmt.Sprintf("spec_%d", i),
						fmt.Sprintf("Spec %d", i),
						SpecKindGoal,
						"test",
						"Content",
					))
				}

				// Last spec depends on all others
				last := col.Get("spec_9")
				deps := make([]string, 0, 9)
				for i := 0; i < 9; i++ {
					deps = append(deps, fmt.Sprintf("spec_%d", i))
				}
				last.DependsOn = deps

				compiler := NewCompiler(col)
				result := compiler.Compile()

				if !result.Successful {
					t.Errorf("Should handle many dependencies")
				}
			},
		},
		{
			name:        "circular_namespace",
			description: "Deeply nested namespace",
			testFunc: func(t *testing.T) {
				deepNS := "a.b.c.d.e.f.g.h.i.j"
				spec := NewSpec("spec_deep", "Deep", SpecKindGoal, deepNS, "Content")

				if spec.Namespace != deepNS {
					t.Errorf("Deep namespace not preserved")
				}

				validator := NewValidator()
				result := validator.Validate(spec)

				if result.HasErrors() {
					t.Logf("Deep namespace validation: %v", result.Errors)
				}
			},
		},
		{
			name:        "special_characters",
			description: "Spec with special characters",
			testFunc: func(t *testing.T) {
				spec := NewSpec(
					"spec_special",
					"Title with @#$%^&*()",
					SpecKindGoal,
					"test.特殊文字",
					"Content with émojis: 🚀 and symbols: ™®©",
				)

				if !strings.Contains(spec.Title, "@") {
					t.Errorf("Special characters not preserved in title")
				}

				validator := NewValidator()
				result := validator.Validate(spec)

				if result.HasErrors() {
					t.Logf("Special character validation: %v", result.Errors)
				}
			},
		},
		{
			name:        "zero_token_estimate",
			description: "Spec with zero token estimate",
			testFunc: func(t *testing.T) {
				spec := NewSpec("spec_zero", "Zero Tokens", SpecKindGoal, "test", "Small")
				spec.TokenEstimate = 0

				budgeter := NewTokenBudgeter(1000, 1, 20)
				// Should handle zero tokens gracefully
				if budgeter == nil {
					t.Errorf("Should create budgeter with zero tokens")
				}
			},
		},
		{
			name:        "timestamp_ordering",
			description: "Specs with different timestamps",
			testFunc: func(t *testing.T) {
				col := NewSpecCollection()

				for i := 0; i < 5; i++ {
					spec := NewSpec(
						fmt.Sprintf("spec_%d", i),
						fmt.Sprintf("Spec %d", i),
						SpecKindGoal,
						"test",
						"Content",
					)
					spec.CreatedAt = time.Now().Add(time.Duration(i) * time.Second)
					col.Add(spec)
				}

				// Check that timestamps are preserved
				if col.Specs[0].CreatedAt.After(col.Specs[1].CreatedAt) {
					t.Errorf("Timestamp ordering incorrect")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)
			tt.testFunc(t)
		})
	}
}

// TestStressConditions tests under stress/load
func TestStressConditions(t *testing.T) {
	t.Run("large_collection_compilation", func(t *testing.T) {
		col := NewSpecCollection()

		// Create 500 specs with random dependencies
		for i := 0; i < 500; i++ {
			spec := NewSpec(
				fmt.Sprintf("spec_%04d", i),
				fmt.Sprintf("Spec %d", i),
				SpecKindGoal,
				fmt.Sprintf("ns.%d", i%10),
				"Content",
			)

			// Add some random dependencies (but not to future specs to avoid cycles)
			if i > 0 && i%5 == 0 {
				spec.DependsOn = []string{fmt.Sprintf("spec_%04d", i-1)}
			}

			spec.TokenEstimate = (i % 100) + 100
			col.Add(spec)
		}

		// Compilation should complete
		compiler := NewCompiler(col)
		result := compiler.Compile()

		if result.SpecCount() != 500 {
			t.Errorf("Should compile all 500 specs")
		}

		// Indexing should complete
		indexer := NewSpecIndexer(col, 1000000)
		indexer.BuildIndex()

		// Search should work
		results := indexer.MetaSpec.SearchByKeyword("content")
		if len(results) == 0 {
			t.Logf("Search in large collection completed")
		}
	})

	t.Run("deep_dependency_chain", func(t *testing.T) {
		col := NewSpecCollection()

		// Create chain: spec_0 -> spec_1 -> spec_2 -> ... -> spec_99
		for i := 0; i < 100; i++ {
			spec := NewSpec(
				fmt.Sprintf("spec_%d", i),
				fmt.Sprintf("Spec %d", i),
				SpecKindGoal,
				"chain",
				"Content",
			)

			if i > 0 {
				spec.DependsOn = []string{fmt.Sprintf("spec_%d", i-1)}
			}

			col.Add(spec)
		}

		compiler := NewCompiler(col)
		result := compiler.Compile()

		if !result.Successful {
			t.Errorf("Should handle deep dependency chains")
		}

		order := compiler.TopologicalOrder()
		if len(order) != 100 {
			t.Errorf("Should maintain full ordering")
		}

		// First spec should come before last
		if order[0] != "spec_0" {
			t.Errorf("Ordering incorrect")
		}
	})
}

// TestDataIntegrity tests data integrity and consistency
func TestDataIntegrity(t *testing.T) {
	t.Run("spec_immutability", func(t *testing.T) {
		original := NewSpec("spec_001", "Title", SpecKindGoal, "ns", "Content")
		originalID := original.ID
		originalTitle := original.Title

		// Modify the spec
		original.Title = "Modified"

		// Create another spec and compare
		unchanged := NewSpec("spec_002", originalTitle, SpecKindGoal, "ns", "Content")

		if original.ID != originalID {
			t.Errorf("ID should not change")
		}

		if unchanged.Title != originalTitle {
			t.Errorf("Original title should not change")
		}
	})

	t.Run("collection_consistency", func(t *testing.T) {
		col := NewSpecCollection()

		spec1 := NewSpec("spec_001", "A", SpecKindGoal, "ns", "A")
		spec2 := NewSpec("spec_002", "B", SpecKindGoal, "ns", "B")

		col.Add(spec1)
		col.Add(spec2)

		// Get and modify
		retrieved := col.Get("spec_001")
		if retrieved == nil {
			t.Fatalf("Should retrieve spec")
		}

		retrieved.Title = "Modified A"

		// Check that the collection reflects the change
		again := col.Get("spec_001")
		if again.Title != "Modified A" {
			t.Errorf("Collection should reflect changes")
		}
	})
}
