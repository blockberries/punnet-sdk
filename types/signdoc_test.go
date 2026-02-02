package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignDoc_NewSignDoc(t *testing.T) {
	sd := NewSignDoc("test-chain", 42, "alice", 1, "test memo")

	assert.Equal(t, SignDocVersion, sd.Version)
	assert.Equal(t, "test-chain", sd.ChainID)
	assert.Equal(t, uint64(42), sd.AccountSequence)
	assert.Equal(t, "alice", sd.Account)
	assert.Equal(t, uint64(1), sd.Nonce)
	assert.Equal(t, "test memo", sd.Memo)
	assert.Empty(t, sd.Messages)
}

func TestSignDoc_AddMessage(t *testing.T) {
	sd := NewSignDoc("test-chain", 1, "alice", 1, "")

	msgData := json.RawMessage(`{"to":"bob","amount":100}`)
	sd.AddMessage("/punnet.bank.v1.MsgSend", msgData)

	require.Len(t, sd.Messages, 1)
	assert.Equal(t, "/punnet.bank.v1.MsgSend", sd.Messages[0].Type)
	assert.Equal(t, msgData, sd.Messages[0].Data)
}

func TestSignDoc_ToJSON_Deterministic(t *testing.T) {
	// INVARIANT: Two calls to ToJSON on the same SignDoc must produce identical bytes.
	sd := NewSignDoc("test-chain", 1, "alice", 1, "test")
	sd.AddMessage("/msg.Type", json.RawMessage(`{"key":"value"}`))

	json1, err1 := sd.ToJSON()
	require.NoError(t, err1)

	json2, err2 := sd.ToJSON()
	require.NoError(t, err2)

	assert.Equal(t, json1, json2, "ToJSON must be deterministic")
}

func TestSignDoc_GetSignBytes(t *testing.T) {
	sd := NewSignDoc("test-chain", 1, "alice", 1, "")
	sd.AddMessage("/msg.Type", json.RawMessage(`{"key":"value"}`))

	signBytes, err := sd.GetSignBytes()
	require.NoError(t, err)

	// SHA-256 produces 32 bytes
	assert.Len(t, signBytes, 32)

	// INVARIANT: Same SignDoc produces same hash
	signBytes2, err2 := sd.GetSignBytes()
	require.NoError(t, err2)
	assert.Equal(t, signBytes, signBytes2)
}

func TestSignDoc_ValidateBasic(t *testing.T) {
	tests := []struct {
		name      string
		signDoc   *SignDoc
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid SignDoc",
			signDoc: func() *SignDoc {
				sd := NewSignDoc("test-chain", 1, "alice", 1, "")
				sd.AddMessage("/msg.Type", json.RawMessage(`{}`))
				return sd
			}(),
			expectErr: false,
		},
		{
			name: "invalid version",
			signDoc: &SignDoc{
				Version:  "99",
				ChainID:  "test",
				Account:  "alice",
				Messages: []SignDocMessage{{Type: "/msg", Data: json.RawMessage(`{}`)}},
			},
			expectErr: true,
			errMsg:    "unsupported SignDoc version",
		},
		{
			name: "empty chain ID",
			signDoc: &SignDoc{
				Version:  SignDocVersion,
				ChainID:  "",
				Account:  "alice",
				Messages: []SignDocMessage{{Type: "/msg", Data: json.RawMessage(`{}`)}},
			},
			expectErr: true,
			errMsg:    "chain_id cannot be empty",
		},
		{
			name: "empty account",
			signDoc: &SignDoc{
				Version:  SignDocVersion,
				ChainID:  "test",
				Account:  "",
				Messages: []SignDocMessage{{Type: "/msg", Data: json.RawMessage(`{}`)}},
			},
			expectErr: true,
			errMsg:    "account cannot be empty",
		},
		{
			name: "no messages",
			signDoc: &SignDoc{
				Version:  SignDocVersion,
				ChainID:  "test",
				Account:  "alice",
				Messages: []SignDocMessage{},
			},
			expectErr: true,
			errMsg:    "must contain at least one message",
		},
		{
			name: "message with empty type",
			signDoc: &SignDoc{
				Version:  SignDocVersion,
				ChainID:  "test",
				Account:  "alice",
				Messages: []SignDocMessage{{Type: "", Data: json.RawMessage(`{}`)}},
			},
			expectErr: true,
			errMsg:    "empty type",
		},
		{
			name: "too many messages (DoS protection)",
			signDoc: func() *SignDoc {
				sd := &SignDoc{
					Version: SignDocVersion,
					ChainID: "test",
					Account: "alice",
				}
				// Create more messages than allowed
				for i := 0; i <= MaxMessagesPerSignDoc; i++ {
					sd.Messages = append(sd.Messages, SignDocMessage{
						Type: "/msg.Type",
						Data: json.RawMessage(`{}`),
					})
				}
				return sd
			}(),
			expectErr: true,
			errMsg:    "too many messages",
		},
		{
			name: "message data too large (DoS protection)",
			signDoc: func() *SignDoc {
				// Create message data larger than MaxMessageDataSize
				largeData := make([]byte, MaxMessageDataSize+1)
				for i := range largeData {
					largeData[i] = 'x'
				}
				return &SignDoc{
					Version: SignDocVersion,
					ChainID: "test",
					Account: "alice",
					Messages: []SignDocMessage{{
						Type: "/msg.Type",
						Data: json.RawMessage(largeData),
					}},
				}
			}(),
			expectErr: true,
			errMsg:    "data too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.signDoc.ValidateBasic()
			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSignDoc_Equals(t *testing.T) {
	sd1 := NewSignDoc("test-chain", 1, "alice", 1, "memo")
	sd1.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	sd2 := NewSignDoc("test-chain", 1, "alice", 1, "memo")
	sd2.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	sd3 := NewSignDoc("different-chain", 1, "alice", 1, "memo")
	sd3.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	assert.True(t, sd1.Equals(sd2), "identical SignDocs should be equal")
	assert.False(t, sd1.Equals(sd3), "different SignDocs should not be equal")
	assert.False(t, sd1.Equals(nil), "should not equal nil")
}

func TestSignDoc_ParseSignDoc(t *testing.T) {
	original := NewSignDoc("test-chain", 42, "alice", 1, "memo")
	original.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	jsonBytes, err := original.ToJSON()
	require.NoError(t, err)

	parsed, err := ParseSignDoc(jsonBytes)
	require.NoError(t, err)

	assert.True(t, original.Equals(parsed), "parsed SignDoc should equal original")
}

func TestSignDoc_Roundtrip(t *testing.T) {
	// This tests the critical security property that serialization is deterministic.
	// INVARIANT: JSON -> parse -> JSON must produce identical bytes.

	original := NewSignDoc("test-chain", 1, "alice", 1, "")
	original.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`))
	original.AddMessage("/punnet.stake.v1.MsgDelegate", json.RawMessage(`{"delegator":"alice","validator":"val1"}`))

	json1, err := original.ToJSON()
	require.NoError(t, err)

	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)

	assert.Equal(t, json1, json2, "roundtrip must produce identical bytes")
}

func TestSortedJSONObject(t *testing.T) {
	// Test that keys are sorted alphabetically
	obj := sortedJSONObject{
		"zebra":    1,
		"apple":    2,
		"mango":    3,
		"banana":   4,
	}

	jsonBytes, err := json.Marshal(obj)
	require.NoError(t, err)

	expected := `{"apple":2,"banana":4,"mango":3,"zebra":1}`
	assert.Equal(t, expected, string(jsonBytes))
}
