// Package client provides the public interface for interacting with the Ignition engine.
package client

import (
	"context"
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/models"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

// EngineClient provides an interface for interacting with the Ignition engine
type EngineClient interface {
	// Status checks if the engine is running
	Status(ctx context.Context) error
	
	// LoadFunction loads a function into the engine
	LoadFunction(ctx context.Context, namespace, name, tag string, config map[string]string) error
	
	// UnloadFunction unloads a function from the engine
	UnloadFunction(ctx context.Context, namespace, name string) error
	
	// StopFunction stops a function in the engine
	StopFunction(ctx context.Context, namespace, name string) error
	
	// CallFunction calls a function
	CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte, config map[string]string) ([]byte, error)
	
	// OneOffCall loads a function temporarily and calls it once
	OneOffCall(ctx context.Context, namespace, name, reference, entrypoint string, payload []byte, config map[string]string) ([]byte, error)
	
	// BuildFunction builds a function
	BuildFunction(ctx context.Context, namespace, name, path, tag string, manifest manifest.FunctionManifest) (*types.BuildResult, error)
	
	// ListFunctions lists all loaded functions
	ListFunctions(ctx context.Context) ([]types.LoadedFunction, error)
	
	// GetFunctionLogs gets logs for a function
	GetFunctionLogs(ctx context.Context, namespace, name string, since time.Duration, tail int) ([]string, error)
	
	// UnloadFunctions unloads multiple functions at once
	UnloadFunctions(ctx context.Context, functions []models.FunctionReference) error
	
	// StopFunctions stops multiple functions at once
	StopFunctions(ctx context.Context, functions []models.FunctionReference) error
}
