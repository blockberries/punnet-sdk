package crypto

// KeyStore provides persistent storage for keys.
// Implementations must be thread-safe.
type KeyStore interface {
	// Get retrieves a key entry by name.
	// Returns ErrKeyNotFound if key doesn't exist.
	// Complexity: Implementation dependent (O(1) for map, O(log n) for B-tree).
	Get(name string) (*KeyEntry, error)

	// Put stores a key entry.
	// Returns ErrKeyExists if key already exists and overwrite is false.
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
