package keycloak

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/pablo-banker/junglegaming-test/internal/auth"
	"github.com/pablo-banker/junglegaming-test/internal/config"
)

// Verifier validates JWT access tokens issued by Keycloak.
type Verifier struct {
	verifier *oidc.IDTokenVerifier
}

// NewVerifier creates a Keycloak token verifier.
func NewVerifier(cfg config.Config) *Verifier {
	keySet := oidc.NewRemoteKeySet(
		context.Background(),
		cfg.KeycloakJWKSURL,
	)

	tokenVerifier := oidc.NewVerifier(
		cfg.KeycloakIssuerURL,
		keySet,
		&oidc.Config{
			ClientID: cfg.KeycloakAudience,
		},
	)

	return &Verifier{
		verifier: tokenVerifier,
	}
}

// Verify validates an access token and returns its authenticated identity.
func (v *Verifier) Verify(
	ctx context.Context,
	rawToken string,
) (auth.Principal, error) {
	rawToken = strings.TrimSpace(rawToken)

	if rawToken == "" {
		return auth.Principal{}, auth.ErrInvalidToken
	}

	token, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return auth.Principal{}, fmt.Errorf(
			"%w: %v",
			auth.ErrInvalidToken,
			err,
		)
	}

	var claims Claims

	if err := token.Claims(&claims); err != nil {
		return auth.Principal{}, fmt.Errorf(
			"%w: failed to decode claims: %v",
			auth.ErrInvalidToken,
			err,
		)
	}

	if claims.Subject == "" {
		return auth.Principal{}, fmt.Errorf(
			"%w: missing subject",
			auth.ErrInvalidToken,
		)
	}

	if claims.AuthorizedParty == "" {
		return auth.Principal{}, fmt.Errorf(
			"%w: missing authorized party",
			auth.ErrInvalidToken,
		)
	}

	return auth.Principal{
		Subject:    claims.Subject,
		ClientID:   claims.AuthorizedParty,
		ProviderID: claims.ProviderID,
		Roles:      claims.RealmAccess.Roles,
	}, nil
}
