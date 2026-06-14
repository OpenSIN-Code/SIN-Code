// SPDX-License-Identifier: MIT
// Purpose: Example usage and test patterns for the Spec Layer (Issue #122).
// Demonstrates all phases: Spectr, SpecD, SDLC, MetaSpec, SpecKit.
// Docs: internal/spec/examples_test.go
package spec

import (
	"testing"
	"time"
)

// ExampleSpecCreation demonstrates basic spec creation and validation.
func TestExampleSpecCreation(t *testing.T) {
	// Create a spec
	spec := NewSpec("spec_auth_001", "User Authentication System", SpecKindGoal)
	spec.Description = "# Authentication\n\nProvide secure user authentication..."
	spec.Goals = "- Support email+password login\n- Support OAuth 2.0\n- Secure session management"
	spec.Constraints = "- No password reuse\n- Force HTTPS\n- Rate limit login attempts"
	spec.TokenEstimate = 5000
	spec.Priority = 8

	// Validate
	result := ValidateSpec(spec)
	if !result.Valid {
		t.Fatalf("Spec validation failed: %v", result.Errors)
	}

	// Archive
	archive := spec.Archive("Version 2 now active")
	if archive.Reason != "Version 2 now active" {
		t.Fatalf("Archive reason mismatch")
	}
}

// ExampleCollectionAndGraph demonstrates collection building and compilation.
func TestExampleCollectionAndGraph(t *testing.T) {
	// Create collection
	collection := NewCollection("project_root", "My SIN-Code Project")

	// Create interdependent specs
	spec1 := NewSpec("spec_auth_001", "Authentication", SpecKindComponent)
	spec1.Status = SpecStatusActive
	spec1.TokenEstimate = 3000
	spec1.Priority = 9

	spec2 := NewSpec("spec_session_001", "Session Management", SpecKindProcess)
	spec2.Status = SpecStatusActive
	spec2.Dependencies = []string{"spec_auth_001"}
	spec2.TokenEstimate = 2000
	spec2.Priority = 8

	spec3 := NewSpec("spec_api_001", "REST API", SpecKindComponent)
	spec3.Status = SpecStatusActive
	spec3.Dependencies = []string{"spec_auth_001", "spec_session_001"}
	spec3.TokenEstimate = 4000
	spec3.Priority = 7

	collection.AddSpec(spec1)
	collection.AddSpec(spec2)
	collection.AddSpec(spec3)

	// Compile
	compiler := NewCompiler(collection)
	compileResult := compiler.Compile()

	if !compileResult.Success {
		t.Fatalf("Compilation failed: %v", compileResult.Errors)
	}

	// Check topological order
	order := compiler.TopologicalOrder()
	if len(order) != 3 {
		t.Fatalf("Expected 3 specs in topological order, got %d", len(order))
	}

	// First should be auth (no dependencies)
	if order[0].ID != "spec_auth_001" {
		t.Fatalf("Expected spec_auth_001 first, got %s", order[0].ID)
	}

	// Check cost estimation
	apiCost := compiler.EstimateCost("spec_api_001")
	expectedCost := 3000 + 2000 + 4000 // auth + session + api
	if apiCost != expectedCost {
		t.Fatalf("Expected cost %d, got %d", expectedCost, apiCost)
	}
}

// ExampleValidationAndGates demonstrates validation and quality gates.
func TestExampleValidationAndGates(t *testing.T) {
	// Create spec
	spec := NewSpec("spec_test_001", "Test Spec", SpecKindGoal)
	spec.Status = SpecStatusActive
	spec.Description = "Test description"
	spec.Goals = "- Goal 1\n- Goal 2"
	spec.TokenEstimate = 5000

	// Validate basic structure
	result := ValidateSpec(spec)
	if !result.Valid {
		t.Fatalf("Basic validation failed")
	}

	// Create collection and run gates
	collection := NewCollection("test", "Test Collection")
	collection.AddSpec(spec)

	registry := NewGateRegistry()
	verifyCtx := &VerificationContext{
		Collection:  collection,
		TokenBudget: 100000,
	}

	verifyResult := registry.Run(spec, verifyCtx)
	if !verifyResult.Passed {
		t.Fatalf("Gate verification failed: %v", verifyResult.Results)
	}

	// Check token budget gate
	if tokenGate, ok := verifyResult.Results[string(GateNameTokenBudget)]; ok {
		if !tokenGate.Passed {
			t.Fatalf("Token budget gate failed")
		}
	}
}

// ExampleMergeConflict demonstrates three-way merge with conflict resolution.
func TestExampleMergeConflict(t *testing.T) {
	// Create base spec
	base := NewSpec("spec_merge_001", "Base", SpecKindGoal)
	base.Description = "Original description"
	base.Goals = "Original goals"

	// Create ours (our changes)
	ours := *base
	ours.Description = "Our updated description"
	ours.UpdatedAt = time.Now().Add(1 * time.Second)
	ours.Version = 2

	// Create theirs (their changes)
	theirs := *base
	theirs.Goals = "Their updated goals"
	theirs.UpdatedAt = time.Now().Add(2 * time.Second)
	theirs.Version = 2

	// Merge with "theirs" strategy
	result := MergeSpecs(&base, &ours, &theirs, StrategyTheirs)

	if !result.Successful {
		t.Fatalf("Merge failed: %v", result.Conflicts)
	}

	// Check merged result
	if result.Merged.Goals != "Their updated goals" {
		t.Fatalf("Goals not merged correctly")
	}
}

// ExampleMetaSpecIndexing demonstrates indexing and search.
func TestExampleMetaSpecIndexing(t *testing.T) {
	// Create collection with specs
	collection := NewCollection("test", "Test")

	spec1 := NewSpec("spec_auth_001", "User Authentication System", SpecKindComponent)
	spec1.Description = "Provides secure authentication mechanisms"
	spec1.Namespace = "security"
	spec1.Status = SpecStatusActive
	spec1.Priority = 9
	spec1.TokenEstimate = 5000

	spec2 := NewSpec("spec_payment_001", "Payment Processing", SpecKindProcess)
	spec2.Description = "Handles payment transactions and settlements"
	spec2.Namespace = "payments"
	spec2.Status = SpecStatusActive
	spec2.Priority = 8
	spec2.TokenEstimate = 4000

	collection.AddSpec(spec1)
	collection.AddSpec(spec2)

	// Build index
	indexer := NewSpecIndexer(collection, 100000)
	indexer.BuildIndex()

	// Search
	results := indexer.MetaSpec.SearchByKeyword("authentication")
	if len(results) == 0 {
		t.Fatalf("Search should find authentication spec")
	}

	// Select by budget
	selected := indexer.MetaSpec.SelectByBudget(50000, 10)
	if len(selected) == 0 {
		t.Fatalf("Should select specs within budget")
	}

	// Select by namespace
	securitySpecs := indexer.MetaSpec.SelectByNamespace("security")
	if len(securitySpecs) != 1 || securitySpecs[0].SpecID != "spec_auth_001" {
		t.Fatalf("Should find security specs")
	}
}

// ExampleTokenBudgeting demonstrates token allocation strategies.
func TestExampleTokenBudgeting(t *testing.T) {
	specs := []*Spec{
		{ID: "spec_1", TokenEstimate: 1000, Priority: 5},
		{ID: "spec_2", TokenEstimate: 2000, Priority: 8},
		{ID: "spec_3", TokenEstimate: 1500, Priority: 3},
	}

	// Proportional allocation
	budgeter := NewTokenBudgeter(10000, 3, 20) // 20% reserve = 2000 reserved, 8000 available
	proportional := budgeter.AllocateProportional(specs)

	totalAllocated := 0
	for _, amount := range proportional {
		totalAllocated += amount
	}
	if totalAllocated != 8000 {
		t.Fatalf("Expected 8000 tokens allocated, got %d", totalAllocated)
	}

	// Priority allocation
	priority := budgeter.AllocatePriority(specs)
	
	// spec_2 has highest priority (8), should get more
	if priority[specs[1].ID] <= priority[specs[0].ID] {
		t.Fatalf("Higher priority spec should get more tokens")
	}
}

// ExampleSpecKit demonstrates chat integration.
func TestExampleSpecKit(t *testing.T) {
	// Create collection
	collection := NewCollection("test", "Test")

	spec := NewSpec("spec_test_001", "Test Goal", SpecKindGoal)
	spec.Status = SpecStatusActive
	spec.Description = "Test description"
	spec.Goals = "- Goal 1\n- Goal 2"
	spec.Priority = 8
	collection.AddSpec(spec)

	// Create SpecKit
	kit := NewSpecKit(collection)

	// Execute /spec list command
	ctx := &CommandContext{
		Args:       []string{"spec", "list"},
		Collection: collection,
		Session:    make(map[string]interface{}),
	}

	result, err := kit.Execute(ctx)
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	if result == "" {
		t.Fatalf("Expected output from /spec list")
	}

	// Execute /help command
	ctx.Args = []string{"help"}
	result, err = kit.Execute(ctx)
	if err != nil {
		t.Fatalf("Help command failed: %v", err)
	}

	if result == "" {
		t.Fatalf("Expected help output")
	}
}

// ExampleEndToEnd demonstrates full workflow from creation to verification.
func TestExampleEndToEnd(t *testing.T) {
	// 1. Create collection
	collection := NewCollection("my_project", "My SIN-Code Project")

	// 2. Create interdependent specs
	authSpec := NewSpec("spec_auth_001", "User Authentication", SpecKindComponent)
	authSpec.Description = "Provides user authentication"
	authSpec.Goals = "- Support email/password\n- Support OAuth"
	authSpec.Constraints = "- HTTPS only\n- Rate limit login"
	authSpec.Status = SpecStatusActive
	authSpec.TokenEstimate = 5000
	authSpec.Priority = 9

	apiSpec := NewSpec("spec_api_001", "REST API", SpecKindComponent)
	apiSpec.Description = "REST API server"
	apiSpec.Goals = "- RESTful endpoints\n- Error handling"
	apiSpec.Dependencies = []string{"spec_auth_001"}
	apiSpec.Status = SpecStatusActive
	apiSpec.TokenEstimate = 4000
	apiSpec.Priority = 8

	collection.AddSpec(authSpec)
	collection.AddSpec(apiSpec)

	// 3. Validate
	if !ValidateSpec(authSpec).Valid {
		t.Fatalf("Auth spec validation failed")
	}

	// 4. Compile
	compiler := NewCompiler(collection)
	compileResult := compiler.Compile()
	if !compileResult.Success {
		t.Fatalf("Compilation failed")
	}

	// 5. Run gates
	registry := NewGateRegistry()
	verifyCtx := &VerificationContext{Collection: collection, TokenBudget: 100000}
	verifyResult := registry.Run(apiSpec, verifyCtx)
	if !verifyResult.Passed {
		t.Fatalf("Verification failed")
	}

	// 6. Build index
	indexer := NewSpecIndexer(collection, 100000)
	indexer.BuildIndex()

	// 7. Search
	results := indexer.MetaSpec.SearchByKeyword("authentication")
	if len(results) == 0 {
		t.Fatalf("Search failed")
	}

	// 8. Allocate budget
	budgeter := NewTokenBudgeter(20000, 2, 10)
	allocation := budgeter.AllocatePriority([]*Spec{authSpec, apiSpec})
	if allocation[authSpec.ID] <= allocation[apiSpec.ID] {
		t.Fatalf("Higher priority spec should get more tokens")
	}

	// 9. Chat commands
	kit := NewSpecKit(collection)
	ctx := &CommandContext{
		Args:       []string{"spec", "show", "spec_auth_001"},
		Collection: collection,
		Session:    make(map[string]interface{}),
	}
	output, err := kit.Execute(ctx)
	if err != nil || output == "" {
		t.Fatalf("Chat command failed")
	}
}
