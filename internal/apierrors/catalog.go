package apierrors

import "net/http"

var (
	ErrDatabaseURL = New(ErrDatabaseURLCode, "DATABASE_URL environment variable is not set", http.StatusInternalServerError)
	ErrConnectDB   = New(ErrConnectDBCode, "error connecting to the database", http.StatusInternalServerError)
	ErrPingDB      = New(ErrPingDBCode, "database is unavailable", http.StatusServiceUnavailable)
	ErrInternal    = New(ErrInternalCode, "an unexpected error occurred", http.StatusInternalServerError)
)

var (
	ErrInvalidPayload   = New(ErrInvalidPayloadCode, "the request payload is invalid", http.StatusBadRequest)
	ErrValidation       = New(ErrValidationCode, "one or more fields are invalid", http.StatusUnprocessableEntity)
	ErrNotFound         = New(ErrNotFoundCode, "resource not found", http.StatusNotFound)
	ErrMethodNotAllowed = New(ErrMethodNotAllowedCode, "method not allowed", http.StatusMethodNotAllowed)
)
