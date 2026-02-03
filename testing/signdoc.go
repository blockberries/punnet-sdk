// Package testing provides test utilities for the Punnet SDK.
package testing

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/require"
)

// AssertSignDocDataDeterminism validates that a SignDocSerializable implementation
// produces deterministic output from SignDocData().
//
// This helper calls SignDocData() N times and asserts all outputs are byte-identical.
// Use this to catch non-determinism issues in message implementations, particularly
// those that use map types internally which may have undefined iteration order.
//
// SECURITY: Non-deterministic SignDocData() implementations can cause signature
// verification failures when the SignDoc is reconstructed on different nodes,
// as different byte serializations produce different hashes.
//
// Note: This function uses require semantics and fails immediately on the first
// non-deterministic result. There is no separate "assert" variant that continues
// after failures.
//
// Usage:
//
//	func TestMyMessage_SignDocDataDeterminism(t *testing.T) {
//	    msg := &MyMessage{From: "alice", To: "bob", Amount: 100}
//	    punnettesting.AssertSignDocDataDeterminism(t, msg, 100)
//	}
//
// Parameters:
//   - t: The testing.T instance
//   - msg: A message implementing SignDocSerializable
//   - iterations: Number of times to call SignDocData() (recommend at least 10,
//     use 100+ for thorough testing as Go map iteration randomization may not
//     manifest on every call)
func AssertSignDocDataDeterminism(t *testing.T, msg types.SignDocSerializable, iterations int) {
	t.Helper()

	if iterations < 2 {
		t.Fatal("AssertSignDocDataDeterminism requires at least 2 iterations")
	}

	first, err := msg.SignDocData()
	require.NoError(t, err, "SignDocData() failed on first call")
	require.NotNil(t, first, "SignDocData() returned nil on first call")

	for i := 1; i < iterations; i++ {
		result, err := msg.SignDocData()
		require.NoError(t, err, "SignDocData() failed on iteration %d", i)
		// Use bytes.Equal to avoid string conversion allocations in the success path
		if !bytes.Equal(first, result) {
			t.Fatalf("SignDocData() returned different bytes on iteration %d.\n"+
				"First:  %s\n"+
				"Got:    %s\n"+
				"This indicates non-deterministic serialization, likely due to "+
				"map iteration order. Ensure map keys are sorted before serialization.",
				i, string(first), string(result))
		}
	}
}

// AssertSignDocDataValid validates that a SignDocSerializable implementation
// returns valid, parseable JSON from SignDocData().
//
// This helper verifies:
// 1. SignDocData() returns without error
// 2. The returned bytes are non-empty
// 3. The returned bytes are valid JSON syntax
// 4. Repeated calls produce identical output (determinism with 100 iterations)
//
// Usage:
//
//	func TestMyMessage_SignDocData(t *testing.T) {
//	    msg := &MyMessage{From: "alice", To: "bob", Amount: 100}
//	    punnettesting.AssertSignDocDataValid(t, msg)
//	}
func AssertSignDocDataValid(t *testing.T, msg types.SignDocSerializable) {
	t.Helper()

	data, err := msg.SignDocData()
	require.NoError(t, err, "SignDocData() returned error")
	require.NotNil(t, data, "SignDocData() returned nil")
	require.True(t, len(data) > 0, "SignDocData() returned empty bytes")

	// Verify it's valid JSON syntax
	require.True(t, json.Valid(data), "SignDocData() returned invalid JSON: %s", string(data))

	// Verify determinism with thorough iteration count
	// Using 100 iterations as recommended for catching Go map iteration randomization
	AssertSignDocDataDeterminism(t, msg, 100)
}

// AssertSignDocDataDeterminismConcurrent validates that a SignDocSerializable
// implementation produces deterministic output even when called concurrently
// from multiple goroutines.
//
// SECURITY: This test catches race conditions in SignDocData() implementations
// that may use shared mutable state (e.g., cached serialization buffers,
// lazy-initialized fields, or incorrectly shared temporaries). Such bugs can
// cause signature verification failures under load, which is especially dangerous
// in validator nodes processing multiple transactions simultaneously.
//
// WARNING: This test does NOT prove thread-safety. A passing test only means
// we didn't observe a race in this run. Use with -race flag for complete
// coverage:
//
//	go test -race ./...
//
// Usage:
//
//	func TestMyMessage_SignDocDataConcurrent(t *testing.T) {
//	    msg := &MyMessage{From: "alice", To: "bob", Amount: 100}
//	    punnettesting.AssertSignDocDataDeterminismConcurrent(t, msg, 10, 100)
//	}
//
// Parameters:
//   - t: The testing.T instance
//   - msg: A message implementing SignDocSerializable
//   - goroutines: Number of concurrent goroutines (recommend 4-16)
//   - iterationsPerGoroutine: Calls per goroutine (recommend 50-100)
//
// IMPLEMENTATION NOTE: The function collects all results and checks them
// after all goroutines complete. This avoids synchronization overhead
// during the hot path while still detecting non-determinism.
func AssertSignDocDataDeterminismConcurrent(t *testing.T, msg types.SignDocSerializable, goroutines, iterationsPerGoroutine int) {
	t.Helper()

	if goroutines < 1 {
		t.Fatal("AssertSignDocDataDeterminismConcurrent requires at least 1 goroutine")
	}
	if iterationsPerGoroutine < 1 {
		t.Fatal("AssertSignDocDataDeterminismConcurrent requires at least 1 iteration per goroutine")
	}

	// Get the reference value first (single-threaded, before concurrent access)
	reference, err := msg.SignDocData()
	require.NoError(t, err, "SignDocData() failed on initial reference call")
	require.NotNil(t, reference, "SignDocData() returned nil on initial reference call")

	// Channel to collect all results
	totalResults := goroutines * iterationsPerGoroutine
	results := make(chan concurrentResult, totalResults)

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < iterationsPerGoroutine; i++ {
				data, err := msg.SignDocData()
				results <- concurrentResult{
					data:        data,
					err:         err,
					goroutineID: goroutineID,
					iteration:   i,
				}
			}
		}(g)
	}
	wg.Wait()
	close(results)

	// Check all results against reference
	for r := range results {
		if r.err != nil {
			t.Fatalf("SignDocData() failed in goroutine %d, iteration %d: %v",
				r.goroutineID, r.iteration, r.err)
		}
		if r.data == nil {
			t.Fatalf("SignDocData() returned nil in goroutine %d, iteration %d",
				r.goroutineID, r.iteration)
		}
		if !bytes.Equal(reference, r.data) {
			t.Fatalf("SignDocData() returned different bytes in goroutine %d, iteration %d.\n"+
				"Reference: %s\n"+
				"Got:       %s\n"+
				"This indicates a race condition or non-thread-safe implementation.\n"+
				"Check for shared mutable state, unsynchronized caches, or lazy initialization.\n"+
				"Run with -race flag for more details: go test -race ./...",
				r.goroutineID, r.iteration, string(reference), string(r.data))
		}
	}
}

// concurrentResult holds the result of a single SignDocData() call during concurrent testing.
type concurrentResult struct {
	data        json.RawMessage
	err         error
	goroutineID int
	iteration   int
}
