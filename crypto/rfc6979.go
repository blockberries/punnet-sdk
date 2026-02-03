package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"math/big"
)

// rfc6979Nonce generates a deterministic nonce k according to RFC 6979.
// This ensures that signing the same message with the same key always produces
// the same signature, eliminating the need for entropy at sign time.
//
// Parameters:
//   - privKey: the private key scalar d (must be in range [1, n-1])
//   - hash: the message digest (SHA-256 of the data to sign)
//   - n: the curve order
//
// Returns a deterministic k value suitable for ECDSA signing.
//
// RFC 6979 Section 3.2 specifies this algorithm:
//  1. Set h1 = H(m) (already provided as hash parameter)
//  2. Set V = 0x01 0x01 ... 0x01 (32 bytes of 0x01)
//  3. Set K = 0x00 0x00 ... 0x00 (32 bytes of 0x00)
//  4. K = HMAC_K(V || 0x00 || int2octets(x) || bits2octets(h1))
//  5. V = HMAC_K(V)
//  6. K = HMAC_K(V || 0x01 || int2octets(x) || bits2octets(h1))
//  7. V = HMAC_K(V)
//  8. Loop until valid k is found
//
// Complexity: O(1) expected (typically finds valid k in first iteration)
// Memory: ~256 bytes for HMAC state and temporary buffers
func rfc6979Nonce(privKey *big.Int, hash []byte, n *big.Int) *big.Int {
	// qLen is the byte length of the curve order
	qLen := (n.BitLen() + 7) / 8

	// Convert private key to fixed-size byte representation
	x := int2octets(privKey, qLen)

	// bits2octets: reduce hash modulo n and convert to octets
	h := bits2octets(hash, n, qLen)

	// Step b: V = 0x01 0x01 ... 0x01 (qLen bytes)
	v := make([]byte, 32) // SHA-256 output size
	for i := range v {
		v[i] = 0x01
	}

	// Step c: K = 0x00 0x00 ... 0x00 (qLen bytes)
	k := make([]byte, 32)

	// Step d: K = HMAC_K(V || 0x00 || int2octets(x) || bits2octets(h1))
	mac := hmac.New(sha256.New, k)
	mac.Write(v)
	mac.Write([]byte{0x00})
	mac.Write(x)
	mac.Write(h)
	k = mac.Sum(nil)

	// Step e: V = HMAC_K(V)
	mac = hmac.New(sha256.New, k)
	mac.Write(v)
	v = mac.Sum(nil)

	// Step f: K = HMAC_K(V || 0x01 || int2octets(x) || bits2octets(h1))
	mac = hmac.New(sha256.New, k)
	mac.Write(v)
	mac.Write([]byte{0x01})
	mac.Write(x)
	mac.Write(h)
	k = mac.Sum(nil)

	// Step g: V = HMAC_K(V)
	mac = hmac.New(sha256.New, k)
	mac.Write(v)
	v = mac.Sum(nil)

	// Step h: Generate candidate k values until valid
	for {
		// Generate T = empty, then fill until we have qLen bits
		t := make([]byte, 0, qLen)
		for len(t) < qLen {
			mac = hmac.New(sha256.New, k)
			mac.Write(v)
			v = mac.Sum(nil)
			t = append(t, v...)
		}

		// Convert T to integer (bits2int operation)
		kCandidate := bits2int(t[:qLen], n)

		// Check if k is valid: 1 <= k < n
		if kCandidate.Sign() > 0 && kCandidate.Cmp(n) < 0 {
			return kCandidate
		}

		// Invalid k, update K and V and try again
		mac = hmac.New(sha256.New, k)
		mac.Write(v)
		mac.Write([]byte{0x00})
		k = mac.Sum(nil)

		mac = hmac.New(sha256.New, k)
		mac.Write(v)
		v = mac.Sum(nil)
	}
}

// int2octets converts a non-negative integer to a fixed-size byte sequence.
// The output has length rLen = ceil(qLen) bytes, with leading zero padding if needed.
func int2octets(x *big.Int, rLen int) []byte {
	b := x.Bytes()
	if len(b) >= rLen {
		return b[:rLen]
	}
	result := make([]byte, rLen)
	copy(result[rLen-len(b):], b)
	return result
}

// bits2octets converts a hash to a fixed-size byte sequence, reduced modulo n.
// This implements Section 2.3.4 of RFC 6979.
func bits2octets(hash []byte, n *big.Int, rLen int) []byte {
	z := bits2int(hash, n)
	if z.Cmp(n) >= 0 {
		z.Sub(z, n)
	}
	return int2octets(z, rLen)
}

// bits2int converts a byte sequence to a non-negative integer.
// If the input has more bits than the curve order, the leftmost bits are used.
func bits2int(b []byte, n *big.Int) *big.Int {
	z := new(big.Int).SetBytes(b)
	// If the bit length of the hash exceeds the curve order bit length,
	// we right-shift to use only the leftmost bits
	excess := len(b)*8 - n.BitLen()
	if excess > 0 {
		z.Rsh(z, uint(excess))
	}
	return z
}
