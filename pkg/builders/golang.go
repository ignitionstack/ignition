package builders

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

type goBuilder struct{}

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
