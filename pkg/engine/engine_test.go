package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestEngine(t *testing.T) (*Engine, string) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "ignition-test-*")
	require.NoError(t, err)

	// Set up a temporary socket path and http address
	socketPath := filepath.Join(tmpDir, "ignition.sock")
	httpAddr := "localhost:0" // Use port 0 to get a random available port
	registryDir := filepath.Join(tmpDir, ".ignition")

	// Create registry directory
	err = os.MkdirAll(registryDir, 0755)
	require.NoError(t, err)

	// Create new engine instance with the temp directory
	engine, err := NewEngine(socketPath, httpAddr, registryDir)
	require.NoError(t, err)

	return engine, tmpDir
}

func cleanupTest(tmpDir string) {
	os.RemoveAll(tmpDir)
}

func TestNewEngine(t *testing.T) {
	engine, tmpDir := setupTestEngine(t)
	defer cleanupTest(tmpDir)

	assert.NotNil(t, engine)
	assert.NotNil(t, engine.registry)
	assert.NotNil(t, engine.plugins)
	assert.NotNil(t, engine.logger)
	assert.Equal(t, filepath.Join(tmpDir, "ignition.sock"), engine.socketPath)
	assert.Equal(t, "localhost:0", engine.httpAddr)
}

func TestBuildFunction(t *testing.T) {
	engine, tmpDir := setupTestEngine(t)
	defer cleanupTest(tmpDir)

	// Create a test function directory
	functionDir := filepath.Join(tmpDir, "test-function")
	err := os.MkdirAll(functionDir, 0755)
	require.NoError(t, err)

	// Create a simple function manifest
	config := manifest.FunctionManifest{
		FunctionSettings: manifest.FunctionSettings{
			Name:     "test-function",
			Language: "rust",
			VersionSettings: manifest.FunctionVersionSettings{
				AllowedUrls: []string{},
			},
		},
	}

	// Test building function
	result, err := engine.BuildFunction("test-namespace", "test-function", functionDir, "latest", config)

	// Since we don't have actual build implementation in test, we expect an error
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestLoadFunction(t *testing.T) {
	engine, tmpDir := setupTestEngine(t)
	defer cleanupTest(tmpDir)

	// Test loading non-existent function
	err := engine.LoadFunction("test-namespace", "non-existent", "latest")
	assert.Error(t, err)

	// Verify no plugin was loaded
	engine.pluginsMux.RLock()
	assert.Equal(t, 0, len(engine.plugins))
	engine.pluginsMux.RUnlock()
}

func TestCallFunction(t *testing.T) {
	engine, tmpDir := setupTestEngine(t)
	defer cleanupTest(tmpDir)

	// Test calling non-existent function
	output, err := engine.CallFunction("test-namespace", "non-existent", "test", []byte("test-payload"))
	assert.Error(t, err)
	assert.Equal(t, ErrFunctionNotLoaded, err)
	assert.Nil(t, output)
}

func TestReassignTag(t *testing.T) {
	engine, tmpDir := setupTestEngine(t)
	defer cleanupTest(tmpDir)

	// Test reassigning tag for non-existent function
	err := engine.ReassignTag("test-namespace", "non-existent", "latest", "new-digest")
	assert.Error(t, err)
}

func TestIntegration(t *testing.T) {
	engine, tmpDir := setupTestEngine(t)
	defer cleanupTest(tmpDir)

	// Create test function
	functionDir := filepath.Join(tmpDir, "test-function")
	err := os.MkdirAll(functionDir, 0755)
	require.NoError(t, err)

	config := manifest.FunctionManifest{
		FunctionSettings: manifest.FunctionSettings{
			Name:     "test-function",
			Language: "rust",
			VersionSettings: manifest.FunctionVersionSettings{
				AllowedUrls: []string{},
			},
		},
	}

	// Build function
	buildResult, err := engine.BuildFunction("test-namespace", "test-function", functionDir, "latest", config)
	require.Error(t, err) // Expected error since we don't have actual build implementation

	if buildResult != nil {
		// Load function
		err = engine.LoadFunction("test-namespace", "test-function", "latest")
		require.Error(t, err) // Expected error since build failed

		// Call function
		output, err := engine.CallFunction("test-namespace", "test-function", "test", []byte("test-payload"))
		assert.Error(t, err)
		assert.Nil(t, output)

		// Reassign tag
		err = engine.ReassignTag("test-namespace", "test-function", "latest", "new-digest")
		assert.Error(t, err)
	}
}
