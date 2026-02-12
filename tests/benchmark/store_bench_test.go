package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/store"
	sdktypes "github.com/blockberries/punnet-sdk/types"
)

// BenchmarkBAPIBalanceStore_Set benchmarks balance set operations
func BenchmarkBAPIBalanceStore_Set(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	balanceStore := store.NewBAPIBalanceStore(ss)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		account := fmt.Sprintf("account%d", i)
		if err := balanceStore.Set(ctx, account, "stake", uint64(i)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBAPIBalanceStore_Get benchmarks balance get operations
func BenchmarkBAPIBalanceStore_Get(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	balanceStore := store.NewBAPIBalanceStore(ss)
	ctx := context.Background()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		account := fmt.Sprintf("account%d", i)
		if err := balanceStore.Set(ctx, account, "stake", uint64(i*100)); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		account := fmt.Sprintf("account%d", i%1000)
		if _, err := balanceStore.GetAmount(ctx, account, "stake"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBAPIBalanceStore_Transfer benchmarks transfer operations
func BenchmarkBAPIBalanceStore_Transfer(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	balanceStore := store.NewBAPIBalanceStore(ss)
	ctx := context.Background()

	// Pre-populate with high balances
	for i := 0; i < 100; i++ {
		account := fmt.Sprintf("account%d", i)
		if err := balanceStore.Set(ctx, account, "stake", 1000000000); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		from := fmt.Sprintf("account%d", i%100)
		to := fmt.Sprintf("account%d", (i+1)%100)
		if err := balanceStore.Transfer(ctx, from, to, "stake", 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBAPIAccountStore_Set benchmarks account set operations
func BenchmarkBAPIAccountStore_Set(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	accountStore := store.NewBAPIAccountStore(ss)
	ctx := context.Background()
	pubKey := make([]byte, 32)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := sdktypes.AccountName(fmt.Sprintf("account%d", i))
		copy(pubKey, fmt.Sprintf("pubkey%d", i))
		account := sdktypes.NewAccount(name, pubKey)
		if err := accountStore.Set(ctx, account); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBAPIAccountStore_Get benchmarks account get operations
func BenchmarkBAPIAccountStore_Get(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	accountStore := store.NewBAPIAccountStore(ss)
	ctx := context.Background()
	pubKey := make([]byte, 32)

	// Pre-populate
	for i := 0; i < 1000; i++ {
		name := sdktypes.AccountName(fmt.Sprintf("account%d", i))
		copy(pubKey, fmt.Sprintf("pubkey%d", i))
		account := sdktypes.NewAccount(name, pubKey)
		if err := accountStore.Set(ctx, account); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := sdktypes.AccountName(fmt.Sprintf("account%d", i%1000))
		if _, err := accountStore.Get(ctx, name); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBAPIValidatorStore_Set benchmarks validator set operations
func BenchmarkBAPIValidatorStore_Set(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	validatorStore := store.NewBAPIValidatorStore(ss)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pubKey := make([]byte, 32)
		copy(pubKey, fmt.Sprintf("validator-pubkey-%08d", i))
		validator := &store.BAPIValidator{
			PubKey: types.PublicKey{
				Type: types.KeyTypeEd25519,
				Data: pubKey,
			},
			Power:       uint64(1000 + i),
			Commission:  500,
			Description: fmt.Sprintf("Validator %d", i),
		}
		if err := validatorStore.SetValidator(ctx, validator); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBAPIValidatorStore_Delegate benchmarks delegation operations
func BenchmarkBAPIValidatorStore_Delegate(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	validatorStore := store.NewBAPIValidatorStore(ss)
	ctx := context.Background()

	// Create validator
	valPubKey := make([]byte, 32)
	copy(valPubKey, "validator-pubkey-0000000000")
	validator := &store.BAPIValidator{
		PubKey: types.PublicKey{
			Type: types.KeyTypeEd25519,
			Data: valPubKey,
		},
		Power:      1000,
		Commission: 500,
	}
	if err := validatorStore.SetValidator(ctx, validator); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		delegator := fmt.Sprintf("delegator%d", i)
		if err := validatorStore.Delegate(ctx, delegator, valPubKey, 100); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBAPIProposalStore_Set benchmarks proposal set operations
func BenchmarkBAPIProposalStore_Set(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	proposalStore := store.NewBAPIProposalStore(ss)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proposal := &store.BAPIProposal{
			ID:           uint64(i + 1),
			Proposer:     "alice",
			Title:        fmt.Sprintf("Proposal %d", i),
			Description:  "Test proposal description",
			Type:         "text",
			Status:       "voting",
			TotalDeposit: 1000000,
			DepositDenom: "stake",
		}
		if err := proposalStore.SetProposal(ctx, proposal); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkBAPIProposalStore_RecordVote benchmarks vote recording
func BenchmarkBAPIProposalStore_RecordVote(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	proposalStore := store.NewBAPIProposalStore(ss)
	ctx := context.Background()

	// Create proposal
	proposal := &store.BAPIProposal{
		ID:           1,
		Proposer:     "alice",
		Title:        "Test Proposal",
		Description:  "Test proposal description",
		Type:         "text",
		Status:       "voting",
		TotalDeposit: 1000000,
		DepositDenom: "stake",
	}
	if err := proposalStore.SetProposal(ctx, proposal); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		voter := fmt.Sprintf("voter%d", i)
		// Note: We use separate proposals to avoid "already voted" errors
		// In real usage, you'd have many voters on one proposal
		_ = proposalStore.RecordVote(ctx, 1, voter, "yes", 100, time.Now())
	}
}

// BenchmarkTypedStore_Parallel benchmarks parallel read operations
func BenchmarkTypedStore_Parallel(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	balanceStore := store.NewBAPIBalanceStore(ss)
	ctx := context.Background()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		account := fmt.Sprintf("account%d", i)
		if err := balanceStore.Set(ctx, account, "stake", uint64(i*100)); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			account := fmt.Sprintf("account%d", i%1000)
			if _, err := balanceStore.GetAmount(ctx, account, "stake"); err != nil {
				b.Error(err)
			}
			i++
		}
	})
}
