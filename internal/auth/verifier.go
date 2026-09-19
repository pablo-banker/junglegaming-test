package auth

import "context"

// TokenVerifier validates an access token and returns its authenticated identity.
type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (Principal, error)
}
