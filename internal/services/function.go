package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/ignitionstack/ignition/pkg/builders"
	"github.com/ignitionstack/ignition/pkg/manifest"
)

// FunctionService defines the interface for function-related operations
type FunctionService interface {
	// InitFunction initializes a new function with the given name and language
	InitFunction(name string, language string) error

	// BuildFunction builds a function and returns the build result
	BuildFunction(path string, functionConfig manifest.FunctionManifest) (result *BuildResult, err error)

	// CalculateHash computes a hash for a function based on its source code and config
	CalculateHash(path string, config manifest.FunctionManifest) (*BuildResult, error)
}

// BuildResult contains information about a successful function build
type BuildResult struct {
	Name   string // Function name
	Path   string // Path to the built WASM file
	Digest string // Content hash of the built WASM file
}

// functionService implements the FunctionService interface
type functionService struct {
	builderFactory BuilderFactory
}

// NewFunctionService creates a new instance of the function service
func NewFunctionService() FunctionService {
	return &functionService{
		builderFactory: NewBuilderFactory(),
	}
}

// BuildFunction builds a function and calculates its hash
func (f *functionService) BuildFunction(path string, functionConfig manifest.FunctionManifest) (*BuildResult, error) {
	language := functionConfig.FunctionSettings.Language
	if language == "" {
		return nil, errors.New("language not specified in function config")
	}

	// Get the appropriate builder for the language
	builder, err := f.builderFactory.GetBuilder(language)
	if err != nil {
		return nil, fmt.Errorf("builder initialization failed: %w", err)
	}

	// Build the function
	buildResult, err := builder.Build(path)
	if err != nil {
		return nil, fmt.Errorf("build failed: %w", err)
	}

	// Calculate hash of the built WASM file
	checksum, err := hashFile(buildResult.OutputPath)
	if err != nil {
		return nil, fmt.Errorf("hash calculation failed: %w", err)
	}

	return &BuildResult{
		Name:   functionConfig.FunctionSettings.Name,
		Path:   buildResult.OutputPath,
		Digest: checksum,
	}, nil
}

// InitFunction initializes a new function from a template
func (f *functionService) InitFunction(name string, language string) error {
	// Validate inputs
	if name == "" {
		return errors.New("function name cannot be empty")
	}

	// Check if language is supported
	templateURL, err := getTemplateURL(language)
	if err != nil {
		return err
	}

	// Create the function directory path
	path := fmt.Sprintf("./%s", name)

	// Check if directory already exists
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return errors.New("directory already exists")
	}

	// Clone the template repository
	if err := cloneTemplate(path, templateURL); err != nil {
		return err
	}

	// Create and write the manifest file
	if err := createManifestFile(path, name, language); err != nil {
		return err
	}

	return nil
}

// CalculateHash computes a hash for a function based on its source code and config
func (f *functionService) CalculateHash(path string, config manifest.FunctionManifest) (*BuildResult, error) {
	// Create a new hash
	h := sha256.New()

	// Hash the source code
	if err := hashSourceCode(h, path); err != nil {
		return nil, err
	}

	// Hash the config
	if err := hashConfig(h, config); err != nil {
		return nil, err
	}

	// Create the final digest
	digest := fmt.Sprintf("sha256:%x", h.Sum(nil))

	return &BuildResult{
		Name:   config.FunctionSettings.Name,
		Path:   path,
		Digest: digest,
	}, nil
}

// BuilderFactory creates language-specific builders
type BuilderFactory interface {
	GetBuilder(language string) (builders.Builder, error)
}

// defaultBuilderFactory is the default implementation of BuilderFactory
type defaultBuilderFactory struct{}

// NewBuilderFactory creates a new builder factory
func NewBuilderFactory() BuilderFactory {
	return &defaultBuilderFactory{}
}

// GetBuilder returns a builder for the specified language
func (f *defaultBuilderFactory) GetBuilder(language string) (builders.Builder, error) {
	switch strings.ToLower(language) {
	case "rust":
		return builders.NewRustBuilder(), nil
	case "typescript":
		return builders.NewJSBuilder(), nil
	case "javascript":
		builder := builders.NewJSBuilder()
		if err := builder.VerifyDependencies(); err != nil {
			return nil, err
		}
		return builder, nil
	case "golang":
		builder := builders.NewGoBuilder()
		if err := builder.VerifyDependencies(); err != nil {
			return nil, err
		}
		return builder, nil
	default:
		return nil, fmt.Errorf("unsupported language: %s", language)
	}
}

// getTemplateURL returns the URL for the template repository for a given language
func getTemplateURL(language string) (string, error) {
	templates := map[string]string{
		"golang":     "https://github.com/extism/go-pdk-template",
		"javascript": "https://github.com/extism/js-pdk-template",
		"typescript": "https://github.com/extism/ts-pdk-template",
		"rust":       "https://github.com/extism/rust-pdk-template",
	}

	url, ok := templates[strings.ToLower(language)]
	if !ok {
		return "", fmt.Errorf("language not supported: %s", language)
	}

	return url, nil
}

// cloneTemplate clones a template repository to the specified path
func cloneTemplate(path, url string) error {
	// Clone the template repository
	_, err := git.PlainClone(path, false, &git.CloneOptions{
		URL: url,
	})
	if err != nil {
		return fmt.Errorf("error cloning template: %w", err)
	}

	// Remove the .git directory to start fresh
	if err := os.RemoveAll(filepath.Join(path, ".git")); err != nil {
		return fmt.Errorf("failed to remove .git directory: %w", err)
	}

	return nil
}

// createManifestFile creates a new manifest file for the function
func createManifestFile(path, name, language string) error {
	// Create the function manifest
	functionManifest := manifest.FunctionManifest{
		FunctionSettings: manifest.FunctionSettings{
			Name:     name,
			Language: language,
			VersionSettings: manifest.FunctionVersionSettings{
				Wasi:        true,
				AllowedUrls: []string{},
			},
		},
	}

	// Marshal to YAML
	marshalledManifest, err := functionManifest.MarhsalYaml()
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Write to file
	manifestPath := filepath.Join(path, "ignition.yml")
	if err := os.WriteFile(manifestPath, marshalledManifest, 0644); err != nil {
		return fmt.Errorf("failed to write manifest file: %w", err)
	}

	return nil
}

// hashFile calculates a SHA-256 hash of a file
func hashFile(filePath string) (string, error) {
	// Create a new hasher
	hasher := sha256.New()

	// Open the file for reading
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for hashing: %w", err)
	}
	defer file.Close()

	// Copy file content to hasher
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to read file for hashing: %w", err)
	}

	// Get the hash sum and encode it to a hex string
	checksum := hex.EncodeToString(hasher.Sum(nil))
	return checksum, nil
}

// hashSourceCode walks through a directory and hashes all source files
func hashSourceCode(hasher io.Writer, path string) error {
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Skip build artifacts and other non-source files
		if shouldSkipFile(filePath) {
			return nil
		}

		// Read and hash the file content
		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", filePath, err)
		}

		if _, err := hasher.Write(fileContent); err != nil {
			return fmt.Errorf("failed to hash file %s: %w", filePath, err)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to hash source code: %w", err)
	}

	return nil
}

// hashConfig hashes a function's configuration
func hashConfig(hasher io.Writer, config manifest.FunctionManifest) error {
	// Marshal the config to JSON
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write the config bytes to the hasher
	if _, err := hasher.Write(configBytes); err != nil {
		return fmt.Errorf("failed to hash config: %w", err)
	}
	return nil
}

// shouldSkipFile determines if a file should be excluded from hashing
func shouldSkipFile(path string) bool {
	skipPatterns := []string{
		".git",
		"node_modules",
		"target",
		"build",
		"dist",
	}

	for _, pattern := range skipPatterns {
		if strings.Contains(path, pattern) {
			return true
		}
	}

	return false
}
