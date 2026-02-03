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

// NOTE: secp256k1 and secp256r1 are not yet implemented.
// These benchmarks are provided as stubs for when they are implemented.

func BenchmarkKeyGenSecp256k1(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		key, err := GeneratePrivateKey(AlgorithmSecp256k1)
		if err != nil {
			b.Skip("secp256k1 not yet implemented")
			return
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
			b.Skip("secp256r1 not yet implemented")
			return
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
		b.Skip("secp256k1 not yet implemented")
		return
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
		b.Skip("secp256r1 not yet implemented")
		return
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
		b.Skip("secp256k1 not yet implemented")
		return
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
		b.Skip("secp256r1 not yet implemented")
		return
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
