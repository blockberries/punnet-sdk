package types

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// signDocMsgA is a message type with a heterogeneous mix of fields (including
// a map) so that the determinism test actually exercises cramberry's sorted
// map-key path — the failure mode T1-6 was rooted in Go map iteration order.
type signDocMsgA struct {
	Field    string            `json:"field"`
	Amount   uint64            `json:"amount"`
	Metadata map[string]string `json:"metadata"`
}

func (m *signDocMsgA) Type() string              { return "test/SignDocMsgA" }
func (m *signDocMsgA) ValidateBasic() error      { return nil }
func (m *signDocMsgA) GetSigners() []AccountName { return []AccountName{"alice"} }

// signDocMsgB exercises a different shape (a numeric map) so the property
// test covers more than one structural pattern.
type signDocMsgB struct {
	Counters map[uint32]int64 `json:"counters"`
	Flag     bool             `json:"flag"`
}

func (m *signDocMsgB) Type() string              { return "test/SignDocMsgB" }
func (m *signDocMsgB) ValidateBasic() error      { return nil }
func (m *signDocMsgB) GetSigners() []AccountName { return []AccountName{"alice"} }

func init() {
	RegisterMessage("test/SignDocMsgA", func() Message { return &signDocMsgA{} })
	RegisterMessage("test/SignDocMsgB", func() Message { return &signDocMsgB{} })
}

// randomTx generates a pseudo-random Transaction whose messages contain maps
// with multiple keys — the regression target of T1-6.
func randomTx(rng *rand.Rand) *Transaction {
	numMsgs := 1 + rng.Intn(3)
	msgs := make([]Message, numMsgs)
	for i := range msgs {
		switch rng.Intn(2) {
		case 0:
			metaLen := 1 + rng.Intn(8)
			meta := make(map[string]string, metaLen)
			for j := 0; j < metaLen; j++ {
				meta[randString(rng, 1+rng.Intn(8))] = randString(rng, 1+rng.Intn(8))
			}
			msgs[i] = &signDocMsgA{
				Field:    randString(rng, 1+rng.Intn(16)),
				Amount:   rng.Uint64(),
				Metadata: meta,
			}
		default:
			ctrLen := 1 + rng.Intn(8)
			ctr := make(map[uint32]int64, ctrLen)
			for j := 0; j < ctrLen; j++ {
				ctr[rng.Uint32()] = rng.Int63()
			}
			msgs[i] = &signDocMsgB{Counters: ctr, Flag: rng.Intn(2) == 0}
		}
	}

	return &Transaction{
		Account:  AccountName("acct" + randString(rng, 4)),
		Messages: msgs,
		Nonce:    rng.Uint64(),
		Memo:     randString(rng, rng.Intn(32)),
	}
}

func randString(rng *rand.Rand, n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}

// TestGetSignBytes_DeterministicAcrossRuns is the property-based regression for
// PLAN T1-6: 1000 random transactions, each producing byte-identical SignDoc
// bytes across two independent runs (different Go map iteration order is the
// failure mode it pins down).
func TestGetSignBytes_DeterministicAcrossRuns(t *testing.T) {
	const iterations = 1000
	rng := rand.New(rand.NewSource(0xC0FFEE))

	for i := 0; i < iterations; i++ {
		tx := randomTx(rng)

		// First computation.
		got1 := tx.GetSignBytes()

		// Second computation — must be byte-identical. If GetSignBytes is
		// non-deterministic (e.g. relies on Go map iteration), this comparison
		// will eventually fail.
		got2 := tx.GetSignBytes()

		if !bytes.Equal(got1, got2) {
			t.Fatalf("iteration %d: GetSignBytes is non-deterministic\n  got1=%x\n  got2=%x", i, got1, got2)
		}

		// Third computation via a fresh Transaction value (same fields) to
		// confirm that determinism survives transaction-identity changes.
		txCopy := &Transaction{
			Account:  tx.Account,
			Messages: tx.Messages,
			Nonce:    tx.Nonce,
			Memo:     tx.Memo,
		}
		got3 := txCopy.GetSignBytes()
		if !bytes.Equal(got1, got3) {
			t.Fatalf("iteration %d: GetSignBytes differs across Tx instances\n  got1=%x\n  got3=%x", i, got1, got3)
		}
	}
}

// TestGetSignBytes_MapOrderIndependent covers the specific T1-6 scenario: two
// logically-equal MsgA values whose metadata maps were populated in different
// orders MUST produce the same SignDoc.
func TestGetSignBytes_MapOrderIndependent(t *testing.T) {
	tx1 := &Transaction{
		Account: "alice",
		Nonce:   1,
		Messages: []Message{&signDocMsgA{
			Field:    "x",
			Amount:   42,
			Metadata: map[string]string{"a": "1", "b": "2", "c": "3"},
		}},
	}
	tx2 := &Transaction{
		Account: "alice",
		Nonce:   1,
		Messages: []Message{&signDocMsgA{
			Field:    "x",
			Amount:   42,
			Metadata: map[string]string{"c": "3", "b": "2", "a": "1"},
		}},
	}

	require.Equal(t, tx1.GetSignBytes(), tx2.GetSignBytes(),
		"SignDoc must be invariant under Go map insertion order")
}

// TestGetSignBytes_DifferentMessagesDiffer is the negative case: two
// transactions that differ only in a single message field must produce
// different SignDoc bytes. The cramberry encoder is deterministic by spec, so
// this confirms collisions are not introduced by the encoding swap.
func TestGetSignBytes_DifferentMessagesDiffer(t *testing.T) {
	tx1 := &Transaction{
		Account:  "alice",
		Nonce:    1,
		Messages: []Message{&signDocMsgA{Field: "a", Amount: 1}},
	}
	tx2 := &Transaction{
		Account:  "alice",
		Nonce:    1,
		Messages: []Message{&signDocMsgA{Field: "b", Amount: 1}},
	}
	assert.NotEqual(t, tx1.GetSignBytes(), tx2.GetSignBytes())
}

// TestTransactionHash_DistinguishesByNonce pins the post-PLAN-T1-6-followup
// fix to Transaction.Hash: previously it hashed only `account || msg.Type()`,
// so two TXs differing only in nonce had identical hashes — a real
// collision class for mempool dedup. The new implementation routes
// through GetSignBytes, which covers account + nonce + per-msg-body +
// memo via cramberry encoding.
func TestTransactionHash_DistinguishesByNonce(t *testing.T) {
	tx1 := &Transaction{
		Account: "alice",
		Nonce:   1,
		Messages: []Message{&signDocMsgA{
			Field:    "transfer",
			Amount:   100,
			Metadata: map[string]string{"memo": "first"},
		}},
	}
	tx2 := &Transaction{
		Account: "alice",
		Nonce:   2, // only difference
		Messages: []Message{&signDocMsgA{
			Field:    "transfer",
			Amount:   100,
			Metadata: map[string]string{"memo": "first"},
		}},
	}
	assert.NotEqual(t, tx1.Hash(), tx2.Hash(),
		"Hash must distinguish TXs that differ only in nonce")
}

// TestTransactionHash_DistinguishesByMessageBody pins the other half of
// the same fix: the old implementation didn't hash message bodies at all
// (only msg.Type()), so two TXs from the same account with the same
// message type but different bodies collided. The new implementation
// includes the cramberry-encoded body.
func TestTransactionHash_DistinguishesByMessageBody(t *testing.T) {
	mkTx := func(amount uint64) *Transaction {
		return &Transaction{
			Account: "alice",
			Nonce:   0,
			Messages: []Message{&signDocMsgA{
				Field:    "transfer",
				Amount:   amount,
				Metadata: map[string]string{"memo": "same"},
			}},
		}
	}
	if bytes.Equal(mkTx(100).Hash(), mkTx(200).Hash()) {
		t.Fatal("Hash must distinguish TXs whose messages differ only in body")
	}
}

// TestTransactionHash_DistinguishesByFee guards the property that the
// signed payload includes the Fee — a submitter cannot mutate the Fee
// after signing without invalidating the signature. PLAN §7 decision
// D8; Phase 0.1.
func TestTransactionHash_DistinguishesByFee(t *testing.T) {
	mkTx := func(fee Fee) *Transaction {
		return &Transaction{
			Account: "alice",
			Nonce:   0,
			Messages: []Message{&signDocMsgA{Field: "transfer", Amount: 100}},
			Fee:      fee,
		}
	}
	base := mkTx(Fee{Priority: 0})
	diffPriority := mkTx(Fee{Priority: 1000})
	diffByteFee := mkTx(Fee{ByteFee: 50})
	diffOpFees := mkTx(Fee{OpFees: []OpFee{{MessageType: "test/SignDocMsgA", Amount: 1}}})

	if bytes.Equal(base.Hash(), diffPriority.Hash()) {
		t.Error("Hash must distinguish TXs that differ only in Fee.Priority")
	}
	if bytes.Equal(base.Hash(), diffByteFee.Hash()) {
		t.Error("Hash must distinguish TXs that differ only in Fee.ByteFee")
	}
	if bytes.Equal(base.Hash(), diffOpFees.Hash()) {
		t.Error("Hash must distinguish TXs that differ only in Fee.OpFees")
	}
}
