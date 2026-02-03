package crypto

import "errors"

// KeyStore errors
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

	// ErrInvalidPassword is returned when decryption fails due to wrong password.
	ErrInvalidPassword = errors.New("invalid password")

	// ErrKeyStoreClosed is returned when operations are attempted on a closed store.
	ErrKeyStoreClosed = errors.New("key store is closed")

	// ErrKeychainUnavailable is returned when the OS keychain cannot be accessed.
	// Common causes:
	//   - Linux: D-Bus not running, or no secret service daemon (gnome-keyring, ksecretservice)
	//   - Headless environments: No GUI session for authentication prompts
	ErrKeychainUnavailable = errors.New("keychain unavailable")
)
