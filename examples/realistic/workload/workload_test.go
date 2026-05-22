package workload

import (
	"context"
	"crypto/ed25519"
	"testing"

	"github.com/blockberries/punnet-sdk/examples/realistic/app"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateBurst_PriorityDistribution: the generator emits N
// txs and the per-bucket counts roughly match the configured
// weights.
func TestGenerateBurst_PriorityDistribution(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	_ = pubKey

	dist := PriorityDist{
		Buckets:       []int64{0, 50, 100},
		BucketWeights: []int{50, 30, 20}, // 50% / 30% / 20%
	}
	const N = 1000
	txs, err := GenerateBurst(N, "alice", privKey, 0, dist)
	require.NoError(t, err)
	require.Len(t, txs, N)

	counts := map[int64]int{}
	for _, tx := range txs {
		counts[tx.Priority]++
	}
	// Round-robin weighted picks should land near the configured
	// percentages. Tolerance ±5%.
	assert.InDelta(t, 500, counts[0], 50, "~50%% at priority 0")
	assert.InDelta(t, 300, counts[50], 50, "~30%% at priority 50")
	assert.InDelta(t, 200, counts[100], 50, "~20%% at priority 100")
}

// TestRunBurst_PriorityOrdering exercises the full pipeline:
// build a 200-tx burst against the realistic app, submit it in
// SortByPriorityDesc order, and assert that higher-priority txs
// landed in earlier (lower block × seq) positions.
//
// This is the PLAN §7 Phase 6 acceptance criterion at unit-test
// scale (200 not 100K — the same code at 100K is the stress
// target).
func TestRunBurst_PriorityOrdering(t *testing.T) {
	a, err := app.Build()
	require.NoError(t, err)

	pubKey, privKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)

	// Seed alice with enough balance + create the alice account.
	ctx := context.Background()
	const startBalance uint64 = 10_000_000_000
	require.NoError(t, a.BalanceStore.Set(ctx, "alice", "stake", startBalance))
	require.NoError(t, a.AccountStore.Set(ctx, &ptypes.Account{
		Name:      "alice",
		Authority: ptypes.NewAuthority(1, pubKey, 1),
		Nonce:     0,
	}))

	dist := PriorityDist{
		Buckets:       []int64{0, 100},
		BucketWeights: []int{50, 50},
	}
	const N = 200
	txs, err := GenerateBurst(N, "alice", privKey, 0, dist)
	require.NoError(t, err)

	// Submit highest-priority first so the included-in-block order
	// reflects priority. The runner submits in input order; sorting
	// here is the "ideal mempool" simulation.
	sorted := SortByPriorityDesc(txs)
	stats, txsPost, err := RunBurst(a.App, sorted, 50)
	require.NoError(t, err)

	// All txs committed? They may not all execute successfully
	// (delegations to non-existent validators etc.), but they
	// must all be PROCESSED (Block != 0).
	for i, tx := range txsPost {
		assert.NotZero(t, tx.Block, "tx %d not processed", i)
	}
	assert.Equal(t, N, stats.TotalTx)
	assert.Equal(t, N, stats.CommittedTx)

	// The PLAN §7 Phase 6 acceptance: high-priority txs land in
	// earlier (lower) block × seq positions than low-priority.
	highPri := stats.PerPriority[100]
	lowPri := stats.PerPriority[0]
	require.NotZero(t, highPri.Count)
	require.NotZero(t, lowPri.Count)
	highPos := highPri.AverageBlock*100 + highPri.AverageSeq
	lowPos := lowPri.AverageBlock*100 + lowPri.AverageSeq
	assert.Less(t, highPos, lowPos,
		"priority-100 average inclusion position %.2f must be earlier than priority-0 %.2f",
		highPos, lowPos)
}

// TestGenerateBurst_RejectsMalformedDist.
func TestGenerateBurst_RejectsMalformedDist(t *testing.T) {
	_, privKey, _ := ed25519.GenerateKey(nil)
	cases := []PriorityDist{
		{},
		{Buckets: []int64{0}, BucketWeights: nil},
		{Buckets: []int64{0}, BucketWeights: []int{0}},
	}
	for _, d := range cases {
		_, err := GenerateBurst(10, "alice", privKey, 0, d)
		assert.Error(t, err)
	}
}
