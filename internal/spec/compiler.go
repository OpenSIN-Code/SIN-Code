// SPDX-License-Identifier: MIT
// Purpose: Spec compiler for dependency graph building, topological sorting,
// and static analysis. All compilation is deterministic and LLM-free (Phase 2: SpecD).
// Docs: internal/spec/compiler.go.doc.md
package spec

import (
	"fmt"
	"sort"
	"time"
)

// CompileError represents a compilation error during spec compilation.
type CompileError struct {
	SpecID  string
	Message string
	Phase   string // "graph_build", "topo_sort", "metadata", etc.
}

// CompilerResult holds the outcome of spec compilation.
type CompilerResult struct {
	Success      bool
	Errors       []CompileError
	Warnings     []string
	CompiledAt   time.Time
	Stats        *CompileStats
}

// CompileStats holds statistics about the compilation process.
type CompileStats struct {
	SpecsCompiled     int
	SpecsFailed       int
	TotalDependencies int
	MaxDepth          int
	CyclesDetected    int
	CompilationTimeMs int64
}

// Compiler orchestrates the full spec compilation pipeline.
type Compiler struct {
	Collection *SpecCollection
	Graph      *DependencyGraph
	Errors     []CompileError
	Warnings   []string
}

// NewCompiler creates a new Compiler for the given collection.
func NewCompiler(collection *SpecCollection) *Compiler {
	return &Compiler{
		Collection: collection,
		Graph:      collection.Graph,
		Errors:     []CompileError{},
		Warnings:   []string{},
	}
}

// Compile executes the full compilation pipeline and updates all specs.
func (c *Compiler) Compile() *CompilerResult {
	startTime := time.Now()
	result := &CompilerResult{
		CompiledAt: startTime,
		Stats:      &CompileStats{},
	}

	// Phase 1: Build dependency graph
	if !c.buildGraph() {
		result.Success = false
		result.Errors = c.Errors
		result.Warnings = c.Warnings
		return result
	}

	// Phase 2: Topological sort
	if !c.topologicalSort() {
		result.Success = false
		result.Errors = c.Errors
		result.Warnings = c.Warnings
		return result
	}

	// Phase 3: Compute static metadata
	c.computeMetadata()

	// Phase 4: Validate compiled state
	c.validateCompilation()

	// Count successes/failures
	result.Stats.SpecsCompiled = len(c.Collection.Specs)
	result.Stats.SpecsFailed = len(c.Errors)
	result.Stats.TotalDependencies = len(c.Graph.Edges)
	result.Stats.MaxDepth = c.findMaxDepth()
	result.Stats.CompilationTimeMs = time.Since(startTime).Milliseconds()

	// Mark specs as compiled
	for _, spec := range c.Collection.Specs {
		if !c.hasErrors(spec.ID) {
			now := time.Now()
			spec.CompiledAt = &now
			spec.Compiled = true
		}
	}

	result.Success = len(c.Errors) == 0
	result.Errors = c.Errors
	result.Warnings = c.Warnings

	return result
}

// buildGraph constructs the dependency graph from specs in the collection.
func (c *Compiler) buildGraph() bool {
	c.Graph.Nodes = make(map[string]*GraphNode)
	c.Graph.Edges = []GraphEdge{}

	// Create nodes for each spec
	for id, spec := range c.Collection.Specs {
		c.Graph.Nodes[id] = &GraphNode{
			SpecID:       id,
			Kind:         spec.Kind,
			Dependencies: spec.Dependencies,
			Dependents:   []string{},
		}
	}

	// Build edges and validate references
	for id, spec := range c.Collection.Specs {
		for _, depID := range spec.Dependencies {
			// Validate dependency exists
			if _, exists := c.Collection.Specs[depID]; !exists {
				c.addError(id, fmt.Sprintf("undefined dependency: %s", depID), "graph_build")
				return false
			}

			// Add edge
			c.Graph.Edges = append(c.Graph.Edges, GraphEdge{
				From:   id,
				To:     depID,
				Weight: 1,
			})

			// Update dependents list
			if node, ok := c.Graph.Nodes[depID]; ok {
				node.Dependents = append(node.Dependents, id)
			}
		}
	}

	return true
}

// topologicalSort performs topological sorting and detects cycles.
func (c *Compiler) topologicalSort() bool {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)
	depths := make(map[string]int)

	var visit func(string) bool
	visit = func(id string) bool {
		if recStack[id] {
			// Cycle detected
			c.addError(id, "cycle detected in dependency graph", "topo_sort")
			c.Collection.Statistics.TotalTokenEstimate-- // Penalize cycle
			return false
		}

		if visited[id] {
			return true // Already processed
		}

		visited[id] = true
		recStack[id] = true

		// Visit all dependencies
		for _, depID := range c.Graph.Nodes[id].Dependencies {
			if !visit(depID) {
				return false
			}
			// Update depth
			if depths[depID]+1 > depths[id] {
				depths[id] = depths[depID] + 1
			}
		}

		recStack[id] = false
		return true
	}

	// Visit all nodes
	for id := range c.Graph.Nodes {
		if !visited[id] {
			if !visit(id) {
				return false
			}
		}
	}

	// Assign depths to nodes
	for id, depth := range depths {
		c.Graph.Nodes[id].Depth = depth
	}

	return len(c.Errors) == 0
}

// computeMetadata calculates static metadata for each spec and the graph.
func (c *Compiler) computeMetadata() {
	// Update collection statistics
	c.Collection.Statistics.MaxDepth = 0

	for id, node := range c.Graph.Nodes {
		spec := c.Collection.Specs[id]
		if spec == nil {
			continue
		}

		// Update depth
		if node.Depth > c.Collection.Statistics.MaxDepth {
			c.Collection.Statistics.MaxDepth = node.Depth
		}

		// Recompute hash
		spec.Hash = spec.ComputeHash()

		// Update timestamps
		spec.UpdatedAt = time.Now()
	}
}

// validateCompilation performs post-compilation validation.
func (c *Compiler) validateCompilation() {
	// Validate all specs pass structural checks
	for id, spec := range c.Collection.Specs {
		result := ValidateSpec(spec)
		if !result.Valid {
			for _, err := range result.Errors {
				c.addWarning(fmt.Sprintf("[%s] %s", id, err.Message))
			}
		}
	}

	// Validate overall dependency structure
	depResult := ValidateDependencies(c.Collection)
	if !depResult.Valid {
		for _, err := range depResult.Errors {
			c.addError(err.SpecID, err.Message, "validate_deps")
		}
	}
}

// findMaxDepth returns the maximum depth in the dependency graph.
func (c *Compiler) findMaxDepth() int {
	maxDepth := 0
	for _, node := range c.Graph.Nodes {
		if node.Depth > maxDepth {
			maxDepth = node.Depth
		}
	}
	return maxDepth
}

// hasErrors checks if a spec has compilation errors.
func (c *Compiler) hasErrors(specID string) bool {
	for _, err := range c.Errors {
		if err.SpecID == specID {
			return true
		}
	}
	return false
}

// addError adds a compilation error.
func (c *Compiler) addError(specID, message, phase string) {
	c.Errors = append(c.Errors, CompileError{
		SpecID:  specID,
		Message: message,
		Phase:   phase,
	})
}

// addWarning adds a compilation warning.
func (c *Compiler) addWarning(message string) {
	c.Warnings = append(c.Warnings, message)
}

// String returns a human-readable summary of the compiler result.
func (cr *CompilerResult) String() string {
	if cr.Success {
		return fmt.Sprintf("✓ Compilation successful: %d specs, max depth %d (%.0fms)",
			cr.Stats.SpecsCompiled, cr.Stats.MaxDepth, float64(cr.Stats.CompilationTimeMs))
	}
	return fmt.Sprintf("✗ Compilation failed: %d errors, %d specs",
		len(cr.Errors), cr.Stats.SpecsFailed)
}

// Details returns a detailed multi-line error report.
func (cr *CompilerResult) Details() string {
	var result string
	result = cr.String() + "\n"

	if len(cr.Errors) > 0 {
		result += fmt.Sprintf("\nErrors (%d):\n", len(cr.Errors))
		for _, err := range cr.Errors {
			result += fmt.Sprintf("  [%s/%s] %s\n", err.SpecID, err.Phase, err.Message)
		}
	}

	if len(cr.Warnings) > 0 {
		result += fmt.Sprintf("\nWarnings (%d):\n", len(cr.Warnings))
		for _, w := range cr.Warnings {
			result += fmt.Sprintf("  ⚠ %s\n", w)
		}
	}

	return result
}

// TopologicalOrder returns all specs in topological order (dependencies first).
// Useful for processing specs in dependency order.
func (c *Compiler) TopologicalOrder() []*Spec {
	order := make([]*Spec, 0, len(c.Collection.Specs))

	// Create a map of in-degrees (number of incoming edges)
	inDegree := make(map[string]int)
	for id := range c.Collection.Specs {
		inDegree[id] = 0
	}

	for _, edge := range c.Graph.Edges {
		inDegree[edge.From]++
	}

	// Use Kahn's algorithm for topological sort
	queue := make([]string, 0)
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	// Sort queue for deterministic output
	sort.Strings(queue)

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		order = append(order, c.Collection.Specs[id])

		// Process dependents
		for _, edge := range c.Graph.Edges {
			if edge.From == id {
				inDegree[edge.To]--
				if inDegree[edge.To] == 0 {
					queue = append(queue, edge.To)
					sort.Strings(queue)
				}
			}
		}
	}

	return order
}

// FindDependencyChain returns the full dependency chain for a spec.
// Returns all specs (recursively) that this spec depends on.
func (c *Compiler) FindDependencyChain(specID string) []*Spec {
	visited := make(map[string]bool)
	var chain []*Spec

	var traverse func(string)
	traverse = func(id string) {
		if visited[id] {
			return
		}
		visited[id] = true

		spec := c.Collection.Specs[id]
		if spec != nil {
			chain = append(chain, spec)
		}

		// Traverse dependencies
		for _, depID := range c.Graph.Nodes[id].Dependencies {
			traverse(depID)
		}
	}

	traverse(specID)
	return chain
}

// EstimateCost estimates the total compilation cost for a spec and its dependencies.
func (c *Compiler) EstimateCost(specID string) int {
	chain := c.FindDependencyChain(specID)
	totalCost := 0
	for _, spec := range chain {
		totalCost += spec.TokenEstimate
	}
	return totalCost
}
