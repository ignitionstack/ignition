package engine

import "net/http"

type RequestError struct {
    Message    string
    StatusCode int
}

func (e RequestError) Error() string {
    return e.Message
}

func NewBadRequestError(message string) RequestError {
    return RequestError{
        Message:    message,
        StatusCode: http.StatusBadRequest,
    }
}

func NewNotFoundError(message string) RequestError {
    return RequestError{
        Message:    message,
        StatusCode: http.StatusNotFound,
    }
}
