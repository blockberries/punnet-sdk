# Cramberry Schema Definitions

This directory contains Cramberry schema definitions for deterministic binary serialization of Punnet SDK types. These schemas replace JSON serialization to ensure deterministic encoding for blockchain consensus.

## Schema Files

### Core Types (`types.cram`)

Defines fundamental types used across the SDK:

- **Account**: Named account with hierarchical permissions
- **Authority**: Authorization structure with key weights and account delegations
- **Authorization**: Proof of authority with signatures and recursive delegations
- **Signature**: Ed25519 signature with public key
- **Coin/Coins**: Token denomination and amount
- **Transaction**: Signed transaction with messages
- **ValidatorUpdate**: Validator power updates for consensus
- **Result types**: TxResult, QueryResult, CommitResult, EndBlockResult
- **Event types**: Event, EventAttribute

### Auth Module (`auth.cram`)

Defines auth module messages:

- **MsgCreateAccount**: Create a new account
- **MsgUpdateAuthority**: Update account authority
- **MsgDeleteAccount**: Delete an account
- **Query types**: AccountQueryRequest/Response, AccountListQueryRequest/Response

### Bank Module (`bank.cram`)

Defines bank module messages and types:

- **MsgSend**: Transfer coins between accounts
- **MsgMultiSend**: Multi-party coin transfer
- **Input/Output**: Multi-send components
- **Balance**: Account balance storage type
- **Query types**: Balance queries, supply queries

### Staking Module (`staking.cram`)

Defines staking module messages and types:

- **MsgCreateValidator**: Create a new validator
- **MsgDelegate**: Delegate tokens to validator
- **MsgUndelegate**: Undelegate tokens from validator
- **Validator**: Validator storage type
- **Delegation**: Delegation storage type
- **UnbondingDelegation**: Unbonding delegation storage type
- **Query types**: Validator queries, delegation queries

## Field Numbering

Field numbers are **STABLE** and follow these rules:

1. **Never reuse** field numbers, even if a field is removed
2. **Never change** existing field numbers
3. **Always append** new fields with new numbers
4. Field numbers 1-15 use 1 byte encoding (use for common fields)
5. Field numbers 16-2047 use 2 bytes encoding

## Encoding Conventions

### Deterministic Encoding

All types use deterministic encoding to ensure consensus:

- **Maps**: Sorted by key (lexicographic for strings, binary for bytes)
- **Repeated fields**: Encoded in order (caller must sort when needed)
- **Bytes**: Length-prefixed
- **Strings**: UTF-8 encoded, length-prefixed
- **Integers**: Varint encoding (compact for small values)
- **Bools**: Single byte (0 or 1)

### Type-Specific Conventions

#### Account Names
- Max 64 bytes
- Pattern: `^[a-z0-9.]+$`
- Examples: `alice`, `bob.delegate`, `system.vault`

#### Public Keys
- Ed25519: 32 bytes
- Stored as raw bytes

#### Signatures
- Ed25519: 64 bytes
- Stored as raw bytes

#### Timestamps
- Unix time in nanoseconds (int64)
- UTC timezone

#### Commission Rates
- Range: 0-10000 (where 10000 = 100%)
- Example: 1000 = 10% commission

#### Validator Power
- int64 value
- Power = 0 means inactive/removed
- Negative power is invalid

## Message Type URLs

Messages are identified by type URLs in the format:
```
/punnet.MODULE.v1.MessageType
```

Examples:
- `/punnet.auth.v1.MsgCreateAccount`
- `/punnet.bank.v1.MsgSend`
- `/punnet.staking.v1.MsgDelegate`

## Code Generation

### TODO: Cramberry Compiler Integration

Once the Cramberry compiler is available, generate Go code with:

```bash
# Generate all schemas
make generate

# Or manually:
cramberry generate -lang go -out ./types/generated ./schema/types.cram
cramberry generate -lang go -out ./modules/auth/generated ./schema/auth.cram
cramberry generate -lang go -out ./modules/bank/generated ./schema/bank.cram
cramberry generate -lang go -out ./modules/staking/generated ./schema/staking.cram
```

Generated code will include:
- Type definitions
- Marshal/Unmarshal methods
- Size calculation
- Deterministic serialization
- Validation helpers

### Manual Implementation (Temporary)

Until Cramberry is available, implement serialization manually:

1. Use the schema as reference for field ordering
2. Implement deterministic encoding following conventions
3. Ensure map keys are sorted
4. Use varint encoding for integers
5. Add comprehensive tests for encoding/decoding

## Migration from JSON

Current codebase uses JSON for serialization. Migration steps:

1. Generate Cramberry code from schemas
2. Implement conversion between Go types and generated types
3. Add `Marshal()`/`Unmarshal()` methods to existing types
4. Update `Transaction.GetSignBytes()` to use Cramberry
5. Update store serializers to use Cramberry
6. Add round-trip tests
7. Performance benchmarks

### Critical Migration Points

These must use Cramberry for consensus determinism:

- `Transaction.GetSignBytes()` - signature verification
- `Transaction.Hash()` - transaction identification
- Store serialization - state commitment
- Message encoding in transactions
- Validator updates to consensus engine

## Testing

Test serialization with:

```bash
# Unit tests for each schema
go test ./types/generated/...
go test ./modules/*/generated/...

# Round-trip tests
go test ./schema/... -run RoundTrip

# Determinism tests (same input -> same output)
go test ./schema/... -run Determinism

# Cross-language tests (if multiple implementations)
go test ./schema/... -run CrossLanguage
```

## Versioning

Schemas use semantic versioning in package names:

- `punnet.MODULE.v1` - Version 1 (current)
- `punnet.MODULE.v2` - Version 2 (future)

Breaking changes require new version:
- Field number reuse
- Field type change
- Required field removal
- Message rename

## Performance Considerations

Cramberry encoding vs JSON:

| Metric | JSON | Cramberry | Improvement |
|--------|------|-----------|-------------|
| Size | 100% | ~40-60% | 40-60% smaller |
| Marshal | 100% | ~200-300% | 2-3x faster |
| Unmarshal | 100% | ~150-250% | 1.5-2.5x faster |
| Determinism | ❌ | ✅ | Required for consensus |

Benefits:
- Smaller transaction size (lower fees)
- Faster serialization (higher throughput)
- Deterministic (consensus requirement)
- Type safety (compile-time checks)

## References

- Cramberry Specification: `../cramberry/README.md`
- Protobuf Encoding: https://developers.google.com/protocol-buffers/docs/encoding
- Cosmos SDK ADR-027: https://docs.cosmos.network/main/architecture/adr-027-deterministic-protobuf-serialization

## TODO

- [ ] Integrate Cramberry compiler into build
- [ ] Generate Go code from schemas
- [ ] Implement conversion helpers
- [ ] Migrate Transaction.GetSignBytes()
- [ ] Migrate store serialization
- [ ] Add round-trip tests
- [ ] Add determinism tests
- [ ] Add benchmarks
- [ ] Update documentation
- [ ] Migration guide for existing data
