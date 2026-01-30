package staking

import (
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// Message type identifiers
const (
	TypeMsgCreateValidator = "/punnet.staking.v1.MsgCreateValidator"
	TypeMsgDelegate        = "/punnet.staking.v1.MsgDelegate"
	TypeMsgUndelegate      = "/punnet.staking.v1.MsgUndelegate"
)

// MsgCreateValidator creates a new validator
type MsgCreateValidator struct {
	// Delegator is the account that controls this validator
	Delegator types.AccountName `json:"delegator"`

	// PubKey is the validator's public key
	PubKey []byte `json:"pub_key"`

	// InitialPower is the initial voting power
	InitialPower int64 `json:"initial_power"`

	// Commission is the commission rate (0-10000, where 10000 = 100%)
	Commission uint64 `json:"commission"`
}

// Type returns the message type
func (m *MsgCreateValidator) Type() string {
	return TypeMsgCreateValidator
}

// ValidateBasic performs stateless validation
func (m *MsgCreateValidator) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message is nil")
	}

	if !m.Delegator.IsValid() {
		return fmt.Errorf("%w: invalid delegator account %s", types.ErrInvalidAccount, m.Delegator)
	}

	if len(m.PubKey) == 0 {
		return fmt.Errorf("public key cannot be empty")
	}

	if m.InitialPower < 0 {
		return fmt.Errorf("initial power cannot be negative")
	}

	if m.Commission > 10000 {
		return fmt.Errorf("commission cannot exceed 100%%")
	}

	return nil
}

// GetSigners returns the accounts that must authorize this message
func (m *MsgCreateValidator) GetSigners() []types.AccountName {
	if m == nil {
		return nil
	}
	return []types.AccountName{m.Delegator}
}

// MsgDelegate delegates tokens to a validator
type MsgDelegate struct {
	// Delegator is the account delegating
	Delegator types.AccountName `json:"delegator"`

	// Validator is the validator's public key
	Validator []byte `json:"validator"`

	// Amount is the amount to delegate
	Amount types.Coin `json:"amount"`
}

// Type returns the message type
func (m *MsgDelegate) Type() string {
	return TypeMsgDelegate
}

// ValidateBasic performs stateless validation
func (m *MsgDelegate) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message is nil")
	}

	if !m.Delegator.IsValid() {
		return fmt.Errorf("%w: invalid delegator account %s", types.ErrInvalidAccount, m.Delegator)
	}

	if len(m.Validator) == 0 {
		return fmt.Errorf("validator public key cannot be empty")
	}

	if !m.Amount.IsValid() {
		return fmt.Errorf("invalid amount")
	}

	if !m.Amount.IsPositive() {
		return fmt.Errorf("amount must be positive")
	}

	return nil
}

// GetSigners returns the accounts that must authorize this message
func (m *MsgDelegate) GetSigners() []types.AccountName {
	if m == nil {
		return nil
	}
	return []types.AccountName{m.Delegator}
}

// MsgUndelegate removes delegation from a validator
type MsgUndelegate struct {
	// Delegator is the account undelegating
	Delegator types.AccountName `json:"delegator"`

	// Validator is the validator's public key
	Validator []byte `json:"validator"`

	// Amount is the amount to undelegate
	Amount types.Coin `json:"amount"`
}

// Type returns the message type
func (m *MsgUndelegate) Type() string {
	return TypeMsgUndelegate
}

// ValidateBasic performs stateless validation
func (m *MsgUndelegate) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message is nil")
	}

	if !m.Delegator.IsValid() {
		return fmt.Errorf("%w: invalid delegator account %s", types.ErrInvalidAccount, m.Delegator)
	}

	if len(m.Validator) == 0 {
		return fmt.Errorf("validator public key cannot be empty")
	}

	if !m.Amount.IsValid() {
		return fmt.Errorf("invalid amount")
	}

	if !m.Amount.IsPositive() {
		return fmt.Errorf("amount must be positive")
	}

	return nil
}

// GetSigners returns the accounts that must authorize this message
func (m *MsgUndelegate) GetSigners() []types.AccountName {
	if m == nil {
		return nil
	}
	return []types.AccountName{m.Delegator}
}
