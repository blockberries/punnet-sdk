package auth

import (
	"fmt"

	"github.com/blockberries/punnet-sdk/types"
)

// Message type identifiers
const (
	TypeMsgCreateAccount    = "/punnet.auth.v1.MsgCreateAccount"
	TypeMsgUpdateAuthority  = "/punnet.auth.v1.MsgUpdateAuthority"
	TypeMsgDeleteAccount    = "/punnet.auth.v1.MsgDeleteAccount"
)

// MsgCreateAccount creates a new account
type MsgCreateAccount struct {
	// Name is the new account name
	Name types.AccountName `json:"name"`

	// PubKey is the public key for the account
	PubKey []byte `json:"pub_key"`

	// Authority is the authorization structure
	Authority types.Authority `json:"authority"`
}

// Type returns the message type
func (m *MsgCreateAccount) Type() string {
	return TypeMsgCreateAccount
}

// ValidateBasic performs stateless validation
func (m *MsgCreateAccount) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message is nil")
	}

	if !m.Name.IsValid() {
		return fmt.Errorf("%w: invalid account name %s", types.ErrInvalidAccount, m.Name)
	}

	if len(m.PubKey) == 0 {
		return fmt.Errorf("public key cannot be empty")
	}

	if err := m.Authority.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid authority: %w", err)
	}

	return nil
}

// GetSigners returns the accounts that must authorize this message
func (m *MsgCreateAccount) GetSigners() []types.AccountName {
	if m == nil {
		return nil
	}
	// The account creating itself must sign
	return []types.AccountName{m.Name}
}

// MsgUpdateAuthority updates an account's authority
type MsgUpdateAuthority struct {
	// Name is the account to update
	Name types.AccountName `json:"name"`

	// NewAuthority is the new authority structure
	NewAuthority types.Authority `json:"new_authority"`
}

// Type returns the message type
func (m *MsgUpdateAuthority) Type() string {
	return TypeMsgUpdateAuthority
}

// ValidateBasic performs stateless validation
func (m *MsgUpdateAuthority) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message is nil")
	}

	if !m.Name.IsValid() {
		return fmt.Errorf("%w: invalid account name %s", types.ErrInvalidAccount, m.Name)
	}

	if err := m.NewAuthority.ValidateBasic(); err != nil {
		return fmt.Errorf("invalid authority: %w", err)
	}

	return nil
}

// GetSigners returns the accounts that must authorize this message
func (m *MsgUpdateAuthority) GetSigners() []types.AccountName {
	if m == nil {
		return nil
	}
	// The account being updated must sign
	return []types.AccountName{m.Name}
}

// MsgDeleteAccount deletes an account
type MsgDeleteAccount struct {
	// Name is the account to delete
	Name types.AccountName `json:"name"`
}

// Type returns the message type
func (m *MsgDeleteAccount) Type() string {
	return TypeMsgDeleteAccount
}

// ValidateBasic performs stateless validation
func (m *MsgDeleteAccount) ValidateBasic() error {
	if m == nil {
		return fmt.Errorf("message is nil")
	}

	if !m.Name.IsValid() {
		return fmt.Errorf("%w: invalid account name %s", types.ErrInvalidAccount, m.Name)
	}

	return nil
}

// GetSigners returns the accounts that must authorize this message
func (m *MsgDeleteAccount) GetSigners() []types.AccountName {
	if m == nil {
		return nil
	}
	// The account being deleted must sign
	return []types.AccountName{m.Name}
}
