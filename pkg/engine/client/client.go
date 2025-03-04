package client

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

	"github.com/ignitionstack/ignition/pkg/engine/api"
	"github.com/ignitionstack/ignition/pkg/engine/models"
)

// clientImpl is the implementation of the api.Client interface
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
func New(opts Options) (api.Client, error) {
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
func (c *clientImpl) Status(ctx context.Context) (*api.StatusResponse, error) {
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

	var status api.StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("failed to decode status response: %w", err)
	}

	return &status, nil
}

// LoadFunction loads a function into the engine
func (c *clientImpl) LoadFunction(ctx context.Context, req api.LoadRequest) (*api.LoadResponse, error) {
	resp, err := c.sendRequest(ctx, http.MethodPost, "load", req)
	if err != nil {
		return nil, fmt.Errorf("failed to send load request: %w", err)
	}
	defer resp.Body.Close()

	var loadResp api.LoadResponse
	if err := json.NewDecoder(resp.Body).Decode(&loadResp); err != nil {
		return nil, fmt.Errorf("failed to decode load response: %w", err)
	}

	return &loadResp, nil
}

// UnloadFunction unloads a function from the engine
func (c *clientImpl) UnloadFunction(ctx context.Context, req api.UnloadRequest) error {
	resp, err := c.sendRequest(ctx, http.MethodPost, "unload", req)
	if err != nil {
		return fmt.Errorf("failed to send unload request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// StopFunction stops a function in the engine
func (c *clientImpl) StopFunction(ctx context.Context, req api.StopRequest) error {
	resp, err := c.sendRequest(ctx, http.MethodPost, "stop", req)
	if err != nil {
		return fmt.Errorf("failed to send stop request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}

// CallFunction calls a function
func (c *clientImpl) CallFunction(ctx context.Context, req api.CallRequest) ([]byte, error) {
	resp, err := c.sendRequest(ctx, http.MethodPost, "call", req)
	if err != nil {
		return nil, fmt.Errorf("failed to send call request: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// OneOffCall loads a function temporarily and calls it
func (c *clientImpl) OneOffCall(ctx context.Context, req api.OneOffCallRequest) ([]byte, error) {
	resp, err := c.sendRequest(ctx, http.MethodPost, "call-once", req)
	if err != nil {
		return nil, fmt.Errorf("failed to send one-off call request: %w", err)
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// BuildFunction builds a function
func (c *clientImpl) BuildFunction(ctx context.Context, req api.BuildRequest) (*api.BuildResponse, error) {
	resp, err := c.sendRequest(ctx, http.MethodPost, "build", req)
	if err != nil {
		return nil, fmt.Errorf("failed to send build request: %w", err)
	}
	defer resp.Body.Close()

	var buildResp api.BuildResponse
	if err := json.NewDecoder(resp.Body).Decode(&buildResp); err != nil {
		return nil, fmt.Errorf("failed to decode build response: %w", err)
	}

	return &buildResp, nil
}

// ListFunctions lists all loaded functions
func (c *clientImpl) ListFunctions(ctx context.Context) ([]models.Function, error) {
	resp, err := c.sendRequest(ctx, http.MethodGet, "loaded", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to send list request: %w", err)
	}
	defer resp.Body.Close()

	var functions []models.Function
	if err := json.NewDecoder(resp.Body).Decode(&functions); err != nil {
		return nil, fmt.Errorf("failed to decode list response: %w", err)
	}

	return functions, nil
}

// GetFunctionLogs gets logs for a function
func (c *clientImpl) GetFunctionLogs(ctx context.Context, namespace, name string, since time.Duration, tail int) (api.LogsResponse, error) {
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

	var logs api.LogsResponse
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return nil, fmt.Errorf("failed to decode logs response: %w", err)
	}

	return logs, nil
}

// UnloadFunctions unloads multiple functions at once
func (c *clientImpl) UnloadFunctions(ctx context.Context, functions []models.FunctionReference) error {
	var unloadErrs []string
	var unloadErrsMu sync.Mutex
	var wg sync.WaitGroup

	for _, function := range functions {
		wg.Add(1)
		go func(namespace, name, serviceName string) {
			defer wg.Done()

			req := api.UnloadRequest{
				BaseRequest: api.BaseRequest{
					Namespace: namespace,
					Name:      name,
				},
			}

			err := c.UnloadFunction(ctx, req)
			if err != nil {
				unloadErrsMu.Lock()
				unloadErrs = append(unloadErrs, fmt.Sprintf("failed to unload function '%s/%s' for service '%s': %v",
					namespace, name, serviceName, err))
				unloadErrsMu.Unlock()
			}
		}(function.Namespace, function.Name, function.Service)
	}

	// Wait for all unload operations to complete
	wg.Wait()

	// Check for errors
	if len(unloadErrs) > 0 {
		return fmt.Errorf("failed to unload some functions:\n%s", strings.Join(unloadErrs, "\n"))
	}

	return nil
}

// StopFunctions stops multiple functions at once
func (c *clientImpl) StopFunctions(ctx context.Context, functions []models.FunctionReference) error {
	var stopErrs []string
	var stopErrsMu sync.Mutex
	var wg sync.WaitGroup

	for _, function := range functions {
		wg.Add(1)
		go func(namespace, name, serviceName string) {
			defer wg.Done()

			req := api.StopRequest{
				BaseRequest: api.BaseRequest{
					Namespace: namespace,
					Name:      name,
				},
			}

			err := c.StopFunction(ctx, req)
			if err != nil {
				stopErrsMu.Lock()
				stopErrs = append(stopErrs, fmt.Sprintf("failed to stop function '%s/%s' for service '%s': %v",
					namespace, name, serviceName, err))
				stopErrsMu.Unlock()
			}
		}(function.Namespace, function.Name, function.Service)
	}

	// Wait for all stop operations to complete
	wg.Wait()

	// Check for errors
	if len(stopErrs) > 0 {
		return fmt.Errorf("failed to stop some functions:\n%s", strings.Join(stopErrs, "\n"))
	}

	return nil
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

		var errResp api.ErrorResponse
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil {
			return nil, errResp
		}

		return nil, fmt.Errorf("request failed (status code %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}
