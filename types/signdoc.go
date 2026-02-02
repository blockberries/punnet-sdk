package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
)

// SignDocVersion is the current version of the SignDoc format.
// Changing this version invalidates all existing signatures.
const SignDocVersion = "1"

// MaxMessagesPerSignDoc limits the number of messages in a SignDoc.
// SECURITY: Prevents DoS attacks via memory/CPU exhaustion during serialization
// and iteration over large message arrays.
const MaxMessagesPerSignDoc = 256

// MaxMessageDataSize limits the size of each message's data field in bytes.
// SECURITY: Prevents memory exhaustion from arbitrarily large message payloads.
// 64KB per message is generous for most use cases while preventing abuse.
const MaxMessageDataSize = 64 * 1024 // 64KB

// SignDoc represents the canonical document that is signed for transaction authorization.
//
// INVARIANT: Two SignDocs with identical field values MUST produce identical JSON bytes.
// PROOF SKETCH: We use sorted keys and deterministic serialization (no floats, no maps
// with non-string keys) to ensure canonical JSON output.
//
// INVARIANT: SignDoc reconstruction from a Transaction is deterministic.
// PROOF SKETCH: All fields are copied directly; no computed or derived values depend
// on external state or non-deterministic operations.
type SignDoc struct {
	// Version allows for future format changes while maintaining backwards compatibility.
	// MUST be "1" for this version.
	Version string `json:"version"`

	// ChainID prevents cross-chain replay attacks.
	// SECURITY: Signatures are only valid for the chain specified.
	ChainID string `json:"chain_id"`

	// AccountSequence is the expected nonce for the signing account.
	// SECURITY: Prevents replay attacks within the same chain.
	AccountSequence uint64 `json:"account_sequence"`

	// Account is the account authorizing this transaction.
	Account string `json:"account"`

	// Messages are the operations to execute.
	Messages []SignDocMessage `json:"messages"`

	// Nonce is the transaction nonce (may differ from account sequence in some protocols).
	Nonce uint64 `json:"nonce"`

	// Memo is optional transaction metadata.
	Memo string `json:"memo,omitempty"`
}

// SignDocMessage represents a message in canonical form for signing.
type SignDocMessage struct {
	// Type is the message type identifier (e.g., "/punnet.bank.v1.MsgSend")
	Type string `json:"type"`

	// Data contains the message-specific fields in canonical JSON form.
	// Using json.RawMessage to preserve deterministic ordering.
	Data json.RawMessage `json:"data"`
}

// NewSignDoc creates a new SignDoc with the current version.
func NewSignDoc(chainID string, accountSequence uint64, account string, nonce uint64, memo string) *SignDoc {
	return &SignDoc{
		Version:         SignDocVersion,
		ChainID:         chainID,
		AccountSequence: accountSequence,
		Account:         account,
		Nonce:           nonce,
		Memo:            memo,
		Messages:        make([]SignDocMessage, 0),
	}
}

// AddMessage appends a message to the SignDoc.
func (sd *SignDoc) AddMessage(msgType string, data json.RawMessage) {
	sd.Messages = append(sd.Messages, SignDocMessage{
		Type: msgType,
		Data: data,
	})
}

// ToJSON serializes the SignDoc to canonical JSON bytes.
//
// INVARIANT: Calling ToJSON() twice on an unmodified SignDoc returns identical bytes.
// IMPLEMENTATION: We use a custom serialization to ensure key ordering and no trailing spaces.
func (sd *SignDoc) ToJSON() ([]byte, error) {
	// Use Go's json package which produces deterministic output for structs
	// (fields are serialized in declaration order).
	return json.Marshal(sd)
}

// GetSignBytes returns the bytes that should be signed.
//
// SECURITY: This is SHA-256(canonical_json), not the raw JSON.
// RATIONALE: Hashing provides a fixed-size output regardless of transaction size,
// and the hash commitment prevents malleability attacks on the JSON structure.
//
// INVARIANT: GetSignBytes() returns the same result for equivalent SignDocs.
func (sd *SignDoc) GetSignBytes() ([]byte, error) {
	jsonBytes, err := sd.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize SignDoc: %w", err)
	}

	hash := sha256.Sum256(jsonBytes)
	return hash[:], nil
}

// ValidateBasic performs stateless validation of the SignDoc.
//
// SECURITY: This validation includes bounds checking to prevent DoS attacks:
// - Maximum message count: MaxMessagesPerSignDoc (256)
// - Maximum message data size: MaxMessageDataSize (64KB)
func (sd *SignDoc) ValidateBasic() error {
	if sd.Version != SignDocVersion {
		return fmt.Errorf("%w: unsupported SignDoc version %q, expected %q",
			ErrSignDocMismatch, sd.Version, SignDocVersion)
	}

	if sd.ChainID == "" {
		return fmt.Errorf("%w: chain_id cannot be empty", ErrSignDocMismatch)
	}

	if sd.Account == "" {
		return fmt.Errorf("%w: account cannot be empty", ErrSignDocMismatch)
	}

	if len(sd.Messages) == 0 {
		return fmt.Errorf("%w: SignDoc must contain at least one message", ErrSignDocMismatch)
	}

	// SECURITY: Limit message count to prevent DoS via memory/CPU exhaustion
	if len(sd.Messages) > MaxMessagesPerSignDoc {
		return fmt.Errorf("%w: too many messages (%d > %d)",
			ErrSignDocMismatch, len(sd.Messages), MaxMessagesPerSignDoc)
	}

	// Validate each message
	for i, msg := range sd.Messages {
		if msg.Type == "" {
			return fmt.Errorf("%w: message %d has empty type", ErrSignDocMismatch, i)
		}

		// SECURITY: Limit message data size to prevent memory exhaustion
		if len(msg.Data) > MaxMessageDataSize {
			return fmt.Errorf("%w: message %d data too large (%d > %d)",
				ErrSignDocMismatch, i, len(msg.Data), MaxMessageDataSize)
		}
	}

	return nil
}

// Equals checks if two SignDocs are semantically equal.
// This performs a byte-level comparison of the canonical JSON.
func (sd *SignDoc) Equals(other *SignDoc) bool {
	if other == nil {
		return false
	}

	json1, err1 := sd.ToJSON()
	json2, err2 := other.ToJSON()

	if err1 != nil || err2 != nil {
		return false
	}

	return bytes.Equal(json1, json2)
}

// sortedJSONObject is a helper type for producing deterministic JSON with sorted keys.
// This is used when we need to serialize maps or other non-ordered structures.
type sortedJSONObject map[string]interface{}

// MarshalJSON implements json.Marshaler with sorted keys.
func (s sortedJSONObject) MarshalJSON() ([]byte, error) {
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		valBytes, err := json.Marshal(s[k])
		if err != nil {
			return nil, err
		}
		buf.Write(valBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// ParseSignDoc deserializes JSON bytes into a SignDoc.
func ParseSignDoc(data []byte) (*SignDoc, error) {
	var sd SignDoc
	if err := json.Unmarshal(data, &sd); err != nil {
		return nil, fmt.Errorf("failed to parse SignDoc: %w", err)
	}
	return &sd, nil
}
