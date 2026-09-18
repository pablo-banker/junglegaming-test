package apierrors

const (
	ErrInfraBase = 100
)

const (
	ErrDatabaseURLCode = ErrInfraBase + iota
	ErrConnectDBCode
	ErrPingDBCode
	ErrInternalCode
)

const (
	ErrHTTPBase = 200
)

const (
	ErrInvalidPayloadCode = ErrHTTPBase + iota
	ErrValidationCode
	ErrNotFoundCode
	ErrMethodNotAllowedCode
)
