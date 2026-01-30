package integration

import (
	"context"
	"testing"
	"time"

	"github.com/blockberries/punnet-sdk/capability"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/module"
	"github.com/blockberries/punnet-sdk/modules/auth"
	"github.com/blockberries/punnet-sdk/modules/bank"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// testEnv encapsulates the test environment
type testEnv struct {
	ctx        context.Context
	backing    *store.MemoryStore
	capManager *capability.CapabilityManager
	authModule module.Module
	bankModule module.Module
	accountCap capability.AccountCapability
	balanceCap capability.BalanceCapability
}

// setupTestEnv creates a test environment with auth and bank modules
func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()

	// Create backing store
	backing := store.NewMemoryStore()
	if backing == nil {
		t.Fatal("failed to create memory store")
	}

	// Create capability manager
	capManager := capability.NewCapabilityManager(backing)
	if capManager == nil {
		t.Fatal("failed to create capability manager")
	}

	// Register modules
	if err := capManager.RegisterModule("auth"); err != nil {
		t.Fatalf("failed to register auth module: %v", err)
	}
	if err := capManager.RegisterModule("bank"); err != nil {
		t.Fatalf("failed to register bank module: %v", err)
	}

	// Grant capabilities
	accountCap, err := capManager.GrantAccountCapability("auth")
	if err != nil {
		t.Fatalf("failed to grant account capability: %v", err)
	}

	balanceCap, err := capManager.GrantBalanceCapability("bank")
	if err != nil {
		t.Fatalf("failed to grant balance capability: %v", err)
	}

	// Create modules using CreateModule (returns module.Module interface)
	authMod, err := auth.CreateModule(accountCap)
	if err != nil {
		t.Fatalf("failed to create auth module: %v", err)
	}

	bankMod, err := bank.CreateModule(balanceCap)
	if err != nil {
		t.Fatalf("failed to create bank module: %v", err)
	}

	return &testEnv{
		ctx:        context.Background(),
		backing:    backing,
		capManager: capManager,
		authModule: authMod,
		bankModule: bankMod,
		accountCap: accountCap,
		balanceCap: balanceCap,
	}
}

// TestBasicAccountCreation tests creating accounts using the capability layer
func TestBasicAccountCreation(t *testing.T) {
	env := setupTestEnv(t)

	// Create test public keys
	pubKey1 := []byte("test-pubkey-alice-001")
	pubKey2 := []byte("test-pubkey-bob-002")

	// Create alice account
	alice, err := env.accountCap.CreateAccount(env.ctx, "alice", pubKey1)
	if err != nil {
		t.Fatalf("failed to create alice account: %v", err)
	}
	if alice == nil {
		t.Fatal("alice account is nil")
	}
	if alice.Name != "alice" {
		t.Errorf("expected alice, got %s", alice.Name)
	}

	// Create bob account
	bob, err := env.accountCap.CreateAccount(env.ctx, "bob", pubKey2)
	if err != nil {
		t.Fatalf("failed to create bob account: %v", err)
	}
	if bob == nil {
		t.Fatal("bob account is nil")
	}
	if bob.Name != "bob" {
		t.Errorf("expected bob, got %s", bob.Name)
	}

	// Verify accounts can be retrieved
	aliceRetrieved, err := env.accountCap.GetAccount(env.ctx, "alice")
	if err != nil {
		t.Fatalf("failed to get alice account: %v", err)
	}
	if aliceRetrieved.Name != "alice" {
		t.Errorf("expected alice, got %s", aliceRetrieved.Name)
	}

	bobRetrieved, err := env.accountCap.GetAccount(env.ctx, "bob")
	if err != nil {
		t.Fatalf("failed to get bob account: %v", err)
	}
	if bobRetrieved.Name != "bob" {
		t.Errorf("expected bob, got %s", bobRetrieved.Name)
	}
}

// TestAuthModuleEffects tests that the auth module generates correct effects
func TestAuthModuleEffects(t *testing.T) {
	env := setupTestEnv(t)

	pubKey := []byte("test-pubkey-alice-001")
	authority := types.NewAuthority(1, pubKey, 1)

	header := runtime.NewBlockHeader(1, time.Now(), "test-chain", []byte("proposer"))
	ctx, err := runtime.NewContext(env.ctx, header, "alice")
	if err != nil {
		t.Fatalf("failed to create context: %v", err)
	}

	// Create message
	msg := &auth.MsgCreateAccount{
		Name:      "alice",
		PubKey:    pubKey,
		Authority: authority,
	}

	// Get handler and call it
	handlers := env.authModule.RegisterMsgHandlers()
	handler := handlers[auth.TypeMsgCreateAccount]
	if handler == nil {
		t.Fatal("no handler found for TypeMsgCreateAccount")
	}

	effs, err := handler(ctx, msg)
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	// Verify effects were generated
	if len(effs) == 0 {
		t.Fatal("expected effects, got none")
	}

	// Check for WriteEffect
	foundWrite := false
	foundEvent := false
	for _, eff := range effs {
		if eff.Type() == effects.EffectTypeWrite {
			foundWrite = true
		}
		if eff.Type() == effects.EffectTypeEvent {
			foundEvent = true
		}
	}

	if !foundWrite {
		t.Error("expected WriteEffect, got none")
	}
	if !foundEvent {
		t.Error("expected EventEffect, got none")
	}
}

// TestBasicTokenTransfer tests token transfers between accounts
func TestBasicTokenTransfer(t *testing.T) {
	env := setupTestEnv(t)

	// Create test accounts
	pubKeyAlice := []byte("test-pubkey-alice-001")
	pubKeyBob := []byte("test-pubkey-bob-002")

	_, err := env.accountCap.CreateAccount(env.ctx, "alice", pubKeyAlice)
	if err != nil {
		t.Fatalf("failed to create alice: %v", err)
	}

	_, err = env.accountCap.CreateAccount(env.ctx, "bob", pubKeyBob)
	if err != nil {
		t.Fatalf("failed to create bob: %v", err)
	}

	// Give alice initial balance
	if err := env.balanceCap.SetBalance(env.ctx, "alice", "token", 1000); err != nil {
		t.Fatalf("failed to set alice balance: %v", err)
	}

	// Verify alice has 1000 tokens
	balance, err := env.balanceCap.GetBalance(env.ctx, "alice", "token")
	if err != nil {
		t.Fatalf("failed to get alice balance: %v", err)
	}
	if balance != 1000 {
		t.Errorf("expected alice balance 1000, got %d", balance)
	}

	// Transfer 100 tokens from alice to bob
	if err := env.balanceCap.Transfer(env.ctx, "alice", "bob", "token", 100); err != nil {
		t.Fatalf("failed to transfer: %v", err)
	}

	// Verify balances
	aliceBalance, err := env.balanceCap.GetBalance(env.ctx, "alice", "token")
	if err != nil {
		t.Fatalf("failed to get alice balance after transfer: %v", err)
	}
	if aliceBalance != 900 {
		t.Errorf("expected alice balance 900, got %d", aliceBalance)
	}

	bobBalance, err := env.balanceCap.GetBalance(env.ctx, "bob", "token")
	if err != nil {
		t.Fatalf("failed to get bob balance: %v", err)
	}
	if bobBalance != 100 {
		t.Errorf("expected bob balance 100, got %d", bobBalance)
	}
}

// TestBankModuleEffects tests that the bank module generates correct effects for transfers
func TestBankModuleEffects(t *testing.T) {
	env := setupTestEnv(t)

	// Create test accounts
	_, err := env.accountCap.CreateAccount(env.ctx, "alice", []byte("alice-key"))
	if err != nil {
		t.Fatalf("failed to create alice: %v", err)
	}

	_, err = env.accountCap.CreateAccount(env.ctx, "bob", []byte("bob-key"))
	if err != nil {
		t.Fatalf("failed to create bob: %v", err)
	}

	// Give alice initial balance
	if err := env.balanceCap.SetBalance(env.ctx, "alice", "token", 1000); err != nil {
		t.Fatalf("failed to set alice balance: %v", err)
	}

	// Create context and message
	header := runtime.NewBlockHeader(1, time.Now(), "test-chain", []byte("proposer"))
	ctx, err := runtime.NewContext(env.ctx, header, "alice")
	if err != nil {
		t.Fatalf("failed to create context: %v", err)
	}

	msg := &bank.MsgSend{
		From:   "alice",
		To:     "bob",
		Amount: types.NewCoin("token", 100),
	}

	// Get handler and call it
	handlers := env.bankModule.RegisterMsgHandlers()
	handler := handlers[bank.TypeMsgSend]
	if handler == nil {
		t.Fatal("no handler found for TypeMsgSend")
	}

	effs, err := handler(ctx, msg)
	if err != nil {
		t.Fatalf("handler failed: %v", err)
	}

	// Verify effects were generated
	if len(effs) == 0 {
		t.Fatal("expected effects, got none")
	}

	// Check for TransferEffect
	foundTransfer := false
	foundEvent := false
	for _, eff := range effs {
		if eff.Type() == effects.EffectTypeTransfer {
			foundTransfer = true
		}
		if eff.Type() == effects.EffectTypeEvent {
			foundEvent = true
		}
	}

	if !foundTransfer {
		t.Error("expected TransferEffect, got none")
	}
	if !foundEvent {
		t.Error("expected EventEffect, got none")
	}
}

// TestInsufficientFunds tests that transfers fail with insufficient funds
func TestInsufficientFunds(t *testing.T) {
	env := setupTestEnv(t)

	// Create test accounts
	_, err := env.accountCap.CreateAccount(env.ctx, "alice", []byte("alice-key"))
	if err != nil {
		t.Fatalf("failed to create alice: %v", err)
	}

	_, err = env.accountCap.CreateAccount(env.ctx, "bob", []byte("bob-key"))
	if err != nil {
		t.Fatalf("failed to create bob: %v", err)
	}

	// Give alice only 50 tokens
	if err := env.balanceCap.SetBalance(env.ctx, "alice", "token", 50); err != nil {
		t.Fatalf("failed to set alice balance: %v", err)
	}

	// Try to transfer 100 tokens (more than alice has)
	err = env.balanceCap.Transfer(env.ctx, "alice", "bob", "token", 100)
	if err == nil {
		t.Fatal("expected error for insufficient funds, got nil")
	}
}

// TestMultipleTransfers tests multiple sequential transfers
func TestMultipleTransfers(t *testing.T) {
	env := setupTestEnv(t)

	// Create three accounts
	accounts := []string{"alice", "bob", "charlie"}
	for _, name := range accounts {
		_, err := env.accountCap.CreateAccount(env.ctx, types.AccountName(name), []byte(name+"-key"))
		if err != nil {
			t.Fatalf("failed to create %s: %v", name, err)
		}
	}

	// Give alice 1000 tokens
	if err := env.balanceCap.SetBalance(env.ctx, "alice", "token", 1000); err != nil {
		t.Fatalf("failed to set alice balance: %v", err)
	}

	// Alice sends 200 to bob
	if err := env.balanceCap.Transfer(env.ctx, "alice", "bob", "token", 200); err != nil {
		t.Fatalf("failed to send alice->bob: %v", err)
	}

	// Bob sends 100 to charlie
	if err := env.balanceCap.Transfer(env.ctx, "bob", "charlie", "token", 100); err != nil {
		t.Fatalf("failed to send bob->charlie: %v", err)
	}

	// Alice sends 300 to charlie
	if err := env.balanceCap.Transfer(env.ctx, "alice", "charlie", "token", 300); err != nil {
		t.Fatalf("failed to send alice->charlie: %v", err)
	}

	// Verify final balances
	expectedBalances := map[string]uint64{
		"alice":   500,  // 1000 - 200 - 300
		"bob":     100,  // 200 - 100
		"charlie": 400,  // 100 + 300
	}

	for name, expected := range expectedBalances {
		balance, err := env.balanceCap.GetBalance(env.ctx, types.AccountName(name), "token")
		if err != nil {
			t.Fatalf("failed to get %s balance: %v", name, err)
		}
		if balance != expected {
			t.Errorf("expected %s balance %d, got %d", name, expected, balance)
		}
	}
}

// TestAccountQuery tests querying account information
func TestAccountQuery(t *testing.T) {
	env := setupTestEnv(t)

	// Create account
	pubKey := []byte("test-pubkey-alice-001")
	alice, err := env.accountCap.CreateAccount(env.ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create alice: %v", err)
	}

	// Query account
	account, err := env.accountCap.GetAccount(env.ctx, "alice")
	if err != nil {
		t.Fatalf("failed to query account: %v", err)
	}

	// Verify account fields
	if account.Name != "alice" {
		t.Errorf("expected name alice, got %s", account.Name)
	}
	if account.Nonce != 0 {
		t.Errorf("expected nonce 0, got %d", account.Nonce)
	}
	if account.Authority.Threshold != alice.Authority.Threshold {
		t.Errorf("expected threshold %d, got %d", alice.Authority.Threshold, account.Authority.Threshold)
	}
}

// TestBalanceQuery tests querying balance information
func TestBalanceQuery(t *testing.T) {
	env := setupTestEnv(t)

	// Create account
	_, err := env.accountCap.CreateAccount(env.ctx, "alice", []byte("alice-key"))
	if err != nil {
		t.Fatalf("failed to create alice: %v", err)
	}

	// Set multiple balances
	balances := map[string]uint64{
		"token":  1000,
		"stake":  500,
		"reward": 250,
	}

	for denom, amount := range balances {
		if err := env.balanceCap.SetBalance(env.ctx, "alice", denom, amount); err != nil {
			t.Fatalf("failed to set %s balance: %v", denom, err)
		}
	}

	// Query each balance
	for denom, expected := range balances {
		balance, err := env.balanceCap.GetBalance(env.ctx, "alice", denom)
		if err != nil {
			t.Fatalf("failed to query %s balance: %v", denom, err)
		}
		if balance != expected {
			t.Errorf("expected %s balance %d, got %d", denom, expected, balance)
		}
	}

	// Query non-existent balance (should return 0)
	balance, err := env.balanceCap.GetBalance(env.ctx, "alice", "nonexistent")
	if err != nil {
		t.Fatalf("failed to query nonexistent balance: %v", err)
	}
	if balance != 0 {
		t.Errorf("expected nonexistent balance 0, got %d", balance)
	}
}

// TestModuleIntegration tests that auth and bank modules work together
func TestModuleIntegration(t *testing.T) {
	env := setupTestEnv(t)

	// Create accounts
	_, err := env.accountCap.CreateAccount(env.ctx, "alice", []byte("alice-key"))
	if err != nil {
		t.Fatalf("failed to create alice: %v", err)
	}

	_, err = env.accountCap.CreateAccount(env.ctx, "bob", []byte("bob-key"))
	if err != nil {
		t.Fatalf("failed to create bob: %v", err)
	}

	// Set balance
	if err := env.balanceCap.SetBalance(env.ctx, "alice", "token", 500); err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Verify HasAccount works
	hasAlice, err := env.accountCap.HasAccount(env.ctx, "alice")
	if err != nil {
		t.Fatalf("failed to check alice existence: %v", err)
	}
	if !hasAlice {
		t.Error("alice should exist")
	}

	hasCharlie, err := env.accountCap.HasAccount(env.ctx, "charlie")
	if err != nil {
		t.Fatalf("failed to check charlie existence: %v", err)
	}
	if hasCharlie {
		t.Error("charlie should not exist")
	}

	// Verify HasBalance works
	hasBalance, err := env.balanceCap.HasBalance(env.ctx, "alice", "token")
	if err != nil {
		t.Fatalf("failed to check balance existence: %v", err)
	}
	if !hasBalance {
		t.Error("alice should have token balance")
	}
}
