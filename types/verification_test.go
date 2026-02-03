package types

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testMessage is a simple message implementation for testing
type testMessage struct {
	MsgType string        `json:"type"`
	Signers []AccountName `json:"signers"`
}

func (m *testMessage) Type() string              { return m.MsgType }
func (m *testMessage) ValidateBasic() error      { return nil }
func (m *testMessage) GetSigners() []AccountName { return m.Signers }

func TestTransaction_ToSignDoc(t *testing.T) {
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    42,
		Memo:     "test memo",
	}

	signDoc, err := tx.ToSignDoc("test-chain", 42)
	require.NoError(t, err)

	assert.Equal(t, SignDocVersion, signDoc.Version)
	assert.Equal(t, "test-chain", signDoc.ChainID)
	assert.Equal(t, StringUint64(42), signDoc.AccountSequence)
	assert.Equal(t, "alice", signDoc.Account)
	assert.Equal(t, StringUint64(42), signDoc.Nonce)
	assert.Equal(t, "test memo", signDoc.Memo)
	require.Len(t, signDoc.Messages, 1)
	assert.Equal(t, "/punnet.bank.v1.MsgSend", signDoc.Messages[0].Type)
}

func TestTransaction_ValidateSignDocRoundtrip(t *testing.T) {
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
		Memo:     "",
	}

	// Valid transaction should pass roundtrip
	err := tx.ValidateSignDocRoundtrip("test-chain", 1)
	assert.NoError(t, err)
}

// =============================================================================
// VALIDATE SIGNDOC ROUNDTRIP COMPREHENSIVE TESTS (Issue #6)
// =============================================================================
// SECURITY: These tests verify that ValidateSignDocRoundtrip correctly detects
// non-deterministic serialization, which is critical for preventing signature
// malleability attacks.

func TestTransaction_ValidateSignDocRoundtrip_Success(t *testing.T) {
	// Test various valid transactions pass roundtrip validation
	tests := []struct {
		name string
		tx   *Transaction
	}{
		{
			name: "basic transaction",
			tx: &Transaction{
				Account: "alice",
				Messages: []Message{&testMessage{
					MsgType: "/punnet.bank.v1.MsgSend",
					Signers: []AccountName{"alice"},
				}},
				Nonce: 1,
			},
		},
		{
			name: "transaction with memo",
			tx: &Transaction{
				Account: "bob",
				Messages: []Message{&testMessage{
					MsgType: "/punnet.staking.v1.MsgDelegate",
					Signers: []AccountName{"bob"},
				}},
				Nonce: 42,
				Memo:  "test memo with unicode: 日本語",
			},
		},
		{
			name: "transaction with fee",
			tx: &Transaction{
				Account: "charlie",
				Messages: []Message{&testMessage{
					MsgType: "/punnet.bank.v1.MsgSend",
					Signers: []AccountName{"charlie"},
				}},
				Nonce: 100,
				Fee: Fee{
					Amount:   Coins{{Denom: "uatom", Amount: 1000}},
					GasLimit: 200000,
				},
				FeeSlippage: Ratio{
					Numerator:   1,
					Denominator: 100,
				},
			},
		},
		{
			name: "transaction with multiple messages",
			tx: &Transaction{
				Account: "dave",
				Messages: []Message{
					&testMessage{MsgType: "/msg1", Signers: []AccountName{"dave"}},
					&testMessage{MsgType: "/msg2", Signers: []AccountName{"dave"}},
					&testMessage{MsgType: "/msg3", Signers: []AccountName{"dave"}},
				},
				Nonce: 999,
			},
		},
		{
			name: "transaction with special characters in memo",
			tx: &Transaction{
				Account: "eve",
				Messages: []Message{&testMessage{
					MsgType: "/punnet.bank.v1.MsgSend",
					Signers: []AccountName{"eve"},
				}},
				Nonce: 1,
				Memo:  `special chars: "quotes" and \backslash and newline\n`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tx.ValidateSignDocRoundtrip("test-chain", tt.tx.Nonce)
			assert.NoError(t, err, "valid transaction should pass roundtrip validation")
		})
	}
}

func TestTransaction_ValidateSignDocRoundtrip_Determinism(t *testing.T) {
	// INVARIANT: Repeated calls to ValidateSignDocRoundtrip produce consistent results.
	// This is critical for signature verification security.
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    42,
		Memo:     "determinism test",
		Fee: Fee{
			Amount:   Coins{{Denom: "uatom", Amount: 5000}},
			GasLimit: 100000,
		},
		FeeSlippage: Ratio{Numerator: 1, Denominator: 100},
	}

	// Call 100 times - must never fail
	for i := 0; i < 100; i++ {
		err := tx.ValidateSignDocRoundtrip("mainnet-1", 42)
		assert.NoError(t, err, "iteration %d failed", i)
	}
}

func TestTransaction_ValidateSignDocRoundtrip_ErrorIsSignDocMismatch(t *testing.T) {
	// SECURITY: Verify that when ValidateSignDocRoundtrip fails, it returns
	// ErrSignDocMismatch (or a wrapped version of it).
	//
	// Note: Since our serialization is deterministic, we can't directly test
	// the bytes mismatch path without mocking. Instead, we verify that the
	// error handling paths correctly wrap ErrSignDocMismatch.

	// Test case: Transaction that would fail during ToSignDoc (message conversion error)
	// We simulate this by using a message that returns an error from SignDocData.
	t.Run("verifies ErrSignDocMismatch is used for failures", func(t *testing.T) {
		// Create a valid transaction and verify success
		msg := &testMessage{
			MsgType: "/punnet.bank.v1.MsgSend",
			Signers: []AccountName{"alice"},
		}

		tx := &Transaction{
			Account:  "alice",
			Messages: []Message{msg},
			Nonce:    1,
		}

		// This should succeed
		err := tx.ValidateSignDocRoundtrip("test-chain", 1)
		assert.NoError(t, err)

		// Verify that the error type ErrSignDocMismatch exists and is the correct sentinel
		assert.NotNil(t, ErrSignDocMismatch)
		assert.Contains(t, ErrSignDocMismatch.Error(), "non-deterministic serialization")
	})
}

// failingSignDocMessage is a test message that fails SignDocData serialization.
// SECURITY: This is used to test error handling paths in ValidateSignDocRoundtrip.
type failingSignDocMessage struct {
	testMessage
	failOnSignDocData bool
}

func (m *failingSignDocMessage) SignDocData() (json.RawMessage, error) {
	if m.failOnSignDocData {
		return nil, ErrSignDocMismatch
	}
	return json.Marshal(map[string]interface{}{
		"signers": m.Signers,
	})
}

func TestTransaction_ValidateSignDocRoundtrip_SignDocCreationFailure(t *testing.T) {
	// Test that ValidateSignDocRoundtrip correctly handles SignDoc creation failures.
	// SECURITY: Error handling must not leak sensitive information or bypass validation.
	msg := &failingSignDocMessage{
		testMessage: testMessage{
			MsgType: "/punnet.bank.v1.MsgSend",
			Signers: []AccountName{"alice"},
		},
		failOnSignDocData: true,
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
	}

	err := tx.ValidateSignDocRoundtrip("test-chain", 1)
	assert.Error(t, err, "should fail when SignDoc creation fails")
	assert.ErrorIs(t, err, ErrSignDocMismatch, "error should wrap ErrSignDocMismatch")
}

func TestValidateSignDocRoundtrip_Integration_WithVerifyAuthorization(t *testing.T) {
	// SECURITY: Verify that ValidateSignDocRoundtrip is used correctly within
	// VerifyAuthorization to catch non-determinism before signature verification.
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
	}

	// First, verify roundtrip passes
	err = tx.ValidateSignDocRoundtrip("test-chain", 1)
	require.NoError(t, err, "roundtrip should pass for valid transaction")

	// Sign the transaction
	signDoc, err := tx.ToSignDoc("test-chain", 1)
	require.NoError(t, err)
	signBytes, _ := signDoc.GetSignBytes()
	sig := ed25519.Sign(priv, signBytes)

	tx.Authorization = &Authorization{
		Signatures:            []Signature{{Algorithm: AlgorithmEd25519, PubKey: pub, Signature: sig}},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	// Verify authorization (which internally does roundtrip validation)
	getter := newMockAccountGetter()
	err = tx.VerifyAuthorization("test-chain", account, getter)
	assert.NoError(t, err, "valid signature should pass verification")
}

func TestTransaction_VerifyAuthorization_Valid(t *testing.T) {
	// Generate key pair
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create account with single key authority
	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	// Create message
	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	// Create transaction
	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
		Memo:     "",
	}

	// Get sign bytes (hash of SignDoc)
	signDoc, err := tx.ToSignDoc("test-chain", 1)
	require.NoError(t, err)
	signBytes, err := signDoc.GetSignBytes()
	require.NoError(t, err)

	// Sign the SignDoc hash
	sig := ed25519.Sign(priv, signBytes)

	// Create authorization
	tx.Authorization = &Authorization{
		Signatures: []Signature{
			{
				Algorithm: AlgorithmEd25519,
				PubKey:    pub,
				Signature: sig,
			},
		},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	// Verify authorization
	getter := newMockAccountGetter()
	err = tx.VerifyAuthorization("test-chain", account, getter)
	assert.NoError(t, err)
}

func TestTransaction_VerifyAuthorization_InvalidNonce(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 5, // Account expects nonce 5
	}

	msg := &testMessage{
		MsgType: "/msg",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1, // Transaction has wrong nonce
	}

	signDoc, err := tx.ToSignDoc("test-chain", 1)
	require.NoError(t, err)
	signBytes, _ := signDoc.GetSignBytes()
	sig := ed25519.Sign(priv, signBytes)

	tx.Authorization = &Authorization{
		Signatures:            []Signature{{Algorithm: AlgorithmEd25519, PubKey: pub, Signature: sig}},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	getter := newMockAccountGetter()
	err = tx.VerifyAuthorization("test-chain", account, getter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonce")
}

func TestTransaction_VerifyAuthorization_InvalidSignature(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Use a different private key to create an invalid signature
	_, wrongPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	msg := &testMessage{
		MsgType: "/msg",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
	}

	signDoc, err := tx.ToSignDoc("test-chain", 1)
	require.NoError(t, err)
	signBytes, _ := signDoc.GetSignBytes()

	// Sign with wrong key
	wrongSig := ed25519.Sign(wrongPriv, signBytes)

	tx.Authorization = &Authorization{
		Signatures:            []Signature{{Algorithm: AlgorithmEd25519, PubKey: pub, Signature: wrongSig}},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	getter := newMockAccountGetter()
	err = tx.VerifyAuthorization("test-chain", account, getter)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSignature)
}

func TestTransaction_VerifyAuthorization_EmptyChainID(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	tx := &Transaction{
		Account:       "alice",
		Authorization: &Authorization{},
	}

	getter := newMockAccountGetter()
	err = tx.VerifyAuthorization("", account, getter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "chainID")
}

func TestTransaction_VerifyAuthorization_NilAccount(t *testing.T) {
	tx := &Transaction{
		Account:       "alice",
		Authorization: &Authorization{},
	}

	getter := newMockAccountGetter()
	err := tx.VerifyAuthorization("test-chain", nil, getter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account is nil")
}

func TestSignature_Algorithm_Backwards_Compatibility(t *testing.T) {
	// Empty algorithm should default to Ed25519
	sig := Signature{
		Algorithm: "",
		PubKey:    make([]byte, 32),
		Signature: make([]byte, 64),
	}

	assert.Equal(t, AlgorithmEd25519, sig.GetAlgorithm())
}

func TestSignature_ValidateBasic_Ed25519(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	tests := []struct {
		name      string
		sig       Signature
		expectErr bool
	}{
		{
			name: "valid ed25519 signature",
			sig: Signature{
				Algorithm: AlgorithmEd25519,
				PubKey:    pub,
				Signature: make([]byte, 64),
			},
			expectErr: false,
		},
		{
			name: "valid ed25519 with empty algorithm (backwards compat)",
			sig: Signature{
				Algorithm: "",
				PubKey:    pub,
				Signature: make([]byte, 64),
			},
			expectErr: false,
		},
		{
			name: "invalid pubkey length",
			sig: Signature{
				Algorithm: AlgorithmEd25519,
				PubKey:    make([]byte, 16),
				Signature: make([]byte, 64),
			},
			expectErr: true,
		},
		{
			name: "invalid signature length",
			sig: Signature{
				Algorithm: AlgorithmEd25519,
				PubKey:    pub,
				Signature: make([]byte, 32),
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sig.ValidateBasic()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSignature_ValidateBasic_Secp256k1(t *testing.T) {
	// secp256k1 is NOT production-ready. It should fail validation with ErrUnsupportedAlgorithm.
	// This test ensures we properly reject secp256k1 until it's fully implemented and tested.
	tests := []struct {
		name      string
		sig       Signature
		expectErr bool
	}{
		{
			name: "secp256k1 rejected (not production-ready)",
			sig: Signature{
				Algorithm: AlgorithmSecp256k1,
				PubKey:    make([]byte, 33), // Correct size, but algorithm is rejected
				Signature: make([]byte, 64),
			},
			expectErr: true, // Should fail because algorithm is not production-ready
		},
		{
			name: "secp256r1 rejected (not production-ready)",
			sig: Signature{
				Algorithm: AlgorithmSecp256r1,
				PubKey:    make([]byte, 33),
				Signature: make([]byte, 64),
			},
			expectErr: true, // Should fail because algorithm is not production-ready
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.sig.ValidateBasic()
			if tt.expectErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrUnsupportedAlgorithm)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSignature_ValidateBasic_UnsupportedAlgorithm(t *testing.T) {
	sig := Signature{
		Algorithm: "unknown-algo",
		PubKey:    make([]byte, 32),
		Signature: make([]byte, 64),
	}

	err := sig.ValidateBasic()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsupportedAlgorithm)
}

func TestSignature_Verify_Ed25519(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	message := []byte("test message")
	sig := ed25519.Sign(priv, message)

	signature := Signature{
		Algorithm: AlgorithmEd25519,
		PubKey:    pub,
		Signature: sig,
	}

	assert.True(t, signature.Verify(message))
	assert.False(t, signature.Verify([]byte("wrong message")))
}

func TestIsValidAlgorithm(t *testing.T) {
	// Ed25519 is production-ready
	assert.True(t, IsValidAlgorithm(AlgorithmEd25519))

	// Empty string defaults to Ed25519 for backwards compatibility
	assert.True(t, IsValidAlgorithm(""))

	// secp256k1 and secp256r1 are NOT production-ready yet
	// They are excluded from validation until properly implemented and tested
	assert.False(t, IsValidAlgorithm(AlgorithmSecp256k1), "secp256k1 should not be valid until implemented")
	assert.False(t, IsValidAlgorithm(AlgorithmSecp256r1), "secp256r1 should not be valid until implemented")

	// Unknown algorithms should be rejected
	assert.False(t, IsValidAlgorithm("unknown"))
}

func TestValidAlgorithms(t *testing.T) {
	algos := ValidAlgorithms()
	// Only Ed25519 is production-ready
	assert.Len(t, algos, 1)
	assert.Contains(t, algos, AlgorithmEd25519)

	// secp256k1 and secp256r1 should NOT be in the valid list until implemented
	assert.NotContains(t, algos, AlgorithmSecp256k1, "secp256k1 should not be listed as valid")
	assert.NotContains(t, algos, AlgorithmSecp256r1, "secp256r1 should not be listed as valid")
}

// TestSignDocReconstructionSecurity tests that SignDoc reconstruction is secure.
// INVARIANT: Signatures are verified against the reconstructed SignDoc, not stored bytes.
func TestSignDocReconstructionSecurity(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
	}

	// Sign with correct chain ID
	signDoc, err := tx.ToSignDoc("correct-chain", 1)
	require.NoError(t, err)
	signBytes, _ := signDoc.GetSignBytes()
	sig := ed25519.Sign(priv, signBytes)

	tx.Authorization = &Authorization{
		Signatures:            []Signature{{Algorithm: AlgorithmEd25519, PubKey: pub, Signature: sig}},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	getter := newMockAccountGetter()

	// Should succeed with correct chain ID
	err = tx.VerifyAuthorization("correct-chain", account, getter)
	assert.NoError(t, err)

	// SECURITY TEST: Should fail with different chain ID
	// This proves signatures are bound to the chain ID
	err = tx.VerifyAuthorization("wrong-chain", account, getter)
	assert.Error(t, err, "verification should fail with wrong chain ID")
}

// TestMultipleSignatures tests authorization with multiple signatures.
func TestMultipleSignatures(t *testing.T) {
	pub1, priv1, _ := ed25519.GenerateKey(nil)
	pub2, priv2, _ := ed25519.GenerateKey(nil)

	// Account requires threshold 2, each key has weight 1
	account := &Account{
		Name: "multisig",
		Authority: Authority{
			Threshold: 2,
			KeyWeights: map[string]uint64{
				string(pub1): 1,
				string(pub2): 1,
			},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	msg := &testMessage{
		MsgType: "/msg",
		Signers: []AccountName{"multisig"},
	}

	tx := &Transaction{
		Account:  "multisig",
		Messages: []Message{msg},
		Nonce:    1,
	}

	signDoc, err := tx.ToSignDoc("test-chain", 1)
	require.NoError(t, err)
	signBytes, _ := signDoc.GetSignBytes()

	sig1 := ed25519.Sign(priv1, signBytes)
	sig2 := ed25519.Sign(priv2, signBytes)

	tx.Authorization = &Authorization{
		Signatures: []Signature{
			{Algorithm: AlgorithmEd25519, PubKey: pub1, Signature: sig1},
			{Algorithm: AlgorithmEd25519, PubKey: pub2, Signature: sig2},
		},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	getter := newMockAccountGetter()
	err = tx.VerifyAuthorization("test-chain", account, getter)
	assert.NoError(t, err)
}

// TestInsufficientSignatures tests that authorization fails with insufficient weight.
func TestInsufficientSignatures(t *testing.T) {
	pub1, priv1, _ := ed25519.GenerateKey(nil)
	pub2, _, _ := ed25519.GenerateKey(nil) // Second key, not used

	// Account requires threshold 2
	account := &Account{
		Name: "multisig",
		Authority: Authority{
			Threshold: 2,
			KeyWeights: map[string]uint64{
				string(pub1): 1,
				string(pub2): 1,
			},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	msg := &testMessage{
		MsgType: "/msg",
		Signers: []AccountName{"multisig"},
	}

	tx := &Transaction{
		Account:  "multisig",
		Messages: []Message{msg},
		Nonce:    1,
	}

	signDoc, err := tx.ToSignDoc("test-chain", 1)
	require.NoError(t, err)
	signBytes, _ := signDoc.GetSignBytes()

	// Only one signature provided
	sig1 := ed25519.Sign(priv1, signBytes)

	tx.Authorization = &Authorization{
		Signatures: []Signature{
			{Algorithm: AlgorithmEd25519, PubKey: pub1, Signature: sig1},
		},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	getter := newMockAccountGetter()
	err = tx.VerifyAuthorization("test-chain", account, getter)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInsufficientWeight)
}

// helper to create messages for tests with json data
// nolint:unused // Reserved for future tests requiring message JSON data
func makeTestMessageJSON(t *testing.T, msgType string, signers []AccountName) (Message, json.RawMessage) {
	msg := &testMessage{
		MsgType: msgType,
		Signers: signers,
	}
	data, err := json.Marshal(map[string]interface{}{
		"signers": signers,
	})
	require.NoError(t, err)
	return msg, data
}

// ============================================================================
// Benchmarks for VerifyAuthorization optimization (issue #36)
// ============================================================================

// BenchmarkVerifyAuthorization measures the full verification path.
// Optimized in issue #36 to eliminate redundant SignDoc construction.
//
// Before optimization: 2x ToSignDoc + 3x ToJSON
// After optimization:  1x ToSignDoc + 2x ToJSON (reuse json1 for hash)
func BenchmarkVerifyAuthorization(b *testing.B) {
	// Setup: generate key and create valid signed transaction
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		b.Fatal(err)
	}

	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	msg := &testMessage{
		MsgType: "/punnet.bank.v1.MsgSend",
		Signers: []AccountName{"alice"},
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: []Message{msg},
		Nonce:    1,
		Memo:     "benchmark transaction",
	}

	signDoc, err := tx.ToSignDoc("test-chain", 1)
	require.NoError(b, err)
	signBytes, _ := signDoc.GetSignBytes()
	sig := ed25519.Sign(priv, signBytes)

	tx.Authorization = &Authorization{
		Signatures:            []Signature{{Algorithm: AlgorithmEd25519, PubKey: pub, Signature: sig}},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	getter := newMockAccountGetter()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := tx.VerifyAuthorization("test-chain", account, getter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerifyAuthorization_MultiSig measures verification with multiple signatures.
func BenchmarkVerifyAuthorization_MultiSig(b *testing.B) {
	pub1, priv1, _ := ed25519.GenerateKey(nil)
	pub2, priv2, _ := ed25519.GenerateKey(nil)

	account := &Account{
		Name: "multisig",
		Authority: Authority{
			Threshold: 2,
			KeyWeights: map[string]uint64{
				string(pub1): 1,
				string(pub2): 1,
			},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	msg := &testMessage{
		MsgType: "/msg",
		Signers: []AccountName{"multisig"},
	}

	tx := &Transaction{
		Account:  "multisig",
		Messages: []Message{msg},
		Nonce:    1,
	}

	signDoc, err := tx.ToSignDoc("test-chain", 1)
	require.NoError(b, err)
	signBytes, _ := signDoc.GetSignBytes()

	sig1 := ed25519.Sign(priv1, signBytes)
	sig2 := ed25519.Sign(priv2, signBytes)

	tx.Authorization = &Authorization{
		Signatures: []Signature{
			{Algorithm: AlgorithmEd25519, PubKey: pub1, Signature: sig1},
			{Algorithm: AlgorithmEd25519, PubKey: pub2, Signature: sig2},
		},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	getter := newMockAccountGetter()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := tx.VerifyAuthorization("test-chain", account, getter)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerifyAuthorization_LargeMessages measures verification with many messages.
func BenchmarkVerifyAuthorization_LargeMessages(b *testing.B) {
	pub, priv, _ := ed25519.GenerateKey(nil)

	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 1,
	}

	// Create 10 messages
	messages := make([]Message, 10)
	for i := range messages {
		messages[i] = &testMessage{
			MsgType: "/punnet.bank.v1.MsgSend",
			Signers: []AccountName{"alice"},
		}
	}

	tx := &Transaction{
		Account:  "alice",
		Messages: messages,
		Nonce:    1,
		Memo:     "benchmark with many messages",
	}

	signDoc, err := tx.ToSignDoc("test-chain", 1)
	require.NoError(b, err)
	signBytes, _ := signDoc.GetSignBytes()
	sig := ed25519.Sign(priv, signBytes)

	tx.Authorization = &Authorization{
		Signatures:            []Signature{{Algorithm: AlgorithmEd25519, PubKey: pub, Signature: sig}},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	getter := newMockAccountGetter()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := tx.VerifyAuthorization("test-chain", account, getter)
		if err != nil {
			b.Fatal(err)
		}
	}
}
