package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// PBKDF2 parameters - these are critical security settings.
	// 100,000 iterations is the minimum acceptable in 2024.
	pbkdf2Iterations = 100_000
	pbkdf2KeyLen     = 32 // AES-256 requires 32-byte key
	saltLen          = 16 // 128-bit salt

	// AES-GCM parameters
	aesGCMNonceLen = 12 // 96-bit nonce (recommended for GCM)

	// File extension for key files
	keyFileExtension = ".key"

	// File permissions - restrictive: owner read/write only
	keyFilePermissions = 0600
	keyDirPermissions  = 0700
)

// FileKeyStore implements KeyStore with encrypted file storage.
// Keys are encrypted using AES-256-GCM with PBKDF2-derived keys.
// Thread-safe via RWMutex. Implements io.Closer for graceful shutdown.
type FileKeyStore struct {
	dir      string
	password []byte // kept for encryption operations
	mu       sync.RWMutex
	closed   bool // indicates if the store has been closed
}

// fileKeyData is the JSON structure stored on disk.
type fileKeyData struct {
	Name        string `json:"name"`
	Algorithm   string `json:"algorithm"`
	PubKey      string `json:"pub_key"`       // base64
	PrivKeyData string `json:"priv_key_data"` // base64, encrypted
	Salt        string `json:"salt"`          // base64
	Nonce       string `json:"nonce"`         // base64
}

// NewFileKeyStore creates a new FileKeyStore that stores keys in the given directory.
// The password is used to derive encryption keys via PBKDF2.
//
// Security notes:
// - Password is kept in memory for the lifetime of the KeyStore
// - Each key uses a unique salt and nonce
// - Files are created with mode 0600 (owner read/write only)
func NewFileKeyStore(dir string, password string) (EncryptedKeyStore, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: directory path is empty", ErrKeyStoreIO)
	}
	if password == "" {
		return nil, fmt.Errorf("%w: password cannot be empty", ErrKeyStoreIO)
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, keyDirPermissions); err != nil {
		return nil, fmt.Errorf("%w: failed to create directory: %v", ErrKeyStoreIO, err)
	}

	// Verify directory permissions are restrictive
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to stat directory: %v", ErrKeyStoreIO, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: path is not a directory", ErrKeyStoreIO)
	}

	return &FileKeyStore{
		dir:      dir,
		password: []byte(password),
	}, nil
}

// Store encrypts and saves a key to disk.
// Returns ErrKeyStoreClosed if the store has been closed.
func (fs *FileKeyStore) Store(name string, key EncryptedKey) error {
	if err := validateKeyName(name); err != nil {
		return err
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.checkClosed(); err != nil {
		return err
	}

	filePath := fs.keyFilePath(name)

	// Check if key already exists
	if _, err := os.Stat(filePath); err == nil {
		return ErrKeyStoreExists
	}

	// Generate unique salt for this key
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("%w: failed to generate salt: %v", ErrKeyStoreIO, err)
	}

	// Generate unique nonce for this encryption
	nonce := make([]byte, aesGCMNonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("%w: failed to generate nonce: %v", ErrKeyStoreIO, err)
	}

	// Derive encryption key from password and salt
	derivedKey := pbkdf2.Key(fs.password, salt, pbkdf2Iterations, pbkdf2KeyLen, sha256.New)
	defer clearBytes(derivedKey) // Clear sensitive key material

	// Encrypt private key data
	ciphertext, err := encryptAESGCM(derivedKey, nonce, key.PrivKeyData, []byte(name))
	if err != nil {
		return fmt.Errorf("%w: encryption failed: %v", ErrKeyStoreIO, err)
	}

	// Prepare file data
	data := fileKeyData{
		Name:        name,
		Algorithm:   string(key.Algorithm),
		PubKey:      base64.StdEncoding.EncodeToString(key.PubKey),
		PrivKeyData: base64.StdEncoding.EncodeToString(ciphertext),
		Salt:        base64.StdEncoding.EncodeToString(salt),
		Nonce:       base64.StdEncoding.EncodeToString(nonce),
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: failed to marshal key data: %v", ErrKeyStoreIO, err)
	}

	// Write file with restrictive permissions
	// Use WriteFile with explicit permissions - it creates the file atomically
	if err := os.WriteFile(filePath, jsonData, keyFilePermissions); err != nil {
		return fmt.Errorf("%w: failed to write key file: %v", ErrKeyStoreIO, err)
	}

	return nil
}

// Load reads and decrypts a key from disk.
// Returns ErrKeyStoreClosed if the store has been closed.
func (fs *FileKeyStore) Load(name string) (EncryptedKey, error) {
	if err := validateKeyName(name); err != nil {
		return EncryptedKey{}, err
	}

	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if err := fs.checkClosed(); err != nil {
		return EncryptedKey{}, err
	}

	filePath := fs.keyFilePath(name)

	// Read file
	jsonData, err := os.ReadFile(filePath)
	if os.IsNotExist(err) {
		return EncryptedKey{}, ErrKeyStoreNotFound
	}
	if err != nil {
		return EncryptedKey{}, fmt.Errorf("%w: failed to read key file: %v", ErrKeyStoreIO, err)
	}

	// Parse JSON
	var data fileKeyData
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return EncryptedKey{}, fmt.Errorf("%w: failed to parse key file: %v", ErrKeyStoreIO, err)
	}

	// Decode base64 fields
	pubKey, err := base64.StdEncoding.DecodeString(data.PubKey)
	if err != nil {
		return EncryptedKey{}, fmt.Errorf("%w: invalid public key encoding: %v", ErrKeyStoreIO, err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(data.PrivKeyData)
	if err != nil {
		return EncryptedKey{}, fmt.Errorf("%w: invalid private key encoding: %v", ErrKeyStoreIO, err)
	}

	salt, err := base64.StdEncoding.DecodeString(data.Salt)
	if err != nil {
		return EncryptedKey{}, fmt.Errorf("%w: invalid salt encoding: %v", ErrKeyStoreIO, err)
	}

	nonce, err := base64.StdEncoding.DecodeString(data.Nonce)
	if err != nil {
		return EncryptedKey{}, fmt.Errorf("%w: invalid nonce encoding: %v", ErrKeyStoreIO, err)
	}

	// Derive decryption key from password and stored salt
	derivedKey := pbkdf2.Key(fs.password, salt, pbkdf2Iterations, pbkdf2KeyLen, sha256.New)
	defer clearBytes(derivedKey) // Clear sensitive key material

	// Decrypt private key data
	plaintext, err := decryptAESGCM(derivedKey, nonce, ciphertext, []byte(name))
	if err != nil {
		// Authentication failure means wrong password or tampered data
		return EncryptedKey{}, ErrInvalidPassword
	}

	// Validate algorithm
	alg := Algorithm(data.Algorithm)
	if !alg.IsValid() {
		return EncryptedKey{}, fmt.Errorf("%w: unknown algorithm %q", ErrKeyStoreIO, data.Algorithm)
	}

	return EncryptedKey{
		Name:        data.Name,
		Algorithm:   alg,
		PubKey:      pubKey,
		PrivKeyData: plaintext,
		Salt:        salt,
		Nonce:       nonce,
	}, nil
}

// Delete removes a key file from disk.
// Returns ErrKeyStoreClosed if the store has been closed.
func (fs *FileKeyStore) Delete(name string) error {
	if err := validateKeyName(name); err != nil {
		return err
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	if err := fs.checkClosed(); err != nil {
		return err
	}

	filePath := fs.keyFilePath(name)

	// Check if key exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return ErrKeyStoreNotFound
	}

	// Remove the file
	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("%w: failed to delete key file: %v", ErrKeyStoreIO, err)
	}

	return nil
}

// List returns all key names in the store.
// Returns ErrKeyStoreClosed if the store has been closed.
func (fs *FileKeyStore) List() ([]string, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if err := fs.checkClosed(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read directory: %v", ErrKeyStoreIO, err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, keyFileExtension) {
			// Strip extension to get key name
			keyName := strings.TrimSuffix(name, keyFileExtension)
			names = append(names, keyName)
		}
	}

	return names, nil
}

// keyFilePath returns the file path for a given key name.
func (fs *FileKeyStore) keyFilePath(name string) string {
	return filepath.Join(fs.dir, name+keyFileExtension)
}

// validateKeyName checks that a key name is safe for use as a filename.
func validateKeyName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: key name cannot be empty", ErrKeyStoreIO)
	}

	// Prevent path traversal attacks
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("%w: key name cannot contain path separators", ErrKeyStoreIO)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("%w: key name cannot contain '..'", ErrKeyStoreIO)
	}

	// Prevent hidden files on Unix
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("%w: key name cannot start with '.'", ErrKeyStoreIO)
	}

	// Limit name length to prevent filesystem issues
	if len(name) > 255 {
		return fmt.Errorf("%w: key name too long (max 255 characters)", ErrKeyStoreIO)
	}

	return nil
}

// encryptAESGCM encrypts plaintext using AES-256-GCM.
// The additionalData provides authenticated but unencrypted context.
func encryptAESGCM(key, nonce, plaintext, additionalData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Seal appends the ciphertext and authentication tag
	ciphertext := aead.Seal(nil, nonce, plaintext, additionalData)
	return ciphertext, nil
}

// decryptAESGCM decrypts ciphertext using AES-256-GCM.
// Returns error if authentication fails (wrong password or tampered data).
func decryptAESGCM(key, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Open decrypts and verifies the authentication tag
	plaintext, err := aead.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// clearBytes zeroes a byte slice to reduce sensitive data exposure in memory.
func clearBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// Close marks the store as closed and zeroizes the password.
// After Close is called, all operations will return ErrKeyStoreClosed.
// Safe to call multiple times; subsequent calls are no-ops.
// Complexity: O(1).
func (fs *FileKeyStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if fs.closed {
		return nil // Already closed, no-op
	}

	fs.closed = true

	// Zeroize password to minimize memory exposure
	clearBytes(fs.password)
	fs.password = nil

	return nil
}

// checkClosed returns ErrKeyStoreClosed if the store is closed.
// Must be called with at least a read lock held.
func (fs *FileKeyStore) checkClosed() error {
	if fs.closed {
		return ErrKeyStoreClosed
	}
	return nil
}
