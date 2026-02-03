# Test Vector Format Specification

Version: 1.0

## Overview

This document describes the format of cross-implementation test vectors for the Punnet SDK signing system. These vectors enable verification of signing implementations across different languages and implementations.

## File Structure

Test vectors are stored in `signing_vectors.json` with the following structure:

```json
{
  "version": "1.0",
  "generated": "2024-01-15T10:00:00Z",
  "description": "Cross-implementation test vectors for Punnet SDK signing system",
  "vectors": [...]
}
```

### Root Fields

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Version of the test vector format |
| `generated` | string | ISO 8601 timestamp of generation (not part of test comparison) |
| `description` | string | Human-readable description |
| `vectors` | array | List of test vectors |

**Note**: The `generated` timestamp is informational only and changes each time vectors are regenerated. Implementations should NOT compare this field when verifying test vectors.

## Test Vector Structure

Each test vector has the following structure:

```json
{
  "name": "simple_send",
  "description": "Simple single-message MsgSend transaction",
  "category": "serialization",
  "input": {...},
  "expected": {...}
}
```

### Vector Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Unique identifier for the vector |
| `description` | string | Human-readable description |
| `category` | string | Category: `serialization`, `algorithm`, or `edge_case` |
| `input` | object | Input data for creating a SignDoc |
| `expected` | object | Expected outputs |

## Input Structure

The `input` object contains all data needed to construct a SignDoc:

```json
{
  "chain_id": "punnet-mainnet-1",
  "account": "alice",
  "account_sequence": "42",
  "nonce": "42",
  "memo": "",
  "messages": [...],
  "fee": {...},
  "fee_slippage": {...}
}
```

### Input Fields

| Field | Type | Description |
|-------|------|-------------|
| `chain_id` | string | Chain identifier for replay protection |
| `account` | string | Account name/address |
| `account_sequence` | string | Account sequence number (decimal string) |
| `nonce` | string | Transaction nonce (decimal string) |
| `memo` | string | Optional transaction memo |
| `messages` | array | List of messages |
| `fee` | object | Transaction fee |
| `fee_slippage` | object | Fee slippage tolerance |

### Message Structure

```json
{
  "type": "/punnet.bank.v1.MsgSend",
  "data": {"from": "alice", "to": "bob", "amount": "1000000"}
}
```

**IMPORTANT: Message Ordering**

Message order is significant for signing. Implementations MUST preserve the exact order of messages as provided in the `messages` array. Reordering messages will produce a different signature. This is intentional—message order can affect transaction semantics (e.g., funds transferred in message 1 may be used in message 2).

### Fee Structure

```json
{
  "amount": [
    {"denom": "stake", "amount": "5000"}
  ],
  "gas_limit": "200000"
}
```

**IMPORTANT: Fee Coin Ordering**

When multiple fee coins are provided, the order is significant for signing. Implementations MUST preserve the exact order of coins in the `amount` array. While implementations may choose to sort coins canonically when creating transactions, the test vectors verify that the serialization matches exactly. If your implementation sorts fee coins, ensure the sorting is deterministic and documented.

### Fee Slippage Structure

```json
{
  "numerator": "1",
  "denominator": "100"
}
```

## Expected Output Structure

The `expected` object contains deterministic outputs:

```json
{
  "sign_doc_json": "{...}",
  "sign_bytes_hex": "a1b2c3...",
  "signatures": {
    "ed25519": {...}
  }
}
```

### Expected Fields

| Field | Type | Description |
|-------|------|-------------|
| `sign_doc_json` | string | Canonical JSON serialization of the SignDoc |
| `sign_bytes_hex` | string | SHA-256 hash of sign_doc_json in hex |
| `signatures` | object | Map of algorithm name to signature data |

### Signature Structure

```json
{
  "private_key_hex": "...",
  "public_key_hex": "...",
  "signature_hex": "..."
}
```

**SECURITY WARNING**: The private keys in test vectors are for testing ONLY. Never use these keys in production.

## Numeric Value Encoding

All numeric values are encoded as **decimal strings** to ensure:
1. JavaScript BigInt compatibility (no precision loss beyond 2^53)
2. Cross-platform determinism
3. Consistent serialization

Examples:
- `"42"` - Small number
- `"18446744073709551615"` - Maximum uint64

## Canonical JSON Serialization

### Field Ordering

The SignDoc MUST be serialized with fields in the following canonical order:

```
version, chain_id, account, account_sequence, messages, nonce, memo (if present), fee, fee_slippage
```

**IMPORTANT**: Standard JSON libraries (like Go's `json.Marshal` with maps) do not guarantee field ordering. Implementations MUST use either:
1. A custom serializer that enforces field order, OR
2. Struct-based serialization where field order is determined by struct definition

The Punnet SDK uses struct-based serialization with explicit JSON field tags to ensure deterministic ordering.

### Whitespace

Canonical JSON uses no whitespace between elements. The serialization should be compact with no spaces after colons or commas.

## Hash Function

Sign bytes are computed as:
```
sign_bytes = SHA-256(canonical_json_bytes)
```

Where `canonical_json_bytes` is the UTF-8 encoded canonical JSON serialization of the SignDoc.

## Supported Algorithms

### Ed25519

- Key size: 32 bytes (public), 64 bytes (private/expanded)
- Seed size: 32 bytes
- Signature size: 64 bytes
- Deterministic signatures: Yes

**Key Format Notes**:

The `ed25519` signature entry uses the **expanded 64-byte private key** (seed || public key), which is the standard format for Ed25519 signing operations.

For key derivation vectors, an additional `ed25519_seed` entry may be present containing:
- `private_key_hex`: The 32-byte **seed** (not the expanded key)
- `public_key_hex`: The derived 32-byte public key
- `signature_hex`: Empty string (seed entries document derivation, not signing)

This distinction is important for cross-implementation testing: some libraries expose only the seed, while others use the expanded form. The seed can always be expanded to the full private key using standard Ed25519 key derivation.

### secp256k1 (Future)

- Key size: 33 bytes (compressed public), 32 bytes (private)
- Signature size: 64 bytes (r||s format)
- Deterministic signatures: RFC 6979

### secp256r1 (Future)

- Key size: 33 bytes (compressed public), 32 bytes (private)
- Signature size: 64 bytes (r||s format)
- Deterministic signatures: RFC 6979

## Test Vector Categories

### Serialization Vectors

Test JSON serialization correctness:
- Simple transactions
- Multi-message transactions
- Transactions with memos
- Fee configurations
- Multiple fee coins

### Algorithm Vectors

Test cryptographic operations:
- Key derivation from seed
- Signature generation
- Signature verification

### Edge Case Vectors

Test boundary conditions:
- Empty memos
- Zero values
- Maximum uint64 values
- Unicode characters
- Special characters (escapes, quotes)
- Minimal valid transactions

## Well-Known Test Keys

Test keys are derived deterministically:

```
Ed25519 seed = SHA-256("punnet-sdk-test-vector-seed-ed25519")
```

This ensures all implementations can reproduce the exact same test keys.

## Implementation Guide

### Verifying Test Vectors

1. Load `signing_vectors.json`
2. For each vector:
   a. Construct SignDoc from `input`
   b. Serialize to canonical JSON
   c. Compare with `expected.sign_doc_json`
   d. Compute SHA-256 hash
   e. Compare with `expected.sign_bytes_hex`
   f. For each algorithm in `expected.signatures`:
      - Derive public key from private key
      - Verify public key matches
      - Generate signature using private key
      - Verify signature matches
      - Verify signature using public key

### Generating New Vectors

Use the provided Go generator:

```bash
GENERATE_VECTORS=1 go test -run TestWriteVectorsFile ./testdata/...
```

## Version History

### 1.0

- Initial format specification
- Ed25519 algorithm support
- Serialization, algorithm, and edge case vectors
