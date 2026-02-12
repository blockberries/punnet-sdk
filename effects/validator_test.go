package effects

import (
	"testing"

	"github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEffectValidator_ValidateEffect(t *testing.T) {
	v := NewEffectValidator(false)

	t.Run("validates nil effect", func(t *testing.T) {
		err := v.ValidateEffect(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nil")
	})

	t.Run("validates valid event effect", func(t *testing.T) {
		effect := NewEventEffect("test.event", map[string][]byte{
			"key": []byte("value"),
		})
		err := v.ValidateEffect(effect)
		assert.NoError(t, err)
	})

	t.Run("validates invalid event effect", func(t *testing.T) {
		effect := EventEffect{EventType: ""}
		err := v.ValidateEffect(effect)
		assert.Error(t, err)
	})

	t.Run("validates valid transfer effect", func(t *testing.T) {
		effect := TransferEffect{
			From:   types.AccountName("alice"),
			To:     types.AccountName("bob"),
			Amount: types.Coins{{Denom: "stake", Amount: 100}},
		}
		err := v.ValidateEffect(effect)
		assert.NoError(t, err)
	})

	t.Run("validates transfer to self", func(t *testing.T) {
		effect := TransferEffect{
			From:   types.AccountName("alice"),
			To:     types.AccountName("alice"),
			Amount: types.Coins{{Denom: "stake", Amount: 100}},
		}
		err := v.ValidateEffect(effect)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot transfer to self")
	})
}

func TestEffectValidator_ValidateBatch(t *testing.T) {
	t.Run("validates empty batch", func(t *testing.T) {
		v := NewEffectValidator(false)
		result := v.ValidateBatch(nil)
		assert.True(t, result.Valid)
	})

	t.Run("validates batch with valid effects", func(t *testing.T) {
		v := NewEffectValidator(false)

		effects := []Effect{
			NewEventEffect("event1", nil),
			NewEventEffect("event2", nil),
			TransferEffect{
				From:   types.AccountName("alice"),
				To:     types.AccountName("bob"),
				Amount: types.Coins{{Denom: "stake", Amount: 100}},
			},
		}

		result := v.ValidateBatch(effects)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Conflicts)
	})

	t.Run("detects nil effect in batch", func(t *testing.T) {
		v := NewEffectValidator(false)

		effects := []Effect{
			NewEventEffect("event1", nil),
			nil,
			NewEventEffect("event2", nil),
		}

		result := v.ValidateBatch(effects)
		assert.False(t, result.Valid)
		assert.Contains(t, result.Errors, 1)
	})

	t.Run("detects write-write conflict", func(t *testing.T) {
		v := NewEffectValidator(false)

		key := []byte("testkey")
		effects := []Effect{
			DeleteEffect[int]{Store: "test", StoreKey: key},
			DeleteEffect[int]{Store: "test", StoreKey: key}, // Same key
		}

		result := v.ValidateBatch(effects)
		assert.False(t, result.Valid)
		assert.Len(t, result.Conflicts, 1)
		assert.Equal(t, ConflictTypeWriteWrite, result.Conflicts[0].Type)
	})
}

func TestTransferEffect_OverflowValidation(t *testing.T) {
	t.Run("rejects amount exceeding max", func(t *testing.T) {
		effect := TransferEffect{
			From:   types.AccountName("alice"),
			To:     types.AccountName("bob"),
			Amount: types.Coins{{Denom: "stake", Amount: MaxTransferAmount + 1}},
		}

		err := effect.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum")
	})

	t.Run("accepts amount at max", func(t *testing.T) {
		effect := TransferEffect{
			From:   types.AccountName("alice"),
			To:     types.AccountName("bob"),
			Amount: types.Coins{{Denom: "stake", Amount: MaxTransferAmount}},
		}

		err := effect.Validate()
		assert.NoError(t, err)
	})

	t.Run("detects duplicate denominations", func(t *testing.T) {
		effect := TransferEffect{
			From: types.AccountName("alice"),
			To:   types.AccountName("bob"),
			Amount: types.Coins{
				{Denom: "stake", Amount: 100},
				{Denom: "stake", Amount: 200}, // Duplicate
			},
		}

		err := effect.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicate denomination")
	})
}

func TestSafeAdd(t *testing.T) {
	t.Run("adds without overflow", func(t *testing.T) {
		result, ok := SafeAdd(100, 200)
		assert.True(t, ok)
		assert.Equal(t, uint64(300), result)
	})

	t.Run("detects overflow", func(t *testing.T) {
		result, ok := SafeAdd(^uint64(0), 1)
		assert.False(t, ok)
		assert.Equal(t, uint64(0), result)
	})

	t.Run("handles max value", func(t *testing.T) {
		result, ok := SafeAdd(^uint64(0)-10, 10)
		assert.True(t, ok)
		assert.Equal(t, ^uint64(0), result)
	})
}

func TestSafeSub(t *testing.T) {
	t.Run("subtracts without underflow", func(t *testing.T) {
		result, ok := SafeSub(200, 100)
		assert.True(t, ok)
		assert.Equal(t, uint64(100), result)
	})

	t.Run("detects underflow", func(t *testing.T) {
		result, ok := SafeSub(100, 200)
		assert.False(t, ok)
		assert.Equal(t, uint64(0), result)
	})

	t.Run("handles zero result", func(t *testing.T) {
		result, ok := SafeSub(100, 100)
		assert.True(t, ok)
		assert.Equal(t, uint64(0), result)
	})
}

func TestCanExecuteInParallel(t *testing.T) {
	t.Run("events can run in parallel", func(t *testing.T) {
		e1 := NewEventEffect("event1", nil)
		e2 := NewEventEffect("event2", nil)
		assert.True(t, CanExecuteInParallel(e1, e2))
	})

	t.Run("event and transfer can run in parallel", func(t *testing.T) {
		e1 := NewEventEffect("event1", nil)
		e2 := TransferEffect{
			From:   types.AccountName("alice"),
			To:     types.AccountName("bob"),
			Amount: types.Coins{{Denom: "stake", Amount: 100}},
		}
		assert.True(t, CanExecuteInParallel(e1, e2))
	})

	t.Run("different key writes can run in parallel", func(t *testing.T) {
		e1 := DeleteEffect[int]{Store: "test", StoreKey: []byte("key1")}
		e2 := DeleteEffect[int]{Store: "test", StoreKey: []byte("key2")}
		assert.True(t, CanExecuteInParallel(e1, e2))
	})

	t.Run("same key writes cannot run in parallel", func(t *testing.T) {
		e1 := DeleteEffect[int]{Store: "test", StoreKey: []byte("key")}
		e2 := DeleteEffect[int]{Store: "test", StoreKey: []byte("key")}
		assert.False(t, CanExecuteInParallel(e1, e2))
	})
}

func TestBuildExecutionPlan(t *testing.T) {
	t.Run("empty effects return nil plan", func(t *testing.T) {
		plan := BuildExecutionPlan(nil)
		assert.Nil(t, plan)
	})

	t.Run("single effect in one batch", func(t *testing.T) {
		effects := []Effect{
			NewEventEffect("event1", nil),
		}
		plan := BuildExecutionPlan(effects)
		assert.Len(t, plan, 1)
		assert.Len(t, plan[0], 1)
	})

	t.Run("parallel effects in one batch", func(t *testing.T) {
		effects := []Effect{
			NewEventEffect("event1", nil),
			NewEventEffect("event2", nil),
			NewEventEffect("event3", nil),
		}
		plan := BuildExecutionPlan(effects)
		assert.Len(t, plan, 1)
		assert.Len(t, plan[0], 3)
	})

	t.Run("conflicting effects in separate batches", func(t *testing.T) {
		effects := []Effect{
			DeleteEffect[int]{Store: "test", StoreKey: []byte("key")},
			DeleteEffect[int]{Store: "test", StoreKey: []byte("key")}, // Conflict
		}
		plan := BuildExecutionPlan(effects)
		assert.Len(t, plan, 2)
		assert.Len(t, plan[0], 1)
		assert.Len(t, plan[1], 1)
	})
}

func TestFindConflictingPairs(t *testing.T) {
	t.Run("no conflicts", func(t *testing.T) {
		effects := []Effect{
			NewEventEffect("event1", nil),
			NewEventEffect("event2", nil),
		}
		conflicts := FindConflictingPairs(effects)
		assert.Empty(t, conflicts)
	})

	t.Run("finds write-write conflict", func(t *testing.T) {
		effects := []Effect{
			DeleteEffect[int]{Store: "test", StoreKey: []byte("key")},
			DeleteEffect[int]{Store: "test", StoreKey: []byte("key")},
		}
		conflicts := FindConflictingPairs(effects)
		assert.Len(t, conflicts, 1)
		assert.Equal(t, ConflictTypeWriteWrite, conflicts[0].Type)
	})
}

func TestGroupByKey(t *testing.T) {
	t.Run("groups effects by key", func(t *testing.T) {
		effects := []Effect{
			DeleteEffect[int]{Store: "test", StoreKey: []byte("key1")},
			DeleteEffect[int]{Store: "test", StoreKey: []byte("key2")},
			DeleteEffect[int]{Store: "test", StoreKey: []byte("key1")}, // Same as first
		}

		groups := GroupByKey(effects)
		assert.Len(t, groups, 2)

		key1 := KeyString([]byte("test/key1"))
		key2 := KeyString([]byte("test/key2"))

		assert.Len(t, groups[key1], 2)
		assert.Len(t, groups[key2], 1)
	})
}

func TestValidationResult_Error(t *testing.T) {
	t.Run("valid result returns nil error", func(t *testing.T) {
		result := NewValidationResult()
		assert.Nil(t, result.Error())
	})

	t.Run("invalid result returns combined error", func(t *testing.T) {
		result := NewValidationResult()
		result.AddError(0, assert.AnError)
		result.AddConflict(&Conflict{
			Type: ConflictTypeWriteWrite,
			Key:  []byte("test"),
		})

		err := result.Error()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "effect 0")
		assert.Contains(t, err.Error(), "conflict")
	})
}
