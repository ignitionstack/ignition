package builders

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type assemblyscriptBuilder struct{}

func (a *assemblyscriptBuilder) VerifyDependencies() error {
	cmd := exec.Command("npx", "--version")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("npx verification failed: %v\n%s", exitErr.Error(), stderr.String())
		}
		return fmt.Errorf("npx is not installed. Please install Node.js and npm from https://nodejs.org")
	}

	// Check for assemblyscript compiler
	ascCmd := exec.Command("npx", "asc", "--version")
	var ascStderr bytes.Buffer
	ascCmd.Stderr = &ascStderr

	if err := ascCmd.Run(); err != nil {
		return fmt.Errorf("assemblyscript compiler (asc) not found. Please install it using 'npm install assemblyscript'")
	}

	return nil
}

func (a *assemblyscriptBuilder) Build(path string) (*BuildResult, error) {
	// Install dependencies
	dependencyCmd := exec.Command("npm", "install")
	dependencyCmd.Dir = path
	if err := runCommandWithOutput(dependencyCmd, "dependency installation"); err != nil {
		return nil, err
	}

	// Determine source file (assuming it's in the root directory with .ts extension)
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %v", err)
	}

	var sourceFile string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".ts" {
			sourceFile = entry.Name()
			break
		}
	}

	if sourceFile == "" {
		return nil, fmt.Errorf("no TypeScript (.ts) file found in the directory")
	}

	outputFile := "plugin.wasm"

	// Build WASM using assemblyscript compiler
	ascCmd := exec.Command("npx", "asc", sourceFile, "--outFile", outputFile, "--use", "abort=")
	ascCmd.Dir = path
	if err := runCommandWithOutput(ascCmd, "AssemblyScript compilation"); err != nil {
		return nil, err
	}

	return &BuildResult{
		OutputPath: filepath.Join(path, outputFile),
	}, nil
}

func NewAssemblyScriptBuilder() Builder {
	return &assemblyscriptBuilder{}
}
