package engine

import (
	"encoding/json"
	"net/http"
	"time"
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

				if e, ok := err.(RequestError); ok {
					reqErr = e
				} else {
					reqErr = RequestError{
						Message:    err.Error(),
						StatusCode: http.StatusInternalServerError,
					}
				}

				h.logger.Errorf("Handler error: %v", err)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(reqErr.StatusCode)

				if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
					"error":  reqErr.Message,
					"status": reqErr.StatusCode,
				}); encodeErr != nil {
					h.logger.Errorf("Failed to encode error response: %v", encodeErr)
				}
			}
			return nil
		}
	}
}

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
