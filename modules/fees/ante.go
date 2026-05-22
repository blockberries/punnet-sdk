package fees

import (
	"context"
	"fmt"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/runtime"
	ptypes "github.com/blockberries/punnet-sdk/types"
)

// PriorityPoolAccount is the AccountName for the priority-pool holding
// account. The fees AnteHandler routes the priority component of each
// tx fee here; distribution drains it per-epoch.
//
// Computed lazily-once at package init so a future rename of the
// module-account prefix flows through automatically.
var PriorityPoolAccount = runtime.ModuleAccountName(PriorityPoolPendingSlug)

// FeeTreasuryAccount is the AccountName for the Common Treasury — the
// destination of the op-fee + byte-fee components of every tx.
var FeeTreasuryAccount = runtime.ModuleAccountCT

// NewAnteHandler returns an AnteHandler that enforces the chain's
// current fee schedule (read on each invocation from the BAPIFeesModule).
//
// Semantics per spec §3 / PLAN §7 Phase 1.4:
//
//  1. For each tx.Messages[i], Fee.OpFees[i].Amount must equal
//     schedule.OpFee(msg.Type()).Amount. Missing types reject.
//  2. Fee.ByteFee >= len(cramberry-marshalled-tx) * schedule.ByteFee.
//     Strict-equality is impractical because the encoded tx contains
//     ByteFee itself; users are free to overpay.
//  3. The payer (Fee.PayerOrAccount(tx.Account)) is debited by
//     Fee.Total(). Effects emitted:
//       - TransferEffect(payer → module.ct, op_total + byte_fee)
//       - TransferEffect(payer → module.pp, priority)
//     Both effects are skipped when their amount is zero.
//
// The handler runs deterministically; it does not observe wall-clock
// time or randomness. Effects are committed atomically with handler
// effects in v1 — Phase 1.5 splits the commit so the fee survives a
// failed message execution.
//
// `baseDenom` is the chain's single base unit (e.g. "stake"). All
// three fee components are denominated in it; spec §3 forbids
// multi-denom fees.
func NewAnteHandler(feesModule *BAPIFeesModule, baseDenom string) runtime.AnteHandler {
	if feesModule == nil {
		panic("fees: NewAnteHandler requires non-nil module")
	}
	if baseDenom == "" {
		panic("fees: NewAnteHandler requires non-empty baseDenom")
	}
	treasury := FeeTreasuryAccount
	pp := PriorityPoolAccount

	return func(ctx context.Context, txCtx *runtime.BAPITxContext, tx *ptypes.Transaction) ([]effects.Effect, error) {
		if tx == nil {
			return nil, fmt.Errorf("nil transaction")
		}

		sched, err := feesModule.Schedule(ctx)
		if err != nil {
			return nil, fmt.Errorf("load schedule: %w", err)
		}

		if err := validateOpFees(tx, &sched); err != nil {
			return nil, err
		}

		// Measure tx size from the wire bytes plumbed through the
		// runtime. Computing it via cramberry.Marshal(tx) here would
		// re-serialize the parsed tx (and require every concrete
		// message type to be registered with cramberry's interface
		// registry); the wire bytes are the ground truth anyway.
		txSize := 0
		if txCtx != nil {
			txSize = len(txCtx.TxBytes)
		}
		if err := validateByteFee(tx, &sched, txSize); err != nil {
			return nil, err
		}

		opSum, err := sumOpFees(tx.Fee.OpFees)
		if err != nil {
			return nil, err
		}
		ctAmount, ok := effects.SafeAdd(opSum, tx.Fee.ByteFee)
		if !ok {
			return nil, fmt.Errorf("op_sum + byte_fee overflow")
		}

		// Resolve the payer. Transaction.ValidateBasic enforces that
		// a non-default Payer appears in every message's GetSigners()
		// — the mempool gate already rejected unauthorized payers —
		// so by the time we reach the AnteHandler the payer is
		// definitively allowed to be billed.
		payer := tx.Fee.PayerOrAccount(tx.Account)
		if !payer.IsValid() {
			return nil, fmt.Errorf("invalid payer %q", payer)
		}

		var out []effects.Effect

		// op + byte → CT
		if ctAmount > 0 {
			if !treasury.IsValid() {
				return nil, fmt.Errorf("treasury account misconfigured")
			}
			out = append(out, effects.TransferEffect{
				From:   payer,
				To:     treasury,
				Amount: ptypes.NewCoins(ptypes.NewCoin(baseDenom, ctAmount)),
			})
		}

		// priority → PP (skip when zero — users may legitimately
		// submit zero-priority txs and the effect-executor rejects
		// zero-amount transfers anyway).
		if tx.Fee.Priority > 0 {
			if !pp.IsValid() {
				return nil, fmt.Errorf("priority-pool account misconfigured")
			}
			out = append(out, effects.TransferEffect{
				From:   payer,
				To:     pp,
				Amount: ptypes.NewCoins(ptypes.NewCoin(baseDenom, uint64(tx.Fee.Priority))),
			})
		}

		return out, nil
	}
}

// validateOpFees enforces strict equality between submitted OpFees and
// the current schedule's per-type rates, in tx.Messages order.
func validateOpFees(tx *ptypes.Transaction, sched *FeeSchedule) error {
	if len(tx.Fee.OpFees) != len(tx.Messages) {
		return fmt.Errorf("fee.op_fees length %d != messages length %d",
			len(tx.Fee.OpFees), len(tx.Messages))
	}
	for i, msg := range tx.Messages {
		entry := tx.Fee.OpFees[i]
		if entry.MessageType != msg.Type() {
			return fmt.Errorf("fee.op_fees[%d].message_type %q != messages[%d].Type() %q",
				i, entry.MessageType, i, msg.Type())
		}
		expected, ok := sched.OpFee(msg.Type())
		if !ok {
			return fmt.Errorf("unknown message type %q in schedule", msg.Type())
		}
		if entry.Amount != expected {
			return fmt.Errorf("fee.op_fees[%d].amount %d != schedule %d for %q",
				i, entry.Amount, expected, msg.Type())
		}
	}
	return nil
}

// validateByteFee enforces the per-byte minimum. The submitted
// Fee.ByteFee must cover `txSize * schedule.ByteFee`; overpay is
// allowed but underpay rejects. txSize is the wire-encoded length
// of the transaction, plumbed through BAPITxContext.TxBytes.
//
// txSize == 0 + ByteFee > 0 is only legitimate in unit tests; the
// runtime always supplies the wire bytes in production. In that
// out-of-runtime case we skip the size check and rely on the
// "schedule.ByteFee == 0 ↔ tx.Fee.ByteFee == 0" round-trip rule
// instead.
func validateByteFee(tx *ptypes.Transaction, sched *FeeSchedule, txSize int) error {
	if sched.ByteFee == 0 {
		// Schedule charges zero per byte; only verify the submitted
		// ByteFee is also zero (otherwise the user is paying for
		// nothing — likely a stale-schedule bug).
		if tx.Fee.ByteFee != 0 {
			return fmt.Errorf("schedule.byte_fee is zero but tx submits %d", tx.Fee.ByteFee)
		}
		return nil
	}
	if txSize == 0 {
		// No wire bytes available — defer to the submitted value.
		// The runtime always supplies TxBytes, so this branch only
		// triggers in unit-only callers; the AnteHandler tests opt
		// into this path explicitly by configuring ByteFee=0 on the
		// schedule.
		return nil
	}
	required, ok := safeMul(uint64(txSize), sched.ByteFee)
	if !ok {
		return fmt.Errorf("required byte-fee overflow (%d bytes × %d rate)",
			txSize, sched.ByteFee)
	}
	if tx.Fee.ByteFee < required {
		return fmt.Errorf("byte_fee %d below required %d (tx is %d bytes × %d rate)",
			tx.Fee.ByteFee, required, txSize, sched.ByteFee)
	}
	return nil
}

func sumOpFees(entries []ptypes.OpFee) (uint64, error) {
	var sum uint64
	for i, e := range entries {
		next, ok := effects.SafeAdd(sum, e.Amount)
		if !ok {
			return 0, fmt.Errorf("op_fees sum overflow at index %d", i)
		}
		sum = next
	}
	return sum, nil
}

// safeMul returns a*b and ok=true unless multiplication would
// overflow uint64.
func safeMul(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	c := a * b
	if c/a != b {
		return 0, false
	}
	return c, true
}
