package testing_test

import (
	"encoding/json"
	"testing"

	punnettesting "github.com/blockberries/punnet-sdk/testing"
	"github.com/blockberries/punnet-sdk/types"
)

// ExampleMessage demonstrates a message implementing SignDocSerializable.
// This is an example for documentation purposes.
type ExampleMessage struct {
	From   string
	To     string
	Amount uint64
	Denom  string
}

func (m *ExampleMessage) Type() string         { return "/example.MsgSend" }
func (m *ExampleMessage) ValidateBasic() error { return nil }
func (m *ExampleMessage) GetSigners() []types.AccountName {
	return []types.AccountName{types.AccountName(m.From)}
}

// SignDocData returns the canonical JSON representation for signing.
// This implementation uses struct serialization which is deterministic
// because Go's encoding/json marshals struct fields in declaration order.
func (m *ExampleMessage) SignDocData() (json.RawMessage, error) {
	data := struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Amount uint64 `json:"amount"`
		Denom  string `json:"denom"`
	}{
		From:   m.From,
		To:     m.To,
		Amount: m.Amount,
		Denom:  m.Denom,
	}
	return json.Marshal(data)
}

// TestExampleMessage_Determinism shows the recommended pattern for testing
// SignDocSerializable implementations.
func TestExampleMessage_Determinism(t *testing.T) {
	msg := &ExampleMessage{
		From:   "alice",
		To:     "bob",
		Amount: 1000,
		Denom:  "uatom",
	}

	// Use 100 iterations for thorough validation
	// This catches map iteration order issues that may not manifest on every call
	punnettesting.AssertSignDocDataDeterminism(t, msg, 100)
}

// TestExampleMessage_Valid shows comprehensive validation of a SignDocSerializable
// implementation including determinism and JSON validity.
func TestExampleMessage_Valid(t *testing.T) {
	msg := &ExampleMessage{
		From:   "alice",
		To:     "bob",
		Amount: 1000,
		Denom:  "uatom",
	}

	punnettesting.AssertSignDocDataValid(t, msg)
}

// TestExampleMessage_Determinism_EdgeCases shows testing with various edge case values.
func TestExampleMessage_Determinism_EdgeCases(t *testing.T) {
	testCases := []struct {
		name string
		msg  *ExampleMessage
	}{
		{
			name: "empty_strings",
			msg:  &ExampleMessage{From: "", To: "", Amount: 0, Denom: ""},
		},
		{
			name: "unicode",
			msg:  &ExampleMessage{From: "alice日本語", To: "bob🚀", Amount: 42, Denom: "ustake"},
		},
		{
			name: "max_uint64",
			msg:  &ExampleMessage{From: "alice", To: "bob", Amount: ^uint64(0), Denom: "uatom"},
		},
		{
			name: "special_chars",
			msg:  &ExampleMessage{From: `alice"quoted"`, To: "bob\nline", Amount: 100, Denom: "uatom"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			punnettesting.AssertSignDocDataDeterminism(t, tc.msg, 100)
		})
	}
}
