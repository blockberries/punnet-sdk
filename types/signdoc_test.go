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
	assert.Equal(t, StringUint64(42), sd.AccountSequence)
	assert.Equal(t, "alice", sd.Account)
	assert.Equal(t, StringUint64(1), sd.Nonce)
	assert.Equal(t, "test memo", sd.Memo)
	assert.Empty(t, sd.Messages)
	// Verify default fee and slippage
	assert.Equal(t, "0", sd.Fee.GasLimit)
	assert.Empty(t, sd.Fee.Amount)
	assert.Equal(t, "0", sd.FeeSlippage.Numerator)
	assert.Equal(t, "1", sd.FeeSlippage.Denominator)
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
				Version:     "99",
				ChainID:     "test",
				Account:     "alice",
				Messages:    []SignDocMessage{{Type: "/msg", Data: json.RawMessage(`{}`)}},
				Fee:         SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
				FeeSlippage: SignDocRatio{Numerator: "0", Denominator: "1"},
			},
			expectErr: true,
			errMsg:    "unsupported SignDoc version",
		},
		{
			name: "empty chain ID",
			signDoc: &SignDoc{
				Version:     SignDocVersion,
				ChainID:     "",
				Account:     "alice",
				Messages:    []SignDocMessage{{Type: "/msg", Data: json.RawMessage(`{}`)}},
				Fee:         SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
				FeeSlippage: SignDocRatio{Numerator: "0", Denominator: "1"},
			},
			expectErr: true,
			errMsg:    "chain_id cannot be empty",
		},
		{
			name: "empty account",
			signDoc: &SignDoc{
				Version:     SignDocVersion,
				ChainID:     "test",
				Account:     "",
				Messages:    []SignDocMessage{{Type: "/msg", Data: json.RawMessage(`{}`)}},
				Fee:         SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
				FeeSlippage: SignDocRatio{Numerator: "0", Denominator: "1"},
			},
			expectErr: true,
			errMsg:    "account cannot be empty",
		},
		{
			name: "no messages",
			signDoc: &SignDoc{
				Version:     SignDocVersion,
				ChainID:     "test",
				Account:     "alice",
				Messages:    []SignDocMessage{},
				Fee:         SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
				FeeSlippage: SignDocRatio{Numerator: "0", Denominator: "1"},
			},
			expectErr: true,
			errMsg:    "must contain at least one message",
		},
		{
			name: "message with empty type",
			signDoc: &SignDoc{
				Version:     SignDocVersion,
				ChainID:     "test",
				Account:     "alice",
				Messages:    []SignDocMessage{{Type: "", Data: json.RawMessage(`{}`)}},
				Fee:         SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
				FeeSlippage: SignDocRatio{Numerator: "0", Denominator: "1"},
			},
			expectErr: true,
			errMsg:    "empty type",
		},
		{
			name: "too many messages (DoS protection)",
			signDoc: func() *SignDoc {
				sd := &SignDoc{
					Version:     SignDocVersion,
					ChainID:     "test",
					Account:     "alice",
					Fee:         SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
					FeeSlippage: SignDocRatio{Numerator: "0", Denominator: "1"},
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
					Fee:         SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
					FeeSlippage: SignDocRatio{Numerator: "0", Denominator: "1"},
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
		"zebra":  1,
		"apple":  2,
		"mango":  3,
		"banana": 4,
	}

	jsonBytes, err := json.Marshal(obj)
	require.NoError(t, err)

	expected := `{"apple":2,"banana":4,"mango":3,"zebra":1}`
	assert.Equal(t, expected, string(jsonBytes))
}

// =============================================================================
// StringUint64 TESTS
// =============================================================================

func TestStringUint64_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		value    StringUint64
		expected string
	}{
		{"zero", StringUint64(0), `"0"`},
		{"one", StringUint64(1), `"1"`},
		{"large", StringUint64(1234567890), `"1234567890"`},
		{"max uint64", StringUint64(18446744073709551615), `"18446744073709551615"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := json.Marshal(tt.value)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(jsonBytes))
		})
	}
}

func TestStringUint64_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  StringUint64
		expectErr bool
	}{
		{"zero", `"0"`, StringUint64(0), false},
		{"one", `"1"`, StringUint64(1), false},
		{"large", `"1234567890"`, StringUint64(1234567890), false},
		{"max uint64", `"18446744073709551615"`, StringUint64(18446744073709551615), false},
		{"not a string", `123`, StringUint64(0), true},
		{"negative", `"-1"`, StringUint64(0), true},
		{"overflow", `"18446744073709551616"`, StringUint64(0), true},
		{"invalid number", `"abc"`, StringUint64(0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result StringUint64
			err := json.Unmarshal([]byte(tt.input), &result)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestStringUint64_Uint64(t *testing.T) {
	s := StringUint64(12345)
	assert.Equal(t, uint64(12345), s.Uint64())
}

func TestStringUint64_Roundtrip(t *testing.T) {
	// INVARIANT: Marshal -> Unmarshal -> Marshal produces identical JSON
	original := StringUint64(9876543210)

	json1, err := json.Marshal(original)
	require.NoError(t, err)

	var parsed StringUint64
	err = json.Unmarshal(json1, &parsed)
	require.NoError(t, err)

	json2, err := json.Marshal(parsed)
	require.NoError(t, err)

	assert.Equal(t, json1, json2)
	assert.Equal(t, original, parsed)
}

// =============================================================================
// SignDocCoin TESTS
// =============================================================================

func TestSignDocCoin_ValidateBasic(t *testing.T) {
	tests := []struct {
		name      string
		coin      SignDocCoin
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid coin",
			coin:      SignDocCoin{Denom: "uatom", Amount: "1000"},
			expectErr: false,
		},
		{
			name:      "zero amount is valid",
			coin:      SignDocCoin{Denom: "uatom", Amount: "0"},
			expectErr: false,
		},
		{
			name:      "empty denom",
			coin:      SignDocCoin{Denom: "", Amount: "100"},
			expectErr: true,
			errMsg:    "denom cannot be empty",
		},
		{
			name:      "denom too long",
			coin:      SignDocCoin{Denom: string(make([]byte, 65)), Amount: "100"},
			expectErr: true,
			errMsg:    "denom too long",
		},
		{
			name:      "empty amount",
			coin:      SignDocCoin{Denom: "uatom", Amount: ""},
			expectErr: true,
			errMsg:    "amount cannot be empty",
		},
		{
			name:      "invalid amount - not a number",
			coin:      SignDocCoin{Denom: "uatom", Amount: "abc"},
			expectErr: true,
			errMsg:    "must be a decimal string",
		},
		{
			name:      "invalid amount - negative",
			coin:      SignDocCoin{Denom: "uatom", Amount: "-100"},
			expectErr: true,
			errMsg:    "must be a decimal string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.coin.ValidateBasic()
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

func TestSignDocCoin_JSONSerialization(t *testing.T) {
	coin := SignDocCoin{Denom: "uatom", Amount: "1000000"}

	jsonBytes, err := json.Marshal(coin)
	require.NoError(t, err)

	expected := `{"denom":"uatom","amount":"1000000"}`
	assert.Equal(t, expected, string(jsonBytes))

	// Roundtrip
	var parsed SignDocCoin
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)
	assert.Equal(t, coin, parsed)
}

// =============================================================================
// SignDocFee TESTS
// =============================================================================

func TestSignDocFee_ValidateBasic(t *testing.T) {
	tests := []struct {
		name      string
		fee       SignDocFee
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid fee with coins",
			fee:       SignDocFee{Amount: []SignDocCoin{{Denom: "uatom", Amount: "1000"}}, GasLimit: "200000"},
			expectErr: false,
		},
		{
			name:      "valid fee with empty coins",
			fee:       SignDocFee{Amount: []SignDocCoin{}, GasLimit: "200000"},
			expectErr: false,
		},
		{
			name:      "valid fee with zero gas",
			fee:       SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
			expectErr: false,
		},
		{
			name:      "empty gas limit",
			fee:       SignDocFee{Amount: []SignDocCoin{}, GasLimit: ""},
			expectErr: true,
			errMsg:    "gas_limit cannot be empty",
		},
		{
			name:      "invalid gas limit - not a number",
			fee:       SignDocFee{Amount: []SignDocCoin{}, GasLimit: "abc"},
			expectErr: true,
			errMsg:    "must be a decimal string",
		},
		{
			name:      "invalid coin in amount",
			fee:       SignDocFee{Amount: []SignDocCoin{{Denom: "", Amount: "100"}}, GasLimit: "200000"},
			expectErr: true,
			errMsg:    "denom cannot be empty",
		},
		{
			name: "too many fee coins (DoS protection)",
			fee: SignDocFee{
				Amount: func() []SignDocCoin {
					coins := make([]SignDocCoin, MaxFeeCoins+1)
					for i := range coins {
						coins[i] = SignDocCoin{Denom: "coin", Amount: "1"}
					}
					return coins
				}(),
				GasLimit: "200000",
			},
			expectErr: true,
			errMsg:    "too many fee coins",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fee.ValidateBasic()
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

func TestSignDocFee_JSONSerialization(t *testing.T) {
	fee := SignDocFee{
		Amount:   []SignDocCoin{{Denom: "uatom", Amount: "5000"}},
		GasLimit: "200000",
	}

	jsonBytes, err := json.Marshal(fee)
	require.NoError(t, err)

	expected := `{"amount":[{"denom":"uatom","amount":"5000"}],"gas_limit":"200000"}`
	assert.Equal(t, expected, string(jsonBytes))

	// Roundtrip
	var parsed SignDocFee
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)
	assert.Equal(t, fee, parsed)
}

// =============================================================================
// SignDocRatio TESTS
// =============================================================================

func TestSignDocRatio_ValidateBasic(t *testing.T) {
	tests := []struct {
		name      string
		ratio     SignDocRatio
		expectErr bool
		errMsg    string
	}{
		{
			name:      "valid ratio 1/100",
			ratio:     SignDocRatio{Numerator: "1", Denominator: "100"},
			expectErr: false,
		},
		{
			name:      "valid ratio 0/1 (zero slippage)",
			ratio:     SignDocRatio{Numerator: "0", Denominator: "1"},
			expectErr: false,
		},
		{
			name:      "valid ratio 5/100 (5%)",
			ratio:     SignDocRatio{Numerator: "5", Denominator: "100"},
			expectErr: false,
		},
		{
			name:      "empty numerator",
			ratio:     SignDocRatio{Numerator: "", Denominator: "100"},
			expectErr: true,
			errMsg:    "numerator cannot be empty",
		},
		{
			name:      "empty denominator",
			ratio:     SignDocRatio{Numerator: "1", Denominator: ""},
			expectErr: true,
			errMsg:    "denominator cannot be empty",
		},
		{
			name:      "zero denominator (division by zero)",
			ratio:     SignDocRatio{Numerator: "1", Denominator: "0"},
			expectErr: true,
			errMsg:    "denominator cannot be zero",
		},
		{
			name:      "invalid numerator - not a number",
			ratio:     SignDocRatio{Numerator: "abc", Denominator: "100"},
			expectErr: true,
			errMsg:    "must be a decimal string",
		},
		{
			name:      "invalid denominator - negative",
			ratio:     SignDocRatio{Numerator: "1", Denominator: "-100"},
			expectErr: true,
			errMsg:    "must be a decimal string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ratio.ValidateBasic()
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

func TestSignDocRatio_JSONSerialization(t *testing.T) {
	ratio := SignDocRatio{Numerator: "5", Denominator: "100"}

	jsonBytes, err := json.Marshal(ratio)
	require.NoError(t, err)

	expected := `{"numerator":"5","denominator":"100"}`
	assert.Equal(t, expected, string(jsonBytes))

	// Roundtrip
	var parsed SignDocRatio
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)
	assert.Equal(t, ratio, parsed)
}

// =============================================================================
// SignDoc with Fee TESTS
// =============================================================================

func TestSignDoc_NewSignDocWithFee(t *testing.T) {
	fee := SignDocFee{
		Amount:   []SignDocCoin{{Denom: "uatom", Amount: "5000"}},
		GasLimit: "200000",
	}
	slippage := SignDocRatio{Numerator: "1", Denominator: "100"}

	sd := NewSignDocWithFee("test-chain", 42, "alice", 1, "memo", fee, slippage)

	assert.Equal(t, SignDocVersion, sd.Version)
	assert.Equal(t, "test-chain", sd.ChainID)
	assert.Equal(t, StringUint64(42), sd.AccountSequence)
	assert.Equal(t, "alice", sd.Account)
	assert.Equal(t, StringUint64(1), sd.Nonce)
	assert.Equal(t, "memo", sd.Memo)
	assert.Equal(t, fee, sd.Fee)
	assert.Equal(t, slippage, sd.FeeSlippage)
}

func TestSignDoc_SetFee(t *testing.T) {
	sd := NewSignDoc("test-chain", 1, "alice", 1, "")

	newFee := SignDocFee{
		Amount:   []SignDocCoin{{Denom: "stake", Amount: "10000"}},
		GasLimit: "500000",
	}
	sd.SetFee(newFee)

	assert.Equal(t, newFee, sd.Fee)
}

func TestSignDoc_SetFeeSlippage(t *testing.T) {
	sd := NewSignDoc("test-chain", 1, "alice", 1, "")

	newSlippage := SignDocRatio{Numerator: "10", Denominator: "100"}
	sd.SetFeeSlippage(newSlippage)

	assert.Equal(t, newSlippage, sd.FeeSlippage)
}

func TestSignDoc_ValidateBasic_WithFee(t *testing.T) {
	tests := []struct {
		name      string
		signDoc   *SignDoc
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid SignDoc with fee",
			signDoc: func() *SignDoc {
				sd := NewSignDocWithFee("test-chain", 1, "alice", 1, "",
					SignDocFee{Amount: []SignDocCoin{{Denom: "uatom", Amount: "1000"}}, GasLimit: "200000"},
					SignDocRatio{Numerator: "1", Denominator: "100"},
				)
				sd.AddMessage("/msg.Type", json.RawMessage(`{}`))
				return sd
			}(),
			expectErr: false,
		},
		{
			name: "invalid fee - bad gas limit",
			signDoc: func() *SignDoc {
				sd := NewSignDoc("test-chain", 1, "alice", 1, "")
				sd.Fee.GasLimit = "invalid"
				sd.AddMessage("/msg.Type", json.RawMessage(`{}`))
				return sd
			}(),
			expectErr: true,
			errMsg:    "invalid fee",
		},
		{
			name: "invalid fee slippage - zero denominator",
			signDoc: func() *SignDoc {
				sd := NewSignDoc("test-chain", 1, "alice", 1, "")
				sd.FeeSlippage = SignDocRatio{Numerator: "1", Denominator: "0"}
				sd.AddMessage("/msg.Type", json.RawMessage(`{}`))
				return sd
			}(),
			expectErr: true,
			errMsg:    "invalid fee_slippage",
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
