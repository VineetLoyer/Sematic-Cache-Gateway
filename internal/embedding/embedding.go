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

const DefaultDimensions = 1536

var ErrEmbeddingFailed = errors.New("embedding generation failed")
var ErrInvalidDimensions = errors.New("embedding has invalid dimensions")

type EmbeddingService interface {
	Generate(ctx context.Context, text string) ([]float32, error)
}

type Config struct {
	APIEndpoint string
	APIKey      string
	ModelName   string
	Dimensions  int
	Timeout     time.Duration
}

// DefaultConfig returns a Config with sensible defaults for OpenAI.
func DefaultConfig(apiKey string) Config {
	return Config{
		APIEndpoint: "https://api.openai.com/v1/embeddings",
		APIKey:      apiKey,
		ModelName:   "text-embedding-ada-002",
		Dimensions:  DefaultDimensions,
		Timeout:     30 * time.Second,
	}
}

type embeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

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

type Service struct {
	config     Config
	httpClient *http.Client
}

// NewService creates a new embedding service with the given configuration.
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
		config:     cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

// Generate creates an embedding vector for the given text.
func (s *Service) Generate(ctx context.Context, text string) ([]float32, error) {
	if text == "" {
		return nil, fmt.Errorf("%w: empty input text", ErrEmbeddingFailed)
	}

	reqBody := embeddingRequest{Input: text, Model: s.config.ModelName}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to marshal request: %v", ErrEmbeddingFailed, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.config.APIEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %v", ErrEmbeddingFailed, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if s.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.config.APIKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: request failed: %v", ErrEmbeddingFailed, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read response: %v", ErrEmbeddingFailed, err)
	}

	var embResp embeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("%w: failed to parse response: %v", ErrEmbeddingFailed, err)
	}

	if embResp.Error != nil {
		return nil, fmt.Errorf("%w: API error: %s", ErrEmbeddingFailed, embResp.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: unexpected status code: %d", ErrEmbeddingFailed, resp.StatusCode)
	}
	if len(embResp.Data) == 0 {
		return nil, fmt.Errorf("%w: no embedding data in response", ErrEmbeddingFailed)
	}

	embedding := embResp.Data[0].Embedding
	if len(embedding) != s.config.Dimensions {
		return nil, fmt.Errorf("%w: expected %d dimensions, got %d", ErrInvalidDimensions, s.config.Dimensions, len(embedding))
	}
	return embedding, nil
}

// Dimensions returns the expected embedding vector size.
func (s *Service) Dimensions() int {
	return s.config.Dimensions
}
