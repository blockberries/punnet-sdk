# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Breaking Changes

- **BREAKING**: Remove deprecated `Transaction.GetSignBytes()` method (#64)
  - Method was vulnerable to cross-chain replay attacks (missing `chainID` in signed bytes)
  - All code should use the canonical `SignDoc.GetSignBytes()` method instead
  - Zero callers found in codebase; safe removal with no migration needed

- **BREAKING**: Change `Transaction.ToSignDoc()` return signature to include error (#58)
  - Old: `ToSignDoc(chainID string, nonce uint64) *SignDoc`
  - New: `ToSignDoc(chainID string, nonce uint64) (*SignDoc, error)`
  - Required to properly propagate serialization failures from `SignDocSerializable` messages
  - All callers must handle the new error return

### Added

- Add SignDoc migration guide documenting transition from binary to JSON signing (#174)
  - Migration overview and rationale (human auditability, hardware wallet support)
  - Breaking changes and version transition plan
  - Client and validator migration guides with code examples
  - API changes and troubleshooting documentation
  - See `docs/migration/SIGNDOC_MIGRATION.md`
- Add `SignDocSerializable` interface for proper message serialization (#58)
  - Messages implementing this interface provide full canonical representation in SignDoc
  - Prevents signature reuse attacks with different message parameters
  - Backwards-compatible fallback for non-implementing messages
- Add `Signer` interface and `SignSignDoc` function (#169)
  - Unified signing abstraction across all key algorithms
  - Support for ed25519, secp256k1, and secp256r1 algorithms
- Add secp256k1 and secp256r1 key algorithm support (#148, #150)
  - Full `PublicKey` and `PrivateKey` implementations
  - RFC 6979 deterministic signing for secp256k1
  - Low-S signature normalization for ECDSA algorithms
- Add signature normalization utilities for ECDSA (#167)
  - `IsLowS()` and `NormalizeLowS()` functions
  - Ensures BIP-62 / BIP-146 compliance
- Add RFC 6979 deterministic signing for secp256r1 (#165)
  - Consistent with secp256k1 implementation
  - Eliminates need for secure random number generator during signing
- Add `PublicKey`/`PrivateKey` interfaces with JSON marshaling (#168)
  - `SerializablePublicKey` type for cross-algorithm serialization
  - Algorithm auto-detection during deserialization
- Add `CachingKeyStore` read-through caching wrapper (#143)
  - LRU-based eviction with configurable cache capacity
  - Write-through semantics for consistency
  - Manual invalidation via `Invalidate()` and `InvalidateAll()`
  - Cache hit/miss statistics via `Stats()` method
  - ~182,000x speedup for cache hits vs FileKeyStore backend
- Add OS keychain `KeychainStore` backend (#134)
  - Secure credential storage using platform keychain (macOS Keychain, Windows Credential Manager, Linux Secret Service)
  - Index repair functionality for corrupted key indices (#139)
- Add in-memory `EncryptedKeyStore` backend (#137)
  - Implements `EncryptedKeyStore` interface for testing and ephemeral use
  - Password-protected key storage with proper zeroization
- Add `Keyring.Close()` with error aggregation (#113)
  - Proper resource cleanup and key zeroization
  - Aggregates errors from underlying store close operations
- Add closed state tracking to `FileKeyStore` (#128)
  - Consistent with `Keyring` and `MemoryStore` lifecycle patterns
- Add SignDoc validation for auth module (#135)
  - Validates `SignDoc` structure before processing
  - Supports versioned SignDoc formats
- Add Unicode NFC normalization validation for SignDoc (#116)
  - Ensures consistent normalization across implementations
  - Prevents signature verification failures from encoding differences
- Add deprecation logging for signers-only SignDoc fallback (#107)
  - Warns when messages don't implement `SignDocSerializable`
  - Helps identify migration needs
- Add duplicate denom validation to `Fee.ValidateBasic()` (#125)
  - Prevents fee manipulation through duplicate denominations
- Add test vectors for secp256k1 and secp256r1 algorithms (#144)
  - Key derivation vectors from deterministic seed
  - Signature generation vectors using RFC 6979 deterministic signatures
- Add test vectors for nil vs empty value serialization (#141)
  - Test vectors for null/empty string memo handling
  - Test vectors for null/empty array fee amount handling
- Add malformed signature security test vectors (#173)
  - Tests for truncated, extended, and malformed signatures
  - Ensures proper rejection of invalid signature formats
- Add cross-implementation validation for test vectors (#171)
  - Python reference implementation for RFC 6979 validation
  - Cross-language signature verification
- Support for 65-byte uncompressed secp256k1 public keys (#170)
  - Backwards compatibility with legacy key formats

### Fixed

- Fix `CurveOrder()`/`HalfCurveOrder()` returning mutable `*big.Int` pointers (#185)
  - Functions now return defensive copies instead of pointers to package-level variables
  - Prevents potential global state corruption if callers accidentally mutate the returned value
  - Small allocation cost (~32 bytes per call) justified by safety benefit
- Fix data race in `MemoryStore.Get()` during concurrent delete (#127)
  - Hold read lock during entire `Clone()` operation
  - Prevents `Zeroize()` from racing with copy
- Hold read lock during `Clone()` in `MemoryStore.Get` (#130)
  - Additional race condition fix for concurrent access
- Handle errcheck warnings for deferred `Close()`/`Flush()` calls (#103)
  - Proper error handling for cleanup operations
  - Pattern: use named return values with deferred error capture
- Address golangci-lint errors revealed by Go 1.25 upgrade (#142)

### Changed

- Improve `Zeroize` to prevent compiler optimization (#97)
  - Uses memory barriers to ensure sensitive data is actually cleared
  - Prevents compiler from optimizing away zeroization
- Use `subtle.ConstantTimeCompare` for `PublicKey.Equals` (#120)
  - Prevents timing side-channel attacks during key comparison
- Optimize `SignDoc.ToJSON()` to reduce allocations (#115)
  - Improved performance for high-throughput signing scenarios

### Security

- Return defensive copies from `CurveOrder()` and `HalfCurveOrder()` (#185)
  - Prevents corruption of package-level curve constants from caller mutation
  - Protects against unpredictable failures in concurrent network handlers
  - Cost: 1 allocation per call (~40 bytes), acceptable for protocol setup paths
- Document empty `chain_id` rejection for replay protection (#124)
  - Empty chain IDs must be rejected to prevent cross-chain replay attacks
  - Added comprehensive test coverage
- Document secp256r1 `Zeroize()` big.Int limitation (#162)
  - big.Int internal representation may retain sensitive data
  - Documented workarounds and security implications
- Add comprehensive concurrent race condition tests (#102, #119, #123, #131)
  - Adversarial concurrent Keyring tests
  - Concurrent read+write tests for FileKeyStore
  - Concurrent delegation mutation tests
  - Concurrent Keyring.Close() race tests

### Documentation

- Document `msg.Data` canonicalization requirements for SignDoc (#96)
  - Callers must provide canonical JSON in `SignDocMessage.Data` field
  - Rationale: Re-canonicalization would lose type information and add overhead
  - SDK serialization code is trusted to produce canonical output
- Document `SignDocSerializable` thread-safety requirements (#122)
  - `SignDocData()` must be safe for concurrent calls
  - Determinism requirements documented
- Add explicit cross-chain replay protection test (#121)
  - Documents and verifies chainID inclusion in signatures
