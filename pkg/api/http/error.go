package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// Error type in API response
type errorType string

// Error response
type apiError struct {
	typ errorType
	err error
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s", e.typ, e.err)
}

// List of predefined errors
const (
	errorNone          errorType = ""
	errorUnauthorized  errorType = "unauthorized"
	errorForbidden     errorType = "forbidden"
	errorTimeout       errorType = "timeout"
	errorCanceled      errorType = "canceled"
	errorExec          errorType = "execution"
	errorBadData       errorType = "bad_data"
	errorInternal      errorType = "internal"
	errorUnavailable   errorType = "unavailable"
	errorNotFound      errorType = "not_found"
	errorNotAcceptable errorType = "not_acceptable"
)

// Custom error codes
const (
	// Non-standard status code (originally introduced by nginx) for the case when a client closes
	// the connection while the server is still processing the request.
	statusClientClosedConnection = 499
)

// Custom errors
var (
	errNoUser            = errors.New("no user identified")
	errNoPrivs           = errors.New("current user does not have admin privileges")
	errInvalidRequest    = errors.New("invalid request")
	errInvalidQueryField = errors.New("invalid query fields")
)

// Return error response for by setting errorString and errorType in response
func errorResponse[T any](w http.ResponseWriter, apiErr *apiError, logger log.Logger, data []T) {
	var code int
	switch apiErr.typ {
	case errorBadData:
		code = http.StatusBadRequest
	case errorUnauthorized:
		code = http.StatusUnauthorized
	case errorForbidden:
		code = http.StatusForbidden
	case errorExec:
		code = http.StatusUnprocessableEntity
	case errorCanceled:
		code = statusClientClosedConnection
	case errorTimeout:
		code = http.StatusServiceUnavailable
	case errorInternal:
		code = http.StatusInternalServerError
	case errorNotFound:
		code = http.StatusNotFound
	case errorNotAcceptable:
		code = http.StatusNotAcceptable
	default:
		code = http.StatusInternalServerError
	}

	w.WriteHeader(code)
	response := Response[T]{
		Status:    "error",
		ErrorType: apiErr.typ,
		Error:     apiErr.err.Error(),
		Data:      data,
	}
	if err := json.NewEncoder(w).Encode(&response); err != nil {
		level.Error(logger).Log("msg", "Failed to encode response", "err", err)
		w.Write([]byte("KO"))
	}
}
