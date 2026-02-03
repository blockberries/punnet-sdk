package types

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/blockberries/cramberry/pkg/cramberry"
)

// =============================================================================
// FUZZ TESTS FOR JSON STRING ESCAPING (EscapeJSONString)
// =============================================================================
// These tests validate that the cramberry.EscapeJSONString function produces
// correct, safe, and deterministic JSON string output.
//
// Run with: go test -fuzz=FuzzEscapeJSONString -fuzztime=60s ./types/...
//
// SECURITY RATIONALE:
// JSON string escaping is a critical component of deterministic signing:
// 1. Incorrect escaping can produce invalid JSON, causing parse failures
// 2. Non-deterministic escaping can cause signature verification failures
// 3. Missing escapes for control characters can enable injection attacks
// 4. Invalid UTF-8 handling could cause panics or undefined behavior

// =============================================================================
// FUZZ TEST: EscapeJSONString with Random Byte Sequences
// =============================================================================

func FuzzEscapeJSONString_RandomBytes(f *testing.F) {
	// Seed corpus: valid UTF-8 strings
	f.Add([]byte("hello world"))
	f.Add([]byte(""))
	f.Add([]byte("Hello, 世界"))
	f.Add([]byte("🚀💰🔐"))
	f.Add([]byte("مرحبا"))  // RTL text
	f.Add([]byte("日本語한국어")) // Multiple scripts

	// Seed corpus: strings requiring escaping
	f.Add([]byte(`"quoted"`))
	f.Add([]byte("line1\nline2"))
	f.Add([]byte("tab\there"))
	f.Add([]byte(`back\slash`))
	f.Add([]byte("null\x00byte"))
	f.Add([]byte("\x01\x02\x03\x04\x05")) // Control characters

	// Seed corpus: JSON special characters
	f.Add([]byte(`{"key":"value"}`))
	f.Add([]byte(`</script>`))
	f.Add([]byte(`\u0000`)) // Literal backslash-u
	f.Add([]byte("\u2028")) // Line separator (U+2028)
	f.Add([]byte("\u2029")) // Paragraph separator (U+2029)

	// Seed corpus: Unicode edge cases
	f.Add([]byte("caf\u00e9"))       // Composed é (NFC)
	f.Add([]byte("cafe\u0301"))      // e + combining acute (NFD)
	f.Add([]byte("\ufeff"))          // BOM
	f.Add([]byte("\u200b"))          // Zero-width space
	f.Add([]byte("\u202e"))          // RTL override
	f.Add([]byte("\ufffd"))          // Replacement character
	f.Add([]byte("👨\u200d👩\u200d👧")) // Family emoji with ZWJ

	// Seed corpus: Surrogate pair edge cases (invalid standalone in UTF-8)
	f.Add([]byte{0xED, 0xA0, 0x80}) // Invalid: high surrogate encoded as UTF-8
	f.Add([]byte{0xED, 0xB0, 0x80}) // Invalid: low surrogate encoded as UTF-8

	// Seed corpus: Invalid UTF-8 sequences
	f.Add([]byte{0xFF})                   // Invalid single byte
	f.Add([]byte{0xFE})                   // Invalid single byte
	f.Add([]byte{0xC0, 0x80})             // Overlong encoding of NULL
	f.Add([]byte{0xE0, 0x80, 0x80})       // Overlong encoding
	f.Add([]byte{0xF0, 0x80, 0x80, 0x80}) // Overlong encoding
	f.Add([]byte{0xC2})                   // Truncated 2-byte sequence
	f.Add([]byte{0xE0, 0xBF})             // Truncated 3-byte sequence
	f.Add([]byte{0xF0, 0x90, 0x80})       // Truncated 4-byte sequence

	// Seed corpus: Boundary code points
	f.Add([]byte("\x7f"))                 // DEL character
	f.Add([]byte(string(rune(0x10FFFF)))) // Maximum valid code point
	f.Add([]byte(string(rune(0x0000))))   // NULL
	f.Add([]byte(string(rune(0x001F))))   // Last control character

	f.Fuzz(func(t *testing.T, input []byte) {
		inputStr := string(input)

		// SECURITY INVARIANT: EscapeJSONString must never panic
		escaped := cramberry.EscapeJSONString(inputStr)

		// SECURITY INVARIANT: Output must be a valid JSON string (quoted)
		if !strings.HasPrefix(escaped, `"`) || !strings.HasSuffix(escaped, `"`) {
			t.Errorf("EscapeJSONString output is not a quoted string: %q", escaped)
			return
		}

		// SECURITY INVARIANT: Output must be valid JSON when wrapped in an object
		testJSON := `{"test":` + escaped + `}`
		if !json.Valid([]byte(testJSON)) {
			t.Errorf("EscapeJSONString produced invalid JSON:\n  Input bytes: %x\n  Input string: %q\n  Escaped: %s\n  Test JSON: %s",
				input, inputStr, escaped, testJSON)
			return
		}

		// SECURITY INVARIANT: JSON parsing should succeed
		var parsed map[string]string
		if err := json.Unmarshal([]byte(testJSON), &parsed); err != nil {
			t.Errorf("Failed to unmarshal escaped string:\n  Input: %q\n  Escaped: %s\n  Error: %v",
				inputStr, escaped, err)
			return
		}

		// SECURITY INVARIANT: For valid UTF-8 input, round-trip should preserve exactly
		if utf8.ValidString(inputStr) {
			if parsed["test"] != inputStr {
				t.Errorf("Round-trip failed for valid UTF-8:\n  Input: %q\n  Parsed: %q",
					inputStr, parsed["test"])
			}
		}

		// SECURITY INVARIANT: Repeated calls must produce identical output (determinism)
		for i := 0; i < 3; i++ {
			escaped2 := cramberry.EscapeJSONString(inputStr)
			if escaped != escaped2 {
				t.Errorf("Non-deterministic output on iteration %d:\n  First: %s\n  Got: %s",
					i, escaped, escaped2)
				return
			}
		}
	})
}

// =============================================================================
// FUZZ TEST: EscapeJSONString Control Character Handling
// =============================================================================

func FuzzEscapeJSONString_ControlChars(f *testing.F) {
	// Test all single-byte inputs (0x00-0xFF)
	for b := 0; b <= 0xFF; b++ {
		f.Add([]byte{byte(b)})
	}

	// Test control characters embedded in strings
	for c := 0; c < 32; c++ {
		f.Add([]byte("prefix" + string(rune(c)) + "suffix"))
	}
	f.Add([]byte("prefix\x7fsuffix")) // DEL

	f.Fuzz(func(t *testing.T, input []byte) {
		inputStr := string(input)
		escaped := cramberry.EscapeJSONString(inputStr)

		// SECURITY INVARIANT: Control characters MUST be escaped per RFC 8259
		// Valid JSON strings cannot contain raw control characters (0x00-0x1F)
		// Note: DEL (0x7F) is NOT required to be escaped by RFC 8259, though some
		// implementations choose to escape it for safety.
		testJSON := `{"v":` + escaped + `}`
		if !json.Valid([]byte(testJSON)) {
			t.Errorf("Control char escaping produced invalid JSON:\n  Input: %x\n  Escaped: %s",
				input, escaped)
			return
		}

		// Additional check: the escaped string between quotes should not contain
		// raw C0 control characters (0x00-0x1F). DEL (0x7F) is allowed per RFC 8259.
		inner := escaped[1 : len(escaped)-1] // Remove surrounding quotes
		inEscape := false
		for i := 0; i < len(inner); i++ {
			b := inner[i]
			if inEscape {
				inEscape = false
				continue
			}
			if b == '\\' {
				inEscape = true
				continue
			}
			// RFC 8259 requires escaping only 0x00-0x1F
			if b < 0x20 {
				t.Errorf("Raw C0 control character (0x%02x) found in escaped output at position %d:\n  Input: %x\n  Escaped: %s",
					b, i, input, escaped)
				return
			}
		}
	})
}

// =============================================================================
// FUZZ TEST: EscapeJSONString Unicode Edge Cases
// =============================================================================

func FuzzEscapeJSONString_UnicodeEdgeCases(f *testing.F) {
	// Combining characters
	f.Add("e\u0301")       // e + combining acute
	f.Add("a\u0300")       // a + combining grave
	f.Add("o\u0302\u0323") // o + circumflex + dot below

	// Zero-width characters
	f.Add("\u200b") // Zero-width space
	f.Add("\u200c") // Zero-width non-joiner
	f.Add("\u200d") // Zero-width joiner
	f.Add("\ufeff") // BOM / zero-width no-break space

	// Directional control
	f.Add("\u200e") // Left-to-right mark
	f.Add("\u200f") // Right-to-left mark
	f.Add("\u202a") // Left-to-right embedding
	f.Add("\u202b") // Right-to-left embedding
	f.Add("\u202c") // Pop directional formatting
	f.Add("\u202d") // Left-to-right override
	f.Add("\u202e") // Right-to-left override

	// Line/paragraph separators (must be escaped in JSON per RFC 8259)
	f.Add("\u2028") // Line separator
	f.Add("\u2029") // Paragraph separator

	// Surrogate pairs (valid in UTF-16, represented as single codepoints in Go)
	f.Add(string(rune(0x10000)))  // First supplementary plane char
	f.Add(string(rune(0x1F600)))  // Grinning face emoji
	f.Add(string(rune(0x10FFFF))) // Maximum valid Unicode code point

	// Private use area
	f.Add(string(rune(0xE000)))   // Start of private use area
	f.Add(string(rune(0xF8FF)))   // End of private use area (BMP)
	f.Add(string(rune(0x100000))) // Supplementary private use area

	// Noncharacters
	f.Add(string(rune(0xFFFE))) // Noncharacter
	f.Add(string(rune(0xFFFF))) // Noncharacter

	// Replacement character
	f.Add("\ufffd") // Replacement character

	f.Fuzz(func(t *testing.T, input string) {
		escaped := cramberry.EscapeJSONString(input)

		// SECURITY INVARIANT: Must produce valid JSON
		testJSON := `{"v":` + escaped + `}`
		if !json.Valid([]byte(testJSON)) {
			t.Errorf("Unicode edge case produced invalid JSON:\n  Input: %q\n  Escaped: %s",
				input, escaped)
			return
		}

		// Round-trip test
		var parsed map[string]string
		if err := json.Unmarshal([]byte(testJSON), &parsed); err != nil {
			t.Errorf("Failed to parse escaped unicode:\n  Input: %q\n  Error: %v", input, err)
			return
		}

		// For valid UTF-8, round-trip must preserve the string exactly
		if utf8.ValidString(input) {
			if parsed["v"] != input {
				t.Errorf("Unicode round-trip failed:\n  Input: %q (%x)\n  Parsed: %q (%x)",
					input, []byte(input), parsed["v"], []byte(parsed["v"]))
			}
		}
	})
}

// =============================================================================
// FUZZ TEST: EscapeJSONString in SignDoc Context
// =============================================================================
// This test verifies that EscapeJSONString works correctly when used in the
// context of SignDoc serialization, which is the actual use case.

func FuzzEscapeJSONString_SignDocContext(f *testing.F) {
	// Common memo patterns
	f.Add("Simple memo")
	f.Add("Memo with \"quotes\"")
	f.Add("Memo with\nnewlines")
	f.Add("Memo with\ttabs")
	f.Add("Multi-line\nmemo\nwith\nseveral\nlines")
	f.Add(`JSON-like: {"key": "value"}`)
	f.Add(`Escaped: \"quoted\"`)
	f.Add("Unicode: 日本語 🚀 مرحبا")

	// Injection attempts
	f.Add(`","evil":"injected","x":"`)
	f.Add(`\",\"evil\":\"injected`)
	f.Add("\"}\n{\"injected\":\"")

	f.Fuzz(func(t *testing.T, memo string) {
		// Skip invalid UTF-8 for this test since SignDoc fields should be valid UTF-8
		if !utf8.ValidString(memo) {
			return
		}

		// Create a SignDoc with the fuzzed memo
		sd := NewSignDoc("chain-1", 1, "alice", 1, memo)
		sd.AddMessage("/msg.Test", json.RawMessage(`{}`))

		// SECURITY INVARIANT: ToJSON must not panic
		jsonBytes, err := sd.ToJSON()
		if err != nil {
			t.Errorf("ToJSON failed for memo %q: %v", memo, err)
			return
		}

		// SECURITY INVARIANT: Output must be valid JSON
		if !json.Valid(jsonBytes) {
			t.Errorf("ToJSON produced invalid JSON for memo %q:\n  Output: %s", memo, jsonBytes)
			return
		}

		// SECURITY INVARIANT: Round-trip must preserve the memo exactly
		parsed, err := ParseSignDoc(jsonBytes)
		if err != nil {
			t.Errorf("ParseSignDoc failed for memo %q: %v\n  JSON: %s", memo, err, jsonBytes)
			return
		}

		if parsed.Memo != memo {
			t.Errorf("Memo not preserved after round-trip:\n  Input: %q\n  Parsed: %q",
				memo, parsed.Memo)
			return
		}

		// SECURITY INVARIANT: Other fields must not be affected (injection prevention)
		if parsed.ChainID != "chain-1" {
			t.Errorf("ChainID modified by memo injection: got %q", parsed.ChainID)
		}
		if parsed.Account != "alice" {
			t.Errorf("Account modified by memo injection: got %q", parsed.Account)
		}
		if uint64(parsed.AccountSequence) != 1 {
			t.Errorf("AccountSequence modified by memo injection: got %d", parsed.AccountSequence)
		}

		// SECURITY INVARIANT: Deterministic serialization
		jsonBytes2, err := sd.ToJSON()
		if err != nil || !bytes.Equal(jsonBytes, jsonBytes2) {
			t.Errorf("Non-deterministic serialization for memo %q", memo)
		}
	})
}

// =============================================================================
// PROPERTY-BASED TESTS: Specific Unicode Categories
// =============================================================================

func TestEscapeJSONString_AllControlChars(t *testing.T) {
	// Test all C0 control characters (U+0000 to U+001F) and DEL (U+007F)
	// JSON requires these to be escaped.
	for c := rune(0); c <= 0x1F; c++ {
		t.Run(controlCharName(c), func(t *testing.T) {
			input := string(c)
			escaped := cramberry.EscapeJSONString(input)

			testJSON := `{"v":` + escaped + `}`
			if !json.Valid([]byte(testJSON)) {
				t.Errorf("Control char U+%04X not properly escaped:\n  Escaped: %s\n  JSON: %s",
					c, escaped, testJSON)
				return
			}

			var parsed map[string]string
			if err := json.Unmarshal([]byte(testJSON), &parsed); err != nil {
				t.Errorf("Failed to unmarshal escaped control char U+%04X: %v", c, err)
				return
			}

			if parsed["v"] != input {
				t.Errorf("Control char U+%04X not preserved: got %q (%x)",
					c, parsed["v"], []byte(parsed["v"]))
			}
		})
	}

	// Test DEL (U+007F)
	t.Run("DEL", func(t *testing.T) {
		input := string(rune(0x7F))
		escaped := cramberry.EscapeJSONString(input)

		testJSON := `{"v":` + escaped + `}`
		if !json.Valid([]byte(testJSON)) {
			t.Errorf("DEL not properly escaped: %s", escaped)
		}
	})
}

func TestEscapeJSONString_LineSeparators(t *testing.T) {
	// U+2028 (Line Separator) and U+2029 (Paragraph Separator) must be escaped
	// per RFC 8259 (they would terminate JavaScript string literals otherwise)
	separators := []struct {
		name string
		char rune
	}{
		{"LINE SEPARATOR", 0x2028},
		{"PARAGRAPH SEPARATOR", 0x2029},
	}

	for _, sep := range separators {
		t.Run(sep.name, func(t *testing.T) {
			input := "before" + string(sep.char) + "after"
			escaped := cramberry.EscapeJSONString(input)

			// Must be valid JSON
			testJSON := `{"v":` + escaped + `}`
			if !json.Valid([]byte(testJSON)) {
				t.Errorf("%s not properly escaped:\n  Input: %q\n  Escaped: %s",
					sep.name, input, escaped)
				return
			}

			// Round-trip must preserve
			var parsed map[string]string
			if err := json.Unmarshal([]byte(testJSON), &parsed); err != nil {
				t.Errorf("Failed to unmarshal %s: %v", sep.name, err)
				return
			}

			if parsed["v"] != input {
				t.Errorf("%s not preserved:\n  Input: %q\n  Parsed: %q",
					sep.name, input, parsed["v"])
			}
		})
	}
}

func TestEscapeJSONString_SurrogatePairs(t *testing.T) {
	// Test emoji and other characters that require surrogate pairs in UTF-16
	// but are single code points in UTF-8/Go strings
	testCases := []struct {
		name  string
		input string
	}{
		{"Emoji", "🚀"},
		{"Multiple emoji", "🎉🎊🎈"},
		{"Emoji with ZWJ", "👨‍👩‍👧"},
		{"High code point", string(rune(0x10FFFF))},
		{"Musical symbol", "𝄞"},
		{"Math symbol", "𝕏"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			escaped := cramberry.EscapeJSONString(tc.input)

			testJSON := `{"v":` + escaped + `}`
			if !json.Valid([]byte(testJSON)) {
				t.Errorf("Surrogate pair char not properly escaped:\n  Input: %q\n  Escaped: %s",
					tc.input, escaped)
				return
			}

			var parsed map[string]string
			if err := json.Unmarshal([]byte(testJSON), &parsed); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
				return
			}

			if parsed["v"] != tc.input {
				t.Errorf("Not preserved:\n  Input: %q (%x)\n  Parsed: %q (%x)",
					tc.input, []byte(tc.input), parsed["v"], []byte(parsed["v"]))
			}
		})
	}
}

func TestEscapeJSONString_InvalidUTF8Sequences(t *testing.T) {
	// Test that invalid UTF-8 sequences don't cause panics and produce valid JSON
	invalidSequences := []struct {
		name  string
		bytes []byte
	}{
		{"High byte", []byte{0xFF}},
		{"Overlong NULL", []byte{0xC0, 0x80}},
		{"Truncated 2-byte", []byte{0xC2}},
		{"Truncated 3-byte", []byte{0xE0, 0xBF}},
		{"Truncated 4-byte", []byte{0xF0, 0x90, 0x80}},
		{"Invalid continuation", []byte{0x80}},
		{"Invalid start", []byte{0xFE}},
		{"High surrogate as UTF-8", []byte{0xED, 0xA0, 0x80}},
		{"Low surrogate as UTF-8", []byte{0xED, 0xB0, 0x80}},
		{"Mixed valid invalid", []byte{'a', 0xFF, 'b'}},
	}

	for _, tc := range invalidSequences {
		t.Run(tc.name, func(t *testing.T) {
			input := string(tc.bytes)

			// Should not panic
			escaped := cramberry.EscapeJSONString(input)

			// Must produce valid JSON (even if the content is modified)
			testJSON := `{"v":` + escaped + `}`
			if !json.Valid([]byte(testJSON)) {
				t.Errorf("Invalid UTF-8 produced invalid JSON:\n  Input bytes: %x\n  Escaped: %s",
					tc.bytes, escaped)
			}

			// Note: We don't require round-trip preservation for invalid UTF-8
			// The important thing is that we don't panic and produce valid JSON
		})
	}
}

func TestEscapeJSONString_CombiningCharacters(t *testing.T) {
	// Test combining characters (diacritical marks, etc.)
	testCases := []struct {
		name  string
		input string
	}{
		{"Combining acute", "e\u0301"},          // é as e + combining acute
		{"Combining grave", "a\u0300"},          // à as a + combining grave
		{"Multiple combining", "o\u0302\u0323"}, // ộ as o + circumflex + dot below
		{"Emoji modifier", "👋🏽"},                // Waving hand with skin tone
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			escaped := cramberry.EscapeJSONString(tc.input)

			testJSON := `{"v":` + escaped + `}`
			if !json.Valid([]byte(testJSON)) {
				t.Errorf("Combining char not properly escaped:\n  Input: %q\n  Escaped: %s",
					tc.input, escaped)
				return
			}

			var parsed map[string]string
			if err := json.Unmarshal([]byte(testJSON), &parsed); err != nil {
				t.Errorf("Failed to unmarshal: %v", err)
				return
			}

			// Combining characters must be preserved exactly (no normalization)
			if parsed["v"] != tc.input {
				t.Errorf("Combining char not preserved exactly:\n  Input: %q (%x)\n  Parsed: %q (%x)",
					tc.input, []byte(tc.input), parsed["v"], []byte(parsed["v"]))
			}
		})
	}
}

func TestEscapeJSONString_RTLText(t *testing.T) {
	// Test right-to-left text and bidirectional control characters
	testCases := []struct {
		name  string
		input string
	}{
		{"Arabic", "مرحبا بالعالم"},
		{"Hebrew", "שלום עולם"},
		{"Mixed LTR/RTL", "Hello مرحبا World"},
		{"With LRM", "text\u200Emore"}, // Left-to-right mark
		{"With RLM", "text\u200Fmore"}, // Right-to-left mark
		{"With LRO", "text\u202Dmore"}, // Left-to-right override
		{"With RLO", "text\u202Emore"}, // Right-to-left override
		{"With PDF", "text\u202Cmore"}, // Pop directional formatting
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			escaped := cramberry.EscapeJSONString(tc.input)

			testJSON := `{"v":` + escaped + `}`
			if !json.Valid([]byte(testJSON)) {
				t.Errorf("RTL text not properly escaped:\n  Input: %q\n  Escaped: %s",
					tc.input, escaped)
				return
			}

			var parsed map[string]string
			if err := json.Unmarshal([]byte(testJSON), &parsed); err != nil {
				t.Errorf("Failed to unmarshal RTL text: %v", err)
				return
			}

			if parsed["v"] != tc.input {
				t.Errorf("RTL text not preserved:\n  Input: %q\n  Parsed: %q",
					tc.input, parsed["v"])
			}
		})
	}
}

func TestEscapeJSONString_Determinism(t *testing.T) {
	// Verify that EscapeJSONString is deterministic across many iterations
	testCases := []string{
		"simple",
		"with \"quotes\"",
		"with\nnewline",
		"unicode 日本語 🚀",
		"control\x00\x01\x02chars",
		"\u2028line separator\u2029paragraph separator",
		"mixed content: {\"json\": \"like\", \"value\": 123}",
	}

	for _, input := range testCases {
		t.Run(input[:min(20, len(input))], func(t *testing.T) {
			first := cramberry.EscapeJSONString(input)

			for i := 0; i < 1000; i++ {
				result := cramberry.EscapeJSONString(input)
				if result != first {
					t.Errorf("Non-deterministic output on iteration %d:\n  First: %s\n  Got: %s",
						i, first, result)
					return
				}
			}
		})
	}
}

func TestEscapeJSONString_CrossGoVersionStability(t *testing.T) {
	// Test that strings.Builder behavior is consistent
	// This test documents expected output for specific inputs
	// If this test fails after a Go version upgrade, it indicates a potential
	// breaking change in the standard library that affects signing

	knownVectors := []struct {
		input    string
		expected string // Expected escaped output including quotes
	}{
		{`hello`, `"hello"`},
		{`"`, `"\""`},
		{`\`, `"\\"`},
		{"\n", `"\n"`},
		{"\t", `"\t"`},
		{"\r", `"\r"`},
		{"\x00", `"\u0000"`},
		{"\x1f", `"\u001f"`},
		// Note: The exact output for U+2028/U+2029 depends on implementation
		// Some implementations use \uXXXX, others may pass through if valid JSON
	}

	for _, vec := range knownVectors {
		t.Run(vec.input, func(t *testing.T) {
			result := cramberry.EscapeJSONString(vec.input)
			if result != vec.expected {
				t.Errorf("Output changed:\n  Input: %q\n  Expected: %s\n  Got: %s",
					vec.input, vec.expected, result)
			}
		})
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// controlCharName returns a human-readable name for control characters
func controlCharName(c rune) string {
	names := map[rune]string{
		0x00: "NUL", 0x01: "SOH", 0x02: "STX", 0x03: "ETX",
		0x04: "EOT", 0x05: "ENQ", 0x06: "ACK", 0x07: "BEL",
		0x08: "BS", 0x09: "HT", 0x0A: "LF", 0x0B: "VT",
		0x0C: "FF", 0x0D: "CR", 0x0E: "SO", 0x0F: "SI",
		0x10: "DLE", 0x11: "DC1", 0x12: "DC2", 0x13: "DC3",
		0x14: "DC4", 0x15: "NAK", 0x16: "SYN", 0x17: "ETB",
		0x18: "CAN", 0x19: "EM", 0x1A: "SUB", 0x1B: "ESC",
		0x1C: "FS", 0x1D: "GS", 0x1E: "RS", 0x1F: "US",
	}
	if name, ok := names[c]; ok {
		return name
	}
	return "UNKNOWN"
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

