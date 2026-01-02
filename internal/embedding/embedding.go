package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultDimensions is the expected embedding vector size for text-embedding-ada-002
const DefaultDimensions = 1536

// ErrEmbeddingFailed indicates the embedding generation failed
var ErrEmbeddingFailed = errors.New("embedding generation failed")

// ErrInvalidDimensions indicates the embedding has unexpected dimensions
var ErrInvalidDimensions = errors.New("embedding has invalid dimensions")

// EmbeddingService defines the interface for generating embeddings
type EmbeddingService interface {
	// Generate creates an embedding vector for the given text
	Generate(ctx context.Context, text string) ([]float32, error)
}

// Config holds configuration for the embedding service
type Config struct {
	// APIEndpoint is the URL for the embedding API
	APIEndpoint string
	// APIKey is the authentication key for the API
	APIKey string
	// ModelName is the embedding model to use
	ModelName string
	// Dimensions is the expected embedding vector size
	Dimensions int
	// Timeout for API requests
	Timeout time.Duration
}

// DefaultConfig returns a Config with sensible defaults for OpenAI
func DefaultConfig(apiKey string) Config {
	return Config{
		APIEndpoint: "https://api.openai.com/v1/embeddings",
		APIKey:      apiKey,
		ModelName:   "text-embedding-ada-002",
		Dimensions:  DefaultDimensions,
		Timeout:     30 * time.Second,
	}
}

// embeddingRequest is the request body for the OpenAI embeddings API
type embeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

// embeddingResponse is the response from the OpenAI embeddings API
type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}


// Service implements EmbeddingService using an HTTP API
type Service struct {
	config     Config
	httpClient *http.Client
}

// NewService creates a new embedding service with the given configuration
func NewService(cfg Config) *Service {
	if cfg.Dimensions == 0 {
		cfg.Dimensions = DefaultDimensions
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.ModelName == "" {
		cfg.ModelName = "text-embedding-ada-002"
	}
	if cfg.APIEndpoint == "" {
		cfg.APIEndpoint = "https://api.openai.com/v1/embeddings"
	}

	return &Service{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Generate creates an embedding vector for the given text
func (s *Service) Generate(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("%w: empty input text", ErrEmbeddingFailed)
	}

	// Build request body
	reqBody := embeddingRequest{
		Input: text,
		Model: s.config.ModelName,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal request: %v", ErrEmbeddingFailed, err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.APIEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %v", ErrEmbeddingFailed, err)
	}

	req.Header.Set("Content-Type", "application/json")
	if s.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.config.APIKey)
	}

	// Execute request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: request failed: %v", ErrEmbeddingFailed, err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response: %v", ErrEmbeddingFailed, err)
	}

	// Parse response
	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse response: %v", ErrEmbeddingFailed, err)
	}

	// Check for API error
	if embResp.Error != nil {
		return nil, fmt.Errorf("%w: API error: %s", ErrEmbeddingFailed, embResp.Error.Message)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status code: %d", ErrEmbeddingFailed, resp.StatusCode)
	}

	// Validate response data
	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("%w: no embedding data in response", ErrEmbeddingFailed)
	}

	embedding := embResp.Data[0].Embedding

	// Validate dimensionality
	if len(embedding) != s.config.Dimensions {
		return nil, fmt.Errorf("%w: expected %d dimensions, got %d", ErrInvalidDimensions, s.config.Dimensions, len(embedding))
	}

	return embedding, nil
}

// Dimensions returns the expected embedding vector size
func (s *Service) Dimensions() int {
	return s.config.Dimensions
}
