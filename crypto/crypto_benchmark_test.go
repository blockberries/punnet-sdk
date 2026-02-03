package crypto

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"testing"
)

// ============================================================================
// Key Generation Benchmarks
// ============================================================================

func BenchmarkKeyGenEd25519(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key, err := GeneratePrivateKey(AlgorithmEd25519)
		if err != nil {
			b.Fatal(err)
		}
		// Ensure key is usable (prevents optimization away)
		_ = key.PublicKey().Bytes()
	}
}

func BenchmarkKeyGenSecp256k1(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key, err := GeneratePrivateKey(AlgorithmSecp256k1)
		if err != nil {
			b.Fatal(err)
		}
		_ = key.PublicKey().Bytes()
	}
}

func BenchmarkKeyGenSecp256r1(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key, err := GeneratePrivateKey(AlgorithmSecp256r1)
		if err != nil {
			b.Fatal(err)
		}
		_ = key.PublicKey().Bytes()
	}
}

// ============================================================================
// Signing Benchmarks
// ============================================================================

// Generate test data of various sizes for signing benchmarks
func generateTestData(size int) []byte {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	return data
}

// Typical signed data sizes:
// - 32 bytes: raw hash (e.g., SHA256)
// - 64 bytes: double hash or concatenated hashes
// - 256 bytes: small message
// - 1024 bytes: typical transaction

func BenchmarkSignEd25519_32Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	data := generateTestData(32) // SHA256 hash size
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := key.Sign(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignEd25519_256Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	data := generateTestData(256)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := key.Sign(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignEd25519_1024Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	data := generateTestData(1024)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := key.Sign(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignSecp256k1_32Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		b.Fatal(err)
	}
	data := generateTestData(32)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := key.Sign(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSignSecp256r1_32Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		b.Fatal(err)
	}
	data := generateTestData(32)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := key.Sign(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Verification Benchmarks
// ============================================================================

func BenchmarkVerifyEd25519_32Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()
	data := generateTestData(32)
	signature, err := key.Sign(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !pubKey.Verify(data, signature) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkVerifyEd25519_256Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()
	data := generateTestData(256)
	signature, err := key.Sign(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !pubKey.Verify(data, signature) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkVerifyEd25519_1024Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()
	data := generateTestData(1024)
	signature, err := key.Sign(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !pubKey.Verify(data, signature) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkVerifySecp256k1_32Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmSecp256k1)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()
	data := generateTestData(32)
	signature, err := key.Sign(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !pubKey.Verify(data, signature) {
			b.Fatal("verification failed")
		}
	}
}

func BenchmarkVerifySecp256r1_32Bytes(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()
	data := generateTestData(32)
	signature, err := key.Sign(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if !pubKey.Verify(data, signature) {
			b.Fatal("verification failed")
		}
	}
}

// ============================================================================
// Data Size Scaling Benchmarks
// ============================================================================

func BenchmarkSignEd25519_DataSizeScaling(b *testing.B) {
	dataSizes := []int{32, 64, 128, 256, 512, 1024, 4096, 16384}
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}

	for _, size := range dataSizes {
		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			data := generateTestData(size)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := key.Sign(data)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkVerifyEd25519_DataSizeScaling(b *testing.B) {
	dataSizes := []int{32, 64, 128, 256, 512, 1024, 4096, 16384}
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()

	for _, size := range dataSizes {
		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			data := generateTestData(size)
			signature, err := key.Sign(data)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if !pubKey.Verify(data, signature) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// ============================================================================
// Key Operations Benchmarks
// ============================================================================

func BenchmarkPublicKeyFromBytes_Ed25519(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKeyBytes := key.PublicKey().Bytes()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := PublicKeyFromBytes(AlgorithmEd25519, pubKeyBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrivateKeyFromBytes_Ed25519(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	privKeyBytes := key.Bytes()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := PrivateKeyFromBytes(AlgorithmEd25519, privKeyBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPrivateKeyPublicKey_Ed25519(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = key.PublicKey()
	}
}

func BenchmarkPublicKeyEquals_Ed25519(b *testing.B) {
	key1, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	key2, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey1 := key1.PublicKey()
	pubKey2 := key2.PublicKey()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pubKey1.Equals(pubKey2)
	}
}

// BenchmarkZeroize measures the time to securely clear sensitive data.
//
// The Zeroize implementation uses crypto/subtle.XORBytes which XORs each byte
// with itself to produce zeros. This cannot be optimized away by the compiler.
// runtime.KeepAlive is called to ensure the slice isn't considered "dead" after
// zeroing, providing defense in depth against dead-store elimination.
//
// Expected scaling: ~0.15-0.30 ns/byte, 0 allocs/op.
func BenchmarkZeroize(b *testing.B) {
	sizes := []int{32, 64, 128, 256, 512, 1024}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			data := make([]byte, size)
			// Fill with non-zero data to ensure zeroing actually happens
			for i := range data {
				data[i] = byte(i)
			}
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				Zeroize(data)
				// Refill for next iteration (not counted in benchmark time)
				b.StopTimer()
				for j := range data {
					data[j] = byte(j)
				}
				b.StartTimer()
			}
		})
	}
}

// BenchmarkZeroize_PrivateKeySize benchmarks zeroing at Ed25519 private key size (64 bytes).
// This is the most common use case for Zeroize in this library.
func BenchmarkZeroize_PrivateKeySize(b *testing.B) {
	data := make([]byte, 64) // Ed25519 private key size
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		Zeroize(data)
	}
}

// ============================================================================
// Verification Failure Benchmarks (Security-relevant: timing analysis)
// ============================================================================

// BenchmarkVerifyEd25519_InvalidSignature measures verification time for invalid signatures.
//
// SECURITY NOTE: Invalid signatures may verify slightly faster (~10%) than valid ones
// due to Ed25519 implementation details in Go's standard library. This is acceptable
// because:
// 1. The timing difference is not key-dependent (no information about the private key leaks)
// 2. Timing attacks on Ed25519 verification typically target key-dependent variations
// 3. The difference is consistent across all invalid signatures regardless of their content
//
// The all-zeros signature case may exit faster due to early rejection paths in ed25519.Verify,
// but this does not constitute a security-relevant timing side channel.
func BenchmarkVerifyEd25519_InvalidSignature(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()
	data := generateTestData(32)
	invalidSig := make([]byte, 64) // All zeros - invalid
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pubKey.Verify(data, invalidSig)
	}
}

// BenchmarkVerifyEd25519_WrongKey measures verification time when using wrong public key.
func BenchmarkVerifyEd25519_WrongKey(b *testing.B) {
	key1, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	key2, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey2 := key2.PublicKey()
	data := generateTestData(32)
	signature, err := key1.Sign(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pubKey2.Verify(data, signature)
	}
}

// BenchmarkVerifyEd25519_WrongData measures verification time when data doesn't match.
func BenchmarkVerifyEd25519_WrongData(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmEd25519)
	if err != nil {
		b.Fatal(err)
	}
	pubKey := key.PublicKey()
	data1 := generateTestData(32)
	data2 := generateTestData(32)
	signature, err := key.Sign(data1)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = pubKey.Verify(data2, signature)
	}
}

// ============================================================================
// Low-S Normalization Benchmarks
// ============================================================================

// BenchmarkNormalizeLowS_NoChange measures overhead when s is already low-S.
// Expected: <10ns per operation (simple comparison).
func BenchmarkNormalizeLowS_NoChange(b *testing.B) {
	n := p256Order() // P-256 curve order
	halfN := new(big.Int).Rsh(n, 1)
	s := new(big.Int).Sub(halfN, big.NewInt(1)) // Already low-S
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = normalizeLowS(s, n)
	}
}

// BenchmarkNormalizeLowS_NeedsNormalization measures overhead when s > n/2.
// Expected: <50ns per operation (comparison + subtraction).
func BenchmarkNormalizeLowS_NeedsNormalization(b *testing.B) {
	n := p256Order() // P-256 curve order
	halfN := new(big.Int).Rsh(n, 1)
	s := new(big.Int).Add(halfN, big.NewInt(1)) // High-S, needs normalization
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = normalizeLowS(s, n)
	}
}

// BenchmarkIsLowS measures IsLowS check overhead.
// Expected: <20ns per operation.
func BenchmarkIsLowS(b *testing.B) {
	n := p256Order()
	sig := make([]byte, 64)
	sig[63] = 0x01 // Low-S signature
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = IsLowS(sig, n)
	}
}

// BenchmarkSignSecp256r1WithLowS measures total signing time including low-S normalization.
// The overhead from normalization should be negligible (<1%) compared to ECDSA signing.
func BenchmarkSignSecp256r1WithLowS(b *testing.B) {
	key, err := GeneratePrivateKey(AlgorithmSecp256r1)
	if err != nil {
		b.Fatal(err)
	}
	data := generateTestData(32)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := key.Sign(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// p256Order returns the P-256 curve order for benchmarks.
func p256Order() *big.Int {
	n, _ := new(big.Int).SetString("115792089210356248762697446949407573529996955224135760342422259061068512044369", 10)
	return n
}

// ============================================================================
// Algorithm Comparison Benchmarks (Issue #156)
// ============================================================================
// These benchmarks provide direct comparison of sign/verify/keygen throughput
// across all supported algorithms, plus memory allocation analysis for
// high-throughput scenarios (validators signing 1000s of attestations/sec).

// BenchmarkSignThroughput compares signing throughput across algorithms.
// This is the primary benchmark for comparing algorithm performance.
//
// Performance varies significantly by platform. On ARM64 (Apple Silicon),
// Go's native P-256 uses assembly optimizations that outperform other curves:
// - secp256r1: ~90,000-100,000 ops/sec (fastest on ARM64, assembly-optimized)
// - Ed25519: ~70,000-80,000 ops/sec
// - secp256k1: ~40,000-50,000 ops/sec
//
// On x86_64 without assembly optimizations, Ed25519 is typically fastest.
//
// Implementation notes:
// - secp256k1: Uses RFC 6979 deterministic nonce generation via dcrd library
// - secp256r1: Uses RFC 6979 deterministic nonce generation (custom implementation)
// - Both ECDSA curves apply low-S normalization for malleability protection
func BenchmarkSignThroughput(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}
	data := []byte("benchmark message for signing throughput test")

	for _, algo := range algorithms {
		b.Run(algo.String(), func(b *testing.B) {
			key, err := GeneratePrivateKey(algo)
			if err != nil {
				b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := key.Sign(data)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkVerifyThroughput compares signature verification throughput.
//
// Verify operations are typically faster than sign operations because:
// - No nonce generation required
// - Single scalar multiplication (sign requires 2)
//
// Expected results (approximate):
// - Ed25519: ~15,000-25,000 ops/sec
// - secp256k1: ~20,000-40,000 ops/sec
// - secp256r1: ~15,000-30,000 ops/sec
func BenchmarkVerifyThroughput(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}
	data := []byte("benchmark message for verify throughput test")

	for _, algo := range algorithms {
		b.Run(algo.String(), func(b *testing.B) {
			key, err := GeneratePrivateKey(algo)
			if err != nil {
				b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
			}
			pubKey := key.PublicKey()
			sig, err := key.Sign(data)
			if err != nil {
				b.Fatalf("Sign(%s): %v", algo, err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !pubKey.Verify(data, sig) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// BenchmarkKeyGenThroughput compares key generation performance.
//
// Key generation is typically the slowest operation because:
// - Requires secure random number generation
// - ECDSA requires point multiplication to derive public key
//
// Expected results (approximate):
// - Ed25519: ~30,000-50,000 ops/sec (lightweight key derivation)
// - secp256k1: ~15,000-30,000 ops/sec
// - secp256r1: ~10,000-20,000 ops/sec
func BenchmarkKeyGenThroughput(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		b.Run(algo.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key, err := GeneratePrivateKey(algo)
				if err != nil {
					b.Fatal(err)
				}
				// Prevent optimization from eliminating the call
				_ = key.PublicKey().Bytes()
			}
		})
	}
}

// BenchmarkSignBatch simulates high-throughput signing scenarios.
// This is relevant for validators signing multiple attestations per second.
//
// Measures: total time to sign N messages with the same key.
// Batch sizes: 100, 1000, 10000 (typical validator workloads)
func BenchmarkSignBatch(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}
	batchSizes := []int{100, 1000}

	// Pre-generate unique messages
	messages := make([][]byte, 1000)
	for i := range messages {
		messages[i] = []byte(fmt.Sprintf("attestation-%d", i))
	}

	for _, algo := range algorithms {
		key, err := GeneratePrivateKey(algo)
		if err != nil {
			b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
		}

		for _, batchSize := range batchSizes {
			b.Run(fmt.Sprintf("%s/batch_%d", algo, batchSize), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					for j := 0; j < batchSize; j++ {
						_, err := key.Sign(messages[j%len(messages)])
						if err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		}
	}
}

// BenchmarkVerifyBatch simulates high-throughput verification scenarios.
// This is relevant for nodes verifying many signatures in gossip protocols.
func BenchmarkVerifyBatch(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}
	batchSizes := []int{100, 1000}

	// Pre-generate unique messages
	messages := make([][]byte, 1000)
	for i := range messages {
		messages[i] = []byte(fmt.Sprintf("message-%d", i))
	}

	for _, algo := range algorithms {
		key, err := GeneratePrivateKey(algo)
		if err != nil {
			b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
		}
		pubKey := key.PublicKey()

		// Pre-sign all messages
		signatures := make([][]byte, len(messages))
		for i, msg := range messages {
			sig, err := key.Sign(msg)
			if err != nil {
				b.Fatalf("Sign(%s): %v", algo, err)
			}
			signatures[i] = sig
		}

		for _, batchSize := range batchSizes {
			b.Run(fmt.Sprintf("%s/batch_%d", algo, batchSize), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					for j := 0; j < batchSize; j++ {
						if !pubKey.Verify(messages[j%len(messages)], signatures[j%len(signatures)]) {
							b.Fatal("verification failed")
						}
					}
				}
			})
		}
	}
}

// BenchmarkSignMemoryPressure measures memory allocation patterns during
// high-throughput signing. This helps identify GC pressure hotspots.
//
// Runs many sign operations and reports total allocations.
// Ideal: minimal allocations per operation to reduce GC pauses.
//
// Measured allocations per sign (run benchmarks for current values):
// - Ed25519: ~1 alloc (64-byte signature slice)
// - secp256k1: ~29 allocs (dcrd library internals + big.Int operations)
// - secp256r1: ~69 allocs (Go crypto/ecdsa internals + big.Int operations)
func BenchmarkSignMemoryPressure(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}
	data := []byte("memory pressure test message")

	for _, algo := range algorithms {
		b.Run(algo.String(), func(b *testing.B) {
			key, err := GeneratePrivateKey(algo)
			if err != nil {
				b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
			}
			b.ReportAllocs()
			b.ResetTimer()

			// Run many iterations to accumulate allocation stats
			for i := 0; i < b.N; i++ {
				sig, err := key.Sign(data)
				if err != nil {
					b.Fatal(err)
				}
				// Prevent compiler from eliminating allocation
				if len(sig) == 0 {
					b.Fatal("empty signature")
				}
			}
		})
	}
}

// BenchmarkVerifyMemoryPressure measures memory allocation patterns during
// high-throughput verification.
func BenchmarkVerifyMemoryPressure(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}
	data := []byte("memory pressure verify test")

	for _, algo := range algorithms {
		b.Run(algo.String(), func(b *testing.B) {
			key, err := GeneratePrivateKey(algo)
			if err != nil {
				b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
			}
			pubKey := key.PublicKey()
			sig, err := key.Sign(data)
			if err != nil {
				b.Fatalf("Sign(%s): %v", algo, err)
			}
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				result := pubKey.Verify(data, sig)
				if !result {
					b.Fatal("verification failed")
				}
			}
		})
	}
}

// BenchmarkKeyGenMemoryPressure measures memory allocation patterns during
// rapid key generation. Useful for identifying if key pooling would help.
func BenchmarkKeyGenMemoryPressure(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		b.Run(algo.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				key, err := GeneratePrivateKey(algo)
				if err != nil {
					b.Fatal(err)
				}
				// Ensure key is fully materialized
				pub := key.PublicKey()
				if len(pub.Bytes()) == 0 {
					b.Fatal("empty public key")
				}
			}
		})
	}
}

// BenchmarkSignDataSizes compares signing performance across data sizes.
// Shows how signing time scales with message length for each algorithm.
//
// Ed25519 hashes internally, so it scales with data size.
// ECDSA pre-hashes to SHA256, so signing time is constant after hashing.
func BenchmarkSignDataSizes(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}
	sizes := []int{32, 256, 1024, 4096}

	for _, algo := range algorithms {
		key, err := GeneratePrivateKey(algo)
		if err != nil {
			b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
		}

		for _, size := range sizes {
			b.Run(fmt.Sprintf("%s/%d_bytes", algo, size), func(b *testing.B) {
				data := generateTestData(size)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					_, err := key.Sign(data)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkVerifyDataSizes compares verification across data sizes.
func BenchmarkVerifyDataSizes(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}
	sizes := []int{32, 256, 1024, 4096}

	for _, algo := range algorithms {
		key, err := GeneratePrivateKey(algo)
		if err != nil {
			b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
		}
		pubKey := key.PublicKey()

		for _, size := range sizes {
			data := generateTestData(size)
			sig, err := key.Sign(data)
			if err != nil {
				b.Fatalf("Sign(%s): %v", algo, err)
			}

			b.Run(fmt.Sprintf("%s/%d_bytes", algo, size), func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if !pubKey.Verify(data, sig) {
						b.Fatal("verification failed")
					}
				}
			})
		}
	}
}

// BenchmarkPublicKeyFromBytes compares public key parsing performance.
func BenchmarkPublicKeyFromBytes(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		key, err := GeneratePrivateKey(algo)
		if err != nil {
			b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
		}
		pubKeyBytes := key.PublicKey().Bytes()

		b.Run(algo.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := PublicKeyFromBytes(algo, pubKeyBytes)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkPrivateKeyFromBytes compares private key parsing performance.
func BenchmarkPrivateKeyFromBytes(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}

	for _, algo := range algorithms {
		key, err := GeneratePrivateKey(algo)
		if err != nil {
			b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
		}
		privKeyBytes := key.Bytes()

		b.Run(algo.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := PrivateKeyFromBytes(algo, privKeyBytes)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkFullSignVerifyCycle measures the complete sign+verify operation.
// This represents the full cost of securing and verifying a single message.
func BenchmarkFullSignVerifyCycle(b *testing.B) {
	algorithms := []Algorithm{AlgorithmEd25519, AlgorithmSecp256k1, AlgorithmSecp256r1}
	data := []byte("full sign-verify cycle benchmark")

	for _, algo := range algorithms {
		key, err := GeneratePrivateKey(algo)
		if err != nil {
			b.Fatalf("GeneratePrivateKey(%s): %v", algo, err)
		}
		pubKey := key.PublicKey()

		b.Run(algo.String(), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				sig, err := key.Sign(data)
				if err != nil {
					b.Fatal(err)
				}
				if !pubKey.Verify(data, sig) {
					b.Fatal("verification failed")
				}
			}
		})
	}
}
