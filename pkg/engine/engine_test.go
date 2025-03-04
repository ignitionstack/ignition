package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestEngine(t *testing.T) (*Engine, string) {
	tmpDir, err := os.MkdirTemp("", "ignition-test-*")
	require.NoError(t, err)

	socketPath := filepath.Join(tmpDir, "ignition.sock")
	httpAddr := "localhost:0"
	registryDir := filepath.Join(tmpDir, ".ignition")

	err = os.MkdirAll(registryDir, 0755)
	require.NoError(t, err)

	logger := logging.NewStdLogger(os.Stdout)
	engine, err := NewEngineWithOptions(socketPath, httpAddr, registryDir, logger, nil)
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
	assert.NotNil(t, engine.pluginManager)
	assert.NotNil(t, engine.circuitBreakers)
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

	// Create context for loading
	ctx := t.Context()

	// Test loading non-existent function
	err := engine.LoadFunctionWithContext(ctx, "test-namespace", "non-existent", "latest", nil)
	assert.Error(t, err)

	// Verify no plugin was loaded
	loadedCount := engine.pluginManager.GetLoadedFunctionCount()
	assert.Equal(t, 0, loadedCount)
}

func TestCallFunction(t *testing.T) {
	engine, tmpDir := setupTestEngine(t)
	defer cleanupTest(tmpDir)

	// Create context for function call
	ctx := t.Context()

	// Test calling non-existent function
	output, err := engine.CallFunctionWithContext(ctx, "test-namespace", "non-existent", "test", []byte("test-payload"))
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
		// Create context for operations
		ctx := t.Context()

		// Load function
		err = engine.LoadFunctionWithContext(ctx, "test-namespace", "test-function", "latest", nil)
		require.Error(t, err) // Expected error since build failed

		// Call function
		output, err := engine.CallFunctionWithContext(ctx, "test-namespace", "test-function", "test", []byte("test-payload"))
		assert.Error(t, err)
		assert.Nil(t, output)

		// Reassign tag
		err = engine.ReassignTag("test-namespace", "test-function", "latest", "new-digest")
		assert.Error(t, err)
	}
}
