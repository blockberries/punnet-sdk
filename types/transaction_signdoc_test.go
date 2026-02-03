package types

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ToSignDoc WITH FEE AND FEE SLIPPAGE TESTS
// =============================================================================

func TestTransaction_ToSignDoc_WithFee(t *testing.T) {
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    42,
		Memo:     "test memo",
		Fee: Fee{
			Amount:   Coins{{Denom: "uatom", Amount: 1000}},
			GasLimit: 200000,
		},
		FeeSlippage: Ratio{
			Numerator:   1,
			Denominator: 100,
		},
	}

	signDoc := tx.ToSignDoc("test-chain", 42)

	// Basic fields
	assert.Equal(t, SignDocVersion, signDoc.Version)
	assert.Equal(t, "test-chain", signDoc.ChainID)
	assert.Equal(t, StringUint64(42), signDoc.AccountSequence)
	assert.Equal(t, "alice", signDoc.Account)
	assert.Equal(t, StringUint64(42), signDoc.Nonce)
	assert.Equal(t, "test memo", signDoc.Memo)

	// Fee fields
	assert.Equal(t, "200000", signDoc.Fee.GasLimit)
	require.Len(t, signDoc.Fee.Amount, 1)
	assert.Equal(t, "uatom", signDoc.Fee.Amount[0].Denom)
	assert.Equal(t, "1000", signDoc.Fee.Amount[0].Amount)

	// FeeSlippage fields
	assert.Equal(t, "1", signDoc.FeeSlippage.Numerator)
	assert.Equal(t, "100", signDoc.FeeSlippage.Denominator)
}

func TestTransaction_ToSignDoc_WithMultipleFeeCoins(t *testing.T) {
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
		Fee: Fee{
			Amount: Coins{
				{Denom: "uatom", Amount: 1000},
				{Denom: "uosmo", Amount: 2000},
			},
			GasLimit: 300000,
		},
		FeeSlippage: Ratio{
			Numerator:   5,
			Denominator: 100,
		},
	}

	signDoc := tx.ToSignDoc("test-chain", 1)

	// Verify coin ordering is preserved
	require.Len(t, signDoc.Fee.Amount, 2)
	assert.Equal(t, "uatom", signDoc.Fee.Amount[0].Denom)
	assert.Equal(t, "1000", signDoc.Fee.Amount[0].Amount)
	assert.Equal(t, "uosmo", signDoc.Fee.Amount[1].Denom)
	assert.Equal(t, "2000", signDoc.Fee.Amount[1].Amount)

	assert.Equal(t, "300000", signDoc.Fee.GasLimit)
	assert.Equal(t, "5", signDoc.FeeSlippage.Numerator)
	assert.Equal(t, "100", signDoc.FeeSlippage.Denominator)
}

func TestTransaction_ToSignDoc_ZeroFee(t *testing.T) {
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
		Fee: Fee{
			Amount:   Coins{},
			GasLimit: 0,
		},
		FeeSlippage: Ratio{
			Numerator:   0,
			Denominator: 1,
		},
	}

	signDoc := tx.ToSignDoc("test-chain", 1)

	assert.Equal(t, "0", signDoc.Fee.GasLimit)
	assert.Empty(t, signDoc.Fee.Amount)
	assert.Equal(t, "0", signDoc.FeeSlippage.Numerator)
	assert.Equal(t, "1", signDoc.FeeSlippage.Denominator)
}

func TestTransaction_ToSignDoc_MaxUint64Values(t *testing.T) {
	// EDGE CASE: Verify handling of maximum uint64 values
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    math.MaxUint64,
		Fee: Fee{
			Amount:   Coins{{Denom: "uatom", Amount: math.MaxUint64}},
			GasLimit: math.MaxUint64,
		},
		FeeSlippage: Ratio{
			Numerator:   math.MaxUint64,
			Denominator: math.MaxUint64,
		},
	}

	signDoc := tx.ToSignDoc("test-chain", math.MaxUint64)

	// Verify max values are correctly serialized as decimal strings
	assert.Equal(t, StringUint64(math.MaxUint64), signDoc.AccountSequence)
	assert.Equal(t, StringUint64(math.MaxUint64), signDoc.Nonce)
	assert.Equal(t, "18446744073709551615", signDoc.Fee.GasLimit)
	assert.Equal(t, "18446744073709551615", signDoc.Fee.Amount[0].Amount)
	assert.Equal(t, "18446744073709551615", signDoc.FeeSlippage.Numerator)
	assert.Equal(t, "18446744073709551615", signDoc.FeeSlippage.Denominator)
}

// =============================================================================
// HELPER CONVERSION FUNCTION TESTS
// =============================================================================

func TestConvertMessages_NilInput(t *testing.T) {
	result := convertMessages(nil)

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertMessages_EmptySlice(t *testing.T) {
	result := convertMessages([]Message{})

	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestConvertMessages_SingleMessage(t *testing.T) {
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	result := convertMessages([]Message{msg})

	require.Len(t, result, 1)
	assert.Equal(t, "/punnet.bank.v1.MsgSend", result[0].Type)

	// Verify data contains signers
	var data map[string][]string
	err := json.Unmarshal(result[0].Data, &data)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice"}, data["signers"])
}

func TestConvertMessages_MultipleMessages(t *testing.T) {
	msg1 := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}
	msg2 := &testMessage{
		MsgType: "/punnet.staking.v1.MsgDelegate",
		Signers: []AccountName{"bob"},
	}

	result := convertMessages([]Message{msg1, msg2})

	// INVARIANT: Message ordering is preserved
	require.Len(t, result, 2)
	assert.Equal(t, "/punnet.bank.v1.MsgSend", result[0].Type)
	assert.Equal(t, "/punnet.staking.v1.MsgDelegate", result[1].Type)
}

func TestConvertMessages_MultipleSigners(t *testing.T) {
	msg := &testMessage{
		MsgType: "/punnet.multisig.v1.MsgMultiSend",
		Signers: []AccountName{"alice", "bob", "charlie"},
	}

	result := convertMessages([]Message{msg})

	require.Len(t, result, 1)

	var data map[string][]string
	err := json.Unmarshal(result[0].Data, &data)
	require.NoError(t, err)
	assert.Equal(t, []string{"alice", "bob", "charlie"}, data["signers"])
}

func TestConvertFee_EmptyCoins(t *testing.T) {
	fee := Fee{
		Amount:   Coins{},
		GasLimit: 100000,
	}

	result := convertFee(fee)

	assert.Equal(t, "100000", result.GasLimit)
	assert.Empty(t, result.Amount)
}

func TestConvertFee_SingleCoin(t *testing.T) {
	fee := Fee{
		Amount:   Coins{{Denom: "uatom", Amount: 5000}},
		GasLimit: 200000,
	}

	result := convertFee(fee)

	assert.Equal(t, "200000", result.GasLimit)
	require.Len(t, result.Amount, 1)
	assert.Equal(t, "uatom", result.Amount[0].Denom)
	assert.Equal(t, "5000", result.Amount[0].Amount)
}

func TestConvertFee_MultipleCoins_OrderPreserved(t *testing.T) {
	// INVARIANT: Coin ordering in Amount is preserved
	fee := Fee{
		Amount: Coins{
			{Denom: "zebra", Amount: 100},
			{Denom: "alpha", Amount: 200},
			{Denom: "beta", Amount: 300},
		},
		GasLimit: 150000,
	}

	result := convertFee(fee)

	require.Len(t, result.Amount, 3)
	// Order must be preserved, not sorted
	assert.Equal(t, "zebra", result.Amount[0].Denom)
	assert.Equal(t, "100", result.Amount[0].Amount)
	assert.Equal(t, "alpha", result.Amount[1].Denom)
	assert.Equal(t, "200", result.Amount[1].Amount)
	assert.Equal(t, "beta", result.Amount[2].Denom)
	assert.Equal(t, "300", result.Amount[2].Amount)
}

func TestConvertFee_ZeroGasLimit(t *testing.T) {
	fee := Fee{
		Amount:   Coins{{Denom: "uatom", Amount: 1000}},
		GasLimit: 0,
	}

	result := convertFee(fee)

	assert.Equal(t, "0", result.GasLimit)
}

func TestConvertRatio_StandardValues(t *testing.T) {
	testCases := []struct {
		name        string
		ratio       Ratio
		expectedNum string
		expectedDen string
	}{
		{
			name:        "1%",
			ratio:       Ratio{Numerator: 1, Denominator: 100},
			expectedNum: "1",
			expectedDen: "100",
		},
		{
			name:        "5%",
			ratio:       Ratio{Numerator: 5, Denominator: 100},
			expectedNum: "5",
			expectedDen: "100",
		},
		{
			name:        "zero slippage",
			ratio:       Ratio{Numerator: 0, Denominator: 1},
			expectedNum: "0",
			expectedDen: "1",
		},
		{
			name:        "half (50%)",
			ratio:       Ratio{Numerator: 1, Denominator: 2},
			expectedNum: "1",
			expectedDen: "2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := convertRatio(tc.ratio)
			assert.Equal(t, tc.expectedNum, result.Numerator)
			assert.Equal(t, tc.expectedDen, result.Denominator)
		})
	}
}

func TestConvertRatio_MaxUint64(t *testing.T) {
	ratio := Ratio{
		Numerator:   math.MaxUint64,
		Denominator: math.MaxUint64,
	}

	result := convertRatio(ratio)

	assert.Equal(t, "18446744073709551615", result.Numerator)
	assert.Equal(t, "18446744073709551615", result.Denominator)
}

// =============================================================================
// DETERMINISM TESTS
// =============================================================================

func TestTransaction_ToSignDoc_Deterministic(t *testing.T) {
	// INVARIANT: Two calls to ToSignDoc with same parameters return equal SignDocs
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    42,
		Memo:     "test",
		Fee: Fee{
			Amount:   Coins{{Denom: "uatom", Amount: 1000}},
			GasLimit: 200000,
		},
		FeeSlippage: Ratio{
			Numerator:   1,
			Denominator: 100,
		},
	}

	// Call ToSignDoc multiple times
	signDoc1 := tx.ToSignDoc("test-chain", 42)
	signDoc2 := tx.ToSignDoc("test-chain", 42)

	// Serialize both to JSON
	json1, err := signDoc1.ToJSON()
	require.NoError(t, err)
	json2, err := signDoc2.ToJSON()
	require.NoError(t, err)

	// Must produce identical bytes
	assert.Equal(t, json1, json2)
}

func TestTransaction_ToSignDoc_RoundtripWithFee(t *testing.T) {
	// Verify roundtrip with fee fields works correctly
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
		Fee: Fee{
			Amount:   Coins{{Denom: "uatom", Amount: 1000}},
			GasLimit: 200000,
		},
		FeeSlippage: Ratio{
			Numerator:   1,
			Denominator: 100,
		},
	}

	// Should pass roundtrip validation
	err := tx.ValidateSignDocRoundtrip("test-chain", 1)
	assert.NoError(t, err)
}

// =============================================================================
// FEE AND RATIO TYPE TESTS
// =============================================================================

func TestFee_JSONSerialization(t *testing.T) {
	fee := Fee{
		Amount:   Coins{{Denom: "uatom", Amount: 1000}},
		GasLimit: 200000,
	}

	data, err := json.Marshal(fee)
	require.NoError(t, err)

	var parsed Fee
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, fee.GasLimit, parsed.GasLimit)
	require.Len(t, parsed.Amount, 1)
	assert.Equal(t, fee.Amount[0].Denom, parsed.Amount[0].Denom)
	assert.Equal(t, fee.Amount[0].Amount, parsed.Amount[0].Amount)
}

func TestRatio_JSONSerialization(t *testing.T) {
	ratio := Ratio{
		Numerator:   5,
		Denominator: 100,
	}

	data, err := json.Marshal(ratio)
	require.NoError(t, err)

	var parsed Ratio
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Equal(t, ratio.Numerator, parsed.Numerator)
	assert.Equal(t, ratio.Denominator, parsed.Denominator)
}

// =============================================================================
// AUTHORIZATION EXCLUSION TEST
// =============================================================================

func TestTransaction_ToSignDoc_ExcludesAuthorization(t *testing.T) {
	// POSTCONDITION: Authorization field is NOT included (it contains the signatures being produced)
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
		Authorization: &Authorization{
			Signatures: []Signature{{
				Algorithm: AlgorithmEd25519,
				PubKey:    []byte("test-key"),
				Signature: []byte("test-sig"),
			}},
		},
		Fee: Fee{
			Amount:   Coins{{Denom: "uatom", Amount: 1000}},
			GasLimit: 200000,
		},
		FeeSlippage: Ratio{
			Numerator:   1,
			Denominator: 100,
		},
	}

	signDoc := tx.ToSignDoc("test-chain", 1)

	// SignDoc should not have any Authorization-related fields
	jsonBytes, err := signDoc.ToJSON()
	require.NoError(t, err)

	// Parse as raw map to check for authorization field
	var rawMap map[string]interface{}
	err = json.Unmarshal(jsonBytes, &rawMap)
	require.NoError(t, err)

	// Authorization should NOT be present
	_, hasAuth := rawMap["authorization"]
	assert.False(t, hasAuth, "SignDoc should not contain authorization field")

	// Signatures should NOT be present
	_, hasSigs := rawMap["signatures"]
	assert.False(t, hasSigs, "SignDoc should not contain signatures field")
}
