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
	"github.com/ignitionstack/ignition/pkg/registry"
	"github.com/ignitionstack/ignition/pkg/types"
)

// Handlers contains HTTP handlers for engine endpoints
type Handlers struct {
	engine    *Engine
	logger    Logger
	validator *validator.Validate
}

// NewHandlers creates a new Handlers instance
func NewHandlers(engine *Engine, logger Logger) *Handlers {
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

	if err := h.engine.LoadFunction(req.Namespace, req.Name, req.Digest, req.Config); err != nil {
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

// handleLoadedFunctions lists currently loaded functions in memory
func (h *Handlers) handleLoadedFunctions(w http.ResponseWriter, _ *http.Request) error {
	h.logger.Printf("Received request to list loaded functions")

	// Get all loaded functions
	h.engine.pluginsMux.RLock()
	loadedFunctions := make([]types.LoadedFunction, 0, len(h.engine.plugins))

	for key := range h.engine.plugins {
		parts := strings.Split(key, "/")
		if len(parts) == 2 {
			loadedFunctions = append(loadedFunctions, types.LoadedFunction{
				Namespace: parts[0],
				Name:      parts[1],
			})
		}
	}
	h.engine.pluginsMux.RUnlock()

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
	// Parse path: /namespace/name/entrypoint
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) != 3 {
		return NewBadRequestError("Invalid URL format: expected /namespace/name/entrypoint")
	}

	var req struct {
		Payload string `json:"payload"`
	}
	if err := h.decodeJSONRequest(r, &req); err != nil {
		return err
	}

	namespace, name, entrypoint := pathParts[0], pathParts[1], pathParts[2]
	h.logger.Printf("Received call request for function: %s/%s, entrypoint: %s",
		namespace, name, entrypoint)

	output, err := h.engine.CallFunction(namespace, name, entrypoint, []byte(req.Payload))
	if err != nil {
		if err == ErrFunctionNotLoaded {
			// Check if this function was previously loaded but was unloaded due to TTL
			
			// Check if the function exists in the registry
			metadata, err := h.engine.GetRegistry().Get(namespace, name)
			if err != nil {
				if err == registry.ErrFunctionNotFound {
					return NewNotFoundError("Function not found in registry")
				}
				return fmt.Errorf("failed to fetch function metadata: %w", err)
			}
			
			// Check if this function was ever loaded before and get previous config
			wasLoaded, previousConfig := h.engine.WasPreviouslyLoaded(namespace, name)
			if !wasLoaded {
				return NewNotFoundError("Function not loaded")
			}
			
			// Function was loaded before, try to reload it
			h.logger.Printf("Function %s/%s was previously loaded, attempting to reload with previous config", namespace, name)
			
			// Find the latest version to load
			if len(metadata.Versions) == 0 {
				return NewNotFoundError("No versions available for this function")
			}
			
			// Try to find a version with the "latest" tag
			var tagToLoad string
			for _, version := range metadata.Versions {
				for _, tag := range version.Tags {
					if tag == "latest" {
						tagToLoad = "latest"
						break
					}
				}
				if tagToLoad != "" {
					break
				}
			}
			
			// If no "latest" tag found, use the most recent version's digest
			if tagToLoad == "" {
				// Versions should be sorted with most recent first
				latestVersion := metadata.Versions[0]
				tagToLoad = latestVersion.FullDigest
			}
			
			// Load the function with the chosen tag and previous config
			if err := h.engine.LoadFunction(namespace, name, tagToLoad, previousConfig); err != nil {
				return fmt.Errorf("failed to reload function: %w", err)
			}
			
			// Try calling the function again
			output, err = h.engine.CallFunction(namespace, name, entrypoint, []byte(req.Payload))
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(output)
	return err
}

// handleOneOffCall handles one-off function calls
func (h *Handlers) handleOneOffCall(w http.ResponseWriter, r *http.Request) error {
	var req types.OneOffCallRequest
	if err := h.decodeAndValidate(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received one-off call request for function: %s/%s (reference: %s, entrypoint: %s)",
		req.Namespace, req.Name, req.Reference, req.Entrypoint)

	// Fetch the function from the registry
	wasmBytes, versionInfo, err := h.engine.GetRegistry().Pull(req.Namespace, req.Name, req.Reference)
	if err != nil {
		if err == registry.ErrFunctionNotFound || err == registry.ErrVersionNotFound {
			return NewNotFoundError(err.Error())
		}
		return err
	}

	// Create a manifest for the function
	manifest := extism.Manifest{
		AllowedHosts: versionInfo.Settings.AllowedUrls,
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
		Config: req.Config,
	}

	// Create and configure the plugin
	pluginConfig := extism.PluginConfig{
		EnableWasi: versionInfo.Settings.Wasi,
	}

	// Initialize the plugin
	plugin, err := extism.NewPlugin(context.Background(), manifest, pluginConfig, []extism.HostFunction{})
	if err != nil {
		return NewInternalServerError(fmt.Sprintf("Failed to initialize plugin: %v", err))
	}
	defer plugin.Close(context.Background())

	// Call the function
	_, output, err := plugin.Call(req.Entrypoint, []byte(req.Payload))
	if err != nil {
		return NewInternalServerError(fmt.Sprintf("Failed to call function: %v", err))
	}

	// Return the output
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(output)
	return err
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
	// In a real implementation, this would gather actual metrics
	status := map[string]interface{}{
		"status":           "running",
		"loaded_functions": len(h.engine.plugins),
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
	functionKey := getFunctionKey(namespace, name)
	h.engine.pluginsMux.RLock()
	_, exists := h.engine.plugins[functionKey]
	h.engine.pluginsMux.RUnlock()

	if !exists {
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
	functionKey := getFunctionKey(namespace, name)

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