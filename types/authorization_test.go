package types

import (
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAccountGetter implements AccountGetter for testing
type mockAccountGetter struct {
	accounts map[AccountName]*Account
}

func newMockAccountGetter() *mockAccountGetter {
	return &mockAccountGetter{
		accounts: make(map[AccountName]*Account),
	}
}

func (m *mockAccountGetter) GetAccount(name AccountName) (*Account, error) {
	acc, ok := m.accounts[name]
	if !ok {
		return nil, ErrNotFound
	}
	return acc, nil
}

func (m *mockAccountGetter) setAccount(acc *Account) {
	m.accounts[acc.Name] = acc
}

func TestSignature_ValidateBasic(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	tests := []struct {
		name      string
		sig       Signature
		expectErr bool
	}{
		{
			"valid signature",
			Signature{
				PubKey:    pub,
				Signature: make([]byte, ed25519.SignatureSize),
			},
			false,
		},
		{
			"invalid pubkey length",
			Signature{
				PubKey:    make([]byte, 16),
				Signature: make([]byte, ed25519.SignatureSize),
			},
			true,
		},
		{
			"invalid signature length",
			Signature{
				PubKey:    pub,
				Signature: make([]byte, 32),
			},
			true,
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

func TestSignature_Verify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	message := []byte("test message")
	sig := ed25519.Sign(priv, message)

	signature := Signature{
		PubKey:    pub,
		Signature: sig,
	}

	assert.True(t, signature.Verify(message))
	assert.False(t, signature.Verify([]byte("wrong message")))
}

func TestAuthorization_ValidateBasic(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	tests := []struct {
		name      string
		auth      *Authorization
		expectErr bool
	}{
		{
			"valid authorization",
			&Authorization{
				Signatures: []Signature{
					{
						PubKey:    pub,
						Signature: make([]byte, ed25519.SignatureSize),
					},
				},
				AccountAuthorizations: make(map[AccountName]*Authorization),
			},
			false,
		},
		{
			"invalid signature",
			&Authorization{
				Signatures: []Signature{
					{
						PubKey:    make([]byte, 16),
						Signature: make([]byte, ed25519.SignatureSize),
					},
				},
				AccountAuthorizations: make(map[AccountName]*Authorization),
			},
			true,
		},
		{
			"nil account authorization",
			&Authorization{
				Signatures: []Signature{},
				AccountAuthorizations: map[AccountName]*Authorization{
					"alice": nil,
				},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.ValidateBasic()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAuthorization_SimpleVerification(t *testing.T) {
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
		Nonce: 0,
	}

	// Create message and sign it
	message := []byte("test transaction")
	sig := ed25519.Sign(priv, message)

	// Create authorization
	auth := &Authorization{
		Signatures: []Signature{
			{
				PubKey:    pub,
				Signature: sig,
			},
		},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	// Verify authorization
	getter := newMockAccountGetter()
	err = auth.VerifyAuthorization(account, message, getter)
	assert.NoError(t, err)
}

func TestAuthorization_InsufficientWeight(t *testing.T) {
	// Generate key pair
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create account with threshold > key weight
	account := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      2,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	// Create message and sign it
	message := []byte("test transaction")
	sig := ed25519.Sign(priv, message)

	// Create authorization with only one signature
	auth := &Authorization{
		Signatures: []Signature{
			{
				PubKey:    pub,
				Signature: sig,
			},
		},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	// Verify authorization should fail
	getter := newMockAccountGetter()
	err = auth.VerifyAuthorization(account, message, getter)
	assert.ErrorIs(t, err, ErrInsufficientWeight)
}

func TestAuthorization_DelegatedAuthorization(t *testing.T) {
	// Generate keys for alice and bob
	alicePub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	bobPub, bobPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create bob's account
	bob := &Account{
		Name: "bob",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(bobPub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	// Create alice's account with delegation to bob
	alice := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:  1,
			KeyWeights: map[string]uint64{string(alicePub): 1},
			AccountWeights: map[AccountName]uint64{
				"bob": 1,
			},
		},
		Nonce: 0,
	}

	// Create message
	message := []byte("test transaction")

	// Alice uses bob's authorization (delegation)
	bobSig := ed25519.Sign(bobPriv, message)
	bobAuth := &Authorization{
		Signatures: []Signature{
			{
				PubKey:    bobPub,
				Signature: bobSig,
			},
		},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	aliceAuth := &Authorization{
		Signatures: []Signature{},
		AccountAuthorizations: map[AccountName]*Authorization{
			"bob": bobAuth,
		},
	}

	// Setup getter with both accounts
	getter := newMockAccountGetter()
	getter.setAccount(alice)
	getter.setAccount(bob)

	// Verify alice's authorization using bob's signature
	err = aliceAuth.VerifyAuthorization(alice, message, getter)
	assert.NoError(t, err)
}

func TestAuthorization_CycleDetection(t *testing.T) {
	// Generate keys
	pub1, priv1, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	pub2, priv2, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Create accounts with circular delegation: alice -> bob -> alice
	alice := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:  1,
			KeyWeights: map[string]uint64{string(pub1): 1},
			AccountWeights: map[AccountName]uint64{
				"bob": 1,
			},
		},
		Nonce: 0,
	}

	bob := &Account{
		Name: "bob",
		Authority: Authority{
			Threshold:  1,
			KeyWeights: map[string]uint64{string(pub2): 1},
			AccountWeights: map[AccountName]uint64{
				"alice": 1, // Creates cycle
			},
		},
		Nonce: 0,
	}

	// Create message
	message := []byte("test transaction")

	// Create authorization chain that forms a cycle
	sig1 := ed25519.Sign(priv1, message)
	sig2 := ed25519.Sign(priv2, message)

	aliceAuth := &Authorization{
		Signatures: []Signature{
			{
				PubKey:    pub1,
				Signature: sig1,
			},
		},
		AccountAuthorizations: map[AccountName]*Authorization{
			"bob": {
				Signatures: []Signature{
					{
						PubKey:    pub2,
						Signature: sig2,
					},
				},
				AccountAuthorizations: map[AccountName]*Authorization{
					"alice": nil, // Will be set to create cycle
				},
			},
		},
	}

	// Complete the cycle
	aliceAuth.AccountAuthorizations["bob"].AccountAuthorizations["alice"] = aliceAuth

	// Setup getter
	getter := newMockAccountGetter()
	getter.setAccount(alice)
	getter.setAccount(bob)

	// Verify should detect cycle
	err = aliceAuth.VerifyAuthorization(alice, message, getter)
	assert.ErrorIs(t, err, ErrAuthorizationCycle)
}

func TestAuthorization_MaxRecursionDepth(t *testing.T) {
	// Create a long chain of delegations exceeding max depth
	getter := newMockAccountGetter()

	// Create accounts in a chain: account0 -> account1 -> account2 -> ... -> account(MaxRecursionDepth+1)
	accounts := make([]*Account, MaxRecursionDepth+3)
	keys := make([]ed25519.PublicKey, MaxRecursionDepth+3)
	privKeys := make([]ed25519.PrivateKey, MaxRecursionDepth+3)

	for i := 0; i < MaxRecursionDepth+3; i++ {
		pub, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		keys[i] = pub
		privKeys[i] = priv

		acctName := AccountName(fmt.Sprintf("account%d", i))
		account := &Account{
			Name: acctName,
			Authority: Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{string(pub): 1},
				AccountWeights: make(map[AccountName]uint64),
			},
			Nonce: 0,
		}

		// Each account delegates to the previous one (except the last)
		if i > 0 {
			account.Authority.AccountWeights[AccountName(fmt.Sprintf("account%d", i-1))] = 1
		}

		accounts[i] = account
		getter.setAccount(account)
	}

	// Create authorization chain that goes deep
	message := []byte("test transaction")

	// Build authorization recursively from the end
	// The deepest account authorizes using the signature from account0
	sig0 := ed25519.Sign(privKeys[0], message)
	bottomAuth := &Authorization{
		Signatures: []Signature{
			{
				PubKey:    keys[0],
				Signature: sig0,
			},
		},
		AccountAuthorizations: make(map[AccountName]*Authorization),
	}

	// Build chain of delegated authorizations
	currentAuth := bottomAuth
	for i := 1; i < MaxRecursionDepth+3; i++ {
		newAuth := &Authorization{
			Signatures: []Signature{},
			AccountAuthorizations: map[AccountName]*Authorization{
				AccountName(fmt.Sprintf("account%d", i-1)): currentAuth,
			},
		}
		currentAuth = newAuth
	}

	// Try to verify authorization for the deepest account
	// This should fail because the delegation chain is too deep
	deepestAccount := accounts[MaxRecursionDepth+2]
	err := currentAuth.VerifyAuthorization(deepestAccount, message, getter)
	assert.ErrorIs(t, err, ErrMaxRecursionDepth)
}
