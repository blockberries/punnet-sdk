package types

// TxResult represents the result of transaction execution
type TxResult struct {
	// Code is the response code (0 = success)
	Code uint32 `json:"code"`

	// Data is the response data
	Data []byte `json:"data,omitempty"`

	// Log is the execution log
	Log string `json:"log,omitempty"`

	// Events are the events emitted during execution
	Events []Event `json:"events,omitempty"`

	// GasUsed is the amount of gas consumed
	GasUsed uint64 `json:"gas_used"`
}

// IsOK returns true if the transaction succeeded
func (r *TxResult) IsOK() bool {
	return r.Code == 0
}

// Event represents a blockchain event
type Event struct {
	// Type is the event type
	Type string `json:"type"`

	// Attributes are the event attributes
	Attributes []EventAttribute `json:"attributes"`
}

// EventAttribute represents a single event attribute
type EventAttribute struct {
	// Key is the attribute key
	Key string `json:"key"`

	// Value is the attribute value
	Value []byte `json:"value"`
}

// NewEvent creates a new event
func NewEvent(typ string) Event {
	return Event{
		Type:       typ,
		Attributes: make([]EventAttribute, 0),
	}
}

// AddAttribute adds an attribute to the event
func (e *Event) AddAttribute(key string, value []byte) {
	e.Attributes = append(e.Attributes, EventAttribute{
		Key:   key,
		Value: value,
	})
}

// QueryResult represents the result of a query
type QueryResult struct {
	// Code is the response code (0 = success)
	Code uint32 `json:"code"`

	// Data is the query response data
	Data []byte `json:"data,omitempty"`

	// Log is the query log
	Log string `json:"log,omitempty"`

	// Height is the block height at which the query was executed
	Height uint64 `json:"height"`
}

// IsOK returns true if the query succeeded
func (r *QueryResult) IsOK() bool {
	return r.Code == 0
}

// ValidatorUpdate represents a validator set update
type ValidatorUpdate struct {
	// PubKey is the validator's public key
	PubKey []byte `json:"pub_key"`

	// Power is the validator's voting power
	Power int64 `json:"power"`
}

// EndBlockResult represents the result of EndBlock
type EndBlockResult struct {
	// ValidatorUpdates are changes to the validator set
	ValidatorUpdates []ValidatorUpdate `json:"validator_updates,omitempty"`

	// Events are the events emitted during EndBlock
	Events []Event `json:"events,omitempty"`
}

// CommitResult represents the result of Commit
type CommitResult struct {
	// AppHash is the application state hash
	AppHash []byte `json:"app_hash"`

	// Height is the committed block height
	Height uint64 `json:"height"`
}
