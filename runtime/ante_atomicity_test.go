package runtime

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"testing"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/punnet-sdk/effects"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAppWithAccountAndBalance is setupAppWithAccount + seeds a
// starting balance for alice. The receiver account "fees" is the
// destination of the AnteHandler's TransferEffect; we don't seed it
// (the balance store auto-creates on first credit).
func setupAppWithAccountAndBalance(t *testing.T, aliceStartBalance uint64, msgHandler BAPIMsgHandler) (*BAPIApplication, ed25519.PrivateKey) {
	t.Helper()
	app, privKey := setupAppWithAccount(t)

	// Override the test module's handler.
	if msgHandler != nil {
		// The setup registered a no-op handler for "test/msg"; the
		// router has it. We can't easily replace it via the router's
		// public API without re-creating the app, so instead we
		// re-register a fresh module via the router directly.
		// Bypass: register a second handler under a distinct type
		// and rebuild the tx accordingly. For these tests we always
		// override "test/msg" because that's what buildSignedTx uses.
		app.router.mu.Lock()
		app.router.msgHandlers["test/msg"] = msgHandler
		app.router.mu.Unlock()
	}

	if aliceStartBalance > 0 {
		require.NoError(t, app.balanceStore.Set(context.Background(), "alice", "stake", aliceStartBalance))
	}
	return app, privKey
}

// TestAnteAtomicity_FailedHandlerStillPays verifies spec §3:
// when an AnteHandler emits a transfer effect and the message handler
// subsequently fails, the ante's effect (the fee deduction) survives
// — the tx outcome is "handler failed" but the fee is consumed.
// PLAN §7 Phase 1.5.
func TestAnteAtomicity_FailedHandlerStillPays(t *testing.T) {
	const (
		startBalance = uint64(1000)
		feeAmount    = uint64(100)
	)

	app, privKey := setupAppWithAccountAndBalance(t, startBalance, func(_ context.Context, _ *BAPITxContext, _ ptypes.Message) ([]effects.Effect, error) {
		return nil, fmt.Errorf("simulated handler failure")
	})

	// AnteHandler emits a transfer alice → "module.ct" worth feeAmount.
	app.RegisterAnteHandler(func(_ context.Context, _ *BAPITxContext, tx *ptypes.Transaction) ([]effects.Effect, error) {
		return []effects.Effect{
			effects.TransferEffect{
				From:   tx.Account,
				To:     ModuleAccountCT,
				Amount: ptypes.NewCoins(ptypes.NewCoin("stake", feeAmount)),
			},
		}, nil
	})

	txBytes := buildSignedTx("alice", 0, privKey)
	outcome, err := app.ExecuteBlock(context.Background(), types.FinalizedBlock{
		Height: 1,
		Txs:    []types.Tx{txBytes},
	})
	require.NoError(t, err)
	require.Len(t, outcome.TxOutcomes, 1)
	assert.Equal(t, uint32(4), outcome.TxOutcomes[0].Code,
		"handler failure should produce code 4")

	// Alice should have been debited even though her message failed.
	alice, err := app.balanceStore.GetAmount(context.Background(), "alice", "stake")
	require.NoError(t, err)
	assert.Equal(t, startBalance-feeAmount, alice,
		"failed-handler tx must still pay the fee (Phase 1.5)")

	ct, err := app.balanceStore.GetAmount(context.Background(), string(ModuleAccountCT), "stake")
	require.NoError(t, err)
	assert.Equal(t, feeAmount, ct, "CT should have received the fee")

	// Alice's nonce should have advanced — failed-execution txs
	// burn their nonce slot just like the fee, so the sender can't
	// replay them.
	acct, err := app.accountStore.Get(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, uint64(1), acct.Nonce, "nonce must advance on failed-handler txs")
}

// TestAnteAtomicity_AnteErrorRollsBackNothing verifies that when the
// AnteHandler itself errors, NO state mutation happens (no fee
// deduction, no nonce bump). This is the "fail-closed" branch of
// the atomicity refactor.
func TestAnteAtomicity_AnteErrorRollsBackNothing(t *testing.T) {
	app, privKey := setupAppWithAccountAndBalance(t, 1000, nil)
	app.RegisterAnteHandler(func(_ context.Context, _ *BAPITxContext, _ *ptypes.Transaction) ([]effects.Effect, error) {
		return nil, fmt.Errorf("ante rejection")
	})

	txBytes := buildSignedTx("alice", 0, privKey)
	outcome, err := app.ExecuteBlock(context.Background(), types.FinalizedBlock{
		Height: 1,
		Txs:    []types.Tx{txBytes},
	})
	require.NoError(t, err)
	require.Len(t, outcome.TxOutcomes, 1)
	assert.Equal(t, uint32(20), outcome.TxOutcomes[0].Code)

	// Alice balance unchanged, nonce unchanged.
	alice, err := app.balanceStore.GetAmount(context.Background(), "alice", "stake")
	require.NoError(t, err)
	assert.Equal(t, uint64(1000), alice, "ante error must not debit")

	acct, err := app.accountStore.Get(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, uint64(0), acct.Nonce, "ante error must not bump nonce")
}

// TestAnteAtomicity_HappyPath confirms both ante and handler effects
// commit on the success path.
func TestAnteAtomicity_HappyPath(t *testing.T) {
	const (
		startBalance = uint64(1000)
		feeAmount    = uint64(100)
		moveAmount   = uint64(50)
	)

	// Handler emits a transfer alice → bob.
	app, privKey := setupAppWithAccountAndBalance(t, startBalance, func(_ context.Context, _ *BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
		return []effects.Effect{
			effects.TransferEffect{
				From:   "alice",
				To:     "bob",
				Amount: ptypes.NewCoins(ptypes.NewCoin("stake", moveAmount)),
			},
		}, nil
	})

	app.RegisterAnteHandler(func(_ context.Context, _ *BAPITxContext, tx *ptypes.Transaction) ([]effects.Effect, error) {
		return []effects.Effect{
			effects.TransferEffect{
				From:   tx.Account,
				To:     ModuleAccountCT,
				Amount: ptypes.NewCoins(ptypes.NewCoin("stake", feeAmount)),
			},
		}, nil
	})

	txBytes := buildSignedTx("alice", 0, privKey)
	outcome, err := app.ExecuteBlock(context.Background(), types.FinalizedBlock{
		Height: 1,
		Txs:    []types.Tx{txBytes},
	})
	require.NoError(t, err)
	require.Len(t, outcome.TxOutcomes, 1)
	assert.Equal(t, uint32(0), outcome.TxOutcomes[0].Code,
		"happy path should yield code 0; got %d info=%q",
		outcome.TxOutcomes[0].Code, outcome.TxOutcomes[0].Info)

	alice, _ := app.balanceStore.GetAmount(context.Background(), "alice", "stake")
	assert.Equal(t, startBalance-feeAmount-moveAmount, alice,
		"alice = start - fee - move")
	bob, _ := app.balanceStore.GetAmount(context.Background(), "bob", "stake")
	assert.Equal(t, moveAmount, bob)
	ct, _ := app.balanceStore.GetAmount(context.Background(), string(ModuleAccountCT), "stake")
	assert.Equal(t, feeAmount, ct)
}

