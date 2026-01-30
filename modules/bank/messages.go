package bank

import (
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// Message type identifiers
const (
	TypeMsgSend      = "/punnet.bank.v1.MsgSend"
	TypeMsgMultiSend = "/punnet.bank.v1.MsgMultiSend"
)

// MsgSend transfers coins from one account to another
type MsgSend struct {
	// From is the sender account
	From types.AccountName `json:"from"`

	// To is the recipient account
	To types.AccountName `json:"to"`

	// Amount is the amount to send
	Amount types.Coin `json:"amount"`
}

// Type returns the message type
func (m *MsgSend) Type() string {
	return TypeMsgSend
}

// ValidateBasic performs stateless validation
func (m *MsgSend) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message is nil")
	}

	if !m.From.IsValid() {
		return fmt.Errorf("%w: invalid sender account %s", types.ErrInvalidAccount, m.From)
	}

	if !m.To.IsValid() {
		return fmt.Errorf("%w: invalid recipient account %s", types.ErrInvalidAccount, m.To)
	}

	if m.From == m.To {
		return fmt.Errorf("cannot send to self")
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
func (m *MsgSend) GetSigners() []types.AccountName {
	if m == nil {
		return nil
	}
	return []types.AccountName{m.From}
}

// Input represents an input for multi-send
type Input struct {
	// Address is the sender account
	Address types.AccountName `json:"address"`

	// Coins is the amount to send
	Coins types.Coins `json:"coins"`
}

// ValidateBasic performs stateless validation
func (i *Input) ValidateBasic() error {
	if i == nil {
		return fmt.Errorf("input is nil")
	}

	if !i.Address.IsValid() {
		return fmt.Errorf("%w: invalid address %s", types.ErrInvalidAccount, i.Address)
	}

	if !i.Coins.IsValid() {
		return fmt.Errorf("invalid coins")
	}

	if !i.Coins.IsAllPositive() {
		return fmt.Errorf("all coin amounts must be positive")
	}

	return nil
}

// Output represents an output for multi-send
type Output struct {
	// Address is the recipient account
	Address types.AccountName `json:"address"`

	// Coins is the amount to receive
	Coins types.Coins `json:"coins"`
}

// ValidateBasic performs stateless validation
func (o *Output) ValidateBasic() error {
	if o == nil {
		return fmt.Errorf("output is nil")
	}

	if !o.Address.IsValid() {
		return fmt.Errorf("%w: invalid address %s", types.ErrInvalidAccount, o.Address)
	}

	if !o.Coins.IsValid() {
		return fmt.Errorf("invalid coins")
	}

	if !o.Coins.IsAllPositive() {
		return fmt.Errorf("all coin amounts must be positive")
	}

	return nil
}

// MsgMultiSend transfers coins from multiple accounts to multiple accounts
type MsgMultiSend struct {
	// Inputs are the sender accounts and amounts
	Inputs []Input `json:"inputs"`

	// Outputs are the recipient accounts and amounts
	Outputs []Output `json:"outputs"`
}

// Type returns the message type
func (m *MsgMultiSend) Type() string {
	return TypeMsgMultiSend
}

// ValidateBasic performs stateless validation
func (m *MsgMultiSend) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message is nil")
	}

	if len(m.Inputs) == 0 {
		return fmt.Errorf("inputs cannot be empty")
	}

	if len(m.Outputs) == 0 {
		return fmt.Errorf("outputs cannot be empty")
	}

	// Validate all inputs
	for i, input := range m.Inputs {
		if err := input.ValidateBasic(); err != nil {
			return fmt.Errorf("input %d: %w", i, err)
		}
	}

	// Validate all outputs
	for i, output := range m.Outputs {
		if err := output.ValidateBasic(); err != nil {
			return fmt.Errorf("output %d: %w", i, err)
		}
	}

	// Calculate total inputs and outputs with overflow protection
	totalInputs := make(map[string]uint64)
	for _, input := range m.Inputs {
		for _, coin := range input.Coins {
			existing := totalInputs[coin.Denom]
			// Check for overflow
			if existing > ^uint64(0)-coin.Amount {
				return fmt.Errorf("input total overflow for denom %s", coin.Denom)
			}
			totalInputs[coin.Denom] = existing + coin.Amount
		}
	}

	totalOutputs := make(map[string]uint64)
	for _, output := range m.Outputs {
		for _, coin := range output.Coins {
			existing := totalOutputs[coin.Denom]
			// Check for overflow
			if existing > ^uint64(0)-coin.Amount {
				return fmt.Errorf("output total overflow for denom %s", coin.Denom)
			}
			totalOutputs[coin.Denom] = existing + coin.Amount
		}
	}

	// Verify inputs equal outputs for each denomination
	for denom, inputAmount := range totalInputs {
		outputAmount := totalOutputs[denom]
		if inputAmount != outputAmount {
			return fmt.Errorf("total inputs and outputs must be equal for denom %s: inputs=%d, outputs=%d",
				denom, inputAmount, outputAmount)
		}
	}

	// Verify no extra outputs
	for denom := range totalOutputs {
		if _, ok := totalInputs[denom]; !ok {
			return fmt.Errorf("output denom %s not present in inputs", denom)
		}
	}

	return nil
}

// GetSigners returns the accounts that must authorize this message
func (m *MsgMultiSend) GetSigners() []types.AccountName {
	if m == nil || len(m.Inputs) == 0 {
		return nil
	}

	// All input addresses must sign
	signers := make([]types.AccountName, 0, len(m.Inputs))
	seen := make(map[types.AccountName]bool)

	for _, input := range m.Inputs {
		if !seen[input.Address] {
			signers = append(signers, input.Address)
			seen[input.Address] = true
		}
	}

	return signers
}
