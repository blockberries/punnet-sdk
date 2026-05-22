package effects

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/store"
)

// BAPIExecutor executes effects using the BAPI store provider.
// This executor uses the new typed stores backed by blockberry's StateStore.
type BAPIExecutor struct {
	storeProvider *store.BAPIStoreProvider
}

// NewBAPIExecutor creates a new BAPI effect executor.
func NewBAPIExecutor(provider *store.BAPIStoreProvider) *BAPIExecutor {
	return &BAPIExecutor{
		storeProvider: provider,
	}
}

// ValidateAll validates all effects without executing them.
func (e *BAPIExecutor) ValidateAll(effects []Effect) error {
	if e == nil {
		return fmt.Errorf("executor is nil")
	}

	for i, effect := range effects {
		if effect == nil {
			return fmt.Errorf("effect %d is nil", i)
		}
		if err := effect.Validate(); err != nil {
			return fmt.Errorf("effect %d: %w", i, err)
		}
	}

	return nil
}

// Execute executes a list of effects in order.
func (e *BAPIExecutor) Execute(effects []Effect) (*ExecutionResult, error) {
	if e == nil {
		return nil, fmt.Errorf("executor is nil")
	}

	result := NewExecutionResult()
	ctx := context.Background()

	for i, effect := range effects {
		if effect == nil {
			return nil, fmt.Errorf("effect %d is nil", i)
		}

		if err := e.executeEffect(ctx, effect, result); err != nil {
			return nil, fmt.Errorf("effect %d: %w", i, err)
		}
	}

	return result, nil
}

// executeEffect executes a single effect.
func (e *BAPIExecutor) executeEffect(ctx context.Context, effect Effect, result *ExecutionResult) error {
	// Validate effect before execution
	if err := effect.Validate(); err != nil {
		return fmt.Errorf("invalid effect: %w", err)
	}

	switch effect.Type() {
	case EffectTypeRead:
		return nil // Reads are no-ops in the executor
	case EffectTypeWrite:
		return e.executeWrite(ctx, effect)
	case EffectTypeDelete:
		return e.executeDelete(ctx, effect)
	case EffectTypeTransfer:
		return e.executeTransfer(ctx, effect)
	case EffectTypeEvent:
		return e.executeEvent(effect, result)
	default:
		return fmt.Errorf("unknown effect type: %v", effect.Type())
	}
}

// executeWrite executes a write effect.
func (e *BAPIExecutor) executeWrite(ctx context.Context, effect Effect) error {
	key := effect.Key()
	if len(key) == 0 {
		return fmt.Errorf("write effect has empty key")
	}

	// Try to get the serialized value
	var value []byte

	// Check if it's a RawWriteEffect
	if rawWrite, ok := effect.(RawWriteEffect); ok {
		value = rawWrite.SerializedValue()
	} else if valueWriter, ok := effect.(ValueWriter); ok {
		value = valueWriter.SerializedValue()
	} else if serializer, ok := effect.(WriteEffectSerializer); ok {
		// WriteEffect[T] uses pointer receiver for SerializedValue() ([]byte, error)
		var err error
		value, err = serializer.SerializedValue()
		if err != nil {
			return fmt.Errorf("serialize write effect: %w", err)
		}
	} else {
		return fmt.Errorf("write effect does not provide serialized value")
	}

	if value == nil {
		return fmt.Errorf("write effect has nil value")
	}

	// Write to the raw state store.
	if err := e.storeProvider.StateStore().Set(key, value); err != nil {
		return err
	}
	// Best-effort notify the typed stores so their in-memory key indices
	// (PLAN B2-4 / B2-5) reflect the write. Unknown key prefixes are no-ops.
	e.storeProvider.NotifyRawWrite(key)
	return nil
}

// executeDelete executes a delete effect.
func (e *BAPIExecutor) executeDelete(ctx context.Context, effect Effect) error {
	key := effect.Key()
	if len(key) == 0 {
		return fmt.Errorf("delete effect has empty key")
	}

	if err := e.storeProvider.StateStore().Delete(key); err != nil {
		return err
	}
	e.storeProvider.NotifyRawDelete(key)
	return nil
}

// executeTransfer executes a transfer effect.
func (e *BAPIExecutor) executeTransfer(ctx context.Context, effect Effect) error {
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

	balanceStore := e.storeProvider.GetBalanceStore()

	// Process each coin denomination
	for _, coin := range transfer.Amount {
		// Use the transfer method which handles both debit and credit atomically
		err := balanceStore.Transfer(ctx, string(transfer.From), string(transfer.To), coin.Denom, coin.Amount)
		if err != nil {
			return fmt.Errorf("transfer %s from %s to %s: %w", coin.Denom, transfer.From, transfer.To, err)
		}
	}

	return nil
}

// executeEvent executes an event effect.
func (e *BAPIExecutor) executeEvent(effect Effect, result *ExecutionResult) error {
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

// ValueWriter is an interface for effects that can provide their serialized value.
type ValueWriter interface {
	Effect
	SerializedValue() []byte
}

// RawWriteEffect is an effect that writes raw bytes to the store.
// This is used by the BAPI executor for effects that have already been serialized.
type RawWriteEffect struct {
	key   []byte
	value []byte
}

// NewRawWriteEffect creates a new raw write effect with key and value.
func NewRawWriteEffect(key, value []byte) RawWriteEffect {
	return RawWriteEffect{
		key:   key,
		value: value,
	}
}

// Type returns the effect type.
func (w RawWriteEffect) Type() EffectType {
	return EffectTypeWrite
}

// Validate validates the effect.
func (w RawWriteEffect) Validate() error {
	if len(w.key) == 0 {
		return fmt.Errorf("write effect key cannot be empty")
	}
	return nil
}

// Dependencies returns dependencies.
func (w RawWriteEffect) Dependencies() []Dependency {
	return nil
}

// Key returns the effect key.
func (w RawWriteEffect) Key() []byte {
	return w.key
}

// SerializedValue returns the serialized value.
func (w RawWriteEffect) SerializedValue() []byte {
	return w.value
}
