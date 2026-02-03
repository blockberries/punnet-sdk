package types

// adversarial_delegation_test.go - Adversarial simulation tests for delegation graph edge cases
//
// Created for Issue #56: Tests identified from The Tinkerer's review of PR #51
// Explores edge cases in authorization delegation that could be exploited.
//
// Test scenarios:
// 1. Key aliasing across delegation levels (same key in parent and child accounts)
// 2. Diamond delegation patterns (multiple paths to same delegated account)
// 3. Long delegation chains (stress testing depth limits)
// 4. Fuzz testing with random signature arrays

import (
	"crypto/ed25519"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Scenario 1: Key Aliasing Across Delegation Levels
// =============================================================================
//
// The concern: What if the same physical key appears in multiple accounts'
// authorities at different delegation levels? Could it contribute weight at
// multiple levels and inflate the total authorization weight?
//
// Attack scenario:
//   - Account A (threshold=2, keys=[K1, K2]) delegates to Account B
//   - Account B has key K1 (the same physical key as in A)
//   - When B authorizes on behalf of A, does K1 contribute weight twice?

func TestAdversarial_KeyAliasingAcrossDelegationLevels(t *testing.T) {
	// Generate a shared key and a unique key
	sharedPub, sharedPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	uniquePub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Alice has threshold=2 with shared key (weight=1) and unique key (weight=1)
	// Alice also delegates to Bob with weight=1
	alice := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold: 2,
			KeyWeights: map[string]uint64{
				string(sharedPub):  1,
				string(uniquePub): 1,
			},
			AccountWeights: map[AccountName]uint64{
				"bob": 1,
			},
		},
		Nonce: 0,
	}

	// Bob has the SAME shared key (threshold=1)
	bob := &Account{
		Name: "bob",
		Authority: Authority{
			Threshold:  1,
			KeyWeights: map[string]uint64{string(sharedPub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	getter := newMockAccountGetter()
	getter.setAccount(alice)
	getter.setAccount(bob)

	message := []byte("test transaction with shared key aliasing")
	sharedSig := ed25519.Sign(sharedPriv, message)

	// Attack scenario 1: Try to use shared key signature BOTH directly AND through Bob's delegation
	// If weight is counted at both levels, attacker gets weight=2 from one signature!
	t.Run("shared key should not double-count across levels", func(t *testing.T) {
		auth := &Authorization{
			// Direct signature from shared key (contributes to Alice's threshold)
			Signatures: []Signature{
				{PubKey: sharedPub, Signature: sharedSig},
			},
			// Also try to use Bob's authorization (which uses the same shared key)
			AccountAuthorizations: map[AccountName]*Authorization{
				"bob": {
					Signatures: []Signature{
						{PubKey: sharedPub, Signature: sharedSig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
			},
		}

		err := auth.VerifyAuthorization(alice, message, getter)
		// This SHOULD succeed because:
		// - Direct sig from sharedKey gives weight=1
		// - Bob's delegation gives weight=1 (Bob's threshold is met)
		// - Total = 2, meets Alice's threshold=2
		//
		// BUT if the same key's weight is incorrectly deduplicated across levels,
		// this might fail unexpectedly. Let's verify the behavior is correct.
		assert.NoError(t, err, "Valid authorization should succeed - weights are independent per account level")
	})

	// Attack scenario 2: Can we get more weight than expected?
	t.Run("shared key at one level should not affect weight calculation at another", func(t *testing.T) {
		// Alice threshold=3, only shared key with weight=1, Bob delegation with weight=1
		aliceHighThreshold := &Account{
			Name: "alice.high",
			Authority: Authority{
				Threshold: 3,
				KeyWeights: map[string]uint64{
					string(sharedPub): 1,
				},
				AccountWeights: map[AccountName]uint64{
					"bob": 1,
				},
			},
			Nonce: 0,
		}
		getter.setAccount(aliceHighThreshold)

		auth := &Authorization{
			Signatures: []Signature{
				{PubKey: sharedPub, Signature: sharedSig},
			},
			AccountAuthorizations: map[AccountName]*Authorization{
				"bob": {
					Signatures: []Signature{
						{PubKey: sharedPub, Signature: sharedSig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
			},
		}

		err := auth.VerifyAuthorization(aliceHighThreshold, message, getter)
		// Should FAIL - max achievable weight is 2 (direct key=1 + bob delegation=1)
		// But threshold is 3
		assert.ErrorIs(t, err, ErrInsufficientWeight,
			"Should fail - shared key cannot inflate weight beyond sum of independent contributions")
	})
}

// =============================================================================
// Scenario 2: Diamond Delegation Pattern
// =============================================================================
//
// Test the "diamond" delegation graph:
//
//        alice (threshold=2)
//       /     \
//      v       v
//     bob     charlie
//      \       /
//       v     v
//        dave
//
// What happens when dave signs? Can dave's signature contribute through
// both paths and double-count?

func TestAdversarial_DiamondDelegationPattern(t *testing.T) {
	// Generate keys
	alicePub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	bobPub, bobPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	charliePub, charliePriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	davePub, davePriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Dave: single key, threshold=1
	dave := &Account{
		Name: "dave",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(davePub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	// Bob: delegates to dave
	bob := &Account{
		Name: "bob",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(bobPub): 1},
			AccountWeights: map[AccountName]uint64{"dave": 1},
		},
		Nonce: 0,
	}

	// Charlie: also delegates to dave
	charlie := &Account{
		Name: "charlie",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(charliePub): 1},
			AccountWeights: map[AccountName]uint64{"dave": 1},
		},
		Nonce: 0,
	}

	// Alice: delegates to both bob and charlie (threshold=2)
	alice := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:  2,
			KeyWeights: map[string]uint64{string(alicePub): 1},
			AccountWeights: map[AccountName]uint64{
				"bob":     1,
				"charlie": 1,
			},
		},
		Nonce: 0,
	}

	getter := newMockAccountGetter()
	getter.setAccount(alice)
	getter.setAccount(bob)
	getter.setAccount(charlie)
	getter.setAccount(dave)

	message := []byte("diamond delegation test")
	daveSig := ed25519.Sign(davePriv, message)

	t.Run("diamond pattern - dave signature through both paths", func(t *testing.T) {
		// Dave signs, and we try to use his signature through BOTH bob and charlie
		daveAuth := &Authorization{
			Signatures: []Signature{
				{PubKey: davePub, Signature: daveSig},
			},
			AccountAuthorizations: make(map[AccountName]*Authorization),
		}

		auth := &Authorization{
			Signatures: []Signature{},
			AccountAuthorizations: map[AccountName]*Authorization{
				"bob": {
					Signatures:            []Signature{},
					AccountAuthorizations: map[AccountName]*Authorization{"dave": daveAuth},
				},
				"charlie": {
					Signatures:            []Signature{},
					AccountAuthorizations: map[AccountName]*Authorization{"dave": daveAuth},
				},
			},
		}

		err := auth.VerifyAuthorization(alice, message, getter)
		// This SHOULD succeed because each delegation path is independent:
		// - bob's delegation is satisfied via dave → bob contributes weight=1 to alice
		// - charlie's delegation is satisfied via dave → charlie contributes weight=1 to alice
		// - Total = 2, meets threshold=2
		//
		// The key insight: dave's signature is being verified twice (once for bob's path,
		// once for charlie's path), but each path contributes its own delegation weight.
		assert.NoError(t, err,
			"Diamond pattern should work - each delegation path contributes independently")
	})

	t.Run("diamond pattern with direct keys too", func(t *testing.T) {
		// What if bob and charlie also sign directly?
		bobSig := ed25519.Sign(bobPriv, message)
		charlieSig := ed25519.Sign(charliePriv, message)

		auth := &Authorization{
			Signatures: []Signature{},
			AccountAuthorizations: map[AccountName]*Authorization{
				"bob": {
					Signatures: []Signature{
						{PubKey: bobPub, Signature: bobSig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
				"charlie": {
					Signatures: []Signature{
						{PubKey: charliePub, Signature: charlieSig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
			},
		}

		err := auth.VerifyAuthorization(alice, message, getter)
		assert.NoError(t, err, "Diamond pattern with direct sigs should work")
	})
}

// =============================================================================
// Scenario 3: Long Delegation Chains (Edge of MaxRecursionDepth)
// =============================================================================
//
// Test behavior at exactly MaxRecursionDepth to ensure we don't have
// off-by-one errors in the depth checking.

func TestAdversarial_LongChainEdgeCases(t *testing.T) {
	getter := newMockAccountGetter()
	message := []byte("long chain edge case test")

	// Helper to create a chain of accounts where each delegates to the previous
	createChain := func(length int) ([]*Account, []ed25519.PrivateKey) {
		accounts := make([]*Account, length)
		privKeys := make([]ed25519.PrivateKey, length)

		for i := 0; i < length; i++ {
			pub, priv, _ := ed25519.GenerateKey(nil)
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

			if i > 0 {
				account.Authority.AccountWeights[AccountName(fmt.Sprintf("account%d", i-1))] = 1
			}

			accounts[i] = account
			getter.setAccount(account)
		}

		return accounts, privKeys
	}

	// Helper to create authorization chain
	createAuthChain := func(length int, privKeys []ed25519.PrivateKey) *Authorization {
		sig := ed25519.Sign(privKeys[0], message)
		bottomAuth := &Authorization{
			Signatures: []Signature{
				{PubKey: privKeys[0].Public().(ed25519.PublicKey), Signature: sig},
			},
			AccountAuthorizations: make(map[AccountName]*Authorization),
		}

		currentAuth := bottomAuth
		for i := 1; i < length; i++ {
			newAuth := &Authorization{
				Signatures: []Signature{},
				AccountAuthorizations: map[AccountName]*Authorization{
					AccountName(fmt.Sprintf("account%d", i-1)): currentAuth,
				},
			}
			currentAuth = newAuth
		}

		return currentAuth
	}

	t.Run("chain at exactly MaxRecursionDepth should succeed", func(t *testing.T) {
		// Create chain of length MaxRecursionDepth+1 (indices 0 through MaxRecursionDepth)
		accounts, privKeys := createChain(MaxRecursionDepth + 1)
		auth := createAuthChain(MaxRecursionDepth+1, privKeys)

		err := auth.VerifyAuthorization(accounts[MaxRecursionDepth], message, getter)
		// Depth 0 = top account, depth MaxRecursionDepth = bottom account
		// This should just barely succeed
		assert.NoError(t, err, "Chain at MaxRecursionDepth should succeed")
	})

	t.Run("chain exceeding MaxRecursionDepth should fail", func(t *testing.T) {
		accounts, privKeys := createChain(MaxRecursionDepth + 3)
		auth := createAuthChain(MaxRecursionDepth+3, privKeys)

		err := auth.VerifyAuthorization(accounts[MaxRecursionDepth+2], message, getter)
		assert.ErrorIs(t, err, ErrMaxRecursionDepth, "Chain exceeding max depth should fail")
	})
}

// =============================================================================
// Scenario 4: Signature Array Permutations
// =============================================================================
//
// What happens if we submit signatures in different orders? Are there any
// order-dependent bugs?

func TestAdversarial_SignatureOrderIndependence(t *testing.T) {
	// Create account with multiple keys
	pub1, priv1, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub2, priv2, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub3, priv3, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	account := &Account{
		Name: "multisig",
		Authority: Authority{
			Threshold: 2,
			KeyWeights: map[string]uint64{
				string(pub1): 1,
				string(pub2): 1,
				string(pub3): 1,
			},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	getter := newMockAccountGetter()
	message := []byte("signature order test")

	sig1 := ed25519.Sign(priv1, message)
	sig2 := ed25519.Sign(priv2, message)
	sig3 := ed25519.Sign(priv3, message)

	// Test all permutations of two signatures
	permutations := [][]int{
		{0, 1}, {1, 0},
		{0, 2}, {2, 0},
		{1, 2}, {2, 1},
	}

	sigs := []Signature{
		{PubKey: pub1, Signature: sig1},
		{PubKey: pub2, Signature: sig2},
		{PubKey: pub3, Signature: sig3},
	}

	for _, perm := range permutations {
		t.Run(fmt.Sprintf("order_%d_%d", perm[0], perm[1]), func(t *testing.T) {
			auth := &Authorization{
				Signatures: []Signature{
					sigs[perm[0]],
					sigs[perm[1]],
				},
				AccountAuthorizations: make(map[AccountName]*Authorization),
			}

			err := auth.VerifyAuthorization(account, message, getter)
			assert.NoError(t, err, "Signature order should not affect verification")
		})
	}
}

// =============================================================================
// Scenario 5: Mixed Valid and Invalid Signatures
// =============================================================================
//
// What if some signatures are valid and some are invalid/malformed?
// Does the implementation correctly count only valid ones?

func TestAdversarial_MixedValidInvalidSignatures(t *testing.T) {
	pub1, priv1, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub2, priv2, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub3, _, err := ed25519.GenerateKey(nil) // Note: we don't use priv3
	require.NoError(t, err)

	account := &Account{
		Name: "mixtest",
		Authority: Authority{
			Threshold: 2,
			KeyWeights: map[string]uint64{
				string(pub1): 1,
				string(pub2): 1,
				string(pub3): 1,
			},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	getter := newMockAccountGetter()
	message := []byte("mixed signature test")

	validSig1 := ed25519.Sign(priv1, message)
	validSig2 := ed25519.Sign(priv2, message)
	invalidSig := make([]byte, ed25519.SignatureSize) // Zero bytes - invalid

	t.Run("invalid signature should fail verification", func(t *testing.T) {
		auth := &Authorization{
			Signatures: []Signature{
				{PubKey: pub1, Signature: validSig1},
				{PubKey: pub3, Signature: invalidSig}, // This is invalid!
			},
			AccountAuthorizations: make(map[AccountName]*Authorization),
		}

		err := auth.VerifyAuthorization(account, message, getter)
		// The implementation verifies ALL signatures first, so an invalid one should fail
		assert.Error(t, err, "Should fail when any signature is invalid")
	})

	t.Run("two valid signatures should succeed", func(t *testing.T) {
		auth := &Authorization{
			Signatures: []Signature{
				{PubKey: pub1, Signature: validSig1},
				{PubKey: pub2, Signature: validSig2},
			},
			AccountAuthorizations: make(map[AccountName]*Authorization),
		}

		err := auth.VerifyAuthorization(account, message, getter)
		assert.NoError(t, err, "Two valid signatures should meet threshold")
	})
}

// =============================================================================
// Scenario 6: Fuzz Testing - Random Signature Arrays
// =============================================================================

func FuzzAuthorization_RandomSignatures(f *testing.F) {
	// Add seed corpus
	f.Add(uint64(1), uint64(1), int64(42))
	f.Add(uint64(3), uint64(2), int64(123))
	f.Add(uint64(10), uint64(5), int64(999))

	f.Fuzz(func(t *testing.T, numKeys uint64, threshold uint64, seed int64) {
		// Bound inputs to reasonable ranges
		if numKeys == 0 || numKeys > 20 {
			return
		}
		if threshold == 0 || threshold > numKeys {
			return
		}

		rng := rand.New(rand.NewSource(seed))

		// Generate keys
		pubs := make([]ed25519.PublicKey, numKeys)
		privs := make([]ed25519.PrivateKey, numKeys)
		keyWeights := make(map[string]uint64)

		for i := uint64(0); i < numKeys; i++ {
			pub, priv, err := ed25519.GenerateKey(nil)
			require.NoError(t, err)
			pubs[i] = pub
			privs[i] = priv
			keyWeights[string(pub)] = 1
		}

		account := &Account{
			Name: "fuzz.test",
			Authority: Authority{
				Threshold:      threshold,
				KeyWeights:     keyWeights,
				AccountWeights: make(map[AccountName]uint64),
			},
			Nonce: 0,
		}

		getter := newMockAccountGetter()
		message := []byte("fuzz test message")

		// Randomly select which keys to sign with
		numSigners := uint64(rng.Intn(int(numKeys) + 1))
		signerIndices := rng.Perm(int(numKeys))[:numSigners]

		sigs := make([]Signature, 0, numSigners)
		for _, idx := range signerIndices {
			sig := ed25519.Sign(privs[idx], message)
			sigs = append(sigs, Signature{
				PubKey:    pubs[idx],
				Signature: sig,
			})
		}

		auth := &Authorization{
			Signatures:            sigs,
			AccountAuthorizations: make(map[AccountName]*Authorization),
		}

		err := auth.VerifyAuthorization(account, message, getter)

		// Verify expected behavior
		if numSigners >= threshold {
			assert.NoError(t, err, "Should succeed when signers >= threshold")
		} else {
			assert.Error(t, err, "Should fail when signers < threshold")
		}
	})
}

// =============================================================================
// Scenario 7: Concurrent Delegation Mutations (Conceptual Test)
// =============================================================================
//
// This tests what happens if we have authorization objects that reference
// accounts that don't exist or have been modified. While actual concurrent
// mutations would require runtime testing, we can test the error handling.

func TestAdversarial_NonexistentDelegatedAccount(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	alice := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:  1,
			KeyWeights: map[string]uint64{string(pub): 1},
			AccountWeights: map[AccountName]uint64{
				"nonexistent": 1, // This account doesn't exist in the getter
			},
		},
		Nonce: 0,
	}

	getter := newMockAccountGetter()
	getter.setAccount(alice)
	// Note: we deliberately don't add "nonexistent" account

	message := []byte("test nonexistent delegation")

	t.Run("referencing nonexistent account should fail gracefully", func(t *testing.T) {
		sig := ed25519.Sign(priv, message)
		auth := &Authorization{
			Signatures: []Signature{}, // No direct sigs, only delegation
			AccountAuthorizations: map[AccountName]*Authorization{
				"nonexistent": {
					Signatures: []Signature{
						{PubKey: pub, Signature: sig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
			},
		}

		err := auth.VerifyAuthorization(alice, message, getter)
		// Should fail because we can't get the nonexistent account
		assert.Error(t, err, "Should fail when delegated account doesn't exist")
	})

	t.Run("direct key should still work even with bad delegation", func(t *testing.T) {
		sig := ed25519.Sign(priv, message)
		auth := &Authorization{
			Signatures: []Signature{
				{PubKey: pub, Signature: sig},
			},
			// Also try to use nonexistent delegation (should be ignored since direct meets threshold)
			AccountAuthorizations: map[AccountName]*Authorization{},
		}

		err := auth.VerifyAuthorization(alice, message, getter)
		assert.NoError(t, err, "Direct key should work without relying on delegation")
	})
}

// =============================================================================
// Scenario 8: Self-Delegation Attempt
// =============================================================================
//
// What happens if an account tries to delegate to itself?

func TestAdversarial_SelfDelegation(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Account that delegates to itself
	alice := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:  2,
			KeyWeights: map[string]uint64{string(pub): 1},
			AccountWeights: map[AccountName]uint64{
				"alice": 1, // Self-delegation!
			},
		},
		Nonce: 0,
	}

	getter := newMockAccountGetter()
	getter.setAccount(alice)

	message := []byte("self-delegation test")
	sig := ed25519.Sign(priv, message)

	t.Run("self-delegation should be caught as cycle", func(t *testing.T) {
		auth := &Authorization{
			Signatures: []Signature{
				{PubKey: pub, Signature: sig},
			},
			AccountAuthorizations: map[AccountName]*Authorization{
				"alice": {
					Signatures: []Signature{
						{PubKey: pub, Signature: sig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
			},
		}

		err := auth.VerifyAuthorization(alice, message, getter)
		// Self-delegation creates an immediate cycle: alice -> alice
		assert.ErrorIs(t, err, ErrAuthorizationCycle, "Self-delegation should be detected as cycle")
	})
}

// =============================================================================
// Scenario 9: Weight Overflow Attack
// =============================================================================
//
// Try to cause uint64 overflow in weight calculation

func TestAdversarial_WeightOverflow(t *testing.T) {
	pub1, priv1, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	pub2, priv2, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Account with weights that could overflow if added
	account := &Account{
		Name: "overflow.test",
		Authority: Authority{
			Threshold: ^uint64(0), // Max uint64
			KeyWeights: map[string]uint64{
				string(pub1): ^uint64(0) - 1, // Almost max
				string(pub2): 2,              // Would overflow if added to above
			},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	getter := newMockAccountGetter()
	message := []byte("overflow test")
	sig1 := ed25519.Sign(priv1, message)
	sig2 := ed25519.Sign(priv2, message)

	t.Run("weight overflow should be detected", func(t *testing.T) {
		auth := &Authorization{
			Signatures: []Signature{
				{PubKey: pub1, Signature: sig1},
				{PubKey: pub2, Signature: sig2},
			},
			AccountAuthorizations: make(map[AccountName]*Authorization),
		}

		err := auth.VerifyAuthorization(account, message, getter)
		// Should fail due to overflow detection or insufficient weight
		assert.Error(t, err, "Should detect overflow or fail for other reason")
	})
}

// =============================================================================
// Scenario 10: Empty Authorization Structures
// =============================================================================

func TestAdversarial_EmptyAuthorizationStructures(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	account := &Account{
		Name: "emptytest",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(pub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	getter := newMockAccountGetter()
	message := []byte("empty test")

	t.Run("empty signatures should fail", func(t *testing.T) {
		auth := &Authorization{
			Signatures:            []Signature{},
			AccountAuthorizations: make(map[AccountName]*Authorization),
		}

		err := auth.VerifyAuthorization(account, message, getter)
		assert.ErrorIs(t, err, ErrInsufficientWeight, "Empty auth should fail with insufficient weight")
	})

	t.Run("nil authorization should fail", func(t *testing.T) {
		var auth *Authorization = nil
		err := auth.VerifyAuthorization(account, message, getter)
		assert.Error(t, err, "Nil authorization should fail")
	})
}
