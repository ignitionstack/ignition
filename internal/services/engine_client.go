package services

import (
	"context"
	"time"

	"github.com/ignitionstack/ignition/internal/config"
	"github.com/ignitionstack/ignition/pkg/engine/api"
	"github.com/ignitionstack/ignition/pkg/engine/client"
	"github.com/ignitionstack/ignition/pkg/engine/models"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/types"
)

// EngineClient implements the client.EngineClient interface
type EngineClient struct {
	client api.Client
}

func NewEngineClientWithDefaults() *EngineClient {
	// Use shared default socket path from global config
	client, _ := client.New(client.Options{
		SocketPath: config.DefaultSocket,
	})

	return &EngineClient{
		client: client,
	}
}

func NewEngineClient(socketPath string) (*EngineClient, error) {
	engineClient, err := client.New(client.Options{
		SocketPath: socketPath,
	})
	if err != nil {
		return nil, err
	}

	return &EngineClient{
		client: engineClient,
	}, nil
}

// Status checks if the engine is running
func (c *EngineClient) Status(ctx context.Context) error {
	_, err := c.client.Status(ctx)
	return err
}

// Ping is an alias for Status
func (c *EngineClient) Ping(ctx context.Context) error {
	return c.Status(ctx)
}

// LoadFunction loads a function into the engine
func (c *EngineClient) LoadFunction(ctx context.Context, namespace, name, tag string, config map[string]string) error {
	req := api.LoadRequest{
		BaseRequest: api.BaseRequest{
			Namespace: namespace,
			Name:      name,
		},
		Digest:    tag,
		Config:    config,
		ForceLoad: true,
	}

	_, err := c.client.LoadFunction(ctx, req)
	return err
}

// UnloadFunction unloads a function from the engine
func (c *EngineClient) UnloadFunction(ctx context.Context, namespace, name string) error {
	req := api.UnloadRequest{
		BaseRequest: api.BaseRequest{
			Namespace: namespace,
			Name:      name,
		},
	}

	return c.client.UnloadFunction(ctx, req)
}

// StopFunction stops a function in the engine
func (c *EngineClient) StopFunction(ctx context.Context, namespace, name string) error {
	req := api.StopRequest{
		BaseRequest: api.BaseRequest{
			Namespace: namespace,
			Name:      name,
		},
	}

	return c.client.StopFunction(ctx, req)
}

// CallFunction calls a function
func (c *EngineClient) CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte, config map[string]string) ([]byte, error) {
	req := api.CallRequest{
		BaseRequest: api.BaseRequest{
			Namespace: namespace,
			Name:      name,
		},
		Entrypoint: entrypoint,
		Payload:    string(payload),
		Config:     config,
	}

	return c.client.CallFunction(ctx, req)
}

// OneOffCall loads a function temporarily and calls it
func (c *EngineClient) OneOffCall(ctx context.Context, namespace, name, reference, entrypoint string, payload []byte, config map[string]string) ([]byte, error) {
	req := api.OneOffCallRequest{
		BaseRequest: api.BaseRequest{
			Namespace: namespace,
			Name:      name,
		},
		Reference:  reference,
		Entrypoint: entrypoint,
		Payload:    string(payload),
		Config:     config,
	}

	return c.client.OneOffCall(ctx, req)
}

// BuildFunction builds a function
func (c *EngineClient) BuildFunction(ctx context.Context, namespace, name, path, tag string, manifest manifest.FunctionManifest) (*types.BuildResult, error) {
	req := api.BuildRequest{
		BaseRequest: api.BaseRequest{
			Namespace: namespace,
			Name:      name,
		},
		Path:     path,
		Tag:      tag,
		Manifest: manifest,
	}

	resp, err := c.client.BuildFunction(ctx, req)
	if err != nil {
		return nil, err
	}

	// Convert to the expected type
	return &types.BuildResult{
		Name:      resp.BuildResult.Name,
		Namespace: resp.BuildResult.Namespace,
		Digest:    resp.BuildResult.Digest,
		BuildTime: resp.BuildResult.BuildTime,
		Tag:       resp.BuildResult.Tag,
		Reused:    resp.BuildResult.Reused,
	}, nil
}

// ListFunctions lists all loaded functions
func (c *EngineClient) ListFunctions(ctx context.Context) ([]types.LoadedFunction, error) {
	modelFunctions, err := c.client.ListFunctions(ctx)
	if err != nil {
		return nil, err
	}

	// Convert from models.Function to types.LoadedFunction
	var result []types.LoadedFunction
	for _, fn := range modelFunctions {
		result = append(result, types.LoadedFunction{
			Namespace: fn.Namespace,
			Name:      fn.Name,
			Status:    fn.Status,
		})
	}

	return result, nil
}

// GetFunctionLogs gets logs for a function
func (c *EngineClient) GetFunctionLogs(ctx context.Context, namespace, name string, since time.Duration, tail int) ([]string, error) {
	return c.client.GetFunctionLogs(ctx, namespace, name, since, tail)
}

// UnloadFunctions unloads multiple functions at once
func (c *EngineClient) UnloadFunctions(ctx context.Context, functions []models.FunctionReference) error {
	return c.client.UnloadFunctions(ctx, functions)
}

// StopFunctions stops multiple functions at once
func (c *EngineClient) StopFunctions(ctx context.Context, functions []models.FunctionReference) error {
	return c.client.StopFunctions(ctx, functions)
}
