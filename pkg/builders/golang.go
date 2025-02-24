package builders

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type goBuilder struct{}

// VerifyDependencies implements Builder.
func (g *goBuilder) VerifyDependencies() error {
	cmd := exec.Command("tinygo", "version")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("TinyGo verification failed: %v", exitErr.Error())
		}
		// If the error is not an ExitError, it likely means the command wasn't found
		return fmt.Errorf("TinyGo is not installed or not found in PATH")
	}
	return nil
}

func (g goBuilder) Build(path string) (*BuildResult, error) {
	cmd := exec.Command("tinygo", "build", "-o", "plugin.wasm", "-target", "wasi", "main.go")
	cmd.Dir = path

	// Create a buffer to capture stderr
	var stderr bytes.Buffer
	// Use MultiWriter to write to both stderr buffer and os.Stderr
	multiWriter := io.MultiWriter(&stderr, os.Stderr)

	cmd.Stdout = os.Stdout
	cmd.Stderr = multiWriter

	if err := cmd.Run(); err != nil {
		return nil, &BuildError{
			Err:    err,
			Stderr: stderr.String(),
		}
	}

	return &BuildResult{
		OutputPath: filepath.Join(path, "plugin.wasm"),
	}, nil
}

func NewGoBuilder() Builder {
	return &goBuilder{}
}
