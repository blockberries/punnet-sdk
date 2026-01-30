package types

import (
	"crypto/sha256"
	"fmt"
)

// Transaction represents a signed transaction
type Transaction struct {
	// Account is the account executing this transaction
	Account AccountName `json:"account"`

	// Messages are the messages to execute
	Messages []Message `json:"messages"`

	// Authorization proves the account authorized this transaction
	Authorization *Authorization `json:"authorization"`

	// Nonce prevents replay attacks
	Nonce uint64 `json:"nonce"`

	// Memo is an optional memo
	Memo string `json:"memo,omitempty"`
}

// NewTransaction creates a new transaction
func NewTransaction(account AccountName, nonce uint64, messages []Message, auth *Authorization) *Transaction {
	return &Transaction{
		Account:       account,
		Messages:      messages,
		Authorization: auth,
		Nonce:         nonce,
	}
}

// ValidateBasic performs basic validation
func (tx *Transaction) ValidateBasic() error {
	if !tx.Account.IsValid() {
		return fmt.Errorf("%w: %s", ErrInvalidAccount, tx.Account)
	}

	if len(tx.Messages) == 0 {
		return fmt.Errorf("%w: transaction must have at least one message", ErrInvalidTransaction)
	}

	if tx.Authorization == nil {
		return fmt.Errorf("%w: authorization cannot be nil", ErrInvalidTransaction)
	}

	// Validate authorization
	if err := tx.Authorization.ValidateBasic(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTransaction, err)
	}

	// Validate all messages
	for i, msg := range tx.Messages {
		if err := msg.ValidateBasic(); err != nil {
			return fmt.Errorf("%w: message %d: %v", ErrInvalidTransaction, i, err)
		}

		// Verify that the transaction account is authorized to send all messages
		signers := msg.GetSigners()
		validSigner := false
		for _, signer := range signers {
			if signer == tx.Account {
				validSigner = true
				break
			}
		}
		if !validSigner {
			return fmt.Errorf("%w: transaction account %s not in message signers", ErrInvalidTransaction, tx.Account)
		}
	}

	// Memo size limit
	if len(tx.Memo) > 512 {
		return fmt.Errorf("%w: memo exceeds 512 bytes", ErrInvalidTransaction)
	}

	return nil
}

// Hash computes the transaction hash
func (tx *Transaction) Hash() []byte {
	// TODO: Use proper serialization (Cramberry) for production
	// For now, use a simple hash of concatenated fields
	h := sha256.New()
	h.Write([]byte(tx.Account))
	for _, msg := range tx.Messages {
		h.Write([]byte(msg.Type()))
	}
	return h.Sum(nil)
}

// GetSignBytes returns the bytes to sign for this transaction
// This is used for signature verification
func (tx *Transaction) GetSignBytes() []byte {
	// TODO: Use proper canonical serialization (Cramberry) for production
	// For now, use a simple concatenation
	h := sha256.New()
	h.Write([]byte(tx.Account))

	// Add nonce
	nonceBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		nonceBytes[i] = byte(tx.Nonce >> (8 * i))
	}
	h.Write(nonceBytes)

	// Add messages
	for _, msg := range tx.Messages {
		h.Write([]byte(msg.Type()))
	}

	// Add memo
	h.Write([]byte(tx.Memo))

	return h.Sum(nil)
}

// VerifyAuthorization verifies the transaction authorization
func (tx *Transaction) VerifyAuthorization(account *Account, getter AccountGetter) error {
	// Check nonce
	if tx.Nonce != account.Nonce {
		return fmt.Errorf("%w: expected nonce %d, got %d", ErrInvalidTransaction, account.Nonce, tx.Nonce)
	}

	// Get sign bytes
	signBytes := tx.GetSignBytes()

	// Verify authorization
	return tx.Authorization.VerifyAuthorization(account, signBytes, getter)
}
