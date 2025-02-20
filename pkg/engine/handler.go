package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	extism "github.com/extism/go-sdk"
	"github.com/go-playground/validator/v10"
	"github.com/ignitionstack/ignition/pkg/manifest"
	"github.com/ignitionstack/ignition/pkg/registry"
)

type Handlers struct {
	engine    *Engine
	logger    Logger
	validator *validator.Validate
}

func NewHandlers(engine *Engine, logger Logger) *Handlers {
	return &Handlers{
		engine:    engine,
		logger:    logger,
		validator: validator.New(),
	}
}

// Route Configuration
func (h *Handlers) UnixSocketHandler() http.Handler {
	mux := http.NewServeMux()

	middlewares := []Middleware{
		h.methodMiddleware(http.MethodPost),
		h.errorMiddleware(),
	}

	mux.HandleFunc("/load", h.withMiddleware(h.handleLoad, middlewares...))
	mux.HandleFunc("/list", h.withMiddleware(h.handleList, middlewares...))
	mux.HandleFunc("/build", h.withMiddleware(h.handleBuild, middlewares...))
	mux.HandleFunc("/reassign-tag", h.withMiddleware(h.handleReassignTag, middlewares...))
	mux.HandleFunc("/call-once", h.withMiddleware(h.handleOneOffCall, middlewares...))

	return mux
}

func (h *Handlers) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", h.withMiddleware(h.handleFunctionCall,
		h.methodMiddleware(http.MethodPost),
		h.errorMiddleware(),
	))
	return mux
}

// Handlers
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

	metadata, err := h.engine.reg.Get(req.Namespace, req.Name)
	if err != nil {
		if err == registry.ErrFunctionNotFound {
			return RequestError{
				Message:    "Function not found",
				StatusCode: http.StatusNotFound,
			}
		}
		return fmt.Errorf("failed to fetch function metadata: %w", err)
	}

	return h.writeJSONResponse(w, metadata)
}

func (h *Handlers) handleListAll(w http.ResponseWriter, _ *http.Request) error {
	h.logger.Printf("Received request to list all functions")

	functions, err := h.engine.reg.ListAll()
	if err != nil {
		return fmt.Errorf("failed to list functions: %w", err)
	}

	return h.writeJSONResponse(w, functions)
}

func (h *Handlers) handleBuild(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		FunctionRequest
		Path   string                    `json:"path"`
		Config manifest.FunctionManifest `json:"manifest"`
		Tag    string                    `json:"tag"`
	}

	if err := h.decodeJSONRequest(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received build request for function: %s/%s", req.Namespace, req.Name)

	if req.Path == "" {
		return RequestError{
			Message:    "Missing required field: path",
			StatusCode: http.StatusBadRequest,
		}
	}

	resp, err := h.engine.BuildFunction(req.Namespace, req.Name, req.Path, req.Tag, req.Config)
	if err != nil {
		return err
	}

	return h.writeJSONResponse(w, resp)
}

func (h *Handlers) handleFunctionCall(w http.ResponseWriter, r *http.Request) error {
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) != 3 {
		return RequestError{
			Message:    "Invalid URL format: expected /namespace/name/entrypoint",
			StatusCode: http.StatusBadRequest,
		}
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
			return RequestError{
				Message:    "Function not loaded",
				StatusCode: http.StatusNotFound,
			}
		}
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(output)
	return err
}

func (h *Handlers) handleOneOffCall(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		FunctionRequest
		Reference  string `json:"reference"`
		Entrypoint string `json:"entrypoint"`
		Payload    string `json:"payload"`
	}

	if err := h.decodeJSONRequest(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received one-off call request for function: %s/%s (reference: %s, entrypoint: %s)",
		req.Namespace, req.Name, req.Reference, req.Entrypoint)

	wasmBytes, _, err := h.engine.reg.Pull(req.Namespace, req.Name, req.Reference)
	if err != nil {
		return err
	}

	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
	}

	plugin, err := extism.NewPlugin(context.Background(), manifest, extism.PluginConfig{EnableWasi: true}, []extism.HostFunction{})
	if err != nil {
		return err
	}
	defer plugin.Close(context.TODO())

	_, output, err := plugin.Call(req.Entrypoint, []byte(req.Payload))
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(output)
	return err
}

func (h *Handlers) handleReassignTag(w http.ResponseWriter, r *http.Request) error {
	var req struct {
		FunctionRequest
		Tag    string `json:"tag"`
		Digest string `json:"digest"`
	}

	if err := h.decodeJSONRequest(r, &req); err != nil {
		return err
	}

	h.logger.Printf("Received reassign tag request for function: %s/%s (tag: %s, digest: %s)",
		req.Namespace, req.Name, req.Tag, req.Digest)

	if err := h.engine.ReassignTag(req.Namespace, req.Name, req.Tag, req.Digest); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return RequestError{
				Message:    err.Error(),
				StatusCode: http.StatusNotFound,
			}
		}
		return err
	}

	return h.writeJSONResponse(w, map[string]string{"message": "Tag reassigned successfully"})
}

func (h *Handlers) decodeJSONRequest(r *http.Request, v interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return RequestError{
			Message:    "Invalid request body",
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (h *Handlers) writeJSONResponse(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(data)
}

func (h *Handlers) decodeAndValidate(r *http.Request, v interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return NewBadRequestError("Invalid request body")
	}

	if err := h.validator.Struct(v); err != nil {
		return NewBadRequestError(fmt.Sprintf("Validation failed: %v", err))
	}

	return nil
}
