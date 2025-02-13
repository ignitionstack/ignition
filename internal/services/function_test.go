package services

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestBuildFunction(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "build-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name           string
		language       string
		setupFiles     func(string) error
		shouldError    bool
		expectedDigest string
	}{
		{
			name:     "rust-build",
			language: "rust",
			setupFiles: func(dir string) error {
				// Create minimal Rust project structure
				if err := os.MkdirAll(filepath.Join(dir, "src"), 0755); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(dir, "src", "lib.rs"), []byte(`
					fn main() {
						println!("Hello, World!");
					}
				`), 0644)
			},
			shouldError: true, // Will error without full Rust project setup
		},
		{
			name:     "typescript-build",
			language: "typescript",
			setupFiles: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, "index.ts"), []byte(`
					console.log("Hello, World!");
				`), 0644)
			},
			shouldError: true, // Will error without full TypeScript project setup
		},
		{
			name:     "invalid-language",
			language: "invalid",
			setupFiles: func(dir string) error {
				return nil
			},
			shouldError: true,
		},
	}

	service := NewFunctionService()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test directory
			testDir := filepath.Join(tmpDir, tt.name)
			err := os.MkdirAll(testDir, 0755)
			require.NoError(t, err)

			// Setup test files
			err = tt.setupFiles(testDir)
			require.NoError(t, err)

			config := manifest.FunctionManifest{
				FunctionSettings: manifest.FunctionSettings{
					Name:     tt.name,
					Language: tt.language,
					VersionSettings: manifest.FunctionVersionSettings{
						AllowedUrls: []string{},
					},
				},
			}

			result, err := service.BuildFunction(testDir, config)
			if tt.shouldError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.name, result.Name)
			if tt.expectedDigest != "" {
				assert.Equal(t, tt.expectedDigest, result.Digest)
			}
		})
	}
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
