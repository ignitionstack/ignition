package engineclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ignitionstack/ignition/pkg/types"
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
	ListFunctions(ctx context.Context) ([]types.LoadedFunction, error)

	// GetFunctionLogs gets logs for a function
	GetFunctionLogs(ctx context.Context, namespace, name string, since time.Duration, tail int) (LogsResponse, error)

	// UnloadFunctions unloads multiple functions at once
	UnloadFunctions(ctx context.Context, functions []FunctionReference) error

	// StopFunctions stops multiple functions at once
	StopFunctions(ctx context.Context, functions []FunctionReference) error
}

// FunctionReference represents a reference to a function with its service name
type FunctionReference struct {
	Namespace string
	Name      string
	Service   string
}

// clientImpl is the implementation of the Client interface
type clientImpl struct {
	socketPath string
	httpClient *http.Client
}

// Options for creating a new engine client
type Options struct {
	SocketPath string
}

// DefaultSocketPath returns the default engine socket path
func DefaultSocketPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return filepath.Join(homeDir, ".ignition", "engine.sock")
}

// New creates a new engine client with the given options
func New(opts Options) (Client, error) {
	socketPath := opts.SocketPath
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}

	// Create an HTTP client that connects to the Unix socket
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	return &clientImpl{
		socketPath: socketPath,
		httpClient: httpClient,
	}, nil
}

// Status checks if the engine is running
func (c *clientImpl) Status(ctx context.Context) (*StatusResponse, error) {
	// Create a context with a short timeout to avoid long hangs
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Send request
	resp, err := c.sendRequest(pingCtx, http.MethodGet, "status", nil)
	if err != nil {
		// Check for common connection errors and provide more helpful messages
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, fmt.Errorf("engine connection timed out: %w", err)
		}

		errMsg := err.Error()
		if strings.Contains(errMsg, "connect: no such file or directory") {
			return nil, errors.New("engine is not running (socket file not found)")
		} else if strings.Contains(errMsg, "connect: connection refused") {
			return nil, errors.New("engine is not running (connection refused)")
		}

		return nil, fmt.Errorf("cannot connect to the engine: %w", err)
	}
	defer resp.Body.Close()

	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}

	return &status, nil
}

// LoadFunction loads a function into the engine
func (c *clientImpl) LoadFunction(ctx context.Context, req LoadRequest) (*LoadResponse, error) {
	resp, err := c.sendRequest(ctx, http.MethodPost, "load", req)
	if err != nil {
		return nil, fmt.Errorf("failed to send load request: %w", err)
	}
	defer resp.Body.Close()

	var loadResp LoadResponse
	if err := json.NewDecoder(resp.Body).Decode(&loadResp); err != nil {
		return nil, fmt.Errorf("failed to decode load response: %w", err)
	}

	return &loadResp, nil
}

// UnloadFunction unloads a function from the engine
func (c *clientImpl) UnloadFunction(ctx context.Context, req UnloadRequest) error {
	resp, err := c.sendRequest(ctx, http.MethodPost, "unload", req)
	if err != nil {
		return fmt.Errorf("failed to send unload request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// StopFunction stops a function in the engine
func (c *clientImpl) StopFunction(ctx context.Context, req StopRequest) error {
	resp, err := c.sendRequest(ctx, http.MethodPost, "stop", req)
	if err != nil {
		return fmt.Errorf("failed to send stop request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// CallFunction calls a function
func (c *clientImpl) CallFunction(ctx context.Context, req CallRequest) ([]byte, error) {
	resp, err := c.sendRequest(ctx, http.MethodPost, "call", req)
	if err != nil {
		return nil, fmt.Errorf("failed to send call request: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// OneOffCall loads a function temporarily and calls it
func (c *clientImpl) OneOffCall(ctx context.Context, req OneOffCallRequest) ([]byte, error) {
	resp, err := c.sendRequest(ctx, http.MethodPost, "call-once", req)
	if err != nil {
		return nil, fmt.Errorf("failed to send one-off call request: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// BuildFunction builds a function
func (c *clientImpl) BuildFunction(ctx context.Context, req BuildRequest) (*BuildResponse, error) {
	resp, err := c.sendRequest(ctx, http.MethodPost, "build", req)
	if err != nil {
		return nil, fmt.Errorf("failed to send build request: %w", err)
	}
	defer resp.Body.Close()

	var buildResp BuildResponse
	if err := json.NewDecoder(resp.Body).Decode(&buildResp); err != nil {
		return nil, fmt.Errorf("failed to decode build response: %w", err)
	}

	return &buildResp, nil
}

// ListFunctions lists all loaded functions
func (c *clientImpl) ListFunctions(ctx context.Context) ([]types.LoadedFunction, error) {
	resp, err := c.sendRequest(ctx, http.MethodGet, "loaded", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send list request: %w", err)
	}
	defer resp.Body.Close()

	var functions []types.LoadedFunction
	if err := json.NewDecoder(resp.Body).Decode(&functions); err != nil {
		return nil, fmt.Errorf("failed to decode list response: %w", err)
	}

	return functions, nil
}

// GetFunctionLogs gets logs for a function
func (c *clientImpl) GetFunctionLogs(ctx context.Context, namespace, name string, since time.Duration, tail int) (LogsResponse, error) {
	// Create query parameters
	query := url.Values{}

	// Add since parameter (in seconds) if specified
	if since > 0 {
		sinceSeconds := int64(since.Seconds())
		query.Add("since", strconv.FormatInt(sinceSeconds, 10))
	}

	// Add tail parameter if specified
	if tail > 0 {
		query.Add("tail", strconv.Itoa(tail))
	}

	// Create the URL with query parameters
	endpoint := fmt.Sprintf("logs/%s/%s", namespace, name)
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	resp, err := c.sendRequest(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send logs request: %w", err)
	}
	defer resp.Body.Close()

	var logs LogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return nil, fmt.Errorf("failed to decode logs response: %w", err)
	}

	return logs, nil
}

// batchFunctionOperation applies an operation to multiple functions concurrently
func (c *clientImpl) batchFunctionOperation(
	ctx context.Context,
	functions []FunctionReference,
	operationName string,
	operation func(context.Context, string, string) error,
) error {
	var errs []string
	var errsMu sync.Mutex
	var wg sync.WaitGroup

	for _, function := range functions {
		wg.Add(1)
		go func(namespace, name, serviceName string) {
			defer wg.Done()

			err := operation(ctx, namespace, name)
			if err != nil {
				errsMu.Lock()
				errs = append(errs, fmt.Sprintf("failed to %s function '%s/%s' for service '%s': %v",
					operationName, namespace, name, serviceName, err))
				errsMu.Unlock()
			}
		}(function.Namespace, function.Name, function.Service)
	}

	// Wait for all operations to complete
	wg.Wait()

	// Check for errors
	if len(errs) > 0 {
		return fmt.Errorf("failed to %s some functions:\n%s", operationName, strings.Join(errs, "\n"))
	}

	return nil
}

// UnloadFunctions unloads multiple functions at once
func (c *clientImpl) UnloadFunctions(ctx context.Context, functions []FunctionReference) error {
	return c.batchFunctionOperation(ctx, functions, "unload", func(ctx context.Context, namespace, name string) error {
		req := UnloadRequest{
			BaseRequest: BaseRequest{
				Namespace: namespace,
				Name:      name,
			},
		}
		return c.UnloadFunction(ctx, req)
	})
}

// StopFunctions stops multiple functions at once
func (c *clientImpl) StopFunctions(ctx context.Context, functions []FunctionReference) error {
	return c.batchFunctionOperation(ctx, functions, "stop", func(ctx context.Context, namespace, name string) error {
		req := StopRequest{
			BaseRequest: BaseRequest{
				Namespace: namespace,
				Name:      name,
			},
		}
		return c.StopFunction(ctx, req)
	})
}

// sendRequest is a helper function to send a request to the engine
func (c *clientImpl) sendRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonData)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(
		ctx,
		method,
		"http://unix/"+endpoint,
		bodyReader,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var errResp ErrorResponse
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil {
			return nil, errResp
		}

		return nil, fmt.Errorf("request failed (status code %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}
