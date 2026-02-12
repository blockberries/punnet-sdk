# Transaction Signing Specification

## Overview

This specification defines the transaction signing mechanism for Punnet SDK, utilizing deterministic JSON serialization from Cramberry's `toJson` method. The design prioritizes human auditability, hardware wallet compatibility, and cross-language consistency.

## Design Goals

1. **Human Auditability**: Users and auditors can see exactly what they are signing in readable JSON format
2. **Hardware Wallet Display**: Hardware wallets (Ledger, Trezor) can display transaction contents before signing
3. **Cross-Language Compatibility**: Different client implementations produce identical signing bytes

## Architecture

### Code Organization

| Package | Contents |
|---------|----------|
| `types/` | `SignDoc` type definition, conversion methods |
| `crypto/` | Signing functions, key management, Signer interface, Keyring |

### Component Relationships

```
Transaction
    │
    ├── ToSignDoc() ─────────► SignDoc
    │                              │
    │                              ├── ToJSON() ─────► []byte (deterministic JSON)
    │                              │                       │
    │                              │                       └── SHA-256 hash
    │                              │                             │
    └── Authorization ◄────────────┴── Sign(hash) ◄──────────────┘
```

## SignDoc Structure

### Definition

The `SignDoc` is a dedicated type containing all fields that are signed. It mirrors the `Transaction` structure but **excludes the Authorization field** (since Authorization contains the signatures being produced).

```go
// SignDoc contains the fields that are signed for a transaction
type SignDoc struct {
    // SignDocVersion enables future format changes
    SignDocVersion string `json:"sign_doc_version"`

    // ChainID prevents cross-chain replay attacks
    ChainID string `json:"chain_id"`

    // Account is the account executing this transaction
    Account string `json:"account"`

    // AccountSequence prevents same-chain replay attacks
    AccountSequence uint64 `json:"account_sequence"`

    // Messages are the messages to execute
    Messages []SignDocMessage `json:"messages"`

    // Nonce from the transaction
    Nonce uint64 `json:"nonce"`

    // Memo is an optional memo
    Memo string `json:"memo"`

    // Fee is the fee to pay for transaction execution
    Fee SignDocFee `json:"fee"`

    // FeeSlippage is the maximum conversion rate slippage tolerance
    FeeSlippage SignDocRatio `json:"fee_slippage"`
}
```

### SignDocVersion

- Initial version: `"1"`
- Version field allows protocol upgrades without breaking existing signatures
- All validators must agree on supported versions

### Replay Protection

Two fields provide replay protection:

1. **ChainID**: Prevents replaying transactions across different chains
2. **AccountSequence**: Prevents replaying transactions on the same chain

Both fields are required and validated during verification.

## JSON Serialization Rules

### Key Ordering

Cramberry's `toJson` implementation determines the canonical key ordering. The SDK relies entirely on Cramberry's deterministic output.

### Formatting

- **Compact JSON**: No whitespace between elements
- Example: `{"account":"alice","chain_id":"mainnet-1",...}`

### Number Representation

All numeric values are serialized as **strings** to prevent JavaScript precision loss with large values:

```json
{
  "account_sequence": "42",
  "nonce": "100"
}
```

### Binary Data Encoding

Binary data (public keys, signatures in nested structures) uses **standard Base64** encoding (RFC 4648):

```json
{
  "pub_key": "7Xaz7fW7Y8J9K3L..."
}
```

### Empty/Null Fields

All fields are **always included**, even if empty, null, or zero:

```json
{
  "memo": "",
  "fee_slippage": {
    "numerator": "0",
    "denominator": "0"
  }
}
```

### Coin Format

Coins are represented as an **array of objects**, sorted by Cramberry:

```json
{
  "fee": {
    "amount": [
      {"denom": "atom", "amount": "1000"},
      {"denom": "stake", "amount": "500"}
    ],
    "gas_limit": "200000"
  }
}
```

The order of coins within the array is determined by Cramberry's serialization (expected to be lexicographic by denom).

### Message Type Encoding

Messages include an `@type` field with a full URL identifying the message type:

```json
{
  "messages": [
    {
      "@type": "/punnet.bank.v1.MsgSend",
      "from_account": "alice",
      "to_account": "bob",
      "amount": [
        {"denom": "atom", "amount": "1000000"}
      ]
    }
  ]
}
```

Type URL format: `/punnet.<module>.v1.<MessageType>`

## Signing Process

### Step 1: Create SignDoc

```go
func (tx *Transaction) ToSignDoc(chainID string, accountSequence uint64) *SignDoc {
    return &SignDoc{
        SignDocVersion:  "1",
        ChainID:         chainID,
        Account:         string(tx.Account),
        AccountSequence: accountSequence,
        Messages:        convertMessages(tx.Messages),
        Nonce:           tx.Nonce,
        Memo:            tx.Memo,
        Fee:             convertFee(tx.Fee),
        FeeSlippage:     convertRatio(tx.FeeSlippage),
    }
}
```

### Step 2: Serialize to JSON

```go
func (sd *SignDoc) ToJSON() ([]byte, error) {
    // Uses Cramberry's deterministic toJson
    return cramberry.ToJSON(sd)
}
```

### Step 3: Hash the JSON

```go
func (sd *SignDoc) GetSignBytes() ([]byte, error) {
    jsonBytes, err := sd.ToJSON()
    if err != nil {
        return nil, err
    }
    hash := sha256.Sum256(jsonBytes)
    return hash[:], nil
}
```

### Step 4: Sign the Hash

```go
func Sign(signDoc *SignDoc, privateKey crypto.PrivateKey) (*Signature, error) {
    signBytes, err := signDoc.GetSignBytes()
    if err != nil {
        return nil, err
    }

    sig := privateKey.Sign(signBytes)
    return &Signature{
        PubKey:    privateKey.PublicKey().Bytes(),
        Signature: sig,
        Algorithm: privateKey.Algorithm(),
    }, nil
}
```

## Verification Process

### Step 1: Reconstruct SignDoc

During verification, the SignDoc is **reconstructed** from the Transaction (no stored bytes):

```go
func (tx *Transaction) VerifySignature(chainID string, account *Account) error {
    // Reconstruct SignDoc from transaction fields
    signDoc := tx.ToSignDoc(chainID, account.Nonce)

    // Get canonical JSON bytes
    signBytes, err := signDoc.GetSignBytes()
    if err != nil {
        return err
    }

    // Verify all signatures against the hash
    return tx.Authorization.VerifySignatures(signBytes)
}
```

### Step 2: Roundtrip Validation

Before accepting a transaction, validators **always verify** that the SignDoc JSON can be reconstructed identically:

```go
func (tx *Transaction) ValidateSignDocRoundtrip(chainID string, accountSequence uint64) error {
    signDoc := tx.ToSignDoc(chainID, accountSequence)

    json1, err := signDoc.ToJSON()
    if err != nil {
        return err
    }

    // Parse and re-serialize
    var parsed SignDoc
    if err := cramberry.FromJSON(json1, &parsed); err != nil {
        return err
    }

    json2, err := cramberry.ToJSON(&parsed)
    if err != nil {
        return err
    }

    if !bytes.Equal(json1, json2) {
        return fmt.Errorf("SignDoc roundtrip validation failed: non-deterministic serialization")
    }

    return nil
}
```

If reconstruction produces different bytes than expected, the transaction is **rejected**.

## Multi-Signature Flow

All required signatures must be provided at once:

```go
// All signers must sign the same SignDoc
signDoc := tx.ToSignDoc(chainID, accountSequence)

sig1, _ := signer1.Sign(signDoc)
sig2, _ := signer2.Sign(signDoc)

tx.Authorization = &Authorization{
    Signatures: []Signature{sig1, sig2},
}
```

Progressive/partial signing is **not supported**. A coordinator must collect all signatures before submitting the transaction.

## Crypto Package

### Algorithm Support

The crypto package supports three signature algorithms:

| Algorithm | Key Size | Signature Size | Use Case |
|-----------|----------|----------------|----------|
| Ed25519 | 32 bytes | 64 bytes | Primary, recommended |
| secp256k1 | 33 bytes | 64 bytes | Ethereum/Bitcoin compatibility |
| secp256r1 (P-256) | 33 bytes | 64 bytes | Hardware security modules |

### Algorithm Encoding in JSON

When serializing public keys and signatures, the algorithm is included as a field:

```json
{
  "pub_key": "7Xaz7fW7Y8J9K3L...",
  "signature": "9dK8sL2...",
  "algorithm": "ed25519"
}
```

Valid algorithm values: `"ed25519"`, `"secp256k1"`, `"secp256r1"`

### Signer Interface

```go
// Signer represents an entity that can sign data
type Signer interface {
    // Sign signs the given data and returns a signature
    Sign(data []byte) ([]byte, error)

    // PublicKey returns the signer's public key
    PublicKey() PublicKey

    // Algorithm returns the signing algorithm
    Algorithm() Algorithm
}
```

### Keyring Interface

```go
// Keyring manages multiple signing keys
type Keyring interface {
    // NewKey generates a new key with the given name and algorithm
    NewKey(name string, algo Algorithm) (Signer, error)

    // ImportKey imports an existing private key
    ImportKey(name string, privKey []byte, algo Algorithm) (Signer, error)

    // ExportKey exports a private key (may require password)
    ExportKey(name string, password string) ([]byte, error)

    // GetKey retrieves a signer by name
    GetKey(name string) (Signer, error)

    // ListKeys returns all key names
    ListKeys() ([]string, error)

    // DeleteKey removes a key
    DeleteKey(name string) error

    // Sign signs data with the named key
    Sign(name string, data []byte) ([]byte, error)
}
```

### Key Storage Backends

The crypto package provides a pluggable storage backend interface:

```go
// KeyStore is the interface for key storage backends
type KeyStore interface {
    // Store saves a key
    Store(name string, key EncryptedKey) error

    // Load retrieves a key
    Load(name string) (EncryptedKey, error)

    // Delete removes a key
    Delete(name string) error

    // List returns all key names
    List() ([]string, error)
}
```

#### Initial Implementations

1. **In-Memory Backend**: For testing and ephemeral use
   ```go
   store := crypto.NewMemoryKeyStore()
   ```

2. **Encrypted File Backend**: Password-protected file storage
   ```go
   store, err := crypto.NewFileKeyStore("/path/to/keydir", password)
   ```
   - Uses PBKDF2 for key derivation
   - Uses AES-256-GCM for encryption
   - One file per key, JSON format

3. **OS Keychain Backend**: Native system keychain integration
   ```go
   store, err := crypto.NewKeychainStore("punnet-sdk")
   ```
   - macOS: Keychain
   - Windows: Credential Store
   - Linux: Secret Service (libsecret)

## Hash Algorithm

SHA-256 is used to hash the JSON bytes before signing:

```
signature = Ed25519.Sign(privateKey, SHA256(signDocJSON))
```

## Example SignDoc JSON

```json
{
  "sign_doc_version": "1",
  "chain_id": "punnet-mainnet-1",
  "account": "alice",
  "account_sequence": "42",
  "messages": [
    {
      "@type": "/punnet.bank.v1.MsgSend",
      "from_account": "alice",
      "to_account": "bob",
      "amount": [
        {
          "denom": "stake",
          "amount": "1000000"
        }
      ]
    }
  ],
  "nonce": "42",
  "memo": "",
  "fee": {
    "amount": [
      {
        "denom": "stake",
        "amount": "1000"
      }
    ],
    "gas_limit": "200000"
  },
  "fee_slippage": {
    "numerator": "0",
    "denominator": "0"
  }
}
```

## Error Handling

### SignDoc Reconstruction Mismatch

If the reconstructed SignDoc produces different bytes than expected during verification:

```go
var ErrSignDocMismatch = errors.New("SignDoc reconstruction mismatch: non-deterministic serialization detected")
```

The transaction is **immediately rejected**. This catches:
- Non-deterministic serialization bugs
- Tampering attempts
- Version mismatches

## Migration Considerations

### From Binary Signing

If migrating from binary Cramberry signing:

1. Announce the change with sufficient lead time
2. Support both signing methods during transition period (distinguished by version)
3. After transition, deprecate binary signing

### Version Upgrades

When updating the SignDoc format:

1. Increment `sign_doc_version` to `"2"`, etc.
2. Validators must support old versions for the duration of any pending transactions
3. Clients should query for supported versions before signing

## Security Considerations

### Determinism Verification

The roundtrip validation ensures that:
- The same transaction always produces the same SignDoc JSON
- Cross-implementation compatibility is maintained
- Subtle serialization bugs don't cause signature verification failures

### Replay Protection

- `chain_id` prevents cross-chain replays
- `account_sequence` prevents same-chain replays
- Both fields are required and validated

### Key Storage

- Private keys are never exposed outside the Signer interface
- Encrypted file backend uses strong key derivation (PBKDF2) and encryption (AES-GCM)
- OS keychain backends leverage hardware-backed security when available

## Implementation Checklist

### Phase 1: Core Types
- [ ] Define `SignDoc` type in `types/sign_doc.go`
- [ ] Implement `Transaction.ToSignDoc()` method
- [ ] Implement `SignDoc.ToJSON()` using Cramberry
- [ ] Implement `SignDoc.GetSignBytes()` (SHA-256 hash)
- [ ] Add roundtrip validation

### Phase 2: Crypto Package
- [ ] Create `crypto/` package structure
- [ ] Define `Algorithm` enum (Ed25519, secp256k1, secp256r1)
- [ ] Implement `PublicKey` and `PrivateKey` types with algorithm support
- [ ] Implement `Signer` interface
- [ ] Implement `Keyring` interface
- [ ] Implement `KeyStore` interface

### Phase 3: Storage Backends
- [ ] Implement in-memory KeyStore
- [ ] Implement encrypted file KeyStore (PBKDF2 + AES-GCM)
- [ ] Implement OS keychain KeyStore (macOS, Windows, Linux)

### Phase 4: Integration
- [ ] Update `Transaction.VerifyAuthorization()` to use SignDoc
- [ ] Update auth module to validate SignDoc
- [ ] Add comprehensive tests for cross-language compatibility
- [ ] Add tests for all three signature algorithms
- [ ] Document migration path from binary signing

### Phase 5: Testing & Validation
- [ ] Unit tests for SignDoc serialization determinism
- [ ] Integration tests for signing/verification
- [ ] Benchmark tests for signing performance
- [ ] Cross-implementation test vectors
