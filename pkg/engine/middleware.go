package engine

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	domainerrors "github.com/ignitionstack/ignition/pkg/engine/errors"
)

type HandlerFunc func(http.ResponseWriter, *http.Request) error

type Middleware func(HandlerFunc) HandlerFunc

func (h *Handlers) withMiddleware(handler HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	for _, middleware := range middlewares {
		handler = middleware(handler)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if err := handler(w, r); err != nil {
			h.logger.Errorf("Unhandled error in handler: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

func (h *Handlers) methodMiddleware(method string) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			if r.Method != method {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return nil
			}
			return next(w, r)
		}
	}
}

func (h *Handlers) errorMiddleware() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			err := next(w, r)
			if err != nil {
				var reqErr RequestError

				// Handle different error types with proper conversion logic
				switch {
				case errors.As(err, &reqErr):
					// Already a RequestError, use as is

				case isDomainError(err):
					// Convert domain error to request error with appropriate status code
					var domainErr *domainerrors.DomainError
					errors.As(err, &domainErr)
					reqErr = DomainErrorToRequestError(domainErr)

				default:
					// Unknown error type, convert to internal server error
					reqErr = RequestError{
						Message:    err.Error(),
						StatusCode: http.StatusInternalServerError,
					}
				}

				// Log the error with context about the request
				h.logger.Errorf("Handler error (%s %s): %v", r.Method, r.URL.Path, err)

				// Send the error response to the client
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(reqErr.StatusCode)

				// Build response with error details
				response := map[string]interface{}{
					"error":  reqErr.Message,
					"status": reqErr.StatusCode,
				}

				// Add domain and code if available for better debugging
				var de *domainerrors.DomainError
				if errors.As(err, &de) {
					response["domain"] = string(de.ErrDomain)
					response["code"] = string(de.ErrCode)
				}

				if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
					h.logger.Errorf("Failed to encode error response: %v", encodeErr)
				}
			}
			return nil
		}
	}
}

// We use the isDomainError function from errors.go

func (h *Handlers) loggingMiddleware() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			start := time.Now()

			rw := newResponseWriter(w)

			err := next(rw, r)

			duration := time.Since(start)
			h.logger.Printf("%s %s %d %s", r.Method, r.URL.Path, rw.statusCode, duration)

			return err
		}
	}
}

func (h *Handlers) corsMiddleware() Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return nil
			}

			return next(w, r)
		}
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
