package localregistry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ignitionstack/ignition/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLocalStorage(t *testing.T) (*localStorage, string, func()) {
	tmpDir, err := os.MkdirTemp("", "local-storage-test-*")
	require.NoError(t, err, "failed to create temp directory")

	storage := NewLocalStorage(tmpDir).(*localStorage)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return storage, tmpDir, cleanup
}

func TestReadWASMFile(t *testing.T) {
	storage, tmpDir, cleanup := setupLocalStorage(t)
	defer cleanup()

	// Create a test WASM file
	wasmPath := filepath.Join(tmpDir, "test.wasm")
	wasmContent := []byte("test wasm content")
	err := os.WriteFile(wasmPath, wasmContent, 0644)
	require.NoError(t, err, "failed to create test WASM file")

	t.Run("read existing WASM file", func(t *testing.T) {
		data, err := storage.ReadWASMFile(wasmPath)
		require.NoError(t, err, "failed to read WASM file")
		assert.Equal(t, wasmContent, data, "WASM file content mismatch")
	})

	t.Run("read nonexistent WASM file", func(t *testing.T) {
		nonexistentPath := filepath.Join(tmpDir, "nonexistent.wasm")
		_, err := storage.ReadWASMFile(nonexistentPath)
		require.Error(t, err, "expected error for nonexistent file")
		assert.ErrorIs(t, err, registry.ErrFunctionNotFound, "error should be ErrFunctionNotFound")
	})

	t.Run("read with permission error", func(t *testing.T) {
		// Create a file with no read permissions
		noPermPath := filepath.Join(tmpDir, "no_perm.wasm")
		err := os.WriteFile(noPermPath, []byte("no permission"), 0222) // Write-only permissions
		require.NoError(t, err, "failed to create no-permission file")

		_, err = storage.ReadWASMFile(noPermPath)
		require.Error(t, err, "expected error for permission issue")
		assert.Contains(t, err.Error(), "failed to read WASM file", "error message mismatch")
	})
}

func TestWriteWASMFile(t *testing.T) {
	storage, tmpDir, cleanup := setupLocalStorage(t)
	defer cleanup()

	wasmPath := filepath.Join(tmpDir, "test.wasm")
	wasmContent := []byte("test wasm content")

	t.Run("write new WASM file", func(t *testing.T) {
		err := storage.WriteWASMFile(wasmPath, wasmContent)
		require.NoError(t, err, "failed to write WASM file")

		// Verify the file was written
		data, err := os.ReadFile(wasmPath)
		require.NoError(t, err, "failed to read written WASM file")
		assert.Equal(t, wasmContent, data, "WASM file content mismatch")
	})

	t.Run("write with directory creation", func(t *testing.T) {
		nestedPath := filepath.Join(tmpDir, "nested", "dir", "test.wasm")
		err := storage.WriteWASMFile(nestedPath, wasmContent)
		require.NoError(t, err, "failed to write WASM file with nested directories")

		// Verify the file was written
		data, err := os.ReadFile(nestedPath)
		require.NoError(t, err, "failed to read written WASM file")
		assert.Equal(t, wasmContent, data, "WASM file content mismatch")
	})

	t.Run("write with permission error", func(t *testing.T) {
		// Create a directory with no write permissions
		noPermDir := filepath.Join(tmpDir, "no_perm_dir")
		err := os.Mkdir(noPermDir, 0444) // Read-only permissions
		require.NoError(t, err, "failed to create no-permission directory")

		noPermPath := filepath.Join(noPermDir, "test.wasm")
		err = storage.WriteWASMFile(noPermPath, wasmContent)
		require.Error(t, err, "expected error for permission issue")
		assert.Contains(t, err.Error(), "failed to write WASM file", "error message mismatch")
	})
}

func TestBuildWASMPath(t *testing.T) {
	storage, tmpDir, cleanup := setupLocalStorage(t)
	defer cleanup()

	t.Run("build WASM path", func(t *testing.T) {
		namespace := "test-namespace"
		name := "test-function"
		shortDigest := "abc123"

		expectedPath := filepath.Join(tmpDir, "storage", namespace, name, "versions", shortDigest+".wasm")
		actualPath := storage.BuildWASMPath(namespace, name, shortDigest)

		assert.Equal(t, expectedPath, actualPath, "WASM path mismatch")
	})
}
