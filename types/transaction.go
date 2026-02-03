package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
)

// Fee represents the transaction fee with gas limit and coin amounts.
//
// INVARIANT: GasLimit is a non-negative value.
// INVARIANT: Amount contains valid coins (non-empty denoms, valid amounts).
type Fee struct {
	// Amount is the fee amount as a collection of coins.
	Amount Coins `json:"amount"`

	// GasLimit is the maximum gas allowed for this transaction.
	GasLimit uint64 `json:"gas_limit"`
}

// Ratio represents a ratio with numerator and denominator.
//
// INVARIANT: Denominator MUST NOT be zero (division by zero is undefined).
//
// This is used for fee slippage tolerance, expressing the maximum acceptable
// conversion rate deviation as a fraction.
type Ratio struct {
	// Numerator is the ratio numerator.
	Numerator uint64 `json:"numerator"`

	// Denominator is the ratio denominator.
	// MUST NOT be zero.
	Denominator uint64 `json:"denominator"`
}

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

	// Fee is the transaction fee
	Fee Fee `json:"fee"`

	// FeeSlippage is the maximum conversion rate slippage tolerance for fee payment.
	// Expressed as a ratio (e.g., {Numerator: 1, Denominator: 100} = 1% slippage).
	FeeSlippage Ratio `json:"fee_slippage"`
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
//
// Performance: Optimized to perform single SignDoc construction and reuse serialized JSON
// for both roundtrip validation and hash computation. See issue #36.
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

	// 1. Reconstruct SignDoc from transaction fields (single construction)
	signDoc := tx.ToSignDoc(chainID, account.Nonce)

	// 2. Serialize to JSON (json1)
	json1, err := signDoc.ToJSON()
	if err != nil {
		return fmt.Errorf("%w: initial serialization failed: %v", ErrSignDocMismatch, err)
	}

	// 3. Validate roundtrip: parse and re-serialize to verify determinism
	// SECURITY: This catches non-deterministic serialization bugs and tampering
	parsed, err := ParseSignDoc(json1)
	if err != nil {
		return fmt.Errorf("%w: parsing failed: %v", ErrSignDocMismatch, err)
	}

	json2, err := parsed.ToJSON()
	if err != nil {
		return fmt.Errorf("%w: re-serialization failed: %v", ErrSignDocMismatch, err)
	}

	if !bytes.Equal(json1, json2) {
		return fmt.Errorf("%w: roundtrip produced different bytes (len %d vs %d)",
			ErrSignDocMismatch, len(json1), len(json2))
	}

	// 4. Compute hash from json1 (reuse, no additional ToJSON call)
	// Complexity: O(n) where n = len(json1)
	hash := sha256.Sum256(json1)
	signBytes := hash[:]

	// 5. Verify all signatures against the hash
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
// PROOF SKETCH: All field conversions are pure functions of their inputs with no external
// state dependency. Numeric conversions use strconv.FormatUint which is deterministic.
// Message ordering is preserved (no sorting). Coin ordering in Fee.Amount is preserved.
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
	signDoc := &SignDoc{
		Version:         SignDocVersion,
		ChainID:         chainID,
		Account:         string(tx.Account),
		AccountSequence: StringUint64(accountSequence),
		Messages:        convertMessages(tx.Messages),
		Nonce:           StringUint64(tx.Nonce),
		Memo:            tx.Memo,
		Fee:             convertFee(tx.Fee),
		FeeSlippage:     convertRatio(tx.FeeSlippage),
	}

	return signDoc
}

// convertMessages converts a slice of Message to SignDocMessage format.
//
// INVARIANT: Message ordering is preserved.
// INVARIANT: Each message's Type() and GetSigners() are captured in the SignDocMessage.
//
// TODO(follow-up): This only serializes signers. Full message content should be
// included for proper signature binding. The json.Marshal error is silently
// ignored here which is not ideal.
func convertMessages(msgs []Message) []SignDocMessage {
	if msgs == nil {
		return make([]SignDocMessage, 0)
	}

	result := make([]SignDocMessage, len(msgs))
	for i, msg := range msgs {
		// TODO(follow-up): This only serializes signers. Full message content should be
		// included for proper signature binding.
		msgData, _ := json.Marshal(map[string]interface{}{
			"signers": msg.GetSigners(),
		})
		result[i] = SignDocMessage{
			Type: msg.Type(),
			Data: msgData,
		}
	}
	return result
}

// convertFee converts a Fee to SignDocFee format.
//
// INVARIANT: Coin ordering in Amount is preserved.
// INVARIANT: GasLimit is converted to decimal string representation.
// INVARIANT: Each coin's Amount is converted to decimal string representation.
func convertFee(fee Fee) SignDocFee {
	coins := make([]SignDocCoin, len(fee.Amount))
	for i, coin := range fee.Amount {
		coins[i] = SignDocCoin{
			Denom:  coin.Denom,
			Amount: strconv.FormatUint(coin.Amount, 10),
		}
	}

	return SignDocFee{
		Amount:   coins,
		GasLimit: strconv.FormatUint(fee.GasLimit, 10),
	}
}

// convertRatio converts a Ratio to SignDocRatio format.
//
// INVARIANT: Numerator and Denominator are converted to decimal string representations.
// ASSUMPTION: Caller ensures Denominator is not zero (validated elsewhere).
func convertRatio(ratio Ratio) SignDocRatio {
	return SignDocRatio{
		Numerator:   strconv.FormatUint(ratio.Numerator, 10),
		Denominator: strconv.FormatUint(ratio.Denominator, 10),
	}
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
