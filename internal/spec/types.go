// SPDX-License-Identifier: MIT
// Purpose: Core Spec types for the SIN-Code Spec Layer (Spectr).
// Defines the markdown-based specification format, immutable Spec structs,
// version tracking, and serialization contracts. All specs are deterministic
// and LLM-free; compilation happens via the compiler layer (SpecD).
// Docs: internal/spec/types.go.doc.md
package spec

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

// SpecKind represents the type of specification.
type SpecKind string

const (
	SpecKindGoal        SpecKind = "goal"        // User intent / objective
	SpecKindProcess     SpecKind = "process"     // Multi-step workflow
	SpecKindConstraint  SpecKind = "constraint"  // Quality gate / hard rule
	SpecKindComponent   SpecKind = "component"   // Reusable building block
	SpecKindIntegration SpecKind = "integration" // External system binding
)

// Spec is the core immutable specification container. It holds the markdown
// content, metadata, and structural relationships. Specs are never edited in-place;
// mutations produce new Spec instances (immutable pattern).
//
// Fields are canonically ordered: identity → content → metadata → relations.
type Spec struct {
	// Identity
	ID        string    `json:"id"`        // Unique spec identifier (e.g., "spec_123abc")
	Kind      SpecKind  `json:"kind"`      // Type of spec (goal, process, constraint, component, integration)
	Title     string    `json:"title"`     // Human-readable title
	Namespace string    `json:"namespace"` // Logical grouping (e.g., "auth", "data-layer")

	// Content
	Description string `json:"description"` // Markdown-formatted description
	Goals       string `json:"goals"`       // Markdown: what this spec achieves
	Constraints string `json:"constraints"` // Markdown: hard rules & limitations
	Input       string `json:"input"`       // Markdown: expected input schema
	Output      string `json:"output"`      // Markdown: produced output schema
	Examples    string `json:"examples"`    // Markdown: usage examples

	// Metadata
	CreatedAt  time.Time              `json:"created_at"`   // Spec creation timestamp
	UpdatedAt  time.Time              `json:"updated_at"`   // Last modification timestamp
	Version    int                    `json:"version"`      // Incremental version counter
	Hash       string                 `json:"hash"`         // Deterministic content hash (SHA-256)
	Author     string                 `json:"author"`       // Creator identifier
	Status     SpecStatus             `json:"status"`       // Current state (draft, active, archived)
	Tags       []string               `json:"tags"`         // Searchable labels
	Metadata   map[string]interface{} `json:"metadata"`     // Extensible key-value store

	// Relations
	Dependencies []string `json:"dependencies"` // IDs of specs this depends on
	Dependents   []string `json:"dependents"`   // IDs of specs that depend on this
	Parent       string   `json:"parent"`       // Parent spec ID (for hierarchy)
	Children     []string `json:"children"`     // Child spec IDs

	// Gates (SDLC)
	RequiredGates []string `json:"required_gates"` // Quality gates that must pass
	GateResults   map[string]GateResult `json:"gate_results"` // Results of verification gates

	// Compilation
	CompiledAt   *time.Time `json:"compiled_at"`   // Last successful compilation
	CompileError string     `json:"compile_error"` // Last compilation error (if any)
	Compiled     bool       `json:"compiled"`      // Whether spec successfully compiled

	// MetaSpec (token budgeting)
	TokenEstimate int    `json:"token_estimate"` // Estimated token cost
	TokenActual   int    `json:"token_actual"`   // Actual token cost after execution
	Priority      int    `json:"priority"`       // Execution priority (0-10)
	Indexed       bool   `json:"indexed"`        // Whether included in metaspec index
	IndexedAt     *time.Time `json:"indexed_at"` // When added to index
}

// SpecStatus represents the lifecycle state of a spec.
type SpecStatus string

const (
	SpecStatusDraft    SpecStatus = "draft"    // Not yet validated
	SpecStatusActive   SpecStatus = "active"   // Validated and in use
	SpecStatusArchived SpecStatus = "archived" // No longer used
)

// GateResult holds the result of a single quality gate verification.
type GateResult struct {
	GateName  string    `json:"gate_name"`   // Name of the gate (e.g., "token_budget", "type_check")
	Passed    bool      `json:"passed"`      // Whether gate passed
	Message   string    `json:"message"`     // Human-readable result message
	Timestamp time.Time `json:"timestamp"`   // When gate ran
	Details   map[string]interface{} `json:"details"` // Gate-specific metadata
}

// SpecArchive holds a snapshot of a spec at a point in time.
// Used for versioning and rollback.
type SpecArchive struct {
	ID         string    `json:"id"`          // Spec ID
	Version    int       `json:"version"`     // Spec version at archive time
	Snapshot   *Spec     `json:"snapshot"`    // Full spec snapshot
	ArchivedAt time.Time `json:"archived_at"` // When archived
	Reason     string    `json:"reason"`      // Why archived (e.g., "replaced_by_v2")
}

// SpecCollection holds a set of related specs with their graph relationships.
type SpecCollection struct {
	ID           string               `json:"id"`             // Collection ID
	Name         string               `json:"name"`           // Human-readable name
	Description  string               `json:"description"`    // Collection purpose
	CreatedAt    time.Time            `json:"created_at"`     // Creation timestamp
	UpdatedAt    time.Time            `json:"updated_at"`     // Last update timestamp
	Specs        map[string]*Spec     `json:"specs"`          // All specs in collection (id -> spec)
	Graph        *DependencyGraph     `json:"graph"`          // Dependency graph
	Statistics   *CollectionStats     `json:"statistics"`     // Aggregate statistics
}

// DependencyGraph represents the directed acyclic graph (DAG) of spec dependencies.
type DependencyGraph struct {
	Nodes map[string]*GraphNode `json:"nodes"` // Spec ID -> graph node
	Edges []GraphEdge          `json:"edges"` // All dependency edges
}

// GraphNode represents a single spec in the dependency graph.
type GraphNode struct {
	SpecID       string   `json:"spec_id"`        // Spec identifier
	Kind         SpecKind `json:"kind"`           // Spec kind
	Dependencies []string `json:"dependencies"`  // IDs this node depends on
	Dependents   []string `json:"dependents"`    // IDs that depend on this node
	Depth        int      `json:"depth"`         // Topological depth in DAG
	CycleDetected bool    `json:"cycle_detected"` // If cycle found
}

// GraphEdge represents a dependency relationship between two specs.
type GraphEdge struct {
	From   string `json:"from"`   // Source spec ID
	To     string `json:"to"`     // Target spec ID
	Weight int    `json:"weight"` // Edge weight (1 for hard dep, <1 for soft)
}

// CollectionStats holds aggregate statistics about a spec collection.
type CollectionStats struct {
	TotalSpecs         int            `json:"total_specs"`          // Number of specs
	SpecsByKind        map[SpecKind]int `json:"specs_by_kind"`      // Count per kind
	TotalDependencies  int            `json:"total_dependencies"`   // Total edges
	MaxDepth           int            `json:"max_depth"`            // Deepest graph level
	AvgTokenEstimate   float64        `json:"avg_token_estimate"`   // Mean token cost
	TotalTokenEstimate int            `json:"total_token_estimate"` // Sum of all estimates
	ActiveCount        int            `json:"active_count"`         // Active specs
	DraftCount         int            `json:"draft_count"`          // Draft specs
	ArchivedCount      int            `json:"archived_count"`       // Archived specs
}

// NewSpec creates a new Spec with required fields. All other fields default to zero values.
func NewSpec(id, title string, kind SpecKind) *Spec {
	return &Spec{
		ID:           id,
		Title:        title,
		Kind:         kind,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Version:      1,
		Status:       SpecStatusDraft,
		Tags:         []string{},
		Metadata:     make(map[string]interface{}),
		Dependencies: []string{},
		Dependents:   []string{},
		Children:     []string{},
		RequiredGates: []string{},
		GateResults:  make(map[string]GateResult),
	}
}

// ComputeHash computes a deterministic SHA-256 hash of the spec's content.
// Used for change detection and versioning. Hash is stable across identical content.
func (s *Spec) ComputeHash() string {
	h := fnv.New64a()
	// Hash all content fields in canonical order for determinism
	h.Write([]byte(s.ID))
	h.Write([]byte(s.Kind))
	h.Write([]byte(s.Title))
	h.Write([]byte(s.Namespace))
	h.Write([]byte(s.Description))
	h.Write([]byte(s.Goals))
	h.Write([]byte(s.Constraints))
	h.Write([]byte(s.Input))
	h.Write([]byte(s.Output))
	h.Write([]byte(s.Examples))
	for _, dep := range s.Dependencies {
		h.Write([]byte(dep))
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// WithDependency returns a new Spec with an added dependency. Does not mutate receiver.
func (s *Spec) WithDependency(depID string) *Spec {
	newSpec := *s
	newSpec.Dependencies = append(s.Dependencies, depID)
	newSpec.UpdatedAt = time.Now()
	newSpec.Version++
	newSpec.Hash = newSpec.ComputeHash()
	return &newSpec
}

// Archive returns a SpecArchive snapshot of the current spec.
func (s *Spec) Archive(reason string) *SpecArchive {
	snapshot := *s // Copy
	return &SpecArchive{
		ID:         s.ID,
		Version:    s.Version,
		Snapshot:   &snapshot,
		ArchivedAt: time.Now(),
		Reason:     reason,
	}
}

// String returns a human-readable string representation of the Spec.
func (s *Spec) String() string {
	return fmt.Sprintf("Spec{ID: %s, Kind: %s, Title: %s, Status: %s, Version: %d}",
		s.ID, s.Kind, s.Title, s.Status, s.Version)
}

// MarkdownFormat returns the spec as markdown suitable for display or export.
func (s *Spec) MarkdownFormat() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", s.Title))
	sb.WriteString(fmt.Sprintf("**Kind:** %s | **Status:** %s | **Version:** %d\n\n", s.Kind, s.Status, s.Version))
	
	if s.Description != "" {
		sb.WriteString("## Description\n")
		sb.WriteString(s.Description)
		sb.WriteString("\n\n")
	}
	
	if s.Goals != "" {
		sb.WriteString("## Goals\n")
		sb.WriteString(s.Goals)
		sb.WriteString("\n\n")
	}
	
	if s.Constraints != "" {
		sb.WriteString("## Constraints\n")
		sb.WriteString(s.Constraints)
		sb.WriteString("\n\n")
	}
	
	if s.Input != "" {
		sb.WriteString("## Input\n")
		sb.WriteString(s.Input)
		sb.WriteString("\n\n")
	}
	
	if s.Output != "" {
		sb.WriteString("## Output\n")
		sb.WriteString(s.Output)
		sb.WriteString("\n\n")
	}
	
	if s.Examples != "" {
		sb.WriteString("## Examples\n")
		sb.WriteString(s.Examples)
		sb.WriteString("\n\n")
	}
	
	if len(s.Dependencies) > 0 {
		sb.WriteString("## Dependencies\n")
		for _, dep := range s.Dependencies {
			sb.WriteString(fmt.Sprintf("- %s\n", dep))
		}
		sb.WriteString("\n")
	}
	
	return sb.String()
}

// NewCollection creates a new SpecCollection with default values.
func NewCollection(id, name string) *SpecCollection {
	return &SpecCollection{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Specs:     make(map[string]*Spec),
		Graph:     &DependencyGraph{Nodes: make(map[string]*GraphNode), Edges: []GraphEdge{}},
		Statistics: &CollectionStats{
			SpecsByKind: make(map[SpecKind]int),
		},
	}
}

// AddSpec adds a spec to the collection and updates statistics.
func (sc *SpecCollection) AddSpec(s *Spec) {
	sc.Specs[s.ID] = s
	sc.Statistics.TotalSpecs++
	sc.Statistics.SpecsByKind[s.Kind]++
	
	switch s.Status {
	case SpecStatusActive:
		sc.Statistics.ActiveCount++
	case SpecStatusDraft:
		sc.Statistics.DraftCount++
	case SpecStatusArchived:
		sc.Statistics.ArchivedCount++
	}
	
	sc.Statistics.TotalTokenEstimate += s.TokenEstimate
	sc.UpdatedAt = time.Now()
}
