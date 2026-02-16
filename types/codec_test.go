package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testMsg is a simple Message implementation for testing the codec.
type testMsg struct {
	Field string `json:"field"`
}

func (m *testMsg) Type() string                { return "test/TestMsg" }
func (m *testMsg) ValidateBasic() error        { return nil }
func (m *testMsg) GetSigners() []AccountName   { return []AccountName{"alice"} }

// testMsg2 is a second Message type for mixed-type tests.
type testMsg2 struct {
	Value int `json:"value"`
}

func (m *testMsg2) Type() string                { return "test/TestMsg2" }
func (m *testMsg2) ValidateBasic() error        { return nil }
func (m *testMsg2) GetSigners() []AccountName   { return []AccountName{"bob"} }

func init() {
	RegisterMessage("test/TestMsg", func() Message { return &testMsg{} })
	RegisterMessage("test/TestMsg2", func() Message { return &testMsg2{} })
}

func TestRegisterAndLookup(t *testing.T) {
	factory, ok := GetMessageFactory("test/TestMsg")
	require.True(t, ok)
	require.NotNil(t, factory)

	msg := factory()
	assert.Equal(t, "test/TestMsg", msg.Type())
}

func TestLookupUnregistered(t *testing.T) {
	factory, ok := GetMessageFactory("nonexistent/Msg")
	assert.False(t, ok)
	assert.Nil(t, factory)
}

func TestMarshalUnmarshalMessageJSON(t *testing.T) {
	original := &testMsg{Field: "hello"}

	data, err := MarshalMessageJSON(original)
	require.NoError(t, err)

	// Verify envelope structure
	var envelope MessageEnvelope
	require.NoError(t, json.Unmarshal(data, &envelope))
	assert.Equal(t, "test/TestMsg", envelope.Type)

	// Unmarshal back
	decoded, err := UnmarshalMessageJSON(data)
	require.NoError(t, err)

	concrete, ok := decoded.(*testMsg)
	require.True(t, ok)
	assert.Equal(t, "hello", concrete.Field)
}

func TestUnmarshalUnknownType(t *testing.T) {
	data := []byte(`{"type":"unknown/Msg","value":{}}`)
	_, err := UnmarshalMessageJSON(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown message type")
}

func TestUnmarshalMissingType(t *testing.T) {
	data := []byte(`{"value":{}}`)
	_, err := UnmarshalMessageJSON(data)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing type field")
}

func TestMarshalNilMessage(t *testing.T) {
	_, err := MarshalMessageJSON(nil)
	assert.Error(t, err)
}

func TestTransactionJSONRoundTrip(t *testing.T) {
	original := &Transaction{
		Account: "alice",
		Messages: []Message{
			&testMsg{Field: "transfer"},
			&testMsg2{Value: 42},
		},
		Authorization: &Authorization{
			Signatures: []Signature{
				{PubKey: []byte("pub"), Signature: []byte("sig")},
			},
		},
		Nonce: 5,
		Memo:  "test memo",
	}

	// Marshal
	data, err := json.Marshal(original)
	require.NoError(t, err)

	// Verify the JSON contains message envelopes with type discrimination
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))

	var messages []MessageEnvelope
	require.NoError(t, json.Unmarshal(raw["messages"], &messages))
	assert.Len(t, messages, 2)
	assert.Equal(t, "test/TestMsg", messages[0].Type)
	assert.Equal(t, "test/TestMsg2", messages[1].Type)

	// Unmarshal back
	var decoded Transaction
	require.NoError(t, json.Unmarshal(data, &decoded))

	assert.Equal(t, AccountName("alice"), decoded.Account)
	assert.Len(t, decoded.Messages, 2)
	assert.Equal(t, uint64(5), decoded.Nonce)
	assert.Equal(t, "test memo", decoded.Memo)

	msg0, ok := decoded.Messages[0].(*testMsg)
	require.True(t, ok)
	assert.Equal(t, "transfer", msg0.Field)

	msg1, ok := decoded.Messages[1].(*testMsg2)
	require.True(t, ok)
	assert.Equal(t, 42, msg1.Value)
}

func TestTransactionJSONNoMessages(t *testing.T) {
	original := &Transaction{
		Account:       "alice",
		Messages:      []Message{},
		Authorization: &Authorization{},
		Nonce:         1,
	}

	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded Transaction
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Empty(t, decoded.Messages)
}

func TestGetSignBytesIncludesContent(t *testing.T) {
	tx1 := &Transaction{
		Account:  "alice",
		Messages: []Message{&testMsg{Field: "value1"}},
		Nonce:    1,
	}
	tx2 := &Transaction{
		Account:  "alice",
		Messages: []Message{&testMsg{Field: "value2"}},
		Nonce:    1,
	}

	// Different message content must produce different sign bytes
	sign1 := tx1.GetSignBytes()
	sign2 := tx2.GetSignBytes()
	assert.NotEqual(t, sign1, sign2, "different message content should produce different sign bytes")
}
