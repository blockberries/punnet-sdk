package capability

import (
	"context"
	"crypto/ed25519"
	"sync"
	"testing"

	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

func setupAccountCapability(t *testing.T) AccountCapability {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	err := cm.RegisterModule("auth")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	cap, err := cm.GrantAccountCapability("auth")
	if err != nil {
		t.Fatalf("failed to grant account capability: %v", err)
	}

	return cap
}

func TestAccountCapability_ModuleName(t *testing.T) {
	cap := setupAccountCapability(t)

	if cap.ModuleName() != "auth" {
		t.Fatalf("expected module name 'auth', got %s", cap.ModuleName())
	}
}

func TestAccountCapability_ModuleName_Nil(t *testing.T) {
	var cap *accountCapability
	if cap.ModuleName() != "" {
		t.Fatal("expected empty module name for nil capability")
	}
}

func TestAccountCapability_CreateAccount(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-pubkey-123456789012345678901234")
	account, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	if account == nil {
		t.Fatal("expected non-nil account")
	}

	if account.Name != "alice" {
		t.Fatalf("expected account name 'alice', got %s", account.Name)
	}

	if account.Nonce != 0 {
		t.Fatalf("expected nonce 0, got %d", account.Nonce)
	}
}

func TestAccountCapability_CreateAccount_AlreadyExists(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-pubkey-123456789012345678901234")
	_, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	_, err = cap.CreateAccount(ctx, "alice", pubKey)
	if err == nil {
		t.Fatal("expected error when creating duplicate account")
	}
}

func TestAccountCapability_CreateAccount_InvalidName(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-pubkey-123456789012345678901234")
	_, err := cap.CreateAccount(ctx, "INVALID", pubKey)
	if err == nil {
		t.Fatal("expected error with invalid account name")
	}
}

func TestAccountCapability_CreateAccount_EmptyPubKey(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	_, err := cap.CreateAccount(ctx, "alice", []byte{})
	if err == nil {
		t.Fatal("expected error with empty public key")
	}
}

func TestAccountCapability_CreateAccount_Nil(t *testing.T) {
	var cap *accountCapability
	ctx := context.Background()

	pubKey := []byte("test-pubkey-123456789012345678901234")
	_, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestAccountCapability_GetAccount(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-pubkey-123456789012345678901234")
	created, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	retrieved, err := cap.GetAccount(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to get account: %v", err)
	}

	if retrieved.Name != created.Name {
		t.Fatalf("expected account name %s, got %s", created.Name, retrieved.Name)
	}
}

func TestAccountCapability_GetAccount_NotFound(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	_, err := cap.GetAccount(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent account")
	}
}

func TestAccountCapability_GetAccount_InvalidName(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	_, err := cap.GetAccount(ctx, "INVALID")
	if err == nil {
		t.Fatal("expected error with invalid account name")
	}
}

func TestAccountCapability_GetAccount_Nil(t *testing.T) {
	var cap *accountCapability
	ctx := context.Background()

	_, err := cap.GetAccount(ctx, "alice")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestAccountCapability_UpdateAccount(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-pubkey-123456789012345678901234")
	account, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	account.Nonce = 42
	err = cap.UpdateAccount(ctx, account)
	if err != nil {
		t.Fatalf("failed to update account: %v", err)
	}

	retrieved, err := cap.GetAccount(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to get account: %v", err)
	}

	if retrieved.Nonce != 42 {
		t.Fatalf("expected nonce 42, got %d", retrieved.Nonce)
	}
}

func TestAccountCapability_UpdateAccount_NotFound(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	account := types.NewAccount("nonexistent", []byte("test-pubkey-123456789012345678901234"))
	err := cap.UpdateAccount(ctx, account)
	if err == nil {
		t.Fatal("expected error when updating nonexistent account")
	}
}

func TestAccountCapability_UpdateAccount_NilAccount(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	err := cap.UpdateAccount(ctx, nil)
	if err == nil {
		t.Fatal("expected error with nil account")
	}
}

func TestAccountCapability_UpdateAccount_Nil(t *testing.T) {
	var cap *accountCapability
	ctx := context.Background()

	account := types.NewAccount("alice", []byte("test-pubkey-123456789012345678901234"))
	err := cap.UpdateAccount(ctx, account)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestAccountCapability_DeleteAccount(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-pubkey-123456789012345678901234")
	_, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	err = cap.DeleteAccount(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to delete account: %v", err)
	}

	has, err := cap.HasAccount(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to check account: %v", err)
	}

	if has {
		t.Fatal("expected account to be deleted")
	}
}

func TestAccountCapability_DeleteAccount_InvalidName(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	err := cap.DeleteAccount(ctx, "INVALID")
	if err == nil {
		t.Fatal("expected error with invalid account name")
	}
}

func TestAccountCapability_DeleteAccount_Nil(t *testing.T) {
	var cap *accountCapability
	ctx := context.Background()

	err := cap.DeleteAccount(ctx, "alice")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestAccountCapability_HasAccount(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	has, err := cap.HasAccount(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to check account: %v", err)
	}

	if has {
		t.Fatal("expected account to not exist")
	}

	pubKey := []byte("test-pubkey-123456789012345678901234")
	_, err = cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	has, err = cap.HasAccount(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to check account: %v", err)
	}

	if !has {
		t.Fatal("expected account to exist")
	}
}

func TestAccountCapability_HasAccount_InvalidName(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	_, err := cap.HasAccount(ctx, "INVALID")
	if err == nil {
		t.Fatal("expected error with invalid account name")
	}
}

func TestAccountCapability_HasAccount_Nil(t *testing.T) {
	var cap *accountCapability
	ctx := context.Background()

	_, err := cap.HasAccount(ctx, "alice")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestAccountCapability_IncrementNonce(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-pubkey-123456789012345678901234")
	_, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	err = cap.IncrementNonce(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to increment nonce: %v", err)
	}

	nonce, err := cap.GetNonce(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to get nonce: %v", err)
	}

	if nonce != 1 {
		t.Fatalf("expected nonce 1, got %d", nonce)
	}
}

func TestAccountCapability_IncrementNonce_NotFound(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	err := cap.IncrementNonce(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent account")
	}
}

func TestAccountCapability_IncrementNonce_Nil(t *testing.T) {
	var cap *accountCapability
	ctx := context.Background()

	err := cap.IncrementNonce(ctx, "alice")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestAccountCapability_GetNonce(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-pubkey-123456789012345678901234")
	_, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	nonce, err := cap.GetNonce(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to get nonce: %v", err)
	}

	if nonce != 0 {
		t.Fatalf("expected nonce 0, got %d", nonce)
	}
}

func TestAccountCapability_GetNonce_NotFound(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	_, err := cap.GetNonce(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent account")
	}
}

func TestAccountCapability_GetNonce_Nil(t *testing.T) {
	var cap *accountCapability
	ctx := context.Background()

	_, err := cap.GetNonce(ctx, "alice")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestAccountCapability_IterateAccounts(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)
	err := cm.RegisterModule("auth")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}
	cap, err := cm.GrantAccountCapability("auth")
	if err != nil {
		t.Fatalf("failed to grant account capability: %v", err)
	}

	ctx := context.Background()

	// Create multiple accounts
	accounts := []types.AccountName{"alice", "bob", "charlie"}
	for _, name := range accounts {
		pubKey := []byte("test-pubkey-123456789012345678901234")
		_, err := cap.CreateAccount(ctx, name, pubKey)
		if err != nil {
			t.Fatalf("failed to create account %s: %v", name, err)
		}
	}

	// Flush to backing store before iterating
	if flushable, ok := cap.(interface{ Flush(context.Context) error }); ok {
		err = flushable.Flush(ctx)
		if err != nil {
			t.Fatalf("failed to flush: %v", err)
		}
	}

	// Iterate and collect names
	var collected []types.AccountName
	err = cap.IterateAccounts(ctx, func(account *types.Account) error {
		collected = append(collected, account.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("failed to iterate accounts: %v", err)
	}

	if len(collected) != len(accounts) {
		t.Fatalf("expected %d accounts, got %d", len(accounts), len(collected))
	}
}

func TestAccountCapability_IterateAccounts_NilCallback(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	err := cap.IterateAccounts(ctx, nil)
	if err == nil {
		t.Fatal("expected error with nil callback")
	}
}

func TestAccountCapability_IterateAccounts_Nil(t *testing.T) {
	var cap *accountCapability
	ctx := context.Background()

	err := cap.IterateAccounts(ctx, func(*types.Account) error { return nil })
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestAccountCapability_VerifyAuthorization(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	// Generate key pair
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Create account
	account, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Sign message
	message := []byte("test message")
	signature := ed25519.Sign(privKey, message)

	// Create authorization
	auth := types.NewAuthorization(types.Signature{
		PubKey:    pubKey,
		Signature: signature,
	})

	// Verify authorization
	err = cap.VerifyAuthorization(ctx, account, auth, message)
	if err != nil {
		t.Fatalf("failed to verify authorization: %v", err)
	}
}

func TestAccountCapability_VerifyAuthorization_InvalidSignature(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	// Generate key pair
	pubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Create account
	account, err := cap.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account: %v", err)
	}

	// Create invalid authorization (wrong signature)
	message := []byte("test message")
	auth := types.NewAuthorization(types.Signature{
		PubKey:    pubKey,
		Signature: make([]byte, ed25519.SignatureSize), // invalid signature
	})

	// Verify authorization should fail
	err = cap.VerifyAuthorization(ctx, account, auth, message)
	if err == nil {
		t.Fatal("expected error with invalid signature")
	}
}

func TestAccountCapability_VerifyAuthorization_Nil(t *testing.T) {
	var cap *accountCapability
	ctx := context.Background()

	account := types.NewAccount("alice", []byte("test-pubkey-123456789012345678901234"))
	auth := types.NewAuthorization()
	message := []byte("test")

	err := cap.VerifyAuthorization(ctx, account, auth, message)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func XTestAccountCapability_ConcurrentOperations_SKIPPED(t *testing.T) {
	cap := setupAccountCapability(t)
	ctx := context.Background()

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	// Concurrent creates
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			name := types.AccountName(string(rune('a' + (id % 26))))
			pubKey := []byte("test-pubkey-123456789012345678901234")
			_, _ = cap.CreateAccount(ctx, name, pubKey) // May succeed or fail, testing concurrency safety
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			name := types.AccountName(string(rune('a' + (id % 26))))
			_, _ = cap.GetAccount(ctx, name) // May succeed or fail, testing concurrency safety
		}(i)
	}

	// Concurrent nonce increments
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			name := types.AccountName(string(rune('a' + (id % 26))))
			_ = cap.IncrementNonce(ctx, name) // May succeed or fail, testing concurrency safety
		}(i)
	}

	wg.Wait()
}
