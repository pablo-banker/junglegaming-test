package keycloak

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/pablo-banker/junglegaming-test/internal/auth"
	"github.com/pablo-banker/junglegaming-test/internal/config"
)

const (
	testIssuer   = "http://localhost:8081/realms/junglegaming"
	testAudience = "junglegaming-api"
	testKeyID    = "test-key"
)

// testIdP serves a JWKS and signs tokens like Keycloak.
type testIdP struct {
	key      *rsa.PrivateKey
	verifier *Verifier
}

// newTestIdP starts a JWKS endpoint and a verifier that trusts it.
func newTestIdP(t *testing.T) *testIdP {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	keySet := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key:       &key.PublicKey,
		KeyID:     testKeyID,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}}}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(keySet)
	}))
	t.Cleanup(server.Close)

	return &testIdP{
		key: key,
		verifier: NewVerifier(config.Config{
			KeycloakIssuerURL: testIssuer,
			KeycloakJWKSURL:   server.URL,
			KeycloakAudience:  testAudience,
		}),
	}
}

// accessClaims returns valid provider access token claims.
func accessClaims() map[string]any {
	now := time.Now()

	return map[string]any{
		"iss":          testIssuer,
		"aud":          []string{testAudience},
		"sub":          "service-account-provider-a",
		"azp":          "provider-a",
		"typ":          "Bearer",
		"provider_id":  "provider-a",
		"realm_access": map[string]any{"roles": []string{"provider"}},
		"iat":          now.Unix(),
		"exp":          now.Add(5 * time.Minute).Unix(),
	}
}

// sign serializes claims signed with the given key and algorithm.
func sign(t *testing.T, key any, algorithm jose.SignatureAlgorithm, claims map[string]any) string {
	t.Helper()

	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: algorithm, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", testKeyID),
	)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}

	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	return token
}

// TestVerifierAcceptsProviderAccessToken verifies a valid token becomes a principal.
func TestVerifierAcceptsProviderAccessToken(t *testing.T) {
	idp := newTestIdP(t)

	principal, err := idp.verifier.Verify(context.Background(), sign(t, idp.key, jose.RS256, accessClaims()))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if principal.ProviderID != "provider-a" || principal.ClientID != "provider-a" || !principal.IsProvider() {
		t.Fatalf("unexpected principal: %+v", principal)
	}
}

// TestVerifierRejectsInvalidTokens verifies expired, foreign and forged tokens are rejected.
func TestVerifierRejectsInvalidTokens(t *testing.T) {
	idp := newTestIdP(t)

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	with := func(change func(map[string]any)) map[string]any {
		claims := accessClaims()
		change(claims)

		return claims
	}

	tests := map[string]string{
		"expired": sign(t, idp.key, jose.RS256, with(func(c map[string]any) {
			c["exp"] = time.Now().Add(-time.Minute).Unix()
		})),
		"wrong audience": sign(t, idp.key, jose.RS256, with(func(c map[string]any) {
			c["aud"] = []string{"another-api"}
		})),
		"wrong issuer": sign(t, idp.key, jose.RS256, with(func(c map[string]any) {
			c["iss"] = "http://localhost:8081/realms/other"
		})),
		"id token": sign(t, idp.key, jose.RS256, with(func(c map[string]any) {
			c["typ"] = "ID"
		})),
		"missing authorized party": sign(t, idp.key, jose.RS256, with(func(c map[string]any) {
			delete(c, "azp")
		})),
		"unknown signing key": sign(t, otherKey, jose.RS256, accessClaims()),
		"symmetric algorithm": sign(t, []byte("a-shared-secret-of-at-least-32-bytes"), jose.HS256, accessClaims()),
		"malformed":           "not-a-jwt",
		"empty":               "",
	}

	for name, token := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := idp.verifier.Verify(context.Background(), token); !errors.Is(err, auth.ErrInvalidToken) {
				t.Fatalf("expected ErrInvalidToken, got %v", err)
			}
		})
	}
}
