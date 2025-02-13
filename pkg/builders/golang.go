package builders

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type goBuilder struct{}

func (g goBuilder) Build(path string) (*BuildResult, error) {
	cmd := exec.Command("tinygo", "build", "-o", "plugin.wasm", "-target", "wasi", "main.go")
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("failed to execute command in go: %s\n", err)
		return nil, err
	}

	return &BuildResult{
		OutputPath: filepath.Join(path, "plugin.wasm"),
	}, nil
}

func NewGoBuilder() Builder {
	return &goBuilder{}
}
