package types

// Message is the interface that all messages must implement
type Message interface {
	// Type returns the message type identifier (e.g., "/punnet.bank.v1.MsgSend")
	Type() string

	// ValidateBasic performs stateless validation
	ValidateBasic() error

	// GetSigners returns the accounts that must authorize this message
	GetSigners() []AccountName
}
