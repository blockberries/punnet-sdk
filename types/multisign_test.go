package types

import (
	"sync"
	"testing"

	"github.com/blockberries/punnet-sdk/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSignDoc creates a valid SignDoc for testing.
func testSignDoc() *SignDoc {
	sd := NewSignDoc("test-chain", 1, "alice", 1, "test memo")
	sd.AddMessage("/test.msg", []byte(`{"amount":"100"}`))
	return sd
}

func TestNewMultiSignCoordinator(t *testing.T) {
	t.Run("valid SignDoc", func(t *testing.T) {
		sd := testSignDoc()
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)
		assert.NotNil(t, coord)
		assert.Equal(t, sd, coord.SignDoc())
		assert.Equal(t, 0, coord.Count())
	})

	t.Run("nil SignDoc", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(nil)
		assert.Error(t, err)
		assert.Nil(t, coord)
		assert.Contains(t, err.Error(), "signDoc cannot be nil")
	})
}

func TestMultiSignCoordinator_AddSignature(t *testing.T) {
	sd := testSignDoc()
	signBytes, err := sd.GetSignBytes()
	require.NoError(t, err)

	// Generate test keys
	priv1, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)
	priv2, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)

	t.Run("add single signature", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		sigBytes, err := priv1.Sign(signBytes)
		require.NoError(t, err)

		sig := Signature{
			Algorithm: crypto.AlgorithmEd25519,
			PubKey:    priv1.PublicKey().Bytes(),
			Signature: sigBytes,
		}

		err = coord.AddSignature(sig)
		require.NoError(t, err)
		assert.Equal(t, 1, coord.Count())
	})

	t.Run("add multiple signatures", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		sig1Bytes, err := priv1.Sign(signBytes)
		require.NoError(t, err)
		sig2Bytes, err := priv2.Sign(signBytes)
		require.NoError(t, err)

		sig1 := Signature{
			Algorithm: crypto.AlgorithmEd25519,
			PubKey:    priv1.PublicKey().Bytes(),
			Signature: sig1Bytes,
		}
		sig2 := Signature{
			Algorithm: crypto.AlgorithmEd25519,
			PubKey:    priv2.PublicKey().Bytes(),
			Signature: sig2Bytes,
		}

		err = coord.AddSignature(sig1)
		require.NoError(t, err)
		err = coord.AddSignature(sig2)
		require.NoError(t, err)
		assert.Equal(t, 2, coord.Count())
	})

	t.Run("reject duplicate public key", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		sigBytes, err := priv1.Sign(signBytes)
		require.NoError(t, err)

		sig := Signature{
			Algorithm: crypto.AlgorithmEd25519,
			PubKey:    priv1.PublicKey().Bytes(),
			Signature: sigBytes,
		}

		err = coord.AddSignature(sig)
		require.NoError(t, err)

		// Try to add same public key again
		err = coord.AddSignature(sig)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateSignature)
		assert.Equal(t, 1, coord.Count()) // Still only 1 signature
	})

	t.Run("reject invalid signature structure", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		// Invalid: wrong signature size
		invalidSig := Signature{
			Algorithm: crypto.AlgorithmEd25519,
			PubKey:    priv1.PublicKey().Bytes(),
			Signature: []byte("too short"),
		}

		err = coord.AddSignature(invalidSig)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid signature")
		assert.Equal(t, 0, coord.Count())
	})
}

func TestMultiSignCoordinator_SignWithSigner(t *testing.T) {
	sd := testSignDoc()

	priv1, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)
	priv2, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)

	signer1 := crypto.NewSigner(priv1)
	signer2 := crypto.NewSigner(priv2)

	t.Run("sign with single signer", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		err = coord.SignWithSigner(signer1)
		require.NoError(t, err)
		assert.Equal(t, 1, coord.Count())

		// Verify the signature was added correctly
		sigs := coord.Signatures()
		require.Len(t, sigs, 1)
		assert.Equal(t, priv1.PublicKey().Bytes(), sigs[0].PubKey)
	})

	t.Run("sign with multiple signers", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		err = coord.SignWithSigner(signer1)
		require.NoError(t, err)
		err = coord.SignWithSigner(signer2)
		require.NoError(t, err)
		assert.Equal(t, 2, coord.Count())
	})

	t.Run("reject duplicate signer", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		err = coord.SignWithSigner(signer1)
		require.NoError(t, err)

		err = coord.SignWithSigner(signer1)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateSignature)
		assert.Equal(t, 1, coord.Count())
	})

	t.Run("nil signer", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		err = coord.SignWithSigner(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "signer cannot be nil")
	})
}

func TestMultiSignCoordinator_ExportSignDoc(t *testing.T) {
	sd := testSignDoc()
	coord, err := NewMultiSignCoordinator(sd)
	require.NoError(t, err)

	t.Run("export returns valid JSON", func(t *testing.T) {
		jsonBytes, err := coord.ExportSignDoc()
		require.NoError(t, err)
		assert.NotEmpty(t, jsonBytes)

		// Verify it's valid JSON by parsing
		parsed, err := ParseSignDoc(jsonBytes)
		require.NoError(t, err)
		assert.True(t, sd.Equals(parsed))
	})

	t.Run("export is deterministic", func(t *testing.T) {
		json1, err := coord.ExportSignDoc()
		require.NoError(t, err)
		json2, err := coord.ExportSignDoc()
		require.NoError(t, err)
		assert.Equal(t, json1, json2)
	})
}

func TestMultiSignCoordinator_ImportSignature(t *testing.T) {
	sd := testSignDoc()
	signBytes, err := sd.GetSignBytes()
	require.NoError(t, err)

	priv1, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)
	priv2, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)

	t.Run("import valid signature", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		// Remote signer signs the SignDoc
		sigBytes, err := priv1.Sign(signBytes)
		require.NoError(t, err)

		// Import the signature
		err = coord.ImportSignature(priv1.PublicKey(), sigBytes)
		require.NoError(t, err)
		assert.Equal(t, 1, coord.Count())
	})

	t.Run("import multiple valid signatures", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		sig1Bytes, err := priv1.Sign(signBytes)
		require.NoError(t, err)
		sig2Bytes, err := priv2.Sign(signBytes)
		require.NoError(t, err)

		err = coord.ImportSignature(priv1.PublicKey(), sig1Bytes)
		require.NoError(t, err)
		err = coord.ImportSignature(priv2.PublicKey(), sig2Bytes)
		require.NoError(t, err)
		assert.Equal(t, 2, coord.Count())
	})

	t.Run("reject invalid signature", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		// Sign with wrong key's private key but claim it's from priv1
		wrongSigBytes, err := priv2.Sign(signBytes)
		require.NoError(t, err)

		err = coord.ImportSignature(priv1.PublicKey(), wrongSigBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidSignature)
		assert.Equal(t, 0, coord.Count())
	})

	t.Run("reject duplicate public key", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		sigBytes, err := priv1.Sign(signBytes)
		require.NoError(t, err)

		err = coord.ImportSignature(priv1.PublicKey(), sigBytes)
		require.NoError(t, err)

		// Try to import again with same public key
		err = coord.ImportSignature(priv1.PublicKey(), sigBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateSignature)
		assert.Equal(t, 1, coord.Count())
	})

	t.Run("nil public key", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		err = coord.ImportSignature(nil, []byte("some sig"))
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidPublicKey)
	})
}

func TestMultiSignCoordinator_Complete(t *testing.T) {
	sd := testSignDoc()
	signBytes, err := sd.GetSignBytes()
	require.NoError(t, err)

	priv1, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)
	priv2, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)

	signer1 := crypto.NewSigner(priv1)
	signer2 := crypto.NewSigner(priv2)

	t.Run("complete with signatures", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		err = coord.SignWithSigner(signer1)
		require.NoError(t, err)
		err = coord.SignWithSigner(signer2)
		require.NoError(t, err)

		auth := coord.Complete()
		require.NotNil(t, auth)
		assert.Len(t, auth.Signatures, 2)

		// Verify signatures are valid
		err = auth.VerifySignatures(signBytes)
		require.NoError(t, err)
	})

	t.Run("complete with empty signatures", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		auth := coord.Complete()
		require.NotNil(t, auth)
		assert.Len(t, auth.Signatures, 0)
	})

	t.Run("complete returns deep copy", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		err = coord.SignWithSigner(signer1)
		require.NoError(t, err)

		auth1 := coord.Complete()
		auth2 := coord.Complete()

		// Modify auth1's signature
		auth1.Signatures[0].PubKey[0] = 0xFF

		// auth2 should be unaffected
		assert.NotEqual(t, auth1.Signatures[0].PubKey[0], auth2.Signatures[0].PubKey[0])
	})
}

func TestMultiSignCoordinator_Signatures(t *testing.T) {
	sd := testSignDoc()

	priv1, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)

	signer1 := crypto.NewSigner(priv1)

	t.Run("returns deep copy", func(t *testing.T) {
		coord, err := NewMultiSignCoordinator(sd)
		require.NoError(t, err)

		err = coord.SignWithSigner(signer1)
		require.NoError(t, err)

		sigs1 := coord.Signatures()
		sigs2 := coord.Signatures()

		// Save original value before modification
		originalByte := sigs2[0].PubKey[0]

		// Modify sigs1 using XOR to flip bits (guaranteed to change)
		sigs1[0].PubKey[0] ^= 0xFF

		// sigs2 should be unaffected
		assert.NotEqual(t, sigs1[0].PubKey[0], sigs2[0].PubKey[0])
		assert.Equal(t, originalByte, sigs2[0].PubKey[0])

		// Original coordinator should be unaffected
		sigs3 := coord.Signatures()
		assert.Equal(t, originalByte, sigs3[0].PubKey[0])
	})
}

func TestMultiSignCoordinator_Reset(t *testing.T) {
	sd := testSignDoc()

	priv1, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)

	signer1 := crypto.NewSigner(priv1)

	coord, err := NewMultiSignCoordinator(sd)
	require.NoError(t, err)

	err = coord.SignWithSigner(signer1)
	require.NoError(t, err)
	assert.Equal(t, 1, coord.Count())

	coord.Reset()
	assert.Equal(t, 0, coord.Count())

	// Should be able to add signatures again
	err = coord.SignWithSigner(signer1)
	require.NoError(t, err)
	assert.Equal(t, 1, coord.Count())
}

func TestMultiSignCoordinator_RemoteSigningScenario(t *testing.T) {
	// Full scenario: Coordinator collects signatures from remote signers

	// Setup: Create SignDoc
	sd := testSignDoc()

	// Coordinator exports SignDoc
	coord, err := NewMultiSignCoordinator(sd)
	require.NoError(t, err)

	exportedJSON, err := coord.ExportSignDoc()
	require.NoError(t, err)

	// Remote signer 1 receives JSON and signs
	remoteSigner1, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)

	remoteSD1, err := ParseSignDoc(exportedJSON)
	require.NoError(t, err)
	remoteSignBytes1, err := remoteSD1.GetSignBytes()
	require.NoError(t, err)
	remoteSig1, err := remoteSigner1.Sign(remoteSignBytes1)
	require.NoError(t, err)

	// Remote signer 2 receives JSON and signs
	remoteSigner2, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
	require.NoError(t, err)

	remoteSD2, err := ParseSignDoc(exportedJSON)
	require.NoError(t, err)
	remoteSignBytes2, err := remoteSD2.GetSignBytes()
	require.NoError(t, err)
	remoteSig2, err := remoteSigner2.Sign(remoteSignBytes2)
	require.NoError(t, err)

	// Coordinator imports signatures from remote signers
	err = coord.ImportSignature(remoteSigner1.PublicKey(), remoteSig1)
	require.NoError(t, err)
	err = coord.ImportSignature(remoteSigner2.PublicKey(), remoteSig2)
	require.NoError(t, err)

	// Complete authorization
	auth := coord.Complete()
	require.NotNil(t, auth)
	assert.Len(t, auth.Signatures, 2)

	// Verify all signatures are valid
	signBytes, err := sd.GetSignBytes()
	require.NoError(t, err)
	err = auth.VerifySignatures(signBytes)
	require.NoError(t, err)
}

func TestMultiSignCoordinator_ConcurrentAccess(t *testing.T) {
	sd := testSignDoc()
	coord, err := NewMultiSignCoordinator(sd)
	require.NoError(t, err)

	const numSigners = 10
	var wg sync.WaitGroup
	errChan := make(chan error, numSigners)

	// Generate signers
	signers := make([]crypto.Signer, numSigners)
	for i := 0; i < numSigners; i++ {
		priv, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
		require.NoError(t, err)
		signers[i] = crypto.NewSigner(priv)
	}

	// Concurrently sign
	for i := 0; i < numSigners; i++ {
		wg.Add(1)
		go func(signer crypto.Signer) {
			defer wg.Done()
			if err := coord.SignWithSigner(signer); err != nil {
				errChan <- err
			}
		}(signers[i])
	}

	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		t.Errorf("unexpected error: %v", err)
	}

	// All signatures should be collected
	assert.Equal(t, numSigners, coord.Count())

	// Complete and verify
	auth := coord.Complete()
	signBytes, err := sd.GetSignBytes()
	require.NoError(t, err)
	err = auth.VerifySignatures(signBytes)
	require.NoError(t, err)
}

func TestMultiSignCoordinator_3of5MultisigScenario(t *testing.T) {
	// Scenario: 3-of-5 multi-signature where only 3 signers participate

	sd := testSignDoc()

	// Generate 5 signers
	signers := make([]crypto.Signer, 5)
	for i := 0; i < 5; i++ {
		priv, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
		require.NoError(t, err)
		signers[i] = crypto.NewSigner(priv)
	}

	// Coordinator collects signatures from only 3 signers
	coord, err := NewMultiSignCoordinator(sd)
	require.NoError(t, err)

	// Only signers 0, 2, and 4 sign
	err = coord.SignWithSigner(signers[0])
	require.NoError(t, err)
	err = coord.SignWithSigner(signers[2])
	require.NoError(t, err)
	err = coord.SignWithSigner(signers[4])
	require.NoError(t, err)

	assert.Equal(t, 3, coord.Count())

	auth := coord.Complete()
	assert.Len(t, auth.Signatures, 3)

	// All 3 signatures should be valid
	signBytes, err := sd.GetSignBytes()
	require.NoError(t, err)
	err = auth.VerifySignatures(signBytes)
	require.NoError(t, err)
}

// Benchmark: AddSignature performance
func BenchmarkMultiSignCoordinator_AddSignature(b *testing.B) {
	sd := testSignDoc()
	signBytes, err := sd.GetSignBytes()
	if err != nil {
		b.Fatal(err)
	}

	// Pre-generate signatures
	sigs := make([]Signature, b.N)
	for i := 0; i < b.N; i++ {
		priv, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
		if err != nil {
			b.Fatal(err)
		}
		sigBytes, err := priv.Sign(signBytes)
		if err != nil {
			b.Fatal(err)
		}
		sigs[i] = Signature{
			Algorithm: crypto.AlgorithmEd25519,
			PubKey:    priv.PublicKey().Bytes(),
			Signature: sigBytes,
		}
	}

	coord, err := NewMultiSignCoordinator(sd)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Note: Each iteration adds to the same coordinator,
		// so duplicate check cost grows with i
		_ = coord.AddSignature(sigs[i])
	}
}

// Benchmark: ImportSignature with verification
func BenchmarkMultiSignCoordinator_ImportSignature(b *testing.B) {
	sd := testSignDoc()
	signBytes, err := sd.GetSignBytes()
	if err != nil {
		b.Fatal(err)
	}

	// Pre-generate keys and signatures
	type testData struct {
		pubKey   crypto.PublicKey
		sigBytes []byte
	}
	data := make([]testData, b.N)
	for i := 0; i < b.N; i++ {
		priv, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
		if err != nil {
			b.Fatal(err)
		}
		sigBytes, err := priv.Sign(signBytes)
		if err != nil {
			b.Fatal(err)
		}
		data[i] = testData{
			pubKey:   priv.PublicKey(),
			sigBytes: sigBytes,
		}
	}

	coord, err := NewMultiSignCoordinator(sd)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = coord.ImportSignature(data[i].pubKey, data[i].sigBytes)
	}
}

// Benchmark: Complete() operation
func BenchmarkMultiSignCoordinator_Complete(b *testing.B) {
	sd := testSignDoc()
	coord, err := NewMultiSignCoordinator(sd)
	if err != nil {
		b.Fatal(err)
	}

	// Add 10 signatures
	for i := 0; i < 10; i++ {
		priv, err := crypto.GeneratePrivateKey(crypto.AlgorithmEd25519)
		if err != nil {
			b.Fatal(err)
		}
		signer := crypto.NewSigner(priv)
		if err := coord.SignWithSigner(signer); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = coord.Complete()
	}
}
