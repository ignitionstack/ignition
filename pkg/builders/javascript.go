package builders

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

type jsBuilder struct{}

// Build implements Builder.
func (j *jsBuilder) Build(path string) (*BuildResult, error) {
	dependencyCmd := exec.Command("npm", "install")
	dependencyCmd.Dir = path
	if err := dependencyCmd.Run(); err != nil {
		fmt.Printf("failed to execute command in js: %s\n", err)
		return nil, err
	}

	esBuildCmd := exec.Command("node", "esbuild.js")
	esBuildCmd.Dir = path
	if error := esBuildCmd.Run(); error != nil {
		fmt.Printf("failed to execute command in js: %s\n", error)
		return nil, error
	}

	cmd := exec.Command("extism-js", "dist/index.js", "-i", "src/index.d.ts", "-o", "dist/plugin.wasm")
	cmd.Dir = path
	if err := cmd.Run(); err != nil {
		fmt.Printf("failed to execute command in js: %s\n", err)
		return nil, err
	}

	return &BuildResult{
		OutputPath: filepath.Join(path, "dist", "plugin.wasm"),
	}, nil
}

func NewJSBuilder() Builder {
	return &jsBuilder{}
}
