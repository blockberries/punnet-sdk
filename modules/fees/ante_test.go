package fees

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noopMsg is a stand-in transaction message used in the ante tests
// so we don't drag a real module (bank) into the fees package.
type noopMsg struct {
	Signer ptypes.AccountName `cramberry:"1" json:"signer"`
	Note   string             `cramberry:"2" json:"note"`
}

func (m *noopMsg) Type() string                       { return "/test.NoopMsg" }
func (m *noopMsg) ValidateBasic() error               { return nil }
func (m *noopMsg) GetSigners() []ptypes.AccountName   { return []ptypes.AccountName{m.Signer} }

func init() {
	ptypes.RegisterMessage("/test.NoopMsg", func() ptypes.Message { return &noopMsg{} })
}

func newAnteTestModule(t *testing.T, sched FeeSchedule) *BAPIFeesModule {
	t.Helper()
	mod := newTestModule(t)
	data, _ := json.Marshal(FeesGenesisState{Schedule: sched})
	require.NoError(t, mod.InitGenesis(context.Background(), data))
	return mod
}

func newTxWithFee(t *testing.T, signer ptypes.AccountName, fee ptypes.Fee) *ptypes.Transaction {
	t.Helper()
	tx := &ptypes.Transaction{
		Account: signer,
		Nonce:   1,
		Messages: []ptypes.Message{
			&noopMsg{Signer: signer, Note: "hello"},
		},
		Authorization: ptypes.NewAuthorization(),
		Fee: fee,
	}
	return tx
}

func TestAnteHandler_HappyPath_EmitsTwoTransfers(t *testing.T) {
	sched := FeeSchedule{
		OpFees:  []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 100}},
		ByteFee: 0, // disable byte fee so the test doesn't need exact sizing
	}
	mod := newAnteTestModule(t, sched)
	ante := NewAnteHandler(mod, "stake")

	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees:   []ptypes.OpFee{{MessageType: "/test.NoopMsg", Amount: 100}},
		Priority: 50,
	})

	eff, err := ante(context.Background(), &runtime.BAPITxContext{}, tx)
	require.NoError(t, err)
	require.Len(t, eff, 2)

	ct, ok := eff[0].(effects.TransferEffect)
	require.True(t, ok)
	assert.Equal(t, ptypes.AccountName("alice"), ct.From)
	assert.Equal(t, FeeTreasuryAccount, ct.To)
	assert.Equal(t, uint64(100), ct.Amount[0].Amount)

	pp, ok := eff[1].(effects.TransferEffect)
	require.True(t, ok)
	assert.Equal(t, ptypes.AccountName("alice"), pp.From)
	assert.Equal(t, PriorityPoolAccount, pp.To)
	assert.Equal(t, uint64(50), pp.Amount[0].Amount)
}

func TestAnteHandler_ZeroPriority_SkipsPPTransfer(t *testing.T) {
	sched := FeeSchedule{
		OpFees: []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 10}},
	}
	mod := newAnteTestModule(t, sched)
	ante := NewAnteHandler(mod, "stake")

	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees:   []ptypes.OpFee{{MessageType: "/test.NoopMsg", Amount: 10}},
		Priority: 0,
	})

	eff, err := ante(context.Background(), &runtime.BAPITxContext{}, tx)
	require.NoError(t, err)
	require.Len(t, eff, 1, "zero-priority tx should not emit PP transfer")
}

func TestAnteHandler_AllZero_NoEffects(t *testing.T) {
	mod := newAnteTestModule(t, FeeSchedule{
		OpFees: []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 0}},
	})
	ante := NewAnteHandler(mod, "stake")

	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees: []ptypes.OpFee{{MessageType: "/test.NoopMsg", Amount: 0}},
	})

	eff, err := ante(context.Background(), &runtime.BAPITxContext{}, tx)
	require.NoError(t, err)
	assert.Empty(t, eff, "all-zero fee should emit no effects")
}

func TestAnteHandler_UnknownMessageType_Rejects(t *testing.T) {
	// Schedule has only "/test.NoopMsg" but the tx carries it — we
	// modify the tx to use a non-registered type.
	sched := FeeSchedule{
		OpFees: []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 100}},
	}
	mod := newAnteTestModule(t, sched)
	ante := NewAnteHandler(mod, "stake")

	// Wrap our message in a tx whose Fee references a type the
	// schedule doesn't know about. The mismatch between Fee.OpFees
	// and msg.Type() trips first.
	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees: []ptypes.OpFee{{MessageType: "/unknown", Amount: 100}},
	})

	_, err := ante(context.Background(), &runtime.BAPITxContext{}, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message_type")
}

func TestAnteHandler_WrongOpFeeAmount_Rejects(t *testing.T) {
	sched := FeeSchedule{
		OpFees: []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 100}},
	}
	mod := newAnteTestModule(t, sched)
	ante := NewAnteHandler(mod, "stake")

	// Schedule says 100; tx submits 50.
	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees: []ptypes.OpFee{{MessageType: "/test.NoopMsg", Amount: 50}},
	})

	_, err := ante(context.Background(), &runtime.BAPITxContext{}, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schedule")
}

func TestAnteHandler_OpFeesLenMismatch_Rejects(t *testing.T) {
	mod := newAnteTestModule(t, FeeSchedule{
		OpFees: []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 1}},
	})
	ante := NewAnteHandler(mod, "stake")

	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees: []ptypes.OpFee{}, // empty, but tx has 1 message
	})

	_, err := ante(context.Background(), &runtime.BAPITxContext{}, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "op_fees length")
}

func TestAnteHandler_ByteFee_UnderpayRejects(t *testing.T) {
	mod := newAnteTestModule(t, FeeSchedule{
		OpFees:  []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 0}},
		ByteFee: 1,
	})
	ante := NewAnteHandler(mod, "stake")

	// 100 wire bytes × 1 rate = required ByteFee 100; submit 50.
	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees:  []ptypes.OpFee{{MessageType: "/test.NoopMsg", Amount: 0}},
		ByteFee: 50,
	})
	txCtx := &runtime.BAPITxContext{TxBytes: make([]byte, 100)}

	_, err := ante(context.Background(), txCtx, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "byte_fee")
}

func TestAnteHandler_ByteFee_OverpayAccepts(t *testing.T) {
	mod := newAnteTestModule(t, FeeSchedule{
		OpFees:  []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 0}},
		ByteFee: 1,
	})
	ante := NewAnteHandler(mod, "stake")

	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees:  []ptypes.OpFee{{MessageType: "/test.NoopMsg", Amount: 0}},
		ByteFee: 200, // 100 required + 100 overpay
	})
	txCtx := &runtime.BAPITxContext{TxBytes: make([]byte, 100)}

	eff, err := ante(context.Background(), txCtx, tx)
	require.NoError(t, err)
	require.Len(t, eff, 1, "expected single CT transfer for overpaid byte fee")
	transfer := eff[0].(effects.TransferEffect)
	assert.Equal(t, uint64(200), transfer.Amount[0].Amount)
}

func TestAnteHandler_ZeroByteFeeSchedule_RejectsNonZeroSubmission(t *testing.T) {
	mod := newAnteTestModule(t, FeeSchedule{
		OpFees:  []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 0}},
		ByteFee: 0,
	})
	ante := NewAnteHandler(mod, "stake")

	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees:  []ptypes.OpFee{{MessageType: "/test.NoopMsg", Amount: 0}},
		ByteFee: 1, // schedule says 0 but submitter is paying — reject
	})

	_, err := ante(context.Background(), &runtime.BAPITxContext{}, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schedule.byte_fee is zero")
}

func TestAnteHandler_DelegatedPayer_UsesPayerAsFromAccount(t *testing.T) {
	mod := newAnteTestModule(t, FeeSchedule{
		OpFees: []OpFeeEntry{{MessageType: "/test.NoopMsg", Amount: 10}},
	})
	ante := NewAnteHandler(mod, "stake")

	tx := newTxWithFee(t, "alice", ptypes.Fee{
		OpFees:   []ptypes.OpFee{{MessageType: "/test.NoopMsg", Amount: 10}},
		Priority: 1,
		Payer:    "bob",
	})
	// We override the message signer set so the implicit validation
	// inside Transaction.ValidateBasic — which the AnteHandler trusts —
	// is consistent: bob must appear in every message's signers when
	// Payer == bob.
	tx.Messages = []ptypes.Message{
		&noopMsg{Signer: "bob"},
	}

	eff, err := ante(context.Background(), &runtime.BAPITxContext{}, tx)
	require.NoError(t, err)
	require.Len(t, eff, 2)
	for _, e := range eff {
		assert.Equal(t, ptypes.AccountName("bob"), e.(effects.TransferEffect).From)
	}
}

func TestAnteHandler_NilTx(t *testing.T) {
	mod := newAnteTestModule(t, FeeSchedule{})
	ante := NewAnteHandler(mod, "stake")
	_, err := ante(context.Background(), &runtime.BAPITxContext{}, nil)
	require.Error(t, err)
}

func TestNewAnteHandler_PanicGuards(t *testing.T) {
	t.Run("nil module", func(t *testing.T) {
		assert.Panics(t, func() { NewAnteHandler(nil, "stake") })
	})
	t.Run("empty base denom", func(t *testing.T) {
		mod := newTestModule(t)
		assert.Panics(t, func() { NewAnteHandler(mod, "") })
	})
}
