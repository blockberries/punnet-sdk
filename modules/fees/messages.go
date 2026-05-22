package fees

import (
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

func init() {
	types.RegisterMessage(TypeMsgProposeFeeUpdate, func() types.Message {
		return &MsgProposeFeeUpdate{}
	})
}

// Message type identifiers.
const (
	TypeMsgProposeFeeUpdate = "/punnet.fees.v1.MsgProposeFeeUpdate"
)

// MsgProposeFeeUpdate proposes a replacement FeeSchedule that takes
// effect at EffectiveHeight. Direct submission is rejected in v1;
// Phase 4 (governance) wires the timelock that creates these
// messages after a passing proposal vote.
//
// The message exists today so the wire format is stable from Phase 1
// onward and so the message registry includes it for serialization
// round-trips.
type MsgProposeFeeUpdate struct {
	// Proposer is the account submitting the proposal. Must match
	// the tx signer's account.
	Proposer types.AccountName `json:"proposer"`

	// Schedule is the proposed replacement schedule. OpFees may be
	// passed unsorted; the handler sorts before persisting.
	Schedule FeeSchedule `json:"schedule"`

	// EffectiveHeight is the block height at which Schedule
	// replaces the current schedule. Must be > current height +
	// the class's timelock minimum (enforced by governance in
	// Phase 4).
	EffectiveHeight uint64 `json:"effective_height"`
}

// Type returns the message type identifier.
func (m *MsgProposeFeeUpdate) Type() string { return TypeMsgProposeFeeUpdate }

// ValidateBasic performs stateless validation.
func (m *MsgProposeFeeUpdate) ValidateBasic() error {
	if !m.Proposer.IsValid() {
		return fmt.Errorf("invalid proposer %q", m.Proposer)
	}
	if m.EffectiveHeight == 0 {
		return fmt.Errorf("effective_height must be > 0")
	}
	// Schedule is validated post-sort in InitGenesis / handler;
	// catch the obvious "duplicate or empty type" case stateless.
	seen := make(map[string]struct{}, len(m.Schedule.OpFees))
	for i, op := range m.Schedule.OpFees {
		if op.MessageType == "" {
			return fmt.Errorf("schedule.op_fees[%d].message_type empty", i)
		}
		if _, dup := seen[op.MessageType]; dup {
			return fmt.Errorf("schedule.op_fees[%d].message_type %q duplicated", i, op.MessageType)
		}
		seen[op.MessageType] = struct{}{}
	}
	return nil
}

// GetSigners returns the set of accounts whose signature must cover
// this message.
func (m *MsgProposeFeeUpdate) GetSigners() []types.AccountName {
	return []types.AccountName{m.Proposer}
}

// FeesGenesisState is the per-module genesis blob. Carries the
// initial fee schedule and any pending updates seeded by the
// genesis operator (typically empty — Phase 4 governance is the
// usual source of pending updates).
type FeesGenesisState struct {
	Schedule FeeSchedule        `json:"schedule"`
	Pending  []PendingFeeUpdate `json:"pending,omitempty"`
}
