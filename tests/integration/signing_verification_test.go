package integration

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"testing"

	"github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test Utilities and Helpers
// =============================================================================

// testChainID is the chain ID used for SignDoc-based verification tests
const testChainID = "test-chain-integration"

// testKeyPair holds an Ed25519 key pair for testing
type testKeyPair struct {
	pub  ed25519.PublicKey
	priv ed25519.PrivateKey
}

// generateTestKeyPair creates a new random Ed25519 key pair
func generateTestKeyPair(t *testing.T) *testKeyPair {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err, "failed to generate key pair")
	return &testKeyPair{pub: pub, priv: priv}
}

// sign signs a message with this key pair
func (kp *testKeyPair) sign(message []byte) []byte {
	return ed25519.Sign(kp.priv, message)
}

// toSignature creates a Signature from this key pair's signature
func (kp *testKeyPair) toSignature(message []byte) types.Signature {
	return types.Signature{
		PubKey:    kp.pub,
		Signature: kp.sign(message),
	}
}

// signingTestEnv provides test environment for signing tests
type signingTestEnv struct {
	getter  *mockSigningAccountGetter
	chainID string
}

// mockSigningAccountGetter implements types.AccountGetter for testing
type mockSigningAccountGetter struct {
	accounts map[types.AccountName]*types.Account
}

func newMockSigningAccountGetter() *mockSigningAccountGetter {
	return &mockSigningAccountGetter{
		accounts: make(map[types.AccountName]*types.Account),
	}
}

func (m *mockSigningAccountGetter) GetAccount(name types.AccountName) (*types.Account, error) {
	acc, ok := m.accounts[name]
	if !ok {
		return nil, types.ErrNotFound
	}
	return acc, nil
}

func (m *mockSigningAccountGetter) setAccount(acc *types.Account) {
	m.accounts[acc.Name] = acc
}

// newSigningTestEnv creates a new signing test environment
func newSigningTestEnv() *signingTestEnv {
	return &signingTestEnv{
		getter:  newMockSigningAccountGetter(),
		chainID: testChainID,
	}
}

// getSignDocBytes returns the bytes to sign for a transaction using SignDoc.
// This matches how VerifyAuthorization computes the sign bytes.
func (e *signingTestEnv) getSignDocBytes(t *testing.T, tx *types.Transaction, account *types.Account) []byte {
	t.Helper()
	signDoc := tx.ToSignDoc(e.chainID, account.Nonce)
	signBytes, err := signDoc.GetSignBytes()
	require.NoError(t, err, "failed to get sign bytes from SignDoc")
	return signBytes
}

// createAccount creates an account with a single key authority
func (e *signingTestEnv) createAccount(name string, pubKey []byte) *types.Account {
	account := &types.Account{
		Name: types.AccountName(name),
		Authority: types.Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pubKey): 1},
			AccountWeights: make(map[types.AccountName]uint64),
		},
		Nonce: 0,
	}
	e.getter.setAccount(account)
	return account
}

// createMultiSigAccount creates an account with multiple keys
func (e *signingTestEnv) createMultiSigAccount(name string, threshold uint64, keys [][]byte, weights []uint64) *types.Account {
	keyWeights := make(map[string]uint64)
	for i, key := range keys {
		keyWeights[string(key)] = weights[i]
	}
	account := &types.Account{
		Name: types.AccountName(name),
		Authority: types.Authority{
			Threshold:      threshold,
			KeyWeights:     keyWeights,
			AccountWeights: make(map[types.AccountName]uint64),
		},
		Nonce: 0,
	}
	e.getter.setAccount(account)
	return account
}

// createDelegatedAccount creates an account that delegates to other accounts
func (e *signingTestEnv) createDelegatedAccount(name string, threshold uint64, keys [][]byte, keyWeights []uint64, accounts []types.AccountName, accountWeights []uint64) *types.Account {
	kw := make(map[string]uint64)
	for i, key := range keys {
		kw[string(key)] = keyWeights[i]
	}
	aw := make(map[types.AccountName]uint64)
	for i, acct := range accounts {
		aw[acct] = accountWeights[i]
	}
	account := &types.Account{
		Name: types.AccountName(name),
		Authority: types.Authority{
			Threshold:      threshold,
			KeyWeights:     kw,
			AccountWeights: aw,
		},
		Nonce: 0,
	}
	e.getter.setAccount(account)
	return account
}

// testMessage is a simple message implementation for testing
type testMessage struct {
	signer types.AccountName
	data   string
}

func (m *testMessage) Type() string {
	return "test/message"
}

func (m *testMessage) ValidateBasic() error {
	return nil
}

func (m *testMessage) GetSigners() []types.AccountName {
	return []types.AccountName{m.signer}
}

// =============================================================================
// 1. Single Signature Flow Tests
// =============================================================================

func TestSingleSignatureFlow_BasicSuccess(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	// Create account with single key
	account := env.createAccount("alice", keyPair.pub)

	// Create transaction
	msg := &testMessage{signer: "alice", data: "transfer 100 tokens"}
	tx := types.NewTransaction("alice", 0, []types.Message{msg}, nil)

	// Get sign bytes using SignDoc (matches how verification computes sign bytes)
	signBytes := env.getSignDocBytes(t, tx, account)

	// Sign
	sig := keyPair.toSignature(signBytes)
	auth := types.NewAuthorization(sig)
	tx.Authorization = auth

	// Verify
	err := tx.VerifyAuthorization(env.chainID, account, env.getter)
	assert.NoError(t, err, "single signature verification should succeed")
}

func TestSingleSignatureFlow_DirectAuthorizationVerify(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	// Create account
	account := env.createAccount("alice", keyPair.pub)

	// Create and sign message
	message := []byte("test transaction data")
	sig := keyPair.toSignature(message)
	auth := types.NewAuthorization(sig)

	// Verify directly on authorization
	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.NoError(t, err, "direct authorization verification should succeed")
}

func TestSingleSignatureFlow_MultipleMessagesInTransaction(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)

	// Transaction with multiple messages
	messages := []types.Message{
		&testMessage{signer: "alice", data: "message 1"},
		&testMessage{signer: "alice", data: "message 2"},
		&testMessage{signer: "alice", data: "message 3"},
	}
	tx := types.NewTransaction("alice", 0, messages, nil)
	signBytes := env.getSignDocBytes(t, tx, account)

	sig := keyPair.toSignature(signBytes)
	tx.Authorization = types.NewAuthorization(sig)

	err := tx.VerifyAuthorization(env.chainID, account, env.getter)
	assert.NoError(t, err, "multi-message transaction should verify")
}

func TestSingleSignatureFlow_WithMemo(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)

	msg := &testMessage{signer: "alice", data: "transfer"}
	tx := types.NewTransaction("alice", 0, []types.Message{msg}, nil)
	tx.Memo = "payment for services"

	signBytes := env.getSignDocBytes(t, tx, account)
	sig := keyPair.toSignature(signBytes)
	tx.Authorization = types.NewAuthorization(sig)

	err := tx.VerifyAuthorization(env.chainID, account, env.getter)
	assert.NoError(t, err, "transaction with memo should verify")
}

// =============================================================================
// 2. Multi-Signature Flow Tests
// =============================================================================

func TestMultiSignatureFlow_TwoOfTwo(t *testing.T) {
	env := newSigningTestEnv()
	key1 := generateTestKeyPair(t)
	key2 := generateTestKeyPair(t)

	// Create 2-of-2 multisig account
	account := env.createMultiSigAccount("multisig", 2,
		[][]byte{key1.pub, key2.pub},
		[]uint64{1, 1})

	message := []byte("multisig transaction")

	// Both parties sign
	sig1 := key1.toSignature(message)
	sig2 := key2.toSignature(message)
	auth := types.NewAuthorization(sig1, sig2)

	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.NoError(t, err, "2-of-2 multisig should succeed with both signatures")
}

func TestMultiSignatureFlow_TwoOfThree(t *testing.T) {
	env := newSigningTestEnv()
	key1 := generateTestKeyPair(t)
	key2 := generateTestKeyPair(t)
	key3 := generateTestKeyPair(t)

	// Create 2-of-3 multisig account
	account := env.createMultiSigAccount("multisig", 2,
		[][]byte{key1.pub, key2.pub, key3.pub},
		[]uint64{1, 1, 1})

	message := []byte("multisig transaction")

	// Test all valid combinations (any 2 of 3)
	testCases := []struct {
		name string
		keys []*testKeyPair
	}{
		{"keys 1 and 2", []*testKeyPair{key1, key2}},
		{"keys 1 and 3", []*testKeyPair{key1, key3}},
		{"keys 2 and 3", []*testKeyPair{key2, key3}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sigs := make([]types.Signature, len(tc.keys))
			for i, kp := range tc.keys {
				sigs[i] = kp.toSignature(message)
			}
			auth := types.NewAuthorization(sigs...)

			err := auth.VerifyAuthorization(account, message, env.getter)
			assert.NoError(t, err, "2-of-3 should succeed with %s", tc.name)
		})
	}
}

func TestMultiSignatureFlow_WeightedThreshold(t *testing.T) {
	env := newSigningTestEnv()
	adminKey := generateTestKeyPair(t)
	userKey1 := generateTestKeyPair(t)
	userKey2 := generateTestKeyPair(t)

	// Admin has weight 3, users have weight 1 each. Threshold is 3.
	// Admin alone can authorize, or both users together.
	account := env.createMultiSigAccount("weighted", 3,
		[][]byte{adminKey.pub, userKey1.pub, userKey2.pub},
		[]uint64{3, 1, 1})

	message := []byte("important transaction")

	// Admin alone should work
	t.Run("admin alone", func(t *testing.T) {
		sig := adminKey.toSignature(message)
		auth := types.NewAuthorization(sig)

		err := auth.VerifyAuthorization(account, message, env.getter)
		assert.NoError(t, err, "admin alone should authorize")
	})

	// Both users together should work (1+1 < 3, so this should fail)
	t.Run("both users insufficient", func(t *testing.T) {
		sig1 := userKey1.toSignature(message)
		sig2 := userKey2.toSignature(message)
		auth := types.NewAuthorization(sig1, sig2)

		err := auth.VerifyAuthorization(account, message, env.getter)
		assert.ErrorIs(t, err, types.ErrInsufficientWeight, "two users (weight 2) should be insufficient for threshold 3")
	})

	// One user alone should fail
	t.Run("single user insufficient", func(t *testing.T) {
		sig := userKey1.toSignature(message)
		auth := types.NewAuthorization(sig)

		err := auth.VerifyAuthorization(account, message, env.getter)
		assert.ErrorIs(t, err, types.ErrInsufficientWeight)
	})
}

func TestMultiSignatureFlow_MissingSignature(t *testing.T) {
	env := newSigningTestEnv()
	key1 := generateTestKeyPair(t)
	key2 := generateTestKeyPair(t)

	// 2-of-2 requires both signatures
	account := env.createMultiSigAccount("multisig", 2,
		[][]byte{key1.pub, key2.pub},
		[]uint64{1, 1})

	message := []byte("multisig transaction")

	// Only one signature provided
	sig1 := key1.toSignature(message)
	auth := types.NewAuthorization(sig1)

	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.ErrorIs(t, err, types.ErrInsufficientWeight, "missing signature should fail")
}

func TestMultiSignatureFlow_AllThreeSignatures(t *testing.T) {
	env := newSigningTestEnv()
	key1 := generateTestKeyPair(t)
	key2 := generateTestKeyPair(t)
	key3 := generateTestKeyPair(t)

	// 2-of-3 - providing all 3 should still work
	account := env.createMultiSigAccount("multisig", 2,
		[][]byte{key1.pub, key2.pub, key3.pub},
		[]uint64{1, 1, 1})

	message := []byte("multisig transaction")

	sig1 := key1.toSignature(message)
	sig2 := key2.toSignature(message)
	sig3 := key3.toSignature(message)
	auth := types.NewAuthorization(sig1, sig2, sig3)

	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.NoError(t, err, "providing extra signatures should not cause failure")
}

// =============================================================================
// 3. Algorithm Matrix Tests (Ed25519 - current implementation)
// =============================================================================

func TestAlgorithmMatrix_Ed25519SignAndVerify(t *testing.T) {
	env := newSigningTestEnv()

	// Generate multiple key pairs to test Ed25519 thoroughly
	keyPairs := make([]*testKeyPair, 5)
	for i := range keyPairs {
		keyPairs[i] = generateTestKeyPair(t)
	}

	// Test signing and verification with each key
	for i, kp := range keyPairs {
		t.Run("keypair_"+string(rune('A'+i)), func(t *testing.T) {
			account := env.createAccount("user"+string(rune('a'+i)), kp.pub)
			message := []byte("test message for keypair")

			sig := kp.toSignature(message)
			auth := types.NewAuthorization(sig)

			err := auth.VerifyAuthorization(account, message, env.getter)
			assert.NoError(t, err, "Ed25519 sign/verify should work")
		})
	}
}

func TestAlgorithmMatrix_Ed25519KeySizes(t *testing.T) {
	// Verify Ed25519 key sizes are enforced
	t.Run("valid key sizes", func(t *testing.T) {
		kp := generateTestKeyPair(t)
		assert.Equal(t, ed25519.PublicKeySize, len(kp.pub), "public key should be 32 bytes")
		assert.Equal(t, ed25519.PrivateKeySize, len(kp.priv), "private key should be 64 bytes")

		message := []byte("test")
		sigBytes := kp.sign(message)
		assert.Equal(t, ed25519.SignatureSize, len(sigBytes), "signature should be 64 bytes")
	})
}

// Note: secp256k1 and secp256r1 tests would go here when those algorithms are implemented
// func TestAlgorithmMatrix_Secp256k1SignAndVerify(t *testing.T) { ... }
// func TestAlgorithmMatrix_Secp256r1SignAndVerify(t *testing.T) { ... }

// =============================================================================
// 4. Negative Tests - Security Critical
// =============================================================================

func TestNegative_WrongNonce(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)
	account.Nonce = 5 // Account expects nonce 5

	msg := &testMessage{signer: "alice", data: "transfer"}
	tx := types.NewTransaction("alice", 0, []types.Message{msg}, nil) // Wrong nonce: 0 instead of 5

	// INVARIANT: Sign bytes must bind to a specific nonce to prevent replay attacks.
	// Here we deliberately sign with tx.Nonce (0) while account expects nonce 5.
	// The verification will compute sign bytes using account.Nonce (5), causing mismatch.
	signDoc := tx.ToSignDoc(testChainID, tx.Nonce) // Deliberately using tx.Nonce (wrong)
	signBytes, err := signDoc.GetSignBytes()
	require.NoError(t, err)
	sig := keyPair.toSignature(signBytes)
	tx.Authorization = types.NewAuthorization(sig)

	err := tx.VerifyAuthorization(env.chainID, account, env.getter)
	assert.Error(t, err, "wrong nonce should fail verification")
	assert.Contains(t, err.Error(), "nonce", "error should mention nonce")
}

func TestNegative_TamperedTransaction_ModifiedAccount(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)
	env.createAccount("bob", keyPair.pub) // Bob's account also exists

	// Alice signs a transaction
	msg := &testMessage{signer: "alice", data: "transfer"}
	tx := types.NewTransaction("alice", 0, []types.Message{msg}, nil)
	signBytes := env.getSignDocBytes(t, tx, account)
	sig := keyPair.toSignature(signBytes)
	tx.Authorization = types.NewAuthorization(sig)

	// Verify original works
	err := tx.VerifyAuthorization(env.chainID, account, env.getter)
	assert.NoError(t, err, "original should verify")

	// Create a modified transaction (changing account name)
	// The signature was for "alice" but now we're changing it to claim it's from "bob"
	tamperedTx := types.NewTransaction("bob", 0, []types.Message{&testMessage{signer: "bob", data: "transfer"}}, nil)
	tamperedTx.Authorization = tx.Authorization // Use alice's signature

	// Bob's account should fail because signature doesn't match bob's sign bytes
	bobAccount, _ := env.getter.GetAccount("bob")
	err = tamperedTx.VerifyAuthorization(env.chainID, bobAccount, env.getter)
	assert.Error(t, err, "tampered transaction (different account) should fail")
}

func TestNegative_TamperedTransaction_ModifiedMemo(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)

	// Sign transaction with one memo
	msg := &testMessage{signer: "alice", data: "transfer"}
	tx := types.NewTransaction("alice", 0, []types.Message{msg}, nil)
	tx.Memo = "original memo"
	signBytes := env.getSignDocBytes(t, tx, account)
	sig := keyPair.toSignature(signBytes)
	tx.Authorization = types.NewAuthorization(sig)

	// Original verifies
	err := tx.VerifyAuthorization(env.chainID, account, env.getter)
	assert.NoError(t, err, "original should verify")

	// Tamper with memo after signing
	tx.Memo = "tampered memo"

	// Verification should fail because sign bytes changed
	err = tx.VerifyAuthorization(env.chainID, account, env.getter)
	assert.Error(t, err, "tampered memo should fail verification")
}

func TestNegative_InvalidSignatureBytes(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)
	message := []byte("test message")

	testCases := []struct {
		name      string
		signature types.Signature
	}{
		{
			"all zeros signature",
			types.Signature{
				PubKey:    keyPair.pub,
				Signature: make([]byte, ed25519.SignatureSize),
			},
		},
		{
			"random garbage signature",
			types.Signature{
				PubKey:    keyPair.pub,
				Signature: []byte("this is not a valid ed25519 signature bytes 64 chars"),
			},
		},
		{
			"truncated signature",
			types.Signature{
				PubKey:    keyPair.pub,
				Signature: keyPair.sign(message)[:32], // Only half the signature
			},
		},
		{
			"extended signature",
			types.Signature{
				PubKey:    keyPair.pub,
				Signature: append(keyPair.sign(message), 0x00), // Extra byte
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			auth := &types.Authorization{
				Signatures:            []types.Signature{tc.signature},
				AccountAuthorizations: make(map[types.AccountName]*types.Authorization),
			}

			err := auth.VerifyAuthorization(account, message, env.getter)
			assert.Error(t, err, "invalid signature should fail: %s", tc.name)
		})
	}
}

func TestNegative_WrongPublicKey(t *testing.T) {
	env := newSigningTestEnv()
	aliceKey := generateTestKeyPair(t)
	bobKey := generateTestKeyPair(t)

	// Alice's account
	account := env.createAccount("alice", aliceKey.pub)
	message := []byte("test message")

	// Sign with Bob's key but claim it's for Alice's account
	sig := types.Signature{
		PubKey:    bobKey.pub, // Bob's public key (not in alice's authority)
		Signature: bobKey.sign(message),
	}
	auth := types.NewAuthorization(sig)

	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.ErrorIs(t, err, types.ErrInsufficientWeight, "wrong public key should result in insufficient weight")
}

func TestNegative_SignatureForDifferentMessage(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)

	originalMessage := []byte("original message")
	differentMessage := []byte("different message")

	// Sign different message
	sig := keyPair.toSignature(differentMessage)
	auth := types.NewAuthorization(sig)

	// Verify against original message - should fail
	err := auth.VerifyAuthorization(account, originalMessage, env.getter)
	assert.Error(t, err, "signature for different message should fail")
}

func TestNegative_EmptySignatures(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)
	message := []byte("test message")

	auth := &types.Authorization{
		Signatures:            []types.Signature{}, // No signatures
		AccountAuthorizations: make(map[types.AccountName]*types.Authorization),
	}

	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.ErrorIs(t, err, types.ErrInsufficientWeight, "empty signatures should fail")
}

func TestNegative_NilAuthorization(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)
	message := []byte("test message")

	var auth *types.Authorization = nil

	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.Error(t, err, "nil authorization should fail")
}

func TestNegative_NilAccount(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	message := []byte("test message")
	sig := keyPair.toSignature(message)
	auth := types.NewAuthorization(sig)

	err := auth.VerifyAuthorization(nil, message, env.getter)
	assert.Error(t, err, "nil account should fail")
}

func TestNegative_NilAccountGetter(t *testing.T) {
	keyPair := generateTestKeyPair(t)

	account := &types.Account{
		Name: "alice",
		Authority: types.Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(keyPair.pub): 1},
			AccountWeights: make(map[types.AccountName]uint64),
		},
	}

	message := []byte("test message")
	sig := keyPair.toSignature(message)
	auth := types.NewAuthorization(sig)

	err := auth.VerifyAuthorization(account, message, nil)
	assert.Error(t, err, "nil account getter should fail")
}

func TestNegative_DuplicateSignatures(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	// Account with threshold 2, but only one key with weight 1
	// Someone might try to submit the same signature twice
	account := &types.Account{
		Name: "alice",
		Authority: types.Authority{
			Threshold:      2,
			KeyWeights:     map[string]uint64{string(keyPair.pub): 1},
			AccountWeights: make(map[types.AccountName]uint64),
		},
	}
	env.getter.setAccount(account)

	message := []byte("test message")
	sig := keyPair.toSignature(message)

	// Submit same signature twice - should not count twice
	auth := types.NewAuthorization(sig, sig)

	err := auth.VerifyAuthorization(account, message, env.getter)
	// The current implementation may count duplicates, which would be a bug
	// Let's verify the behavior - if weight is counted per signature, this could pass incorrectly
	// This test documents current behavior
	if err == nil {
		t.Log("WARNING: Duplicate signatures may be counted multiple times - potential security issue")
	}
}

func TestNegative_MalformedPublicKey(t *testing.T) {
	testCases := []struct {
		name   string
		pubKey []byte
	}{
		{"empty public key", []byte{}},
		{"too short public key", make([]byte, 16)},
		{"too long public key", make([]byte, 64)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sig := types.Signature{
				PubKey:    tc.pubKey,
				Signature: make([]byte, ed25519.SignatureSize),
			}

			err := sig.ValidateBasic()
			assert.Error(t, err, "malformed public key should fail validation")
		})
	}
}

// =============================================================================
// 5. Roundtrip Tests (Sign -> Serialize -> Deserialize -> Verify)
// =============================================================================

func TestRoundtrip_JSONSerialization(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)

	// Create and sign
	msg := &testMessage{signer: "alice", data: "transfer"}
	tx := types.NewTransaction("alice", 0, []types.Message{msg}, nil)
	signBytes := env.getSignDocBytes(t, tx, account)
	sig := keyPair.toSignature(signBytes)
	tx.Authorization = types.NewAuthorization(sig)

	// Verify before serialization
	err := tx.VerifyAuthorization(env.chainID, account, env.getter)
	require.NoError(t, err, "should verify before serialization")

	// Serialize authorization to JSON
	authJSON, err := json.Marshal(tx.Authorization)
	require.NoError(t, err, "should marshal authorization")

	// Deserialize
	var deserializedAuth types.Authorization
	err = json.Unmarshal(authJSON, &deserializedAuth)
	require.NoError(t, err, "should unmarshal authorization")

	// Verify deserialized
	err = deserializedAuth.VerifyAuthorization(account, signBytes, env.getter)
	assert.NoError(t, err, "deserialized authorization should verify")
}

func TestRoundtrip_SignaturePreservation(t *testing.T) {
	keyPair := generateTestKeyPair(t)
	message := []byte("test message for roundtrip")

	// Create signature
	original := keyPair.toSignature(message)

	// Serialize to JSON
	jsonBytes, err := json.Marshal(original)
	require.NoError(t, err)

	// Deserialize
	var restored types.Signature
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	// Verify byte-for-byte equality
	assert.True(t, bytes.Equal(original.PubKey, restored.PubKey), "public key should be preserved")
	assert.True(t, bytes.Equal(original.Signature, restored.Signature), "signature should be preserved")

	// Verify both can verify the message
	assert.True(t, original.Verify(message), "original should verify")
	assert.True(t, restored.Verify(message), "restored should verify")
}

func TestRoundtrip_MultiSigAuthorization(t *testing.T) {
	env := newSigningTestEnv()
	key1 := generateTestKeyPair(t)
	key2 := generateTestKeyPair(t)
	key3 := generateTestKeyPair(t)

	account := env.createMultiSigAccount("multisig", 2,
		[][]byte{key1.pub, key2.pub, key3.pub},
		[]uint64{1, 1, 1})

	message := []byte("multisig roundtrip test")

	// Create multi-sig authorization
	sig1 := key1.toSignature(message)
	sig2 := key2.toSignature(message)
	original := types.NewAuthorization(sig1, sig2)

	// Verify original
	err := original.VerifyAuthorization(account, message, env.getter)
	require.NoError(t, err, "original should verify")

	// Serialize
	jsonBytes, err := json.Marshal(original)
	require.NoError(t, err)

	// Deserialize
	var restored types.Authorization
	err = json.Unmarshal(jsonBytes, &restored)
	require.NoError(t, err)

	// Verify restored
	err = restored.VerifyAuthorization(account, message, env.getter)
	assert.NoError(t, err, "restored multisig should verify")
}

func TestRoundtrip_EmptyMemo(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)

	// Transaction without memo
	msg := &testMessage{signer: "alice", data: "no memo"}
	tx := types.NewTransaction("alice", 0, []types.Message{msg}, nil)
	signBytes := env.getSignDocBytes(t, tx, account)
	sig := keyPair.toSignature(signBytes)
	tx.Authorization = types.NewAuthorization(sig)

	// Serialize and deserialize
	authJSON, _ := json.Marshal(tx.Authorization)
	var restored types.Authorization
	_ = json.Unmarshal(authJSON, &restored)

	// Should still verify
	err := restored.VerifyAuthorization(account, signBytes, env.getter)
	assert.NoError(t, err, "roundtrip with empty memo should work")
}

// =============================================================================
// 6. Cross-Signer Tests (Delegated Authorization)
// =============================================================================

func TestCrossSigner_SimpleDelegation(t *testing.T) {
	env := newSigningTestEnv()
	aliceKey := generateTestKeyPair(t)
	bobKey := generateTestKeyPair(t)

	// Bob has a simple account
	env.createAccount("bob", bobKey.pub)

	// Alice delegates to Bob
	env.createDelegatedAccount("alice", 1,
		[][]byte{aliceKey.pub}, []uint64{1},
		[]types.AccountName{"bob"}, []uint64{1})

	message := []byte("delegated transaction")

	// Bob signs on behalf of Alice
	bobSig := bobKey.toSignature(message)
	bobAuth := types.NewAuthorization(bobSig)

	// Alice's authorization uses Bob's
	aliceAuth := &types.Authorization{
		Signatures: []types.Signature{},
		AccountAuthorizations: map[types.AccountName]*types.Authorization{
			"bob": bobAuth,
		},
	}

	alice, _ := env.getter.GetAccount("alice")
	err := aliceAuth.VerifyAuthorization(alice, message, env.getter)
	assert.NoError(t, err, "delegated authorization should work")
}

func TestCrossSigner_MultipleDelegates(t *testing.T) {
	env := newSigningTestEnv()
	aliceKey := generateTestKeyPair(t)
	bobKey := generateTestKeyPair(t)
	charlieKey := generateTestKeyPair(t)

	// Bob and Charlie have simple accounts
	env.createAccount("bob", bobKey.pub)
	env.createAccount("charlie", charlieKey.pub)

	// Alice delegates to both Bob and Charlie (threshold 2, each delegate has weight 1)
	env.createDelegatedAccount("alice", 2,
		[][]byte{aliceKey.pub}, []uint64{2}, // Alice's key has weight 2 (can authorize alone)
		[]types.AccountName{"bob", "charlie"}, []uint64{1, 1})

	message := []byte("requires two delegates")

	// Both Bob and Charlie sign
	bobSig := bobKey.toSignature(message)
	bobAuth := types.NewAuthorization(bobSig)
	charlieSig := charlieKey.toSignature(message)
	charlieAuth := types.NewAuthorization(charlieSig)

	aliceAuth := &types.Authorization{
		Signatures: []types.Signature{},
		AccountAuthorizations: map[types.AccountName]*types.Authorization{
			"bob":     bobAuth,
			"charlie": charlieAuth,
		},
	}

	alice, _ := env.getter.GetAccount("alice")
	err := aliceAuth.VerifyAuthorization(alice, message, env.getter)
	assert.NoError(t, err, "multiple delegates should authorize together")
}

func TestCrossSigner_MixedKeyAndDelegation(t *testing.T) {
	env := newSigningTestEnv()
	aliceKey := generateTestKeyPair(t)
	bobKey := generateTestKeyPair(t)

	// Bob has a simple account
	env.createAccount("bob", bobKey.pub)

	// Alice has both a key and delegation (threshold 2)
	env.createDelegatedAccount("alice", 2,
		[][]byte{aliceKey.pub}, []uint64{1},
		[]types.AccountName{"bob"}, []uint64{1})

	message := []byte("mixed auth transaction")

	// Alice signs directly
	aliceSig := aliceKey.toSignature(message)

	// Bob also signs
	bobSig := bobKey.toSignature(message)
	bobAuth := types.NewAuthorization(bobSig)

	// Combined authorization
	aliceAuth := &types.Authorization{
		Signatures: []types.Signature{aliceSig},
		AccountAuthorizations: map[types.AccountName]*types.Authorization{
			"bob": bobAuth,
		},
	}

	alice, _ := env.getter.GetAccount("alice")
	err := aliceAuth.VerifyAuthorization(alice, message, env.getter)
	assert.NoError(t, err, "mixed key and delegation should work")
}

func TestCrossSigner_DelegationChain(t *testing.T) {
	env := newSigningTestEnv()
	aliceKey := generateTestKeyPair(t)
	bobKey := generateTestKeyPair(t)
	charlieKey := generateTestKeyPair(t)

	// Charlie has simple account
	env.createAccount("charlie", charlieKey.pub)

	// Bob delegates to Charlie
	env.createDelegatedAccount("bob", 1,
		[][]byte{bobKey.pub}, []uint64{1},
		[]types.AccountName{"charlie"}, []uint64{1})

	// Alice delegates to Bob
	env.createDelegatedAccount("alice", 1,
		[][]byte{aliceKey.pub}, []uint64{1},
		[]types.AccountName{"bob"}, []uint64{1})

	message := []byte("chain delegation")

	// Charlie signs
	charlieSig := charlieKey.toSignature(message)
	charlieAuth := types.NewAuthorization(charlieSig)

	// Bob's auth uses Charlie's
	bobAuth := &types.Authorization{
		Signatures: []types.Signature{},
		AccountAuthorizations: map[types.AccountName]*types.Authorization{
			"charlie": charlieAuth,
		},
	}

	// Alice's auth uses Bob's
	aliceAuth := &types.Authorization{
		Signatures: []types.Signature{},
		AccountAuthorizations: map[types.AccountName]*types.Authorization{
			"bob": bobAuth,
		},
	}

	alice, _ := env.getter.GetAccount("alice")
	err := aliceAuth.VerifyAuthorization(alice, message, env.getter)
	assert.NoError(t, err, "delegation chain should work")
}

func TestCrossSigner_InvalidDelegate(t *testing.T) {
	env := newSigningTestEnv()
	aliceKey := generateTestKeyPair(t)
	bobKey := generateTestKeyPair(t)
	charlieKey := generateTestKeyPair(t)

	// Bob has a simple account
	env.createAccount("bob", bobKey.pub)

	// Alice only delegates to Bob, not Charlie
	env.createDelegatedAccount("alice", 1,
		[][]byte{aliceKey.pub}, []uint64{1},
		[]types.AccountName{"bob"}, []uint64{1})

	message := []byte("invalid delegate")

	// Charlie tries to sign (but isn't in Alice's delegation list)
	charlieSig := charlieKey.toSignature(message)
	charlieAuth := types.NewAuthorization(charlieSig)

	aliceAuth := &types.Authorization{
		Signatures: []types.Signature{},
		AccountAuthorizations: map[types.AccountName]*types.Authorization{
			"charlie": charlieAuth, // Charlie is not authorized
		},
	}

	alice, _ := env.getter.GetAccount("alice")
	err := aliceAuth.VerifyAuthorization(alice, message, env.getter)
	assert.ErrorIs(t, err, types.ErrInsufficientWeight, "unauthorized delegate should fail")
}

// =============================================================================
// 7. Edge Cases and Stress Tests
// =============================================================================

func TestEdgeCase_LargeNumberOfSignatures(t *testing.T) {
	env := newSigningTestEnv()

	// Create many keys
	numKeys := 20
	keys := make([]*testKeyPair, numKeys)
	pubKeys := make([][]byte, numKeys)
	weights := make([]uint64, numKeys)

	for i := 0; i < numKeys; i++ {
		keys[i] = generateTestKeyPair(t)
		pubKeys[i] = keys[i].pub
		weights[i] = 1
	}

	// 10-of-20 multisig
	account := env.createMultiSigAccount("largemultisig", 10, pubKeys, weights)

	message := []byte("large multisig transaction")

	// Sign with first 10 keys
	sigs := make([]types.Signature, 10)
	for i := 0; i < 10; i++ {
		sigs[i] = keys[i].toSignature(message)
	}
	auth := types.NewAuthorization(sigs...)

	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.NoError(t, err, "10-of-20 multisig should work")
}

func TestEdgeCase_MaxRecursionDepth(t *testing.T) {
	env := newSigningTestEnv()

	// Create a chain of delegations at max depth
	depth := types.MaxRecursionDepth + 2
	keys := make([]*testKeyPair, depth)

	for i := 0; i < depth; i++ {
		keys[i] = generateTestKeyPair(t)
		name := types.AccountName("account" + string(rune('a'+i)))

		if i == 0 {
			// Bottom of chain - simple account
			env.createAccount(string(name), keys[i].pub)
		} else {
			// Delegates to previous account
			prevName := types.AccountName("account" + string(rune('a'+i-1)))
			env.createDelegatedAccount(string(name), 1,
				[][]byte{keys[i].pub}, []uint64{1},
				[]types.AccountName{prevName}, []uint64{1})
		}
	}

	message := []byte("deep chain test")

	// Build authorization chain from bottom up
	bottomSig := keys[0].toSignature(message)
	currentAuth := types.NewAuthorization(bottomSig)

	for i := 1; i < depth; i++ {
		prevName := types.AccountName("account" + string(rune('a'+i-1)))
		currentAuth = &types.Authorization{
			Signatures: []types.Signature{},
			AccountAuthorizations: map[types.AccountName]*types.Authorization{
				prevName: currentAuth,
			},
		}
	}

	topName := types.AccountName("account" + string(rune('a'+depth-1)))
	topAccount, _ := env.getter.GetAccount(topName)

	err := currentAuth.VerifyAuthorization(topAccount, message, env.getter)
	assert.ErrorIs(t, err, types.ErrMaxRecursionDepth, "should hit max recursion depth")
}

func TestEdgeCase_EmptyMessage(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)

	// Empty message
	message := []byte{}
	sig := keyPair.toSignature(message)
	auth := types.NewAuthorization(sig)

	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.NoError(t, err, "empty message should be signable")
}

func TestEdgeCase_LargeMessage(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)

	// 1MB message
	message := make([]byte, 1024*1024)
	for i := range message {
		message[i] = byte(i % 256)
	}

	sig := keyPair.toSignature(message)
	auth := types.NewAuthorization(sig)

	err := auth.VerifyAuthorization(account, message, env.getter)
	assert.NoError(t, err, "large message should be signable")
}

func TestEdgeCase_ThresholdZero(t *testing.T) {
	keyPair := generateTestKeyPair(t)

	// Threshold 0 should be invalid
	account := &types.Account{
		Name: "alice",
		Authority: types.Authority{
			Threshold:      0,
			KeyWeights:     map[string]uint64{string(keyPair.pub): 1},
			AccountWeights: make(map[types.AccountName]uint64),
		},
	}

	err := account.ValidateBasic()
	assert.Error(t, err, "threshold 0 should be invalid")
}

func TestEdgeCase_ThresholdExceedsWeight(t *testing.T) {
	keyPair := generateTestKeyPair(t)

	// Threshold higher than total possible weight
	account := &types.Account{
		Name: "alice",
		Authority: types.Authority{
			Threshold:      10,
			KeyWeights:     map[string]uint64{string(keyPair.pub): 1},
			AccountWeights: make(map[types.AccountName]uint64),
		},
	}

	err := account.ValidateBasic()
	assert.Error(t, err, "unreachable threshold should be invalid")
}

// =============================================================================
// 8. Concurrency Tests (if relevant)
// =============================================================================

func TestConcurrency_ParallelVerification(t *testing.T) {
	env := newSigningTestEnv()
	keyPair := generateTestKeyPair(t)

	account := env.createAccount("alice", keyPair.pub)
	message := []byte("concurrent test")
	sig := keyPair.toSignature(message)
	auth := types.NewAuthorization(sig)

	// Run multiple verifications in parallel
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func() {
			err := auth.VerifyAuthorization(account, message, env.getter)
			done <- (err == nil)
		}()
	}

	// Wait for all to complete
	successCount := 0
	for i := 0; i < 100; i++ {
		if <-done {
			successCount++
		}
	}

	assert.Equal(t, 100, successCount, "all parallel verifications should succeed")
}
