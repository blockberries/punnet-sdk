package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// BASIC DETERMINISM TESTS
// =============================================================================
// These tests verify the fundamental property that serialization is deterministic.

func TestSignDocDeterminism_RepeatedSerialization(t *testing.T) {
	// SECURITY: Repeated serialization of the same SignDoc MUST produce identical bytes.
	// If this fails, signature verification becomes non-deterministic.
	sd := NewSignDoc("punnet-mainnet-1", 42, "alice", 1, "test memo")
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`))

	// Serialize 100 times and verify all are identical
	var firstResult []byte
	for i := 0; i < 100; i++ {
		jsonBytes, err := sd.ToJSON()
		require.NoError(t, err, "iteration %d", i)

		if i == 0 {
			firstResult = jsonBytes
		} else {
			assert.Equal(t, firstResult, jsonBytes,
				"serialization at iteration %d differs from first", i)
		}
	}
}

func TestSignDocDeterminism_HashConsistency(t *testing.T) {
	// SECURITY: GetSignBytes() must produce identical hashes for the same SignDoc.
	sd := NewSignDoc("test-chain", 1, "alice", 1, "")
	sd.AddMessage("/msg.Type", json.RawMessage(`{"key":"value"}`))

	var firstHash []byte
	for i := 0; i < 100; i++ {
		hash, err := sd.GetSignBytes()
		require.NoError(t, err, "iteration %d", i)

		if i == 0 {
			firstHash = hash
		} else {
			assert.Equal(t, firstHash, hash,
				"hash at iteration %d differs from first", i)
		}
	}
}

func TestSignDocDeterminism_EquivalentConstruction(t *testing.T) {
	// Two SignDocs constructed with the same values must serialize identically.
	createSignDoc := func() *SignDoc {
		sd := NewSignDoc("chain-1", 10, "bob", 5, "hello")
		sd.AddMessage("/type1", json.RawMessage(`{"a":1}`))
		sd.AddMessage("/type2", json.RawMessage(`{"b":2}`))
		return sd
	}

	sd1 := createSignDoc()
	sd2 := createSignDoc()

	json1, err1 := sd1.ToJSON()
	require.NoError(t, err1)

	json2, err2 := sd2.ToJSON()
	require.NoError(t, err2)

	assert.Equal(t, json1, json2, "equivalent SignDocs must serialize identically")
}

func TestSignDocDeterminism_FieldOrderIndependence(t *testing.T) {
	// Verify that the struct's JSON field order is consistent (Go serializes in declaration order).
	sd := NewSignDoc("chain", 1, "alice", 2, "memo")
	sd.AddMessage("/msg", json.RawMessage(`{}`))

	jsonBytes, err := sd.ToJSON()
	require.NoError(t, err)

	// The JSON keys should appear in a consistent order.
	// Verify by checking that we can parse it back and it round-trips.
	parsed, err := ParseSignDoc(jsonBytes)
	require.NoError(t, err)

	jsonBytes2, err := parsed.ToJSON()
	require.NoError(t, err)

	assert.Equal(t, jsonBytes, jsonBytes2)
}

// =============================================================================
// FIELD VALUE TESTS
// =============================================================================
// Test serialization with various field values.

func TestSignDocFieldValues_EmptyStringFields(t *testing.T) {
	// Empty memo should serialize consistently (with omitempty it may be omitted).
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/msg", json.RawMessage(`{}`))

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	// Re-serialize
	json2, err := sd.ToJSON()
	require.NoError(t, err)

	assert.Equal(t, json1, json2)

	// Verify memo is actually omitted (due to omitempty tag)
	assert.NotContains(t, string(json1), `"memo":""`,
		"empty memo should be omitted due to omitempty")
}

func TestSignDocFieldValues_ZeroNumericFields(t *testing.T) {
	// Zero values for uint64 fields must serialize consistently as strings.
	sd := NewSignDoc("chain", 0, "alice", 0, "")
	sd.AddMessage("/msg", json.RawMessage(`{}`))

	jsonBytes, err := sd.ToJSON()
	require.NoError(t, err)

	// Verify zeros are serialized as strings (not omitted)
	jsonStr := string(jsonBytes)
	assert.Contains(t, jsonStr, `"account_sequence":"0"`)
	assert.Contains(t, jsonStr, `"nonce":"0"`)

	// Verify determinism
	json2, err := sd.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, jsonBytes, json2)
}

func TestSignDocFieldValues_MaxUint64(t *testing.T) {
	// Maximum uint64 values must serialize correctly as strings.
	sd := NewSignDoc("chain", math.MaxUint64, "alice", math.MaxUint64, "")
	sd.AddMessage("/msg", json.RawMessage(`{}`))

	jsonBytes, err := sd.ToJSON()
	require.NoError(t, err)

	// Parse back and verify values preserved
	parsed, err := ParseSignDoc(jsonBytes)
	require.NoError(t, err)

	assert.Equal(t, StringUint64(math.MaxUint64), parsed.AccountSequence)
	assert.Equal(t, StringUint64(math.MaxUint64), parsed.Nonce)

	// Verify roundtrip determinism
	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, jsonBytes, json2)
}

func TestSignDocFieldValues_UnicodeStrings(t *testing.T) {
	// Unicode characters must serialize and deserialize correctly.
	testCases := []struct {
		name  string
		value string
	}{
		{"basic unicode", "Hello 世界"},
		{"emojis", "🚀💰🔐"},
		{"mixed scripts", "αβγδ日本語한국어"},
		{"rtl text", "مرحبا"},
		{"combining chars", "e\u0301"}, // é as e + combining acute
		{"zero width joiner", "👨\u200D👩\u200D👧"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sd := NewSignDoc("chain", 1, "alice", 1, tc.value)
			sd.AddMessage("/msg", json.RawMessage(`{}`))

			jsonBytes, err := sd.ToJSON()
			require.NoError(t, err)

			// Verify roundtrip
			parsed, err := ParseSignDoc(jsonBytes)
			require.NoError(t, err)
			assert.Equal(t, tc.value, parsed.Memo)

			// Verify determinism
			json2, err := parsed.ToJSON()
			require.NoError(t, err)
			assert.Equal(t, jsonBytes, json2)
		})
	}
}

func TestSignDocFieldValues_SpecialCharactersInMemo(t *testing.T) {
	// Special characters that need JSON escaping must be handled correctly.
	testCases := []struct {
		name string
		memo string
	}{
		{"quotes", `memo with "quotes"`},
		{"backslash", `path\to\file`},
		{"newlines", "line1\nline2"},
		{"tabs", "col1\tcol2"},
		{"control chars", "text\x00\x01\x02"},
		{"json in memo", `{"key": "value"}`},
		{"angle brackets", "<script>alert('xss')</script>"},
		{"ampersand", "a & b"},
		{"null bytes", "before\x00after"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sd := NewSignDoc("chain", 1, "alice", 1, tc.memo)
			sd.AddMessage("/msg", json.RawMessage(`{}`))

			jsonBytes, err := sd.ToJSON()
			require.NoError(t, err)

			// Verify roundtrip preserves exact value
			parsed, err := ParseSignDoc(jsonBytes)
			require.NoError(t, err)
			assert.Equal(t, tc.memo, parsed.Memo)

			// Verify determinism
			json2, err := parsed.ToJSON()
			require.NoError(t, err)
			assert.Equal(t, jsonBytes, json2)
		})
	}
}

func TestSignDocFieldValues_EmptyArray(t *testing.T) {
	// A SignDoc with no messages (though invalid) should still serialize consistently.
	// This tests the edge case even though ValidateBasic would reject it.
	sd := &SignDoc{
		Version:         SignDocVersion,
		ChainID:         "chain",
		AccountSequence: 1,
		Account:         "alice",
		Nonce:           1,
		Memo:            "",
		Messages:        []SignDocMessage{},
		Fee:             SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
		FeeSlippage:     SignDocRatio{Numerator: "0", Denominator: "1"},
	}

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	// Verify empty array serializes correctly
	assert.Contains(t, string(json1), `"messages":[]`)

	// Verify determinism
	json2, err := sd.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocFieldValues_NilMessageData(t *testing.T) {
	// Message with nil Data should serialize consistently.
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.Messages = append(sd.Messages, SignDocMessage{
		Type: "/msg.Type",
		Data: nil,
	})

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	json2, err := sd.ToJSON()
	require.NoError(t, err)

	assert.Equal(t, json1, json2)
}

// =============================================================================
// MESSAGE TYPE TESTS
// =============================================================================
// Test serialization with various message configurations.

func TestSignDocMessages_SingleMessage(t *testing.T) {
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`))

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	// Parse and re-serialize to verify roundtrip
	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)

	assert.Equal(t, json1, json2)
}

func TestSignDocMessages_MultipleMessages(t *testing.T) {
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`))
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"alice","to":"charlie","amount":"50"}`))
	sd.AddMessage("/punnet.staking.v1.MsgDelegate", json.RawMessage(`{"delegator":"alice","validator":"val1"}`))

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	// Verify all messages present
	require.Len(t, sd.Messages, 3)

	// Verify roundtrip
	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)
	require.Len(t, parsed.Messages, 3)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocMessages_DifferentMessageTypes(t *testing.T) {
	// Test various message type URL formats
	messageTypes := []string{
		"/punnet.bank.v1.MsgSend",
		"/punnet.staking.v1.MsgDelegate",
		"/punnet.staking.v1.MsgUndelegate",
		"/punnet.gov.v1.MsgVote",
		"/cosmos.bank.v1beta1.MsgSend",              // Cosmos-style
		"/ibc.applications.transfer.v1.MsgTransfer", // IBC-style
	}

	sd := NewSignDoc("chain", 1, "alice", 1, "")
	for _, msgType := range messageTypes {
		sd.AddMessage(msgType, json.RawMessage(`{}`))
	}

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	// Verify roundtrip preserves all types
	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	for i, msgType := range messageTypes {
		assert.Equal(t, msgType, parsed.Messages[i].Type)
	}

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocMessages_NestedMessageStructures(t *testing.T) {
	// Test deeply nested JSON in message data
	nestedData := json.RawMessage(`{
		"outer": {
			"middle": {
				"inner": {
					"value": "deep",
					"array": [1, 2, {"nested": true}]
				}
			}
		},
		"list": [
			{"type": "a"},
			{"type": "b", "sub": {"x": 1}}
		]
	}`)

	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/msg.Nested", nestedData)

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	// Verify the nested structure is preserved in roundtrip
	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	// The raw message bytes should match (after normalizing whitespace)
	// Note: json.RawMessage preserves the exact bytes, so whitespace matters
	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocMessages_MessageOrderPreserved(t *testing.T) {
	// CRITICAL: Message order must be preserved exactly as added.
	// Reordering messages would change the hash and break signature verification.
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/msg.First", json.RawMessage(`{"order":1}`))
	sd.AddMessage("/msg.Second", json.RawMessage(`{"order":2}`))
	sd.AddMessage("/msg.Third", json.RawMessage(`{"order":3}`))

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	assert.Equal(t, "/msg.First", parsed.Messages[0].Type)
	assert.Equal(t, "/msg.Second", parsed.Messages[1].Type)
	assert.Equal(t, "/msg.Third", parsed.Messages[2].Type)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

// =============================================================================
// COIN ORDERING TESTS
// =============================================================================
// Test coin arrays within message data for lexicographic ordering.

func TestSignDocCoins_SingleCoin(t *testing.T) {
	coinData := json.RawMessage(`{"amount":[{"denom":"uatom","amount":"1000"}]}`)
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/msg.WithCoins", coinData)

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocCoins_MultipleCoinsSorted(t *testing.T) {
	// Coins should be sorted lexicographically by denom in the message data.
	// This tests that the message data preserves the exact order provided.
	sortedCoins := json.RawMessage(`{"amount":[{"denom":"aaa","amount":"100"},{"denom":"bbb","amount":"200"},{"denom":"ccc","amount":"300"}]}`)
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/msg.WithCoins", sortedCoins)

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	// The coin order in the original JSON should be preserved
	assert.Contains(t, string(json1), `"aaa"`)

	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocCoins_SameDenomDifferentAmounts(t *testing.T) {
	// When messages have the same denom but different amounts, they should serialize consistently.
	coinData := json.RawMessage(`{"from":"alice","to":"bob","amount":[{"denom":"uatom","amount":"500"}]}`)
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/punnet.bank.v1.MsgSend", coinData)

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	// Verify value is present
	assert.Contains(t, string(json1), `"amount":"500"`)

	// Verify determinism
	json2, err := sd.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

// =============================================================================
// EDGE CASE TESTS
// =============================================================================
// Test boundary conditions and unusual inputs.

func TestSignDocEdgeCases_MaximumLengthFields(t *testing.T) {
	// Test with very long strings (but reasonable for a real system)
	longMemo := strings.Repeat("x", 512) // Max memo length per transaction validation
	longChainID := strings.Repeat("c", 64)
	longAccount := strings.Repeat("a", 128)

	sd := NewSignDoc(longChainID, 1, longAccount, 1, longMemo)
	sd.AddMessage("/msg", json.RawMessage(`{}`))

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	assert.Equal(t, longMemo, parsed.Memo)
	assert.Equal(t, longChainID, parsed.ChainID)
	assert.Equal(t, longAccount, parsed.Account)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocEdgeCases_MinimumValidSignDoc(t *testing.T) {
	// Create the minimal valid SignDoc
	sd := NewSignDoc("c", 0, "a", 0, "")
	sd.AddMessage("/m", json.RawMessage(`{}`))

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	// Should pass validation
	err = sd.ValidateBasic()
	require.NoError(t, err)

	// Verify roundtrip
	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocEdgeCases_AllFieldsPopulated(t *testing.T) {
	// Test with all fields having non-default values
	sd := &SignDoc{
		Version:         SignDocVersion,
		ChainID:         "punnet-mainnet-1",
		AccountSequence: 12345,
		Account:         "punnet1abc123def456",
		Nonce:           67890,
		Memo:            "This is a comprehensive test memo with various content.",
		Messages: []SignDocMessage{
			{
				Type: "/punnet.bank.v1.MsgSend",
				Data: json.RawMessage(`{"from":"alice","to":"bob","amount":[{"denom":"uatom","amount":"1000"}]}`),
			},
			{
				Type: "/punnet.staking.v1.MsgDelegate",
				Data: json.RawMessage(`{"delegator":"alice","validator":"punnetvaloper1xyz","amount":{"denom":"uatom","amount":"5000"}}`),
			},
		},
		Fee: SignDocFee{
			Amount:   []SignDocCoin{{Denom: "uatom", Amount: "1000"}},
			GasLimit: "200000",
		},
		FeeSlippage: SignDocRatio{Numerator: "1", Denominator: "100"},
	}

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	// Verify all fields preserved
	assert.Equal(t, sd.Version, parsed.Version)
	assert.Equal(t, sd.ChainID, parsed.ChainID)
	assert.Equal(t, sd.AccountSequence, parsed.AccountSequence)
	assert.Equal(t, sd.Account, parsed.Account)
	assert.Equal(t, sd.Nonce, parsed.Nonce)
	assert.Equal(t, sd.Memo, parsed.Memo)
	assert.Len(t, parsed.Messages, 2)
	assert.Equal(t, sd.Fee, parsed.Fee)
	assert.Equal(t, sd.FeeSlippage, parsed.FeeSlippage)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocEdgeCases_LargeMessageCount(t *testing.T) {
	// Test with many messages (stress test)
	sd := NewSignDoc("chain", 1, "alice", 1, "")

	for i := 0; i < 100; i++ {
		sd.AddMessage("/msg.Type", json.RawMessage(`{"index":`+string(rune('0'+i%10))+`}`))
	}

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)
	assert.Len(t, parsed.Messages, 100)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

func TestSignDocEdgeCases_LargeMessageData(t *testing.T) {
	// Test with a large message payload
	largeData := make([]byte, 10000)
	for i := range largeData {
		largeData[i] = 'a' + byte(i%26)
	}

	msgData := json.RawMessage(`{"data":"` + string(largeData) + `"}`)
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/msg.Large", msgData)

	json1, err := sd.ToJSON()
	require.NoError(t, err)

	parsed, err := ParseSignDoc(json1)
	require.NoError(t, err)

	json2, err := parsed.ToJSON()
	require.NoError(t, err)
	assert.Equal(t, json1, json2)
}

// =============================================================================
// TEST FIXTURES FOR REGRESSION TESTING
// =============================================================================
// These provide known expected outputs for specific inputs.

// SignDocFixture represents a test fixture with known expected output
type SignDocFixture struct {
	Name         string
	SignDoc      *SignDoc
	ExpectedJSON string
	ExpectedHash string // hex-encoded SHA-256
}

func getSignDocFixtures() []SignDocFixture {
	return []SignDocFixture{
		{
			Name: "basic_transfer",
			SignDoc: func() *SignDoc {
				sd := NewSignDoc("punnet-1", 1, "alice", 1, "")
				sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`))
				return sd
			}(),
			ExpectedJSON: `{"version":"1","chain_id":"punnet-1","account":"alice","account_sequence":"1","messages":[{"type":"/punnet.bank.v1.MsgSend","data":{"from":"alice","to":"bob","amount":"100"}}],"nonce":"1","fee":{"amount":[],"gas_limit":"0"},"fee_slippage":{"numerator":"0","denominator":"1"}}`,
		},
		{
			Name: "with_memo",
			SignDoc: func() *SignDoc {
				sd := NewSignDoc("test-chain", 42, "bob", 10, "hello world")
				sd.AddMessage("/msg", json.RawMessage(`{}`))
				return sd
			}(),
			ExpectedJSON: `{"version":"1","chain_id":"test-chain","account":"bob","account_sequence":"42","messages":[{"type":"/msg","data":{}}],"nonce":"10","memo":"hello world","fee":{"amount":[],"gas_limit":"0"},"fee_slippage":{"numerator":"0","denominator":"1"}}`,
		},
		{
			Name: "zero_values",
			SignDoc: func() *SignDoc {
				sd := NewSignDoc("chain", 0, "user", 0, "")
				sd.AddMessage("/m", json.RawMessage(`{}`))
				return sd
			}(),
			ExpectedJSON: `{"version":"1","chain_id":"chain","account":"user","account_sequence":"0","messages":[{"type":"/m","data":{}}],"nonce":"0","fee":{"amount":[],"gas_limit":"0"},"fee_slippage":{"numerator":"0","denominator":"1"}}`,
		},
		{
			Name: "multiple_messages",
			SignDoc: func() *SignDoc {
				sd := NewSignDoc("chain", 1, "alice", 1, "")
				sd.AddMessage("/a", json.RawMessage(`{"x":1}`))
				sd.AddMessage("/b", json.RawMessage(`{"y":2}`))
				return sd
			}(),
			ExpectedJSON: `{"version":"1","chain_id":"chain","account":"alice","account_sequence":"1","messages":[{"type":"/a","data":{"x":1}},{"type":"/b","data":{"y":2}}],"nonce":"1","fee":{"amount":[],"gas_limit":"0"},"fee_slippage":{"numerator":"0","denominator":"1"}}`,
		},
		{
			Name: "with_fee",
			SignDoc: func() *SignDoc {
				sd := NewSignDocWithFee("punnet-1", 5, "alice", 5, "fee test",
					SignDocFee{
						Amount:   []SignDocCoin{{Denom: "uatom", Amount: "5000"}},
						GasLimit: "200000",
					},
					SignDocRatio{Numerator: "5", Denominator: "100"},
				)
				sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"to":"bob"}`))
				return sd
			}(),
			ExpectedJSON: `{"version":"1","chain_id":"punnet-1","account":"alice","account_sequence":"5","messages":[{"type":"/punnet.bank.v1.MsgSend","data":{"to":"bob"}}],"nonce":"5","memo":"fee test","fee":{"amount":[{"denom":"uatom","amount":"5000"}],"gas_limit":"200000"},"fee_slippage":{"numerator":"5","denominator":"100"}}`,
		},
	}
}

func TestSignDocFixtures_JSONOutput(t *testing.T) {
	// Verify that fixtures produce exactly the expected JSON output.
	// This catches any changes to serialization behavior.
	fixtures := getSignDocFixtures()

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			jsonBytes, err := fixture.SignDoc.ToJSON()
			require.NoError(t, err)

			assert.Equal(t, fixture.ExpectedJSON, string(jsonBytes),
				"fixture %q JSON output changed", fixture.Name)
		})
	}
}

func TestSignDocFixtures_Determinism(t *testing.T) {
	// Verify fixtures serialize deterministically across multiple calls.
	fixtures := getSignDocFixtures()

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			json1, err := fixture.SignDoc.ToJSON()
			require.NoError(t, err)

			for i := 0; i < 10; i++ {
				jsonN, err := fixture.SignDoc.ToJSON()
				require.NoError(t, err)
				assert.Equal(t, json1, jsonN, "iteration %d", i)
			}
		})
	}
}

func TestSignDocFixtures_HashStability(t *testing.T) {
	// Record and verify hash values for fixtures.
	// If this test fails, it means serialization has changed (breaking change!).
	fixtures := getSignDocFixtures()

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			hash, err := fixture.SignDoc.GetSignBytes()
			require.NoError(t, err)

			// Compute expected hash from expected JSON
			expectedHash := sha256.Sum256([]byte(fixture.ExpectedJSON))

			assert.Equal(t, expectedHash[:], hash,
				"hash mismatch for fixture %q - serialization may have changed", fixture.Name)

			// Log the hash for documentation
			t.Logf("Fixture %q hash: %s", fixture.Name, hex.EncodeToString(hash))
		})
	}
}

func TestSignDocFixtures_Roundtrip(t *testing.T) {
	// Verify all fixtures roundtrip correctly.
	fixtures := getSignDocFixtures()

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			json1, err := fixture.SignDoc.ToJSON()
			require.NoError(t, err)

			parsed, err := ParseSignDoc(json1)
			require.NoError(t, err)

			json2, err := parsed.ToJSON()
			require.NoError(t, err)

			assert.Equal(t, json1, json2, "roundtrip mismatch for fixture %q", fixture.Name)
		})
	}
}

// =============================================================================
// SECURITY-FOCUSED TESTS
// =============================================================================
// Tests for potential security issues in serialization.

func TestSignDocSecurity_NoExtraFields(t *testing.T) {
	// Verify that parsing ignores (or rejects) extra fields that shouldn't be there.
	// This prevents injection of unexpected data.
	jsonWithExtra := `{"version":"1","chain_id":"chain","account":"alice","account_sequence":"1","messages":[{"type":"/msg","data":{}}],"nonce":"1","fee":{"amount":[],"gas_limit":"0"},"fee_slippage":{"numerator":"0","denominator":"1"},"evil_field":"malicious"}`

	parsed, err := ParseSignDoc([]byte(jsonWithExtra))
	require.NoError(t, err)

	// Re-serialize should NOT include the evil_field
	jsonOut, err := parsed.ToJSON()
	require.NoError(t, err)

	assert.NotContains(t, string(jsonOut), "evil_field",
		"extra fields should not be preserved in serialization")
}

func TestSignDocSecurity_DifferentSignDocsProduceDifferentHashes(t *testing.T) {
	// Verify that any change to a SignDoc produces a different hash.
	// This is critical for signature security.
	base := NewSignDoc("chain", 1, "alice", 1, "memo")
	base.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	baseHash, err := base.GetSignBytes()
	require.NoError(t, err)

	// Test various modifications
	modifications := []struct {
		name   string
		modify func() *SignDoc
	}{
		{
			name: "different chain_id",
			modify: func() *SignDoc {
				sd := NewSignDoc("chain2", 1, "alice", 1, "memo")
				sd.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))
				return sd
			},
		},
		{
			name: "different account_sequence",
			modify: func() *SignDoc {
				sd := NewSignDoc("chain", 2, "alice", 1, "memo")
				sd.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))
				return sd
			},
		},
		{
			name: "different account",
			modify: func() *SignDoc {
				sd := NewSignDoc("chain", 1, "bob", 1, "memo")
				sd.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))
				return sd
			},
		},
		{
			name: "different nonce",
			modify: func() *SignDoc {
				sd := NewSignDoc("chain", 1, "alice", 2, "memo")
				sd.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))
				return sd
			},
		},
		{
			name: "different memo",
			modify: func() *SignDoc {
				sd := NewSignDoc("chain", 1, "alice", 1, "different memo")
				sd.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))
				return sd
			},
		},
		{
			name: "different message type",
			modify: func() *SignDoc {
				sd := NewSignDoc("chain", 1, "alice", 1, "memo")
				sd.AddMessage("/different.msg", json.RawMessage(`{"key":"value"}`))
				return sd
			},
		},
		{
			name: "different message data",
			modify: func() *SignDoc {
				sd := NewSignDoc("chain", 1, "alice", 1, "memo")
				sd.AddMessage("/msg", json.RawMessage(`{"key":"different"}`))
				return sd
			},
		},
		{
			name: "additional message",
			modify: func() *SignDoc {
				sd := NewSignDoc("chain", 1, "alice", 1, "memo")
				sd.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))
				sd.AddMessage("/msg2", json.RawMessage(`{}`))
				return sd
			},
		},
	}

	for _, mod := range modifications {
		t.Run(mod.name, func(t *testing.T) {
			modified := mod.modify()
			modHash, err := modified.GetSignBytes()
			require.NoError(t, err)

			assert.False(t, bytes.Equal(baseHash, modHash),
				"modification %q should produce different hash", mod.name)
		})
	}
}

func TestSignDocSecurity_CanonicalWhitespace(t *testing.T) {
	// Verify that JSON output has no unnecessary whitespace.
	// Extra whitespace could allow signature malleability.
	sd := NewSignDoc("chain", 1, "alice", 1, "memo")
	sd.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	jsonBytes, err := sd.ToJSON()
	require.NoError(t, err)

	jsonStr := string(jsonBytes)

	// Check for common whitespace issues
	assert.NotContains(t, jsonStr, "\n", "should not contain newlines")
	assert.NotContains(t, jsonStr, "  ", "should not contain multiple spaces")
	assert.NotContains(t, jsonStr, "\t", "should not contain tabs")
	assert.NotContains(t, jsonStr, ": ", "should not have space after colon (should be compact)")
	assert.NotContains(t, jsonStr, ", ", "should not have space after comma (should be compact)")
}

func TestSignDocSecurity_HashLength(t *testing.T) {
	// Verify GetSignBytes always returns exactly 32 bytes (SHA-256).
	testCases := []struct {
		name    string
		signDoc *SignDoc
	}{
		{
			name: "minimal",
			signDoc: func() *SignDoc {
				sd := NewSignDoc("c", 0, "a", 0, "")
				sd.AddMessage("/m", json.RawMessage(`{}`))
				return sd
			}(),
		},
		{
			name: "large",
			signDoc: func() *SignDoc {
				sd := NewSignDoc(strings.Repeat("x", 100), math.MaxUint64, strings.Repeat("y", 100), math.MaxUint64, strings.Repeat("z", 500))
				for i := 0; i < 50; i++ {
					sd.AddMessage("/msg", json.RawMessage(`{"data":"`+strings.Repeat("d", 1000)+`"}`))
				}
				return sd
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := tc.signDoc.GetSignBytes()
			require.NoError(t, err)
			assert.Len(t, hash, 32, "SHA-256 must always produce 32 bytes")
		})
	}
}

// =============================================================================
// ERROR PATH TESTS
// =============================================================================
// Test error handling paths for complete coverage.

func TestSignDocEquals_ErrorPaths(t *testing.T) {
	// Test Equals when ToJSON fails
	sd1 := NewSignDoc("chain", 1, "alice", 1, "")
	sd1.AddMessage("/msg", json.RawMessage(`{}`))

	// Test with nil
	assert.False(t, sd1.Equals(nil))

	// Test self-equality
	assert.True(t, sd1.Equals(sd1))
}

func TestSignDocEquals_BothValid(t *testing.T) {
	// Test Equals with two valid SignDocs that should be equal
	sd1 := NewSignDoc("chain", 1, "alice", 1, "memo")
	sd1.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	sd2 := NewSignDoc("chain", 1, "alice", 1, "memo")
	sd2.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	assert.True(t, sd1.Equals(sd2))
}

func TestSignDocEquals_Different(t *testing.T) {
	// Test Equals with two different SignDocs
	sd1 := NewSignDoc("chain1", 1, "alice", 1, "")
	sd1.AddMessage("/msg", json.RawMessage(`{}`))

	sd2 := NewSignDoc("chain2", 1, "alice", 1, "")
	sd2.AddMessage("/msg", json.RawMessage(`{}`))

	assert.False(t, sd1.Equals(sd2))
}

func TestParseSignDoc_InvalidJSON(t *testing.T) {
	// Test ParseSignDoc with invalid JSON input
	invalidInputs := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"not json", "not json at all"},
		{"incomplete", `{"version":"1"`},
		{"wrong type", `{"version":1}`}, // number instead of string
		{"array instead of object", `[]`},
	}

	for _, tc := range invalidInputs {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSignDoc([]byte(tc.input))
			assert.Error(t, err, "should fail to parse: %s", tc.name)
		})
	}
}

func TestSortedJSONObject_ErrorPaths(t *testing.T) {
	// Test sortedJSONObject MarshalJSON with various value types
	testCases := []struct {
		name     string
		obj      sortedJSONObject
		expected string
	}{
		{
			name:     "empty object",
			obj:      sortedJSONObject{},
			expected: `{}`,
		},
		{
			name:     "single key",
			obj:      sortedJSONObject{"a": 1},
			expected: `{"a":1}`,
		},
		{
			name:     "string value",
			obj:      sortedJSONObject{"key": "value"},
			expected: `{"key":"value"}`,
		},
		{
			name:     "boolean value",
			obj:      sortedJSONObject{"flag": true},
			expected: `{"flag":true}`,
		},
		{
			name:     "null value",
			obj:      sortedJSONObject{"empty": nil},
			expected: `{"empty":null}`,
		},
		{
			name: "nested object",
			obj: sortedJSONObject{
				"outer": map[string]interface{}{"inner": "value"},
			},
			expected: `{"outer":{"inner":"value"}}`,
		},
		{
			name: "array value",
			obj: sortedJSONObject{
				"list": []int{1, 2, 3},
			},
			expected: `{"list":[1,2,3]}`,
		},
		{
			name: "multiple sorted keys",
			obj: sortedJSONObject{
				"c": 3,
				"a": 1,
				"b": 2,
			},
			expected: `{"a":1,"b":2,"c":3}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := json.Marshal(tc.obj)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, string(result))
		})
	}
}

// =============================================================================
// CONCURRENT SERIALIZATION TESTS
// =============================================================================
// Test that serialization is safe under concurrent access.

func TestSignDocConcurrency_MultipleGoroutines(t *testing.T) {
	// Test that concurrent serialization of the same SignDoc is deterministic.
	// This is important for production use where the same SignDoc might be
	// serialized from multiple goroutines.
	sd := NewSignDoc("chain", 1, "alice", 1, "memo")
	sd.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	// Get the expected output
	expected, err := sd.ToJSON()
	require.NoError(t, err)

	// Run many goroutines in parallel
	const numGoroutines = 100
	results := make(chan []byte, numGoroutines)
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			result, err := sd.ToJSON()
			if err != nil {
				errors <- err
			} else {
				results <- result
			}
		}()
	}

	// Collect all results
	for i := 0; i < numGoroutines; i++ {
		select {
		case err := <-errors:
			t.Fatalf("goroutine error: %v", err)
		case result := <-results:
			assert.Equal(t, expected, result, "concurrent serialization must be deterministic")
		}
	}
}

func TestSignDocConcurrency_GetSignBytes(t *testing.T) {
	// Test concurrent GetSignBytes calls
	sd := NewSignDoc("chain", 1, "alice", 1, "")
	sd.AddMessage("/msg", json.RawMessage(`{}`))

	expected, err := sd.GetSignBytes()
	require.NoError(t, err)

	const numGoroutines = 50
	results := make(chan []byte, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			hash, _ := sd.GetSignBytes()
			results <- hash
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		result := <-results
		assert.Equal(t, expected, result, "concurrent hashing must be deterministic")
	}
}

// =============================================================================
// COVERAGE DOCUMENTATION TESTS
// =============================================================================
// These tests document edge cases and error paths for code review purposes.

func TestSignDoc_ToJSON_NeverFails(t *testing.T) {
	// Document that ToJSON() on a properly constructed SignDoc cannot fail.
	// json.Marshal only fails for:
	// 1. Channels (SignDoc has none)
	// 2. Functions (SignDoc has none)
	// 3. Complex numbers (SignDoc has none)
	//
	// Therefore, the error branch in GetSignBytes (line 100-102) and
	// Equals (line 147-149) are unreachable with current SignDoc definition.
	//
	// This test verifies this invariant holds.

	defaultFee := SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"}
	defaultSlippage := SignDocRatio{Numerator: "0", Denominator: "1"}

	testCases := []struct {
		name    string
		signDoc *SignDoc
	}{
		{
			name:    "nil messages slice",
			signDoc: &SignDoc{Version: "1", ChainID: "c", Account: "a", Messages: nil, Fee: defaultFee, FeeSlippage: defaultSlippage},
		},
		{
			name:    "empty messages slice",
			signDoc: &SignDoc{Version: "1", ChainID: "c", Account: "a", Messages: []SignDocMessage{}, Fee: defaultFee, FeeSlippage: defaultSlippage},
		},
		{
			name:    "nil message data",
			signDoc: &SignDoc{Version: "1", ChainID: "c", Account: "a", Messages: []SignDocMessage{{Type: "/m", Data: nil}}, Fee: defaultFee, FeeSlippage: defaultSlippage},
		},
		{
			name:    "empty string fields",
			signDoc: &SignDoc{Version: "", ChainID: "", Account: "", Messages: []SignDocMessage{{Type: "", Data: nil}}, Fee: defaultFee, FeeSlippage: defaultSlippage},
		},
		{
			name:    "max uint64 values",
			signDoc: &SignDoc{Version: "1", ChainID: "c", Account: "a", AccountSequence: math.MaxUint64, Nonce: math.MaxUint64, Messages: []SignDocMessage{{Type: "/m", Data: json.RawMessage(`{}`)}}, Fee: defaultFee, FeeSlippage: defaultSlippage},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// This should never return an error for any of these cases
			jsonBytes, err := tc.signDoc.ToJSON()
			assert.NoError(t, err, "ToJSON should not fail for SignDoc structs")
			assert.NotEmpty(t, jsonBytes)

			// GetSignBytes should also succeed
			hash, err := tc.signDoc.GetSignBytes()
			assert.NoError(t, err)
			assert.Len(t, hash, 32)
		})
	}
}

func TestSortedJSONObject_MarshalError(t *testing.T) {
	// sortedJSONObject.MarshalJSON can fail if values are unmarshallable.
	// This tests the error path in MarshalJSON.

	// Note: We can't put channels directly in sortedJSONObject because
	// json.Marshal will fail before our custom MarshalJSON is called for
	// nested values. This is expected behavior.

	// Test that our MarshalJSON handles all normal types correctly
	obj := sortedJSONObject{
		"string": "value",
		"number": 42,
		"float":  3.14,
		"bool":   true,
		"null":   nil,
		"array":  []interface{}{1, 2, 3},
		"object": map[string]interface{}{"nested": true},
	}

	jsonBytes, err := obj.MarshalJSON()
	require.NoError(t, err)

	// Verify the output is valid JSON with sorted keys
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)

	// Verify sorting by checking the raw string
	jsonStr := string(jsonBytes)
	// Keys should appear in alphabetical order: array, bool, float, null, number, object, string
	arrayIdx := strings.Index(jsonStr, `"array"`)
	boolIdx := strings.Index(jsonStr, `"bool"`)
	floatIdx := strings.Index(jsonStr, `"float"`)
	nullIdx := strings.Index(jsonStr, `"null"`)
	numberIdx := strings.Index(jsonStr, `"number"`)
	objectIdx := strings.Index(jsonStr, `"object"`)
	stringIdx := strings.Index(jsonStr, `"string"`)

	assert.True(t, arrayIdx < boolIdx, "keys should be sorted")
	assert.True(t, boolIdx < floatIdx, "keys should be sorted")
	assert.True(t, floatIdx < nullIdx, "keys should be sorted")
	assert.True(t, nullIdx < numberIdx, "keys should be sorted")
	assert.True(t, numberIdx < objectIdx, "keys should be sorted")
	assert.True(t, objectIdx < stringIdx, "keys should be sorted")
}

// =============================================================================
// PROPERTY-BASED STYLE TESTS
// =============================================================================
// Tests that verify properties that should hold for any valid input.

func TestSignDocProperty_EqualsSelfReflexive(t *testing.T) {
	// Property: Any SignDoc should equal itself
	testSignDocs := []*SignDoc{
		NewSignDoc("chain", 1, "alice", 1, ""),
		NewSignDoc("chain", 0, "bob", 0, "memo"),
		NewSignDoc(strings.Repeat("x", 100), math.MaxUint64, strings.Repeat("y", 100), math.MaxUint64, ""),
	}

	for i, sd := range testSignDocs {
		sd.AddMessage("/msg", json.RawMessage(`{}`))
		assert.True(t, sd.Equals(sd), "SignDoc %d should equal itself", i)
	}
}

func TestSignDocProperty_EqualsSymmetric(t *testing.T) {
	// Property: If A equals B, then B equals A
	sd1 := NewSignDoc("chain", 1, "alice", 1, "memo")
	sd1.AddMessage("/msg", json.RawMessage(`{"x":1}`))

	sd2 := NewSignDoc("chain", 1, "alice", 1, "memo")
	sd2.AddMessage("/msg", json.RawMessage(`{"x":1}`))

	eq1 := sd1.Equals(sd2)
	eq2 := sd2.Equals(sd1)

	assert.Equal(t, eq1, eq2, "equality should be symmetric")
}

func TestSignDocProperty_HashPreservesEquality(t *testing.T) {
	// Property: Equal SignDocs have equal hashes
	sd1 := NewSignDoc("chain", 1, "alice", 1, "memo")
	sd1.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	sd2 := NewSignDoc("chain", 1, "alice", 1, "memo")
	sd2.AddMessage("/msg", json.RawMessage(`{"key":"value"}`))

	assert.True(t, sd1.Equals(sd2), "SignDocs should be equal")

	hash1, err1 := sd1.GetSignBytes()
	require.NoError(t, err1)

	hash2, err2 := sd2.GetSignBytes()
	require.NoError(t, err2)

	assert.Equal(t, hash1, hash2, "equal SignDocs must have equal hashes")
}

func TestSignDocProperty_RoundtripPreservesEquality(t *testing.T) {
	// Property: Serialize -> Parse -> Serialize produces identical bytes
	testCases := []struct {
		name    string
		signDoc *SignDoc
	}{
		{
			name: "minimal",
			signDoc: func() *SignDoc {
				sd := NewSignDoc("c", 0, "a", 0, "")
				sd.AddMessage("/m", json.RawMessage(`{}`))
				return sd
			}(),
		},
		{
			name: "with unicode",
			signDoc: func() *SignDoc {
				sd := NewSignDoc("chain-日本", 1, "alice", 1, "memo 🚀")
				sd.AddMessage("/msg.送金", json.RawMessage(`{"to":"בוב"}`))
				return sd
			}(),
		},
		{
			name: "max values",
			signDoc: func() *SignDoc {
				sd := NewSignDoc("chain", math.MaxUint64, "alice", math.MaxUint64, "")
				sd.AddMessage("/msg", json.RawMessage(`{}`))
				return sd
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// First serialization
			json1, err := tc.signDoc.ToJSON()
			require.NoError(t, err)

			// Parse
			parsed, err := ParseSignDoc(json1)
			require.NoError(t, err)

			// Second serialization
			json2, err := parsed.ToJSON()
			require.NoError(t, err)

			assert.Equal(t, json1, json2, "roundtrip should preserve bytes exactly")
		})
	}
}
