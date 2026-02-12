package benchmark

import (
	"context"
	"fmt"
	"testing"

	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/store"
	"github.com/blockberries/punnet-sdk/types"
)

// BenchmarkTransferEffect_Validate benchmarks transfer effect validation
func BenchmarkTransferEffect_Validate(b *testing.B) {
	effect := effects.TransferEffect{
		From: "alice",
		To:   "bob",
		Amount: types.Coins{
			{Denom: "stake", Amount: 1000},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := effect.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTransferEffect_Execute benchmarks transfer effect execution via balance store
func BenchmarkTransferEffect_Execute(b *testing.B) {
	ss, err := statestore.NewMemoryIAVLStore(10000)
	if err != nil {
		b.Fatal(err)
	}
	defer ss.Close()

	balanceStore := store.NewBAPIBalanceStore(ss)
	ctx := context.Background()

	// Pre-populate with high balances
	if err := balanceStore.Set(ctx, "alice", "stake", 1000000000); err != nil {
		b.Fatal(err)
	}
	if err := balanceStore.Set(ctx, "bob", "stake", 1000000000); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Execute transfer directly via store (simulating effect execution)
		if err := balanceStore.Transfer(ctx, "alice", "bob", "stake", 1); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEventEffect_Creation benchmarks event effect creation
func BenchmarkEventEffect_Creation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = effects.NewEventEffect("transfer", map[string][]byte{
			"from":   []byte("alice"),
			"to":     []byte("bob"),
			"amount": []byte("1000"),
			"denom":  []byte("stake"),
		})
	}
}

// BenchmarkEventEffect_Validate benchmarks event effect validation
func BenchmarkEventEffect_Validate(b *testing.B) {
	event := effects.NewEventEffect("transfer", map[string][]byte{
		"from":   []byte("alice"),
		"to":     []byte("bob"),
		"amount": []byte("1000"),
		"denom":  []byte("stake"),
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := event.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEffectBatch_Validation benchmarks batch effect validation
func BenchmarkEffectBatch_Validation(b *testing.B) {
	// Create a batch of transfer effects
	batch := make([]effects.TransferEffect, 100)
	for i := 0; i < 100; i++ {
		batch[i] = effects.TransferEffect{
			From: types.AccountName(fmt.Sprintf("sender%d", i)),
			To:   types.AccountName(fmt.Sprintf("receiver%d", i)),
			Amount: types.Coins{
				{Denom: "stake", Amount: uint64(i + 1)},
			},
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, eff := range batch {
			if err := eff.Validate(); err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkTransferEffect_MultiCoin benchmarks multi-coin transfer validation
func BenchmarkTransferEffect_MultiCoin(b *testing.B) {
	effect := effects.TransferEffect{
		From: "alice",
		To:   "bob",
		Amount: types.Coins{
			{Denom: "atom", Amount: 100},
			{Denom: "osmo", Amount: 200},
			{Denom: "stake", Amount: 300},
			{Denom: "token", Amount: 400},
			{Denom: "usd", Amount: 500},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := effect.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTransferEffect_LargeAmount benchmarks large amount transfer
func BenchmarkTransferEffect_LargeAmount(b *testing.B) {
	effect := effects.TransferEffect{
		From: "alice",
		To:   "bob",
		Amount: types.Coins{
			{Denom: "stake", Amount: effects.MaxTransferAmount - 1},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := effect.Validate(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEffectType_Identification benchmarks effect type identification
func BenchmarkEffectType_Identification(b *testing.B) {
	transfer := effects.TransferEffect{From: "alice", To: "bob", Amount: types.Coins{{Denom: "stake", Amount: 100}}}
	event := effects.NewEventEffect("test", nil)

	effectsList := []effects.Effect{transfer, event}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, eff := range effectsList {
			switch eff.Type() {
			case effects.EffectTypeTransfer:
				// process transfer
			case effects.EffectTypeEvent:
				// process event
			}
		}
	}
}

// BenchmarkCoinValidation benchmarks coin validation
func BenchmarkCoinValidation(b *testing.B) {
	coins := types.Coins{
		{Denom: "atom", Amount: 100},
		{Denom: "osmo", Amount: 200},
		{Denom: "stake", Amount: 300},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !coins.IsValid() {
			b.Fatal("coins should be valid")
		}
	}
}

// BenchmarkAccountNameValidation benchmarks account name validation
func BenchmarkAccountNameValidation(b *testing.B) {
	name := types.AccountName("alice")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !name.IsValid() {
			b.Fatal("name should be valid")
		}
	}
}

// BenchmarkTransferEffect_Key benchmarks effect key generation
func BenchmarkTransferEffect_Key(b *testing.B) {
	effect := effects.TransferEffect{
		From: "alice",
		To:   "bob",
		Amount: types.Coins{
			{Denom: "stake", Amount: 1000},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = effect.Key()
	}
}

// BenchmarkTransferEffect_Dependencies benchmarks dependency listing
func BenchmarkTransferEffect_Dependencies(b *testing.B) {
	effect := effects.TransferEffect{
		From: "alice",
		To:   "bob",
		Amount: types.Coins{
			{Denom: "stake", Amount: 1000},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = effect.Dependencies()
	}
}

// BenchmarkSafeAdd benchmarks overflow-safe addition
func BenchmarkSafeAdd(b *testing.B) {
	a := uint64(1000000)
	addend := uint64(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = effects.SafeAdd(a, addend)
	}
}

// BenchmarkSafeSub benchmarks underflow-safe subtraction
func BenchmarkSafeSub(b *testing.B) {
	a := uint64(1000000)
	sub := uint64(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = effects.SafeSub(a, sub)
	}
}
