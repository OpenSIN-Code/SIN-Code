# Spec Layer API Documentation

## Overview

The Spec Layer (`internal/spec`) provides a complete specification management system for SIN-Code. It implements all 5 phases: Spectr, SpecD, SDLC, MetaSpec, and SpecKit.

## Core Types

### Spec

A specification is an immutable container for structured information about system requirements, processes, constraints, components, or integrations.

```go
type Spec struct {
    ID           string        // Unique identifier (required)
    Kind         SpecKind      // Type of spec (required)
    Title        string        // Human-readable title (required)
    Content      string        // Markdown content (required)
    Namespace    string        // Hierarchical namespace (required)
    Status       SpecStatus    // Lifecycle status (required)
    Dependencies []string      // IDs of dependent specs
    CreatedAt    time.Time     // Creation timestamp
    UpdatedAt    time.Time     // Last update timestamp
}
```

### SpecKind

Enumeration of spec types:

```go
const (
    SpecKindGoal        SpecKind = 0  // Strategic goals
    SpecKindProcess     SpecKind = 1  // Process definitions
    SpecKindConstraint  SpecKind = 2  // Constraints and requirements
    SpecKindComponent   SpecKind = 3  // System components
    SpecKindIntegration SpecKind = 4  // External integrations
)
```

### SpecStatus

Specification lifecycle states:

```go
const (
    SpecStatusDraft    SpecStatus = 0  // Draft (editable)
    SpecStatusActive   SpecStatus = 1  // Active (in use)
    SpecStatusArchived SpecStatus = 2  // Archived (historical)
)
```

### SpecCollection

Container for related specifications with dependency tracking:

```go
type SpecCollection struct {
    Specs map[string]*Spec       // Specs by ID
    Graph *DependencyGraph       // Dependency graph
}
```

## Phase 1: Spectr (Core Backbone)

### Spec Creation

```go
spec := &Spec{
    ID:        "spec_auth_001",
    Kind:      SpecKindGoal,
    Title:     "User Authentication",
    Content:   "# Goal\n\nImplement OAuth2 authentication",
    Namespace: "auth",
    Status:    SpecStatusDraft,
    CreatedAt: time.Now(),
}

// Validate spec
if err := spec.Validate(); err != nil {
    log.Fatal(err)
}
```

### Validation

Specs are validated on creation:

- **Required Fields**: ID, Title, Content, Namespace, Kind, Status
- **Format Rules**: Valid namespace format, markdown content
- **Dependencies**: All referenced specs must exist
- **Status Transitions**: Only valid transitions allowed

```go
// Validation is automatic on Validate() call
err := spec.Validate()

// Common validation errors
// - empty ID
// - empty title
// - invalid namespace format
// - non-existent dependencies
// - invalid status
```

### Collection Operations

```go
collection := &SpecCollection{
    Specs: make(map[string]*Spec),
}

// Add specs
collection.Specs["spec_001"] = spec1
collection.Specs["spec_002"] = spec2

// Retrieve
retrieved := collection.Specs["spec_001"]

// Validate all
for _, spec := range collection.Specs {
    if err := spec.Validate(); err != nil {
        log.Printf("Invalid spec %s: %v", spec.ID, err)
    }
}
```

### Merge Operations

Three-way merge for conflict resolution:

```go
// Three-way merge: base (common ancestor), ours (our changes), theirs (their changes)
merged := ThreeWayMerge(baseSpec, ourSpec, theirSpec)

if merged != nil {
    // Merge successful
    err := merged.Validate()
}
```

## Phase 2: SpecD (Compiler)

### Graph Building

```go
collection := &SpecCollection{
    Specs: map[string]*Spec{
        "goal_1": spec1,
        "goal_2": spec2,
        // ...
    },
}

// Compile
compiler := NewCompiler(collection)
result := compiler.Compile()

if !result.Successful {
    for _, err := range result.Errors {
        log.Printf("Compilation error: %v", err)
    }
    return
}

// Get topological order
order := result.Order // []string of spec IDs

// Access depths
depth := result.Depths["spec_id"]

// Check for cycles (result.Successful will be false if cycles exist)
if !result.Successful {
    log.Println("Graph has cycles")
}
```

### Performance

- Graph Building: O(V + E)
- Cycle Detection: O(V + E) using DFS
- Topological Sort: O(V + E) using Kahn's algorithm
- Depth Calculation: O(V + E)

## Phase 3: SDLC (Quality Gates)

### Built-in Gates

1. **RequiredFieldsGate** - Verify all required fields
2. **MarkdownSyntaxGate** - Check markdown syntax
3. **TokenBudgetGate** - Verify token estimates
4. **StatusGate** - Validate status values
5. **DependenciesGate** - Check dependency references

### Using Gates

```go
registry := NewGateRegistry()

// Gates are registered by default

// Create verification context
context := &VerificationContext{
    Budget:    100000,
    Namespace: "auth",
}

// Run gates
results := registry.Run(spec, context)

// Check results
for _, result := range results {
    if result.Failed && result.Gate.Critical() {
        log.Printf("Critical gate failed: %s", result.Gate.Name())
    }
}
```

### Custom Gates

```go
type CustomGate struct{}

func (g *CustomGate) Name() string {
    return "CustomGate"
}

func (g *CustomGate) Run(spec *Spec, ctx *VerificationContext) *GateResult {
    // Custom logic
    if spec.Title == "" {
        return &GateResult{
            Gate:    g,
            Failed:  true,
            Message: "Title is empty",
        }
    }
    return &GateResult{Gate: g, Failed: false}
}

func (g *CustomGate) Critical() bool {
    return true
}

// Register custom gate
registry.Register(g)
```

## Phase 4: MetaSpec (Token Optimization)

### Indexing

```go
collection := &SpecCollection{...}

// Create indexer
indexer := NewSpecIndexer(collection, maxTokens)

// Build index
indexer.BuildIndex()

// Now can use MetaSpec for searches and selection
```

### Searching

```go
// Full-text search
results := indexer.MetaSpec.SearchByKeyword("authentication")

// Fuzzy search with scoring
for _, spec := range results {
    log.Printf("Found: %s (score: %.2f)", spec.Title, spec.Score)
}
```

### Filtering

```go
// By kind
goalSpecs := indexer.MetaSpec.SelectByKind(SpecKindGoal)

// By namespace
authSpecs := indexer.MetaSpec.SelectByNamespace("auth")

// By status
activeSpecs := indexer.MetaSpec.SelectByStatus(SpecStatusActive)

// By token budget
selectedSpecs := indexer.MetaSpec.SelectByBudget(budget, limit)
```

### Token Budgeting

```go
budgeter := NewTokenBudgeter(totalBudget, specCount, avgTokensPerSpec)

// Proportional allocation
allocation := budgeter.AllocateProportional(specs)

// Priority-based allocation
allocation := budgeter.AllocatePriority(specs)

// Check allocation for a spec
if tokens, ok := allocation[spec.ID]; ok {
    log.Printf("Allocated %d tokens to %s", tokens, spec.ID)
}
```

## Phase 5: SpecKit (Chat Integration)

### Commands

Slash-commands for interactive spec management:

```
/spec list                        - List all specs
/spec show <id>                   - Show spec details
/spec search <query>              - Search specs
/goal                            - Show active goals
/verify <id>                     - Run quality gates
/compile                         - Build dependency graph
/budget                          - Show token allocation
/search <query>                  - Full-text search
/deps <id>                       - Show dependencies
```

### Programmatic Usage

```go
kit := NewSpecKit(collection)

ctx := &CommandContext{
    Command:    "/spec list",
    Args:       []string{"spec", "list"},
    Collection: collection,
}

result, err := kit.Execute(ctx)
if err != nil {
    log.Printf("Command failed: %v", err)
}

log.Printf("Result: %s", result)
```

## Testing

### Unit Tests

```bash
# Test individual modules
go test ./internal/spec -run TestSpec -v
go test ./internal/spec -run TestValidator -v
go test ./internal/spec -run TestCompiler -v
```

### Integration Tests

```bash
# Test complete workflows
go test ./internal/spec -run TestIntegration -v
```

### Benchmarks

```bash
# Run all benchmarks
go test ./internal/spec -bench=. -benchmem

# Run specific benchmark
go test ./internal/spec -bench=BenchmarkCompilation -benchmem

# Run with CPU profiling
go test ./internal/spec -bench=. -cpuprofile=cpu.prof

# Analyze profile
go tool pprof cpu.prof
```

### Fuzz Tests

```bash
# Run fuzz tests (Go 1.18+)
go test ./internal/spec -fuzz=Fuzz -fuzztime=30s
```

## Performance Characteristics

### Creation
- Spec: ~1-2 µs
- Collection: O(n) where n is number of specs

### Validation
- Simple spec: ~10-20 µs
- Complex spec (1000 deps): ~100 µs
- Collection (100 specs): ~1-2 ms

### Compilation
- Small graph (10 specs): ~100 µs
- Medium graph (100 specs): ~1 ms
- Large graph (1000 specs): ~10-20 ms

### Search
- Keyword lookup: O(1)
- Fuzzy match: O(n log n)
- Result ranking: O(n)

## Best Practices

1. **Always validate specs after creation**
   ```go
   if err := spec.Validate(); err != nil {
       return err
   }
   ```

2. **Use namespaces hierarchically**
   ```
   auth.oauth2.google
   auth.oauth2.microsoft
   auth.saml
   ```

3. **Document dependencies clearly**
   ```
   spec.Dependencies = []string{"auth", "database", "cache"}
   ```

4. **Handle merge conflicts gracefully**
   ```go
   merged := ThreeWayMerge(base, ours, theirs)
   if merged == nil {
       // Handle conflict
   }
   ```

5. **Use gates for validation**
   ```go
   registry := NewGateRegistry()
   results := registry.Run(spec, context)
   ```

6. **Optimize token usage**
   ```go
   budgeter := NewTokenBudgeter(totalBudget, count, avgTokens)
   allocation := budgeter.AllocateProportional(specs)
   ```

## Error Handling

```go
// Common error patterns
if err := spec.Validate(); err != nil {
    switch err {
    case ErrEmptyID:
        log.Println("Spec ID is required")
    case ErrEmptyTitle:
        log.Println("Spec title is required")
    default:
        log.Printf("Validation error: %v", err)
    }
}
```

## Concurrency

All Spec Layer operations are thread-safe for read operations. For concurrent writes:

```go
// Safe for concurrent reads
go func() {
    _ = spec.Validate()
}()

// Create new instances for modifications
newSpec := *spec // Copy
newSpec.Title = "Modified"
```

## Integration with Agent Loop

The Spec Layer integrates with the Agent Loop:

1. **Verification Phase**: Gates run before execution
2. **Context Window**: MetaSpec selects relevant specs
3. **Goal Encoding**: Goals become active specs
4. **Constraint Management**: Constraints enforced via gates
5. **Decision Recording**: Decisions stored as specs

```go
// In Agent Loop
compiler := NewCompiler(collection)
result := compiler.Compile()

if !result.Successful {
    return errors.New("specification compilation failed")
}

registry := NewGateRegistry()
gateResults := registry.Run(spec, context)

if gateResults.HasCriticalFailure {
    return errors.New("critical gate failed")
}
```

## References

- [Implementation Guide](IMPLEMENTATION.md)
- [Test Examples](examples_test.go)
- [Package Documentation](doc.go)
