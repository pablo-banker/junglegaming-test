package config

import (
	"errors"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPAddr          string
	DatabaseURL       string
	ShutdownTimeout   time.Duration
	KeycloakIssuerURL string
	KeycloakJWKSURL   string
	KeycloakAudience  string
}

func Load() (Config, error) {
	_ = godotenv.Load()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL environment variable not set")
	}

	httpAddr := os.Getenv("HTTP_PORT")
	if httpAddr == "" {
		httpAddr = "8080"
	}

	keycloakIssuerURL := os.Getenv("KEYCLOAK_ISSUER_URL")
	if keycloakIssuerURL == "" {
		return Config{}, errors.New("KEYCLOAK_ISSUER_URL environment variable not set")
	}

	keycloakJWKSURL := os.Getenv("KEYCLOAK_JWKS_URL")
	if keycloakJWKSURL == "" {
		return Config{}, errors.New("KEYCLOAK_JWKS_URL environment variable not set")
	}

	keycloakAudience := os.Getenv("KEYCLOAK_AUDIENCE")
	if keycloakAudience == "" {
		return Config{}, errors.New("KEYCLOAK_AUDIENCE environment variable not set")
	}

	return Config{
		HTTPAddr:          ":" + httpAddr,
		DatabaseURL:       databaseURL,
		ShutdownTimeout:   10 * time.Second,
		KeycloakIssuerURL: keycloakIssuerURL,
		KeycloakJWKSURL:   keycloakJWKSURL,
		KeycloakAudience:  keycloakAudience,
	}, nil
}
