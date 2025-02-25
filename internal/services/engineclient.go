package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// EngineClient is a client for communicating with the Ignition engine
type EngineClient struct {
	socketPath string
	httpClient *http.Client
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
func (c *EngineClient) ListFunctions(ctx context.Context) ([]FunctionDetails, error) {
	// Create HTTP request to the list endpoint
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"http://unix/list",
		bytes.NewBuffer([]byte("{}")), // Empty JSON object to list all functions
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create list request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	
	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send list request: %w", err)
	}
	defer resp.Body.Close()
	
	// Check response
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list functions (status code %d): %s", 
			resp.StatusCode, string(responseBody))
	}
	
	// Parse response
	var functions []FunctionDetails
	if err := json.NewDecoder(resp.Body).Decode(&functions); err != nil {
		return nil, fmt.Errorf("failed to decode list response: %w", err)
	}
	
	return functions, nil
}