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
// INVARIANT: SignDocData() must be safe for concurrent calls from multiple goroutines.
// This is required because validator nodes may process multiple transactions
// simultaneously, calling SignDocData() concurrently on the same message instance.
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
