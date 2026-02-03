package types

import (
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"
)

// isValidUTF8 checks if a string is valid UTF-8.
// This is used in fuzz tests to skip invalid inputs that would be mangled during JSON encoding.
func isValidUTF8(s string) bool {
	return utf8.ValidString(s)
}

// =============================================================================
// UNICODE NFC NORMALIZATION TESTS
// =============================================================================
// These tests verify that SignDoc properly validates Unicode NFC normalization
// to prevent signature mismatches caused by different Unicode representations
// of visually identical strings.
//
// SECURITY RATIONALE:
// Two strings that look identical can have different Unicode representations:
// - Composed (NFC): "café" using U+00E9 (LATIN SMALL LETTER E WITH ACUTE)
// - Decomposed (NFD): "café" using U+0065 + U+0301 (e + COMBINING ACUTE ACCENT)
// These produce different JSON bytes → different signatures for 'same' content.
//
// By validating NFC normalization and rejecting non-NFC input, we ensure
// consistent signatures across implementations.

// =============================================================================
// TEST VECTORS: NFC vs NFD representations
// =============================================================================

// testNFCVector holds test cases for NFC normalization
type testNFCVector struct {
	name        string
	nfc         string // NFC-normalized form (should pass)
	nfd         string // NFD form (should fail)
	description string
}

// Common Unicode normalization test vectors
var nfcTestVectors = []testNFCVector{
	{
		name:        "Latin e with acute (é)",
		nfc:         "caf\u00e9",  // café with composed é (U+00E9)
		nfd:         "cafe\u0301", // café with e + combining acute (U+0065 U+0301)
		description: "Classic example: composed vs decomposed accented character",
	},
	{
		name:        "Latin n with tilde (ñ)",
		nfc:         "\u00f1",  // ñ composed (U+00F1)
		nfd:         "n\u0303", // n + combining tilde (U+006E U+0303)
		description: "Spanish ñ: composed vs decomposed",
	},
	{
		name:        "Greek omicron with tonos (ό)",
		nfc:         "\u03cc",       // ό composed (U+03CC)
		nfd:         "\u03bf\u0301", // ο + combining acute (U+03BF U+0301)
		description: "Greek accented vowel",
	},
	{
		name:        "Hangul syllable (가)",
		nfc:         "\uac00",       // 가 composed (U+AC00)
		nfd:         "\u1100\u1161", // ᄀ + ᅡ decomposed (U+1100 U+1161)
		description: "Korean Hangul syllable",
	},
	{
		name:        "A with ring above (Å)",
		nfc:         "\u00c5",  // Å composed (U+00C5)
		nfd:         "A\u030a", // A + combining ring above (U+0041 U+030A)
		description: "Scandinavian Å",
	},
	{
		name:        "O with umlaut (Ö)",
		nfc:         "\u00d6",  // Ö composed (U+00D6)
		nfd:         "O\u0308", // O + combining diaeresis (U+004F U+0308)
		description: "German umlaut",
	},
	{
		name:        "Multiple combining marks",
		nfc:         "\u1e69",        // ṩ composed (U+1E69) - s with dot below and dot above
		nfd:         "s\u0323\u0307", // s + combining dot below + combining dot above
		description: "Character with multiple combining marks",
	},
}

// =============================================================================
// UNIT TESTS: isNFCNormalized helper function
// =============================================================================

func TestIsNFCNormalized(t *testing.T) {
	t.Run("ASCII strings are always NFC", func(t *testing.T) {
		asciiStrings := []string{
			"",
			"hello",
			"HelloWorld123",
			"!@#$%^&*()",
			"chain-id-1",
			"/punnet.bank.v1.MsgSend",
		}
		for _, s := range asciiStrings {
			assert.True(t, isNFCNormalized(s), "ASCII string %q should be NFC", s)
		}
	})

	t.Run("NFC-normalized strings pass", func(t *testing.T) {
		for _, tc := range nfcTestVectors {
			t.Run(tc.name, func(t *testing.T) {
				assert.True(t, isNFCNormalized(tc.nfc),
					"%s: NFC form %q should pass", tc.description, tc.nfc)
			})
		}
	})

	t.Run("Non-NFC strings fail", func(t *testing.T) {
		for _, tc := range nfcTestVectors {
			t.Run(tc.name, func(t *testing.T) {
				// Skip if NFC and NFD are identical (rare edge cases)
				if tc.nfc == tc.nfd {
					t.Skip("NFC and NFD forms are identical for this case")
				}
				assert.False(t, isNFCNormalized(tc.nfd),
					"%s: NFD form %q should fail", tc.description, tc.nfd)
			})
		}
	})

	t.Run("Emojis are NFC-safe", func(t *testing.T) {
		emojis := []string{
			"🚀",
			"💰",
			"🔐",
			"Hello 🌍 World",
			"🎉🎊🎈",
		}
		for _, s := range emojis {
			assert.True(t, isNFCNormalized(s), "Emoji string %q should be NFC", s)
		}
	})

	t.Run("CJK characters are NFC-safe", func(t *testing.T) {
		cjk := []string{
			"日本語",
			"中文",
			"한국어",
			"テスト",
		}
		for _, s := range cjk {
			assert.True(t, isNFCNormalized(s), "CJK string %q should be NFC", s)
		}
	})
}

// =============================================================================
// UNIT TESTS: SignDoc.ValidateBasic NFC validation
// =============================================================================

func TestSignDoc_ValidateBasic_NFCValidation(t *testing.T) {
	// Helper to create a valid base SignDoc
	validSignDoc := func() *SignDoc {
		return &SignDoc{
			Version:         SignDocVersion,
			ChainID:         "test-chain-1",
			Account:         "alice",
			AccountSequence: 1,
			Nonce:           1,
			Memo:            "",
			Messages:        []SignDocMessage{{Type: "/test.Msg", Data: json.RawMessage(`{}`)}},
			Fee:             SignDocFee{Amount: []SignDocCoin{}, GasLimit: "100"},
			FeeSlippage:     SignDocRatio{Numerator: "0", Denominator: "1"},
		}
	}

	t.Run("NFC-normalized ChainID passes", func(t *testing.T) {
		sd := validSignDoc()
		sd.ChainID = "test-\u00e9" // NFC é
		err := sd.ValidateBasic()
		assert.NoError(t, err)
	})

	t.Run("Non-NFC ChainID fails", func(t *testing.T) {
		sd := validSignDoc()
		sd.ChainID = "test-e\u0301" // NFD é (e + combining acute)
		err := sd.ValidateBasic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chain_id")
		assert.Contains(t, err.Error(), "NFC")
	})

	t.Run("NFC-normalized Account passes", func(t *testing.T) {
		sd := validSignDoc()
		sd.Account = "caf\u00e9" // NFC café
		err := sd.ValidateBasic()
		assert.NoError(t, err)
	})

	t.Run("Non-NFC Account fails", func(t *testing.T) {
		sd := validSignDoc()
		sd.Account = "cafe\u0301" // NFD café
		err := sd.ValidateBasic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "account")
		assert.Contains(t, err.Error(), "NFC")
	})

	t.Run("NFC-normalized Memo passes", func(t *testing.T) {
		sd := validSignDoc()
		sd.Memo = "Payment for caf\u00e9" // NFC
		err := sd.ValidateBasic()
		assert.NoError(t, err)
	})

	t.Run("Non-NFC Memo fails", func(t *testing.T) {
		sd := validSignDoc()
		sd.Memo = "Payment for cafe\u0301" // NFD
		err := sd.ValidateBasic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "memo")
		assert.Contains(t, err.Error(), "NFC")
	})

	t.Run("NFC-normalized message type passes", func(t *testing.T) {
		sd := validSignDoc()
		sd.Messages[0].Type = "/test.\u00e9" // NFC
		err := sd.ValidateBasic()
		assert.NoError(t, err)
	})

	t.Run("Non-NFC message type fails", func(t *testing.T) {
		sd := validSignDoc()
		sd.Messages[0].Type = "/test.e\u0301" // NFD
		err := sd.ValidateBasic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "message")
		assert.Contains(t, err.Error(), "NFC")
	})

	t.Run("All test vectors: NFC forms pass", func(t *testing.T) {
		for _, tc := range nfcTestVectors {
			t.Run(tc.name+"-chainid", func(t *testing.T) {
				sd := validSignDoc()
				sd.ChainID = tc.nfc
				err := sd.ValidateBasic()
				assert.NoError(t, err, "NFC form should pass for ChainID")
			})
			t.Run(tc.name+"-account", func(t *testing.T) {
				sd := validSignDoc()
				sd.Account = tc.nfc
				err := sd.ValidateBasic()
				assert.NoError(t, err, "NFC form should pass for Account")
			})
			t.Run(tc.name+"-memo", func(t *testing.T) {
				sd := validSignDoc()
				sd.Memo = tc.nfc
				err := sd.ValidateBasic()
				assert.NoError(t, err, "NFC form should pass for Memo")
			})
		}
	})

	t.Run("All test vectors: NFD forms fail", func(t *testing.T) {
		for _, tc := range nfcTestVectors {
			// Skip if NFC and NFD are identical
			if tc.nfc == tc.nfd {
				continue
			}
			t.Run(tc.name+"-chainid", func(t *testing.T) {
				sd := validSignDoc()
				sd.ChainID = tc.nfd
				err := sd.ValidateBasic()
				require.Error(t, err, "NFD form should fail for ChainID")
				assert.Contains(t, err.Error(), "NFC")
			})
			t.Run(tc.name+"-account", func(t *testing.T) {
				sd := validSignDoc()
				sd.Account = tc.nfd
				err := sd.ValidateBasic()
				require.Error(t, err, "NFD form should fail for Account")
				assert.Contains(t, err.Error(), "NFC")
			})
			t.Run(tc.name+"-memo", func(t *testing.T) {
				sd := validSignDoc()
				sd.Memo = tc.nfd
				err := sd.ValidateBasic()
				require.Error(t, err, "NFD form should fail for Memo")
				assert.Contains(t, err.Error(), "NFC")
			})
		}
	})
}

// =============================================================================
// UNIT TESTS: SignDocCoin.ValidateBasic NFC validation
// =============================================================================

func TestSignDocCoin_ValidateBasic_NFCValidation(t *testing.T) {
	t.Run("NFC-normalized denom passes", func(t *testing.T) {
		coin := SignDocCoin{Denom: "u\u00e9", Amount: "1000"} // NFC é
		err := coin.ValidateBasic()
		assert.NoError(t, err)
	})

	t.Run("Non-NFC denom fails", func(t *testing.T) {
		coin := SignDocCoin{Denom: "ue\u0301", Amount: "1000"} // NFD é
		err := coin.ValidateBasic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "denom")
		assert.Contains(t, err.Error(), "NFC")
	})

	t.Run("ASCII denom passes", func(t *testing.T) {
		coin := SignDocCoin{Denom: "uatom", Amount: "1000"}
		err := coin.ValidateBasic()
		assert.NoError(t, err)
	})
}

// =============================================================================
// SECURITY TESTS: Attack scenarios
// =============================================================================

func TestNFC_AttackScenarios(t *testing.T) {
	t.Run("Visual spoofing attack: account names", func(t *testing.T) {
		// ATTACK: An attacker creates an account name that looks identical to
		// another account but uses different Unicode normalization.
		// This could trick users into signing transactions to the wrong account.

		nfcAccount := "caf\u00e9"  // NFC: café
		nfdAccount := "cafe\u0301" // NFD: café (visually identical)

		// Verify they look the same when printed
		assert.Equal(t, len([]rune(nfcAccount)), len([]rune(nfdAccount))-1,
			"NFD has extra combining character")

		// But our validation should reject the NFD form
		sd := &SignDoc{
			Version:         SignDocVersion,
			ChainID:         "test",
			Account:         nfdAccount, // Attacker's spoofed account
			AccountSequence: 1,
			Nonce:           1,
			Messages:        []SignDocMessage{{Type: "/test", Data: json.RawMessage(`{}`)}},
			Fee:             SignDocFee{Amount: []SignDocCoin{}, GasLimit: "100"},
			FeeSlippage:     SignDocRatio{Numerator: "0", Denominator: "1"},
		}

		err := sd.ValidateBasic()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "NFC")
	})

	t.Run("Cross-implementation signature mismatch prevention", func(t *testing.T) {
		// ATTACK: A transaction signed with one normalization form might
		// produce a different signature than the same visual content in
		// another form, leading to verification failures or replay attacks.

		nfcMemo := "Payment to caf\u00e9"
		nfdMemo := "Payment to cafe\u0301"

		// Both create valid-looking SignDocs
		sdNFC := NewSignDoc("test", 1, "alice", 1, nfcMemo)
		sdNFC.AddMessage("/test", json.RawMessage(`{}`))

		sdNFD := NewSignDoc("test", 1, "alice", 1, nfdMemo)
		sdNFD.AddMessage("/test", json.RawMessage(`{}`))

		// NFC version should validate
		err := sdNFC.ValidateBasic()
		assert.NoError(t, err, "NFC form should pass validation")

		// NFD version should be rejected
		err = sdNFD.ValidateBasic()
		require.Error(t, err, "NFD form should be rejected")
		assert.Contains(t, err.Error(), "NFC")

		// This prevents the attack: only one form is valid,
		// so there can be no signature mismatch
	})

	t.Run("Zero-width characters are NFC-safe", func(t *testing.T) {
		// Zero-width characters (like ZWSP U+200B) can be used to create
		// visually identical but different strings. While NFC doesn't remove
		// them, we should ensure they don't break our validation.

		memo := "hello\u200Bworld" // Zero-width space
		sd := NewSignDoc("test", 1, "alice", 1, memo)
		sd.AddMessage("/test", json.RawMessage(`{}`))

		// Should pass NFC validation (ZWSP is already in NFC form)
		err := sd.ValidateBasic()
		assert.NoError(t, err)
	})
}

// =============================================================================
// FUZZ TESTS: Unicode NFC validation
// =============================================================================

func FuzzNFCValidation(f *testing.F) {
	// Seed with NFC test vectors
	for _, tc := range nfcTestVectors {
		f.Add(tc.nfc)
		f.Add(tc.nfd)
	}

	// Seed with other interesting strings
	f.Add("")
	f.Add("hello")
	f.Add("日本語")
	f.Add("🚀💰🔐")
	f.Add("\u0000")              // Null
	f.Add("\u200B")              // Zero-width space
	f.Add("\uFEFF")              // BOM
	f.Add("a\u0300\u0301\u0302") // Multiple combining marks
	f.Add("\u1E69")              // Precomposed with multiple marks

	f.Fuzz(func(t *testing.T, input string) {
		// INVARIANT: isNFCNormalized should match norm.NFC.IsNormalString
		expected := norm.NFC.IsNormalString(input)
		actual := isNFCNormalized(input)

		if expected != actual {
			t.Errorf("isNFCNormalized(%q) = %v, want %v", input, actual, expected)
		}

		// INVARIANT: If we normalize to NFC, it should always pass validation
		normalized := norm.NFC.String(input)
		if !isNFCNormalized(normalized) {
			t.Errorf("NFC-normalized string %q failed isNFCNormalized", normalized)
		}

		// INVARIANT: ValidateBasic should reject non-NFC strings in string fields
		sd := &SignDoc{
			Version:         SignDocVersion,
			ChainID:         "test",
			Account:         "alice",
			AccountSequence: 1,
			Nonce:           1,
			Memo:            input, // Test with fuzzed input
			Messages:        []SignDocMessage{{Type: "/test", Data: json.RawMessage(`{}`)}},
			Fee:             SignDocFee{Amount: []SignDocCoin{}, GasLimit: "100"},
			FeeSlippage:     SignDocRatio{Numerator: "0", Denominator: "1"},
		}

		err := sd.ValidateBasic()

		// If input is NFC, validation should pass (or fail for other reasons)
		// If input is not NFC, validation should fail with NFC error
		if !expected && err == nil {
			t.Errorf("non-NFC memo %q should have failed validation", input)
		}
	})
}

func FuzzNFCSignDocRoundtrip(f *testing.F) {
	// Seed with various Unicode strings
	f.Add("hello", "world", "test")
	f.Add("caf\u00e9", "alice", "payment")
	f.Add("日本語", "中文", "한국어")
	f.Add("🚀", "💰", "🔐")

	f.Fuzz(func(t *testing.T, chainID, account, memo string) {
		// Skip invalid UTF-8 (JSON encoding will mangle it)
		if !isValidUTF8(chainID) || !isValidUTF8(account) || !isValidUTF8(memo) {
			return
		}

		// Normalize all inputs to NFC
		chainID = norm.NFC.String(chainID)
		account = norm.NFC.String(account)
		memo = norm.NFC.String(memo)

		// Skip if any field is empty (invalid for other reasons)
		if chainID == "" || account == "" {
			return
		}

		sd := NewSignDoc(chainID, 1, account, 1, memo)
		sd.AddMessage("/test", json.RawMessage(`{}`))

		// NFC-normalized input should always pass validation
		err := sd.ValidateBasic()
		if err != nil {
			t.Errorf("NFC-normalized SignDoc failed validation: %v", err)
			return
		}

		// Serialization should succeed
		jsonBytes, err := sd.ToJSON()
		if err != nil {
			t.Errorf("ToJSON failed: %v", err)
			return
		}

		// Roundtrip should preserve all fields exactly
		sd2, err := ParseSignDoc(jsonBytes)
		if err != nil {
			t.Errorf("ParseSignDoc failed: %v", err)
			return
		}

		if sd2.ChainID != chainID {
			t.Errorf("ChainID changed: %q -> %q", chainID, sd2.ChainID)
		}
		if sd2.Account != account {
			t.Errorf("Account changed: %q -> %q", account, sd2.Account)
		}
		if sd2.Memo != memo {
			t.Errorf("Memo changed: %q -> %q", memo, sd2.Memo)
		}
	})
}

// =============================================================================
// BENCHMARK: NFC validation overhead
// =============================================================================

func BenchmarkIsNFCNormalized(b *testing.B) {
	benchmarks := []struct {
		name string
		s    string
	}{
		{"ASCII-short", "hello"},
		{"ASCII-long", "hello world this is a longer string for testing"},
		{"NFC-short", "caf\u00e9"},
		{"NFC-long", "This is a café with a niño eating jalapeño"},
		{"NFD-short", "cafe\u0301"},
		{"CJK", "日本語テスト"},
		{"Emoji", "🚀💰🔐🎉🌍"},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = isNFCNormalized(bm.s)
			}
		})
	}
}

func BenchmarkSignDocValidateBasic_WithNFC(b *testing.B) {
	sd := &SignDoc{
		Version:         SignDocVersion,
		ChainID:         "test-chain-1",
		Account:         "alice",
		AccountSequence: 1,
		Nonce:           1,
		Memo:            "Payment for café",
		Messages:        []SignDocMessage{{Type: "/punnet.bank.v1.MsgSend", Data: json.RawMessage(`{"from":"alice","to":"bob","amount":"1000"}`)}},
		Fee:             SignDocFee{Amount: []SignDocCoin{{Denom: "uatom", Amount: "5000"}}, GasLimit: "200000"},
		FeeSlippage:     SignDocRatio{Numerator: "1", Denominator: "100"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sd.ValidateBasic()
	}
}
