package bank

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	bapitypes "github.com/blockberries/bapi/types"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/require"
)

// randomMultiSend builds a MsgMultiSend with a single sender (so the test
// stays focused on output-side determinism — the failure mode of T1-7) and
// 2..8 recipient outputs. Recipients are addressed by short pseudo-random
// names so map iteration order will genuinely differ between Go runs.
func randomMultiSend(rng *rand.Rand, sender ptypes.AccountName) *MsgMultiSend {
	const denom = "stake"
	numOutputs := 2 + rng.Intn(7)

	// Make recipient names distinct.
	seen := make(map[string]struct{}, numOutputs)
	outputs := make([]Output, 0, numOutputs)
	totalNeeded := uint64(0)
	for len(outputs) < numOutputs {
		name := fmt.Sprintf("r%s", randomName(rng, 4))
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		amount := uint64(1 + rng.Intn(100))
		totalNeeded += amount
		outputs = append(outputs, Output{
			Address: ptypes.AccountName(name),
			Coins:   ptypes.Coins{{Denom: denom, Amount: amount}},
		})
	}

	return &MsgMultiSend{
		Inputs: []Input{
			{
				Address: sender,
				Coins:   ptypes.Coins{{Denom: denom, Amount: totalNeeded}},
			},
		},
		Outputs: outputs,
	}
}

func randomName(rng *rand.Rand, n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}

// effectSignature reduces an effects slice to a comparable structural
// fingerprint that ignores nothing — TransferEffect carries account names and
// amounts; EventEffect carries the type — so two runs that disagree on
// ordering will not collide.
func effectSignature(effs []effects.Effect) []string {
	out := make([]string, 0, len(effs))
	for _, e := range effs {
		switch v := e.(type) {
		case effects.TransferEffect:
			out = append(out, fmt.Sprintf("transfer:%s->%s:%v", v.From, v.To, v.Amount))
		case effects.EventEffect:
			out = append(out, fmt.Sprintf("event:%s", v.EventType))
		default:
			out = append(out, fmt.Sprintf("other:%T", e))
		}
	}
	return out
}

// TestHandleMultiSend_DeterministicEffectOrder is the PLAN T1-7 / B1-2
// property test: for 1000 random MsgMultiSend values, two back-to-back
// handler invocations must produce byte-identical effect lists. Previously
// the handler iterated `outputNeeds` (a Go map) directly, so the recipient
// order — and therefore the effect order — depended on Go's map randomization.
func TestHandleMultiSend_DeterministicEffectOrder(t *testing.T) {
	mod, balanceStore := createTestBankModule(t)
	ctx := context.Background()

	const iterations = 1000
	const senderName = "alice"
	// One large balance is enough; the handler reads but the test compares
	// effect lists, not committed state.
	require.NoError(t, balanceStore.Set(ctx, senderName, "stake", 1<<60))

	blockCtx := &runtime.BAPIBlockContext{
		Height:  1,
		Time:    bapitypes.TimeToTimestamp(time.Now()),
		ChainID: "test-chain",
	}
	txCtx := &runtime.BAPITxContext{
		BAPIBlockContext: blockCtx,
		Account:          ptypes.AccountName(senderName),
		TxIndex:          0,
	}

	rng := rand.New(rand.NewSource(0xDEADBEEF))

	for i := 0; i < iterations; i++ {
		msg := randomMultiSend(rng, ptypes.AccountName(senderName))

		first, err := mod.handleMultiSend(ctx, txCtx, msg)
		require.NoErrorf(t, err, "iteration %d: handleMultiSend failed", i)

		second, err := mod.handleMultiSend(ctx, txCtx, msg)
		require.NoErrorf(t, err, "iteration %d: handleMultiSend (second call) failed", i)

		sigA := effectSignature(first)
		sigB := effectSignature(second)
		if !reflect.DeepEqual(sigA, sigB) {
			t.Fatalf("iteration %d: handleMultiSend produced non-deterministic effect order\n  first:  %v\n  second: %v",
				i, sigA, sigB)
		}

		// Also confirm the recipient ordering inside transfer effects is
		// sorted ascending — the canonical order — so different validators
		// running the same code produce the same list. Filter out the
		// trailing EventEffect.
		prev := ""
		for _, e := range first {
			te, ok := e.(effects.TransferEffect)
			if !ok {
				continue
			}
			to := string(te.To)
			if prev != "" && to < prev {
				t.Fatalf("iteration %d: transfer recipients not in sorted order: saw %q after %q\n  full: %v",
					i, to, prev, sigA)
			}
			prev = to
		}
	}
}
