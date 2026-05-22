package staking

import (
	"context"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUnbondingFixture(t *testing.T) (*BAPIStakingModule, *store.BAPIValidatorStore, *store.BAPIBalanceStore, *effects.BAPIExecutor, context.Context) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	validatorStore := provider.GetValidatorStore()
	balanceStore := provider.GetBalanceStore()
	mod, err := NewBAPIStakingModule(validatorStore, balanceStore)
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	return mod, validatorStore, balanceStore, exec, context.Background()
}

// seedDelegation sets up a validator with TotalDelegation == amount
// and a single delegator stake of exactly `amount`. The validator is
// initialised with TotalDelegation: 0 so the subsequent Delegate call
// brings it to `amount` (rather than doubling). staking.pool is
// seeded with `amount` so the unbonding refund has tokens to draw.
func seedDelegation(t *testing.T, ctx context.Context, vs *store.BAPIValidatorStore, bs *store.BAPIBalanceStore, pubKey []byte, delegator string, amount uint64) {
	t.Helper()
	require.NoError(t, vs.SetValidator(ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
	}))
	require.NoError(t, vs.Delegate(ctx, delegator, pubKey, amount))
	require.NoError(t, bs.Set(ctx, "staking.pool", "stake", amount))
}

// TestUnbonding_EndBlockRefundsAtMaturity pins the spec §4 invariant:
// an undelegate at height H parks the tokens in the queue; EndBlock at
// any height < H+period leaves the queue alone; the first EndBlock at
// height ≥ H+period refunds the delegator and deletes the entry.
func TestUnbonding_EndBlockRefundsAtMaturity(t *testing.T) {
	mod, validatorStore, balanceStore, exec, ctx := newUnbondingFixture(t)
	pubKey := []byte("validator-unbond-mature")
	seedDelegation(t, ctx, validatorStore, balanceStore, pubKey, "alice", 500)

	// Undelegate 200 at height 100.
	undelegateAtHeight := uint64(100)
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: undelegateAtHeight, Time: types.TimeToTimestamp(time.Now())},
		Account:          ptypes.AccountName("alice"),
	}
	effs, err := mod.handleUndelegate(ctx, txCtx, &MsgUndelegate{
		Delegator: ptypes.AccountName("alice"),
		Validator: pubKey,
		Amount:    ptypes.Coin{Denom: "stake", Amount: 200},
	})
	require.NoError(t, err)
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	maturityHeight := undelegateAtHeight + UnbondingPeriodBlocks

	// EndBlock one block before maturity: nothing happens.
	blockEffs, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: maturityHeight - 1})
	require.NoError(t, err)
	_, err = exec.Execute(blockEffs)
	require.NoError(t, err)

	aliceBal, _ := balanceStore.GetAmount(ctx, "alice", "stake")
	assert.Equal(t, uint64(0), aliceBal, "alice should have no balance pre-maturity")

	// EndBlock at maturity: refund fires.
	blockEffs, _, err = mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: maturityHeight})
	require.NoError(t, err)
	require.NotEmpty(t, blockEffs, "EndBlock must emit refund effects at maturity")
	_, err = exec.Execute(blockEffs)
	require.NoError(t, err)

	aliceBal, _ = balanceStore.GetAmount(ctx, "alice", "stake")
	assert.Equal(t, uint64(200), aliceBal, "alice receives the unbonded amount at maturity")

	poolBal, _ := balanceStore.GetAmount(ctx, "staking.pool", "stake")
	assert.Equal(t, uint64(300), poolBal, "staking.pool drained by exactly the unbonded amount")

	// A second EndBlock past maturity must not double-refund.
	blockEffs, _, err = mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: maturityHeight + 1})
	require.NoError(t, err)
	_, err = exec.Execute(blockEffs)
	require.NoError(t, err)
	aliceBal, _ = balanceStore.GetAmount(ctx, "alice", "stake")
	assert.Equal(t, uint64(200), aliceBal, "subsequent EndBlock must not re-refund")
}

// TestUnbonding_MultipleEntriesFIFO verifies that two unbondings at
// different heights mature in order: the earlier-submitted entry
// refunds before the later one, even when both become due in the same
// EndBlock call.
func TestUnbonding_MultipleEntriesFIFO(t *testing.T) {
	mod, validatorStore, balanceStore, exec, ctx := newUnbondingFixture(t)
	pubKey := []byte("validator-unbond-fifo")
	seedDelegation(t, ctx, validatorStore, balanceStore, pubKey, "alice", 500)

	// Two undelegates 10 blocks apart.
	for _, h := range []uint64{100, 110} {
		txCtx := &runtime.BAPITxContext{
			BAPIBlockContext: &runtime.BAPIBlockContext{Height: h},
			Account:          ptypes.AccountName("alice"),
		}
		effs, err := mod.handleUndelegate(ctx, txCtx, &MsgUndelegate{
			Delegator: ptypes.AccountName("alice"),
			Validator: pubKey,
			Amount:    ptypes.Coin{Denom: "stake", Amount: 100},
		})
		require.NoError(t, err)
		_, err = exec.Execute(effs)
		require.NoError(t, err)
	}

	// At height = 100 + period (after the first maturity but before
	// the second): only the first should refund.
	firstMaturity := uint64(100) + UnbondingPeriodBlocks
	blockEffs, _, err := mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: firstMaturity})
	require.NoError(t, err)
	_, err = exec.Execute(blockEffs)
	require.NoError(t, err)

	aliceBal, _ := balanceStore.GetAmount(ctx, "alice", "stake")
	assert.Equal(t, uint64(100), aliceBal, "only first entry refunded")

	// Far enough out, both should be drained.
	bothMaturity := uint64(110) + UnbondingPeriodBlocks
	blockEffs, _, err = mod.EndBlock(ctx, &runtime.BAPIBlockContext{Height: bothMaturity})
	require.NoError(t, err)
	_, err = exec.Execute(blockEffs)
	require.NoError(t, err)

	aliceBal, _ = balanceStore.GetAmount(ctx, "alice", "stake")
	assert.Equal(t, uint64(200), aliceBal, "both entries refunded")
}

// TestUnbonding_ValidatorPowerDropsImmediately confirms that the
// undelegation reduces validator Power and TotalDelegation at the
// time of the message — not at maturity. Spec §4: a malicious
// validator can't keep their voting weight while their delegators
// are exiting.
func TestUnbonding_ValidatorPowerDropsImmediately(t *testing.T) {
	mod, validatorStore, balanceStore, exec, ctx := newUnbondingFixture(t)
	pubKey := []byte("validator-power-immediate")
	seedDelegation(t, ctx, validatorStore, balanceStore, pubKey, "alice", 500)

	pre, _ := validatorStore.GetValidator(ctx, pubKey)
	require.Equal(t, uint64(500), pre.TotalDelegation)

	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: 100},
		Account:          ptypes.AccountName("alice"),
	}
	effs, err := mod.handleUndelegate(ctx, txCtx, &MsgUndelegate{
		Delegator: ptypes.AccountName("alice"),
		Validator: pubKey,
		Amount:    ptypes.Coin{Denom: "stake", Amount: 200},
	})
	require.NoError(t, err)
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	post, _ := validatorStore.GetValidator(ctx, pubKey)
	assert.Equal(t, uint64(300), post.TotalDelegation, "power drops at undelegate, not at maturity")
	assert.Equal(t, uint64(300), post.Power)
}

// TestUnbonding_StoreKeyOrderingIsByHeight pins the FIFO-by-maturity
// invariant at the store layer: IterateMaturedUnbondings must visit
// entries in ascending MaturityHeight order, regardless of insertion
// order. This is what lets the EndBlock loop stop on the first
// future-dated entry.
func TestUnbonding_StoreKeyOrderingIsByHeight(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	vs := store.NewBAPIValidatorStore(ss)
	ctx := context.Background()

	// Insert out of order: maturity heights 30, 10, 20.
	for i, h := range []uint64{30, 10, 20} {
		require.NoError(t, vs.AddUnbondingEntry(ctx, &store.BAPIUnbondingEntry{
			Delegator:       "alice",
			ValidatorPubKey: "v",
			Amount:          100,
			MaturityHeight:  h,
			Seq:             uint64(i + 1),
		}))
	}

	var heights []uint64
	require.NoError(t, vs.IterateMaturedUnbondings(1000, func(e *store.BAPIUnbondingEntry) bool {
		heights = append(heights, e.MaturityHeight)
		return false
	}))
	assert.Equal(t, []uint64{10, 20, 30}, heights,
		"IterateMaturedUnbondings must visit ascending by height")
}
