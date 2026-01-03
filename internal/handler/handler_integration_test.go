// Package handler provides integration tests for the full request flow.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"semantic-cache-gateway/internal/cache"
	"semantic-cache-gateway/internal/logger"
	"semantic-cache-gateway/internal/middleware"
	"semantic-cache-gateway/internal/models"
)

// mockCacheService implements cache.CacheService for testing
type mockCacheService struct {
	exactMatchEntry   *cache.CacheEntry
	exactMatchErr     error
	similarEntry      *cache.CacheEntry
	similarScore      float64
	similarErr        error
	storedEntries     []*cache.CacheEntry
	checkExactCalled  bool
	searchSimilarCalled bool
}

func (m *mockCacheService) CheckExactMatch(ctx context.Context, queryHash string) (*cache.CacheEntry, error) {
	m.checkExactCalled = true
	return m.exactMatchEntry, m.exactMatchErr
}

func (m *mockCacheService) SearchSimilar(ctx context.Context, embedding []float32, threshold float64) (*cache.CacheEntry, float64, error) {
	m.searchSimilarCalled = true
	return m.similarEntry, m.similarScore, m.similarErr
}

func (m *mockCacheService) StoreAsync(entry *cache.CacheEntry) {
	m.storedEntries = append(m.storedEntries, entry)
}

func (m *mockCacheService) Close() error {
	return nil
}


// mockEmbeddingService implements embedding.EmbeddingService for testing
type mockEmbeddingService struct {
	embedding []float32
	err       error
	called    bool
}

func (m *mockEmbeddingService) Generate(ctx context.Context, text string) ([]float32, error) {
	m.called = true
	return m.embedding, m.err
}

// mockUpstreamProxy implements proxy.UpstreamProxy for testing
type mockUpstreamProxy struct {
	response *http.Response
	err      error
	called   bool
}

func (m *mockUpstreamProxy) Forward(ctx context.Context, req *http.Request) (*http.Response, error) {
	m.called = true
	return m.response, m.err
}

// createTestRequest creates a test HTTP request with a chat completion body
func createTestRequest(t *testing.T, messages []models.Message) *http.Request {
	t.Helper()
	body := models.ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: messages,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("failed to marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	
	// Apply body buffer middleware context
	ctx := middleware.SetBufferedBody(req.Context(), bodyBytes)
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return req.WithContext(ctx)
}

// createMockLLMResponse creates a mock LLM response
func createMockLLMResponse(content string) *http.Response {
	respBody := map[string]interface{}{
		"id":      "chatcmpl-test123",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": content,
				},
				"finish_reason": "stop",
			},
		},
	}
	bodyBytes, _ := json.Marshal(respBody)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		Header:     make(http.Header),
	}
}

// generateTestEmbedding creates a test embedding vector
func generateTestEmbedding() []float32 {
	embedding := make([]float32, 1536)
	for i := range embedding {
		embedding[i] = float32(i) * 0.001
	}
	return embedding
}


// TestIntegration_CacheHit_ExactMatch tests the cache hit scenario with exact hash match.
// Requirements: 1.1, 4.2, 4.3
func TestIntegration_CacheHit_ExactMatch(t *testing.T) {
	// Setup cached response
	cachedResponse := `{"id":"cached-123","choices":[{"message":{"content":"cached response"}}]}`
	
	mockCache := &mockCacheService{
		exactMatchEntry: &cache.CacheEntry{
			ID:          "cache:test-hash",
			QueryHash:   "sha256:testhash",
			QueryText:   "What is the weather?",
			LLMResponse: cachedResponse,
			CreatedAt:   time.Now().Unix(),
		},
	}
	mockEmbed := &mockEmbeddingService{
		embedding: generateTestEmbedding(),
	}
	mockProxy := &mockUpstreamProxy{}
	log := logger.New()

	handler := New(mockCache, mockEmbed, mockProxy, log, nil)

	// Create request
	req := createTestRequest(t, []models.Message{
		{Role: "user", Content: "What is the weather?"},
	})
	rr := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rr, req)

	// Verify cache hit response
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Verify X-Cache-Status header
	cacheStatus := rr.Header().Get("X-Cache-Status")
	if cacheStatus != "HIT" {
		t.Errorf("expected X-Cache-Status HIT, got %s", cacheStatus)
	}

	// Verify exact match was checked
	if !mockCache.checkExactCalled {
		t.Error("expected CheckExactMatch to be called")
	}

	// Verify embedding was NOT generated (exact match found)
	if mockEmbed.called {
		t.Error("embedding should not be generated for exact match")
	}

	// Verify upstream was NOT called
	if mockProxy.called {
		t.Error("upstream should not be called for cache hit")
	}

	// Verify response body matches cached response
	if !bytes.Equal(rr.Body.Bytes(), []byte(cachedResponse)) {
		t.Errorf("response body mismatch: got %s, want %s", rr.Body.String(), cachedResponse)
	}
}


// TestIntegration_CacheHit_SemanticMatch tests the cache hit scenario with vector similarity match.
// Requirements: 1.1, 4.2, 4.3
func TestIntegration_CacheHit_SemanticMatch(t *testing.T) {
	// Setup cached response for semantic match
	cachedResponse := `{"id":"semantic-123","choices":[{"message":{"content":"semantic cached response"}}]}`
	
	mockCache := &mockCacheService{
		exactMatchEntry: nil, // No exact match
		similarEntry: &cache.CacheEntry{
			ID:          "cache:similar-hash",
			QueryHash:   "sha256:similarhash",
			QueryText:   "What's the weather like?",
			LLMResponse: cachedResponse,
			CreatedAt:   time.Now().Unix(),
		},
		similarScore: 0.98, // Above 0.95 threshold
	}
	mockEmbed := &mockEmbeddingService{
		embedding: generateTestEmbedding(),
	}
	mockProxy := &mockUpstreamProxy{}
	log := logger.New()

	handler := New(mockCache, mockEmbed, mockProxy, log, nil)

	// Create request with semantically similar query
	req := createTestRequest(t, []models.Message{
		{Role: "user", Content: "What is the weather today?"},
	})
	rr := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rr, req)

	// Verify cache hit response
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Verify X-Cache-Status header
	cacheStatus := rr.Header().Get("X-Cache-Status")
	if cacheStatus != "HIT" {
		t.Errorf("expected X-Cache-Status HIT, got %s", cacheStatus)
	}

	// Verify exact match was checked first
	if !mockCache.checkExactCalled {
		t.Error("expected CheckExactMatch to be called")
	}

	// Verify embedding was generated (for vector search)
	if !mockEmbed.called {
		t.Error("expected embedding to be generated for semantic search")
	}

	// Verify vector search was performed
	if !mockCache.searchSimilarCalled {
		t.Error("expected SearchSimilar to be called")
	}

	// Verify upstream was NOT called
	if mockProxy.called {
		t.Error("upstream should not be called for cache hit")
	}

	// Verify response body matches cached response
	if !bytes.Equal(rr.Body.Bytes(), []byte(cachedResponse)) {
		t.Errorf("response body mismatch: got %s, want %s", rr.Body.String(), cachedResponse)
	}
}


// TestIntegration_CacheMiss tests the cache miss scenario where request is forwarded to upstream.
// Requirements: 1.1, 4.2, 4.3
func TestIntegration_CacheMiss(t *testing.T) {
	mockCache := &mockCacheService{
		exactMatchEntry: nil, // No exact match
		similarEntry:    nil, // No semantic match
		similarScore:    0.7, // Below threshold
	}
	mockEmbed := &mockEmbeddingService{
		embedding: generateTestEmbedding(),
	}
	
	upstreamResponse := createMockLLMResponse("This is the upstream response")
	mockProxy := &mockUpstreamProxy{
		response: upstreamResponse,
	}
	log := logger.New()

	handler := New(mockCache, mockEmbed, mockProxy, log, nil)

	// Create request
	req := createTestRequest(t, []models.Message{
		{Role: "user", Content: "Tell me something new"},
	})
	rr := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rr, req)

	// Verify successful response
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Verify X-Cache-Status header indicates miss
	cacheStatus := rr.Header().Get("X-Cache-Status")
	if cacheStatus != "MISS" {
		t.Errorf("expected X-Cache-Status MISS, got %s", cacheStatus)
	}

	// Verify exact match was checked
	if !mockCache.checkExactCalled {
		t.Error("expected CheckExactMatch to be called")
	}

	// Verify embedding was generated
	if !mockEmbed.called {
		t.Error("expected embedding to be generated")
	}

	// Verify vector search was performed
	if !mockCache.searchSimilarCalled {
		t.Error("expected SearchSimilar to be called")
	}

	// Verify upstream was called
	if !mockProxy.called {
		t.Error("expected upstream to be called for cache miss")
	}

	// Verify cache entry was queued for storage (async)
	// Give a small delay for async operation
	time.Sleep(10 * time.Millisecond)
	if len(mockCache.storedEntries) == 0 {
		t.Error("expected cache entry to be stored after cache miss")
	}
}


// TestIntegration_GracefulDegradation_RedisFailure tests graceful degradation when Redis fails.
// Requirements: 6.4
func TestIntegration_GracefulDegradation_RedisFailure(t *testing.T) {
	// Simulate Redis failure on exact match check
	mockCache := &mockCacheService{
		exactMatchErr: errors.New("redis connection refused"),
		similarErr:    errors.New("redis connection refused"),
	}
	mockEmbed := &mockEmbeddingService{
		embedding: generateTestEmbedding(),
	}
	
	upstreamResponse := createMockLLMResponse("Upstream response when Redis is down")
	mockProxy := &mockUpstreamProxy{
		response: upstreamResponse,
	}
	log := logger.New()

	handler := New(mockCache, mockEmbed, mockProxy, log, nil)

	// Create request
	req := createTestRequest(t, []models.Message{
		{Role: "user", Content: "What happens when Redis fails?"},
	})
	rr := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rr, req)

	// Verify request still succeeds (graceful degradation)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 (graceful degradation), got %d", rr.Code)
	}

	// Verify upstream was called as fallback
	if !mockProxy.called {
		t.Error("expected upstream to be called when Redis fails")
	}

	// Verify X-Cache-Status indicates miss (fallback behavior)
	cacheStatus := rr.Header().Get("X-Cache-Status")
	if cacheStatus != "MISS" {
		t.Errorf("expected X-Cache-Status MISS on Redis failure, got %s", cacheStatus)
	}
}

// TestIntegration_GracefulDegradation_EmbeddingFailure tests graceful degradation when embedding service fails.
// Requirements: 2.3
func TestIntegration_GracefulDegradation_EmbeddingFailure(t *testing.T) {
	mockCache := &mockCacheService{
		exactMatchEntry: nil, // No exact match
	}
	// Simulate embedding service failure
	mockEmbed := &mockEmbeddingService{
		err: errors.New("embedding API unavailable"),
	}
	
	upstreamResponse := createMockLLMResponse("Upstream response when embedding fails")
	mockProxy := &mockUpstreamProxy{
		response: upstreamResponse,
	}
	log := logger.New()

	handler := New(mockCache, mockEmbed, mockProxy, log, nil)

	// Create request
	req := createTestRequest(t, []models.Message{
		{Role: "user", Content: "What happens when embedding fails?"},
	})
	rr := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rr, req)

	// Verify request still succeeds (graceful degradation)
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 (graceful degradation), got %d", rr.Code)
	}

	// Verify embedding was attempted
	if !mockEmbed.called {
		t.Error("expected embedding generation to be attempted")
	}

	// Verify upstream was called as fallback
	if !mockProxy.called {
		t.Error("expected upstream to be called when embedding fails")
	}

	// Verify vector search was NOT called (no embedding available)
	if mockCache.searchSimilarCalled {
		t.Error("vector search should not be called when embedding fails")
	}
}


// TestIntegration_UpstreamError tests error handling when upstream LLM fails.
// Requirements: 1.4
func TestIntegration_UpstreamError(t *testing.T) {
	mockCache := &mockCacheService{
		exactMatchEntry: nil,
		similarEntry:    nil,
	}
	mockEmbed := &mockEmbeddingService{
		embedding: generateTestEmbedding(),
	}
	// Simulate upstream failure
	mockProxy := &mockUpstreamProxy{
		err: errors.New("upstream connection timeout"),
	}
	log := logger.New()

	handler := New(mockCache, mockEmbed, mockProxy, log, nil)

	// Create request
	req := createTestRequest(t, []models.Message{
		{Role: "user", Content: "What happens when upstream fails?"},
	})
	rr := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rr, req)

	// Verify error response
	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected status 502 (Bad Gateway), got %d", rr.Code)
	}

	// Verify error response format
	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if errResp.Error.Type != "upstream_error" {
		t.Errorf("expected error type 'upstream_error', got %s", errResp.Error.Type)
	}
}

// TestIntegration_InvalidRequest tests handling of invalid request body.
// Requirements: 7.3
func TestIntegration_InvalidRequest(t *testing.T) {
	mockCache := &mockCacheService{}
	mockEmbed := &mockEmbeddingService{}
	mockProxy := &mockUpstreamProxy{}
	log := logger.New()

	handler := New(mockCache, mockEmbed, mockProxy, log, nil)

	// Create request with invalid JSON (but valid enough to pass middleware)
	invalidBody := []byte(`{"model": "gpt-4", "messages": "not an array"}`)
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", bytes.NewReader(invalidBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := middleware.SetBufferedBody(req.Context(), invalidBody)
	req = req.WithContext(ctx)
	
	rr := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rr, req)

	// Verify error response
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 (Bad Request), got %d", rr.Code)
	}
}

// TestIntegration_NoUserMessages tests handling of request with no user messages.
// Requirements: 1.2
func TestIntegration_NoUserMessages(t *testing.T) {
	mockCache := &mockCacheService{}
	mockEmbed := &mockEmbeddingService{}
	mockProxy := &mockUpstreamProxy{}
	log := logger.New()

	handler := New(mockCache, mockEmbed, mockProxy, log, nil)

	// Create request with only system message (no user messages)
	req := createTestRequest(t, []models.Message{
		{Role: "system", Content: "You are a helpful assistant"},
	})
	rr := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rr, req)

	// Verify error response
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 (Bad Request), got %d", rr.Code)
	}

	// Verify error message
	var errResp struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}

	if !strings.Contains(errResp.Error.Message, "No user messages") {
		t.Errorf("expected error about no user messages, got: %s", errResp.Error.Message)
	}
}


// TestIntegration_CacheStorageOnMiss tests that cache entries are stored after cache miss.
// Requirements: 5.1, 5.2, 5.3
func TestIntegration_CacheStorageOnMiss(t *testing.T) {
	mockCache := &mockCacheService{
		exactMatchEntry: nil,
		similarEntry:    nil,
	}
	testEmbedding := generateTestEmbedding()
	mockEmbed := &mockEmbeddingService{
		embedding: testEmbedding,
	}
	
	upstreamResponse := createMockLLMResponse("Response to be cached")
	mockProxy := &mockUpstreamProxy{
		response: upstreamResponse,
	}
	log := logger.New()

	handler := New(mockCache, mockEmbed, mockProxy, log, nil)

	// Create request
	queryText := "What should be cached?"
	req := createTestRequest(t, []models.Message{
		{Role: "user", Content: queryText},
	})
	rr := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(rr, req)

	// Verify successful response
	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	// Wait for async storage
	time.Sleep(50 * time.Millisecond)

	// Verify cache entry was stored
	if len(mockCache.storedEntries) != 1 {
		t.Fatalf("expected 1 stored entry, got %d", len(mockCache.storedEntries))
	}

	storedEntry := mockCache.storedEntries[0]

	// Verify entry has required fields (Requirements 5.3)
	if storedEntry.QueryHash == "" {
		t.Error("stored entry missing query_hash")
	}
	if storedEntry.QueryText != queryText {
		t.Errorf("stored entry query_text mismatch: got %s, want %s", storedEntry.QueryText, queryText)
	}
	if len(storedEntry.Embedding) != len(testEmbedding) {
		t.Errorf("stored entry embedding length mismatch: got %d, want %d", len(storedEntry.Embedding), len(testEmbedding))
	}
	if len(storedEntry.LLMResponse) == 0 {
		t.Error("stored entry missing llm_response")
	}
	if storedEntry.CreatedAt == 0 {
		t.Error("stored entry missing created_at timestamp")
	}
}

// TestIntegration_SimilarityThreshold tests that similarity threshold is respected.
// Requirements: 4.2, 4.3
func TestIntegration_SimilarityThreshold(t *testing.T) {
	tests := []struct {
		name           string
		similarity     float64
		expectCacheHit bool
	}{
		{
			name:           "above threshold (0.96)",
			similarity:     0.96,
			expectCacheHit: true,
		},
		{
			name:           "at threshold (0.95)",
			similarity:     0.95,
			expectCacheHit: false, // Must be > 0.95, not >=
		},
		{
			name:           "below threshold (0.90)",
			similarity:     0.90,
			expectCacheHit: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cachedResponse := `{"id":"test","choices":[]}`
			
			var similarEntry *cache.CacheEntry
			if tt.expectCacheHit {
				similarEntry = &cache.CacheEntry{
					ID:          "cache:test",
					LLMResponse: cachedResponse,
				}
			}

			mockCache := &mockCacheService{
				exactMatchEntry: nil,
				similarEntry:    similarEntry,
				similarScore:    tt.similarity,
			}
			mockEmbed := &mockEmbeddingService{
				embedding: generateTestEmbedding(),
			}
			
			upstreamResponse := createMockLLMResponse("Upstream response")
			mockProxy := &mockUpstreamProxy{
				response: upstreamResponse,
			}
			log := logger.New()

			handler := New(mockCache, mockEmbed, mockProxy, log, nil)

			req := createTestRequest(t, []models.Message{
				{Role: "user", Content: "Test query"},
			})
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			cacheStatus := rr.Header().Get("X-Cache-Status")
			if tt.expectCacheHit {
				if cacheStatus != "HIT" {
					t.Errorf("expected cache HIT for similarity %.2f, got %s", tt.similarity, cacheStatus)
				}
				if mockProxy.called {
					t.Error("upstream should not be called for cache hit")
				}
			} else {
				if cacheStatus != "MISS" {
					t.Errorf("expected cache MISS for similarity %.2f, got %s", tt.similarity, cacheStatus)
				}
				if !mockProxy.called {
					t.Error("upstream should be called for cache miss")
				}
			}
		})
	}
}
