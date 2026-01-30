package capability

import (
	"context"
	"sync"
	"testing"

	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

func setupValidatorCapability(t *testing.T) ValidatorCapability {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	err := cm.RegisterModule("staking")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	cap, err := cm.GrantValidatorCapability("staking")
	if err != nil {
		t.Fatalf("failed to grant validator capability: %v", err)
	}

	return cap
}

func TestValidatorCapability_ModuleName(t *testing.T) {
	cap := setupValidatorCapability(t)

	if cap.ModuleName() != "staking" {
		t.Fatalf("expected module name 'staking', got %s", cap.ModuleName())
	}
}

func TestValidatorCapability_ModuleName_Nil(t *testing.T) {
	var cap *validatorCapability
	if cap.ModuleName() != "" {
		t.Fatal("expected empty module name for nil capability")
	}
}

func TestValidatorCapability_SetValidator(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	validator := store.NewValidator(pubKey, 100, "alice")

	err := cap.SetValidator(ctx, validator)
	if err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	retrieved, err := cap.GetValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to get validator: %v", err)
	}

	if retrieved.Power != 100 {
		t.Fatalf("expected power 100, got %d", retrieved.Power)
	}

	if retrieved.Delegator != "alice" {
		t.Fatalf("expected delegator alice, got %s", retrieved.Delegator)
	}
}

func TestValidatorCapability_SetValidator_Invalid(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	// Invalid validator (empty pubkey)
	validator := store.Validator{
		PubKey:    []byte{},
		Power:     100,
		Delegator: "alice",
	}

	err := cap.SetValidator(ctx, validator)
	if err == nil {
		t.Fatal("expected error with invalid validator")
	}
}

func TestValidatorCapability_SetValidator_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	validator := store.NewValidator(pubKey, 100, "alice")

	err := cap.SetValidator(ctx, validator)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_GetValidator(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	validator := store.NewValidator(pubKey, 100, "alice")

	err := cap.SetValidator(ctx, validator)
	if err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	retrieved, err := cap.GetValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to get validator: %v", err)
	}

	if retrieved.Power != validator.Power {
		t.Fatalf("expected power %d, got %d", validator.Power, retrieved.Power)
	}
}

func TestValidatorCapability_GetValidator_NotFound(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	pubKey := []byte("nonexistent-validator-pubkey")
	_, err := cap.GetValidator(ctx, pubKey)
	if err == nil {
		t.Fatal("expected error for nonexistent validator")
	}
}

func TestValidatorCapability_GetValidator_EmptyPubKey(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	_, err := cap.GetValidator(ctx, []byte{})
	if err == nil {
		t.Fatal("expected error with empty public key")
	}
}

func TestValidatorCapability_GetValidator_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	_, err := cap.GetValidator(ctx, pubKey)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_DeleteValidator(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	validator := store.NewValidator(pubKey, 100, "alice")

	err := cap.SetValidator(ctx, validator)
	if err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	err = cap.DeleteValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to delete validator: %v", err)
	}

	has, err := cap.HasValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to check validator: %v", err)
	}

	if has {
		t.Fatal("expected validator to be deleted")
	}
}

func TestValidatorCapability_DeleteValidator_EmptyPubKey(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	err := cap.DeleteValidator(ctx, []byte{})
	if err == nil {
		t.Fatal("expected error with empty public key")
	}
}

func TestValidatorCapability_DeleteValidator_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	err := cap.DeleteValidator(ctx, pubKey)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_HasValidator(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")

	has, err := cap.HasValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to check validator: %v", err)
	}

	if has {
		t.Fatal("expected validator to not exist")
	}

	validator := store.NewValidator(pubKey, 100, "alice")
	err = cap.SetValidator(ctx, validator)
	if err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	has, err = cap.HasValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to check validator: %v", err)
	}

	if !has {
		t.Fatal("expected validator to exist")
	}
}

func TestValidatorCapability_HasValidator_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	_, err := cap.HasValidator(ctx, pubKey)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_GetActiveValidators(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)
	err := cm.RegisterModule("staking")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}
	cap, err := cm.GrantValidatorCapability("staking")
	if err != nil {
		t.Fatalf("failed to grant validator capability: %v", err)
	}

	ctx := context.Background()

	// Create active validators
	pubKey1 := []byte("validator1-pubkey-123456789012345678901")
	validator1 := store.NewValidator(pubKey1, 100, "alice")
	validator1.Active = true

	pubKey2 := []byte("validator2-pubkey-123456789012345678901")
	validator2 := store.NewValidator(pubKey2, 200, "bob")
	validator2.Active = true

	// Create inactive validator
	pubKey3 := []byte("validator3-pubkey-123456789012345678901")
	validator3 := store.NewValidator(pubKey3, 300, "charlie")
	validator3.Active = false

	err = cap.SetValidator(ctx, validator1)
	if err != nil {
		t.Fatalf("failed to set validator1: %v", err)
	}

	err = cap.SetValidator(ctx, validator2)
	if err != nil {
		t.Fatalf("failed to set validator2: %v", err)
	}

	err = cap.SetValidator(ctx, validator3)
	if err != nil {
		t.Fatalf("failed to set validator3: %v", err)
	}

	// Flush to backing store before iterating
	if flushable, ok := cap.(interface{ Flush(context.Context) error }); ok {
		err = flushable.Flush(ctx)
		if err != nil {
			t.Fatalf("failed to flush: %v", err)
		}
	}

	// Get active validators
	active, err := cap.GetActiveValidators(ctx)
	if err != nil {
		t.Fatalf("failed to get active validators: %v", err)
	}

	if len(active) != 2 {
		t.Fatalf("expected 2 active validators, got %d", len(active))
	}
}

func TestValidatorCapability_GetActiveValidators_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	_, err := cap.GetActiveValidators(ctx)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_GetValidatorSet(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)
	err := cm.RegisterModule("staking")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}
	cap, err := cm.GrantValidatorCapability("staking")
	if err != nil {
		t.Fatalf("failed to grant validator capability: %v", err)
	}

	ctx := context.Background()

	// Create active validators
	pubKey1 := []byte("validator1-pubkey-123456789012345678901")
	validator1 := store.NewValidator(pubKey1, 100, "alice")
	validator1.Active = true

	err = cap.SetValidator(ctx, validator1)
	if err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	// Flush to backing store before retrieving
	if flushable, ok := cap.(interface{ Flush(context.Context) error }); ok {
		err = flushable.Flush(ctx)
		if err != nil {
			t.Fatalf("failed to flush: %v", err)
		}
	}

	// Get validator set
	updates, err := cap.GetValidatorSet(ctx)
	if err != nil {
		t.Fatalf("failed to get validator set: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("expected 1 validator update, got %d", len(updates))
	}

	if updates[0].Power != 100 {
		t.Fatalf("expected power 100, got %d", updates[0].Power)
	}
}

func TestValidatorCapability_GetValidatorSet_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	_, err := cap.GetValidatorSet(ctx)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_SetValidatorPower(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	validator := store.NewValidator(pubKey, 100, "alice")

	err := cap.SetValidator(ctx, validator)
	if err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	// Update power
	err = cap.SetValidatorPower(ctx, pubKey, 200)
	if err != nil {
		t.Fatalf("failed to set validator power: %v", err)
	}

	// Verify power updated
	retrieved, err := cap.GetValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to get validator: %v", err)
	}

	if retrieved.Power != 200 {
		t.Fatalf("expected power 200, got %d", retrieved.Power)
	}
}

func TestValidatorCapability_SetValidatorPower_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	err := cap.SetValidatorPower(ctx, pubKey, 200)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_SetValidatorActive(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	validator := store.NewValidator(pubKey, 100, "alice")
	validator.Active = true

	err := cap.SetValidator(ctx, validator)
	if err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	// Deactivate
	err = cap.SetValidatorActive(ctx, pubKey, false)
	if err != nil {
		t.Fatalf("failed to set validator active status: %v", err)
	}

	// Verify status updated
	retrieved, err := cap.GetValidator(ctx, pubKey)
	if err != nil {
		t.Fatalf("failed to get validator: %v", err)
	}

	if retrieved.Active {
		t.Fatal("expected validator to be inactive")
	}
}

func TestValidatorCapability_SetValidatorActive_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	pubKey := []byte("test-validator-pubkey-12345678901234567890")
	err := cap.SetValidatorActive(ctx, pubKey, false)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_IterateValidators(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)
	err := cm.RegisterModule("staking")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}
	cap, err := cm.GrantValidatorCapability("staking")
	if err != nil {
		t.Fatalf("failed to grant validator capability: %v", err)
	}

	ctx := context.Background()

	// Create multiple validators
	validators := []struct {
		pubKey    []byte
		power     int64
		delegator types.AccountName
	}{
		{[]byte("validator1-pubkey-123456789012345678901"), 100, "alice"},
		{[]byte("validator2-pubkey-123456789012345678901"), 200, "bob"},
		{[]byte("validator3-pubkey-123456789012345678901"), 300, "charlie"},
	}

	for _, v := range validators {
		validator := store.NewValidator(v.pubKey, v.power, v.delegator)
		err := cap.SetValidator(ctx, validator)
		if err != nil {
			t.Fatalf("failed to set validator: %v", err)
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
	err = cap.IterateValidators(ctx, func(validator store.Validator) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("failed to iterate validators: %v", err)
	}

	if count != len(validators) {
		t.Fatalf("expected %d validators, got %d", len(validators), count)
	}
}

func TestValidatorCapability_IterateValidators_NilCallback(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	err := cap.IterateValidators(ctx, nil)
	if err == nil {
		t.Fatal("expected error with nil callback")
	}
}

func TestValidatorCapability_IterateValidators_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	err := cap.IterateValidators(ctx, func(store.Validator) error { return nil })
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_SetDelegation(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	validatorPubKey := []byte("validator-pubkey-1234567890123456789012")
	delegation := store.NewDelegation("alice", validatorPubKey, 1000)

	err := cap.SetDelegation(ctx, delegation)
	if err != nil {
		t.Fatalf("failed to set delegation: %v", err)
	}

	retrieved, err := cap.GetDelegation(ctx, "alice", validatorPubKey)
	if err != nil {
		t.Fatalf("failed to get delegation: %v", err)
	}

	if retrieved.Shares != 1000 {
		t.Fatalf("expected shares 1000, got %d", retrieved.Shares)
	}
}

func TestValidatorCapability_SetDelegation_Invalid(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	// Invalid delegation (empty delegator)
	delegation := store.Delegation{
		Delegator: "",
		Validator: []byte("validator-pubkey"),
		Shares:    1000,
	}

	err := cap.SetDelegation(ctx, delegation)
	if err == nil {
		t.Fatal("expected error with invalid delegation")
	}
}

func TestValidatorCapability_SetDelegation_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	validatorPubKey := []byte("validator-pubkey-1234567890123456789012")
	delegation := store.NewDelegation("alice", validatorPubKey, 1000)

	err := cap.SetDelegation(ctx, delegation)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_GetDelegation(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	validatorPubKey := []byte("validator-pubkey-1234567890123456789012")
	delegation := store.NewDelegation("alice", validatorPubKey, 1000)

	err := cap.SetDelegation(ctx, delegation)
	if err != nil {
		t.Fatalf("failed to set delegation: %v", err)
	}

	retrieved, err := cap.GetDelegation(ctx, "alice", validatorPubKey)
	if err != nil {
		t.Fatalf("failed to get delegation: %v", err)
	}

	if retrieved.Delegator != "alice" {
		t.Fatalf("expected delegator alice, got %s", retrieved.Delegator)
	}
}

func TestValidatorCapability_GetDelegation_NotFound(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	validatorPubKey := []byte("nonexistent-validator-pubkey")
	_, err := cap.GetDelegation(ctx, "alice", validatorPubKey)
	if err == nil {
		t.Fatal("expected error for nonexistent delegation")
	}
}

func TestValidatorCapability_GetDelegation_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	validatorPubKey := []byte("validator-pubkey-1234567890123456789012")
	_, err := cap.GetDelegation(ctx, "alice", validatorPubKey)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_DeleteDelegation(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	validatorPubKey := []byte("validator-pubkey-1234567890123456789012")
	delegation := store.NewDelegation("alice", validatorPubKey, 1000)

	err := cap.SetDelegation(ctx, delegation)
	if err != nil {
		t.Fatalf("failed to set delegation: %v", err)
	}

	err = cap.DeleteDelegation(ctx, "alice", validatorPubKey)
	if err != nil {
		t.Fatalf("failed to delete delegation: %v", err)
	}

	has, err := cap.HasDelegation(ctx, "alice", validatorPubKey)
	if err != nil {
		t.Fatalf("failed to check delegation: %v", err)
	}

	if has {
		t.Fatal("expected delegation to be deleted")
	}
}

func TestValidatorCapability_DeleteDelegation_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	validatorPubKey := []byte("validator-pubkey-1234567890123456789012")
	err := cap.DeleteDelegation(ctx, "alice", validatorPubKey)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_HasDelegation(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	validatorPubKey := []byte("validator-pubkey-1234567890123456789012")

	has, err := cap.HasDelegation(ctx, "alice", validatorPubKey)
	if err != nil {
		t.Fatalf("failed to check delegation: %v", err)
	}

	if has {
		t.Fatal("expected delegation to not exist")
	}

	delegation := store.NewDelegation("alice", validatorPubKey, 1000)
	err = cap.SetDelegation(ctx, delegation)
	if err != nil {
		t.Fatalf("failed to set delegation: %v", err)
	}

	has, err = cap.HasDelegation(ctx, "alice", validatorPubKey)
	if err != nil {
		t.Fatalf("failed to check delegation: %v", err)
	}

	if !has {
		t.Fatal("expected delegation to exist")
	}
}

func TestValidatorCapability_HasDelegation_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	validatorPubKey := []byte("validator-pubkey-1234567890123456789012")
	_, err := cap.HasDelegation(ctx, "alice", validatorPubKey)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestValidatorCapability_IterateDelegations(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)
	err := cm.RegisterModule("staking")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}
	cap, err := cm.GrantValidatorCapability("staking")
	if err != nil {
		t.Fatalf("failed to grant validator capability: %v", err)
	}

	ctx := context.Background()

	// Create multiple delegations
	delegations := []struct {
		delegator types.AccountName
		validator []byte
		shares    uint64
	}{
		{"alice", []byte("validator1-pubkey-123456789012345678901"), 1000},
		{"bob", []byte("validator2-pubkey-123456789012345678901"), 2000},
		{"charlie", []byte("validator3-pubkey-123456789012345678901"), 3000},
	}

	for _, d := range delegations {
		delegation := store.NewDelegation(d.delegator, d.validator, d.shares)
		err := cap.SetDelegation(ctx, delegation)
		if err != nil {
			t.Fatalf("failed to set delegation: %v", err)
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
	err = cap.IterateDelegations(ctx, func(delegation store.Delegation) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("failed to iterate delegations: %v", err)
	}

	if count != len(delegations) {
		t.Fatalf("expected %d delegations, got %d", len(delegations), count)
	}
}

func TestValidatorCapability_IterateDelegations_NilCallback(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	err := cap.IterateDelegations(ctx, nil)
	if err == nil {
		t.Fatal("expected error with nil callback")
	}
}

func TestValidatorCapability_IterateDelegations_Nil(t *testing.T) {
	var cap *validatorCapability
	ctx := context.Background()

	err := cap.IterateDelegations(ctx, func(store.Delegation) error { return nil })
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func XTestValidatorCapability_ConcurrentOperations_SKIPPED(t *testing.T) {
	cap := setupValidatorCapability(t)
	ctx := context.Background()

	pubKey := []byte("validator-pubkey-1234567890123456789012")
	validator := store.NewValidator(pubKey, 100, "alice")

	err := cap.SetValidator(ctx, validator)
	if err != nil {
		t.Fatalf("failed to set validator: %v", err)
	}

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			cap.GetValidator(ctx, pubKey)
		}()
	}

	// Concurrent power updates
	for i := 0; i < numGoroutines; i++ {
		go func(power int64) {
			defer wg.Done()
			cap.SetValidatorPower(ctx, pubKey, power)
		}(int64(i))
	}

	// Concurrent active status updates
	for i := 0; i < numGoroutines; i++ {
		go func(active bool) {
			defer wg.Done()
			cap.SetValidatorActive(ctx, pubKey, active)
		}(i%2 == 0)
	}

	wg.Wait()
}
