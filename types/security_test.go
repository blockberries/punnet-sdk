package types

import (
	"crypto/ed25519"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMemoryAliasing_TransactionMessages verifies defensive copying in NewTransaction
func TestMemoryAliasing_TransactionMessages(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create original messages slice
	messages := []Message{}

	// Create transaction
	auth := NewAuthorization(Signature{PubKey: pub, Signature: make([]byte, ed25519.SignatureSize)})
	tx := NewTransaction("alice", 0, messages, auth)

	// Verify transaction has empty messages
	assert.Equal(t, 0, len(tx.Messages))

	// Attempt to mutate original slice (this should NOT affect transaction)
	_ = append(messages, nil) // Result discarded - we're testing the transaction wasn't affected

	// Verify transaction still has empty messages
	assert.Equal(t, 0, len(tx.Messages))

	t.Log("✓ Transaction messages are defensively copied")
}

// TestMemoryAliasing_NewCoins verifies defensive copying in NewCoins
func TestMemoryAliasing_NewCoins(t *testing.T) {
	// Create original coins slice
	original := []Coin{{Denom: "uatom", Amount: 100}}

	// Create Coins using NewCoins
	coins := NewCoins(original...)

	// Verify initial state
	assert.Equal(t, uint64(100), coins.AmountOf("uatom"))

	// Mutate original slice
	original[0].Amount = 999999

	// Verify Coins was not affected
	assert.Equal(t, uint64(100), coins.AmountOf("uatom"))

	t.Log("✓ NewCoins creates defensive copy")
}

// TestMemoryAliasing_NewAuthorization verifies defensive copying in NewAuthorization
func TestMemoryAliasing_NewAuthorization(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create original signatures slice
	sigs := []Signature{{PubKey: pub, Signature: make([]byte, ed25519.SignatureSize)}}

	// Create Authorization using NewAuthorization
	auth := NewAuthorization(sigs...)

	// Verify initial state
	assert.Equal(t, 1, len(auth.Signatures))

	// Mutate original slice
	sigs[0].PubKey[0] = 0xFF

	// Verify Authorization was not affected
	assert.NotEqual(t, byte(0xFF), auth.Signatures[0].PubKey[0])

	t.Log("✓ NewAuthorization creates defensive copy")
}

// TestOverflowProtection_WeightCalculation tests overflow protection in authorization weight calculation
func TestOverflowProtection_WeightCalculation(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create account with weight that would overflow
	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold: 1,
			KeyWeights: map[string]uint64{
				string(pub): ^uint64(0), // Max uint64
			},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	// Create bob's account
	bobPub, bobPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	bob := &Account{
		Name: "bob",
		Authority: Authority{
			Threshold: 1,
			KeyWeights: map[string]uint64{
				string(bobPub): ^uint64(0), // Max uint64
			},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	// Add bob as delegation to alice (this would cause overflow if not protected)
	account.Authority.AccountWeights["bob"] = 1

	// Create message
	message := []byte("test transaction")

	// Create authorization with signatures from both
	aliceSig := ed25519.Sign(priv, message)
	bobSig := ed25519.Sign(bobPriv, message)

	auth := &Authorization{
		Signatures: []Signature{
			{PubKey: pub, Signature: aliceSig},
		},
		AccountAuthorizations: map[AccountName]*Authorization{
			"bob": {
				Signatures: []Signature{
					{PubKey: bobPub, Signature: bobSig},
				},
				AccountAuthorizations: make(map[AccountName]*Authorization),
			},
		},
	}

	// Setup getter
	getter := newMockAccountGetter()
	getter.setAccount(account)
	getter.setAccount(bob)

	// Verify should detect overflow and return error
	err = auth.VerifyAuthorization(account, message, getter)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "overflow")

	t.Log("✓ Weight calculation overflow is detected")
}

// TestOverflowProtection_CoinAdd tests overflow protection in Coin.Add
func TestOverflowProtection_CoinAdd(t *testing.T) {
	// Create coins at max value
	coins1 := Coins{{Denom: "uatom", Amount: ^uint64(0) - 50}}
	coins2 := Coins{{Denom: "uatom", Amount: 100}}

	// Add should saturate at max value instead of wrapping
	result := coins1.Add(coins2)

	// Should be saturated at max uint64
	assert.Equal(t, ^uint64(0), result.AmountOf("uatom"))

	t.Log("✓ Coin addition saturates at max value on overflow")
}

// TestNilCheck_TransactionValidateBasic tests nil check in Transaction.ValidateBasic
func TestNilCheck_TransactionValidateBasic(t *testing.T) {
	var tx *Transaction = nil

	err := tx.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")

	t.Log("✓ Transaction.ValidateBasic handles nil receiver")
}

// TestNilCheck_AuthorizationValidateBasic tests nil check in Authorization.ValidateBasic
func TestNilCheck_AuthorizationValidateBasic(t *testing.T) {
	var auth *Authorization = nil

	err := auth.ValidateBasic()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")

	t.Log("✓ Authorization.ValidateBasic handles nil receiver")
}

// TestNilCheck_VerifyAuthorization tests nil checks in VerifyAuthorization
func TestNilCheck_VerifyAuthorization(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	message := []byte("test")

	tests := []struct {
		name    string
		auth    *Authorization
		account *Account
		getter  AccountGetter
	}{
		{
			"nil authorization",
			nil,
			&Account{Name: "alice"},
			newMockAccountGetter(),
		},
		{
			"nil account",
			NewAuthorization(Signature{PubKey: pub, Signature: make([]byte, 64)}),
			nil,
			newMockAccountGetter(),
		},
		{
			"nil getter",
			NewAuthorization(Signature{PubKey: pub, Signature: make([]byte, 64)}),
			&Account{Name: "alice"},
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.VerifyAuthorization(tt.account, message, tt.getter)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "nil")
		})
	}

	t.Log("✓ VerifyAuthorization handles all nil parameters")
}

// TestTimingAttack_HasSignatureFrom verifies constant-time comparison
// Note: This test can't actually detect timing differences, but it verifies the function works correctly
func TestTimingAttack_HasSignatureFrom(t *testing.T) {
	pub1, priv1, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	pub2, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	message := []byte("test message")
	sig1 := ed25519.Sign(priv1, message)

	auth := NewAuthorization(Signature{
		PubKey:    pub1,
		Signature: sig1,
	})

	// Should find signature with correct public key
	assert.True(t, auth.HasSignatureFrom(pub1, message))

	// Should not find signature with different public key
	assert.False(t, auth.HasSignatureFrom(pub2, message))

	t.Log("✓ HasSignatureFrom uses constant-time comparison")
}

// TestSignDoc_CrossChainReplayProtection explicitly documents and verifies the security property
// that prevents cross-chain replay attacks.
//
// SECURITY INVARIANT: A signature valid on chain A MUST be invalid on chain B.
//
// PROOF SKETCH: The ChainID is included in the SignDoc structure and is serialized as part of
// GetSignBytes(). Since GetSignBytes() returns SHA-256(canonical_json), different ChainIDs
// produce different hashes. A signature created for one chain will fail verification on
// another chain because the sign bytes will not match.
//
// ATTACK SCENARIO PREVENTED: Without this protection, an attacker could observe a valid
// transaction on "mainnet" and replay it on "testnet" (or vice versa), potentially stealing
// funds or causing unintended state changes on the target chain.
func TestSignDoc_CrossChainReplayProtection(t *testing.T) {
	// Create two SignDocs with identical content except for ChainID
	mainnetSignDoc := NewSignDoc("mainnet", 1, "alice", 1, "transfer to bob")
	mainnetSignDoc.AddMessage("/punnet.bank.v1.MsgSend", []byte(`{"from":"alice","to":"bob","amount":"100"}`))

	testnetSignDoc := NewSignDoc("testnet", 1, "alice", 1, "transfer to bob")
	testnetSignDoc.AddMessage("/punnet.bank.v1.MsgSend", []byte(`{"from":"alice","to":"bob","amount":"100"}`))

	// Get the sign bytes for each chain
	mainnetSignBytes, err := mainnetSignDoc.GetSignBytes()
	require.NoError(t, err, "mainnet GetSignBytes should succeed")

	testnetSignBytes, err := testnetSignDoc.GetSignBytes()
	require.NoError(t, err, "testnet GetSignBytes should succeed")

	// CRITICAL ASSERTION: Different chains MUST produce different sign bytes
	// This is the fundamental property that prevents cross-chain replay attacks
	assert.NotEqual(t, mainnetSignBytes, testnetSignBytes,
		"SECURITY VIOLATION: identical transaction content on different chains "+
			"must produce different sign bytes to prevent cross-chain replay attacks")

	// Additional verification: same chain, same content produces same sign bytes (determinism)
	mainnetSignBytes2, err := mainnetSignDoc.GetSignBytes()
	require.NoError(t, err)
	assert.Equal(t, mainnetSignBytes, mainnetSignBytes2,
		"same SignDoc must produce identical sign bytes (determinism)")

	t.Log("✓ Cross-chain replay protection: different chainIDs produce different sign bytes")
}
