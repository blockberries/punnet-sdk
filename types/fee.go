package types

import (
	"fmt"
	"math"
)

// MaxOpFees bounds the OpFees slice to prevent DoS via iteration on the
// AnteHandler / GetSignBytes hot paths. A single tx never legitimately
// carries more than a few messages — 64 leaves headroom for atomic-bundle
// patterns without admitting pathological inputs.
const MaxOpFees = 64

// OpFee is the per-operation fee component for a single message in the
// transaction. Pre-computed from the FeeSchedule at tx-construction time
// so the AnteHandler can validate by comparison rather than recomputation.
//
// MessageType MUST match the Type() of the corresponding tx.Messages[i].
// Amount is denominated in the chain's base unit (no multi-denom).
type OpFee struct {
	MessageType string `cramberry:"1" json:"message_type"`
	Amount      uint64 `cramberry:"2" json:"amount"`
}

// Fee carries the three-component fee payload per the tokenomics spec §3.
//
// Total fee for a tx is `sum(OpFees) + ByteFee + Priority`. Routing at
// the AnteHandler:
//
//   - OpFees + ByteFee → CT (Common Treasury)
//   - Priority         → Priority Pool (drained per-epoch by distribution)
//
// All three components are denominated in the chain's single base unit;
// there is no multi-denom or gas model. Per-op fees are governance-set
// via the FeeSchedule; per-byte fees are governance-set; priority is
// user-set as a fixed token amount (not relative to OpFee/ByteFee — see
// PLAN §7 decision D14).
//
// Payer defaults to the transaction's signing Account but can differ in
// delegated-payment models. Both Account and Payer (if different) must
// be in the GetSigners() set so the authorization actually covers the
// fee charge.
type Fee struct {
	// OpFees carries one entry per message in tx.Messages, in the same
	// order. OpFees[i].MessageType MUST equal tx.Messages[i].Type().
	OpFees []OpFee `cramberry:"1" json:"op_fees"`

	// ByteFee is the per-byte component: `len(signed_tx_bytes) *
	// schedule.ByteFee` evaluated at tx-construction time. The
	// AnteHandler recomputes from the included tx and verifies
	// equality.
	ByteFee uint64 `cramberry:"2" json:"byte_fee"`

	// Priority is the user-set bid for mempool ordering. Forwarded to
	// the looseberry worker's priority heap via the bapi
	// GateVerdict.Priority field. MUST be ≥ 0.
	Priority int64 `cramberry:"3" json:"priority"`

	// Payer is the account charged for this fee. Empty means
	// tx.Account pays. When non-empty, the AnteHandler verifies
	// Payer is among tx.GetSigners() so the signature covers the
	// charge.
	Payer AccountName `cramberry:"4" json:"payer,omitempty"`
}

// ValidateBasic performs stateless validation of a Fee.
//
// Checks:
//   - OpFees slice is bounded (MaxOpFees) — DoS guard.
//   - No duplicate MessageType in OpFees (each msg slot is single-valued).
//   - Priority is non-negative.
//   - Payer, if set, is a valid AccountName.
//   - Total fee doesn't overflow uint64 across the three components.
//
// Per-message-type matching against tx.Messages happens in Transaction.
// ValidateBasic — Fee alone has no view onto the message list.
func (f *Fee) ValidateBasic() error {
	if f == nil {
		return fmt.Errorf("%w: fee is nil", ErrInvalidTransaction)
	}

	if len(f.OpFees) > MaxOpFees {
		return fmt.Errorf("%w: op_fees length %d exceeds max %d",
			ErrInvalidTransaction, len(f.OpFees), MaxOpFees)
	}

	seen := make(map[string]struct{}, len(f.OpFees))
	for i, op := range f.OpFees {
		if op.MessageType == "" {
			return fmt.Errorf("%w: op_fees[%d].message_type empty",
				ErrInvalidTransaction, i)
		}
		if _, dup := seen[op.MessageType]; dup {
			return fmt.Errorf("%w: op_fees[%d] duplicate message_type %q",
				ErrInvalidTransaction, i, op.MessageType)
		}
		seen[op.MessageType] = struct{}{}
	}

	if f.Priority < 0 {
		return fmt.Errorf("%w: priority %d must be non-negative",
			ErrInvalidTransaction, f.Priority)
	}

	if f.Payer != "" && !f.Payer.IsValid() {
		return fmt.Errorf("%w: payer %q invalid", ErrInvalidAccount, f.Payer)
	}

	// Overflow guard on Total() so callers can rely on it not panicking.
	if _, err := f.Total(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTransaction, err)
	}

	return nil
}

// Total returns the total fee charged for this transaction:
// sum(OpFees) + ByteFee + Priority. Returns an error if the sum
// overflows uint64.
//
// Priority is added as uint64 — ValidateBasic guarantees it's
// non-negative.
func (f *Fee) Total() (uint64, error) {
	var sum uint64
	for i, op := range f.OpFees {
		if math.MaxUint64-sum < op.Amount {
			return 0, fmt.Errorf("total fee overflow at op_fees[%d]", i)
		}
		sum += op.Amount
	}
	if math.MaxUint64-sum < f.ByteFee {
		return 0, fmt.Errorf("total fee overflow at byte_fee")
	}
	sum += f.ByteFee
	if f.Priority < 0 {
		return 0, fmt.Errorf("priority must be non-negative; got %d", f.Priority)
	}
	prio := uint64(f.Priority)
	if math.MaxUint64-sum < prio {
		return 0, fmt.Errorf("total fee overflow at priority")
	}
	sum += prio
	return sum, nil
}

// PayerOrAccount returns the account charged for this fee, falling back
// to `account` (the transaction's signing account) when Payer is empty.
func (f *Fee) PayerOrAccount(account AccountName) AccountName {
	if f.Payer != "" {
		return f.Payer
	}
	return account
}
