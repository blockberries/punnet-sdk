package crypto

// EncryptedKey represents a stored key (may be encrypted or plaintext).
type EncryptedKey struct {
	// Name is the identifier for this key.
	Name string `json:"name"`

	// Algorithm is the signing algorithm.
	Algorithm Algorithm `json:"algorithm"`

	// PubKey is the public key bytes.
	PubKey []byte `json:"pub_key"`

	// PrivKeyData contains the private key data.
	// For encrypted stores, this is the ciphertext.
	// For plaintext stores (like in-memory), this is the raw key.
	PrivKeyData []byte `json:"priv_key_data"`

	// Salt is the PBKDF2 salt (only for encrypted stores).
	Salt []byte `json:"salt,omitempty"`

	// Nonce is the AES-GCM nonce (only for encrypted stores).
	Nonce []byte `json:"nonce,omitempty"`
}

// KeyStore is the interface for key storage backends.
type KeyStore interface {
	// Store saves a key to the store.
	// Returns ErrKeyStoreExists if a key with the same name already exists.
	Store(name string, key EncryptedKey) error

	// Load retrieves a key from the store.
	// Returns ErrKeyStoreNotFound if the key does not exist.
	Load(name string) (EncryptedKey, error)

	// Delete removes a key from the store.
	// Returns ErrKeyStoreNotFound if the key does not exist.
	Delete(name string) error

	// List returns all key names in the store.
	List() ([]string, error)
}
