package runtime

import (
	"strings"
	"testing"

	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
)

func TestTokenomicsGenesis_AllocationConstants(t *testing.T) {
	// Static assertion that the five allocation percentages sum to
	// 100% (10000 basis points). The compile-time check in
	// tokenomics_genesis.go would already have rejected the build,
	// but assert it here too so a test failure flags the issue if
	// someone disables the compile-time guard.
	total := AllocPctVRP + AllocPctCT + AllocPctAirdrop +
		AllocPctEcosystem + AllocPctBootstrap
	assert.Equal(t, uint64(10000), total,
		"allocation percentages must sum to 10000 basis points (100%%)")
}

func TestTokenomicsGenesis_InitialAllocations(t *testing.T) {
	t.Run("nil tg yields zero allocations", func(t *testing.T) {
		var tg *TokenomicsGenesis
		vrp, ct, ad, eco, bs := tg.InitialAllocations()
		assert.Equal(t, uint64(0), vrp+ct+ad+eco+bs)
	})

	t.Run("zero supply yields zero allocations", func(t *testing.T) {
		tg := &TokenomicsGenesis{TotalSupply: 0}
		vrp, ct, ad, eco, bs := tg.InitialAllocations()
		assert.Equal(t, uint64(0), vrp+ct+ad+eco+bs)
	})

	t.Run("non-zero supply sums exactly", func(t *testing.T) {
		tg := &TokenomicsGenesis{TotalSupply: 1_000_000_000_000}
		vrp, ct, ad, eco, bs := tg.InitialAllocations()
		assert.Equal(t, tg.TotalSupply, vrp+ct+ad+eco+bs,
			"allocations must sum exactly to TotalSupply")
		// Spot-check the canonical 25/30/30/10/5 split.
		assert.Equal(t, uint64(250_000_000_000), vrp)
		assert.Equal(t, uint64(300_000_000_000), ct)
		assert.Equal(t, uint64(300_000_000_000), ad)
		assert.Equal(t, uint64(100_000_000_000), eco)
		assert.Equal(t, uint64(50_000_000_000), bs)
	})

	t.Run("rounding remainder lands in bootstrap", func(t *testing.T) {
		// Supply that doesn't divide evenly by 10000.
		tg := &TokenomicsGenesis{TotalSupply: 1_000_000_007}
		vrp, ct, ad, eco, bs := tg.InitialAllocations()
		assert.Equal(t, tg.TotalSupply, vrp+ct+ad+eco+bs,
			"allocations must sum exactly to TotalSupply even with rounding")
	})
}

func TestTokenomicsGenesis_ValidateBasic(t *testing.T) {
	t.Run("nil tg is fine", func(t *testing.T) {
		var tg *TokenomicsGenesis
		assert.NoError(t, tg.ValidateBasic())
	})

	t.Run("zero supply rejected", func(t *testing.T) {
		tg := &TokenomicsGenesis{TotalSupply: 0}
		err := tg.ValidateBasic()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "total_supply")
	})

	t.Run("valid bootstrap validators accepted", func(t *testing.T) {
		tg := &TokenomicsGenesis{
			TotalSupply: 1_000_000_000_000,
			BootstrapValidators: []BootstrapValidator{
				{Name: ptypes.AccountName("alice"), PubKey: make([]byte, 32)},
				{Name: ptypes.AccountName("bob"), PubKey: make([]byte, 32)},
			},
		}
		assert.NoError(t, tg.ValidateBasic())
	})

	t.Run("invalid bootstrap name rejected", func(t *testing.T) {
		tg := &TokenomicsGenesis{
			TotalSupply: 1,
			BootstrapValidators: []BootstrapValidator{
				{Name: ptypes.AccountName("bad name!"), PubKey: make([]byte, 32)},
			},
		}
		err := tg.ValidateBasic()
		assert.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "name")
	})

	t.Run("duplicate bootstrap name rejected", func(t *testing.T) {
		tg := &TokenomicsGenesis{
			TotalSupply: 1,
			BootstrapValidators: []BootstrapValidator{
				{Name: ptypes.AccountName("alice"), PubKey: make([]byte, 32)},
				{Name: ptypes.AccountName("alice"), PubKey: make([]byte, 32)},
			},
		}
		err := tg.ValidateBasic()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "duplicat")
	})

	t.Run("wrong pubkey length rejected", func(t *testing.T) {
		tg := &TokenomicsGenesis{
			TotalSupply: 1,
			BootstrapValidators: []BootstrapValidator{
				{Name: ptypes.AccountName("alice"), PubKey: make([]byte, 31)},
			},
		}
		err := tg.ValidateBasic()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pub_key")
	})
}
