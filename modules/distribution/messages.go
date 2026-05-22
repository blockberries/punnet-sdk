package distribution

import (
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

func init() {
	types.RegisterMessage(TypeMsgWithdrawDelegatorReward, func() types.Message {
		return &MsgWithdrawDelegatorReward{}
	})
	types.RegisterMessage(TypeMsgWithdrawValidatorCommission, func() types.Message {
		return &MsgWithdrawValidatorCommission{}
	})
}

const (
	TypeMsgWithdrawDelegatorReward     = "/punnet.distribution.v1.MsgWithdrawDelegatorReward"
	TypeMsgWithdrawValidatorCommission = "/punnet.distribution.v1.MsgWithdrawValidatorCommission"
)

// MsgWithdrawDelegatorReward triggers the F1 claim path for one
// (delegator, validator) pair. The handler computes stake ×
// (R_v_now − StartIndex), credits the delegator's account, and
// snapshots R_v_now as the new StartIndex.
//
// Spec §4.4 mandates this is a per-delegator-per-validator
// operation; bulk-claim across all of a delegator's validators is
// a client-side convenience.
type MsgWithdrawDelegatorReward struct {
	Delegator types.AccountName `json:"delegator"`
	Validator []byte            `json:"validator"`
}

func (m *MsgWithdrawDelegatorReward) Type() string { return TypeMsgWithdrawDelegatorReward }

func (m *MsgWithdrawDelegatorReward) ValidateBasic() error {
	if !m.Delegator.IsValid() {
		return fmt.Errorf("invalid delegator %q", m.Delegator)
	}
	if len(m.Validator) == 0 {
		return fmt.Errorf("validator pubkey required")
	}
	return nil
}

func (m *MsgWithdrawDelegatorReward) GetSigners() []types.AccountName {
	return []types.AccountName{m.Delegator}
}

// MsgWithdrawValidatorCommission lets the validator operator
// withdraw their accrued commission. Separate from the delegator
// claim because commission accumulates on a different accumulator
// (OutstandingCommissionMicro) and does not interact with the F1
// R_v index.
type MsgWithdrawValidatorCommission struct {
	// Operator is the validator's operator account (the account
	// that submitted MsgCreateValidator). Must equal the tx
	// signer.
	Operator types.AccountName `json:"operator"`
	// Validator is the hex-encoded validator pubkey.
	Validator []byte `json:"validator"`
}

func (m *MsgWithdrawValidatorCommission) Type() string {
	return TypeMsgWithdrawValidatorCommission
}

func (m *MsgWithdrawValidatorCommission) ValidateBasic() error {
	if !m.Operator.IsValid() {
		return fmt.Errorf("invalid operator %q", m.Operator)
	}
	if len(m.Validator) == 0 {
		return fmt.Errorf("validator pubkey required")
	}
	return nil
}

func (m *MsgWithdrawValidatorCommission) GetSigners() []types.AccountName {
	return []types.AccountName{m.Operator}
}

// DistributionGenesisState is the per-module genesis blob.
type DistributionGenesisState struct {
	Params *DistributionParams `json:"params,omitempty"`
}
