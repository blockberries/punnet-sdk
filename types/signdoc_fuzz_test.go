package types

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

// =============================================================================
// FUZZ TESTS FOR SIGNDOC JSON PARSING
// =============================================================================
// These fuzz tests target security-critical parsing operations in SignDoc.
// Goal: Discover panics, memory exhaustion, and parser inconsistencies.
//
// Run with: go test -fuzz=FuzzParseSignDoc -fuzztime=60s ./types/...
//
// SECURITY RATIONALE:
// SignDoc parsing is security-critical because:
// 1. It's the entry point for externally-provided transaction data
// 2. Malformed input could cause DoS via panics or memory exhaustion
// 3. Parser inconsistencies could lead to signature verification bypasses

// =============================================================================
// FUZZ TEST: ParseSignDoc - Malformed JSON
// =============================================================================

func FuzzParseSignDoc(f *testing.F) {
	// Seed corpus: known-good inputs
	f.Add([]byte(`{"version":"1","chain_id":"test","account":"alice","account_sequence":"1","messages":[{"type":"/msg","data":{}}],"nonce":"1","fee":{"amount":[],"gas_limit":"0"},"fee_slippage":{"numerator":"0","denominator":"1"}}`))
	f.Add([]byte(`{"version":"1","chain_id":"punnet-mainnet-1","account":"bob","account_sequence":"42","messages":[{"type":"/punnet.bank.v1.MsgSend","data":{"from":"bob","to":"alice","amount":"100"}}],"nonce":"10","memo":"hello","fee":{"amount":[{"denom":"uatom","amount":"5000"}],"gas_limit":"200000"},"fee_slippage":{"numerator":"1","denominator":"100"}}`))

	// Seed corpus: adversarial inputs
	f.Add([]byte(`{}`))                                                                                                          // Empty object
	f.Add([]byte(`[]`))                                                                                                          // Array instead of object
	f.Add([]byte(`null`))                                                                                                        // Null value
	f.Add([]byte(`"string"`))                                                                                                    // String instead of object
	f.Add([]byte(`123`))                                                                                                         // Number instead of object
	f.Add([]byte(`{"version":"1"}`))                                                                                             // Incomplete object
	f.Add([]byte(`{`))                                                                                                           // Truncated JSON
	f.Add([]byte(`{"version":"1","version":"2"}`))                                                                               // Duplicate keys
	f.Add([]byte(`{"version":1}`))                                                                                               // Wrong type for version
	f.Add([]byte(`{"version":"1","account_sequence":1}`))                                                                        // Number instead of string for sequence
	f.Add([]byte(`{"version":"1","account_sequence":"not_a_number"}`))                                                           // Invalid number string
	f.Add([]byte(`{"version":"1","account_sequence":"-1"}`))                                                                     // Negative number
	f.Add([]byte(`{"version":"1","account_sequence":"18446744073709551616"}`))                                                   // Overflow uint64
	f.Add([]byte(`{"version":"1","messages":null}`))                                                                             // Null messages
	f.Add([]byte(`{"version":"1","messages":"not_array"}`))                                                                      // String instead of array
	f.Add([]byte(`{"version":"1","messages":[null]}`))                                                                           // Null message in array
	f.Add([]byte(`{"version":"1","messages":[{"type":null}]}`))                                                                  // Null type
	f.Add([]byte(`{"version":"1","messages":[{"type":"","data":null}]}`))                                                        // Empty type, null data
	f.Add([]byte(`{"version":"1","fee":{"amount":null}}`))                                                                       // Null fee amount
	f.Add([]byte(`{"version":"1","fee":{"gas_limit":null}}`))                                                                    // Null gas limit
	f.Add([]byte(`{"version":"1","fee_slippage":{"denominator":"0"}}`))                                                          // Zero denominator
	f.Add([]byte(`{"version":"1","chain_id":"\u0000\u0001\u0002"}`))                                                             // Control characters
	f.Add([]byte(`{"version":"1","memo":"` + strings.Repeat("x", 10000) + `"}`))                                                 // Very long memo
	f.Add([]byte(`{"version":"1","unknown_field":"value"}`))                                                                     // Unknown field
	f.Add([]byte(`{"version":"1","messages":[` + strings.Repeat(`{"type":"/m","data":{}},`, 100) + `{"type":"/m","data":{}}]}`)) // Many messages

	// Deeply nested JSON
	nested := `{"version":"1","messages":[{"type":"/m","data":`
	for i := 0; i < 100; i++ {
		nested += `{"nested":`
	}
	nested += `"deep"`
	for i := 0; i < 100; i++ {
		nested += `}`
	}
	nested += `}]}`
	f.Add([]byte(nested))

	f.Fuzz(func(t *testing.T, data []byte) {
		// SECURITY INVARIANT: ParseSignDoc must never panic on any input
		// SECURITY INVARIANT: ParseSignDoc must not cause excessive memory allocation

		sd, err := ParseSignDoc(data)

		if err != nil {
			// Parsing failed - this is expected for malformed input
			// Just verify we didn't panic
			return
		}

		// If parsing succeeded, verify basic invariants

		// INVARIANT: Roundtrip must be consistent
		// Parse -> Serialize -> Parse should give equivalent SignDoc
		jsonBytes, err := sd.ToJSON()
		if err != nil {
			// ToJSON should never fail for a successfully parsed SignDoc
			t.Errorf("ToJSON failed after successful parse: %v", err)
			return
		}

		sd2, err := ParseSignDoc(jsonBytes)
		if err != nil {
			t.Errorf("ParseSignDoc failed on re-serialized JSON: %v", err)
			return
		}

		// INVARIANT: Re-serialization must be identical
		jsonBytes2, err := sd2.ToJSON()
		if err != nil {
			t.Errorf("Second ToJSON failed: %v", err)
			return
		}

		if !bytes.Equal(jsonBytes, jsonBytes2) {
			t.Errorf("roundtrip not idempotent:\nfirst:  %s\nsecond: %s", jsonBytes, jsonBytes2)
		}

		// INVARIANT: Hashing must be consistent
		hash1, err := sd.GetSignBytes()
		if err != nil {
			t.Errorf("GetSignBytes failed: %v", err)
			return
		}

		hash2, err := sd2.GetSignBytes()
		if err != nil {
			t.Errorf("GetSignBytes failed on roundtripped SignDoc: %v", err)
			return
		}

		if !bytes.Equal(hash1, hash2) {
			t.Errorf("hash mismatch after roundtrip")
		}
	})
}

// =============================================================================
// FUZZ TEST: StringUint64 JSON Parsing
// =============================================================================

func FuzzStringUint64Unmarshal(f *testing.F) {
	// Valid inputs
	f.Add([]byte(`"0"`))
	f.Add([]byte(`"1"`))
	f.Add([]byte(`"18446744073709551615"`)) // Max uint64

	// Invalid inputs
	f.Add([]byte(`""`))                     // Empty string
	f.Add([]byte(`"-1"`))                   // Negative
	f.Add([]byte(`"18446744073709551616"`)) // Overflow
	f.Add([]byte(`"abc"`))                  // Non-numeric
	f.Add([]byte(`" 123"`))                 // Leading space
	f.Add([]byte(`"123 "`))                 // Trailing space
	f.Add([]byte(`"12.34"`))                // Decimal
	f.Add([]byte(`"1e10"`))                 // Scientific notation
	f.Add([]byte(`123`))                    // Number without quotes
	f.Add([]byte(`null`))                   // Null
	f.Add([]byte(`"00123"`))                // Leading zeros
	f.Add([]byte(`"+123"`))                 // Plus sign
	f.Add([]byte(`"0x123"`))                // Hex
	f.Add([]byte(`"0b101"`))                // Binary
	f.Add([]byte(`"0o777"`))                // Octal

	f.Fuzz(func(t *testing.T, data []byte) {
		var su StringUint64
		err := json.Unmarshal(data, &su)

		if err != nil {
			// Parsing failed - expected for invalid input
			return
		}

		// INVARIANT: Roundtrip must preserve value
		marshaled, err := json.Marshal(su)
		if err != nil {
			t.Errorf("Marshal failed after successful unmarshal: %v", err)
			return
		}

		var su2 StringUint64
		if err := json.Unmarshal(marshaled, &su2); err != nil {
			t.Errorf("Unmarshal failed on re-marshaled data: %v", err)
			return
		}

		if su != su2 {
			t.Errorf("value changed after roundtrip: %d -> %d", su, su2)
		}
	})
}

// =============================================================================
// FUZZ TEST: SignDocMessage Data Parsing
// =============================================================================

func FuzzSignDocMessageData(f *testing.F) {
	// Valid message data
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"key":"value"}`))
	f.Add([]byte(`{"from":"alice","to":"bob","amount":"100"}`))
	f.Add([]byte(`{"nested":{"deep":{"value":1}}}`))
	f.Add([]byte(`{"array":[1,2,3]}`))
	f.Add([]byte(`{"mixed":[1,"two",{"three":3}]}`))

	// Edge cases
	f.Add([]byte(`{"":"empty_key"}`))                           // Empty key
	f.Add([]byte(`{"key":""}`))                                 // Empty value
	f.Add([]byte(`{"unicode":"日本語🚀"}`))                         // Unicode
	f.Add([]byte(`{"escape":"\"\\\/\b\f\n\r\t"}`))              // Escaped chars
	f.Add([]byte(`{"nullval":null}`))                           // Null value
	f.Add([]byte(`{"bools":[true,false]}`))                     // Boolean values
	f.Add([]byte(`{"numbers":[-1,0,1,1.5,-1.5,1e10,1E-10]}`))   // Various numbers
	f.Add([]byte(`{"large":18446744073709551615}`))             // Large number
	f.Add([]byte(`{"negative":-9223372036854775808}`))          // Min int64
	f.Add([]byte(`{"path":"../../../etc/passwd"}`))             // Path traversal attempt
	f.Add([]byte(`{"script":"<script>alert('xss')</script>"}`)) // XSS attempt
	f.Add([]byte(`{"sql":"'; DROP TABLE users; --"}`))          // SQL injection attempt
	f.Add([]byte(`{"control":"\u0000\u001f"}`))                 // Control characters
	f.Add([]byte(`{"key":"value","key":"duplicate"}`))          // Duplicate keys

	// Large/stress inputs
	f.Add([]byte(`{"` + strings.Repeat("x", 1000) + `":"value"}`)) // Long key
	f.Add([]byte(`{"key":"` + strings.Repeat("x", 10000) + `"}`))  // Long value

	f.Fuzz(func(t *testing.T, data []byte) {
		// Test that message data can be used in a SignDoc without panicking
		sd := NewSignDoc("chain", 1, "alice", 1, "")
		sd.AddMessage("/test.msg", json.RawMessage(data))

		// SECURITY INVARIANT: ToJSON must not panic on any message data
		jsonBytes, err := sd.ToJSON()
		if err != nil {
			// This is unexpected - ToJSON shouldn't fail on struct serialization
			// but malformed RawMessage might cause issues
			return
		}

		// SECURITY INVARIANT: Roundtrip must be consistent if serialization succeeds
		sd2, err := ParseSignDoc(jsonBytes)
		if err != nil {
			// Parse could fail if the raw message data wasn't valid JSON
			return
		}

		jsonBytes2, err := sd2.ToJSON()
		if err != nil {
			t.Errorf("Second ToJSON failed: %v", err)
			return
		}

		if !bytes.Equal(jsonBytes, jsonBytes2) {
			t.Errorf("roundtrip not idempotent")
		}
	})
}

// =============================================================================
// FUZZ TEST: SignDocFee Validation
// =============================================================================

func FuzzSignDocFeeValidation(f *testing.F) {
	// Valid fees
	f.Add("0", "uatom", "1000")
	f.Add("200000", "stake", "0")
	f.Add("18446744073709551615", "denom", "18446744073709551615")

	// Invalid inputs
	f.Add("", "uatom", "1000")                   // Empty gas limit
	f.Add("-1", "uatom", "1000")                 // Negative gas limit
	f.Add("abc", "uatom", "1000")                // Non-numeric gas limit
	f.Add("0", "", "1000")                       // Empty denom
	f.Add("0", "uatom", "")                      // Empty amount
	f.Add("0", "uatom", "-1")                    // Negative amount
	f.Add("0", "uatom", "abc")                   // Non-numeric amount
	f.Add("1e10", "uatom", "1000")               // Scientific notation
	f.Add("0", strings.Repeat("x", 100), "1000") // Long denom

	f.Fuzz(func(t *testing.T, gasLimit, denom, amount string) {
		fee := SignDocFee{
			GasLimit: gasLimit,
			Amount: []SignDocCoin{
				{Denom: denom, Amount: amount},
			},
		}

		// SECURITY INVARIANT: ValidateBasic must not panic
		err := fee.ValidateBasic()

		if err != nil {
			// Validation failed - expected for invalid input
			return
		}

		// If validation passed, verify the values meet constraints
		if gasLimit == "" {
			t.Error("empty gas_limit should have failed validation")
		}
		if denom == "" {
			t.Error("empty denom should have failed validation")
		}
		if amount == "" {
			t.Error("empty amount should have failed validation")
		}
	})
}

// =============================================================================
// FUZZ TEST: SignDocRatio Validation (Division by Zero Prevention)
// =============================================================================

func FuzzSignDocRatioValidation(f *testing.F) {
	// Valid ratios
	f.Add("0", "1")
	f.Add("1", "100")
	f.Add("5", "100")
	f.Add("18446744073709551615", "18446744073709551615")

	// Invalid inputs
	f.Add("1", "0")   // Division by zero!
	f.Add("", "1")    // Empty numerator
	f.Add("1", "")    // Empty denominator
	f.Add("-1", "1")  // Negative numerator
	f.Add("1", "-1")  // Negative denominator
	f.Add("abc", "1") // Non-numeric numerator
	f.Add("1", "abc") // Non-numeric denominator
	f.Add("1.5", "1") // Decimal numerator
	f.Add("1", "1.5") // Decimal denominator
	f.Add("0", "0")   // Both zero

	f.Fuzz(func(t *testing.T, numerator, denominator string) {
		ratio := SignDocRatio{
			Numerator:   numerator,
			Denominator: denominator,
		}

		// SECURITY INVARIANT: ValidateBasic must not panic
		err := ratio.ValidateBasic()

		if err != nil {
			// Validation failed - expected for invalid input
			return
		}

		// SECURITY INVARIANT: Denominator must not be zero if validation passed
		if denominator == "0" {
			t.Error("zero denominator should have failed validation")
		}

		// SECURITY INVARIANT: Both values should be non-empty
		if numerator == "" || denominator == "" {
			t.Error("empty values should have failed validation")
		}
	})
}

// =============================================================================
// FUZZ TEST: SignDoc Complete Validation
// =============================================================================

func FuzzSignDocValidateBasic(f *testing.F) {
	// Seed with known inputs
	f.Add("1", "chain", "alice", uint64(1), uint64(1), "memo", "/msg.Type", `{}`)
	f.Add("1", "", "alice", uint64(0), uint64(0), "", "/msg", `{}`)       // Empty chain_id
	f.Add("1", "chain", "", uint64(0), uint64(0), "", "/msg", `{}`)       // Empty account
	f.Add("99", "chain", "alice", uint64(1), uint64(1), "", "/msg", `{}`) // Invalid version
	f.Add("1", "chain", "alice", uint64(1), uint64(1), "", "", `{}`)      // Empty message type

	f.Fuzz(func(t *testing.T, version, chainID, account string, seq, nonce uint64, memo, msgType, msgData string) {
		// Build a SignDoc manually
		sd := &SignDoc{
			Version:         version,
			ChainID:         chainID,
			Account:         account,
			AccountSequence: StringUint64(seq),
			Nonce:           StringUint64(nonce),
			Memo:            memo,
			Messages:        []SignDocMessage{{Type: msgType, Data: json.RawMessage(msgData)}},
			Fee:             SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
			FeeSlippage:     SignDocRatio{Numerator: "0", Denominator: "1"},
		}

		// SECURITY INVARIANT: ValidateBasic must not panic
		err := sd.ValidateBasic()

		if err != nil {
			// Validation failed - expected for invalid input
			return
		}

		// If validation passed, verify basic constraints
		if version != SignDocVersion {
			t.Errorf("version %q should have failed validation (expected %q)", version, SignDocVersion)
		}
		if chainID == "" {
			t.Error("empty chain_id should have failed validation")
		}
		if account == "" {
			t.Error("empty account should have failed validation")
		}
		if msgType == "" {
			t.Error("empty message type should have failed validation")
		}
	})
}

// =============================================================================
// FUZZ TEST: Unicode and Special Character Handling
// =============================================================================

func FuzzSignDocUnicodeHandling(f *testing.F) {
	// Various Unicode strings
	f.Add("Hello World")
	f.Add("日本語")
	f.Add("🚀💰🔐")
	f.Add("مرحبا")                  // RTL
	f.Add("e\u0301")                // Combining character
	f.Add("\u200B")                 // Zero-width space
	f.Add("\u202E")                 // RTL override
	f.Add("\uFEFF")                 // BOM
	f.Add("\u0000")                 // Null
	f.Add("\x00\x01\x02\x03")       // Control chars
	f.Add(strings.Repeat("🎉", 100)) // Many emojis
	f.Add("a\xc0\xc1")              // Invalid UTF-8
	f.Add("\xff\xfe")               // Invalid UTF-8

	f.Fuzz(func(t *testing.T, input string) {
		// Test that we handle all string inputs gracefully

		// Test in memo field
		sd := NewSignDoc("chain", 1, "alice", 1, input)
		sd.AddMessage("/msg", json.RawMessage(`{}`))

		jsonBytes, err := sd.ToJSON()
		if err != nil {
			// ToJSON might fail for invalid strings, but shouldn't panic
			return
		}

		// If serialization succeeded, verify roundtrip
		sd2, err := ParseSignDoc(jsonBytes)
		if err != nil {
			// This would be concerning - we serialized but can't parse
			t.Errorf("failed to parse our own output: %v", err)
			return
		}

		// INVARIANT: Roundtrip must preserve the memo
		if sd2.Memo != input {
			// Only fail if it's valid UTF-8 - invalid UTF-8 might be normalized
			if utf8.ValidString(input) {
				t.Errorf("memo not preserved: %q -> %q", input, sd2.Memo)
			}
		}
	})
}

// =============================================================================
// FUZZ TEST: Large Input Handling (Memory Exhaustion Prevention)
// =============================================================================

func FuzzSignDocLargeInputs(f *testing.F) {
	// Various size parameters
	f.Add(1, 1, 1)
	f.Add(10, 10, 100)
	f.Add(100, 100, 1000)
	f.Add(257, 1, 100)  // More than MaxMessagesPerSignDoc
	f.Add(1, 65537, 10) // More than MaxMessageDataSize

	f.Fuzz(func(t *testing.T, numMessages, dataSize, chainIDLen int) {
		// Bound inputs to prevent test timeout
		if numMessages < 0 || numMessages > 300 {
			return
		}
		if dataSize < 0 || dataSize > 100000 {
			return
		}
		if chainIDLen < 0 || chainIDLen > 1000 {
			return
		}

		chainID := strings.Repeat("c", chainIDLen)
		if chainID == "" {
			chainID = "c"
		}

		sd := &SignDoc{
			Version:         SignDocVersion,
			ChainID:         chainID,
			Account:         "alice",
			AccountSequence: 1,
			Nonce:           1,
			Messages:        make([]SignDocMessage, 0, numMessages),
			Fee:             SignDocFee{Amount: []SignDocCoin{}, GasLimit: "0"},
			FeeSlippage:     SignDocRatio{Numerator: "0", Denominator: "1"},
		}

		// Create data of specified size
		data := make([]byte, dataSize)
		for i := range data {
			data[i] = 'x'
		}
		msgData := json.RawMessage(`{"data":"` + string(data) + `"}`)

		// Add messages
		for i := 0; i < numMessages; i++ {
			sd.Messages = append(sd.Messages, SignDocMessage{
				Type: "/msg",
				Data: msgData,
			})
		}

		// SECURITY INVARIANT: ValidateBasic should reject oversized inputs
		err := sd.ValidateBasic()

		// Check DoS protection limits
		if numMessages > MaxMessagesPerSignDoc && err == nil {
			t.Errorf("should reject %d messages (> %d)", numMessages, MaxMessagesPerSignDoc)
		}

		if dataSize > MaxMessageDataSize && err == nil && numMessages > 0 {
			t.Errorf("should reject message data of size %d (> %d)", dataSize, MaxMessageDataSize)
		}
	})
}

// =============================================================================
// FUZZ TEST: Duplicate Key Handling
// =============================================================================
// The Theorist specifically noted: "Test that ParseSignDoc rejects JSON with
// duplicate keys, as this could affect determinism."

func FuzzSignDocDuplicateKeys(f *testing.F) {
	// JSON with duplicate keys at various levels
	f.Add(`{"version":"1","version":"2"}`)
	f.Add(`{"version":"1","chain_id":"a","chain_id":"b"}`)
	f.Add(`{"version":"1","messages":[{"type":"a","type":"b"}]}`)
	f.Add(`{"version":"1","fee":{"gas_limit":"1","gas_limit":"2"}}`)
	f.Add(`{"version":"1","fee_slippage":{"numerator":"1","numerator":"2"}}`)

	f.Fuzz(func(t *testing.T, jsonStr string) {
		data := []byte(jsonStr)

		sd, err := ParseSignDoc(data)
		if err != nil {
			// Parsing failed - acceptable
			return
		}

		// If parsing succeeded, check for determinism
		// SECURITY: If we parsed JSON with duplicate keys, verify behavior is consistent

		json1, err := sd.ToJSON()
		if err != nil {
			return
		}

		sd2, err := ParseSignDoc(json1)
		if err != nil {
			t.Error("failed to parse our own output")
			return
		}

		json2, err := sd2.ToJSON()
		if err != nil {
			t.Error("second ToJSON failed")
			return
		}

		// INVARIANT: Canonical form must be deterministic
		if !bytes.Equal(json1, json2) {
			t.Error("non-deterministic serialization after parsing duplicate keys")
		}
	})
}

// =============================================================================
// FUZZ TEST: JSON Injection Prevention
// =============================================================================

func FuzzSignDocJSONInjection(f *testing.F) {
	// Strings that might break JSON parsing or cause injection
	f.Add(`"injected":"field"`)
	f.Add(`},"evil":{"nested":"attack"},"ignored":{"`)
	f.Add(`","account":"hacker","memo":"`)
	f.Add(`\",\"account\":\"hacker`)
	f.Add("\",\"nonce\":\"999")

	f.Fuzz(func(t *testing.T, payload string) {
		// Try to inject the payload through the memo field
		sd := NewSignDoc("chain", 1, "alice", 1, payload)
		sd.AddMessage("/msg", json.RawMessage(`{}`))

		jsonBytes, err := sd.ToJSON()
		if err != nil {
			return
		}

		// SECURITY INVARIANT: The payload must be properly escaped
		sd2, err := ParseSignDoc(jsonBytes)
		if err != nil {
			// If we can serialize but not parse, that's a problem
			t.Errorf("failed to parse serialized SignDoc: %v", err)
			return
		}

		// SECURITY INVARIANT: The memo must match exactly
		if sd2.Memo != payload {
			// For valid UTF-8, the memo should be preserved exactly
			if utf8.ValidString(payload) {
				t.Errorf("memo changed: %q -> %q", payload, sd2.Memo)
			}
		}

		// SECURITY INVARIANT: Other fields must not be affected by the payload
		if sd2.ChainID != "chain" {
			t.Error("chain_id was modified by injection attempt")
		}
		if sd2.Account != "alice" {
			t.Error("account was modified by injection attempt")
		}
		if uint64(sd2.AccountSequence) != 1 {
			t.Error("account_sequence was modified by injection attempt")
		}
		if uint64(sd2.Nonce) != 1 {
			t.Error("nonce was modified by injection attempt")
		}
	})
}

// =============================================================================
// FUZZ TEST: SignDocCoin Validation
// =============================================================================

func FuzzSignDocCoinValidation(f *testing.F) {
	f.Add("uatom", "1000")
	f.Add("", "1000")                      // Empty denom
	f.Add("uatom", "")                     // Empty amount
	f.Add("uatom", "-1")                   // Negative
	f.Add("uatom", "abc")                  // Non-numeric
	f.Add(strings.Repeat("x", 65), "1000") // Denom too long
	f.Add(strings.Repeat("x", 64), "1000") // Denom at max length
	f.Add("uatom", "18446744073709551615") // Max uint64
	f.Add("uatom", "18446744073709551616") // Overflow

	f.Fuzz(func(t *testing.T, denom, amount string) {
		coin := SignDocCoin{
			Denom:  denom,
			Amount: amount,
		}

		// SECURITY INVARIANT: ValidateBasic must not panic
		err := coin.ValidateBasic()

		if err != nil {
			return
		}

		// If validation passed, verify constraints
		if denom == "" {
			t.Error("empty denom should have failed validation")
		}
		if len(denom) > 64 {
			t.Error("denom > 64 chars should have failed validation")
		}
		if amount == "" {
			t.Error("empty amount should have failed validation")
		}
	})
}

// =============================================================================
// FUZZ TEST: Maximum Value Boundaries
// =============================================================================

func FuzzSignDocBoundaryValues(f *testing.F) {
	f.Add(uint64(0))
	f.Add(uint64(1))
	f.Add(uint64(math.MaxUint64 - 1))
	f.Add(uint64(math.MaxUint64))
	f.Add(uint64(math.MaxInt64))
	f.Add(uint64(math.MaxInt64 + 1))

	f.Fuzz(func(t *testing.T, value uint64) {
		// Test with the value in various fields
		sd := NewSignDoc("chain", value, "alice", value, "")
		sd.AddMessage("/msg", json.RawMessage(`{}`))

		// SECURITY INVARIANT: Serialization must not panic
		jsonBytes, err := sd.ToJSON()
		if err != nil {
			t.Errorf("ToJSON failed for value %d: %v", value, err)
			return
		}

		// SECURITY INVARIANT: Roundtrip must preserve values exactly
		sd2, err := ParseSignDoc(jsonBytes)
		if err != nil {
			t.Errorf("ParseSignDoc failed for value %d: %v", value, err)
			return
		}

		if uint64(sd2.AccountSequence) != value {
			t.Errorf("account_sequence not preserved: %d -> %d", value, uint64(sd2.AccountSequence))
		}
		if uint64(sd2.Nonce) != value {
			t.Errorf("nonce not preserved: %d -> %d", value, uint64(sd2.Nonce))
		}
	})
}
