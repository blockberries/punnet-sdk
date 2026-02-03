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
// INVARIANT: Amount contains no duplicate denominations.
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

// ValidateBasic performs stateless validation of Fee.
//
// INVARIANT: All coins in Amount MUST be valid (non-empty denom, denom <= 64 chars).
// INVARIANT: Number of fee coins MUST NOT exceed MaxFeeCoins.
// INVARIANT: Fee coins MUST contain no duplicate denominations.
//
// PROOF SKETCH (no duplicates): Duplicate denominations would cause ambiguity in
// fee calculations downstream. If the same denom appears twice (e.g., {uatom: 1000},
// {uatom: 2000}), the total fee for that denom is undefined. By rejecting duplicates
// at validation time, we ensure Fee.Amount defines a unique mapping from denom to amount.
//
// SECURITY: This validation prevents malformed fees from entering the gossip layer.
// Downstream components (e.g., ToSignDoc, fee deduction) assume valid fee structure.
func (f *Fee) ValidateBasic() error {
	// SECURITY: Limit number of fee coins to prevent DoS via iteration
	if len(f.Amount) > MaxFeeCoins {
		return fmt.Errorf("too many fee coins (%d > %d)", len(f.Amount), MaxFeeCoins)
	}

	// Track seen denominations for duplicate detection
	// COMPLEXITY: O(n) time, O(n) space where n = len(Amount)
	seenDenoms := make(map[string]struct{}, len(f.Amount))

	// Validate each coin and check for duplicates
	for i, coin := range f.Amount {
		if !coin.IsValid() {
			return fmt.Errorf("fee coin %d: invalid (empty or oversized denom)", i)
		}

		// Check for duplicate denomination
		if _, exists := seenDenoms[coin.Denom]; exists {
			return fmt.Errorf("fee coin %d: duplicate denomination %q", i, coin.Denom)
		}
		seenDenoms[coin.Denom] = struct{}{}
	}

	return nil
}

// ValidateBasic performs stateless validation of Ratio.
//
// PRECONDITION: None (called on any Ratio value).
// POSTCONDITION: If nil returned, Denominator is guaranteed non-zero.
//
// INVARIANT: Denominator MUST NOT be zero.
// PROOF SKETCH: The only way to pass validation is if Denominator != 0.
// This is explicitly checked before returning nil.
//
// SECURITY: Zero denominator would cause division by zero in downstream calculations.
// This validation ensures the invariant is enforced at the earliest possible point,
// preventing malformed transactions from entering the gossip layer.
func (r *Ratio) ValidateBasic() error {
	if r.Denominator == 0 {
		return fmt.Errorf("denominator cannot be zero")
	}
	return nil
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

	// Validate Fee
	// SECURITY: Validate fee before transaction enters gossip layer to prevent
	// malformed transactions from propagating through the network.
	if err := tx.Fee.ValidateBasic(); err != nil {
		return fmt.Errorf("%w: invalid fee: %v", ErrInvalidTransaction, err)
	}

	// Validate FeeSlippage
	// SECURITY: Zero denominator in Ratio would cause undefined behavior downstream.
	// This validation ensures malformed FeeSlippage is caught at the earliest point.
	if err := tx.FeeSlippage.ValidateBasic(); err != nil {
		return fmt.Errorf("%w: invalid fee_slippage: %v", ErrInvalidTransaction, err)
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
	signDoc, err := tx.ToSignDoc(chainID, account.Nonce)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTransaction, err)
	}

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
// MESSAGE SERIALIZATION:
// - If a message implements SignDocSerializable, its SignDocData() is used (full content).
// - Otherwise, only signers are included (backwards-compatible fallback).
// See SignDocSerializable interface for rationale and security implications.
//
// Returns an error if message serialization fails.
func (tx *Transaction) ToSignDoc(chainID string, accountSequence uint64) (*SignDoc, error) {
	messages, err := convertMessages(tx.Messages)
	if err != nil {
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	signDoc := &SignDoc{
		Version:         SignDocVersion,
		ChainID:         chainID,
		Account:         string(tx.Account),
		AccountSequence: StringUint64(accountSequence),
		Messages:        messages,
		Nonce:           StringUint64(tx.Nonce),
		Memo:            tx.Memo,
		Fee:             convertFee(tx.Fee),
		FeeSlippage:     convertRatio(tx.FeeSlippage),
	}

	return signDoc, nil
}

// convertMessages converts a slice of Message to SignDocMessage format.
//
// INVARIANT: Message ordering is preserved.
// INVARIANT: Each message's Type() is captured in the SignDocMessage.
// INVARIANT: If a message implements SignDocSerializable, its full canonical data is used.
// INVARIANT: If a message does not implement SignDocSerializable, only signers are included
//
//	(backwards-compatible fallback behavior).
//
// Returns an error if any message's SignDocData() fails or if JSON marshaling fails.
func convertMessages(msgs []Message) ([]SignDocMessage, error) {
	if msgs == nil {
		return make([]SignDocMessage, 0), nil
	}

	result := make([]SignDocMessage, len(msgs))
	for i, msg := range msgs {
		var msgData json.RawMessage
		var err error

		// Check if message implements SignDocSerializable for full content
		if serializable, ok := msg.(SignDocSerializable); ok {
			msgData, err = serializable.SignDocData()
			if err != nil {
				return nil, fmt.Errorf("message %d SignDocData failed: %w", i, err)
			}
		} else {
			// Fallback: only include signers (backwards-compatible)
			//
			// DEPRECATED: This fallback is a security weakness and will be removed
			// in a future version. Messages should implement SignDocSerializable
			// to ensure signatures bind to full message content.
			//
			// DEPRECATION TIMELINE:
			// - v0.x: Warning logged when fallback is used (current behavior)
			// - v1.0: Consider making SignDocSerializable required
			// - Future: Remove signers-only fallback entirely
			//
			// SECURITY NOTE: This fallback loses message content. Signatures only
			// bind to signers, not to amounts, recipients, or other message fields.
			// This could allow signature reuse attacks where different messages
			// with the same signers share signatures.
			SignersOnlyFallbackDeprecation(msg)

			msgData, err = json.Marshal(map[string]interface{}{
				"signers": msg.GetSigners(),
			})
			if err != nil {
				return nil, fmt.Errorf("message %d signers serialization failed: %w", i, err)
			}
		}

		result[i] = SignDocMessage{
			Type: msg.Type(),
			Data: msgData,
		}
	}
	return result, nil
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
	signDoc, err := tx.ToSignDoc(chainID, accountSequence)
	if err != nil {
		return fmt.Errorf("%w: SignDoc creation failed: %v", ErrSignDocMismatch, err)
	}

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
