// registry/local/registry_test.go
package localRegistry

import (
	"os"
	"path/filepath"
	"testing"

	badger "github.com/dgraph-io/badger/v4"
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
		err := setup.registry.Push("test", "func1", payload, digest, "latest")
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

	t.Run("new function", func(t *testing.T) {
		err := setup.registry.Push("test", "func1", firstPayload, firstDigest, "latest")
		require.NoError(t, err)

		metadata, err := setup.registry.Get("test", "func1")
		require.NoError(t, err)
		assert.Equal(t, "test", metadata.Namespace)
		assert.Equal(t, "func1", metadata.Name)
		assert.Len(t, metadata.Versions, 1)
		assert.Equal(t, firstDigest, metadata.Versions[0].FullDigest)
		assert.Equal(t, []string{"latest"}, metadata.Versions[0].Tags)
		assert.Equal(t, int64(len(firstPayload)), metadata.Versions[0].Size)
	})

	secondPayload := []byte("test wasm v2")
	secondDigest := "newdigest123"

	t.Run("existing function new version", func(t *testing.T) {
		err := setup.registry.Push("test", "func1", secondPayload, secondDigest, "v2")
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
	})

	thirdPayload := []byte("test wasm v3")
	thirdDigest := "newdigest456"

	t.Run("reassign existing tag", func(t *testing.T) {
		err := setup.registry.Push("test", "func1", thirdPayload, thirdDigest, "latest")
		require.NoError(t, err)

		metadata, err := setup.registry.Get("test", "func1")
		require.NoError(t, err)

		// Verify only new version has latest tag
		latestCount := 0
		var latestVersion *registry.VersionInfo
		for _, v := range metadata.Versions {
			if registry.HasTag(v.Tags, "latest") {
				latestCount++
				latestVersion = &v
			}
		}
		assert.Equal(t, 1, latestCount)
		require.NotNil(t, latestVersion)
		assert.Equal(t, thirdDigest, latestVersion.FullDigest)
	})
}

func TestPull(t *testing.T) {
	setup := setupTestRegistry(t)
	defer setup.cleanup()

	// Push a function with multiple versions
	payload1 := []byte("test wasm v1")
	digest1 := "digest1"
	err := setup.registry.Push("test", "func1", payload1, digest1, "v1")
	require.NoError(t, err)

	payload2 := []byte("test wasm v2")
	digest2 := "digest2"
	err = setup.registry.Push("test", "func1", payload2, digest2, "latest")
	require.NoError(t, err)

	t.Run("pull by digest", func(t *testing.T) {
		wasmBytes, fullDigest, err := setup.registry.Pull("test", "func1", "digest1")
		require.NoError(t, err)
		assert.Equal(t, payload1, wasmBytes)
		assert.Equal(t, digest1, fullDigest)
	})

	t.Run("pull by tag", func(t *testing.T) {
		wasmBytes, fullDigest, err := setup.registry.Pull("test", "func1", "latest")
		require.NoError(t, err)
		assert.Equal(t, payload2, wasmBytes)
		assert.Equal(t, digest2, fullDigest)
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
	err := setup.registry.Push("test", "func1", payload1, digest1, "v1")
	require.NoError(t, err)

	payload2 := []byte("test wasm v2")
	digest2 := "digest2"
	err = setup.registry.Push("test", "func1", payload2, digest2, "v2")
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
			}
		}
		assert.Equal(t, 1, v1Count, "should have exactly one version with v1 tag")
	})

	t.Run("reassign to nonexistent digest", func(t *testing.T) {
		err := setup.registry.ReassignTag("test", "func1", "v1", "nonexistent")
		assert.ErrorIs(t, err, registry.ErrDigestNotFound)
	})

	t.Run("reassign tag for nonexistent function", func(t *testing.T) {
		err := setup.registry.ReassignTag("test", "nonexistent", "v1", digest1)
		assert.ErrorIs(t, err, registry.ErrDigestNotFound)
	})
}

func TestDigestExists(t *testing.T) {
	setup := setupTestRegistry(t)
	defer setup.cleanup()

	payload := []byte("test wasm")
	digest := "digest123"
	err := setup.registry.Push("test", "func1", payload, digest, "latest")
	require.NoError(t, err)

	t.Run("existing digest", func(t *testing.T) {
		exists, err := setup.registry.DigestExists("test", "func1", digest)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("nonexistent digest", func(t *testing.T) {
		exists, err := setup.registry.DigestExists("test", "func1", "nonexistent")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("nonexistent function", func(t *testing.T) {
		exists, err := setup.registry.DigestExists("test", "nonexistent", digest)
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestListAll(t *testing.T) {
	setup := setupTestRegistry(t)
	defer setup.cleanup()

	// Push multiple functions
	payload1 := []byte("test wasm 1")
	err := setup.registry.Push("test1", "func1", payload1, "digest1", "latest")
	require.NoError(t, err)

	payload2 := []byte("test wasm 2")
	err = setup.registry.Push("test2", "func2", payload2, "digest2", "latest")
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

	t.Run("list empty registry", func(t *testing.T) {
		// Create new empty registry
		emptySetup := setupTestRegistry(t)
		defer emptySetup.cleanup()

		functions, err := emptySetup.registry.ListAll()
		require.NoError(t, err)
		assert.Len(t, functions, 0)
	})
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

	// First lets push by digest
	payload := []byte("test wasm")
	digest := "digest123"

	err = registry.Push("test", "func1", payload, digest, "")
	require.NoError(t, err)

	// Then pull by digest to verify
	wasmBytes, fullDigest, err := registry.Pull("test", "func1", digest)
	require.NoError(t, err)
	assert.Equal(t, payload, wasmBytes)
	assert.Equal(t, digest, fullDigest)
}
