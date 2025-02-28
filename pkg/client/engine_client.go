package client

import (
	"context"
	"net"
	"net/http"

	"github.com/ignitionstack/ignition/pkg/types"
)

type EngineClient interface {
	LoadFunction(ctx context.Context, namespace, name, tag string, config map[string]string) error
	UnloadFunction(ctx context.Context, namespace, name string) error
	CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte, config map[string]string) ([]byte, error)
	BuildFunction(ctx context.Context, request *types.BuildRequest) (*types.BuildResult, error)
}

type HTTPEngineClient struct {
	client     *http.Client
	socketPath string
}

func (c *HTTPEngineClient) BuildFunction(ctx context.Context, request *types.BuildRequest) (*types.BuildResult, error) {
	// TODO: Implement this method
	return &types.BuildResult{}, nil
}

func (c *HTTPEngineClient) CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte, config map[string]string) ([]byte, error) {
	// TODO: Implement this method
	return []byte{}, nil
}

func (c *HTTPEngineClient) LoadFunction(ctx context.Context, namespace, name, tag string, config map[string]string) error {
	// TODO: Implement this method
	return nil
}

func (c *HTTPEngineClient) UnloadFunction(ctx context.Context, namespace, name string) error {
	// TODO: Implement this method
	return nil
}

func NewEngineClient(socketPath string) EngineClient {
	return &HTTPEngineClient{
		client: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		},
		socketPath: socketPath,
	}
}
