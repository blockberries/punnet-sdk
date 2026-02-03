package crypto

// PublicKey represents a public key for any supported algorithm.
type PublicKey struct {
	Algorithm Algorithm
	Bytes     []byte
}

// PrivateKey represents a private key for any supported algorithm.
// Private keys should be handled with care and cleared from memory when no longer needed.
type PrivateKey struct {
	Algorithm Algorithm
	Bytes     []byte
}

// Clear zeroes the private key bytes to reduce exposure in memory.
// This should be called when the key is no longer needed.
func (pk *PrivateKey) Clear() {
	for i := range pk.Bytes {
		pk.Bytes[i] = 0
	}
}
