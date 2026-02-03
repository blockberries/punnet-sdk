package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/blockberries/punnet-sdk/crypto"
)

// ============================================================================
// End-to-End Signing Flow Benchmarks
// ============================================================================
//
// These benchmarks measure the complete flow from SignDoc creation to signature.
// Flow: NewSignDoc → AddMessage → ToJSON → GetSignBytes (SHA256) → Sign

func BenchmarkFullSignFlow_Ed25519_Small(b *testing.B) {
	key, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create SignDoc
		sd := NewSignDoc("test-chain", uint64(i), "alice", uint64(i), "")
		sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`))

		// Get sign bytes (ToJSON + SHA256)
		signBytes, err := sd.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}

		// Sign
		_, err = key.Sign(signBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFullSignFlow_Ed25519_Medium(b *testing.B) {
	key, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sd := NewSignDoc("mainnet-production", uint64(i), "cosmos1abc...xyz", uint64(i), "batch transfer")
		sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"cosmos1abc...xyz","to":"cosmos1def...uvw","amount":{"denom":"stake","amount":"1000000"}}`))
		sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"cosmos1abc...xyz","to":"cosmos1ghi...rst","amount":{"denom":"stake","amount":"500000"}}`))
		sd.AddMessage("/punnet.staking.v1.MsgDelegate", json.RawMessage(`{"delegator":"cosmos1abc...xyz","validator":"cosmosvaloper1xyz...","amount":{"denom":"stake","amount":"2000000"}}`))

		signBytes, err := sd.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}

		_, err = key.Sign(signBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFullSignFlow_Ed25519_Large(b *testing.B) {
	key, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	memo := strings.Repeat("x", 512)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sd := NewSignDoc("mainnet-production-chain-id", uint64(i), "cosmos1verylongaddress", uint64(i), memo)
		for j := 0; j < 10; j++ {
			data := fmt.Sprintf(`{"from":"cosmos1sender%d","to":"cosmos1receiver%d","amount":{"denom":"ustake","amount":"%d"}}`, j, j, j*1000000)
			sd.AddMessage(fmt.Sprintf("/punnet.bank.v1.MsgSend%d", j), json.RawMessage(data))
		}

		signBytes, err := sd.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}

		_, err = key.Sign(signBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// End-to-End Verification Flow Benchmarks
// ============================================================================
//
// These benchmarks measure the complete verification flow.
// Flow: ParseSignDoc → ValidateBasic → GetSignBytes → Verify

func BenchmarkFullVerifyFlow_Ed25519_Small(b *testing.B) {
	// Setup: create a signed SignDoc
	key, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()

	sd := NewSignDoc("test-chain", 1, "alice", 1, "")
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`))

	jsonBytes, err := sd.ToJSON()
	if err != nil {
		b.Fatal(err)
	}
	signBytes, err := sd.GetSignBytes()
	if err != nil {
		b.Fatal(err)
	}
	signature, err := key.Sign(signBytes)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Parse SignDoc from JSON
		parsedSD, err := ParseSignDoc(jsonBytes)
		if err != nil {
			b.Fatal(err)
		}

		// Validate
		if err := parsedSD.ValidateBasic(); err != nil {
			b.Fatal(err)
		}

		// Get sign bytes
		verifyBytes, err := parsedSD.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}

		// Verify signature
		if !pubKey.Verify(verifyBytes, signature) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkFullVerifyFlow_Ed25519_Medium(b *testing.B) {
	key, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()

	sd := NewSignDoc("mainnet-production", 12345, "cosmos1abc...xyz", 12345, "batch transfer")
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"cosmos1abc...xyz","to":"cosmos1def...uvw","amount":{"denom":"stake","amount":"1000000"}}`))
	sd.AddMessage("/punnet.bank.v1.MsgSend", json.RawMessage(`{"from":"cosmos1abc...xyz","to":"cosmos1ghi...rst","amount":{"denom":"stake","amount":"500000"}}`))
	sd.AddMessage("/punnet.staking.v1.MsgDelegate", json.RawMessage(`{"delegator":"cosmos1abc...xyz","validator":"cosmosvaloper1xyz...","amount":{"denom":"stake","amount":"2000000"}}`))

	jsonBytes, err := sd.ToJSON()
	if err != nil {
		b.Fatal(err)
	}
	signBytes, err := sd.GetSignBytes()
	if err != nil {
		b.Fatal(err)
	}
	signature, err := key.Sign(signBytes)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parsedSD, err := ParseSignDoc(jsonBytes)
		if err != nil {
			b.Fatal(err)
		}

		if err := parsedSD.ValidateBasic(); err != nil {
			b.Fatal(err)
		}

		verifyBytes, err := parsedSD.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}

		if !pubKey.Verify(verifyBytes, signature) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkFullVerifyFlow_Ed25519_Large(b *testing.B) {
	key, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()

	memo := strings.Repeat("x", 512)
	sd := NewSignDoc("mainnet-production-chain-id", 999999999, "cosmos1verylongaddress", 999999999, memo)
	for j := 0; j < 10; j++ {
		data := fmt.Sprintf(`{"from":"cosmos1sender%d","to":"cosmos1receiver%d","amount":{"denom":"ustake","amount":"%d"}}`, j, j, j*1000000)
		sd.AddMessage(fmt.Sprintf("/punnet.bank.v1.MsgSend%d", j), json.RawMessage(data))
	}

	jsonBytes, err := sd.ToJSON()
	if err != nil {
		b.Fatal(err)
	}
	signBytes, err := sd.GetSignBytes()
	if err != nil {
		b.Fatal(err)
	}
	signature, err := key.Sign(signBytes)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		parsedSD, err := ParseSignDoc(jsonBytes)
		if err != nil {
			b.Fatal(err)
		}

		if err := parsedSD.ValidateBasic(); err != nil {
			b.Fatal(err)
		}

		verifyBytes, err := parsedSD.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}

		if !pubKey.Verify(verifyBytes, signature) {
			b.Fatal("verification failed")
		}
	}
}

// ============================================================================
// Comparison Benchmarks: Isolated Steps
// ============================================================================
//
// These benchmarks isolate each step of the flow for comparison.

func BenchmarkIsolated_NewSignDoc(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = NewSignDoc("test-chain", uint64(i), "alice", uint64(i), "memo")
	}
}

func BenchmarkIsolated_AddMessage(b *testing.B) {
	sd := NewSignDoc("test-chain", 1, "alice", 1, "")
	msg := json.RawMessage(`{"from":"alice","to":"bob","amount":"100"}`)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Reset messages to measure just AddMessage
		sd.Messages = sd.Messages[:0]
		sd.AddMessage("/punnet.bank.v1.MsgSend", msg)
	}
}

func BenchmarkIsolated_ToJSON_PrebuiltSmall(b *testing.B) {
	sd := createSmallSignDoc()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = sd.ToJSON()
	}
}

func BenchmarkIsolated_SHA256_SmallJSON(b *testing.B) {
	sd := createSmallSignDoc()
	jsonBytes, _ := sd.ToJSON()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// This is essentially what GetSignBytes does after ToJSON
		_ = sha256Sum(jsonBytes)
	}
}

func BenchmarkIsolated_Ed25519Sign_32Bytes(b *testing.B) {
	key, _ := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	data := make([]byte, 32) // SHA256 output size
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = key.Sign(data)
	}
}

func BenchmarkIsolated_Ed25519Verify_32Bytes(b *testing.B) {
	key, _ := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	pubKey := key.PublicKey()
	data := make([]byte, 32)
	sig, _ := key.Sign(data)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pubKey.Verify(data, sig)
	}
}

// sha256Sum computes SHA256 hash for isolated benchmarking.
func sha256Sum(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// ============================================================================
// Throughput Benchmarks
// ============================================================================
//
// These benchmarks measure signatures/verifications per second with realistic workload.

func BenchmarkThroughput_SignVerify_Ed25519(b *testing.B) {
	// Simulate realistic workload: sign and verify same transaction
	key, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()

	sd := createMediumSignDoc()
	signBytes, err := sd.GetSignBytes()
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Sign
		signature, err := key.Sign(signBytes)
		if err != nil {
			b.Fatal(err)
		}
		// Verify immediately (common pattern)
		if !pubKey.Verify(signBytes, signature) {
			b.Fatal("verification failed")
		}
	}
}

// BenchmarkParallel_Verify_Ed25519 measures verification throughput under concurrent load.
func BenchmarkParallel_Verify_Ed25519(b *testing.B) {
	key, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()

	sd := createMediumSignDoc()
	signBytes, err := sd.GetSignBytes()
	if err != nil {
		b.Fatal(err)
	}
	signature, err := key.Sign(signBytes)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if !pubKey.Verify(signBytes, signature) {
				b.Fatal("verification failed")
			}
		}
	})
}

// ============================================================================
// Memory Pressure Benchmarks
// ============================================================================

// BenchmarkMemoryPressure_SignDocCreation measures allocation pressure
// when creating many SignDocs in quick succession.
func BenchmarkMemoryPressure_SignDocCreation(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Create and immediately discard - measures GC pressure
		sd := createMediumSignDoc()
		_, _ = sd.GetSignBytes()
	}
}

// BenchmarkMemoryPressure_BatchVerification simulates batch verification
// where multiple signatures are verified in sequence.
func BenchmarkMemoryPressure_BatchVerification(b *testing.B) {
	batchSize := 100

	// Pre-generate keys and signatures
	keys := make([]crypto.PrivateKey, batchSize)
	pubKeys := make([]crypto.PublicKey, batchSize)
	signatures := make([][]byte, batchSize)
	signBytesArr := make([][]byte, batchSize)

	for i := 0; i < batchSize; i++ {
		key, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
		if err != nil {
			b.Fatal(err)
		}
		keys[i] = key
		pubKeys[i] = key.PublicKey()

		sd := NewSignDoc("test-chain", uint64(i), fmt.Sprintf("account%d", i), uint64(i), "")
		sd.AddMessage("/msg", json.RawMessage(`{}`))
		signBytes, err := sd.GetSignBytes()
		if err != nil {
			b.Fatal(err)
		}
		signBytesArr[i] = signBytes

		sig, err := key.Sign(signBytes)
		if err != nil {
			b.Fatal(err)
		}
		signatures[i] = sig
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Verify all signatures in batch
		for j := 0; j < batchSize; j++ {
			idx := j % batchSize
			if !pubKeys[idx].Verify(signBytesArr[idx], signatures[idx]) {
				b.Fatal("verification failed")
			}
		}
	}
}
