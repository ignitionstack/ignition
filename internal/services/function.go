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

type FunctionService interface {
	InitFunction(name string, language string) error
	BuildFunction(path string, functionConfig manifest.FunctionManifest) (result *BuildResult, err error)
	CalculateHash(path string, config manifest.FunctionManifest) (*BuildResult, error)
}

type functionService struct{}

type BuildResult struct {
	Name   string
	Path   string
	Digest string
}

// BuildFunction implements FunctionService.
func (f *functionService) BuildFunction(path string, functionConfig manifest.FunctionManifest) (result *BuildResult, err error) {
	// Build
	outputPath, err := f.build(functionConfig.FunctionSettings.Language, path)
	if err != nil {
		return nil, err
	}

	// Calculate hash
	checksum, err := hashFile(outputPath)
	if err != nil {
		return nil, err
	}

	return &BuildResult{
		Name:   functionConfig.FunctionSettings.Name,
		Path:   outputPath,
		Digest: checksum,
	}, nil
}

// InitFunction implements FunctionService.
func (f *functionService) InitFunction(name string, language string) error {
	path := fmt.Sprintf("./%s", name)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return errors.New("directory already exists")
	}

	url, err := languageToSDK(language)
	if err != nil {
		return err
	}

	_, err = git.PlainClone(path, false, &git.CloneOptions{
		URL: url,
	})
	if err != nil {
		return fmt.Errorf("error cloning template: %w", err)
	}

	if err := os.RemoveAll(filepath.Join(path, ".git")); err != nil {
		return err
	}

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

	marshalledManifest, err := functionManifest.MarhsalYaml()
	if err != nil {
		return err
	}

	err = os.WriteFile(filepath.Join(path, "ignition.yml"), marshalledManifest, 0644)
	if err != nil {
		return err
	}

	return nil
}

func NewFunctionService() FunctionService {
	return &functionService{}

}

func (f *functionService) build(language string, dir string) (string, error) {
	var outputPath string

	switch language {
	case "rust":
		builder := builders.NewRustBuilder()
		result, err := builder.Build(dir)
		if err != nil {
			return "", err
		}

		outputPath = result.OutputPath
	case "typescript":
		builder := builders.NewJSBuilder()
		result, err := builder.Build(dir)
		if err != nil {
			return "", err
		}

		outputPath = result.OutputPath

	case "javascript":
		builder := builders.NewJSBuilder()

		if err := builder.VerifyDependencies(); err != nil {
			return "", err
		}

		result, err := builder.Build(dir)
		if err != nil {
			return "", err
		}

		outputPath = result.OutputPath
	case "golang":
		builder := builders.NewGoBuilder()

		if err := builder.VerifyDependencies(); err != nil {
			return "", err
		}

		result, err := builder.Build(dir)
		if err != nil {
			return "", err
		}

		outputPath = result.OutputPath
	default:
		return "", errors.New("language not supported")
	}

	return outputPath, nil
}

func languageToSDK(language string) (string, error) {
	switch language {
	case "golang":
		return "https://github.com/extism/go-pdk-template", nil
	case "javascript":
		return "https://github.com/extism/js-pdk-template", nil
	case "typescript":
		return "https://github.com/extism/ts-pdk-template", nil
	case "rust":
		return "https://github.com/extism/rust-pdk-template", nil
	}
	return "", fmt.Errorf("language not supported: %s", language)
}

func hashFile(outputPath string) (string, error) {
	hasher := sha256.New()
	file, err := os.Open(outputPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Use io.Copy to read the file and write to the hasher
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	// Get the hash sum and encode it to a hex string
	cksum := hex.EncodeToString(hasher.Sum(nil))
	return cksum, nil
}

func (f *functionService) CalculateHash(path string, config manifest.FunctionManifest) (*BuildResult, error) {
	// Calculate hash of source code and config
	h := sha256.New()

	// Hash the source code
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Skip certain files/directories
		if shouldSkipFile(filePath) {
			return nil
		}

		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		h.Write(fileContent)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to hash source code: %w", err)
	}

	// Hash the config
	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	h.Write(configBytes)

	digest := fmt.Sprintf("sha256:%x", h.Sum(nil))

	return &BuildResult{
		Name:   config.FunctionSettings.Name,
		Path:   path,
		Digest: digest,
	}, nil
}

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
