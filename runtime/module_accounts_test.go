package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/blockberries/bapi/types"
	ptypes "github.com/blockberries/punnet-sdk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleAccountName(t *testing.T) {
	t.Run("derives valid name", func(t *testing.T) {
		n := ModuleAccountName("vrp")
		assert.Equal(t, ptypes.AccountName("module.vrp"), n)
		assert.True(t, n.IsValid())
	})

	t.Run("rejects invalid slug", func(t *testing.T) {
		// Uppercase rejected by AccountName regex.
		n := ModuleAccountName("VRP")
		assert.Equal(t, ptypes.AccountName(""), n)
	})

	t.Run("rejects slug pushing over length", func(t *testing.T) {
		// AccountName max is 64; "module." (7) + 58 = 65 → invalid.
		long := strings.Repeat("a", 58)
		n := ModuleAccountName(long)
		assert.Equal(t, ptypes.AccountName(""), n)
	})
}

func TestProtocolAccounts(t *testing.T) {
	t.Run("returns the canonical four", func(t *testing.T) {
		accts := ProtocolAccounts()
		assert.Len(t, accts, 4)
		assert.Contains(t, accts, ModuleAccountVRP)
		assert.Contains(t, accts, ModuleAccountCT)
		assert.Contains(t, accts, ModuleAccountEcosystem)
		assert.Contains(t, accts, ModuleAccountBootstrap)
	})

	t.Run("permission semantics match the spec", func(t *testing.T) {
		accts := ProtocolAccounts()
		// VRP: drained by mint, never receives.
		vrp := accts[ModuleAccountVRP]
		assert.True(t, vrp.TransferOut, "VRP must be debitable (mint emission)")
		assert.False(t, vrp.TransferIn, "VRP must NOT receive (fixed supply, seeded once)")
		assert.False(t, vrp.Mint, "Mint forbidden under fixed-supply")
		assert.False(t, vrp.Burn)
		// CT: bidirectional.
		ct := accts[ModuleAccountCT]
		assert.True(t, ct.TransferOut, "CT must be debitable (governance withdrawals)")
		assert.True(t, ct.TransferIn, "CT must receive (fee revenue + slashing)")
	})
}

func TestSortedProtocolAccountNames(t *testing.T) {
	names := SortedProtocolAccountNames()
	assert.Len(t, names, 4)
	for i := 1; i < len(names); i++ {
		assert.True(t, names[i-1] < names[i],
			"SortedProtocolAccountNames must be lexicographically ordered for determinism")
	}
}

func TestProtocolAccountForSlug(t *testing.T) {
	assert.Equal(t, ModuleAccountVRP, ProtocolAccountForSlug("vrp"))
	assert.Equal(t, ModuleAccountCT, ProtocolAccountForSlug("ct"))
	assert.Equal(t, ModuleAccountEcosystem, ProtocolAccountForSlug("eco"))
	assert.Equal(t, ModuleAccountBootstrap, ProtocolAccountForSlug("bl"))
	assert.Equal(t, ptypes.AccountName(""), ProtocolAccountForSlug("unknown"))
}

// TestTokenomicsGenesis_ProtocolSeeding integrates the genesis path
// end-to-end: a chain initialised with TokenomicsGenesis has the four
// protocol accounts populated with the correct initial balances
// (summing to TotalSupply) after initGenesis completes. PLAN §7
// Phase 0.6 acceptance.
func TestTokenomicsGenesis_ProtocolSeeding(t *testing.T) {
	ss := createTestStateStore(t)

	// No module registration needed — the runtime's protocol-account
	// seeding writes directly via a.balanceStore after the module
	// loop. The bank module is the eventual consumer of that store
	// in production, but the genesis seed itself doesn't require it.
	app, err := NewBAPIApplication(BAPIApplicationConfig{
		ChainID:    "test-chain",
		StateStore: ss,
		Modules:    []BAPIModule{},
	})
	require.NoError(t, err)

	// Construct app state with TokenomicsGenesis set.
	appState, err := json.Marshal(BAPIGenesisState{
		Modules: map[string]json.RawMessage{},
		Tokenomics: &TokenomicsGenesis{
			TotalSupply: 1_000_000_000_000, // 1T micro-tokens
			// Bootstrap validators left empty; not required for
			// this test (genesis seeding works without them, the
			// BL allocation just sits in the BL account until
			// Phase 2 wires distribution).
		},
	})
	require.NoError(t, err)

	genesis := &types.GenesisDoc{
		ChainID:       "test-chain",
		InitialHeight: 1,
		ConsensusParams: types.ConsensusParams{
			MaxBlockBytes: 22020096,
			MaxTxBytes:    1048576,
		},
		Validators: []types.ValidatorUpdate{
			{PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: make([]byte, 32)}, Power: 100},
		},
		AppState: appState,
	}

	_, err = app.Handshake(context.Background(), types.HandshakeRequest{
		LastCommitted: nil,
		Genesis:       genesis,
	})
	require.NoError(t, err)

	// Verify the parsed TokenomicsGenesis is cached.
	tg := app.TokenomicsGenesis()
	require.NotNil(t, tg)
	assert.Equal(t, uint64(1_000_000_000_000), tg.TotalSupply)

	// Verify each protocol account got its expected balance.
	ctx := context.Background()
	const baseDenom = "stake"

	get := func(name ptypes.AccountName) uint64 {
		v, err := app.balanceStore.GetAmount(ctx, string(name), baseDenom)
		require.NoError(t, err, "balance %s", name)
		return v
	}

	assert.Equal(t, uint64(250_000_000_000), get(ModuleAccountVRP),
		"VRP should hold 25%% of supply")
	assert.Equal(t, uint64(300_000_000_000), get(ModuleAccountCT),
		"CT should hold 30%% of supply")
	assert.Equal(t, uint64(100_000_000_000), get(ModuleAccountEcosystem),
		"Ecosystem should hold 10%% of supply")
	// Bootstrap gets 5% + any rounding remainder. With 1T supply
	// the split is exact, so this is exactly 50_000_000_000.
	assert.Equal(t, uint64(50_000_000_000), get(ModuleAccountBootstrap),
		"BL should hold 5%% of supply (no rounding remainder at this supply)")

	// Sanity: the four protocol-account balances sum to 90% of
	// supply. The remaining 30% (airdrop) is carried separately
	// in the bank module's GenesisBalance list — verified by the
	// chain operator at genesis, not by the runtime.
	total := get(ModuleAccountVRP) + get(ModuleAccountCT) +
		get(ModuleAccountEcosystem) + get(ModuleAccountBootstrap)
	assert.Equal(t, uint64(700_000_000_000), total,
		"VRP+CT+Eco+BL should equal (1 - airdrop%%) * supply")
}
