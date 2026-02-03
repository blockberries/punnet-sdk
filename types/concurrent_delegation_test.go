package types

// concurrent_delegation_test.go - Tests for concurrent delegation mutation during authorization
//
// Created for Issue #74: Tests behavior when delegation relationships change during verification.
// This addresses a concern raised in Issue #56 comments by The Tinkerer.
//
// Key question: What happens if delegation relationships change while authorization is being verified?
//
// Test scenarios:
// 1. Delegation addition during verification - new delegation added mid-verification
// 2. Delegation removal during verification - existing delegation removed mid-verification
// 3. Documentation of observed behavior (snapshot vs live semantics)
//
// EXPECTED BEHAVIOR:
// The current implementation uses "live semantics" - each call to AccountGetter.GetAccount()
// fetches the current state. This means:
// - If a delegation is added after verification starts, it MAY be visible depending on timing
// - If a delegation is removed after verification starts, it MAY cause verification to fail
//
// SECURITY IMPLICATIONS:
// - Live semantics could allow TOCTOU (time-of-check-time-of-use) attacks
// - An attacker who can modify delegations concurrently could potentially:
//   1. Add a temporary delegation, get it verified, then remove it
//   2. Remove a delegation mid-verification to cause inconsistent state
// - For production use, consider snapshot semantics at the storage layer

import (
	"crypto/ed25519"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Concurrent Mock AccountGetter
// =============================================================================

// concurrentMockAccountGetter extends mockAccountGetter with thread-safe operations
// and the ability to mutate state during verification callbacks.
type concurrentMockAccountGetter struct {
	mu       sync.RWMutex
	accounts map[AccountName]*Account

	// Callback hooks for mutation during verification
	// These are called AFTER GetAccount returns, simulating concurrent modification
	onGetAccount func(name AccountName)

	// Counters for tracking access patterns
	getAccountCalls atomic.Int64
}

func newConcurrentMockAccountGetter() *concurrentMockAccountGetter {
	return &concurrentMockAccountGetter{
		accounts: make(map[AccountName]*Account),
	}
}

func (m *concurrentMockAccountGetter) GetAccount(name AccountName) (*Account, error) {
	m.getAccountCalls.Add(1)

	// Read and copy while holding the lock to prevent races
	m.mu.RLock()
	acc, ok := m.accounts[name]
	var accCopy *Account
	if ok {
		accCopy = m.copyAccountUnsafe(acc)
	}
	m.mu.RUnlock()

	if !ok {
		return nil, ErrNotFound
	}

	// Call hook after returning account (simulates concurrent modification)
	if m.onGetAccount != nil {
		m.onGetAccount(name)
	}

	// Return the copy made while holding the lock
	return accCopy, nil
}

func (m *concurrentMockAccountGetter) setAccount(acc *Account) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accounts[acc.Name] = m.copyAccountUnsafe(acc)
}

func (m *concurrentMockAccountGetter) deleteAccount(name AccountName) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.accounts, name)
}

func (m *concurrentMockAccountGetter) updateAccount(name AccountName, updater func(*Account)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if acc, ok := m.accounts[name]; ok {
		// Create a copy, apply the update, and store the copy
		// This avoids races when GetAccount returns a copy that's being read
		// while we're updating the stored account
		accCopy := m.copyAccountUnsafe(acc)
		updater(accCopy)
		m.accounts[name] = accCopy
	}
}

// copyAccountUnsafe creates a deep copy without acquiring locks.
// MUST be called while holding m.mu lock.
func (m *concurrentMockAccountGetter) copyAccountUnsafe(acc *Account) *Account {
	if acc == nil {
		return nil
	}

	// Deep copy the account
	accCopy := &Account{
		Name:  acc.Name,
		Nonce: acc.Nonce,
		Authority: Authority{
			Threshold:      acc.Authority.Threshold,
			KeyWeights:     make(map[string]uint64),
			AccountWeights: make(map[AccountName]uint64),
		},
	}

	for k, v := range acc.Authority.KeyWeights {
		accCopy.Authority.KeyWeights[k] = v
	}
	for k, v := range acc.Authority.AccountWeights {
		accCopy.Authority.AccountWeights[k] = v
	}

	return accCopy
}

// copyAccount creates a deep copy with proper locking.
func (m *concurrentMockAccountGetter) copyAccount(acc *Account) *Account {
	// Note: This is called on an already-copied account from GetAccount,
	// so we don't need the lock here. The account passed in is already
	// a safe copy that's not in the shared map.
	return m.copyAccountUnsafe(acc)
}

// =============================================================================
// Scenario 1: Delegation Addition During Verification
// =============================================================================
//
// Test what happens when a new delegation is added while verification is in progress.
// This simulates a race condition where:
// 1. Verification starts for account A
// 2. Account A has delegation to B, which in turn delegates to C
// 3. While verifying B's authorization, a new delegation D is added to A
// 4. Question: Does D's authorization get considered?

func TestConcurrent_DelegationAdditionDuringVerification(t *testing.T) {
	// Generate keys
	alicePub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	bobPub, bobPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	charliePub, charliePriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	getter := newConcurrentMockAccountGetter()

	// Charlie: simple account
	charlie := &Account{
		Name: "charlie",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(charliePub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	// Bob: delegates to charlie
	bob := &Account{
		Name: "bob",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(bobPub): 1},
			AccountWeights: map[AccountName]uint64{"charlie": 1},
		},
		Nonce: 0,
	}

	// Alice: threshold=2, has her own key (weight=1) and delegates to bob (weight=1)
	// She does NOT yet delegate to charlie directly
	alice := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      2,
			KeyWeights:     map[string]uint64{string(alicePub): 1},
			AccountWeights: map[AccountName]uint64{"bob": 1},
		},
		Nonce: 0,
	}

	getter.setAccount(alice)
	getter.setAccount(bob)
	getter.setAccount(charlie)

	message := []byte("test concurrent delegation addition")
	bobSig := ed25519.Sign(bobPriv, message)
	charlieSig := ed25519.Sign(charliePriv, message)

	t.Run("delegation added mid-verification uses live semantics", func(t *testing.T) {
		// This test documents that the current implementation uses LIVE semantics,
		// meaning changes to the getter during verification CAN affect the result.
		//
		// IMPORTANT: This is documenting current behavior, not necessarily desired behavior.
		// For production, snapshot semantics might be preferable for security.

		delegationAdded := atomic.Bool{}

		// Set up callback to add charlie delegation when bob is accessed
		getter.onGetAccount = func(name AccountName) {
			if name == "bob" && !delegationAdded.Load() {
				delegationAdded.Store(true)
				// Add charlie as direct delegation to alice
				getter.updateAccount("alice", func(acc *Account) {
					acc.Authority.AccountWeights["charlie"] = 1
				})
			}
		}

		// Authorization that uses both bob (through delegation) and charlie (direct)
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

		// Get fresh copy of alice for verification
		// Note: The authorization references charlie, but alice's initial state
		// does NOT include charlie as a delegate. The delegation is added during verification.
		aliceForVerify, _ := getter.GetAccount("alice")

		// Reset state for clean test
		getter.updateAccount("alice", func(acc *Account) {
			delete(acc.Authority.AccountWeights, "charlie")
		})
		delegationAdded.Store(false)

		// With LIVE semantics, this COULD succeed if:
		// 1. Bob's authorization is verified first (triggering the callback)
		// 2. Charlie delegation is added to alice
		// 3. Charlie's authorization is then verified against the updated alice
		//
		// However, the verification uses the alice account passed to VerifyAuthorization,
		// not a fresh fetch. So this should FAIL because alice's authority at call time
		// doesn't include charlie.

		err := auth.VerifyAuthorization(aliceForVerify, message, getter)

		// DOCUMENTED BEHAVIOR: The implementation uses the passed-in account's Authority
		// for weight calculation, not a fresh fetch. So mid-verification changes to the
		// account in the getter do NOT affect the outcome for the top-level account.
		//
		// However, for DELEGATED accounts (like bob), the getter IS consulted.
		assert.Error(t, err, "Should fail - charlie delegation not in alice's authority at verification start")

		// Clean up callback
		getter.onGetAccount = nil
	})

	t.Run("documents snapshot semantics for top-level account", func(t *testing.T) {
		// SECURITY PROPERTY: The top-level account's authority is snapshot at call time.
		// Changes to the account in the getter during verification do NOT affect
		// which delegations are considered valid for the top-level account.

		// Reset alice to initial state
		getter.updateAccount("alice", func(acc *Account) {
			delete(acc.Authority.AccountWeights, "charlie")
		})

		// Get alice BEFORE any modifications
		aliceSnapshot, _ := getter.GetAccount("alice")
		assert.NotContains(t, aliceSnapshot.Authority.AccountWeights, AccountName("charlie"),
			"Alice should not have charlie delegation initially")

		// Now add charlie delegation in the getter
		getter.updateAccount("alice", func(acc *Account) {
			acc.Authority.AccountWeights["charlie"] = 1
		})

		// Verification using the SNAPSHOT should still fail
		auth := &Authorization{
			Signatures: []Signature{},
			AccountAuthorizations: map[AccountName]*Authorization{
				"charlie": {
					Signatures: []Signature{
						{PubKey: charliePub, Signature: charlieSig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
			},
		}

		err := auth.VerifyAuthorization(aliceSnapshot, message, getter)
		assert.Error(t, err, "Should fail - using snapshot without charlie delegation")
	})
}

// =============================================================================
// Scenario 2: Delegation Removal During Verification
// =============================================================================
//
// Test what happens when an existing delegation is removed while verification is in progress.
// This is potentially more dangerous as it could cause inconsistent state.

func TestConcurrent_DelegationRemovalDuringVerification(t *testing.T) {
	// Generate keys
	alicePub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	bobPub, _, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	charliePub, charliePriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	getter := newConcurrentMockAccountGetter()

	// Charlie: simple account
	charlie := &Account{
		Name: "charlie",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(charliePub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	// Bob: delegates to charlie
	bob := &Account{
		Name: "bob",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(bobPub): 1},
			AccountWeights: map[AccountName]uint64{"charlie": 1},
		},
		Nonce: 0,
	}

	// Alice: threshold=1, delegates to bob
	alice := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(alicePub): 1},
			AccountWeights: map[AccountName]uint64{"bob": 1},
		},
		Nonce: 0,
	}

	getter.setAccount(alice)
	getter.setAccount(bob)
	getter.setAccount(charlie)

	message := []byte("test concurrent delegation removal")
	charlieSig := ed25519.Sign(charliePriv, message)

	t.Run("delegation removal from delegated account mid-verification", func(t *testing.T) {
		// Scenario: Alice delegates to Bob, Bob delegates to Charlie.
		// Authorization uses Charlie's signature through Bob.
		// During verification of Bob, Charlie's delegation is removed from Bob.

		// Track if removal happened
		removalDone := atomic.Bool{}

		// Remove charlie's delegation from bob when bob is accessed
		getter.onGetAccount = func(name AccountName) {
			if name == "bob" && !removalDone.Load() {
				removalDone.Store(true)
				getter.updateAccount("bob", func(acc *Account) {
					delete(acc.Authority.AccountWeights, "charlie")
				})
			}
		}

		auth := &Authorization{
			Signatures: []Signature{},
			AccountAuthorizations: map[AccountName]*Authorization{
				"bob": {
					Signatures: []Signature{},
					AccountAuthorizations: map[AccountName]*Authorization{
						"charlie": {
							Signatures: []Signature{
								{PubKey: charliePub, Signature: charlieSig},
							},
							AccountAuthorizations: make(map[AccountName]*Authorization),
						},
					},
				},
			},
		}

		// Get fresh alice (not affected by removal)
		aliceForVerify, _ := getter.GetAccount("alice")

		// Reset bob's state for clean test
		getter.updateAccount("bob", func(acc *Account) {
			acc.Authority.AccountWeights["charlie"] = 1
		})
		removalDone.Store(false)

		// The verification will:
		// 1. Process alice's delegations
		// 2. Get bob from getter (triggers removal of charlie delegation)
		// 3. Try to verify charlie's authorization for bob
		// 4. But bob's authority (from getter) no longer includes charlie!

		err := auth.VerifyAuthorization(aliceForVerify, message, getter)

		// DOCUMENTED BEHAVIOR: For delegated accounts, the getter IS consulted
		// to get the current state. So if charlie's delegation is removed from bob
		// BEFORE bob's authority is checked, the verification fails.
		//
		// NOTE: The exact behavior depends on timing. If the implementation caches
		// the account state, behavior may differ.
		//
		// SECURITY IMPLICATION: This is a TOCTOU risk. An attacker who can
		// modify delegations concurrently could cause verification to fail
		// even for legitimate authorizations.

		// The test verifies the implementation handles this gracefully
		// (either succeeds or fails cleanly, no panics or inconsistent state)
		if err != nil {
			t.Logf("Verification failed (expected with live semantics): %v", err)
		} else {
			t.Log("Verification succeeded (delegation check happened before removal)")
		}

		// Clean up
		getter.onGetAccount = nil
	})

	t.Run("delegated account completely removed mid-verification", func(t *testing.T) {
		// More extreme case: the delegated account is completely deleted
		removalDone := atomic.Bool{}

		// Delete charlie account when bob is accessed
		getter.onGetAccount = func(name AccountName) {
			if name == "bob" && !removalDone.Load() {
				removalDone.Store(true)
				getter.deleteAccount("charlie")
			}
		}

		// Reset state
		getter.setAccount(charlie) // Re-add charlie
		getter.updateAccount("bob", func(acc *Account) {
			acc.Authority.AccountWeights["charlie"] = 1
		})
		removalDone.Store(false)

		auth := &Authorization{
			Signatures: []Signature{},
			AccountAuthorizations: map[AccountName]*Authorization{
				"bob": {
					Signatures: []Signature{},
					AccountAuthorizations: map[AccountName]*Authorization{
						"charlie": {
							Signatures: []Signature{
								{PubKey: charliePub, Signature: charlieSig},
							},
							AccountAuthorizations: make(map[AccountName]*Authorization),
						},
					},
				},
			},
		}

		aliceForVerify, _ := getter.GetAccount("alice")

		err := auth.VerifyAuthorization(aliceForVerify, message, getter)

		// Should handle gracefully - either succeed or fail with appropriate error
		if err != nil {
			// Check we get a sensible error, not a panic
			t.Logf("Verification failed as expected when account deleted: %v", err)
		} else {
			t.Log("Verification succeeded - account lookup happened before deletion")
		}

		// Clean up
		getter.onGetAccount = nil
		getter.setAccount(charlie) // Restore for other tests
	})
}

// =============================================================================
// Scenario 3: Parallel Verification Stress Test
// =============================================================================
//
// Run verification concurrently while mutations happen.
// This tests for race conditions in the verification logic itself.

func TestConcurrent_ParallelVerificationWithMutations(t *testing.T) {
	// Generate keys
	alicePub, alicePriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	bobPub, bobPriv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	getter := newConcurrentMockAccountGetter()

	bob := &Account{
		Name: "bob",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(bobPub): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce: 0,
	}

	alice := &Account{
		Name: "alice",
		Authority: Authority{
			Threshold:      1,
			KeyWeights:     map[string]uint64{string(alicePub): 1},
			AccountWeights: map[AccountName]uint64{"bob": 1},
		},
		Nonce: 0,
	}

	getter.setAccount(alice)
	getter.setAccount(bob)

	message := []byte("parallel verification test")
	aliceSig := ed25519.Sign(alicePriv, message)
	bobSig := ed25519.Sign(bobPriv, message)

	t.Run("parallel verifications with concurrent mutations", func(t *testing.T) {
		const numGoroutines = 10
		const iterationsPerGoroutine = 100

		var wg sync.WaitGroup
		wg.Add(numGoroutines * 2) // verification goroutines + mutation goroutines

		// Track results
		var successCount, failCount atomic.Int64
		var panicCount atomic.Int64

		// Start verification goroutines
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						panicCount.Add(1)
						t.Errorf("PANIC during verification: %v", r)
					}
				}()

				for j := 0; j < iterationsPerGoroutine; j++ {
					// Alternate between direct auth and delegated auth
					var auth *Authorization
					if j%2 == 0 {
						auth = &Authorization{
							Signatures: []Signature{
								{PubKey: alicePub, Signature: aliceSig},
							},
							AccountAuthorizations: make(map[AccountName]*Authorization),
						}
					} else {
						auth = &Authorization{
							Signatures: []Signature{},
							AccountAuthorizations: map[AccountName]*Authorization{
								"bob": {
									Signatures: []Signature{
										{PubKey: bobPub, Signature: bobSig},
									},
									AccountAuthorizations: make(map[AccountName]*Authorization),
								},
							},
						}
					}

					// Get fresh account reference
					aliceRef, _ := getter.GetAccount("alice")
					if aliceRef == nil {
						continue
					}

					err := auth.VerifyAuthorization(aliceRef, message, getter)
					if err == nil {
						successCount.Add(1)
					} else {
						failCount.Add(1)
					}
				}
			}()
		}

		// Start mutation goroutines
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()

				for j := 0; j < iterationsPerGoroutine; j++ {
					// Toggle bob's delegation weight
					getter.updateAccount("alice", func(acc *Account) {
						if j%2 == 0 {
							acc.Authority.AccountWeights["bob"] = 1
						} else {
							acc.Authority.AccountWeights["bob"] = 0
						}
					})
				}
			}()
		}

		wg.Wait()

		// Report results
		t.Logf("Parallel test results: successes=%d, failures=%d, panics=%d",
			successCount.Load(), failCount.Load(), panicCount.Load())

		// Key assertions
		assert.Zero(t, panicCount.Load(), "No panics should occur during concurrent operations")
		assert.Greater(t, successCount.Load()+failCount.Load(), int64(0),
			"Some verifications should have completed")
	})
}

// =============================================================================
// Scenario 4: Semantic Documentation Tests
// =============================================================================
//
// These tests explicitly document the semantics of the current implementation.

func TestConcurrent_DocumentedSemantics(t *testing.T) {
	t.Run("top-level account uses passed-in authority (snapshot semantics)", func(t *testing.T) {
		// DOCUMENTED BEHAVIOR:
		// When VerifyAuthorization is called, the passed-in account's Authority is used
		// for the top-level weight calculation. Changes to the account in the getter
		// do NOT affect the top-level verification.
		//
		// This provides snapshot semantics for the root account.

		pub, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		getter := newConcurrentMockAccountGetter()

		alice := &Account{
			Name: "alice",
			Authority: Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{string(pub): 1},
				AccountWeights: make(map[AccountName]uint64),
			},
			Nonce: 0,
		}

		getter.setAccount(alice)

		// Take a snapshot
		aliceSnapshot, _ := getter.GetAccount("alice")

		// Modify the account in getter (increase threshold)
		getter.updateAccount("alice", func(acc *Account) {
			acc.Authority.Threshold = 10 // Now impossible to satisfy
		})

		message := []byte("test")
		sig := ed25519.Sign(priv, message)

		auth := &Authorization{
			Signatures: []Signature{
				{PubKey: pub, Signature: sig},
			},
			AccountAuthorizations: make(map[AccountName]*Authorization),
		}

		// Using snapshot - should succeed with original threshold=1
		err = auth.VerifyAuthorization(aliceSnapshot, message, getter)
		assert.NoError(t, err, "Snapshot semantics: original threshold should be used")

		// Using fresh fetch - would fail with threshold=10
		aliceFresh, _ := getter.GetAccount("alice")
		err = auth.VerifyAuthorization(aliceFresh, message, getter)
		assert.Error(t, err, "Fresh fetch should see new threshold=10")
	})

	t.Run("delegated accounts use live semantics from getter", func(t *testing.T) {
		// DOCUMENTED BEHAVIOR:
		// When verifying delegated account authorizations, the implementation
		// calls getter.GetAccount() to fetch the delegated account's current state.
		// This is "live semantics" - changes to delegated accounts ARE visible.
		//
		// SECURITY IMPLICATION: This could allow TOCTOU attacks where:
		// 1. Attacker sets up temporary delegation
		// 2. Starts verification
		// 3. Removes delegation after check but before weight calculation
		// 4. Or vice versa

		alicePub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		bobPub, bobPriv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		getter := newConcurrentMockAccountGetter()

		bob := &Account{
			Name: "bob",
			Authority: Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{string(bobPub): 1},
				AccountWeights: make(map[AccountName]uint64),
			},
			Nonce: 0,
		}

		alice := &Account{
			Name: "alice",
			Authority: Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{string(alicePub): 1},
				AccountWeights: map[AccountName]uint64{"bob": 1},
			},
			Nonce: 0,
		}

		getter.setAccount(alice)
		getter.setAccount(bob)

		message := []byte("test")
		bobSig := ed25519.Sign(bobPriv, message)

		auth := &Authorization{
			Signatures: []Signature{},
			AccountAuthorizations: map[AccountName]*Authorization{
				"bob": {
					Signatures: []Signature{
						{PubKey: bobPub, Signature: bobSig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
			},
		}

		// Take snapshot of alice
		aliceSnapshot, _ := getter.GetAccount("alice")

		// Modify bob's threshold to be impossible to satisfy
		getter.updateAccount("bob", func(acc *Account) {
			acc.Authority.Threshold = 10
		})

		// Even with alice snapshot, bob's authority is fetched fresh
		err = auth.VerifyAuthorization(aliceSnapshot, message, getter)
		assert.Error(t, err, "Live semantics: bob's new threshold=10 should be used")
	})
}

// =============================================================================
// Scenario 5: Race Detector Validation
// =============================================================================
//
// These tests are specifically designed to trigger race conditions if any exist.
// Run with: go test -race ./types -run TestConcurrent

// =============================================================================
// Scenario 5: TOCTOU Exploit Test
// =============================================================================
//
// This test explicitly attempts to exploit the TOCTOU gap documented above.
// It's designed to demonstrate whether the current implementation is
// "accidentally safe" or actually vulnerable to concurrent manipulation.
//
// Suggested by The Tinkerer in PR review.

func TestConcurrent_TOCTOUExploitAttempt(t *testing.T) {
	// SCENARIO: Attacker attempts to exploit TOCTOU window
	//
	// Setup:
	// - Alice has threshold=2
	// - Alice delegates to Bob (weight=1)
	// - Bob has threshold=1 and a valid key
	// - Attacker has Bob's signature
	//
	// Attack attempt:
	// 1. Verification starts
	// 2. Bob is fetched via getter (triggers callback)
	// 3. Callback adds a new "malicious" delegation to Alice with weight=1
	// 4. If successful, attacker achieves threshold=2 with only Bob's weight=1
	//    plus the injected malicious delegation weight=1
	//
	// EXPECTED RESULT: Attack FAILS because top-level account uses snapshot semantics

	t.Run("TOCTOU exploit attempt via mid-verification delegation injection", func(t *testing.T) {
		bobPub, bobPriv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		maliciousPub, maliciousPriv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		getter := newConcurrentMockAccountGetter()

		// Malicious account controlled by attacker
		malicious := &Account{
			Name: "malicious",
			Authority: Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{string(maliciousPub): 1},
				AccountWeights: make(map[AccountName]uint64),
			},
			Nonce: 0,
		}

		// Bob: legitimate delegate
		bob := &Account{
			Name: "bob",
			Authority: Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{string(bobPub): 1},
				AccountWeights: make(map[AccountName]uint64),
			},
			Nonce: 0,
		}

		// Alice: threshold=2, only delegates to Bob (weight=1)
		// Attacker cannot satisfy threshold with Bob alone
		alice := &Account{
			Name: "alice",
			Authority: Authority{
				Threshold:      2, // Requires 2, but only Bob (weight=1) is delegated
				KeyWeights:     make(map[string]uint64),
				AccountWeights: map[AccountName]uint64{"bob": 1},
			},
			Nonce: 0,
		}

		getter.setAccount(alice)
		getter.setAccount(bob)
		getter.setAccount(malicious)

		message := []byte("TOCTOU exploit test")
		bobSig := ed25519.Sign(bobPriv, message)
		maliciousSig := ed25519.Sign(maliciousPriv, message)

		// Track if injection happened
		injectionDone := atomic.Bool{}

		// ATTACK: When bob is accessed, inject malicious delegation into alice
		getter.onGetAccount = func(name AccountName) {
			if name == "bob" && !injectionDone.Load() {
				injectionDone.Store(true)
				// Inject malicious delegation with weight=1
				getter.updateAccount("alice", func(acc *Account) {
					acc.Authority.AccountWeights["malicious"] = 1
				})
			}
		}

		// Authorization includes both Bob and Malicious
		// If TOCTOU exploit works, both would contribute weight
		auth := &Authorization{
			Signatures: []Signature{},
			AccountAuthorizations: map[AccountName]*Authorization{
				"bob": {
					Signatures: []Signature{
						{PubKey: bobPub, Signature: bobSig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
				"malicious": {
					Signatures: []Signature{
						{PubKey: maliciousPub, Signature: maliciousSig},
					},
					AccountAuthorizations: make(map[AccountName]*Authorization),
				},
			},
		}

		// Get alice BEFORE injection (this is the snapshot used for verification)
		aliceSnapshot, _ := getter.GetAccount("alice")
		injectionDone.Store(false) // Reset for test

		err = auth.VerifyAuthorization(aliceSnapshot, message, getter)

		// SECURITY PROPERTY: Attack should FAIL
		// The top-level account (alice) uses snapshot semantics.
		// The malicious delegation injected during verification is NOT
		// visible to the weight calculation for the top-level account.
		assert.Error(t, err, "TOCTOU exploit should FAIL - top-level uses snapshot semantics")

		if err != nil {
			t.Logf("Attack correctly blocked: %v", err)
		} else {
			t.Error("SECURITY VULNERABILITY: TOCTOU exploit succeeded! Top-level snapshot semantics may be broken.")
		}

		// Verify injection did happen (to confirm the test setup worked)
		assert.True(t, injectionDone.Load(), "Injection callback should have been triggered")

		// Clean up
		getter.onGetAccount = nil
	})

	t.Run("TOCTOU exploit via delegated account modification", func(t *testing.T) {
		// SCENARIO: More subtle attack targeting delegated accounts
		//
		// Setup:
		// - Alice delegates to Bob
		// - Bob delegates to Charlie (threshold=2, needs 2 keys)
		// - Charlie has 1 key with weight=1 (insufficient)
		// - Attacker has Charlie's signature
		//
		// Attack:
		// 1. During verification, when Bob's delegations are checked
		// 2. Attacker modifies Charlie's threshold from 2 to 1
		// 3. Now Charlie's single signature satisfies the lowered threshold
		//
		// EXPECTED: Attack MAY succeed due to live semantics for delegated accounts

		charliePub, charliePriv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		bobPub, _, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)

		getter := newConcurrentMockAccountGetter()

		// Charlie: initially threshold=2, has only 1 key with weight=1
		// Cannot satisfy own threshold legitimately
		charlie := &Account{
			Name: "charlie",
			Authority: Authority{
				Threshold:      2, // Impossible to satisfy with single weight=1 key
				KeyWeights:     map[string]uint64{string(charliePub): 1},
				AccountWeights: make(map[AccountName]uint64),
			},
			Nonce: 0,
		}

		// Bob: delegates to Charlie
		bob := &Account{
			Name: "bob",
			Authority: Authority{
				Threshold:      1,
				KeyWeights:     map[string]uint64{string(bobPub): 1},
				AccountWeights: map[AccountName]uint64{"charlie": 1},
			},
			Nonce: 0,
		}

		// Alice: delegates to Bob
		alice := &Account{
			Name: "alice",
			Authority: Authority{
				Threshold:      1,
				KeyWeights:     make(map[string]uint64),
				AccountWeights: map[AccountName]uint64{"bob": 1},
			},
			Nonce: 0,
		}

		getter.setAccount(alice)
		getter.setAccount(bob)
		getter.setAccount(charlie)

		message := []byte("delegated TOCTOU test")
		charlieSig := ed25519.Sign(charliePriv, message)

		// Track modification
		modificationDone := atomic.Bool{}

		// ATTACK: When bob is accessed, lower charlie's threshold
		getter.onGetAccount = func(name AccountName) {
			if name == "bob" && !modificationDone.Load() {
				modificationDone.Store(true)
				// Lower Charlie's threshold to make it satisfiable
				getter.updateAccount("charlie", func(acc *Account) {
					acc.Authority.Threshold = 1
				})
			}
		}

		auth := &Authorization{
			Signatures: []Signature{},
			AccountAuthorizations: map[AccountName]*Authorization{
				"bob": {
					Signatures: []Signature{},
					AccountAuthorizations: map[AccountName]*Authorization{
						"charlie": {
							Signatures: []Signature{
								{PubKey: charliePub, Signature: charlieSig},
							},
							AccountAuthorizations: make(map[AccountName]*Authorization),
						},
					},
				},
			},
		}

		aliceSnapshot, _ := getter.GetAccount("alice")
		modificationDone.Store(false)

		err = auth.VerifyAuthorization(aliceSnapshot, message, getter)

		// DOCUMENTED BEHAVIOR: This attack MAY succeed because delegated accounts
		// use live semantics. The result depends on ordering:
		// - If charlie's threshold is lowered BEFORE charlie is fetched: SUCCESS
		// - If charlie is fetched BEFORE threshold is lowered: FAIL
		//
		// In this test, we lower threshold when BOB is accessed, which happens
		// before charlie is fetched, so the attack should succeed.

		if err == nil {
			t.Log("KNOWN VULNERABILITY: Delegated account TOCTOU exploit succeeded.")
			t.Log("This is expected with live semantics for delegated accounts.")
			t.Log("Production systems should implement snapshot semantics at storage layer.")
		} else {
			t.Logf("Attack blocked (timing-dependent): %v", err)
		}

		// This isn't an assertion failure - we're documenting behavior
		assert.True(t, modificationDone.Load(), "Modification callback should have been triggered")

		getter.onGetAccount = nil
	})
}

func TestConcurrent_RaceDetectorValidation(t *testing.T) {
	// Generate multiple key pairs
	keys := make([]struct {
		pub  ed25519.PublicKey
		priv ed25519.PrivateKey
	}, 5)

	for i := range keys {
		pub, priv, err := ed25519.GenerateKey(nil)
		require.NoError(t, err)
		keys[i].pub = pub
		keys[i].priv = priv
	}

	t.Run("concurrent reads and writes to mock getter", func(t *testing.T) {
		getter := newConcurrentMockAccountGetter()

		// Create initial accounts
		for i := 0; i < 5; i++ {
			acc := &Account{
				Name: AccountName("account" + string(rune('0'+i))),
				Authority: Authority{
					Threshold:      1,
					KeyWeights:     map[string]uint64{string(keys[i].pub): 1},
					AccountWeights: make(map[AccountName]uint64),
				},
				Nonce: 0,
			}
			getter.setAccount(acc)
		}

		message := []byte("race test")

		var wg sync.WaitGroup
		wg.Add(20)

		// Readers
		for i := 0; i < 10; i++ {
			go func(idx int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					name := AccountName("account" + string(rune('0'+idx%5)))
					acc, _ := getter.GetAccount(name)
					if acc != nil {
						// Do verification
						sig := ed25519.Sign(keys[idx%5].priv, message)
						auth := &Authorization{
							Signatures: []Signature{
								{PubKey: keys[idx%5].pub, Signature: sig},
							},
							AccountAuthorizations: make(map[AccountName]*Authorization),
						}
						_ = auth.VerifyAuthorization(acc, message, getter)
					}
				}
			}(i)
		}

		// Writers
		for i := 0; i < 10; i++ {
			go func(idx int) {
				defer wg.Done()
				for j := 0; j < 100; j++ {
					name := AccountName("account" + string(rune('0'+idx%5)))
					getter.updateAccount(name, func(acc *Account) {
						acc.Nonce = uint64(j)
					})
				}
			}(i)
		}

		wg.Wait()
		// If we get here without -race complaining, we're good
		t.Log("Race detector validation passed")
	})
}
