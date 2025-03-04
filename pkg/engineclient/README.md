# Ignition Engine Client (Deprecated)

**DEPRECATED: This package is deprecated and will be removed in a future release. 
Please use `pkg/engine/api` and `pkg/engine/client` instead.**

This package contains the original engine client implementation. It has been replaced
by a more structured implementation in the `pkg/engine/api` and `pkg/engine/client` packages.

## Migration

To migrate from the old client to the new one:

```go
// Old usage
import "github.com/ignitionstack/ignition/pkg/engineclient"

client, _ := engineclient.New(engineclient.Options{
    SocketPath: "/path/to/socket",
})

// New usage
import (
    "github.com/ignitionstack/ignition/pkg/engine/api"
    "github.com/ignitionstack/ignition/pkg/engine/client"
)

engineClient, _ := client.New(client.Options{
    SocketPath: "/path/to/socket",
})

// Using the client with structured types
req := api.LoadRequest{
    BaseRequest: api.BaseRequest{
        Namespace: "default",
        Name:      "my-function",
    },
    Digest: "latest",
}

resp, err := engineClient.LoadFunction(ctx, req)
```