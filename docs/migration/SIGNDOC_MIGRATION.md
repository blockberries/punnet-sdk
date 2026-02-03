# Migration Guide: Binary to JSON SignDoc Signing

This guide documents the migration from binary Cramberry-based transaction signing to JSON-based SignDoc signing in the Punnet SDK.

## Table of Contents

1. [Overview](#overview)
2. [Why This Change](#why-this-change)
3. [Breaking Changes](#breaking-changes)
4. [Version Transition Plan](#version-transition-plan)
5. [Client Migration Guide](#client-migration-guide)
6. [Validator Migration Guide](#validator-migration-guide)
7. [API Changes](#api-changes)
8. [Troubleshooting](#troubleshooting)

---

## Overview

The Punnet SDK has transitioned from binary Cramberry serialization to JSON-based SignDoc serialization for transaction signing. This change affects how transactions are prepared for signing and how signatures are verified.

### What Changed

| Aspect | Old (Binary) | New (JSON SignDoc) |
|--------|--------------|-------------------|
| Serialization | Cramberry binary encoding | Deterministic JSON |
| Sign bytes | `cramberry.ToBinary(tx)` | `signDoc.GetSignBytes()` |
| Hash algorithm | SHA-256 | SHA-256 (unchanged) |
| Human readability | Not readable | Human-auditable |
| Hardware wallet support | Limited | Full support |

### Current Version

The current SignDoc version is **"1"** (JSON-based). The SDK only supports version "1" signatures.

```go
const SignDocVersion = "1"
var SupportedSignDocVersions = []string{"1"}
```

---

## Why This Change

### 1. Human Auditability

JSON-based SignDocs are human-readable, allowing users to inspect exactly what they're signing before authorizing a transaction:

```json
{
  "version": "1",
  "chain_id": "punnet-testnet-1",
  "account": "alice",
  "account_sequence": "42",
  "messages": [
    {
      "type": "/punnet.bank.v1.MsgSend",
      "data": {"from":"alice","to":"bob","amount":"1000stake"}
    }
  ],
  "nonce": "42",
  "memo": "Payment for services",
  "fee": {"amount":[],"gas_limit":"100000"},
  "fee_slippage": {"numerator":"1","denominator":"100"}
}
```

### 2. Hardware Wallet Support

Hardware wallets (Ledger, Trezor, etc.) can now display transaction details on their screens, enabling users to verify transactions before signing. Binary serialization made this impossible.

### 3. Cross-Platform Consistency

JSON serialization provides deterministic output across all platforms and programming languages, reducing the risk of signature mismatches due to serialization differences.

### 4. JavaScript BigInt Safety

Numeric values are serialized as strings (e.g., `"account_sequence": "42"`) to ensure safe handling in JavaScript clients where `Number.MAX_SAFE_INTEGER` (2^53 - 1) would otherwise cause precision loss.

---

## Breaking Changes

### Summary

1. **SignDoc structure is different** - Transactions now use the `SignDoc` type for signing
2. **Sign bytes computation changed** - From `sha256(cramberry.ToBinary(tx))` to `sha256(signDoc.ToJSON())`
3. **New required fields** - SignDoc requires `version`, `chain_id`, `account_sequence`, and other fields
4. **Message serialization** - Messages must implement `SignDocSerializable` interface

### Detailed Changes

#### Before: Binary Signing (Deprecated)

```go
// Old approach - NO LONGER SUPPORTED
signBytes := cramberry.ToBinary(tx)
sig := privateKey.Sign(sha256.Sum256(signBytes))
```

#### After: JSON SignDoc Signing

```go
// New approach - CURRENT
signDoc, err := tx.ToSignDoc(chainID, accountSequence)
if err != nil {
    return err
}
signBytes, err := signDoc.GetSignBytes()
if err != nil {
    return err
}
sig, err := privateKey.Sign(signBytes)
```

### Signature Incompatibility

**Signatures created with the old binary format are NOT compatible with the new JSON format.** This is intentional - the sign bytes are fundamentally different.

---

## Version Transition Plan

The migration follows a phased approach:

### Phase A: JSON-Only (Current State)

- Only JSON-based SignDoc version "1" is supported
- All transactions must use `ToSignDoc()` for signing
- Binary signing is not available

**Configuration:**
```go
// Version validation happens automatically
if err := types.ValidateSignDocVersion(signDoc.Version); err != nil {
    return err // Rejects unsupported versions
}
```

### Phase B: Maintenance

- No changes to signing format
- Focus on performance optimizations and bug fixes
- Documentation improvements

### Phase C: Future Versions

When introducing SignDoc version "2" or later:

1. Add new version to `SupportedSignDocVersions`
2. Maintain backwards compatibility for validation
3. Provide migration path documentation
4. Deprecate older versions with appropriate timeline

**Example future versioning:**
```go
// Future: supporting multiple versions
var SupportedSignDocVersions = []string{"1", "2"}
```

---

## Client Migration Guide

### Step 1: Update Dependencies

Ensure you're using the latest Punnet SDK:

```bash
go get github.com/blockberries/punnet-sdk@latest
```

### Step 2: Update Transaction Signing Code

#### Before (Old Pattern)

```go
// This pattern is NO LONGER SUPPORTED
func signTransaction(tx *types.Transaction, privKey crypto.PrivateKey) ([]byte, error) {
    // Old: Direct binary serialization
    signBytes := cramberry.ToBinary(tx)
    hash := sha256.Sum256(signBytes)
    return privKey.Sign(hash[:])
}
```

#### After (New Pattern)

```go
import (
    "github.com/blockberries/punnet-sdk/crypto"
    "github.com/blockberries/punnet-sdk/types"
)

func signTransaction(tx *types.Transaction, chainID string, accountSequence uint64, privKey crypto.PrivateKey) (*crypto.Signature, error) {
    // Step 1: Convert transaction to SignDoc
    signDoc, err := tx.ToSignDoc(chainID, accountSequence)
    if err != nil {
        return nil, fmt.Errorf("failed to create SignDoc: %w", err)
    }

    // Step 2: Validate the SignDoc
    if err := signDoc.ValidateBasic(); err != nil {
        return nil, fmt.Errorf("invalid SignDoc: %w", err)
    }

    // Step 3: Sign using the crypto package helper
    signature, err := crypto.SignSignDoc(signDoc, privKey)
    if err != nil {
        return nil, fmt.Errorf("signing failed: %w", err)
    }

    return signature, nil
}
```

### Step 3: Update Message Types

Messages should implement the `SignDocSerializable` interface for full content binding:

```go
// types/message.go
type SignDocSerializable interface {
    // SignDocData returns the canonical JSON representation for signing
    SignDocData() (json.RawMessage, error)
}
```

#### Example Implementation

```go
type MsgSend struct {
    From   types.AccountName `json:"from"`
    To     types.AccountName `json:"to"`
    Amount types.Coins       `json:"amount"`
}

// Implement SignDocSerializable
func (m *MsgSend) SignDocData() (json.RawMessage, error) {
    // Use compact, deterministic JSON
    data := map[string]interface{}{
        "from":   string(m.From),
        "to":     string(m.To),
        "amount": m.Amount.String(),
    }
    return json.Marshal(data)
}
```

### Step 4: Update Signature Verification

```go
func verifyTransaction(tx *types.Transaction, chainID string, account *types.Account, getter types.AccountGetter) error {
    // This method handles SignDoc reconstruction and verification internally
    return tx.VerifyAuthorization(chainID, account, getter)
}
```

### Step 5: Handle Unicode Normalization

SignDoc requires all string fields to be NFC-normalized:

```go
import "golang.org/x/text/unicode/norm"

// Normalize user input before creating transactions
memo := norm.NFC.String(userProvidedMemo)
chainID := norm.NFC.String(chainIDInput)
```

### Testing Your Migration

```go
func TestSignDocMigration(t *testing.T) {
    // Create a test transaction
    tx := types.NewTransaction(
        types.AccountName("alice"),
        1, // nonce
        []types.Message{&MsgSend{From: "alice", To: "bob", Amount: types.NewCoins(100, "stake")}},
        nil, // auth will be added after signing
    )

    // Create SignDoc
    signDoc, err := tx.ToSignDoc("test-chain", 1)
    require.NoError(t, err)

    // Validate
    require.NoError(t, signDoc.ValidateBasic())

    // Get sign bytes
    signBytes, err := signDoc.GetSignBytes()
    require.NoError(t, err)
    require.Len(t, signBytes, 32) // SHA-256 hash

    // Verify determinism
    signBytes2, err := signDoc.GetSignBytes()
    require.NoError(t, err)
    require.Equal(t, signBytes, signBytes2)
}
```

---

## Validator Migration Guide

### Node Configuration

No special configuration is required. Validators automatically:
1. Validate SignDoc version on incoming transactions
2. Reject transactions with unsupported versions
3. Reconstruct SignDoc for signature verification

### Version Compatibility Matrix

| SDK Version | SignDoc v1 (JSON) | Notes |
|-------------|-------------------|-------|
| v0.1.x+     | Supported         | Current |

### Upgrade Process

1. **Update SDK**: Deploy nodes with the latest SDK version
2. **Monitor logs**: Watch for SignDoc validation errors
3. **Verify signatures**: Ensure all transactions pass verification

### Rollback Procedure

If issues arise:

1. **Identify the issue**: Check logs for signature verification failures
2. **Preserve state**: SignDoc changes don't affect state storage
3. **Rollback binaries**: Deploy previous SDK version if needed

**Note**: Rollback is only possible if no transactions with new SignDoc format have been committed.

### Security Considerations for Validators

1. **Version validation is mandatory**: Never skip `ValidateSignDocVersion()`
2. **Reject unknown versions**: This prevents forward-compatibility attacks
3. **Verify determinism**: The SDK validates roundtrip serialization automatically

```go
// This check happens automatically in VerifyAuthorization
if err := types.ValidateSignDocVersion(signDoc.Version); err != nil {
    // MUST reject - different nodes may interpret unknown versions differently
    return err
}
```

---

## API Changes

### New Types

#### `SignDoc` (types/signdoc.go)

```go
type SignDoc struct {
    Version         string           `json:"version"`
    ChainID         string           `json:"chain_id"`
    Account         string           `json:"account"`
    AccountSequence StringUint64     `json:"account_sequence"`
    Messages        []SignDocMessage `json:"messages"`
    Nonce           StringUint64     `json:"nonce"`
    Memo            string           `json:"memo"`
    Fee             SignDocFee       `json:"fee"`
    FeeSlippage     SignDocRatio     `json:"fee_slippage"`
}
```

#### `SignDocMessage`

```go
type SignDocMessage struct {
    Type string          `json:"type"`
    Data json.RawMessage `json:"data"`
}
```

#### `StringUint64`

```go
// Serializes uint64 as JSON string for JavaScript BigInt safety
type StringUint64 uint64
```

### New Methods

#### `Transaction.ToSignDoc()`

```go
func (tx *Transaction) ToSignDoc(chainID string, accountSequence uint64) (*SignDoc, error)
```

Converts a transaction to a SignDoc for signing.

#### `SignDoc.GetSignBytes()`

```go
func (sd *SignDoc) GetSignBytes() ([]byte, error)
```

Returns SHA-256 hash of the canonical JSON representation.

#### `SignDoc.ToJSON()`

```go
func (sd *SignDoc) ToJSON() ([]byte, error)
```

Returns deterministic JSON bytes for the SignDoc.

#### `SignDoc.ValidateBasic()`

```go
func (sd *SignDoc) ValidateBasic() error
```

Performs stateless validation including:
- Version check
- Required field validation
- Message count limits (max 256)
- Message data size limits (max 64KB each)
- Unicode NFC normalization

#### `crypto.SignSignDoc()`

```go
func SignSignDoc(signDoc SignBytesProvider, privateKey PrivateKey) (*Signature, error)
```

Signs a SignDoc and returns a complete Signature struct.

### Deprecated/Removed

- Direct binary serialization for signing is not available
- `cramberry.ToBinary()` should not be used for transaction signing

### Interface Changes

#### `SignDocSerializable` (new)

Messages should implement this interface:

```go
type SignDocSerializable interface {
    SignDocData() (json.RawMessage, error)
}
```

#### `SignBytesProvider` (crypto package)

```go
type SignBytesProvider interface {
    GetSignBytes() ([]byte, error)
}
```

SignDoc implements this interface.

---

## Troubleshooting

### Common Errors

#### "unsupported SignDoc version"

**Cause**: Transaction created with unsupported version.

**Solution**: Ensure transactions use `SignDocVersion` constant:
```go
signDoc := types.NewSignDoc(chainID, sequence, account, nonce, memo)
// Version is automatically set to SignDocVersion ("1")
```

#### "chain_id is not Unicode NFC-normalized"

**Cause**: String field contains non-NFC Unicode characters.

**Solution**: Normalize all string inputs:
```go
import "golang.org/x/text/unicode/norm"
chainID = norm.NFC.String(chainID)
```

#### "message data is not compact JSON"

**Cause**: Message data contains whitespace outside strings.

**Solution**: Ensure `SignDocData()` returns compact JSON:
```go
// Use json.Marshal, not json.MarshalIndent
data, err := json.Marshal(msgContent)
```

#### "roundtrip produced different bytes"

**Cause**: Non-deterministic JSON serialization.

**Solution**:
- Check for floating-point numbers (not allowed)
- Ensure all strings are NFC-normalized
- Verify message data is compact JSON

### Debugging Tips

1. **Inspect SignDoc JSON**:
```go
jsonBytes, _ := signDoc.ToJSON()
fmt.Printf("SignDoc: %s\n", jsonBytes)
```

2. **Compare sign bytes**:
```go
signBytes1, _ := signDoc1.GetSignBytes()
signBytes2, _ := signDoc2.GetSignBytes()
fmt.Printf("Match: %v\n", bytes.Equal(signBytes1, signBytes2))
```

3. **Validate before signing**:
```go
if err := signDoc.ValidateBasic(); err != nil {
    log.Printf("Validation failed: %v", err)
}
```

---

## Additional Resources

- [SignDoc Implementation](../../types/signdoc.go)
- [Transaction Signing](../../types/transaction.go)
- [Crypto Package](../../crypto/signer.go)
- [Test Vectors](../../testdata/signing_vectors.json)

---

*Document version: 1.0*
*Last updated: 2026-02-03*
