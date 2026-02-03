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
| `chain_id` | string | Chain identifier for replay protection (**REQUIRED**, **MUST NOT be empty**) |
| `account` | string | Account name/address |
| `account_sequence` | string | Account sequence number (decimal string) |
| `nonce` | string | Transaction nonce (decimal string) |
| `memo` | string | Optional transaction memo |
| `messages` | array | List of messages |
| `fee` | object | Transaction fee |
| `fee_slippage` | object | Fee slippage tolerance |

### Chain ID Requirements

**SECURITY CRITICAL**: The `chain_id` field is essential for cross-chain replay protection.

#### Validation Rules

1. **MUST NOT be empty**: An empty `chain_id` (`""`) MUST be rejected by implementations.
2. **MUST be included in signature**: The `chain_id` is part of the signed payload and cannot be modified after signing.

#### Security Rationale

The `chain_id` field prevents **cross-chain replay attacks**:

- Without `chain_id`, a signed transaction on Chain A could be replayed on Chain B
- An empty `chain_id` would allow replay across ALL chains that accept empty chain IDs
- This is why empty `chain_id` validation is mandatory, not optional

#### Attack Scenario (Empty Chain ID)

1. Attacker creates a malicious chain that accepts transactions with `chain_id: ""`
2. Victim signs a transaction with empty `chain_id` on the malicious chain
3. Attacker replays that signature on any other chain accepting empty `chain_id`
4. Result: Unauthorized transactions execute on victim's behalf

#### Implementation

Implementations MUST call `ValidateBasic()` (or equivalent) before processing any SignDoc. This validation rejects empty `chain_id` with the error: `chain_id cannot be empty`.

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

### secp256k1

- Key size: 33 bytes (compressed public), 32 bytes (private)
- Signature size: 64 bytes (R || S format, big-endian)
- Deterministic signatures: RFC 6979

**Key Format Notes**:

The secp256k1 keys use **compressed public key format** (SEC1):
- Public key: `0x02` or `0x03` prefix (1 byte) + X coordinate (32 bytes) = 33 bytes
- Private key: 32-byte scalar
- Signature: 64 bytes (R || S concatenated, each 32 bytes big-endian)

**Key Derivation**:
```
seed = SHA-256("punnet-sdk-test-vector-seed-secp256k1")
private_key = secp256k1.PrivateKeyFromScalar(seed)
public_key = private_key.PubKey().SerializeCompressed()
```

For key derivation vectors, a `secp256k1_seed` entry documents the 32-byte seed.

### secp256r1 (P-256/prime256v1)

- Key size: 33 bytes (compressed public), 32 bytes (private)
- Signature size: 64 bytes (R || S format, big-endian)
- Deterministic signatures: RFC 6979

**Key Format Notes**:

The secp256r1 (P-256) keys use **compressed public key format** (SEC1):
- Public key: `0x02` or `0x03` prefix (1 byte) + X coordinate (32 bytes) = 33 bytes
- Private key: 32-byte scalar
- Signature: 64 bytes (R || S concatenated, each 32 bytes big-endian)

**Key Derivation**:
```
seed = SHA-256("punnet-sdk-test-vector-seed-secp256r1")
private_key = P256.PrivateKeyFromScalar(seed)
public_key = Compress(private_key.PublicKey())
```

For key derivation vectors, a `secp256r1_seed` entry documents the 32-byte seed.

**Compressed Public Key Format** (for both secp256k1 and secp256r1):
```
If Y coordinate is even: 0x02 || X (33 bytes total)
If Y coordinate is odd:  0x03 || X (33 bytes total)
```

**Signature Malleability Note**:

ECDSA signatures have inherent malleability: both (R, S) and (R, n-S) are valid signatures for the same message, where n is the curve order. This can be a concern for transaction systems.

The test vectors use signatures **as produced by the signing algorithm** without low-S normalization (BIP-146 style). Specifically:
- secp256k1: The dcrd library produces canonical signatures by default
- secp256r1: Go's crypto/ecdsa with RFC 6979 produces signatures without explicit normalization

If your implementation applies low-S normalization (ensuring S ≤ n/2), the generated signatures may differ from the test vectors. For verification testing, either:
1. Disable low-S normalization during test vector verification, OR
2. Verify that both the normalized and non-normalized forms are valid

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
- Nil vs empty value serialization (see below)

## Nil vs Empty Value Handling

**CRITICAL**: Different programming languages may serialize null/nil vs empty values differently. To ensure cross-implementation compatibility, the Punnet SDK defines canonical serialization rules for these cases.

### Memo Field

| Input Representation | Canonical Serialization | Notes |
|---------------------|------------------------|-------|
| `null` / `nil` / `None` | `"memo":""` | Absent memo is empty string |
| `undefined` (JS) | `"memo":""` | Absent memo is empty string |
| `""` (empty string) | `"memo":""` | Empty string is preserved |
| `"text"` (non-empty) | `"memo":"text"` | Non-empty is preserved |

**Rule**: Implementations MUST normalize null/nil/undefined memo values to empty string `""`. The memo field MUST always be present in the canonical JSON.

### Fee Amount Field

| Input Representation | Canonical Serialization | Notes |
|---------------------|------------------------|-------|
| `null` / `nil` / `None` | `"amount":[]` | Absent amounts is empty array |
| `undefined` (JS) | `"amount":[]` | Absent amounts is empty array |
| `[]` (empty array) | `"amount":[]` | Empty array is preserved |
| `[{...}]` (with coins) | `"amount":[{...}]` | Non-empty is preserved |

**Rule**: Implementations MUST normalize null/nil/undefined fee amounts to empty array `[]`. The amount field MUST always be present in the canonical JSON.

### Message Data Field

| Input Representation | Canonical Serialization | Notes |
|---------------------|------------------------|-------|
| `null` / `nil` / `None` | `"data":null` | Null is **preserved** |
| `{}` (empty object) | `"data":{}` | Empty object is **preserved** |

**SECURITY**: Unlike memo and fee amounts, message data `null` and `{}` are **different values** that produce **different signatures**. Implementations MUST distinguish between:
- `"data":null` - represents absence of data
- `"data":{}` - represents an empty data object

This distinction is intentional because message semantics may differ between "no data provided" vs "empty data provided".

### Test Vectors

The following test vectors verify correct nil vs empty handling:

| Vector Name | Purpose |
|------------|---------|
| `nil_vs_empty_memo_string` | Verifies empty string memo serialization |
| `nil_vs_empty_fee_amount` | Verifies empty array fee amount serialization |
| `nil_vs_empty_combined` | Verifies both empty memo AND empty fee amounts |
| `nil_vs_empty_message_data_object` | Verifies `{}` message data |
| `nil_vs_empty_message_data_null` | Verifies `null` message data |

### Language-Specific Guidance

**Go**:
```go
// Both nil and empty slice serialize to []
var coins []SignDocCoin = nil  // OK: serializes to "amount":[]
coins = []SignDocCoin{}        // OK: serializes to "amount":[]
```

**JavaScript/TypeScript**:
```javascript
// Normalize before serialization
const memo = input.memo ?? "";          // null/undefined → ""
const amount = input.fee?.amount ?? []; // null/undefined → []
```

**Rust**:
```rust
// Use Option<T> with default serialization
let memo: String = input.memo.unwrap_or_default();
let amount: Vec<Coin> = input.amount.unwrap_or_default();
```

**Python**:
```python
# Normalize None values
memo = input.memo if input.memo is not None else ""
amount = input.fee.amount if input.fee.amount is not None else []
```

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
GENERATE_VECTORS=1 go test -run TestWriteVectorsFile ./testing/vectors/...
```

## Version History

### 1.0

- Initial format specification
- Ed25519 algorithm support
- Serialization, algorithm, and edge case vectors
