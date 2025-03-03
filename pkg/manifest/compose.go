package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// ComposeManifest represents the structure of an ignition-compose.yml file
type ComposeManifest struct {
	Version  string                    `yaml:"version,omitempty"`
	Services map[string]ComposeService `yaml:"services"`
}

// ComposeService represents a single function service in the compose file
type ComposeService struct {
	Function      string            `yaml:"function"` // namespace/name:tag format
	Config        map[string]string `yaml:"config,omitempty"`
	DependsOn     []string          `yaml:"depends_on,omitempty"`
	HostName      string            `yaml:"hostname,omitempty"`
	RestartPolicy string            `yaml:"restart,omitempty"` // "always", "on-failure", "no"
	Ports         []string          `yaml:"ports,omitempty"`   // For future use with network config
}

// ParseComposeFile parses an ignition-compose.yml file and returns a ComposeManifest
func ParseComposeFile(filePath string) (*ComposeManifest, error) {
	// If no file path is provided, check for default file
	if filePath == "" {
		defaultFiles := []string{"ignition-compose.yml", "ignition-compose.yaml"}
		found := false
		for _, file := range defaultFiles {
			if _, err := os.Stat(file); err == nil {
				filePath = file
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no compose file found, expected %s in current directory", defaultFiles[0])
		}
	}

	// Check if the file exists
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve compose file path: %w", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("compose file not found: %s", absPath)
	}

	// Read and parse the file
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file: %w", err)
	}

	var manifest ComposeManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse compose file: %w", err)
	}

	// Validate the manifest
	if len(manifest.Services) == 0 {
		return nil, fmt.Errorf("compose file must contain at least one service")
	}

	for name, service := range manifest.Services {
		if service.Function == "" {
			return nil, fmt.Errorf("service '%s' is missing required 'function' field", name)
		}
	}

	return &manifest, nil
}
