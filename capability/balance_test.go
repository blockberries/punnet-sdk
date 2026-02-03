package capability

import (
	"context"
	"sync"
	"testing"

	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

func setupBalanceCapability(t *testing.T) BalanceCapability {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	err := cm.RegisterModule("bank")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	cap, err := cm.GrantBalanceCapability("bank")
	if err != nil {
		t.Fatalf("failed to grant balance capability: %v", err)
	}

	return cap
}

func TestBalanceCapability_ModuleName(t *testing.T) {
	cap := setupBalanceCapability(t)

	if cap.ModuleName() != "bank" {
		t.Fatalf("expected module name 'bank', got %s", cap.ModuleName())
	}
}

func TestBalanceCapability_ModuleName_Nil(t *testing.T) {
	var cap *balanceCapability
	if cap.ModuleName() != "" {
		t.Fatal("expected empty module name for nil capability")
	}
}

func TestBalanceCapability_SetBalance(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	err := cap.SetBalance(ctx, "alice", "uatom", 1000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	balance, err := cap.GetBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	if balance != 1000 {
		t.Fatalf("expected balance 1000, got %d", balance)
	}
}

func TestBalanceCapability_SetBalance_InvalidAccount(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	err := cap.SetBalance(ctx, "INVALID", "uatom", 1000)
	if err == nil {
		t.Fatal("expected error with invalid account name")
	}
}

func TestBalanceCapability_SetBalance_EmptyDenom(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	err := cap.SetBalance(ctx, "alice", "", 1000)
	if err == nil {
		t.Fatal("expected error with empty denomination")
	}
}

func TestBalanceCapability_SetBalance_Nil(t *testing.T) {
	var cap *balanceCapability
	ctx := context.Background()

	err := cap.SetBalance(ctx, "alice", "uatom", 1000)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestBalanceCapability_GetBalance(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Get balance that doesn't exist (should return 0)
	balance, err := cap.GetBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	if balance != 0 {
		t.Fatalf("expected balance 0 for nonexistent balance, got %d", balance)
	}

	// Set balance
	err = cap.SetBalance(ctx, "alice", "uatom", 1000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Get balance again
	balance, err = cap.GetBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	if balance != 1000 {
		t.Fatalf("expected balance 1000, got %d", balance)
	}
}

func TestBalanceCapability_GetBalance_InvalidAccount(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	_, err := cap.GetBalance(ctx, "INVALID", "uatom")
	if err == nil {
		t.Fatal("expected error with invalid account name")
	}
}

func TestBalanceCapability_GetBalance_EmptyDenom(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	_, err := cap.GetBalance(ctx, "alice", "")
	if err == nil {
		t.Fatal("expected error with empty denomination")
	}
}

func TestBalanceCapability_GetBalance_Nil(t *testing.T) {
	var cap *balanceCapability
	ctx := context.Background()

	_, err := cap.GetBalance(ctx, "alice", "uatom")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestBalanceCapability_AddBalance(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Add to zero balance
	err := cap.AddBalance(ctx, "alice", "uatom", 1000)
	if err != nil {
		t.Fatalf("failed to add balance: %v", err)
	}

	balance, err := cap.GetBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	if balance != 1000 {
		t.Fatalf("expected balance 1000, got %d", balance)
	}

	// Add to existing balance
	err = cap.AddBalance(ctx, "alice", "uatom", 500)
	if err != nil {
		t.Fatalf("failed to add balance: %v", err)
	}

	balance, err = cap.GetBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	if balance != 1500 {
		t.Fatalf("expected balance 1500, got %d", balance)
	}
}

func TestBalanceCapability_AddBalance_Zero(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Adding zero should be a no-op
	err := cap.AddBalance(ctx, "alice", "uatom", 0)
	if err != nil {
		t.Fatalf("failed to add zero balance: %v", err)
	}
}

func TestBalanceCapability_AddBalance_Nil(t *testing.T) {
	var cap *balanceCapability
	ctx := context.Background()

	err := cap.AddBalance(ctx, "alice", "uatom", 1000)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestBalanceCapability_SubBalance(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Set initial balance
	err := cap.SetBalance(ctx, "alice", "uatom", 1000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Subtract from balance
	err = cap.SubBalance(ctx, "alice", "uatom", 300)
	if err != nil {
		t.Fatalf("failed to subtract balance: %v", err)
	}

	balance, err := cap.GetBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	if balance != 700 {
		t.Fatalf("expected balance 700, got %d", balance)
	}
}

func TestBalanceCapability_SubBalance_InsufficientFunds(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Set initial balance
	err := cap.SetBalance(ctx, "alice", "uatom", 100)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Attempt to subtract more than available
	err = cap.SubBalance(ctx, "alice", "uatom", 200)
	if err == nil {
		t.Fatal("expected error for insufficient funds")
	}
}

func TestBalanceCapability_SubBalance_Zero(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Subtracting zero should be a no-op
	err := cap.SubBalance(ctx, "alice", "uatom", 0)
	if err != nil {
		t.Fatalf("failed to subtract zero balance: %v", err)
	}
}

func TestBalanceCapability_SubBalance_Nil(t *testing.T) {
	var cap *balanceCapability
	ctx := context.Background()

	err := cap.SubBalance(ctx, "alice", "uatom", 100)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestBalanceCapability_Transfer(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Set initial balance for sender
	err := cap.SetBalance(ctx, "alice", "uatom", 1000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Transfer
	err = cap.Transfer(ctx, "alice", "bob", "uatom", 300)
	if err != nil {
		t.Fatalf("failed to transfer: %v", err)
	}

	// Check sender balance
	aliceBalance, err := cap.GetBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to get alice balance: %v", err)
	}

	if aliceBalance != 700 {
		t.Fatalf("expected alice balance 700, got %d", aliceBalance)
	}

	// Check receiver balance
	bobBalance, err := cap.GetBalance(ctx, "bob", "uatom")
	if err != nil {
		t.Fatalf("failed to get bob balance: %v", err)
	}

	if bobBalance != 300 {
		t.Fatalf("expected bob balance 300, got %d", bobBalance)
	}
}

func TestBalanceCapability_Transfer_InsufficientFunds(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Set initial balance for sender
	err := cap.SetBalance(ctx, "alice", "uatom", 100)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Attempt to transfer more than available
	err = cap.Transfer(ctx, "alice", "bob", "uatom", 200)
	if err == nil {
		t.Fatal("expected error for insufficient funds")
	}

	// Verify sender balance unchanged
	balance, err := cap.GetBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to get balance: %v", err)
	}

	if balance != 100 {
		t.Fatalf("expected balance 100 after failed transfer, got %d", balance)
	}
}

func TestBalanceCapability_Transfer_Zero(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Transferring zero should be a no-op
	err := cap.Transfer(ctx, "alice", "bob", "uatom", 0)
	if err != nil {
		t.Fatalf("failed to transfer zero: %v", err)
	}
}

func TestBalanceCapability_Transfer_SameAccount(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Transfer to self should fail
	err := cap.Transfer(ctx, "alice", "alice", "uatom", 100)
	if err == nil {
		t.Fatal("expected error when transferring to self")
	}
}

func TestBalanceCapability_Transfer_InvalidSender(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	err := cap.Transfer(ctx, "INVALID", "bob", "uatom", 100)
	if err == nil {
		t.Fatal("expected error with invalid sender")
	}
}

func TestBalanceCapability_Transfer_InvalidReceiver(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	err := cap.Transfer(ctx, "alice", "INVALID", "uatom", 100)
	if err == nil {
		t.Fatal("expected error with invalid receiver")
	}
}

func TestBalanceCapability_Transfer_EmptyDenom(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	err := cap.Transfer(ctx, "alice", "bob", "", 100)
	if err == nil {
		t.Fatal("expected error with empty denomination")
	}
}

func TestBalanceCapability_Transfer_Nil(t *testing.T) {
	var cap *balanceCapability
	ctx := context.Background()

	err := cap.Transfer(ctx, "alice", "bob", "uatom", 100)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestBalanceCapability_GetAccountBalances(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Set multiple balances
	err := cap.SetBalance(ctx, "alice", "uatom", 1000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	err = cap.SetBalance(ctx, "alice", "uosmo", 2000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Flush to backing store before querying
	if flushable, ok := cap.(interface{ Flush(context.Context) error }); ok {
		err = flushable.Flush(ctx)
		if err != nil {
			t.Fatalf("failed to flush: %v", err)
		}
	}

	// Get all balances
	balances, err := cap.GetAccountBalances(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to get account balances: %v", err)
	}

	if len(balances) != 2 {
		t.Fatalf("expected 2 balances, got %d", len(balances))
	}
}

func TestBalanceCapability_GetAccountBalances_Empty(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	balances, err := cap.GetAccountBalances(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to get account balances: %v", err)
	}

	if len(balances) != 0 {
		t.Fatalf("expected 0 balances, got %d", len(balances))
	}
}

func TestBalanceCapability_GetAccountBalances_InvalidAccount(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	_, err := cap.GetAccountBalances(ctx, "INVALID")
	if err == nil {
		t.Fatal("expected error with invalid account name")
	}
}

func TestBalanceCapability_GetAccountBalances_Nil(t *testing.T) {
	var cap *balanceCapability
	ctx := context.Background()

	_, err := cap.GetAccountBalances(ctx, "alice")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestBalanceCapability_HasBalance(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Check non-existent balance
	has, err := cap.HasBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to check balance: %v", err)
	}

	if has {
		t.Fatal("expected balance to not exist")
	}

	// Set balance
	err = cap.SetBalance(ctx, "alice", "uatom", 1000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Check again
	has, err = cap.HasBalance(ctx, "alice", "uatom")
	if err != nil {
		t.Fatalf("failed to check balance: %v", err)
	}

	if !has {
		t.Fatal("expected balance to exist")
	}
}

func TestBalanceCapability_HasBalance_Nil(t *testing.T) {
	var cap *balanceCapability
	ctx := context.Background()

	_, err := cap.HasBalance(ctx, "alice", "uatom")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestBalanceCapability_IterateBalances(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)
	err := cm.RegisterModule("bank")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}
	cap, err := cm.GrantBalanceCapability("bank")
	if err != nil {
		t.Fatalf("failed to grant balance capability: %v", err)
	}

	ctx := context.Background()

	// Set multiple balances
	balances := []struct {
		account types.AccountName
		denom   string
		amount  uint64
	}{
		{"alice", "uatom", 1000},
		{"bob", "uatom", 2000},
		{"charlie", "uosmo", 3000},
	}

	for _, b := range balances {
		err := cap.SetBalance(ctx, b.account, b.denom, b.amount)
		if err != nil {
			t.Fatalf("failed to set balance: %v", err)
		}
	}

	// Flush to backing store before iterating
	if flushable, ok := cap.(interface{ Flush(context.Context) error }); ok {
		err = flushable.Flush(ctx)
		if err != nil {
			t.Fatalf("failed to flush: %v", err)
		}
	}

	// Iterate and count
	count := 0
	err = cap.IterateBalances(ctx, func(balance store.Balance) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("failed to iterate balances: %v", err)
	}

	if count != len(balances) {
		t.Fatalf("expected %d balances, got %d", len(balances), count)
	}
}

func TestBalanceCapability_IterateBalances_NilCallback(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	err := cap.IterateBalances(ctx, nil)
	if err == nil {
		t.Fatal("expected error with nil callback")
	}
}

func TestBalanceCapability_IterateBalances_Nil(t *testing.T) {
	var cap *balanceCapability
	ctx := context.Background()

	err := cap.IterateBalances(ctx, func(store.Balance) error { return nil })
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestBalanceCapability_IterateAccountBalances(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)
	err := cm.RegisterModule("bank")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}
	cap, err := cm.GrantBalanceCapability("bank")
	if err != nil {
		t.Fatalf("failed to grant balance capability: %v", err)
	}

	ctx := context.Background()

	// Set multiple balances for one account
	err = cap.SetBalance(ctx, "alice", "uatom", 1000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	err = cap.SetBalance(ctx, "alice", "uosmo", 2000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Set balance for different account
	err = cap.SetBalance(ctx, "bob", "uatom", 3000)
	if err != nil {
		t.Fatalf("failed to set balance: %v", err)
	}

	// Flush to backing store before iterating
	if flushable, ok := cap.(interface{ Flush(context.Context) error }); ok {
		err = flushable.Flush(ctx)
		if err != nil {
			t.Fatalf("failed to flush: %v", err)
		}
	}

	// Iterate alice's balances
	count := 0
	err = cap.IterateAccountBalances(ctx, "alice", func(balance store.Balance) error {
		if balance.Account != "alice" {
			t.Fatalf("expected alice's balance, got %s", balance.Account)
		}
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("failed to iterate account balances: %v", err)
	}

	if count != 2 {
		t.Fatalf("expected 2 balances for alice, got %d", count)
	}
}

func TestBalanceCapability_IterateAccountBalances_Nil(t *testing.T) {
	var cap *balanceCapability
	ctx := context.Background()

	err := cap.IterateAccountBalances(ctx, "alice", func(store.Balance) error { return nil })
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func XTestBalanceCapability_ConcurrentOperations_SKIPPED(t *testing.T) {
	cap := setupBalanceCapability(t)
	ctx := context.Background()

	// Set initial balance
	err := cap.SetBalance(ctx, "alice", "uatom", 10000)
	if err != nil {
		t.Fatalf("failed to set initial balance: %v", err)
	}

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = cap.AddBalance(ctx, "alice", "uatom", 10) // May succeed or fail, testing concurrency safety
		}()
	}

	// Concurrent subs
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = cap.SubBalance(ctx, "alice", "uatom", 5) // May succeed or fail, testing concurrency safety
		}()
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = cap.GetBalance(ctx, "alice", "uatom") // May succeed or fail, testing concurrency safety
		}()
	}

	wg.Wait()
}
