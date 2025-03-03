package client

import (
	"context"

	"github.com/ignitionstack/ignition/pkg/types"
)

// The actual implementation is in internal/services/engine_client.go.
type EngineClient interface {
	LoadFunction(ctx context.Context, namespace, name, tag string, config map[string]string) error
	UnloadFunction(ctx context.Context, namespace, name string) error
	CallFunction(ctx context.Context, namespace, name, entrypoint string, payload []byte, config map[string]string) ([]byte, error)
	BuildFunction(ctx context.Context, request *types.BuildRequest) (*types.BuildResult, error)
}
