package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/blockberries/cramberry/pkg/cramberry"
)

// Transaction represents a signed transaction
type Transaction struct {
	// Account is the account executing this transaction
	Account AccountName `json:"account"`

	// Messages are the messages to execute
	Messages []Message `json:"messages"`

	// Authorization proves the account authorized this transaction
	Authorization *Authorization `json:"authorization"`

	// Nonce prevents replay attacks
	Nonce uint64 `json:"nonce"`

	// Memo is an optional memo
	Memo string `json:"memo,omitempty"`

	// Fee carries the three-component fee payload per the tokenomics
	// spec §3 (OpFees, ByteFee, Priority, Payer). The Fee is part of
	// the signed payload — submitters cannot mutate it after signing.
	// AnteHandler (added in tokenomics Phase 1) validates components
	// against the current FeeSchedule and routes per the spec.
	//
	// Today's omitempty behavior: a Transaction with a zero Fee
	// (no OpFees, ByteFee=0, Priority=0) is accepted by the runtime
	// pre-AnteHandler. Once Phase 1 lands the AnteHandler will reject
	// any tx whose Fee doesn't match the schedule. PLAN §7 Phase 0.1.
	Fee Fee `json:"fee,omitempty"`
}

// NewTransaction creates a new transaction
// Creates defensive copies of slices to prevent external mutation
func NewTransaction(account AccountName, nonce uint64, messages []Message, auth *Authorization) *Transaction {
	// Create defensive copy of messages slice
	msgsCopy := make([]Message, len(messages))
	copy(msgsCopy, messages)

	return &Transaction{
		Account:       account,
		Messages:      msgsCopy,
		Authorization: auth,
		Nonce:         nonce,
	}
}

// ValidateBasic performs basic validation
func (tx *Transaction) ValidateBasic() error {
	if tx == nil {
		return fmt.Errorf("%w: transaction is nil", ErrInvalidTransaction)
	}

	if !tx.Account.IsValid() {
		return fmt.Errorf("%w: %s", ErrInvalidAccount, tx.Account)
	}

	if len(tx.Messages) == 0 {
		return fmt.Errorf("%w: transaction must have at least one message", ErrInvalidTransaction)
	}

	if tx.Authorization == nil {
		return fmt.Errorf("%w: authorization cannot be nil", ErrInvalidTransaction)
	}

	// Validate authorization
	if err := tx.Authorization.ValidateBasic(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTransaction, err)
	}

	// Validate all messages
	for i, msg := range tx.Messages {
		if err := msg.ValidateBasic(); err != nil {
			return fmt.Errorf("%w: message %d: %v", ErrInvalidTransaction, i, err)
		}

		// Verify that the transaction account is authorized to send all messages
		signers := msg.GetSigners()
		validSigner := false
		for _, signer := range signers {
			if signer == tx.Account {
				validSigner = true
				break
			}
		}
		if !validSigner {
			return fmt.Errorf("%w: transaction account %s not in message signers", ErrInvalidTransaction, tx.Account)
		}
	}

	// Memo size limit
	if len(tx.Memo) > 512 {
		return fmt.Errorf("%w: memo exceeds 512 bytes", ErrInvalidTransaction)
	}

	// Fee structural validation. Component overflow, payer validity,
	// duplicate-message-type detection all happen in Fee.ValidateBasic.
	if err := tx.Fee.ValidateBasic(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTransaction, err)
	}

	// Cross-validate Fee.OpFees against tx.Messages: when OpFees is
	// populated, it must carry exactly one entry per message (same
	// order, same Type()). Empty OpFees is allowed here — that's the
	// "fee-less submission" path used by pre-AnteHandler tests; once
	// Phase 1 lands, the AnteHandler rejects fee-less txs at execution.
	if len(tx.Fee.OpFees) > 0 {
		if len(tx.Fee.OpFees) != len(tx.Messages) {
			return fmt.Errorf("%w: fee op_fees count %d != messages count %d",
				ErrInvalidTransaction, len(tx.Fee.OpFees), len(tx.Messages))
		}
		for i, op := range tx.Fee.OpFees {
			if op.MessageType != tx.Messages[i].Type() {
				return fmt.Errorf("%w: fee op_fees[%d].message_type %q != messages[%d].Type() %q",
					ErrInvalidTransaction, i, op.MessageType, i, tx.Messages[i].Type())
			}
		}
	}

	// Payer authorization: when Fee.Payer is set and differs from
	// tx.Account, it must appear among the signers of every message
	// — otherwise the signature doesn't cover the fee charge.
	payer := tx.Fee.PayerOrAccount(tx.Account)
	if payer != tx.Account {
		for i, msg := range tx.Messages {
			covered := false
			for _, signer := range msg.GetSigners() {
				if signer == payer {
					covered = true
					break
				}
			}
			if !covered {
				return fmt.Errorf("%w: fee payer %s not in signers of message %d",
					ErrInvalidTransaction, payer, i)
			}
		}
	}

	return nil
}

// Hash computes a deterministic identifier for this transaction.
//
// The hash inputs are the same fields covered by GetSignBytes (account,
// nonce, per-message Type + cramberry-encoded body, memo), so two TXs
// that share an account+nonce but differ in messages produce different
// hashes. This is the same recipe we use for SignDoc bytes — signature
// is excluded to keep the hash stable across signature recomputation
// (e.g. multi-sig reconstruction).
//
// The previous implementation hashed only `account || msg.Type()`,
// which collapsed any two TXs from the same account with the same
// message-type sequence into the same hash — a real collision class
// for mempool dedup. The fix is cramberry-encoded message bodies,
// matching the determinism guarantees from PLAN T1-6.
func (tx *Transaction) Hash() []byte {
	// Reuse GetSignBytes — both produce a deterministic hash of the
	// transaction's authorizing content. If we later need to include
	// the signature (so the hash distinguishes resigned TXs), swap to
	// `sha256(cramberry.Marshal(tx))`; today there's no consumer that
	// needs that.
	return tx.GetSignBytes()
}

// GetSignBytes returns the bytes to sign for this transaction.
//
// Determinism is consensus-critical: every validator must compute byte-identical
// SignDoc bytes for the same transaction. Previously this function used
// json.Marshal(msg) for message bodies, which iterates Go maps in random order
// and therefore produces different SignDocs on different validators for the same
// logical transaction (PLAN T1-6). The implementation now uses
// cramberry.Marshal, whose wire format is deterministic by spec: struct fields
// are emitted in field-number order, map keys are sorted, floats are normalized.
//
// The hash inputs are deliberately the same as before (account, nonce,
// msg.Type(), msg-bytes, memo) so that the only behaviour change is "stable
// across Go map-iteration runs". This is a wire-format change: nodes running
// the JSON variant cannot interoperate with nodes running this variant.
func (tx *Transaction) GetSignBytes() []byte {
	h := sha256.New()
	h.Write([]byte(tx.Account))

	// Add nonce
	nonceBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		nonceBytes[i] = byte(tx.Nonce >> (8 * i))
	}
	h.Write(nonceBytes)

	// Add full message content for each message using cramberry's deterministic
	// binary encoding. Errors are intentionally swallowed (as before): a
	// non-serializable message contributes only its Type() string, which
	// preserves the prior "best effort" semantics.
	for _, msg := range tx.Messages {
		h.Write([]byte(msg.Type()))
		msgBytes, err := cramberry.Marshal(msg)
		if err == nil {
			h.Write(msgBytes)
		}
	}

	// Add memo
	h.Write([]byte(tx.Memo))

	// Add Fee — the signed payload includes the fee so submitters
	// can't undercut by mutating it after signing. cramberry's
	// deterministic encoding is what makes this stable across Go's
	// non-deterministic map iteration. PLAN §7 decision D8.
	if feeBytes, err := cramberry.Marshal(&tx.Fee); err == nil {
		h.Write(feeBytes)
	}

	return h.Sum(nil)
}

// transactionJSON is the private JSON wire format for Transaction.
type transactionJSON struct {
	Account       AccountName       `json:"account"`
	Messages      []MessageEnvelope `json:"messages"`
	Authorization *Authorization    `json:"authorization"`
	Nonce         uint64            `json:"nonce"`
	Memo          string            `json:"memo,omitempty"`
	Fee           Fee               `json:"fee,omitempty"`
}

// MarshalJSON implements json.Marshaler with type-discriminated messages.
func (tx Transaction) MarshalJSON() ([]byte, error) {
	envelopes := make([]MessageEnvelope, len(tx.Messages))
	for i, msg := range tx.Messages {
		value, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("marshal message %d: %w", i, err)
		}
		envelopes[i] = MessageEnvelope{
			Type:  msg.Type(),
			Value: value,
		}
	}

	return json.Marshal(transactionJSON{
		Account:       tx.Account,
		Messages:      envelopes,
		Authorization: tx.Authorization,
		Nonce:         tx.Nonce,
		Memo:          tx.Memo,
		Fee:           tx.Fee,
	})
}

// UnmarshalJSON implements json.Unmarshaler with type-discriminated messages.
func (tx *Transaction) UnmarshalJSON(data []byte) error {
	var raw transactionJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("unmarshal transaction: %w", err)
	}

	msgs := make([]Message, len(raw.Messages))
	for i, env := range raw.Messages {
		msg, err := UnmarshalMessageJSON(mustMarshalEnvelope(env))
		if err != nil {
			return fmt.Errorf("unmarshal message %d: %w", i, err)
		}
		msgs[i] = msg
	}

	tx.Account = raw.Account
	tx.Messages = msgs
	tx.Authorization = raw.Authorization
	tx.Nonce = raw.Nonce
	tx.Memo = raw.Memo
	tx.Fee = raw.Fee
	return nil
}

// mustMarshalEnvelope marshals a MessageEnvelope back to JSON bytes.
func mustMarshalEnvelope(env MessageEnvelope) []byte {
	data, err := json.Marshal(env)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal message envelope: %v", err))
	}
	return data
}

// VerifyAuthorization verifies the transaction authorization
func (tx *Transaction) VerifyAuthorization(account *Account, getter AccountGetter) error {
	// Check nonce
	if tx.Nonce != account.Nonce {
		return fmt.Errorf("%w: expected nonce %d, got %d", ErrInvalidTransaction, account.Nonce, tx.Nonce)
	}

	// Get sign bytes
	signBytes := tx.GetSignBytes()

	// Verify authorization
	return tx.Authorization.VerifyAuthorization(account, signBytes, getter)
}
