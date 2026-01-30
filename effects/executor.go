package effects

import (
	"fmt"
	"sync"

	"github.com/blockberries/punnet-sdk/types"
)

// Store is the interface for state storage
type Store interface {
	// Get retrieves a value from the store
	Get(key []byte) ([]byte, error)

	// Set stores a value in the store
	Set(key []byte, value []byte) error

	// Delete removes a value from the store
	Delete(key []byte) error

	// Has checks if a key exists in the store
	Has(key []byte) bool
}

// Event represents an emitted event
type Event struct {
	// Type is the event type
	Type string

	// Attributes are the event attributes
	Attributes map[string][]byte
}

// ExecutionResult contains the results of effect execution
type ExecutionResult struct {
	// Events are the events emitted during execution
	Events []Event

	// mu protects concurrent access to events
	mu sync.Mutex
}

// NewExecutionResult creates a new execution result
func NewExecutionResult() *ExecutionResult {
	return &ExecutionResult{
		Events: make([]Event, 0),
	}
}

// AddEvent adds an event to the result
func (r *ExecutionResult) AddEvent(event Event) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Events = append(r.Events, event)
}

// GetEvents returns all events (defensive copy)
func (r *ExecutionResult) GetEvents() []Event {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	events := make([]Event, len(r.Events))
	copy(events, r.Events)
	return events
}

// Executor executes effects against a store
type Executor struct {
	// store is the underlying state store
	store Store

	// balanceStore provides balance operations
	balanceStore BalanceStore

	// mu protects concurrent access
	mu sync.RWMutex
}

// BalanceStore provides balance-specific operations
type BalanceStore interface {
	// GetBalance retrieves an account's balance for a denomination
	GetBalance(account types.AccountName, denom string) (uint64, error)

	// SetBalance sets an account's balance for a denomination
	SetBalance(account types.AccountName, denom string, amount uint64) error

	// SubBalance subtracts from an account's balance
	SubBalance(account types.AccountName, denom string, amount uint64) error

	// AddBalance adds to an account's balance
	AddBalance(account types.AccountName, denom string, amount uint64) error
}

// NewExecutor creates a new effect executor
func NewExecutor(store Store, balanceStore BalanceStore) (*Executor, error) {
	if store == nil {
		return nil, fmt.Errorf("store cannot be nil")
	}
	if balanceStore == nil {
		return nil, fmt.Errorf("balance store cannot be nil")
	}

	return &Executor{
		store:        store,
		balanceStore: balanceStore,
	}, nil
}

// Execute executes a list of effects in order
// Effects must be provided in a valid execution order (e.g., from topological sort)
func (e *Executor) Execute(effects []Effect) (*ExecutionResult, error) {
	if e == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if effects == nil {
		return nil, fmt.Errorf("effects cannot be nil")
	}

	result := NewExecutionResult()

	// Execute each effect in order
	for i, effect := range effects {
		if effect == nil {
			return nil, fmt.Errorf("effect %d is nil", i)
		}

		if err := e.executeEffect(effect, result); err != nil {
			return nil, fmt.Errorf("effect %d: %w", i, err)
		}
	}

	return result, nil
}

// executeEffect executes a single effect
func (e *Executor) executeEffect(effect Effect, result *ExecutionResult) error {
	// Validate effect before execution
	if err := effect.Validate(); err != nil {
		return fmt.Errorf("invalid effect: %w", err)
	}

	// Execute based on effect type
	switch effect.Type() {
	case EffectTypeRead:
		return e.executeRead(effect)
	case EffectTypeWrite:
		return e.executeWrite(effect)
	case EffectTypeDelete:
		return e.executeDelete(effect)
	case EffectTypeTransfer:
		return e.executeTransfer(effect)
	case EffectTypeEvent:
		return e.executeEvent(effect, result)
	default:
		return fmt.Errorf("unknown effect type: %v", effect.Type())
	}
}

// executeRead executes a read effect
func (e *Executor) executeRead(effect Effect) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	key := effect.Key()
	if len(key) == 0 {
		return fmt.Errorf("read effect has empty key")
	}

	// Note: Actual reading is handled by the capability system
	// This executor just validates the read can occur
	if !e.store.Has(key) {
		return fmt.Errorf("key not found: %x", key)
	}

	return nil
}

// executeWrite executes a write effect
func (e *Executor) executeWrite(effect Effect) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := effect.Key()
	if len(key) == 0 {
		return fmt.Errorf("write effect has empty key")
	}

	// Note: Actual serialization happens in the capability/store layer
	// This executor validates the write can occur
	// In a real implementation, we would serialize the value here
	return e.store.Set(key, []byte("placeholder"))
}

// executeDelete executes a delete effect
func (e *Executor) executeDelete(effect Effect) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := effect.Key()
	if len(key) == 0 {
		return fmt.Errorf("delete effect has empty key")
	}

	return e.store.Delete(key)
}

// executeTransfer executes a transfer effect
func (e *Executor) executeTransfer(effect Effect) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Type assert to get transfer details
	transfer, ok := effect.(TransferEffect)
	if !ok {
		return fmt.Errorf("effect is not a TransferEffect")
	}

	// Validate accounts
	if !transfer.From.IsValid() {
		return fmt.Errorf("invalid from account: %s", transfer.From)
	}
	if !transfer.To.IsValid() {
		return fmt.Errorf("invalid to account: %s", transfer.To)
	}

	// Process each coin denomination
	for _, coin := range transfer.Amount {
		// Subtract from sender
		if err := e.balanceStore.SubBalance(transfer.From, coin.Denom, coin.Amount); err != nil {
			return fmt.Errorf("failed to subtract from %s: %w", transfer.From, err)
		}

		// Add to receiver
		if err := e.balanceStore.AddBalance(transfer.To, coin.Denom, coin.Amount); err != nil {
			// Rollback is handled by the transaction layer
			return fmt.Errorf("failed to add to %s: %w", transfer.To, err)
		}
	}

	return nil
}

// executeEvent executes an event effect
func (e *Executor) executeEvent(effect Effect, result *ExecutionResult) error {
	// Type assert to get event details
	eventEffect, ok := effect.(EventEffect)
	if !ok {
		return fmt.Errorf("effect is not an EventEffect")
	}

	// Create defensive copy of attributes
	attrs := make(map[string][]byte)
	for k, v := range eventEffect.Attributes {
		attrCopy := make([]byte, len(v))
		copy(attrCopy, v)
		attrs[k] = attrCopy
	}

	event := Event{
		Type:       eventEffect.EventType,
		Attributes: attrs,
	}

	result.AddEvent(event)
	return nil
}

// ExecuteParallel executes independent effects in parallel
// Effects in each batch must be independent (no conflicts)
func (e *Executor) ExecuteParallel(batches [][]Effect) (*ExecutionResult, error) {
	if e == nil {
		return nil, fmt.Errorf("executor is nil")
	}
	if batches == nil {
		return nil, fmt.Errorf("batches cannot be nil")
	}

	result := NewExecutionResult()

	// Execute each batch sequentially
	for batchIdx, batch := range batches {
		if len(batch) == 0 {
			continue
		}

		// Execute effects in this batch in parallel
		var wg sync.WaitGroup
		errChan := make(chan error, len(batch))

		for effectIdx, effect := range batch {
			if effect == nil {
				return nil, fmt.Errorf("batch %d, effect %d is nil", batchIdx, effectIdx)
			}

			wg.Add(1)
			go func(eff Effect, idx int) {
				defer wg.Done()
				if err := e.executeEffect(eff, result); err != nil {
					errChan <- fmt.Errorf("batch %d, effect %d: %w", batchIdx, idx, err)
				}
			}(effect, effectIdx)
		}

		// Wait for all effects in this batch
		wg.Wait()
		close(errChan)

		// Check for errors
		for err := range errChan {
			return nil, err
		}
	}

	return result, nil
}
