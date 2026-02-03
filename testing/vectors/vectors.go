// Package vectors provides cross-implementation test vectors for the Punnet SDK signing system.
//
// These test vectors enable verification of signing implementations across different
// languages and implementations. Each vector contains deterministic inputs and expected
// outputs that any conforming implementation must produce.
//
// SECURITY: Test vectors use well-known test keys. NEVER use these keys in production.
package vectors

import (
	"encoding/hex"
	"encoding/json"
	"time"
)

// TestVectorFile is the root structure of the test vector JSON file.
type TestVectorFile struct {
	// Version of the test vector format.
	Version string `json:"version"`

	// Generated timestamp in RFC3339 format.
	Generated time.Time `json:"generated"`

	// Description of this test vector file.
	Description string `json:"description"`

	// Vectors is the list of test vectors.
	Vectors []TestVector `json:"vectors"`
}

// TestVector represents a single test case for cross-implementation testing.
type TestVector struct {
	// Name is a unique identifier for this test vector.
	Name string `json:"name"`

	// Description explains what this test vector tests.
	Description string `json:"description"`

	// Category groups related test vectors (serialization, algorithm, edge_case).
	Category string `json:"category"`

	// Input contains the SignDoc input fields.
	Input TestVectorInput `json:"input"`

	// Expected contains the expected outputs.
	Expected TestVectorExpected `json:"expected"`
}

// TestVectorInput contains the input data for creating a SignDoc.
type TestVectorInput struct {
	// ChainID for replay protection.
	ChainID string `json:"chain_id"`

	// Account is the signing account name.
	Account string `json:"account"`

	// AccountSequence is the nonce for the account.
	AccountSequence string `json:"account_sequence"`

	// Nonce is the transaction nonce.
	Nonce string `json:"nonce"`

	// Memo is optional transaction metadata.
	Memo string `json:"memo,omitempty"`

	// Messages is the list of messages in the transaction.
	Messages []TestVectorMessage `json:"messages"`

	// Fee is the transaction fee.
	Fee TestVectorFee `json:"fee"`

	// FeeSlippage is the fee slippage tolerance.
	FeeSlippage TestVectorRatio `json:"fee_slippage"`
}

// TestVectorMessage represents a message in a test vector.
type TestVectorMessage struct {
	// Type is the message type identifier.
	Type string `json:"type"`

	// Data is the message data as a JSON object.
	Data json.RawMessage `json:"data"`
}

// TestVectorFee represents fee information in a test vector.
type TestVectorFee struct {
	// Amount is the list of fee coins.
	Amount []TestVectorCoin `json:"amount"`

	// GasLimit as a decimal string.
	GasLimit string `json:"gas_limit"`
}

// TestVectorCoin represents a coin in a test vector.
type TestVectorCoin struct {
	// Denom is the coin denomination.
	Denom string `json:"denom"`

	// Amount as a decimal string.
	Amount string `json:"amount"`
}

// TestVectorRatio represents a ratio in a test vector.
type TestVectorRatio struct {
	// Numerator as a decimal string.
	Numerator string `json:"numerator"`

	// Denominator as a decimal string.
	Denominator string `json:"denominator"`
}

// TestVectorExpected contains the expected outputs for a test vector.
type TestVectorExpected struct {
	// SignDocJSON is the expected canonical JSON serialization of the SignDoc.
	SignDocJSON string `json:"sign_doc_json"`

	// SignBytesHex is the expected SHA-256 hash of the SignDoc JSON, in hex.
	SignBytesHex string `json:"sign_bytes_hex"`

	// Signatures contains expected signatures for different algorithms.
	Signatures map[string]TestVectorSignature `json:"signatures"`
}

// TestVectorSignature contains key material and expected signature for an algorithm.
type TestVectorSignature struct {
	// PrivateKeyHex is the private key in hex format.
	// SECURITY: These are TEST KEYS ONLY. Never use in production.
	PrivateKeyHex string `json:"private_key_hex"`

	// PublicKeyHex is the public key in hex format.
	PublicKeyHex string `json:"public_key_hex"`

	// SignatureHex is the expected signature in hex format.
	SignatureHex string `json:"signature_hex"`
}

// HexBytes is a helper type for hex-encoded bytes in JSON.
type HexBytes []byte

// MarshalJSON encodes bytes as hex string.
func (h HexBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(h))
}

// UnmarshalJSON decodes hex string to bytes.
func (h *HexBytes) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	*h = b
	return nil
}
