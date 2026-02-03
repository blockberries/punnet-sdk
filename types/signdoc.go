package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/blockberries/cramberry/pkg/cramberry"
	"golang.org/x/text/unicode/norm"
)

// SignDocVersion is the current version of the SignDoc format.
// Changing this version invalidates all existing signatures.
const SignDocVersion = "1"

// SupportedSignDocVersions is the list of SignDoc versions that this implementation
// can validate and process. This is the authoritative source for version support.
//
// SECURITY: Nodes MUST reject transactions with unsupported versions to prevent
// forward-compatibility attacks where different nodes interpret unknown versions
// differently.
var SupportedSignDocVersions = []string{"1"}

// ValidateSignDocVersion checks if the given SignDoc version is supported.
//
// PRECONDITION: version is a string representing the SignDoc version
// POSTCONDITION: Returns nil if version is in SupportedSignDocVersions
// POSTCONDITION: Returns ErrUnsupportedVersion if version is not supported
//
// SECURITY: Rejecting unknown versions is critical for consensus safety.
// If nodes disagree on version support, they may validate the same transaction
// differently, leading to chain forks.
func ValidateSignDocVersion(version string) error {
	for _, v := range SupportedSignDocVersions {
		if version == v {
			return nil
		}
	}
	return fmt.Errorf("%w: %q (supported: %v)", ErrUnsupportedVersion, version, SupportedSignDocVersions)
}

// MaxMessagesPerSignDoc limits the number of messages in a SignDoc.
// SECURITY: Prevents DoS attacks via memory/CPU exhaustion during serialization
// and iteration over large message arrays.
const MaxMessagesPerSignDoc = 256

// MaxMessageDataSize limits the size of each message's data field in bytes.
// SECURITY: Prevents memory exhaustion from arbitrarily large message payloads.
// 64KB per message is generous for most use cases while preventing abuse.
const MaxMessageDataSize = 64 * 1024 // 64KB

// MaxFeeCoins limits the number of coins in a fee.
// SECURITY: Prevents DoS attacks via iteration over large coin arrays.
const MaxFeeCoins = 16

// SignDocCoin represents a coin in the SignDoc with string-serialized amount.
//
// INVARIANT: Amount MUST be a valid decimal string representation of a non-negative integer.
// RATIONALE: String serialization ensures JavaScript BigInt compatibility and prevents
// precision loss for large values that exceed Number.MAX_SAFE_INTEGER (2^53 - 1).
type SignDocCoin struct {
	// Denom is the coin denomination (e.g., "stake", "uatom").
	Denom string `json:"denom"`

	// Amount is the coin amount as a decimal string.
	// MUST be a non-negative integer in string form (e.g., "1000000").
	Amount string `json:"amount"`
}

// SignDocFee represents the transaction fee in the SignDoc.
//
// INVARIANT: GasLimit MUST be a valid decimal string representation of a non-negative integer.
type SignDocFee struct {
	// Amount is the fee amount as a list of coins.
	Amount []SignDocCoin `json:"amount"`

	// GasLimit is the maximum gas allowed for this transaction as a decimal string.
	GasLimit string `json:"gas_limit"`
}

// SignDocRatio represents a ratio with numerator and denominator.
//
// INVARIANT: Both Numerator and Denominator MUST be valid decimal string representations.
// INVARIANT: Denominator MUST NOT be "0" (division by zero is undefined).
//
// This is used for fee slippage tolerance, expressing the maximum acceptable
// conversion rate deviation as a fraction.
type SignDocRatio struct {
	// Numerator is the ratio numerator as a decimal string.
	Numerator string `json:"numerator"`

	// Denominator is the ratio denominator as a decimal string.
	// MUST NOT be "0".
	Denominator string `json:"denominator"`
}

// StringUint64 is a uint64 that serializes to/from a JSON string.
//
// RATIONALE: JavaScript's Number type cannot precisely represent integers
// larger than 2^53 - 1 (Number.MAX_SAFE_INTEGER). By serializing as strings,
// we ensure safe handling in JavaScript clients using BigInt.
//
// INVARIANT: JSON serialization produces a quoted decimal string (e.g., "12345").
// INVARIANT: JSON deserialization accepts only quoted decimal strings.
type StringUint64 uint64

// MarshalJSON implements json.Marshaler for StringUint64.
func (s StringUint64) MarshalJSON() ([]byte, error) {
	return json.Marshal(strconv.FormatUint(uint64(s), 10))
}

// UnmarshalJSON implements json.Unmarshaler for StringUint64.
func (s *StringUint64) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return fmt.Errorf("StringUint64 must be a quoted string: %w", err)
	}
	val, err := strconv.ParseUint(str, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid StringUint64 value %q: %w", str, err)
	}
	*s = StringUint64(val)
	return nil
}

// Uint64 returns the underlying uint64 value.
func (s StringUint64) Uint64() uint64 {
	return uint64(s)
}

// SignDoc represents the canonical document that is signed for transaction authorization.
//
// INVARIANT: Two SignDocs with identical field values MUST produce identical JSON bytes.
// PROOF SKETCH: We use sorted keys and deterministic serialization (no floats, no maps
// with non-string keys) to ensure canonical JSON output. Numeric values are serialized
// as strings to guarantee determinism across platforms with different numeric precision.
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

	// Account is the account authorizing this transaction.
	Account string `json:"account"`

	// AccountSequence is the expected nonce for the signing account.
	// SECURITY: Prevents replay attacks within the same chain.
	AccountSequence StringUint64 `json:"account_sequence"`

	// Messages are the operations to execute.
	Messages []SignDocMessage `json:"messages"`

	// Nonce is the transaction nonce (may differ from account sequence in some protocols).
	Nonce StringUint64 `json:"nonce"`

	// Memo is optional transaction metadata.
	// Note: No omitempty - empty memo is still serialized for deterministic hashing.
	Memo string `json:"memo"`

	// Fee is the transaction fee.
	Fee SignDocFee `json:"fee"`

	// FeeSlippage is the maximum conversion rate slippage tolerance for fee payment.
	// Expressed as a ratio (e.g., {numerator: "1", denominator: "100"} = 1% slippage).
	FeeSlippage SignDocRatio `json:"fee_slippage"`
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
//
// PRECONDITION: chainID is non-empty (cross-chain replay protection).
// PRECONDITION: account is non-empty.
// POSTCONDITION: Returned SignDoc has Version = SignDocVersion.
// POSTCONDITION: Fee and FeeSlippage are zero-valued and must be set separately.
func NewSignDoc(chainID string, accountSequence uint64, account string, nonce uint64, memo string) *SignDoc {
	return &SignDoc{
		Version:         SignDocVersion,
		ChainID:         chainID,
		AccountSequence: StringUint64(accountSequence),
		Account:         account,
		Nonce:           StringUint64(nonce),
		Memo:            memo,
		Messages:        make([]SignDocMessage, 0),
		Fee:             SignDocFee{Amount: make([]SignDocCoin, 0), GasLimit: "0"},
		FeeSlippage:     SignDocRatio{Numerator: "0", Denominator: "1"},
	}
}

// NewSignDocWithFee creates a new SignDoc with the current version and fee configuration.
//
// PRECONDITION: chainID is non-empty (cross-chain replay protection).
// PRECONDITION: account is non-empty.
// PRECONDITION: fee.GasLimit is a valid decimal string.
// PRECONDITION: feeSlippage.Denominator is not "0".
func NewSignDocWithFee(chainID string, accountSequence uint64, account string, nonce uint64, memo string, fee SignDocFee, feeSlippage SignDocRatio) *SignDoc {
	return &SignDoc{
		Version:         SignDocVersion,
		ChainID:         chainID,
		AccountSequence: StringUint64(accountSequence),
		Account:         account,
		Nonce:           StringUint64(nonce),
		Memo:            memo,
		Messages:        make([]SignDocMessage, 0),
		Fee:             fee,
		FeeSlippage:     feeSlippage,
	}
}

// SetFee sets the fee on the SignDoc.
func (sd *SignDoc) SetFee(fee SignDocFee) {
	sd.Fee = fee
}

// SetFeeSlippage sets the fee slippage tolerance on the SignDoc.
func (sd *SignDoc) SetFeeSlippage(slippage SignDocRatio) {
	sd.FeeSlippage = slippage
}

// AddMessage appends a message to the SignDoc.
//
// PRECONDITION: data MUST be canonical JSON if deterministic signing is required.
// Non-canonical JSON (e.g., {"b":1,"a":2}) will be preserved as-is, which may cause
// signature mismatches across implementations that canonicalize message data.
func (sd *SignDoc) AddMessage(msgType string, data json.RawMessage) {
	sd.Messages = append(sd.Messages, SignDocMessage{
		Type: msgType,
		Data: data,
	})
}

// ToJSON serializes the SignDoc to canonical JSON bytes using Cramberry's
// deterministic serialization functions.
//
// INVARIANT: Calling ToJSON() twice on an unmodified SignDoc returns identical bytes.
// INVARIANT: Output is compact (no whitespace between elements).
// INVARIANT: All fields are included, even if empty/zero-valued.
// INVARIANT: Numeric values are serialized as quoted strings (JavaScript BigInt safety).
//
// IMPLEMENTATION: Uses Cramberry library's deterministic JSON helpers:
// - cramberry.EscapeJSONString for safe string escaping with proper Unicode handling
// - Field order follows struct declaration order for reproducibility
// - No reliance on map iteration (which is non-deterministic in Go)
//
// UNICODE NORMALIZATION:
// ValidateBasic() now enforces that all string fields are NFC-normalized.
// This ensures that visually identical strings (e.g., "café" composed vs decomposed)
// produce identical signatures across all implementations. If you receive a
// validation error about non-NFC strings, normalize your input using
// golang.org/x/text/unicode/norm.NFC.String() before creating the SignDoc.
//
// OPTIMIZATION: Uses bytes.Buffer instead of strings.Builder to avoid the double
// allocation that occurs with strings.Builder.String() + []byte(...) conversion.
// bytes.Buffer.Bytes() returns a slice of the internal buffer directly.
func (sd *SignDoc) ToJSON() ([]byte, error) {
	var b bytes.Buffer

	// Pre-allocate reasonable capacity to reduce allocations
	b.Grow(256)

	b.WriteString(`{"version":`)
	b.WriteString(cramberry.EscapeJSONString(sd.Version))

	b.WriteString(`,"chain_id":`)
	b.WriteString(cramberry.EscapeJSONString(sd.ChainID))

	b.WriteString(`,"account":`)
	b.WriteString(cramberry.EscapeJSONString(sd.Account))

	b.WriteString(`,"account_sequence":`)
	b.WriteString(cramberry.EscapeJSONString(strconv.FormatUint(uint64(sd.AccountSequence), 10)))

	b.WriteString(`,"messages":`)
	if err := sd.writeMessagesJSON(&b); err != nil {
		return nil, err
	}

	b.WriteString(`,"nonce":`)
	b.WriteString(cramberry.EscapeJSONString(strconv.FormatUint(uint64(sd.Nonce), 10)))

	// Memo is always included, even if empty (no omitempty behavior)
	b.WriteString(`,"memo":`)
	b.WriteString(cramberry.EscapeJSONString(sd.Memo))

	b.WriteString(`,"fee":`)
	sd.Fee.writeJSON(&b)

	b.WriteString(`,"fee_slippage":`)
	sd.FeeSlippage.writeJSON(&b)

	b.WriteString(`}`)

	// bytes.Buffer.Bytes() returns a slice of the internal buffer. Since this buffer
	// is not reused after this function returns, returning the slice directly is safe.
	// This avoids the double allocation that occurred with strings.Builder:
	// Old: strings.Builder.String() (alloc 1) -> []byte(string) (alloc 2)
	// New: bytes.Buffer.Bytes() returns internal buffer slice (no extra alloc)
	return b.Bytes(), nil
}

// writeMessagesJSON writes the messages array to the buffer.
func (sd *SignDoc) writeMessagesJSON(b *bytes.Buffer) error {
	b.WriteString(`[`)
	for i, msg := range sd.Messages {
		if i > 0 {
			b.WriteString(`,`)
		}
		b.WriteString(`{"type":`)
		b.WriteString(cramberry.EscapeJSONString(msg.Type))
		b.WriteString(`,"data":`)
		// SECURITY: msg.Data is written directly without re-canonicalization.
		// This is safe because ValidateBasic() ensures msg.Data is compact JSON (no whitespace).
		//
		// NOTE: Key ordering is NOT validated - msg.Data with {"b":1,"a":2} vs {"a":2,"b":1}
		// will produce different signatures. This is acceptable because:
		// 1. Message data typically comes from our own serialization code which is consistent
		// 2. Re-canonicalization would add significant overhead for large messages
		// 3. ValidateBasic() catches the most common issue (pretty-printed JSON from files)
		if msg.Data == nil {
			b.WriteString(`null`)
		} else {
			b.Write(msg.Data)
		}
		b.WriteString(`}`)
	}
	b.WriteString(`]`)
	return nil
}

// writeJSON writes the SignDocFee to the buffer in deterministic JSON format.
func (f *SignDocFee) writeJSON(b *bytes.Buffer) {
	b.WriteString(`{"amount":[`)
	for i, coin := range f.Amount {
		if i > 0 {
			b.WriteString(`,`)
		}
		b.WriteString(`{"denom":`)
		b.WriteString(cramberry.EscapeJSONString(coin.Denom))
		b.WriteString(`,"amount":`)
		b.WriteString(cramberry.EscapeJSONString(coin.Amount))
		b.WriteString(`}`)
	}
	b.WriteString(`],"gas_limit":`)
	b.WriteString(cramberry.EscapeJSONString(f.GasLimit))
	b.WriteString(`}`)
}

// writeJSON writes the SignDocRatio to the buffer in deterministic JSON format.
func (r *SignDocRatio) writeJSON(b *bytes.Buffer) {
	b.WriteString(`{"numerator":`)
	b.WriteString(cramberry.EscapeJSONString(r.Numerator))
	b.WriteString(`,"denominator":`)
	b.WriteString(cramberry.EscapeJSONString(r.Denominator))
	b.WriteString(`}`)
}

// EncodeBase64 encodes binary data as standard Base64 (RFC 4648).
// This is a convenience wrapper around Cramberry's EncodeBase64 function.
//
// INVARIANT: Same input always produces identical output.
func EncodeBase64(data []byte) string {
	return cramberry.EncodeBase64(data)
}

// isCompactJSON checks if JSON bytes contain no unnecessary whitespace.
// This is a security check to ensure msg.Data is in canonical form.
//
// SECURITY: Non-compact JSON in message data can lead to signature mismatches
// across implementations that may canonicalize differently.
//
// Returns true if the JSON is compact (no whitespace outside of strings),
// false if there are spaces, tabs, or newlines outside of quoted strings.
func isCompactJSON(data []byte) bool {
	if len(data) == 0 {
		return true
	}

	inString := false
	escape := false

	for _, b := range data {
		if escape {
			escape = false
			continue
		}

		if inString {
			switch b {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}

		// Not in a string
		switch b {
		case '"':
			inString = true
		case ' ', '\t', '\n', '\r':
			// Whitespace outside of a string - not compact
			return false
		}
	}

	return true
}

// isNFCNormalized checks if a string is already in Unicode NFC (Canonical Decomposition,
// followed by Canonical Composition) form.
//
// SECURITY RATIONALE:
// Two strings that appear visually identical can differ in Unicode representation:
// - Composed: "café" using U+00E9 (LATIN SMALL LETTER E WITH ACUTE)
// - Decomposed: "café" using U+0065 + U+0301 (e + COMBINING ACUTE ACCENT)
// These produce different JSON bytes → different signatures for 'same' content.
//
// To prevent signature mismatches and potential attack vectors:
// - We require all strings to be NFC-normalized before signing
// - Non-NFC strings are rejected with a clear error message
// - Applications must normalize user input to NFC before creating SignDocs
//
// NFC is the standard "composed" form used by most text systems and is the
// W3C recommended normalization form for the web.
func isNFCNormalized(s string) bool {
	return norm.NFC.IsNormalString(s)
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
// - Maximum fee coin count: MaxFeeCoins (16)
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

	// SECURITY: Validate Unicode NFC normalization for all string fields.
	// Non-NFC strings can cause signature mismatches across implementations
	// that normalize differently. Failing fast ensures consistent behavior.
	// See: https://unicode.org/reports/tr15/
	if !isNFCNormalized(sd.ChainID) {
		return fmt.Errorf("%w: chain_id is not Unicode NFC-normalized (normalize with golang.org/x/text/unicode/norm.NFC.String before signing)", ErrSignDocMismatch)
	}
	if !isNFCNormalized(sd.Account) {
		return fmt.Errorf("%w: account is not Unicode NFC-normalized (normalize with golang.org/x/text/unicode/norm.NFC.String before signing)", ErrSignDocMismatch)
	}
	if !isNFCNormalized(sd.Memo) {
		return fmt.Errorf("%w: memo is not Unicode NFC-normalized (normalize with golang.org/x/text/unicode/norm.NFC.String before signing)", ErrSignDocMismatch)
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

		// SECURITY: Validate message type is NFC-normalized
		if !isNFCNormalized(msg.Type) {
			return fmt.Errorf("%w: message %d type is not Unicode NFC-normalized", ErrSignDocMismatch, i)
		}

		// SECURITY: Limit message data size to prevent memory exhaustion
		if len(msg.Data) > MaxMessageDataSize {
			return fmt.Errorf("%w: message %d data too large (%d > %d)",
				ErrSignDocMismatch, i, len(msg.Data), MaxMessageDataSize)
		}

		// SECURITY: Validate message data is compact JSON to ensure deterministic signing.
		// Non-compact JSON (with whitespace outside strings) can cause signature mismatches
		// across implementations. This catches the most common canonicalization issues.
		// NOTE: This does NOT validate key ordering - that remains the caller's responsibility.
		if !isCompactJSON(msg.Data) {
			return fmt.Errorf("%w: message %d data is not compact JSON (contains whitespace outside strings)",
				ErrSignDocMismatch, i)
		}
	}

	// Validate fee
	if err := sd.Fee.ValidateBasic(); err != nil {
		return fmt.Errorf("%w: invalid fee: %v", ErrSignDocMismatch, err)
	}

	// Validate fee slippage
	if err := sd.FeeSlippage.ValidateBasic(); err != nil {
		return fmt.Errorf("%w: invalid fee_slippage: %v", ErrSignDocMismatch, err)
	}

	return nil
}

// ValidateBasic performs stateless validation of SignDocFee.
//
// INVARIANT: GasLimit MUST be a valid non-negative decimal string.
// INVARIANT: All coins in Amount MUST be valid.
func (f *SignDocFee) ValidateBasic() error {
	// Validate gas limit is a valid uint64 string
	if f.GasLimit == "" {
		return fmt.Errorf("gas_limit cannot be empty")
	}
	if _, err := strconv.ParseUint(f.GasLimit, 10, 64); err != nil {
		return fmt.Errorf("invalid gas_limit %q: must be a decimal string", f.GasLimit)
	}

	// SECURITY: Limit number of fee coins to prevent DoS
	if len(f.Amount) > MaxFeeCoins {
		return fmt.Errorf("too many fee coins (%d > %d)", len(f.Amount), MaxFeeCoins)
	}

	// Validate each coin
	for i, coin := range f.Amount {
		if err := coin.ValidateBasic(); err != nil {
			return fmt.Errorf("fee coin %d: %w", i, err)
		}
	}

	return nil
}

// ValidateBasic performs stateless validation of SignDocRatio.
//
// INVARIANT: Both Numerator and Denominator MUST be valid non-negative decimal strings.
// INVARIANT: Denominator MUST NOT be "0".
func (r *SignDocRatio) ValidateBasic() error {
	if r.Numerator == "" {
		return fmt.Errorf("numerator cannot be empty")
	}
	if _, err := strconv.ParseUint(r.Numerator, 10, 64); err != nil {
		return fmt.Errorf("invalid numerator %q: must be a decimal string", r.Numerator)
	}

	if r.Denominator == "" {
		return fmt.Errorf("denominator cannot be empty")
	}
	denom, err := strconv.ParseUint(r.Denominator, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid denominator %q: must be a decimal string", r.Denominator)
	}
	if denom == 0 {
		return fmt.Errorf("denominator cannot be zero")
	}

	return nil
}

// ValidateBasic performs stateless validation of SignDocCoin.
//
// INVARIANT: Denom MUST be non-empty and at most 64 characters.
// INVARIANT: Denom MUST be Unicode NFC-normalized.
// INVARIANT: Amount MUST be a valid non-negative decimal string.
func (c *SignDocCoin) ValidateBasic() error {
	if c.Denom == "" {
		return fmt.Errorf("denom cannot be empty")
	}
	if len(c.Denom) > 64 {
		return fmt.Errorf("denom too long (%d > 64)", len(c.Denom))
	}
	// SECURITY: Validate denom is NFC-normalized to prevent signature mismatches
	if !isNFCNormalized(c.Denom) {
		return fmt.Errorf("denom is not Unicode NFC-normalized")
	}

	if c.Amount == "" {
		return fmt.Errorf("amount cannot be empty")
	}
	if _, err := strconv.ParseUint(c.Amount, 10, 64); err != nil {
		return fmt.Errorf("invalid amount %q: must be a decimal string", c.Amount)
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
