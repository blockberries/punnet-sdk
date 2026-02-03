package types

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// SIGNDOCSERIALIZABLE INTERFACE TESTS
// =============================================================================

// serializableMessage is a test message that implements SignDocSerializable
type serializableMessage struct {
	MsgType string        `json:"type"`
	Signers []AccountName `json:"signers"`
	From    string        `json:"from"`
	To      string        `json:"to"`
	Amount  uint64        `json:"amount"`
	Denom   string        `json:"denom"`
}

func (m *serializableMessage) Type() string              { return m.MsgType }
func (m *serializableMessage) ValidateBasic() error      { return nil }
func (m *serializableMessage) GetSigners() []AccountName { return m.Signers }

// SignDocData implements SignDocSerializable
// INVARIANT: Returns deterministic JSON with full message content
func (m *serializableMessage) SignDocData() (json.RawMessage, error) {
	// Use sorted keys via struct marshaling (Go structs serialize in field order)
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

// failingMessage is a test message where SignDocData returns an error
type failingMessage struct {
	MsgType string        `json:"type"`
	Signers []AccountName `json:"signers"`
}

func (m *failingMessage) Type() string              { return m.MsgType }
func (m *failingMessage) ValidateBasic() error      { return nil }
func (m *failingMessage) GetSigners() []AccountName { return m.Signers }

// SignDocData implements SignDocSerializable but always fails
func (m *failingMessage) SignDocData() (json.RawMessage, error) {
	return nil, fmt.Errorf("intentional failure for testing")
}

// =============================================================================
// TESTS
// =============================================================================

func TestConvertMessages_WithSignDocSerializable(t *testing.T) {
	// INVARIANT: Messages implementing SignDocSerializable use their full content
	msg := &serializableMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
		From:    "alice",
		To:      "bob",
		Amount:  1000,
		Denom:   "uatom",
	}

	result, err := convertMessages([]Message{msg})
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, "/punnet.bank.v1.MsgSend", result[0].Type)

	// Verify full message content is included (not just signers)
	var data map[string]interface{}
	err = json.Unmarshal(result[0].Data, &data)
	require.NoError(t, err)

	assert.Equal(t, "alice", data["from"])
	assert.Equal(t, "bob", data["to"])
	assert.Equal(t, float64(1000), data["amount"]) // JSON numbers are float64
	assert.Equal(t, "uatom", data["denom"])

	// Signers should NOT be in the data (it's the full message, not fallback)
	_, hasSigners := data["signers"]
	assert.False(t, hasSigners, "SignDocSerializable message should not have signers field")
}

func TestConvertMessages_WithoutSignDocSerializable(t *testing.T) {
	// INVARIANT: Messages not implementing SignDocSerializable use signers-only fallback
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	result, err := convertMessages([]Message{msg})
	require.NoError(t, err)
	require.Len(t, result, 1)

	// Should contain only signers (fallback behavior)
	var data map[string][]string
	err = json.Unmarshal(result[0].Data, &data)
	require.NoError(t, err)

	assert.Equal(t, []string{"alice"}, data["signers"])
}

func TestConvertMessages_MixedMessages(t *testing.T) {
	// INVARIANT: Each message is handled according to whether it implements the interface
	msg1 := &serializableMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
		From:    "alice",
		To:      "bob",
		Amount:  1000,
		Denom:   "uatom",
	}
	msg2 := &testMessage{
		MsgType: "/punnet.staking.v1.MsgDelegate",
		Signers: []AccountName{"alice"},
	}

	result, err := convertMessages([]Message{msg1, msg2})
	require.NoError(t, err)
	require.Len(t, result, 2)

	// First message should have full content
	var data1 map[string]interface{}
	err = json.Unmarshal(result[0].Data, &data1)
	require.NoError(t, err)
	assert.Equal(t, "alice", data1["from"])
	assert.Equal(t, "bob", data1["to"])

	// Second message should have only signers
	var data2 map[string][]string
	err = json.Unmarshal(result[1].Data, &data2)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice"}, data2["signers"])
}

func TestConvertMessages_SignDocDataError(t *testing.T) {
	// INVARIANT: Errors from SignDocData are propagated (not swallowed)
	msg := &failingMessage{
		MsgType: "/punnet.test.v1.FailingMsg",
		Signers: []AccountName{"alice"},
	}

	result, err := convertMessages([]Message{msg})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SignDocData failed")
	assert.Contains(t, err.Error(), "intentional failure")
}

func TestToSignDoc_WithSignDocSerializable(t *testing.T) {
	// Full integration test: SignDocSerializable message through ToSignDoc
	msg := &serializableMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
		From:    "alice",
		To:      "bob",
		Amount:  1000,
		Denom:   "uatom",
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    42,
		Memo:     "test transfer",
		Fee: Fee{
			Amount:   Coins{{Denom: "uatom", Amount: 500}},
			GasLimit: 100000,
		},
		FeeSlippage: Ratio{
			Numerator:   1,
			Denominator: 100,
		},
	}

	signDoc, err := tx.ToSignDoc("test-chain", 42)
	require.NoError(t, err)

	// Verify basic SignDoc fields
	assert.Equal(t, SignDocVersion, signDoc.Version)
	assert.Equal(t, "test-chain", signDoc.ChainID)
	assert.Equal(t, "alice", signDoc.Account)
	require.Len(t, signDoc.Messages, 1)

	// Verify message data includes full content
	var msgData map[string]interface{}
	err = json.Unmarshal(signDoc.Messages[0].Data, &msgData)
	require.NoError(t, err)

	assert.Equal(t, "alice", msgData["from"])
	assert.Equal(t, "bob", msgData["to"])
	assert.Equal(t, float64(1000), msgData["amount"])
	assert.Equal(t, "uatom", msgData["denom"])
}

func TestToSignDoc_SignDocDataError(t *testing.T) {
	// Verify ToSignDoc propagates SignDocData errors
	msg := &failingMessage{
		MsgType: "/punnet.test.v1.FailingMsg",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    42,
	}

	signDoc, err := tx.ToSignDoc("test-chain", 42)

	assert.Nil(t, signDoc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to convert messages")
}

func TestSignDocSerializable_Determinism(t *testing.T) {
	// INVARIANT: SignDocData must be deterministic
	msg := &serializableMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
		From:    "alice",
		To:      "bob",
		Amount:  1000,
		Denom:   "uatom",
	}

	// Call multiple times
	data1, err := msg.SignDocData()
	require.NoError(t, err)

	data2, err := msg.SignDocData()
	require.NoError(t, err)

	// Must produce identical bytes
	assert.Equal(t, string(data1), string(data2))
}

func TestSignDocRoundtrip_WithSignDocSerializable(t *testing.T) {
	// Verify roundtrip validation passes with SignDocSerializable messages
	msg := &serializableMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
		From:    "alice",
		To:      "bob",
		Amount:  1000,
		Denom:   "uatom",
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    42,
		Fee: Fee{
			Amount:   Coins{{Denom: "uatom", Amount: 500}},
			GasLimit: 100000,
		},
		FeeSlippage: Ratio{
			Numerator:   1,
			Denominator: 100,
		},
	}

	err := tx.ValidateSignDocRoundtrip("test-chain", 42)
	assert.NoError(t, err)
}
