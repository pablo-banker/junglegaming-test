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
	AWSRegion         string
	SQSEndpoint       string
	SQSQueueURL       string
	SQSDLQURL         string
	SQSEventQueueURL  string
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

	return Config{
		HTTPAddr:          ":" + httpAddr,
		DatabaseURL:       databaseURL,
		ShutdownTimeout:   10 * time.Second,
		KeycloakIssuerURL: keycloakIssuerURL,
		KeycloakJWKSURL:   keycloakJWKSURL,
		KeycloakAudience:  keycloakAudience,
		AWSRegion:         awsRegion,
		SQSEndpoint:       sqsEndpoint,
		SQSQueueURL:       sqsQueueURL,
		SQSDLQURL:         sqsDLQURL,
		SQSEventQueueURL:  sqsEventQueueURL,
	}, nil
}
