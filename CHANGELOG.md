# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add SignDoc migration guide documenting transition from binary to JSON signing (#22)
  - Migration overview and rationale (human auditability, hardware wallet support)
  - Breaking changes and version transition plan
  - Client and validator migration guides with code examples
  - API changes and troubleshooting documentation
  - See `docs/migration/SIGNDOC_MIGRATION.md`
- Add `CachingKeyStore` read-through caching wrapper for `EncryptedKeyStore` backends (#138)
  - LRU-based eviction with configurable cache capacity
  - Write-through semantics for consistency
  - Manual invalidation via `Invalidate()` and `InvalidateAll()`
  - Cache hit/miss statistics via `Stats()` method
  - ~182,000x speedup for cache hits vs FileKeyStore backend
- Add test vectors for secp256k1 and secp256r1 algorithms (#67)
  - Key derivation vectors from deterministic seed
  - Signature generation vectors using RFC 6979 deterministic signatures
  - Updated FORMAT.md with complete algorithm documentation
- Add test vectors for nil vs empty value serialization (#66)
  - Test vectors for null/empty string memo handling
  - Test vectors for null/empty array fee amount handling
