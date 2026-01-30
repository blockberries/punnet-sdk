package capability

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/blockberries/punnet-sdk/store"
)

func TestNewCapabilityManager(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	if cm == nil {
		t.Fatal("expected non-nil capability manager")
	}

	if cm.modules == nil {
		t.Fatal("expected non-nil modules map")
	}
}

func TestNewCapabilityManager_NilBacking(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic with nil backing store")
		}
	}()

	NewCapabilityManager(nil)
}

func TestRegisterModule(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	err := cm.RegisterModule("auth")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	if !cm.IsModuleRegistered("auth") {
		t.Fatal("expected module to be registered")
	}
}

func TestRegisterModule_EmptyName(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	err := cm.RegisterModule("")
	if err == nil {
		t.Fatal("expected error with empty module name")
	}
}

func TestRegisterModule_Duplicate(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	err := cm.RegisterModule("auth")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	err = cm.RegisterModule("auth")
	if !errors.Is(err, ErrDuplicateModule) {
		t.Fatalf("expected ErrDuplicateModule, got %v", err)
	}
}

func TestRegisterModule_Nil(t *testing.T) {
	var cm *CapabilityManager
	err := cm.RegisterModule("auth")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestIsModuleRegistered(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	if cm.IsModuleRegistered("auth") {
		t.Fatal("expected module to not be registered")
	}

	err := cm.RegisterModule("auth")
	if err != nil {
		t.Fatalf("failed to register module: %v", err)
	}

	if !cm.IsModuleRegistered("auth") {
		t.Fatal("expected module to be registered")
	}
}

func TestIsModuleRegistered_Nil(t *testing.T) {
	var cm *CapabilityManager
	if cm.IsModuleRegistered("auth") {
		t.Fatal("expected false for nil manager")
	}
}

func TestGrantAccountCapability(t *testing.T) {
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

	if cap == nil {
		t.Fatal("expected non-nil capability")
	}

	if cap.ModuleName() != "auth" {
		t.Fatalf("expected module name 'auth', got %s", cap.ModuleName())
	}
}

func TestGrantAccountCapability_UnregisteredModule(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	_, err := cm.GrantAccountCapability("auth")
	if !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("expected ErrModuleNotFound, got %v", err)
	}
}

func TestGrantAccountCapability_Nil(t *testing.T) {
	var cm *CapabilityManager
	_, err := cm.GrantAccountCapability("auth")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestGrantBalanceCapability(t *testing.T) {
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

	if cap == nil {
		t.Fatal("expected non-nil capability")
	}

	if cap.ModuleName() != "bank" {
		t.Fatalf("expected module name 'bank', got %s", cap.ModuleName())
	}
}

func TestGrantBalanceCapability_UnregisteredModule(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	_, err := cm.GrantBalanceCapability("bank")
	if !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("expected ErrModuleNotFound, got %v", err)
	}
}

func TestGrantBalanceCapability_Nil(t *testing.T) {
	var cm *CapabilityManager
	_, err := cm.GrantBalanceCapability("bank")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestGrantValidatorCapability(t *testing.T) {
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

	if cap == nil {
		t.Fatal("expected non-nil capability")
	}

	if cap.ModuleName() != "staking" {
		t.Fatalf("expected module name 'staking', got %s", cap.ModuleName())
	}
}

func TestGrantValidatorCapability_UnregisteredModule(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	_, err := cm.GrantValidatorCapability("staking")
	if !errors.Is(err, ErrModuleNotFound) {
		t.Fatalf("expected ErrModuleNotFound, got %v", err)
	}
}

func TestGrantValidatorCapability_Nil(t *testing.T) {
	var cm *CapabilityManager
	_, err := cm.GrantValidatorCapability("staking")
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestFlush(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	ctx := context.Background()
	err := cm.Flush(ctx)
	if err != nil {
		t.Fatalf("failed to flush: %v", err)
	}
}

func TestFlush_Nil(t *testing.T) {
	var cm *CapabilityManager
	ctx := context.Background()
	err := cm.Flush(ctx)
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestClose(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	err := cm.Close()
	if err != nil {
		t.Fatalf("failed to close: %v", err)
	}
}

func TestClose_Nil(t *testing.T) {
	var cm *CapabilityManager
	err := cm.Close()
	if err != ErrCapabilityNil {
		t.Fatalf("expected ErrCapabilityNil, got %v", err)
	}
}

func TestModuleIsolation(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)
	ctx := context.Background()

	// Register two modules
	err := cm.RegisterModule("module1")
	if err != nil {
		t.Fatalf("failed to register module1: %v", err)
	}

	err = cm.RegisterModule("module2")
	if err != nil {
		t.Fatalf("failed to register module2: %v", err)
	}

	// Grant account capabilities to both modules
	cap1, err := cm.GrantAccountCapability("module1")
	if err != nil {
		t.Fatalf("failed to grant account capability to module1: %v", err)
	}

	cap2, err := cm.GrantAccountCapability("module2")
	if err != nil {
		t.Fatalf("failed to grant account capability to module2: %v", err)
	}

	// Create account in module1
	pubKey := []byte("test-pubkey-123456789012345678901234")
	account1, err := cap1.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account in module1: %v", err)
	}

	// Verify account exists in module1
	has, err := cap1.HasAccount(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to check account in module1: %v", err)
	}
	if !has {
		t.Fatal("expected account to exist in module1")
	}

	// Verify account does NOT exist in module2 (namespace isolation)
	has, err = cap2.HasAccount(ctx, "alice")
	if err != nil {
		t.Fatalf("failed to check account in module2: %v", err)
	}
	if has {
		t.Fatal("expected account to not exist in module2 (namespace isolation)")
	}

	// Create account with same name in module2
	account2, err := cap2.CreateAccount(ctx, "alice", pubKey)
	if err != nil {
		t.Fatalf("failed to create account in module2: %v", err)
	}

	// Verify both accounts exist in their respective namespaces
	if account1.Name != account2.Name {
		t.Fatal("expected accounts to have same name")
	}

	// Verify they are isolated
	has, err = cap1.HasAccount(ctx, "alice")
	if err != nil || !has {
		t.Fatal("expected account in module1")
	}

	has, err = cap2.HasAccount(ctx, "alice")
	if err != nil || !has {
		t.Fatal("expected account in module2")
	}
}

func TestConcurrentRegisterModule(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			moduleName := string(rune('a' + (id % 26)))
			cm.RegisterModule(moduleName)
		}(i)
	}

	wg.Wait()
}

func TestConcurrentGrantCapabilities(t *testing.T) {
	backing := store.NewMemoryStore()
	cm := NewCapabilityManager(backing)

	// Pre-register modules
	modules := []string{"auth", "bank", "staking"}
	for _, mod := range modules {
		if err := cm.RegisterModule(mod); err != nil {
			t.Fatalf("failed to register module %s: %v", mod, err)
		}
	}

	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := cm.GrantAccountCapability("auth")
			if err != nil {
				t.Errorf("failed to grant account capability: %v", err)
			}
		}()

		go func() {
			defer wg.Done()
			_, err := cm.GrantBalanceCapability("bank")
			if err != nil {
				t.Errorf("failed to grant balance capability: %v", err)
			}
		}()

		go func() {
			defer wg.Done()
			_, err := cm.GrantValidatorCapability("staking")
			if err != nil {
				t.Errorf("failed to grant validator capability: %v", err)
			}
		}()
	}

	wg.Wait()
}
