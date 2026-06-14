# SIN-Code Spec Layer (Spectr) — Issue #122 Implementation

## Overview

The **Spec Layer** is a deterministic, markdown-based specification system integrated into SIN-Code. It provides a non-breaking way to encode project requirements, architecture decisions, and quality gates as executable specifications.

This implementation completes all **5 phases** of the Spec Layer roadmap (Issue #122):

- **Phase 1 (Spectr)**: Core spec backbone with types, validation, CLI ✓
- **Phase 2 (SpecD)**: Spec compiler with dependency graph ✓
- **Phase 3 (SDLC)**: Quality gates and verification hooks ✓
- **Phase 4 (MetaSpec)**: Token optimization and indexing ✓
- **Phase 5 (SpecKit)**: Chat integration with slash-commands ✓

## File Structure

```
internal/spec/
├── types.go          # Core types: Spec, SpecKind, DependencyGraph
├── validate.go       # Spec validation and error handling
├── merge.go          # Three-way merge with conflict resolution
├── compiler.go       # Dependency graph building and topological sort
├── gates.go          # Quality gates (token budget, markdown, dependencies, etc.)
├── metaspec.go       # Token optimization, indexing, search
├── speckit.go        # Chat integration with slash-commands
└── doc.go            # Package documentation

cmd/sin-code/
└── spec_cmd.go       # CLI commands: init, validate, create, archive, list, show, merge
```

## Key Features

### Phase 1: Core Backbone (Spectr)

**Type System**:
- `Spec`: Immutable specification container (ID, title, kind, status, description, goals, constraints, etc.)
- `SpecKind`: Enum (goal, process, constraint, component, integration)
- `SpecStatus`: Enum (draft, active, archived)
- `SpecCollection`: Set of related specs with dependency graph
- `DependencyGraph`: Directed acyclic graph (DAG) of spec dependencies

**Validation**:
- Required fields validation
- Markdown syntax checking
- Dependency graph validation (cycles, missing refs)
- Token budget verification

**CLI Commands**:
```bash
sin-code spec init              # Initialize collection in .sin/specs/
sin-code spec create            # Create new spec
sin-code spec validate          # Validate all specs
sin-code spec archive           # Archive spec
sin-code spec list              # List all specs
sin-code spec show <id>         # Display spec
sin-code spec merge <base> <ours> <theirs>  # Three-way merge
```

### Phase 2: Compiler (SpecD)

**Dependency Resolution**:
- Builds dependency graph from spec references
- Detects cycles and undefined dependencies
- Topological sorting (Kahn's algorithm)
- Depth calculation for each spec

**Compilation Pipeline**:
1. Build graph (validate all references)
2. Topological sort (detect cycles)
3. Compute metadata (hash, depth, etc.)
4. Validate compiled state

**API**:
```go
compiler := spec.NewCompiler(collection)
result := compiler.Compile()

// Access results
if result.Successful {
    specs := compiler.TopologicalOrder()  // Process in order
    cost := compiler.EstimateCost(specID) // Total cost + deps
}
```

### Phase 3: SDLC Quality Gates

**Built-in Gates**:
- **TokenBudgetGate**: Verify token estimates within budget
- **MarkdownSyntaxGate**: Check markdown formatting
- **DependenciesGate**: Validate dependency references
- **RequiredFieldsGate**: Ensure required fields present
- **StatusGate**: Check spec status allowed for execution

**Verification API**:
```go
registry := spec.NewGateRegistry()
verifyCtx := &spec.VerificationContext{
    Collection:  collection,
    TokenBudget: 100000,
}

results := registry.Run(spec, verifyCtx)
if results.HasCriticalFailure {
    // Block execution
}
```

### Phase 4: MetaSpec Token Optimization

**Indexing & Search**:
- Full-text search with term inversion
- N-gram based fuzzy matching
- Keyword extraction from specs
- Relevance scoring (status, priority, token cost)

**Smart Selection**:
```go
indexer := spec.NewSpecIndexer(collection, maxTokens)
indexer.BuildIndex()
metaspec := indexer.MetaSpec

// Select by budget
selected := metaspec.SelectByBudget(50000, 20)  // Top 20 within 50k tokens

// Search
results := metaspec.SearchByKeyword("authentication")

// Filter by namespace/kind/status
authSpecs := metaspec.SelectByNamespace("auth")
```

**Token Budgeting**:
```go
budgeter := spec.NewTokenBudgeter(100000, numSpecs, 20) // 20% reserve

// Allocate proportionally to current estimates
allocation := budgeter.AllocateProportional(specs)

// Allocate by priority
allocation := budgeter.AllocatePriority(specs)
```

### Phase 5: SpecKit Chat Commands

**Slash Commands** (usable in chat):
```
/spec list              List all specs
/spec show <id>         Show spec details
/spec search <query>    Search specs
/goal                   Show all active goals
/verify <id>            Run quality gates
/compile                Build dependency graph
/budget                 Show token allocation
/search <query>         Full-text search
/deps <id>              Show spec dependencies
/help [cmd]             Show help
```

**Integration Example**:
```go
kit := spec.NewSpecKit(collection)
ctx := &spec.CommandContext{
    Command:    "/spec show spec_auth_001",
    Args:       []string{"spec", "show", "spec_auth_001"},
    Collection: collection,
}
result, err := kit.Execute(ctx)
```

## Usage Examples

### 1. Initialize a Collection
```bash
sin-code spec init
# Creates: .sin/specs/{active,drafts,archive}/
```

### 2. Create a Spec
```bash
sin-code spec create \
  --title "User Authentication System" \
  --kind goal \
  --namespace auth
```

### 3. Validate All Specs
```bash
sin-code spec validate --check-cycles --check-tokens --max-tokens 100000
```

### 4. Show Spec Details
```bash
sin-code spec show spec_auth_001
```

### 5. Compile and Build Graph
```go
compiler := spec.NewCompiler(collection)
result := compiler.Compile()

if result.Successful {
    fmt.Printf("Max depth: %d\n", result.Stats.MaxDepth)
    fmt.Printf("Total tokens: %d\n", result.Stats.TotalDependencies)
}
```

### 6. Run Quality Gates
```go
registry := spec.NewGateRegistry()
results := registry.Run(spec, &spec.VerificationContext{
    Collection: collection,
    TokenBudget: 100000,
})

fmt.Println(results.Details())
```

### 7. Search and Select Specs
```go
indexer := spec.NewSpecIndexer(collection, 100000)
indexer.BuildIndex()

// Full-text search
results := indexer.MetaSpec.SearchByKeyword("auth")

// Smart selection by budget
selected := indexer.MetaSpec.SelectByBudget(50000, 20)
```

## Design Principles

1. **100% Deterministic**: No LLM calls in spec layer. All operations are synchronous and reproducible.

2. **Markdown-First**: Specs are stored as markdown + JSON. Human-readable, version-control friendly.

3. **Immutable Semantics**: Specs never mutate in-place. Changes produce new instances, enabling clean versioning.

4. **Non-Breaking**: Spec Layer is opt-in. Existing Agent Loop workflows continue unchanged.

5. **Lightweight**: Only stdlib dependencies (no heavy frameworks). Small binary footprint.

## Integration with Agent Loop

The Spec Layer integrates with the Agent Loop via:

1. **Verification Phase**: Quality gates run before execution
2. **Context Selection**: MetaSpec selects relevant specs for context window
3. **Requirement Encoding**: Goals/constraints become executable specifications
4. **Decision Documentation**: Specs record architectural decisions

Future versions will add:
- Automatic spec generation from conversations
- Dynamic spec updates based on agent decisions
- Spec-driven test generation
- Trace-based spec refinement

## Testing

All modules include comprehensive validation:
```bash
# Validate collection
sin-code spec validate --check-cycles

# Run gates on spec
sin-code spec verify spec_auth_001

# Test compilation
compiler := spec.NewCompiler(collection)
result := compiler.Compile()
assert(result.Successful)
```

## Performance Notes

- **Graph Building**: O(V + E) where V=specs, E=dependencies
- **Topological Sort**: O(V + E) Kahn's algorithm
- **Search**: O(1) term lookup, O(n) result ranking
- **Budget Allocation**: O(n log n) sorting by priority

## Future Enhancements

- Phase 6: Spec migrations and versioning
- Phase 7: Automated spec repair and suggestions
- Phase 8: Spec-driven code generation
- Phase 9: Multi-agent spec orchestration
- Phase 10: Spec marketplace and templates

## References

- Issue #122: https://github.com/OpenSIN-Code/SIN-Code/issues/122
- Related: Issue #75 (Eval/Observability)
- Architecture Docs: `internal/spec/doc.go`
