package services

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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ignitionstack/ignition/pkg/types"
)

// EngineClient is a client for communicating with the Ignition engine.
type EngineClient struct {
	socketPath string
	httpClient *http.Client
}

// EngineFunctionDetails represents basic information about a function for the EngineClient.
type EngineFunctionDetails struct {
	Namespace string
	Name      string
	Status    string
}

// FunctionReference represents a reference to a function with its service name.
type FunctionReference struct {
	Namespace string
	Name      string
	Service   string
}

func NewEngineClientWithDefaults() *EngineClient {
	return &EngineClient{
		socketPath: "/tmp/ignition-engine.sock",
		httpClient: &http.Client{},
	}
}

func NewEngineClient(socketPath string) (*EngineClient, error) {
	// Create an HTTP client that connects to the Unix socket
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	return &EngineClient{
		socketPath: socketPath,
		httpClient: httpClient,
	}, nil
}

// Ping checks if the engine is running.
func (c *EngineClient) Ping(ctx context.Context) error {
	// Create a context with a short timeout to avoid long hangs
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// Create HTTP request
	req, err := http.NewRequestWithContext(
		pingCtx,
		http.MethodGet,
		"http://unix/status",
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to create ping request: %w", err)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Check for common connection errors and provide more helpful messages
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return fmt.Errorf("engine connection timed out: %w", err)
		}

		errMsg := err.Error()
		if strings.Contains(errMsg, "connect: no such file or directory") {
			return errors.New("engine is not running (socket file not found)")
		} else if strings.Contains(errMsg, "connect: connection refused") {
			return errors.New("engine is not running (connection refused)")
		}

		return fmt.Errorf("cannot connect to the engine: %w", err)
	}
	defer resp.Body.Close()

	// Check for a 200 OK response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("engine returned unexpected status: %s", resp.Status)
	}

	return nil
}

func (c *EngineClient) LoadFunction(ctx context.Context, namespace, name, tag string, config map[string]string) error {
	// Create HTTP request body
	reqBody := map[string]interface{}{
		"namespace": namespace,
		"name":      name,
		"digest":    tag,
	}

	// Add config if provided
	if len(config) > 0 {
		reqBody["config"] = config
	}

	// Convert to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal load request: %w", err)
	}

	// Create HTTP request to the load endpoint
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://unix/load",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to create load request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send load request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to load function (status code %d): %s",
			resp.StatusCode, string(responseBody))
	}

	return nil
}

// sendNamespaceNameRequest is a helper function to send a request with namespace and name.
func (c *EngineClient) sendNamespaceNameRequest(ctx context.Context, endpoint, action string, namespace, name string) error {
	// Create HTTP request body
	reqBody := map[string]interface{}{
		"namespace": namespace,
		"name":      name,
	}

	// Convert to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal %s request: %w", action, err)
	}

	// Create HTTP request to the endpoint
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://unix/"+endpoint,
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to create %s request: %w", action, err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send %s request: %w", action, err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to %s function (status code %d): %s",
			action, resp.StatusCode, string(responseBody))
	}

	return nil
}

func (c *EngineClient) UnloadFunction(ctx context.Context, namespace, name string) error {
	return c.sendNamespaceNameRequest(ctx, "unload", "unload", namespace, name)
}

// StopFunction stops a function and prevents it from being automatically reloaded.
func (c *EngineClient) StopFunction(ctx context.Context, namespace, name string) error {
	return c.sendNamespaceNameRequest(ctx, "stop", "stop", namespace, name)
}

func (c *EngineClient) ListFunctions(ctx context.Context) ([]EngineFunctionDetails, error) {
	// Create HTTP request to the loaded endpoint to get actually loaded functions
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://unix/loaded", // Use the new endpoint for loaded functions
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create loaded functions request: %w", err)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send loaded functions request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list loaded functions (status code %d): %s",
			resp.StatusCode, string(responseBody))
	}

	// Parse response
	var loadedFunctions []types.LoadedFunction
	if err := json.NewDecoder(resp.Body).Decode(&loadedFunctions); err != nil {
		return nil, fmt.Errorf("failed to decode loaded functions response: %w", err)
	}

	// Convert to EngineFunctionDetails
	var functions []EngineFunctionDetails
	for _, fn := range loadedFunctions {
		functions = append(functions, EngineFunctionDetails{
			Namespace: fn.Namespace,
			Name:      fn.Name,
			Status:    fn.Status,
		})
	}

	return functions, nil
}

func (c *EngineClient) GetFunctionLogs(ctx context.Context, namespace, name string, since time.Duration, tail int) ([]string, error) {
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
	logURL := fmt.Sprintf("http://unix/logs/%s/%s", namespace, name)
	if len(query) > 0 {
		logURL += "?" + query.Encode()
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		logURL,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create logs request: %w", err)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send logs request: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to retrieve logs (status code %d): %s",
			resp.StatusCode, string(responseBody))
	}

	// Parse response
	var logs []string
	if err := json.NewDecoder(resp.Body).Decode(&logs); err != nil {
		return nil, fmt.Errorf("failed to decode logs response: %w", err)
	}

	return logs, nil
}

// UnloadFunctions unloads all functions in the provided list.
func (c *EngineClient) UnloadFunctions(ctx context.Context, functions []FunctionReference) error {
	var unloadErrs []string
	var unloadErrsMu sync.Mutex
	var wg sync.WaitGroup

	for _, function := range functions {
		wg.Add(1)
		go func(namespace, name, serviceName string) {
			defer wg.Done()

			err := c.UnloadFunction(ctx, namespace, name)
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
