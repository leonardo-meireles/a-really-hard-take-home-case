package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	// Database paths
	SQLitePath string `mapstructure:"sqlite-path"`
	FSMDBPath  string `mapstructure:"fsm-db-path"`

	// S3 configuration
	S3Bucket string `mapstructure:"s3-bucket"`
	S3Region string `mapstructure:"s3-region"`

	// Working directory
	WorkDir string `mapstructure:"work-dir"`

	// Security limits
	MaxFileSize         int64   `mapstructure:"max-file-size"`
	MaxTotalSize        int64   `mapstructure:"max-total-size"`
	MaxCompressionRatio float64 `mapstructure:"max-compression-ratio"`

	// Feature flags
	DMEnabled bool `mapstructure:"dm-enabled"`

	// FSM configuration
	FSMMaxRetries int `mapstructure:"fsm-max-retries"`
}

// Load reads configuration from environment, config file, and defaults
func Load() (*Config, error) {
	// Set defaults
	viper.SetDefault("sqlite-path", ".artifacts/images.db")
	viper.SetDefault("fsm-db-path", ".artifacts/fsm.db")
	viper.SetDefault("s3-bucket", "flyio-platform-hiring-challenge")
	viper.SetDefault("s3-region", "us-east-1")
	viper.SetDefault("work-dir", "/tmp/flyio-machine")
	viper.SetDefault("max-file-size", 2*1024*1024*1024)
	viper.SetDefault("max-total-size", 20*1024*1024*1024)
	viper.SetDefault("max-compression-ratio", 100.0)
	viper.SetDefault("dm-enabled", false)
	viper.SetDefault("fsm-max-retries", 5)

	// Environment variables (will be FLYIO_SQLITE_PATH, etc.)
	viper.SetEnvPrefix("FLYIO")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))

	// Config file (optional)
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/.flyio")

	// Read config file (ignore if not found)
	_ = viper.ReadInConfig()

	// Unmarshal into config struct
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &cfg, nil
}

// Validate checks configuration for errors
func (c *Config) Validate() error {
	if c.SQLitePath == "" {
		return fmt.Errorf("sqlite-path cannot be empty")
	}
	if c.FSMDBPath == "" {
		return fmt.Errorf("fsm-db-path cannot be empty")
	}
	if c.S3Bucket == "" {
		return fmt.Errorf("s3-bucket cannot be empty")
	}
	if c.MaxFileSize <= 0 {
		return fmt.Errorf("max-file-size must be positive")
	}
	if c.MaxTotalSize <= 0 {
		return fmt.Errorf("max-total-size must be positive")
	}
	if c.MaxCompressionRatio <= 0 {
		return fmt.Errorf("max-compression-ratio must be positive")
	}
	if c.FSMMaxRetries < 0 {
		return fmt.Errorf("fsm-max-retries must be non-negative")
	}
	return nil
}
