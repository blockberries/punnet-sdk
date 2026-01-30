package store

import (
	"context"
	"testing"

	"github.com/blockberries/punnet-sdk/types"
)

func TestAccountStore_Basic(t *testing.T) {
	backing := NewMemoryStore()
	as := NewAccountStore(backing)
	defer as.Close()

	ctx := context.Background()
	name := types.AccountName("alice")
	account := types.NewAccount(name, []byte("pubkey"))

	// Set
	err := as.Set(ctx, account)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	got, err := as.Get(ctx, name)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Name != name {
		t.Errorf("expected name %s, got %s", name, got.Name)
	}
}

func TestAccountStore_GetNotFound(t *testing.T) {
	backing := NewMemoryStore()
	as := NewAccountStore(backing)
	defer as.Close()

	ctx := context.Background()

	_, err := as.Get(ctx, types.AccountName("nonexistent"))
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAccountStore_Delete(t *testing.T) {
	backing := NewMemoryStore()
	as := NewAccountStore(backing)
	defer as.Close()

	ctx := context.Background()
	name := types.AccountName("alice")
	account := types.NewAccount(name, []byte("pubkey"))

	_ = as.Set(ctx, account)

	err := as.Delete(ctx, name)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = as.Get(ctx, name)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestAccountStore_Has(t *testing.T) {
	backing := NewMemoryStore()
	as := NewAccountStore(backing)
	defer as.Close()

	ctx := context.Background()
	name := types.AccountName("alice")
	account := types.NewAccount(name, []byte("pubkey"))

	has, err := as.Has(ctx, name)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if has {
		t.Error("expected Has to return false")
	}

	_ = as.Set(ctx, account)

	has, err = as.Has(ctx, name)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if !has {
		t.Error("expected Has to return true")
	}
}

func TestAccountStore_GetBatch(t *testing.T) {
	backing := NewMemoryStore()
	as := NewAccountStore(backing)
	defer as.Close()

	ctx := context.Background()

	// Create accounts
	names := []types.AccountName{"alice", "bob", "charlie"}
	for _, name := range names {
		account := types.NewAccount(name, []byte("pubkey"))
		_ = as.Set(ctx, account)
	}

	// Get batch
	accounts, err := as.GetBatch(ctx, names)
	if err != nil {
		t.Fatalf("GetBatch failed: %v", err)
	}

	if len(accounts) != len(names) {
		t.Errorf("expected %d accounts, got %d", len(names), len(accounts))
	}

	for _, name := range names {
		if _, ok := accounts[name]; !ok {
			t.Errorf("missing account %s", name)
		}
	}
}

func TestAccountStore_SetBatch(t *testing.T) {
	backing := NewMemoryStore()
	as := NewAccountStore(backing)
	defer as.Close()

	ctx := context.Background()

	// Create accounts
	accounts := []*types.Account{
		types.NewAccount("alice", []byte("pubkey1")),
		types.NewAccount("bob", []byte("pubkey2")),
	}

	err := as.SetBatch(ctx, accounts)
	if err != nil {
		t.Fatalf("SetBatch failed: %v", err)
	}

	// Verify
	for _, account := range accounts {
		got, err := as.Get(ctx, account.Name)
		if err != nil {
			t.Fatalf("Get failed for %s: %v", account.Name, err)
		}
		if got.Name != account.Name {
			t.Errorf("expected name %s, got %s", account.Name, got.Name)
		}
	}
}

func TestAccountStore_InvalidAccount(t *testing.T) {
	backing := NewMemoryStore()
	as := NewAccountStore(backing)
	defer as.Close()

	ctx := context.Background()

	// Invalid name
	account := types.NewAccount("INVALID", []byte("pubkey"))
	err := as.Set(ctx, account)
	if err == nil {
		t.Error("expected error for invalid account name")
	}

	// Nil account
	err = as.Set(ctx, nil)
	if err != ErrInvalidValue {
		t.Errorf("expected ErrInvalidValue, got %v", err)
	}
}

func TestBalanceStore_Basic(t *testing.T) {
	backing := NewMemoryStore()
	bs := NewBalanceStore(backing)
	defer bs.Close()

	ctx := context.Background()
	account := types.AccountName("alice")
	denom := "token"
	amount := uint64(1000)

	balance := NewBalance(account, denom, amount)

	// Set
	err := bs.Set(ctx, balance)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	got, err := bs.Get(ctx, account, denom)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Amount != amount {
		t.Errorf("expected amount %d, got %d", amount, got.Amount)
	}
}

func TestBalanceStore_GetNotFound(t *testing.T) {
	backing := NewMemoryStore()
	bs := NewBalanceStore(backing)
	defer bs.Close()

	ctx := context.Background()

	// Get non-existent balance should return zero balance
	balance, err := bs.Get(ctx, types.AccountName("alice"), "token")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if balance.Amount != 0 {
		t.Errorf("expected zero amount, got %d", balance.Amount)
	}
}

func TestBalanceStore_AddAmount(t *testing.T) {
	backing := NewMemoryStore()
	bs := NewBalanceStore(backing)
	defer bs.Close()

	ctx := context.Background()
	account := types.AccountName("alice")
	denom := "token"

	// Add to non-existent balance
	err := bs.AddAmount(ctx, account, denom, 100)
	if err != nil {
		t.Fatalf("AddAmount failed: %v", err)
	}

	balance, _ := bs.Get(ctx, account, denom)
	if balance.Amount != 100 {
		t.Errorf("expected amount 100, got %d", balance.Amount)
	}

	// Add more
	err = bs.AddAmount(ctx, account, denom, 50)
	if err != nil {
		t.Fatalf("AddAmount failed: %v", err)
	}

	balance, _ = bs.Get(ctx, account, denom)
	if balance.Amount != 150 {
		t.Errorf("expected amount 150, got %d", balance.Amount)
	}
}

func TestBalanceStore_SubAmount(t *testing.T) {
	backing := NewMemoryStore()
	bs := NewBalanceStore(backing)
	defer bs.Close()

	ctx := context.Background()
	account := types.AccountName("alice")
	denom := "token"

	// Set initial balance
	_ = bs.Set(ctx, NewBalance(account, denom, 100))

	// Subtract
	err := bs.SubAmount(ctx, account, denom, 30)
	if err != nil {
		t.Fatalf("SubAmount failed: %v", err)
	}

	balance, _ := bs.Get(ctx, account, denom)
	if balance.Amount != 70 {
		t.Errorf("expected amount 70, got %d", balance.Amount)
	}
}

func TestBalanceStore_SubAmount_InsufficientFunds(t *testing.T) {
	backing := NewMemoryStore()
	bs := NewBalanceStore(backing)
	defer bs.Close()

	ctx := context.Background()
	account := types.AccountName("alice")
	denom := "token"

	_ = bs.Set(ctx, NewBalance(account, denom, 50))

	// Try to subtract more than available
	err := bs.SubAmount(ctx, account, denom, 100)
	if err != types.ErrInsufficientFunds {
		t.Errorf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestBalanceStore_Transfer(t *testing.T) {
	backing := NewMemoryStore()
	bs := NewBalanceStore(backing)
	defer bs.Close()

	ctx := context.Background()
	alice := types.AccountName("alice")
	bob := types.AccountName("bob")
	denom := "token"

	// Set initial balances
	_ = bs.Set(ctx, NewBalance(alice, denom, 1000))
	_ = bs.Set(ctx, NewBalance(bob, denom, 500))

	// Transfer
	err := bs.Transfer(ctx, alice, bob, denom, 300)
	if err != nil {
		t.Fatalf("Transfer failed: %v", err)
	}

	// Check balances
	aliceBalance, _ := bs.Get(ctx, alice, denom)
	if aliceBalance.Amount != 700 {
		t.Errorf("expected alice balance 700, got %d", aliceBalance.Amount)
	}

	bobBalance, _ := bs.Get(ctx, bob, denom)
	if bobBalance.Amount != 800 {
		t.Errorf("expected bob balance 800, got %d", bobBalance.Amount)
	}
}

func TestBalanceStore_GetAccountBalances(t *testing.T) {
	backing := NewMemoryStore()
	bs := NewBalanceStore(backing)
	defer bs.Close()

	ctx := context.Background()
	account := types.AccountName("alice")

	// Set multiple denominations
	_ = bs.Set(ctx, NewBalance(account, "token1", 100))
	_ = bs.Set(ctx, NewBalance(account, "token2", 200))
	_ = bs.Set(ctx, NewBalance(account, "token3", 0)) // Zero balance should not be included

	// Flush to backing store for iteration
	bs.Flush(ctx)

	coins, err := bs.GetAccountBalances(ctx, account)
	if err != nil {
		t.Fatalf("GetAccountBalances failed: %v", err)
	}

	if len(coins) != 2 {
		t.Errorf("expected 2 coins, got %d", len(coins))
	}
}

func TestValidatorStore_Basic(t *testing.T) {
	backing := NewMemoryStore()
	vs := NewValidatorStore(backing)
	defer vs.Close()

	ctx := context.Background()
	pubKey := []byte("validator-pubkey")
	delegator := types.AccountName("alice")

	validator := NewValidator(pubKey, 100, delegator)

	// Set
	err := vs.Set(ctx, validator)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	got, err := vs.Get(ctx, pubKey)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Power != validator.Power {
		t.Errorf("expected power %d, got %d", validator.Power, got.Power)
	}
}

func TestValidatorStore_SetPower(t *testing.T) {
	backing := NewMemoryStore()
	vs := NewValidatorStore(backing)
	defer vs.Close()

	ctx := context.Background()
	pubKey := []byte("validator-pubkey")
	delegator := types.AccountName("alice")

	validator := NewValidator(pubKey, 100, delegator)
	_ = vs.Set(ctx, validator)

	// Update power
	err := vs.SetPower(ctx, pubKey, 200)
	if err != nil {
		t.Fatalf("SetPower failed: %v", err)
	}

	got, _ := vs.Get(ctx, pubKey)
	if got.Power != 200 {
		t.Errorf("expected power 200, got %d", got.Power)
	}
}

func TestValidatorStore_SetActive(t *testing.T) {
	backing := NewMemoryStore()
	vs := NewValidatorStore(backing)
	defer vs.Close()

	ctx := context.Background()
	pubKey := []byte("validator-pubkey")
	delegator := types.AccountName("alice")

	validator := NewValidator(pubKey, 100, delegator)
	_ = vs.Set(ctx, validator)

	// Deactivate
	err := vs.SetActive(ctx, pubKey, false)
	if err != nil {
		t.Fatalf("SetActive failed: %v", err)
	}

	got, _ := vs.Get(ctx, pubKey)
	if got.Active {
		t.Error("expected validator to be inactive")
	}
}

func TestValidatorStore_GetActiveValidators(t *testing.T) {
	backing := NewMemoryStore()
	vs := NewValidatorStore(backing)
	defer vs.Close()

	ctx := context.Background()

	// Add validators
	_ = vs.Set(ctx, NewValidator([]byte("val1"), 100, "alice"))
	_ = vs.Set(ctx, NewValidator([]byte("val2"), 0, "bob"))      // Zero power
	_ = vs.Set(ctx, NewValidator([]byte("val3"), 200, "charlie"))

	// Deactivate one
	_ = vs.SetActive(ctx, []byte("val1"), false)

	// Flush for iteration
	vs.Flush(ctx)

	active, err := vs.GetActiveValidators(ctx)
	if err != nil {
		t.Fatalf("GetActiveValidators failed: %v", err)
	}

	// Should only get val3 (val1 is inactive, val2 has zero power)
	if len(active) != 1 {
		t.Errorf("expected 1 active validator, got %d", len(active))
	}

	if len(active) > 0 && active[0].Power != 200 {
		t.Errorf("expected power 200, got %d", active[0].Power)
	}
}

func TestDelegationStore_Basic(t *testing.T) {
	backing := NewMemoryStore()
	ds := NewDelegationStore(backing)
	defer ds.Close()

	ctx := context.Background()
	delegator := types.AccountName("alice")
	validator := []byte("validator-pubkey")
	shares := uint64(1000)

	delegation := NewDelegation(delegator, validator, shares)

	// Set
	err := ds.Set(ctx, delegation)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	got, err := ds.Get(ctx, delegator, validator)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Shares != shares {
		t.Errorf("expected shares %d, got %d", shares, got.Shares)
	}
}

func TestDelegationStore_Delete(t *testing.T) {
	backing := NewMemoryStore()
	ds := NewDelegationStore(backing)
	defer ds.Close()

	ctx := context.Background()
	delegator := types.AccountName("alice")
	validator := []byte("validator-pubkey")

	delegation := NewDelegation(delegator, validator, 1000)
	_ = ds.Set(ctx, delegation)

	err := ds.Delete(ctx, delegator, validator)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = ds.Get(ctx, delegator, validator)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestBalanceKey(t *testing.T) {
	account := types.AccountName("alice")
	denom := "token"

	key := BalanceKey(account, denom)

	expected := []byte("alice/token")
	if string(key) != string(expected) {
		t.Errorf("expected %s, got %s", expected, key)
	}
}

func TestDelegationKey(t *testing.T) {
	delegator := types.AccountName("alice")
	validator := []byte{0x01, 0x02, 0x03}

	key := DelegationKey(delegator, validator)

	// Should contain both delegator and hex-encoded validator
	if len(key) == 0 {
		t.Error("expected non-empty key")
	}
}

func TestValidatorToValidatorUpdate(t *testing.T) {
	pubKey := []byte("validator-pubkey")
	validator := NewValidator(pubKey, 100, "alice")

	update := validator.ToValidatorUpdate()

	if update.Power != validator.Power {
		t.Errorf("expected power %d, got %d", validator.Power, update.Power)
	}

	if string(update.PubKey) != string(pubKey) {
		t.Error("public key mismatch")
	}
}
