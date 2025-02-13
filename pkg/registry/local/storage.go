package localRegistry

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ignitionstack/ignition/pkg/registry"
)

type localStorage struct {
	rootDir string
}

func NewLocalStorage(rootDir string) registry.Storage {
	return &localStorage{rootDir: rootDir}
}

func (s *localStorage) ReadWASMFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("WASM file not found: %w", registry.ErrFunctionNotFound)
		}
		return nil, fmt.Errorf("failed to read WASM file: %w", err)
	}
	return data, nil
}

func (s *localStorage) WriteWASMFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write WASM file: %w", err)
	}
	return nil
}

func (s *localStorage) BuildWASMPath(namespace, name, shortDigest string) string {
	return filepath.Join(s.rootDir, "storage", namespace, name, "versions", shortDigest+".wasm")
}
