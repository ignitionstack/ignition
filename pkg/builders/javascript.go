package builders

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type jsBuilder struct{}

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

// Build implements Builder.
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
