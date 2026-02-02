package crypto

import "errors"

// KeyStore error types.
var (
	// ErrKeyStoreNotFound is returned when a key is not found in the store.
	ErrKeyStoreNotFound = errors.New("key not found in store")

	// ErrKeyStoreExists is returned when attempting to store a key that already exists.
	ErrKeyStoreExists = errors.New("key already exists in store")

	// ErrKeyStoreIO is returned when an I/O error occurs during store operations.
	ErrKeyStoreIO = errors.New("key store I/O error")
)

// EncryptedKey represents a stored key that may be encrypted or plaintext.
// For encrypted backends, PrivKeyData contains ciphertext encrypted with AES-GCM.
// For plaintext backends (e.g., in-memory), PrivKeyData contains the raw key bytes.
//
// Memory layout optimized for cache efficiency: frequently accessed fields first.
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
	PrivKeyData []byte `json:"priv_key_data"`

	// Salt is the PBKDF2 salt used for key derivation.
	// Nil for plaintext storage.
	Salt []byte `json:"salt,omitempty"`

	// Nonce is the AES-GCM nonce used for encryption.
	// Nil for plaintext storage.
	Nonce []byte `json:"nonce,omitempty"`
}

// IsEncrypted returns true if this key is stored with encryption.
// Complexity: O(1)
func (k *EncryptedKey) IsEncrypted() bool {
	return len(k.Salt) > 0 && len(k.Nonce) > 0
}

// KeyStore is the interface for key storage backends.
//
// Implementations must be safe for concurrent use by multiple goroutines.
// All operations should be atomic - partial writes must not corrupt state.
//
// Performance characteristics vary by implementation:
//   - MemoryKeyStore: O(1) average for all operations
//   - FileKeyStore: O(1) memory ops + O(n) disk I/O where n = key size
//   - EncryptedKeyStore: adds O(key_derivation) for password-based encryption
type KeyStore interface {
	// Store saves a key to the store.
	// Returns ErrKeyStoreExists if a key with the same name already exists.
	// Returns ErrKeyStoreIO if the underlying storage fails.
	//
	// Implementations should validate that the key name is non-empty.
	Store(name string, key EncryptedKey) error

	// Load retrieves a key from the store.
	// Returns ErrKeyStoreNotFound if no key exists with the given name.
	// Returns ErrKeyStoreIO if the underlying storage fails.
	//
	// The returned EncryptedKey may contain encrypted or plaintext data
	// depending on the storage backend configuration.
	Load(name string) (EncryptedKey, error)

	// Delete removes a key from the store.
	// Returns ErrKeyStoreNotFound if no key exists with the given name.
	// Returns ErrKeyStoreIO if the underlying storage fails.
	//
	// Implementations should ensure the key is securely wiped from storage.
	Delete(name string) error

	// List returns all key names in the store.
	// Returns an empty slice if no keys exist.
	// Returns ErrKeyStoreIO if the underlying storage fails.
	//
	// The returned slice is not guaranteed to be in any particular order.
	// Callers should not modify the returned slice.
	List() ([]string, error)
}
