package builders

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type pythonBuilder struct{}

func (p *pythonBuilder) VerifyDependencies() error {
	cmd := exec.Command("extism-py", "--version")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("extism-py verification failed: %v\n%s", exitErr.Error(), stderr.String())
		}
		// If the error is not an ExitError, it likely means extism-py isn't installed
		return errors.New("extism-py is not installed. Please install it by following the documentation: https://github.com/extism/python-pdk")
	}
	return nil
}

func (p *pythonBuilder) Build(path string) (*BuildResult, error) {
	outputFile := "plugin.wasm"

	// Check if plugin directory structure exists
	pluginDir := filepath.Join(path, "plugin")
	initPyPath := filepath.Join(pluginDir, "__init__.py")

	if _, err := os.Stat(initPyPath); err == nil {
		// We found the plugin/__init__.py structure
		// Build WASM using extism-py with the plugin directory
		buildCmd := exec.Command("extism-py", initPyPath, "-o", outputFile)
		buildCmd.Dir = path

		if err := runCommandWithOutput(buildCmd, "Python compilation"); err != nil {
			return nil, err
		}
	} else {
		// Fall back to looking for any Python file in the root directory
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read directory: %w", err)
		}

		var sourceFile string
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".py" {
				// Prioritize main Python files if they exist
				if entry.Name() == "plugin.py" || entry.Name() == "main.py" {
					sourceFile = entry.Name()
					break
				}
				// Otherwise use the first .py file found
				if sourceFile == "" {
					sourceFile = entry.Name()
				}
			}
		}

		if sourceFile == "" {
			return nil, errors.New("no Python (.py) file found in the directory or plugin/__init__.py structure")
		}

		// Build WASM using extism-py with the discovered file
		buildCmd := exec.Command("extism-py", sourceFile, "-o", outputFile)
		buildCmd.Dir = path

		if err := runCommandWithOutput(buildCmd, "Python compilation"); err != nil {
			return nil, err
		}
	}

	return &BuildResult{
		OutputPath: filepath.Join(path, outputFile),
	}, nil
}

func NewPythonBuilder() Builder {
	return &pythonBuilder{}
}
