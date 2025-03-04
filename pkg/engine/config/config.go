package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// Configuration constants
const (
	// DefaultConfigPath is the default path to the config file
	DefaultConfigPath = "~/.ignition/config.yaml"

	// EnvPrefix is the prefix for environment variables
	EnvPrefix = "IGNITION_"
)

// Config holds all configuration for the ignition engine
type Config struct {
	// Engine options
	Engine EngineConfig `koanf:"engine"`

	// Server options
	Server ServerConfig `koanf:"server"`
}

// EngineConfig holds engine-specific configuration
type EngineConfig struct {
	// Default timeout for function operations
	DefaultTimeout time.Duration `koanf:"default_timeout"`

	// Capacity of the log store
	LogStoreCapacity int `koanf:"log_store_capacity"`

	// Circuit breaker settings
	CircuitBreaker CircuitBreakerConfig `koanf:"circuit_breaker"`

	// Plugin manager settings
	PluginManager PluginManagerConfig `koanf:"plugin_manager"`
}

// ServerConfig holds server-specific configuration
type ServerConfig struct {
	// Socket path for Unix socket
	SocketPath string `koanf:"socket_path"`

	// HTTP address to listen on
	HTTPAddr string `koanf:"http_addr"`

	// Registry directory path
	RegistryDir string `koanf:"registry_dir"`
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	// Failure threshold before circuit opens
	FailureThreshold int `koanf:"failure_threshold"`

	// Reset timeout after which to try again
	ResetTimeout time.Duration `koanf:"reset_timeout"`
}

// PluginManagerConfig holds plugin manager configuration
type PluginManagerConfig struct {
	// How long to keep unused plugins loaded
	TTL time.Duration `koanf:"ttl"`

	// How often to run the cleanup routine
	CleanupInterval time.Duration `koanf:"cleanup_interval"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	return &Config{
		Engine: EngineConfig{
			DefaultTimeout:   30 * time.Second,
			LogStoreCapacity: 1000,
			CircuitBreaker: CircuitBreakerConfig{
				FailureThreshold: 5,
				ResetTimeout:     30 * time.Second,
			},
			PluginManager: PluginManagerConfig{
				TTL:             10 * time.Minute,
				CleanupInterval: 1 * time.Minute,
			},
		},
		Server: ServerConfig{
			SocketPath:  filepath.Join(homeDir, ".ignition", "engine.sock"),
			HTTPAddr:    "localhost:8080",
			RegistryDir: filepath.Join(homeDir, ".ignition", "registry"),
		},
	}
}

// LoadConfig loads configuration from the specified path and environment variables
func LoadConfig(configPath string) (*Config, error) {
	k := koanf.New(".")

	// Set default values
	defaultConfig := DefaultConfig()
	err := k.Load(newStructProvider(defaultConfig), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to load default config: %w", err)
	}

	// Expand tilde in config path if needed
	expandedPath := configPath
	if strings.HasPrefix(configPath, "~/") {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			expandedPath = filepath.Join(homeDir, configPath[2:])
		}
	}

	// Try to load from config file (if it exists)
	if _, err := os.Stat(expandedPath); err == nil {
		if err := k.Load(file.Provider(expandedPath), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Load from environment variables
	if err := k.Load(env.Provider(EnvPrefix, ".", func(s string) string {
		return strings.Replace(strings.ToLower(strings.TrimPrefix(s, EnvPrefix)), "_", ".", -1)
	}), nil); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	// Unmarshal into Config struct
	var config Config
	if err := k.UnmarshalWithConf("", &config, koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook: mapstructure.ComposeDecodeHookFunc(
				mapstructure.StringToTimeDurationHookFunc(),
				mapstructure.StringToSliceHookFunc(","),
			),
			Result:      &config,
			ErrorUnused: true,
		},
	}); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &config, nil
}

// structProvider is a provider that loads configuration from a struct
type structProvider struct {
	cfg interface{}
}

// newStructProvider creates a new struct provider
func newStructProvider(cfg interface{}) *structProvider {
	return &structProvider{cfg: cfg}
}

// Read reads the configuration from the struct
func (s *structProvider) Read() (map[string]interface{}, error) {
	var out map[string]interface{}

	// Use mapstructure to convert struct to map
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &out,
		TagName: "koanf",
	})
	if err != nil {
		return nil, err
	}

	if err := decoder.Decode(s.cfg); err != nil {
		return nil, err
	}

	return out, nil
}

// ReadBytes is required by the Provider interface but not used for struct providers
func (s *structProvider) ReadBytes() ([]byte, error) {
	return nil, fmt.Errorf("ReadBytes not supported for struct provider")
}
