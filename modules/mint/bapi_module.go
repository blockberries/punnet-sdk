package mint

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
)

// keyParams is the typed-store key for the persisted MintParams.
const keyParams = "params/active"

// BAPIMintModule implements the per-block VRP drain. Implements
// runtime.BAPIModule, runtime.BAPIBlockProcessor,
// runtime.BAPIGenesisInitializer, runtime.BAPIGenesisExporter, and
// runtime.BAPITokenomicsConsumer.
type BAPIMintModule struct {
	balanceStore *store.BAPIBalanceStore
	paramsStore  *store.TypedStore[*MintParams]

	// totalSupply is captured from TokenomicsGenesis via
	// ConsumeTokenomics so we can recompute V_threshold if
	// governance lowers it via params. For v1 it's also used to
	// initialise V_threshold at genesis.
	totalSupply uint64
}

// NewBAPIMintModule constructs the module backed by the given
// stores. paramsStore is created on the same StateStore as the
// balance store.
func NewBAPIMintModule(balanceStore *store.BAPIBalanceStore) (*BAPIMintModule, error) {
	if balanceStore == nil {
		return nil, fmt.Errorf("balance store cannot be nil")
	}
	return &BAPIMintModule{
		balanceStore: balanceStore,
		paramsStore:  store.NewTypedStore[*MintParams](balanceStore.StateStore(), "mint/"),
	}, nil
}

// Name returns the module name.
func (m *BAPIMintModule) Name() string { return ModuleName }

// RegisterMsgHandlers returns the (empty) message handler table.
// Phase 4 governance will add MsgSetRho here per spec §4.2.
func (m *BAPIMintModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{}
}

// RegisterQueryHandlers returns query handlers.
func (m *BAPIMintModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/mint/params": m.handleQueryParams,
	}
}

// InitGenesis seeds MintParams from the per-module genesis blob.
// When the blob is empty (no genesis override), the defaults from
// types.go apply: RhoScaled = DefaultRhoScaled,
// VThresholdMicro = 5% × TotalSupply. TotalSupply must already be
// known via ConsumeTokenomics; if it isn't (a non-tokenomics test
// chain), V_threshold is left at zero, which makes taper always
// zero — emission disabled, intentional.
func (m *BAPIMintModule) InitGenesis(ctx context.Context, data []byte) error {
	params := &MintParams{
		RhoScaled:       DefaultRhoScaled,
		VThresholdMicro: m.totalSupply * VThresholdFractionBps / 10000,
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, params); err != nil {
			return fmt.Errorf("unmarshal mint genesis: %w", err)
		}
	}
	if params.RhoScaled == 0 {
		params.RhoScaled = DefaultRhoScaled
	}
	if params.VThresholdMicro == 0 {
		params.VThresholdMicro = m.totalSupply * VThresholdFractionBps / 10000
	}
	return m.paramsStore.Set(ctx, keyParams, params)
}

// ExportGenesis writes the current MintParams.
func (m *BAPIMintModule) ExportGenesis(ctx context.Context) ([]byte, error) {
	p, err := m.paramsStore.Get(ctx, keyParams)
	if err != nil {
		return json.Marshal(&MintParams{
			RhoScaled:       DefaultRhoScaled,
			VThresholdMicro: m.totalSupply * VThresholdFractionBps / 10000,
		})
	}
	return json.Marshal(p)
}

// ConsumeTokenomics implements runtime.BAPITokenomicsConsumer. Stores
// TotalSupply so InitGenesis can compute V_threshold.
func (m *BAPIMintModule) ConsumeTokenomics(_ context.Context, p runtime.TokenomicsParams) error {
	if m == nil {
		return fmt.Errorf("module is nil")
	}
	m.totalSupply = p.TotalSupply
	return nil
}

// BeginBlock is a no-op. The mint module operates entirely at
// EndBlock so per-block transaction handlers can still touch VRP
// (e.g. governance VRP-refill from CT in Phase 4) before the
// emission calculation runs.
func (m *BAPIMintModule) BeginBlock(_ context.Context, _ *runtime.BAPIBlockContext) ([]effects.Effect, error) {
	return nil, nil
}

// EndBlock computes B_t per spec §4.1 and emits a TransferEffect
// VRP → Emission Pool. The whole calculation runs in big.Int
// because rho_scaled × CS × taper_scaled easily overflows uint64
// for any realistic supply.
//
// Edge cases:
//   - VRP empty (B_t = 0): no transfer effect, nil emission
//   - VRP < B_t (very late tail): saturate B_t to VRP
//   - TotalSupply unset (no tokenomics genesis): no effect
//
// Returned effects.Effect slice always has at most one entry.
func (m *BAPIMintModule) EndBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	if m == nil || m.balanceStore == nil {
		return nil, nil, nil
	}
	params, err := m.paramsStore.Get(ctx, keyParams)
	if err != nil || params == nil || params.RhoScaled == 0 || params.VThresholdMicro == 0 {
		// Module not configured for emission — defensible for
		// pre-Phase-3 test chains.
		return nil, nil, nil
	}

	vrpMicro, err := m.balanceStore.GetAmount(ctx, VRPAccount, StakingDenom)
	if err != nil {
		return nil, nil, fmt.Errorf("read VRP balance: %w", err)
	}
	if vrpMicro == 0 {
		return nil, nil, nil
	}
	ctMicro, _ := m.balanceStore.GetAmount(ctx, CTAccount, StakingDenom)
	ecoMicro, _ := m.balanceStore.GetAmount(ctx, EcosystemAccount, StakingDenom)
	blMicro, _ := m.balanceStore.GetAmount(ctx, BootstrapAccount, StakingDenom)

	if m.totalSupply == 0 {
		return nil, nil, nil
	}
	// Spec §2: CS_t = S − VRP_t − CT_t − E_t − BL_t. Staked
	// tokens count as circulating (held by users, merely bonded).
	csMicro := m.totalSupply
	if sub := vrpMicro + ctMicro + ecoMicro + blMicro; sub <= m.totalSupply {
		csMicro -= sub
	} else {
		// Defensive: protocol accounts somehow exceed supply. Set
		// CS=0 to suppress emission rather than wrap the subtraction.
		csMicro = 0
	}
	if csMicro == 0 {
		return nil, nil, nil
	}

	bMicro := computeEmission(params.RhoScaled, csMicro, vrpMicro, params.VThresholdMicro)
	if bMicro > vrpMicro {
		bMicro = vrpMicro
	}
	if bMicro == 0 {
		return nil, nil, nil
	}

	// Apply the transfer directly to the balance store rather than
	// returning a TransferEffect. The distribution module's EndBlock
	// reads EmissionPoolAccount at the same epoch-close height —
	// returning an effect (which the runtime would only execute after
	// every module's EndBlock has run) would leave distribution
	// reading a stale EmissionPool balance. Bookkeeping operations
	// (emission schedule, participation counters, validator-set
	// refresh snapshot) bypass the effects pipeline so downstream
	// consumers see settled state. PLAN §7 Phase 3.7.
	if err := m.balanceStore.Transfer(ctx, VRPAccount, EmissionPoolAccount, StakingDenom, bMicro); err != nil {
		return nil, nil, fmt.Errorf("emit B_t: %w", err)
	}
	return nil, nil, nil
}

// computeEmission is the spec §4.1 formula:
//
//	B_t = rho · CS · taper(VRP)
//	taper(V) = min(1, V / V_threshold)
//
// rho_scaled is rho × 10^18; all other inputs are micro-tokens.
// Return value is micro-tokens, rounded down. The fractional
// residual stays in VRP (the next block picks it up since CS
// grows by exactly the credited amount).
//
// Math:
//
//	taper_scaled = min(VRP × 10^18 / V_threshold, 10^18)
//	B_t = (rho_scaled × CS × taper_scaled) / 10^36
//
// big.Int is required: rho_scaled (~10^9) × CS (~10^15) × taper_scaled
// (~10^18) = ~10^42, far above uint64.
func computeEmission(rhoScaled, csMicro, vrpMicro, vThresholdMicro uint64) uint64 {
	if rhoScaled == 0 || csMicro == 0 || vThresholdMicro == 0 {
		return 0
	}

	// taper_scaled in [0, 10^18]
	taperScaled := new(big.Int).SetUint64(vrpMicro)
	taperScaled.Mul(taperScaled, new(big.Int).SetUint64(RhoScale))
	taperScaled.Div(taperScaled, new(big.Int).SetUint64(vThresholdMicro))
	if taperScaled.Cmp(new(big.Int).SetUint64(RhoScale)) > 0 {
		taperScaled.SetUint64(RhoScale)
	}

	num := new(big.Int).SetUint64(rhoScaled)
	num.Mul(num, new(big.Int).SetUint64(csMicro))
	num.Mul(num, taperScaled)

	denom := new(big.Int).SetUint64(RhoScale)
	denom.Mul(denom, new(big.Int).SetUint64(RhoScale))

	num.Div(num, denom)
	if !num.IsUint64() {
		// Saturate at uint64 max. Realistic supplies stay well
		// below this; the guard exists so a malformed input
		// (huge rho_scaled override) can't panic the chain.
		return ^uint64(0)
	}
	return num.Uint64()
}

func (m *BAPIMintModule) handleQueryParams(ctx context.Context, _ []byte, _ int64) ([]byte, error) {
	p, err := m.paramsStore.Get(ctx, keyParams)
	if err != nil {
		return nil, fmt.Errorf("get params: %w", err)
	}
	return json.Marshal(p)
}

// Verify interface compliance at compile time.
var (
	_ runtime.BAPIModule             = (*BAPIMintModule)(nil)
	_ runtime.BAPIBlockProcessor     = (*BAPIMintModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIMintModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIMintModule)(nil)
	_ runtime.BAPITokenomicsConsumer = (*BAPIMintModule)(nil)
)
