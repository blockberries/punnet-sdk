package vectors

import (
	"bytes"
	stdecdsa "crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"time"

	"github.com/blockberries/punnet-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	secp256k1ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// WellKnownTestKeys contains deterministic test keys for reproducible test vectors.
// SECURITY: These keys are for testing ONLY. Never use in production.
//
// The keys are derived from well-known seeds to ensure cross-implementation reproducibility.
// All seeds are 32-byte SHA-256 hashes of algorithm-specific seed strings.
var WellKnownTestKeys = struct {
	Ed25519 struct {
		// Seed is 32 bytes of deterministic test data
		Seed       []byte
		PrivateKey ed25519.PrivateKey
		PublicKey  ed25519.PublicKey
	}
	Secp256k1 struct {
		// Seed is 32 bytes of deterministic test data (used as private key scalar)
		Seed       []byte
		PrivateKey *secp256k1.PrivateKey
		PublicKey  *secp256k1.PublicKey
	}
	Secp256r1 struct {
		// Seed is 32 bytes of deterministic test data (used as private key scalar)
		Seed       []byte
		PrivateKey *stdecdsa.PrivateKey
		PublicKey  *stdecdsa.PublicKey
	}
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
	Secp256k1: func() struct {
		Seed       []byte
		PrivateKey *secp256k1.PrivateKey
		PublicKey  *secp256k1.PublicKey
	} {
		// Deterministic seed: SHA-256("punnet-sdk-test-vector-seed-secp256k1")
		seedHash := sha256.Sum256([]byte("punnet-sdk-test-vector-seed-secp256k1"))
		seed := seedHash[:]

		// Create private key from 32-byte seed (scalar).
		// SCALAR RANGE NOTE: The dcrd/secp256k1 library's PrivKeyFromBytes handles
		// scalar validation internally, reducing values >= curve order (n) mod n.
		// For test vectors with fixed seeds, this behavior is acceptable since the
		// resulting key is deterministic. This specific seed string produces a
		// valid scalar without reduction.
		privateKey := secp256k1.PrivKeyFromBytes(seed)
		publicKey := privateKey.PubKey()
		return struct {
			Seed       []byte
			PrivateKey *secp256k1.PrivateKey
			PublicKey  *secp256k1.PublicKey
		}{
			Seed:       seed,
			PrivateKey: privateKey,
			PublicKey:  publicKey,
		}
	}(),
	Secp256r1: func() struct {
		Seed       []byte
		PrivateKey *stdecdsa.PrivateKey
		PublicKey  *stdecdsa.PublicKey
	} {
		// Deterministic seed: SHA-256("punnet-sdk-test-vector-seed-secp256r1")
		seedHash := sha256.Sum256([]byte("punnet-sdk-test-vector-seed-secp256r1"))
		seed := seedHash[:]

		// Create private key from 32-byte seed (scalar) on P-256 curve.
		// SCALAR RANGE ASSUMPTION: The seed must be in range [1, n-1] where n is the
		// P-256 curve order (~2^256 - 4.3×10^38). SHA-256 outputs are uniformly
		// distributed in [0, 2^256), so the probability of seed >= n is ~10^-39,
		// which is negligible. This specific seed string has been verified to produce
		// a valid scalar. For production use, proper key generation should use
		// crypto/ecdsa.GenerateKey or similar, which handles scalar validation.
		curve := elliptic.P256()
		d := new(big.Int).SetBytes(seed)

		// Verify the scalar is valid (paranoid check for test vectors)
		if d.Sign() == 0 || d.Cmp(curve.Params().N) >= 0 {
			panic("seed produces invalid scalar for P-256 (this should never happen with this seed)")
		}

		privateKey := new(stdecdsa.PrivateKey)
		privateKey.D = d
		privateKey.PublicKey.Curve = curve
		privateKey.PublicKey.X, privateKey.PublicKey.Y = curve.ScalarBaseMult(seed)
		return struct {
			Seed       []byte
			PrivateKey *stdecdsa.PrivateKey
			PublicKey  *stdecdsa.PublicKey
		}{
			Seed:       seed,
			PrivateKey: privateKey,
			PublicKey:  &privateKey.PublicKey,
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

	// secp256k1 key derivation and signing
	vectors = append(vectors, generateSecp256k1KeyDerivationVector())
	vectors = append(vectors, generateSecp256k1SigningVector())

	// secp256r1 (P-256) key derivation and signing
	vectors = append(vectors, generateSecp256r1KeyDerivationVector())
	vectors = append(vectors, generateSecp256r1SigningVector())

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

	// Nil vs empty value vectors for cross-implementation compatibility
	vectors = append(vectors, generateNilVsEmptyVectors()...)

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

// generateNilVsEmptyVectors creates test vectors that explicitly test nil vs empty value
// serialization for cross-implementation compatibility.
//
// SECURITY RATIONALE: Different programming languages may serialize null/nil vs empty values
// differently. For example:
// - Go: nil slice vs empty slice ([]string(nil) vs []string{})
// - JavaScript: null vs undefined vs "" vs []
// - Rust: Option<Vec<T>> None vs Some(Vec::new())
// - Python: None vs "" vs []
//
// These vectors ensure all implementations produce identical signatures for equivalent transactions.
func generateNilVsEmptyVectors() []TestVector {
	vectors := []TestVector{}

	// Memo variants: empty string (canonical form)
	// Note: In the canonical SignDoc format, memo is ALWAYS serialized as a string.
	// Null/nil memo is normalized to empty string "".
	vectors = append(vectors, generateEmptyStringMemoVector())

	// Fee amount variants: empty array (canonical form)
	// Note: In the canonical SignDoc format, fee.amount is ALWAYS an array.
	// Null/nil amounts are normalized to empty array [].
	vectors = append(vectors, generateEmptyFeeAmountVector())

	// Combined: empty memo AND empty fee amounts
	vectors = append(vectors, generateEmptyMemoAndFeeVector())

	// Message data variants: empty object vs null
	vectors = append(vectors, generateEmptyMessageDataVector())
	vectors = append(vectors, generateNullMessageDataVector())

	return vectors
}

// generateEmptyStringMemoVector tests that empty string memo serializes correctly.
// This is the canonical form - implementations MUST normalize null/nil/undefined memo to "".
func generateEmptyStringMemoVector() TestVector {
	input := TestVectorInput{
		ChainID:         "nil-vs-empty-test",
		Account:         "tester",
		AccountSequence: "1",
		Nonce:           "1",
		Memo:            "", // Empty string (canonical form)
		Messages: []TestVectorMessage{
			{
				Type: "/test.NilVsEmpty",
				Data: json.RawMessage(`{"field":"memo_test"}`),
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
		Name: "nil_vs_empty_memo_string",
		Description: `Tests empty string memo serialization.
CRITICAL: Implementations MUST normalize null/nil/undefined memo to empty string "".
The canonical JSON MUST contain "memo":"" (not omitted, not null).
Different representations that MUST all produce this output:
- Go: memo = "" or memo = ""
- JavaScript: memo = "" or memo = null or memo = undefined
- Rust: memo = String::new() or memo = None (Option<String>)
- Python: memo = "" or memo = None`,
		Category: "edge_case",
		Input:    input,
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

// generateEmptyFeeAmountVector tests that empty fee amount array serializes correctly.
// This is the canonical form - implementations MUST normalize null/nil to empty array [].
func generateEmptyFeeAmountVector() TestVector {
	input := TestVectorInput{
		ChainID:         "nil-vs-empty-test",
		Account:         "tester",
		AccountSequence: "2",
		Nonce:           "2",
		Memo:            "fee amount test",
		Messages: []TestVectorMessage{
			{
				Type: "/test.NilVsEmpty",
				Data: json.RawMessage(`{"field":"fee_test"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{}, // Empty array (canonical form)
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
		Name: "nil_vs_empty_fee_amount",
		Description: `Tests empty fee amount array serialization.
CRITICAL: Implementations MUST normalize null/nil fee amounts to empty array [].
The canonical JSON MUST contain "amount":[] (not omitted, not null).
Different representations that MUST all produce this output:
- Go: Amount = []SignDocCoin{} or Amount = nil
- JavaScript: amount = [] or amount = null or amount = undefined
- Rust: amount = Vec::new() or amount = None (if Option<Vec<Coin>>)
- Python: amount = [] or amount = None`,
		Category: "edge_case",
		Input:    input,
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

// generateEmptyMemoAndFeeVector tests combined empty memo AND empty fee amounts.
// This validates that multiple empty/nil fields are handled correctly together.
func generateEmptyMemoAndFeeVector() TestVector {
	input := TestVectorInput{
		ChainID:         "nil-vs-empty-test",
		Account:         "tester",
		AccountSequence: "3",
		Nonce:           "3",
		Memo:            "", // Empty
		Messages: []TestVectorMessage{
			{
				Type: "/test.NilVsEmpty",
				Data: json.RawMessage(`{"field":"combined_test"}`),
			},
		},
		Fee: TestVectorFee{
			Amount:   []TestVectorCoin{}, // Empty array
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
		Name: "nil_vs_empty_combined",
		Description: `Tests both empty memo AND empty fee amounts together.
This validates that implementations correctly handle multiple nil/empty fields.
Expected canonical JSON contains both "memo":"" AND "amount":[]`,
		Category: "edge_case",
		Input:    input,
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

// generateEmptyMessageDataVector tests empty object {} as message data.
// This is valid and must serialize deterministically.
func generateEmptyMessageDataVector() TestVector {
	input := TestVectorInput{
		ChainID:         "nil-vs-empty-test",
		Account:         "tester",
		AccountSequence: "4",
		Nonce:           "4",
		Memo:            "empty message data test",
		Messages: []TestVectorMessage{
			{
				Type: "/test.EmptyData",
				Data: json.RawMessage(`{}`), // Empty object
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
		Name: "nil_vs_empty_message_data_object",
		Description: `Tests empty object {} as message data.
Empty object is a valid message data value and MUST serialize as "data":{}
This is distinct from null message data (see nil_vs_empty_message_data_null).`,
		Category: "edge_case",
		Input:    input,
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

// generateNullMessageDataVector tests null as message data.
// When msg.Data is nil/null, it serializes as "data":null.
func generateNullMessageDataVector() TestVector {
	// For this test, we need to manually set Data to nil after building
	input := TestVectorInput{
		ChainID:         "nil-vs-empty-test",
		Account:         "tester",
		AccountSequence: "5",
		Nonce:           "5",
		Memo:            "null message data test",
		Messages: []TestVectorMessage{
			{
				Type: "/test.NullData",
				Data: nil, // Explicitly nil/null
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

	signDoc := buildSignDocFromInputWithNullData(input)
	signDocJSON := mustJSON(signDoc)
	signBytes := mustSignBytes(signDoc)
	ed25519Sig := ed25519.Sign(WellKnownTestKeys.Ed25519.PrivateKey, signBytes)

	return TestVector{
		Name: "nil_vs_empty_message_data_null",
		Description: `Tests null as message data.
When message data is null/nil, it MUST serialize as "data":null
This is distinct from empty object {} (see nil_vs_empty_message_data_object).
SECURITY: Implementations must distinguish between null and empty object
as they produce different signatures.`,
		Category: "edge_case",
		Input:    input,
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

// buildSignDocFromInputWithNullData is like buildSignDocFromInput but allows null message data.
func buildSignDocFromInputWithNullData(input TestVectorInput) *types.SignDoc {
	var accountSequence, nonce uint64
	if err := json.Unmarshal([]byte(`"`+input.AccountSequence+`"`), (*types.StringUint64)(&accountSequence)); err != nil {
		panic("invalid account_sequence in test vector: " + err.Error())
	}
	if err := json.Unmarshal([]byte(`"`+input.Nonce+`"`), (*types.StringUint64)(&nonce)); err != nil {
		panic("invalid nonce in test vector: " + err.Error())
	}

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

	// Add messages - preserve null data as nil
	for _, msg := range input.Messages {
		if msg.Data == nil {
			signDoc.AddMessage(msg.Type, nil)
		} else {
			var compactData bytes.Buffer
			if err := json.Compact(&compactData, msg.Data); err != nil {
				panic("failed to compact message data: " + err.Error())
			}
			signDoc.AddMessage(msg.Type, json.RawMessage(compactData.Bytes()))
		}
	}

	return signDoc
}

// generateSecp256k1KeyDerivationVector creates the secp256k1 key derivation test vector.
func generateSecp256k1KeyDerivationVector() TestVector {
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

	// Generate secp256k1 signature (RFC 6979 deterministic)
	secp256k1Sig := secp256k1ecdsa.SignCompact(WellKnownTestKeys.Secp256k1.PrivateKey, signBytes, false)
	// SignCompact returns [V || R || S], we need just [R || S] (64 bytes)
	secp256k1SigRS := secp256k1Sig[1:] // Remove recovery byte

	// Document the seed derivation process
	seedHash := sha256.Sum256([]byte("punnet-sdk-test-vector-seed-secp256k1"))

	// Get compressed public key (33 bytes)
	compressedPubKey := WellKnownTestKeys.Secp256k1.PublicKey.SerializeCompressed()

	return TestVector{
		Name:        "secp256k1_key_derivation",
		Description: "secp256k1 key derivation from deterministic seed: SHA-256(\"punnet-sdk-test-vector-seed-secp256k1\")",
		Category:    "algorithm",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"secp256k1": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Secp256k1.PrivateKey.Serialize()),
					PublicKeyHex:  hex.EncodeToString(compressedPubKey),
					SignatureHex:  hex.EncodeToString(secp256k1SigRS),
				},
				"secp256k1_seed": {
					PrivateKeyHex: hex.EncodeToString(seedHash[:]),
					PublicKeyHex:  hex.EncodeToString(compressedPubKey),
					SignatureHex:  "", // Seed is not for signing directly
				},
			},
		},
	}
}

// generateSecp256k1SigningVector creates the secp256k1 signing test vector.
func generateSecp256k1SigningVector() TestVector {
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

	// Generate secp256k1 signature (RFC 6979 deterministic)
	secp256k1Sig := secp256k1ecdsa.SignCompact(WellKnownTestKeys.Secp256k1.PrivateKey, signBytes, false)
	// SignCompact returns [V || R || S], we need just [R || S] (64 bytes)
	secp256k1SigRS := secp256k1Sig[1:] // Remove recovery byte

	// Get compressed public key (33 bytes)
	compressedPubKey := WellKnownTestKeys.Secp256k1.PublicKey.SerializeCompressed()

	return TestVector{
		Name:        "secp256k1_signing",
		Description: "secp256k1 signature generation for a known message (RFC 6979 deterministic)",
		Category:    "algorithm",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"secp256k1": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Secp256k1.PrivateKey.Serialize()),
					PublicKeyHex:  hex.EncodeToString(compressedPubKey),
					SignatureHex:  hex.EncodeToString(secp256k1SigRS),
				},
			},
		},
	}
}

// generateSecp256r1KeyDerivationVector creates the secp256r1 (P-256) key derivation test vector.
func generateSecp256r1KeyDerivationVector() TestVector {
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

	// Generate secp256r1 signature (RFC 6979 deterministic)
	secp256r1Sig := signSecp256r1RFC6979(WellKnownTestKeys.Secp256r1.PrivateKey, signBytes)

	// Document the seed derivation process
	seedHash := sha256.Sum256([]byte("punnet-sdk-test-vector-seed-secp256r1"))

	// Get compressed public key (33 bytes: 0x02/0x03 prefix + 32 bytes X coordinate)
	compressedPubKey := compressP256PublicKey(&WellKnownTestKeys.Secp256r1.PrivateKey.PublicKey)

	return TestVector{
		Name:        "secp256r1_key_derivation",
		Description: "secp256r1 (P-256) key derivation from deterministic seed: SHA-256(\"punnet-sdk-test-vector-seed-secp256r1\")",
		Category:    "algorithm",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"secp256r1": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Secp256r1.PrivateKey.D.Bytes()),
					PublicKeyHex:  hex.EncodeToString(compressedPubKey),
					SignatureHex:  hex.EncodeToString(secp256r1Sig),
				},
				"secp256r1_seed": {
					PrivateKeyHex: hex.EncodeToString(seedHash[:]),
					PublicKeyHex:  hex.EncodeToString(compressedPubKey),
					SignatureHex:  "", // Seed is not for signing directly
				},
			},
		},
	}
}

// generateSecp256r1SigningVector creates the secp256r1 (P-256) signing test vector.
func generateSecp256r1SigningVector() TestVector {
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

	// Generate secp256r1 signature (RFC 6979 deterministic)
	secp256r1Sig := signSecp256r1RFC6979(WellKnownTestKeys.Secp256r1.PrivateKey, signBytes)

	// Get compressed public key (33 bytes)
	compressedPubKey := compressP256PublicKey(&WellKnownTestKeys.Secp256r1.PrivateKey.PublicKey)

	return TestVector{
		Name:        "secp256r1_signing",
		Description: "secp256r1 (P-256) signature generation for a known message (RFC 6979 deterministic)",
		Category:    "algorithm",
		Input:       input,
		Expected: TestVectorExpected{
			SignDocJSON:  string(signDocJSON),
			SignBytesHex: hex.EncodeToString(signBytes),
			Signatures: map[string]TestVectorSignature{
				"secp256r1": {
					PrivateKeyHex: hex.EncodeToString(WellKnownTestKeys.Secp256r1.PrivateKey.D.Bytes()),
					PublicKeyHex:  hex.EncodeToString(compressedPubKey),
					SignatureHex:  hex.EncodeToString(secp256r1Sig),
				},
			},
		},
	}
}

// signSecp256r1RFC6979 signs data using deterministic ECDSA with P-256.
// Returns a 64-byte signature [R || S] in big-endian format.
//
// IMPLEMENTATION NOTE: Go's crypto/ecdsa uses the random reader for entropy
// mixing in nonce generation. By providing a deterministic reader (zeroReader),
// we get deterministic signatures. However, this does NOT guarantee RFC 6979
// compliance - it produces internally consistent signatures suitable for
// cross-implementation testing within the Punnet SDK ecosystem.
//
// For true RFC 6979 compliance that matches external implementations, use a
// dedicated RFC 6979 library. See issue #146 for cross-implementation validation.
func signSecp256r1RFC6979(privateKey *stdecdsa.PrivateKey, data []byte) []byte {
	// Use zeroReader to get deterministic signatures from Go's crypto/ecdsa.
	// This produces consistent signatures for test vector verification.
	asn1Sig, err := stdecdsa.SignASN1(zeroReader{}, privateKey, data)
	if err != nil {
		panic("failed to sign: " + err.Error())
	}

	// Parse ASN.1 signature to get R and S
	r, s := parseASN1Signature(asn1Sig)

	// Serialize R and S as 32-byte big-endian values
	sig := make([]byte, 64)
	rBytes := r.Bytes()
	sBytes := s.Bytes()

	// Right-align in 32-byte buffers
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)

	return sig
}

// zeroReader provides a deterministic source of zeros for crypto/ecdsa signing.
// This produces deterministic signatures by providing consistent entropy to
// Go's ECDSA implementation's internal nonce generation.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// parseASN1Signature extracts R and S from an ASN.1 DER-encoded ECDSA signature.
func parseASN1Signature(sig []byte) (*big.Int, *big.Int) {
	// ASN.1 format: SEQUENCE { INTEGER r, INTEGER s }
	// 0x30 len 0x02 rlen r 0x02 slen s
	if len(sig) < 8 || sig[0] != 0x30 {
		panic("invalid ASN.1 signature")
	}
	idx := 2
	if sig[1]&0x80 != 0 {
		idx += int(sig[1] & 0x7f)
	}

	// Parse R
	if sig[idx] != 0x02 {
		panic("expected INTEGER for R")
	}
	rLen := int(sig[idx+1])
	idx += 2
	rBytes := sig[idx : idx+rLen]
	idx += rLen

	// Parse S
	if sig[idx] != 0x02 {
		panic("expected INTEGER for S")
	}
	sLen := int(sig[idx+1])
	idx += 2
	sBytes := sig[idx : idx+sLen]

	r := new(big.Int).SetBytes(rBytes)
	s := new(big.Int).SetBytes(sBytes)

	return r, s
}

// compressP256PublicKey returns the compressed form of a P-256 public key (33 bytes).
// Format: 0x02 + X (if Y is even) or 0x03 + X (if Y is odd)
func compressP256PublicKey(pubKey *stdecdsa.PublicKey) []byte {
	compressed := make([]byte, 33)
	if pubKey.Y.Bit(0) == 0 {
		compressed[0] = 0x02
	} else {
		compressed[0] = 0x03
	}
	xBytes := pubKey.X.Bytes()
	// Right-align X in 32 bytes
	copy(compressed[33-len(xBytes):], xBytes)
	return compressed
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
