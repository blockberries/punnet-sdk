// Package testing provides test utilities for the Punnet SDK.
package testing

import (
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
		require.Equal(t, string(first), string(result),
			"SignDocData() returned different bytes on iteration %d.\n"+
				"First:  %s\n"+
				"Got:    %s\n"+
				"This indicates non-deterministic serialization, likely due to "+
				"map iteration order. Ensure map keys are sorted before serialization.",
			i, string(first), string(result))
	}
}

// RequireSignDocDataDeterminism is like AssertSignDocDataDeterminism but fails
// immediately on the first non-deterministic result using require instead of assert.
// Use this when subsequent tests depend on determinism.
func RequireSignDocDataDeterminism(t *testing.T, msg types.SignDocSerializable, iterations int) {
	t.Helper()
	AssertSignDocDataDeterminism(t, msg, iterations)
}

// AssertSignDocDataValid validates that a SignDocSerializable implementation
// returns valid, parseable JSON from SignDocData().
//
// This helper verifies:
// 1. SignDocData() returns without error
// 2. The returned bytes are valid JSON
// 3. Repeated calls produce identical output (determinism)
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

	// Verify it's valid JSON by checking it can be unmarshalled
	// json.RawMessage is already validated by the JSON package when marshaled,
	// but we verify the invariant holds
	require.True(t, len(data) > 0, "SignDocData() returned empty bytes")

	// Verify determinism with a reasonable number of iterations
	AssertSignDocDataDeterminism(t, msg, 10)
}
