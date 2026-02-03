# Benchmark Baseline Management

This directory contains benchmark baseline files used for regression detection in CI.

## How It Works

1. **On Pull Requests**: CI runs benchmarks on both the PR branch and the base branch (main), then compares them using `benchstat`.

2. **On Merge to Main**: CI generates a new baseline and uploads it as an artifact.

3. **Regression Threshold**: By default, a >10% regression in any benchmark will fail the PR check.

## Overriding Regression Checks

If a regression is intentional (e.g., adding necessary safety checks), add the `skip-benchmark-check` label to the PR.

## Running Benchmarks Locally

```bash
# Run all benchmarks
go test -bench=. -benchmem ./...

# Run with multiple iterations for statistical comparison
go test -bench=. -benchmem -count=5 ./...

# Compare two benchmark runs
go install golang.org/x/perf/cmd/benchstat@latest
go test -bench=. -count=5 ./... > old.txt
# make changes
go test -bench=. -count=5 ./... > new.txt
benchstat old.txt new.txt
```

## Current Benchmarks

The SDK includes benchmarks for:

- **Cache operations**: Set, Get, Concurrent access
- **Store operations**: MemoryStore, IAVLStore, CachedObjectStore
- **Object pools**: GetPut, Concurrent operations

## Adding New Benchmarks

Follow Go's standard benchmark conventions:

```go
func BenchmarkMyOperation(b *testing.B) {
    // Setup code here
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Code to benchmark
    }
}
```

For memory-intensive operations, use `b.ReportAllocs()` to track allocations.
