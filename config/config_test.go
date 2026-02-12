package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.App.ChainID != "punnet-chain-1" {
		t.Errorf("expected chain_id 'punnet-chain-1', got %s", cfg.App.ChainID)
	}

	if cfg.Consensus.MaxBlockBytes != 22020096 {
		t.Errorf("expected max_block_bytes 22020096, got %d", cfg.Consensus.MaxBlockBytes)
	}

	if cfg.Modules.Staking.BondDenom != "stake" {
		t.Errorf("expected bond_denom 'stake', got %s", cfg.Modules.Staking.BondDenom)
	}

	if cfg.Modules.Governance.Quorum != 3333 {
		t.Errorf("expected quorum 3333, got %d", cfg.Modules.Governance.Quorum)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "empty chain_id",
			modify: func(c *Config) {
				c.App.ChainID = ""
			},
			wantErr: true,
		},
		{
			name: "zero max_block_bytes",
			modify: func(c *Config) {
				c.Consensus.MaxBlockBytes = 0
			},
			wantErr: true,
		},
		{
			name: "max_tx_bytes exceeds max_block_bytes",
			modify: func(c *Config) {
				c.Consensus.MaxTxBytes = c.Consensus.MaxBlockBytes + 1
			},
			wantErr: true,
		},
		{
			name: "empty bond_denom",
			modify: func(c *Config) {
				c.Modules.Staking.BondDenom = ""
			},
			wantErr: true,
		},
		{
			name: "zero max_validators",
			modify: func(c *Config) {
				c.Modules.Staking.MaxValidators = 0
			},
			wantErr: true,
		},
		{
			name: "quorum exceeds 10000",
			modify: func(c *Config) {
				c.Modules.Governance.Quorum = 10001
			},
			wantErr: true,
		},
		{
			name: "invalid pruning_strategy",
			modify: func(c *Config) {
				c.App.PruningStrategy = "invalid"
			},
			wantErr: true,
		},
		{
			name: "invalid logging level",
			modify: func(c *Config) {
				c.Logging.Level = "trace"
			},
			wantErr: true,
		},
		{
			name: "invalid logging format",
			modify: func(c *Config) {
				c.Logging.Format = "yaml"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "app.toml")

	// Create and save config
	cfg := DefaultConfig()
	cfg.App.ChainID = "test-chain-1"
	cfg.Modules.Governance.MinDeposit = 5000000

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Load config
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.App.ChainID != "test-chain-1" {
		t.Errorf("expected chain_id 'test-chain-1', got %s", loaded.App.ChainID)
	}

	if loaded.Modules.Governance.MinDeposit != 5000000 {
		t.Errorf("expected min_deposit 5000000, got %d", loaded.Modules.Governance.MinDeposit)
	}
}

func TestConfigLoadOrCreate(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "subdir", "app.toml")

	// File doesn't exist, should create default
	cfg, err := LoadOrCreate(configPath)
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}

	if cfg.App.ChainID != "punnet-chain-1" {
		t.Errorf("expected default chain_id, got %s", cfg.App.ChainID)
	}

	// File should now exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Modify and reload
	cfg.App.ChainID = "modified-chain"
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reloaded, err := LoadOrCreate(configPath)
	if err != nil {
		t.Fatalf("LoadOrCreate() error = %v", err)
	}

	if reloaded.App.ChainID != "modified-chain" {
		t.Errorf("expected chain_id 'modified-chain', got %s", reloaded.App.ChainID)
	}
}

func TestLoadNonExistent(t *testing.T) {
	_, err := Load("/nonexistent/path/app.toml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "app.toml")

	// Write invalid TOML
	if err := os.WriteFile(configPath, []byte("this is not valid toml {{"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}

func TestDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"1s", time.Second, false},
		{"5m", 5 * time.Minute, false},
		{"24h", 24 * time.Hour, false},
		{"1h30m", 90 * time.Minute, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalText([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("UnmarshalText() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && d.Duration != tt.expected {
				t.Errorf("UnmarshalText() = %v, want %v", d.Duration, tt.expected)
			}
		})
	}
}

func TestDurationMarshal(t *testing.T) {
	d := Duration{5 * time.Minute}
	text, err := d.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}
	if string(text) != "5m0s" {
		t.Errorf("MarshalText() = %s, want '5m0s'", string(text))
	}
}

func TestConfigClone(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RPC.CORSAllowedOrigins = []string{"http://localhost:3000"}
	cfg.Logging.ModuleLevels = map[string]string{
		"auth": "debug",
		"bank": "warn",
	}

	clone := cfg.Clone()

	// Modify original
	cfg.App.ChainID = "modified"
	cfg.RPC.CORSAllowedOrigins[0] = "http://modified"
	cfg.Logging.ModuleLevels["auth"] = "error"

	// Clone should be unchanged
	if clone.App.ChainID == "modified" {
		t.Error("clone was modified when original changed")
	}

	if clone.RPC.CORSAllowedOrigins[0] == "http://modified" {
		t.Error("clone CORS was modified when original changed")
	}

	if clone.Logging.ModuleLevels["auth"] == "error" {
		t.Error("clone module levels was modified when original changed")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "app.toml")

	original := DefaultConfig()
	original.App.ChainID = "roundtrip-chain"
	original.Consensus.TimeoutPropose = Duration{5 * time.Second}
	original.Modules.Staking.UnbondingPeriod = Duration{7 * 24 * time.Hour}
	original.RPC.CORSAllowedOrigins = []string{"http://localhost:3000", "http://localhost:8080"}
	original.Logging.ModuleLevels = map[string]string{
		"governance": "debug",
	}

	// Save
	if err := original.Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Compare
	if loaded.App.ChainID != original.App.ChainID {
		t.Errorf("chain_id mismatch: got %s, want %s", loaded.App.ChainID, original.App.ChainID)
	}

	if loaded.Consensus.TimeoutPropose.Duration != original.Consensus.TimeoutPropose.Duration {
		t.Errorf("timeout_propose mismatch: got %v, want %v",
			loaded.Consensus.TimeoutPropose.Duration, original.Consensus.TimeoutPropose.Duration)
	}

	if loaded.Modules.Staking.UnbondingPeriod.Duration != original.Modules.Staking.UnbondingPeriod.Duration {
		t.Errorf("unbonding_period mismatch: got %v, want %v",
			loaded.Modules.Staking.UnbondingPeriod.Duration, original.Modules.Staking.UnbondingPeriod.Duration)
	}

	if len(loaded.RPC.CORSAllowedOrigins) != len(original.RPC.CORSAllowedOrigins) {
		t.Errorf("CORS origins count mismatch: got %d, want %d",
			len(loaded.RPC.CORSAllowedOrigins), len(original.RPC.CORSAllowedOrigins))
	}

	if loaded.Logging.ModuleLevels["governance"] != "debug" {
		t.Errorf("module_levels mismatch: got %s, want 'debug'",
			loaded.Logging.ModuleLevels["governance"])
	}
}

func TestValidPruningStrategies(t *testing.T) {
	strategies := []string{"nothing", "everything", "custom"}

	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.App.PruningStrategy = strategy
			if err := cfg.Validate(); err != nil {
				t.Errorf("Validate() error = %v for valid strategy %s", err, strategy)
			}
		})
	}
}

func TestValidLoggingLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Logging.Level = level
			if err := cfg.Validate(); err != nil {
				t.Errorf("Validate() error = %v for valid level %s", err, level)
			}
		})
	}
}
