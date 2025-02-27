package client

import (
	"context"
	"net"
	"net/http"

	"github.com/ignitionstack/ignition/pkg/types"
)

type EngineClient interface {
	LoadFunction(ctx context.Context, namespace, name, tag string) error
	UnloadFunction(ctx context.Context, namespace, name string) error
	CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte) ([]byte, error)
	BuildFunction(ctx context.Context, request *types.BuildRequest) (*types.BuildResult, error)
}

type HTTPEngineClient struct {
	client     *http.Client
	socketPath string
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