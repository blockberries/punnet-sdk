package crypto

import (
	"crypto/sha256"
	"testing"
)

// BenchmarkIsLowSForAlgorithm measures the cost of checking signature form.
// Complexity: O(1) - single big.Int comparison
func BenchmarkIsLowSForAlgorithm(b *testing.B) {
	seed := sha256.Sum256([]byte("benchmark-key"))
	key, _ := PrivateKeyFromBytes(AlgorithmSecp256k1, seed[:])
	sig, _ := key.Sign([]byte("benchmark message"))

	b.Run("secp256k1", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			IsLowSForAlgorithm(sig, AlgorithmSecp256k1)
		}
	})

	key2, _ := GeneratePrivateKey(AlgorithmSecp256r1)
	sig2, _ := key2.Sign([]byte("benchmark message"))

	b.Run("secp256r1", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			IsLowSForAlgorithm(sig2, AlgorithmSecp256r1)
		}
	})
}

// BenchmarkNormalizeSignature measures the cost of signature normalization.
// This is the overhead added to every Sign() operation for secp256r1.
func BenchmarkNormalizeSignature(b *testing.B) {
	seed := sha256.Sum256([]byte("benchmark-key"))
	key, _ := PrivateKeyFromBytes(AlgorithmSecp256k1, seed[:])
	sig, _ := key.Sign([]byte("benchmark message"))

	// Create a high-S signature to force normalization work
	highS := MakeHighS(sig, AlgorithmSecp256k1)

	b.Run("secp256k1/high_to_low", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			NormalizeSignature(highS, AlgorithmSecp256k1)
		}
	})

	b.Run("secp256k1/already_low", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			NormalizeSignature(sig, AlgorithmSecp256k1)
		}
	})

	key2, _ := GeneratePrivateKey(AlgorithmSecp256r1)
	sig2, _ := key2.Sign([]byte("benchmark message"))
	highS2 := MakeHighS(sig2, AlgorithmSecp256r1)

	b.Run("secp256r1/high_to_low", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			NormalizeSignature(highS2, AlgorithmSecp256r1)
		}
	})

	b.Run("secp256r1/already_low", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			NormalizeSignature(sig2, AlgorithmSecp256r1)
		}
	})
}

// BenchmarkSignWithNormalization compares signing performance.
// secp256k1 uses dcrd's built-in low-S; secp256r1 normalizes afterward.
func BenchmarkSignWithNormalization(b *testing.B) {
	seed := sha256.Sum256([]byte("benchmark-key"))
	message := []byte("benchmark message for signing")

	b.Run("secp256k1", func(b *testing.B) {
		key, _ := PrivateKeyFromBytes(AlgorithmSecp256k1, seed[:])
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key.Sign(message)
		}
	})

	b.Run("secp256r1", func(b *testing.B) {
		key, _ := GeneratePrivateKey(AlgorithmSecp256r1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			key.Sign(message)
		}
	})
}
