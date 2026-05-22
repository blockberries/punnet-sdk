package distribution

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/blockberries/bapi/types"
	"github.com/blockberries/blockberry/pkg/statestore"
	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/modules/participation"
	"github.com/blockberries/punnet-sdk/modules/staking"
	"github.com/blockberries/punnet-sdk/runtime"
	"github.com/blockberries/punnet-sdk/store"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// BAPIDistributionModule implements the F1 reward split. It depends
// on three peer modules:
//   - mint:          source of EmissionPool tokens
//   - fees:          source of PriorityPool tokens
//   - participation: source of (leader_blocks, batches_certified)
//
// All three couplings are at the bank-account and store-key level
// (no direct module pointers) so cyclic imports are avoided.
type BAPIDistributionModule struct {
	balanceStore   *store.BAPIBalanceStore
	validatorStore *store.BAPIValidatorStore
	participation  *participation.BAPIParticipationModule

	validatorDist *store.TypedStore[*ValidatorDistribution]
	delegatorDist *store.TypedStore[*DelegatorReward]
	paramsStore   *store.TypedStore[*DistributionParams]
}

// NewBAPIDistributionModule constructs the module. validatorStore
// is needed to look up commission rates and total-stake values at
// epoch close; participation is needed to read frozen
// per-epoch counters; balanceStore is the source/sink for all
// reward transfers.
func NewBAPIDistributionModule(
	ss statestore.StateStore,
	balanceStore *store.BAPIBalanceStore,
	validatorStore *store.BAPIValidatorStore,
	participationMod *participation.BAPIParticipationModule,
) (*BAPIDistributionModule, error) {
	if ss == nil || balanceStore == nil || validatorStore == nil || participationMod == nil {
		return nil, fmt.Errorf("all stores + participation module are required")
	}
	return &BAPIDistributionModule{
		balanceStore:   balanceStore,
		validatorStore: validatorStore,
		participation:  participationMod,
		validatorDist:  store.NewTypedStore[*ValidatorDistribution](ss, StorePrefix),
		delegatorDist:  store.NewTypedStore[*DelegatorReward](ss, StorePrefix),
		paramsStore:    store.NewTypedStore[*DistributionParams](ss, StorePrefix),
	}, nil
}

func (m *BAPIDistributionModule) Name() string { return ModuleName }

func (m *BAPIDistributionModule) RegisterMsgHandlers() map[string]runtime.BAPIMsgHandler {
	return map[string]runtime.BAPIMsgHandler{
		TypeMsgWithdrawDelegatorReward:     m.handleWithdrawDelegatorReward,
		TypeMsgWithdrawValidatorCommission: m.handleWithdrawValidatorCommission,
	}
}

func (m *BAPIDistributionModule) RegisterQueryHandlers() map[string]runtime.BAPIQueryHandler {
	return map[string]runtime.BAPIQueryHandler{
		"/distribution/params":             m.handleQueryParams,
		"/distribution/validator":          m.handleQueryValidator,
		"/distribution/delegator_reward":   m.handleQueryDelegatorReward,
	}
}

func (m *BAPIDistributionModule) InitGenesis(ctx context.Context, data []byte) error {
	params := &DistributionParams{AlphaScaled: AlphaScaledDefault}
	if len(data) > 0 {
		var gs DistributionGenesisState
		if err := json.Unmarshal(data, &gs); err != nil {
			return fmt.Errorf("unmarshal distribution genesis: %w", err)
		}
		if gs.Params != nil && gs.Params.AlphaScaled != 0 {
			params.AlphaScaled = gs.Params.AlphaScaled
		}
	}
	return m.paramsStore.Set(ctx, keyParams, params)
}

func (m *BAPIDistributionModule) ExportGenesis(ctx context.Context) ([]byte, error) {
	p, err := m.paramsStore.Get(ctx, keyParams)
	if err != nil {
		p = &DistributionParams{AlphaScaled: AlphaScaledDefault}
	}
	return json.Marshal(DistributionGenesisState{Params: p})
}

func (m *BAPIDistributionModule) BeginBlock(_ context.Context, _ *runtime.BAPIBlockContext) ([]effects.Effect, error) {
	return nil, nil
}

// EndBlock runs the epoch-close settlement (PLAN §7 Phase 3.5 / spec
// §3.4 §4.4). On non-epoch heights it's a no-op.
//
// Algorithm (no-slash F1 base case):
//
//  1. Read EpochTotals from participation. If totals are zero, no
//     credit (nothing to do — empty epoch).
//  2. Read EmissionPool + PriorityPool balances; their sum is the
//     epoch's reward pot R.
//  3. For each validator with frozen participation in this epoch:
//     - share(v) = α/10000 · (leader_v / total_leader) +
//                  (1-α/10000) · (batches_v / total_batches)
//     - r_v = R · share(v)  (round down to micro-tokens)
//     - commission_v = r_v · c / 10000
//     - delegator_pool_v = r_v - commission_v
//     - Bump validator's OutstandingCommissionMicro.
//     - Bump validator's R_v by delegator_pool_v × 10^18 / total_stake_on_v
//  4. Drain EmissionPool + PriorityPool → distribution module account
//     so the credited tokens are held in the module's name until
//     delegators / operators claim.
//
// Returns the effects representing the pool drains. Per-validator
// state mutations are written directly (outside the effect pipeline)
// — same pattern as the participation module's freeze: they're
// bookkeeping updates that don't compose with the tx-effect batch.
func (m *BAPIDistributionModule) EndBlock(ctx context.Context, blockCtx *runtime.BAPIBlockContext) ([]effects.Effect, []types.ValidatorUpdate, error) {
	if blockCtx == nil || !staking.IsEpochCloseHeight(uint64(blockCtx.Height)) {
		return nil, nil, nil
	}
	epochNum := uint64(blockCtx.Height) / staking.EpochBlocks

	totals, err := m.participation.GetEpochTotals(ctx, epochNum)
	if err != nil || totals == nil {
		return nil, nil, nil
	}
	if totals.LeaderBlocks == 0 && totals.BatchesCertified == 0 {
		return nil, nil, nil
	}

	emissionMicro, _ := m.balanceStore.GetAmount(ctx, EmissionPoolAccount, StakingDenom)
	priorityMicro, _ := m.balanceStore.GetAmount(ctx, PriorityPoolAccount, StakingDenom)
	pool := emissionMicro + priorityMicro
	if pool == 0 {
		return nil, nil, nil
	}

	params, _ := m.paramsStore.Get(ctx, keyParams)
	if params == nil || params.AlphaScaled == 0 {
		params = &DistributionParams{AlphaScaled: AlphaScaledDefault}
	}

	// Phase 1: collect frozen participation entries. We can't
	// credit them inside the iteration callback — the underlying
	// IAVL store's Set() takes a write lock that conflicts with
	// the Iterate() read lock, deadlocking. Snapshot the entries
	// first, then process them after the iteration unwinds.
	type entry struct {
		hex string
		pv  *participation.ValidatorParticipation
	}
	var entries []entry
	err = m.participation.IterateEpochParticipation(epochNum, func(validatorHex string, pv *participation.ValidatorParticipation) bool {
		entries = append(entries, entry{validatorHex, pv})
		return false
	})
	if err != nil {
		// Iteration unsupported (in-memory test stores): caller
		// won't see any credit. Acceptable for unit tests; real
		// chains run on iterable backends.
		return nil, nil, nil
	}

	// Phase 2: credit each validator. Iteration is done; Set is safe.
	var creditedTotal uint64
	for _, e := range entries {
		creditedTotal += m.creditValidator(ctx, e.hex, e.pv, totals, params.AlphaScaled, pool)
	}

	if creditedTotal == 0 {
		return nil, nil, nil
	}

	// Drain pools to the distribution module account. The module
	// account holds the tokens until delegators / operators claim.
	var out []effects.Effect
	if emissionMicro > 0 {
		take := min(emissionMicro, creditedTotal)
		out = append(out, effects.TransferEffect{
			From:   ptypes.AccountName(EmissionPoolAccount),
			To:     ptypes.AccountName(distributionAccount),
			Amount: ptypes.NewCoins(ptypes.NewCoin(StakingDenom, take)),
		})
		creditedTotal -= take
	}
	if priorityMicro > 0 && creditedTotal > 0 {
		take := min(priorityMicro, creditedTotal)
		out = append(out, effects.TransferEffect{
			From:   ptypes.AccountName(PriorityPoolAccount),
			To:     ptypes.AccountName(distributionAccount),
			Amount: ptypes.NewCoins(ptypes.NewCoin(StakingDenom, take)),
		})
	}
	return out, nil, nil
}

// distributionAccount holds the tokens that are owed to delegators
// and validators but not yet claimed. Refilled at epoch close;
// drained by claim handlers.
const distributionAccount = "module.distribution"

// creditValidator computes share(v), the validator's commission cut,
// and bumps R_v + OutstandingCommission for that validator. Returns
// the total micro-tokens credited (commission + delegator pool) so
// the caller can drain the source pools by exactly that amount.
//
// Rounding is floor; the residual stays in the EmissionPool /
// PriorityPool for the next epoch.
func (m *BAPIDistributionModule) creditValidator(
	ctx context.Context,
	validatorHex string,
	pv *participation.ValidatorParticipation,
	totals *participation.EpochTotals,
	alphaScaled uint64,
	pool uint64,
) uint64 {
	// share(v) scaled by 10^18 for fixed-point math:
	//   leader_share  = leader_v / total_leader  × 10^18
	//   batches_share = batches_v / total_batches × 10^18
	//   share         = α · leader_share + (1−α) · batches_share
	//                 (α scaled by 10000)
	var leaderShare, batchesShare *big.Int
	if totals.LeaderBlocks > 0 {
		leaderShare = new(big.Int).SetUint64(pv.LeaderBlocks)
		leaderShare.Mul(leaderShare, new(big.Int).SetUint64(RewardScale))
		leaderShare.Div(leaderShare, new(big.Int).SetUint64(totals.LeaderBlocks))
	} else {
		leaderShare = new(big.Int)
	}
	if totals.BatchesCertified > 0 {
		batchesShare = new(big.Int).SetUint64(pv.BatchesCertified)
		batchesShare.Mul(batchesShare, new(big.Int).SetUint64(RewardScale))
		batchesShare.Div(batchesShare, new(big.Int).SetUint64(totals.BatchesCertified))
	} else {
		batchesShare = new(big.Int)
	}

	alphaBig := new(big.Int).SetUint64(alphaScaled)
	complementBig := new(big.Int).SetUint64(AlphaScale - alphaScaled)
	scale := new(big.Int).SetUint64(AlphaScale)

	share := new(big.Int).Mul(leaderShare, alphaBig)
	share.Add(share, new(big.Int).Mul(batchesShare, complementBig))
	share.Div(share, scale) // back to 10^18-scaled

	// r_v_micro = pool × share / 10^18  (floor)
	rvBig := new(big.Int).SetUint64(pool)
	rvBig.Mul(rvBig, share)
	rvBig.Div(rvBig, new(big.Int).SetUint64(RewardScale))
	if !rvBig.IsUint64() {
		return 0
	}
	rvMicro := rvBig.Uint64()
	if rvMicro == 0 {
		return 0
	}

	// Look up validator state. The participation key is a 20-byte
	// hex address; the validator store is keyed by 32-byte hex
	// pubkey. Cross-walk via GetValidatorByAddress.
	addrBytes, err := hex.DecodeString(validatorHex)
	if err != nil || len(addrBytes) != 20 {
		return 0
	}
	var addr types.ValidatorAddress
	copy(addr[:], addrBytes)
	validator, err := m.validatorStore.GetValidatorByAddress(addr)
	if err != nil || validator == nil {
		return 0
	}

	// commission_v = rvMicro × commission / 10000
	commission := uint64(validator.Commission)
	commissionMicro := rvMicro * commission / 10000
	delegatorPool := rvMicro - commissionMicro

	// ValidatorDistribution is keyed by full pubkey hex so the
	// claim handlers (which take pubkey from the message) and the
	// epoch-credit path agree on the lookup.
	pubKeyHex := hex.EncodeToString(validator.PubKey.Data)
	dist, err := m.validatorDist.Get(ctx, keyValidatorPrefix+pubKeyHex)
	if err != nil || dist == nil {
		dist = &ValidatorDistribution{}
	}
	dist.OutstandingCommissionMicro += commissionMicro

	// R_v += delegatorPool · 10^18 / total_stake
	if validator.TotalDelegation > 0 && delegatorPool > 0 {
		curRv := new(big.Int)
		if len(dist.RewardPerShareScaled) > 0 {
			curRv.SetBytes(dist.RewardPerShareScaled)
		}
		delta := new(big.Int).SetUint64(delegatorPool)
		delta.Mul(delta, new(big.Int).SetUint64(RewardScale))
		delta.Div(delta, new(big.Int).SetUint64(validator.TotalDelegation))
		curRv.Add(curRv, delta)
		dist.RewardPerShareScaled = curRv.Bytes()
	}
	dist.TotalStakeMicro = validator.TotalDelegation

	if err := m.validatorDist.Set(ctx, keyValidatorPrefix+pubKeyHex, dist); err != nil {
		// Mid-epoch persistence failure — log via return-0 to skip
		// this validator without aborting the whole settlement.
		return 0
	}
	return rvMicro
}

// handleWithdrawDelegatorReward pays the F1 claim for one
// (delegator, validator) pair. Computes stake × (R_v_now − R_start)
// and emits a TransferEffect from the distribution module account
// to the delegator.
func (m *BAPIDistributionModule) handleWithdrawDelegatorReward(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	mr, ok := msg.(*MsgWithdrawDelegatorReward)
	if !ok {
		return nil, fmt.Errorf("invalid message type: expected *MsgWithdrawDelegatorReward, got %T", msg)
	}
	if mr.Delegator != txCtx.Account {
		return nil, fmt.Errorf("delegator must be transaction account")
	}

	validatorHex := hex.EncodeToString(mr.Validator)
	dist, err := m.validatorDist.Get(ctx, keyValidatorPrefix+validatorHex)
	if err != nil || dist == nil {
		return nil, fmt.Errorf("validator distribution record not found")
	}

	delegationKey := string(mr.Delegator) + "/" + validatorHex
	delegRecord, _ := m.delegatorDist.Get(ctx, keyDelegatorPrefix+delegationKey)
	if delegRecord == nil {
		delegRecord = &DelegatorReward{}
	}

	// Refresh the cached stake from the staking module (delegator
	// may have changed their stake since the last snapshot). For
	// the no-slash base case the F1 formula uses the CURRENT
	// stake; Phase 3.6 will need per-period stake tracking.
	delegation, err := m.validatorStore.GetDelegation(ctx, string(mr.Delegator), mr.Validator)
	if err != nil {
		return nil, fmt.Errorf("get delegation: %w", err)
	}
	currentStake := delegation.Amount
	if delegRecord.StakeMicro == 0 {
		delegRecord.StakeMicro = currentStake
	}

	// rewards = stake × (R_v_now − R_start) / 10^18
	curRv := new(big.Int).SetBytes(dist.RewardPerShareScaled)
	startRv := new(big.Int).SetBytes(delegRecord.StartIndexScaled)
	deltaRv := new(big.Int).Sub(curRv, startRv)
	if deltaRv.Sign() < 0 {
		// Shouldn't happen — R_v is monotone non-decreasing in
		// the no-slash case. Treat as zero rewards.
		deltaRv.SetUint64(0)
	}
	rewardsBig := new(big.Int).SetUint64(delegRecord.StakeMicro)
	rewardsBig.Mul(rewardsBig, deltaRv)
	rewardsBig.Div(rewardsBig, new(big.Int).SetUint64(RewardScale))
	if !rewardsBig.IsUint64() {
		return nil, fmt.Errorf("rewards overflow uint64")
	}
	rewards := rewardsBig.Uint64()

	// Snapshot the current index + current stake on this record.
	delegRecord.StartIndexScaled = curRv.Bytes()
	delegRecord.StartPeriod = dist.CurrentPeriod
	delegRecord.StakeMicro = currentStake
	if err := m.delegatorDist.Set(ctx, keyDelegatorPrefix+delegationKey, delegRecord); err != nil {
		return nil, fmt.Errorf("persist delegator record: %w", err)
	}

	if rewards == 0 {
		return nil, nil
	}
	return []effects.Effect{
		effects.TransferEffect{
			From:   ptypes.AccountName(distributionAccount),
			To:     mr.Delegator,
			Amount: ptypes.NewCoins(ptypes.NewCoin(StakingDenom, rewards)),
		},
		effects.NewEventEffect("distribution.delegator_claimed", map[string][]byte{
			"delegator": []byte(mr.Delegator),
			"validator": []byte(validatorHex),
			"amount":    []byte(fmt.Sprintf("%d", rewards)),
		}),
	}, nil
}

// handleWithdrawValidatorCommission pays the validator's
// outstanding commission to the operator account. Per spec §4.4:
// "Validator operator commission ... credited to the operator's
// account on claim."
func (m *BAPIDistributionModule) handleWithdrawValidatorCommission(ctx context.Context, txCtx *runtime.BAPITxContext, msg ptypes.Message) ([]effects.Effect, error) {
	mr, ok := msg.(*MsgWithdrawValidatorCommission)
	if !ok {
		return nil, fmt.Errorf("invalid message type: got %T", msg)
	}
	if mr.Operator != txCtx.Account {
		return nil, fmt.Errorf("operator must be transaction account")
	}

	validatorHex := hex.EncodeToString(mr.Validator)
	dist, err := m.validatorDist.Get(ctx, keyValidatorPrefix+validatorHex)
	if err != nil || dist == nil {
		return nil, fmt.Errorf("validator distribution record not found")
	}
	if dist.OutstandingCommissionMicro == 0 {
		return nil, nil
	}

	amount := dist.OutstandingCommissionMicro
	dist.OutstandingCommissionMicro = 0
	if err := m.validatorDist.Set(ctx, keyValidatorPrefix+validatorHex, dist); err != nil {
		return nil, fmt.Errorf("persist validator dist: %w", err)
	}
	return []effects.Effect{
		effects.TransferEffect{
			From:   ptypes.AccountName(distributionAccount),
			To:     mr.Operator,
			Amount: ptypes.NewCoins(ptypes.NewCoin(StakingDenom, amount)),
		},
		effects.NewEventEffect("distribution.commission_claimed", map[string][]byte{
			"operator":  []byte(mr.Operator),
			"validator": []byte(validatorHex),
			"amount":    []byte(fmt.Sprintf("%d", amount)),
		}),
	}, nil
}

func (m *BAPIDistributionModule) handleQueryParams(ctx context.Context, _ []byte, _ int64) ([]byte, error) {
	p, err := m.paramsStore.Get(ctx, keyParams)
	if err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

func (m *BAPIDistributionModule) handleQueryValidator(ctx context.Context, data []byte, _ int64) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("validator hex required")
	}
	dist, err := m.validatorDist.Get(ctx, keyValidatorPrefix+string(data))
	if err != nil || dist == nil {
		return nil, fmt.Errorf("not found")
	}
	return json.Marshal(dist)
}

func (m *BAPIDistributionModule) handleQueryDelegatorReward(ctx context.Context, data []byte, _ int64) ([]byte, error) {
	// Expects "delegator/<hex-pubkey>" as the query payload.
	dr, err := m.delegatorDist.Get(ctx, keyDelegatorPrefix+string(data))
	if err != nil {
		return nil, err
	}
	return json.Marshal(dr)
}

var (
	_ runtime.BAPIModule             = (*BAPIDistributionModule)(nil)
	_ runtime.BAPIBlockProcessor     = (*BAPIDistributionModule)(nil)
	_ runtime.BAPIGenesisInitializer = (*BAPIDistributionModule)(nil)
	_ runtime.BAPIGenesisExporter    = (*BAPIDistributionModule)(nil)
)
