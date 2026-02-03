package effects

import (
	"fmt"
	"sync"
	"testing"

	"github.com/blockberries/punnet-sdk/types"
)

// MockStore implements Store interface for testing
type MockStore struct {
	data map[string][]byte
	mu   sync.RWMutex
}

func NewMockStore() *MockStore {
	return &MockStore{
		data: make(map[string][]byte),
	}
}

func (m *MockStore) Get(key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if val, ok := m.data[string(key)]; ok {
		result := make([]byte, len(val))
		copy(result, val)
		return result, nil
	}
	return nil, fmt.Errorf("key not found")
}

func (m *MockStore) Set(key []byte, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	valCopy := make([]byte, len(value))
	copy(valCopy, value)
	m.data[string(key)] = valCopy
	return nil
}

func (m *MockStore) Delete(key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, string(key))
	return nil
}

func (m *MockStore) Has(key []byte) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.data[string(key)]
	return ok
}

// MockBalanceStore implements BalanceStore interface for testing
type MockBalanceStore struct {
	balances map[string]map[string]uint64 // account -> denom -> amount
	mu       sync.RWMutex
}

func NewMockBalanceStore() *MockBalanceStore {
	return &MockBalanceStore{
		balances: make(map[string]map[string]uint64),
	}
}

func (m *MockBalanceStore) GetBalance(account types.AccountName, denom string) (uint64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if denoms, ok := m.balances[string(account)]; ok {
		return denoms[denom], nil
	}
	return 0, nil
}

func (m *MockBalanceStore) SetBalance(account types.AccountName, denom string, amount uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.balances[string(account)] == nil {
		m.balances[string(account)] = make(map[string]uint64)
	}
	m.balances[string(account)][denom] = amount
	return nil
}

func (m *MockBalanceStore) SubBalance(account types.AccountName, denom string, amount uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.balances[string(account)] == nil {
		return fmt.Errorf("account not found: %s", account)
	}
	current := m.balances[string(account)][denom]
	if current < amount {
		return fmt.Errorf("insufficient balance: have %d, need %d", current, amount)
	}
	m.balances[string(account)][denom] = current - amount
	return nil
}

func (m *MockBalanceStore) AddBalance(account types.AccountName, denom string, amount uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.balances[string(account)] == nil {
		m.balances[string(account)] = make(map[string]uint64)
	}
	m.balances[string(account)][denom] += amount
	return nil
}

func TestNewExecutor(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()

	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}
	if executor == nil {
		t.Fatal("NewExecutor returned nil executor")
	}
}

func TestNewExecutor_NilStore(t *testing.T) {
	balanceStore := NewMockBalanceStore()

	executor, err := NewExecutor(nil, balanceStore)
	if err == nil {
		t.Error("NewExecutor should fail with nil store")
	}
	if executor != nil {
		t.Error("NewExecutor should return nil executor with nil store")
	}
}

func TestNewExecutor_NilBalanceStore(t *testing.T) {
	store := NewMockStore()

	executor, err := NewExecutor(store, nil)
	if err == nil {
		t.Error("NewExecutor should fail with nil balance store")
	}
	if executor != nil {
		t.Error("NewExecutor should return nil executor with nil balance store")
	}
}

func TestExecutor_Execute_Empty(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	result, err := executor.Execute([]Effect{})
	if err != nil {
		t.Errorf("Execute(empty) failed: %v", err)
	}
	if result == nil {
		t.Error("Execute should return result for empty effects")
	}
	if len(result.GetEvents()) != 0 {
		t.Errorf("Empty execution should have 0 events, got %d", len(result.GetEvents()))
	}
}

func TestExecutor_Execute_NilEffects(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	_, err = executor.Execute(nil)
	if err == nil {
		t.Error("Execute should fail with nil effects")
	}
}

func TestExecutor_Execute_NilExecutor(t *testing.T) {
	var executor *Executor
	_, err := executor.Execute([]Effect{})
	if err == nil {
		t.Error("Execute on nil executor should fail")
	}
}

func TestExecutor_Execute_Write(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Value:    "value",
		},
	}

	result, err := executor.Execute(effects)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}

	// Verify key was written
	if !store.Has([]byte("test/key")) {
		t.Error("Key was not written to store")
	}
}

func TestExecutor_Execute_Delete(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	// First write a key
	key := []byte("test/key")
	_ = store.Set(key, []byte("value"))

	// Then delete it
	effects := []Effect{
		DeleteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
		},
	}

	_, err = executor.Execute(effects)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}

	// Verify key was deleted
	if store.Has(key) {
		t.Error("Key was not deleted from store")
	}
}

func TestExecutor_Execute_Read(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	// Write a key first
	key := []byte("test/key")
	_ = store.Set(key, []byte("value"))

	// Read it
	var dest string
	effects := []Effect{
		ReadEffect[string]{
			Store:    "test",
			StoreKey: []byte("key"),
			Dest:     &dest,
		},
	}

	_, err = executor.Execute(effects)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
}

func TestExecutor_Execute_Read_NotFound(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	// Try to read non-existent key
	var dest string
	effects := []Effect{
		ReadEffect[string]{
			Store:    "test",
			StoreKey: []byte("nonexistent"),
			Dest:     &dest,
		},
	}

	_, err = executor.Execute(effects)
	if err == nil {
		t.Error("Execute should fail when reading non-existent key")
	}
}

func TestExecutor_Execute_Transfer(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	// Set initial balances
	from := types.AccountName("alice")
	to := types.AccountName("bob")
	_ = balanceStore.SetBalance(from, "token", 1000)
	_ = balanceStore.SetBalance(to, "token", 500)

	// Transfer
	effects := []Effect{
		TransferEffect{
			From: from,
			To:   to,
			Amount: types.NewCoins(
				types.NewCoin("token", 100),
			),
		},
	}

	result, err := executor.Execute(effects)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}

	// Verify balances
	fromBalance, _ := balanceStore.GetBalance(from, "token")
	toBalance, _ := balanceStore.GetBalance(to, "token")

	if fromBalance != 900 {
		t.Errorf("From balance = %d, want 900", fromBalance)
	}
	if toBalance != 600 {
		t.Errorf("To balance = %d, want 600", toBalance)
	}
}

func TestExecutor_Execute_Transfer_InsufficientBalance(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	// Set initial balance (insufficient)
	from := types.AccountName("alice")
	to := types.AccountName("bob")
	_ = balanceStore.SetBalance(from, "token", 50)

	// Try to transfer more than balance
	effects := []Effect{
		TransferEffect{
			From: from,
			To:   to,
			Amount: types.NewCoins(
				types.NewCoin("token", 100),
			),
		},
	}

	_, err = executor.Execute(effects)
	if err == nil {
		t.Error("Execute should fail with insufficient balance")
	}
}

func TestExecutor_Execute_Event(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	effects := []Effect{
		EventEffect{
			EventType: "test.event",
			Attributes: map[string][]byte{
				"key1": []byte("value1"),
				"key2": []byte("value2"),
			},
		},
	}

	result, err := executor.Execute(effects)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}

	events := result.GetEvents()
	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.Type != "test.event" {
		t.Errorf("Event type = %s, want test.event", event.Type)
	}
	if len(event.Attributes) != 2 {
		t.Errorf("Event attributes count = %d, want 2", len(event.Attributes))
	}
}

func TestExecutor_Execute_MultipleEffects(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	effects := []Effect{
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key1"),
			Value:    "value1",
		},
		WriteEffect[string]{
			Store:    "test",
			StoreKey: []byte("key2"),
			Value:    "value2",
		},
		EventEffect{
			EventType:  "test",
			Attributes: map[string][]byte{"action": []byte("write")},
		},
	}

	result, err := executor.Execute(effects)
	if err != nil {
		t.Errorf("Execute failed: %v", err)
	}
	if result == nil {
		t.Fatal("Execute returned nil result")
	}

	// Verify writes
	if !store.Has([]byte("test/key1")) {
		t.Error("Key1 was not written")
	}
	if !store.Has([]byte("test/key2")) {
		t.Error("Key2 was not written")
	}

	// Verify event
	events := result.GetEvents()
	if len(events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(events))
	}
}

func TestExecutor_ExecuteParallel_NilExecutor(t *testing.T) {
	var executor *Executor
	_, err := executor.ExecuteParallel(nil)
	if err == nil {
		t.Error("ExecuteParallel on nil executor should fail")
	}
}

func TestExecutor_ExecuteParallel_NilBatches(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	_, err = executor.ExecuteParallel(nil)
	if err == nil {
		t.Error("ExecuteParallel should fail with nil batches")
	}
}

func TestExecutor_ExecuteParallel_SingleBatch(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	batches := [][]Effect{
		{
			WriteEffect[string]{
				Store:    "test",
				StoreKey: []byte("key1"),
				Value:    "value1",
			},
			WriteEffect[string]{
				Store:    "test",
				StoreKey: []byte("key2"),
				Value:    "value2",
			},
			WriteEffect[string]{
				Store:    "test",
				StoreKey: []byte("key3"),
				Value:    "value3",
			},
		},
	}

	result, err := executor.ExecuteParallel(batches)
	if err != nil {
		t.Errorf("ExecuteParallel failed: %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteParallel returned nil result")
	}

	// Verify all keys were written
	for i := 1; i <= 3; i++ {
		key := []byte(fmt.Sprintf("test/key%d", i))
		if !store.Has(key) {
			t.Errorf("Key%d was not written", i)
		}
	}
}

func TestExecutor_ExecuteParallel_MultipleBatches(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	batches := [][]Effect{
		{
			WriteEffect[string]{
				Store:    "test",
				StoreKey: []byte("key1"),
				Value:    "value1",
			},
			WriteEffect[string]{
				Store:    "test",
				StoreKey: []byte("key2"),
				Value:    "value2",
			},
		},
		{
			WriteEffect[string]{
				Store:    "test",
				StoreKey: []byte("key3"),
				Value:    "value3",
			},
			WriteEffect[string]{
				Store:    "test",
				StoreKey: []byte("key4"),
				Value:    "value4",
			},
		},
	}

	result, err := executor.ExecuteParallel(batches)
	if err != nil {
		t.Errorf("ExecuteParallel failed: %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteParallel returned nil result")
	}

	// Verify all keys were written
	for i := 1; i <= 4; i++ {
		key := []byte(fmt.Sprintf("test/key%d", i))
		if !store.Has(key) {
			t.Errorf("Key%d was not written", i)
		}
	}
}

func TestExecutor_ExecuteParallel_WithEvents(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	batches := [][]Effect{
		{
			EventEffect{
				EventType:  "event1",
				Attributes: map[string][]byte{"id": []byte("1")},
			},
			EventEffect{
				EventType:  "event2",
				Attributes: map[string][]byte{"id": []byte("2")},
			},
			EventEffect{
				EventType:  "event3",
				Attributes: map[string][]byte{"id": []byte("3")},
			},
		},
	}

	result, err := executor.ExecuteParallel(batches)
	if err != nil {
		t.Errorf("ExecuteParallel failed: %v", err)
	}
	if result == nil {
		t.Fatal("ExecuteParallel returned nil result")
	}

	events := result.GetEvents()
	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}
}

func TestExecutionResult_Concurrent(t *testing.T) {
	result := NewExecutionResult()

	const numGoroutines = 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result.AddEvent(Event{
				Type:       fmt.Sprintf("event%d", index),
				Attributes: map[string][]byte{"id": []byte(fmt.Sprintf("%d", index))},
			})
		}(i)
	}

	wg.Wait()

	events := result.GetEvents()
	if len(events) != numGoroutines {
		t.Errorf("Expected %d events, got %d", numGoroutines, len(events))
	}
}

func TestExecutor_Execute_InvalidEffect(t *testing.T) {
	store := NewMockStore()
	balanceStore := NewMockBalanceStore()
	executor, err := NewExecutor(store, balanceStore)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	effects := []Effect{
		WriteEffect[string]{
			Store:    "", // Invalid
			StoreKey: []byte("key"),
			Value:    "value",
		},
	}

	_, err = executor.Execute(effects)
	if err == nil {
		t.Error("Execute should fail with invalid effect")
	}
}

func TestExecutionResult_GetEvents_Immutability(t *testing.T) {
	result := NewExecutionResult()
	result.AddEvent(Event{
		Type:       "test",
		Attributes: map[string][]byte{"key": []byte("value")},
	})

	// Get events and modify (result discarded - we're testing the original wasn't affected)
	events1 := result.GetEvents()
	_ = append(events1, Event{Type: "modified"})

	// Get events again
	events2 := result.GetEvents()

	// Verify original wasn't modified
	if len(events2) != 1 {
		t.Errorf("Events were modified: got %d events, want 1", len(events2))
	}
	if events2[0].Type != "test" {
		t.Errorf("Event type was modified: got %s, want test", events2[0].Type)
	}
}

func TestExecutionResult_NilResult(t *testing.T) {
	var result *ExecutionResult

	// Should not panic
	result.AddEvent(Event{Type: "test"})

	events := result.GetEvents()
	if events != nil {
		t.Error("GetEvents on nil result should return nil")
	}
}
