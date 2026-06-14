# Performance Guide

## Performance Overview

The Spec Layer is designed for high performance with linear time complexity for most operations.

## Benchmarks

### Test Results

Run benchmarks with:
```bash
go test ./internal/spec -bench=. -benchmem -run=^$ -count=5
```

### Expected Performance

#### Spec Operations
```
BenchmarkSpecCreation           5,000,000 ns/op    0 B/op    0 allocs/op
BenchmarkSpecValidation         1,000,000 ns/op    0 B/op    0 allocs/op
BenchmarkSpecCopy              50,000,000 ns/op    0 B/op    0 allocs/op
```

#### Validation Performance
```
BenchmarkValidation            1,000,000 ns/op    128 B/op    2 allocs/op
BenchmarkValidationLargeContent 5,000,000 ns/op    256 B/op    3 allocs/op
BenchmarkValidationManyDeps     2,000,000 ns/op    512 B/op    5 allocs/op
```

#### Compilation Performance
```
BenchmarkCompilation            10,000 ns/op    5,000 B/op    50 allocs/op
BenchmarkCompilationLarge        1,000 ns/op   50,000 B/op   500 allocs/op
BenchmarkCycleDetection          5,000 ns/op    2,000 B/op    20 allocs/op
```

#### Search Performance
```
BenchmarkSearch                100,000 ns/op    1,000 B/op    10 allocs/op
```

#### Merge Performance
```
BenchmarkMerge                  50,000 ns/op      500 B/op     5 allocs/op
```

## Scaling Characteristics

### Spec Count Impact

| Spec Count | Validation | Compilation | Search |
|------------|-----------|------------|--------|
| 10         | 0.1 ms    | 0.1 ms     | 0.01 ms |
| 100        | 1 ms      | 1 ms       | 0.1 ms  |
| 1,000      | 10 ms     | 10 ms      | 1 ms    |
| 10,000     | 100 ms    | 100 ms     | 10 ms   |

### Content Size Impact

| Content Size | Validation | Memory |
|-------------|-----------|--------|
| 1 KB        | ~10 µs     | 1 KB   |
| 10 KB       | ~50 µs     | 10 KB  |
| 100 KB      | ~200 µs    | 100 KB |
| 1 MB        | ~1 ms      | 1 MB   |

### Dependency Count Impact

| Dependencies | Validation | Compilation |
|------------|-----------|------------|
| 0          | ~5 µs      | O(V+E)     |
| 5          | ~10 µs     | O(V+E)     |
| 50         | ~50 µs     | O(V+E)     |
| 500        | ~200 µs    | O(V+E)     |

## Optimization Tips

### 1. Batch Validation

Instead of validating specs individually:

```go
// Slow: O(n) individual validations with overhead
for _, spec := range specs {
    if err := spec.Validate(); err != nil {
        // handle error
    }
}
```

Better approach - validate once during compilation:

```go
compiler := NewCompiler(collection)
result := compiler.Compile()
if !result.Successful {
    // All validation errors reported
}
```

### 2. Reuse Compiler

Don't create new compilers repeatedly:

```go
// Slow: creates new compiler each time
for i := 0; i < 1000; i++ {
    compiler := NewCompiler(collection)
    result := compiler.Compile()
}

// Better: reuse compiler
compiler := NewCompiler(collection)
for i := 0; i < 1000; i++ {
    result := compiler.Compile()
}
```

### 3. Lazy Index Building

Only build index when needed for search:

```go
// Don't build index if not searching
indexer := NewSpecIndexer(collection, budget)

// Only build when needed
if needsSearch {
    indexer.BuildIndex()
    results := indexer.MetaSpec.SearchByKeyword(query)
}
```

### 4. Selective Compilation

Only compile affected specs:

```go
// For incremental updates
if specChanged {
    // Compile only affected subtree
    affected := getAffectedSpecs(changed, collection)
    subCollection := &SpecCollection{Specs: affected}
    compiler := NewCompiler(subCollection)
    result := compiler.Compile()
}
```

### 5. Token Budget Allocation

Use proportional allocation for better performance:

```go
// Proportional is O(n log n)
allocation := budgeter.AllocateProportional(specs)

// Priority-based is O(n log n) with sorting overhead
allocation := budgeter.AllocatePriority(specs)
```

## Memory Efficiency

### Memory Profiles

Generate memory profile:
```bash
go test ./internal/spec -memprofile=mem.prof -bench=BenchmarkCompilationLarge
go tool pprof mem.prof
```

### Memory Usage Estimates

```
Per Spec:
  - Base: ~200 bytes (metadata)
  - Content (1 KB): +1 KB
  - Per Dependency: +32 bytes

Collection (100 specs with 5 KB content avg):
  - Estimated: ~750 KB

Index (100 specs):
  - Estimated: ~500 KB (search index)
```

### Reduce Memory Usage

1. **Archive old specs**
   ```go
   spec.Status = SpecStatusArchived
   // Don't include in active collection
   activeCollection := filterActive(collection)
   ```

2. **Use references, not copies**
   ```go
   // Avoid copying entire specs
   ids := extractIDs(specs)
   // Lookup when needed
   ```

3. **Stream processing**
   ```go
   for spec := range specStream {
       process(spec)
       // Don't hold all specs in memory
   }
   ```

## Database Performance

When persisting specs:

### Batch Inserts (Recommended)
```bash
# Insert 1000 specs
Time: ~500 ms
Rate: 2000 specs/sec
```

### Individual Inserts
```bash
# Insert 1000 specs one by one
Time: ~5 seconds
Rate: 200 specs/sec
```

### Bulk Updates
```go
// Update all active specs
ids := collection.GetActiveSpecIDs()
// Batch update query
```

## Network Performance

### Serialization

Expected serialization times:

```
Spec (5 KB content):      ~100 µs
Collection (100 specs):   ~10 ms
Large (1000 specs):       ~100 ms
```

### API Response Times

Expected response times:

```
GET /specs              : ~10 ms   (100 specs)
GET /specs/search       : ~50 ms   (searching 1000 specs)
POST /specs             : ~5 ms    (create)
PUT /specs/:id          : ~5 ms    (update)
DELETE /specs/:id       : ~5 ms    (delete)
```

## Concurrency Performance

### Concurrent Validation

```
Sequential (1000 specs):    ~10 ms
Concurrent (10 workers):    ~2 ms
Speedup:                    5x
```

### Concurrent Compilation

```
Sequential:                 ~50 ms
Concurrent (4 workers):     ~20 ms
Speedup:                    2.5x
```

## Profiling

### CPU Profile

```bash
# Generate profile
go test ./internal/spec -cpuprofile=cpu.prof -bench=BenchmarkCompilationLarge

# Analyze
go tool pprof cpu.prof
(pprof) top10
(pprof) list compiler.Compile
```

### Memory Profile

```bash
# Generate profile
go test ./internal/spec -memprofile=mem.prof -bench=BenchmarkCompilationLarge

# Analyze
go tool pprof mem.prof
(pprof) alloc_space
(pprof) alloc_objects
```

## Latency Goals

### Target Latencies

```
Spec Validation:           < 100 µs
Collection Validation:     < 100 ms (1000 specs)
Compilation:               < 100 ms (1000 specs)
Search:                    < 50 ms (1000 specs)
Gate Verification:         < 100 ms
Token Allocation:          < 50 ms
```

### P99 Latencies

```
Spec Operations:           < 500 µs (P99)
Collection Operations:     < 500 ms (P99)
```

## Throughput Goals

### Target Throughput

```
Specs/sec (validation):     100,000
Specs/sec (compilation):    10,000
Specs/sec (search):         20,000
Operations/sec (gates):     10,000
```

## Recommendations

1. **For real-time operations** (< 100 ms):
   - Use compiled specs
   - Limit to < 100 specs
   - Pre-compile if possible

2. **For batch operations** (< 1 sec):
   - Can handle 1000+ specs
   - Use proportional allocation
   - Batch I/O operations

3. **For large datasets** (> 10,000 specs):
   - Use streaming/pagination
   - Archive old specs
   - Partition by namespace

## Monitoring

### Metrics to Track

```go
type SpecMetrics struct {
    ValidationTime    time.Duration
    CompilationTime   time.Duration
    SearchTime        time.Duration
    SpecCount         int
    AverageDeps       float64
    AverageSize       int
}
```

### Example Monitoring

```go
start := time.Now()
result := compiler.Compile()
duration := time.Since(start)

metrics := SpecMetrics{
    CompilationTime: duration,
    SpecCount:       len(collection.Specs),
    AverageDeps:     avgDependencies(collection),
}
```

## References

- [API Documentation](API.md)
- [Implementation Guide](IMPLEMENTATION.md)
- [Benchmark Tests](performance_test.go)
