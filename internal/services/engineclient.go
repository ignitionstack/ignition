package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
	
	"github.com/ignitionstack/ignition/pkg/types"
)

// EngineClient is a client for communicating with the Ignition engine
type EngineClient struct {
	socketPath string
	httpClient *http.Client
}

// EngineFunctionDetails represents basic information about a function for the EngineClient
type EngineFunctionDetails struct {
	Namespace string
	Name      string
}

// NewEngineClientWithDefaults creates a minimal client with default values
func NewEngineClientWithDefaults() *EngineClient {
	return &EngineClient{
		socketPath: "/tmp/ignition-engine.sock",
		httpClient: &http.Client{},
	}
}

// NewEngineClient creates a new client for the Ignition engine
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

// Ping checks if the engine is running
func (c *EngineClient) Ping(ctx context.Context) error {
	// Create HTTP request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://unix/status",
		nil,
	)
	if err != nil {
		return err
	}
	
	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	return nil
}

// LoadFunction loads a function into the engine
func (c *EngineClient) LoadFunction(ctx context.Context, namespace, name, tag string) error {
	// Create HTTP request body
	reqBody := map[string]interface{}{
		"namespace": namespace,
		"name":      name,
		"digest":    tag,
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

// UnloadFunction unloads a function from the engine
func (c *EngineClient) UnloadFunction(ctx context.Context, namespace, name string) error {
	// Create HTTP request body
	reqBody := map[string]interface{}{
		"namespace": namespace,
		"name":      name,
	}
	
	// Convert to JSON
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal unload request: %w", err)
	}
	
	// Create HTTP request to the unload endpoint
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://unix/unload",  // This endpoint would need to be implemented in the engine
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return fmt.Errorf("failed to create unload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	
	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send unload request: %w", err)
	}
	defer resp.Body.Close()
	
	// Check response
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to unload function (status code %d): %s", 
			resp.StatusCode, string(responseBody))
	}
	
	return nil
}

// ListFunctions lists all functions loaded in the engine
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
		})
	}
	
	return functions, nil
}

// GetFunctionLogs retrieves logs for a specific function
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