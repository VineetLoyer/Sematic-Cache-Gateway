package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	UpstreamURL         string
	RedisURL            string
	SimilarityThreshold float64
	Port                int
	EmbeddingAPIKey     string
	UpstreamAPIKey      string
}

const (
	DefaultUpstreamURL         = "https://api.openai.com/v1"
	DefaultRedisURL            = "redis://localhost:6379"
	DefaultSimilarityThreshold = 0.95
	DefaultPort                = 8080
)

// Load reads configuration from environment variables with defaults.
func Load() (*Config, error) {
	cfg := &Config{
		UpstreamURL:         getEnvOrDefault("UPSTREAM_URL", DefaultUpstreamURL),
		RedisURL:            getEnvOrDefault("REDIS_URL", DefaultRedisURL),
		EmbeddingAPIKey:     os.Getenv("EMBEDDING_API_KEY"),
		UpstreamAPIKey:      os.Getenv("UPSTREAM_API_KEY"),
		SimilarityThreshold: DefaultSimilarityThreshold,
		Port:                DefaultPort,
	}

	if thresholdStr := os.Getenv("SIMILARITY_THRESHOLD"); thresholdStr != "" {
		threshold, err := strconv.ParseFloat(thresholdStr, 64)
		if err != nil {
			return nil, errors.New("SIMILARITY_THRESHOLD must be a valid float")
		}
		cfg.SimilarityThreshold = threshold
	}

	if portStr := os.Getenv("PORT"); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, errors.New("PORT must be a valid integer")
		}
		cfg.Port = port
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that all configuration values are within acceptable ranges.
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

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
