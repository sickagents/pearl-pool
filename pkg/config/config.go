package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Pool     PoolConfig
	Node     NodeConfig
	Database DatabaseConfig
	Redis    RedisConfig
	Stratum  StratumConfig
	Payout   PayoutConfig
	API      APIConfig
}

type PoolConfig struct {
	Name                string
	Fee                 float64 // Pool fee percentage (e.g., 1.0 = 1%)
	RewardMode          string  // "pplns" or "prop"
	PPLNSWindow         int     // Number of shares in PPLNS window
	MinPayout           float64 // Minimum payout threshold in PEARL
	PayoutInterval      time.Duration
	ConfirmationDepth   int // Blocks to wait before crediting rewards
	SoloMiningEnabled   bool
	MaxWorkersPerMiner  int
	ShareDifficultyBase float64 // Base difficulty for share validation
}

type NodeConfig struct {
	Host            string
	Port            int
	RPCUser         string
	RPCPass         string
	TLS             bool
	PollInterval    time.Duration // Block template polling interval
	SubmitTimeout   time.Duration
	ValidationMode  string // "node" or "embedded" (for ZK proof validation)
}

type DatabaseConfig struct {
	Host         string
	Port         int
	User         string
	Password     string
	Database     string
	MaxConns     int
	MaxIdleConns int
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

type StratumConfig struct {
	Ports []StratumPort
}

type StratumPort struct {
	Port       int
	TLS        bool
	Difficulty float64 // Starting difficulty for this port
	MaxHashrate float64 // Recommended max hashrate (0 = unlimited)
}

type PayoutConfig struct {
	Enabled         bool
	Interval        time.Duration
	MinPayout       float64
	MaxBatchSize    int // Max number of payouts per transaction
	GasBuffer       float64 // Extra PEARL to reserve for tx fees
}

type APIConfig struct {
	Host            string
	Port            int
	TLS             bool
	CertFile        string
	KeyFile         string
	EnableMetrics   bool
	MetricsPath     string
	CORSEnabled     bool
	CORSOrigins     []string
}

// Load reads config from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()
	
	// Set defaults
	setDefaults(v)
	
	// Read config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/pearl-pool")
	}
	
	// Environment variables override
	v.SetEnvPrefix("PEARL_POOL")
	v.AutomaticEnv()
	
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Validation
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	// Pool defaults
	v.SetDefault("pool.name", "PEARL Pool")
	v.SetDefault("pool.fee", 1.0)
	v.SetDefault("pool.rewardMode", "pplns")
	v.SetDefault("pool.pplnsWindow", 10000)
	v.SetDefault("pool.minPayout", 10.0)
	v.SetDefault("pool.payoutInterval", "10m")
	v.SetDefault("pool.confirmationDepth", 100)
	v.SetDefault("pool.soloMiningEnabled", true)
	v.SetDefault("pool.maxWorkersPerMiner", 100)
	v.SetDefault("pool.shareDifficultyBase", 2000000.0)
	
	// Node defaults
	v.SetDefault("node.host", "localhost")
	v.SetDefault("node.port", 44107)
	v.SetDefault("node.tls", true)
	v.SetDefault("node.pollInterval", "30s")
	v.SetDefault("node.submitTimeout", "10s")
	v.SetDefault("node.validationMode", "node")
	
	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.database", "pearl_pool")
	v.SetDefault("database.maxConns", 50)
	v.SetDefault("database.maxIdleConns", 10)
	
	// Redis defaults
	v.SetDefault("redis.host", "localhost")
	v.SetDefault("redis.port", 6379)
	v.SetDefault("redis.db", 0)
	
	// Stratum defaults
	v.SetDefault("stratum.ports", []StratumPort{
		{Port: 3360, TLS: false, Difficulty: 2000000, MaxHashrate: 500000000000000}, // <500 TH/s
		{Port: 3361, TLS: false, Difficulty: 4000000, MaxHashrate: 1000000000000000}, // 500-1000 TH/s
		{Port: 3362, TLS: false, Difficulty: 8000000, MaxHashrate: 0}, // >1000 TH/s
	})
	
	// Payout defaults
	v.SetDefault("payout.enabled", true)
	v.SetDefault("payout.interval", "15m")
	v.SetDefault("payout.minPayout", 10.0)
	v.SetDefault("payout.maxBatchSize", 50)
	v.SetDefault("payout.gasBuffer", 0.1)
	
	// API defaults
	v.SetDefault("api.host", "0.0.0.0")
	v.SetDefault("api.port", 8080)
	v.SetDefault("api.tls", false)
	v.SetDefault("api.enableMetrics", true)
	v.SetDefault("api.metricsPath", "/metrics")
	v.SetDefault("api.corsEnabled", true)
	v.SetDefault("api.corsOrigins", []string{"*"})
}

func (c *Config) Validate() error {
	if c.Pool.Fee < 0 || c.Pool.Fee > 100 {
		return fmt.Errorf("pool fee must be between 0 and 100")
	}
	
	if c.Pool.RewardMode != "pplns" && c.Pool.RewardMode != "prop" {
		return fmt.Errorf("rewardMode must be 'pplns' or 'prop'")
	}
	
	if c.Pool.MinPayout <= 0 {
		return fmt.Errorf("minPayout must be positive")
	}
	
	if c.Pool.ConfirmationDepth < 1 {
		return fmt.Errorf("confirmationDepth must be at least 1")
	}
	
	if c.Node.ValidationMode != "node" && c.Node.ValidationMode != "embedded" {
		return fmt.Errorf("node.validationMode must be 'node' or 'embedded'")
	}
	
	if len(c.Stratum.Ports) == 0 {
		return fmt.Errorf("at least one stratum port must be configured")
	}
	
	return nil
}
