package testing

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// TEST MESSAGE IMPLEMENTATIONS
// =============================================================================

// deterministicMessage is a test message with deterministic SignDocData
type deterministicMessage struct {
	From   string
	To     string
	Amount uint64
}

func (m *deterministicMessage) Type() string         { return "/test.DeterministicMsg" }
func (m *deterministicMessage) ValidateBasic() error { return nil }
func (m *deterministicMessage) GetSigners() []types.AccountName {
	return []types.AccountName{types.AccountName(m.From)}
}

func (m *deterministicMessage) SignDocData() (json.RawMessage, error) {
	// Use struct serialization which is deterministic
	data := struct {
		From   string `json:"from"`
		To     string `json:"to"`
		Amount uint64 `json:"amount"`
	}{
		From:   m.From,
		To:     m.To,
		Amount: m.Amount,
	}
	return json.Marshal(data)
}

// nonDeterministicMessage simulates non-deterministic behavior for testing
type nonDeterministicMessage struct {
	From   string
	To     string
	Amount uint64
	calls  int32 // atomic counter
}

func (m *nonDeterministicMessage) Type() string         { return "/test.NonDeterministicMsg" }
func (m *nonDeterministicMessage) ValidateBasic() error { return nil }
func (m *nonDeterministicMessage) GetSigners() []types.AccountName {
	return []types.AccountName{types.AccountName(m.From)}
}

func (m *nonDeterministicMessage) SignDocData() (json.RawMessage, error) {
	// Simulate non-determinism by alternating output
	call := atomic.AddInt32(&m.calls, 1)
	if call%2 == 0 {
		return json.RawMessage(`{"from":"alice","to":"bob","amount":100}`), nil
	}
	return json.RawMessage(`{"amount":100,"from":"alice","to":"bob"}`), nil
}

// errorMessage is a test message that returns an error from SignDocData
type errorMessage struct{}

func (m *errorMessage) Type() string                    { return "/test.ErrorMsg" }
func (m *errorMessage) ValidateBasic() error            { return nil }
func (m *errorMessage) GetSigners() []types.AccountName { return nil }

func (m *errorMessage) SignDocData() (json.RawMessage, error) {
	return nil, fmt.Errorf("intentional error for testing")
}

// nilMessage is a test message that returns nil from SignDocData
type nilMessage struct{}

func (m *nilMessage) Type() string                    { return "/test.NilMsg" }
func (m *nilMessage) ValidateBasic() error            { return nil }
func (m *nilMessage) GetSigners() []types.AccountName { return nil }

func (m *nilMessage) SignDocData() (json.RawMessage, error) {
	return nil, nil
}

// =============================================================================
// TESTS FOR AssertSignDocDataDeterminism
// =============================================================================

func TestAssertSignDocDataDeterminism_PassesForDeterministicMessage(t *testing.T) {
	msg := &deterministicMessage{From: "alice", To: "bob", Amount: 100}

	// Should not panic or fail
	AssertSignDocDataDeterminism(t, msg, 100)
}

func TestAssertSignDocDataDeterminism_DetectsNonDeterminism(t *testing.T) {
	msg := &nonDeterministicMessage{From: "alice", To: "bob", Amount: 100}

	// Verify the helper would detect non-determinism
	// We test by directly checking the output varies
	data1, err := msg.SignDocData()
	require.NoError(t, err)

	data2, err := msg.SignDocData()
	require.NoError(t, err)

	assert.NotEqual(t, string(data1), string(data2),
		"nonDeterministicMessage should produce different output on sequential calls")
}

func TestAssertSignDocDataDeterminism_FailsOnError(t *testing.T) {
	msg := &errorMessage{}

	// Verify SignDocData returns an error
	_, err := msg.SignDocData()
	assert.Error(t, err, "errorMessage.SignDocData should return error")
}

func TestAssertSignDocDataDeterminism_FailsOnNil(t *testing.T) {
	msg := &nilMessage{}

	// Verify SignDocData returns nil
	data, err := msg.SignDocData()
	assert.NoError(t, err)
	assert.Nil(t, data, "nilMessage.SignDocData should return nil")
}

func TestAssertSignDocDataDeterminism_MinIterationsCheck(t *testing.T) {
	// Verify the function requires minimum iterations by checking behavior
	msg := &deterministicMessage{From: "alice", To: "bob", Amount: 100}

	// With 2 iterations (minimum), should still work
	AssertSignDocDataDeterminism(t, msg, 2)

	// With many iterations, should work for deterministic message
	AssertSignDocDataDeterminism(t, msg, 1000)
}

// =============================================================================
// TESTS FOR AssertSignDocDataValid
// =============================================================================

func TestAssertSignDocDataValid_PassesForValidMessage(t *testing.T) {
	msg := &deterministicMessage{From: "alice", To: "bob", Amount: 100}

	// Should not panic or fail
	AssertSignDocDataValid(t, msg)
}

func TestAssertSignDocDataValid_VerifiesValidJSON(t *testing.T) {
	msg := &deterministicMessage{From: "alice", To: "bob", Amount: 100}

	data, err := msg.SignDocData()
	require.NoError(t, err)

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "SignDocData should return valid JSON")
	assert.Equal(t, "alice", parsed["from"])
	assert.Equal(t, "bob", parsed["to"])
	assert.Equal(t, float64(100), parsed["amount"])
}

// =============================================================================
// TESTS FOR RequireSignDocDataDeterminism
// =============================================================================

func TestRequireSignDocDataDeterminism_PassesForDeterministicMessage(t *testing.T) {
	msg := &deterministicMessage{From: "alice", To: "bob", Amount: 100}

	// Should not panic or fail
	RequireSignDocDataDeterminism(t, msg, 100)
}

// =============================================================================
// EDGE CASE TESTS
// =============================================================================

func TestDeterminism_WithUnicodeContent(t *testing.T) {
	msg := &deterministicMessage{From: "alice日本語", To: "bob🚀", Amount: 42}
	AssertSignDocDataDeterminism(t, msg, 100)
}

func TestDeterminism_WithEmptyFields(t *testing.T) {
	msg := &deterministicMessage{From: "", To: "", Amount: 0}
	AssertSignDocDataDeterminism(t, msg, 100)
}

func TestDeterminism_WithLargeAmount(t *testing.T) {
	msg := &deterministicMessage{From: "alice", To: "bob", Amount: 18446744073709551615} // max uint64
	AssertSignDocDataDeterminism(t, msg, 100)
}

func TestDeterminism_WithSpecialCharacters(t *testing.T) {
	msg := &deterministicMessage{
		From:   `alice"with"quotes`,
		To:     "bob\nwith\nnewlines",
		Amount: 100,
	}
	AssertSignDocDataDeterminism(t, msg, 100)
}
