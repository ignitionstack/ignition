package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ignitionstack/ignition/pkg/builders"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBuilderFactory is a test implementation of BuilderFactory.
type mockBuilderFactory struct {
	mockBuilder builders.Builder
}

func (f *mockBuilderFactory) GetBuilder(_ string) (builders.Builder, error) {
	return f.mockBuilder, nil
}

// mockBuilder is a test implementation of builders.Builder.
type mockBuilder struct {
	buildFunc              func(path string) (*builders.BuildResult, error)
	verifyDependenciesFunc func() error
}

func (b *mockBuilder) Build(path string) (*builders.BuildResult, error) {
	return b.buildFunc(path)
}

func (b *mockBuilder) VerifyDependencies() error {
	return b.verifyDependenciesFunc()
}

func TestInitFunction(t *testing.T) {
	// Create a temporary directory for the test
	tmpDir, err := os.MkdirTemp("", "function-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Change to the temporary directory
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	err = os.Chdir(tmpDir)
	require.NoError(t, err)
	defer func() {
		if err := os.Chdir(originalDir); err != nil {
			t.Errorf("Failed to change back to original directory: %v", err)
		}
	}()

	tests := []struct {
		name        string
		language    string
		shouldError bool
	}{
		{
			name:        "rust-function",
			language:    "rust",
			shouldError: false,
		},
		{
			name:        "typescript-function",
			language:    "typescript",
			shouldError: false,
		},
		{
			name:        "javascript-function",
			language:    "javascript",
			shouldError: false,
		},
		{
			name:        "golang-function",
			language:    "golang",
			shouldError: false,
		},
		{
			name:        "invalid-function",
			language:    "invalid",
			shouldError: true,
		},
	}

	service := NewFunctionService()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.InitFunction(tt.name, tt.language)
			if tt.shouldError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Check if directory was created
			dirPath := filepath.Join(tmpDir, tt.name)
			_, err = os.Stat(dirPath)
			assert.NoError(t, err)

			// Check if manifest file was created
			manifestPath := filepath.Join(dirPath, "ignition.yml")
			_, err = os.Stat(manifestPath)
			assert.NoError(t, err)

			// Verify manifest content
			manifestContent, err := os.ReadFile(manifestPath)
			assert.NoError(t, err)
			assert.Contains(t, string(manifestContent), tt.name)
			assert.Contains(t, string(manifestContent), tt.language)

			// Check that .git directory was removed
			gitPath := filepath.Join(dirPath, ".git")
			_, err = os.Stat(gitPath)
			assert.True(t, os.IsNotExist(err))
		})
	}
}

func TestBuildFunction_WithMock(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "function-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test WASM file
	wasmPath := filepath.Join(tempDir, "test.wasm")
	err = os.WriteFile(wasmPath, []byte("mock wasm content"), 0644)
	require.NoError(t, err)

	// Create a mock builder that returns our test WASM file
	mockBuilder := &mockBuilder{
		buildFunc: func(_ string) (*builders.BuildResult, error) {
			return &builders.BuildResult{
				OutputPath: wasmPath,
			}, nil
		},
		verifyDependenciesFunc: func() error {
			return nil
		},
	}

	// Create a function service with our mock builder factory
	service := &functionService{
		builderFactory: &mockBuilderFactory{
			mockBuilder: mockBuilder,
		},
	}

	// Create a test function config
	config := manifest.FunctionManifest{
		FunctionSettings: manifest.FunctionSettings{
			Name:     "test-function",
			Language: "test-language",
		},
	}

	// Test building a function
	result, err := service.BuildFunction(tempDir, config)
	require.NoError(t, err)
	assert.Equal(t, "test-function", result.Name)
	assert.Equal(t, wasmPath, result.Path)
	assert.NotEmpty(t, result.Digest)
}

func TestCalculateHash(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "hash-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create test files
	testFiles := map[string]string{
		"main.go":        "package main\n\nfunc main() {}\n",
		"lib/helper.go":  "package lib\n\nfunc Helper() {}\n",
		"test/test.go":   "package test\n\nfunc Test() {}\n",
		"build/temp.go":  "package build\n\nfunc Temp() {}\n", // Should be skipped
		".git/config":    "[core]\n\tbare = false\n",          // Should be skipped
		"go.mod":         "module test\n\ngo 1.16\n",
		"node_modules/x": "dummy", // Should be skipped
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(content), 0644)
		require.NoError(t, err)
	}

	service := NewFunctionService()

	config := manifest.FunctionManifest{
		FunctionSettings: manifest.FunctionSettings{
			Name:     "test-function",
			Language: "golang",
			VersionSettings: manifest.FunctionVersionSettings{
				AllowedUrls: []string{"https://example.com"},
			},
		},
	}

	result1, err := service.CalculateHash(tmpDir, config)
	require.NoError(t, err)
	assert.NotEmpty(t, result1.Digest)
	assert.Equal(t, "test-function", result1.Name)
	assert.Equal(t, tmpDir, result1.Path)

	// Calculate hash again to verify consistency
	result2, err := service.CalculateHash(tmpDir, config)
	require.NoError(t, err)
	assert.Equal(t, result1.Digest, result2.Digest)

	// Modify a file and verify hash changes
	err = os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() { println(\"changed\") }\n"), 0644)
	require.NoError(t, err)

	result3, err := service.CalculateHash(tmpDir, config)
	require.NoError(t, err)
	assert.NotEqual(t, result1.Digest, result3.Digest)
}

func TestShouldSkipFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/project/.git/config", true},
		{"/project/node_modules/package.json", true},
		{"/project/target/debug/binary", true},
		{"/project/build/temp.js", true},
		{"/project/dist/bundle.js", true},
		{"/project/src/main.go", false},
		{"/project/lib/helper.js", false},
		{"/project/test/test.go", false},
		{"/project/README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := shouldSkipFile(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetTemplateURL(t *testing.T) {
	tests := []struct {
		language    string
		expectedURL string
		expectError bool
	}{
		{"golang", "https://github.com/extism/go-pdk-template", false},
		{"javascript", "https://github.com/extism/js-pdk-template", false},
		{"typescript", "https://github.com/extism/ts-pdk-template", false},
		{"rust", "https://github.com/extism/rust-pdk-template", false},
		{"unknown", "", true},
	}

	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			url, err := getTemplateURL(test.language)
			if test.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.expectedURL, url)
			}
		})
	}
}
