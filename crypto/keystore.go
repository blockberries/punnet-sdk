package crypto

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// KeyStore error types.
var (
	// ErrKeyStoreNotFound is returned when a key is not found in the store.
	ErrKeyStoreNotFound = errors.New("key not found in store")

	// ErrKeyStoreExists is returned when attempting to store a key that already exists.
	ErrKeyStoreExists = errors.New("key already exists in store")

	// ErrKeyStoreIO is returned when an I/O error occurs during store operations.
	ErrKeyStoreIO = errors.New("key store I/O error")

	// ErrInvalidKeyName is returned when a key name fails validation.
	ErrInvalidKeyName = errors.New("invalid key name")

	// ErrInvalidEncryptionParams is returned when encryption parameters are invalid.
	ErrInvalidEncryptionParams = errors.New("invalid encryption parameters")

	// ErrInvalidAlgorithm is returned when an algorithm is not recognized.
	ErrInvalidAlgorithm = errors.New("invalid algorithm")

	// ErrKeyNameMismatch is returned when the name parameter differs from EncryptedKey.Name.
	ErrKeyNameMismatch = errors.New("key name parameter does not match EncryptedKey.Name")
)

// Cryptographic parameter constants per NIST recommendations.
const (
	// MinSaltLength is the minimum salt length for PBKDF2 (NIST SP 800-132).
	// 16 bytes (128 bits) provides sufficient entropy against rainbow tables.
	MinSaltLength = 16

	// AESGCMNonceLength is the required nonce length for AES-GCM.
	// MUST be exactly 12 bytes (96 bits) per NIST SP 800-38D.
	AESGCMNonceLength = 12

	// MaxKeyNameLength prevents DoS via extremely long names.
	// 256 bytes is sufficient for any reasonable key identifier.
	MaxKeyNameLength = 256
)

// forbiddenKeyNameChars contains characters not allowed in key names.
// Prevents path traversal and filesystem issues.
const forbiddenKeyNameChars = "/\\:*?\"<>|"

// KeyStore provides persistent storage for keys using KeyEntry.
// Implementations must be thread-safe.
// Used by Keyring implementation for simple key management.
//
// CORRECTNESS NOTE: The Keyring relies on KeyStore.Put with overwrite=false
// being atomic with respect to existence checks. This is the linearization
// point for key creation - concurrent Put calls for the same name must
// result in exactly one success and all others returning ErrKeyStoreExists.
type KeyStore interface {
	// Get retrieves a key entry by name.
	// Returns ErrKeyNotFound if key doesn't exist.
	// Complexity: Implementation dependent (O(1) for map, O(log n) for B-tree).
	Get(name string) (*KeyEntry, error)

	// Put stores a key entry.
	// Returns ErrKeyExists if key already exists and overwrite is false.
	// INVARIANT: Put with overwrite=false MUST be atomic with respect to
	// existence check. This is the linearization point for key creation.
	// Complexity: Implementation dependent.
	Put(entry *KeyEntry, overwrite bool) error

	// Delete removes a key entry.
	// Returns ErrKeyNotFound if key doesn't exist.
	// Complexity: Implementation dependent.
	Delete(name string) error

	// List returns all key names.
	// Complexity: O(n) where n is number of keys.
	// Memory: Allocates slice of strings.
	List() ([]string, error)

	// Has returns true if a key exists.
	// More efficient than Get when you don't need the key data.
	// Complexity: Implementation dependent (typically O(1) or O(log n)).
	Has(name string) (bool, error)
}

// EncryptedKeyStore is the interface for encrypted key storage backends.
// Uses EncryptedKey with full encryption parameter support.
//
// Implementations must be safe for concurrent use by multiple goroutines.
// All operations should be atomic - partial writes must not corrupt state.
//
// INVARIANT: For all stored keys k, ValidateKeyName(k.Name) == nil
//
// Performance characteristics vary by implementation:
//   - MemoryKeyStore: O(1) average for all operations
//   - FileKeyStore: O(1) memory ops + O(n) disk I/O where n = key size
//   - EncryptedKeyStore: adds O(key_derivation) for password-based encryption
type EncryptedKeyStore interface {
	// Store saves a key to the store.
	//
	// REQUIREMENTS:
	//   - name MUST be non-empty and pass ValidateKeyName()
	//   - name MUST equal key.Name (prevents lookup/storage mismatch)
	//   - key.Algorithm MUST be valid (pass Algorithm.IsValid())
	//   - If key has encryption params, they MUST pass ValidateEncryptionParams()
	//
	// Returns ErrInvalidKeyName if name fails validation.
	// Returns ErrKeyNameMismatch if name != key.Name.
	// Returns ErrInvalidAlgorithm if algorithm is not recognized.
	// Returns ErrInvalidEncryptionParams if encryption metadata is malformed.
	// Returns ErrKeyStoreExists if a key with the same name already exists.
	// Returns ErrKeyStoreIO if the underlying storage fails.
	Store(name string, key EncryptedKey) error

	// Load retrieves a key from the store.
	// Returns ErrKeyStoreNotFound if no key exists with the given name.
	// Returns ErrKeyStoreIO if the underlying storage fails.
	//
	// The returned EncryptedKey may contain encrypted or plaintext data
	// depending on the storage backend configuration.
	//
	// SECURITY: Caller should call Wipe() on the returned key when done
	// to zero sensitive data from memory.
	Load(name string) (EncryptedKey, error)

	// Delete removes a key from the store.
	// Returns ErrKeyStoreNotFound if no key exists with the given name.
	// Returns ErrKeyStoreIO if the underlying storage fails.
	//
	// SECURITY: Implementations MUST:
	//   1. Zero all byte slices (PrivKeyData, Salt, Nonce) before removal
	//   2. For file backends: overwrite file contents before deletion
	//   3. Call any provided secure deletion facilities
	//
	// Note: Go's GC may retain copies of data. See EncryptedKey.Wipe() docs.
	Delete(name string) error

	// List returns all key names in the store.
	// Returns an empty slice if no keys exist.
	// Returns ErrKeyStoreIO if the underlying storage fails.
	//
	// The returned slice is not guaranteed to be in any particular order.
	// Callers should not modify the returned slice.
	List() ([]string, error)
}

// KeyEntry represents a stored key with metadata.
type KeyEntry struct {
	// Name is the unique identifier for this key.
	Name string `json:"name"`

	// Algorithm is the key's signing algorithm.
	Algorithm Algorithm `json:"algorithm"`

	// PrivateKey is the encrypted or raw private key bytes.
	// For encrypted storage, this contains the ciphertext.
	PrivateKey []byte `json:"private_key"`

	// PublicKey is the public key bytes.
	PublicKey []byte `json:"public_key"`

	// Encrypted indicates whether PrivateKey is encrypted.
	Encrypted bool `json:"encrypted"`
}

// Clone creates a deep copy of the KeyEntry.
// Prevents external mutation of stored keys.
// Complexity: O(n) where n is total byte size.
// Memory: Allocates new slices for all byte fields.
func (e *KeyEntry) Clone() *KeyEntry {
	if e == nil {
		return nil
	}
	clone := &KeyEntry{
		Name:      e.Name,
		Algorithm: e.Algorithm,
		Encrypted: e.Encrypted,
	}
	if e.PrivateKey != nil {
		clone.PrivateKey = make([]byte, len(e.PrivateKey))
		copy(clone.PrivateKey, e.PrivateKey)
	}
	if e.PublicKey != nil {
		clone.PublicKey = make([]byte, len(e.PublicKey))
		copy(clone.PublicKey, e.PublicKey)
	}
	return clone
}

// EncryptedKey represents a stored key that may be encrypted or plaintext.
// For encrypted backends, PrivKeyData contains ciphertext encrypted with AES-GCM.
// For plaintext backends (e.g., in-memory), PrivKeyData contains the raw key bytes.
//
// Memory layout optimized for cache efficiency: frequently accessed fields first.
//
// SECURITY: When finished with an EncryptedKey containing sensitive data,
// call Wipe() to zero out private key material from memory.
type EncryptedKey struct {
	// Name is the unique identifier for this key.
	Name string `json:"name"`

	// Algorithm specifies the cryptographic algorithm for this key.
	Algorithm Algorithm `json:"algorithm"`

	// PubKey contains the public key bytes.
	// Size varies by algorithm: Ed25519=32, secp256k1/secp256r1=33 (compressed).
	PubKey []byte `json:"pub_key"`

	// PrivKeyData contains the private key data.
	// For encrypted storage: AES-GCM ciphertext.
	// For plaintext storage: raw private key bytes.
	// SECURITY: Contains sensitive material. Call Wipe() when done.
	PrivKeyData []byte `json:"priv_key_data"`

	// Salt is the PBKDF2 salt used for key derivation.
	// Nil for plaintext storage.
	// MUST be at least MinSaltLength (16) bytes when present.
	Salt []byte `json:"salt,omitempty"`

	// Nonce is the AES-GCM nonce used for encryption.
	// Nil for plaintext storage.
	// MUST be exactly AESGCMNonceLength (12) bytes when present.
	Nonce []byte `json:"nonce,omitempty"`
}

// IsEncrypted returns true if this key is stored with encryption.
// Returns false for plaintext keys or keys with invalid encryption parameters.
// Use ValidateEncryptionParams() for detailed validation.
// Complexity: O(1)
func (k *EncryptedKey) IsEncrypted() bool {
	return len(k.Salt) >= MinSaltLength && len(k.Nonce) == AESGCMNonceLength
}

// ValidateEncryptionParams checks that encryption parameters are valid.
// Returns nil for valid plaintext keys (no Salt/Nonce) or valid encrypted keys.
// Returns ErrInvalidEncryptionParams for malformed encryption metadata.
//
// Valid states:
//   - Plaintext: Salt == nil && Nonce == nil
//   - Encrypted: len(Salt) >= 16 && len(Nonce) == 12
//
// Complexity: O(1)
func (k *EncryptedKey) ValidateEncryptionParams() error {
	hasSalt := len(k.Salt) > 0
	hasNonce := len(k.Nonce) > 0

	// Plaintext: neither Salt nor Nonce
	if !hasSalt && !hasNonce {
		return nil
	}

	// Encrypted: must have both with correct lengths
	if hasSalt && hasNonce {
		if len(k.Salt) < MinSaltLength {
			return ErrInvalidEncryptionParams
		}
		if len(k.Nonce) != AESGCMNonceLength {
			return ErrInvalidEncryptionParams
		}
		return nil
	}

	// Partial: one present, one missing - invalid
	return ErrInvalidEncryptionParams
}

// Validate performs comprehensive validation of the EncryptedKey.
// Checks algorithm validity and encryption parameters.
// Complexity: O(1)
func (k *EncryptedKey) Validate() error {
	if !k.Algorithm.IsValid() {
		return ErrInvalidAlgorithm
	}
	return k.ValidateEncryptionParams()
}

// Wipe securely zeros all sensitive data in the EncryptedKey.
// Call this when the key is no longer needed to minimize exposure
// of private key material in memory.
//
// IMPORTANT: Go's garbage collector may have already copied this data
// elsewhere in memory. This method zeros the current buffer but cannot
// guarantee all copies are erased. For high-security applications,
// consider using OS-level secure memory facilities.
//
// Complexity: O(n) where n = len(PrivKeyData) + len(Salt) + len(Nonce)
func (k *EncryptedKey) Wipe() {
	// Zero private key data
	for i := range k.PrivKeyData {
		k.PrivKeyData[i] = 0
	}
	// Zero salt
	for i := range k.Salt {
		k.Salt[i] = 0
	}
	// Zero nonce
	for i := range k.Nonce {
		k.Nonce[i] = 0
	}
}

// SafeString returns a string representation suitable for logging.
// Sensitive fields (PrivKeyData, Salt, Nonce) are redacted.
// Complexity: O(1)
func (k *EncryptedKey) SafeString() string {
	encrypted := "plaintext"
	if k.IsEncrypted() {
		encrypted = "encrypted"
	}
	return "EncryptedKey{Name:" + k.Name + ", Algorithm:" + k.Algorithm.String() + ", Storage:" + encrypted + "}"
}

// ValidateKeyName checks that a key name is valid for storage.
// Returns nil if valid, ErrInvalidKeyName if invalid.
//
// Validation rules:
//   - MUST be non-empty
//   - MUST be valid UTF-8
//   - MUST NOT exceed MaxKeyNameLength (256) bytes
//   - MUST NOT contain path traversal sequences (..)
//   - MUST NOT contain filesystem-unsafe characters (:, *, ?, ", <, >, |, /, \)
//   - MUST NOT contain control characters (< 32) or null bytes
//
// Complexity: O(n) where n = len(name)
func ValidateKeyName(name string) error {
	// Check empty
	if name == "" {
		return ErrInvalidKeyName
	}

	// Check length (prevents DoS via extremely long names)
	if len(name) > MaxKeyNameLength {
		return ErrInvalidKeyName
	}

	// Check valid UTF-8
	if !utf8.ValidString(name) {
		return ErrInvalidKeyName
	}

	// Check for path traversal sequences
	if strings.Contains(name, "..") {
		return ErrInvalidKeyName
	}

	// Check for control characters, null bytes, and forbidden characters.
	// This prevents path traversal in file-based backends and filesystem issues.
	for _, r := range name {
		if r < 32 || r == 0 {
			return ErrInvalidKeyName
		}
		if strings.ContainsRune(forbiddenKeyNameChars, r) {
			return ErrInvalidKeyName
		}
	}

	return nil
}
