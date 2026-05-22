package fees

import (
	"fmt"
	"sort"
)

// ModuleName is the unique module name used for genesis blob keying
// and for RegisterModule discovery.
const ModuleName = "fees"

// StorePrefix is the typed-store prefix for the fees module's state.
// Sub-keys live under this prefix; the runtime sees a single
// flat namespace. The trailing slash matches the bank/account/proposal
// store convention so a key like "current" lands at "fees/current"
// rather than "feescurrent".
const StorePrefix = "fees/"

// PriorityPoolPendingSlug is the bank-namespace slug for the
// priority-pool holding account. The fees AnteHandler routes the
// priority component of each tx fee here; the distribution module
// drains the account per-epoch into validator reward accumulators
// (Phase 3). PLAN §7 decision D14 (priority is a fixed token amount).
const PriorityPoolPendingSlug = "pp"

// OpFeeEntry is one row of the OpFees table — the fee charged per
// occurrence of a given message type. Stored as a sorted slice (by
// MessageType) for deterministic cramberry encoding.
type OpFeeEntry struct {
	MessageType string `cramberry:"1" json:"message_type"`
	Amount      uint64 `cramberry:"2" json:"amount"`
}

// FeeSchedule is the chain-wide fee schedule consulted by the
// AnteHandler. OpFees MUST be sorted by MessageType (enforced by
// Validate); ByteFee is the per-byte component charged on the
// signed-tx-bytes length at inclusion time.
//
// Apps that want different fee shapes (e.g., a percentage of msg
// value) can layer their own AnteHandler on top — this is the
// canonical "flat per-op + per-byte" schedule.
type FeeSchedule struct {
	OpFees  []OpFeeEntry `cramberry:"1" json:"op_fees"`
	ByteFee uint64       `cramberry:"2" json:"byte_fee"`
}

// Validate enforces the schedule's structural invariants: OpFees is
// sorted by MessageType, no duplicates, no empty message types.
// Called at every read site that mutates the schedule so future
// governance-driven updates (Phase 4) cannot install a malformed
// table.
func (s *FeeSchedule) Validate() error {
	if s == nil {
		return fmt.Errorf("fee schedule is nil")
	}
	for i, op := range s.OpFees {
		if op.MessageType == "" {
			return fmt.Errorf("op_fees[%d].message_type empty", i)
		}
		if i > 0 && s.OpFees[i-1].MessageType >= op.MessageType {
			return fmt.Errorf("op_fees not sorted by message_type at index %d (%q !< %q)",
				i, s.OpFees[i-1].MessageType, op.MessageType)
		}
	}
	return nil
}

// OpFee returns the per-message-type fee for messageType, plus a
// bool indicating whether the type is in the schedule. Missing types
// are NOT charged a fee at this lookup — the AnteHandler decides
// how to treat unknown types (the v1 policy is "reject the tx with
// an unknown_message_type code"; Phase 1.4 enforces it).
//
// Binary search over the sorted slice; O(log N) for N message types.
func (s *FeeSchedule) OpFee(messageType string) (uint64, bool) {
	if s == nil {
		return 0, false
	}
	i := sort.Search(len(s.OpFees), func(i int) bool {
		return s.OpFees[i].MessageType >= messageType
	})
	if i < len(s.OpFees) && s.OpFees[i].MessageType == messageType {
		return s.OpFees[i].Amount, true
	}
	return 0, false
}

// SortedOpFees returns a defensive copy of OpFees in sorted order,
// suitable for marshaling without disturbing the original. Useful
// for tests and for governance proposal construction that doesn't
// trust the caller to have sorted the slice already.
func SortedOpFees(entries []OpFeeEntry) []OpFeeEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]OpFeeEntry, len(entries))
	copy(out, entries)
	sort.Slice(out, func(i, j int) bool {
		return out[i].MessageType < out[j].MessageType
	})
	return out
}

// PendingFeeUpdate is a queued schedule change waiting for its
// effective height. Today these can only be inserted by direct
// genesis or by InitGenesis seed; Phase 4 wires the governance
// proposal type that creates them after a 7-day timelock.
//
// The runtime's EndBlock walks the pending list at every height; on
// the first height ≥ EffectiveHeight the Schedule replaces the
// current schedule and the entry is removed.
type PendingFeeUpdate struct {
	EffectiveHeight uint64      `cramberry:"1" json:"effective_height"`
	Schedule        FeeSchedule `cramberry:"2" json:"schedule"`
}
