package builders

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type jsBuilder struct{}

func (j *jsBuilder) VerifyDependencies() error {
	cmd := exec.Command("extism-js", "--version")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("extism-js verification failed: %v\n%s", exitErr.Error(), stderr.String())
		}
		// If the error is not an ExitError, it likely means npx or extism-js isn't installed
		return fmt.Errorf("extism-js is not installed. Please see the installation instructions here: https://github.com/extism/js-pdk?tab=readme-ov-file#install-script")
	}
	return nil
}

func runCommandWithOutput(cmd *exec.Cmd, step string) error {
	var stderr bytes.Buffer
	multiWriter := io.MultiWriter(&stderr, os.Stderr)

	cmd.Stdout = os.Stdout
	cmd.Stderr = multiWriter

	if err := cmd.Run(); err != nil {
		return &BuildError{
			Err:    err,
			Stderr: stderr.String(),
			Step:   step,
		}
	}
	return nil
}

func (j *jsBuilder) Build(path string) (*BuildResult, error) {
	// Install dependencies
	dependencyCmd := exec.Command("npm", "install")
	dependencyCmd.Dir = path
	if err := runCommandWithOutput(dependencyCmd, "dependency installation"); err != nil {
		return nil, err
	}

	// Run esbuild
	esBuildCmd := exec.Command("node", "esbuild.js")
	esBuildCmd.Dir = path
	if err := runCommandWithOutput(esBuildCmd, "esbuild"); err != nil {
		return nil, err
	}

	// Build WASM
	wasmCmd := exec.Command("extism-js", "dist/index.js", "-i", "src/index.d.ts", "-o", "dist/plugin.wasm")
	wasmCmd.Dir = path
	if err := runCommandWithOutput(wasmCmd, "WASM compilation"); err != nil {
		return nil, err
	}

	return &BuildResult{
		OutputPath: filepath.Join(path, "dist", "plugin.wasm"),
	}, nil
}

func NewJSBuilder() Builder {
	return &jsBuilder{}
}
