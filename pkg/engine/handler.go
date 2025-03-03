package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	extism "github.com/extism/go-sdk"
	"github.com/go-playground/validator/v10"
	"github.com/ignitionstack/ignition/pkg/engine/components"
	"github.com/ignitionstack/ignition/pkg/engine/logging"
	"github.com/ignitionstack/ignition/pkg/registry"
	"github.com/ignitionstack/ignition/pkg/types"
)

// Handlers contains HTTP handlers for engine endpoints
type Handlers struct {
	engine    *Engine // The engine instance that provides all functionality
	logger    logging.Logger
	validator *validator.Validate
}

// NewHandlers creates a new Handlers instance
func NewHandlers(engine *Engine, logger logging.Logger) *Handlers {
	return &Handlers{
		engine:    engine,
		logger:    logger,
		validator: validator.New(),
	}
}

// Route Configuration
// UnixSocketHandler returns a HTTP handler for unix socket endpoints
func (h *Handlers) UnixSocketHandler() http.Handler {
	mux := http.NewServeMux()

	// Common middleware stack for socket handlers
	commonMiddleware := []Middleware{
		h.methodMiddleware(http.MethodPost),
		h.loggingMiddleware(),
		h.errorMiddleware(),
	}

	// Common middleware stack for GET endpoints
	getMiddleware := []Middleware{
		h.methodMiddleware(http.MethodGet),
		h.loggingMiddleware(),
		h.errorMiddleware(),
	}

	// Register socket endpoints
	mux.HandleFunc("/load", h.withMiddleware(h.handleLoad, commonMiddleware...))
	mux.HandleFunc("/unload", h.withMiddleware(h.handleUnload, commonMiddleware...))
	mux.HandleFunc("/stop", h.withMiddleware(h.handleStop, commonMiddleware...))
	mux.HandleFunc("/list", h.withMiddleware(h.handleList, commonMiddleware...))
	mux.HandleFunc("/build", h.withMiddleware(h.handleBuild, commonMiddleware...))
	mux.HandleFunc("/reassign-tag", h.withMiddleware(h.handleReassignTag, commonMiddleware...))
	mux.HandleFunc("/call-once", h.withMiddleware(h.handleOneOffCall, commonMiddleware...))
	mux.HandleFunc("/status", h.withMiddleware(h.handleStatus, h.methodMiddleware(http.MethodGet), h.errorMiddleware()))
	mux.HandleFunc("/loaded", h.withMiddleware(h.handleLoadedFunctions, h.methodMiddleware(http.MethodGet), h.errorMiddleware()))
	mux.HandleFunc("/logs/", h.withMiddleware(h.handleFunctionLogs, getMiddleware...))

	return mux
}

// HTTPHandler returns a HTTP handler for public endpoints
func (h *Handlers) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// Common middleware stack for HTTP handlers
	commonMiddleware := []Middleware{
		h.corsMiddleware(),
		h.loggingMiddleware(),
		h.errorMiddleware(),
	}

	// Register HTTP endpoints
	mux.HandleFunc("/", h.withMiddleware(h.handleFunctionCall,
		append(commonMiddleware, h.methodMiddleware(http.MethodPost))...))

	// Add health check endpoint
	mux.HandleFunc("/health", h.withMiddleware(h.handleHealth,
		h.methodMiddleware(http.MethodGet), h.errorMiddleware()))

	return mux
}

// Utility methods for request/response handling
// decodeJSONRequest decodes a JSON request body into a struct
func (h *Handlers) decodeJSONRequest(r *http.Request, v interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return NewBadRequestError("Invalid request body")
	}
	return nil
}

// decodeAndValidate decodes and validates a request
func (h *Handlers) decodeAndValidate(r *http.Request, v interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return NewBadRequestError("Invalid request body")
	}

	if err := h.validator.Struct(v); err != nil {
		return NewBadRequestError(fmt.Sprintf("Validation failed: %v", err))
	}

	return nil
}

// writeJSONResponse writes a JSON response
func (h *Handlers) writeJSONResponse(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(data)
}

// Handler Implementations
// handleLoad loads a function into memory
func (h *Handlers) handleLoad(w http.ResponseWriter, r *http.Request) error {
	var req types.LoadRequest
	if err := h.decodeAndValidate(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received load request for function: %s/%s (digest: %s)",
		req.Namespace, req.Name, req.Digest)

	ctx := r.Context()
	if err := h.engine.LoadFunctionWithForce(ctx, req.Namespace, req.Name, req.Digest, req.Config, req.ForceLoad); err != nil {
		return err
	}

	return h.writeJSONResponse(w, map[string]string{"message": "Function loaded successfully"})
}

// handleList lists functions in the registry
func (h *Handlers) handleList(w http.ResponseWriter, r *http.Request) error {
	var req types.FunctionRequest
	if err := h.decodeJSONRequest(r, &req); err != nil {
		// If decoding fails, it might be an empty request for listing all functions
		if req.Namespace == "" && req.Name == "" {
			return h.handleListAll(w, r)
		}
		return err
	}

	// If both namespace and name are empty, list all functions
	if req.Namespace == "" && req.Name == "" {
		return h.handleListAll(w, r)
	}

	h.logger.Printf("Received list request for function: %s/%s", req.Namespace, req.Name)

	metadata, err := h.engine.GetRegistry().Get(req.Namespace, req.Name)
	if err != nil {
		if err == registry.ErrFunctionNotFound {
			return NewNotFoundError("Function not found")
		}
		return fmt.Errorf("failed to fetch function metadata: %w", err)
	}

	return h.writeJSONResponse(w, metadata)
}

// handleListAll lists all functions in the registry
func (h *Handlers) handleListAll(w http.ResponseWriter, _ *http.Request) error {
	h.logger.Printf("Received request to list all functions")

	functions, err := h.engine.GetRegistry().ListAll()
	if err != nil {
		return fmt.Errorf("failed to list functions: %w", err)
	}

	return h.writeJSONResponse(w, functions)
}

// handleLoadedFunctions lists currently loaded, previously loaded, and stopped functions
func (h *Handlers) handleLoadedFunctions(w http.ResponseWriter, _ *http.Request) error {
	h.logger.Printf("Received request to list loaded functions")

	// Get list of loaded functions from the plugin manager
	functionKeys := h.engine.pluginManager.ListLoadedFunctions()

	// Create a set of loaded functions for fast lookup
	loadedFunctionsSet := make(map[string]bool, len(functionKeys))
	for _, key := range functionKeys {
		loadedFunctionsSet[key] = true
	}

	// Start with currently running functions
	loadedFunctions := make([]types.LoadedFunction, 0, len(functionKeys))
	for _, key := range functionKeys {
		parts := strings.Split(key, "/")
		if len(parts) == 2 {
			loadedFunctions = append(loadedFunctions, types.LoadedFunction{
				Namespace: parts[0],
				Name:      parts[1],
				Status:    "running",
			})
		}
	}

	// Get previously loaded functions and stopped functions
	previouslyLoadedMap := h.engine.pluginManager.GetPreviouslyLoadedFunctions()
	stoppedFunctionsMap := h.engine.pluginManager.GetStoppedFunctions()

	// Process all function keys we know about
	for key := range previouslyLoadedMap {
		// Skip if it's already in the loaded list
		if loadedFunctionsSet[key] {
			continue
		}

		parts := strings.Split(key, "/")
		if len(parts) == 2 {
			// Determine the status - "stopped" takes precedence over "unloaded"
			status := "unloaded"
			if _, isStopped := stoppedFunctionsMap[key]; isStopped {
				status = "stopped"
			}

			loadedFunctions = append(loadedFunctions, types.LoadedFunction{
				Namespace: parts[0],
				Name:      parts[1],
				Status:    status,
			})
		}
	}

	return h.writeJSONResponse(w, loadedFunctions)
}

// handleBuild builds a function and stores it in the registry
func (h *Handlers) handleBuild(w http.ResponseWriter, r *http.Request) error {
	var req ExtendedBuildRequest
	if err := h.decodeAndValidate(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received build request for function: %s/%s", req.Namespace, req.Name)

	result, err := h.engine.BuildFunction(req.Namespace, req.Name, req.Path, req.Tag, req.Manifest)
	if err != nil {
		return NewInternalServerError(fmt.Sprintf("Build failed: %v", err))
	}

	response := types.BuildResponse{
		Digest:    result.Digest,
		Tag:       result.Tag,
		Status:    "success",
		BuildTime: result.BuildTime.String(),
	}

	return h.writeJSONResponse(w, response)
}

// handleFunctionCall handles function calls via HTTP
func (h *Handlers) handleFunctionCall(w http.ResponseWriter, r *http.Request) error {
	// Parse the request
	callParams, payload, err := h.parseFunctionCallRequest(r)
	if err != nil {
		return err
	}

	// Log the request
	h.logger.Printf("Received call request for function: %s/%s, entrypoint: %s",
		callParams.namespace, callParams.name, callParams.entrypoint)

	// Execute the function with auto-reload capability
	output, err := h.executeFunction(r.Context(), callParams, payload)
	if err != nil {
		return err
	}

	// Send the response
	return h.sendFunctionResponse(w, output)
}

// functionCallParams contains the parsed parameters of a function call
type functionCallParams struct {
	namespace  string
	name       string
	entrypoint string
}

// parseFunctionCallRequest parses the HTTP request for a function call
func (h *Handlers) parseFunctionCallRequest(r *http.Request) (*functionCallParams, string, error) {
	// Parse path: /namespace/name/entrypoint
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) != 3 {
		return nil, "", NewBadRequestError("Invalid URL format: expected /namespace/name/entrypoint")
	}

	// Parse payload from request body
	var req struct {
		Payload string `json:"payload"`
	}
	if err := h.decodeJSONRequest(r, &req); err != nil {
		return nil, "", err
	}

	// Return the parsed parameters
	return &functionCallParams{
		namespace:  pathParts[0],
		name:       pathParts[1],
		entrypoint: pathParts[2],
	}, req.Payload, nil
}

// executeFunction attempts to call a function, trying auto-reload if needed
func (h *Handlers) executeFunction(ctx context.Context, params *functionCallParams, payload string) ([]byte, error) {
	// Try calling the function with the request context
	output, err := h.engine.CallFunctionWithContext(
		ctx,
		params.namespace,
		params.name,
		params.entrypoint,
		[]byte(payload),
	)

	// Handle different error cases
	if err != nil {
		if err == ErrFunctionNotLoaded {
			// Try auto-reload if the function isn't loaded
			return h.handleFunctionAutoReload(ctx, params.namespace, params.name, params.entrypoint, payload)
		}

		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, NewRequestError("Request cancelled by client", http.StatusRequestTimeout)
		}

		// Return other errors unchanged
		return nil, err
	}

	return output, nil
}

// sendFunctionResponse sends the function output as the HTTP response
func (h *Handlers) sendFunctionResponse(w http.ResponseWriter, output []byte) error {
	w.Header().Set("Content-Type", "application/json")
	_, err := w.Write(output)
	return err
}

// handleFunctionAutoReload attempts to auto-reload a previously loaded function
func (h *Handlers) handleFunctionAutoReload(ctx context.Context, namespace, name, entrypoint, payload string) ([]byte, error) {
	// Validate preconditions for auto-reload
	if err := h.validateAutoReloadPreconditions(namespace, name); err != nil {
		return nil, err
	}

	// Get the function metadata and configuration
	metadata, previousConfig, err := h.getMetadataAndConfig(namespace, name)
	if err != nil {
		return nil, err
	}

	// Function was loaded before, try to reload it
	h.logger.Printf("Function %s/%s was previously loaded, attempting to reload with previous config", namespace, name)

	// Find the latest version tag and reload the function
	if err := h.reloadFunction(ctx, metadata, namespace, name, previousConfig); err != nil {
		return nil, err
	}

	// Try calling the function again
	return h.engine.CallFunctionWithContext(ctx, namespace, name, entrypoint, []byte(payload))
}

// validateAutoReloadPreconditions checks if auto-reload is allowed for this function
func (h *Handlers) validateAutoReloadPreconditions(namespace, name string) error {
	// Check if this function was ever loaded before
	wasLoaded, _ := h.engine.WasPreviouslyLoaded(namespace, name)
	if !wasLoaded {
		return NewNotFoundError("Function not loaded")
	}

	// Check if function is explicitly stopped - prevent auto-reload
	if h.engine.IsFunctionStopped(namespace, name) {
		h.logger.Printf("Function %s/%s is stopped and will not be auto-reloaded", namespace, name)
		return NewNotFoundError("Function was explicitly stopped and will not be auto-reloaded")
	}

	return nil
}

// getMetadataAndConfig retrieves function metadata and previous configuration
func (h *Handlers) getMetadataAndConfig(namespace, name string) (*registry.FunctionMetadata, map[string]string, error) {
	// Check if the function exists in the registry
	metadata, err := h.engine.GetRegistry().Get(namespace, name)
	if err != nil {
		if err == registry.ErrFunctionNotFound {
			return nil, nil, NewNotFoundError("Function not found in registry")
		}
		return nil, nil, fmt.Errorf("failed to fetch function metadata: %w", err)
	}

	// Check if there are any versions available
	if len(metadata.Versions) == 0 {
		return nil, nil, NewNotFoundError("No versions available for this function")
	}

	// Get the previous configuration
	_, previousConfig := h.engine.WasPreviouslyLoaded(namespace, name)

	return metadata, previousConfig, nil
}

// reloadFunction reloads a function with the latest tag and previous configuration
func (h *Handlers) reloadFunction(ctx context.Context, metadata *registry.FunctionMetadata, namespace, name string, previousConfig map[string]string) error {
	// Find the best tag to load
	tagToLoad := h.findLatestTag(metadata)

	// Load the function with the chosen tag and previous config
	if err := h.engine.LoadFunctionWithContext(ctx, namespace, name, tagToLoad, previousConfig); err != nil {
		return fmt.Errorf("failed to reload function: %w", err)
	}

	return nil
}

// findLatestTag looks for the latest tag in metadata, falling back to the most recent digest
func (h *Handlers) findLatestTag(metadata *registry.FunctionMetadata) string {
	// Try to find a version with the "latest" tag
	for _, version := range metadata.Versions {
		for _, tag := range version.Tags {
			if tag == "latest" {
				return "latest"
			}
		}
	}

	// If no "latest" tag found, use the most recent version's digest
	// (Versions should be sorted with most recent first)
	if len(metadata.Versions) > 0 {
		return metadata.Versions[0].FullDigest
	}

	return ""
}

// handleOneOffCall handles one-off function calls by splitting the process into clear stages
func (h *Handlers) handleOneOffCall(w http.ResponseWriter, r *http.Request) error {
	// Parse and validate the request
	req, err := h.parseOneOffCallRequest(r)
	if err != nil {
		return err
	}

	// Log the request
	h.logger.Printf("Received one-off call request for function: %s/%s (reference: %s, entrypoint: %s)",
		req.Namespace, req.Name, req.Reference, req.Entrypoint)

	// Get context from the request for cancellation support
	ctx := r.Context()

	// Execute the one-off call with cancellation support
	output, err := h.executeOneOffCall(ctx, req)
	if err != nil {
		return err
	}

	// Return the output
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(output)
	return err
}

// parseOneOffCallRequest parses and validates the one-off call request
func (h *Handlers) parseOneOffCallRequest(r *http.Request) (*types.OneOffCallRequest, error) {
	var req types.OneOffCallRequest
	if err := h.decodeAndValidate(r, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// executeOneOffCall processes a one-off call request through the necessary stages:
// 1. Pull the function from registry
// 2. Create a plugin
// 3. Call the function
func (h *Handlers) executeOneOffCall(ctx context.Context, req *types.OneOffCallRequest) ([]byte, error) {
	// Stage 1: Pull the function
	wasmBytes, versionInfo, err := h.pullFunction(ctx, req.Namespace, req.Name, req.Reference)
	if err != nil {
		return nil, err
	}

	// Stage 2: Create plugin
	plugin, err := h.createPlugin(ctx, wasmBytes, versionInfo, req.Config)
	if err != nil {
		return nil, err
	}
	defer plugin.Close(context.Background())

	// Stage 3: Call the function
	return h.callFunction(ctx, plugin, req.Entrypoint, req.Payload)
}

// pullFunction pulls a function from the registry with cancellation support
func (h *Handlers) pullFunction(ctx context.Context, namespace, name, reference string) ([]byte, *registry.VersionInfo, error) {
	// Define result type
	type pullResult struct {
		wasmBytes   []byte
		versionInfo *registry.VersionInfo
		err         error
	}

	// Create a channel for the result
	pullCh := make(chan pullResult, 1)

	// Fetch the function in a goroutine
	go func() {
		wasmBytes, versionInfo, err := h.engine.GetRegistry().Pull(namespace, name, reference)
		select {
		case pullCh <- pullResult{wasmBytes, versionInfo, err}:
		case <-ctx.Done():
			// Context cancelled, but we need to send something to avoid goroutine leak
			select {
			case pullCh <- pullResult{nil, nil, ctx.Err()}:
			default:
			}
		}
	}()

	// Wait for result or context cancellation
	var result pullResult
	select {
	case result = <-pullCh:
	case <-ctx.Done():
		result = <-pullCh // Clean up goroutine
	}

	// Handle errors
	if result.err != nil {
		if result.err == registry.ErrFunctionNotFound || result.err == registry.ErrVersionNotFound {
			return nil, nil, NewNotFoundError(result.err.Error())
		}
		if ctx.Err() != nil {
			return nil, nil, NewRequestError("Request cancelled by client", http.StatusRequestTimeout)
		}
		return nil, nil, result.err
	}

	return result.wasmBytes, result.versionInfo, nil
}

// createPlugin creates an Extism plugin from WASM bytes with cancellation support
func (h *Handlers) createPlugin(ctx context.Context, wasmBytes []byte, versionInfo *registry.VersionInfo, config map[string]string) (*extism.Plugin, error) {
	// Define result type
	type pluginResult struct {
		plugin *extism.Plugin
		err    error
	}

	// Create channel for the result
	pluginCh := make(chan pluginResult, 1)

	// Initialize the plugin in a goroutine
	go func() {
		plugin, err := components.CreatePlugin(wasmBytes, versionInfo, config)
		select {
		case pluginCh <- pluginResult{plugin, err}:
		case <-ctx.Done():
			if plugin != nil && err == nil {
				plugin.Close(context.Background()) // Clean up resources
			}
			select {
			case pluginCh <- pluginResult{nil, ctx.Err()}:
			default:
			}
		}
	}()

	// Wait for plugin creation or context cancellation
	var pluginRes pluginResult
	select {
	case pluginRes = <-pluginCh:
	case <-ctx.Done():
		pluginRes = <-pluginCh // Clean up goroutine
	}

	// Handle errors
	if pluginRes.err != nil {
		if ctx.Err() != nil {
			return nil, NewRequestError("Request cancelled by client", http.StatusRequestTimeout)
		}
		return nil, NewInternalServerError(fmt.Sprintf("Failed to initialize plugin: %v", pluginRes.err))
	}

	return pluginRes.plugin, nil
}

// callFunction calls a function in a plugin with cancellation support
func (h *Handlers) callFunction(ctx context.Context, plugin *extism.Plugin, entrypoint string, payload string) ([]byte, error) {
	// Define result type
	type callResult struct {
		output []byte
		err    error
	}

	// Create channel for the result
	callCh := make(chan callResult, 1)

	// Call the function in a goroutine
	go func() {
		_, output, err := plugin.Call(entrypoint, []byte(payload))
		select {
		case callCh <- callResult{output, err}:
		case <-ctx.Done():
			select {
			case callCh <- callResult{nil, ctx.Err()}:
			default:
			}
		}
	}()

	// Wait for call result or context cancellation
	var callRes callResult
	select {
	case callRes = <-callCh:
	case <-ctx.Done():
		callRes = <-callCh // Clean up goroutine
	}

	// Handle errors
	if callRes.err != nil {
		if ctx.Err() != nil {
			return nil, NewRequestError("Request cancelled by client", http.StatusRequestTimeout)
		}
		return nil, NewInternalServerError(fmt.Sprintf("Failed to call function: %v", callRes.err))
	}

	return callRes.output, nil
}

// handleReassignTag handles tag reassignment requests
func (h *Handlers) handleReassignTag(w http.ResponseWriter, r *http.Request) error {
	var req types.ReassignTagRequest
	if err := h.decodeAndValidate(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received reassign tag request for function: %s/%s (tag: %s, digest: %s)",
		req.Namespace, req.Name, req.Tag, req.Digest)

	if err := h.engine.ReassignTag(req.Namespace, req.Name, req.Tag, req.Digest); err != nil {
		if IsNotFoundError(err) {
			return NewNotFoundError(err.Error())
		}
		return err
	}

	return h.writeJSONResponse(w, map[string]string{"message": "Tag reassigned successfully"})
}

// handleStatus returns the current status of the engine
func (h *Handlers) handleStatus(w http.ResponseWriter, r *http.Request) error {
	// Get the count of loaded functions from the plugin manager
	loadedCount := h.engine.pluginManager.GetLoadedFunctionCount()

	status := map[string]interface{}{
		"status":           "running",
		"loaded_functions": loadedCount,
	}

	return h.writeJSONResponse(w, status)
}

// handleHealth is a simple health check endpoint
func (h *Handlers) handleHealth(w http.ResponseWriter, r *http.Request) error {
	return h.writeJSONResponse(w, map[string]string{"status": "ok"})
}

// handleUnload unloads a function from memory
func (h *Handlers) handleUnload(w http.ResponseWriter, r *http.Request) error {
	var req types.FunctionRequest
	if err := h.decodeAndValidate(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received unload request for function: %s/%s", req.Namespace, req.Name)

	if err := h.engine.UnloadFunction(req.Namespace, req.Name); err != nil {
		return err
	}

	return h.writeJSONResponse(w, map[string]string{"message": "Function unloaded successfully"})
}

// handleStop stops a function and prevents automatic reloading
func (h *Handlers) handleStop(w http.ResponseWriter, r *http.Request) error {
	var req types.FunctionRequest
	if err := h.decodeAndValidate(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received stop request for function: %s/%s", req.Namespace, req.Name)

	if err := h.engine.StopFunction(req.Namespace, req.Name); err != nil {
		return err
	}

	return h.writeJSONResponse(w, map[string]string{"message": "Function stopped successfully"})
}

// handleFunctionLogs returns logs for a specific function
func (h *Handlers) handleFunctionLogs(w http.ResponseWriter, r *http.Request) error {
	// Parse path: /logs/namespace/name
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/logs/"), "/")
	if len(pathParts) != 2 {
		return NewBadRequestError("Invalid URL format: expected /logs/namespace/name")
	}

	namespace, name := pathParts[0], pathParts[1]

	h.logger.Printf("Received logs request for function: %s/%s", namespace, name)

	// Check if function exists and is loaded
	if !h.engine.IsLoaded(namespace, name) {
		return NewNotFoundError(fmt.Sprintf("Function %s/%s is not loaded", namespace, name))
	}

	// Parse query parameters
	query := r.URL.Query()

	// Parse since parameter (seconds)
	var since time.Time
	if sinceStr := query.Get("since"); sinceStr != "" {
		sinceSeconds, err := strconv.ParseInt(sinceStr, 10, 64)
		if err != nil {
			return NewBadRequestError(fmt.Sprintf("Invalid 'since' parameter: %v", err))
		}
		since = time.Now().Add(-time.Duration(sinceSeconds) * time.Second)
	}

	// Parse tail parameter
	var tail int
	if tailStr := query.Get("tail"); tailStr != "" {
		var err error
		tail, err = strconv.Atoi(tailStr)
		if err != nil {
			return NewBadRequestError(fmt.Sprintf("Invalid 'tail' parameter: %v", err))
		}
	} else {
		// Default to returning all logs if tail is not specified
		tail = 0
	}

	// Get logs from the engine's logger for this function
	logs := h.getEngineLogs(namespace, name, since, tail)

	// Return logs as a JSON array
	return h.writeJSONResponse(w, logs)
}

// getEngineLogs retrieves logs for a specific function from the engine's log store
func (h *Handlers) getEngineLogs(namespace, name string, since time.Time, tail int) []string {
	functionKey := components.GetFunctionKey(namespace, name)

	// Retrieve logs from the engine's log store
	logs := h.engine.logStore.GetLogs(functionKey, since, tail)

	// If there are no logs, add an informational message
	if len(logs) == 0 {
		if h.engine.IsLoaded(namespace, name) {
			return []string{
				fmt.Sprintf("[%s] No logs available for function %s. The function is loaded but has not recorded any activity yet.",
					time.Now().Format(time.RFC3339), functionKey),
			}
		} else {
			return []string{
				fmt.Sprintf("[%s] No logs available for function %s. The function is not currently loaded.",
					time.Now().Format(time.RFC3339), functionKey),
			}
		}
	}

	return logs
}
