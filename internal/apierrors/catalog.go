package apierrors

import "net/http"

var (
	ErrPingDB         = New(ErrPingDBCode, "database is unavailable", http.StatusServiceUnavailable)
	ErrInternal       = New(ErrInternalCode, "an unexpected error occurred", http.StatusInternalServerError)
	ErrSQSUnavailable = New(ErrSQSUnavailableCode, "service unavailable", http.StatusServiceUnavailable)
)

var (
	ErrInvalidPayload   = New(ErrInvalidPayloadCode, "the request payload is invalid", http.StatusBadRequest)
	ErrValidation       = New(ErrValidationCode, "one or more fields are invalid", http.StatusUnprocessableEntity)
	ErrUnauthorized     = New(ErrUnauthorizedCode, "authentication is required", http.StatusUnauthorized)
	ErrForbidden        = New(ErrForbiddenCode, "access denied", http.StatusForbidden)
	ErrNotFound         = New(ErrNotFoundCode, "resource not found", http.StatusNotFound)
	ErrMethodNotAllowed = New(ErrMethodNotAllowedCode, "method not allowed", http.StatusMethodNotAllowed)
	ErrConflict         = New(ErrConflictCode, "resource already exists", http.StatusConflict)
)
