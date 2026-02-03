package vectors

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/blockberries/punnet-sdk/types"
)

// WellKnownTestKeys contains deterministic test keys for reproducible test vectors.
// SECURITY: These keys are for testing ONLY. Never use in production.
//
// The keys are derived from well-known seeds to ensure cross-implementation reproducibility.
var WellKnownTestKeys = struct {
	Ed25519 struct {
		// Seed is 32 bytes of deterministic test data
		Seed       []byte
		PrivateKey ed25519.PrivateKey
		PublicKey  ed25519.PublicKey
	}
	// Secp256k1 and Secp256r1 would be added when implemented
}{
	Ed25519: func() struct {
		Seed       []byte
		PrivateKey ed25519.PrivateKey
		PublicKey  ed25519.PublicKey
	} {
		// Deterministic seed: SHA-256("punnet-sdk-test-vector-seed-ed25519")
		seedHash := sha256.Sum256([]byte("punnet-sdk-test-vector-seed-ed25519"))
		seed := seedHash[:]
		privateKey := ed25519.NewKeyFromSeed(seed)
		publicKey := privateKey.Public().(ed25519.PublicKey)
		return struct {
			Seed       []byte
			PrivateKey ed25519.PrivateKey
			PublicKey  ed25519.PublicKey
		}{
			Seed:       seed,
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		}
	}(),
}

// mustJSON serializes the SignDoc to JSON, panicking on error.
// Used in test vector generation where errors indicate bugs.
func mustJSON(signDoc *types.SignDoc) []byte {
	json, err := signDoc.ToJSON()
	if err != nil {
		panic("failed to serialize SignDoc to JSON: " + err.Error())
	}
	return json
}

// mustSignBytes computes the sign bytes, panicking on error.
// Used in test vector generation where errors indicate bugs.
func mustSignBytes(signDoc *types.SignDoc) []byte {
	bytes, err := signDoc.GetSignBytes()
	if err != nil {
		panic("failed to compute sign bytes: " + err.Error())
	}
	return bytes
}

// GenerateTestVectors creates the complete test vector file.
func GenerateTestVectors() (*TestVectorFile, error) {
	vectors := []TestVector{}

	// Add serialization vectors
	serializationVectors := generateSerializationVectors()
	vectors = append(vectors, serializationVectors...)

	// Add algorithm vectors
	algorithmVectors := generateAlgorithmVectors()
	vectors = append(vectors, algorithmVectors...)

	// Add edge case vectors
	edgeCaseVectors := generateEdgeCaseVectors()
	vectors = append(vectors, edgeCaseVectors...)

	return &TestVectorFile{
		Version:     "1.0",
		Generated:   time.Now().UTC(),
		Description: "Cross-implementation test vectors for Punnet SDK signing system",
		Vectors:     vectors,
	}, nil
}

// generateSerializationVectors creates test vectors for JSON serialization.
func generateSerializationVectors() []TestVector {
	vectors := []TestVector{}

	// 1. Simple single-message transaction
	vectors = append(vectors, generateSimpleSendVector())

	// 2. Multi-message transaction
	vectors = append(vectors, generateMultiMessageVector())

	// 3. Transaction with memo
	vectors = append(vectors, generateMemoVector())

	// 4. Transaction with fees
	vectors = append(vectors, generateFeesVector())

	// 5. Transaction with multiple fee coins
	vectors = append(vectors, generateMultipleFeeCoinsVector())

	return vectors
}

// generateAlgorithmVectors creates test vectors for cryptographic algorithms.
func generateAlgorithmVectors() []TestVector {
	vectors := []TestVector{}

	// Ed25519 key derivation and signing
	vectors = append(vectors, generateEd25519KeyDerivationVector())
	vectors = append(vectors, generateEd25519SigningVector())

	// Note: secp256k1 and secp256r1 vectors would be added when implemented
	// vectors = append(vectors, generateSecp256k1KeyDerivationVector())
	// vectors = append(vectors, generateSecp256r1KeyDerivationVector())

	return vectors
}

// generateEdgeCaseVectors creates test vectors for edge cases.
func generateEdgeCaseVectors() []TestVector {
	vectors := []TestVector{}

	// Empty memo
	vectors = append(vectors, generateEmptyMemoVector())

	// Zero values
	vectors = append(vectors, generateZeroValuesVector())

	// Large sequence numbers (uint64 boundary)
	vectors = append(vectors, generateLargeNumbersVector())

	// Unicode in memo
	vectors = append(vectors, generateUnicodeVector())

	// Special characters in various fields
	vectors = append(vectors, generateSpecialCharsVector())

	// Minimum valid transaction
	vectors = append(vectors, generateMinimalVector())

	return vectors
}

func generateSimpleSendVector() TestVector {
	input := TestVectorInput{
		ChainID:         "punnet-mainnet-1",
		Account:         "alice",
		AccountSequence: "42",
		Nonce:           "42",
		Memo:            "",
		Messages: []TestVectorMessage{
			{
				Type: "/punnet.bank.v1.MsgSend",
				Data: json.RawMessage(`{"from":"alice","to":"bob","amount":"1000000"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{{Denom: "stake", Amount: "5000"}},
			GasLimit: "200000",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "1",
			Denominator: "100",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)

	// Generate Ed25519 signature
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "simple_send",
		Description: "Simple single-message MsgSend transaction",
		Category:    "serialization",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateMultiMessageVector() TestVector {
	input := TestVectorInput{
		ChainID:         "punnet-mainnet-1",
		Account:         "alice",
		AccountSequence: "10",
		Nonce:           "10",
		Memo:            "",
		Messages: []TestVectorMessage{
			{
				Type: "/punnet.bank.v1.MsgSend",
				Data: json.RawMessage(`{"from":"alice","to":"bob","amount":"500000"}`),
			},
			{
				Type: "/punnet.bank.v1.MsgSend",
				Data: json.RawMessage(`{"from":"alice","to":"charlie","amount":"300000"}`),
			},
			{
				Type: "/punnet.staking.v1.MsgDelegate",
				Data: json.RawMessage(`{"delegator":"alice","validator":"val1","amount":"200000"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{{Denom: "stake", Amount: "10000"}},
			GasLimit: "500000",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "5",
			Denominator: "100",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "multi_message",
		Description: "Transaction with multiple messages of different types",
		Category:    "serialization",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateMemoVector() TestVector {
	input := TestVectorInput{
		ChainID:         "punnet-testnet-1",
		Account:         "sender",
		AccountSequence: "100",
		Nonce:           "100",
		Memo:            "Payment for services rendered - Invoice #12345",
		Messages: []TestVectorMessage{
			{
				Type: "/punnet.bank.v1.MsgSend",
				Data: json.RawMessage(`{"from":"sender","to":"recipient","amount":"1000000"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{{Denom: "uatom", Amount: "2500"}},
			GasLimit: "100000",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "0",
			Denominator: "1",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "with_memo",
		Description: "Transaction with a memo field",
		Category:    "serialization",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateFeesVector() TestVector {
	input := TestVectorInput{
		ChainID:         "punnet-mainnet-1",
		Account:         "alice",
		AccountSequence: "5",
		Nonce:           "5",
		Memo:            "",
		Messages: []TestVectorMessage{
			{
				Type: "/punnet.bank.v1.MsgSend",
				Data: json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{{Denom: "stake", Amount: "1000000"}},
			GasLimit: "1000000",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "10",
			Denominator: "100",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "with_fees",
		Description: "Transaction with significant gas limit and fee slippage",
		Category:    "serialization",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateMultipleFeeCoinsVector() TestVector {
	input := TestVectorInput{
		ChainID:         "punnet-mainnet-1",
		Account:         "alice",
		AccountSequence: "20",
		Nonce:           "20",
		Memo:            "",
		Messages: []TestVectorMessage{
			{
				Type: "/punnet.bank.v1.MsgSend",
				Data: json.RawMessage(`{"from":"alice","to":"bob","amount":"1000"}`),
			},
		},
		Fee: TestVectorFee{
			Amount: []TestVectorCoin{
				{Denom: "stake", Amount: "5000"},
				{Denom: "uatom", Amount: "3000"},
				{Denom: "token", Amount: "1000"},
			},
			GasLimit: "300000",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "2",
			Denominator: "100",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "multiple_fee_coins",
		Description: "Transaction with multiple fee coins",
		Category:    "serialization",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateEd25519KeyDerivationVector() TestVector {
	// This vector tests that key derivation from seed produces expected results
	input := TestVectorInput{
		ChainID:         "key-derivation-test",
		Account:         "test",
		AccountSequence: "0",
		Nonce:           "0",
		Memo:            "",
		Messages: []TestVectorMessage{
			{
				Type: "/test.KeyDerivation",
				Data: json.RawMessage(`{}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{},
			GasLimit: "0",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "0",
			Denominator: "1",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	// Document the seed derivation process
	seedHash := sha256.Sum256([]byte("punnet-sdk-test-vector-seed-ed25519"))

	return TestVector{
		Name:        "ed25519_key_derivation",
		Description: "Ed25519 key derivation from deterministic seed: SHA-256(\"punnet-sdk-test-vector-seed-ed25519\")",
		Category:    "algorithm",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
				"ed25519_seed": {
					PrivateKeyHex: hex.EncodeToString(seedHash[:]),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  "", // Seed is not for signing directly
				},
			},
		},
	}
}

func generateEd25519SigningVector() TestVector {
	// This vector tests that signing a known message produces expected signature
	input := TestVectorInput{
		ChainID:         "signing-test",
		Account:         "signer",
		AccountSequence: "1",
		Nonce:           "1",
		Memo:            "Sign this message",
		Messages: []TestVectorMessage{
			{
				Type: "/test.SignMe",
				Data: json.RawMessage(`{"content":"test data for signing"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{{Denom: "stake", Amount: "100"}},
			GasLimit: "50000",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "0",
			Denominator: "1",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "ed25519_signing",
		Description: "Ed25519 signature generation for a known message",
		Category:    "algorithm",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateEmptyMemoVector() TestVector {
	input := TestVectorInput{
		ChainID:         "punnet-mainnet-1",
		Account:         "alice",
		AccountSequence: "1",
		Nonce:           "1",
		Memo:            "", // Explicitly empty
		Messages: []TestVectorMessage{
			{
				Type: "/punnet.bank.v1.MsgSend",
				Data: json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{},
			GasLimit: "0",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "0",
			Denominator: "1",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "empty_memo",
		Description: "Transaction with explicitly empty memo field",
		Category:    "edge_case",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateZeroValuesVector() TestVector {
	input := TestVectorInput{
		ChainID:         "zero-test",
		Account:         "zero",
		AccountSequence: "0",
		Nonce:           "0",
		Memo:            "",
		Messages: []TestVectorMessage{
			{
				Type: "/test.Zero",
				Data: json.RawMessage(`{"value":"0"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{{Denom: "stake", Amount: "0"}},
			GasLimit: "0",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "0",
			Denominator: "1",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "zero_values",
		Description: "Transaction with zero values for sequence, nonce, amounts, and gas",
		Category:    "edge_case",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateLargeNumbersVector() TestVector {
	// Use max uint64 values to test boundary conditions
	input := TestVectorInput{
		ChainID:         "large-numbers-test",
		Account:         "bignum",
		AccountSequence: "18446744073709551615", // max uint64
		Nonce:           "18446744073709551615", // max uint64
		Memo:            "",
		Messages: []TestVectorMessage{
			{
				Type: "/test.LargeNumber",
				Data: json.RawMessage(`{"amount":"18446744073709551615"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{{Denom: "stake", Amount: "18446744073709551615"}},
			GasLimit: "18446744073709551615",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "18446744073709551615",
			Denominator: "18446744073709551615",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "large_numbers",
		Description: "Transaction with maximum uint64 values (18446744073709551615)",
		Category:    "edge_case",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateUnicodeVector() TestVector {
	input := TestVectorInput{
		ChainID:         "unicode-test",
		Account:         "unicode",
		AccountSequence: "1",
		Nonce:           "1",
		Memo:            "Hello 世界! Привет мир! مرحبا بالعالم 🌍🚀",
		Messages: []TestVectorMessage{
			{
				Type: "/test.Unicode",
				Data: json.RawMessage(`{"greeting":"こんにちは","emoji":"👋"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{{Denom: "stake", Amount: "100"}},
			GasLimit: "50000",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "1",
			Denominator: "100",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "unicode_memo",
		Description: "Transaction with Unicode characters in memo and message data",
		Category:    "edge_case",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateSpecialCharsVector() TestVector {
	input := TestVectorInput{
		ChainID:         "special-chars-test",
		Account:         "special",
		AccountSequence: "1",
		Nonce:           "1",
		Memo: `Special chars: "quotes", 'apostrophe', \backslash, /slash, tab:	newline:
end`,
		Messages: []TestVectorMessage{
			{
				Type: "/test.SpecialChars",
				Data: json.RawMessage(`{"text":"line1\nline2\ttab","quoted":"\"value\""}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{{Denom: "stake", Amount: "100"}},
			GasLimit: "50000",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "1",
			Denominator: "100",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "special_chars",
		Description: "Transaction with special characters (quotes, escapes, newlines, tabs)",
		Category:    "edge_case",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

func generateMinimalVector() TestVector {
	input := TestVectorInput{
		ChainID:         "m",
		Account:         "a",
		AccountSequence: "0",
		Nonce:           "0",
		Memo:            "",
		Messages: []TestVectorMessage{
			{
				Type: "/m",
				Data: json.RawMessage(`{}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{},
			GasLimit: "0",
		},
		FeeSlippage: TestVectorRatio{
			Numerator:   "0",
			Denominator: "1",
		},
	}

	signDoc := buildSignDocFromInput(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name:        "minimal",
		Description: "Minimal valid transaction with shortest possible valid values",
		Category:    "edge_case",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"ed25519": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Ed25519.PrivateKey),
					PublicKeyHex:  hex.EncodeToString(WellKnownTestKeys.Ed25519.PublicKey),
					SignatureHex:  hex.EncodeToString(ed25519Sig),
				},
			},
		},
	}
}

// buildSignDocFromInput constructs a SignDoc from test vector input.
// Panics on invalid input since test vectors should always be valid.
func buildSignDocFromInput(input TestVectorInput) *types.SignDoc {
	// Parse numeric strings
	var accountSequence, nonce uint64
	if err := json.Unmarshal([]byte(`"`+input.AccountSequence+`"`), (*types.StringUint64)(&accountSequence)); err != nil {
		panic("invalid account_sequence in test vector: " + err.Error())
	}
	if err := json.Unmarshal([]byte(`"`+input.Nonce+`"`), (*types.StringUint64)(&nonce)); err != nil {
		panic("invalid nonce in test vector: " + err.Error())
	}

	// Build fee coins
	feeCoins := make([]types.SignDocCoin, len(input.Fee.Amount))
	for i, coin := range input.Fee.Amount {
		feeCoins[i] = types.SignDocCoin{
			Denom:  coin.Denom,
			Amount: coin.Amount,
		}
	}

	fee := types.SignDocFee{
		Amount:   feeCoins,
		GasLimit: input.Fee.GasLimit,
	}

	slippage := types.SignDocRatio{
		Numerator:   input.FeeSlippage.Numerator,
		Denominator: input.FeeSlippage.Denominator,
	}

	signDoc := types.NewSignDocWithFee(
		input.ChainID,
		accountSequence,
		input.Account,
		nonce,
		input.Memo,
		fee,
		slippage,
	)

	// Add messages
	// NOTE: msg.Data from JSON file may have whitespace; compact it to ensure
	// deterministic serialization matches the expected sign_doc_json.
	for _, msg := range input.Messages {
		// Compact the JSON to remove any formatting whitespace
		var compactData bytes.Buffer
		if err := json.Compact(&compactData, msg.Data); err != nil {
			panic("failed to compact message data: " + err.Error())
		}
		signDoc.AddMessage(msg.Type, json.RawMessage(compactData.Bytes()))
	}

	return signDoc
}
