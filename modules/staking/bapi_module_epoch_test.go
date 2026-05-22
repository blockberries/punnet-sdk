package staking

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type epochFixture struct {
	mod    *BAPIStakingModule
	store  *store.BAPIValidatorStore
	exec   *effects.BAPIExecutor
	ctx    context.Context
}

func newEpochFixture(t *testing.T) *epochFixture {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	mod, err := NewBAPIStakingModule(provider.GetValidatorStore(), provider.GetBalanceStore())
	require.NoError(t, err)
	return &epochFixture{
		mod:   mod,
		store: provider.GetValidatorStore(),
		exec:  effects.NewBAPIExecutor(provider),
		ctx:   context.Background(),
	}
}

func seedValidator(t *testing.T, vs *store.BAPIValidatorStore, ctx context.Context, pubKey []byte, power uint64) {
	t.Helper()
	require.NoError(t, vs.SetValidator(ctx, &store.BAPIValidator{
		PubKey:          types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:           power,
		TotalDelegation: power,
	}))
}

// TestEpoch_NoUpdatesMidEpoch verifies that EndBlock between epoch
// boundaries returns no ValidatorUpdates regardless of in-block
// validator changes. Spec D6: mid-epoch power changes accumulate;
// only epoch-close emits a refresh.
func TestEpoch_NoUpdatesMidEpoch(t *testing.T) {
	f := newEpochFixture(t)
	seedValidator(t, f.store, f.ctx, []byte("validator-mid-epoch-test"), 100)

	for _, h := range []uint64{1, 100, 1000, EpochBlocks - 1} {
		_, updates, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: h})
		require.NoError(t, err, "EndBlock at height %d", h)
		assert.Nil(t, updates, "mid-epoch EndBlock at height %d must not emit updates", h)
	}
}

// TestEpoch_EmitsDiffAtClose verifies that EndBlock at an epoch
// boundary emits ValidatorUpdates for the diff between the previous
// active set (initially empty) and the new top-N. Two validators
// with power 100 and 200 should both appear in the first epoch's
// updates.
func TestEpoch_EmitsDiffAtClose(t *testing.T) {
	f := newEpochFixture(t)
	pkLow := []byte("01-low-mid-epoch-validator-key0")
	pkHigh := []byte("99-top-of-set-validator-key-100")
	seedValidator(t, f.store, f.ctx, pkLow, 100)
	seedValidator(t, f.store, f.ctx, pkHigh, 200)

	_, updates, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks})
	require.NoError(t, err)
	require.Len(t, updates, 2, "first epoch close emits both validators")

	// Sorted by hex pubkey ascending — pkLow's hex starts with "303" (digit 0),
	// pkHigh's with "393" (digit 9), so pkLow comes first.
	assert.Equal(t, pkLow, updates[0].PubKey.Data)
	assert.Equal(t, uint64(100), updates[0].Power)
	assert.Equal(t, pkHigh, updates[1].PubKey.Data)
	assert.Equal(t, uint64(200), updates[1].Power)
}

// TestEpoch_NoOpWhenSetUnchanged: two epoch closes back to back with
// no validator changes must produce a non-nil first update (initial
// admission) and a nil/empty second one.
func TestEpoch_NoOpWhenSetUnchanged(t *testing.T) {
	f := newEpochFixture(t)
	seedValidator(t, f.store, f.ctx, []byte("validator-unchanged-pk-1234567"), 100)

	_, updates, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks})
	require.NoError(t, err)
	require.Len(t, updates, 1, "first epoch admits the validator")

	_, updates, err = f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks * 2})
	require.NoError(t, err)
	assert.Empty(t, updates, "unchanged set produces no diff at next epoch close")
}

// TestEpoch_PowerChangeEmitsAtClose: a validator whose power changed
// mid-epoch (e.g. via undelegate or slashing) emits a single update
// at the next epoch close.
func TestEpoch_PowerChangeEmitsAtClose(t *testing.T) {
	f := newEpochFixture(t)
	pubKey := []byte("validator-power-changes-test-1")
	seedValidator(t, f.store, f.ctx, pubKey, 1000)

	// First epoch close admits at power 1000.
	_, updates, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks})
	require.NoError(t, err)
	require.Len(t, updates, 1)
	require.Equal(t, uint64(1000), updates[0].Power)

	// Mid-epoch: drop power to 500 (simulating an undelegate).
	v, _ := f.store.GetValidator(f.ctx, pubKey)
	v.Power = 500
	v.TotalDelegation = 500
	require.NoError(t, f.store.SetValidator(f.ctx, v))

	// Mid-epoch EndBlock: nothing.
	_, updates, err = f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks + 100})
	require.NoError(t, err)
	assert.Nil(t, updates)

	// Next epoch close: emit the diff.
	_, updates, err = f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks * 2})
	require.NoError(t, err)
	require.Len(t, updates, 1)
	assert.Equal(t, uint64(500), updates[0].Power)
	assert.Equal(t, pubKey, updates[0].PubKey.Data)
}

// TestEpoch_TopNCapsActiveSet: with 3 validators and ActiveSetSize
// pinned to 2, only the top 2 by power should appear in updates.
// The third should emit Power=0 only if it had previously been in
// the set (which it hasn't, since this is the first epoch close).
func TestEpoch_TopNCapsActiveSet(t *testing.T) {
	f := newEpochFixture(t)
	f.mod.activeSetSize = 2

	seedValidator(t, f.store, f.ctx, []byte("validator-top-N-power-3000-pk1"), 3000)
	seedValidator(t, f.store, f.ctx, []byte("validator-top-N-power-2000-pk2"), 2000)
	seedValidator(t, f.store, f.ctx, []byte("validator-top-N-power-1000-pk3"), 1000)

	_, updates, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks})
	require.NoError(t, err)
	require.Len(t, updates, 2, "ActiveSetSize=2 limits emission to top 2")

	// Both updates must have power 2000 or 3000, never 1000.
	for _, u := range updates {
		assert.True(t, u.Power == 3000 || u.Power == 2000,
			"only the top 2 by power should be admitted, got Power=%d", u.Power)
	}
}

// TestEpoch_EvictedValidatorEmitsPowerZero: a validator that was in
// last epoch's set but drops out (e.g. another validator overtook
// them or they were slashed below the cutoff) must emit Power=0 at
// the next epoch close so consensus removes them.
func TestEpoch_EvictedValidatorEmitsPowerZero(t *testing.T) {
	f := newEpochFixture(t)
	f.mod.activeSetSize = 1

	pkLow := []byte("01-evictable-validator-pk-12345")
	pkHigh := []byte("99-overtake-validator-pk-678901")

	// Epoch 1: only pkLow exists; it's admitted.
	seedValidator(t, f.store, f.ctx, pkLow, 100)
	_, updates, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks})
	require.NoError(t, err)
	require.Len(t, updates, 1)
	assert.Equal(t, pkLow, updates[0].PubKey.Data)
	assert.Equal(t, uint64(100), updates[0].Power)

	// Now add pkHigh with higher power; pkLow gets bumped out.
	seedValidator(t, f.store, f.ctx, pkHigh, 1000)

	// Epoch 2: pkLow evicted, pkHigh admitted.
	_, updates, err = f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks * 2})
	require.NoError(t, err)
	require.Len(t, updates, 2)

	// Sort observation: pkLow's hex < pkHigh's hex.
	assert.Equal(t, pkLow, updates[0].PubKey.Data)
	assert.Equal(t, uint64(0), updates[0].Power, "evicted validator gets Power=0")
	assert.Equal(t, pkHigh, updates[1].PubKey.Data)
	assert.Equal(t, uint64(1000), updates[1].Power)
}

// TestEpoch_DeterministicTiebreak: when two validators have equal
// power, the tiebreaker is hex-pubkey ascending. ActiveSetSize=1
// should always pick the lexically-lower one regardless of
// insertion order.
func TestEpoch_DeterministicTiebreak(t *testing.T) {
	f := newEpochFixture(t)
	f.mod.activeSetSize = 1

	pkA := []byte("aa-equal-power-validator-key123")
	pkB := []byte("bb-equal-power-validator-key456")

	// Insert B first to ensure no accidental insertion-order dependency.
	seedValidator(t, f.store, f.ctx, pkB, 500)
	seedValidator(t, f.store, f.ctx, pkA, 500)

	_, updates, err := f.mod.EndBlock(f.ctx, &runtime.BAPIBlockContext{Height: EpochBlocks})
	require.NoError(t, err)
	require.Len(t, updates, 1)
	assert.Equal(t, pkA, updates[0].PubKey.Data,
		"tiebreaker is hex-ascending pubkey; pkA's hex %q < pkB's hex %q",
		hex.EncodeToString(pkA), hex.EncodeToString(pkB))
}

// TestIsEpochCloseHeight covers the helper that gates EndBlock's
// active-set refresh. Height 0 must never be an epoch close (no
// chain starts there); multiples of EpochBlocks must all be.
func TestIsEpochCloseHeight(t *testing.T) {
	assert.False(t, IsEpochCloseHeight(0))
	assert.False(t, IsEpochCloseHeight(1))
	assert.False(t, IsEpochCloseHeight(EpochBlocks-1))
	assert.True(t, IsEpochCloseHeight(EpochBlocks))
	assert.False(t, IsEpochCloseHeight(EpochBlocks+1))
	assert.True(t, IsEpochCloseHeight(EpochBlocks*42))
}

// Use the staking package's ptypes import.
var _ = ptypes.AccountName("")
