package participation

import (
	"context"
	"encoding/hex"
	"testing"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/modules/staking"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newPartFixture(t *testing.T) (*BAPIParticipationModule, context.Context) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	mod, err := NewBAPIParticipationModule(ss)
	require.NoError(t, err)
	return mod, context.Background()
}

func addr(b byte) types.ValidatorAddress {
	var a types.ValidatorAddress
	a[0] = b
	return a
}

// TestOnBatchCertified_BumpsCounter pins the §3.4 batches_certified
// rule: each cert-quorum event increments the authoring validator's
// counter by 1.
func TestOnBatchCertified_BumpsCounter(t *testing.T) {
	mod, ctx := newPartFixture(t)

	v := addr(0xa1)
	for i := 0; i < 3; i++ {
		require.NoError(t, mod.OnBatchCertified(ctx, types.BatchCertifiedEvent{Validator: v}))
	}

	row, err := mod.store.Get(ctx, keyCurrentValidatorPrefix+hex.EncodeToString(v[:]))
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, uint64(3), row.BatchesCertified)
	assert.Equal(t, uint64(0), row.LeaderBlocks)

	totals, _ := mod.totalsStore.Get(ctx, keyCurrentTotals)
	require.NotNil(t, totals)
	assert.Equal(t, uint64(3), totals.BatchesCertified)
}

// TestOnBlockConstructed_LazyLeaderEarnsNothing pins PLAN D11 /
// spec §3.4: a block with zero certified batches earns no leader
// credit. The counter must stay at zero.
func TestOnBlockConstructed_LazyLeaderEarnsNothing(t *testing.T) {
	mod, ctx := newPartFixture(t)

	v := addr(0xa2)
	require.NoError(t, mod.OnBlockConstructed(ctx, types.BlockConstructedEvent{
		Leader:              v,
		IncludedBatchHashes: nil, // empty block
	}))

	row, _ := mod.store.Get(ctx, keyCurrentValidatorPrefix+hex.EncodeToString(v[:]))
	if row != nil {
		assert.Equal(t, uint64(0), row.LeaderBlocks,
			"empty block must not award leader credit")
	}
	totals, _ := mod.totalsStore.Get(ctx, keyCurrentTotals)
	if totals != nil {
		assert.Equal(t, uint64(0), totals.LeaderBlocks)
	}
}

// TestOnBlockConstructed_NonEmptyBlockCounts: a block with at least
// one certified batch increments leader_blocks.
func TestOnBlockConstructed_NonEmptyBlockCounts(t *testing.T) {
	mod, ctx := newPartFixture(t)

	v := addr(0xa3)
	for i := 0; i < 2; i++ {
		require.NoError(t, mod.OnBlockConstructed(ctx, types.BlockConstructedEvent{
			Leader:              v,
			IncludedBatchHashes: []types.Hash{{}}, // one batch
		}))
	}

	row, _ := mod.store.Get(ctx, keyCurrentValidatorPrefix+hex.EncodeToString(v[:]))
	require.NotNil(t, row)
	assert.Equal(t, uint64(2), row.LeaderBlocks)

	totals, _ := mod.totalsStore.Get(ctx, keyCurrentTotals)
	require.NotNil(t, totals)
	assert.Equal(t, uint64(2), totals.LeaderBlocks)
}

// TestEndBlock_FreezesAtEpochClose: at an epoch-close height, the
// current counters move under "epoch/<num>/" and the current rows
// reset to zero. Mid-epoch heights leave state alone.
func TestEndBlock_FreezesAtEpochClose(t *testing.T) {
	mod, ctx := newPartFixture(t)

	v := addr(0xa4)
	// Seed some counter values.
	require.NoError(t, mod.OnBatchCertified(ctx, types.BatchCertifiedEvent{Validator: v}))
	require.NoError(t, mod.OnBatchCertified(ctx, types.BatchCertifiedEvent{Validator: v}))
	require.NoError(t, mod.OnBlockConstructed(ctx, types.BlockConstructedEvent{
		Leader: v, IncludedBatchHashes: []types.Hash{{}},
	}))

	// Mid-epoch EndBlock: nothing changes.
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks / 2})
	require.NoError(t, err)
	totals, _ := mod.totalsStore.Get(ctx, keyCurrentTotals)
	require.NotNil(t, totals)
	assert.Equal(t, uint64(2), totals.BatchesCertified, "mid-epoch EndBlock must not reset")
	assert.Equal(t, uint64(1), totals.LeaderBlocks)

	// Epoch-close EndBlock: freeze + reset.
	_, _, err = mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)

	// Current totals row is deleted (or zeroed). Either way, reading
	// it back surfaces an empty record.
	totals, _ = mod.totalsStore.Get(ctx, keyCurrentTotals)
	if totals != nil {
		assert.Equal(t, uint64(0), totals.BatchesCertified)
		assert.Equal(t, uint64(0), totals.LeaderBlocks)
	}

	// Frozen epoch-1 record is queryable.
	frozenTotals, err := mod.GetEpochTotals(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, frozenTotals)
	assert.Equal(t, uint64(2), frozenTotals.BatchesCertified)
	assert.Equal(t, uint64(1), frozenTotals.LeaderBlocks)

	frozenV, err := mod.GetEpochParticipation(ctx, 1, v)
	require.NoError(t, err)
	require.NotNil(t, frozenV)
	assert.Equal(t, uint64(2), frozenV.BatchesCertified)
	assert.Equal(t, uint64(1), frozenV.LeaderBlocks)
}

// TestEndBlock_NextEpochStartsClean: after a freeze, new counters
// for the next epoch don't carry over the previous epoch's values.
func TestEndBlock_NextEpochStartsClean(t *testing.T) {
	mod, ctx := newPartFixture(t)
	v := addr(0xa5)

	require.NoError(t, mod.OnBatchCertified(ctx, types.BatchCertifiedEvent{Validator: v}))
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)

	// New batches in the next epoch.
	require.NoError(t, mod.OnBatchCertified(ctx, types.BatchCertifiedEvent{Validator: v}))
	require.NoError(t, mod.OnBatchCertified(ctx, types.BatchCertifiedEvent{Validator: v}))

	row, _ := mod.store.Get(ctx, keyCurrentValidatorPrefix+hex.EncodeToString(v[:]))
	require.NotNil(t, row)
	assert.Equal(t, uint64(2), row.BatchesCertified,
		"new epoch starts at zero; only sees the 2 new certs")
}

// TestIterateEpochParticipation walks one epoch's frozen validators
// in deterministic order.
func TestIterateEpochParticipation(t *testing.T) {
	mod, ctx := newPartFixture(t)

	for _, b := range []byte{0xc1, 0xc2, 0xc3} {
		v := addr(b)
		require.NoError(t, mod.OnBatchCertified(ctx, types.BatchCertifiedEvent{Validator: v}))
	}
	_, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: staking.EpochBlocks})
	require.NoError(t, err)

	var seen []string
	require.NoError(t, mod.IterateEpochParticipation(1, func(hexKey string, v *ValidatorParticipation) bool {
		seen = append(seen, hexKey)
		return false
	}))
	assert.Len(t, seen, 3, "iterator sees all three frozen validators")
	// hex sort order: c1 < c2 < c3
	for i := 1; i < len(seen); i++ {
		assert.Less(t, seen[i-1], seen[i], "iteration order is hex-ascending")
	}
}
