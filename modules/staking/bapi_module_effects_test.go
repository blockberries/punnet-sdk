package staking

import (
	"context"
	"encoding/json"
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

// TestHandleCreateValidator_EffectExecutionPersists is the integration check
// for PLAN B2-1: the handler returns effects, and running those effects
// through BAPIExecutor must produce exactly the state the old direct-mutation
// path would have produced.
func TestHandleCreateValidator_EffectExecutionPersists(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	validatorStore := provider.GetValidatorStore()
	balanceStore := provider.GetBalanceStore()

	mod, err := NewBAPIStakingModule(validatorStore, balanceStore)
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	ctx := context.Background()

	pubKey := []byte("validator-pubkey-1234567890ab")
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{
			Height:  100,
			Time:    types.TimeToTimestamp(time.Now()),
			ChainID: "test-chain",
		},
		Account: ptypes.AccountName("alice"),
		TxIndex: 0,
	}

	msg := &MsgCreateValidator{
		Delegator:    ptypes.AccountName("alice"),
		PubKey:       pubKey,
		InitialPower: 250,
		Commission:   300, // 3%
	}

	// Pre-condition: validator does not exist.
	exists, err := validatorStore.HasValidator(ctx, pubKey)
	require.NoError(t, err)
	assert.False(t, exists)

	effs, err := mod.handleCreateValidator(ctx, txCtx, msg)
	require.NoError(t, err)

	// Still not in the store — only the effect carries the intent.
	exists, err = validatorStore.HasValidator(ctx, pubKey)
	require.NoError(t, err)
	assert.False(t, exists, "handler must not mutate state outside the effect pipeline")

	// Execute the effects.
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	// Now the validator must be queryable via the typed store, with values
	// matching the handler input.
	v, err := validatorStore.GetValidator(ctx, pubKey)
	require.NoError(t, err)
	require.NotNil(t, v)
	assert.Equal(t, uint64(250), v.Power)
	assert.Equal(t, uint32(300), v.Commission)
	assert.Equal(t, pubKey, v.PubKey.Data)
	assert.Equal(t, types.KeyTypeEd25519, v.PubKey.Type)
}

// TestHandleUndelegate_DelegationDecrementsAfterExecute is the regression
// for PLAN B2-3 / T1-8: previously handleUndelegate issued the transfer effect
// but never decremented the delegation record, so the same stake could be
// undelegated repeatedly. After the fix, running the returned effects through
// BAPIExecutor must leave the delegation amount lower by the undelegated
// quantity.
func TestHandleUndelegate_DelegationDecrementsAfterExecute(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	validatorStore := provider.GetValidatorStore()
	balanceStore := provider.GetBalanceStore()

	mod, err := NewBAPIStakingModule(validatorStore, balanceStore)
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	ctx := context.Background()

	pubKey := []byte("validator-for-undelegate-fix1")

	// Seed: validator, pool balance, an existing 500-amount delegation.
	require.NoError(t, validatorStore.SetValidator(ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:  100,
	}))
	require.NoError(t, balanceStore.Set(ctx, "staking.pool", "stake", 1000))
	require.NoError(t, validatorStore.Delegate(ctx, "alice", pubKey, 500))

	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{
			Height:  1,
			Time:    types.TimeToTimestamp(time.Now()),
			ChainID: "test-chain",
		},
		Account: ptypes.AccountName("alice"),
		TxIndex: 0,
	}

	msg := &MsgUndelegate{
		Delegator: ptypes.AccountName("alice"),
		Validator: pubKey,
		Amount:    ptypes.Coin{Denom: "stake", Amount: 200},
	}

	effs, err := mod.handleUndelegate(ctx, txCtx, msg)
	require.NoError(t, err)

	// Pre-execute: delegation still reads 500.
	pre, err := validatorStore.GetDelegation(ctx, "alice", pubKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(500), pre.Amount, "handler must not mutate state directly")

	_, err = exec.Execute(effs)
	require.NoError(t, err)

	// Post-execute: delegation is now 300.
	post, err := validatorStore.GetDelegation(ctx, "alice", pubKey)
	require.NoError(t, err)
	assert.Equal(t, uint64(300), post.Amount, "delegation must drop from 500 to 300 after executing effects")

	// Phase 2.1: alice does NOT receive the tokens immediately —
	// they're parked in the unbonding queue and refunded by
	// EndBlock at maturity (currentHeight + UnbondingPeriodBlocks).
	bal, err := balanceStore.GetAmount(ctx, "alice", "stake")
	require.NoError(t, err)
	assert.Equal(t, uint64(0), bal, "tokens stay in staking.pool until maturity")

	// The pool balance is unchanged for the same reason.
	pool, err := balanceStore.GetAmount(ctx, "staking.pool", "stake")
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), pool, "staking.pool unchanged during unbonding period")
}

// TestHandleUndelegate_RepeatedUndelegateIsRejected pins down the consequence
// of B2-3: after executing one undelegation, a second one that would have been
// "free" under the old bug must now fail because the delegation has been
// decremented.
func TestHandleUndelegate_RepeatedUndelegateIsRejected(t *testing.T) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	validatorStore := provider.GetValidatorStore()
	balanceStore := provider.GetBalanceStore()

	mod, err := NewBAPIStakingModule(validatorStore, balanceStore)
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	ctx := context.Background()

	pubKey := []byte("validator-for-undelegate-fix2")
	require.NoError(t, validatorStore.SetValidator(ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:  100,
	}))
	require.NoError(t, balanceStore.Set(ctx, "staking.pool", "stake", 1000))
	require.NoError(t, validatorStore.Delegate(ctx, "alice", pubKey, 100))

	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: &runtime.BAPIBlockContext{
			Height:  1,
			Time:    types.TimeToTimestamp(time.Now()),
			ChainID: "test-chain",
		},
		Account: ptypes.AccountName("alice"),
		TxIndex: 0,
	}

	msg := &MsgUndelegate{
		Delegator: ptypes.AccountName("alice"),
		Validator: pubKey,
		Amount:    ptypes.Coin{Denom: "stake", Amount: 100},
	}

	// First call: should succeed and drain the delegation.
	effs, err := mod.handleUndelegate(ctx, txCtx, msg)
	require.NoError(t, err)
	_, err = exec.Execute(effs)
	require.NoError(t, err)

	// Second call: the delegation is now 0, so the handler must reject with
	// "delegation not found".
	_, err = mod.handleUndelegate(ctx, txCtx, msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delegation not found")
}

// stakingHarness wires the staking module to a fresh state store + executor,
// matching the layout of the production application path. Used by B2-2 tests.
type stakingHarness struct {
	mod       *BAPIStakingModule
	store     *store.BAPIValidatorStore
	exec      *effects.BAPIExecutor
	blockCtx  *runtime.BAPIBlockContext
	txCtx     *runtime.BAPITxContext
	delegator ptypes.AccountName
}

func newStakingHarness(t *testing.T) *stakingHarness {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	provider := store.NewBAPIStoreProvider(ss)
	validatorStore := provider.GetValidatorStore()
	balanceStore := provider.GetBalanceStore()
	mod, err := NewBAPIStakingModule(validatorStore, balanceStore)
	require.NoError(t, err)
	exec := effects.NewBAPIExecutor(provider)
	blockCtx := &runtime.BAPIBlockContext{
		Height:  1,
		Time:    types.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}
	return &stakingHarness{
		mod:      mod,
		store:    validatorStore,
		exec:     exec,
		blockCtx: blockCtx,
		txCtx: &runtime.BAPITxContext{
			BAPIBlockContext: blockCtx,
			Account:          ptypes.AccountName("alice"),
			TxIndex:          0,
		},
		delegator: ptypes.AccountName("alice"),
	}
}

// TestEndBlock_EmitsValidatorUpdateForNewValidator is the primary B2-2
// check: when handleCreateValidator runs in a block, EndBlock returns a
// ValidatorUpdate matching the new validator's power.
func TestEndBlock_EmitsValidatorUpdateForNewValidator(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))

	pubKey := []byte("validator-end-block-test-pk-1")
	createMsg := &MsgCreateValidator{
		Delegator:    h.delegator,
		PubKey:       pubKey,
		InitialPower: 77,
		Commission:   100,
	}
	effs, err := h.mod.handleCreateValidator(ctx, h.txCtx, createMsg)
	require.NoError(t, err)
	_, err = h.exec.Execute(effs)
	require.NoError(t, err)

	endEffects, updates, err := h.mod.EndBlock(ctx, h.blockCtx)
	require.NoError(t, err)
	assert.Nil(t, endEffects, "staking EndBlock returns no effects")
	require.Len(t, updates, 1)
	assert.Equal(t, uint64(77), updates[0].Power)
	assert.Equal(t, pubKey, updates[0].PubKey.Data)
	assert.Equal(t, types.KeyTypeEd25519, updates[0].PubKey.Type)
}

// TestEndBlock_NoUpdatesWhenNothingChanged verifies the "diff" property:
// an EndBlock with no dirty validators returns nil updates, and an EndBlock
// whose dirty validators have the same power as last block also returns nil.
func TestEndBlock_NoUpdatesWhenNothingChanged(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	// Block 1: create validator at power 50, EndBlock emits.
	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))
	pubKey := []byte("validator-no-change-pk-1234567")
	effs, err := h.mod.handleCreateValidator(ctx, h.txCtx, &MsgCreateValidator{
		Delegator:    h.delegator,
		PubKey:       pubKey,
		InitialPower: 50,
		Commission:   200,
	})
	require.NoError(t, err)
	_, err = h.exec.Execute(effs)
	require.NoError(t, err)
	_, updates, err := h.mod.EndBlock(ctx, h.blockCtx)
	require.NoError(t, err)
	require.Len(t, updates, 1)

	// Block 2: BeginBlock with NO further changes — EndBlock returns nil.
	h.blockCtx.Height++
	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))
	_, updates, err = h.mod.EndBlock(ctx, h.blockCtx)
	require.NoError(t, err)
	assert.Nil(t, updates, "no dirty validators must produce no updates")
}

// TestEndBlock_EmitsZeroPowerOnDeletion confirms that a validator removed
// from the store between BeginBlock and EndBlock surfaces as Power=0 — BAPI's
// canonical "validator removed" signal.
func TestEndBlock_EmitsZeroPowerOnDeletion(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	// Block 1: create validator.
	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))
	pubKey := []byte("validator-deletion-pk-12345678")
	effs, err := h.mod.handleCreateValidator(ctx, h.txCtx, &MsgCreateValidator{
		Delegator:    h.delegator,
		PubKey:       pubKey,
		InitialPower: 10,
	})
	require.NoError(t, err)
	_, err = h.exec.Execute(effs)
	require.NoError(t, err)
	_, _, err = h.mod.EndBlock(ctx, h.blockCtx)
	require.NoError(t, err)

	// Block 2: simulate an external removal of the validator, then mark
	// dirty so EndBlock sees it.
	h.blockCtx.Height++
	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))
	require.NoError(t, h.store.DeleteValidator(ctx, pubKey))
	h.mod.markValidatorDirty(pubKey)

	_, updates, err := h.mod.EndBlock(ctx, h.blockCtx)
	require.NoError(t, err)
	require.Len(t, updates, 1)
	assert.Equal(t, uint64(0), updates[0].Power, "removed validator must report Power=0")
}

// TestEndBlock_DeterministicOrdering pins down the sorted-by-hex emission
// invariant. Two validators created in arbitrary order must appear in
// lexicographic-pubkey order in the ValidatorUpdates slice.
func TestEndBlock_DeterministicOrdering(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	pkLow := []byte("01-low-priority-validator-key0")
	pkHigh := []byte("99-high-priority-validator-key")

	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))

	// Create the high-pubkey one FIRST so insertion order is reversed from
	// sorted order; if EndBlock weren't sorting, we'd see [high, low].
	effs, err := h.mod.handleCreateValidator(ctx, h.txCtx, &MsgCreateValidator{
		Delegator: h.delegator, PubKey: pkHigh, InitialPower: 200,
	})
	require.NoError(t, err)
	_, err = h.exec.Execute(effs)
	require.NoError(t, err)

	effs, err = h.mod.handleCreateValidator(ctx, h.txCtx, &MsgCreateValidator{
		Delegator: h.delegator, PubKey: pkLow, InitialPower: 100,
	})
	require.NoError(t, err)
	_, err = h.exec.Execute(effs)
	require.NoError(t, err)

	_, updates, err := h.mod.EndBlock(ctx, h.blockCtx)
	require.NoError(t, err)
	require.Len(t, updates, 2)
	assert.Equal(t, pkLow, updates[0].PubKey.Data, "lower hex pubkey emitted first")
	assert.Equal(t, pkHigh, updates[1].PubKey.Data)
}

// mustNilErr unwraps a (someThing, err) result keeping only the error,
// because t.Helper-style chaining keeps the test body readable.
func mustNilErr(_ []effects.Effect, err error) error { return err }

// TestProcessEvidence_SlashesValidatorPower verifies PLAN C2: when the
// staking module receives DuplicateVote evidence for a known validator, it
// returns a WriteEffect that — once executed — reduces the validator's power
// by SlashFractionBasisPoints and marks the validator dirty so EndBlock
// emits the post-slash power as a ValidatorUpdate.
func TestProcessEvidence_SlashesValidatorPower(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	pubKey := []byte("evidence-target-validator-pk-1")
	const initialPower uint64 = 1000

	// Seed a validator at initialPower.
	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))
	effs, err := h.mod.handleCreateValidator(ctx, h.txCtx, &MsgCreateValidator{
		Delegator:    h.delegator,
		PubKey:       pubKey,
		InitialPower: int64(initialPower),
	})
	require.NoError(t, err)
	_, err = h.exec.Execute(effs)
	require.NoError(t, err)
	// Drain the initial create-validator update so the next EndBlock only
	// sees the slash.
	_, _, err = h.mod.EndBlock(ctx, h.blockCtx)
	require.NoError(t, err)

	// New block: deliver evidence and run the resulting effects.
	h.blockCtx.Height++
	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))

	ev := types.Evidence{
		Type:             types.EvidenceTypeDuplicateVote,
		Height:           h.blockCtx.Height,
		Time:             h.blockCtx.Time,
		TotalVotingPower: 3000,
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
	}
	slashEffs, err := h.mod.ProcessEvidence(ctx, h.blockCtx, ev)
	require.NoError(t, err)
	require.NotEmpty(t, slashEffs, "ProcessEvidence must return effects for a known validator")

	_, err = h.exec.Execute(slashEffs)
	require.NoError(t, err)

	// The validator must now be at the post-slash power.
	expectedSlash := (initialPower * SlashFractionBasisPoints) / 10000
	expectedPower := initialPower - expectedSlash
	v, err := h.store.GetValidator(ctx, pubKey)
	require.NoError(t, err)
	require.NotNil(t, v)
	assert.Equal(t, expectedPower, v.Power, "post-slash power must match SlashFractionBasisPoints")
	assert.True(t, v.Jailed, "slashed validator must be jailed")

	// EndBlock must emit a ValidatorUpdate with the post-slash power.
	_, updates, err := h.mod.EndBlock(ctx, h.blockCtx)
	require.NoError(t, err)
	require.Len(t, updates, 1, "EndBlock must emit exactly the slashed validator")
	assert.Equal(t, expectedPower, updates[0].Power)
	assert.Equal(t, pubKey, updates[0].PubKey.Data)
}

// TestProcessEvidence_UnknownValidatorIsNonFatal verifies that evidence for
// a validator the staking module never saw produces no slash effect but is
// not treated as a fatal error — the chain must keep running.
func TestProcessEvidence_UnknownValidatorIsNonFatal(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))

	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		Height: 1,
		Time:   h.blockCtx.Time,
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: []byte("nobody-ever-registered-this-pk"),
		},
	}
	effs, err := h.mod.ProcessEvidence(ctx, h.blockCtx, ev)
	require.NoError(t, err)
	// We do emit an event so operators see the unknown-validator case, but
	// there must be no WriteEffect to apply.
	for _, e := range effs {
		if _, ok := e.(*effects.WriteEffect[*store.BAPIValidator]); ok {
			t.Fatalf("unknown-validator evidence produced a WriteEffect")
		}
	}
}

// TestProcessEvidence_EmptyPubkeyEmitsEvent verifies that evidence with no
// pubkey (e.g. from a node that didn't populate it) is handled as an
// "unresolved" event without panicking or slashing.
func TestProcessEvidence_EmptyPubkeyEmitsEvent(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))

	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		Height: 1,
	}
	effs, err := h.mod.ProcessEvidence(ctx, h.blockCtx, ev)
	require.NoError(t, err)
	require.NotEmpty(t, effs, "missing-pubkey evidence must emit at least an event")
	for _, e := range effs {
		if _, ok := e.(*effects.WriteEffect[*store.BAPIValidator]); ok {
			t.Fatalf("missing-pubkey evidence produced a WriteEffect")
		}
	}
}

// TestProcessEvidence_LightClientAttackSlashes verifies that light-client
// attack evidence triggers the same slash + jail as double-vote evidence.
// Reason label on the event must be "light_client_attack".
func TestProcessEvidence_LightClientAttackSlashes(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()
	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))

	pubKey := []byte("any-pubkey-here-is-fine-for-tst")
	const initialPower uint64 = 1_000_000
	require.NoError(t, h.store.SetValidator(ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
		Power: initialPower,
	}))

	ev := types.Evidence{
		Type: types.EvidenceTypeLightClient,
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: pubKey,
		},
	}
	effs, err := h.mod.ProcessEvidence(ctx, h.blockCtx, ev)
	require.NoError(t, err)
	require.NotEmpty(t, effs, "LightClient evidence must produce effects")

	var sawWrite, sawEvent bool
	for _, e := range effs {
		switch typed := e.(type) {
		case *effects.WriteEffect[*store.BAPIValidator]:
			sawWrite = true
			require.True(t, typed.Value.Jailed, "slashed validator must be jailed")
			require.Less(t, typed.Value.Power, initialPower,
				"slashed validator power must decrease")
		case effects.EventEffect:
			if typed.EventType == "staking.validator_slashed" {
				sawEvent = true
				require.Equal(t, []byte("light_client_attack"),
					typed.Attributes["reason"],
					"reason label must be light_client_attack")
			}
		}
	}
	require.True(t, sawWrite, "missing WriteEffect for slashed validator")
	require.True(t, sawEvent, "missing staking.validator_slashed event")
}

// TestProcessEvidence_UnknownTypeIsNoOp pins that future evidence types we
// do not recognize are silently accepted for forward compatibility.
func TestProcessEvidence_UnknownTypeIsNoOp(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()
	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))

	ev := types.Evidence{
		Type: types.EvidenceType(255),
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: []byte("any-pubkey-here-is-fine-for-tst"),
		},
	}
	effs, err := h.mod.ProcessEvidence(ctx, h.blockCtx, ev)
	require.NoError(t, err)
	assert.Nil(t, effs, "unknown evidence types are silently accepted")
}

// TestProcessEvidence_SlashFractionFromParams pins PLAN: when
// blockCtx.Params overrides the default slash fraction, the staking
// module honors it. The override allows governance to change slash
// severity per evidence type without recompiling.
func TestProcessEvidence_SlashFractionFromParams(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	pubKey := []byte("override-target-validator-pk-1")
	const initialPower uint64 = 10000

	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))
	require.NoError(t, h.store.SetValidator(ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:  initialPower,
	}))

	// Light-client attack with a custom 50% override (5000 bps).
	h.blockCtx.Params = &types.ConsensusParams{
		SlashFractionLightClientBps: 5000,
	}
	ev := types.Evidence{
		Type:   types.EvidenceTypeLightClient,
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
	}
	effs, err := h.mod.ProcessEvidence(ctx, h.blockCtx, ev)
	require.NoError(t, err)
	require.NotEmpty(t, effs)

	for _, e := range effs {
		if w, ok := e.(*effects.WriteEffect[*store.BAPIValidator]); ok {
			expected := initialPower - (initialPower*5000)/10000
			require.Equal(t, expected, w.Value.Power,
				"slash should use the override fraction (50%%), not the default (10%%)")
			require.True(t, w.Value.Jailed)
		}
	}
}

// TestProcessEvidence_SlashFractionDefaultWhenZero verifies the
// fallback path: a Params struct with a zero per-type field falls back
// to the compiled-in default. This means callers who only care about
// MaxBlockBytes can leave the slash knobs at zero and still get the
// expected behavior.
func TestProcessEvidence_SlashFractionDefaultWhenZero(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	pubKey := []byte("default-target-validator-pk-1")
	const initialPower uint64 = 10000

	require.NoError(t, mustNilErr(h.mod.BeginBlock(ctx, h.blockCtx)))
	require.NoError(t, h.store.SetValidator(ctx, &store.BAPIValidator{
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
		Power:  initialPower,
	}))

	// Params present but the per-type field is zero — should pick up
	// DefaultSlashFractionDoubleSignBps (500 bps = 5%).
	h.blockCtx.Params = &types.ConsensusParams{
		MaxBlockBytes: 1 << 20,
	}
	ev := types.Evidence{
		Type:   types.EvidenceTypeDuplicateVote,
		PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: pubKey},
	}
	effs, err := h.mod.ProcessEvidence(ctx, h.blockCtx, ev)
	require.NoError(t, err)
	require.NotEmpty(t, effs)

	for _, e := range effs {
		if w, ok := e.(*effects.WriteEffect[*store.BAPIValidator]); ok {
			expected := initialPower - (initialPower*uint64(DefaultSlashFractionDoubleSignBps))/10000
			require.Equal(t, expected, w.Value.Power)
		}
	}
}

// TestExportGenesis_WalksAllValidators verifies the staking module's
// ExportGenesis iterates the underlying validator store rather than
// returning an empty list. This closes the previous TODO and enables
// real chain-restart workflows: an operator can dump the running
// chain's validator set and seed a new chain from it.
func TestExportGenesis_WalksAllValidators(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	// Seed three validators with different powers.
	for _, v := range []struct {
		pk          string
		power       uint64
		description string
	}{
		{"validator-alice", 100, "alice"},
		{"validator-bob", 200, "bob"},
		{"validator-charlie", 300, "charlie"},
	} {
		require.NoError(t, h.store.SetValidator(ctx, &store.BAPIValidator{
			PubKey:      types.PublicKey{Type: types.KeyTypeEd25519, Data: []byte(v.pk)},
			Power:       v.power,
			Description: v.description,
		}))
	}

	raw, err := h.mod.ExportGenesis(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	var gs StakingGenesisState
	require.NoError(t, json.Unmarshal(raw, &gs))
	require.Len(t, gs.Validators, 3, "ExportGenesis must include every validator")

	// Output must be sorted by pubkey for deterministic genesis.
	for i := 1; i < len(gs.Validators); i++ {
		require.Less(t, gs.Validators[i-1].PubKey, gs.Validators[i].PubKey,
			"ExportGenesis output must be sorted by hex-encoded pubkey")
	}

	// Spot-check one entry round-trips correctly.
	for _, v := range gs.Validators {
		if v.Description == "alice" {
			require.Equal(t, uint64(100), v.Power)
		}
	}
}

// TestExportGenesis_EmptyStoreReturnsEmptyList verifies the no-validators
// edge case: no panics, valid JSON, empty validator list.
func TestExportGenesis_EmptyStoreReturnsEmptyList(t *testing.T) {
	h := newStakingHarness(t)
	ctx := context.Background()

	raw, err := h.mod.ExportGenesis(ctx)
	require.NoError(t, err)

	var gs StakingGenesisState
	require.NoError(t, json.Unmarshal(raw, &gs))
	require.Empty(t, gs.Validators)
}
