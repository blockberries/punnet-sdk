package mint

// ModuleName is the module's unique name used for genesis blob
// keying and the BAPI router registry.
const ModuleName = "mint"

// EmissionPoolAccount is the AccountName for the chain-wide
// Emission Pool. Spec §4.1: every block's B_t is credited here;
// distribution drains the whole balance into validator
// accumulators at every epoch close (spec §3.4 "Settlement at
// epoch close").
const EmissionPoolAccount = "module.emission"

// DefaultRhoScaled is the per-block emission rate ρ scaled by 10^18
// so the value fits in a uint64. The spec §4.1 derives
//
//	ρ = y* · r* / blocks_per_year
//	  = 0.05 × (2/3) / 31_536_000
//	  ≈ 1.057 × 10⁻⁹
//
// In 18-decimal form: ρ × 10^18 ≈ 1.057 × 10⁹.
//
// Governance may multiply ρ by [0.8, 1.25] every ~6 months per
// spec §4.2; the value lives in module state (initialised from
// genesis with this default) and is mutable via a future MsgSetRho.
const DefaultRhoScaled uint64 = 1_057_000_000

// VThresholdFractionBps is V_threshold expressed as basis points
// of TotalSupply. Spec §4.1: V_threshold = 5% · S = 500 bps. Below
// this VRP balance the emission tapers linearly toward zero.
const VThresholdFractionBps uint64 = 500

// RhoScale is the 10^18 denominator used to interpret
// DefaultRhoScaled (and any future stored ρ value) as a real
// number. Pulled out of the math sites for clarity.
const RhoScale uint64 = 1_000_000_000_000_000_000 // 10^18

// StakingDenom is the base denom of all chain accounts.
// Module-internal constant; the canonical chain-wide denom
// override lives in the bank module's configuration (Phase 1
// wiring uses a fixed "stake" denom per chain today).
const StakingDenom = "stake"

// VRPAccount is the source of B_t. Mirrors the constant in the
// runtime's protocol-account table (runtime.ModuleAccountVRP).
// Inlined here as a string to avoid a runtime-package import in
// the type-only file; the actual debit goes via TransferEffect
// which takes an AccountName.
const VRPAccount = "module.vrp"

// CTAccount, EcosystemAccount, BootstrapAccount are the other
// allocation buckets per spec §1. Used by CS computation:
//
//	CS_t = S − VRP_t − CT_t − E_t − BL_t
//
// Like VRPAccount they're inlined to keep this file
// runtime-package-free.
const (
	CTAccount        = "module.ct"
	EcosystemAccount = "module.eco"
	BootstrapAccount = "module.bl"
)

// MintParams is the module's persisted parameter state. Stored
// under the typed-store key "params/active". Mutable by future
// governance handler; initialised from genesis blob.
type MintParams struct {
	// RhoScaled is the per-block emission rate × 10^18. See
	// DefaultRhoScaled for the spec derivation.
	RhoScaled uint64 `cramberry:"1" json:"rho_scaled"`

	// VThresholdMicro is V_threshold in micro-tokens. Computed
	// from TotalSupply at genesis time. Below this VRP balance
	// emission tapers; at zero VRP emission is zero.
	VThresholdMicro uint64 `cramberry:"2" json:"v_threshold_micro"`
}
