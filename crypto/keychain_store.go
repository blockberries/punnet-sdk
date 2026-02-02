package crypto

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

const (
	// keychainListKey is a special key name used to track all stored key names.
	// This is necessary because most keychains don't support listing all items
	// for a service, so we maintain our own list.
	keychainListKey = "__key_list__"

	// Maximum key name length for keychain compatibility.
	// Some keychains have limits on account name length.
	maxKeychainKeyNameLen = 200
)

// KeychainStore implements KeyStore using the operating system's native keychain.
//
// Platform support:
//   - macOS: Keychain (via Security framework)
//   - Windows: Credential Store
//   - Linux: Secret Service (libsecret/GNOME Keyring)
//
// Security properties:
//   - Keys are stored in plaintext within the keychain (OS handles encryption)
//   - OS-level access control protects stored keys
//   - No application-level encryption (the keychain is trusted)
//   - Hardware-backed security when available (Secure Enclave, TPM)
type KeychainStore struct {
	serviceName string
	mu          sync.RWMutex
}

// keychainKeyData is the JSON structure stored in the keychain.
// Unlike FileKeyStore, we don't need Salt/Nonce since the keychain
// handles encryption.
type keychainKeyData struct {
	Name        string `json:"name"`
	Algorithm   string `json:"algorithm"`
	PubKey      []byte `json:"pub_key"`       // Raw bytes (JSON will base64 encode)
	PrivKeyData []byte `json:"priv_key_data"` // Raw private key bytes
}

// NewKeychainStore creates a new KeychainStore using the OS keychain.
//
// The serviceName is used as the keychain service identifier. All keys
// will be stored under this service name, typically "punnet-sdk" or
// an application-specific identifier.
//
// Returns an error if:
//   - serviceName is empty
//   - serviceName contains invalid characters
//   - The keychain is unavailable
func NewKeychainStore(serviceName string) (KeyStore, error) {
	if err := validateServiceName(serviceName); err != nil {
		return nil, err
	}

	// Test keychain availability by attempting a no-op operation.
	// We try to get a key that likely doesn't exist - if the keychain
	// itself is unavailable, we'll get a different error than "not found".
	_, err := keyring.Get(serviceName, keychainListKey)
	if err != nil && !isNotFoundError(err) {
		return nil, fmt.Errorf("%w: keychain unavailable: %v", ErrKeyStoreIO, err)
	}

	return &KeychainStore{
		serviceName: serviceName,
	}, nil
}

// Store saves a key to the OS keychain.
func (ks *KeychainStore) Store(name string, key EncryptedKey) error {
	if err := validateKeychainKeyName(name); err != nil {
		return err
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Check if key already exists
	_, err := keyring.Get(ks.serviceName, name)
	if err == nil {
		return ErrKeyStoreExists
	}
	if !isNotFoundError(err) {
		return fmt.Errorf("%w: failed to check existing key: %v", ErrKeyStoreIO, err)
	}

	// Prepare key data for storage
	data := keychainKeyData{
		Name:        name,
		Algorithm:   string(key.Algorithm),
		PubKey:      key.PubKey,
		PrivKeyData: key.PrivKeyData,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal key data: %v", ErrKeyStoreIO, err)
	}

	// Store in keychain
	if err := keyring.Set(ks.serviceName, name, string(jsonData)); err != nil {
		return fmt.Errorf("%w: failed to store key in keychain: %v", ErrKeyStoreIO, err)
	}

	// Update the key list
	if err := ks.addToKeyList(name); err != nil {
		// Attempt to clean up the stored key
		_ = keyring.Delete(ks.serviceName, name)
		return fmt.Errorf("%w: failed to update key list: %v", ErrKeyStoreIO, err)
	}

	return nil
}

// Load retrieves a key from the OS keychain.
func (ks *KeychainStore) Load(name string) (EncryptedKey, error) {
	if err := validateKeychainKeyName(name); err != nil {
		return EncryptedKey{}, err
	}

	ks.mu.RLock()
	defer ks.mu.RUnlock()

	// Get from keychain
	secret, err := keyring.Get(ks.serviceName, name)
	if isNotFoundError(err) {
		return EncryptedKey{}, ErrKeyStoreNotFound
	}
	if err != nil {
		return EncryptedKey{}, fmt.Errorf("%w: failed to load key from keychain: %v", ErrKeyStoreIO, err)
	}

	// Parse JSON
	var data keychainKeyData
	if err := json.Unmarshal([]byte(secret), &data); err != nil {
		return EncryptedKey{}, fmt.Errorf("%w: failed to parse key data: %v", ErrKeyStoreIO, err)
	}

	// Validate algorithm
	alg := Algorithm(data.Algorithm)
	if !alg.IsValid() {
		return EncryptedKey{}, fmt.Errorf("%w: unknown algorithm %q", ErrKeyStoreIO, data.Algorithm)
	}

	// Return EncryptedKey without Salt/Nonce (not needed for keychain storage)
	return EncryptedKey{
		Name:        data.Name,
		Algorithm:   alg,
		PubKey:      data.PubKey,
		PrivKeyData: data.PrivKeyData,
		// Salt and Nonce are nil - keychain handles encryption
	}, nil
}

// Delete removes a key from the OS keychain.
func (ks *KeychainStore) Delete(name string) error {
	if err := validateKeychainKeyName(name); err != nil {
		return err
	}

	ks.mu.Lock()
	defer ks.mu.Unlock()

	// Check if key exists
	_, err := keyring.Get(ks.serviceName, name)
	if isNotFoundError(err) {
		return ErrKeyStoreNotFound
	}
	if err != nil {
		return fmt.Errorf("%w: failed to check key existence: %v", ErrKeyStoreIO, err)
	}

	// Delete from keychain
	if err := keyring.Delete(ks.serviceName, name); err != nil {
		return fmt.Errorf("%w: failed to delete key from keychain: %v", ErrKeyStoreIO, err)
	}

	// Remove from key list
	if err := ks.removeFromKeyList(name); err != nil {
		// Log but don't fail - the key is already deleted
		// In production, this should be logged properly
		_ = err
	}

	return nil
}

// List returns all key names stored in the keychain for this service.
func (ks *KeychainStore) List() ([]string, error) {
	ks.mu.RLock()
	defer ks.mu.RUnlock()

	return ks.getKeyList()
}

// getKeyList retrieves the list of stored key names.
func (ks *KeychainStore) getKeyList() ([]string, error) {
	listData, err := keyring.Get(ks.serviceName, keychainListKey)
	if isNotFoundError(err) {
		// No keys stored yet
		return []string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get key list: %v", ErrKeyStoreIO, err)
	}

	var names []string
	if err := json.Unmarshal([]byte(listData), &names); err != nil {
		return nil, fmt.Errorf("%w: failed to parse key list: %v", ErrKeyStoreIO, err)
	}

	return names, nil
}

// addToKeyList adds a key name to the stored list.
// Must be called with the write lock held.
func (ks *KeychainStore) addToKeyList(name string) error {
	names, err := ks.getKeyList()
	if err != nil {
		return err
	}

	// Check if already in list (shouldn't happen, but be defensive)
	for _, n := range names {
		if n == name {
			return nil
		}
	}

	names = append(names, name)

	listData, err := json.Marshal(names)
	if err != nil {
		return fmt.Errorf("failed to marshal key list: %w", err)
	}

	return keyring.Set(ks.serviceName, keychainListKey, string(listData))
}

// removeFromKeyList removes a key name from the stored list.
// Must be called with the write lock held.
func (ks *KeychainStore) removeFromKeyList(name string) error {
	names, err := ks.getKeyList()
	if err != nil {
		return err
	}

	// Find and remove the name
	filtered := make([]string, 0, len(names))
	for _, n := range names {
		if n != name {
			filtered = append(filtered, n)
		}
	}

	listData, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Errorf("failed to marshal key list: %w", err)
	}

	return keyring.Set(ks.serviceName, keychainListKey, string(listData))
}

// validateServiceName checks that a service name is valid for keychain use.
func validateServiceName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: service name cannot be empty", ErrKeyStoreIO)
	}

	// Prevent overly long service names
	if len(name) > 100 {
		return fmt.Errorf("%w: service name too long (max 100 characters)", ErrKeyStoreIO)
	}

	// Allow alphanumeric, hyphen, underscore, and dot
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return fmt.Errorf("%w: service name contains invalid character: %q", ErrKeyStoreIO, r)
		}
	}

	return nil
}

// validateKeychainKeyName checks that a key name is valid for keychain use.
func validateKeychainKeyName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: key name cannot be empty", ErrKeyStoreIO)
	}

	// Prevent reserved names
	if name == keychainListKey {
		return fmt.Errorf("%w: key name is reserved", ErrKeyStoreIO)
	}

	// Prevent path traversal (defensive, though keychain shouldn't be vulnerable)
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("%w: key name cannot contain path separators", ErrKeyStoreIO)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: key name cannot contain '..'", ErrKeyStoreIO)
	}

	// Prevent hidden key names
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("%w: key name cannot start with '.'", ErrKeyStoreIO)
	}

	// Limit name length
	if len(name) > maxKeychainKeyNameLen {
		return fmt.Errorf("%w: key name too long (max %d characters)", ErrKeyStoreIO, maxKeychainKeyNameLen)
	}

	return nil
}

// isNotFoundError checks if an error indicates the key was not found.
// This is necessary because go-keyring returns different error types
// on different platforms.
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// Check for common "not found" error messages across platforms
	notFoundPatterns := []string{
		"secret not found",       // Linux (libsecret)
		"not found",              // Generic
		"The specified item could not be found", // macOS
		"Element not found",      // Windows
		"item not found",         // Alternative macOS
		keyring.ErrNotFound.Error(), // go-keyring's own error
	}

	for _, pattern := range notFoundPatterns {
		if strings.Contains(strings.ToLower(errStr), strings.ToLower(pattern)) {
			return true
		}
	}

	// Also check if it's the exact ErrNotFound from go-keyring
	if err == keyring.ErrNotFound {
		return true
	}

	return false
}

// Compile-time verification that KeychainStore implements KeyStore
var _ KeyStore = (*KeychainStore)(nil)

// clearKeychainKeyData zeroes sensitive data in the struct.
// This is a defensive measure - while the keychain handles encryption,
// we still want to minimize the time sensitive data spends in memory.
func clearKeychainKeyData(data *keychainKeyData) {
	if data == nil {
		return
	}
	for i := range data.PrivKeyData {
		data.PrivKeyData[i] = 0
	}
	for i := range data.PubKey {
		data.PubKey[i] = 0
	}
	data.Name = ""
	data.Algorithm = ""
}
