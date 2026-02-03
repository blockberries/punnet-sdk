package crypto

import (
	"crypto/ed25519"
	"strings"
	"testing"
	"unicode/utf8"
)

// =============================================================================
// FUZZ TESTS FOR KEYRING KEY NAME VALIDATION
// =============================================================================
// These fuzz tests target security-critical key name validation operations.
// Goal: Discover panics, path traversal bypasses, and edge cases.
//
// Run with: go test -fuzz=FuzzValidateKeyNameSimple -fuzztime=60s ./crypto/...
//
// SECURITY RATIONALE:
// Key name validation is security-critical because:
// 1. Invalid names could enable path traversal attacks in file-based backends
// 2. Control characters or null bytes could corrupt storage
// 3. Overly long names could cause DoS via resource exhaustion

// =============================================================================
// FUZZ TEST: validateKeyNameSimple (used by Keyring)
// =============================================================================

func FuzzValidateKeyNameSimple(f *testing.F) {
	// Seed corpus: valid names
	f.Add("mykey")
	f.Add("key-with-dashes")
	f.Add("key_with_underscores")
	f.Add("key.with.dots")
	f.Add("key123")
	f.Add("UPPERCASE")
	f.Add("MixedCase")
	f.Add(strings.Repeat("a", 256)) // At max length

	// Seed corpus: invalid names (should be rejected)
	f.Add("")                       // Empty
	f.Add(strings.Repeat("a", 257)) // Over max length
	f.Add("../etc/passwd")          // Path traversal
	f.Add("..\\windows\\system32")  // Windows path traversal
	f.Add("/absolute/path")         // Absolute path
	f.Add("\\absolute\\windows")    // Windows absolute
	f.Add("key/with/slash")         // Embedded slash
	f.Add("key\\with\\backslash")   // Embedded backslash
	f.Add("key\x00null")            // Embedded null byte
	f.Add("key\nnewline")           // Embedded newline
	f.Add("key\ttab")               // Embedded tab
	f.Add("key\rcarriage")          // Embedded carriage return
	f.Add("\x00")                   // Just null
	f.Add("\n")                     // Just newline
	f.Add("\x01\x02\x03")           // Control characters only
	f.Add("key\x1fcontrol")         // Control char at boundary (0x1F)
	f.Add("key\x1a")                // SUB character (Ctrl+Z)
	f.Add("key\x7f")                // DEL character

	// Unicode edge cases
	f.Add("日本語")             // Japanese
	f.Add("🔐🔑")              // Emoji
	f.Add("مرحبا")           // Arabic (RTL)
	f.Add("e\u0301")         // Combining character (é)
	f.Add("\u200B")          // Zero-width space
	f.Add("\u200D")          // Zero-width joiner
	f.Add("\u202E")          // Right-to-left override
	f.Add("\uFEFF")          // BOM
	f.Add("a\xc0\xc1")       // Invalid UTF-8
	f.Add("\xff\xfe")        // Invalid UTF-8 (BOM-like)
	f.Add("key\u0000suffix") // Unicode null

	// Path traversal variants
	f.Add("..")
	f.Add("...")
	f.Add("..%2f")         // URL-encoded
	f.Add("..%5c")         // URL-encoded backslash
	f.Add("..%00")         // Null-terminated
	f.Add("..\x00")        // Literal null after ..
	f.Add("....//")        // Double traversal
	f.Add("..\\..\\")      // Windows double traversal
	f.Add("key/../secret") // Embedded traversal
	f.Add("key/./current") // Current dir reference
	f.Add("~")             // Home directory
	f.Add("~root")         // User home directory
	f.Add("$HOME")         // Environment variable
	f.Add("${HOME}")       // Environment variable expansion

	// Long strings at various boundaries
	f.Add(strings.Repeat("a", 255))  // One under max
	f.Add(strings.Repeat("a", 256))  // At max
	f.Add(strings.Repeat("a", 1000)) // Well over max
	f.Add(strings.Repeat(".", 300))  // Many dots
	f.Add(strings.Repeat("/", 100))  // Many slashes

	f.Fuzz(func(t *testing.T, name string) {
		// SECURITY INVARIANT: validateKeyNameSimple must never panic
		err := validateKeyNameSimple(name)

		if err != nil {
			// Validation failed - verify expected rejections
			return
		}

		// If validation passed, verify security invariants

		// INVARIANT: Empty names must be rejected
		if name == "" {
			t.Error("empty name should have been rejected")
		}

		// INVARIANT: Names over MaxKeyNameLength must be rejected
		if len(name) > MaxKeyNameLength {
			t.Errorf("name of length %d should have been rejected (max %d)", len(name), MaxKeyNameLength)
		}

		// INVARIANT: Path separators must be rejected
		if strings.Contains(name, "/") {
			t.Error("name containing '/' should have been rejected")
		}
		if strings.Contains(name, "\\") {
			t.Error("name containing '\\' should have been rejected")
		}

		// INVARIANT: Null bytes must be rejected
		if strings.Contains(name, "\x00") {
			t.Error("name containing null byte should have been rejected")
		}

		// INVARIANT: Control characters (< 32) must be rejected
		for _, r := range name {
			if r < 32 {
				t.Errorf("name containing control character %d should have been rejected", r)
				break
			}
		}
	})
}

// =============================================================================
// FUZZ TEST: validateKeyName (used by FileKeyStore)
// =============================================================================

func FuzzValidateKeyName(f *testing.F) {
	// Same seed corpus as validateKeyNameSimple
	f.Add("mykey")
	f.Add("key-with-dashes")
	f.Add("key_with_underscores")
	f.Add("key.with.dots")
	f.Add(strings.Repeat("a", 255)) // At max for file systems

	// Invalid names
	f.Add("")
	f.Add(strings.Repeat("a", 256)) // Over max (255 for filesystem)
	f.Add("../etc/passwd")
	f.Add("..\\windows\\system32")
	f.Add("/absolute/path")
	f.Add("key/slash")
	f.Add("key\\backslash")
	f.Add(".hidden")  // Hidden file
	f.Add("..hidden") // Path traversal attempt
	f.Add("..file")   // Path traversal-like
	f.Add("...")      // Triple dot
	f.Add("..")       // Double dot
	f.Add(".")        // Single dot (current dir)
	f.Add("a..")      // Ends with ..
	f.Add("..a")      // Starts with ..

	f.Fuzz(func(t *testing.T, name string) {
		// SECURITY INVARIANT: validateKeyName must never panic
		err := validateKeyName(name)

		if err != nil {
			// Validation failed - expected for invalid input
			return
		}

		// If validation passed, verify security invariants

		// INVARIANT: Empty names must be rejected
		if name == "" {
			t.Error("empty name should have been rejected")
		}

		// INVARIANT: Names over 255 chars must be rejected
		if len(name) > 255 {
			t.Errorf("name of length %d should have been rejected (max 255)", len(name))
		}

		// INVARIANT: Path separators must be rejected
		if strings.Contains(name, "/") || strings.Contains(name, "\\") {
			t.Error("name containing path separator should have been rejected")
		}

		// INVARIANT: Path traversal sequences must be rejected
		if strings.Contains(name, "..") {
			t.Error("name containing '..' should have been rejected")
		}

		// INVARIANT: Hidden files (starting with .) must be rejected
		if strings.HasPrefix(name, ".") {
			t.Error("name starting with '.' should have been rejected")
		}
	})
}

// =============================================================================
// FUZZ TEST: ImportKey with malformed key data
// =============================================================================

func FuzzImportKey(f *testing.F) {
	// Generate a real valid Ed25519 private key for seeding
	_, realPrivKey, _ := ed25519.GenerateKey(nil)
	f.Add("validkey", []byte(realPrivKey))

	// Invalid sizes
	f.Add("short", []byte{1, 2, 3})
	f.Add("empty", []byte{})
	f.Add("toosmall", make([]byte, 63))
	f.Add("toolarge", make([]byte, 65))
	f.Add("way-too-large", make([]byte, 1000))

	// Edge case sizes around 64 bytes
	f.Add("size63", make([]byte, 63))
	f.Add("size65", make([]byte, 65))

	// Names with special characters (using real valid key)
	f.Add("name-with-dash", []byte(realPrivKey))
	f.Add("name_with_underscore", []byte(realPrivKey))
	f.Add("Name.With.Dots", []byte(realPrivKey))
	f.Add("../traversal", []byte(realPrivKey))
	f.Add("key\x00null", []byte(realPrivKey))
	f.Add("", []byte(realPrivKey)) // Empty name

	f.Fuzz(func(t *testing.T, name string, keyData []byte) {
		store := NewMemoryStore()
		kr := NewKeyring(store)

		// SECURITY INVARIANT: ImportKey must never panic
		signer, err := kr.ImportKey(name, keyData, AlgorithmEd25519)

		if err != nil {
			// Import failed - expected for invalid input
			// Verify we got a proper error, not a panic
			return
		}

		// If import succeeded, verify invariants

		// INVARIANT: Only valid key data should succeed
		if len(keyData) != ed25519.PrivateKeySize {
			t.Errorf("ImportKey succeeded with key data of length %d (expected %d)",
				len(keyData), ed25519.PrivateKeySize)
		}

		// INVARIANT: Only valid names should succeed
		if name == "" {
			t.Error("ImportKey succeeded with empty name")
		}
		if strings.Contains(name, "/") || strings.Contains(name, "\\") {
			t.Error("ImportKey succeeded with path separator in name")
		}
		if strings.Contains(name, "\x00") {
			t.Error("ImportKey succeeded with null byte in name")
		}

		// INVARIANT: Signer must be usable
		if signer == nil {
			t.Error("ImportKey returned nil signer without error")
			return
		}

		// INVARIANT: Signer should sign without panicking
		testData := []byte("test message")
		sig, err := signer.Sign(testData)
		if err != nil {
			t.Errorf("Signer.Sign failed: %v", err)
			return
		}

		// Note: Ed25519 accepts any 64-byte value as a private key, but
		// the resulting signatures may not verify if the key wasn't
		// generated properly. The important security invariant is that
		// Sign() doesn't panic, not that arbitrary bytes produce valid
		// signatures. For proper signature verification tests, use keys
		// generated via ed25519.GenerateKey().
		_ = sig // Signature created successfully (no panic)
	})
}

// =============================================================================
// FUZZ TEST: Key name with Unicode edge cases
// =============================================================================

func FuzzKeyNameUnicode(f *testing.F) {
	// Various Unicode strings
	f.Add("hello")
	f.Add("日本語")
	f.Add("🔐🔑💰")
	f.Add("مرحبا")                  // Arabic
	f.Add("שלום")                   // Hebrew
	f.Add("Ελληνικά")               // Greek
	f.Add("e\u0301")                // e + combining acute
	f.Add("é")                      // Precomposed é
	f.Add("\u200B")                 // Zero-width space
	f.Add("\u200D")                 // Zero-width joiner
	f.Add("\u202E")                 // RTL override
	f.Add("\uFEFF")                 // BOM
	f.Add("\uFFFD")                 // Replacement character
	f.Add(strings.Repeat("🎉", 100)) // Many emoji

	// Mixed scripts
	f.Add("hello日本語world")
	f.Add("مرحباhello")

	// Normalization forms
	f.Add("A\u0308") // A + combining umlaut (NFD)
	f.Add("\u00C4")  // Precomposed Ä (NFC)
	f.Add("ﬁ")       // fi ligature
	f.Add("ﬀ")       // ff ligature

	f.Fuzz(func(t *testing.T, name string) {
		store := NewMemoryStore()
		kr := NewKeyring(store)

		// SECURITY INVARIANT: Never panic on any Unicode input
		signer, err := kr.NewKey(name, AlgorithmEd25519)

		if err != nil {
			// Creation failed - expected for invalid names
			return
		}

		// If key creation succeeded, verify roundtrip
		retrieved, err := kr.GetKey(name)
		if err != nil {
			// This is a bug - we just created it!
			t.Errorf("GetKey failed for key we just created: %v", err)
			return
		}

		// INVARIANT: Public keys must match
		if !signer.PublicKey().Equals(retrieved.PublicKey()) {
			t.Error("PublicKey mismatch after GetKey")
		}

		// INVARIANT: Key should be listable
		keys, err := kr.ListKeys()
		if err != nil {
			t.Errorf("ListKeys failed: %v", err)
			return
		}

		found := false
		for _, k := range keys {
			if k == name {
				found = true
				break
			}
		}
		if !found {
			// Only fail for valid UTF-8 - invalid UTF-8 might be normalized
			if utf8.ValidString(name) {
				t.Errorf("Created key %q not found in ListKeys", name)
			}
		}

		// Clean up
		_ = kr.DeleteKey(name)
	})
}

// =============================================================================
// FUZZ TEST: Sign data boundaries
// =============================================================================

func FuzzSignDataBoundaries(f *testing.F) {
	// Various data sizes
	f.Add(0)
	f.Add(1)
	f.Add(64)
	f.Add(1024)
	f.Add(65536)
	f.Add(MaxSignDataLength)
	f.Add(MaxSignDataLength - 1)
	f.Add(MaxSignDataLength + 1)

	f.Fuzz(func(t *testing.T, dataSize int) {
		// Bound to prevent OOM
		if dataSize < 0 || dataSize > MaxSignDataLength+1024 {
			return
		}

		store := NewMemoryStore()
		kr := NewKeyring(store)

		_, err := kr.NewKey("test", AlgorithmEd25519)
		if err != nil {
			t.Fatalf("NewKey failed: %v", err)
		}

		data := make([]byte, dataSize)

		// SECURITY INVARIANT: Sign must not panic
		sig, err := kr.Sign("test", data)

		if dataSize > MaxSignDataLength {
			// INVARIANT: Oversized data must be rejected
			if err != ErrDataTooLarge {
				t.Errorf("Sign with %d bytes should return ErrDataTooLarge, got: %v",
					dataSize, err)
			}
			return
		}

		// Data within limits should succeed
		if err != nil {
			t.Errorf("Sign with %d bytes failed: %v", dataSize, err)
			return
		}

		// INVARIANT: Signature must be valid
		signer, _ := kr.GetKey("test")
		if !signer.PublicKey().Verify(data, sig) {
			t.Errorf("Signature verification failed for data of size %d", dataSize)
		}
	})
}

// =============================================================================
// FUZZ TEST: PrivateKeyFromBytes
// =============================================================================

func FuzzPrivateKeyFromBytes(f *testing.F) {
	// Generate a real valid Ed25519 private key for seeding
	_, realPrivKey, _ := ed25519.GenerateKey(nil)
	f.Add([]byte(realPrivKey))

	// Invalid sizes
	f.Add([]byte{})
	f.Add([]byte{1})
	f.Add(make([]byte, 32))  // Public key size
	f.Add(make([]byte, 63))  // One short
	f.Add(make([]byte, 65))  // One over
	f.Add(make([]byte, 128)) // Double

	f.Fuzz(func(t *testing.T, data []byte) {
		// SECURITY INVARIANT: Must never panic
		privKey, err := PrivateKeyFromBytes(AlgorithmEd25519, data)

		if err != nil {
			// Expected for invalid input
			return
		}

		// INVARIANT: Only 64-byte keys should succeed
		if len(data) != ed25519.PrivateKeySize {
			t.Errorf("PrivateKeyFromBytes succeeded with %d bytes (expected %d)",
				len(data), ed25519.PrivateKeySize)
		}

		// INVARIANT: Sign should not panic
		testData := []byte("test")
		sig, err := privKey.Sign(testData)
		if err != nil {
			t.Errorf("Sign failed: %v", err)
			return
		}

		// Note: Ed25519 accepts any 64-byte value as a private key, but
		// signatures from arbitrarily constructed keys may not verify.
		// The important security invariant is that Sign() doesn't panic.
		_ = sig // Signature created successfully (no panic)

		// INVARIANT: Zeroize should not panic
		privKey.Zeroize()
	})
}

// =============================================================================
// FUZZ TEST: Concurrent operations
// =============================================================================

func FuzzKeyringConcurrent(f *testing.F) {
	f.Add("key1", "key2", 10)
	f.Add("a", "b", 50)
	f.Add("same", "same", 20)

	f.Fuzz(func(t *testing.T, name1, name2 string, iterations int) {
		// Bound iterations to prevent timeout
		if iterations < 1 || iterations > 100 {
			return
		}

		// Skip if names are invalid
		if err := validateKeyNameSimple(name1); err != nil {
			return
		}
		if err := validateKeyNameSimple(name2); err != nil {
			return
		}

		store := NewMemoryStore()
		kr := NewKeyring(store)

		// Pre-create keys
		_, _ = kr.NewKey(name1, AlgorithmEd25519)
		_, _ = kr.NewKey(name2, AlgorithmEd25519)

		done := make(chan bool, iterations*4)

		// SECURITY INVARIANT: Concurrent operations must not panic
		for i := 0; i < iterations; i++ {
			go func() {
				defer func() { done <- true }()
				_, _ = kr.GetKey(name1)
			}()
			go func() {
				defer func() { done <- true }()
				_, _ = kr.GetKey(name2)
			}()
			go func() {
				defer func() { done <- true }()
				_, _ = kr.Sign(name1, []byte("data"))
			}()
			go func() {
				defer func() { done <- true }()
				_, _ = kr.ListKeys()
			}()
		}

		// Wait for all goroutines
		for i := 0; i < iterations*4; i++ {
			<-done
		}
	})
}
