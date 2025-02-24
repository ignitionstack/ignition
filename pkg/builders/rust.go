package builders

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type cargo struct {
	Package struct {
		Name string `toml:"name"`
	} `toml:"package"`
}

type rustBuilder struct{}

func (r *rustBuilder) VerifyDependencies() error {
	// Check if cargo is installed
	cargoCmd := exec.Command("cargo", "--version")
	if err := cargoCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("cargo verification failed: %v", exitErr.Error())
		}
		return fmt.Errorf("cargo is not installed. Please install Rust and Cargo from https://rustup.rs")
	}

	// Check if wasm32-wasi target is installed
	targetCmd := exec.Command("rustup", "target", "list", "--installed")
	var stdout bytes.Buffer
	targetCmd.Stdout = &stdout

	if err := targetCmd.Run(); err != nil {
		return fmt.Errorf("failed to check installed targets: %v", err)
	}

	if !bytes.Contains(stdout.Bytes(), []byte("wasm32-unknown-unknown")) {
		return fmt.Errorf("wasm32-wasi target is not installed. Please install it using 'rustup target add wasm32-unknown-unknown'")
	}

	return nil
}

func (r rustBuilder) Build(path string) (*BuildResult, error) {
	// Read cargo.toml to get binary output
	cargoFile, err := os.ReadFile(filepath.Join(path, "Cargo.toml"))
	if err != nil {
		return nil, err
	}

	var cargoConfig *cargo
	err = toml.Unmarshal(cargoFile, &cargoConfig)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command("cargo", "build", "-r", "-q")
	cmd.Dir = path
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return &BuildResult{
		OutputPath: filepath.Join(path, "target", "wasm32-unknown-unknown", "release", fmt.Sprintf("%s.wasm", strings.Replace(cargoConfig.Package.Name, "-", "_", -1))),
	}, nil
}

func NewRustBuilder() Builder {
	return &rustBuilder{}
}
