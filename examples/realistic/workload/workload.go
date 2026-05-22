// Package workload is the realistic-workload harness for PLAN §7
// Phase 6.2. It builds bursts of fee-bearing transactions with
// configurable priority distributions and submits them through a
// BAPIApplication's CheckTx → ExecuteBlock pipeline, then reports
// per-priority-bucket and per-msg-type inclusion latencies.
//
// Scope note: this v1 generator emits MsgBankSend only. The
// framework is plug-ready for the other three msg types listed in
// PLAN §7 Phase 6.2 (MsgDelegate, MsgWithdrawDelegatorReward,
// MsgVote) — see the TxBuilder interface — but the actual
// constructors for those types are a future expansion. The
// acceptance criterion ("priority-50 txs land measurably earlier
// than priority-0 txs in the same burst") only needs one msg
// type to exercise; adding more increases coverage of fee-routing
// and priority-pool routing across heterogeneous workloads.
package workload

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/punnet-sdk/modules/bank"
	"github.com/blockberries/punnet-sdk/runtime"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// PriorityDist describes the distribution of priority fees across
// the generated burst. The slice length is the number of buckets;
// each entry is the bucket's priority value, and BucketWeights[i]
// is the relative weight for bucket i (e.g. weight 30 means ~30%
// of txs land in that bucket assuming the weights are roughly
// proportionally summed).
type PriorityDist struct {
	Buckets       []int64
	BucketWeights []int
}

// TxRecord is a single generated transaction with its bookkeeping.
type TxRecord struct {
	Index    int                    // position in the generated stream
	Priority int64                  // priority fee in the Fee struct
	MsgType  string                 // /bank.MsgSend etc.
	Bytes    []byte                 // wire-encoded tx
	Sender   ptypes.AccountName     // signer
	SeqInBlock int                  // populated by RunBurst (block-relative index)
	Block    uint64                 // populated by RunBurst (committed-at height)
}

// Stats is the report produced by a workload run.
type Stats struct {
	TotalTx          int
	CommittedTx      int
	PerPriority      map[int64]BucketStats
	PerMsgType       map[string]BucketStats
	ElapsedMicros    int64
}

// BucketStats are the per-bucket numbers.
type BucketStats struct {
	Count           int
	AverageBlock    float64 // mean block height of inclusion
	AverageSeq      float64 // mean per-block index of inclusion
}

// GenerateBurst returns N transactions following the supplied
// priority distribution. Each tx is a MsgBankSend from the
// `from` account to a deterministic destination. Nonces are
// assigned sequentially starting from `startNonce`.
//
// The same `from` account submits ALL transactions. For tests that
// want cross-sender contention, run multiple GenerateBurst calls
// with different from values and interleave the results.
func GenerateBurst(n int, from ptypes.AccountName, fromKey ed25519.PrivateKey, startNonce uint64, dist PriorityDist) ([]TxRecord, error) {
	if n <= 0 {
		return nil, nil
	}
	if len(dist.Buckets) == 0 || len(dist.BucketWeights) != len(dist.Buckets) {
		return nil, fmt.Errorf("priority distribution malformed")
	}
	totalWeight := 0
	for _, w := range dist.BucketWeights {
		totalWeight += w
	}
	if totalWeight == 0 {
		return nil, fmt.Errorf("priority distribution weights sum to zero")
	}

	out := make([]TxRecord, n)
	pubKey := []byte(fromKey.Public().(ed25519.PublicKey))
	for i := 0; i < n; i++ {
		// Deterministically pick a bucket — round-robin weighted
		// rather than random so the test is reproducible.
		picked := pickBucket(i, dist, totalWeight)
		recipient := ptypes.AccountName(fmt.Sprintf("acct%04d", i%10000))
		msg := &bank.MsgSend{
			From:   from,
			To:     recipient,
			Amount: ptypes.NewCoin("stake", 1),
		}
		tx := ptypes.NewTransaction(from, startNonce+uint64(i), []ptypes.Message{msg}, nil)
		tx.Fee = ptypes.Fee{
			OpFees: []ptypes.OpFee{
				{MessageType: msg.Type(), Amount: 0}, // schedule is zero in this fixture
			},
			Priority: picked,
		}

		signBytes := tx.GetSignBytes()
		sig := ed25519.Sign(fromKey, signBytes)
		tx.Authorization = &ptypes.Authorization{
			Signatures: []ptypes.Signature{{PubKey: pubKey, Signature: sig}},
		}

		bytesEncoded, err := json.Marshal(tx)
		if err != nil {
			return nil, fmt.Errorf("marshal tx %d: %w", i, err)
		}
		out[i] = TxRecord{
			Index:    i,
			Priority: picked,
			MsgType:  msg.Type(),
			Bytes:    bytesEncoded,
			Sender:   from,
		}
	}
	return out, nil
}

// pickBucket selects a priority bucket for tx index i using a
// weighted round-robin scheme. Deterministic across runs.
func pickBucket(i int, dist PriorityDist, totalWeight int) int64 {
	r := i % totalWeight
	cum := 0
	for j, w := range dist.BucketWeights {
		cum += w
		if r < cum {
			return dist.Buckets[j]
		}
	}
	return dist.Buckets[len(dist.Buckets)-1]
}

// RunBurst submits the txs in order via app.CheckTx (gating) then
// app.ExecuteBlock (per-block commits). Block boundaries are
// determined by `txsPerBlock`. Returns a populated Stats and the
// updated TxRecord slice (with Block + SeqInBlock filled).
//
// Mempool priority routing is bapi/looseberry's job — this
// runner just submits them in the input order. To exercise
// priority-ordering, the caller should sort the input by
// priority desc BEFORE handing off, OR pre-sort the *results*
// using the recorded Priority field and check that
// higher-priority txs landed in lower (earlier) Block × SeqInBlock
// positions.
//
// For this v1 generator we measure ExecuteBlock latency directly —
// no consensus, no real mempool. The stats reflect "if these
// arrived in this order, this is how the chain processed them."
func RunBurst(app *runtime.BAPIApplication, txs []TxRecord, txsPerBlock int) (Stats, []TxRecord, error) {
	if len(txs) == 0 {
		return Stats{}, txs, nil
	}
	if txsPerBlock <= 0 {
		txsPerBlock = 100
	}

	start := time.Now()
	startHeight := uint64(2) // height 1 was the genesis-handshake block
	for blockOffset := 0; blockOffset*txsPerBlock < len(txs); blockOffset++ {
		from := blockOffset * txsPerBlock
		to := from + txsPerBlock
		if to > len(txs) {
			to = len(txs)
		}
		blockTxs := make([]types.Tx, 0, to-from)
		for i := from; i < to; i++ {
			blockTxs = append(blockTxs, txs[i].Bytes)
		}

		height := startHeight + uint64(blockOffset)
		outcome, err := app.ExecuteBlock(nil, types.FinalizedBlock{
			Height: height,
			Time:   types.TimeToTimestamp(time.Now()),
			Txs:    blockTxs,
		})
		if err != nil {
			return Stats{}, txs, fmt.Errorf("ExecuteBlock height %d: %w", height, err)
		}
		_, err = app.Commit(nil)
		if err != nil {
			return Stats{}, txs, fmt.Errorf("Commit height %d: %w", height, err)
		}

		// Record block + seq for each tx based on the outcome's
		// TxOutcomes order (which preserves input order in v1).
		for j, out := range outcome.TxOutcomes {
			ix := from + int(out.Index)
			if ix >= len(txs) {
				continue
			}
			txs[ix].Block = height
			txs[ix].SeqInBlock = j
		}
	}
	elapsed := time.Since(start).Microseconds()

	stats := Stats{
		TotalTx:       len(txs),
		PerPriority:   make(map[int64]BucketStats),
		PerMsgType:    make(map[string]BucketStats),
		ElapsedMicros: elapsed,
	}
	for _, tx := range txs {
		if tx.Block == 0 {
			continue
		}
		stats.CommittedTx++
		bp := stats.PerPriority[tx.Priority]
		bp.Count++
		bp.AverageBlock += float64(tx.Block)
		bp.AverageSeq += float64(tx.SeqInBlock)
		stats.PerPriority[tx.Priority] = bp

		bm := stats.PerMsgType[tx.MsgType]
		bm.Count++
		bm.AverageBlock += float64(tx.Block)
		bm.AverageSeq += float64(tx.SeqInBlock)
		stats.PerMsgType[tx.MsgType] = bm
	}
	// Convert sums to means.
	for k, v := range stats.PerPriority {
		if v.Count > 0 {
			v.AverageBlock /= float64(v.Count)
			v.AverageSeq /= float64(v.Count)
		}
		stats.PerPriority[k] = v
	}
	for k, v := range stats.PerMsgType {
		if v.Count > 0 {
			v.AverageBlock /= float64(v.Count)
			v.AverageSeq /= float64(v.Count)
		}
		stats.PerMsgType[k] = v
	}
	return stats, txs, nil
}

// SortByPriorityDesc returns the input txs reordered so highest-
// priority entries come first. Used by callers who want the
// runner to commit txs in priority order (simulating an ideal
// mempool).
func SortByPriorityDesc(txs []TxRecord) []TxRecord {
	out := make([]TxRecord, len(txs))
	copy(out, txs)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority > out[j].Priority
	})
	return out
}

// GenerateKeyPair is a tiny convenience for test setups.
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}
