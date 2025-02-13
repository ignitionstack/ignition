package engine

import "net/http"

type HandlerFunc func(http.ResponseWriter, *http.Request) error

type Middleware func(HandlerFunc) HandlerFunc

func (h *Handlers) withMiddleware(handler HandlerFunc, middlewares ...Middleware) http.HandlerFunc {
	for _, m := range middlewares {
		handler = m(handler)
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
			if err := next(w, r); err != nil {
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
				http.Error(w, reqErr.Message, reqErr.StatusCode)
			}
			return nil
		}
	}
}
