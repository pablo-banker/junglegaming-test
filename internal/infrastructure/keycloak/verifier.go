package keycloak

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"github.com/pablo-banker/junglegaming-test/internal/auth"
	"github.com/pablo-banker/junglegaming-test/internal/config"
)

// Verifier validates JWT access tokens issued by Keycloak.
type Verifier struct {
	verifier *oidc.IDTokenVerifier
}

// jwksTimeout bounds each download of the signing keys, so an unreachable IdP fails fast.
const jwksTimeout = 5 * time.Second

// NewVerifier creates a Keycloak token verifier. Signature, issuer, audience and expiry
// are checked by go-oidc; keys are cached and refreshed when an unknown key id appears.
func NewVerifier(cfg config.Config) *Verifier {
	keySet := oidc.NewRemoteKeySet(
		oidc.ClientContext(context.Background(), &http.Client{Timeout: jwksTimeout}),
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

	// Keycloak marks access tokens as Bearer; ID and refresh tokens are not accepted.
	if claims.Type != "Bearer" {
		return auth.Principal{}, fmt.Errorf(
			"%w: token type %q is not an access token",
			auth.ErrInvalidToken,
			claims.Type,
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
