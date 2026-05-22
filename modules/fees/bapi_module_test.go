package fees

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) statestore.StateStore {
	t.Helper()
	ss, err := statestore.NewMemoryIAVLStore(1000)
	require.NoError(t, err)
	return ss
}

func newTestModule(t *testing.T) *BAPIFeesModule {
	t.Helper()
	mod, err := NewBAPIFeesModule(newTestStore(t))
	require.NoError(t, err)
	return mod
}

func sampleSchedule() FeeSchedule {
	return FeeSchedule{
		OpFees: []OpFeeEntry{
			{MessageType: "/bank.MsgSend", Amount: 100},
			{MessageType: "/staking.MsgDelegate", Amount: 250},
		},
		ByteFee: 2,
	}
}

func TestNewBAPIFeesModule(t *testing.T) {
	t.Run("constructs with non-nil state store", func(t *testing.T) {
		mod := newTestModule(t)
		assert.Equal(t, ModuleName, mod.Name())
	})

	t.Run("rejects nil state store", func(t *testing.T) {
		_, err := NewBAPIFeesModule(nil)
		assert.Error(t, err)
	})
}

func TestFeesModule_RegisterHandlers(t *testing.T) {
	mod := newTestModule(t)

	t.Run("registers MsgProposeFeeUpdate", func(t *testing.T) {
		h := mod.RegisterMsgHandlers()
		require.Len(t, h, 1)
		_, ok := h[TypeMsgProposeFeeUpdate]
		assert.True(t, ok)
	})

	t.Run("registers four query paths", func(t *testing.T) {
		h := mod.RegisterQueryHandlers()
		assert.Len(t, h, 4)
		for _, path := range []string{"/fees/schedule", "/fees/pending", "/fees/op_fee", "/fees/byte_fee"} {
			_, ok := h[path]
			assert.True(t, ok, "missing handler for %s", path)
		}
	})
}

func TestFeesModule_GenesisRoundTrip(t *testing.T) {
	mod := newTestModule(t)
	ctx := context.Background()

	gs := FeesGenesisState{
		Schedule: sampleSchedule(),
		Pending: []PendingFeeUpdate{
			{
				EffectiveHeight: 100,
				Schedule: FeeSchedule{
					OpFees:  []OpFeeEntry{{MessageType: "/bank.MsgSend", Amount: 50}},
					ByteFee: 1,
				},
			},
		},
	}
	data, err := json.Marshal(gs)
	require.NoError(t, err)

	require.NoError(t, mod.InitGenesis(ctx, data))

	exported, err := mod.ExportGenesis(ctx)
	require.NoError(t, err)

	var got FeesGenesisState
	require.NoError(t, json.Unmarshal(exported, &got))
	assert.Equal(t, gs.Schedule, got.Schedule)
	require.Len(t, got.Pending, 1)
	assert.Equal(t, uint64(100), got.Pending[0].EffectiveHeight)
}

func TestFeesModule_InitGenesis_EmptyData(t *testing.T) {
	mod := newTestModule(t)
	ctx := context.Background()

	require.NoError(t, mod.InitGenesis(ctx, nil))

	sched, err := mod.Schedule(ctx)
	require.NoError(t, err)
	assert.Empty(t, sched.OpFees, "empty genesis installs zero schedule")
	assert.Equal(t, uint64(0), sched.ByteFee)
}

func TestFeesModule_InitGenesis_SortsOpFees(t *testing.T) {
	mod := newTestModule(t)
	ctx := context.Background()

	gs := FeesGenesisState{
		Schedule: FeeSchedule{
			OpFees: []OpFeeEntry{
				{MessageType: "/z.Msg", Amount: 99},
				{MessageType: "/a.Msg", Amount: 1},
				{MessageType: "/m.Msg", Amount: 50},
			},
		},
	}
	data, _ := json.Marshal(gs)
	require.NoError(t, mod.InitGenesis(ctx, data))

	sched, err := mod.Schedule(ctx)
	require.NoError(t, err)
	require.Len(t, sched.OpFees, 3)
	assert.Equal(t, "/a.Msg", sched.OpFees[0].MessageType)
	assert.Equal(t, "/m.Msg", sched.OpFees[1].MessageType)
	assert.Equal(t, "/z.Msg", sched.OpFees[2].MessageType)
}

func TestFeesModule_InitGenesis_RejectsInvalid(t *testing.T) {
	mod := newTestModule(t)
	ctx := context.Background()

	// Empty message type is invalid even after sort.
	gs := FeesGenesisState{
		Schedule: FeeSchedule{
			OpFees: []OpFeeEntry{{MessageType: "", Amount: 1}},
		},
	}
	data, _ := json.Marshal(gs)
	assert.Error(t, mod.InitGenesis(ctx, data))
}

func TestFeesModule_BeginBlock_ActivatesPending(t *testing.T) {
	mod := newTestModule(t)
	ctx := context.Background()

	// Seed via genesis: initial schedule + two pending updates at h=10
	// and h=20.
	gs := FeesGenesisState{
		Schedule: sampleSchedule(),
		Pending: []PendingFeeUpdate{
			{
				EffectiveHeight: 10,
				Schedule:        FeeSchedule{OpFees: []OpFeeEntry{{MessageType: "/a", Amount: 1}}, ByteFee: 10},
			},
			{
				EffectiveHeight: 20,
				Schedule:        FeeSchedule{OpFees: []OpFeeEntry{{MessageType: "/a", Amount: 2}}, ByteFee: 20},
			},
		},
	}
	data, _ := json.Marshal(gs)
	require.NoError(t, mod.InitGenesis(ctx, data))

	// Height 5: nothing due, schedule unchanged.
	_, err := mod.BeginBlock(ctx, &runtime.BAPIBlockContext{Height: 5})
	require.NoError(t, err)
	sched, _ := mod.Schedule(ctx)
	assert.Equal(t, sampleSchedule().ByteFee, sched.ByteFee)

	// Height 10: the h=10 update activates.
	_, err = mod.BeginBlock(ctx, &runtime.BAPIBlockContext{Height: 10})
	require.NoError(t, err)
	sched, _ = mod.Schedule(ctx)
	assert.Equal(t, uint64(10), sched.ByteFee)

	// Height 25: the h=20 update (still pending after the h=10
	// activation removed only the h=10 entry) activates.
	_, err = mod.BeginBlock(ctx, &runtime.BAPIBlockContext{Height: 25})
	require.NoError(t, err)
	sched, _ = mod.Schedule(ctx)
	assert.Equal(t, uint64(20), sched.ByteFee)

	// All pending entries should now be drained.
	exported, _ := mod.ExportGenesis(ctx)
	var out FeesGenesisState
	require.NoError(t, json.Unmarshal(exported, &out))
	assert.Empty(t, out.Pending)
}

func TestFeesModule_BeginBlock_HighestWins(t *testing.T) {
	mod := newTestModule(t)
	ctx := context.Background()

	// Two pending updates at h=10 and h=15. A BeginBlock at h=20
	// sees both as due — the higher-EffectiveHeight schedule wins.
	gs := FeesGenesisState{
		Schedule: sampleSchedule(),
		Pending: []PendingFeeUpdate{
			{EffectiveHeight: 10, Schedule: FeeSchedule{ByteFee: 10}},
			{EffectiveHeight: 15, Schedule: FeeSchedule{ByteFee: 15}},
		},
	}
	data, _ := json.Marshal(gs)
	require.NoError(t, mod.InitGenesis(ctx, data))

	_, err := mod.BeginBlock(ctx, &runtime.BAPIBlockContext{Height: 20})
	require.NoError(t, err)
	sched, _ := mod.Schedule(ctx)
	assert.Equal(t, uint64(15), sched.ByteFee)
}

func TestFeesModule_QuerySchedule(t *testing.T) {
	mod := newTestModule(t)
	ctx := context.Background()

	gs := FeesGenesisState{Schedule: sampleSchedule()}
	data, _ := json.Marshal(gs)
	require.NoError(t, mod.InitGenesis(ctx, data))

	handlers := mod.RegisterQueryHandlers()

	t.Run("schedule", func(t *testing.T) {
		out, err := handlers["/fees/schedule"](ctx, nil, 0)
		require.NoError(t, err)
		var got FeeSchedule
		require.NoError(t, json.Unmarshal(out, &got))
		assert.Equal(t, sampleSchedule(), got)
	})

	t.Run("op_fee hit", func(t *testing.T) {
		out, err := handlers["/fees/op_fee"](ctx, []byte("/bank.MsgSend"), 0)
		require.NoError(t, err)
		var got uint64
		require.NoError(t, json.Unmarshal(out, &got))
		assert.Equal(t, uint64(100), got)
	})

	t.Run("op_fee miss returns zero", func(t *testing.T) {
		out, err := handlers["/fees/op_fee"](ctx, []byte("/unknown"), 0)
		require.NoError(t, err)
		var got uint64
		require.NoError(t, json.Unmarshal(out, &got))
		assert.Equal(t, uint64(0), got)
	})

	t.Run("byte_fee", func(t *testing.T) {
		out, err := handlers["/fees/byte_fee"](ctx, nil, 0)
		require.NoError(t, err)
		var got uint64
		require.NoError(t, json.Unmarshal(out, &got))
		assert.Equal(t, uint64(2), got)
	})

	t.Run("op_fee requires non-empty data", func(t *testing.T) {
		_, err := handlers["/fees/op_fee"](ctx, nil, 0)
		assert.Error(t, err)
	})
}

func TestFeesModule_HandleProposeFeeUpdate_Rejected(t *testing.T) {
	mod := newTestModule(t)
	ctx := context.Background()
	require.NoError(t, mod.InitGenesis(ctx, nil))

	h := mod.RegisterMsgHandlers()[TypeMsgProposeFeeUpdate]
	_, err := h(ctx, nil, &MsgProposeFeeUpdate{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet enabled")
}

func TestMsgProposeFeeUpdate_ValidateBasic(t *testing.T) {
	good := &MsgProposeFeeUpdate{
		Proposer:        "alice",
		Schedule:        sampleSchedule(),
		EffectiveHeight: 100,
	}
	require.NoError(t, good.ValidateBasic())

	t.Run("invalid proposer", func(t *testing.T) {
		m := *good
		m.Proposer = ""
		assert.Error(t, m.ValidateBasic())
	})

	t.Run("zero effective height", func(t *testing.T) {
		m := *good
		m.EffectiveHeight = 0
		assert.Error(t, m.ValidateBasic())
	})

	t.Run("duplicate op_fee message types", func(t *testing.T) {
		m := *good
		m.Schedule = FeeSchedule{
			OpFees: []OpFeeEntry{
				{MessageType: "/x", Amount: 1},
				{MessageType: "/x", Amount: 2},
			},
		}
		err := m.ValidateBasic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicated")
	})
}
