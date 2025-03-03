package builders

/*
TODO: Fix the zig builder, it's currently not working.
*/

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type zigBuilder struct{}

func (z *zigBuilder) VerifyDependencies() error {
	cmd := exec.Command("zig", "version")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("zig verification failed: %v\n%s", exitErr.Error(), stderr.String())
		}
		return fmt.Errorf("zig is not installed. Please install Zig from https://ziglang.org/download/")
	}
	return nil
}

func (z *zigBuilder) Build(path string) (*BuildResult, error) {
	// Find main source file
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %v", err)
	}

	var sourceFile string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".zig" {
			// Prioritize main.zig if it exists
			if entry.Name() == "main.zig" {
				sourceFile = "main.zig"
				break
			}
			// Otherwise use the first .zig file found
			if sourceFile == "" {
				sourceFile = entry.Name()
			}
		}
	}

	if sourceFile == "" {
		return nil, fmt.Errorf("no Zig (.zig) file found in the directory")
	}

	outputFile := "plugin.wasm"

	// Build WASM using Zig compiler
	// Using build-exe with wasm32-wasi target and output to plugin.wasm
	buildCmd := exec.Command("zig", "build-exe", sourceFile,
		"-target", "wasm32-wasi",
		"-O", "ReleaseFast", // Optimize for performance
		"-fno-entry",     // Skip runtime startup
		"--export-table", // Export function table
		"-femit-bin="+outputFile)
	buildCmd.Dir = path

	if err := runCommandWithOutput(buildCmd, "Zig compilation"); err != nil {
		return nil, err
	}

	return &BuildResult{
		OutputPath: filepath.Join(path, outputFile),
	}, nil
}

func NewZigBuilder() Builder {
	return &zigBuilder{}
}
