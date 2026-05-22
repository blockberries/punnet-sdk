// Package app builds the realistic-workload example BAPIApplication
// per PLAN §7 Phase 6.1: bank + staking + governance + fees + mint
// + participation + distribution + wsync + auth wired together.
//
// Used both by the cmd/realistic entry point (a CLI that prints
// module list as a smoke test) and by the workload-generator
// tests in ../workload/.
package app

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/modules/auth"
	"github.com/blockberries/punnet-sdk/modules/bank"
	"github.com/blockberries/punnet-sdk/modules/distribution"
	"github.com/blockberries/punnet-sdk/modules/fees"
	"github.com/blockberries/punnet-sdk/modules/governance"
	"github.com/blockberries/punnet-sdk/modules/mint"
	"github.com/blockberries/punnet-sdk/modules/participation"
	"github.com/blockberries/punnet-sdk/modules/staking"
	"github.com/blockberries/punnet-sdk/modules/wsync"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
)

// Realistic bundles the constructed application + the underlying
// typed stores so workload tests can seed balances + accounts
// without needing accessor methods on BAPIApplication.
type Realistic struct {
	App            *runtime.BAPIApplication
	BalanceStore   *store.BAPIBalanceStore
	AccountStore   *store.BAPIAccountStore
	ValidatorStore *store.BAPIValidatorStore
}

// Build constructs the full-stack tokenomics application, runs
// the initial Handshake, and returns the configured
// BAPIApplication wrapped in a Realistic struct. Genesis state
// seeds module.bl with the 5% BL allocation per spec §1; other
// protocol accounts are handled by the runtime's Phase 0.6 seeding
// loop.
func Build() (*Realistic, error) {
	ss, err := statestore.NewMemoryIAVLStore(1000)
	if err != nil {
		return nil, fmt.Errorf("state store: %w", err)
	}

	provider := store.NewBAPIStoreProvider(ss)
	accountStore := provider.GetAccountStore()
	balanceStore := provider.GetBalanceStore()
	validatorStore := provider.GetValidatorStore()
	proposalStore := store.NewBAPIProposalStore(ss)

	authMod, err := auth.NewBAPIAuthModule(accountStore)
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	bankMod, err := bank.NewBAPIBankModule(balanceStore)
	if err != nil {
		return nil, fmt.Errorf("bank: %w", err)
	}
	stakingMod, err := staking.NewBAPIStakingModule(validatorStore, balanceStore)
	if err != nil {
		return nil, fmt.Errorf("staking: %w", err)
	}
	govMod, err := governance.NewBAPIGovernanceModule(proposalStore, balanceStore)
	if err != nil {
		return nil, fmt.Errorf("gov: %w", err)
	}
	feesMod, err := fees.NewBAPIFeesModule(ss)
	if err != nil {
		return nil, fmt.Errorf("fees: %w", err)
	}
	mintMod, err := mint.NewBAPIMintModule(balanceStore)
	if err != nil {
		return nil, fmt.Errorf("mint: %w", err)
	}
	partMod, err := participation.NewBAPIParticipationModule(ss)
	if err != nil {
		return nil, fmt.Errorf("participation: %w", err)
	}
	distMod, err := distribution.NewBAPIDistributionModule(ss, balanceStore, validatorStore, partMod)
	if err != nil {
		return nil, fmt.Errorf("distribution: %w", err)
	}
	wsyncMod, err := wsync.NewBAPIWsyncModule(ss)
	if err != nil {
		return nil, fmt.Errorf("wsync: %w", err)
	}

	if err := govMod.RegisterParameterTarget("fees", feesMod); err != nil {
		return nil, fmt.Errorf("register fees as gov target: %w", err)
	}

	// Phase 3.6 wiring: distribution registers as a slash
	// observer so it can end the validator's period before the
	// slash applies. Without this, F1 reward math underpays
	// pre-slash delegators.
	stakingMod.RegisterSlashObserver(distMod)
	if err := govMod.Parameters.Register(governance.ParameterBand{
		Name:    "byte_fee",
		SoftMin: 0, SoftMax: 100,
		HardMin: 0, HardMax: 1000,
	}); err != nil {
		return nil, fmt.Errorf("register byte_fee band: %w", err)
	}

	app, err := runtime.NewBAPIApplication(runtime.BAPIApplicationConfig{
		ChainID:    "realistic-stealth",
		StateStore: ss,
		Modules: []runtime.BAPIModule{
			authMod,
			bankMod,
			stakingMod,
			govMod,
			feesMod,
			mintMod,
			partMod,
			distMod,
			wsyncMod,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}

	const totalSupply uint64 = 1_000_000_000_000_000
	bootstrapPubKey := make([]byte, 32)
	bootstrapPubKey[0] = 0xb0

	tokGenesis := runtime.TokenomicsGenesis{
		TotalSupply: totalSupply,
		BootstrapValidators: []runtime.BootstrapValidator{
			{Name: "boot1", PubKey: bootstrapPubKey},
		},
	}
	bankGenesis, _ := json.Marshal(map[string]any{
		"balances": []map[string]any{
			{"account": "module.bl", "denom": "stake", "amount": uint64(totalSupply / 20)},
		},
	})
	appState, err := json.Marshal(runtime.BAPIGenesisState{
		Tokenomics: &tokGenesis,
		Modules: map[string]json.RawMessage{
			"bank": bankGenesis,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal genesis: %w", err)
	}

	_, err = app.Handshake(context.Background(), types.HandshakeRequest{
		Genesis: &types.GenesisDoc{
			ChainID:       "realistic-stealth",
			InitialHeight: 1,
			ConsensusParams: types.ConsensusParams{
				MaxBlockBytes: 22020096,
				MaxTxBytes:    1048576,
			},
			Validators: []types.ValidatorUpdate{
				{
					PubKey: types.PublicKey{Type: types.KeyTypeEd25519, Data: bootstrapPubKey},
					Power:  100,
				},
			},
			AppState: appState,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("handshake: %w", err)
	}
	return &Realistic{
		App:            app,
		BalanceStore:   balanceStore,
		AccountStore:   accountStore,
		ValidatorStore: validatorStore,
	}, nil
}
