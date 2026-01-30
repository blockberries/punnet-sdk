package types

import (
	"fmt"
	"regexp"
	"time"
)

// AccountName is a human-readable account identifier
type AccountName string

// String converts AccountName to string
func (a AccountName) String() string {
	return string(a)
}

// IsValid checks if the account name is valid
func (a AccountName) IsValid() bool {
	if len(a) == 0 || len(a) > 64 {
		return false
	}
	// Account names must match: ^[a-z0-9.]+$
	matched, _ := regexp.MatchString("^[a-z0-9.]+$", string(a))
	return matched
}

// Account represents a named account with hierarchical permissions
type Account struct {
	// Name is the human-readable account identifier
	Name AccountName `json:"name"`

	// Authority defines who can act on behalf of this account
	Authority Authority `json:"authority"`

	// Nonce prevents replay attacks
	Nonce uint64 `json:"nonce"`

	// CreatedAt is when the account was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the account was last modified
	UpdatedAt time.Time `json:"updated_at"`
}

// NewAccount creates a new account with default authority
func NewAccount(name AccountName, pubKey []byte) *Account {
	return &Account{
		Name: name,
		Authority: Authority{
			Threshold:    1,
			KeyWeights:   map[string]uint64{string(pubKey): 1},
			AccountWeights: make(map[AccountName]uint64),
		},
		Nonce:     0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// ValidateBasic performs basic validation
func (a *Account) ValidateBasic() error {
	if !a.Name.IsValid() {
		return fmt.Errorf("%w: %s", ErrInvalidAccount, a.Name)
	}
	if err := a.Authority.ValidateBasic(); err != nil {
		return err
	}
	return nil
}

// Authority defines who can authorize actions for an account
type Authority struct {
	// Threshold is the minimum weight required for authorization
	Threshold uint64 `json:"threshold"`

	// KeyWeights maps public keys to their authorization weight
	KeyWeights map[string]uint64 `json:"key_weights"`

	// AccountWeights maps account names to their delegation weight
	// This enables hierarchical permissions where accounts can delegate authority
	AccountWeights map[AccountName]uint64 `json:"account_weights"`
}

// NewAuthority creates a new authority with a single key
func NewAuthority(threshold uint64, pubKey []byte, weight uint64) Authority {
	return Authority{
		Threshold:      threshold,
		KeyWeights:     map[string]uint64{string(pubKey): weight},
		AccountWeights: make(map[AccountName]uint64),
	}
}

// ValidateBasic performs basic validation
func (a Authority) ValidateBasic() error {
	if a.Threshold == 0 {
		return fmt.Errorf("%w: threshold cannot be zero", ErrInvalidAuthority)
	}

	// Calculate total possible weight with overflow protection
	var totalWeight uint64
	for _, weight := range a.KeyWeights {
		// Check for overflow before adding
		if totalWeight > ^uint64(0)-weight {
			return fmt.Errorf("%w: total weight overflow", ErrInvalidAuthority)
		}
		totalWeight += weight
	}
	for _, weight := range a.AccountWeights {
		// Check for overflow before adding
		if totalWeight > ^uint64(0)-weight {
			return fmt.Errorf("%w: total weight overflow", ErrInvalidAuthority)
		}
		totalWeight += weight
	}

	// Threshold must be achievable
	if totalWeight < a.Threshold {
		return fmt.Errorf("%w: threshold %d exceeds total weight %d", ErrInvalidAuthority, a.Threshold, totalWeight)
	}

	// Validate account names in delegations
	for acct := range a.AccountWeights {
		if !acct.IsValid() {
			return fmt.Errorf("%w: invalid delegated account %s", ErrInvalidAuthority, acct)
		}
	}

	return nil
}

// HasKey checks if a public key is in the authority
func (a Authority) HasKey(pubKey []byte) bool {
	_, ok := a.KeyWeights[string(pubKey)]
	return ok
}

// GetKeyWeight returns the weight of a public key
func (a Authority) GetKeyWeight(pubKey []byte) uint64 {
	return a.KeyWeights[string(pubKey)]
}

// HasAccount checks if an account is delegated
func (a Authority) HasAccount(account AccountName) bool {
	_, ok := a.AccountWeights[account]
	return ok
}

// GetAccountWeight returns the delegation weight of an account
func (a Authority) GetAccountWeight(account AccountName) uint64 {
	return a.AccountWeights[account]
}

// TotalKeyWeight returns the total weight from all keys in the authority
func (a Authority) TotalKeyWeight() uint64 {
	var total uint64
	for _, weight := range a.KeyWeights {
		total += weight
	}
	return total
}

// TotalAccountWeight returns the total weight from all delegated accounts
func (a Authority) TotalAccountWeight() uint64 {
	var total uint64
	for _, weight := range a.AccountWeights {
		total += weight
	}
	return total
}

// TotalPossibleWeight returns the maximum achievable weight
func (a Authority) TotalPossibleWeight() uint64 {
	return a.TotalKeyWeight() + a.TotalAccountWeight()
}
