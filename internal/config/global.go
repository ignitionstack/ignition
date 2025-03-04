package config

import (
	"os"
	"path/filepath"

	"github.com/ignitionstack/ignition/pkg/engine/config"
)

// DefaultSocketPath returns the default socket path used by the engine
func DefaultSocketPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return filepath.Join(homeDir, ".ignition", "engine.sock")
}

// Global configuration variables
var (
	// ConfigPath is the path to the configuration file (only used by engine start command)
	ConfigPath = config.DefaultConfigPath

	// DefaultSocket is the default path to the engine socket
	DefaultSocket = DefaultSocketPath()
)
