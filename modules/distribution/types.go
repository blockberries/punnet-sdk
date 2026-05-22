package distribution

// ModuleName is the module's unique name.
const ModuleName = "distribution"

// StorePrefix is the typed-store namespace.
const StorePrefix = "distribution/"

// StakingDenom mirrors the base denom used by mint + staking.
const StakingDenom = "stake"

// RewardScale is the 10^18 scaling used for the R_v reward-per-share
// index. Spec §2: "emission accumulators carry 18 decimals internally
// to avoid rounding loss; round down to micro-tokens on credit to
// per-validator accumulators."
const RewardScale uint64 = 1_000_000_000_000_000_000 // 10^18

// AlphaScaledDefault is the participation-share weight α per spec
// §3.4, scaled by 10000. Default α = 0.3 = 3000 bps; the
// share-of-leader-blocks term weights 30% and share-of-batches-
// certified weights 70%.
const AlphaScaledDefault uint64 = 3000

// AlphaScale is the 10000 denominator for AlphaScaledDefault, kept
// as a named constant so the formula sites are self-documenting.
const AlphaScale uint64 = 10000

// EmissionPoolAccount + PriorityPoolAccount are the source accounts
// drained at every epoch close. Inlined as strings (rather than
// imported from mint/fees) to avoid an import cycle.
const (
	EmissionPoolAccount = "module.emission"
	PriorityPoolAccount = "module.pp"
)

// ValidatorDistribution is the per-validator F1 state. Phase 3.6
// expansion: per-period historical snapshots now live in
// ValidatorHistoricalRewards (keyed by period). The fields here
// track only the IN-PROGRESS period: CurrentRewardsScaled is the
// reward accumulator that the next IncrementPeriod folds into the
// cumulative ratio.
//
// At period boundaries, IncrementValidatorPeriod:
//   1. Computes delta_CRR = CurrentRewardsScaled / TotalStakeMicro
//      (both stored as 10^18-scaled values)
//   2. cumulative_new = cumulative_prev + delta_CRR
//   3. Stores historical[period] = cumulative_new (snapshot
//      AFTER this period's credit)
//   4. Resets CurrentRewardsScaled to 0
//   5. Increments CurrentPeriod
//
// Delegators reference the snapshot at the END of their joined
// period; the difference snapshot[end] − snapshot[start] gives
// the per-share rewards earned over that span.
type ValidatorDistribution struct {
	// CumulativeRewardRatioScaled: cumulative R_v × 10^18, before
	// folding in CurrentRewardsScaled. Updated at every
	// IncrementValidatorPeriod.
	CumulativeRewardRatioScaled []byte `cramberry:"1" json:"cumulative_reward_ratio_scaled"`

	// CurrentRewardsScaled: the in-progress period's accumulator
	// scaled by 10^18. Credit at epoch close adds to this; the
	// next IncrementPeriod folds it into the CRR and resets.
	CurrentRewardsScaled []byte `cramberry:"2" json:"current_rewards_scaled"`

	// OutstandingCommissionMicro: micro-tokens of commission
	// accrued since the validator last claimed.
	OutstandingCommissionMicro uint64 `cramberry:"3" json:"outstanding_commission_micro"`

	// CurrentPeriod: monotonically incremented at every state
	// change that ends a period (epoch credit, slash, delegate,
	// claim). Slash events reference the period that's ending
	// when the slash fires.
	CurrentPeriod uint64 `cramberry:"4" json:"current_period"`

	// TotalStakeMicro: validator's TotalDelegation at last update.
	// Refreshed by IncrementValidatorPeriod and by claim handlers
	// when they call out to staking.
	TotalStakeMicro uint64 `cramberry:"5" json:"total_stake_micro"`
}

// ValidatorHistoricalRewards is the per-period snapshot of the
// validator's cumulative reward-per-share ratio at the END of
// that period. Stored under "validator/<hex>/history/<period>".
// Phase 3.6.
//
// In cosmos-sdk x/distribution, each entry also carries a
// reference count for GC; this v1 omits the GC pass and lets
// entries accumulate. Bounded by lifetime period-count per
// validator — acceptable at v1 scales.
type ValidatorHistoricalRewards struct {
	CumulativeRewardRatioScaled []byte `cramberry:"1" json:"cumulative_reward_ratio_scaled"`
}

// ValidatorSlashEvent is recorded by RecordSlash at the moment a
// validator is slashed. Stored under "validator/<hex>/slash/<period>".
// The Period is the period that's ENDING due to this slash —
// the snapshot at this period reflects pre-slash stake.
//
// FractionBps is the slash severity (e.g. 500 = 5% per spec §9).
// On claim, the delegator's stake is reduced by this fraction
// when walking past the corresponding period boundary.
type ValidatorSlashEvent struct {
	Period      uint64 `cramberry:"1" json:"period"`
	Height      uint64 `cramberry:"2" json:"height"`
	FractionBps uint64 `cramberry:"3" json:"fraction_bps"`
}

// DelegatorReward is the per-(delegator, validator) F1 state.
//
// - StakeMicro: the delegator's stake at the time of the last
//   delegation-modify or claim. Used as the "stake at start of
//   walk" by the claim algorithm; slash factors reduce this as
//   the walk crosses slash boundaries.
// - PreviousPeriod: the validator's CurrentPeriod when the
//   delegator's stake was last snapshot. Claim walks from
//   PreviousPeriod to current, applying slash events along the
//   way.
type DelegatorReward struct {
	StakeMicro     uint64 `cramberry:"1" json:"stake_micro"`
	PreviousPeriod uint64 `cramberry:"2" json:"previous_period"`
}

// DistributionParams is the persisted module parameters. Currently
// just α (the leader/batch weight). Phase 4 governance can update.
type DistributionParams struct {
	AlphaScaled uint64 `cramberry:"1" json:"alpha_scaled"`
}

// Typed-store key formats. Validator records under
// "validator/<hex>"; delegator records under
// "delegator/<delegator>/<validator-hex>".
const (
	keyValidatorPrefix = "validator/"
	keyDelegatorPrefix = "delegator/"
	keyParams          = "params"
)
