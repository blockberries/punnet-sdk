package crypto

import "errors"

// KeyStore errors
var (
	// ErrKeyStoreNotFound is returned when a key is not found in the store.
	ErrKeyStoreNotFound = errors.New("key not found in store")

	// ErrKeyStoreExists is returned when attempting to store a key that already exists.
	ErrKeyStoreExists = errors.New("key already exists in store")

	// ErrKeyStoreIO is returned for key store I/O errors.
	ErrKeyStoreIO = errors.New("key store I/O error")

	// ErrInvalidPassword is returned when decryption fails due to wrong password.
	ErrInvalidPassword = errors.New("invalid password")
)
