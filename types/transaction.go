package types

import (
	"crypto/sha256"
	"encoding/json"
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
// Creates defensive copies of slices to prevent external mutation
func NewTransaction(account AccountName, nonce uint64, messages []Message, auth *Authorization) *Transaction {
	// Create defensive copy of messages slice
	msgsCopy := make([]Message, len(messages))
	copy(msgsCopy, messages)

	return &Transaction{
		Account:       account,
		Messages:      msgsCopy,
		Authorization: auth,
		Nonce:         nonce,
	}
}

// ValidateBasic performs basic validation
func (tx *Transaction) ValidateBasic() error {
	if tx == nil {
		return fmt.Errorf("%w: transaction is nil", ErrInvalidTransaction)
	}

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

// GetSignBytes returns the bytes to sign for this transaction.
// Includes full message content (not just type strings) for security.
func (tx *Transaction) GetSignBytes() []byte {
	h := sha256.New()
	h.Write([]byte(tx.Account))

	// Add nonce
	nonceBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		nonceBytes[i] = byte(tx.Nonce >> (8 * i))
	}
	h.Write(nonceBytes)

	// Add full message content for each message
	for _, msg := range tx.Messages {
		h.Write([]byte(msg.Type()))
		// Include JSON-serialized message content to prevent signature malleability
		msgBytes, err := json.Marshal(msg)
		if err == nil {
			h.Write(msgBytes)
		}
	}

	// Add memo
	h.Write([]byte(tx.Memo))

	return h.Sum(nil)
}

// transactionJSON is the private JSON wire format for Transaction.
type transactionJSON struct {
	Account       AccountName      `json:"account"`
	Messages      []MessageEnvelope `json:"messages"`
	Authorization *Authorization   `json:"authorization"`
	Nonce         uint64           `json:"nonce"`
	Memo          string           `json:"memo,omitempty"`
}

// MarshalJSON implements json.Marshaler with type-discriminated messages.
func (tx Transaction) MarshalJSON() ([]byte, error) {
	envelopes := make([]MessageEnvelope, len(tx.Messages))
	for i, msg := range tx.Messages {
		value, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("marshal message %d: %w", i, err)
		}
		envelopes[i] = MessageEnvelope{
			Type:  msg.Type(),
			Value: value,
		}
	}

	return json.Marshal(transactionJSON{
		Account:       tx.Account,
		Messages:      envelopes,
		Authorization: tx.Authorization,
		Nonce:         tx.Nonce,
		Memo:          tx.Memo,
	})
}

// UnmarshalJSON implements json.Unmarshaler with type-discriminated messages.
func (tx *Transaction) UnmarshalJSON(data []byte) error {
	var raw transactionJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal transaction: %w", err)
	}

	msgs := make([]Message, len(raw.Messages))
	for i, env := range raw.Messages {
		msg, err := UnmarshalMessageJSON(mustMarshalEnvelope(env))
		if err != nil {
			return fmt.Errorf("unmarshal message %d: %w", i, err)
		}
		msgs[i] = msg
	}

	tx.Account = raw.Account
	tx.Messages = msgs
	tx.Authorization = raw.Authorization
	tx.Nonce = raw.Nonce
	tx.Memo = raw.Memo
	return nil
}

// mustMarshalEnvelope marshals a MessageEnvelope back to JSON bytes.
func mustMarshalEnvelope(env MessageEnvelope) []byte {
	data, err := json.Marshal(env)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal message envelope: %v", err))
	}
	return data
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
