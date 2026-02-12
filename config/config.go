// Package config provides application configuration management for Punnet SDK applications.
// It supports loading configuration from TOML files and environment variable overrides.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Config is the root configuration for a Punnet SDK application.
type Config struct {
	// App contains application-level configuration.
	App AppConfig `toml:"app"`

	// Consensus contains consensus parameter configuration.
	Consensus ConsensusConfig `toml:"consensus"`

	// Modules contains module-specific configuration.
	Modules ModulesConfig `toml:"modules"`

	// RPC contains RPC server configuration.
	RPC RPCConfig `toml:"rpc"`

	// Logging contains logging configuration.
	Logging LoggingConfig `toml:"logging"`
}

// AppConfig contains application-level configuration.
type AppConfig struct {
	// ChainID is the unique identifier for this blockchain.
	ChainID string `toml:"chain_id"`

	// DataDir is the directory for persistent data storage.
	DataDir string `toml:"data_dir"`

	// MinGasPrice is the minimum gas price for transactions (in base denom).
	MinGasPrice uint64 `toml:"min_gas_price"`

	// IndexEvents enables transaction event indexing.
	IndexEvents bool `toml:"index_events"`

	// PruningStrategy defines how historical state is pruned.
	// Options: "nothing", "everything", "custom"
	PruningStrategy string `toml:"pruning_strategy"`

	// PruningKeepRecent is the number of recent heights to keep when using custom pruning.
	PruningKeepRecent uint64 `toml:"pruning_keep_recent"`

	// PruningInterval is how often pruning runs (in blocks).
	PruningInterval uint64 `toml:"pruning_interval"`
}

// ConsensusConfig contains consensus parameter configuration.
type ConsensusConfig struct {
	// MaxBlockBytes is the maximum size of a block in bytes.
	MaxBlockBytes int64 `toml:"max_block_bytes"`

	// MaxTxBytes is the maximum size of a transaction in bytes.
	MaxTxBytes int64 `toml:"max_tx_bytes"`

	// MaxGasPerBlock is the maximum gas allowed in a block.
	MaxGasPerBlock int64 `toml:"max_gas_per_block"`

	// MaxEvidenceAge is the maximum age of evidence (duration string).
	MaxEvidenceAge Duration `toml:"max_evidence_age"`

	// TimeoutPropose is the timeout for the propose phase (duration string).
	TimeoutPropose Duration `toml:"timeout_propose"`

	// TimeoutPrevote is the timeout for the prevote phase.
	TimeoutPrevote Duration `toml:"timeout_prevote"`

	// TimeoutPrecommit is the timeout for the precommit phase.
	TimeoutPrecommit Duration `toml:"timeout_precommit"`

	// TimeoutCommit is the timeout for the commit phase.
	TimeoutCommit Duration `toml:"timeout_commit"`
}

// ModulesConfig contains configuration for all modules.
type ModulesConfig struct {
	// Auth contains auth module configuration.
	Auth AuthModuleConfig `toml:"auth"`

	// Bank contains bank module configuration.
	Bank BankModuleConfig `toml:"bank"`

	// Staking contains staking module configuration.
	Staking StakingModuleConfig `toml:"staking"`

	// Governance contains governance module configuration.
	Governance GovernanceModuleConfig `toml:"governance"`
}

// AuthModuleConfig contains auth module configuration.
type AuthModuleConfig struct {
	// MaxMemoLength is the maximum length of transaction memos.
	MaxMemoLength uint64 `toml:"max_memo_length"`

	// TxSigLimit is the maximum number of signatures per transaction.
	TxSigLimit uint64 `toml:"tx_sig_limit"`

	// SigVerifyCostSecp256k1 is the gas cost for secp256k1 signature verification.
	SigVerifyCostSecp256k1 uint64 `toml:"sig_verify_cost_secp256k1"`

	// SigVerifyCostEd25519 is the gas cost for ed25519 signature verification.
	SigVerifyCostEd25519 uint64 `toml:"sig_verify_cost_ed25519"`
}

// BankModuleConfig contains bank module configuration.
type BankModuleConfig struct {
	// SendEnabled enables or disables token transfers globally.
	SendEnabled bool `toml:"send_enabled"`

	// DefaultSendEnabled is the default for new denominations.
	DefaultSendEnabled bool `toml:"default_send_enabled"`
}

// StakingModuleConfig contains staking module configuration.
type StakingModuleConfig struct {
	// BondDenom is the denomination used for staking.
	BondDenom string `toml:"bond_denom"`

	// UnbondingPeriod is the duration tokens are locked after unbonding.
	UnbondingPeriod Duration `toml:"unbonding_period"`

	// MaxValidators is the maximum number of active validators.
	MaxValidators uint32 `toml:"max_validators"`

	// MinSelfDelegation is the minimum self-delegation for validators.
	MinSelfDelegation uint64 `toml:"min_self_delegation"`

	// MaxEntries is the maximum unbonding entries per delegator/validator pair.
	MaxEntries uint32 `toml:"max_entries"`

	// HistoricalEntries is the number of historical staking entries to retain.
	HistoricalEntries uint32 `toml:"historical_entries"`
}

// GovernanceModuleConfig contains governance module configuration.
type GovernanceModuleConfig struct {
	// MinDeposit is the minimum deposit required to move a proposal to voting.
	MinDeposit uint64 `toml:"min_deposit"`

	// DepositDenom is the denomination for deposits.
	DepositDenom string `toml:"deposit_denom"`

	// DepositPeriod is the duration of the deposit period.
	DepositPeriod Duration `toml:"deposit_period"`

	// VotingPeriod is the duration of the voting period.
	VotingPeriod Duration `toml:"voting_period"`

	// Quorum is the minimum percentage of voting power required (basis points, e.g., 3333 = 33.33%).
	Quorum uint32 `toml:"quorum"`

	// Threshold is the minimum percentage of yes votes required (basis points).
	Threshold uint32 `toml:"threshold"`

	// VetoThreshold is the maximum percentage of veto votes before rejection (basis points).
	VetoThreshold uint32 `toml:"veto_threshold"`

	// BurnVoteVeto burns deposits if proposal is vetoed.
	BurnVoteVeto bool `toml:"burn_vote_veto"`
}

// RPCConfig contains RPC server configuration.
type RPCConfig struct {
	// Enable enables the RPC server.
	Enable bool `toml:"enable"`

	// ListenAddress is the address to listen on.
	ListenAddress string `toml:"listen_address"`

	// MaxOpenConnections is the maximum number of concurrent connections.
	MaxOpenConnections int `toml:"max_open_connections"`

	// ReadTimeout is the read timeout for HTTP requests.
	ReadTimeout Duration `toml:"read_timeout"`

	// WriteTimeout is the write timeout for HTTP requests.
	WriteTimeout Duration `toml:"write_timeout"`

	// CORSAllowedOrigins lists allowed CORS origins.
	CORSAllowedOrigins []string `toml:"cors_allowed_origins"`
}

// LoggingConfig contains logging configuration.
type LoggingConfig struct {
	// Level is the minimum log level ("debug", "info", "warn", "error").
	Level string `toml:"level"`

	// Format is the log format ("json", "text").
	Format string `toml:"format"`

	// ModuleLevels allows per-module log levels.
	ModuleLevels map[string]string `toml:"module_levels"`
}

// Duration is a wrapper for time.Duration that supports TOML marshaling.
type Duration struct {
	time.Duration
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		App: AppConfig{
			ChainID:           "punnet-chain-1",
			DataDir:           "./data",
			MinGasPrice:       0,
			IndexEvents:       true,
			PruningStrategy:   "nothing",
			PruningKeepRecent: 100,
			PruningInterval:   10,
		},
		Consensus: ConsensusConfig{
			MaxBlockBytes:    22020096, // 21 MB
			MaxTxBytes:       1048576,  // 1 MB
			MaxGasPerBlock:   -1,       // unlimited
			MaxEvidenceAge:   Duration{48 * time.Hour},
			TimeoutPropose:   Duration{3 * time.Second},
			TimeoutPrevote:   Duration{1 * time.Second},
			TimeoutPrecommit: Duration{1 * time.Second},
			TimeoutCommit:    Duration{1 * time.Second},
		},
		Modules: ModulesConfig{
			Auth: AuthModuleConfig{
				MaxMemoLength:          256,
				TxSigLimit:             7,
				SigVerifyCostSecp256k1: 1000,
				SigVerifyCostEd25519:   590,
			},
			Bank: BankModuleConfig{
				SendEnabled:        true,
				DefaultSendEnabled: true,
			},
			Staking: StakingModuleConfig{
				BondDenom:         "stake",
				UnbondingPeriod:   Duration{21 * 24 * time.Hour}, // 21 days
				MaxValidators:     100,
				MinSelfDelegation: 1,
				MaxEntries:        7,
				HistoricalEntries: 10000,
			},
			Governance: GovernanceModuleConfig{
				MinDeposit:    1000000,
				DepositDenom:  "stake",
				DepositPeriod: Duration{14 * 24 * time.Hour}, // 14 days
				VotingPeriod:  Duration{14 * 24 * time.Hour}, // 14 days
				Quorum:        3333,                          // 33.33%
				Threshold:     5000,                          // 50%
				VetoThreshold: 3333,                          // 33.33%
				BurnVoteVeto:  true,
			},
		},
		RPC: RPCConfig{
			Enable:             true,
			ListenAddress:      "tcp://0.0.0.0:26657",
			MaxOpenConnections: 900,
			ReadTimeout:        Duration{10 * time.Second},
			WriteTimeout:       Duration{10 * time.Second},
			CORSAllowedOrigins: []string{},
		},
		Logging: LoggingConfig{
			Level:        "info",
			Format:       "text",
			ModuleLevels: map[string]string{},
		},
	}
}

// Load reads configuration from a TOML file.
// If the file doesn't exist, it returns an error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	config := DefaultConfig()
	if err := toml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return config, nil
}

// LoadOrCreate loads configuration from a file, or creates a default if it doesn't exist.
func LoadOrCreate(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		config := DefaultConfig()
		if err := config.Save(path); err != nil {
			return nil, fmt.Errorf("create default config: %w", err)
		}
		return config, nil
	}

	return Load(path)
}

// Save writes the configuration to a TOML file.
func (c *Config) Save(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	// Add header comment
	header := []byte("# Punnet SDK Application Configuration\n# See documentation for detailed configuration options.\n\n")
	data = append(header, data...)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}

	return nil
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	if c.App.ChainID == "" {
		return fmt.Errorf("app.chain_id cannot be empty")
	}

	if c.Consensus.MaxBlockBytes <= 0 {
		return fmt.Errorf("consensus.max_block_bytes must be positive")
	}

	if c.Consensus.MaxTxBytes <= 0 {
		return fmt.Errorf("consensus.max_tx_bytes must be positive")
	}

	if c.Consensus.MaxTxBytes > c.Consensus.MaxBlockBytes {
		return fmt.Errorf("consensus.max_tx_bytes cannot exceed max_block_bytes")
	}

	if c.Modules.Staking.BondDenom == "" {
		return fmt.Errorf("modules.staking.bond_denom cannot be empty")
	}

	if c.Modules.Staking.MaxValidators == 0 {
		return fmt.Errorf("modules.staking.max_validators must be positive")
	}

	if c.Modules.Governance.DepositDenom == "" {
		return fmt.Errorf("modules.governance.deposit_denom cannot be empty")
	}

	if c.Modules.Governance.Quorum > 10000 {
		return fmt.Errorf("modules.governance.quorum cannot exceed 10000 basis points")
	}

	if c.Modules.Governance.Threshold > 10000 {
		return fmt.Errorf("modules.governance.threshold cannot exceed 10000 basis points")
	}

	if c.Modules.Governance.VetoThreshold > 10000 {
		return fmt.Errorf("modules.governance.veto_threshold cannot exceed 10000 basis points")
	}

	switch c.App.PruningStrategy {
	case "nothing", "everything", "custom":
		// valid
	default:
		return fmt.Errorf("invalid pruning_strategy: %s (must be nothing, everything, or custom)", c.App.PruningStrategy)
	}

	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
		// valid
	default:
		return fmt.Errorf("invalid logging.level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}

	switch c.Logging.Format {
	case "json", "text":
		// valid
	default:
		return fmt.Errorf("invalid logging.format: %s (must be json or text)", c.Logging.Format)
	}

	return nil
}

// Clone returns a deep copy of the configuration.
func (c *Config) Clone() *Config {
	clone := *c

	// Deep copy slices and maps
	if c.RPC.CORSAllowedOrigins != nil {
		clone.RPC.CORSAllowedOrigins = make([]string, len(c.RPC.CORSAllowedOrigins))
		copy(clone.RPC.CORSAllowedOrigins, c.RPC.CORSAllowedOrigins)
	}

	if c.Logging.ModuleLevels != nil {
		clone.Logging.ModuleLevels = make(map[string]string, len(c.Logging.ModuleLevels))
		for k, v := range c.Logging.ModuleLevels {
			clone.Logging.ModuleLevels[k] = v
		}
	}

	return &clone
}
