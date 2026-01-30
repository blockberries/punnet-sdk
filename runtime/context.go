package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/blockberries/punnet-sdk/effects"
	"github.com/blockberries/punnet-sdk/types"
)

// BlockHeader contains block metadata
type BlockHeader struct {
	// Height is the block height
	Height uint64

	// Time is the block timestamp
	Time time.Time

	// ChainID is the blockchain identifier
	ChainID string

	// ProposerAddress is the address of the block proposer
	ProposerAddress []byte
}

// NewBlockHeader creates a new block header
func NewBlockHeader(height uint64, timestamp time.Time, chainID string, proposer []byte) *BlockHeader {
	// Create defensive copy of proposer address
	proposerCopy := make([]byte, len(proposer))
	copy(proposerCopy, proposer)

	return &BlockHeader{
		Height:          height,
		Time:            timestamp,
		ChainID:         chainID,
		ProposerAddress: proposerCopy,
	}
}

// ValidateBasic performs basic validation
func (h *BlockHeader) ValidateBasic() error {
	if h == nil {
		return fmt.Errorf("block header is nil")
	}

	if h.Height == 0 {
		return fmt.Errorf("block height cannot be zero")
	}

	if h.ChainID == "" {
		return fmt.Errorf("chain ID cannot be empty")
	}

	if h.Time.IsZero() {
		return fmt.Errorf("block time cannot be zero")
	}

	return nil
}

// Context provides execution context for message handlers
// It carries block information, transaction account, and effect collection
type Context struct {
	// ctx is the underlying Go context
	ctx context.Context

	// header contains block metadata
	header *BlockHeader

	// account is the account executing the current transaction
	account types.AccountName

	// collector accumulates effects from message handlers
	collector *effects.Collector

	// readOnly indicates if this is a read-only context (for CheckTx)
	readOnly bool

	// gasUsed tracks gas consumption (placeholder for future gas metering)
	gasUsed uint64
}

// NewContext creates a new execution context
func NewContext(ctx context.Context, header *BlockHeader, account types.AccountName) (*Context, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	if header == nil {
		return nil, fmt.Errorf("block header cannot be nil")
	}

	if err := header.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("invalid block header: %w", err)
	}

	if !account.IsValid() {
		return nil, fmt.Errorf("invalid account: %s", account)
	}

	return &Context{
		ctx:       ctx,
		header:    header,
		account:   account,
		collector: effects.NewCollector(),
		readOnly:  false,
		gasUsed:   0,
	}, nil
}

// NewReadOnlyContext creates a read-only context for CheckTx
func NewReadOnlyContext(ctx context.Context, header *BlockHeader, account types.AccountName) (*Context, error) {
	rctx, err := NewContext(ctx, header, account)
	if err != nil {
		return nil, err
	}

	rctx.readOnly = true
	return rctx, nil
}

// Context returns the underlying Go context
func (c *Context) Context() context.Context {
	if c == nil {
		return context.Background()
	}
	return c.ctx
}

// BlockHeight returns the current block height
func (c *Context) BlockHeight() uint64 {
	if c == nil || c.header == nil {
		return 0
	}
	return c.header.Height
}

// BlockTime returns the current block time
func (c *Context) BlockTime() time.Time {
	if c == nil || c.header == nil {
		return time.Time{}
	}
	return c.header.Time
}

// ChainID returns the chain identifier
func (c *Context) ChainID() string {
	if c == nil || c.header == nil {
		return ""
	}
	return c.header.ChainID
}

// ProposerAddress returns the block proposer address
func (c *Context) ProposerAddress() []byte {
	if c == nil || c.header == nil {
		return nil
	}

	// Return defensive copy
	proposer := make([]byte, len(c.header.ProposerAddress))
	copy(proposer, c.header.ProposerAddress)
	return proposer
}

// Account returns the account executing the current transaction
func (c *Context) Account() types.AccountName {
	if c == nil {
		return ""
	}
	return c.account
}

// IsReadOnly returns true if this is a read-only context
func (c *Context) IsReadOnly() bool {
	if c == nil {
		return true
	}
	return c.readOnly
}

// EmitEffect adds an effect to the collector
// Returns error if context is read-only
func (c *Context) EmitEffect(effect effects.Effect) error {
	if c == nil {
		return fmt.Errorf("context is nil")
	}

	if c.readOnly {
		return fmt.Errorf("cannot emit effects in read-only context")
	}

	if c.collector == nil {
		return fmt.Errorf("effect collector is nil")
	}

	return c.collector.Add(effect)
}

// EmitEffects adds multiple effects to the collector
// Returns error if context is read-only
func (c *Context) EmitEffects(effects []effects.Effect) error {
	if c == nil {
		return fmt.Errorf("context is nil")
	}

	if c.readOnly {
		return fmt.Errorf("cannot emit effects in read-only context")
	}

	if c.collector == nil {
		return fmt.Errorf("effect collector is nil")
	}

	return c.collector.AddMultiple(effects)
}

// CollectEffects returns all collected effects and clears the collector
func (c *Context) CollectEffects() []effects.Effect {
	if c == nil || c.collector == nil {
		return nil
	}
	return c.collector.Collect()
}

// EffectCount returns the number of collected effects
func (c *Context) EffectCount() int {
	if c == nil || c.collector == nil {
		return 0
	}
	return c.collector.Count()
}

// ClearEffects clears all collected effects
func (c *Context) ClearEffects() {
	if c == nil || c.collector == nil {
		return
	}
	c.collector.Clear()
}

// GasUsed returns the amount of gas used (placeholder for future gas metering)
func (c *Context) GasUsed() uint64 {
	if c == nil {
		return 0
	}
	return c.gasUsed
}

// ConsumeGas adds to the gas usage counter (placeholder for future gas metering)
func (c *Context) ConsumeGas(amount uint64) {
	if c == nil {
		return
	}
	c.gasUsed += amount
}

// WithContext returns a new Context with the given Go context
func (c *Context) WithContext(ctx context.Context) *Context {
	if c == nil {
		return nil
	}

	if ctx == nil {
		return c
	}

	return &Context{
		ctx:       ctx,
		header:    c.header,
		account:   c.account,
		collector: c.collector,
		readOnly:  c.readOnly,
		gasUsed:   c.gasUsed,
	}
}

// WithAccount returns a new Context with the given account
func (c *Context) WithAccount(account types.AccountName) (*Context, error) {
	if c == nil {
		return nil, fmt.Errorf("context is nil")
	}

	if !account.IsValid() {
		return nil, fmt.Errorf("invalid account: %s", account)
	}

	return &Context{
		ctx:       c.ctx,
		header:    c.header,
		account:   account,
		collector: effects.NewCollector(), // New collector for new account
		readOnly:  c.readOnly,
		gasUsed:   0, // Reset gas for new account
	}, nil
}
