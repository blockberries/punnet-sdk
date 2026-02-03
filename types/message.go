package types

import "encoding/json"

// Message is the interface that all messages must implement
type Message interface {
	// Type returns the message type identifier (e.g., "/punnet.bank.v1.MsgSend")
	Type() string

	// ValidateBasic performs stateless validation
	ValidateBasic() error

	// GetSigners returns the accounts that must authorize this message
	GetSigners() []AccountName
}

// SignDocSerializable is an optional interface that messages can implement to provide
// their canonical representation for inclusion in SignDoc.
//
// INVARIANT: SignDocData() must be deterministic - repeated calls with identical
// message state must return byte-identical JSON.
//
// INVARIANT: The returned JSON must be valid and parseable.
//
// # Thread Safety Requirements
//
// INVARIANT: SignDocData() must be safe for concurrent calls from multiple goroutines.
//
// WHY CONCURRENT ACCESS IS EXPECTED:
// In a P2P network, nodes receive transactions from multiple peers simultaneously.
// During signature verification, nodes call SignDocData() to reconstruct the signed
// bytes. Consider the case where:
//   - Peer A sends transaction T1 containing message M
//   - Peer B sends transaction T2 containing the same message M (e.g., relayed)
//   - The node verifies signatures from both peers concurrently
//   - Both verification goroutines call M.SignDocData() simultaneously
//
// If SignDocData() has shared mutable state without synchronization, this concurrent
// access can cause data races, corrupted output, or non-deterministic results.
//
// SAFE IMPLEMENTATION PATTERN:
// The simplest approach is to ensure SignDocData() has no shared mutable state.
// Use local variables and avoid lazy-initialized caches:
//
//	type MsgSend struct {
//	    From   string
//	    To     string
//	    Amount uint64
//	}
//
//	// GOOD: No shared state, safe for concurrent use
//	func (m *MsgSend) SignDocData() (json.RawMessage, error) {
//	    // Local struct ensures deterministic field ordering without shared state
//	    data := struct {
//	        From   string `json:"from"`
//	        To     string `json:"to"`
//	        Amount uint64 `json:"amount"`
//	    }{
//	        From:   m.From,
//	        To:     m.To,
//	        Amount: m.Amount,
//	    }
//	    return json.Marshal(data)
//	}
//
//	// BAD: Shared buffer causes data races under concurrent access
//	// var sharedBuffer bytes.Buffer  // Don't do this!
//
// RATIONALE: By default, only signers are included in the SignDoc message data.
// This loses information: the actual message content (e.g., recipient, amount)
// is not signed. Messages implementing this interface can include their full
// canonical representation, ensuring signatures bind to the complete message content.
//
// SECURITY: Including full message content prevents signature reuse attacks where
// an attacker might try to reuse a signature with different message parameters
// that happen to have the same signers.
//
// TESTING: Use punnettesting helpers to validate implementations:
//
//	func TestMyMessage_SignDocDataDeterminism(t *testing.T) {
//	    msg := &MyMessage{From: "alice", To: "bob", Amount: 100}
//	    // Test sequential determinism
//	    punnettesting.AssertSignDocDataDeterminism(t, msg, 100)
//	    // Test concurrent safety (run with -race flag)
//	    punnettesting.AssertSignDocDataDeterminismConcurrent(t, msg, 10, 100)
//	}
type SignDocSerializable interface {
	// SignDocData returns the canonical JSON representation of this message
	// for inclusion in SignDoc.
	//
	// PRECONDITION: The message is in a valid state (ValidateBasic would pass).
	//
	// POSTCONDITION: Returned JSON is deterministic and valid.
	// POSTCONDITION: The JSON encodes all fields relevant for signature binding.
	//
	// The implementation should use sorted keys for any maps to ensure determinism.
	// For struct types, Go's encoding/json produces deterministic output based on
	// field declaration order.
	//
	// THREAD SAFETY: This method must be safe for concurrent calls. Avoid:
	// - Shared mutable state without synchronization
	// - Lazy-initialized caches without proper locking
	// - Reusing buffers across calls
	SignDocData() (json.RawMessage, error)
}
