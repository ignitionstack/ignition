package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	extism "github.com/extism/go-sdk"
	"github.com/go-playground/validator/v10"
	"github.com/ignitionstack/ignition/pkg/registry"
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

	// Register socket endpoints
	mux.HandleFunc("/load", h.withMiddleware(h.handleLoad, commonMiddleware...))
	mux.HandleFunc("/list", h.withMiddleware(h.handleList, commonMiddleware...))
	mux.HandleFunc("/build", h.withMiddleware(h.handleBuild, commonMiddleware...))
	mux.HandleFunc("/reassign-tag", h.withMiddleware(h.handleReassignTag, commonMiddleware...))
	mux.HandleFunc("/call-once", h.withMiddleware(h.handleOneOffCall, commonMiddleware...))
	mux.HandleFunc("/status", h.withMiddleware(h.handleStatus, h.methodMiddleware(http.MethodGet), h.errorMiddleware()))

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
	var req LoadRequest
	if err := h.decodeAndValidate(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received load request for function: %s/%s (digest: %s)",
		req.Namespace, req.Name, req.Digest)

	if err := h.engine.LoadFunction(req.Namespace, req.Name, req.Digest); err != nil {
		return err
	}

	return h.writeJSONResponse(w, map[string]string{"message": "Function loaded successfully"})
}

// handleList lists functions in the registry
func (h *Handlers) handleList(w http.ResponseWriter, r *http.Request) error {
	var req FunctionRequest
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

// handleBuild builds a function and stores it in the registry
func (h *Handlers) handleBuild(w http.ResponseWriter, r *http.Request) error {
	var req BuildRequest
	if err := h.decodeAndValidate(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received build request for function: %s/%s", req.Namespace, req.Name)

	result, err := h.engine.BuildFunction(req.Namespace, req.Name, req.Path, req.Tag, req.Manifest)
	if err != nil {
		return NewInternalServerError(fmt.Sprintf("Build failed: %v", err))
	}

	response := BuildResponse{
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
			return NewNotFoundError("Function not loaded")
		}
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(output)
	return err
}

// handleOneOffCall handles one-off function calls
func (h *Handlers) handleOneOffCall(w http.ResponseWriter, r *http.Request) error {
	var req OneOffCallRequest
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
	}

	// Create and configure the plugin
	config := extism.PluginConfig{
		EnableWasi: versionInfo.Settings.Wasi,
	}

	// Initialize the plugin
	plugin, err := extism.NewPlugin(context.Background(), manifest, config, []extism.HostFunction{})
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
	var req ReassignTagRequest
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
