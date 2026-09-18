package apierrors

type APIError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
	Internal   string `json:"-"`
	HTTPStatus int    `json:"-"`

	cause error
}

func (e *APIError) Error() string {
	return e.Message
}

func (e *APIError) Unwrap() error {
	return e.cause
}

// WithInternal returns a copy of the API error with an internal message.
func (e *APIError) WithInternal(internalMsg string) *APIError {
	clone := *e
	clone.Internal = internalMsg

	return &clone
}

// WithDetails returns a copy of the API error with public details.
func (e *APIError) WithDetails(details string) *APIError {
	clone := *e
	clone.Details = details

	return &clone
}

// SetHTTPStatus returns a copy of the API error with a different HTTP status.
func (e *APIError) SetHTTPStatus(statusCode int) *APIError {
	clone := *e
	clone.HTTPStatus = statusCode

	return &clone
}

// WithCause returns a copy of the API error wrapping the original cause.
func (e *APIError) WithCause(err error) *APIError {
	clone := *e
	clone.cause = err

	if err != nil {
		clone.Internal = err.Error()
	}

	return &clone
}
