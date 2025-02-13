package builders

import (
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
