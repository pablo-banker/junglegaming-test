package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	LogLevel          slog.Level
	HTTPAddr          string
	DatabaseURL       string
	KeycloakIssuerURL string
	KeycloakJWKSURL   string
	KeycloakAudience  string
	AWSRegion         string
	SQSEndpoint       string
	SQSQueueURL       string
	SQSDLQURL         string
	SQSEventQueueURL  string

	// SQSAllowedProviders are the providers accepted in SQS messages.
	SQSAllowedProviders []string

	ReferenceRetryInitialDelay time.Duration
	ReferenceRetryMaxDelay     time.Duration
	ReferenceTTL               time.Duration
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

	awsRegion := os.Getenv("AWS_REGION")
	if awsRegion == "" {
		return Config{}, errors.New("AWS_REGION environment variable not set")
	}

	sqsEndpoint := os.Getenv("SQS_ENDPOINT")
	if sqsEndpoint == "" {
		return Config{}, errors.New("SQS_ENDPOINT environment variable not set")
	}

	sqsQueueURL := os.Getenv("SQS_WAGER_QUEUE_URL")
	if sqsQueueURL == "" {
		return Config{}, errors.New("SQS_WAGER_QUEUE_URL environment variable not set")
	}

	sqsDLQURL := os.Getenv("SQS_WAGER_DLQ_URL")
	if sqsDLQURL == "" {
		return Config{}, errors.New("SQS_WAGER_DLQ_URL environment variable not set")
	}

	sqsEventQueueURL := os.Getenv("SQS_EVENT_QUEUE_URL")
	if sqsEventQueueURL == "" {
		return Config{}, errors.New("SQS_EVENT_QUEUE_URL is required")
	}

	allowedProviders := splitList(os.Getenv("SQS_ALLOWED_PROVIDERS"))
	if len(allowedProviders) == 0 {
		return Config{}, errors.New("SQS_ALLOWED_PROVIDERS environment variable not set")
	}

	var logLevel slog.Level
	if err := logLevel.UnmarshalText([]byte(envOrDefault("LOG_LEVEL", "info"))); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL must be debug, info, warn or error: %w", err)
	}

	referenceRetryInitialDelay, err := durationEnv("REFERENCE_RETRY_INITIAL_DELAY", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	referenceRetryMaxDelay, err := durationEnv("REFERENCE_RETRY_MAX_DELAY", 5*time.Minute)
	if err != nil {
		return Config{}, err
	}

	referenceTTL, err := durationEnv("REFERENCE_TTL", 24*time.Hour)
	if err != nil {
		return Config{}, err
	}

	return Config{
		SQSAllowedProviders:        allowedProviders,
		LogLevel:                   logLevel,
		ReferenceRetryInitialDelay: referenceRetryInitialDelay,
		ReferenceRetryMaxDelay:     referenceRetryMaxDelay,
		ReferenceTTL:               referenceTTL,
		HTTPAddr:                   ":" + httpAddr,
		DatabaseURL:                databaseURL,
		KeycloakIssuerURL:          keycloakIssuerURL,
		KeycloakJWKSURL:            keycloakJWKSURL,
		KeycloakAudience:           keycloakAudience,
		AWSRegion:                  awsRegion,
		SQSEndpoint:                sqsEndpoint,
		SQSQueueURL:                sqsQueueURL,
		SQSDLQURL:                  sqsDLQURL,
		SQSEventQueueURL:           sqsEventQueueURL,
	}, nil
}

// durationEnv reads a positive duration such as "5s" or "24h", falling back to a default.
func durationEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", name, raw)
	}

	return value, nil
}

// envOrDefault returns the trimmed variable or a default when it is empty.
func envOrDefault(name string, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}

	return fallback
}

// splitList parses a comma separated list, ignoring blanks.
func splitList(raw string) []string {
	var values []string

	for value := range strings.SplitSeq(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}

	return values
}
