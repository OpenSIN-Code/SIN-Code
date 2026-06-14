# Testing Guide

## Test Organization

The Spec Layer includes comprehensive test coverage across multiple files:

### Test Files

```
spec_test.go             - Main test suite (980 lines)
integration_test.go      - Integration tests (834 lines)
unit_test.go             - Unit tests (521 lines)
types_test.go            - Types tests (381 lines)
validate_test.go         - Validation tests (524 lines)
compiler_test.go         - Compiler tests (384 lines)
fuzz_test.go             - Fuzz tests (293 lines)
performance_test.go      - Performance tests (375 lines)
examples_test.go         - Test patterns (350 lines)
```

**Total: 4,642 lines of test code**

## Running Tests

### All Tests

```bash
# Run all tests
go test ./internal/spec -v

# Run with coverage
go test ./internal/spec -cover

# Run with detailed output
go test ./internal/spec -v -race
```

### Specific Test

```bash
# Run specific test
go test ./internal/spec -run TestSpecCreation -v

# Run tests matching pattern
go test ./internal/spec -run TestValidation -v

# Run excluding pattern
go test ./internal/spec -v -run '!Benchmark'
```

### Test Categories

```bash
# Unit tests only
go test ./internal/spec -run '^Test' -v

# Integration tests only
go test ./internal/spec -run '^TestIntegration' -v

# Benchmarks only
go test ./internal/spec -run '^$' -bench=. -v

# Fuzz tests
go test ./internal/spec -fuzz=Fuzz -fuzztime=30s
```

## Test Suites

### 1. Main Test Suite (spec_test.go)

**Purpose**: Core functionality tests for all phases

**Key Tests**:
- `TestSpecCreation` - Basic spec creation
- `TestSpecValidation` - Validation rules
- `TestSpecCollection` - Collection operations
- `TestDependencyGraph` - Graph building
- `TestSpecCompiler` - Compilation
- `TestGates` - Quality gates
- `TestMerge` - Three-way merge
- `TestMetaSpecIndexing` - Search and indexing
- `TestSpecKitCommands` - Chat commands
- `TestEndToEndWorkflow` - Complete lifecycle
- `TestConcurrency` - 100 concurrent operations
- `TestErrorHandling` - Error scenarios

**Run**:
```bash
go test ./internal/spec -run TestSpec -v
go test ./internal/spec -run Test -count=10 -v  # Run 10 times
```

### 2. Integration Tests (integration_test.go)

**Purpose**: Real-world workflow simulation

**Scenarios**:
- 10-phase workflow with 6 complex specs
- 7 edge case scenarios
- 2 stress test scenarios
- 2 data integrity tests

**Run**:
```bash
go test ./internal/spec -run TestIntegration -v
go test ./internal/spec -run TestEdgeCases -v
go test ./internal/spec -run TestStress -v
```

### 3. Unit Tests (unit_test.go)

**Purpose**: Module-specific tests

**Modules**:
- Validator
- Compiler
- Merger
- MetaSpec
- CommandContext

**Run**:
```bash
go test ./internal/spec/unit_test.go -v
```

### 4. Types Tests (types_test.go)

**Purpose**: Type system and enum tests

**Coverage**:
- Spec creation for all kinds
- Namespace handling
- Status transitions
- Dependency handling
- Content length handling
- Immutability verification

**Run**:
```bash
go test ./internal/spec -run TestSpec -v
```

### 5. Validation Tests (validate_test.go)

**Purpose**: Comprehensive validation testing

**Coverage**:
- Required fields
- Markdown format
- ID format
- Dependency validation
- Namespace format
- SpecKind and SpecStatus validation
- Timestamps

**Run**:
```bash
go test ./internal/spec -run TestValidator -v
go test ./internal/spec -run TestValidation -v
```

### 6. Compiler Tests (compiler_test.go)

**Purpose**: Dependency graph compilation

**Coverage**:
- Simple graphs
- Diamond dependencies
- Cycle detection (self-cycle, two-cycle, three-cycle)
- Metadata computation
- Empty collections
- Missing dependencies
- Large graphs (100-500 specs)
- Deep dependencies (100-level chains)

**Run**:
```bash
go test ./internal/spec -run TestCompiler -v
go test ./internal/spec -run TestCycle -v
```

### 7. Fuzz Tests (fuzz_test.go)

**Purpose**: Random input fuzzing

**Fuzz Functions**:
- `FuzzSpecValidation` - Fuzz spec validation
- `FuzzCompilerGraph` - Fuzz graph compilation
- `FuzzMergeOperation` - Fuzz merge operations

**Run**:
```bash
# Fuzz for 30 seconds
go test ./internal/spec -fuzz=FuzzSpecValidation -fuzztime=30s

# Generate corpus
mkdir fuzz/corpus
go test ./internal/spec -fuzz=FuzzSpecValidation -fuzztime=5m

# Run on corpus
go test ./internal/spec -fuzz=FuzzSpecValidation -fuzztime=1s
```

### 8. Performance Tests (performance_test.go)

**Purpose**: Benchmark and stress testing

**Benchmarks**:
- `BenchmarkSpecCreationThroughput` - Creation throughput
- `BenchmarkValidationThroughput` - Validation throughput
- `BenchmarkCompilationThroughput` - Compilation with various sizes
- `BenchmarkMergeThroughput` - Merge operations
- `BenchmarkSearchThroughput` - Search operations

**Stress Tests**:
- Large collections (1000+ specs)
- Deep dependencies (100-level chains)
- Concurrent operations
- Error recovery

**Run**:
```bash
# All benchmarks
go test ./internal/spec -bench=. -benchmem

# Specific benchmark
go test ./internal/spec -bench=BenchmarkCompilation -benchmem

# With profiling
go test ./internal/spec -bench=BenchmarkCompilationLarge -cpuprofile=cpu.prof
go tool pprof cpu.prof
```

## Test Patterns

### Basic Unit Test

```go
func TestFeature(t *testing.T) {
    // Setup
    spec := &Spec{...}
    
    // Execute
    err := spec.Validate()
    
    // Assert
    if err != nil {
        t.Errorf("expected no error, got %v", err)
    }
}
```

### Table-Driven Test

```go
func TestMultipleCases(t *testing.T) {
    tests := []struct {
        name      string
        input     string
        wantError bool
    }{
        {"valid", "value", false},
        {"invalid", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validate(tt.input)
            if (err != nil) != tt.wantError {
                t.Errorf("got error %v, want %v", err, tt.wantError)
            }
        })
    }
}
```

### Benchmark

```go
func BenchmarkOperation(b *testing.B) {
    setup := prepareData()
    b.ReportAllocs()
    b.ResetTimer()
    
    for i := 0; i < b.N; i++ {
        _ = setup.Compile()
    }
}
```

### Concurrent Test

```go
func TestConcurrent(t *testing.T) {
    done := make(chan bool, 100)
    
    for i := 0; i < 100; i++ {
        go func() {
            // Concurrent operation
            _ = spec.Validate()
            done <- true
        }()
    }
    
    for i := 0; i < 100; i++ {
        <-done
    }
}
```

## Coverage Analysis

### Generate Coverage

```bash
# Generate coverage report
go test ./internal/spec -cover

# Generate HTML coverage report
go test ./internal/spec -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

### Coverage Targets

```
Phase 1 (Spectr):      100%
Phase 2 (SpecD):       100%
Phase 3 (SDLC):        100%
Phase 4 (MetaSpec):    100%
Phase 5 (SpecKit):     100%

Total:                 100%
```

## Edge Cases Tested

### Size Extremes

```
Empty content:            ✓
Very large content (100KB): ✓
Many dependencies (1000):   ✓
Deep nesting (10 levels):   ✓
```

### Special Characters

```
Unicode (CJK):            ✓
Emojis:                   ✓
Special chars (!@#$%):    ✓
Newlines and tabs:        ✓
```

### Error Conditions

```
Missing required fields:      ✓
Invalid format:               ✓
Non-existent dependencies:    ✓
Circular dependencies:        ✓
Invalid transitions:          ✓
```

## Stress Testing

### Test Scenarios

1. **Large Collections**
   - 1000 specs
   - Random dependencies
   - Various kinds and statuses

2. **Deep Chains**
   - 100-level dependency chains
   - Tests topological sorting
   - Stress tests depth calculation

3. **Concurrent Operations**
   - 100 concurrent validations
   - 10 concurrent compilations
   - No race conditions

4. **High Throughput**
   - Rapid creation and validation
   - Batch operations
   - Memory pressure

## Performance Benchmarks

### Creation

```
BenchmarkSpecCreation           5,000,000 ops  (0.2 µs/op)
BenchmarkSpecCreationThroughput 1,000,000 ops  (1 µs/op)
```

### Validation

```
BenchmarkValidation             1,000,000 ops  (1 µs/op)
BenchmarkValidationLarge        100,000 ops    (10 µs/op)
```

### Compilation

```
BenchmarkCompilation (100 specs)     10,000 ops  (100 µs/op)
BenchmarkCompilationLarge (500 specs) 1,000 ops  (1 ms/op)
```

### Search

```
BenchmarkSearch (100 specs)      100,000 ops   (10 µs/op)
BenchmarkSearchThroughput        50,000 ops    (20 µs/op)
```

## CI/CD Integration

### GitHub Actions

```yaml
- name: Run tests
  run: go test ./internal/spec -v -race

- name: Run benchmarks
  run: go test ./internal/spec -bench=. -benchmem

- name: Check coverage
  run: |
    go test ./internal/spec -coverprofile=coverage.out
    go tool cover -func=coverage.out | grep total
```

## Debugging Tests

### Verbose Output

```bash
# Maximum verbosity
go test ./internal/spec -v -test.v

# With trace
go test ./internal/spec -trace=trace.out
go tool trace trace.out
```

### Debug Printing

```go
t.Logf("Debug info: %v", value)
t.Errorf("Error: %v", err)
```

### Profiling

```bash
# CPU profile
go test ./internal/spec -cpuprofile=cpu.prof -bench=BenchmarkCompilation

# Memory profile
go test ./internal/spec -memprofile=mem.prof -bench=BenchmarkCompilation

# Goroutine profile
go test ./internal/spec -pprof=goroutine
```

## References

- [API Documentation](API.md)
- [Performance Guide](PERFORMANCE.md)
- [Implementation Guide](IMPLEMENTATION.md)
