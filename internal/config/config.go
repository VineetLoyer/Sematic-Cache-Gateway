 package config

import (
	"errors"
	"os"
	"strconv"
)

// Config holds all configuration values for the gateway
type Config struct {
	// UpstreamURL is the URL of the upstream LLM API (e.g., OpenAI)
	UpstreamURL string

	// RedisURL is the connection string for Redis Stack
	RedisURL string

	// SimilarityThreshold is the minimum cosine similarity score for cache hits (0.0-1.0)
	SimilarityThreshold float64

	// Port is the HTTP server port
	Port int

	// EmbeddingAPIKey is the API key for the embedding service
	EmbeddingAPIKey string
}

// Default configuration values
const (
	DefaultUpstreamURL         = "https://api.openai.com/v1"
	DefaultRedisURL            = "redis://localhost:6379"
	DefaultSimilarityThreshold = 0.95
	DefaultPort                = 8080
)

// Load reads configuration from environment variables with defaults
func Load() (*Config, error) {
	cfg := &Config{
		UpstreamURL:         getEnvOrDefault("UPSTREAM_URL", DefaultUpstreamURL),
		RedisURL:            getEnvOrDefault("REDIS_URL", DefaultRedisURL),
		EmbeddingAPIKey:     os.Getenv("EMBEDDING_API_KEY"),
		SimilarityThreshold: DefaultSimilarityThreshold,
		Port:                DefaultPort,
	}

	// Parse similarity threshold
	if thresholdStr := os.Getenv("SIMILARITY_THRESHOLD"); thresholdStr != "" {
		threshold, err := strconv.ParseFloat(thresholdStr, 64)
		if err != nil {
			return nil, errors.New("SIMILARITY_THRESHOLD must be a valid float")
		}
		cfg.SimilarityThreshold = threshold
	}

	// Parse port
	if portStr := os.Getenv("PORT"); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, errors.New("PORT must be a valid integer")
		}
		cfg.Port = port
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}


// Validate checks that the configuration values are valid
func (c *Config) Validate() error {
	if c.UpstreamURL == "" {
		return errors.New("UPSTREAM_URL is required")
	}

	if c.RedisURL == "" {
		return errors.New("REDIS_URL is required")
	}

	if c.SimilarityThreshold < 0.0 || c.SimilarityThreshold > 1.0 {
		return errors.New("SIMILARITY_THRESHOLD must be between 0.0 and 1.0")
	}

	if c.Port < 1 || c.Port > 65535 {
		return errors.New("PORT must be between 1 and 65535")
	}

	return nil
}

// getEnvOrDefault returns the environment variable value or a default
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
