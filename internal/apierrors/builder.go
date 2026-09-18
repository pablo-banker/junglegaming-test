package apierrors

import "fmt"

const errorPrefix = "JG"

// New creates a reusable API error definition.
func New(code int, message string, httpStatus int) *APIError {
	return &APIError{
		Code:       formatCode(code),
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

func formatCode(code int) string {
	return fmt.Sprintf(
		"%s-%06d",
		errorPrefix,
		code,
	)
}
