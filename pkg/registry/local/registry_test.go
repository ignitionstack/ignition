// registry/local/registry_test.go
package localRegistry

import (
	"os"
	"path/filepath"
	"testing"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testSetup struct {
	registry registry.Registry
	db       *badger.DB
	tmpDir   string
	cleanup  func()
}

func setupTestRegistry(t *testing.T) *testSetup {
	tmpDir, err := os.MkdirTemp("", "registry-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "db")
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil // Disable logging for tests

	db, err := badger.Open(opts)
	require.NoError(t, err)

	registry := NewLocalRegistry(tmpDir, db)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return &testSetup{
		registry: registry,
		db:       db,
		tmpDir:   tmpDir,
		cleanup:  cleanup,
	}
}

var defaultSettings = manifest.FunctionVersionSettings{
	Wasi:        true,
	AllowedUrls: []string{},
}

func TestGet(t *testing.T) {
	setup := setupTestRegistry(t)
	defer setup.cleanup()

	t.Run("not found", func(t *testing.T) {
		metadata, err := setup.registry.Get("test", "nonexistent")
		assert.ErrorIs(t, err, registry.ErrFunctionNotFound)
		assert.Nil(t, metadata)
	})

	t.Run("found", func(t *testing.T) {
		// Push a function first
		payload := []byte("test wasm")
		digest := "full123"
		err := setup.registry.Push("test", "func1", payload, digest, "latest", defaultSettings)
		require.NoError(t, err)

		// Get the function
		metadata, err := setup.registry.Get("test", "func1")
		require.NoError(t, err)
		assert.Equal(t, "test", metadata.Namespace)
		assert.Equal(t, "func1", metadata.Name)
		assert.Len(t, metadata.Versions, 1)
		assert.Equal(t, digest, metadata.Versions[0].FullDigest)
		assert.Equal(t, []string{"latest"}, metadata.Versions[0].Tags)
	})
}

func TestPush(t *testing.T) {
	setup := setupTestRegistry(t)
	defer setup.cleanup()

	firstPayload := []byte("test wasm v1")
	firstDigest := "digest123456789"
	settings1 := manifest.FunctionVersionSettings{
		Wasi:        true,
		AllowedUrls: []string{"https://api.example.com"},
	}

	t.Run("new function", func(t *testing.T) {
		err := setup.registry.Push("test", "func1", firstPayload, firstDigest, "latest", settings1)
		require.NoError(t, err)

		metadata, err := setup.registry.Get("test", "func1")
		require.NoError(t, err)
		assert.Equal(t, "test", metadata.Namespace)
		assert.Equal(t, "func1", metadata.Name)
		assert.Len(t, metadata.Versions, 1)
		assert.Equal(t, firstDigest, metadata.Versions[0].FullDigest)
		assert.Equal(t, []string{"latest"}, metadata.Versions[0].Tags)
		assert.Equal(t, int64(len(firstPayload)), metadata.Versions[0].Size)
		assert.Equal(t, settings1, metadata.Versions[0].Settings)
	})

	secondPayload := []byte("test wasm v2")
	secondDigest := "newdigest123"
	settings2 := manifest.FunctionVersionSettings{
		Wasi:        false,
		AllowedUrls: []string{"https://api2.example.com"},
	}

	t.Run("existing function new version", func(t *testing.T) {
		err := setup.registry.Push("test", "func1", secondPayload, secondDigest, "v2", settings2)
		require.NoError(t, err)

		metadata, err := setup.registry.Get("test", "func1")
		require.NoError(t, err)
		assert.Len(t, metadata.Versions, 2)

		// Find v2 version
		var v2Version *registry.VersionInfo
		for _, v := range metadata.Versions {
			if registry.HasTag(v.Tags, "v2") {
				v2Version = &v
				break
			}
		}
		require.NotNil(t, v2Version)
		assert.Equal(t, secondDigest, v2Version.FullDigest)
		assert.Equal(t, int64(len(secondPayload)), v2Version.Size)
		assert.Equal(t, settings2, v2Version.Settings)
	})
}

func TestPull(t *testing.T) {
	setup := setupTestRegistry(t)
	defer setup.cleanup()

	// Push a function with multiple versions
	payload1 := []byte("test wasm v1")
	digest1 := "digest1"
	err := setup.registry.Push("test", "func1", payload1, digest1, "v1", defaultSettings)
	require.NoError(t, err)

	payload2 := []byte("test wasm v2")
	digest2 := "digest2"
	err = setup.registry.Push("test", "func1", payload2, digest2, "latest", defaultSettings)
	require.NoError(t, err)

	t.Run("pull by digest", func(t *testing.T) {
		wasmBytes, versionInfo, err := setup.registry.Pull("test", "func1", "digest1")
		require.NoError(t, err)
		assert.Equal(t, payload1, wasmBytes)
		require.NotNil(t, versionInfo)
		assert.Equal(t, digest1, versionInfo.FullDigest)
	})

	t.Run("pull by tag", func(t *testing.T) {
		wasmBytes, versionInfo, err := setup.registry.Pull("test", "func1", "latest")
		require.NoError(t, err)
		assert.Equal(t, payload2, wasmBytes)
		require.NotNil(t, versionInfo)
		assert.Equal(t, digest2, versionInfo.FullDigest)
	})

	t.Run("pull nonexistent reference", func(t *testing.T) {
		_, _, err := setup.registry.Pull("test", "func1", "nonexistent")
		assert.ErrorIs(t, err, registry.ErrTagNotFound)
	})

	t.Run("pull nonexistent function", func(t *testing.T) {
		_, _, err := setup.registry.Pull("test", "nonexistent", "latest")
		assert.ErrorIs(t, err, registry.ErrFunctionNotFound)
	})
}

func TestReassignTag(t *testing.T) {
	setup := setupTestRegistry(t)
	defer setup.cleanup()

	// Push a function with multiple versions
	payload1 := []byte("test wasm v1")
	digest1 := "digest1"
	err := setup.registry.Push("test", "func1", payload1, digest1, "v1", defaultSettings)
	require.NoError(t, err)

	payload2 := []byte("test wasm v2")
	digest2 := "digest2"
	err = setup.registry.Push("test", "func1", payload2, digest2, "v2", defaultSettings)
	require.NoError(t, err)

	t.Run("reassign existing tag", func(t *testing.T) {
		err := setup.registry.ReassignTag("test", "func1", "v1", digest2)
		require.NoError(t, err)

		metadata, err := setup.registry.Get("test", "func1")
		require.NoError(t, err)

		// Verify tag was moved
		v1Count := 0
		for _, v := range metadata.Versions {
			if registry.HasTag(v.Tags, "v1") {
				v1Count++
				assert.Equal(t, digest2, v.FullDigest)
			}
		}
		assert.Equal(t, 1, v1Count, "should have exactly one version with v1 tag")
	})
}

func TestListAll(t *testing.T) {
	setup := setupTestRegistry(t)
	defer setup.cleanup()

	// Push multiple functions
	payload1 := []byte("test wasm 1")
	err := setup.registry.Push("test1", "func1", payload1, "digest1", "latest", defaultSettings)
	require.NoError(t, err)

	payload2 := []byte("test wasm 2")
	err = setup.registry.Push("test2", "func2", payload2, "digest2", "latest", defaultSettings)
	require.NoError(t, err)

	t.Run("list all functions", func(t *testing.T) {
		functions, err := setup.registry.ListAll()
		require.NoError(t, err)
		assert.Len(t, functions, 2)

		// Verify functions are present
		foundTest1 := false
		foundTest2 := false
		for _, f := range functions {
			switch f.Namespace {
			case "test1":
				assert.Equal(t, "func1", f.Name)
				foundTest1 = true
			case "test2":
				assert.Equal(t, "func2", f.Name)
				foundTest2 = true
			}
		}
		assert.True(t, foundTest1)
		assert.True(t, foundTest2)
	})
}

func TestWithMockStorage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-mock-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "db")
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil

	db, err := badger.Open(opts)
	require.NoError(t, err)
	defer db.Close()

	mockStorage := newMockStorage()
	registry := &localRegistry{
		db:      db,
		storage: mockStorage,
	}

	// Push with version settings
	payload := []byte("test wasm")
	digest := "digest123"

	err = registry.Push("test", "func1", payload, digest, "", defaultSettings)
	require.NoError(t, err)

	// Pull and verify
	wasmBytes, versionInfo, err := registry.Pull("test", "func1", digest)
	require.NoError(t, err)
	assert.Equal(t, payload, wasmBytes)
	assert.Equal(t, digest, versionInfo.FullDigest)
	assert.Equal(t, defaultSettings, versionInfo.Settings)
}

// Mock implementations for testing
type mockStorage struct {
	files map[string][]byte
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		files: make(map[string][]byte),
	}
}

func (m *mockStorage) ReadWASMFile(path string) ([]byte, error) {
	if data, ok := m.files[path]; ok {
		return data, nil
	}
	return nil, os.ErrNotExist
}

func (m *mockStorage) WriteWASMFile(path string, data []byte) error {
	m.files[path] = data
	return nil
}

func (m *mockStorage) BuildWASMPath(namespace, name, shortDigest string) string {
	return filepath.Join(namespace, name, shortDigest+".wasm")
}
