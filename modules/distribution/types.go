// Package distribution implements the validator/delegator reward
// split per the cosmos-sdk-style F1 algorithm described in
// tokenomics spec §4.4.
//
// Each validator maintains a cumulative reward-per-share index R_v
// (18-decimal internal precision). At every epoch close the
// Emission Pool (mint) + Priority Pool (fees) are partitioned by
// participation share — share(v) = α·leader_share + (1-α)·batch_share
// per spec §3.4 — and each validator's R_v advances by
//
//	R_v += share(v) · (EmissionPool + PriorityPool) · (1 - c) / total_stake_on_v
//
// Delegators pull their accumulated rewards via
// MsgWithdrawDelegatorReward: at claim time they receive
// delegator_stake · (R_v_now - R_v_when_joined). The validator's
// commission share (c · share(v) · pools) accrues into a separate
// outstanding-commission accumulator that the validator account
// withdraws via MsgWithdrawValidatorCommission.
//
// **Scope note (Phase 3.5):** this v1 module covers the F1 base
// case correctly. Slash-period interaction (spec §4.4 — "the
// subtlest part of F1; getting it wrong silently over/underpays
// delegators after the first slash") is NOT yet implemented; a
// delegator who delegated before a slash will be credited against
// post-slash stake for pre-slash rewards (underpay) until Phase
// 3.6 expansion adds per-period snapshot + slash-factor walking
// per the cosmos-sdk x/distribution algebra. The
// outstanding-state machinery (period counter, snapshot map) is
// scaffolded so the expansion is additive, not a rewrite.
//
// PLAN §7 Phase 3.5 / 3.6.
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

// ValidatorDistribution is the per-validator F1 state.
//
// - RewardPerShareScaled: cumulative R_v × 10^18, encoded as
//   big-endian bytes. Stored as bytes so we can use math/big
//   internally without bumping the on-disk schema each time the
//   encoding format changes.
// - OutstandingCommissionMicro: micro-tokens of commission accrued
//   since the validator last claimed.
// - CurrentPeriod: monotonically incremented at slash events
//   (Phase 3.6). Currently always 0 — slash-period snapshot logic
//   not yet wired.
// - TotalStakeMicro: validator's TotalDelegation at last update.
//   Cached here so per-block credit math doesn't reach into the
//   staking module store.
type ValidatorDistribution struct {
	RewardPerShareScaled       []byte `cramberry:"1" json:"reward_per_share_scaled"`
	OutstandingCommissionMicro uint64 `cramberry:"2" json:"outstanding_commission_micro"`
	CurrentPeriod              uint64 `cramberry:"3" json:"current_period"`
	TotalStakeMicro            uint64 `cramberry:"4" json:"total_stake_micro"`
}

// DelegatorReward is the per-(delegator, validator) F1 state.
//
// - StakeMicro: the delegator's stake on the validator at the last
//   index-snapshot time. Updated on delegate / undelegate / claim.
// - StartIndexScaled: R_v at the moment of the last snapshot. The
//   unclaimed reward is stake × (current R_v − StartIndex).
// - StartPeriod: the validator's CurrentPeriod when the snapshot
//   was taken. Phase 3.6 will use this for slash-period walking.
type DelegatorReward struct {
	StakeMicro       uint64 `cramberry:"1" json:"stake_micro"`
	StartIndexScaled []byte `cramberry:"2" json:"start_index_scaled"`
	StartPeriod      uint64 `cramberry:"3" json:"start_period"`
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
