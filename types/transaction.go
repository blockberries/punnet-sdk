package types

import (
	"bytes"
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
//
// Deprecated: This method uses a non-standard serialization format that does not
// include chainID for replay protection. Use ToSignDoc().GetSignBytes() instead
// for SignDoc-based verification with proper cross-chain replay attack prevention.
//
// TODO(follow-up): Remove this method or rename to LegacyGetSignBytes() once all
// callers are migrated to SignDoc-based verification. See PR #25 review from
// Conductor for details.
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

// VerifyAuthorization verifies the transaction authorization using SignDoc-based verification.
//
// PRECONDITION: account is not nil
// PRECONDITION: chainID is non-empty
// PRECONDITION: tx.Authorization is not nil
//
// POSTCONDITION: If nil error returned, all signatures are valid against the SignDoc hash.
// POSTCONDITION: If nil error returned, the authorization meets the account's threshold.
//
// SECURITY: This method reconstructs the SignDoc from transaction fields (no stored bytes),
// validates deterministic roundtrip, then verifies all signatures against the hash.
//
// INVARIANT: Verification is deterministic - same inputs always produce same result.
func (tx *Transaction) VerifyAuthorization(chainID string, account *Account, getter AccountGetter) error {
	if account == nil {
		return fmt.Errorf("%w: account is nil", ErrInvalidTransaction)
	}

	if chainID == "" {
		return fmt.Errorf("%w: chainID cannot be empty", ErrInvalidTransaction)
	}

	// Check nonce
	// SECURITY: Nonce verification prevents replay attacks
	if tx.Nonce != account.Nonce {
		return fmt.Errorf("%w: expected nonce %d, got %d", ErrInvalidTransaction, account.Nonce, tx.Nonce)
	}

	// 1. Reconstruct SignDoc from transaction fields
	signDoc := tx.ToSignDoc(chainID, account.Nonce)

	// 2. Validate roundtrip to ensure determinism
	// SECURITY: This catches non-deterministic serialization bugs and tampering
	if err := tx.ValidateSignDocRoundtrip(chainID, account.Nonce); err != nil {
		return err
	}

	// 3. Get canonical JSON bytes and hash
	signBytes, err := signDoc.GetSignBytes()
	if err != nil {
		return fmt.Errorf("%w: failed to get sign bytes: %v", ErrInvalidTransaction, err)
	}

	// 4. Verify all signatures against the hash
	// First verify the signatures are valid, then check authorization weight
	return tx.Authorization.VerifyAuthorization(account, signBytes, getter)
}

// ToSignDoc converts the transaction to a SignDoc for signing.
//
// PRECONDITION: tx has at least one message
// POSTCONDITION: returned SignDoc contains all signable transaction data
// POSTCONDITION: Authorization field is NOT included (it contains the signatures being produced)
//
// INVARIANT: Two calls to ToSignDoc with same parameters return equal SignDocs.
//
// TODO(follow-up): The current message serialization only includes signers, not full message data.
// This is architecturally problematic because:
// 1. Information loss - actual message content isn't in the SignDoc
// 2. Future compatibility - when messages have fields beyond signers, this breaks
//
// Recommended fix: Define a SignDocSerializable interface:
//
//	type SignDocSerializable interface {
//	    SignDocData() (json.RawMessage, error)
//	}
//
// And implement it on Message types or extract full message data.
// See PR #25 review from Conductor for details.
func (tx *Transaction) ToSignDoc(chainID string, accountSequence uint64) *SignDoc {
	signDoc := NewSignDoc(chainID, accountSequence, string(tx.Account), tx.Nonce, tx.Memo)

	// Convert messages to SignDoc format
	for _, msg := range tx.Messages {
		// TODO(follow-up): This only serializes signers. Full message content should be
		// included for proper signature binding. The json.Marshal error is silently
		// ignored here which is not ideal.
		msgData, _ := json.Marshal(map[string]interface{}{
			"signers": msg.GetSigners(),
		})
		signDoc.AddMessage(msg.Type(), msgData)
	}

	return signDoc
}

// ValidateSignDocRoundtrip validates that SignDoc serialization is deterministic.
//
// This is a CRITICAL security check that ensures:
// 1. The SignDoc can be serialized to JSON
// 2. The JSON can be parsed back to a SignDoc
// 3. Re-serializing produces identical bytes
//
// SECURITY: Non-deterministic serialization could allow signature malleability attacks
// where an attacker modifies the transaction representation without invalidating signatures.
//
// INVARIANT: If this returns nil, json1 == json2 byte-for-byte.
func (tx *Transaction) ValidateSignDocRoundtrip(chainID string, accountSequence uint64) error {
	// Create SignDoc from transaction
	signDoc := tx.ToSignDoc(chainID, accountSequence)

	// Serialize to JSON (json1)
	json1, err := signDoc.ToJSON()
	if err != nil {
		return fmt.Errorf("%w: initial serialization failed: %v", ErrSignDocMismatch, err)
	}

	// Parse JSON back to SignDoc struct
	parsed, err := ParseSignDoc(json1)
	if err != nil {
		return fmt.Errorf("%w: parsing failed: %v", ErrSignDocMismatch, err)
	}

	// Re-serialize to JSON (json2)
	json2, err := parsed.ToJSON()
	if err != nil {
		return fmt.Errorf("%w: re-serialization failed: %v", ErrSignDocMismatch, err)
	}

	// Compare json1 and json2 byte-for-byte
	if !bytes.Equal(json1, json2) {
		return fmt.Errorf("%w: roundtrip produced different bytes (len %d vs %d)",
			ErrSignDocMismatch, len(json1), len(json2))
	}

	return nil
}
