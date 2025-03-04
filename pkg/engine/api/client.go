package api

import (
	"context"
	"time"

	"github.com/ignitionstack/ignition/pkg/engine/models"
)

// Client is the interface for communicating with the Ignition engine
type Client interface {
	// Status checks if the engine is running
	Status(ctx context.Context) (*StatusResponse, error)

	// LoadFunction loads a function into the engine
	LoadFunction(ctx context.Context, req LoadRequest) (*LoadResponse, error)

	// UnloadFunction unloads a function from the engine
	UnloadFunction(ctx context.Context, req UnloadRequest) error

	// StopFunction stops a function in the engine
	StopFunction(ctx context.Context, req StopRequest) error

	// CallFunction calls a function
	CallFunction(ctx context.Context, req CallRequest) ([]byte, error)

	// OneOffCall loads a function temporarily and calls it
	OneOffCall(ctx context.Context, req OneOffCallRequest) ([]byte, error)

	// BuildFunction builds a function
	BuildFunction(ctx context.Context, req BuildRequest) (*BuildResponse, error)

	// ListFunctions lists all loaded functions
	ListFunctions(ctx context.Context) ([]models.Function, error)

	// GetFunctionLogs gets logs for a function
	GetFunctionLogs(ctx context.Context, namespace, name string, since time.Duration, tail int) (LogsResponse, error)

	// UnloadFunctions unloads multiple functions at once
	UnloadFunctions(ctx context.Context, functions []models.FunctionReference) error

	// StopFunctions stops multiple functions at once
	StopFunctions(ctx context.Context, functions []models.FunctionReference) error
}
