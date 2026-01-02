// Package embedding contains property-based tests for embedding service.
package embedding

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pgregory.net/rapid"
)

// **Feature: semantic-cache-gateway, Property 10: Embedding Dimensionality Consistency**
// **Validates: Requirements 2.2**
//
// For any input text, the generated embedding vector SHALL have exactly
// 1536 dimensions (or the configured dimension count).
func TestEmbeddingDimensionality_Consistency(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a random expected dimension count
		expectedDimensions := rapid.IntRange(128, 2048).Draw(t, "expectedDimensions")

		// Generate a random embedding vector with the expected dimensions
		embedding := make([]float32, expectedDimensions)
		for i := 0; i < expectedDimensions; i++ {
			embedding[i] = rapid.Float32().Draw(t, "embeddingValue")
		}

		// Create a mock server that returns the embedding
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := embeddingResponse{
				Data: []struct {
					Embedding []float32 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Embedding: embedding, Index: 0},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		// Create service with the expected dimensions
		cfg := Config{
			APIEndpoint: server.URL,
			APIKey:      "test-key",
			ModelName:   "test-model",
			Dimensions:  expectedDimensions,
		}
		svc := NewService(cfg)

		// Generate random input text (non-empty)
		inputText := rapid.StringMatching(`[a-zA-Z0-9 ]{1,100}`).Draw(t, "inputText")

		// Generate embedding
		result, err := svc.Generate(context.Background(), inputText)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}

		// Property: The returned embedding must have exactly the configured dimensions
		if len(result) != expectedDimensions {
			t.Fatalf("Embedding dimensionality mismatch: expected %d, got %d", expectedDimensions, len(result))
		}

		// Property: The Dimensions() method must return the configured value
		if svc.Dimensions() != expectedDimensions {
			t.Fatalf("Dimensions() mismatch: expected %d, got %d", expectedDimensions, svc.Dimensions())
		}
	})
}

// **Feature: semantic-cache-gateway, Property 10: Embedding Dimensionality Consistency**
// **Validates: Requirements 2.2**
//
// For any embedding with incorrect dimensions, the service SHALL reject it
// with an ErrInvalidDimensions error.
func TestEmbeddingDimensionality_RejectsIncorrectDimensions(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate expected dimensions
		expectedDimensions := rapid.IntRange(128, 2048).Draw(t, "expectedDimensions")

		// Generate actual dimensions that differ from expected
		// Either smaller or larger, but not equal
		actualDimensions := rapid.IntRange(1, 4096).Filter(func(d int) bool {
			return d != expectedDimensions
		}).Draw(t, "actualDimensions")

		// Generate a random embedding vector with incorrect dimensions
		embedding := make([]float32, actualDimensions)
		for i := 0; i < actualDimensions; i++ {
			embedding[i] = rapid.Float32().Draw(t, "embeddingValue")
		}

		// Create a mock server that returns the incorrectly-sized embedding
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := embeddingResponse{
				Data: []struct {
					Embedding []float32 `json:"embedding"`
					Index     int       `json:"index"`
				}{
					{Embedding: embedding, Index: 0},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		// Create service with the expected dimensions
		cfg := Config{
			APIEndpoint: server.URL,
			APIKey:      "test-key",
			ModelName:   "test-model",
			Dimensions:  expectedDimensions,
		}
		svc := NewService(cfg)

		// Generate random input text (non-empty)
		inputText := rapid.StringMatching(`[a-zA-Z0-9 ]{1,100}`).Draw(t, "inputText")

		// Generate embedding - should fail due to dimension mismatch
		_, err := svc.Generate(context.Background(), inputText)

		// Property: Service must reject embeddings with incorrect dimensions
		if err == nil {
			t.Fatalf("Expected error for dimension mismatch (expected %d, got %d), but got nil",
				expectedDimensions, actualDimensions)
		}
	})
}

// Test that default dimensions are applied correctly
func TestEmbeddingDimensionality_DefaultDimensions(t *testing.T) {
	// Create service with zero dimensions (should default to 1536)
	cfg := Config{
		APIEndpoint: "http://localhost",
		APIKey:      "test-key",
	}
	svc := NewService(cfg)

	// Property: Default dimensions should be 1536
	if svc.Dimensions() != DefaultDimensions {
		t.Fatalf("Default dimensions mismatch: expected %d, got %d", DefaultDimensions, svc.Dimensions())
	}
}
