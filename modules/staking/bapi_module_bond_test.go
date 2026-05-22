package staking

import (
	"context"
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

func newBondFixture(t *testing.T, totalSupply uint64) (*BAPIStakingModule, *store.BAPIValidatorStore, *store.BAPIBalanceStore, *effects.BAPIExecutor, context.Context) {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	mod, err := NewBAPIStakingModule(provider.GetValidatorStore(), provider.GetBalanceStore())
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	ctx := context.Background()
	// Drive the runtime hook so totalSupply lands on the module.
	require.NoError(t, mod.ConsumeTokenomics(ctx, runtime.TokenomicsParams{
		TotalSupply: totalSupply,
	}))
	return mod, provider.GetValidatorStore(), provider.GetBalanceStore(), exec, ctx
}

// TestBond_ChargedOnCreate verifies the bond is debited from the
// creator and credited to module.bonds when handleCreateValidator
// runs against a chain with a configured TotalSupply.
func TestBond_ChargedOnCreate(t *testing.T) {
	const totalSupply uint64 = 1_000_000_000 // 1B; bond = 100_000
	mod, vs, bs, exec, ctx := newBondFixture(t, totalSupply)

	require.NoError(t, bs.Set(ctx, "alice", "stake", totalSupply))

	pubKey := make([]byte, 32)
	pubKey[0] = 0xb1
	effs, err := mod.handleCreateValidator(ctx, &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: 1},
		Account:          ptypes.AccountName("alice"),
	}, &MsgCreateValidator{
		Delegator:    ptypes.AccountName("alice"),
		PubKey:       pubKey,
		InitialPower: 100,
		Commission:   500,
	})
	require.NoError(t, err)
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	expectedBond := totalSupply / 10000 // 100k for 1B supply
	aliceBal, _ := bs.GetAmount(ctx, "alice", "stake")
	bondBal, _ := bs.GetAmount(ctx, BondEscrowAccount, "stake")
	assert.Equal(t, totalSupply-expectedBond, aliceBal,
		"alice debited by bond amount")
	assert.Equal(t, expectedBond, bondBal, "module.bonds credited by bond")

	v, _ := vs.GetValidator(ctx, pubKey)
	require.NotNil(t, v)
	assert.Equal(t, expectedBond, v.Bond,
		"validator record stores the bond amount for refund/forfeit decision")
}

// TestBond_ZeroSupplySkipsCharge confirms the fallback: a chain
// without a configured TotalSupply (TokenomicsGenesis absent) charges
// no bond, but MsgCreateValidator still succeeds.
func TestBond_ZeroSupplySkipsCharge(t *testing.T) {
	mod, vs, bs, exec, ctx := newBondFixture(t, 0)

	pubKey := make([]byte, 32)
	pubKey[0] = 0xb2
	effs, err := mod.handleCreateValidator(ctx, &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{Height: 1},
		Account:          ptypes.AccountName("alice"),
	}, &MsgCreateValidator{
		Delegator:    ptypes.AccountName("alice"),
		PubKey:       pubKey,
		InitialPower: 100,
		Commission:   500,
	})
	require.NoError(t, err)
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	bondBal, _ := bs.GetAmount(ctx, BondEscrowAccount, "stake")
	assert.Equal(t, uint64(0), bondBal)
	v, _ := vs.GetValidator(ctx, pubKey)
	assert.Equal(t, uint64(0), v.Bond)
}

// TestBond_ForfeitedOnSlash verifies that ProcessEvidence transfers
// the validator's Bond to module.ct and zeroes the field, so a
// re-slash doesn't double-forfeit.
func TestBond_ForfeitedOnSlash(t *testing.T) {
	const totalSupply uint64 = 1_000_000_000
	mod, vs, bs, exec, ctx := newBondFixture(t, totalSupply)

	const expectedBond uint64 = totalSupply / 10000

	pubKey := make([]byte, 32)
	pubKey[0] = 0xb3
	require.NoError(t, vs.SetValidator(ctx, &store.BAPIValidator{
		PubKey:          types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:           1_000,
		TotalDelegation: 1_000,
		Bond:            expectedBond,
	}))
	require.NoError(t, bs.Set(ctx, "staking.pool", "stake", 1_000))
	require.NoError(t, bs.Set(ctx, BondEscrowAccount, "stake", expectedBond))

	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
	}
	effs, err := mod.ProcessEvidence(ctx, &runtime.BAPIBlockContext{Height: 1}, ev)
	require.NoError(t, err)
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	bondBal, _ := bs.GetAmount(ctx, BondEscrowAccount, "stake")
	ctBal, _ := bs.GetAmount(ctx, string(runtime.ModuleAccountCT), "stake")
	assert.Equal(t, uint64(0), bondBal, "bond escrow drained on forfeit")
	// CT received the slash AND the bond.
	assert.GreaterOrEqual(t, ctBal, expectedBond,
		"module.ct received at least the forfeited bond")

	v, _ := vs.GetValidator(ctx, pubKey)
	assert.Equal(t, uint64(0), v.Bond,
		"validator.Bond cleared post-forfeit so re-slash can't double-spend")
}

// TestBond_FractionPinAtOnePercentile pins the constant; if
// somebody changes it, this test fires so they reconfirm the spec.
func TestBond_FractionPinAtOnePercentile(t *testing.T) {
	assert.Equal(t, uint64(1), ValidatorBondFractionBps,
		"§7 D23 fixes the bond at 0.01%% = 1 bp; changing this needs a spec amendment")
}
